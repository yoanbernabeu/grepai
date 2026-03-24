package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSearch_WithEmbedModelFilter_ReturnsOnlyMatching(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.gob")

	s := NewGOBStore(indexPath)
	ctx := context.Background()

	chunks := []Chunk{
		{
			ID:         "c1",
			FilePath:   "a.go",
			StartLine:  1,
			EndLine:    10,
			Content:    "func A() {}",
			Vector:     []float32{1.0, 0.0, 0.0},
			Hash:       "h1",
			EmbedModel: "ollama/nomic-embed-text",
			UpdatedAt:  time.Now(),
		},
		{
			ID:         "c2",
			FilePath:   "b.go",
			StartLine:  1,
			EndLine:    10,
			Content:    "func B() {}",
			Vector:     []float32{0.9, 0.1, 0.0},
			Hash:       "h2",
			EmbedModel: "openai/text-embedding-3-small",
			UpdatedAt:  time.Now(),
		},
		{
			ID:         "c3",
			FilePath:   "c.go",
			StartLine:  1,
			EndLine:    10,
			Content:    "func C() {}",
			Vector:     []float32{0.8, 0.2, 0.0},
			Hash:       "h3",
			EmbedModel: "ollama/nomic-embed-text",
			UpdatedAt:  time.Now(),
		},
	}

	if err := s.SaveChunks(ctx, chunks); err != nil {
		t.Fatalf("failed to save chunks: %v", err)
	}

	// Filter for ollama model only
	results, err := s.Search(ctx, []float32{1.0, 0.0, 0.0}, 10, SearchOptions{
		EmbedModel: "ollama/nomic-embed-text",
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results for ollama model, got %d", len(results))
	}

	for _, r := range results {
		if r.Chunk.EmbedModel != "ollama/nomic-embed-text" {
			t.Errorf("expected all results to have EmbedModel ollama/nomic-embed-text, got %q", r.Chunk.EmbedModel)
		}
	}
}

func TestSearch_WithEmbedModelFilter_ExcludesEmptyTag(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.gob")

	s := NewGOBStore(indexPath)
	ctx := context.Background()

	chunks := []Chunk{
		{
			ID:         "c1",
			FilePath:   "a.go",
			StartLine:  1,
			EndLine:    10,
			Content:    "func A() {}",
			Vector:     []float32{1.0, 0.0, 0.0},
			Hash:       "h1",
			EmbedModel: "ollama/nomic-embed-text",
			UpdatedAt:  time.Now(),
		},
		{
			ID:        "c2",
			FilePath:  "b.go",
			StartLine: 1,
			EndLine:   10,
			Content:   "func B() {}",
			Vector:    []float32{0.9, 0.1, 0.0},
			Hash:      "h2",
			// EmbedModel intentionally empty (legacy chunk)
			UpdatedAt: time.Now(),
		},
	}

	if err := s.SaveChunks(ctx, chunks); err != nil {
		t.Fatalf("failed to save chunks: %v", err)
	}

	// Filter for ollama model; empty-tagged chunks must be excluded
	results, err := s.Search(ctx, []float32{1.0, 0.0, 0.0}, 10, SearchOptions{
		EmbedModel: "ollama/nomic-embed-text",
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result (empty tag excluded), got %d", len(results))
	}

	if len(results) > 0 && results[0].Chunk.ID != "c1" {
		t.Errorf("expected c1, got %s", results[0].Chunk.ID)
	}
}

func TestSearch_WithoutFilter_ReturnsAll(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.gob")

	s := NewGOBStore(indexPath)
	ctx := context.Background()

	chunks := []Chunk{
		{
			ID:         "c1",
			FilePath:   "a.go",
			StartLine:  1,
			EndLine:    10,
			Content:    "func A() {}",
			Vector:     []float32{1.0, 0.0, 0.0},
			Hash:       "h1",
			EmbedModel: "ollama/nomic-embed-text",
			UpdatedAt:  time.Now(),
		},
		{
			ID:        "c2",
			FilePath:  "b.go",
			StartLine: 1,
			EndLine:   10,
			Content:   "func B() {}",
			Vector:    []float32{0.9, 0.1, 0.0},
			Hash:      "h2",
			// Empty EmbedModel
			UpdatedAt: time.Now(),
		},
		{
			ID:         "c3",
			FilePath:   "c.go",
			StartLine:  1,
			EndLine:    10,
			Content:    "func C() {}",
			Vector:     []float32{0.8, 0.2, 0.0},
			Hash:       "h3",
			EmbedModel: "openai/text-embedding-3-small",
			UpdatedAt:  time.Now(),
		},
	}

	if err := s.SaveChunks(ctx, chunks); err != nil {
		t.Fatalf("failed to save chunks: %v", err)
	}

	// No EmbedModel filter: all chunks returned (default behavior)
	results, err := s.Search(ctx, []float32{1.0, 0.0, 0.0}, 10, SearchOptions{})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results (no filter), got %d", len(results))
	}
}
