package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
)

type deletingMetadataStore struct {
	*store.GOBStore
	once   sync.Once
	delete func()
}

func (s *deletingMetadataStore) ListDocumentMetadata(ctx context.Context) ([]store.DocumentMetadata, error) {
	metadata, err := s.GOBStore.ListDocumentMetadata(ctx)
	if err == nil {
		s.once.Do(s.delete)
	}
	return metadata, err
}

func TestRunInitialScanReconcilesFileDeletedAfterMetadata(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := "deleted.go"
	absolute := filepath.Join(root, path)
	if err := os.WriteFile(absolute, []byte("package deleted\nfunc Target() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore)
	initial, err := scanner.ScanFile(path)
	if err != nil || initial == nil {
		t.Fatalf("initial snapshot=%v err=%v", initial, err)
	}
	baseVectorStore := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	vectorStore := &deletingMetadataStore{GOBStore: baseVectorStore, delete: func() { _ = os.Remove(absolute) }}
	if err := vectorStore.SaveDocument(ctx, store.Document{Path: path, Hash: initial.Hash, ChunkIDs: []string{"old-chunk"}}); err != nil {
		t.Fatal(err)
	}
	symbolStore := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	symbols := []trace.Symbol{{Name: "Target", Kind: trace.KindFunction, File: path, Line: 2}}
	refs := []trace.Reference{{SymbolName: "Target", File: path, Line: 3, CallerName: "Caller"}}
	if err := symbolStore.SaveFileWithSignature(ctx, path, initial.Hash, "old-version", symbols, refs); err != nil {
		t.Fatal(err)
	}
	idx := indexer.NewIndexer(root, vectorStore, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
	stats, err := runInitialScan(ctx, idx, scanner, trace.NewRegexExtractor(), symbolStore, []string{".go"}, time.Time{}, true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesRemoved != 1 {
		t.Fatalf("removed=%d, want 1", stats.FilesRemoved)
	}
	if len(stats.ScannedFiles) != 0 {
		t.Fatalf("known-missing paths remained consumer-visible: %v", stats.ScannedFiles)
	}
	assertMissingRows(t, ctx, vectorStore, symbolStore, path)
}

func TestRunInitialScanPreservesRowsWhenRootDisappearsAfterMetadata(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := "kept.go"
	absolute := filepath.Join(root, path)
	if err := os.WriteFile(absolute, []byte("package kept\nfunc Target() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore)
	initial, err := scanner.ScanFile(path)
	if err != nil || initial == nil {
		t.Fatalf("initial snapshot=%v err=%v", initial, err)
	}
	baseVectorStore := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	vectorStore := &deletingMetadataStore{GOBStore: baseVectorStore, delete: func() { _ = os.RemoveAll(root) }}
	if err := vectorStore.SaveDocument(ctx, store.Document{Path: path, Hash: initial.Hash, ChunkIDs: []string{"old-chunk"}}); err != nil {
		t.Fatal(err)
	}
	symbolStore := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	if err := symbolStore.SaveFileWithSignature(ctx, path, initial.Hash, "old-version", []trace.Symbol{{Name: "Target", File: path}}, nil); err != nil {
		t.Fatal(err)
	}
	idx := indexer.NewIndexer(root, vectorStore, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
	stats, err := runInitialScan(ctx, idx, scanner, trace.NewRegexExtractor(), symbolStore, []string{".go"}, time.Time{}, true, nil, nil)
	if err == nil || stats != nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stats=%v err=%v, want root-loss failure", stats, err)
	}
	if doc, err := vectorStore.GetDocument(ctx, path); err != nil || doc == nil {
		t.Fatalf("vector row removed with root: doc=%v err=%v", doc, err)
	}
	if !symbolStore.IsFileIndexed(path) {
		t.Fatal("symbol row removed with root")
	}
}

func TestIndexInitialSymbolsReconcilesObservedENOENT(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := "vanished.go"
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore)
	vectorStore := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	if err := vectorStore.SaveDocument(ctx, store.Document{Path: path, Hash: "old", ChunkIDs: []string{"old-chunk"}}); err != nil {
		t.Fatal(err)
	}
	symbolStore := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	if err := symbolStore.SaveFileWithSignature(ctx, path, "old", "old-version",
		[]trace.Symbol{{Name: "Target", File: path}},
		[]trace.Reference{{SymbolName: "Target", File: path, CallerName: "Caller"}},
	); err != nil {
		t.Fatal(err)
	}
	idx := indexer.NewIndexer(root, vectorStore, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
	count, changes, err := indexInitialSymbols(ctx, idx, scanner, trace.NewRegexExtractor(), symbolStore,
		initialSymbolFingerprints{}, []indexer.FileMeta{{Path: path}}, nil, []string{".go"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || len(changes.removed) != 1 || changes.removed[0] != path {
		t.Fatalf("count=%d changes=%v", count, changes)
	}
	assertMissingRows(t, ctx, vectorStore, symbolStore, path)
}

func assertMissingRows(t *testing.T, ctx context.Context, vectorStore store.VectorStore, symbolStore trace.SymbolStore, path string) {
	t.Helper()
	if doc, err := vectorStore.GetDocument(ctx, path); err != nil || doc != nil {
		t.Fatalf("vector row persisted: doc=%v err=%v", doc, err)
	}
	if symbolStore.IsFileIndexed(path) {
		t.Fatal("symbol file marker persisted")
	}
	if symbols, err := symbolStore.GetSymbolsForFile(ctx, path); err != nil || len(symbols) != 0 {
		t.Fatalf("symbols=%v err=%v", symbols, err)
	}
	if refs, err := symbolStore.LookupCallers(ctx, "Target"); err != nil || len(refs) != 0 {
		t.Fatalf("references=%v err=%v", refs, err)
	}
}

func TestRemoveMissingDuringSymbolScanRejectsReappearedPath(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := "reappeared.go"
	if err := os.WriteFile(filepath.Join(root, path), []byte("package reappeared\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore)
	vectors := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	if err := vectors.SaveDocument(ctx, store.Document{Path: path, Hash: "old"}); err != nil {
		t.Fatal(err)
	}
	symbols := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	if err := symbols.SaveFileWithSignature(ctx, path, "old", "version", nil, nil); err != nil {
		t.Fatal(err)
	}
	idx := indexer.NewIndexer(root, vectors, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
	removed, err := removeFileMissingDuringSymbolScan(ctx, idx, scanner, symbols, path)
	if err == nil || removed {
		t.Fatalf("removed=%v err=%v, want changed-state error", removed, err)
	}
	if doc, _ := vectors.GetDocument(ctx, path); doc == nil || !symbols.IsFileIndexed(path) {
		t.Fatal("reappeared-path error changed ownership")
	}
}

func TestEmptySymbolMissingResolutionRejectsReappearedPath(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := "reappeared.go"
	if err := os.WriteFile(filepath.Join(root, path), []byte("package reappeared\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore)
	vectors := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	symbols := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	idx := indexer.NewIndexer(root, vectors, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
	inspect := func(string) (*indexer.FileInfo, indexer.PathExclusionReason, error) {
		return nil, "", &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
	resolution, err := reconcileEmptySymbolScanWithInspect(ctx, idx, scanner, symbols, path, inspect)
	if err == nil || resolution.file != nil || resolution.removed {
		t.Fatalf("resolution=%+v err=%v, want changed-state error", resolution, err)
	}
}
