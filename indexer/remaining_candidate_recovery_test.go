package indexer

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

type creatingMetadataStore struct {
	*store.GOBStore
	once   sync.Once
	create func() error
}

func (s *creatingMetadataStore) ListDocumentMetadata(ctx context.Context) ([]store.DocumentMetadata, error) {
	metadata, err := s.GOBStore.ListDocumentMetadata(ctx)
	if err == nil {
		s.once.Do(func() { err = s.create() })
	}
	return metadata, err
}

func TestRemainingEligibleCandidateIsFreshlyIndexed(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := "created.go"
	content := "package created\nfunc Current() {}\n"
	base := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	if err := base.SaveChunks(ctx, []store.Chunk{{ID: "old", FilePath: path}}); err != nil {
		t.Fatal(err)
	}
	if err := base.SaveDocument(ctx, store.Document{Path: path, Hash: "old", ChunkIDs: []string{"old"}}); err != nil {
		t.Fatal(err)
	}
	st := &creatingMetadataStore{GOBStore: base, create: func() error {
		return os.WriteFile(filepath.Join(root, path), []byte(content), 0o644)
	}}
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), NewScanner(root, ignore), time.Time{})
	stats, err := idx.IndexAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesIndexed != 1 || len(stats.ScannedFiles) != 1 || stats.ScannedFiles[0].Path != path {
		t.Fatalf("indexed=%d scanned=%v", stats.FilesIndexed, stats.ScannedFiles)
	}
	doc, err := st.GetDocument(ctx, path)
	if err != nil || doc == nil || doc.Hash == "old" || len(doc.ChunkIDs) == 0 || doc.ChunkIDs[0] == "old" {
		t.Fatalf("document=%+v err=%v", doc, err)
	}
	_, chunks := st.Stats()
	if chunks == 0 {
		t.Fatal("fresh document has no chunks")
	}
}
