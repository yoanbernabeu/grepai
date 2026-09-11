package trace

import "testing"

func TestCanonicalCallEdgeCandidatesOrdersEveryStableField(t *testing.T) {
	// Given candidates differing at each canonical comparison field.
	edges := []callEdgeCandidate{
		{edge: CallEdge{Caller: "B", Callee: "A", File: "a.go", Line: 1}},
		{edge: CallEdge{Caller: "A", Callee: "B", File: "a.go", Line: 1}},
		{edge: CallEdge{Caller: "A", Callee: "A", File: "z.go", Line: 1}},
		{edge: CallEdge{Caller: "A", Callee: "A", File: "a.go", Line: 2}},
		{edge: CallEdge{Caller: "A", Callee: "A", File: "a.go", Line: 1, CallType: "z"}},
		{edge: CallEdge{Caller: "A", Callee: "A", File: "a.go", Line: 1, CallType: "a"}, ordinal: 2},
		{edge: CallEdge{Caller: "A", Callee: "A", File: "a.go", Line: 1, CallType: "a"}, ordinal: 1},
	}
	originalFirst := edges[0]

	// When canonicalized.
	ordered := canonicalCallEdgeCandidates(edges)

	// Then caller/callee/file/line/type/ordinal determine stable order and the
	// persisted input remains unchanged.
	if ordered[0].ordinal != 1 || ordered[1].ordinal != 2 || ordered[len(ordered)-1].edge.Caller != "B" {
		t.Fatalf("unexpected canonical order: %#v", ordered)
	}
	if edges[0] != originalFirst {
		t.Fatal("canonicalization mutated input")
	}
}
