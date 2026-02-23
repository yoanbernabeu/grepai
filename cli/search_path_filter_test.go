package cli

import (
	"context"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/search"
	"github.com/yoanbernabeu/grepai/store"
)

// MockStore is a test mock for VectorStore
type MockStore struct {
	chunks map[string]store.Chunk
}

func NewMockStore() *MockStore {
	return &MockStore{
		chunks: make(map[string]store.Chunk),
	}
}

func (m *MockStore) SaveChunks(ctx context.Context, chunks []store.Chunk) error {
	for _, chunk := range chunks {
		m.chunks[chunk.ID] = chunk
	}
	return nil
}

func (m *MockStore) DeleteByFile(ctx context.Context, filePath string) error {
	for id, chunk := range m.chunks {
		if chunk.FilePath == filePath {
			delete(m.chunks, id)
		}
	}
	return nil
}

func (m *MockStore) Search(ctx context.Context, queryVector []float32, limit int, opts store.SearchOptions) ([]store.SearchResult, error) {
	results := make([]store.SearchResult, 0)
	for _, chunk := range m.chunks {
		// Filter by path prefix if provided
		if opts.PathPrefix != "" && len(chunk.FilePath) < len(opts.PathPrefix) {
			continue
		}
		if opts.PathPrefix != "" && chunk.FilePath[:len(opts.PathPrefix)] != opts.PathPrefix {
			continue
		}

		// Simple similarity score based on vector
		score := float32(0)
		for i := range queryVector {
			if i < len(chunk.Vector) {
				score += queryVector[i] * chunk.Vector[i]
			}
		}

		results = append(results, store.SearchResult{
			Chunk: chunk,
			Score: score,
		})

		if len(results) >= limit && limit > 0 {
			break
		}
	}
	return results, nil
}

func (m *MockStore) GetDocument(ctx context.Context, filePath string) (*store.Document, error) {
	return nil, nil
}

func (m *MockStore) SaveDocument(ctx context.Context, doc store.Document) error {
	return nil
}

func (m *MockStore) DeleteDocument(ctx context.Context, filePath string) error {
	return nil
}

func (m *MockStore) ListDocuments(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (m *MockStore) Load(ctx context.Context) error {
	return nil
}

func (m *MockStore) Persist(ctx context.Context) error {
	return nil
}

func (m *MockStore) Close() error {
	return nil
}

func (m *MockStore) GetStats(ctx context.Context) (*store.IndexStats, error) {
	return nil, nil
}

func (m *MockStore) ListFilesWithStats(ctx context.Context) ([]store.FileStats, error) {
	return nil, nil
}

func (m *MockStore) GetChunksForFile(ctx context.Context, filePath string) ([]store.Chunk, error) {
	return nil, nil
}

func (m *MockStore) GetAllChunks(ctx context.Context) ([]store.Chunk, error) {
	chunks := make([]store.Chunk, 0, len(m.chunks))
	for _, chunk := range m.chunks {
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

// MockEmbedder is a test mock for Embedder
type MockEmbedder struct{}

func (m *MockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	// Return a simple constant vector for testing
	return []float32{0.9, 0.1, 0.0}, nil
}

func (m *MockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	// Return constant vectors for all texts
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = []float32{0.9, 0.1, 0.0}
	}
	return result, nil
}

func (m *MockEmbedder) Dimensions() int {
	return 3
}

func (m *MockEmbedder) Close() error {
	return nil
}

// TestSearcherWithPathPrefix tests the searcher integration with path prefix filtering
func TestSearcherWithPathPrefix(t *testing.T) {
	ctx := context.Background()
	mockStore := NewMockStore()
	mockEmbedder := &MockEmbedder{}

	// Add test chunks
	chunks := []store.Chunk{
		{
			ID:        "1",
			FilePath:  "src/handlers/user.go",
			StartLine: 1,
			EndLine:   10,
			Content:   "func HandleUser() {}",
			Vector:    []float32{0.9, 0.1, 0.0},
			Hash:      "hash1",
			UpdatedAt: time.Now(),
		},
		{
			ID:        "2",
			FilePath:  "src/models/user.go",
			StartLine: 1,
			EndLine:   15,
			Content:   "type User struct {}",
			Vector:    []float32{0.8, 0.2, 0.0},
			Hash:      "hash2",
			UpdatedAt: time.Now(),
		},
		{
			ID:        "3",
			FilePath:  "test/user_test.go",
			StartLine: 1,
			EndLine:   20,
			Content:   "func TestUser() {}",
			Vector:    []float32{0.85, 0.15, 0.0},
			Hash:      "hash3",
			UpdatedAt: time.Now(),
		},
	}

	if err := mockStore.SaveChunks(ctx, chunks); err != nil {
		t.Fatalf("failed to save chunks: %v", err)
	}

	// Create searcher
	cfg := config.SearchConfig{
		Boost:  config.BoostConfig{},
		Hybrid: config.HybridConfig{Enabled: false},
	}
	searcher := search.NewSearcher(mockStore, mockEmbedder, cfg)

	tests := []struct {
		name       string
		pathPrefix string
		wantCount  int
	}{
		{
			name:       "no path filter returns all results",
			pathPrefix: "",
			wantCount:  3,
		},
		{
			name:       "filter by src/ directory",
			pathPrefix: "src/",
			wantCount:  2,
		},
		{
			name:       "filter by src/handlers/ subdirectory",
			pathPrefix: "src/handlers/",
			wantCount:  1,
		},
		{
			name:       "filter by test/ directory",
			pathPrefix: "test/",
			wantCount:  1,
		},
		{
			name:       "filter with non-existent path",
			pathPrefix: "nonexistent/",
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := searcher.Search(ctx, "test query", 10, tt.pathPrefix)
			if err != nil {
				t.Fatalf("search failed: %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
			}

			// Verify all results match the path prefix
			for _, result := range results {
				if tt.pathPrefix != "" {
					if len(result.Chunk.FilePath) < len(tt.pathPrefix) ||
						result.Chunk.FilePath[:len(tt.pathPrefix)] != tt.pathPrefix {
						t.Errorf("result %s doesn't start with prefix %s", result.Chunk.FilePath, tt.pathPrefix)
					}
				}
			}
		})
	}
}
