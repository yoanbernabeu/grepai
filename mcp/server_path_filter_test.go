package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/search"
	storelib "github.com/yoanbernabeu/grepai/store"
)

// MockMCPStore is a mock VectorStore for MCP tests
type MockMCPStore struct {
	chunks map[string]storelib.Chunk
}

func NewMockMCPStore() *MockMCPStore {
	return &MockMCPStore{
		chunks: make(map[string]storelib.Chunk),
	}
}

func (m *MockMCPStore) SaveChunks(ctx context.Context, chunks []storelib.Chunk) error {
	for _, chunk := range chunks {
		m.chunks[chunk.ID] = chunk
	}
	return nil
}

func (m *MockMCPStore) DeleteByFile(ctx context.Context, filePath string) error {
	for id, chunk := range m.chunks {
		if chunk.FilePath == filePath {
			delete(m.chunks, id)
		}
	}
	return nil
}

func (m *MockMCPStore) Search(ctx context.Context, queryVector []float32, limit int, opts storelib.SearchOptions) ([]storelib.SearchResult, error) {
	results := make([]storelib.SearchResult, 0)
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

		results = append(results, storelib.SearchResult{
			Chunk: chunk,
			Score: score,
		})

		if len(results) >= limit && limit > 0 {
			break
		}
	}
	return results, nil
}

func (m *MockMCPStore) GetDocument(ctx context.Context, filePath string) (*storelib.Document, error) {
	return nil, nil
}

func (m *MockMCPStore) SaveDocument(ctx context.Context, doc storelib.Document) error {
	return nil
}

func (m *MockMCPStore) DeleteDocument(ctx context.Context, filePath string) error {
	return nil
}

func (m *MockMCPStore) ListDocuments(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (m *MockMCPStore) Load(ctx context.Context) error {
	return nil
}

func (m *MockMCPStore) Persist(ctx context.Context) error {
	return nil
}

func (m *MockMCPStore) Close() error {
	return nil
}

func (m *MockMCPStore) GetStats(ctx context.Context) (*storelib.IndexStats, error) {
	return nil, nil
}

func (m *MockMCPStore) ListFilesWithStats(ctx context.Context) ([]storelib.FileStats, error) {
	return nil, nil
}

func (m *MockMCPStore) GetChunksForFile(ctx context.Context, filePath string) ([]storelib.Chunk, error) {
	return nil, nil
}

func (m *MockMCPStore) GetAllChunks(ctx context.Context) ([]storelib.Chunk, error) {
	chunks := make([]storelib.Chunk, 0, len(m.chunks))
	for _, chunk := range m.chunks {
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

// MockMCPEmbedder is a mock embedder for MCP tests
type MockMCPEmbedder struct{}

func (m *MockMCPEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{0.9, 0.1, 0.0}, nil
}

func (m *MockMCPEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = []float32{0.9, 0.1, 0.0}
	}
	return result, nil
}

func (m *MockMCPEmbedder) Dimensions() int {
	return 3
}

func (m *MockMCPEmbedder) Close() error {
	return nil
}

// TestMCPSearchWithPathParameter tests the path parameter in MCP search
func TestMCPSearchWithPathParameter(t *testing.T) {
	ctx := context.Background()
	mockStore := NewMockMCPStore()
	embedder := &MockMCPEmbedder{}

	// Setup test chunks
	chunks := []storelib.Chunk{
		{
			ID:        "1",
			FilePath:  "src/handlers/auth.go",
			StartLine: 1,
			EndLine:   10,
			Content:   "func HandleAuth() {}",
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
			FilePath:  "api/v1/routes.go",
			StartLine: 1,
			EndLine:   20,
			Content:   "func SetupRoutes() {}",
			Vector:    []float32{0.85, 0.15, 0.0},
			Hash:      "hash3",
			UpdatedAt: time.Now(),
		},
		{
			ID:        "4",
			FilePath:  "test/unit/auth_test.go",
			StartLine: 1,
			EndLine:   12,
			Content:   "func TestAuth() {}",
			Vector:    []float32{0.7, 0.3, 0.0},
			Hash:      "hash4",
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
	searcher := search.NewSearcher(mockStore, embedder, cfg)

	tests := []struct {
		name       string
		pathPrefix string
		wantCount  int
		wantFiles  []string
	}{
		{
			name:       "no path filter returns all",
			pathPrefix: "",
			wantCount:  4,
		},
		{
			name:       "filter by src/ directory",
			pathPrefix: "src/",
			wantCount:  2,
			wantFiles:  []string{"src/handlers/auth.go", "src/models/user.go"},
		},
		{
			name:       "filter by src/handlers/ subdirectory",
			pathPrefix: "src/handlers/",
			wantCount:  1,
			wantFiles:  []string{"src/handlers/auth.go"},
		},
		{
			name:       "filter by api/ directory",
			pathPrefix: "api/",
			wantCount:  1,
			wantFiles:  []string{"api/v1/routes.go"},
		},
		{
			name:       "filter by test/ directory",
			pathPrefix: "test/",
			wantCount:  1,
			wantFiles:  []string{"test/unit/auth_test.go"},
		},
		{
			name:       "filter with non-existent path",
			pathPrefix: "nonexistent/",
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := searcher.Search(ctx, "test", 10, tt.pathPrefix)
			if err != nil {
				t.Fatalf("search failed: %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
			}

			// Verify expected files
			if len(tt.wantFiles) > 0 {
				resultFiles := make(map[string]bool)
				for _, r := range results {
					resultFiles[r.Chunk.FilePath] = true
				}
				for _, file := range tt.wantFiles {
					if !resultFiles[file] {
						t.Errorf("expected file %s not in results", file)
					}
				}
			}

			// Verify all results match prefix
			for _, result := range results {
				if tt.pathPrefix != "" {
					if len(result.Chunk.FilePath) < len(tt.pathPrefix) ||
						result.Chunk.FilePath[:len(tt.pathPrefix)] != tt.pathPrefix {
						t.Errorf("result %s doesn't match prefix %s", result.Chunk.FilePath, tt.pathPrefix)
					}
				}
			}
		})
	}
}

// TestMCPSearchPathWithLimit tests path filtering combined with result limit
func TestMCPSearchPathWithLimit(t *testing.T) {
	ctx := context.Background()
	mockStore := NewMockMCPStore()
	embedder := &MockMCPEmbedder{}

	// Setup chunks in src/ directory
	chunks := []storelib.Chunk{
		{
			ID:        "1",
			FilePath:  "src/a.go",
			StartLine: 1,
			EndLine:   5,
			Content:   "code a",
			Vector:    []float32{0.9, 0.1, 0.0},
			Hash:      "hash1",
			UpdatedAt: time.Now(),
		},
		{
			ID:        "2",
			FilePath:  "src/b.go",
			StartLine: 1,
			EndLine:   5,
			Content:   "code b",
			Vector:    []float32{0.8, 0.2, 0.0},
			Hash:      "hash2",
			UpdatedAt: time.Now(),
		},
		{
			ID:        "3",
			FilePath:  "src/c.go",
			StartLine: 1,
			EndLine:   5,
			Content:   "code c",
			Vector:    []float32{0.7, 0.3, 0.0},
			Hash:      "hash3",
			UpdatedAt: time.Now(),
		},
		{
			ID:        "4",
			FilePath:  "other/d.go",
			StartLine: 1,
			EndLine:   5,
			Content:   "code d",
			Vector:    []float32{0.6, 0.4, 0.0},
			Hash:      "hash4",
			UpdatedAt: time.Now(),
		},
	}

	if err := mockStore.SaveChunks(ctx, chunks); err != nil {
		t.Fatalf("failed to save chunks: %v", err)
	}

	cfg := config.SearchConfig{
		Boost:  config.BoostConfig{},
		Hybrid: config.HybridConfig{Enabled: false},
	}
	searcher := search.NewSearcher(mockStore, embedder, cfg)

	// Test combining path filter with limit
	results, err := searcher.Search(ctx, "test", 2, "src/")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("with limit=2 and path=src/, got %d results, want 2", len(results))
	}

	// All results should be from src/
	for _, r := range results {
		if len(r.Chunk.FilePath) < 4 || r.Chunk.FilePath[:4] != "src/" {
			t.Errorf("result %s doesn't match src/ prefix", r.Chunk.FilePath)
		}
	}
}
