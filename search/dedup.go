package search

import "github.com/yoanbernabeu/grepai/store"

// DeduplicateByFile keeps only the highest-scoring chunk per file path.
func DeduplicateByFile(results []store.SearchResult) []store.SearchResult {
	seen := make(map[string]bool, len(results))
	deduped := make([]store.SearchResult, 0, len(results))
	for _, r := range results {
		if seen[r.Chunk.FilePath] {
			continue
		}
		seen[r.Chunk.FilePath] = true
		deduped = append(deduped, r)
	}
	return deduped
}
