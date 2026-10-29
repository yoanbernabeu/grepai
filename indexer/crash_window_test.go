package indexer

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yoanbernabeu/grepai/store"
)

// A crash between DeleteByFile and the save of the new chunks must not leave a
// document that makes the next scan skip the file (#356).
func TestIndexAll_ReindexesFileAfterCrashBetweenDeleteAndSave(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	indexPath := filepath.Join(root, "index.gob")
	writeManyFuncs(t, root, "func last() {}\n")

	gobStore := store.NewGOBStore(indexPath)
	if _, err := newBatchTestIndexer(t, root, gobStore, &countingBatchEmbedder{}).IndexAll(ctx); err != nil {
		t.Fatal(err)
	}

	// Re-index of the same content is interrupted right after the delete,
	// and the index is persisted in that state (periodic persist, then crash).
	if err := gobStore.DeleteByFile(ctx, "main.go"); err != nil {
		t.Fatal(err)
	}
	if err := gobStore.Persist(ctx); err != nil {
		t.Fatal(err)
	}

	restarted := store.NewGOBStore(indexPath)
	if err := restarted.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := newBatchTestIndexer(t, root, restarted, &countingBatchEmbedder{}).IndexAll(ctx); err != nil {
		t.Fatal(err)
	}
	chunks, err := restarted.GetChunksForFile(ctx, "main.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("file has no chunks after restart: the scan skipped it")
	}
}
