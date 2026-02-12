package rpg

import "testing"

func TestGraphRemoveEdgesIf(t *testing.T) {
	g := NewGraph()

	fileA := &Node{ID: "file:a.go", Kind: KindFile, Path: "a.go"}
	fileB := &Node{ID: "file:b.go", Kind: KindFile, Path: "b.go"}
	symA := &Node{ID: "sym:a.go:FuncA", Kind: KindSymbol, Path: "a.go", SymbolName: "FuncA"}
	symB := &Node{ID: "sym:b.go:FuncB", Kind: KindSymbol, Path: "b.go", SymbolName: "FuncB"}

	g.AddNode(fileA)
	g.AddNode(fileB)
	g.AddNode(symA)
	g.AddNode(symB)

	inv := &Edge{From: symA.ID, To: symB.ID, Type: EdgeInvokes}
	imp := &Edge{From: fileA.ID, To: fileB.ID, Type: EdgeImports}
	contains := &Edge{From: fileA.ID, To: symA.ID, Type: EdgeContains}
	g.AddEdge(inv)
	g.AddEdge(imp)
	g.AddEdge(contains)

	g.RemoveEdgesIf(func(e *Edge) bool {
		return e.Type == EdgeInvokes || e.Type == EdgeImports
	})

	if got := len(g.Edges); got != 1 {
		t.Fatalf("expected 1 edge after filtering, got %d", got)
	}
	if g.Edges[0].Type != EdgeContains {
		t.Fatalf("expected remaining edge type %q, got %q", EdgeContains, g.Edges[0].Type)
	}

	if got := len(g.GetOutgoing(symA.ID)); got != 0 {
		t.Fatalf("expected no outgoing edges from %s, got %d", symA.ID, got)
	}
	if got := len(g.GetOutgoing(fileA.ID)); got != 1 {
		t.Fatalf("expected one outgoing edge from %s, got %d", fileA.ID, got)
	}
}

func TestGraphNodePath(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Node{ID: "file:a.go", Kind: KindFile, Path: "a.go"})
	g.AddNode(&Node{ID: "area:auth", Kind: KindArea})

	path, ok := g.NodePath("file:a.go")
	if !ok {
		t.Fatal("expected NodePath to return true for file node")
	}
	if path != "a.go" {
		t.Fatalf("expected path a.go, got %s", path)
	}

	if _, ok := g.NodePath("area:auth"); ok {
		t.Fatal("expected NodePath to return false for node without path")
	}

	if _, ok := g.NodePath("missing"); ok {
		t.Fatal("expected NodePath to return false for missing node")
	}
}
