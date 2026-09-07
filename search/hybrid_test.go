package search

import (
	"context"
	"testing"

	"github.com/yoanbernabeu/grepai/store"
)

func TestTextSearch(t *testing.T) {
	chunks := []store.Chunk{
		{ID: "1", Content: "function handleLogin(user, password) { return auth(user); }"},
		{ID: "2", Content: "function handleLogout() { session.clear(); }"},
		{ID: "3", Content: "const user = { name: 'test', email: 'test@example.com' };"},
		{ID: "4", Content: "function validateEmail(email) { return email.includes('@'); }"},
	}

	ctx := context.Background()

	t.Run("single word match", func(t *testing.T) {
		results := TextSearch(ctx, chunks, "login", 10, "")
		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d", len(results))
			return
		}
		if results[0].Chunk.ID != "1" {
			t.Errorf("expected ID '1', got '%s'", results[0].Chunk.ID)
		}
	})

	t.Run("multiple word match - best first", func(t *testing.T) {
		results := TextSearch(ctx, chunks, "user email", 10, "")
		if len(results) != 3 {
			t.Errorf("expected 3 results, got %d", len(results))
			return
		}
		// Chunk 3 has both "user" and "email" -> score 1.0
		if results[0].Chunk.ID != "3" {
			t.Errorf("expected first result to be '3' (has both words), got '%s'", results[0].Chunk.ID)
		}
		// Chunks 1 and 4 each have one word -> score 0.5
		if results[0].Score != 1.0 {
			t.Errorf("expected first result score 1.0, got %f", results[0].Score)
		}
	})

	t.Run("no match", func(t *testing.T) {
		results := TextSearch(ctx, chunks, "database connection", 10, "")
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})
}

func TestTextSearch_EmptyQuery(t *testing.T) {
	chunks := []store.Chunk{
		{ID: "1", Content: "some content"},
	}

	results := TextSearch(context.Background(), chunks, "", 10, "")
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty query, got %d", len(results))
	}
}

func TestReciprocalRankFusion(t *testing.T) {
	list1 := []store.SearchResult{
		{Chunk: store.Chunk{ID: "a"}, Score: 0.9},
		{Chunk: store.Chunk{ID: "b"}, Score: 0.8},
		{Chunk: store.Chunk{ID: "c"}, Score: 0.7},
	}

	list2 := []store.SearchResult{
		{Chunk: store.Chunk{ID: "b"}, Score: 0.95},
		{Chunk: store.Chunk{ID: "d"}, Score: 0.85},
		{Chunk: store.Chunk{ID: "a"}, Score: 0.75},
	}

	results := ReciprocalRankFusion(60, 10, list1, list2)

	// "a" is rank 0 in list1 (1/61) and rank 2 in list2 (1/63) = ~0.0322
	// "b" is rank 1 in list1 (1/62) and rank 0 in list2 (1/61) = ~0.0324
	// So "b" should be first, then "a"

	if len(results) != 4 {
		t.Errorf("expected 4 results, got %d", len(results))
		return
	}

	// "b" should have highest combined score
	if results[0].Chunk.ID != "b" {
		t.Errorf("expected first result to be 'b', got '%s'", results[0].Chunk.ID)
	}

	// "a" should be second
	if results[1].Chunk.ID != "a" {
		t.Errorf("expected second result to be 'a', got '%s'", results[1].Chunk.ID)
	}
}

func TestReciprocalRankFusion_Limit(t *testing.T) {
	list1 := []store.SearchResult{
		{Chunk: store.Chunk{ID: "a"}, Score: 0.9},
		{Chunk: store.Chunk{ID: "b"}, Score: 0.8},
		{Chunk: store.Chunk{ID: "c"}, Score: 0.7},
	}

	results := ReciprocalRankFusion(60, 2, list1)

	if len(results) != 2 {
		t.Errorf("expected 2 results with limit, got %d", len(results))
	}
}

func TestReciprocalRankFusion_EmptyLists(t *testing.T) {
	results := ReciprocalRankFusion(60, 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty lists, got %d", len(results))
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		query    string
		expected []string
	}{
		{"hello world", []string{"hello", "world"}},
		{"UPPER CASE", []string{"upper", "case"}},
		{"a b c", []string{}}, // single letters filtered
		{"the user login", []string{"the", "user", "login"}},
		{"", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result := tokenize(tt.query)
			if len(result) != len(tt.expected) {
				t.Errorf("tokenize(%q) = %v, want %v", tt.query, result, tt.expected)
			}
		})
	}
}

func fusionIDs(results []store.SearchResult) []string {
	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.Chunk.ID)
	}
	return ids
}

func sameIDs(got, want []string) bool {
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

// Two disjoint lists give every rank a pair of chunks with the identical RRF
// score, which is the normal shape of a hybrid search: the vector list and the
// text list rarely agree on their top hits. Fusion collects them out of a map,
// so the ranking has to be pinned by something other than iteration order.
func TestReciprocalRankFusion_TiedScoresAreDeterministic(t *testing.T) {
	vector := []store.SearchResult{
		{Chunk: store.Chunk{ID: "a"}, Score: 0.9},
		{Chunk: store.Chunk{ID: "c"}, Score: 0.8},
		{Chunk: store.Chunk{ID: "e"}, Score: 0.7},
	}
	text := []store.SearchResult{
		{Chunk: store.Chunk{ID: "b"}, Score: 1.0},
		{Chunk: store.Chunk{ID: "d"}, Score: 0.5},
		{Chunk: store.Chunk{ID: "f"}, Score: 0.5},
	}

	// a and b are both rank 0 (1/61), c and d both rank 1 (1/62), e and f both
	// rank 2 (1/63), so each pair ties and orders by chunk ID.
	want := []string{"a", "b", "c", "d", "e", "f"}
	for i := 0; i < 20; i++ {
		got := fusionIDs(ReciprocalRankFusion(60, 10, vector, text))
		if !sameIDs(got, want) {
			t.Fatalf("call %d returned %v, want %v", i, got, want)
		}
	}
}

// With a limit the tie decides which results the caller ever sees, so the same
// two lists must not yield a different set of chunks on the next call.
func TestReciprocalRankFusion_TiedScoresKeepSameSubsetUnderLimit(t *testing.T) {
	vector := []store.SearchResult{{Chunk: store.Chunk{ID: "a"}, Score: 0.9}}
	text := []store.SearchResult{{Chunk: store.Chunk{ID: "b"}, Score: 0.9}}

	want := []string{"a"}
	for i := 0; i < 50; i++ {
		got := fusionIDs(ReciprocalRankFusion(60, 1, vector, text))
		if !sameIDs(got, want) {
			t.Fatalf("call %d returned %v, want %v", i, got, want)
		}
	}
}

// TextSearch scores on how many query words a chunk contains, so equal scores
// are common. Its input arrives from GetAllChunks, which the GOB store builds
// by ranging over a map, so it cannot inherit an order from its caller.
func TestTextSearch_TiedScoresOrderByChunkID(t *testing.T) {
	chunks := []store.Chunk{
		{ID: "d", Content: "func handleLogin() {}"},
		{ID: "c", Content: "func handleLogin() {}"},
		{ID: "b", Content: "func handleLogin() {}"},
		{ID: "a", Content: "func handleLogin() {}"},
	}

	got := fusionIDs(TextSearch(context.Background(), chunks, "handleLogin", 10, ""))
	want := []string{"a", "b", "c", "d"}
	if !sameIDs(got, want) {
		t.Errorf("TextSearch() = %v, want %v", got, want)
	}
}
