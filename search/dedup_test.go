package search

import (
	"testing"

	"github.com/yoanbernabeu/grepai/store"
)

func TestDeduplicateByFile(t *testing.T) {
	results := []store.SearchResult{
		{Chunk: store.Chunk{ID: "a_0", FilePath: "a.go"}, Score: 0.9},
		{Chunk: store.Chunk{ID: "b_0", FilePath: "b.go"}, Score: 0.8},
		{Chunk: store.Chunk{ID: "a_1", FilePath: "a.go"}, Score: 0.7},
		{Chunk: store.Chunk{ID: "c_0", FilePath: "c.go"}, Score: 0.6},
		{Chunk: store.Chunk{ID: "b_1", FilePath: "b.go"}, Score: 0.5},
	}

	deduped := DeduplicateByFile(results)

	if len(deduped) != 3 {
		t.Fatalf("expected 3 results, got %d", len(deduped))
	}

	expected := []struct {
		id    string
		score float32
	}{
		{"a_0", 0.9},
		{"b_0", 0.8},
		{"c_0", 0.6},
	}

	for i, want := range expected {
		if deduped[i].Chunk.ID != want.id {
			t.Errorf("result[%d]: expected ID %q, got %q", i, want.id, deduped[i].Chunk.ID)
		}
		if deduped[i].Score != want.score {
			t.Errorf("result[%d]: expected score %v, got %v", i, want.score, deduped[i].Score)
		}
	}
}

func TestDeduplicateByFile_Empty(t *testing.T) {
	deduped := DeduplicateByFile(nil)
	if len(deduped) != 0 {
		t.Fatalf("expected 0 results, got %d", len(deduped))
	}
}

func TestDeduplicateByFile_AllUnique(t *testing.T) {
	results := []store.SearchResult{
		{Chunk: store.Chunk{ID: "a_0", FilePath: "a.go"}, Score: 0.9},
		{Chunk: store.Chunk{ID: "b_0", FilePath: "b.go"}, Score: 0.8},
		{Chunk: store.Chunk{ID: "c_0", FilePath: "c.go"}, Score: 0.7},
	}

	deduped := DeduplicateByFile(results)

	if len(deduped) != 3 {
		t.Fatalf("expected 3 results, got %d", len(deduped))
	}
}
