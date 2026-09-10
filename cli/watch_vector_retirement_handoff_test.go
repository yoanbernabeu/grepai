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

func TestVectorExcludedReversalRetirementReachesSymbolConsumer(t *testing.T) {
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
			fingerprints := map[string]trace.FileFingerprint{"Foo.go": {ContentHash: "old", HasContentHash: true}}
			if aliasInSnapshot {
				fingerprints["foo.go"] = trace.FileFingerprint{ContentHash: "temporary", HasContentHash: true}
			}
			stats := &indexer.IndexStats{
				ScannedFiles:   []indexer.FileMeta{{Path: "foo.go"}},
				ExcludedFiles:  []string{"Foo.go"},
				RetiredAliases: []indexer.RetiredAlias{{Path: "foo.go", CanonicalPath: "Foo.go"}},
			}
			idx := indexer.NewIndexer(root, vectors, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
			if err := consumeRetiredAliases(ctx, idx, scanner, symbols, fingerprints, stats, stats.RetiredAliases); err != nil {
				t.Fatal(err)
			}
			stats.RetiredAliases = nil
			if _, err := removeOfflineSymbolFilesForScan(ctx, scanner, symbols, fingerprints, stats.ScannedFiles, stats.ExcludedFiles); err != nil {
				t.Fatal(err)
			}
			if len(stats.ScannedFiles) != 0 {
				t.Fatalf("scanned files=%v", stats.ScannedFiles)
			}
			if documents, err := vectors.ListDocuments(ctx); err != nil || len(documents) != 0 {
				t.Fatalf("vectors=%v err=%v", documents, err)
			}
			stored, err := symbols.ListFileFingerprints(ctx)
			if err != nil || len(stored) != 0 {
				t.Fatalf("symbol fingerprints=%v err=%v", stored, err)
			}
		})
	}
}
