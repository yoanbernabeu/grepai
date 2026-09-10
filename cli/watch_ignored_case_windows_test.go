//go:build windows

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

func TestIgnoredCurrentCaseRemovesOldAliasOwnership(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	oldPath := filepath.Join(root, "Foo.go")
	temporaryPath := filepath.Join(root, "case-rename-temp.go")
	currentPath := filepath.Join(root, "foo.go")
	if err := os.WriteFile(oldPath, []byte("package ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(oldPath, temporaryPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporaryPath, currentPath); err != nil {
		t.Fatal(err)
	}
	ignore, err := indexer.NewIgnoreMatcher(root, []string{"foo.go"}, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore)
	vectors := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	if err := vectors.SaveDocument(ctx, store.Document{Path: "Foo.go", Hash: "old", ChunkIDs: []string{"old"}}); err != nil {
		t.Fatal(err)
	}
	symbols := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	if err := symbols.SaveFileWithSignature(ctx, "Foo.go", "old", "version", nil, nil); err != nil {
		t.Fatal(err)
	}
	idx := indexer.NewIndexer(root, vectors, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
	stats, err := runInitialScan(ctx, idx, scanner, trace.NewRegexExtractor(), symbols, []string{".go"}, time.Time{}, true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesRemoved != 1 {
		t.Fatalf("removed=%d, want old ownership removed", stats.FilesRemoved)
	}
	if documents, err := vectors.ListDocuments(ctx); err != nil || len(documents) != 0 {
		t.Fatalf("vectors=%v err=%v", documents, err)
	}
	if fingerprints, err := symbols.ListFileFingerprints(ctx); err != nil || len(fingerprints) != 0 {
		t.Fatalf("symbols=%v err=%v", fingerprints, err)
	}
	if _, err := os.Stat(currentPath); err != nil {
		t.Fatalf("ignored physical file changed: %v", err)
	}
}
