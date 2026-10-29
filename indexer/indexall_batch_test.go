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

	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/store"
)

// countingBatchEmbedder counts the chunks sent through EmbedBatches and can be
// told to fail, to exercise the IndexAll batch path.
type countingBatchEmbedder struct {
	mockBatchEmbedder
	embedded int
	fail     bool
}

func (c *countingBatchEmbedder) EmbedBatches(ctx context.Context, batches []embedder.Batch, progress embedder.BatchProgress) ([]embedder.BatchResult, error) {
	if c.fail {
		return nil, errors.New("embedder unavailable")
	}
	for _, b := range batches {
		c.embedded += b.Size()
	}
	return c.mockBatchEmbedder.EmbedBatches(ctx, batches, progress)
}

func writeManyFuncs(t *testing.T, root, last string) {
	t.Helper()
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

func newBatchTestIndexer(t *testing.T, root string, vectorStore store.VectorStore, emb *countingBatchEmbedder) *Indexer {
	t.Helper()
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	return NewIndexer(root, vectorStore, emb, NewChunker(256, 20), NewScanner(root, ignore), time.Time{})
}

func TestIndexAllBatched_ReusesUnchangedChunksOfSameFile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	vectorStore := store.NewGOBStore(filepath.Join(root, "index.gob"))
	emb := &countingBatchEmbedder{}

	writeManyFuncs(t, root, "func last() {}\n")
	stats, err := newBatchTestIndexer(t, root, vectorStore, emb).IndexAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.ChunksCreated < 3 {
		t.Fatalf("expected several chunks, got %d", stats.ChunksCreated)
	}

	emb.embedded = 0
	writeManyFuncs(t, root, "func last() { println(\"changed\") }\n")
	stats, err = newBatchTestIndexer(t, root, vectorStore, emb).IndexAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	n := stats.ChunksCreated
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

func TestIndexAllBatched_EmbedFailureKeepsPreviousChunks(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	vectorStore := store.NewGOBStore(filepath.Join(root, "index.gob"))
	emb := &countingBatchEmbedder{}

	writeManyFuncs(t, root, "func last() {}\n")
	if _, err := newBatchTestIndexer(t, root, vectorStore, emb).IndexAll(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := vectorStore.GetChunksForFile(ctx, "main.go")
	if err != nil || len(before) == 0 {
		t.Fatalf("GetChunksForFile: %d chunks, err %v", len(before), err)
	}

	writeManyFuncs(t, root, "func last() { println(\"changed\") }\n")
	emb.fail = true
	if _, err := newBatchTestIndexer(t, root, vectorStore, emb).IndexAll(ctx); err == nil {
		t.Fatal("expected an embedding error")
	}
	after, err := vectorStore.GetChunksForFile(ctx, "main.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("file has %d chunks after a failed re-index, want the previous %d", len(after), len(before))
	}
}

// failingReplaceStore fails the delete or the save step of a file's chunk swap.
type failingReplaceStore struct {
	*store.GOBStore
	failDelete, failSave bool
}

func (s *failingReplaceStore) DeleteByFile(ctx context.Context, path string) error {
	if s.failDelete {
		return context.Canceled
	}
	return s.GOBStore.DeleteByFile(ctx, path)
}

func (s *failingReplaceStore) SaveChunks(ctx context.Context, chunks []store.Chunk) error {
	if s.failSave {
		return errors.New("store unavailable")
	}
	return s.GOBStore.SaveChunks(ctx, chunks)
}

func TestIndexAllBatched_ReplaceFailure(t *testing.T) {
	for _, tc := range []struct {
		name         string
		failDelete   bool
		failSave     bool
		wantPrevious bool
		wantDocument bool
	}{
		// Nothing was deleted: the previous index stays usable.
		{name: "delete fails", failDelete: true, wantPrevious: true, wantDocument: true},
		// Previous chunks are gone: drop the file so the next scan retries it.
		{name: "save fails", failSave: true, wantPrevious: false, wantDocument: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			gobStore := store.NewGOBStore(filepath.Join(root, "index.gob"))
			emb := &countingBatchEmbedder{}

			writeManyFuncs(t, root, "func last() {}\n")
			if _, err := newBatchTestIndexer(t, root, gobStore, emb).IndexAll(ctx); err != nil {
				t.Fatal(err)
			}
			before, err := gobStore.GetChunksForFile(ctx, "main.go")
			if err != nil || len(before) == 0 {
				t.Fatalf("GetChunksForFile: %d chunks, err %v", len(before), err)
			}

			writeManyFuncs(t, root, "func last() { println(\"changed\") }\n")
			failing := &failingReplaceStore{GOBStore: gobStore, failDelete: tc.failDelete, failSave: tc.failSave}
			if _, err := newBatchTestIndexer(t, root, failing, emb).IndexAll(ctx); err == nil {
				t.Fatal("expected a store error")
			}

			after, err := gobStore.GetChunksForFile(ctx, "main.go")
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantPrevious && len(after) != len(before) {
				t.Fatalf("file has %d chunks, want the previous %d", len(after), len(before))
			}
			if !tc.wantPrevious && len(after) != 0 {
				t.Fatalf("file has %d chunks, want none", len(after))
			}
			doc, err := gobStore.GetDocument(ctx, "main.go")
			if err != nil {
				t.Fatal(err)
			}
			if (doc != nil) != tc.wantDocument {
				t.Fatalf("document present = %v, want %v", doc != nil, tc.wantDocument)
			}
		})
	}
}
