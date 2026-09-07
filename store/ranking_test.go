package store

import (
	"context"
	"path/filepath"
	"testing"
)

func resultIDs(results []SearchResult) []string {
	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.Chunk.ID)
	}
	return ids
}

func equalIDs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestSortResultsByScore_ScoreWinsOverID(t *testing.T) {
	results := []SearchResult{
		{Chunk: Chunk{ID: "a"}, Score: 0.1},
		{Chunk: Chunk{ID: "b"}, Score: 0.9},
		{Chunk: Chunk{ID: "c"}, Score: 0.5},
	}

	SortResultsByScore(results)

	want := []string{"b", "c", "a"}
	if got := resultIDs(results); !equalIDs(got, want) {
		t.Errorf("SortResultsByScore() = %v, want %v", got, want)
	}
}

func TestSortResultsByScore_TiesOrderByChunkID(t *testing.T) {
	results := []SearchResult{
		{Chunk: Chunk{ID: "d"}, Score: 0.5},
		{Chunk: Chunk{ID: "b"}, Score: 0.5},
		{Chunk: Chunk{ID: "c"}, Score: 0.5},
		{Chunk: Chunk{ID: "a"}, Score: 0.5},
	}

	SortResultsByScore(results)

	want := []string{"a", "b", "c", "d"}
	if got := resultIDs(results); !equalIDs(got, want) {
		t.Errorf("SortResultsByScore() = %v, want %v", got, want)
	}
}

// tiedScoreChunks returns chunks that all share one vector, so a search scores
// every one of them identically.
func tiedScoreChunks(ids ...string) []Chunk {
	chunks := make([]Chunk, 0, len(ids))
	for _, id := range ids {
		chunks = append(chunks, Chunk{
			ID:        id,
			FilePath:  id + ".go",
			StartLine: 1,
			EndLine:   2,
			Content:   "func " + id + "() {}",
			Vector:    []float32{1, 0, 0},
			Hash:      id,
		})
	}
	return chunks
}

// Chunks holding identical vectors score identically, and the store ranges over
// a map to collect them. Ranking has to be reproducible anyway.
func TestGOBStore_SearchTiedScoresAreDeterministic(t *testing.T) {
	ctx := context.Background()
	st := NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))

	ids := []string{"h", "g", "f", "e", "d", "c", "b", "a"}
	if err := st.SaveChunks(ctx, tiedScoreChunks(ids...)); err != nil {
		t.Fatalf("failed to save chunks: %v", err)
	}

	want := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	for i := 0; i < 20; i++ {
		results, err := st.Search(ctx, []float32{1, 0, 0}, 0, SearchOptions{})
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		if got := resultIDs(results); !equalIDs(got, want) {
			t.Fatalf("call %d returned %v, want %v", i, got, want)
		}
	}
}

// The order also decides which tied chunks survive the limit, so an unchanged
// index must not return a different set of results on the next search.
func TestGOBStore_SearchTiedScoresKeepSameSubsetUnderLimit(t *testing.T) {
	ctx := context.Background()
	st := NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))

	ids := []string{"h", "g", "f", "e", "d", "c", "b", "a"}
	if err := st.SaveChunks(ctx, tiedScoreChunks(ids...)); err != nil {
		t.Fatalf("failed to save chunks: %v", err)
	}

	want := []string{"a", "b", "c"}
	for i := 0; i < 20; i++ {
		results, err := st.Search(ctx, []float32{1, 0, 0}, 3, SearchOptions{})
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		if got := resultIDs(results); !equalIDs(got, want) {
			t.Fatalf("call %d returned %v, want %v", i, got, want)
		}
	}
}

// A better score still outranks a lexicographically smaller ID.
func TestGOBStore_SearchRanksByScoreFirst(t *testing.T) {
	ctx := context.Background()
	st := NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))

	chunks := []Chunk{
		{ID: "z", FilePath: "z.go", Content: "z", Vector: []float32{1, 0, 0}},
		{ID: "a", FilePath: "a.go", Content: "a", Vector: []float32{0, 1, 0}},
	}
	if err := st.SaveChunks(ctx, chunks); err != nil {
		t.Fatalf("failed to save chunks: %v", err)
	}

	results, err := st.Search(ctx, []float32{1, 0, 0}, 0, SearchOptions{})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	want := []string{"z", "a"}
	if got := resultIDs(results); !equalIDs(got, want) {
		t.Errorf("search returned %v, want %v", got, want)
	}
}
