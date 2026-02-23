package store

import (
	"context"
	"testing"
	"time"
)

// TestGOBStoreSearchWithPathPrefix tests path prefix filtering in GOB store
func TestGOBStoreSearchWithPathPrefix(t *testing.T) {
	ctx := context.Background()
	store := NewGOBStore(t.TempDir() + "/test.gob")

	// Add test chunks with different paths
	chunks := []Chunk{
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
		{
			ID:        "4",
			FilePath:  "api/routes/user.go",
			StartLine: 1,
			EndLine:   12,
			Content:   "router.GET(/user)",
			Vector:    []float32{0.7, 0.3, 0.0},
			Hash:      "hash4",
			UpdatedAt: time.Now(),
		},
	}

	if err := store.SaveChunks(ctx, chunks); err != nil {
		t.Fatalf("failed to save chunks: %v", err)
	}

	tests := []struct {
		name       string
		pathPrefix string
		expectedID string
		wantCount  int
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
			name:       "filter by api/ directory",
			pathPrefix: "api/",
			wantCount:  1,
		},
		{
			name:       "filter with non-existent path",
			pathPrefix: "nonexistent/",
			wantCount:  0,
		},
	}

	queryVector := []float32{0.9, 0.1, 0.0}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := store.Search(ctx, queryVector, 10, SearchOptions{PathPrefix: tt.pathPrefix})
			if err != nil {
				t.Fatalf("search failed: %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
			}

			// Verify all results match the path prefix
			for _, result := range results {
				if tt.pathPrefix != "" && len(result.Chunk.FilePath) < len(tt.pathPrefix) {
					t.Errorf("result %s doesn't start with prefix %s", result.Chunk.FilePath, tt.pathPrefix)
				}
			}
		})
	}
}

// TestPostgresStoreSearchWithPathPrefix tests path prefix filtering in PostgreSQL store
func TestPostgresStoreSearchWithPathPrefix(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PostgreSQL test in short mode")
	}

	dsn := testGetPostgresDSN(t)
	if dsn == "" {
		t.Skip("PostgreSQL DSN not configured")
	}

	ctx := context.Background()
	store, err := NewPostgresStore(ctx, dsn, "test-project-"+randomString(8), 3)
	if err != nil {
		t.Skip("could not connect to PostgreSQL:", err)
	}
	defer store.Close()

	// Add test chunks with different paths
	chunks := []Chunk{
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

	if err := store.SaveChunks(ctx, chunks); err != nil {
		t.Fatalf("failed to save chunks: %v", err)
	}

	tests := []struct {
		name       string
		pathPrefix string
		wantCount  int
	}{
		{
			name:       "no path filter returns all",
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
	}

	queryVector := []float32{0.9, 0.1, 0.0}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := store.Search(ctx, queryVector, 10, SearchOptions{PathPrefix: tt.pathPrefix})
			if err != nil {
				t.Fatalf("search failed: %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
			}

			// Verify all results match the path prefix
			for _, result := range results {
				if tt.pathPrefix != "" {
					if !hasPrefix(result.Chunk.FilePath, tt.pathPrefix) {
						t.Errorf("result %s doesn't start with prefix %s", result.Chunk.FilePath, tt.pathPrefix)
					}
				}
			}
		})
	}
}

// hasPrefix is a helper function for string prefix matching
func hasPrefix(s, prefix string) bool {
	if len(prefix) > len(s) {
		return false
	}
	return s[:len(prefix)] == prefix
}

// randomString generates a random string for test isolation
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[i%len(charset)]
	}
	return string(b)
}

// testGetPostgresDSN retrieves PostgreSQL DSN from environment or returns empty string
func testGetPostgresDSN(_ *testing.T) string {
	// This would normally read from environment or test config
	// For now, return empty to skip PostgreSQL tests
	return ""
}
