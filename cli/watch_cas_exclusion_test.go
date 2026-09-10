package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
)

type casMutationVectorStore struct {
	*store.GOBStore
	once   sync.Once
	mutate func() error
}

func (s *casMutationVectorStore) RefreshDocumentModTime(ctx context.Context, path, expectedHash string, modTime time.Time) (bool, error) {
	var mutationErr error
	ran := false
	s.once.Do(func() {
		ran = true
		mutationErr = s.mutate()
	})
	if mutationErr != nil || ran {
		return false, mutationErr
	}
	return s.GOBStore.RefreshDocumentModTime(ctx, path, expectedHash, modTime)
}

func TestCASRetryExclusionsRemoveVectorAndSymbolRows(t *testing.T) {
	for _, tc := range []struct {
		name   string
		path   string
		mutate func(t *testing.T, path string) func() error
	}{
		{name: "binary", path: "binary.go", mutate: func(_ *testing.T, path string) func() error {
			return func() error { return os.WriteFile(path, []byte{'p', 'k', 'g', 0}, 0o644) }
		}},
		{name: "too-large", path: "large.go", mutate: func(_ *testing.T, path string) func() error {
			return func() error { return os.WriteFile(path, []byte(strings.Repeat("x", 1*1024*1024+1)), 0o644) }
		}},
		{name: "minified", path: "source.cas-minified.go", mutate: func(t *testing.T, _ string) func() error {
			return func() error {
				original := append([]string(nil), indexer.MinifiedPatterns...)
				indexer.MinifiedPatterns = append(indexer.MinifiedPatterns, ".cas-minified.go")
				t.Cleanup(func() { indexer.MinifiedPatterns = original })
				return nil
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			absolutePath := filepath.Join(root, tc.path)
			if err := os.WriteFile(absolutePath, []byte("package source\nfunc Existing() {}\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			scanner := indexer.NewScanner(root, ignore)
			initial, err := scanner.ScanFile(tc.path)
			if err != nil || initial == nil {
				t.Fatalf("initial snapshot=%v err=%v", initial, err)
			}
			base := store.NewGOBStore(filepath.Join(root, "index.gob"))
			vectorStore := &casMutationVectorStore{GOBStore: base, mutate: tc.mutate(t, absolutePath)}
			if err := vectorStore.SaveDocument(ctx, store.Document{Path: tc.path, Hash: initial.Hash, ChunkIDs: []string{"old-chunk"}}); err != nil {
				t.Fatal(err)
			}
			symbolStore := trace.NewGOBSymbolStore(filepath.Join(root, "symbols.gob"))
			if err := symbolStore.SaveFileWithSignature(ctx, tc.path, initial.Hash, "old-version", nil, nil); err != nil {
				t.Fatal(err)
			}
			idx := indexer.NewIndexer(root, vectorStore, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
			stats, err := runInitialScan(ctx, idx, scanner, trace.NewRegexExtractor(), symbolStore, []string{".go"}, time.Time{}, true, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if stats.FilesRemoved != 1 {
				t.Fatalf("removed=%d, want stale vector row removed", stats.FilesRemoved)
			}
			if doc, err := vectorStore.GetDocument(ctx, tc.path); err != nil || doc != nil {
				t.Fatalf("vector row persisted: doc=%v err=%v", doc, err)
			}
			if symbolStore.IsFileIndexed(tc.path) {
				t.Fatal("symbol row persisted after CAS exclusion")
			}
		})
	}
}
