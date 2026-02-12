package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
	"github.com/yoanbernabeu/grepai/watcher"
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

type countingEmbedder struct {
	noOpEmbedder
	embedCalls      int
	embedBatchCalls int
}

func (e *countingEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	e.embedCalls++
	return e.noOpEmbedder.Embed(ctx, text)
}

func (e *countingEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	e.embedBatchCalls++
	return e.noOpEmbedder.EmbedBatch(ctx, texts)
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

func TestRunInitialScan_SkipsIndexedFileByLastIndexTime(t *testing.T) {
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

	emb := &countingEmbedder{}
	scanner := indexer.NewScanner(projectRoot, ignoreMatcher)
	chunker := indexer.NewChunker(512, 50)
	vecStore := store.NewGOBStore(filepath.Join(projectRoot, "index.gob"))
	idx := indexer.NewIndexer(projectRoot, vecStore, emb, chunker, scanner, time.Now().Add(1*time.Hour))

	symbolStore := trace.NewGOBSymbolStore(filepath.Join(projectRoot, "symbols.gob"))
	defer symbolStore.Close()

	if err := symbolStore.SaveFile(ctx, "main.go", []trace.Symbol{
		{
			Name:     "sentinel",
			Kind:     trace.KindFunction,
			File:     "main.go",
			Line:     1,
			Language: "go",
		},
	}, nil); err != nil {
		t.Fatalf("failed to seed symbol store: %v", err)
	}

	lastIndexTime := time.Now().Add(1 * time.Hour)
	extractor := trace.NewRegexExtractor()
	if _, err := runInitialScan(ctx, idx, scanner, extractor, symbolStore, []string{".go"}, lastIndexTime, true); err != nil {
		t.Fatalf("runInitialScan failed: %v", err)
	}

	sentinelSymbols, err := symbolStore.LookupSymbol(ctx, "sentinel")
	if err != nil {
		t.Fatalf("failed to lookup sentinel symbol: %v", err)
	}
	if len(sentinelSymbols) == 0 {
		t.Fatal("expected sentinel symbol to remain when file is skipped by lastIndexTime")
	}

	realSymbols, err := symbolStore.LookupSymbol(ctx, "real")
	if err != nil {
		t.Fatalf("failed to lookup real symbol: %v", err)
	}
	if len(realSymbols) != 0 {
		t.Fatalf("expected real symbol extraction to be skipped, found %d symbols", len(realSymbols))
	}

	if emb.embedCalls != 0 || emb.embedBatchCalls != 0 {
		t.Fatalf("expected no embedding calls for skipped startup path, got embed=%d embedBatch=%d", emb.embedCalls, emb.embedBatchCalls)
	}
}

func TestHandleFileEvent_SkipsUnchangedFile(t *testing.T) {
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

	emb := &countingEmbedder{}
	scanner := indexer.NewScanner(projectRoot, ignoreMatcher)
	chunker := indexer.NewChunker(512, 50)
	vecStore := store.NewGOBStore(filepath.Join(projectRoot, "index.gob"))
	idx := indexer.NewIndexer(projectRoot, vecStore, emb, chunker, scanner, time.Time{})

	fileInfo, err := scanner.ScanFile("main.go")
	if err != nil || fileInfo == nil {
		t.Fatalf("failed to scan source file: %v", err)
	}
	if err := vecStore.SaveDocument(ctx, store.Document{
		Path:    "main.go",
		Hash:    fileInfo.Hash,
		ModTime: time.Unix(fileInfo.ModTime, 0),
	}); err != nil {
		t.Fatalf("failed to seed document: %v", err)
	}

	symbolStore := trace.NewGOBSymbolStore(filepath.Join(projectRoot, "symbols.gob"))
	defer symbolStore.Close()
	if err := symbolStore.SaveFile(ctx, "main.go", []trace.Symbol{
		{
			Name:     "sentinel",
			Kind:     trace.KindFunction,
			File:     "main.go",
			Line:     1,
			Language: "go",
		},
	}, nil); err != nil {
		t.Fatalf("failed to seed symbol store: %v", err)
	}

	cfg := config.DefaultConfig()
	lastWrite := time.Time{}
	handleFileEvent(
		ctx,
		idx,
		scanner,
		trace.NewRegexExtractor(),
		symbolStore,
		nil,
		nil,
		[]string{".go"},
		projectRoot,
		cfg,
		&lastWrite,
		nil,
		watcher.FileEvent{Type: watcher.EventModify, Path: "main.go"},
	)

	if emb.embedCalls != 0 || emb.embedBatchCalls != 0 {
		t.Fatalf("expected unchanged file to skip embedding, got embed=%d embedBatch=%d", emb.embedCalls, emb.embedBatchCalls)
	}

	realSymbols, err := symbolStore.LookupSymbol(ctx, "real")
	if err != nil {
		t.Fatalf("failed to lookup real symbol: %v", err)
	}
	if len(realSymbols) != 0 {
		t.Fatalf("expected no new symbol extraction for unchanged file, got %d", len(realSymbols))
	}

	if !cfg.Watch.LastIndexTime.IsZero() {
		t.Fatalf("expected config last index time to remain zero on skip, got %v", cfg.Watch.LastIndexTime)
	}
}

func TestHandleWorkspaceFileEvent_SkipsUnchangedFile(t *testing.T) {
	ctx := context.Background()
	tmpRoot := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(tmpRoot); err != nil {
		t.Fatalf("failed to chdir to temp root: %v", err)
	}
	defer func() {
		_ = os.Chdir(origWD)
	}()

	projectPath := "proj"
	projectRoot := filepath.Join(tmpRoot, projectPath)
	if err := os.MkdirAll(filepath.Join(projectRoot, "proj"), 0755); err != nil {
		t.Fatalf("failed to create project dirs: %v", err)
	}

	srcPath := filepath.Join(projectRoot, "proj", "main.go")
	srcContent := "package main\n\nfunc real() {}\n"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	ignoreMatcher, err := indexer.NewIgnoreMatcher(projectPath, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}
	scanner := indexer.NewScanner(projectPath, ignoreMatcher)
	fileInfo, err := scanner.ScanFile("proj/main.go")
	if err != nil || fileInfo == nil {
		t.Fatalf("failed to scan source file: %v", err)
	}

	st := store.NewGOBStore(filepath.Join(projectRoot, "workspace-index.gob"))
	workspaceName := "ws"
	projectName := "proj"
	prefixedPath := workspaceName + "/" + projectName + "/proj/main.go"
	if err := st.SaveDocument(ctx, store.Document{
		Path:    prefixedPath,
		Hash:    fileInfo.Hash,
		ModTime: time.Unix(fileInfo.ModTime, 0),
	}); err != nil {
		t.Fatalf("failed to seed workspace document: %v", err)
	}

	emb := &countingEmbedder{}
	wrappedStore := &projectPrefixStore{
		store:         st,
		workspaceName: workspaceName,
		projectName:   projectName,
		projectPath:   projectPath,
	}
	chunker := indexer.NewChunker(512, 64)
	idx := indexer.NewIndexer(projectPath, wrappedStore, emb, chunker, scanner, time.Time{})
	extractor := trace.NewRegexExtractor()
	cfg := config.DefaultConfig()
	var lastConfigWrite time.Time

	handleFileEvent(ctx, idx, scanner, extractor, nil, nil, wrappedStore, nil, projectPath, cfg, &lastConfigWrite, nil, watcher.FileEvent{
		Type: watcher.EventModify,
		Path: "proj/main.go",
	})

	if emb.embedCalls != 0 || emb.embedBatchCalls != 0 {
		t.Fatalf("expected unchanged workspace file to skip embedding, got embed=%d embedBatch=%d", emb.embedCalls, emb.embedBatchCalls)
	}

	stats, err := st.GetStats(ctx)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}
	if stats.TotalFiles != 1 {
		t.Fatalf("expected workspace docs to remain unchanged, got total files %d", stats.TotalFiles)
	}
}

func TestHandleFileEvent_IndexesChangedFileAndUpdatesSymbols(t *testing.T) {
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

	emb := &countingEmbedder{}
	scanner := indexer.NewScanner(projectRoot, ignoreMatcher)
	chunker := indexer.NewChunker(512, 50)
	vecStore := store.NewGOBStore(filepath.Join(projectRoot, "index.gob"))
	idx := indexer.NewIndexer(projectRoot, vecStore, emb, chunker, scanner, time.Time{})

	// Seed old hash to force NeedsReindex == true.
	fileInfo, err := scanner.ScanFile("main.go")
	if err != nil || fileInfo == nil {
		t.Fatalf("failed to scan source file: %v", err)
	}
	if err := vecStore.SaveDocument(ctx, store.Document{
		Path:    "main.go",
		Hash:    "old-hash",
		ModTime: time.Unix(fileInfo.ModTime, 0),
	}); err != nil {
		t.Fatalf("failed to seed old document: %v", err)
	}

	symbolStore := trace.NewGOBSymbolStore(filepath.Join(projectRoot, "symbols.gob"))
	defer symbolStore.Close()

	cfg := config.DefaultConfig()
	lastWrite := time.Time{}
	handleFileEvent(
		ctx,
		idx,
		scanner,
		trace.NewRegexExtractor(),
		symbolStore,
		nil,
		nil,
		[]string{".go"},
		projectRoot,
		cfg,
		&lastWrite,
		nil,
		watcher.FileEvent{Type: watcher.EventModify, Path: "main.go"},
	)

	if emb.embedCalls == 0 && emb.embedBatchCalls == 0 {
		t.Fatal("expected changed file to trigger embedding")
	}
	if cfg.Watch.LastIndexTime.IsZero() {
		t.Fatal("expected changed file to update config last index time")
	}
	if lastWrite.IsZero() {
		t.Fatal("expected last config write timestamp to be updated")
	}

	realSymbols, err := symbolStore.LookupSymbol(ctx, "real")
	if err != nil {
		t.Fatalf("failed to lookup real symbol: %v", err)
	}
	if len(realSymbols) == 0 {
		t.Fatal("expected symbols to be extracted and saved for changed file")
	}
}

func TestHandleFileEvent_DeleteRemovesIndexAndSymbols(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()

	ignoreMatcher, err := indexer.NewIgnoreMatcher(projectRoot, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	emb := &countingEmbedder{}
	scanner := indexer.NewScanner(projectRoot, ignoreMatcher)
	chunker := indexer.NewChunker(512, 50)
	vecStore := store.NewGOBStore(filepath.Join(projectRoot, "index.gob"))
	idx := indexer.NewIndexer(projectRoot, vecStore, emb, chunker, scanner, time.Time{})

	if err := vecStore.SaveDocument(ctx, store.Document{
		Path: "main.go",
		Hash: "hash",
	}); err != nil {
		t.Fatalf("failed to seed document: %v", err)
	}

	symbolStore := trace.NewGOBSymbolStore(filepath.Join(projectRoot, "symbols.gob"))
	defer symbolStore.Close()
	if err := symbolStore.SaveFile(ctx, "main.go", []trace.Symbol{
		{
			Name:     "sentinel",
			Kind:     trace.KindFunction,
			File:     "main.go",
			Line:     1,
			Language: "go",
		},
	}, nil); err != nil {
		t.Fatalf("failed to seed symbol store: %v", err)
	}

	cfg := config.DefaultConfig()
	lastWrite := time.Time{}
	handleFileEvent(
		ctx,
		idx,
		scanner,
		trace.NewRegexExtractor(),
		symbolStore,
		nil,
		nil,
		[]string{".go"},
		projectRoot,
		cfg,
		&lastWrite,
		nil,
		watcher.FileEvent{Type: watcher.EventDelete, Path: "main.go"},
	)

	doc, err := vecStore.GetDocument(ctx, "main.go")
	if err != nil {
		t.Fatalf("failed to read document: %v", err)
	}
	if doc != nil {
		t.Fatal("expected document to be deleted on delete event")
	}
	if symbolStore.IsFileIndexed("main.go") {
		t.Fatal("expected symbols to be deleted on delete event")
	}
}
