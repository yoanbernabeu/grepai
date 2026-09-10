package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

func TestOrdinaryExcludedReeligibilityRejectsDifferentIndexKey(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Foo.go"), []byte("package foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(root, ignore)
	scanner.readSnapshot = func(string, string) (*FileInfo, error) {
		return &FileInfo{Path: "FOO.go", Content: "package changed\n", Hash: "changed", ObservedModTime: time.Now()}, nil
	}
	st := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	if err := st.SaveDocument(ctx, store.Document{Path: "Foo.go", Hash: "old", ChunkIDs: []string{"old"}}); err != nil {
		t.Fatal(err)
	}
	idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), scanner, time.Time{})
	_, reconciliation, err := idx.removeCandidatesWithRevalidation(ctx,
		map[string]store.DocumentMetadata{"Foo.go": {Path: "Foo.go"}},
		map[string]string{"Foo.go": "cached exclusion"}, nil,
	)
	if err == nil {
		t.Fatalf("reconciliation=%v err=nil, want changed-key error", reconciliation)
	}
	if doc, readErr := st.GetDocument(ctx, "Foo.go"); readErr != nil || doc == nil {
		t.Fatalf("original ownership changed: doc=%v err=%v", doc, readErr)
	}
	if doc, readErr := st.GetDocument(ctx, "FOO.go"); readErr != nil || doc != nil {
		t.Fatalf("different key published: doc=%v err=%v", doc, readErr)
	}
}
