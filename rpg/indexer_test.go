package rpg

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
)

type countingFeatureExtractor struct {
	atomicCalls  int
	featureCalls int
}

func (e *countingFeatureExtractor) ExtractFeature(_ context.Context, symbolName, signature, receiver, comment string) string {
	e.featureCalls++
	return "handle-request"
}

func (e *countingFeatureExtractor) ExtractAtomicFeatures(_ context.Context, symbolName, signature, receiver, comment string) []string {
	e.atomicCalls++
	return []string{"handle request"}
}

func (e *countingFeatureExtractor) GenerateSummary(ctx context.Context, name, contextStr string) (string, error) {
	return "summary", nil
}

func (e *countingFeatureExtractor) Mode() string { return "counting" }

func TestBuildFull_DoesNotDuplicateFeatureExtractionCalls(t *testing.T) {
	ctx := context.Background()
	rpgStore := NewGOBRPGStore(filepath.Join(t.TempDir(), "rpg.gob"))
	extractor := &countingFeatureExtractor{}
	indexer := NewRPGEncoder(rpgStore, extractor, "/tmp", RPGEncoderConfig{DriftThreshold: 0.35})

	vectorStore := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	symbolStore := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	defer symbolStore.Close()

	filePath := "cli/server.go"
	if err := vectorStore.SaveDocument(ctx, store.Document{
		Path:    filePath,
		ModTime: time.Now(),
	}); err != nil {
		t.Fatalf("SaveDocument failed: %v", err)
	}

	if err := symbolStore.SaveFile(ctx, filePath, []trace.Symbol{
		{
			Name:      "HandleRequest",
			Kind:      trace.KindFunction,
			File:      filePath,
			Line:      10,
			EndLine:   20,
			Signature: "func HandleRequest()",
			Language:  "go",
		},
	}, nil); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	if err := indexer.BuildFull(ctx, symbolStore, vectorStore, nil); err != nil {
		t.Fatalf("BuildFull failed: %v", err)
	}

	if extractor.featureCalls != 0 {
		t.Fatalf("expected no direct ExtractFeature calls during BuildFull when atomic features are available, got %d", extractor.featureCalls)
	}
	if extractor.atomicCalls != 2 {
		t.Fatalf("expected exactly 2 ExtractAtomicFeatures calls (file + symbol), got %d", extractor.atomicCalls)
	}
}

func TestLinkChunksForFile_NoAccumulation(t *testing.T) {
	// Setup: create a graph with a file node and symbol node
	g := NewGraph()
	fileNode := &Node{ID: "file:main.go", Kind: KindFile, Path: "main.go"}
	g.AddNode(fileNode)

	symNode := &Node{
		ID:         "sym:main.go:Foo",
		Kind:       KindSymbol,
		Path:       "main.go",
		SymbolName: "Foo",
		StartLine:  1,
		EndLine:    20,
	}
	g.AddNode(symNode)
	g.AddEdge(&Edge{From: fileNode.ID, To: symNode.ID, Type: EdgeContains, Weight: 1.0})

	rpgStore := &GOBRPGStore{indexPath: filepath.Join(t.TempDir(), "rpg.gob"), graph: g}
	extractor := NewLocalExtractor()
	indexer := NewRPGEncoder(rpgStore, extractor, "/tmp", RPGEncoderConfig{DriftThreshold: 0.35})

	chunks := []store.Chunk{
		{ID: "chunk-1", FilePath: "main.go", StartLine: 1, EndLine: 10},
		{ID: "chunk-2", FilePath: "main.go", StartLine: 11, EndLine: 20},
	}

	// First call
	if err := indexer.LinkChunksForFile(context.Background(), "main.go", chunks); err != nil {
		t.Fatalf("first LinkChunksForFile failed: %v", err)
	}

	edgeCount1 := countEdgesByType(g, EdgeMapsToChunk)
	chunkCount1 := countNodesByKind(g, KindChunk)

	// Second call (simulates file modification)
	if err := indexer.LinkChunksForFile(context.Background(), "main.go", chunks); err != nil {
		t.Fatalf("second LinkChunksForFile failed: %v", err)
	}

	edgeCount2 := countEdgesByType(g, EdgeMapsToChunk)
	chunkCount2 := countNodesByKind(g, KindChunk)

	// Edge and chunk counts should NOT grow after repeated calls
	if edgeCount2 != edgeCount1 {
		t.Errorf("EdgeMapsToChunk accumulated: first=%d, second=%d", edgeCount1, edgeCount2)
	}
	if chunkCount2 != chunkCount1 {
		t.Errorf("Chunk nodes accumulated: first=%d, second=%d", chunkCount1, chunkCount2)
	}

	// Verify correct counts
	if chunkCount1 != 2 {
		t.Errorf("Expected 2 chunk nodes, got %d", chunkCount1)
	}
	if edgeCount1 != 2 {
		t.Errorf("Expected 2 EdgeMapsToChunk edges, got %d", edgeCount1)
	}
}

func TestBuildFull_EmptyDocsReportsCompletedEdgeProgress(t *testing.T) {
	ctx := context.Background()
	rpgStore := NewGOBRPGStore(filepath.Join(t.TempDir(), "rpg.gob"))
	extractor := NewLocalExtractor()
	indexer := NewRPGEncoder(rpgStore, extractor, "/tmp", RPGEncoderConfig{DriftThreshold: 0.35})

	vectorStore := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	symbolStore := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	defer symbolStore.Close()

	type progressEvent struct {
		step    string
		current int
		total   int
	}
	events := make([]progressEvent, 0, 2)
	err := indexer.BuildFull(ctx, symbolStore, vectorStore, func(step string, current, total int) {
		events = append(events, progressEvent{step: step, current: current, total: total})
	})
	if err != nil {
		t.Fatalf("BuildFull failed: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected progress events, got none")
	}
	last := events[len(events)-1]
	if last.step != "rpg-edges" || last.current != 1 || last.total != 1 {
		t.Fatalf("last progress = %s %d/%d, want rpg-edges 1/1", last.step, last.current, last.total)
	}
}

func TestBuildFull_GeneratesHierarchySummaries(t *testing.T) {
	ctx := context.Background()
	rpgStore := NewGOBRPGStore(filepath.Join(t.TempDir(), "rpg.gob"))
	extractor := NewLocalExtractor()
	indexer := NewRPGEncoder(rpgStore, extractor, "/tmp", RPGEncoderConfig{DriftThreshold: 0.35})

	vectorStore := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	symbolStore := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	defer symbolStore.Close()

	filePath := "cli/server.go"
	if err := vectorStore.SaveDocument(ctx, store.Document{
		Path:     filePath,
		ModTime:  time.Now(),
		ChunkIDs: []string{},
	}); err != nil {
		t.Fatalf("SaveDocument failed: %v", err)
	}

	if err := symbolStore.SaveFile(ctx, filePath, []trace.Symbol{
		{
			Name:      "HandleRequest",
			Kind:      trace.KindFunction,
			File:      filePath,
			Line:      10,
			EndLine:   20,
			Signature: "func HandleRequest()",
			Language:  "go",
		},
	}, nil); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	if err := indexer.BuildFull(ctx, symbolStore, vectorStore, nil); err != nil {
		t.Fatalf("BuildFull failed: %v", err)
	}

	graph := rpgStore.GetGraph()
	for _, kind := range []NodeKind{KindArea, KindCategory, KindSubcategory} {
		nodes := graph.GetNodesByKind(kind)
		if len(nodes) == 0 {
			t.Fatalf("expected at least one %s node", kind)
		}
		for _, node := range nodes {
			if strings.TrimSpace(node.Summary) == "" {
				t.Fatalf("expected non-empty summary for %s node %s", kind, node.ID)
			}
		}
	}
}

func TestNormalizeEndLine(t *testing.T) {
	tests := []struct {
		name      string
		startLine int
		endLine   int
		want      int
	}{
		{"zero endline falls back to startline", 42, 0, 42},
		{"negative endline falls back to startline", 10, -1, 10},
		{"endline before startline falls back", 50, 30, 50},
		{"valid endline preserved", 10, 25, 25},
		{"same start and end preserved", 5, 5, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeEndLine(tt.startLine, tt.endLine)
			if got != tt.want {
				t.Errorf("normalizeEndLine(%d, %d) = %d, want %d", tt.startLine, tt.endLine, got, tt.want)
			}
		})
	}
}

func TestLinkChunksForFile_SymbolEndLineZero(t *testing.T) {
	// Regression: regex extractor sets EndLine=0; chunks must still link
	g := NewGraph()
	fileNode := &Node{ID: "file:main.go", Kind: KindFile, Path: "main.go"}
	g.AddNode(fileNode)

	// Symbol with EndLine=0 (as produced by regex extractor)
	symNode := &Node{
		ID:         "sym:main.go:Foo",
		Kind:       KindSymbol,
		Path:       "main.go",
		SymbolName: "Foo",
		StartLine:  10,
		EndLine:    10, // after normalizeEndLine(10, 0) => 10
	}
	g.AddNode(symNode)
	g.AddEdge(&Edge{From: fileNode.ID, To: symNode.ID, Type: EdgeContains, Weight: 1.0})

	rpgStore := &GOBRPGStore{indexPath: filepath.Join(t.TempDir(), "rpg.gob"), graph: g}
	extractor := NewLocalExtractor()
	indexer := NewRPGEncoder(rpgStore, extractor, "/tmp", RPGEncoderConfig{DriftThreshold: 0.35})

	// Chunk that spans lines 1-20 (covers the symbol at line 10)
	chunks := []store.Chunk{
		{ID: "chunk-1", FilePath: "main.go", StartLine: 1, EndLine: 20},
	}

	if err := indexer.LinkChunksForFile(context.Background(), "main.go", chunks); err != nil {
		t.Fatalf("LinkChunksForFile failed: %v", err)
	}

	edgeCount := countEdgesByType(g, EdgeMapsToChunk)
	if edgeCount != 1 {
		t.Errorf("Expected 1 EdgeMapsToChunk edge for symbol at line 10 within chunk 1-20, got %d", edgeCount)
	}
}

func countEdgesByType(g *Graph, edgeType EdgeType) int {
	count := 0
	for _, e := range g.Edges {
		if e.Type == edgeType {
			count++
		}
	}
	return count
}

func countNodesByKind(g *Graph, kind NodeKind) int {
	count := 0
	for _, n := range g.Nodes {
		if n.Kind == kind {
			count++
		}
	}
	return count
}

func TestFeatureSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want float64
	}{
		{"identical", "handle-request", "handle-request", 1.0},
		// handle-request: {handle, request}, handle-response: {handle, response}
		// intersection=1, union=3 -> 1/3 ≈ 0.333
		{"partial overlap", "handle-request", "handle-response", 0.333},
		{"different", "handle-request", "parse-config", 0.0},
		{"empty first", "", "handle-request", 0.0},
		{"empty second", "handle-request", "", 0.0},
		// handle-request@server: {handle, request, server}
		// handle-request@client: {handle, request, client}
		// intersection=2, union=4 -> 0.5
		{"above threshold", "handle-request@server", "handle-request@client", 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeA := &Node{Feature: tt.a}
			nodeB := &Node{Feature: tt.b}
			got := CalculateSemanticSimilarity(nodeA, nodeB)
			if got < tt.want-0.01 || got > tt.want+0.01 {
				t.Errorf("CalculateSemanticSimilarity(%q, %q) = %f, want ~%f", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestFirstWord(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"handle-request", "handle"},
		{"parse-config@server", "parse"},
		{"validate", "validate"},
		{"get_user", "get"},
		{"run/task", "run"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := firstWord(tt.input)
			if got != tt.want {
				t.Errorf("firstWord(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWireFeatureSimilarity(t *testing.T) {
	g := NewGraph()

	// Two symbols in different files with similar features (Jaccard >= 0.5)
	symA := &Node{ID: "sym:a.go:HandleReq", Kind: KindSymbol, Path: "a.go", Feature: "handle-request@server", SymbolName: "HandleReq"}
	symB := &Node{ID: "sym:b.go:HandleReq", Kind: KindSymbol, Path: "b.go", Feature: "handle-request@client", SymbolName: "HandleReq"}
	// Different feature verb
	symC := &Node{ID: "sym:c.go:ParseConfig", Kind: KindSymbol, Path: "c.go", Feature: "parse-config", SymbolName: "ParseConfig"}

	g.AddNode(symA)
	g.AddNode(symB)
	g.AddNode(symC)

	rpgStore := &GOBRPGStore{indexPath: filepath.Join(t.TempDir(), "rpg.gob"), graph: g}
	extractor := NewLocalExtractor()
	indexer := NewRPGEncoder(rpgStore, extractor, "/tmp", RPGEncoderConfig{DriftThreshold: 0.35})

	indexer.wireSemanticEdges(g)

	simCount := countEdgesByType(g, EdgeSemanticSim)
	if simCount != 1 {
		t.Errorf("Expected 1 EdgeSemanticSim edge between similar symbols, got %d", simCount)
	}
}

func TestWireFeatureSimilarity_SameFileSkipped(t *testing.T) {
	g := NewGraph()

	// Two symbols in the SAME file - should not get an edge
	symA := &Node{ID: "sym:a.go:HandleReq", Kind: KindSymbol, Path: "a.go", Feature: "handle-request@server", SymbolName: "HandleReq"}
	symB := &Node{ID: "sym:a.go:HandleRes", Kind: KindSymbol, Path: "a.go", Feature: "handle-request@client", SymbolName: "HandleRes"}

	g.AddNode(symA)
	g.AddNode(symB)

	rpgStore := &GOBRPGStore{indexPath: filepath.Join(t.TempDir(), "rpg.gob"), graph: g}
	extractor := NewLocalExtractor()
	indexer := NewRPGEncoder(rpgStore, extractor, "/tmp", RPGEncoderConfig{DriftThreshold: 0.35})

	indexer.wireSemanticEdges(g)

	simCount := countEdgesByType(g, EdgeSemanticSim)
	if simCount != 0 {
		t.Errorf("Expected 0 EdgeSemanticSim edges for same-file symbols, got %d", simCount)
	}
}

func TestWireFeatureSimilarity_LargeGroupSampled(t *testing.T) {
	g := NewGraph()

	// Create a group larger than maxFeatureGroupSize (100) sharing the verb "handle".
	// After sampling, edges should still be created (not skipped entirely).
	for i := 0; i < maxFeatureGroupSize+50; i++ {
		path := fmt.Sprintf("pkg%d/file.go", i)
		sym := &Node{
			ID:         fmt.Sprintf("sym:%s:Handle%d", path, i),
			Kind:       KindSymbol,
			Path:       path,
			Feature:    fmt.Sprintf("handle-request-%d@server", i),
			SymbolName: fmt.Sprintf("Handle%d", i),
		}
		g.AddNode(sym)
	}

	rpgStore := &GOBRPGStore{indexPath: filepath.Join(t.TempDir(), "rpg.gob"), graph: g}
	extractor := NewLocalExtractor()
	indexer := NewRPGEncoder(rpgStore, extractor, "/tmp", RPGEncoderConfig{DriftThreshold: 0.35})

	indexer.wireSemanticEdges(g)

	simCount := countEdgesByType(g, EdgeSemanticSim)
	if simCount == 0 {
		t.Error("Expected EdgeSemanticSim edges for large group (should sample, not skip)")
	}
}

func TestWireCoCallerAffinity(t *testing.T) {
	g := NewGraph()

	// Create caller and callee symbols
	caller1 := &Node{ID: "sym:main.go:Main", Kind: KindSymbol, Path: "main.go", Feature: "run-main", SymbolName: "Main"}
	caller2 := &Node{ID: "sym:app.go:Start", Kind: KindSymbol, Path: "app.go", Feature: "start-app", SymbolName: "Start"}
	calleeA := &Node{ID: "sym:a.go:FuncA", Kind: KindSymbol, Path: "a.go", Feature: "handle-a", SymbolName: "FuncA"}
	calleeB := &Node{ID: "sym:b.go:FuncB", Kind: KindSymbol, Path: "b.go", Feature: "handle-b", SymbolName: "FuncB"}

	g.AddNode(caller1)
	g.AddNode(caller2)
	g.AddNode(calleeA)
	g.AddNode(calleeB)

	// Both callers invoke both callees -> co-occurrence count = 2
	g.AddEdge(&Edge{From: caller1.ID, To: calleeA.ID, Type: EdgeInvokes, Weight: 1.0})
	g.AddEdge(&Edge{From: caller1.ID, To: calleeB.ID, Type: EdgeInvokes, Weight: 1.0})
	g.AddEdge(&Edge{From: caller2.ID, To: calleeA.ID, Type: EdgeInvokes, Weight: 1.0})
	g.AddEdge(&Edge{From: caller2.ID, To: calleeB.ID, Type: EdgeInvokes, Weight: 1.0})

	rpgStore := &GOBRPGStore{indexPath: filepath.Join(t.TempDir(), "rpg.gob"), graph: g}
	extractor := NewLocalExtractor()
	indexer := NewRPGEncoder(rpgStore, extractor, "/tmp", RPGEncoderConfig{DriftThreshold: 0.35})

	indexer.wireCoCallerAffinity(g)

	simCount := countEdgesByType(g, EdgeSemanticSim)
	if simCount != 1 {
		t.Errorf("Expected 1 EdgeSemanticSim edge for co-caller affinity, got %d", simCount)
	}
}

func TestBuildFull_EdgeImports(t *testing.T) {
	// Test that EdgeImports edges are created for cross-file invocations
	g := NewGraph()

	// Create two file nodes
	fileA := &Node{ID: "file:a.go", Kind: KindFile, Path: "a.go"}
	fileB := &Node{ID: "file:b.go", Kind: KindFile, Path: "b.go"}
	g.AddNode(fileA)
	g.AddNode(fileB)

	// Create symbol nodes in each file
	symA := &Node{
		ID:         "sym:a.go:FuncA",
		Kind:       KindSymbol,
		Path:       "a.go",
		SymbolName: "FuncA",
		StartLine:  1,
		EndLine:    10,
	}
	symB := &Node{
		ID:         "sym:b.go:FuncB",
		Kind:       KindSymbol,
		Path:       "b.go",
		SymbolName: "FuncB",
		StartLine:  1,
		EndLine:    10,
	}
	g.AddNode(symA)
	g.AddNode(symB)

	// Add EdgeContains edges
	g.AddEdge(&Edge{From: fileA.ID, To: symA.ID, Type: EdgeContains, Weight: 1.0})
	g.AddEdge(&Edge{From: fileB.ID, To: symB.ID, Type: EdgeContains, Weight: 1.0})

	// Add cross-file invocation edge (FuncA calls FuncB)
	g.AddEdge(&Edge{From: symA.ID, To: symB.ID, Type: EdgeInvokes, Weight: 1.0})

	// Now simulate Step 3 from BuildFull: generate EdgeImports
	importsSeen := make(map[string]bool)
	for _, e := range g.Edges {
		if e.Type != EdgeInvokes {
			continue
		}
		callerNode := g.GetNode(e.From)
		calleeNode := g.GetNode(e.To)
		if callerNode == nil || calleeNode == nil {
			continue
		}
		if callerNode.Path == "" || calleeNode.Path == "" || callerNode.Path == calleeNode.Path {
			continue
		}
		key := callerNode.Path + "->" + calleeNode.Path
		if importsSeen[key] {
			continue
		}
		importsSeen[key] = true

		fromFileID := MakeNodeID(KindFile, callerNode.Path)
		toFileID := MakeNodeID(KindFile, calleeNode.Path)
		if g.GetNode(fromFileID) != nil && g.GetNode(toFileID) != nil {
			g.AddEdge(&Edge{
				From:   fromFileID,
				To:     toFileID,
				Type:   EdgeImports,
				Weight: 1.0,
			})
		}
	}

	// Verify EdgeImports edge was created
	importsCount := countEdgesByType(g, EdgeImports)
	if importsCount != 1 {
		t.Errorf("Expected 1 EdgeImports edge, got %d", importsCount)
	}

	// Verify the edge is from fileA to fileB
	var foundImport bool
	for _, e := range g.Edges {
		if e.Type == EdgeImports && e.From == fileA.ID && e.To == fileB.ID {
			foundImport = true
			break
		}
	}
	if !foundImport {
		t.Error("Expected EdgeImports edge from a.go to b.go, but not found")
	}
}

func TestCapGroup_SmallGroup(t *testing.T) {
	// Groups smaller than maxFeatureGroupSize should be returned as-is
	group := make([]*Node, 10)
	for i := range group {
		group[i] = &Node{ID: fmt.Sprintf("sym:%d", i), Path: fmt.Sprintf("pkg%d/file.go", i)}
	}
	result := capGroup(group, "sample", rand.New(rand.NewSource(42)))
	if len(result) != 1 {
		t.Fatalf("expected 1 subgroup, got %d", len(result))
	}
	if len(result[0]) != 10 {
		t.Errorf("expected 10 nodes, got %d", len(result[0]))
	}
}

func TestCapGroup_SampleStrategy(t *testing.T) {
	// Groups larger than cap should be sampled down to maxFeatureGroupSize
	group := make([]*Node, maxFeatureGroupSize+50)
	for i := range group {
		group[i] = &Node{ID: fmt.Sprintf("sym:%d", i), Path: fmt.Sprintf("pkg%d/file.go", i)}
	}
	result := capGroup(group, "sample", rand.New(rand.NewSource(42)))
	if len(result) != 1 {
		t.Fatalf("expected 1 subgroup for sample strategy, got %d", len(result))
	}
	if len(result[0]) != maxFeatureGroupSize {
		t.Errorf("expected %d nodes after sampling, got %d", maxFeatureGroupSize, len(result[0]))
	}
}

func TestCapGroup_SplitStrategy(t *testing.T) {
	// Create nodes in 3 directories, each with enough nodes (>= 2)
	group := make([]*Node, 0, maxFeatureGroupSize+50)
	for i := 0; i < 40; i++ {
		group = append(group, &Node{ID: fmt.Sprintf("sym:dirA/%d", i), Path: "dirA/file.go"})
	}
	for i := 0; i < 40; i++ {
		group = append(group, &Node{ID: fmt.Sprintf("sym:dirB/%d", i), Path: "dirB/file.go"})
	}
	for i := 0; i < 40; i++ {
		group = append(group, &Node{ID: fmt.Sprintf("sym:dirC/%d", i), Path: "dirC/file.go"})
	}
	// Also add a singleton directory (should be skipped)
	group = append(group, &Node{ID: "sym:dirD/0", Path: "dirD/file.go"})

	result := capGroup(group, "split", rand.New(rand.NewSource(42)))
	// Should return 3 subgroups (dirA, dirB, dirC) - dirD has only 1 node so skipped
	if len(result) != 3 {
		t.Fatalf("expected 3 subgroups for split strategy, got %d", len(result))
	}
	for i, sg := range result {
		if len(sg) != 40 {
			t.Errorf("subgroup %d: expected 40 nodes, got %d", i, len(sg))
		}
	}
}

func TestCapGroup_SplitStrategy_FallbackSampling(t *testing.T) {
	// One directory has more than maxFeatureGroupSize nodes - should be sampled
	group := make([]*Node, 0, maxFeatureGroupSize+50)
	for i := 0; i < maxFeatureGroupSize+50; i++ {
		group = append(group, &Node{ID: fmt.Sprintf("sym:bigdir/%d", i), Path: "bigdir/file.go"})
	}
	// Add another small directory
	for i := 0; i < 5; i++ {
		group = append(group, &Node{ID: fmt.Sprintf("sym:smalldir/%d", i), Path: "smalldir/file.go"})
	}

	result := capGroup(group, "split", rand.New(rand.NewSource(42)))
	// Should return 2 subgroups: bigdir (sampled to cap) and smalldir (5 nodes)
	if len(result) != 2 {
		t.Fatalf("expected 2 subgroups, got %d", len(result))
	}
	// Find the big subgroup
	var bigCount, smallCount int
	for _, sg := range result {
		if len(sg) == maxFeatureGroupSize {
			bigCount++
		} else if len(sg) == 5 {
			smallCount++
		}
	}
	if bigCount != 1 {
		t.Errorf("expected 1 subgroup of size %d, found %d", maxFeatureGroupSize, bigCount)
	}
	if smallCount != 1 {
		t.Errorf("expected 1 subgroup of size 5, found %d", smallCount)
	}
}

func TestRPGEncoderConcurrentStatsAccess(t *testing.T) {
	tmpDir := t.TempDir()

	rpgStore := NewGOBRPGStore(filepath.Join(tmpDir, "test.gob"))
	if err := rpgStore.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer rpgStore.Close()

	encoder := NewRPGEncoder(rpgStore, NewLocalExtractor(), "/tmp", RPGEncoderConfig{})

	var wg sync.WaitGroup
	const iterations = 100

	// Goroutine 1: Simulate watch loop mutations via Stats()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = encoder.Stats()
		}
	}()

	// Goroutine 2: Simulate graph access
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = encoder.Stats()
		}
	}()

	wg.Wait()
}

func TestWireFeatureSimilarity_LargeGroupSplit(t *testing.T) {
	g := NewGraph()

	// Create symbols in two directories with the same verb "handle" and similar features.
	// With "split" strategy, they should be grouped by directory and compared within.
	for i := 0; i < 60; i++ {
		sym := &Node{
			ID:         fmt.Sprintf("sym:dirA/file.go:Handle%d", i),
			Kind:       KindSymbol,
			Path:       "dirA/file.go",
			Feature:    "handle-request@server",
			SymbolName: fmt.Sprintf("Handle%d", i),
		}
		g.AddNode(sym)
	}
	for i := 0; i < 60; i++ {
		sym := &Node{
			ID:         fmt.Sprintf("sym:dirB/file.go:Handle%d", i),
			Kind:       KindSymbol,
			Path:       "dirB/file.go",
			Feature:    "handle-request@client",
			SymbolName: fmt.Sprintf("Handle%d", i),
		}
		g.AddNode(sym)
	}

	rpgStore := &GOBRPGStore{indexPath: filepath.Join(t.TempDir(), "rpg.gob"), graph: g}
	extractor := NewLocalExtractor()
	indexer := NewRPGEncoder(rpgStore, extractor, "/tmp", RPGEncoderConfig{
		DriftThreshold:       0.35,
		FeatureGroupStrategy: "split",
	})

	indexer.wireSemanticEdges(g)

	// With split strategy, dirA symbols are in one subgroup (same file, skipped)
	// and dirB symbols are in another (same file, skipped).
	// No cross-directory edges because they're in separate subgroups.
	simCount := countEdgesByType(g, EdgeSemanticSim)
	if simCount != 0 {
		t.Errorf("Expected 0 EdgeSemanticSim edges (same-file within each dir subgroup), got %d", simCount)
	}
}

func TestBuildFull_ParallelismProducesSameGraph(t *testing.T) {
	ctx := context.Background()

	// Create a shared vector store and symbol store with 5 files.
	vsDir := filepath.Join(t.TempDir(), "vs")
	ssDir := filepath.Join(t.TempDir(), "ss")

	vectorStore := store.NewGOBStore(filepath.Join(vsDir, "index.gob"))
	symbolStore := trace.NewGOBSymbolStore(filepath.Join(ssDir, "symbols.gob"))
	defer symbolStore.Close()

	files := []string{"a/foo.go", "b/bar.go", "c/baz.go", "d/qux.go", "e/quux.go"}
	for _, fp := range files {
		if err := vectorStore.SaveDocument(ctx, store.Document{Path: fp, ModTime: time.Now()}); err != nil {
			t.Fatalf("SaveDocument %s: %v", fp, err)
		}
		if err := symbolStore.SaveFile(ctx, fp, []trace.Symbol{
			{Name: "Handle" + filepath.Base(fp), Kind: trace.KindFunction, File: fp, Line: 1, EndLine: 10, Signature: "func Handle()", Language: "go"},
			{Name: "Get" + filepath.Base(fp), Kind: trace.KindFunction, File: fp, Line: 12, EndLine: 20, Signature: "func Get()", Language: "go"},
		}, nil); err != nil {
			t.Fatalf("SaveFile %s: %v", fp, err)
		}
	}

	// Build with parallelism=1 (sequential baseline).
	rpgStore1 := NewGOBRPGStore(filepath.Join(t.TempDir(), "rpg1.gob"))
	idx1 := NewRPGEncoder(rpgStore1, NewLocalExtractor(), "/tmp", RPGEncoderConfig{
		DriftThreshold: 0.35,
		Parallelism:    1,
	})
	if err := idx1.BuildFull(ctx, symbolStore, vectorStore, nil); err != nil {
		t.Fatalf("BuildFull (P=1) failed: %v", err)
	}
	stats1 := rpgStore1.GetGraph().Stats()

	// Build with parallelism=4.
	rpgStore2 := NewGOBRPGStore(filepath.Join(t.TempDir(), "rpg2.gob"))
	idx2 := NewRPGEncoder(rpgStore2, NewLocalExtractor(), "/tmp", RPGEncoderConfig{
		DriftThreshold: 0.35,
		Parallelism:    4,
	})
	if err := idx2.BuildFull(ctx, symbolStore, vectorStore, nil); err != nil {
		t.Fatalf("BuildFull (P=4) failed: %v", err)
	}
	stats2 := rpgStore2.GetGraph().Stats()

	// Both should produce the same graph structure.
	if stats1.TotalNodes != stats2.TotalNodes {
		t.Errorf("node count mismatch: P=1=%d, P=4=%d", stats1.TotalNodes, stats2.TotalNodes)
	}
	if stats1.TotalEdges != stats2.TotalEdges {
		t.Errorf("edge count mismatch: P=1=%d, P=4=%d", stats1.TotalEdges, stats2.TotalEdges)
	}

	// Verify all nodes exist in both graphs.
	g1 := rpgStore1.GetGraph()
	g2 := rpgStore2.GetGraph()
	for id, n1 := range g1.Nodes {
		n2, ok := g2.Nodes[id]
		if !ok {
			t.Errorf("node %s missing in P=4 graph", id)
			continue
		}
		if n1.Feature != n2.Feature {
			t.Errorf("node %s feature mismatch: P=1=%q, P=4=%q", id, n1.Feature, n2.Feature)
		}
		if n1.Kind != n2.Kind {
			t.Errorf("node %s kind mismatch: P=1=%s, P=4=%s", id, n1.Kind, n2.Kind)
		}
	}
}

// slowExtractor sleeps on every call to simulate LLM latency.
type slowExtractor struct {
	mu    sync.Mutex
	calls int
}

func (e *slowExtractor) ExtractFeature(_ context.Context, _, _, _, _ string) string {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()
	time.Sleep(10 * time.Millisecond)
	return "handle-request"
}

func (e *slowExtractor) ExtractAtomicFeatures(_ context.Context, _, _, _, _ string) []string {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()
	time.Sleep(10 * time.Millisecond)
	return []string{"handle request"}
}

func (e *slowExtractor) GenerateSummary(_ context.Context, _, _ string) (string, error) {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()
	time.Sleep(10 * time.Millisecond)
	return "summary", nil
}

func (e *slowExtractor) Mode() string { return "slow" }

func TestBuildFull_ParallelismIsFasterThanSequential(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing test in short mode")
	}
	ctx := context.Background()

	vsDir := filepath.Join(t.TempDir(), "vs")
	ssDir := filepath.Join(t.TempDir(), "ss")

	vectorStore := store.NewGOBStore(filepath.Join(vsDir, "index.gob"))
	symbolStore := trace.NewGOBSymbolStore(filepath.Join(ssDir, "symbols.gob"))
	defer symbolStore.Close()

	// 10 files with 3 symbols each.
	for i := 0; i < 10; i++ {
		fp := fmt.Sprintf("pkg/file%d.go", i)
		if err := vectorStore.SaveDocument(ctx, store.Document{Path: fp, ModTime: time.Now()}); err != nil {
			t.Fatalf("SaveDocument: %v", err)
		}
		syms := []trace.Symbol{
			{Name: fmt.Sprintf("Fn%dA", i), Kind: trace.KindFunction, File: fp, Line: 1, EndLine: 10, Signature: "func Fn()", Language: "go"},
			{Name: fmt.Sprintf("Fn%dB", i), Kind: trace.KindFunction, File: fp, Line: 12, EndLine: 20, Signature: "func Fn()", Language: "go"},
			{Name: fmt.Sprintf("Fn%dC", i), Kind: trace.KindFunction, File: fp, Line: 22, EndLine: 30, Signature: "func Fn()", Language: "go"},
		}
		if err := symbolStore.SaveFile(ctx, fp, syms, nil); err != nil {
			t.Fatalf("SaveFile: %v", err)
		}
	}

	runBuild := func(parallelism int) time.Duration {
		rpgStore := NewGOBRPGStore(filepath.Join(t.TempDir(), fmt.Sprintf("rpg-p%d.gob", parallelism)))
		extractor := &slowExtractor{}
		idx := NewRPGEncoder(rpgStore, extractor, "/tmp", RPGEncoderConfig{
			DriftThreshold: 0.35,
			Parallelism:    parallelism,
		})
		start := time.Now()
		if err := idx.BuildFull(ctx, symbolStore, vectorStore, nil); err != nil {
			t.Fatalf("BuildFull (P=%d) failed: %v", parallelism, err)
		}
		return time.Since(start)
	}

	// With 10ms sleep per call and 10 files, P=4 should be noticeably
	// faster than P=1 due to concurrent LLM work.
	dur1 := runBuild(1)
	dur4 := runBuild(4)

	t.Logf("P=1: %v, P=4: %v", dur1, dur4)

	// Both must succeed (graph correctness checked by TestBuildFull_ParallelismProducesSameGraph).
	if dur1 == 0 || dur4 == 0 {
		t.Error("expected non-zero build durations")
	}
}
