package rpg

import (
	"context"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoanbernabeu/grepai/trace"
)

// Evolver handles incremental updates to the RPG graph.
// Implements "RPG Evolution: Incremental Maintenance" from arXiv:2602.02084.
type Evolver struct {
	graph          *Graph
	extractor      FeatureExtractor
	hierarchy      *HierarchyBuilder
	driftThreshold float64
}

// NewEvolver creates an Evolver with the given drift threshold.
// driftThreshold controls when a file is considered to have changed
// semantically (0.0 = never, 1.0 = always). A typical value is 0.3.
func NewEvolver(graph *Graph, extractor FeatureExtractor, hierarchy *HierarchyBuilder, driftThreshold float64) *Evolver {
	return &Evolver{
		graph:          graph,
		extractor:      extractor,
		hierarchy:      hierarchy,
		driftThreshold: driftThreshold,
	}
}

// HandleDelete removes all nodes associated with a file and prunes orphaned
// hierarchy nodes.
func (ev *Evolver) HandleDelete(ctx context.Context, filePath string) {
	nodes := ev.graph.GetNodesByFile(filePath)
	if len(nodes) == 0 {
		return
	}

	parentIDs := make(map[string]bool)
	for _, node := range nodes {
		for _, edge := range ev.graph.GetIncoming(node.ID) {
			if edge.Type == EdgeFeatureParent {
				parentIDs[edge.From] = true
			}
		}
	}

	toRemove := make([]string, len(nodes))
	for i, node := range nodes {
		toRemove[i] = node.ID
	}
	for _, nodeID := range toRemove {
		ev.graph.RemoveNode(nodeID)
	}

	for parentID := range parentIDs {
		ev.pruneOrphans(parentID)
	}
}

// HandleModify re-extracts semantic features for all symbols in the changed file,
// updates file-level semantics, and reroutes file hierarchy placement when drift
// crosses the configured threshold.
func (ev *Evolver) HandleModify(ctx context.Context, filePath string, symbols []trace.Symbol) {
	now := time.Now()
	fileNode := ev.ensureFileNode(ctx, filePath, now)
	oldFileFeatures := append([]string(nil), getNodeAtomicFeatures(fileNode)...)

	existingSymbols := make(map[string]*Node)
	for _, node := range ev.graph.GetNodesByFile(filePath) {
		if node.Kind == KindSymbol {
			existingSymbols[node.ID] = node
		}
	}

	newSymbolIDs := make(map[string]bool, len(symbols))
	for _, sym := range symbols {
		nodeID := makeSymbolNodeID(filePath, sym)
		newSymbolIDs[nodeID] = true

		atomicFeatures := ev.extractor.ExtractAtomicFeatures(ctx, sym.Name, sym.Signature, sym.Receiver, sym.Docstring)
		primaryFeature := ev.extractor.ExtractFeature(ctx, sym.Name, sym.Signature, sym.Receiver, sym.Docstring)

		if existing, ok := existingSymbols[nodeID]; ok {
			setNodeFeatures(existing, atomicFeatures, primaryFeature)
			existing.SymbolName = sym.Name
			existing.Receiver = sym.Receiver
			existing.Language = sym.Language
			existing.Signature = sym.Signature
			existing.StartLine = sym.Line
			existing.EndLine = normalizeEndLine(sym.Line, sym.EndLine)
			existing.UpdatedAt = now
			continue
		}

		ev.addSymbolNode(filePath, sym, atomicFeatures, primaryFeature, now)
	}

	for nodeID := range existingSymbols {
		if !newSymbolIDs[nodeID] {
			ev.graph.RemoveNode(nodeID)
		}
	}

	ev.refreshFileSemantics(ctx, fileNode, filePath, now)
	ev.ensureFileHierarchyPlacement(filePath, oldFileFeatures, getNodeAtomicFeatures(fileNode), now)
}

// HandleAdd adds nodes for a newly created file.
func (ev *Evolver) HandleAdd(ctx context.Context, filePath string, symbols []trace.Symbol) {
	now := time.Now()
	fileNode := ev.ensureFileNode(ctx, filePath, now)

	for _, sym := range symbols {
		atomicFeatures := ev.extractor.ExtractAtomicFeatures(ctx, sym.Name, sym.Signature, sym.Receiver, sym.Docstring)
		primaryFeature := ev.extractor.ExtractFeature(ctx, sym.Name, sym.Signature, sym.Receiver, sym.Docstring)
		ev.addSymbolNode(filePath, sym, atomicFeatures, primaryFeature, now)
	}

	ev.refreshFileSemantics(ctx, fileNode, filePath, now)
	ev.ensureFileHierarchyPlacement(filePath, nil, getNodeAtomicFeatures(fileNode), now)
}

func (ev *Evolver) ensureFileNode(ctx context.Context, filePath string, now time.Time) *Node {
	fileID := MakeNodeID(KindFile, filePath)
	existing := ev.graph.GetNode(fileID)
	if existing != nil {
		return existing
	}

	stem := fileNameStem(filepath.Base(filePath))
	fallbackPrimary := ev.extractor.ExtractFeature(ctx, stem, "", "", "")
	fallbackAtomic := ev.extractor.ExtractAtomicFeatures(ctx, stem, "", "", "")

	fileNode := &Node{
		ID:        fileID,
		Kind:      KindFile,
		Path:      filePath,
		UpdatedAt: now,
	}
	setNodeFeatures(fileNode, fallbackAtomic, fallbackPrimary)
	ev.graph.AddNode(fileNode)
	return fileNode
}

func (ev *Evolver) refreshFileSemantics(ctx context.Context, fileNode *Node, filePath string, now time.Time) {
	if fileNode == nil {
		return
	}

	symbolFeatures := ev.collectFileSymbolFeatures(filePath)
	fallbackPrimary := ev.extractor.ExtractFeature(ctx, fileNameStem(filepath.Base(filePath)), "", "", "")
	if len(symbolFeatures) == 0 {
		symbolFeatures = ev.extractor.ExtractAtomicFeatures(ctx, fileNameStem(filepath.Base(filePath)), "", "", "")
	}
	setNodeFeatures(fileNode, symbolFeatures, fallbackPrimary)

	summary, err := ev.extractor.GenerateSummary(ctx, filePath, buildSummaryContext(filePath, getNodeAtomicFeatures(fileNode)))
	if err == nil {
		fileNode.Summary = strings.TrimSpace(summary)
	}
	fileNode.UpdatedAt = now
}

func (ev *Evolver) collectFileSymbolFeatures(filePath string) []string {
	features := make([]string, 0)
	for _, node := range ev.graph.GetNodesByFile(filePath) {
		if node.Kind != KindSymbol {
			continue
		}
		features = append(features, getNodeAtomicFeatures(node)...)
	}
	return aggregateAtomicFeatures(features, 5)
}

func (ev *Evolver) ensureFileHierarchyPlacement(filePath string, oldFeatures, newFeatures []string, now time.Time) {
	fileID := MakeNodeID(KindFile, filePath)
	if ev.graph.GetNode(fileID) == nil {
		return
	}

	areaName, catName := ev.hierarchy.ClassifyFile(filePath)
	areaID := ev.hierarchy.EnsureArea(areaName)
	catID := ev.hierarchy.EnsureCategory(areaID, catName)

	subcatName := ev.selectSubcategoryForFile(filePath)
	if subcatName == "" {
		subcatName = "general"
	}

	currentParents := ev.currentFileFeatureParents(fileID)
	if len(currentParents) == 0 {
		subcatID := ev.hierarchy.EnsureSubcategory(catID, subcatName)
		ev.graph.AddEdge(&Edge{
			From:      subcatID,
			To:        fileID,
			Type:      EdgeFeatureParent,
			Weight:    1.0,
			UpdatedAt: now,
		})
		return
	}

	drift := calculateDrift(oldFeatures, newFeatures)
	if drift < ev.driftThreshold {
		return
	}

	subcatID := ev.hierarchy.EnsureSubcategory(catID, subcatName)

	for _, parentID := range currentParents {
		ev.graph.RemoveEdgesBetweenOfType(parentID, fileID, EdgeFeatureParent)
	}
	if !edgeExists(ev.graph, subcatID, fileID, EdgeFeatureParent) {
		ev.graph.AddEdge(&Edge{
			From:      subcatID,
			To:        fileID,
			Type:      EdgeFeatureParent,
			Weight:    1.0,
			UpdatedAt: now,
		})
	}

	// Remove stale hierarchy branches that lost their last file child after reroute.
	for _, parentID := range currentParents {
		ev.pruneOrphans(parentID)
	}
}

func (ev *Evolver) currentFileFeatureParents(fileID string) []string {
	parents := make([]string, 0)
	seen := make(map[string]struct{})
	for _, edge := range ev.graph.GetIncoming(fileID) {
		if edge.Type != EdgeFeatureParent {
			continue
		}
		if _, ok := seen[edge.From]; ok {
			continue
		}
		seen[edge.From] = struct{}{}
		parents = append(parents, edge.From)
	}
	return parents
}

func (ev *Evolver) selectSubcategoryForFile(filePath string) string {
	return selectSubcategoryByFrequency(ev.graph.GetNodesByFile(filePath), "", ev.hierarchy)
}

// addSymbolNode creates or updates a symbol node and links it to its file.
func (ev *Evolver) addSymbolNode(filePath string, sym trace.Symbol, atomicFeatures []string, primaryFeature string, now time.Time) {
	nodeID := makeSymbolNodeID(filePath, sym)
	symNode := &Node{
		ID:         nodeID,
		Kind:       KindSymbol,
		Path:       filePath,
		SymbolName: sym.Name,
		Receiver:   sym.Receiver,
		Language:   sym.Language,
		StartLine:  sym.Line,
		EndLine:    normalizeEndLine(sym.Line, sym.EndLine),
		Signature:  sym.Signature,
		UpdatedAt:  now,
	}
	setNodeFeatures(symNode, atomicFeatures, primaryFeature)
	ev.graph.AddNode(symNode)

	fileID := MakeNodeID(KindFile, filePath)
	if !edgeExists(ev.graph, fileID, nodeID, EdgeContains) {
		ev.graph.AddEdge(&Edge{
			From:      fileID,
			To:        nodeID,
			Type:      EdgeContains,
			Weight:    1.0,
			UpdatedAt: now,
		})
	}
}

// pruneOrphans removes hierarchy nodes that have no feature-hierarchy children.
func (ev *Evolver) pruneOrphans(nodeID string) {
	node := ev.graph.GetNode(nodeID)
	if node == nil {
		return
	}
	if node.Kind != KindArea && node.Kind != KindCategory && node.Kind != KindSubcategory {
		return
	}

	hasChildren := false
	for _, edge := range ev.graph.GetOutgoing(nodeID) {
		if edge.Type != EdgeFeatureParent {
			continue
		}
		child := ev.graph.GetNode(edge.To)
		if child == nil {
			continue
		}
		if child.Kind == KindCategory || child.Kind == KindSubcategory || child.Kind == KindFile {
			hasChildren = true
			break
		}
	}
	if hasChildren {
		return
	}

	parentID := ""
	for _, edge := range ev.graph.GetIncoming(nodeID) {
		if edge.Type == EdgeFeatureParent {
			parentID = edge.From
			break
		}
	}

	ev.graph.RemoveNode(nodeID)
	if parentID != "" {
		ev.pruneOrphans(parentID)
	}
}

// calculateDrift computes a drift score between two feature sets.
// Returns 0.0 (identical) to 1.0 (completely different).
func calculateDrift(oldFeatures, newFeatures []string) float64 {
	if len(oldFeatures) == 0 && len(newFeatures) == 0 {
		return 0.0
	}

	oldWords := atomicWordSet(oldFeatures)
	newWords := atomicWordSet(newFeatures)

	if len(oldWords) == 0 && len(newWords) == 0 {
		return 0.0
	}
	if len(oldWords) == 0 || len(newWords) == 0 {
		return 1.0
	}

	union := make(map[string]bool)
	for word := range oldWords {
		union[word] = true
	}
	for word := range newWords {
		union[word] = true
	}

	intersectionCount := 0
	for word := range oldWords {
		if newWords[word] {
			intersectionCount++
		}
	}

	if len(union) == 0 {
		return 0.0
	}

	jaccard := float64(intersectionCount) / float64(len(union))
	return math.Round((1.0-jaccard)*1000) / 1000
}
