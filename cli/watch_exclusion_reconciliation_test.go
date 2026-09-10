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

func TestRunInitialScanPurgesExcludedSymbols(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	fixtures := map[string][]byte{
		"ignored.go":      []byte("package ignored\n"),
		"unsupported.xyz": []byte("unsupported\n"),
		"bundle.min.js":   []byte("const minified = true\n"),
		"binary.go":       {'p', 'a', 'c', 'k', 'a', 'g', 'e', 0, 'x'},
	}
	for path, content := range fixtures {
		if err := os.WriteFile(filepath.Join(root, path), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "large.go"), []byte(strings.Repeat("x", 1*1024*1024+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := indexer.NewIgnoreMatcher(root, []string{"ignored.go"}, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore)
	vectorStore := store.NewGOBStore(filepath.Join(root, "index.gob"))
	idx := indexer.NewIndexer(root, vectorStore, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
	symbolStore := trace.NewGOBSymbolStore(filepath.Join(root, "symbols.gob"))
	paths := []string{"ignored.go", "unsupported.xyz", "bundle.min.js", "large.go", "binary.go"}
	for _, path := range paths {
		if err := symbolStore.SaveFileWithSignature(ctx, path, "old", "version", nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runInitialScan(ctx, idx, scanner, trace.NewRegexExtractor(), symbolStore, []string{".go", ".js"}, time.Time{}, true, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"ignored.go", "unsupported.xyz", "bundle.min.js", "large.go", "binary.go"} {
		if symbolStore.IsFileIndexed(path) {
			t.Errorf("excluded symbol path retained: %s", path)
		}
	}
}
