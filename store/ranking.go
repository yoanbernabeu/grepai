package store

import "sort"

// SortResultsByScore orders results by descending score and breaks ties on the
// chunk ID.
//
// The tiebreak is what makes a ranking reproducible. Callers assemble their
// result slice by ranging over a Go map, whose iteration order is randomized on
// every call, and sort.Slice is not stable, so chunks that score exactly the
// same came back in a different order on every search over an unchanged index.
// Because callers truncate to a limit after sorting, that order also decided
// which results the caller saw at all.
//
// Chunk IDs are unique within a store, so the comparator is a total order and
// the output no longer depends on the order the caller happened to build.
func SortResultsByScore(results []SearchResult) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Chunk.ID < results[j].Chunk.ID
		}
		return results[i].Score > results[j].Score
	})
}
