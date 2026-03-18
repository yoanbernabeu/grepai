package search

import (
	"context"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/store"
)

type roleAwareTestEmbedder struct {
	lastText string
	lastRole embedder.InputRole
}

func (e *roleAwareTestEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	e.lastText = text
	e.lastRole = embedder.RoleGeneric
	return []float32{1, 2, 3}, nil
}

func (e *roleAwareTestEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1, 2, 3}
	}
	return out, nil
}

func (e *roleAwareTestEmbedder) EmbedWithRole(ctx context.Context, text string, role embedder.InputRole) ([]float32, error) {
	e.lastText = text
	e.lastRole = role
	return []float32{1, 2, 3}, nil
}

func (e *roleAwareTestEmbedder) EmbedBatchWithRole(ctx context.Context, texts []string, role embedder.InputRole) ([][]float32, error) {
	e.lastRole = role
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1, 2, 3}
	}
	return out, nil
}

func (e *roleAwareTestEmbedder) Dimensions() int { return 3 }
func (e *roleAwareTestEmbedder) Close() error    { return nil }

type searchStoreStub struct{}

func (s *searchStoreStub) SaveChunks(context.Context, []store.Chunk) error { return nil }
func (s *searchStoreStub) DeleteByFile(context.Context, string) error      { return nil }
func (s *searchStoreStub) Search(context.Context, []float32, int, store.SearchOptions) ([]store.SearchResult, error) {
	return nil, nil
}
func (s *searchStoreStub) GetDocument(context.Context, string) (*store.Document, error) {
	return nil, nil
}
func (s *searchStoreStub) SaveDocument(context.Context, store.Document) error { return nil }
func (s *searchStoreStub) DeleteDocument(context.Context, string) error       { return nil }
func (s *searchStoreStub) ListDocuments(context.Context) ([]string, error)    { return nil, nil }
func (s *searchStoreStub) Load(context.Context) error                         { return nil }
func (s *searchStoreStub) Persist(context.Context) error                      { return nil }
func (s *searchStoreStub) Close() error                                       { return nil }
func (s *searchStoreStub) GetStats(context.Context) (*store.IndexStats, error) {
	return &store.IndexStats{}, nil
}
func (s *searchStoreStub) ListFilesWithStats(context.Context) ([]store.FileStats, error) {
	return nil, nil
}
func (s *searchStoreStub) GetChunksForFile(context.Context, string) ([]store.Chunk, error) {
	return nil, nil
}
func (s *searchStoreStub) GetAllChunks(context.Context) ([]store.Chunk, error) { return nil, nil }

func TestSearchUsesQueryRoleWhenSupported(t *testing.T) {
	emb := &roleAwareTestEmbedder{}
	searcher := NewSearcher(&searchStoreStub{}, emb, config.SearchConfig{})

	_, err := searcher.Search(context.Background(), "llama", 10, "")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if emb.lastRole != embedder.RoleQuery {
		t.Fatalf("expected query role, got %s", emb.lastRole)
	}
	if emb.lastText != "llama" {
		t.Fatalf("expected raw query text, got %q", emb.lastText)
	}
}
