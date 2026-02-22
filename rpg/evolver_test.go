package rpg

import (
	"context"
	"testing"

	"github.com/yoanbernabeu/grepai/trace"
)

func TestHandleAdd(t *testing.T) {
	g := NewGraph()
	ext := NewLocalExtractor()
	h := NewHierarchyBuilder(g, ext)
	ev := NewEvolver(g, ext, h, 0.3)

	symbols := []trace.Symbol{
		{
			Name:      "HandleRequest",
			Signature: "func HandleRequest() error",
			Language:  "go",
			Line:      10,
			EndLine:   20,
		},
		{
			Name:      "ValidateToken",
			Signature: "func ValidateToken(token string) bool",
			Language:  "go",
			Line:      25,
			EndLine:   35,
		},
	}

	ev.HandleAdd(context.Background(), "cli/server.go", symbols)

	// Verify file node was created
	fileNode := g.GetNode("file:cli/server.go")
	if fileNode == nil {
		t.Fatal("File node should be created")
	}
	if fileNode.Kind != KindFile {
		t.Errorf("Expected kind %s, got %s", KindFile, fileNode.Kind)
	}

	// Verify symbol nodes were created
	sym1Node := g.GetNode("sym:cli/server.go:HandleRequest")
	if sym1Node == nil {
		t.Fatal("Symbol node 'HandleRequest' should be created")
	}
	if sym1Node.Kind != KindSymbol {
		t.Errorf("Expected kind %s, got %s", KindSymbol, sym1Node.Kind)
	}
	if sym1Node.Feature != "handle-request" {
		t.Errorf("Expected feature 'handle-request', got %s", sym1Node.Feature)
	}
	if sym1Node.SymbolName != "HandleRequest" {
		t.Errorf("Expected symbol name 'HandleRequest', got %s", sym1Node.SymbolName)
	}

	sym2Node := g.GetNode("sym:cli/server.go:ValidateToken")
	if sym2Node == nil {
		t.Fatal("Symbol node 'ValidateToken' should be created")
	}

	// Verify hierarchy was created
	areaNode := g.GetNode("area:cli")
	if areaNode == nil {
		t.Fatal("Area node should be created")
	}

	catNode := g.GetNode("cat:cli/server")
	if catNode == nil {
		t.Fatal("Category node should be created")
	}

	subcatNode := g.GetNode("subcat:cli/server/request")
	if subcatNode == nil {
		t.Fatal("Subcategory node should be created")
	}

	// Verify subcategory -> file edge (parent -> child)
	fileIncoming := g.GetIncoming(fileNode.ID)
	hasFeatureParent := false
	for _, e := range fileIncoming {
		if e.Type == EdgeFeatureParent && e.From == subcatNode.ID {
			hasFeatureParent = true
			break
		}
	}
	if !hasFeatureParent {
		t.Error("File should have incoming EdgeFeatureParent from subcategory")
	}

	// Verify file -> symbol edge
	fileEdges := g.GetOutgoing(fileNode.ID)
	hasContains := false
	for _, e := range fileEdges {
		if e.Type == EdgeContains && (e.To == sym1Node.ID || e.To == sym2Node.ID) {
			hasContains = true
			break
		}
	}
	if !hasContains {
		t.Error("File should have EdgeContains to symbols")
	}
}

func TestHandleDelete(t *testing.T) {
	g := NewGraph()
	ext := NewLocalExtractor()
	h := NewHierarchyBuilder(g, ext)
	ev := NewEvolver(g, ext, h, 0.3)

	// Add a file with symbols first
	symbols := []trace.Symbol{
		{Name: "HandleRequest", Signature: "func HandleRequest()", Language: "go", Line: 10, EndLine: 20},
	}
	ev.HandleAdd(context.Background(), "cli/server.go", symbols)

	// Verify nodes exist
	if g.GetNode("file:cli/server.go") == nil {
		t.Fatal("Setup: file node should exist")
	}
	if g.GetNode("sym:cli/server.go:HandleRequest") == nil {
		t.Fatal("Setup: symbol node should exist")
	}

	initialNodeCount := len(g.Nodes)

	// Delete the file
	ev.HandleDelete(context.Background(), "cli/server.go")

	// Verify file node was removed
	if g.GetNode("file:cli/server.go") != nil {
		t.Error("File node should be removed")
	}

	// Verify symbol node was removed
	if g.GetNode("sym:cli/server.go:HandleRequest") != nil {
		t.Error("Symbol node should be removed")
	}

	// Verify orphaned hierarchy nodes were pruned
	finalNodeCount := len(g.Nodes)
	if finalNodeCount >= initialNodeCount {
		t.Error("Some nodes should have been removed")
	}

	// Area, category, and subcategory should be pruned if orphaned
	// (in this test, they should be pruned since there are no other files)
	if g.GetNode("subcat:cli/server/request") != nil {
		t.Error("Orphaned subcategory should be pruned")
	}
}

func TestHandleModify(t *testing.T) {
	g := NewGraph()
	ext := NewLocalExtractor()
	h := NewHierarchyBuilder(g, ext)
	ev := NewEvolver(g, ext, h, 0.3)

	// Add initial symbols
	initialSymbols := []trace.Symbol{
		{Name: "HandleRequest", Signature: "func HandleRequest()", Language: "go", Line: 10, EndLine: 20},
		{Name: "ValidateToken", Signature: "func ValidateToken()", Language: "go", Line: 25, EndLine: 35},
	}
	ev.HandleAdd(context.Background(), "server.go", initialSymbols)

	sym1 := g.GetNode("sym:server.go:HandleRequest")
	if sym1 == nil {
		t.Fatal("Setup: symbol should exist")
	}
	originalFeature := sym1.Feature

	// Modify with similar symbol (low drift)
	modifiedSymbols := []trace.Symbol{
		{Name: "HandleRequest", Signature: "func HandleRequest(ctx context.Context)", Language: "go", Line: 10, EndLine: 25},
		{Name: "ProcessRequest", Signature: "func ProcessRequest()", Language: "go", Line: 30, EndLine: 40},
	}
	ev.HandleModify(context.Background(), "server.go", modifiedSymbols)

	// HandleRequest should still exist (low drift, in-place update)
	sym1After := g.GetNode("sym:server.go:HandleRequest")
	if sym1After == nil {
		t.Fatal("Symbol 'HandleRequest' should still exist after low-drift modification")
	}
	if sym1After.Signature != "func HandleRequest(ctx context.Context)" {
		t.Error("Symbol signature should be updated")
	}
	if sym1After.EndLine != 25 {
		t.Errorf("Symbol end line should be updated to 25, got %d", sym1After.EndLine)
	}

	// ValidateToken should be removed
	if g.GetNode("sym:server.go:ValidateToken") != nil {
		t.Error("Removed symbol 'ValidateToken' should not exist")
	}

	// ProcessRequest should be added
	sym3 := g.GetNode("sym:server.go:ProcessRequest")
	if sym3 == nil {
		t.Error("New symbol 'ProcessRequest' should be added")
	}

	// Test high drift case - add a symbol with completely different feature
	veryDifferentSymbols := []trace.Symbol{
		{Name: "CompletelyDifferentFunc", Signature: "func CompletelyDifferentFunc()", Language: "go", Line: 10, EndLine: 20},
	}

	initialCount := len(g.Nodes)
	ev.HandleModify(context.Background(), "server.go", veryDifferentSymbols)

	// HandleRequest should be gone
	if g.GetNode("sym:server.go:HandleRequest") != nil {
		t.Error("Old symbol should be removed")
	}

	// New symbol should exist
	if g.GetNode("sym:server.go:CompletelyDifferentFunc") == nil {
		t.Error("New symbol should be added")
	}

	_ = originalFeature
	_ = initialCount

	fileNode := g.GetNode("file:server.go")
	if fileNode == nil {
		t.Fatal("file node should exist")
	}
	hasOperateParent := false
	for _, e := range g.GetIncoming(fileNode.ID) {
		if e.Type == EdgeFeatureParent && e.From == "subcat:root/server/operate" {
			hasOperateParent = true
			break
		}
	}
	if !hasOperateParent {
		t.Error("file should be re-routed to operate subcategory after high drift")
	}
	if g.GetNode("subcat:root/server/request") != nil {
		t.Error("old request subcategory should be pruned after reroute")
	}
}

func TestHandleModify_DoesNotCreateOrphanSubcategoryWhenDriftBelowThreshold(t *testing.T) {
	g := NewGraph()
	ext := NewLocalExtractor()
	h := NewHierarchyBuilder(g, ext)
	// Use a high threshold so semantic movement does not trigger reroute.
	ev := NewEvolver(g, ext, h, 0.9)

	ev.HandleAdd(context.Background(), "server.go", []trace.Symbol{
		{Name: "HandleRequest", Signature: "func HandleRequest()", Language: "go", Line: 10, EndLine: 20},
	})

	if g.GetNode("subcat:root/server/request") == nil {
		t.Fatal("setup failed: expected request subcategory to exist")
	}
	if g.GetNode("subcat:root/server/process") != nil {
		t.Fatal("setup failed: process subcategory should not exist yet")
	}

	// This changes the dominant subcategory candidate from request -> process,
	// but drift (0.667) stays below threshold (0.9), so no reroute should happen.
	ev.HandleModify(context.Background(), "server.go", []trace.Symbol{
		{Name: "ProcessRequest", Signature: "func ProcessRequest()", Language: "go", Line: 10, EndLine: 20},
	})

	if g.GetNode("subcat:root/server/process") != nil {
		t.Error("low-drift modification should not create orphan process subcategory")
	}

	fileNode := g.GetNode("file:server.go")
	if fileNode == nil {
		t.Fatal("file node should exist")
	}

	hasRequestParent := false
	for _, e := range g.GetIncoming(fileNode.ID) {
		if e.Type == EdgeFeatureParent && e.From == "subcat:root/server/request" {
			hasRequestParent = true
			break
		}
	}
	if !hasRequestParent {
		t.Error("file should remain attached to request subcategory when drift is below threshold")
	}
}

func TestCalculateDrift(t *testing.T) {
	tests := []struct {
		name        string
		oldFeatures []string
		newFeatures []string
		expected    float64
		description string
	}{
		{
			name:        "identical",
			oldFeatures: []string{"handle request"},
			newFeatures: []string{"handle request"},
			expected:    0.0,
			description: "same strings should have zero drift",
		},
		{
			name:        "completely different",
			oldFeatures: []string{"handle request"},
			newFeatures: []string{"validate token"},
			expected:    1.0,
			description: "no overlapping words",
		},
		{
			name:        "partial overlap",
			oldFeatures: []string{"handle request"},
			newFeatures: []string{"handle response"},
			expected:    0.667,
			description: "one word in common (handle) out of 3 unique words, jaccard distance = 1 - 1/3 = 0.667",
		},
		{
			name:        "empty old",
			oldFeatures: nil,
			newFeatures: []string{"handle request"},
			expected:    1.0,
			description: "empty string treated as completely different",
		},
		{
			name:        "empty new",
			oldFeatures: []string{"handle request"},
			newFeatures: nil,
			expected:    1.0,
			description: "empty string treated as completely different",
		},
		{
			name:        "both empty",
			oldFeatures: nil,
			newFeatures: nil,
			expected:    0.0,
			description: "both empty returns 0.0 per implementation",
		},
		{
			name:        "multi feature set",
			oldFeatures: []string{"handle request", "validate token"},
			newFeatures: []string{"handle request", "parse config"},
			expected:    0.667,
			description: "intersection 2 words (handle/request) over union 6 words",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateDrift(tt.oldFeatures, tt.newFeatures)
			if result != tt.expected {
				t.Errorf("Expected drift %.3f, got %.3f (%s)", tt.expected, result, tt.description)
			}
		})
	}
}

func TestSplitFeatureWords(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]bool
	}{
		{
			name:     "hyphen separated",
			input:    "handle-request",
			expected: map[string]bool{"handle": true, "request": true},
		},
		{
			name:     "with receiver",
			input:    "handle-request@server",
			expected: map[string]bool{"handle": true, "request": true, "server": true},
		},
		{
			name:     "slash separated",
			input:    "cli/watch",
			expected: map[string]bool{"cli": true, "watch": true},
		},
		{
			name:     "underscore separated",
			input:    "get_user_id",
			expected: map[string]bool{"get": true, "user": true, "id": true},
		},
		{
			name:     "mixed separators",
			input:    "handle-request/server@instance",
			expected: map[string]bool{"handle": true, "request": true, "server": true, "instance": true},
		},
		{
			name:     "uppercase",
			input:    "Handle-Request",
			expected: map[string]bool{"handle": true, "request": true},
		},
		{
			name:     "empty",
			input:    "",
			expected: map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitFeatureWords(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d words, got %d", len(tt.expected), len(result))
			}
			for word := range tt.expected {
				if !result[word] {
					t.Errorf("Expected word %s not found in result", word)
				}
			}
			for word := range result {
				if !tt.expected[word] {
					t.Errorf("Unexpected word %s found in result", word)
				}
			}
		})
	}
}

func TestMakeSymbolNodeID(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		symbol   trace.Symbol
		expected string
	}{
		{
			name:     "without receiver",
			filePath: "file.go",
			symbol:   trace.Symbol{Name: "HandleRequest", Receiver: ""},
			expected: "sym:file.go:HandleRequest",
		},
		{
			name:     "with receiver",
			filePath: "file.go",
			symbol:   trace.Symbol{Name: "Save", Receiver: "Config"},
			expected: "sym:file.go:Config.Save",
		},
		{
			name:     "with pointer receiver",
			filePath: "server.go",
			symbol:   trace.Symbol{Name: "Start", Receiver: "*Server"},
			expected: "sym:server.go:*Server.Start",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := makeSymbolNodeID(tt.filePath, tt.symbol)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestCollectFeatureParents(t *testing.T) {
	g := NewGraph()

	node := &Node{ID: "sym1", Kind: KindSymbol}
	parent1 := &Node{ID: "subcat1", Kind: KindSubcategory}
	parent2 := &Node{ID: "file1", Kind: KindFile}

	g.AddNode(node)
	g.AddNode(parent1)
	g.AddNode(parent2)

	g.AddEdge(&Edge{From: "subcat1", To: "sym1", Type: EdgeFeatureParent})
	g.AddEdge(&Edge{From: "file1", To: "sym1", Type: EdgeContains}) // Should not be collected

	parents := collectFeatureParents(g, "sym1")

	if len(parents) != 1 {
		t.Errorf("Expected 1 feature parent, got %d", len(parents))
	}
	if !parents["subcat1"] {
		t.Error("Should have collected subcat1")
	}
	if parents["file1"] {
		t.Error("Should not have collected file1 (wrong edge type)")
	}
}
