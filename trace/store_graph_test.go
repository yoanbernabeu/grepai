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
