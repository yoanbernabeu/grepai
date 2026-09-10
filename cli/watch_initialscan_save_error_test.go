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

type failingInitialSaveStore struct {
	trace.SymbolStore
	source       trace.FileFingerprintSource
	err          error
	persistCalls int
}

func (s *failingInitialSaveStore) ListFileFingerprints(ctx context.Context) (map[string]trace.FileFingerprint, error) {
	return s.source.ListFileFingerprints(ctx)
}

func (s *failingInitialSaveStore) SaveFile(context.Context, string, []trace.Symbol, []trace.Reference) error {
	return s.err
}

func (s *failingInitialSaveStore) Persist(context.Context) error {
	s.persistCalls++
	return nil
}

type failingInitialSignatureStore struct{ *failingInitialSaveStore }

func (s *failingInitialSignatureStore) SaveFileWithSignature(context.Context, string, string, string, []trace.Symbol, []trace.Reference) error {
	return s.err
}

func TestRunInitialScanReturnsSymbolSaveErrors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		signature bool
	}{
		{name: "SaveFile", signature: false},
		{name: "SaveFileWithSignature", signature: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc Saved() {}\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			scanner := indexer.NewScanner(root, ignore)
			vectorStore := store.NewGOBStore(filepath.Join(root, "index.gob"))
			idx := indexer.NewIndexer(root, vectorStore, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
			underlying := trace.NewGOBSymbolStore(filepath.Join(root, "symbols.gob"))
			wantErr := errors.New("symbol save failed")
			base := &failingInitialSaveStore{SymbolStore: underlying, source: underlying, err: wantErr}
			var symbolStore trace.SymbolStore = base
			if tc.signature {
				symbolStore = &failingInitialSignatureStore{failingInitialSaveStore: base}
			}

			stats, err := runInitialScan(ctx, idx, scanner, trace.NewRegexExtractor(), symbolStore, []string{".go"}, time.Time{}, true, nil, nil)
			if !errors.Is(err, wantErr) {
				t.Fatalf("error = %v, want wrapped %v", err, wantErr)
			}
			if stats != nil {
				t.Fatalf("successful stats returned after symbol save failure: %+v", stats)
			}
			if base.persistCalls != 0 {
				t.Fatalf("Persist calls = %d, want 0 after save failure", base.persistCalls)
			}
			if underlying.IsFileIndexed("main.go") {
				t.Fatal("failed symbol save mutated the underlying store")
			}
		})
	}
}
