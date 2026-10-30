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
