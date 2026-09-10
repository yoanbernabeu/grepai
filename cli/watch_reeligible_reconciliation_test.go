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

func TestRunInitialScanReindexesFileEligibleAgainBeforeCleanup(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := "binary.go"
	absolute := filepath.Join(root, path)
	if err := os.WriteFile(absolute, []byte{'p', 'k', 'g', 0}, 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore)
	vectorStore := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	if err := vectorStore.SaveDocument(ctx, store.Document{Path: path, Hash: "stale", ChunkIDs: []string{"old-chunk"}}); err != nil {
		t.Fatal(err)
	}
	symbolStore := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	if err := symbolStore.SaveFileWithSignature(ctx, path, "stale", "old-version", []trace.Symbol{{Name: "Old", File: path}}, nil); err != nil {
		t.Fatal(err)
	}
	idx := indexer.NewIndexer(root, vectorStore, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
	restored := false
	onScan := func(_, _ int, _ string) {
		if restored {
			return
		}
		restored = true
		if err := os.WriteFile(absolute, []byte("package current\nfunc Current() {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := runInitialScan(ctx, idx, scanner, trace.NewRegexExtractor(), symbolStore, []string{".go"}, time.Time{}, true, onScan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesIndexed != 1 || stats.FilesRemoved != 0 || len(stats.ExcludedFiles) != 0 {
		t.Fatalf("indexed=%d removed=%d excluded=%v", stats.FilesIndexed, stats.FilesRemoved, stats.ExcludedFiles)
	}
	doc, err := vectorStore.GetDocument(ctx, path)
	if err != nil || doc == nil || doc.Hash == "stale" {
		t.Fatalf("vector document remained stale: doc=%v err=%v", doc, err)
	}
	if old, err := symbolStore.LookupSymbol(ctx, "Old"); err != nil || len(old) != 0 {
		t.Fatalf("old symbols=%v err=%v", old, err)
	}
	if current, err := symbolStore.LookupSymbol(ctx, "Current"); err != nil || len(current) != 1 {
		t.Fatalf("current symbols=%v err=%v", current, err)
	}
}
