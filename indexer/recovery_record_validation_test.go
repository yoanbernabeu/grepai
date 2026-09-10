package indexer

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

type mutatingRecoveryStore struct {
	*store.GOBStore
	once        sync.Once
	mutate      func() error
	mutationErr error
	getCalls    int
}

func (s *mutatingRecoveryStore) GetDocument(ctx context.Context, path string) (*store.Document, error) {
	s.getCalls++
	s.once.Do(func() { s.mutationErr = s.mutate() })
	if s.mutationErr != nil {
		return nil, s.mutationErr
	}
	return s.GOBStore.GetDocument(ctx, path)
}

func (s *mutatingRecoveryStore) GetCompleteDocument(ctx context.Context, path string) (*store.Document, error) {
	s.getCalls++
	s.once.Do(func() { s.mutationErr = s.mutate() })
	if s.mutationErr != nil {
		return nil, s.mutationErr
	}
	return s.GOBStore.GetCompleteDocument(ctx, path)
}

func setupMutatingRecovery(t *testing.T) (context.Context, string, string, *Scanner, *store.GOBStore, *FileInfo) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	target := t.TempDir()
	relative := filepath.Join("linked", "a.go")
	absolute := filepath.Join(target, "a.go")
	if err := os.WriteFile(absolute, []byte("package old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(root, ignore)
	initial, err := scanner.ScanFile(relative)
	if err != nil || initial == nil {
		t.Fatalf("initial=%v err=%v", initial, err)
	}
	base := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	if err := base.SaveChunks(ctx, []store.Chunk{{ID: "old", FilePath: relative, Vector: []float32{1, 2, 3}}}); err != nil {
		t.Fatal(err)
	}
	if err := base.SaveDocument(ctx, store.Document{Path: relative, Hash: initial.Hash, ModTime: initial.ObservedModTime, HasExactModTime: true, ChunkIDs: []string{"old"}}); err != nil {
		t.Fatal(err)
	}
	return ctx, root, absolute, scanner, base, initial
}

func TestRecoveredExactMetadataRecreatesDeletedCurrentRecord(t *testing.T) {
	ctx, root, _, scanner, base, _ := setupMutatingRecovery(t)
	relative := filepath.Join("linked", "a.go")
	st := &mutatingRecoveryStore{GOBStore: base}
	st.mutate = func() error {
		if err := base.DeleteByFile(ctx, relative); err != nil {
			return err
		}
		return base.DeleteDocument(ctx, relative)
	}
	embedder := newMockEmbedder()
	stats, err := NewIndexer(root, st, embedder, NewChunker(512, 50), scanner, time.Now()).IndexAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.getCalls == 0 || stats.FilesIndexed != 1 || !embedder.embedCalled {
		t.Fatalf("getCalls=%d indexed=%d embedCalled=%v", st.getCalls, stats.FilesIndexed, embedder.embedCalled)
	}
	doc, err := base.GetDocument(ctx, relative)
	if err != nil || doc == nil || len(doc.ChunkIDs) == 0 {
		t.Fatalf("recreated document=%+v err=%v", doc, err)
	}
}

func TestRecoveredExactMetadataRecreatesChunkOnlyDeletedRecord(t *testing.T) {
	ctx, root, _, scanner, base, _ := setupMutatingRecovery(t)
	relative := filepath.Join("linked", "a.go")
	st := &mutatingRecoveryStore{GOBStore: base, mutate: func() error {
		return base.DeleteByFile(ctx, relative)
	}}
	embedder := newMockEmbedder()
	stats, err := NewIndexer(root, st, embedder, NewChunker(512, 50), scanner, time.Now()).IndexAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesIndexed != 1 || !embedder.embedCalled {
		t.Fatalf("indexed=%d embedCalled=%v", stats.FilesIndexed, embedder.embedCalled)
	}
	doc, err := base.GetCompleteDocument(ctx, relative)
	if err != nil || doc == nil || len(doc.ChunkIDs) == 0 {
		t.Fatalf("complete document=%+v err=%v", doc, err)
	}
}

func TestRecoveredExactMetadataUsesConcurrentRecordAndLatestSource(t *testing.T) {
	ctx, root, absolute, scanner, base, _ := setupMutatingRecovery(t)
	relative := filepath.Join("linked", "a.go")
	st := &mutatingRecoveryStore{GOBStore: base}
	st.mutate = func() error {
		if err := os.WriteFile(absolute, []byte("package latest\n"), 0o644); err != nil {
			return err
		}
		latest, err := scanner.ScanFile(relative)
		if err != nil {
			return err
		}
		if err := base.DeleteByFile(ctx, relative); err != nil {
			return err
		}
		if err := base.SaveChunks(ctx, []store.Chunk{{ID: "latest", FilePath: relative, Vector: []float32{4, 5, 6}}}); err != nil {
			return err
		}
		return base.SaveDocument(ctx, store.Document{Path: relative, Hash: latest.Hash, ModTime: latest.ObservedModTime, HasExactModTime: true, ChunkIDs: []string{"latest"}})
	}
	embedder := newMockEmbedder()
	stats, err := NewIndexer(root, st, embedder, NewChunker(512, 50), scanner, time.Now()).IndexAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := scanner.ScanFile(relative)
	if err != nil {
		t.Fatal(err)
	}
	verified := stats.VerifiedUnchangedFiles[relative]
	if st.getCalls == 0 || verified.Hash != latest.Hash || verified.Size != latest.Size || !verified.ModTime.Equal(latest.ObservedModTime) {
		t.Fatalf("getCalls=%d verified=%+v latest=%+v", st.getCalls, verified, latest)
	}
	if stats.FilesIndexed != 0 || embedder.embedCalled {
		t.Fatalf("indexed=%d embedCalled=%v", stats.FilesIndexed, embedder.embedCalled)
	}
	doc, err := base.GetDocument(ctx, relative)
	if err != nil || doc == nil || doc.Hash != latest.Hash || len(doc.ChunkIDs) != 1 || doc.ChunkIDs[0] != "latest" {
		t.Fatalf("document=%+v err=%v", doc, err)
	}
}
