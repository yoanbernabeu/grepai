package rpg

import (
	"testing"
	"time"
)

func TestNewGraph(t *testing.T) {
	g := NewGraph()

	if g.Nodes == nil {
		t.Error("Nodes map should be initialized")
	}
	if g.Edges == nil {
		t.Error("Edges slice should be initialized")
	}
	if g.byKind == nil {
		t.Error("byKind index should be initialized")
	}
	if g.byFile == nil {
		t.Error("byFile index should be initialized")
	}
	if g.byFeaturePath == nil {
		t.Error("byFeaturePath index should be initialized")
	}
	if g.adjForward == nil {
		t.Error("adjForward index should be initialized")
	}
	if g.adjReverse == nil {
		t.Error("adjReverse index should be initialized")
	}

	if len(g.Nodes) != 0 {
		t.Errorf("Expected 0 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 0 {
		t.Errorf("Expected 0 edges, got %d", len(g.Edges))
	}
}

func TestAddNode(t *testing.T) {
	g := NewGraph()
	now := time.Now()

	// Add a symbol node
	symNode := &Node{
		ID:         "sym:file.go:TestFunc",
		Kind:       KindSymbol,
		Feature:    "test-function",
		Path:       "file.go",
		SymbolName: "TestFunc",
		UpdatedAt:  now,
	}
	g.AddNode(symNode)

	// Verify node is in main map
	if g.Nodes[symNode.ID] != symNode {
		t.Error("Node not found in Nodes map")
	}

	// Verify byKind index
	if len(g.byKind[KindSymbol]) != 1 {
		t.Errorf("Expected 1 symbol node in byKind index, got %d", len(g.byKind[KindSymbol]))
	}
	if g.byKind[KindSymbol][0] != symNode {
		t.Error("Symbol node not correctly indexed in byKind")
	}

	// Verify byFile index
	if len(g.byFile["file.go"]) != 1 {
		t.Errorf("Expected 1 node in byFile index for file.go, got %d", len(g.byFile["file.go"]))
	}
	if g.byFile["file.go"][0] != symNode {
		t.Error("Node not correctly indexed in byFile")
	}

	// Add an area node
	areaNode := &Node{
		ID:        "area:cli",
		Kind:      KindArea,
		Feature:   "cli",
		UpdatedAt: now,
	}
	g.AddNode(areaNode)

	// Verify byFeaturePath index for hierarchy nodes
	if g.byFeaturePath["cli"] != areaNode {
		t.Error("Area node not correctly indexed in byFeaturePath")
	}

	// Add a category node
	catNode := &Node{
		ID:        "cat:cli/watch",
		Kind:      KindCategory,
		Feature:   "cli/watch",
		UpdatedAt: now,
	}
	g.AddNode(catNode)

	if g.byFeaturePath["cli/watch"] != catNode {
		t.Error("Category node not correctly indexed in byFeaturePath")
	}
}

func TestRemoveNode(t *testing.T) {
	g := NewGraph()
	now := time.Now()

	// Add a symbol node with edges
	symNode := &Node{
		ID:         "sym:file.go:TestFunc",
		Kind:       KindSymbol,
		Feature:    "test-function",
		Path:       "file.go",
		SymbolName: "TestFunc",
		UpdatedAt:  now,
	}
	g.AddNode(symNode)

	fileNode := &Node{
		ID:        "file:file.go",
		Kind:      KindFile,
		Path:      "file.go",
		UpdatedAt: now,
	}
	g.AddNode(fileNode)

	// Add edge from file to symbol
	edge := &Edge{
		From:      fileNode.ID,
		To:        symNode.ID,
		Type:      EdgeContains,
		Weight:    1.0,
		UpdatedAt: now,
	}
	g.AddEdge(edge)

	// Verify edge exists
	if len(g.Edges) != 1 {
		t.Fatalf("Expected 1 edge, got %d", len(g.Edges))
	}

	// Remove symbol node
	g.RemoveNode(symNode.ID)

	// Verify node is removed from main map
	if g.Nodes[symNode.ID] != nil {
		t.Error("Node should be removed from Nodes map")
	}

	// Verify byKind index is updated
	if len(g.byKind[KindSymbol]) != 0 {
		t.Errorf("Expected 0 symbol nodes in byKind index, got %d", len(g.byKind[KindSymbol]))
	}

	// Verify byFile index is updated
	if len(g.byFile["file.go"]) != 1 {
		t.Errorf("Expected 1 node (file) in byFile index, got %d", len(g.byFile["file.go"]))
	}

	// Verify edges are removed
	if len(g.Edges) != 0 {
		t.Errorf("Expected 0 edges after node removal, got %d", len(g.Edges))
	}

	// Verify adjacency indexes are cleaned up
	if len(g.adjForward[fileNode.ID]) != 0 {
		t.Error("Forward adjacency should be cleaned up")
	}
	if len(g.adjReverse[symNode.ID]) != 0 {
		t.Error("Reverse adjacency should be cleaned up")
	}
}

func TestAddEdge(t *testing.T) {
	g := NewGraph()
	now := time.Now()

	node1 := &Node{ID: "node1", Kind: KindSymbol, UpdatedAt: now}
	node2 := &Node{ID: "node2", Kind: KindSymbol, UpdatedAt: now}
	g.AddNode(node1)
	g.AddNode(node2)

	edge := &Edge{
		From:      "node1",
		To:        "node2",
		Type:      EdgeInvokes,
		Weight:    1.0,
		UpdatedAt: now,
	}
	g.AddEdge(edge)

	// Verify edge in main list
	if len(g.Edges) != 1 {
		t.Fatalf("Expected 1 edge, got %d", len(g.Edges))
	}
	if g.Edges[0] != edge {
		t.Error("Edge not found in Edges list")
	}

	// Verify forward adjacency
	if len(g.adjForward["node1"]) != 1 {
		t.Fatalf("Expected 1 outgoing edge from node1, got %d", len(g.adjForward["node1"]))
	}
	if g.adjForward["node1"][0] != edge {
		t.Error("Edge not correctly indexed in adjForward")
	}

	// Verify reverse adjacency
	if len(g.adjReverse["node2"]) != 1 {
		t.Fatalf("Expected 1 incoming edge to node2, got %d", len(g.adjReverse["node2"]))
	}
	if g.adjReverse["node2"][0] != edge {
		t.Error("Edge not correctly indexed in adjReverse")
	}
}

func TestRemoveEdgesBetween(t *testing.T) {
	g := NewGraph()
	now := time.Now()

	node1 := &Node{ID: "node1", Kind: KindSymbol, UpdatedAt: now}
	node2 := &Node{ID: "node2", Kind: KindSymbol, UpdatedAt: now}
	node3 := &Node{ID: "node3", Kind: KindSymbol, UpdatedAt: now}
	g.AddNode(node1)
	g.AddNode(node2)
	g.AddNode(node3)

	edge1 := &Edge{From: "node1", To: "node2", Type: EdgeInvokes, Weight: 1.0, UpdatedAt: now}
	edge2 := &Edge{From: "node1", To: "node3", Type: EdgeInvokes, Weight: 1.0, UpdatedAt: now}
	g.AddEdge(edge1)
	g.AddEdge(edge2)

	// Remove edge between node1 and node2
	g.RemoveEdgesBetween("node1", "node2")

	// Verify edge1 is removed
	if len(g.Edges) != 1 {
		t.Fatalf("Expected 1 edge remaining, got %d", len(g.Edges))
	}
	if g.Edges[0] != edge2 {
		t.Error("Wrong edge remained")
	}

	// Verify forward adjacency
	if len(g.adjForward["node1"]) != 1 {
		t.Fatalf("Expected 1 outgoing edge from node1, got %d", len(g.adjForward["node1"]))
	}
	if g.adjForward["node1"][0].To != "node3" {
		t.Error("Wrong edge in forward adjacency")
	}

	// Verify reverse adjacency
	if len(g.adjReverse["node2"]) != 0 {
		t.Errorf("Expected 0 incoming edges to node2, got %d", len(g.adjReverse["node2"]))
	}
	if len(g.adjReverse["node3"]) != 1 {
		t.Fatalf("Expected 1 incoming edge to node3, got %d", len(g.adjReverse["node3"]))
	}
}

func TestGetNodesByKind(t *testing.T) {
	g := NewGraph()
	now := time.Now()

	sym1 := &Node{ID: "sym1", Kind: KindSymbol, UpdatedAt: now}
	sym2 := &Node{ID: "sym2", Kind: KindSymbol, UpdatedAt: now}
	file1 := &Node{ID: "file1", Kind: KindFile, UpdatedAt: now}

	g.AddNode(sym1)
	g.AddNode(sym2)
	g.AddNode(file1)

	symbols := g.GetNodesByKind(KindSymbol)
	if len(symbols) != 2 {
		t.Errorf("Expected 2 symbol nodes, got %d", len(symbols))
	}

	files := g.GetNodesByKind(KindFile)
	if len(files) != 1 {
		t.Errorf("Expected 1 file node, got %d", len(files))
	}

	areas := g.GetNodesByKind(KindArea)
	if len(areas) != 0 {
		t.Errorf("Expected 0 area nodes, got %d", len(areas))
	}
}

func TestGetNodesByFile(t *testing.T) {
	g := NewGraph()
	now := time.Now()

	file1 := &Node{ID: "file1", Kind: KindFile, Path: "file1.go", UpdatedAt: now}
	sym1 := &Node{ID: "sym1", Kind: KindSymbol, Path: "file1.go", UpdatedAt: now}
	sym2 := &Node{ID: "sym2", Kind: KindSymbol, Path: "file1.go", UpdatedAt: now}
	sym3 := &Node{ID: "sym3", Kind: KindSymbol, Path: "file2.go", UpdatedAt: now}

	g.AddNode(file1)
	g.AddNode(sym1)
	g.AddNode(sym2)
	g.AddNode(sym3)

	file1Nodes := g.GetNodesByFile("file1.go")
	if len(file1Nodes) != 3 {
		t.Errorf("Expected 3 nodes for file1.go, got %d", len(file1Nodes))
	}

	file2Nodes := g.GetNodesByFile("file2.go")
	if len(file2Nodes) != 1 {
		t.Errorf("Expected 1 node for file2.go, got %d", len(file2Nodes))
	}
}

func TestGetNeighbors(t *testing.T) {
	g := NewGraph()
	now := time.Now()

	node1 := &Node{ID: "node1", Kind: KindSymbol, UpdatedAt: now}
	node2 := &Node{ID: "node2", Kind: KindSymbol, UpdatedAt: now}
	node3 := &Node{ID: "node3", Kind: KindSymbol, UpdatedAt: now}
	node4 := &Node{ID: "node4", Kind: KindSymbol, UpdatedAt: now}

	g.AddNode(node1)
	g.AddNode(node2)
	g.AddNode(node3)
	g.AddNode(node4)

	// node1 -> node2 (forward)
	// node3 -> node1 (reverse, so node3 is incoming to node1)
	// node1 -> node4 (forward)
	g.AddEdge(&Edge{From: "node1", To: "node2", Type: EdgeInvokes, Weight: 1.0, UpdatedAt: now})
	g.AddEdge(&Edge{From: "node3", To: "node1", Type: EdgeInvokes, Weight: 1.0, UpdatedAt: now})
	g.AddEdge(&Edge{From: "node1", To: "node4", Type: EdgeInvokes, Weight: 1.0, UpdatedAt: now})

	// Test forward direction
	forward := g.GetNeighbors("node1", "forward")
	if len(forward) != 2 {
		t.Errorf("Expected 2 forward neighbors, got %d", len(forward))
	}
	forwardMap := make(map[string]bool)
	for _, id := range forward {
		forwardMap[id] = true
	}
	if !forwardMap["node2"] || !forwardMap["node4"] {
		t.Error("Forward neighbors should be node2 and node4")
	}

	// Test reverse direction
	reverse := g.GetNeighbors("node1", "reverse")
	if len(reverse) != 1 {
		t.Errorf("Expected 1 reverse neighbor, got %d", len(reverse))
	}
	if reverse[0] != "node3" {
		t.Error("Reverse neighbor should be node3")
	}

	// Test both directions
	both := g.GetNeighbors("node1", "both")
	if len(both) != 3 {
		t.Errorf("Expected 3 neighbors in both directions, got %d", len(both))
	}
	bothMap := make(map[string]bool)
	for _, id := range both {
		bothMap[id] = true
	}
	if !bothMap["node2"] || !bothMap["node3"] || !bothMap["node4"] {
		t.Error("Both neighbors should be node2, node3, and node4")
	}
}

func TestRebuildIndexes(t *testing.T) {
	g := NewGraph()
	now := time.Now()

	// Add nodes and edges directly to maps (simulating deserialization)
	sym1 := &Node{ID: "sym1", Kind: KindSymbol, Path: "file.go", UpdatedAt: now}
	sym2 := &Node{ID: "sym2", Kind: KindSymbol, Path: "file.go", UpdatedAt: now}
	area1 := &Node{ID: "area:cli", Kind: KindArea, Feature: "cli", UpdatedAt: now}

	g.Nodes = map[string]*Node{
		"sym1":     sym1,
		"sym2":     sym2,
		"area:cli": area1,
	}

	edge1 := &Edge{From: "sym1", To: "sym2", Type: EdgeInvokes, Weight: 1.0, UpdatedAt: now}
	g.Edges = []*Edge{edge1}

	// Indexes should be empty before rebuild
	if len(g.byKind) != 0 {
		t.Error("byKind should be empty before rebuild")
	}

	// Rebuild indexes
	g.RebuildIndexes()

	// Verify byKind
	if len(g.byKind[KindSymbol]) != 2 {
		t.Errorf("Expected 2 symbol nodes in byKind, got %d", len(g.byKind[KindSymbol]))
	}
	if len(g.byKind[KindArea]) != 1 {
		t.Errorf("Expected 1 area node in byKind, got %d", len(g.byKind[KindArea]))
	}

	// Verify byFile
	if len(g.byFile["file.go"]) != 2 {
		t.Errorf("Expected 2 nodes in byFile for file.go, got %d", len(g.byFile["file.go"]))
	}

	// Verify byFeaturePath
	if g.byFeaturePath["cli"] != area1 {
		t.Error("Area node not found in byFeaturePath")
	}

	// Verify adjacency
	if len(g.adjForward["sym1"]) != 1 {
		t.Errorf("Expected 1 forward edge from sym1, got %d", len(g.adjForward["sym1"]))
	}
	if len(g.adjReverse["sym2"]) != 1 {
		t.Errorf("Expected 1 reverse edge to sym2, got %d", len(g.adjReverse["sym2"]))
	}
}

func TestStats(t *testing.T) {
	g := NewGraph()
	now := time.Now()

	sym1 := &Node{ID: "sym1", Kind: KindSymbol, UpdatedAt: now}
	sym2 := &Node{ID: "sym2", Kind: KindSymbol, UpdatedAt: now}
	file1 := &Node{ID: "file1", Kind: KindFile, UpdatedAt: now}
	area1 := &Node{ID: "area1", Kind: KindArea, UpdatedAt: now}

	g.AddNode(sym1)
	g.AddNode(sym2)
	g.AddNode(file1)
	g.AddNode(area1)

	edge1 := &Edge{From: "sym1", To: "sym2", Type: EdgeInvokes, Weight: 1.0, UpdatedAt: now}
	edge2 := &Edge{From: "file1", To: "sym1", Type: EdgeContains, Weight: 1.0, UpdatedAt: now}
	g.AddEdge(edge1)
	g.AddEdge(edge2)

	stats := g.Stats()

	if stats.TotalNodes != 4 {
		t.Errorf("Expected 4 total nodes, got %d", stats.TotalNodes)
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
	if stats.NodesByKind[KindArea] != 1 {
		t.Errorf("Expected 1 area node, got %d", stats.NodesByKind[KindArea])
	}

	if stats.EdgesByType[EdgeInvokes] != 1 {
		t.Errorf("Expected 1 invokes edge, got %d", stats.EdgesByType[EdgeInvokes])
	}
	if stats.EdgesByType[EdgeContains] != 1 {
		t.Errorf("Expected 1 contains edge, got %d", stats.EdgesByType[EdgeContains])
	}

	if stats.LastUpdated.Before(now) {
		t.Error("LastUpdated should be at or after the test start time")
	}
}

func TestMakeNodeID(t *testing.T) {
	tests := []struct {
		name     string
		kind     NodeKind
		parts    []string
		expected string
	}{
		{
			name:     "symbol with receiver",
			kind:     KindSymbol,
			parts:    []string{"file.go", "Server", "HandleRequest"},
			expected: "sym:file.go:Server.HandleRequest",
		},
		{
			name:     "symbol without receiver",
			kind:     KindSymbol,
			parts:    []string{"file.go", "HandleRequest"},
			expected: "sym:file.go:HandleRequest",
		},
		{
			name:     "symbol with empty receiver - treated as 2 parts",
			kind:     KindSymbol,
			parts:    []string{"file.go", "HandleRequest"},
			expected: "sym:file.go:HandleRequest",
		},
		{
			name:     "file",
			kind:     KindFile,
			parts:    []string{"file.go"},
			expected: "file:file.go",
		},
		{
			name:     "area",
			kind:     KindArea,
			parts:    []string{"cli"},
			expected: "area:cli",
		},
		{
			name:     "category with parent",
			kind:     KindCategory,
			parts:    []string{"cli", "watch"},
			expected: "cat:cli/watch",
		},
		{
			name:     "category single part",
			kind:     KindCategory,
			parts:    []string{"watch"},
			expected: "cat:watch",
		},
		{
			name:     "subcategory with parent",
			kind:     KindSubcategory,
			parts:    []string{"cli/watch", "handle"},
			expected: "subcat:cli/watch/handle",
		},
		{
			name:     "subcategory single part",
			kind:     KindSubcategory,
			parts:    []string{"handle"},
			expected: "subcat:handle",
		},
		{
			name:     "chunk",
			kind:     KindChunk,
			parts:    []string{"chunk123"},
			expected: "chunk:chunk123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MakeNodeID(tt.kind, tt.parts...)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}
