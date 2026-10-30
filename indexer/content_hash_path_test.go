package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yoanbernabeu/grepai/store"
)

// A vector is computed from "File: <path>\n\n" + body, so reusing it for the
// same body under another path would keep the old path in the embedding
// (issue #366). A renamed file must be re-embedded.
func TestIndexAll_DoesNotReuseVectorsEmbeddedUnderAnotherPath(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	vectorStore := store.NewGOBStore(filepath.Join(root, "index.gob"))
	emb := &countingBatchEmbedder{}

	writeManyFuncs(t, root, "func last() {}\n")
	if _, err := newBatchTestIndexer(t, root, vectorStore, emb).IndexAll(ctx); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(root, "moved"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "main.go"), filepath.Join(root, "moved", "renamed.go")); err != nil {
		t.Fatal(err)
	}

	emb.embedded = 0
	stats, err := newBatchTestIndexer(t, root, vectorStore, emb).IndexAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.ChunksCreated == 0 || emb.embedded != stats.ChunksCreated {
		t.Fatalf("embedded %d of %d chunks after a rename, want all of them (vectors of the old path must not be reused)", emb.embedded, stats.ChunksCreated)
	}
}

func TestLookupCachedEmbeddings_RequiresFileHeader(t *testing.T) {
	ctx := context.Background()
	vectorStore := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	if err := vectorStore.SaveChunks(ctx, []store.Chunk{
		{ID: "a.go_0", FilePath: "a.go", Content: "File: a.go\n\nbody", Vector: []float32{1, 0, 0}, ContentHash: "body-hash"},
	}); err != nil {
		t.Fatal(err)
	}
	idx := &Indexer{store: vectorStore}

	for _, tc := range []struct {
		name  string
		chunk ChunkInfo
		hits  int
	}{
		{"same header", ChunkInfo{FilePath: "a.go", EmbedContent: "File: a.go\n\nbody", ContentHash: "body-hash"}, 1},
		{"other path", ChunkInfo{FilePath: "b.go", EmbedContent: "File: b.go\n\nbody", ContentHash: "body-hash"}, 0},
		{"no header", ChunkInfo{FilePath: "a.go", EmbedContent: "body", ContentHash: "body-hash"}, 0},
	} {
		if _, hits := idx.lookupCachedEmbeddings(ctx, []ChunkInfo{tc.chunk}); hits != tc.hits {
			t.Errorf("%s: %d cache hits, want %d", tc.name, hits, tc.hits)
		}
	}
}
