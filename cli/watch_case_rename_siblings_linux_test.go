//go:build linux

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

func TestCachedSymbolCaseRenamePreservesActualCaseSibling(t *testing.T) {
	for _, hardlink := range []bool{false, true} {
		name := "distinct"
		if hardlink {
			name = "hardlink"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			upper := filepath.Join(root, "Foo.go")
			lower := filepath.Join(root, "foo.go")
			if err := os.WriteFile(upper, []byte("package upper\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if hardlink {
				if err := os.Link(upper, lower); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(lower, []byte("package lower\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			scanner := indexer.NewScanner(root, ignore)
			symbols := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
			for _, path := range []string{"Foo.go", "foo.go"} {
				if err := symbols.SaveFileWithSignature(ctx, path, path, "version", nil, nil); err != nil {
					t.Fatal(err)
				}
			}
			finder := func(string, []string, []indexer.FileMeta) map[string]string {
				return map[string]string{"Foo.go": "foo.go"}
			}
			snapshot := map[string]trace.FileFingerprint{"Foo.go": {ContentHash: "stale", HasContentHash: true}}
			reconciliation, err := removeOfflineSymbolFilesForScanWithSeams(ctx, scanner, symbols, snapshot, []indexer.FileMeta{{Path: "foo.go"}}, nil, finder, scanner.InspectExistingPath)
			if err != nil {
				t.Fatal(err)
			}
			if len(reconciliation.reeligible) != 1 || reconciliation.reeligible[0].Path != "Foo.go" || len(reconciliation.retiredAliases) != 0 {
				t.Fatalf("reconciliation=%v", reconciliation)
			}
			idx := indexer.NewIndexer(root, store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob")), &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
			if _, _, err := indexInitialSymbols(ctx, idx, scanner, trace.NewRegexExtractor(), symbols,
				initialSymbolFingerprints{snapshot: snapshot}, []indexer.FileMeta{{Path: "Foo.go"}}, nil, []string{".go"}, time.Time{}); err != nil {
				t.Fatal(err)
			}
			if !symbols.IsFileIndexed("Foo.go") || !symbols.IsFileIndexed("foo.go") {
				t.Fatalf("case siblings: Foo=%v foo=%v", symbols.IsFileIndexed("Foo.go"), symbols.IsFileIndexed("foo.go"))
			}
			fingerprints, err := symbols.ListFileFingerprints(ctx)
			if err != nil || fingerprints["Foo.go"].ContentHash != reconciliation.reeligible[0].Hash || fingerprints["foo.go"].ContentHash != "foo.go" {
				t.Fatalf("fingerprints=%v err=%v", fingerprints, err)
			}
		})
	}
}
