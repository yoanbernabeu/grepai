package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
)

func TestExcludedSymbolCaseReversalHandsOffRetirement(t *testing.T) {
	for _, aliasInSnapshot := range []bool{false, true} {
		name := "alias-absent-from-snapshot"
		if aliasInSnapshot {
			name = "alias-present-in-snapshot"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "Foo.go"), []byte("package ignored\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			ignore, err := indexer.NewIgnoreMatcher(root, []string{"Foo.go"}, "")
			if err != nil {
				t.Fatal(err)
			}
			scanner := indexer.NewScanner(root, ignore)
			vectors := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
			if err := vectors.SaveDocument(ctx, store.Document{Path: "foo.go", Hash: "temporary"}); err != nil {
				t.Fatal(err)
			}
			symbols := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
			for _, path := range []string{"Foo.go", "foo.go"} {
				if err := symbols.SaveFileWithSignature(ctx, path, path, "version", nil, nil); err != nil {
					t.Fatal(err)
				}
			}
			snapshot := map[string]trace.FileFingerprint{"Foo.go": {ContentHash: "old", HasContentHash: true}}
			if aliasInSnapshot {
				snapshot["foo.go"] = trace.FileFingerprint{ContentHash: "temporary", HasContentHash: true}
			}
			finder := func(string, []string, []indexer.FileMeta) map[string]string {
				return map[string]string{"Foo.go": "foo.go"}
			}
			reconciliation, err := removeOfflineSymbolFilesForScanWithSeams(ctx, scanner, symbols, snapshot,
				[]indexer.FileMeta{{Path: "foo.go"}}, []string{"Foo.go"}, finder, scanner.InspectExistingPath)
			if err != nil {
				t.Fatal(err)
			}
			if len(reconciliation.reeligible) != 0 || len(reconciliation.retiredAliases) != 1 {
				t.Fatalf("reconciliation=%v", reconciliation)
			}
			stats := &indexer.IndexStats{ScannedFiles: []indexer.FileMeta{{Path: "foo.go"}}}
			idx := indexer.NewIndexer(root, vectors, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
			if err := consumeRetiredAliases(ctx, idx, scanner, symbols, snapshot, stats, reconciliation.retiredAliases); err != nil {
				t.Fatal(err)
			}
			if len(stats.ScannedFiles) != 0 {
				t.Fatalf("scanned files=%v", stats.ScannedFiles)
			}
			if documents, err := vectors.ListDocuments(ctx); err != nil || len(documents) != 0 {
				t.Fatalf("vectors=%v err=%v", documents, err)
			}
			fingerprints, err := symbols.ListFileFingerprints(ctx)
			if err != nil || len(fingerprints) != 0 {
				t.Fatalf("fingerprints=%v err=%v", fingerprints, err)
			}
		})
	}
}
