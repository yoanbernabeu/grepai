package cli

import (
	"context"
	"errors"
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

type startupFingerprintStore struct {
	*trace.GOBSymbolStore
	listErr error
	saveErr error
	listed  int
	saved   int
}

func (s *startupFingerprintStore) ListFileFingerprints(ctx context.Context) (map[string]trace.FileFingerprint, error) {
	s.listed++
	if s.listErr != nil {
		return nil, s.listErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return map[string]trace.FileFingerprint{}, nil
}

func (s *startupFingerprintStore) SaveFileWithSignature(ctx context.Context, path, hash, version string, symbols []trace.Symbol, refs []trace.Reference) error {
	s.saved++
	if s.saveErr != nil {
		return s.saveErr
	}
	return s.GOBSymbolStore.SaveFileWithSignature(ctx, path, hash, version, symbols, refs)
}

func newInitialScanFixture(t *testing.T, emb *countingEmbedder) (*indexer.Indexer, *indexer.Scanner, *store.GOBStore, string) {
	t.Helper()
	projectRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectRoot, "main.go"), []byte("package main\n\nfunc real() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignoreMatcher, err := indexer.NewIgnoreMatcher(projectRoot, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(projectRoot, ignoreMatcher)
	vectorStore := store.NewGOBStore(filepath.Join(projectRoot, "index.gob"))
	idx := indexer.NewIndexer(projectRoot, vectorStore, emb, indexer.NewChunker(512, 50), scanner, time.Time{})
	return idx, scanner, vectorStore, projectRoot
}

func TestRunInitialScanFingerprintErrorPreventsVectorAndSymbolMutation(t *testing.T) {
	wantErr := errors.New("metadata unavailable")
	emb := &countingEmbedder{}
	idx, scanner, vectorStore, root := newInitialScanFixture(t, emb)
	symbolStore := &startupFingerprintStore{
		GOBSymbolStore: trace.NewGOBSymbolStore(filepath.Join(root, "symbols.gob")),
		listErr:        wantErr,
	}

	_, err := runInitialScan(context.Background(), idx, scanner, trace.NewRegexExtractor(), symbolStore, []string{".go"}, time.Time{}, true, nil, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("runInitialScan error = %v, want %v", err, wantErr)
	}
	if symbolStore.listed != 1 || symbolStore.saved != 0 || emb.embedCalls != 0 || emb.embedBatchCalls != 0 {
		t.Fatalf("listed=%d saved=%d embed=%d batch=%d; metadata failure must precede mutations", symbolStore.listed, symbolStore.saved, emb.embedCalls, emb.embedBatchCalls)
	}
	stats, statsErr := vectorStore.GetStats(context.Background())
	if statsErr != nil || stats.TotalFiles != 0 || stats.TotalChunks != 0 {
		t.Fatalf("vector store mutated after metadata failure: stats=%#v err=%v", stats, statsErr)
	}
}

func TestRunInitialScanFingerprintSnapshotHonorsCancellation(t *testing.T) {
	emb := &countingEmbedder{}
	idx, scanner, _, root := newInitialScanFixture(t, emb)
	symbolStore := &startupFingerprintStore{GOBSymbolStore: trace.NewGOBSymbolStore(filepath.Join(root, "symbols.gob"))}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runInitialScan(ctx, idx, scanner, trace.NewRegexExtractor(), symbolStore, []string{".go"}, time.Time{}, true, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runInitialScan error = %v, want context canceled", err)
	}
	if symbolStore.saved != 0 || emb.embedCalls != 0 || emb.embedBatchCalls != 0 {
		t.Fatalf("saved=%d embed=%d batch=%d; cancellation must precede mutations", symbolStore.saved, emb.embedCalls, emb.embedBatchCalls)
	}
}

func TestRunInitialScanReturnsSymbolSaveFailure(t *testing.T) {
	wantErr := errors.New("symbol save failed")
	emb := &countingEmbedder{}
	idx, scanner, _, root := newInitialScanFixture(t, emb)
	symbolStore := &startupFingerprintStore{
		GOBSymbolStore: trace.NewGOBSymbolStore(filepath.Join(root, "symbols.gob")),
		saveErr:        wantErr,
	}

	_, err := runInitialScan(context.Background(), idx, scanner, trace.NewRegexExtractor(), symbolStore, []string{".go"}, time.Time{}, true, nil, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("runInitialScan error = %v, want %v; startup must not publish ready", err, wantErr)
	}
	if symbolStore.saved != 1 {
		t.Fatalf("save calls = %d, want 1", symbolStore.saved)
	}
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
	// Seed using the new SaveFileWithSignature so the persisted
	// extractor version matches what RegexExtractor.Version() returns;
	// otherwise runInitialScan correctly re-extracts (invalidating the
	// seeded "sentinel") because the cache predates the current
	// extractor version. Plain SaveFileWithContentHash would simulate
	// an old gob file that the dedup deliberately treats as stale.
	extractor := trace.NewRegexExtractor()
	if err := symbolStore.SaveFileWithSignature(ctx, fileInfo.Path, fileInfo.Hash, extractor.Version(), sentinel, nil); err != nil {
		t.Fatalf("failed to seed symbol store: %v", err)
	}

	if _, err := runInitialScan(ctx, idx, scanner, extractor, symbolStore, []string{".go"}, time.Time{}, true, nil, nil); err != nil {
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

	// Seed a document with ChunkIDs so the lastIndexTime gate can skip it.
	// The new logic requires doc != nil && len(doc.ChunkIDs) > 0 to skip.
	if err := vecStore.SaveDocument(ctx, store.Document{
		Path:     "main.go",
		Hash:     "seeded",
		ChunkIDs: []string{"c1"},
	}); err != nil {
		t.Fatalf("failed to seed document: %v", err)
	}

	symbolStore := trace.NewGOBSymbolStore(filepath.Join(projectRoot, "symbols.gob"))
	defer symbolStore.Close()

	// Seed with the current extractor signature: the mtime fast-path only
	// applies when the persisted extractor version still matches. A
	// version-less seed models a pre-upgrade cache and is covered by
	// TestRunInitialScan_ReExtractsWhenMtimeFastPathHitsStaleVersion.
	extractor := trace.NewRegexExtractor()
	if err := symbolStore.SaveFileWithSignature(ctx, "main.go", "seeded", extractor.Version(), []trace.Symbol{
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
	if _, err := runInitialScan(ctx, idx, scanner, extractor, symbolStore, []string{".go"}, lastIndexTime, true, nil, nil); err != nil {
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
		Path:     "main.go",
		Hash:     fileInfo.Hash,
		ModTime:  time.Unix(fileInfo.ModTime, 0),
		ChunkIDs: []string{"c1"},
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
		nil,
		nil,
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
		Path:     prefixedPath,
		Hash:     fileInfo.Hash,
		ModTime:  time.Unix(fileInfo.ModTime, 0),
		ChunkIDs: []string{"c1"},
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
	}, nil, nil)

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
		nil,
		nil,
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
		nil,
		nil,
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

func TestEmitInitialStatsSnapshot_ReportsExistingTotals(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()

	vecStore := store.NewGOBStore(filepath.Join(projectRoot, "index.gob"))
	if err := vecStore.SaveChunks(ctx, []store.Chunk{
		{ID: "chunk-1", FilePath: "main.go"},
		{ID: "chunk-2", FilePath: "main.go"},
	}); err != nil {
		t.Fatalf("failed to seed chunks: %v", err)
	}
	if err := vecStore.SaveDocument(ctx, store.Document{
		Path:     "main.go",
		Hash:     "hash",
		ChunkIDs: []string{"chunk-1", "chunk-2"},
	}); err != nil {
		t.Fatalf("failed to seed document: %v", err)
	}

	symbolStore := trace.NewGOBSymbolStore(filepath.Join(projectRoot, "symbols.gob"))
	defer symbolStore.Close()
	if err := symbolStore.SaveFile(ctx, "main.go", []trace.Symbol{
		{
			Name:     "Foo",
			Kind:     trace.KindFunction,
			File:     "main.go",
			Line:     1,
			Language: "go",
		},
		{
			Name:     "Bar",
			Kind:     trace.KindFunction,
			File:     "main.go",
			Line:     5,
			Language: "go",
		},
	}, nil); err != nil {
		t.Fatalf("failed to seed symbol store: %v", err)
	}

	var got watchStatsDelta
	calls := 0
	emitInitialStatsSnapshot(ctx, vecStore, symbolStore, projectRoot, func(_ string, delta watchStatsDelta) {
		calls++
		got = delta
	})

	if calls != 1 {
		t.Fatalf("stats callback calls = %d, want 1", calls)
	}
	if got.ChunksCreated != 2 {
		t.Fatalf("chunks created = %d, want 2", got.ChunksCreated)
	}
	if got.FilesIndexed != 1 {
		t.Fatalf("files indexed = %d, want 1", got.FilesIndexed)
	}
	if got.SymbolsFound != 2 {
		t.Fatalf("symbols found = %d, want 2", got.SymbolsFound)
	}
	if !got.Snapshot {
		t.Fatal("expected snapshot delta to be marked as Snapshot")
	}
}

// TestRunInitialScan_ReExtractsWhenExtractorVersionDiffers confirms the
// dedup invalidates a cached symbol set when the persisted extractor
// version no longer matches the running extractor — the scenario a
// user hits after upgrading to a release that ships better symbol
// extraction without touching their source files.
func TestRunInitialScan_ReExtractsWhenExtractorVersionDiffers(t *testing.T) {
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

	// Seed with a STALE extractor version. The content hash matches the
	// current file, but the version recorded is a prior release. The
	// dedup must invalidate and re-extract.
	staleSentinel := []trace.Symbol{
		{Name: "stale_sentinel", Kind: trace.KindFunction, File: "main.go", Line: 1, Language: "go"},
	}
	if err := symbolStore.SaveFileWithSignature(ctx, fileInfo.Path, fileInfo.Hash, "regex-vOLD", staleSentinel, nil); err != nil {
		t.Fatalf("failed to seed symbol store: %v", err)
	}

	extractor := trace.NewRegexExtractor()
	if _, err := runInitialScan(ctx, idx, scanner, extractor, symbolStore, []string{".go"}, time.Time{}, true, nil, nil); err != nil {
		t.Fatalf("runInitialScan failed: %v", err)
	}

	// After re-extraction, the stale sentinel must be gone and the real
	// "real" function from the source must be in the store.
	stale, err := symbolStore.LookupSymbol(ctx, "stale_sentinel")
	if err != nil {
		t.Fatalf("LookupSymbol(stale_sentinel) failed: %v", err)
	}
	if len(stale) != 0 {
		t.Errorf("expected stale sentinel to be invalidated, found %d entries", len(stale))
	}

	real, err := symbolStore.LookupSymbol(ctx, "real")
	if err != nil {
		t.Fatalf("LookupSymbol(real) failed: %v", err)
	}
	if len(real) == 0 {
		t.Error("expected real to be re-extracted after version mismatch, got none")
	}
}

// TestRunInitialScan_ReExtractsWhenExtractorVersionIsMissing confirms the
// upgrade path from gob files written by older grepai releases that lack
// the FileExtractorVersions field altogether — the dedup must treat the
// cache as stale and re-extract once.
func TestRunInitialScan_ReExtractsWhenExtractorVersionIsMissing(t *testing.T) {
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

	// Use the legacy SaveFileWithContentHash, which leaves the
	// extractor-version map empty for this file — exactly the shape of
	// gob files produced by older grepai releases.
	if err := symbolStore.SaveFileWithContentHash(ctx, fileInfo.Path, fileInfo.Hash, []trace.Symbol{
		{Name: "stale_sentinel", Kind: trace.KindFunction, File: "main.go", Line: 1, Language: "go"},
	}, nil); err != nil {
		t.Fatalf("failed to seed symbol store: %v", err)
	}

	extractor := trace.NewRegexExtractor()
	if _, err := runInitialScan(ctx, idx, scanner, extractor, symbolStore, []string{".go"}, time.Time{}, true, nil, nil); err != nil {
		t.Fatalf("runInitialScan failed: %v", err)
	}

	stale, err := symbolStore.LookupSymbol(ctx, "stale_sentinel")
	if err != nil {
		t.Fatalf("LookupSymbol(stale_sentinel) failed: %v", err)
	}
	if len(stale) != 0 {
		t.Errorf("expected stale sentinel from version-less cache to be invalidated, found %d entries", len(stale))
	}
}

// TestRunInitialScan_ReExtractsWhenMtimeFastPathHitsStaleVersion is the
// end-to-end upgrade scenario: sources untouched (mtime older than
// lastIndexTime), already tracked in symbols.gob, but the persisted store
// predates the extractor-version metadata. The mtime fast-path must not
// short-circuit — the file has to be re-extracted so the improved symbol
// output becomes visible.
func TestRunInitialScan_ReExtractsWhenMtimeFastPathHitsStaleVersion(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()

	srcPath := filepath.Join(projectRoot, "main.go")
	srcContent := "package main\n\nfunc real() {}\n"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	// A real upgrade sees files whose mtime predates the last index run.
	lastIndexTime := time.Now().Add(-1 * time.Hour)
	oldMtime := lastIndexTime.Add(-1 * time.Hour)
	if err := os.Chtimes(srcPath, oldMtime, oldMtime); err != nil {
		t.Fatalf("failed to backdate source mtime: %v", err)
	}

	ignoreMatcher, err := indexer.NewIgnoreMatcher(projectRoot, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	scanner := indexer.NewScanner(projectRoot, ignoreMatcher)
	chunker := indexer.NewChunker(512, 50)
	vecStore := store.NewGOBStore(filepath.Join(projectRoot, "index.gob"))
	idx := indexer.NewIndexer(projectRoot, vecStore, &noOpEmbedder{}, chunker, scanner, lastIndexTime)

	fileInfo, err := scanner.ScanFile("main.go")
	if err != nil || fileInfo == nil {
		t.Fatalf("failed to scan source file: %v", err)
	}

	// Already embedded, so the vector-index side is a no-op too.
	if err := vecStore.SaveDocument(ctx, store.Document{
		Path:     "main.go",
		Hash:     fileInfo.Hash,
		ChunkIDs: []string{"c1"},
	}); err != nil {
		t.Fatalf("failed to seed document: %v", err)
	}

	symbolStore := trace.NewGOBSymbolStore(filepath.Join(projectRoot, "symbols.gob"))
	if err := symbolStore.Load(ctx); err != nil {
		t.Fatalf("failed to load symbol store: %v", err)
	}
	defer symbolStore.Close()

	// Legacy seed: content hash present, extractor version absent — the
	// shape of a symbols.gob written before this feature existed.
	if err := symbolStore.SaveFileWithContentHash(ctx, fileInfo.Path, fileInfo.Hash, []trace.Symbol{
		{Name: "stale_sentinel", Kind: trace.KindFunction, File: "main.go", Line: 1, Language: "go"},
	}, nil); err != nil {
		t.Fatalf("failed to seed symbol store: %v", err)
	}

	extractor := trace.NewRegexExtractor()
	if _, err := runInitialScan(ctx, idx, scanner, extractor, symbolStore, []string{".go"}, lastIndexTime, true, nil, nil); err != nil {
		t.Fatalf("runInitialScan failed: %v", err)
	}

	stale, err := symbolStore.LookupSymbol(ctx, "stale_sentinel")
	if err != nil {
		t.Fatalf("LookupSymbol(stale_sentinel) failed: %v", err)
	}
	if len(stale) != 0 {
		t.Errorf("expected the version-less cache entry to be invalidated, found %d entries", len(stale))
	}

	real, err := symbolStore.LookupSymbol(ctx, "real")
	if err != nil {
		t.Fatalf("LookupSymbol(real) failed: %v", err)
	}
	if len(real) == 0 {
		t.Error("expected real to be re-extracted despite the mtime fast-path, got none")
	}

	// And the cache must now carry the current signature, so a second
	// scan skips cleanly instead of re-extracting forever.
	if v, ok := symbolStore.GetFileExtractorVersion(fileInfo.Path); !ok || v != extractor.Version() {
		t.Errorf("expected extractor version %q to be persisted, got %q (ok=%v)", extractor.Version(), v, ok)
	}
}
