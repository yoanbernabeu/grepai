package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
)

func TestLateSymbolExclusionRemovesBothIndexes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(string) error
	}{
		{name: "binary", mutate: func(path string) error { return os.WriteFile(path, []byte{'p', 'k', 'g', 0}, 0o644) }},
		{name: "oversize", mutate: func(path string) error { return os.WriteFile(path, []byte(strings.Repeat("x", 1*1024*1024+1)), 0o644) }},
		{name: "nonregular", mutate: func(path string) error {
			if err := os.Remove(path); err != nil {
				return err
			}
			return os.Mkdir(path, 0o755)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			path := "late.go"
			absolute := filepath.Join(root, path)
			if err := os.WriteFile(absolute, []byte("package late\nfunc Old() {}\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			scanner := indexer.NewScanner(root, ignore)
			vectors := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
			symbols := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
			if err := symbols.SaveFileWithSignature(ctx, path, "old", "version",
				[]trace.Symbol{{Name: "Old", File: path}}, []trace.Reference{{SymbolName: "Old", File: path, CallerName: "Caller"}}); err != nil {
				t.Fatal(err)
			}
			idx := indexer.NewIndexer(root, vectors, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
			mutated := false
			onEmbed := func(indexer.BatchProgressInfo) {
				if !mutated {
					mutated = true
					if err := tc.mutate(absolute); err != nil {
						t.Error(err)
					}
				}
			}
			stats, err := runInitialScan(ctx, idx, scanner, trace.NewRegexExtractor(), symbols, []string{".go"}, time.Time{}, true, nil, onEmbed)
			if err != nil {
				t.Fatal(err)
			}
			if stats.FilesRemoved != 1 || len(stats.ScannedFiles) != 0 {
				t.Fatalf("removed=%d scanned=%v", stats.FilesRemoved, stats.ScannedFiles)
			}
			if documents, err := vectors.ListDocuments(ctx); err != nil || len(documents) != 0 {
				t.Fatalf("vectors=%v err=%v", documents, err)
			}
			if fingerprints, err := symbols.ListFileFingerprints(ctx); err != nil || len(fingerprints) != 0 {
				t.Fatalf("symbols=%v err=%v", fingerprints, err)
			}
		})
	}
}

func TestLateSymbolExclusionPreservesOwnershipWhenRootIsLost(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	vectors := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	if err := vectors.SaveDocument(ctx, store.Document{Path: "late.go", Hash: "old"}); err != nil {
		t.Fatal(err)
	}
	symbols := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	if err := symbols.SaveFileWithSignature(ctx, "late.go", "old", "version", nil, nil); err != nil {
		t.Fatal(err)
	}
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore)
	idx := indexer.NewIndexer(root, vectors, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := removeExcludedDuringSymbolScan(ctx, idx, scanner, symbols, "late.go", indexer.PathExcludedBinary); err == nil {
		t.Fatal("root loss returned successful exclusion cleanup")
	}
	if doc, _ := vectors.GetDocument(ctx, "late.go"); doc == nil || !symbols.IsFileIndexed("late.go") {
		t.Fatal("root loss removed existing ownership")
	}
}

func TestEmptySymbolScanRevalidatesEligibleFile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := "late.go"
	if err := os.WriteFile(filepath.Join(root, path), []byte("package late\nfunc Current() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore)
	vectors := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	if err := vectors.SaveDocument(ctx, store.Document{Path: path, Hash: "stale", ChunkIDs: []string{"old"}}); err != nil {
		t.Fatal(err)
	}
	symbols := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	idx := indexer.NewIndexer(root, vectors, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
	resolution, err := reconcileEmptySymbolScan(ctx, idx, scanner, symbols, path)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.removed || resolution.file == nil || resolution.file.Path != path {
		t.Fatalf("resolution=%+v", resolution)
	}
	doc, err := vectors.GetDocument(ctx, path)
	if err != nil || doc == nil || doc.Hash == "stale" {
		t.Fatalf("vector repair failed: doc=%v err=%v", doc, err)
	}
}
