package trace

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func saveDuplicateGraphEdges(t *testing.T, store SymbolStore, files []string) {
	t.Helper()
	ctx := context.Background()
	if err := store.SaveFile(ctx, "defs.go", []Symbol{
		{Name: "A", File: "defs.go", Line: 1},
		{Name: "B", File: "defs.go", Line: 2},
		{Name: "C", File: "defs.go", Line: 3},
	}, nil); err != nil {
		t.Fatal(err)
	}
	refs := map[string][]Reference{
		"z.go": {
			{SymbolName: "B", Kind: RefKindCall, File: "z.go", Line: 20, CallerName: "A"},
			{SymbolName: "A", Kind: RefKindCall, File: "z.go", Line: 30, CallerName: "C"},
		},
		"a.go": {
			{SymbolName: "B", Kind: RefKindCall, File: "a.go", Line: 5, CallerName: "A"},
			{SymbolName: "A", Kind: RefKindCall, File: "a.go", Line: 7, CallerName: "C"},
		},
	}
	for _, file := range files {
		if err := store.SaveFile(ctx, file, nil, refs[file]); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGOBGetCallGraphCanonicalizesDuplicateEdges(t *testing.T) {
	ctx := context.Background()
	forward := NewGOBSymbolStore(filepath.Join(t.TempDir(), "forward.gob"))
	reverse := NewGOBSymbolStore(filepath.Join(t.TempDir(), "reverse.gob"))
	saveDuplicateGraphEdges(t, forward, []string{"z.go", "a.go"})
	saveDuplicateGraphEdges(t, reverse, []string{"a.go", "z.go"})

	forwardGraph, err := forward.GetCallGraph(ctx, "A", 1)
	if err != nil {
		t.Fatal(err)
	}
	reverseGraph, err := reverse.GetCallGraph(ctx, "A", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(forwardGraph, reverseGraph) {
		t.Fatalf("graph depends on save order:\nforward=%#v\nreverse=%#v", forwardGraph, reverseGraph)
	}
	if len(forwardGraph.Edges) != 2 || forwardGraph.Edges[0].File != "a.go" || forwardGraph.Edges[0].Line != 5 || forwardGraph.Edges[1].File != "a.go" || forwardGraph.Edges[1].Line != 7 {
		t.Fatalf("unexpected canonical edges: %#v", forwardGraph.Edges)
	}
	if len(forward.index.CallGraph) != 4 || forward.index.CallGraph[0].File != "z.go" {
		t.Fatalf("GetCallGraph mutated persisted edge order: %#v", forward.index.CallGraph)
	}
	saveDuplicateGraphEdges(t, forward, []string{"a.go", "z.go"})
	saveDuplicateGraphEdges(t, reverse, []string{"z.go", "a.go"})
	forwardGraph, err = forward.GetCallGraph(ctx, "A", 1)
	if err != nil {
		t.Fatal(err)
	}
	reverseGraph, err = reverse.GetCallGraph(ctx, "A", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(forwardGraph, reverseGraph) || forwardGraph.Edges[0].File != "a.go" || forwardGraph.Edges[1].File != "a.go" {
		t.Fatalf("graph depends on update order:\nforward=%#v\nreverse=%#v", forwardGraph, reverseGraph)
	}
}
