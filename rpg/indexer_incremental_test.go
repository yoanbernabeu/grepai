package rpg

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yoanbernabeu/grepai/trace"
)

func TestRefreshDerivedEdgesIncremental_RebuildsChangedFileEdges(t *testing.T) {
	ctx := context.Background()
	graph := NewGraph()

	fileA := &Node{ID: "file:a.go", Kind: KindFile, Path: "a.go"}
	fileB := &Node{ID: "file:b.go", Kind: KindFile, Path: "b.go"}
	symCaller := &Node{
		ID:         "sym:a.go:Caller",
		Kind:       KindSymbol,
		Path:       "a.go",
		SymbolName: "Caller",
		Feature:    "handle-request@server",
		StartLine:  1,
		EndLine:    30,
	}
	symPeer := &Node{
		ID:         "sym:b.go:Peer",
		Kind:       KindSymbol,
		Path:       "b.go",
		SymbolName: "Peer",
		Feature:    "parse-config",
		StartLine:  1,
		EndLine:    20,
	}
	symCallee := &Node{
		ID:         "sym:b.go:Callee",
		Kind:       KindSymbol,
		Path:       "b.go",
		SymbolName: "Callee",
		Feature:    "handle-request@client",
		StartLine:  1,
		EndLine:    20,
	}

	graph.AddNode(fileA)
	graph.AddNode(fileB)
	graph.AddNode(symCaller)
	graph.AddNode(symPeer)
	graph.AddNode(symCallee)
	graph.AddEdge(&Edge{From: fileA.ID, To: symCaller.ID, Type: EdgeContains, Weight: 1.0})
	graph.AddEdge(&Edge{From: fileB.ID, To: symPeer.ID, Type: EdgeContains, Weight: 1.0})
	graph.AddEdge(&Edge{From: fileB.ID, To: symCallee.ID, Type: EdgeContains, Weight: 1.0})

	// Seed stale derived edges that should be removed for changed file a.go.
	graph.AddEdge(&Edge{From: symCaller.ID, To: symPeer.ID, Type: EdgeInvokes, Weight: 1.0})
	graph.AddEdge(&Edge{From: fileA.ID, To: fileB.ID, Type: EdgeImports, Weight: 1.0})
	graph.AddEdge(&Edge{From: symCaller.ID, To: symPeer.ID, Type: EdgeSemanticSim, Weight: 0.8})

	rpgStore := &GOBRPGStore{indexPath: filepath.Join(t.TempDir(), "rpg.gob"), graph: graph}
	indexer := NewRPGIndexer(rpgStore, NewLocalExtractor(), t.TempDir(), RPGIndexerConfig{
		DriftThreshold:       0.35,
		FeatureGroupStrategy: "sample",
	})

	symbolStore := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	defer symbolStore.Close()
	if err := symbolStore.SaveFile(ctx, "a.go", []trace.Symbol{
		{Name: "Caller", File: "a.go", Line: 1, EndLine: 30, Language: "go"},
	}, []trace.Reference{
		{SymbolName: "Callee", File: "a.go", Line: 10, CallerName: "Caller"},
	}); err != nil {
		t.Fatalf("failed to seed symbol store for a.go: %v", err)
	}
	if err := symbolStore.SaveFile(ctx, "b.go", []trace.Symbol{
		{Name: "Callee", File: "b.go", Line: 1, EndLine: 20, Language: "go"},
		{Name: "Peer", File: "b.go", Line: 21, EndLine: 40, Language: "go"},
	}, nil); err != nil {
		t.Fatalf("failed to seed symbol store for b.go: %v", err)
	}

	if err := indexer.RefreshDerivedEdgesIncremental(ctx, symbolStore, []string{"a.go"}); err != nil {
		t.Fatalf("RefreshDerivedEdgesIncremental() failed: %v", err)
	}

	if hasEdge(graph, symCaller.ID, symPeer.ID, EdgeInvokes) {
		t.Fatalf("stale invoke edge was not removed")
	}
	if !hasEdge(graph, symCaller.ID, symCallee.ID, EdgeInvokes) {
		t.Fatalf("expected invoke edge from Caller to Callee")
	}
	if !hasEdge(graph, fileA.ID, fileB.ID, EdgeImports) {
		t.Fatalf("expected import edge from a.go to b.go")
	}

	foundSemantic := false
	for _, e := range graph.Edges {
		if e.Type != EdgeSemanticSim {
			continue
		}
		if e.From == symCaller.ID || e.To == symCaller.ID {
			foundSemantic = true
			break
		}
	}
	if !foundSemantic {
		t.Fatalf("expected semantic_sim edge involving changed symbol %s", symCaller.ID)
	}
}

func TestRefreshDerivedEdgesFull_RebuildsAllDerivedEdges(t *testing.T) {
	ctx := context.Background()
	graph := NewGraph()

	fileA := &Node{ID: "file:a.go", Kind: KindFile, Path: "a.go"}
	fileB := &Node{ID: "file:b.go", Kind: KindFile, Path: "b.go"}
	symCaller := &Node{ID: "sym:a.go:Caller", Kind: KindSymbol, Path: "a.go", SymbolName: "Caller", StartLine: 1, EndLine: 20}
	symCallee := &Node{ID: "sym:b.go:Callee", Kind: KindSymbol, Path: "b.go", SymbolName: "Callee", StartLine: 1, EndLine: 20}

	graph.AddNode(fileA)
	graph.AddNode(fileB)
	graph.AddNode(symCaller)
	graph.AddNode(symCallee)
	graph.AddEdge(&Edge{From: fileA.ID, To: symCaller.ID, Type: EdgeContains, Weight: 1.0})
	graph.AddEdge(&Edge{From: fileB.ID, To: symCallee.ID, Type: EdgeContains, Weight: 1.0})
	graph.AddEdge(&Edge{From: symCaller.ID, To: symCaller.ID, Type: EdgeInvokes, Weight: 1.0})
	graph.AddEdge(&Edge{From: symCaller.ID, To: symCallee.ID, Type: EdgeSemanticSim, Weight: 0.7})

	rpgStore := &GOBRPGStore{indexPath: filepath.Join(t.TempDir(), "rpg.gob"), graph: graph}
	indexer := NewRPGIndexer(rpgStore, NewLocalExtractor(), t.TempDir(), RPGIndexerConfig{
		DriftThreshold:       0.35,
		FeatureGroupStrategy: "sample",
	})

	symbolStore := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	defer symbolStore.Close()
	if err := symbolStore.SaveFile(ctx, "a.go", []trace.Symbol{
		{Name: "Caller", File: "a.go", Line: 1, EndLine: 20, Language: "go"},
	}, []trace.Reference{
		{SymbolName: "Callee", File: "a.go", Line: 10, CallerName: "Caller"},
	}); err != nil {
		t.Fatalf("failed to seed symbol store for a.go: %v", err)
	}
	if err := symbolStore.SaveFile(ctx, "b.go", []trace.Symbol{
		{Name: "Callee", File: "b.go", Line: 1, EndLine: 20, Language: "go"},
	}, nil); err != nil {
		t.Fatalf("failed to seed symbol store for b.go: %v", err)
	}

	if err := indexer.RefreshDerivedEdgesFull(ctx, symbolStore); err != nil {
		t.Fatalf("RefreshDerivedEdgesFull() failed: %v", err)
	}

	if hasEdge(graph, symCaller.ID, symCaller.ID, EdgeInvokes) {
		t.Fatalf("stale self invoke edge should be removed on full refresh")
	}
	if !hasEdge(graph, symCaller.ID, symCallee.ID, EdgeInvokes) {
		t.Fatalf("expected invoke edge from Caller to Callee after full refresh")
	}
	if !hasEdge(graph, fileA.ID, fileB.ID, EdgeImports) {
		t.Fatalf("expected import edge from a.go to b.go after full refresh")
	}
}

func hasEdge(graph *Graph, fromID, toID string, edgeType EdgeType) bool {
	for _, e := range graph.Edges {
		if e.From == fromID && e.To == toID && e.Type == edgeType {
			return true
		}
	}
	return false
}
