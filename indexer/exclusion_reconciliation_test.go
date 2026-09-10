package indexer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

func writeExclusionFixtures(t *testing.T, root string) []string {
	t.Helper()
	files := map[string][]byte{
		"ignored.go":      []byte("package ignored\n"),
		"unsupported.xyz": []byte("unsupported\n"),
		"bundle.min.js":   []byte("const minified = true\n"),
		"binary.go":       {'p', 'a', 'c', 'k', 'a', 'g', 'e', 0, 'x'},
		"unreadable.go":   []byte("package unreadable\n"),
	}
	paths := make([]string, 0, len(files)+1)
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(root, path), content, 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	if err := os.WriteFile(filepath.Join(root, "large.go"), []byte(strings.Repeat("x", maxFileSize+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	return append(paths, "large.go")
}

func TestIndexAllPurgesPolicyExclusionsButPreservesReadErrors(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	paths := writeExclusionFixtures(t, root)
	ignore, err := NewIgnoreMatcher(root, []string{"ignored.go"}, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(root, ignore)
	scanner.readSnapshot = func(path, relPath string) (*FileInfo, error) {
		if relPath == "unreadable.go" {
			return nil, errors.New("simulated read failure")
		}
		return readFileSnapshot(path, relPath)
	}
	st := store.NewGOBStore(filepath.Join(root, "index.gob"))
	for _, path := range paths {
		if err := st.SaveDocument(ctx, store.Document{Path: path, Hash: "old", ChunkIDs: []string{"chunk"}}); err != nil {
			t.Fatal(err)
		}
	}
	idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), scanner, time.Time{})
	if _, err := idx.IndexAll(ctx); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"ignored.go", "unsupported.xyz", "bundle.min.js", "large.go", "binary.go"} {
		if doc, err := st.GetDocument(ctx, path); err != nil || doc != nil {
			t.Errorf("excluded %s retained: doc=%v err=%v", path, doc, err)
		}
	}
	if doc, err := st.GetDocument(ctx, "unreadable.go"); err != nil || doc == nil {
		t.Fatalf("read-error document removed: doc=%v err=%v", doc, err)
	}
}

func TestExcludedFileBecomingEligibleBeforeCleanupIsReindexed(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "binary.go")
	if err := os.WriteFile(path, []byte{'p', 'k', 'g', 0}, 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(root, ignore)
	reads := 0
	scanner.readSnapshot = func(snapshotPath, relPath string) (*FileInfo, error) {
		reads++
		result, err := readFileSnapshot(snapshotPath, relPath)
		if reads == 2 {
			if writeErr := os.WriteFile(path, []byte("package current\nfunc Current() {}\n"), 0o644); writeErr != nil {
				t.Fatal(writeErr)
			}
		}
		return result, err
	}
	st := store.NewGOBStore(filepath.Join(root, "index.gob"))
	if err := st.SaveDocument(ctx, store.Document{Path: "binary.go", Hash: "stale", ChunkIDs: []string{"old-chunk"}}); err != nil {
		t.Fatal(err)
	}
	idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), scanner, time.Time{})
	stats, err := idx.IndexAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesIndexed != 1 || stats.FilesRemoved != 0 {
		t.Fatalf("indexed=%d removed=%d", stats.FilesIndexed, stats.FilesRemoved)
	}
	doc, err := st.GetDocument(ctx, "binary.go")
	if err != nil || doc == nil || doc.Hash == "stale" {
		t.Fatalf("re-eligible file not refreshed: doc=%v err=%v", doc, err)
	}
}

func TestExcludedFileRevalidationReadErrorPreservesIndex(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "binary.go")
	if err := os.WriteFile(path, []byte{'p', 'k', 'g', 0}, 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(root, ignore)
	reads := 0
	scanner.readSnapshot = func(snapshotPath, relPath string) (*FileInfo, error) {
		reads++
		if reads >= 3 {
			return nil, errors.New("simulated revalidation read failure")
		}
		return readFileSnapshot(snapshotPath, relPath)
	}
	st := store.NewGOBStore(filepath.Join(root, "index.gob"))
	if err := st.SaveDocument(ctx, store.Document{Path: "binary.go", Hash: "stale", ChunkIDs: []string{"old-chunk"}}); err != nil {
		t.Fatal(err)
	}
	idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), scanner, time.Time{})
	stats, err := idx.IndexAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesIndexed != 0 || stats.FilesRemoved != 0 {
		t.Fatalf("indexed=%d removed=%d", stats.FilesIndexed, stats.FilesRemoved)
	}
	if doc, err := st.GetDocument(ctx, "binary.go"); err != nil || doc == nil || doc.Hash != "stale" {
		t.Fatalf("uncertain file index changed: doc=%v err=%v", doc, err)
	}
}
