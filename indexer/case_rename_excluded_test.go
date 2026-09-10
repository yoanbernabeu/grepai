package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

func TestExcludedCaseRenameReversalRetiresAliasMetadata(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Foo.go"), []byte("package ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := NewIgnoreMatcher(root, []string{"Foo.go"}, "")
	if err != nil {
		t.Fatal(err)
	}
	st := store.NewGOBStore(filepath.Join(root, "index.gob"))
	for _, doc := range []store.Document{
		{Path: "Foo.go", Hash: "old", ChunkIDs: []string{"old"}},
		{Path: "foo.go", Hash: "temporary", ChunkIDs: []string{"temporary"}},
	} {
		if err := st.SaveDocument(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), NewScanner(root, ignore), time.Time{})
	removed, reconciliation, err := idx.removeCandidatesWithRevalidation(ctx,
		map[string]store.DocumentMetadata{"Foo.go": {Path: "Foo.go"}}, nil,
		map[string]string{"Foo.go": "foo.go"},
	)
	if err != nil {
		t.Fatal(err)
	}
	stats := &IndexStats{ScannedFiles: []FileMeta{{Path: "foo.go"}}}
	if err := idx.applyRemovalReconciliation(ctx, stats, reconciliation); err != nil {
		t.Fatal(err)
	}
	if removed != 2 || len(reconciliation.reeligible) != 0 || len(reconciliation.retiredAliases) != 1 {
		t.Fatalf("removed=%d reconciliation=%v", removed, reconciliation)
	}
	if len(stats.ScannedFiles) != 0 || len(stats.RetiredAliases) != 1 {
		t.Fatalf("scanned=%v retired=%v", stats.ScannedFiles, stats.RetiredAliases)
	}
	if documents, err := st.ListDocuments(ctx); err != nil || len(documents) != 0 {
		t.Fatalf("documents=%v err=%v", documents, err)
	}
}
