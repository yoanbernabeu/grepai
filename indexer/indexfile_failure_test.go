package indexer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

type failingEmbedder struct{ mockEmbedder }

func (f *failingEmbedder) EmbedBatch(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("embedder unavailable")
}

// When embedding fails, the document must go, otherwise the next scan trusts
// its stale chunk IDs and never re-indexes the file (#311). Its chunks go too,
// so none are left orphaned.
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

	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc b() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if file, err = scanner.ScanFile("main.go"); err != nil || file == nil {
		t.Fatalf("ScanFile: %v", err)
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
	all, err := vectorStore.GetAllChunks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("%d orphaned chunks left after a failed re-index", len(all))
	}
}

type countingEmbedder struct {
	mockEmbedder
	embedded int
}

func (c *countingEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	c.embedded += len(texts)
	return c.mockEmbedder.EmbedBatch(ctx, texts)
}

// Re-indexing a file after a small edit embeds only the chunks that changed;
// the others reuse the vectors already stored for the file.
func TestIndexFile_ReusesUnchangedChunksOfSameFile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	write := func(last string) {
		var b strings.Builder
		b.WriteString("package main\n\n")
		for i := 0; i < 200; i++ {
			fmt.Fprintf(&b, "func f%d() int { return %d }\n\n", i, i)
		}
		b.WriteString(last)
		if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(root, ignore)
	vectorStore := store.NewGOBStore(filepath.Join(root, "index.gob"))
	emb := &countingEmbedder{mockEmbedder: *newMockEmbedder()}
	idx := NewIndexer(root, vectorStore, emb, NewChunker(256, 20), scanner, time.Time{})

	write("func last() {}\n")
	file, err := scanner.ScanFile("main.go")
	if err != nil || file == nil {
		t.Fatalf("ScanFile: %v", err)
	}
	total, err := idx.IndexFile(ctx, *file)
	if err != nil {
		t.Fatal(err)
	}
	if total < 3 {
		t.Fatalf("expected several chunks, got %d", total)
	}

	emb.embedded = 0
	write("func last() { println(\"changed\") }\n")
	if file, err = scanner.ScanFile("main.go"); err != nil || file == nil {
		t.Fatalf("ScanFile: %v", err)
	}
	n, err := idx.IndexFile(ctx, *file)
	if err != nil {
		t.Fatal(err)
	}
	if emb.embedded == 0 || emb.embedded >= n {
		t.Fatalf("embedded %d of %d chunks after a one-line edit, want only the changed ones", emb.embedded, n)
	}
	chunks, err := vectorStore.GetChunksForFile(ctx, "main.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != n {
		t.Fatalf("store holds %d chunks for the file, want %d", len(chunks), n)
	}
}
