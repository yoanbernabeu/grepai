package rpg

import (
	"sort"
	"strings"
	"unicode"
)

// normalizeAtomicFeature normalizes a feature phrase to lowercase words
// separated by single spaces (e.g. "Validate-Token" -> "validate token").
func normalizeAtomicFeature(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.Trim(raw, "\"'`.,;:!?()[]{}<>")
	if raw == "" {
		return ""
	}

	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}

// atomicFromPrimaryFeature converts a kebab-like label to a normalized atomic phrase.
func atomicFromPrimaryFeature(primary string) string {
	return normalizeAtomicFeature(primary)
}

// primaryFromAtomicFeature converts an atomic phrase to kebab-case.
func primaryFromAtomicFeature(atomic string) string {
	normalized := normalizeAtomicFeature(atomic)
	if normalized == "" {
		return ""
	}
	return strings.ReplaceAll(normalized, " ", "-")
}

// dedupeAtomicFeatures deduplicates normalized atomic features while preserving order.
// If limit is > 0, at most limit features are returned.
func dedupeAtomicFeatures(features []string, limit int) []string {
	result := make([]string, 0, len(features))
	seen := make(map[string]struct{}, len(features))
	for _, feature := range features {
		normalized := normalizeAtomicFeature(feature)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}

// aggregateAtomicFeatures ranks atomic features by frequency and returns top entries.
func aggregateAtomicFeatures(features []string, limit int) []string {
	if limit <= 0 {
		limit = len(features)
	}

	counts := make(map[string]int, len(features))
	for _, feature := range features {
		normalized := normalizeAtomicFeature(feature)
		if normalized == "" {
			continue
		}
		counts[normalized]++
	}
	if len(counts) == 0 {
		return nil
	}

	type entry struct {
		feature string
		count   int
	}
	entries := make([]entry, 0, len(counts))
	for feature, count := range counts {
		entries = append(entries, entry{feature: feature, count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}
		return entries[i].feature < entries[j].feature
	})

	out := make([]string, 0, limit)
	for _, item := range entries {
		out = append(out, item.feature)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// getNodeAtomicFeatures returns normalized atomic features for the node.
func getNodeAtomicFeatures(node *Node) []string {
	if node == nil {
		return nil
	}
	if len(node.Features) > 0 {
		return dedupeAtomicFeatures(node.Features, 0)
	}
	if node.Feature == "" {
		return nil
	}
	atomic := atomicFromPrimaryFeature(node.Feature)
	if atomic == "" {
		return nil
	}
	return []string{atomic}
}

// setNodeFeatures sets Node.Features and keeps Node.Feature in sync for compatibility.
func setNodeFeatures(node *Node, features []string, fallbackPrimary string) {
	if node == nil {
		return
	}

	normalized := dedupeAtomicFeatures(features, 0)
	if len(normalized) == 0 && fallbackPrimary != "" {
		atomic := atomicFromPrimaryFeature(fallbackPrimary)
		if atomic != "" {
			normalized = []string{atomic}
		}
	}

	node.Features = normalized
	if len(normalized) > 0 {
		node.Feature = primaryFromAtomicFeature(normalized[0])
		return
	}
	if fallbackPrimary != "" {
		node.Feature = primaryFromAtomicFeature(atomicFromPrimaryFeature(fallbackPrimary))
	}
}

func atomicWordSet(features []string) map[string]bool {
	words := make(map[string]bool)
	for _, feature := range features {
		normalized := normalizeAtomicFeature(feature)
		if normalized == "" {
			continue
		}
		for _, token := range strings.Fields(normalized) {
			words[token] = true
		}
	}
	return words
}

func buildSummaryContext(name string, features []string) string {
	var sb strings.Builder
	sb.WriteString("Node: ")
	sb.WriteString(name)
	sb.WriteString("\nChildren:\n")
	for _, feature := range dedupeAtomicFeatures(features, 20) {
		sb.WriteString("- ")
		sb.WriteString(feature)
		sb.WriteString("\n")
	}
	return sb.String()
}
