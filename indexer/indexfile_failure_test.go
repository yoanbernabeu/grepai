package indexer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

type failingEmbedder struct{ mockEmbedder }

func (f *failingEmbedder) EmbedBatch(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("embedder unavailable")
}

// When embedding fails after the file's chunks were deleted, the document
// must go too, otherwise the next scan trusts its stale chunk IDs and never
// re-indexes the file (#311).
func TestIndexFile_FailureDropsDocument(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc a() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(root, ignore)
	vectorStore := store.NewGOBStore(filepath.Join(root, "index.gob"))

	file, err := scanner.ScanFile("main.go")
	if err != nil || file == nil {
		t.Fatalf("ScanFile: %v", err)
	}
	ok := NewIndexer(root, vectorStore, newMockEmbedder(), NewChunker(512, 50), scanner, time.Time{})
	if _, err := ok.IndexFile(ctx, *file); err != nil {
		t.Fatalf("initial IndexFile: %v", err)
	}

	failing := NewIndexer(root, vectorStore, &failingEmbedder{}, NewChunker(512, 50), scanner, time.Time{})
	if _, err := failing.IndexFile(ctx, *file); err == nil {
		t.Fatal("expected an embedding error")
	}
	doc, err := vectorStore.GetDocument(ctx, "main.go")
	if err != nil {
		t.Fatal(err)
	}
	if doc != nil {
		t.Fatalf("document kept after a failed re-index: %+v", doc)
	}
}
