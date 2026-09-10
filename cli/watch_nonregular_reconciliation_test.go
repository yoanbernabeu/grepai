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

func TestRunInitialScanRemovesFileIndexReplacedByDirectory(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	oldPath := "foo.go"
	directory := filepath.Join(root, oldPath)
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(directory, "bar.go")
	if err := os.WriteFile(child, []byte("package bar\nfunc Current() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore)
	vectorStore := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	if err := vectorStore.SaveDocument(ctx, store.Document{Path: oldPath, Hash: "stale", ChunkIDs: []string{"old-chunk"}}); err != nil {
		t.Fatal(err)
	}
	symbolStore := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	if err := symbolStore.SaveFileWithSignature(ctx, oldPath, "stale", "version",
		[]trace.Symbol{{Name: "Old", File: oldPath}},
		[]trace.Reference{{SymbolName: "Old", File: oldPath, CallerName: "Caller"}},
	); err != nil {
		t.Fatal(err)
	}
	idx := indexer.NewIndexer(root, vectorStore, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
	stats, err := runInitialScan(ctx, idx, scanner, trace.NewRegexExtractor(), symbolStore, []string{".go"}, time.Time{}, true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesRemoved != 1 {
		t.Fatalf("removed=%d, want stale file ownership removed", stats.FilesRemoved)
	}
	if doc, err := vectorStore.GetDocument(ctx, oldPath); err != nil || doc != nil {
		t.Fatalf("old vector ownership retained: doc=%v err=%v", doc, err)
	}
	if symbolStore.IsFileIndexed(oldPath) {
		t.Fatal("old symbol ownership retained")
	}
	if _, err := os.Stat(child); err != nil {
		t.Fatalf("replacement directory contents were physically changed: %v", err)
	}
}
