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

func TestOrdinarySymbolReeligibilityRejectsDifferentIndexKey(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Foo.go"), []byte("package foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore)
	symbols := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	if err := symbols.SaveFileWithSignature(ctx, "Foo.go", "old", "version", nil, nil); err != nil {
		t.Fatal(err)
	}
	inspect := func(string) (*indexer.FileInfo, indexer.PathExclusionReason, error) {
		return &indexer.FileInfo{Path: "FOO.go", Content: "package changed\n"}, "", nil
	}
	result, err := removeOfflineSymbolFilesForScanWithSeams(ctx, scanner, symbols,
		map[string]trace.FileFingerprint{"Foo.go": {ContentHash: "old", HasContentHash: true}},
		nil, []string{"Foo.go"}, indexer.FindCaseRenameWitnesses, inspect)
	if err == nil {
		t.Fatalf("result=%v err=nil, want changed-key error", result)
	}
	if !symbols.IsFileIndexed("Foo.go") || symbols.IsFileIndexed("FOO.go") {
		t.Fatalf("symbol ownership changed: Foo=%v FOO=%v", symbols.IsFileIndexed("Foo.go"), symbols.IsFileIndexed("FOO.go"))
	}
}

func TestConsumeRetiredAliasRejectsDifferentReappearedKey(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "foo.go"), []byte("package sibling\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore)
	vectors := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	if err := vectors.SaveDocument(ctx, store.Document{Path: "foo.go", Hash: "old"}); err != nil {
		t.Fatal(err)
	}
	symbols := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	if err := symbols.SaveFileWithSignature(ctx, "foo.go", "old", "version", nil, nil); err != nil {
		t.Fatal(err)
	}
	inspect := func(string) (*indexer.FileInfo, indexer.PathExclusionReason, error) {
		return &indexer.FileInfo{Path: "FOO.go", Content: "package changed\n"}, "", nil
	}
	idx := indexer.NewIndexer(root, vectors, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
	err = consumeRetiredAliasesWithInspect(ctx, idx, scanner, symbols, nil, &indexer.IndexStats{},
		[]indexer.RetiredAlias{{Path: "foo.go", CanonicalPath: "Foo.go"}}, inspect)
	if err == nil {
		t.Fatal("unexpected reappeared key was accepted")
	}
	if doc, readErr := vectors.GetDocument(ctx, "foo.go"); readErr != nil || doc == nil || !symbols.IsFileIndexed("foo.go") {
		t.Fatalf("ownership changed on error: doc=%v err=%v indexed=%v", doc, readErr, symbols.IsFileIndexed("foo.go"))
	}
}
