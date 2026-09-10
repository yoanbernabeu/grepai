package cli

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
)

type emptyCreatingMetadataStore struct {
	*store.GOBStore
	once   sync.Once
	create func() error
}

func (s *emptyCreatingMetadataStore) ListDocumentMetadata(context.Context) ([]store.DocumentMetadata, error) {
	var err error
	s.once.Do(func() { err = s.create() })
	return nil, err
}

func TestSymbolOnlyRemainingPathIsFreshlyRecovered(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := "created.go"
	content := "package created\nfunc Current() {}\n"
	base := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	st := &emptyCreatingMetadataStore{GOBStore: base, create: func() error {
		return os.WriteFile(filepath.Join(root, path), []byte(content), 0o644)
	}}
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore)
	symbols := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	if err := symbols.SaveFileWithSignature(ctx, path, "old", "version", []trace.Symbol{{Name: "Old", File: path}}, nil); err != nil {
		t.Fatal(err)
	}
	idx := indexer.NewIndexer(root, st, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Time{})
	stats, err := runInitialScan(ctx, idx, scanner, trace.NewRegexExtractor(), symbols, []string{".go"}, time.Time{}, true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesIndexed != 1 || len(stats.ScannedFiles) != 1 || stats.ScannedFiles[0].Path != path {
		t.Fatalf("indexed=%d scanned=%v", stats.FilesIndexed, stats.ScannedFiles)
	}
	if doc, err := st.GetDocument(ctx, path); err != nil || doc == nil || doc.Hash == "" || doc.Hash == "old" || len(doc.ChunkIDs) == 0 {
		t.Fatalf("vector document=%+v err=%v", doc, err)
	}
	if old, _ := symbols.LookupSymbol(ctx, "Old"); len(old) != 0 {
		t.Fatalf("old symbols=%v", old)
	}
	if current, _ := symbols.LookupSymbol(ctx, "Current"); len(current) != 1 {
		t.Fatalf("current symbols=%v", current)
	}
}
