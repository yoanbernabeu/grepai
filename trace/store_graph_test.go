package trace

import (
	"context"
	"path/filepath"
	"testing"
)

func hasEdge(edges []CallEdge, caller, callee string) bool {
	for _, e := range edges {
		if e.Caller == caller && e.Callee == callee {
			return true
		}
	}
	return false
}

func countEdge(edges []CallEdge, caller, callee string) int {
	count := 0
	for _, e := range edges {
		if e.Caller == caller && e.Callee == callee {
			count++
		}
	}
	return count
}

func edgeLine(edges []CallEdge, caller, callee string) int {
	for _, e := range edges {
		if e.Caller == caller && e.Callee == callee {
			return e.Line
		}
	}
	return -1
}

func TestGetCallGraph_DoesNotTraverseUnknownIntermediateSymbol(t *testing.T) {
	ctx := context.Background()
	store := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))

	err := store.SaveFile(ctx, "root.go", []Symbol{
		{Name: "RootFn", Kind: KindFunction, File: "root.go", Line: 1, Language: "go"},
	}, []Reference{
		{SymbolName: "ExternalCall", File: "root.go", Line: 10, CallerName: "RootFn"},
	})
	if err != nil {
		t.Fatalf("SaveFile(root) failed: %v", err)
	}

	err = store.SaveFile(ctx, "other_a.go", []Symbol{
		{Name: "OtherA", Kind: KindFunction, File: "other_a.go", Line: 1, Language: "go"},
	}, []Reference{
		{SymbolName: "ExternalCall", File: "other_a.go", Line: 11, CallerName: "OtherA"},
	})
	if err != nil {
		t.Fatalf("SaveFile(other_a) failed: %v", err)
	}

	err = store.SaveFile(ctx, "other_b.go", []Symbol{
		{Name: "OtherB", Kind: KindFunction, File: "other_b.go", Line: 1, Language: "go"},
	}, []Reference{
		{SymbolName: "ExternalCall", File: "other_b.go", Line: 12, CallerName: "OtherB"},
	})
	if err != nil {
		t.Fatalf("SaveFile(other_b) failed: %v", err)
	}

	graph, err := store.GetCallGraph(ctx, "RootFn", 2)
	if err != nil {
		t.Fatalf("GetCallGraph failed: %v", err)
	}

	if !hasEdge(graph.Edges, "RootFn", "ExternalCall") {
		t.Fatalf("expected direct edge RootFn -> ExternalCall to be present")
	}
	if hasEdge(graph.Edges, "OtherA", "ExternalCall") || hasEdge(graph.Edges, "OtherB", "ExternalCall") {
		t.Fatalf("unexpected expansion through unknown symbol ExternalCall")
	}
}

func TestGetCallGraph_DoesNotTraverseAmbiguousIntermediateSymbol(t *testing.T) {
	ctx := context.Background()
	store := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))

	err := store.SaveFile(ctx, "root.go", []Symbol{
		{Name: "RootFn", Kind: KindFunction, File: "root.go", Line: 1, Language: "go"},
	}, []Reference{
		{SymbolName: "Load", File: "root.go", Line: 10, CallerName: "RootFn"},
	})
	if err != nil {
		t.Fatalf("SaveFile(root) failed: %v", err)
	}

	// Ambiguous symbol: two distinct definitions of "Load".
	err = store.SaveFile(ctx, "loader_a.go", []Symbol{
		{Name: "Load", Kind: KindFunction, File: "loader_a.go", Line: 1, Language: "go"},
	}, nil)
	if err != nil {
		t.Fatalf("SaveFile(loader_a) failed: %v", err)
	}
	err = store.SaveFile(ctx, "loader_b.go", []Symbol{
		{Name: "Load", Kind: KindFunction, File: "loader_b.go", Line: 1, Language: "go"},
	}, nil)
	if err != nil {
		t.Fatalf("SaveFile(loader_b) failed: %v", err)
	}

	err = store.SaveFile(ctx, "other.go", []Symbol{
		{Name: "OtherCaller", Kind: KindFunction, File: "other.go", Line: 1, Language: "go"},
	}, []Reference{
		{SymbolName: "Load", File: "other.go", Line: 15, CallerName: "OtherCaller"},
	})
	if err != nil {
		t.Fatalf("SaveFile(other) failed: %v", err)
	}

	graph, err := store.GetCallGraph(ctx, "RootFn", 2)
	if err != nil {
		t.Fatalf("GetCallGraph failed: %v", err)
	}

	if !hasEdge(graph.Edges, "RootFn", "Load") {
		t.Fatalf("expected direct edge RootFn -> Load to be present")
	}
	if hasEdge(graph.Edges, "OtherCaller", "Load") {
		t.Fatalf("unexpected expansion through ambiguous symbol Load")
	}
}

func TestGetCallGraph_DeduplicatesEdgesAcrossTraversal(t *testing.T) {
	ctx := context.Background()
	store := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))

	err := store.SaveFile(ctx, "ab.go", []Symbol{
		{Name: "A", Kind: KindFunction, File: "ab.go", Line: 1, Language: "go"},
		{Name: "B", Kind: KindFunction, File: "ab.go", Line: 10, Language: "go"},
	}, []Reference{
		{SymbolName: "B", File: "ab.go", Line: 5, CallerName: "A"},
	})
	if err != nil {
		t.Fatalf("SaveFile(ab) failed: %v", err)
	}

	err = store.SaveFile(ctx, "bc.go", []Symbol{
		{Name: "C", Kind: KindFunction, File: "bc.go", Line: 1, Language: "go"},
	}, []Reference{
		{SymbolName: "C", File: "bc.go", Line: 5, CallerName: "B"},
	})
	if err != nil {
		t.Fatalf("SaveFile(bc) failed: %v", err)
	}

	graph, err := store.GetCallGraph(ctx, "A", 2)
	if err != nil {
		t.Fatalf("GetCallGraph failed: %v", err)
	}

	if got := countEdge(graph.Edges, "A", "B"); got != 1 {
		t.Fatalf("expected edge A->B exactly once, got %d", got)
	}
	if got := countEdge(graph.Edges, "B", "C"); got != 1 {
		t.Fatalf("expected edge B->C exactly once, got %d", got)
	}
}

func TestGetCallGraph_SkipsDeclarationSelfEdgeArtifacts(t *testing.T) {
	ctx := context.Background()
	store := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))

	err := store.SaveFile(ctx, "loop.go", []Symbol{
		{Name: "Loop", Kind: KindFunction, File: "loop.go", Line: 1, Language: "go"},
	}, []Reference{
		// Declaration artifact (not a real call): func Loop()
		{SymbolName: "Loop", File: "loop.go", Line: 1, CallerName: "Loop"},
		// Real recursive call inside the body.
		{SymbolName: "Loop", File: "loop.go", Line: 3, CallerName: "Loop"},
	})
	if err != nil {
		t.Fatalf("SaveFile(loop) failed: %v", err)
	}

	graph, err := store.GetCallGraph(ctx, "Loop", 1)
	if err != nil {
		t.Fatalf("GetCallGraph failed: %v", err)
	}

	if got := countEdge(graph.Edges, "Loop", "Loop"); got != 1 {
		t.Fatalf("expected one Loop->Loop edge, got %d", got)
	}
	if gotLine := edgeLine(graph.Edges, "Loop", "Loop"); gotLine != 3 {
		t.Fatalf("expected runtime recursive edge at line 3, got line %d", gotLine)
	}
}

func TestGetCallGraph_DoesNotPullUnrelatedIncomingForIntermediateNode(t *testing.T) {
	ctx := context.Background()
	store := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))

	err := store.SaveFile(ctx, "ab.go", []Symbol{
		{Name: "A", Kind: KindFunction, File: "ab.go", Line: 1, Language: "go"},
		{Name: "B", Kind: KindFunction, File: "ab.go", Line: 10, Language: "go"},
	}, []Reference{
		{SymbolName: "B", File: "ab.go", Line: 5, CallerName: "A"},
	})
	if err != nil {
		t.Fatalf("SaveFile(ab) failed: %v", err)
	}

	err = store.SaveFile(ctx, "bc.go", []Symbol{
		{Name: "C", Kind: KindFunction, File: "bc.go", Line: 1, Language: "go"},
	}, []Reference{
		{SymbolName: "C", File: "bc.go", Line: 7, CallerName: "B"},
	})
	if err != nil {
		t.Fatalf("SaveFile(bc) failed: %v", err)
	}

	err = store.SaveFile(ctx, "xb.go", []Symbol{
		{Name: "X", Kind: KindFunction, File: "xb.go", Line: 1, Language: "go"},
	}, []Reference{
		{SymbolName: "B", File: "xb.go", Line: 9, CallerName: "X"},
	})
	if err != nil {
		t.Fatalf("SaveFile(xb) failed: %v", err)
	}

	graph, err := store.GetCallGraph(ctx, "A", 2)
	if err != nil {
		t.Fatalf("GetCallGraph failed: %v", err)
	}

	if !hasEdge(graph.Edges, "A", "B") || !hasEdge(graph.Edges, "B", "C") {
		t.Fatalf("expected forward path A->B->C to be present")
	}
	if hasEdge(graph.Edges, "X", "B") {
		t.Fatalf("unexpected unrelated incoming edge X->B for intermediate node B")
	}
}
