package trace

import "sort"

type callEdgeCandidate struct {
	edge    CallEdge
	ordinal int
}

// canonicalCallEdgeCandidates returns a sorted copy so callers never mutate
// persisted backend ordering while selecting the visible edge for a pair.
func canonicalCallEdgeCandidates(edges []callEdgeCandidate) []callEdgeCandidate {
	ordered := append([]callEdgeCandidate(nil), edges...)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.edge.Caller != b.edge.Caller {
			return a.edge.Caller < b.edge.Caller
		}
		if a.edge.Callee != b.edge.Callee {
			return a.edge.Callee < b.edge.Callee
		}
		if a.edge.File != b.edge.File {
			return a.edge.File < b.edge.File
		}
		if a.edge.Line != b.edge.Line {
			return a.edge.Line < b.edge.Line
		}
		if a.edge.CallType != b.edge.CallType {
			return a.edge.CallType < b.edge.CallType
		}
		return a.ordinal < b.ordinal
	})
	return ordered
}
