//go:build linux

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

func TestConsumeRetiredAliasRejectsNewExcludedSibling(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(string) error
	}{
		{name: "binary", build: func(path string) error { return os.WriteFile(path, []byte{'p', 'k', 'g', 0}, 0o644) }},
		{name: "nonregular", build: func(path string) error { return os.Mkdir(path, 0o755) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "Foo.go"), []byte("package canonical\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := tc.build(filepath.Join(root, "foo.go")); err != nil {
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
			stats := &indexer.IndexStats{}
			idx := indexer.NewIndexer(root, vectors, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
			err = consumeRetiredAliases(ctx, idx, scanner, symbols, nil, stats,
				[]indexer.RetiredAlias{{Path: "foo.go", CanonicalPath: "Foo.go"}})
			if err == nil || !strings.Contains(err.Error(), "excluded") {
				t.Fatalf("error=%v, want bounded changed-to-excluded error", err)
			}
			if doc, _ := vectors.GetDocument(ctx, "foo.go"); doc == nil || !symbols.IsFileIndexed("foo.go") {
				t.Fatal("error path mutated existing ownership")
			}
		})
	}
}

func TestConsumeRetiredAliasInvalidatesVerifiedSymbolFastPath(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Foo.go"), []byte("package canonical\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := "foo.go"
	absolute := filepath.Join(root, path)
	oldContent := "package sibling\nfunc Old() {}\n"
	newContent := "package sibling\nfunc New() {}\n"
	if len(oldContent) != len(newContent) {
		t.Fatal("fixture sizes differ")
	}
	if err := os.WriteFile(absolute, []byte(oldContent), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore)
	old, err := scanner.ScanFile(path)
	if err != nil {
		t.Fatal(err)
	}
	extractor := trace.NewRegexExtractor()
	symbols := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	if err := symbols.SaveFileWithSignature(ctx, path, old.Hash, extractor.Version(), []trace.Symbol{{Name: "Old", Kind: trace.KindFunction, File: path}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte(newContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(absolute, old.ObservedModTime, old.ObservedModTime); err != nil {
		t.Fatal(err)
	}
	vectors := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	idx := indexer.NewIndexer(root, vectors, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
	stats := &indexer.IndexStats{
		ScannedFiles: []indexer.FileMeta{{Path: path, Size: old.Size, ModTime: old.ModTime, ObservedModTime: old.ObservedModTime}},
		VerifiedUnchangedFiles: map[string]indexer.VerifiedFile{
			path: {Hash: old.Hash, Size: old.Size, ModTime: old.ObservedModTime},
		},
	}
	fingerprints := map[string]trace.FileFingerprint{path: {ContentHash: old.Hash, ExtractorVersion: extractor.Version(), HasContentHash: true, HasExtractorVersion: true}}
	if err := consumeRetiredAliases(ctx, idx, scanner, symbols, fingerprints, stats,
		[]indexer.RetiredAlias{{Path: path, CanonicalPath: "Foo.go"}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := stats.VerifiedUnchangedFiles[path]; ok {
		t.Fatal("stale verified metadata survived fresh alias indexing")
	}
	if _, _, err := indexInitialSymbols(ctx, idx, scanner, extractor, symbols, initialSymbolFingerprints{snapshot: fingerprints}, stats.ScannedFiles, stats.VerifiedUnchangedFiles, []string{".go"}, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if oldSymbols, _ := symbols.LookupSymbol(ctx, "Old"); len(oldSymbols) != 0 {
		t.Fatalf("old symbols retained: %v", oldSymbols)
	}
	if newSymbols, _ := symbols.LookupSymbol(ctx, "New"); len(newSymbols) != 1 {
		t.Fatalf("new symbols=%v", newSymbols)
	}
}
