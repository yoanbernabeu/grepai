package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
)

// cancelAfterSymbolSaveStore cancels the run context as soon as the final
// expected symbol save completes, then records whether Persist was still
// invoked. GOB Persist does not itself honor a canceled context, so only an
// explicit cancellation guard in runInitialScan can keep a canceled startup
// from flushing symbols that were saved moments before shutdown.
type cancelAfterSymbolSaveStore struct {
	trace.SymbolStore
	cancel       context.CancelFunc
	saves        int
	persistCalls int
}

func (s *cancelAfterSymbolSaveStore) SaveFileWithSignature(ctx context.Context, filePath, contentHash, extractorVersion string, symbols []trace.Symbol, refs []trace.Reference) error {
	saver, ok := s.SymbolStore.(signatureSaver)
	if !ok {
		return errors.New("wrapped store lacks SaveFileWithSignature")
	}
	if err := saver.SaveFileWithSignature(ctx, filePath, contentHash, extractorVersion, symbols, refs); err != nil {
		return err
	}
	s.saves++
	s.cancel()
	return nil
}

func (s *cancelAfterSymbolSaveStore) ListFileFingerprints(ctx context.Context) (map[string]trace.FileFingerprint, error) {
	source, ok := s.SymbolStore.(trace.FileFingerprintSource)
	if !ok {
		return nil, trace.ErrFileFingerprintsUnsupported
	}
	return source.ListFileFingerprints(ctx)
}

func (s *cancelAfterSymbolSaveStore) Persist(ctx context.Context) error {
	s.persistCalls++
	return s.SymbolStore.Persist(ctx)
}

func TestRunInitialScan_SkipsSymbolPersistWhenCancelledAfterLastSave(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	projectRoot := t.TempDir()
	srcPath := filepath.Join(projectRoot, "main.go")
	if err := os.WriteFile(srcPath, []byte("package main\n\nfunc real() {}\n"), 0644); err != nil {
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

	base := trace.NewGOBSymbolStore(filepath.Join(projectRoot, "symbols.gob"))
	if err := base.Load(ctx); err != nil {
		t.Fatalf("failed to load symbol store: %v", err)
	}
	defer base.Close()

	symbolStore := &cancelAfterSymbolSaveStore{SymbolStore: base, cancel: cancel}
	extractor := trace.NewRegexExtractor()

	_, err = runInitialScan(ctx, idx, scanner, extractor, symbolStore, []string{".go"}, time.Time{}, true, nil, nil)
	if err == nil {
		t.Fatal("expected cancellation error from runInitialScan")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
	if symbolStore.saves == 0 {
		t.Fatal("expected at least one symbol save before cancellation")
	}
	if symbolStore.persistCalls != 0 {
		t.Fatalf("symbol Persist called %d times after context cancellation", symbolStore.persistCalls)
	}
}
