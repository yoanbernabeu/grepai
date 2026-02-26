package rpg

import (
	"context"
	"strings"
	"testing"
)

func TestSearchNode(t *testing.T) {
	g := NewGraph()
	ext := NewLocalExtractor()
	h := NewHierarchyBuilder(g, ext)

	// Add symbols with different features
	sym1 := &Node{
		ID:         "sym1",
		Kind:       KindSymbol,
		Feature:    "handle-request",
		SymbolName: "HandleRequest",
		Path:       "server.go",
	}
	sym2 := &Node{
		ID:         "sym2",
		Kind:       KindSymbol,
		Feature:    "validate-token",
		SymbolName: "ValidateToken",
		Path:       "auth.go",
	}
	sym3 := &Node{
		ID:         "sym3",
		Kind:       KindSymbol,
		Feature:    "handle-response",
		SymbolName: "HandleResponse",
		Path:       "server.go",
	}

	g.AddNode(sym1)
	g.AddNode(sym2)
	g.AddNode(sym3)

	// Build hierarchy
	h.BuildHierarchy()

	qe := NewQueryEngine(g)

	t.Run("search with exact match", func(t *testing.T) {
		req := SearchNodeRequest{
			Query: "handle request",
			Limit: 10,
		}

		results, err := qe.SearchNode(context.Background(), req)
		if err != nil {
			t.Fatalf("SearchNode failed: %v", err)
		}

		if len(results) == 0 {
			t.Fatal("Expected at least one result")
		}

		// sym1 should rank highest (both words match)
		if results[0].Node.ID != sym1.ID {
			t.Errorf("Expected sym1 to rank first, got %s", results[0].Node.ID)
		}

		if results[0].Score <= 0 {
			t.Error("Score should be positive")
		}
	})

	t.Run("search with partial match", func(t *testing.T) {
		req := SearchNodeRequest{
			Query: "handle",
			Limit: 10,
		}

		results, err := qe.SearchNode(context.Background(), req)
		if err != nil {
			t.Fatalf("SearchNode failed: %v", err)
		}

		if len(results) < 2 {
			t.Errorf("Expected at least 2 results (handle-request and handle-response), got %d", len(results))
		}

		// Both sym1 and sym3 should be in results
		foundSym1 := false
		foundSym3 := false
		for _, r := range results {
			if r.Node.ID == sym1.ID {
				foundSym1 = true
			}
			if r.Node.ID == sym3.ID {
				foundSym3 = true
			}
		}

		if !foundSym1 || !foundSym3 {
			t.Error("Both 'handle' symbols should be found")
		}
	})

	t.Run("search with no matches", func(t *testing.T) {
		req := SearchNodeRequest{
			Query: "nonexistent function",
			Limit: 10,
		}

		results, err := qe.SearchNode(context.Background(), req)
		if err != nil {
			t.Fatalf("SearchNode failed: %v", err)
		}

		if len(results) != 0 {
			t.Errorf("Expected no results, got %d", len(results))
		}
	})

	t.Run("limit results", func(t *testing.T) {
		req := SearchNodeRequest{
			Query: "handle",
			Limit: 1,
		}

		results, err := qe.SearchNode(context.Background(), req)
		if err != nil {
			t.Fatalf("SearchNode failed: %v", err)
		}

		if len(results) > 1 {
			t.Errorf("Expected at most 1 result, got %d", len(results))
		}
	})
}

func TestSearchNode_WithScope(t *testing.T) {
	g := NewGraph()
	ext := NewLocalExtractor()
	h := NewHierarchyBuilder(g, ext)

	// Add symbols in different areas.
	sym1 := &Node{
		ID:         "sym1",
		Kind:       KindSymbol,
		Feature:    "handle-request",
		SymbolName: "HandleRequest",
		Path:       "cli/server.go",
	}
	sym2 := &Node{
		ID:         "sym2",
		Kind:       KindSymbol,
		Feature:    "handle-request",
		SymbolName: "HandleRequest",
		Path:       "store/server.go",
	}
	file1 := &Node{
		ID:      "file:cli/server.go",
		Kind:    KindFile,
		Path:    "cli/server.go",
		Feature: "server",
	}
	file2 := &Node{
		ID:      "file:store/server.go",
		Kind:    KindFile,
		Path:    "store/server.go",
		Feature: "server",
	}

	g.AddNode(file1)
	g.AddNode(file2)
	g.AddNode(sym1)
	g.AddNode(sym2)
	g.AddEdge(&Edge{From: file1.ID, To: sym1.ID, Type: EdgeContains})
	g.AddEdge(&Edge{From: file2.ID, To: sym2.ID, Type: EdgeContains})

	// Build hierarchy
	h.BuildHierarchy()

	qe := NewQueryEngine(g)

	t.Run("scope filters to specific area", func(t *testing.T) {
		req := SearchNodeRequest{
			Query: "handle request",
			Scope: "cli",
			Limit: 10,
		}

		results, err := qe.SearchNode(context.Background(), req)
		if err != nil {
			t.Fatalf("SearchNode failed: %v", err)
		}

		if len(results) != 1 {
			t.Fatalf("Expected 1 result in 'cli' scope, got %d", len(results))
		}

		if results[0].Node.ID != sym1.ID {
			t.Error("Should only find symbol in cli area")
		}
	})

	t.Run("scope with slash", func(t *testing.T) {
		req := SearchNodeRequest{
			Query: "handle",
			Scope: "cli",
			Limit: 10,
		}

		results, err := qe.SearchNode(context.Background(), req)
		if err != nil {
			t.Fatalf("SearchNode failed: %v", err)
		}

		if len(results) < 1 {
			t.Fatalf("Expected at least 1 result, got %d", len(results))
		}

		if results[0].Node.ID != sym1.ID {
			t.Error("Should find symbol in cli area")
		}
	})
}

func TestFetchNode(t *testing.T) {
	g := NewGraph()
	ext := NewLocalExtractor()
	h := NewHierarchyBuilder(g, ext)

	file := &Node{
		ID:      "file:cli/server.go",
		Kind:    KindFile,
		Path:    "cli/server.go",
		Feature: "server",
	}
	sym := &Node{
		ID:         "sym1",
		Kind:       KindSymbol,
		Feature:    "handle-request",
		SymbolName: "HandleRequest",
		Path:       "cli/server.go",
	}
	sym2 := &Node{
		ID:         "sym2",
		Kind:       KindSymbol,
		Feature:    "validate-token",
		SymbolName: "ValidateToken",
		Path:       "cli/server.go",
	}

	g.AddNode(file)
	g.AddNode(sym)
	g.AddNode(sym2)
	g.AddEdge(&Edge{From: file.ID, To: sym.ID, Type: EdgeContains})
	g.AddEdge(&Edge{From: file.ID, To: sym2.ID, Type: EdgeContains})

	// Build hierarchy
	h.BuildHierarchy()

	qe := NewQueryEngine(g)

	t.Run("fetch existing node", func(t *testing.T) {
		req := FetchNodeRequest{
			NodeID: sym.ID,
		}

		result, err := qe.FetchNode(context.Background(), req)
		if err != nil {
			t.Fatalf("FetchNode failed: %v", err)
		}

		if result == nil {
			t.Fatal("Expected result, got nil")
		}

		if result.Node.ID != sym.ID {
			t.Error("Wrong node returned")
		}

		// Should have feature path
		if result.FeaturePath == "" {
			t.Error("Feature path should not be empty")
		}
		if !strings.HasPrefix(result.FeaturePath, "cli/server") {
			t.Errorf("Expected feature path to start with cli/server, got %q", result.FeaturePath)
		}

		// Should have parents (hierarchy nodes)
		if len(result.Parents) == 0 {
			t.Error("Expected hierarchy parents")
		}

		// Should have incoming file containment edge.
		if len(result.Incoming) == 0 {
			t.Error("Expected incoming containment edge")
		}
	})

	t.Run("fetch non-existent node", func(t *testing.T) {
		req := FetchNodeRequest{
			NodeID: "nonexistent",
		}

		result, err := qe.FetchNode(context.Background(), req)
		if err != nil {
			t.Fatalf("FetchNode failed: %v", err)
		}

		if result != nil {
			t.Error("Expected nil result for non-existent node")
		}
	})
}

func TestExplore(t *testing.T) {
	g := NewGraph()

	// Create a small graph
	node1 := &Node{ID: "node1", Kind: KindSymbol}
	node2 := &Node{ID: "node2", Kind: KindSymbol}
	node3 := &Node{ID: "node3", Kind: KindSymbol}
	node4 := &Node{ID: "node4", Kind: KindSymbol}

	g.AddNode(node1)
	g.AddNode(node2)
	g.AddNode(node3)
	g.AddNode(node4)

	// Create edges: node1 -> node2 -> node3
	//               node1 -> node4
	g.AddEdge(&Edge{From: "node1", To: "node2", Type: EdgeInvokes})
	g.AddEdge(&Edge{From: "node2", To: "node3", Type: EdgeInvokes})
	g.AddEdge(&Edge{From: "node1", To: "node4", Type: EdgeContains})

	qe := NewQueryEngine(g)

	t.Run("explore forward with depth 1", func(t *testing.T) {
		req := ExploreRequest{
			StartNodeID: "node1",
			Direction:   "forward",
			Depth:       1,
			Limit:       100,
		}

		result, err := qe.Explore(context.Background(), req)
		if err != nil {
			t.Fatalf("Explore failed: %v", err)
		}

		if result.StartNode.ID != "node1" {
			t.Error("Wrong start node")
		}

		// Should have node1, node2, node4 (immediate neighbors)
		if len(result.Nodes) < 2 {
			t.Errorf("Expected at least 2 nodes (start + neighbors), got %d", len(result.Nodes))
		}

		if result.Nodes["node2"] == nil {
			t.Error("Should include node2")
		}
		if result.Nodes["node4"] == nil {
			t.Error("Should include node4")
		}

		// Should NOT have node3 (depth 2 away)
		if result.Nodes["node3"] != nil {
			t.Error("Should not include node3 at depth 1")
		}
	})

	t.Run("explore forward with depth 2", func(t *testing.T) {
		req := ExploreRequest{
			StartNodeID: "node1",
			Direction:   "forward",
			Depth:       2,
			Limit:       100,
		}

		result, err := qe.Explore(context.Background(), req)
		if err != nil {
			t.Fatalf("Explore failed: %v", err)
		}

		// Should now include node3
		if result.Nodes["node3"] == nil {
			t.Error("Should include node3 at depth 2")
		}

		if result.Depth < 1 {
			t.Errorf("Expected depth >= 1, got %d", result.Depth)
		}
	})

	t.Run("explore reverse", func(t *testing.T) {
		req := ExploreRequest{
			StartNodeID: "node3",
			Direction:   "reverse",
			Depth:       2,
			Limit:       100,
		}

		result, err := qe.Explore(context.Background(), req)
		if err != nil {
			t.Fatalf("Explore failed: %v", err)
		}

		// Starting from node3, going reverse should find node2 and node1
		if result.Nodes["node2"] == nil {
			t.Error("Should include node2 (direct parent)")
		}
		if result.Nodes["node1"] == nil {
			t.Error("Should include node1 (grandparent)")
		}
	})

	t.Run("explore both directions", func(t *testing.T) {
		req := ExploreRequest{
			StartNodeID: "node2",
			Direction:   "both",
			Depth:       1,
			Limit:       100,
		}

		result, err := qe.Explore(context.Background(), req)
		if err != nil {
			t.Fatalf("Explore failed: %v", err)
		}

		// From node2, should find node1 (reverse) and node3 (forward)
		if result.Nodes["node1"] == nil {
			t.Error("Should include node1 (incoming)")
		}
		if result.Nodes["node3"] == nil {
			t.Error("Should include node3 (outgoing)")
		}
	})

	t.Run("explore with limit", func(t *testing.T) {
		req := ExploreRequest{
			StartNodeID: "node1",
			Direction:   "forward",
			Depth:       10,
			Limit:       2, // Start node + 1 neighbor
		}

		result, err := qe.Explore(context.Background(), req)
		if err != nil {
			t.Fatalf("Explore failed: %v", err)
		}

		if len(result.Nodes) > 2 {
			t.Errorf("Expected at most 2 nodes due to limit, got %d", len(result.Nodes))
		}
	})
}

func TestExplore_DirectionFilter(t *testing.T) {
	g := NewGraph()

	node1 := &Node{ID: "node1", Kind: KindSymbol}
	node2 := &Node{ID: "node2", Kind: KindSymbol}
	node3 := &Node{ID: "node3", Kind: KindSymbol}

	g.AddNode(node1)
	g.AddNode(node2)
	g.AddNode(node3)

	// node1 -> node2, node3 -> node1
	g.AddEdge(&Edge{From: "node1", To: "node2", Type: EdgeInvokes})
	g.AddEdge(&Edge{From: "node3", To: "node1", Type: EdgeInvokes})

	qe := NewQueryEngine(g)

	t.Run("forward only", func(t *testing.T) {
		req := ExploreRequest{
			StartNodeID: "node1",
			Direction:   "forward",
			Depth:       1,
		}

		result, err := qe.Explore(context.Background(), req)
		if err != nil {
			t.Fatalf("Explore failed: %v", err)
		}

		if result.Nodes["node2"] == nil {
			t.Error("Should include node2 (forward)")
		}
		if result.Nodes["node3"] != nil {
			t.Error("Should NOT include node3 (reverse only)")
		}
	})

	t.Run("reverse only", func(t *testing.T) {
		req := ExploreRequest{
			StartNodeID: "node1",
			Direction:   "reverse",
			Depth:       1,
		}

		result, err := qe.Explore(context.Background(), req)
		if err != nil {
			t.Fatalf("Explore failed: %v", err)
		}

		if result.Nodes["node3"] == nil {
			t.Error("Should include node3 (reverse)")
		}
		if result.Nodes["node2"] != nil {
			t.Error("Should NOT include node2 (forward only)")
		}
	})
}

func TestExplore_EdgeTypeFilter(t *testing.T) {
	g := NewGraph()

	node1 := &Node{ID: "node1", Kind: KindSymbol}
	node2 := &Node{ID: "node2", Kind: KindSymbol}
	node3 := &Node{ID: "node3", Kind: KindSymbol}

	g.AddNode(node1)
	g.AddNode(node2)
	g.AddNode(node3)

	// Different edge types
	g.AddEdge(&Edge{From: "node1", To: "node2", Type: EdgeInvokes})
	g.AddEdge(&Edge{From: "node1", To: "node3", Type: EdgeContains})

	qe := NewQueryEngine(g)

	t.Run("filter by edge type", func(t *testing.T) {
		req := ExploreRequest{
			StartNodeID: "node1",
			Direction:   "forward",
			Depth:       1,
			EdgeTypes:   []EdgeType{EdgeInvokes},
		}

		result, err := qe.Explore(context.Background(), req)
		if err != nil {
			t.Fatalf("Explore failed: %v", err)
		}

		if result.Nodes["node2"] == nil {
			t.Error("Should include node2 (EdgeInvokes)")
		}
		if result.Nodes["node3"] != nil {
			t.Error("Should NOT include node3 (EdgeContains, filtered out)")
		}
	})

	t.Run("filter by multiple edge types", func(t *testing.T) {
		req := ExploreRequest{
			StartNodeID: "node1",
			Direction:   "forward",
			Depth:       1,
			EdgeTypes:   []EdgeType{EdgeInvokes, EdgeContains},
		}

		result, err := qe.Explore(context.Background(), req)
		if err != nil {
			t.Fatalf("Explore failed: %v", err)
		}

		if result.Nodes["node2"] == nil {
			t.Error("Should include node2")
		}
		if result.Nodes["node3"] == nil {
			t.Error("Should include node3")
		}
	})
}

func TestScoreMatch(t *testing.T) {
	tests := []struct {
		name        string
		queryWords  []string
		node        *Node
		expectScore float64
	}{
		{
			name:       "exact match",
			queryWords: []string{"handle", "request"},
			node: &Node{
				Feature:    "handle-request",
				SymbolName: "HandleRequest",
			},
			expectScore: 0.67, // 2 matches out of 3 unique words (handle, request, handlerequest)
		},
		{
			name:       "partial match",
			queryWords: []string{"handle"},
			node: &Node{
				Feature:    "handle-request",
				SymbolName: "HandleRequest",
			},
			expectScore: 0.33, // 1 match out of 3 unique words (handle, request, handlerequest)
		},
		{
			name:       "no match",
			queryWords: []string{"validate"},
			node: &Node{
				Feature:    "handle-request",
				SymbolName: "HandleRequest",
			},
			expectScore: 0,
		},
		{
			name:       "empty node",
			queryWords: []string{"handle"},
			node: &Node{
				Feature:    "",
				SymbolName: "",
			},
			expectScore: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := scoreMatch(tt.queryWords, tt.node)
			// Use a small epsilon for floating-point comparison
			epsilon := 0.01
			if score < tt.expectScore-epsilon || score > tt.expectScore+epsilon {
				t.Errorf("Expected score %.2f, got %.2f", tt.expectScore, score)
			}
		})
	}
}

func TestNormalizeWords(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "kebab-case",
			input:    "handle-request",
			expected: []string{"handle", "request"},
		},
		{
			name:     "camelCase symbol",
			input:    "HandleRequest",
			expected: []string{"handlerequest"},
		},
		{
			name:     "with receiver",
			input:    "handle-request@server",
			expected: []string{"handle", "request", "server"},
		},
		{
			name:     "natural language",
			input:    "handle the request",
			expected: []string{"handle", "the", "request"},
		},
		{
			name:     "mixed separators",
			input:    "handle-request/server@instance",
			expected: []string{"handle", "request", "server", "instance"},
		},
		{
			name:     "deduplication",
			input:    "handle-handle-request",
			expected: []string{"handle", "request"},
		},
		{
			name:     "empty",
			input:    "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeWords(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d words, got %d: %v", len(tt.expected), len(result), result)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("Word %d: expected %s, got %s", i, tt.expected[i], result[i])
				}
			}
		})
	}
}

func TestFindParentID(t *testing.T) {
	g := NewGraph()

	node := &Node{ID: "node1", Kind: KindSymbol}
	parent := &Node{ID: "parent1", Kind: KindSubcategory}

	g.AddNode(node)
	g.AddNode(parent)

	g.AddEdge(&Edge{From: "parent1", To: "node1", Type: EdgeFeatureParent})
	g.AddEdge(&Edge{From: "node1", To: "other", Type: EdgeInvokes})

	parentID := findParentID(g, "node1")
	if parentID != "parent1" {
		t.Errorf("Expected parent1, got %s", parentID)
	}

	// Test node with no parent
	noParent := findParentID(g, "parent1")
	if noParent != "" {
		t.Errorf("Expected empty parent, got %s", noParent)
	}
}

func TestGetFeaturePath(t *testing.T) {
	g := NewGraph()

	// Create hierarchy: area -> category -> subcategory -> file -> symbol.
	area := &Node{ID: "area:cli", Kind: KindArea, Feature: "cli"}
	cat := &Node{ID: "cat:cli/watch", Kind: KindCategory, Feature: "cli/watch"}
	subcat := &Node{ID: "subcat:cli/watch/handle", Kind: KindSubcategory, Feature: "cli/watch/handle"}
	file := &Node{ID: "file:cli/watch.go", Kind: KindFile, Feature: "watch", Path: "cli/watch.go"}
	sym := &Node{ID: "sym:cli/watch.go:HandleEvent", Kind: KindSymbol, Feature: "handle-event", Path: "cli/watch.go", SymbolName: "HandleEvent"}

	g.AddNode(area)
	g.AddNode(cat)
	g.AddNode(subcat)
	g.AddNode(file)
	g.AddNode(sym)

	// Edges: area->cat->subcat->file and file->symbol.
	g.AddEdge(&Edge{From: area.ID, To: cat.ID, Type: EdgeFeatureParent})
	g.AddEdge(&Edge{From: cat.ID, To: subcat.ID, Type: EdgeFeatureParent})
	g.AddEdge(&Edge{From: subcat.ID, To: file.ID, Type: EdgeFeatureParent})
	g.AddEdge(&Edge{From: file.ID, To: sym.ID, Type: EdgeContains})

	qe := NewQueryEngine(g)

	tests := []struct {
		name     string
		nodeID   string
		expected string
	}{
		{"area returns own feature", area.ID, "cli"},
		{"category returns own feature", cat.ID, "cli/watch"},
		{"subcategory returns own feature", subcat.ID, "cli/watch/handle"},
		{"file returns parent feature path", file.ID, "cli/watch/handle"},
		{"symbol returns parent feature path", sym.ID, "cli/watch/handle"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := qe.getFeaturePath(tt.nodeID)
			if got != tt.expected {
				t.Errorf("getFeaturePath(%q) = %q, want %q", tt.nodeID, got, tt.expected)
			}
		})
	}
}

func TestSearchNode_ExtendedModes(t *testing.T) {
	g := NewGraph()
	ext := NewLocalExtractor()
	h := NewHierarchyBuilder(g, ext)

	file := &Node{ID: "file:cli/server.go", Kind: KindFile, Path: "cli/server.go", Feature: "server"}
	sym1 := &Node{ID: "sym1", Kind: KindSymbol, Feature: "handle-request", SymbolName: "HandleRequest", Path: "cli/server.go"}
	sym2 := &Node{ID: "sym2", Kind: KindSymbol, Feature: "validate-token", SymbolName: "ValidateToken", Path: "cli/server.go"}
	g.AddNode(file)
	g.AddNode(sym1)
	g.AddNode(sym2)
	g.AddEdge(&Edge{From: file.ID, To: sym1.ID, Type: EdgeContains})
	g.AddEdge(&Edge{From: file.ID, To: sym2.ID, Type: EdgeContains})
	h.BuildHierarchy()

	qe := NewQueryEngine(g)

	t.Run("features mode uses feature terms", func(t *testing.T) {
		results, err := qe.SearchNode(context.Background(), SearchNodeRequest{
			Mode:         "features",
			FeatureTerms: []string{"validate token"},
			Limit:        5,
		})
		if err != nil {
			t.Fatalf("SearchNode failed: %v", err)
		}
		if len(results) == 0 || results[0].Node.ID != sym2.ID {
			t.Fatalf("expected sym2 to rank first in features mode, got %+v", results)
		}
	})

	t.Run("auto mode falls back to search terms", func(t *testing.T) {
		results, err := qe.SearchNode(context.Background(), SearchNodeRequest{
			Mode:         "auto",
			FeatureTerms: []string{"nonexistent behavior"},
			SearchTerms:  []string{"handle request"},
			Limit:        5,
		})
		if err != nil {
			t.Fatalf("SearchNode failed: %v", err)
		}
		if len(results) == 0 || results[0].Node.ID != sym1.ID {
			t.Fatalf("expected sym1 to rank first in auto fallback mode, got %+v", results)
		}
	})
}

func TestSearchNode_ExtendedScopeAndPathFilters(t *testing.T) {
	g := NewGraph()
	ext := NewLocalExtractor()
	h := NewHierarchyBuilder(g, ext)

	fileCLI := &Node{ID: "file:cli/server.go", Kind: KindFile, Path: "cli/server.go", Feature: "server"}
	fileStore := &Node{ID: "file:store/server.go", Kind: KindFile, Path: "store/server.go", Feature: "server"}
	symCLI := &Node{ID: "sym-cli", Kind: KindSymbol, Feature: "handle-request", SymbolName: "HandleRequest", Path: "cli/server.go"}
	symStore := &Node{ID: "sym-store", Kind: KindSymbol, Feature: "handle-request", SymbolName: "HandleRequest", Path: "store/server.go"}

	g.AddNode(fileCLI)
	g.AddNode(fileStore)
	g.AddNode(symCLI)
	g.AddNode(symStore)
	g.AddEdge(&Edge{From: fileCLI.ID, To: symCLI.ID, Type: EdgeContains})
	g.AddEdge(&Edge{From: fileStore.ID, To: symStore.ID, Type: EdgeContains})
	h.BuildHierarchy()

	qe := NewQueryEngine(g)

	results, err := qe.SearchNode(context.Background(), SearchNodeRequest{
		Mode:         "features",
		FeatureTerms: []string{"handle request"},
		SearchScopes: []string{"store"},
		Limit:        5,
	})
	if err != nil {
		t.Fatalf("SearchNode failed: %v", err)
	}
	if len(results) != 1 || results[0].Node.ID != symStore.ID {
		t.Fatalf("expected only store symbol with search_scopes filter, got %+v", results)
	}

	results, err = qe.SearchNode(context.Background(), SearchNodeRequest{
		Mode:              "features",
		FeatureTerms:      []string{"handle request"},
		FilePathOrPattern: "cli/*",
		Limit:             5,
	})
	if err != nil {
		t.Fatalf("SearchNode failed: %v", err)
	}
	if len(results) != 1 || results[0].Node.ID != symCLI.ID {
		t.Fatalf("expected only cli symbol with file path pattern, got %+v", results)
	}
}

func TestFetchNodes_ResolvesCodeAndFeatureEntities(t *testing.T) {
	g := NewGraph()
	ext := NewLocalExtractor()
	h := NewHierarchyBuilder(g, ext)

	file := &Node{ID: "file:cli/server.go", Kind: KindFile, Path: "cli/server.go", Feature: "server"}
	sym := &Node{ID: "sym1", Kind: KindSymbol, Feature: "handle-request", SymbolName: "HandleRequest", Path: "cli/server.go"}
	g.AddNode(file)
	g.AddNode(sym)
	g.AddEdge(&Edge{From: file.ID, To: sym.ID, Type: EdgeContains})
	h.BuildHierarchy()

	qe := NewQueryEngine(g)

	codeResults, err := qe.FetchNodes(context.Background(), FetchNodeRequest{
		CodeEntities: []string{"HandleRequest"},
	})
	if err != nil {
		t.Fatalf("FetchNodes failed: %v", err)
	}
	if len(codeResults) == 0 || codeResults[0].Node.SymbolName != "HandleRequest" {
		t.Fatalf("expected symbol match by code entity, got %+v", codeResults)
	}

	featureResults, err := qe.FetchNodes(context.Background(), FetchNodeRequest{
		FeatureEntities: []string{"cli/server/request"},
	})
	if err != nil {
		t.Fatalf("FetchNodes failed: %v", err)
	}
	if len(featureResults) == 0 || featureResults[0].Node.Kind != KindSubcategory {
		t.Fatalf("expected subcategory match by feature entity, got %+v", featureResults)
	}
}

func TestExplore_UsesStartCodeEntitiesAndEntityFilter(t *testing.T) {
	g := NewGraph()
	file := &Node{ID: "file:main.go", Kind: KindFile, Path: "main.go"}
	sym := &Node{ID: "sym:main.go:Handle", Kind: KindSymbol, Path: "main.go", SymbolName: "Handle"}
	g.AddNode(file)
	g.AddNode(sym)
	g.AddEdge(&Edge{From: file.ID, To: sym.ID, Type: EdgeContains})

	qe := NewQueryEngine(g)

	result, err := qe.Explore(context.Background(), ExploreRequest{
		StartCodeEntities: []string{"main.go"},
		Direction:         "forward",
		Depth:             1,
		EntityTypeFilter:  "function",
	})
	if err != nil {
		t.Fatalf("Explore failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil explore result")
	}
	if result.Nodes[sym.ID] == nil {
		t.Fatalf("expected symbol node to be included with function filter, got %+v", result.Nodes)
	}
}
