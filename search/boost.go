package search

import (
	"strings"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/store"
)

// ApplyBoost applies structural boosting to search results based on file path patterns.
// Penalties reduce scores (factor < 1), bonuses increase scores (factor > 1).
// Results are re-sorted by adjusted score after boosting.
func ApplyBoost(results []store.SearchResult, boostCfg config.BoostConfig) []store.SearchResult {
	if !boostCfg.Enabled || len(results) == 0 {
		return results
	}

	for i := range results {
		boost := computeBoostFactor(results[i].Chunk.FilePath, boostCfg)
		results[i].Score *= boost
	}

	store.SortResultsByScore(results)

	return results
}

// computeBoostFactor calculates the combined boost factor for a file path.
// Multiple matching rules are multiplied together.
func computeBoostFactor(filePath string, boostCfg config.BoostConfig) float32 {
	factor := float32(1.0)

	for _, rule := range boostCfg.Penalties {
		if matchesPattern(filePath, rule.Pattern) {
			factor *= rule.Factor
		}
	}

	for _, rule := range boostCfg.Bonuses {
		if matchesPattern(filePath, rule.Pattern) {
			factor *= rule.Factor
		}
	}

	return factor
}

// matchesPattern checks if a file path contains the given pattern.
// Patterns are simple substring matches (case-sensitive).
func matchesPattern(filePath, pattern string) bool {
	return strings.Contains(filePath, pattern)
}
