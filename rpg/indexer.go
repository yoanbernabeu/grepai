package rpg

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
)

const (
	// Similarity thresholds for semantic edge creation.
	minFeatureSimilarity    = 0.5
	maxFeatureGroupSize     = 100 // cap verb groups to avoid O(n²) for common verbs
	minCoCallerCoOccurrence = 2
	coCallerWeightNorm      = 5.0 // normalization factor for co-caller edge weight
)

// RPGIndexer orchestrates building and maintaining the RPG graph.
// It connects the trace symbol store, vector store, and RPG graph.
type RPGIndexer struct {
	store       RPGStore
	extractor   FeatureExtractor
	hierarchy   *HierarchyBuilder
	evolver     *Evolver
	projectRoot string
	cfg         RPGIndexerConfig
	mu          sync.Mutex
}

// RPGIndexerConfig configures the RPG indexer behavior.
type RPGIndexerConfig struct {
	DriftThreshold       float64
	MaxTraversalDepth    int
	FeatureGroupStrategy string
}

// NewRPGIndexer creates a new RPG indexer instance.
func NewRPGIndexer(rpgStore RPGStore, extractor FeatureExtractor, projectRoot string, cfg RPGIndexerConfig) *RPGIndexer {
	graph := rpgStore.GetGraph()
	hierarchy := NewHierarchyBuilder(graph, extractor)
	evolver := NewEvolver(graph, extractor, hierarchy, cfg.DriftThreshold)

	return &RPGIndexer{
		store:       rpgStore,
		extractor:   extractor,
		hierarchy:   hierarchy,
		evolver:     evolver,
		projectRoot: projectRoot,
		cfg:         cfg,
	}
}

// BuildFull performs a complete rebuild of the RPG graph from scratch.
func (idx *RPGIndexer) BuildFull(ctx context.Context, symbolStore trace.SymbolStore, vectorStore store.VectorStore) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	graph := idx.store.GetGraph()

	// Clear existing graph data in-place (not reassigning the pointer)
	graph.Nodes = make(map[string]*Node)
	graph.Edges = make([]*Edge, 0)
	graph.RebuildIndexes()

	// Get all documents from vector store to know which files to process
	docs, err := vectorStore.ListDocuments(ctx)
	if err != nil {
		return fmt.Errorf("failed to list documents: %w", err)
	}

	// Track which files we've seen
	filesProcessed := make(map[string]bool)

	// Step 1: Create file and symbol nodes from trace store
	// First, we need to get all symbols - we'll iterate through all files
	for _, filePath := range docs {
		if filesProcessed[filePath] {
			continue
		}
		filesProcessed[filePath] = true

		// Create file node
		fileNodeID := MakeNodeID(KindFile, filePath)
		fileNode := &Node{
			ID:        fileNodeID,
			Kind:      KindFile,
			Path:      filePath,
			UpdatedAt: time.Now(),
		}
		graph.AddNode(fileNode)

		// Get symbols for this file from the symbol store
		symbols, symErr := symbolStore.GetSymbolsForFile(ctx, filePath)
		if symErr != nil {
			continue
		}

		for _, sym := range symbols {
			feature := idx.extractor.ExtractFeature(sym.Name, sym.Signature, sym.Receiver, "")
			nodeID := makeSymbolNodeID(filePath, sym)
			symNode := &Node{
				ID:         nodeID,
				Kind:       KindSymbol,
				Feature:    feature,
				Path:       filePath,
				SymbolName: sym.Name,
				Receiver:   sym.Receiver,
				Language:   sym.Language,
				StartLine:  sym.Line,
				EndLine:    normalizeEndLine(sym.Line, sym.EndLine),
				Signature:  sym.Signature,
				UpdatedAt:  time.Now(),
			}
			graph.AddNode(symNode)

			// Link file -> symbol via EdgeContains
			graph.AddEdge(&Edge{
				From:      fileNodeID,
				To:        nodeID,
				Type:      EdgeContains,
				Weight:    1.0,
				UpdatedAt: time.Now(),
			})
		}
	}

	// Step 2: Rebuild derived edges (invokes/imports/semantic).
	if err := idx.refreshDerivedEdgesFullLocked(ctx, symbolStore); err != nil {
		return fmt.Errorf("failed to refresh derived edges: %w", err)
	}

	// Step 4: Link vector chunks to symbols
	for _, filePath := range docs {
		chunks, err := vectorStore.GetChunksForFile(ctx, filePath)
		if err != nil {
			continue
		}

		if err := idx.linkChunksToSymbols(graph, filePath, chunks); err != nil {
			return fmt.Errorf("failed to link chunks for file %s: %w", filePath, err)
		}
	}

	// Step 5: Build hierarchy
	idx.hierarchy.BuildHierarchy()

	// Step 5b: Enrich hierarchy with semantic labels
	idx.hierarchy.EnrichLabels()

	// Step 6: Persist
	if err := idx.store.Persist(ctx); err != nil {
		return fmt.Errorf("failed to persist RPG store: %w", err)
	}

	return nil
}

// HandleFileEvent handles incremental updates for file events.
// The caller is responsible for persisting the store after updates.
func (idx *RPGIndexer) HandleFileEvent(ctx context.Context, eventType string, filePath string, symbols []trace.Symbol) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	switch strings.ToLower(eventType) {
	case "create":
		idx.evolver.HandleAdd(filePath, symbols)
	case "modify":
		idx.evolver.HandleModify(filePath, symbols)
	case "delete":
		idx.evolver.HandleDelete(filePath)
	default:
		return fmt.Errorf("unknown event type: %s", eventType)
	}

	return nil
}

// RefreshDerivedEdgesFull rebuilds all derived edges from current graph nodes.
func (idx *RPGIndexer) RefreshDerivedEdgesFull(ctx context.Context, symbolStore trace.SymbolStore) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	return idx.refreshDerivedEdgesFullLocked(ctx, symbolStore)
}

// RefreshDerivedEdgesIncremental updates derived edges for changed files.
func (idx *RPGIndexer) RefreshDerivedEdgesIncremental(ctx context.Context, symbolStore trace.SymbolStore, changedFiles []string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	return idx.refreshDerivedEdgesIncrementalLocked(ctx, symbolStore, changedFiles)
}

func (idx *RPGIndexer) refreshDerivedEdgesFullLocked(ctx context.Context, symbolStore trace.SymbolStore) error {
	graph := idx.store.GetGraph()
	graph.RemoveEdgesIf(func(e *Edge) bool {
		return isDerivedEdgeType(e.Type)
	})

	callEdges, err := symbolStore.GetCallEdges(ctx)
	if err != nil {
		return fmt.Errorf("failed to load call edges: %w", err)
	}

	idx.wireInvocationEdges(graph, callEdges, nil)
	idx.wireImportEdges(graph, nil)
	idx.wireFeatureSimilarity(graph)
	idx.wireCoCallerAffinity(graph)
	return nil
}

func (idx *RPGIndexer) refreshDerivedEdgesIncrementalLocked(ctx context.Context, symbolStore trace.SymbolStore, changedFiles []string) error {
	if len(changedFiles) == 0 {
		return nil
	}

	graph := idx.store.GetGraph()
	changedSet := make(map[string]struct{}, len(changedFiles))
	for _, file := range changedFiles {
		if file == "" {
			continue
		}
		changedSet[file] = struct{}{}
	}
	if len(changedSet) == 0 {
		return nil
	}

	graph.RemoveEdgesIf(func(e *Edge) bool {
		if !isDerivedEdgeType(e.Type) {
			return false
		}
		fromPath, okFrom := graph.NodePath(e.From)
		toPath, okTo := graph.NodePath(e.To)
		if okFrom {
			if _, ok := changedSet[fromPath]; ok {
				return true
			}
		}
		if okTo {
			if _, ok := changedSet[toPath]; ok {
				return true
			}
		}
		return false
	})

	callEdges, err := symbolStore.GetCallEdges(ctx)
	if err != nil {
		return fmt.Errorf("failed to load call edges: %w", err)
	}

	idx.wireInvocationEdges(graph, callEdges, changedSet)
	idx.wireImportEdges(graph, changedSet)
	idx.wireFeatureSimilarityIncremental(graph, changedSet)
	idx.wireCoCallerAffinityIncremental(graph, changedSet)
	return nil
}

// LinkChunksForFile links vector chunks to overlapping symbols in the graph.
// The caller is responsible for persisting the store after updates.
func (idx *RPGIndexer) LinkChunksForFile(ctx context.Context, filePath string, chunks []store.Chunk) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	graph := idx.store.GetGraph()

	if err := idx.linkChunksToSymbols(graph, filePath, chunks); err != nil {
		return fmt.Errorf("failed to link chunks: %w", err)
	}

	return nil
}

// GetGraph returns the underlying graph.
func (idx *RPGIndexer) GetGraph() *Graph {
	return idx.store.GetGraph()
}

// GetEvolver returns the evolver for direct use.
func (idx *RPGIndexer) GetEvolver() *Evolver {
	return idx.evolver
}

// linkChunksToSymbols creates EdgeMapsToChunk edges for overlapping chunks and symbols.
// It first removes existing chunk nodes for the file to prevent edge accumulation.
func (idx *RPGIndexer) linkChunksToSymbols(graph *Graph, filePath string, chunks []store.Chunk) error {
	// Clean up existing chunk nodes for this file to prevent edge accumulation
	// RemoveNode handles all edge and index cleanup
	fileNodes := graph.GetNodesByFile(filePath)
	for _, node := range fileNodes {
		if node.Kind == KindChunk {
			graph.RemoveNode(node.ID)
		}
	}

	// Re-fetch file nodes after cleanup (byFile index was modified)
	fileNodes = graph.GetNodesByFile(filePath)
	var symbolNodes []*Node
	for _, node := range fileNodes {
		if node.Kind == KindSymbol {
			symbolNodes = append(symbolNodes, node)
		}
	}

	// For each chunk, find overlapping symbols
	for _, chunk := range chunks {
		// Create chunk node if it doesn't exist
		chunkNodeID := MakeNodeID(KindChunk, chunk.ID)
		chunkNode := graph.GetNode(chunkNodeID)
		if chunkNode == nil {
			chunkNode = &Node{
				ID:        chunkNodeID,
				Kind:      KindChunk,
				Path:      chunk.FilePath,
				StartLine: chunk.StartLine,
				EndLine:   chunk.EndLine,
				ChunkID:   chunk.ID,
				UpdatedAt: time.Now(),
			}
			graph.AddNode(chunkNode)
		}

		// Find symbols that overlap with this chunk
		for _, symbolNode := range symbolNodes {
			// Check if chunk and symbol line ranges overlap
			if overlaps(chunk.StartLine, chunk.EndLine, symbolNode.StartLine, symbolNode.EndLine) {
				// Create EdgeMapsToChunk from symbol to chunk
				edge := &Edge{
					From:      symbolNode.ID,
					To:        chunkNodeID,
					Type:      EdgeMapsToChunk,
					Weight:    1.0,
					UpdatedAt: time.Now(),
				}
				graph.AddEdge(edge)
			}
		}
	}

	return nil
}

// findSymbolNodeID finds the node ID for a symbol by name, file, and line.
func findSymbolNodeID(graph *Graph, symbolName, filePath string, line int) string {
	// Get all nodes for this file
	nodes := graph.GetNodesByFile(filePath)

	// Find symbol node with matching name and line
	for _, node := range nodes {
		if node.Kind == KindSymbol && node.SymbolName == symbolName {
			// Check if line is within symbol's range
			if line >= node.StartLine && line <= node.EndLine {
				return node.ID
			}
		}
	}

	return ""
}

// normalizeEndLine returns endLine if it's valid, otherwise falls back to
// startLine. The regex-based trace extractor does not populate EndLine, so
// this prevents overlap checks from always failing when EndLine is 0.
func normalizeEndLine(startLine, endLine int) int {
	if endLine <= 0 || endLine < startLine {
		return startLine
	}
	return endLine
}

// overlaps checks if two line ranges overlap.
func overlaps(start1, end1, start2, end2 int) bool {
	return start1 <= end2 && start2 <= end1
}

func isDerivedEdgeType(edgeType EdgeType) bool {
	return edgeType == EdgeInvokes || edgeType == EdgeImports || edgeType == EdgeSemanticSim
}

func edgeExists(graph *Graph, fromID, toID string, edgeType EdgeType) bool {
	for _, e := range graph.GetOutgoing(fromID) {
		if e.To == toID && e.Type == edgeType {
			return true
		}
	}
	return false
}

func semanticEdgeExistsBetween(graph *Graph, aID, bID string) bool {
	return edgeExists(graph, aID, bID, EdgeSemanticSim) || edgeExists(graph, bID, aID, EdgeSemanticSim)
}

func canonicalPair(a, b string) (string, string) {
	if a <= b {
		return a, b
	}
	return b, a
}

func (idx *RPGIndexer) wireInvocationEdges(graph *Graph, callEdges []trace.CallEdge, changedFiles map[string]struct{}) {
	seen := make(map[string]struct{})

	for _, ce := range callEdges {
		callerID := findSymbolNodeID(graph, ce.Caller, ce.File, ce.Line)
		if callerID == "" {
			continue
		}

		bestMatch := findBestCalleeNode(graph, ce.Callee, ce.File)
		if bestMatch == nil {
			continue
		}

		if changedFiles != nil {
			_, callerChanged := changedFiles[ce.File]
			_, calleeChanged := changedFiles[bestMatch.Path]
			if !callerChanged && !calleeChanged {
				continue
			}
		}

		key := callerID + "|" + bestMatch.ID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		if edgeExists(graph, callerID, bestMatch.ID, EdgeInvokes) {
			continue
		}
		graph.AddEdge(&Edge{
			From:      callerID,
			To:        bestMatch.ID,
			Type:      EdgeInvokes,
			Weight:    1.0,
			UpdatedAt: time.Now(),
		})
	}
}

func findBestCalleeNode(graph *Graph, calleeName, callerFile string) *Node {
	calleeNodes := graph.GetNodesByKind(KindSymbol)
	var bestMatch *Node
	var samePackageMatch *Node

	for _, cn := range calleeNodes {
		if cn.SymbolName != calleeName {
			continue
		}
		if cn.Path == callerFile {
			return cn
		}
		if samePackageMatch == nil && filepath.Dir(cn.Path) == filepath.Dir(callerFile) {
			samePackageMatch = cn
		}
		if bestMatch == nil {
			bestMatch = cn
		}
	}

	if samePackageMatch != nil {
		return samePackageMatch
	}
	return bestMatch
}

func (idx *RPGIndexer) wireImportEdges(graph *Graph, changedFiles map[string]struct{}) {
	importsSeen := make(map[string]bool)
	for _, e := range graph.Edges {
		if e.Type != EdgeInvokes {
			continue
		}
		callerNode := graph.GetNode(e.From)
		calleeNode := graph.GetNode(e.To)
		if callerNode == nil || calleeNode == nil {
			continue
		}
		if callerNode.Path == "" || calleeNode.Path == "" || callerNode.Path == calleeNode.Path {
			continue
		}
		if changedFiles != nil {
			_, callerChanged := changedFiles[callerNode.Path]
			_, calleeChanged := changedFiles[calleeNode.Path]
			if !callerChanged && !calleeChanged {
				continue
			}
		}

		key := callerNode.Path + "->" + calleeNode.Path
		if importsSeen[key] {
			continue
		}
		importsSeen[key] = true

		fromFileID := MakeNodeID(KindFile, callerNode.Path)
		toFileID := MakeNodeID(KindFile, calleeNode.Path)
		if graph.GetNode(fromFileID) == nil || graph.GetNode(toFileID) == nil {
			continue
		}
		if edgeExists(graph, fromFileID, toFileID, EdgeImports) {
			continue
		}
		graph.AddEdge(&Edge{
			From:      fromFileID,
			To:        toFileID,
			Type:      EdgeImports,
			Weight:    1.0,
			UpdatedAt: time.Now(),
		})
	}
}

func collectChangedSymbolIDs(graph *Graph, changedFiles map[string]struct{}) map[string]struct{} {
	changedSymbols := make(map[string]struct{})
	for filePath := range changedFiles {
		for _, node := range graph.GetNodesByFile(filePath) {
			if node.Kind == KindSymbol {
				changedSymbols[node.ID] = struct{}{}
			}
		}
	}
	return changedSymbols
}

// capGroup splits or samples a verb group that exceeds maxFeatureGroupSize.
// Strategy "split" partitions by directory, preserving locality; "sample" (default)
// randomly samples down to cap size.
func capGroup(group []*Node, strategy string) [][]*Node {
	if len(group) <= maxFeatureGroupSize {
		return [][]*Node{group}
	}
	switch strategy {
	case "split":
		byDir := map[string][]*Node{}
		for _, n := range group {
			dir := filepath.Dir(n.Path)
			byDir[dir] = append(byDir[dir], n)
		}
		var result [][]*Node
		for _, sub := range byDir {
			if len(sub) < 2 {
				continue
			}
			if len(sub) > maxFeatureGroupSize {
				rand.Shuffle(len(sub), func(i, j int) { sub[i], sub[j] = sub[j], sub[i] })
				sub = sub[:maxFeatureGroupSize]
			}
			result = append(result, sub)
		}
		return result
	default: // "sample"
		rand.Shuffle(len(group), func(i, j int) { group[i], group[j] = group[j], group[i] })
		return [][]*Node{group[:maxFeatureGroupSize]}
	}
}

// wireFeatureSimilarity creates EdgeSemanticSim edges between symbols in different
// files whose feature labels have high Jaccard similarity. To avoid O(n^2),
// symbols are grouped by their feature verb (first word) and only compared within groups.
func (idx *RPGIndexer) wireFeatureSimilarity(graph *Graph) {
	symbolNodes := graph.GetNodesByKind(KindSymbol)

	// Group symbols by feature verb for efficiency
	byVerb := make(map[string][]*Node)
	for _, n := range symbolNodes {
		if n.Feature == "" {
			continue
		}
		verb := firstWord(n.Feature)
		if verb != "" {
			byVerb[verb] = append(byVerb[verb], n)
		}
	}

	seen := make(map[string]bool) // dedup "idA|idB"

	for _, group := range byVerb {
		if len(group) < 2 {
			continue
		}
		subgroups := capGroup(group, idx.cfg.FeatureGroupStrategy)
		for _, sg := range subgroups {
			for i := range len(sg) {
				for j := i + 1; j < len(sg); j++ {
					a, b := sg[i], sg[j]
					if a.Path == b.Path {
						continue // skip same-file pairs
					}

					// Canonical key for dedup
					key := a.ID + "|" + b.ID
					if a.ID > b.ID {
						key = b.ID + "|" + a.ID
					}
					if seen[key] {
						continue
					}

					sim := featureSimilarity(a.Feature, b.Feature)
					if sim >= minFeatureSimilarity {
						seen[key] = true
						if semanticEdgeExistsBetween(graph, a.ID, b.ID) {
							continue
						}
						graph.AddEdge(&Edge{
							From:      a.ID,
							To:        b.ID,
							Type:      EdgeSemanticSim,
							Weight:    sim,
							UpdatedAt: time.Now(),
						})
					}
				}
			}
		}
	}
}

func (idx *RPGIndexer) wireFeatureSimilarityIncremental(graph *Graph, changedFiles map[string]struct{}) {
	changedSymbols := collectChangedSymbolIDs(graph, changedFiles)
	if len(changedSymbols) == 0 {
		return
	}

	symbolNodes := graph.GetNodesByKind(KindSymbol)
	byVerb := make(map[string][]*Node)
	for _, n := range symbolNodes {
		if n.Feature == "" {
			continue
		}
		verb := firstWord(n.Feature)
		if verb != "" {
			byVerb[verb] = append(byVerb[verb], n)
		}
	}

	seen := make(map[string]struct{})
	for changedID := range changedSymbols {
		anchor := graph.GetNode(changedID)
		if anchor == nil || anchor.Feature == "" {
			continue
		}
		group := byVerb[firstWord(anchor.Feature)]
		for _, candidate := range group {
			if candidate.ID == anchor.ID {
				continue
			}
			if anchor.Path == candidate.Path {
				continue
			}

			sim := featureSimilarity(anchor.Feature, candidate.Feature)
			if sim < minFeatureSimilarity {
				continue
			}

			fromID, toID := canonicalPair(anchor.ID, candidate.ID)
			key := fromID + "|" + toID
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}

			if semanticEdgeExistsBetween(graph, fromID, toID) {
				continue
			}
			graph.AddEdge(&Edge{
				From:      fromID,
				To:        toID,
				Type:      EdgeSemanticSim,
				Weight:    sim,
				UpdatedAt: time.Now(),
			})
		}
	}
}

// wireCoCallerAffinity creates EdgeSemanticSim edges between symbols that are
// frequently called by the same callers (co-callees pattern).
func (idx *RPGIndexer) wireCoCallerAffinity(graph *Graph) {
	// Build caller -> callees map from EdgeInvokes
	callerToCallees := make(map[string][]string)
	for _, e := range graph.Edges {
		if e.Type == EdgeInvokes {
			callerToCallees[e.From] = append(callerToCallees[e.From], e.To)
		}
	}

	// Count co-occurrences
	cooccurrence := make(map[string]int)
	for _, callees := range callerToCallees {
		if len(callees) < 2 {
			continue
		}
		for i := 0; i < len(callees); i++ {
			for j := i + 1; j < len(callees); j++ {
				a, b := callees[i], callees[j]
				if a > b {
					a, b = b, a
				}
				cooccurrence[a+"|"+b]++
			}
		}
	}

	// Create edges for pairs with enough co-occurrences
	for key, count := range cooccurrence {
		if count < minCoCallerCoOccurrence {
			continue
		}
		parts := strings.SplitN(key, "|", 2)
		if len(parts) != 2 {
			continue
		}
		nodeA := graph.GetNode(parts[0])
		nodeB := graph.GetNode(parts[1])
		if nodeA == nil || nodeB == nil {
			continue
		}
		// Skip same-file (feature overlap already handles those)
		if nodeA.Path == nodeB.Path {
			continue
		}
		if semanticEdgeExistsBetween(graph, parts[0], parts[1]) {
			continue
		}

		// Weight: normalize count (cap at 1.0)
		weight := float64(count) / coCallerWeightNorm
		if weight > 1.0 {
			weight = 1.0
		}

		graph.AddEdge(&Edge{
			From:      parts[0],
			To:        parts[1],
			Type:      EdgeSemanticSim,
			Weight:    weight,
			UpdatedAt: time.Now(),
		})
	}
}

func (idx *RPGIndexer) wireCoCallerAffinityIncremental(graph *Graph, changedFiles map[string]struct{}) {
	changedSymbols := collectChangedSymbolIDs(graph, changedFiles)

	callerToCallees := make(map[string][]string)
	for _, e := range graph.Edges {
		if e.Type == EdgeInvokes {
			callerToCallees[e.From] = append(callerToCallees[e.From], e.To)
		}
	}

	impactedCallers := make(map[string]struct{})
	for callerID, callees := range callerToCallees {
		callerPath, _ := graph.NodePath(callerID)
		if _, ok := changedFiles[callerPath]; ok {
			impactedCallers[callerID] = struct{}{}
			continue
		}
		for _, calleeID := range callees {
			if _, ok := changedSymbols[calleeID]; ok {
				impactedCallers[callerID] = struct{}{}
				break
			}
		}
	}
	if len(impactedCallers) == 0 {
		return
	}

	impactedPairs := make(map[string]struct{})
	for callerID := range impactedCallers {
		callees := callerToCallees[callerID]
		if len(callees) < 2 {
			continue
		}
		for i := 0; i < len(callees); i++ {
			for j := i + 1; j < len(callees); j++ {
				a, b := canonicalPair(callees[i], callees[j])
				impactedPairs[a+"|"+b] = struct{}{}
			}
		}
	}
	if len(impactedPairs) == 0 {
		return
	}

	cooccurrence := make(map[string]int)
	for _, callees := range callerToCallees {
		if len(callees) < 2 {
			continue
		}
		for i := 0; i < len(callees); i++ {
			for j := i + 1; j < len(callees); j++ {
				a, b := canonicalPair(callees[i], callees[j])
				key := a + "|" + b
				if _, ok := impactedPairs[key]; ok {
					cooccurrence[key]++
				}
			}
		}
	}

	for key, count := range cooccurrence {
		if count < minCoCallerCoOccurrence {
			continue
		}

		parts := strings.SplitN(key, "|", 2)
		if len(parts) != 2 {
			continue
		}
		nodeA := graph.GetNode(parts[0])
		nodeB := graph.GetNode(parts[1])
		if nodeA == nil || nodeB == nil {
			continue
		}
		if nodeA.Path == nodeB.Path {
			continue
		}
		if semanticEdgeExistsBetween(graph, parts[0], parts[1]) {
			continue
		}

		weight := float64(count) / coCallerWeightNorm
		if weight > 1.0 {
			weight = 1.0
		}
		graph.AddEdge(&Edge{
			From:      parts[0],
			To:        parts[1],
			Type:      EdgeSemanticSim,
			Weight:    weight,
			UpdatedAt: time.Now(),
		})
	}
}

// featureSimilarity computes Jaccard similarity between two feature label word sets.
func featureSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	wordsA := splitFeatureWords(a)
	wordsB := splitFeatureWords(b)
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0
	}

	union := make(map[string]bool)
	for w := range wordsA {
		union[w] = true
	}
	for w := range wordsB {
		union[w] = true
	}

	intersection := 0
	for w := range wordsA {
		if wordsB[w] {
			intersection++
		}
	}

	if len(union) == 0 {
		return 0
	}
	return float64(intersection) / float64(len(union))
}

// firstWord returns the first word of a feature label (the verb).
func firstWord(feature string) string {
	if idx := strings.IndexAny(feature, "-_/ @"); idx >= 0 {
		return feature[:idx]
	}
	return feature
}
