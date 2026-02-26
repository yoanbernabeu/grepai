package rpg

import (
	"strings"

	"github.com/yoanbernabeu/grepai/trace"
)

// makeSymbolNodeID builds a deterministic node ID for a trace.Symbol.
func makeSymbolNodeID(filePath string, sym trace.Symbol) string {
	if sym.Receiver != "" {
		return MakeNodeID(KindSymbol, filePath, sym.Receiver, sym.Name)
	}
	return MakeNodeID(KindSymbol, filePath, sym.Name)
}

// splitFeatureWords splits a feature string by common delimiters into a
// lowercase word set.
func splitFeatureWords(s string) map[string]bool {
	s = strings.ToLower(s)
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-' || r == ' ' || r == '/' || r == '_' || r == '@'
	})
	set := make(map[string]bool, len(parts))
	for _, part := range parts {
		if part != "" {
			set[part] = true
		}
	}
	return set
}

// collectFeatureParents returns the set of hierarchy parents pointing to nodeID.
func collectFeatureParents(g *Graph, nodeID string) map[string]bool {
	parents := make(map[string]bool)
	for _, edge := range g.GetIncoming(nodeID) {
		if edge.Type == EdgeFeatureParent {
			parents[edge.From] = true
		}
	}
	return parents
}
