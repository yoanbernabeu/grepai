package indexer

import "strings"

func parseNegationRule(pattern string) (negationRule, bool) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return negationRule{}, false
	}
	anchored := strings.HasPrefix(pattern, "/")
	pattern = strings.TrimPrefix(pattern, "/")
	directoryOnly := strings.HasSuffix(pattern, "/")
	pattern = strings.TrimSuffix(pattern, "/")
	if pattern == "" {
		return negationRule{broad: true}, true
	}
	// Escaping changes how metacharacters and separators are interpreted. Treat
	// such rules as broad rather than making an unsafe negative reachability claim.
	if strings.Contains(pattern, `\`) {
		return negationRule{broad: true}, true
	}
	if !anchored && !strings.Contains(pattern, "/") {
		return negationRule{broad: true}, true
	}

	parts := strings.Split(pattern, "/")
	literalParts := make([]string, 0, len(parts))
	hasWildcard := false
	for _, part := range parts {
		if strings.ContainsAny(part, "*?[") {
			hasWildcard = true
			break
		}
		literalParts = append(literalParts, part)
	}
	if len(literalParts) == 0 {
		return negationRule{broad: true}, true
	}
	return negationRule{
		literalPrefix: strings.Join(literalParts, "/"),
		hasWildcard:   hasWildcard,
		directoryOnly: directoryOnly,
	}, true
}

func (m *IgnoreMatcher) mayReincludeDescendant(path string) bool {
	path = strings.Trim(path, "/")
	for _, matcher := range m.grepaiMatchers {
		base := strings.Trim(filepathToSlash(matcher.baseDir), "/")
		rel := path
		if base != "" {
			switch {
			case pathAncestorOrEqual(path, base):
				return len(matcher.negations) > 0
			case pathAncestorOrEqual(base, path):
				rel = strings.TrimPrefix(path, base)
				rel = strings.TrimPrefix(rel, "/")
			default:
				continue
			}
		}
		for _, rule := range matcher.negations {
			if rule.mayMatchBelow(rel) {
				return true
			}
		}
	}
	return false
}

func (r negationRule) mayMatchBelow(dir string) bool {
	if r.broad {
		return true
	}
	dir = strings.Trim(dir, "/")
	if r.hasWildcard || r.directoryOnly {
		return pathAncestorOrEqual(dir, r.literalPrefix) || pathAncestorOrEqual(r.literalPrefix, dir)
	}
	return pathAncestorOrEqual(dir, r.literalPrefix)
}

func pathAncestorOrEqual(parent, child string) bool {
	if parent == "" {
		return true
	}
	return child == parent || strings.HasPrefix(child, parent+"/")
}

func filepathToSlash(path string) string {
	return strings.ReplaceAll(path, `\`, "/")
}
