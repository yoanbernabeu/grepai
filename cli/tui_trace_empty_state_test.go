package cli

import (
	"strings"
	"testing"

	"github.com/yoanbernabeu/grepai/trace"
)

func TestTraceEmptyState_NoSymbolMatch(t *testing.T) {
	title, why, action := traceEmptyState(traceViewCallers, trace.TraceResult{Query: "Missing"})

	if title != "No symbol match" {
		t.Fatalf("title = %q, want %q", title, "No symbol match")
	}
	if !strings.Contains(strings.ToLower(why), "no symbol matched") {
		t.Fatalf("unexpected why message: %q", why)
	}
	if !strings.Contains(strings.ToLower(action), "different symbol") {
		t.Fatalf("unexpected action message: %q", action)
	}
}

func TestTraceEmptyState_NoGraphData(t *testing.T) {
	title, why, action := traceEmptyState(traceViewGraph, trace.TraceResult{
		Query: "MissingGraph",
		Graph: &trace.CallGraph{
			Nodes: map[string]trace.Symbol{},
			Edges: []trace.CallEdge{},
			Depth: 2,
		},
	})

	if title != "No graph data" {
		t.Fatalf("title = %q, want %q", title, "No graph data")
	}
	if !strings.Contains(strings.ToLower(why), "no call graph nodes or edges") {
		t.Fatalf("unexpected why message: %q", why)
	}
	if !strings.Contains(action, "--depth") {
		t.Fatalf("unexpected action message: %q", action)
	}
}

func TestBuildTraceRows_GraphWithEdgesWithoutNodes(t *testing.T) {
	result := trace.TraceResult{
		Query: "EdgeOnly",
		Graph: &trace.CallGraph{
			Nodes: map[string]trace.Symbol{},
			Edges: []trace.CallEdge{
				{Caller: "A", Callee: "B", File: "main.go", Line: 12},
			},
			Depth: 2,
		},
	}

	rows := buildTraceRows(result, traceViewGraph)
	if len(rows) != 1 {
		t.Fatalf("rows length = %d, want 1", len(rows))
	}
	if rows[0].title != "A -> B" {
		t.Fatalf("row title = %q, want %q", rows[0].title, "A -> B")
	}
	if !strings.Contains(strings.Join(rows[0].detail, "\n"), "callsite: main.go:12") {
		t.Fatalf("expected callsite detail in row: %+v", rows[0].detail)
	}
}
