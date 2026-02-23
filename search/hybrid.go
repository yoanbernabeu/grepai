package search

import (
	"context"
	"sort"
	"strings"

	"github.com/yoanbernabeu/grepai/store"
)

// TextSearch performs a simple text-based search on chunks.
// It scores chunks based on the number of query words they contain.
// If pathPrefix is provided, only chunks from files starting with that prefix are included.
func TextSearch(ctx context.Context, chunks []store.Chunk, query string, limit int, pathPrefix string) []store.SearchResult {
	words := tokenize(query)
	if len(words) == 0 {
		return nil
	}

	var results []store.SearchResult

	for _, chunk := range chunks {
		// Filter by path prefix if provided
		if pathPrefix != "" && !strings.HasPrefix(chunk.FilePath, pathPrefix) {
			continue
		}

		contentLower := strings.ToLower(chunk.Content)
		matchCount := 0

		for _, word := range words {
			if strings.Contains(contentLower, word) {
				matchCount++
			}
		}

		if matchCount > 0 {
			score := float32(matchCount) / float32(len(words))
			results = append(results, store.SearchResult{
				Chunk: chunk,
				Score: score,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// ReciprocalRankFusion merges multiple result lists using RRF.
// k is the RRF constant (typically 60).
// Results are deduplicated by chunk ID and sorted by combined RRF score.
func ReciprocalRankFusion(k float32, limit int, lists ...[]store.SearchResult) []store.SearchResult {
	scores := make(map[string]float32)       // chunkID -> RRF score
	chunkMap := make(map[string]store.Chunk) // chunkID -> chunk

	for _, list := range lists {
		for rank, result := range list {
			id := result.Chunk.ID
			scores[id] += 1.0 / (k + float32(rank) + 1)
			chunkMap[id] = result.Chunk
		}
	}

	results := make([]store.SearchResult, 0, len(scores))
	for id, score := range scores {
		results = append(results, store.SearchResult{
			Chunk: chunkMap[id],
			Score: score,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// tokenize splits a query into lowercase words, filtering out short words.
func tokenize(query string) []string {
	fields := strings.Fields(strings.ToLower(query))
	var words []string
	for _, f := range fields {
		// Skip very short words (articles, prepositions)
		if len(f) >= 2 {
			words = append(words, f)
		}
	}
	return words
}
