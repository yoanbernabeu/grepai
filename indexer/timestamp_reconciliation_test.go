package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

type conflictRefreshStore struct {
	*mockStore
	path       string
	newContent string
	mode       string
	calls      int
}

func (s *conflictRefreshStore) RefreshDocumentModTime(_ context.Context, path, expectedHash string, modTime time.Time) (bool, error) {
	s.calls++
	if s.calls == 1 {
		if err := os.WriteFile(s.path, []byte(s.newContent), 0o644); err != nil {
			return false, err
		}
		hash := sha256.Sum256([]byte(s.newContent))
		s.mu.Lock()
		switch s.mode {
		case "missing":
			delete(s.documents, path)
		case "zero":
			s.documents[path] = store.Document{Path: path, Hash: hex.EncodeToString(hash[:])}
		default:
			s.documents[path] = store.Document{Path: path, Hash: hex.EncodeToString(hash[:]), ChunkIDs: []string{"new-chunk"}}
		}
		s.mu.Unlock()
		return false, nil
	}
	s.mu.Lock()
	doc := s.documents[path]
	defer s.mu.Unlock()
	if doc.Hash != expectedHash || len(doc.ChunkIDs) == 0 {
		return false, nil
	}
	doc.ModTime, doc.HasExactModTime = modTime, true
	s.documents[path] = doc
	return true, nil
}

func timestampTestScanner(t *testing.T, root string) *Scanner {
	t.Helper()
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	return NewScanner(root, ignore)
}

// decideFileScan is a test-only adapter for exercising the point-read conflict
// path directly; production reconciliation supplies the same metadata in bulk.
func (idx *Indexer) decideFileScan(ctx context.Context, fileMeta FileMeta) (fileScanDecision, error) {
	doc, err := idx.store.GetDocument(ctx, fileMeta.Path)
	if err != nil {
		return fileScanDecision{}, err
	}
	var existing *store.DocumentMetadata
	if doc != nil {
		existing = &store.DocumentMetadata{Path: fileMeta.Path, Hash: doc.Hash, HasChunks: len(doc.ChunkIDs) > 0, ModTime: doc.ModTime, HasExactModTime: doc.HasExactModTime}
	}
	return idx.decideFileScanFromMeta(ctx, fileMeta, existing)
}

func TestLegacyTimestampBackfillsOnceThenSkipsContent(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	if err := os.WriteFile(path, []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wholeSecond := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(path, wholeSecond, wholeSecond); err != nil {
		t.Fatal(err)
	}
	scanner := timestampTestScanner(t, root)
	snapshot, err := scanner.ScanFile("a.go")
	if err != nil {
		t.Fatal(err)
	}
	st := store.NewGOBStore(filepath.Join(root, "index.gob"))
	if err := st.SaveDocument(ctx, store.Document{Path: "a.go", Hash: snapshot.Hash, ChunkIDs: []string{"c1"}}); err != nil {
		t.Fatal(err)
	}
	reads := 0
	scanner.readSnapshot = func(path, rel string) (*FileInfo, error) {
		reads++
		return readFileSnapshot(path, rel)
	}
	idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), scanner, time.Now().Add(time.Hour))
	if _, err := idx.IndexAll(ctx); err != nil {
		t.Fatal(err)
	}
	if reads != 1 {
		t.Fatalf("first scan content reads = %d, want 1", reads)
	}
	doc, _ := st.GetDocument(ctx, "a.go")
	if !doc.HasExactModTime || !doc.ModTime.Equal(snapshot.ObservedModTime) {
		t.Fatalf("legacy timestamp was not refreshed: %+v", doc)
	}
	if _, err := idx.IndexAll(ctx); err != nil {
		t.Fatal(err)
	}
	if reads != 1 {
		t.Fatalf("second scan performed a content read; total = %d", reads)
	}
}

func TestSameSecondDifferentNanosecondsRequiresHashRead(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	if err := os.WriteFile(path, []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := time.Unix(1_700_000_000, 100)
	second := time.Unix(1_700_000_000, 200)
	if err := os.Chtimes(path, first, first); err != nil {
		t.Fatal(err)
	}
	scanner := timestampTestScanner(t, root)
	snapshot, _ := scanner.ScanFile("a.go")
	st := store.NewGOBStore(filepath.Join(root, "index.gob"))
	if err := st.SaveDocument(ctx, store.Document{Path: "a.go", Hash: snapshot.Hash, ModTime: first, HasExactModTime: true, ChunkIDs: []string{"c1"}}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, second, second); err != nil {
		t.Fatal(err)
	}
	reads := 0
	scanner.readSnapshot = func(path, rel string) (*FileInfo, error) {
		reads++
		return readFileSnapshot(path, rel)
	}
	idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), scanner, time.Time{})
	if _, err := idx.IndexAll(ctx); err != nil {
		t.Fatal(err)
	}
	if reads != 1 {
		t.Fatalf("content reads = %d, want 1", reads)
	}
	doc, _ := st.GetDocument(ctx, "a.go")
	if !doc.ModTime.Equal(second) {
		t.Fatalf("refreshed timestamp = %v, want %v", doc.ModTime, second)
	}
}

func TestRefreshConflictDiscardsStaleSourceSnapshot(t *testing.T) {
	for _, mode := range []string{"matching", "missing", "zero"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "a.go")
			oldContent, newContent := "package old\n", "package newer\n"
			if err := os.WriteFile(path, []byte(oldContent), 0o644); err != nil {
				t.Fatal(err)
			}
			scanner := timestampTestScanner(t, root)
			old, err := scanner.ScanFile("a.go")
			if err != nil {
				t.Fatal(err)
			}
			st := &conflictRefreshStore{mockStore: newMockStore(), path: path, newContent: newContent, mode: mode}
			st.documents["a.go"] = store.Document{Path: "a.go", Hash: old.Hash, ChunkIDs: []string{"old-chunk"}}
			idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), scanner, time.Time{})
			decision, err := idx.decideFileScan(context.Background(), FileMeta{Path: "a.go"})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "matching" {
				if decision.file != nil || decision.verified == nil {
					t.Fatalf("matching newer state decision = %+v", decision)
				}
				doc, err := st.GetDocument(context.Background(), "a.go")
				if err != nil || doc == nil || doc.Hash != decision.verified.Hash || len(doc.ChunkIDs) != 1 || doc.ChunkIDs[0] != "new-chunk" {
					t.Fatalf("newer indexed state was overwritten: doc=%+v err=%v", doc, err)
				}
				return
			}
			if decision.file == nil || decision.file.Content != newContent {
				t.Fatalf("decision retained stale snapshot: %+v", decision.file)
			}
		})
	}
}

func TestZeroLastIndexTimeForcesHashVerification(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	oldContent := "package a\nfunc A() {}\n"
	newContent := "package a\nfunc B() {}\n"
	if len(oldContent) != len(newContent) {
		t.Fatal("fixture contents must have equal size")
	}
	if err := os.WriteFile(path, []byte(oldContent), 0o644); err != nil {
		t.Fatal(err)
	}
	scanner := timestampTestScanner(t, root)
	old, err := scanner.ScanFile("a.go")
	if err != nil {
		t.Fatal(err)
	}
	st := store.NewGOBStore(filepath.Join(root, "index.gob"))
	if err := st.SaveDocument(ctx, store.Document{Path: "a.go", Hash: old.Hash, ModTime: old.ObservedModTime, HasExactModTime: true, ChunkIDs: []string{"old-chunk"}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, old.ObservedModTime, old.ObservedModTime); err != nil {
		t.Fatal(err)
	}
	idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), scanner, time.Time{})
	stats, err := idx.IndexAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesIndexed != 1 {
		t.Fatalf("indexed = %d, want changed content reindexed", stats.FilesIndexed)
	}
	doc, err := st.GetDocument(ctx, "a.go")
	if err != nil || doc == nil || doc.Hash == old.Hash {
		t.Fatalf("document hash was not refreshed: doc=%+v err=%v", doc, err)
	}
}
