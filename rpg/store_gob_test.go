package rpg

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGOBRPGStore_PersistLoad(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "rpg.gob")

	// Create store and add some data
	store := NewGOBRPGStore(indexPath)
	graph := store.GetGraph()

	// Add nodes
	node1 := &Node{
		ID:         "sym1",
		Kind:       KindSymbol,
		Feature:    "handle-request",
		SymbolName: "HandleRequest",
		Path:       "server.go",
		StartLine:  10,
		EndLine:    20,
		UpdatedAt:  time.Now(),
	}
	node2 := &Node{
		ID:        "area:cli",
		Kind:      KindArea,
		Feature:   "cli",
		UpdatedAt: time.Now(),
	}

	graph.AddNode(node1)
	graph.AddNode(node2)

	// Add edge
	edge := &Edge{
		From:      node1.ID,
		To:        node2.ID,
		Type:      EdgeFeatureParent,
		Weight:    1.0,
		UpdatedAt: time.Now(),
	}
	graph.AddEdge(edge)

	// Persist
	err := store.Persist(context.Background())
	if err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Fatal("Index file was not created")
	}

	// Create new store and load
	store2 := NewGOBRPGStore(indexPath)
	err = store2.Load(context.Background())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	graph2 := store2.GetGraph()

	// Verify nodes were loaded
	if len(graph2.Nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(graph2.Nodes))
	}

	loadedNode1 := graph2.GetNode(node1.ID)
	if loadedNode1 == nil {
		t.Fatal("Node 'sym1' not loaded")
	}
	if loadedNode1.Kind != KindSymbol {
		t.Errorf("Wrong kind for sym1: %s", loadedNode1.Kind)
	}
	if loadedNode1.Feature != "handle-request" {
		t.Errorf("Wrong feature for sym1: %s", loadedNode1.Feature)
	}
	if loadedNode1.SymbolName != "HandleRequest" {
		t.Errorf("Wrong symbol name for sym1: %s", loadedNode1.SymbolName)
	}
	if loadedNode1.Path != "server.go" {
		t.Errorf("Wrong path for sym1: %s", loadedNode1.Path)
	}

	loadedNode2 := graph2.GetNode(node2.ID)
	if loadedNode2 == nil {
		t.Fatal("Node 'area:cli' not loaded")
	}

	// Verify edges were loaded
	if len(graph2.Edges) != 1 {
		t.Errorf("Expected 1 edge, got %d", len(graph2.Edges))
	}

	// Verify indexes were rebuilt
	symbolNodes := graph2.GetNodesByKind(KindSymbol)
	if len(symbolNodes) != 1 {
		t.Errorf("Expected 1 symbol in byKind index, got %d", len(symbolNodes))
	}

	areaNodes := graph2.GetNodesByKind(KindArea)
	if len(areaNodes) != 1 {
		t.Errorf("Expected 1 area in byKind index, got %d", len(areaNodes))
	}

	fileNodes := graph2.GetNodesByFile("server.go")
	if len(fileNodes) != 1 {
		t.Errorf("Expected 1 node for server.go in byFile index, got %d", len(fileNodes))
	}

	// Verify adjacency indexes
	outgoing := graph2.GetOutgoing(node1.ID)
	if len(outgoing) != 1 {
		t.Errorf("Expected 1 outgoing edge from sym1, got %d", len(outgoing))
	}

	incoming := graph2.GetIncoming(node2.ID)
	if len(incoming) != 1 {
		t.Errorf("Expected 1 incoming edge to area:cli, got %d", len(incoming))
	}
}

func TestGOBRPGStore_EmptyLoad(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "nonexistent.gob")

	store := NewGOBRPGStore(indexPath)

	// Load should succeed even if file doesn't exist
	err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load on non-existent file should succeed, got error: %v", err)
	}

	graph := store.GetGraph()

	// Graph should be empty but initialized
	if graph.Nodes == nil {
		t.Error("Nodes map should be initialized")
	}
	if len(graph.Nodes) != 0 {
		t.Errorf("Expected 0 nodes in empty graph, got %d", len(graph.Nodes))
	}

	if graph.Edges == nil {
		t.Error("Edges slice should be initialized")
	}
	if len(graph.Edges) != 0 {
		t.Errorf("Expected 0 edges in empty graph, got %d", len(graph.Edges))
	}
}

func TestGOBRPGStore_GetGraph(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "rpg.gob")

	store := NewGOBRPGStore(indexPath)
	graph := store.GetGraph()

	if graph == nil {
		t.Fatal("GetGraph should return non-nil graph")
	}

	// Add a node
	node := &Node{
		ID:        "test",
		Kind:      KindSymbol,
		UpdatedAt: time.Now(),
	}
	graph.AddNode(node)

	// GetGraph should return the same graph instance
	graph2 := store.GetGraph()
	if graph2.GetNode("test") == nil {
		t.Error("GetGraph should return the same graph instance")
	}
}

func TestGOBRPGStore_GetStats(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "rpg.gob")

	store := NewGOBRPGStore(indexPath)
	graph := store.GetGraph()

	// Add some nodes and edges
	node1 := &Node{ID: "node1", Kind: KindSymbol, UpdatedAt: time.Now()}
	node2 := &Node{ID: "node2", Kind: KindSymbol, UpdatedAt: time.Now()}
	node3 := &Node{ID: "node3", Kind: KindFile, UpdatedAt: time.Now()}

	graph.AddNode(node1)
	graph.AddNode(node2)
	graph.AddNode(node3)

	edge1 := &Edge{From: "node1", To: "node2", Type: EdgeInvokes, UpdatedAt: time.Now()}
	edge2 := &Edge{From: "node3", To: "node1", Type: EdgeContains, UpdatedAt: time.Now()}

	graph.AddEdge(edge1)
	graph.AddEdge(edge2)

	// Get stats
	stats, err := store.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalNodes != 3 {
		t.Errorf("Expected 3 total nodes, got %d", stats.TotalNodes)
	}
	if stats.TotalEdges != 2 {
		t.Errorf("Expected 2 total edges, got %d", stats.TotalEdges)
	}

	if stats.NodesByKind[KindSymbol] != 2 {
		t.Errorf("Expected 2 symbol nodes, got %d", stats.NodesByKind[KindSymbol])
	}
	if stats.NodesByKind[KindFile] != 1 {
		t.Errorf("Expected 1 file node, got %d", stats.NodesByKind[KindFile])
	}

	if stats.EdgesByType[EdgeInvokes] != 1 {
		t.Errorf("Expected 1 invokes edge, got %d", stats.EdgesByType[EdgeInvokes])
	}
	if stats.EdgesByType[EdgeContains] != 1 {
		t.Errorf("Expected 1 contains edge, got %d", stats.EdgesByType[EdgeContains])
	}
}

func TestGOBRPGStore_Close(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "rpg.gob")

	store := NewGOBRPGStore(indexPath)
	graph := store.GetGraph()

	// Add a node
	node := &Node{
		ID:        "test",
		Kind:      KindSymbol,
		UpdatedAt: time.Now(),
	}
	graph.AddNode(node)

	// Close should persist
	err := store.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Error("Index file should be created on Close")
	}

	// Load in new store to verify
	store2 := NewGOBRPGStore(indexPath)
	err = store2.Load(context.Background())
	if err != nil {
		t.Fatalf("Load after Close failed: %v", err)
	}

	if store2.GetGraph().GetNode("test") == nil {
		t.Error("Node should be persisted on Close")
	}
}

func TestGOBRPGStore_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "rpg.gob")

	store := NewGOBRPGStore(indexPath)
	graph := store.GetGraph()

	// Add initial node
	node := &Node{
		ID:        "test",
		Kind:      KindSymbol,
		UpdatedAt: time.Now(),
	}
	graph.AddNode(node)

	// Test concurrent reads
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			_, err := store.GetStats(context.Background())
			if err != nil {
				t.Errorf("Concurrent GetStats failed: %v", err)
			}
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			g := store.GetGraph()
			_ = g.GetNode("test")
		}
		done <- true
	}()

	<-done
	<-done
}

func TestGOBRPGStore_LargeGraph(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large graph test in short mode")
	}

	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "rpg.gob")

	store := NewGOBRPGStore(indexPath)
	graph := store.GetGraph()

	// Add many nodes
	numNodes := 1000
	for i := 0; i < numNodes; i++ {
		node := &Node{
			ID:        string(rune(i)),
			Kind:      KindSymbol,
			Feature:   "test-feature",
			UpdatedAt: time.Now(),
		}
		graph.AddNode(node)
	}

	// Persist
	err := store.Persist(context.Background())
	if err != nil {
		t.Fatalf("Persist large graph failed: %v", err)
	}

	// Load
	store2 := NewGOBRPGStore(indexPath)
	err = store2.Load(context.Background())
	if err != nil {
		t.Fatalf("Load large graph failed: %v", err)
	}

	graph2 := store2.GetGraph()
	if len(graph2.Nodes) != numNodes {
		t.Errorf("Expected %d nodes after load, got %d", numNodes, len(graph2.Nodes))
	}
}

func TestGOBRPGStore_PersistCreatesMissingParentDir(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "missing", ".grepai", "rpg.gob")

	store := NewGOBRPGStore(indexPath)
	if err := store.Persist(context.Background()); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("expected persisted rpg index file at %s: %v", indexPath, err)
	}
}
