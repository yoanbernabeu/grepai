package rpg

import (
	"math"
	"strings"
	"time"

	"github.com/yoanbernabeu/grepai/trace"
)

// Evolver handles incremental updates to the RPG graph.
type Evolver struct {
	graph          *Graph
	extractor      FeatureExtractor
	hierarchy      *HierarchyBuilder
	driftThreshold float64
}

// NewEvolver creates an Evolver with the given drift threshold.
// driftThreshold controls when a symbol is considered to have changed
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
func (ev *Evolver) HandleDelete(filePath string) {
	nodes := ev.graph.GetNodesByFile(filePath)
	if len(nodes) == 0 {
		return
	}

	// Collect hierarchy parents before removing nodes so we can prune them.
	parentIDs := make(map[string]bool)
	for _, n := range nodes {
		for _, e := range ev.graph.GetOutgoing(n.ID) {
			if e.Type == EdgeFeatureParent {
				parentIDs[e.To] = true
			}
		}
	}

	// Remove every node belonging to this file (file, symbol, chunk nodes).
	// Graph.RemoveNode handles edge cleanup.
	// Copy the slice because RemoveNode mutates the byFile index.
	toRemove := make([]string, len(nodes))
	for i, n := range nodes {
		toRemove[i] = n.ID
	}
	for _, id := range toRemove {
		ev.graph.RemoveNode(id)
	}

	// Prune orphaned hierarchy nodes upward.
	for pid := range parentIDs {
		ev.pruneOrphans(pid)
	}
}

// HandleModify re-extracts features for a modified file and applies drift
// detection. Symbols that drifted beyond the threshold are removed and
// re-created with fresh hierarchy placement; others are updated in-place.
func (ev *Evolver) HandleModify(filePath string, symbols []trace.Symbol) {
	existingNodes := ev.graph.GetNodesByFile(filePath)

	// Build lookup of existing symbol nodes by their ID.
	existingSymbols := make(map[string]*Node)
	for _, n := range existingNodes {
		if n.Kind == KindSymbol {
			existingSymbols[n.ID] = n
		}
	}

	// Build set of new symbol IDs so we can detect removals.
	newSymbolIDs := make(map[string]bool, len(symbols))

	now := time.Now()

	for _, sym := range symbols {
		nodeID := makeSymbolNodeID(filePath, sym)
		newSymbolIDs[nodeID] = true

		newFeature := ev.extractor.ExtractFeature(sym.Name, sym.Signature, sym.Receiver, "")

		if existing, ok := existingSymbols[nodeID]; ok {
			// Existing symbol -- check drift.
			drift := calculateDrift(existing.Feature, newFeature)

			if drift < ev.driftThreshold {
				// In-place update: patch fields without hierarchy change.
				existing.Feature = newFeature
				existing.Signature = sym.Signature
				existing.StartLine = sym.Line
				existing.EndLine = normalizeEndLine(sym.Line, sym.EndLine)
				existing.UpdatedAt = now
			} else {
				// Semantic drift: remove old node and re-create with new
				// hierarchy placement.
				parentIDs := collectFeatureParents(ev.graph, existing.ID)
				ev.graph.RemoveNode(existing.ID)

				for pid := range parentIDs {
					ev.pruneOrphans(pid)
				}

				ev.addSymbolNode(filePath, sym, newFeature, now)
			}
		} else {
			// Brand new symbol -- treat as add.
			ev.addSymbolNode(filePath, sym, newFeature, now)
		}
	}

	// Handle removed symbols: present in old graph but not in new symbol list.
	for id, n := range existingSymbols {
		if !newSymbolIDs[id] {
			parentIDs := collectFeatureParents(ev.graph, n.ID)
			ev.graph.RemoveNode(id)
			for pid := range parentIDs {
				ev.pruneOrphans(pid)
			}
		}
	}
}

// HandleAdd adds new nodes for a newly created file.
func (ev *Evolver) HandleAdd(filePath string, symbols []trace.Symbol) {
	now := time.Now()

	// Create file node.
	fileID := MakeNodeID(KindFile, filePath)
	if ev.graph.GetNode(fileID) == nil {
		fileNode := &Node{
			ID:        fileID,
			Kind:      KindFile,
			Path:      filePath,
			UpdatedAt: now,
		}
		ev.graph.AddNode(fileNode)
	}

	// Classify file into hierarchy and ensure hierarchy nodes exist.
	areaName, catName := ev.hierarchy.ClassifyFile(filePath)
	areaID := ev.hierarchy.EnsureArea(areaName)
	catID := ev.hierarchy.EnsureCategory(areaID, catName)

	// Link file -> category via EdgeFeatureParent.
	ev.graph.AddEdge(&Edge{
		From:      fileID,
		To:        catID,
		Type:      EdgeFeatureParent,
		Weight:    1.0,
		UpdatedAt: now,
	})

	// Create symbol nodes.
	for _, sym := range symbols {
		feature := ev.extractor.ExtractFeature(sym.Name, sym.Signature, sym.Receiver, "")
		ev.addSymbolNode(filePath, sym, feature, now)
	}
}

// addSymbolNode creates a symbol node, links it to its file via
// EdgeContains, classifies it in the hierarchy, and adds an
// EdgeFeatureParent edge to the appropriate subcategory.
func (ev *Evolver) addSymbolNode(filePath string, sym trace.Symbol, feature string, now time.Time) {
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
		UpdatedAt:  now,
	}
	ev.graph.AddNode(symNode)

	// EdgeContains: file -> symbol.
	fileID := MakeNodeID(KindFile, filePath)
	ev.graph.AddEdge(&Edge{
		From:      fileID,
		To:        nodeID,
		Type:      EdgeContains,
		Weight:    1.0,
		UpdatedAt: now,
	})

	// Classify into hierarchy.
	areaName, catName := ev.hierarchy.ClassifyFile(filePath)
	areaID := ev.hierarchy.EnsureArea(areaName)
	catID := ev.hierarchy.EnsureCategory(areaID, catName)

	subcatName := ev.hierarchy.ClassifySymbol(feature)
	subcatID := ev.hierarchy.EnsureSubcategory(catID, subcatName)

	// EdgeFeatureParent: symbol -> subcategory.
	ev.graph.AddEdge(&Edge{
		From:      nodeID,
		To:        subcatID,
		Type:      EdgeFeatureParent,
		Weight:    1.0,
		UpdatedAt: now,
	})
}

// pruneOrphans removes hierarchy nodes that have no children.
// A node is orphaned when no other node points to it via EdgeFeatureParent
// (for subcategories) or EdgeContains (for categories/areas).
func (ev *Evolver) pruneOrphans(nodeID string) {
	node := ev.graph.GetNode(nodeID)
	if node == nil {
		return
	}

	// Only prune hierarchy nodes.
	if node.Kind != KindArea && node.Kind != KindCategory && node.Kind != KindSubcategory {
		return
	}

	// Check if any node still has an incoming edge pointing TO this node.
	// Subcategories receive EdgeFeatureParent from symbols.
	// Categories receive EdgeContains from subcategories and EdgeFeatureParent from files.
	// Areas receive EdgeContains from categories.
	incoming := ev.graph.GetIncoming(nodeID)
	hasChildren := false
	for _, e := range incoming {
		if e.Type == EdgeFeatureParent || e.Type == EdgeContains {
			hasChildren = true
			break
		}
	}

	if hasChildren {
		return
	}

	// Find this node's parent before removal.
	// Subcategories -> category via EdgeContains.
	// Categories -> area via EdgeContains.
	var parentID string
	for _, e := range ev.graph.GetOutgoing(nodeID) {
		if e.Type == EdgeContains || e.Type == EdgeFeatureParent {
			parentID = e.To
			break
		}
	}

	ev.graph.RemoveNode(nodeID)

	// Recursively check parent.
	if parentID != "" {
		ev.pruneOrphans(parentID)
	}
}

// calculateDrift computes a simple drift score between two feature strings.
// Returns 0.0 (identical) to 1.0 (completely different).
// Uses Jaccard distance on word sets, splitting features by "-".
func calculateDrift(oldFeature, newFeature string) float64 {
	if oldFeature == newFeature {
		return 0.0
	}
	if oldFeature == "" || newFeature == "" {
		return 1.0
	}

	oldWords := splitFeatureWords(oldFeature)
	newWords := splitFeatureWords(newFeature)

	if len(oldWords) == 0 && len(newWords) == 0 {
		return 0.0
	}

	// Build union and intersection.
	union := make(map[string]bool)
	for w := range oldWords {
		union[w] = true
	}
	for w := range newWords {
		union[w] = true
	}

	intersectionCount := 0
	for w := range oldWords {
		if newWords[w] {
			intersectionCount++
		}
	}

	if len(union) == 0 {
		return 0.0
	}

	jaccard := float64(intersectionCount) / float64(len(union))
	// Round to avoid floating point noise.
	return math.Round((1.0-jaccard)*1000) / 1000
}

// splitFeatureWords splits a feature string by common delimiters into a
// lowercase word set.
func splitFeatureWords(s string) map[string]bool {
	s = strings.ToLower(s)
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-' || r == ' ' || r == '/' || r == '_' || r == '@'
	})
	set := make(map[string]bool, len(parts))
	for _, p := range parts {
		if p != "" {
			set[p] = true
		}
	}
	return set
}

// makeSymbolNodeID builds a deterministic node ID for a trace.Symbol.
func makeSymbolNodeID(filePath string, sym trace.Symbol) string {
	if sym.Receiver != "" {
		return MakeNodeID(KindSymbol, filePath, sym.Receiver, sym.Name)
	}
	return MakeNodeID(KindSymbol, filePath, sym.Name)
}

// collectFeatureParents returns the set of node IDs that a node points to
// via EdgeFeatureParent edges.
func collectFeatureParents(g *Graph, nodeID string) map[string]bool {
	parents := make(map[string]bool)
	for _, e := range g.GetOutgoing(nodeID) {
		if e.Type == EdgeFeatureParent {
			parents[e.To] = true
		}
	}
	return parents
}
