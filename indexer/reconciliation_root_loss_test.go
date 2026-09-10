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

func TestRemovalPreservesIndexWhenRootDisappearsDuringReconciliation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	st := store.NewGOBStore(filepath.Join(root, "index.gob"))
	if err := st.SaveDocument(ctx, store.Document{Path: "old.go", Hash: "old"}); err != nil {
		t.Fatal(err)
	}
	idx := NewIndexer(root, st, nil, nil, nil, time.Time{})
	removed, err := idx.removeMissingFilesWith(ctx, map[string]store.DocumentMetadata{"old.go": {Path: "old.go"}}, func(string) (os.FileInfo, error) {
		if err := os.RemoveAll(root); err != nil {
			t.Fatal(err)
		}
		return nil, os.ErrNotExist
	})
	if !errors.Is(err, os.ErrNotExist) || removed != 0 {
		t.Fatalf("removed=%d err=%v", removed, err)
	}
	if doc, err := st.GetDocument(ctx, "old.go"); err != nil || doc == nil {
		t.Fatalf("stored document lost: doc=%v err=%v", doc, err)
	}
}
