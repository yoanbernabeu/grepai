package indexer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

type noRefreshMutatingStore struct {
	store.VectorStore
	source       store.DocumentMetadataSource
	once         sync.Once
	mutate       func() error
	mutationErr  error
	mutateOnList bool
}

func (s *noRefreshMutatingStore) ListDocumentMetadata(ctx context.Context) ([]store.DocumentMetadata, error) {
	metadata, err := s.source.ListDocumentMetadata(ctx)
	if err == nil && s.mutateOnList {
		s.once.Do(func() { s.mutationErr = s.mutate() })
		err = s.mutationErr
	}
	return metadata, err
}

func (s *noRefreshMutatingStore) GetDocument(ctx context.Context, path string) (*store.Document, error) {
	s.once.Do(func() { s.mutationErr = s.mutate() })
	if s.mutationErr != nil {
		return nil, s.mutationErr
	}
	return s.VectorStore.GetDocument(ctx, path)
}

type unsupportedRefreshMutatingStore struct{ *noRefreshMutatingStore }

type unsupportedCompleteStore struct {
	store.VectorStore
	source store.DocumentMetadataSource
}

func (s *unsupportedCompleteStore) ListDocumentMetadata(ctx context.Context) ([]store.DocumentMetadata, error) {
	return s.source.ListDocumentMetadata(ctx)
}

func (*unsupportedCompleteStore) GetCompleteDocument(context.Context, string) (*store.Document, error) {
	return nil, store.ErrCompleteDocumentUnsupported
}

func (s *unsupportedRefreshMutatingStore) ListDocumentMetadata(ctx context.Context) ([]store.DocumentMetadata, error) {
	return s.source.ListDocumentMetadata(ctx)
}

func (s *unsupportedRefreshMutatingStore) GetCompleteDocument(ctx context.Context, path string) (*store.Document, error) {
	s.once.Do(func() { s.mutationErr = s.mutate() })
	if s.mutationErr != nil {
		return nil, s.mutationErr
	}
	return s.VectorStore.(store.CompleteDocumentSource).GetCompleteDocument(ctx, path)
}

func (*unsupportedRefreshMutatingStore) RefreshDocumentModTime(context.Context, string, string, time.Time) (bool, error) {
	return false, store.ErrRefreshUnsupported
}

func TestRecoveredDeletedRecordWithoutRefreshCapabilityUsesFreshSource(t *testing.T) {
	ctx, root, _, scanner, base, _ := setupMutatingRecovery(t)
	relative := filepath.Join("linked", "a.go")
	st := &noRefreshMutatingStore{VectorStore: base, source: base, mutateOnList: true}
	st.mutate = func() error {
		if err := base.DeleteByFile(ctx, relative); err != nil {
			return err
		}
		return base.DeleteDocument(ctx, relative)
	}
	embedder := newMockEmbedder()
	stats, err := NewIndexer(root, st, embedder, NewChunker(512, 50), scanner, time.Now()).IndexAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesIndexed != 1 || !embedder.embedCalled {
		t.Fatalf("indexed=%d embedCalled=%v", stats.FilesIndexed, embedder.embedCalled)
	}
	doc, err := base.GetDocument(ctx, relative)
	if err != nil || doc == nil || len(doc.ChunkIDs) == 0 {
		t.Fatalf("recreated document=%+v err=%v", doc, err)
	}
}

func TestRecoveredUnsupportedCompleteCapabilityIndexesConservatively(t *testing.T) {
	ctx, root, _, scanner, base, _ := setupMutatingRecovery(t)
	st := &unsupportedCompleteStore{VectorStore: base, source: base}
	embedder := newMockEmbedder()
	stats, err := NewIndexer(root, st, embedder, NewChunker(512, 50), scanner, time.Now()).IndexAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesIndexed != 1 || !embedder.embedCalled {
		t.Fatalf("indexed=%d embedCalled=%v", stats.FilesIndexed, embedder.embedCalled)
	}
}

func TestRecoveredUnsupportedCompleteClassifiesNonRegularLeaf(t *testing.T) {
	for _, kind := range []string{"directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			path := filepath.Join(root, "leaf.go")
			if kind == "directory" {
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				target := filepath.Join(t.TempDir(), "target.go")
				if err := os.WriteFile(target, []byte("package target\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			}
			ignore, err := NewIgnoreMatcher(root, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			base := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
			st := &unsupportedCompleteStore{VectorStore: base, source: base}
			idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), NewScanner(root, ignore), time.Now())
			decision, err := idx.reconcileRecoveredCandidate(ctx, &FileInfo{Path: "leaf.go", Hash: "old"}, store.DocumentMetadata{Path: "leaf.go", Hash: "old", HasChunks: true})
			if err != nil || !decision.excluded || decision.file != nil || decision.verified != nil {
				t.Fatalf("decision=%+v err=%v", decision, err)
			}
		})
	}
}

func TestRecoveredUnsupportedCompletePropagatesInspectionError(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	if err := os.WriteFile(path, []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(root, ignore)
	want := errors.New("read failed")
	scanner.readSnapshot = func(string, string) (*FileInfo, error) { return nil, want }
	base := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	st := &unsupportedCompleteStore{VectorStore: base, source: base}
	idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), scanner, time.Now())
	_, err = idx.reconcileRecoveredCandidate(ctx, &FileInfo{Path: "a.go", Hash: "old"}, store.DocumentMetadata{Path: "a.go", Hash: "old", HasChunks: true})
	if !errors.Is(err, want) {
		t.Fatalf("error=%v, want %v", err, want)
	}
}

func TestRecoveredReplacedRecordWithUnsupportedRefreshUsesLatestState(t *testing.T) {
	ctx, root, absolute, scanner, base, _ := setupMutatingRecovery(t)
	relative := filepath.Join("linked", "a.go")
	hidden := &noRefreshMutatingStore{VectorStore: base, source: base}
	hidden.mutate = func() error {
		if err := os.WriteFile(absolute, []byte("package latest\n"), 0o644); err != nil {
			return err
		}
		latest, err := scanner.ScanFile(relative)
		if err != nil {
			return err
		}
		if err := base.DeleteByFile(ctx, relative); err != nil {
			return err
		}
		if err := base.SaveChunks(ctx, []store.Chunk{{ID: "latest", FilePath: relative, Vector: []float32{4, 5, 6}}}); err != nil {
			return err
		}
		return base.SaveDocument(ctx, store.Document{Path: relative, Hash: latest.Hash, ModTime: latest.ObservedModTime, HasExactModTime: true, ChunkIDs: []string{"latest"}})
	}
	st := &unsupportedRefreshMutatingStore{noRefreshMutatingStore: hidden}
	embedder := newMockEmbedder()
	stats, err := NewIndexer(root, st, embedder, NewChunker(512, 50), scanner, time.Now()).IndexAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := scanner.ScanFile(relative)
	if err != nil {
		t.Fatal(err)
	}
	verified := stats.VerifiedUnchangedFiles[relative]
	if verified.Hash != latest.Hash || verified.Size != latest.Size || !verified.ModTime.Equal(latest.ObservedModTime) {
		t.Fatalf("verified=%+v latest=%+v", verified, latest)
	}
	if stats.FilesIndexed != 0 || embedder.embedCalled {
		t.Fatalf("indexed=%d embedCalled=%v", stats.FilesIndexed, embedder.embedCalled)
	}
	doc, err := base.GetDocument(ctx, relative)
	if err != nil || doc == nil || doc.Hash != latest.Hash || len(doc.ChunkIDs) != 1 || doc.ChunkIDs[0] != "latest" {
		t.Fatalf("document=%+v err=%v", doc, err)
	}
}
