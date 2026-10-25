package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIndexAll_RestartKeepsUnchangedDocuments(t *testing.T) {
	tmpDir := t.TempDir()
	nested := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "main.go"), []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	mockStore := newMockStore()
	mockEmb := newMockBatchEmbedder()
	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(tmpDir, ignoreMatcher)
	chunker := NewChunker(512, 50)
	idx := NewIndexer(tmpDir, mockStore, mockEmb, chunker, scanner, time.Time{})

	first, err := idx.IndexAllWithBatchProgress(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.FilesRemoved != 0 {
		t.Fatalf("first pass files removed = %d, want 0", first.FilesRemoved)
	}
	docsAfterFirst, err := mockStore.ListDocuments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(docsAfterFirst) == 0 {
		t.Fatal("expected indexed documents")
	}

	second, err := idx.IndexAllWithBatchProgress(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.FilesRemoved != 0 {
		t.Fatalf("restart files removed = %d, want 0 (orphan-map slash regression)", second.FilesRemoved)
	}
	docsAfterSecond, err := mockStore.ListDocuments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(docsAfterSecond) != len(docsAfterFirst) {
		t.Fatalf("document count %d -> %d after restart", len(docsAfterFirst), len(docsAfterSecond))
	}
}
