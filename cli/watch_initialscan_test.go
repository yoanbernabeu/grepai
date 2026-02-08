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

type noOpEmbedder struct{}

func (e *noOpEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3}, nil
}

func (e *noOpEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for i := range texts {
		vectors[i] = []float32{0.1, 0.2, 0.3}
	}
	return vectors, nil
}

func (e *noOpEmbedder) Dimensions() int {
	return 3
}

func (e *noOpEmbedder) Close() error {
	return nil
}

func TestRunInitialScan_SkipsSymbolExtractionWhenContentHashMatches(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()

	srcPath := filepath.Join(projectRoot, "main.go")
	srcContent := "package main\n\nfunc real() {}\n"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	ignoreMatcher, err := indexer.NewIgnoreMatcher(projectRoot, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	scanner := indexer.NewScanner(projectRoot, ignoreMatcher)
	chunker := indexer.NewChunker(512, 50)
	vecStore := store.NewGOBStore(filepath.Join(projectRoot, "index.gob"))
	idx := indexer.NewIndexer(projectRoot, vecStore, &noOpEmbedder{}, chunker, scanner, time.Now().Add(1*time.Hour))

	symbolStore := trace.NewGOBSymbolStore(filepath.Join(projectRoot, "symbols.gob"))
	if err := symbolStore.Load(ctx); err != nil {
		t.Fatalf("failed to load symbol store: %v", err)
	}
	defer symbolStore.Close()

	fileInfo, err := scanner.ScanFile("main.go")
	if err != nil {
		t.Fatalf("failed to scan file: %v", err)
	}
	if fileInfo == nil {
		t.Fatal("expected scanned file info")
	}

	sentinel := []trace.Symbol{
		{
			Name:     "sentinel",
			Kind:     trace.KindFunction,
			File:     "main.go",
			Line:     1,
			Language: "go",
		},
	}
	if err := symbolStore.SaveFileWithContentHash(ctx, fileInfo.Path, fileInfo.Hash, sentinel, nil); err != nil {
		t.Fatalf("failed to seed symbol store: %v", err)
	}

	extractor := trace.NewRegexExtractor()
	if _, err := runInitialScan(ctx, idx, scanner, extractor, symbolStore, []string{".go"}, time.Time{}, true); err != nil {
		t.Fatalf("runInitialScan failed: %v", err)
	}

	sentinelSymbols, err := symbolStore.LookupSymbol(ctx, "sentinel")
	if err != nil {
		t.Fatalf("failed to lookup sentinel symbol: %v", err)
	}
	if len(sentinelSymbols) == 0 {
		t.Fatal("expected seeded sentinel symbol to remain when hash matches")
	}

	realSymbols, err := symbolStore.LookupSymbol(ctx, "real")
	if err != nil {
		t.Fatalf("failed to lookup real symbol: %v", err)
	}
	if len(realSymbols) != 0 {
		t.Fatalf("expected real symbol extraction to be skipped, found %d symbols", len(realSymbols))
	}
}
