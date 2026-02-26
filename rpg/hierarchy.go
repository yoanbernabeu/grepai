package rpg

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// HierarchyBuilder constructs the area/category/subcategory hierarchy for RPG nodes.
type HierarchyBuilder struct {
	graph     *Graph
	extractor FeatureExtractor
}

// NewHierarchyBuilder creates a new hierarchy builder for the given graph and feature extractor.
func NewHierarchyBuilder(graph *Graph, extractor FeatureExtractor) *HierarchyBuilder {
	return &HierarchyBuilder{
		graph:     graph,
		extractor: extractor,
	}
}

// BuildHierarchy constructs hierarchy nodes and edges for the entire graph.
// It examines all KindFile and KindSymbol nodes and organizes them into
// area/category/subcategory based on directory structure and feature labels.
//
// NOTE: This implements a heuristic approximation of the RPG-Encoder "Structure Reorganization" (Phase 2).
// While the paper describes an LLM-driven clustering approach to induce functional centroids,
// this implementation utilizes the explicit directory structure as a proxy for high-level
// functional areas ($V_H$), combined with symbol-level verb extraction for finer granularity.
// This trade-off significantly reduces indexing cost while maintaining structural coherence.
//
// Strategy:
//  1. Group files by top-level directory -> these become "areas"
//  2. Within each area, group by subdirectory or file stem -> "categories"
//  3. Within each category, group symbols by feature verb -> "subcategories"
//  4. Connect hierarchy as area -> category -> subcategory -> file via EdgeFeatureParent
//  5. Keep implementation containment as file -> symbol via EdgeContains
func (h *HierarchyBuilder) BuildHierarchy() {
	now := time.Now()

	// Process all file nodes: create area/category hierarchy and group files.
	fileNodes := h.graph.GetNodesByKind(KindFile)
	sort.Slice(fileNodes, func(i, j int) bool {
		return fileNodes[i].Path < fileNodes[j].Path
	})

	type categoryKey struct {
		areaName string
		catName  string
	}
	byCategoryFiles := make(map[categoryKey][]*Node)

	for _, fn := range fileNodes {
		areaName, catName := h.ClassifyFile(fn.Path)
		key := categoryKey{areaName, catName}
		byCategoryFiles[key] = append(byCategoryFiles[key], fn)

		areaID := h.EnsureArea(areaName)
		h.EnsureCategory(areaID, catName)
	}

	// Process all symbol nodes: group by category for subcategory clustering.
	symbolNodes := h.graph.GetNodesByKind(KindSymbol)
	sort.Slice(symbolNodes, func(i, j int) bool {
		return symbolNodes[i].ID < symbolNodes[j].ID
	})

	byCategory := make(map[categoryKey][]*Node)
	for _, sn := range symbolNodes {
		areaName, catName := h.ClassifyFile(sn.Path)
		key := categoryKey{areaName, catName}
		byCategory[key] = append(byCategory[key], sn)
	}

	// Union keys (files/symbols) for deterministic processing.
	seenKeys := make(map[categoryKey]struct{})
	keys := make([]categoryKey, 0, len(byCategory)+len(byCategoryFiles))
	for k := range byCategory {
		seenKeys[k] = struct{}{}
		keys = append(keys, k)
	}
	for k := range byCategoryFiles {
		if _, ok := seenKeys[k]; ok {
			continue
		}
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].areaName != keys[j].areaName {
			return keys[i].areaName < keys[j].areaName
		}
		return keys[i].catName < keys[j].catName
	})

	for _, key := range keys {
		symbols := byCategory[key]
		areaID := h.EnsureArea(key.areaName)
		catID := h.EnsureCategory(areaID, key.catName)

		// Cluster symbols within this category.
		clusters := h.ClusterSymbols(symbols)
		if len(clusters) == 0 {
			clusters = map[string][]*Node{
				"general": {},
			}
		}

		// Create subcategories and remember IDs.
		var clusterNames []string
		for name := range clusters {
			clusterNames = append(clusterNames, name)
		}
		sort.Strings(clusterNames)
		clusterIDs := make(map[string]string, len(clusterNames))

		for _, name := range clusterNames {
			clusterIDs[name] = h.EnsureSubcategory(catID, name)
		}

		files := byCategoryFiles[key]
		sort.Slice(files, func(i, j int) bool {
			return files[i].Path < files[j].Path
		})

		for _, fileNode := range files {
			subcatName := h.selectFileSubcategory(fileNode.Path, symbols)
			if subcatName == "" {
				subcatName = "general"
			}

			subcatID, ok := clusterIDs[subcatName]
			if !ok {
				subcatID = h.EnsureSubcategory(catID, subcatName)
				clusterIDs[subcatName] = subcatID
			}

			if edgeExists(h.graph, subcatID, fileNode.ID, EdgeFeatureParent) {
				continue
			}
			h.graph.AddEdge(&Edge{
				From:      subcatID,
				To:        fileNode.ID,
				Type:      EdgeFeatureParent,
				Weight:    1.0,
				UpdatedAt: now,
			})
		}
	}
}

// extractClusterKey determines the best clustering key for a feature string.
// It prioritizes specific verbs, but falls back to the object if the verb is generic.
func (h *HierarchyBuilder) extractClusterKey(feature string) string {
	genericVerbs := map[string]bool{
		"get": true, "set": true, "do": true, "handle": true, "process": true, "run": true, "create": true, "new": true,
	}

	// Default: cluster by verb (first word)
	clusterName := firstWord(feature)

	// Refinement: if verb is generic, try to use object (second word)
	// We need to split by separators to get the second part
	f := func(c rune) bool {
		return c == '-' || c == '_' || c == '/' || c == ' ' || c == '@'
	}
	parts := strings.FieldsFunc(feature, f)

	if len(parts) >= 2 && genericVerbs[clusterName] {
		// e.g. "handle-request" -> "request"
		clusterName = parts[1]
	}

	if clusterName == "" {
		clusterName = "misc"
	}
	return clusterName
}

// ClusterSymbols groups symbols based on their feature labels.
// It uses a heuristic to determine the best clustering strategy (e.g. by verb, or by object if verb is generic).
func (h *HierarchyBuilder) ClusterSymbols(symbols []*Node) map[string][]*Node {
	clusters := make(map[string][]*Node)
	for _, sn := range symbols {
		feature := ""
		atomics := getNodeAtomicFeatures(sn)
		if len(atomics) > 0 {
			feature = atomics[0]
		}
		if feature == "" {
			feature = h.extractor.ExtractFeature(context.Background(), sn.SymbolName, sn.Signature, sn.Receiver, "")
			setNodeFeatures(sn, []string{atomicFromPrimaryFeature(feature)}, feature)
		}

		clusterName := h.extractClusterKey(feature)
		clusters[clusterName] = append(clusters[clusterName], sn)
	}
	return clusters
}

// selectSubcategoryByFrequency determines the most frequent cluster key among
// symbol atomic features for a given file path. When filePath is non-empty,
// only symbols whose Path matches are considered. Symbols without atomic
// features are skipped.
func selectSubcategoryByFrequency(symbols []*Node, filePath string, hb *HierarchyBuilder) string {
	counts := make(map[string]int)
	for _, sym := range symbols {
		if sym.Kind != KindSymbol {
			continue
		}
		if filePath != "" && sym.Path != filePath {
			continue
		}
		features := getNodeAtomicFeatures(sym)
		if len(features) == 0 {
			continue
		}
		cluster := hb.extractClusterKey(features[0])
		if cluster != "" {
			counts[cluster]++
		}
	}
	if len(counts) == 0 {
		return ""
	}

	type entry struct {
		name  string
		count int
	}
	entries := make([]entry, 0, len(counts))
	for name, count := range counts {
		entries = append(entries, entry{name: name, count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}
		return entries[i].name < entries[j].name
	})
	return entries[0].name
}

func (h *HierarchyBuilder) selectFileSubcategory(filePath string, symbols []*Node) string {
	return selectSubcategoryByFrequency(symbols, filePath, h)
}

// EnsureArea ensures an area node exists and returns its ID.
// If the node already exists, it is not recreated.
func (h *HierarchyBuilder) EnsureArea(name string) string {
	id := MakeNodeID(KindArea, name)
	if existing := h.graph.GetNode(id); existing != nil {
		return id
	}

	h.graph.AddNode(&Node{
		ID:        id,
		Kind:      KindArea,
		Feature:   name,
		UpdatedAt: time.Now(),
	})
	return id
}

// EnsureCategory ensures a category node exists under an area and returns its ID.
// The areaID is used to extract the area name for building the category's node ID
// and feature path. A EdgeFeatureParent edge from area to category is created if the
// category is new.
func (h *HierarchyBuilder) EnsureCategory(areaID, name string) string {
	// Extract the area name from the areaID for building the category ID.
	// areaID format: "area:<name>"
	areaName := strings.TrimPrefix(areaID, "area:")

	id := MakeNodeID(KindCategory, areaName, name)
	if existing := h.graph.GetNode(id); existing != nil {
		return id
	}

	featurePath := areaName + "/" + name
	h.graph.AddNode(&Node{
		ID:        id,
		Kind:      KindCategory,
		Feature:   featurePath,
		UpdatedAt: time.Now(),
	})

	// Link area -> category via EdgeFeatureParent (parent points to child).
	h.graph.AddEdge(&Edge{
		From:      areaID,
		To:        id,
		Type:      EdgeFeatureParent,
		Weight:    1.0,
		UpdatedAt: time.Now(),
	})

	return id
}

// EnsureSubcategory ensures a subcategory node exists under a category and returns its ID.
// The catID is used to extract the category feature path for building the subcategory's
// node ID and feature path. A EdgeFeatureParent edge from category to subcategory is created
// if the subcategory is new.
func (h *HierarchyBuilder) EnsureSubcategory(catID, name string) string {
	// Extract the category path from the catID for building the subcategory ID.
	// catID format: "cat:<area>/<category>"
	catPath := strings.TrimPrefix(catID, "cat:")

	id := MakeNodeID(KindSubcategory, catPath, name)
	if existing := h.graph.GetNode(id); existing != nil {
		return id
	}

	featurePath := catPath + "/" + name
	h.graph.AddNode(&Node{
		ID:        id,
		Kind:      KindSubcategory,
		Feature:   featurePath,
		UpdatedAt: time.Now(),
	})

	// Link category -> subcategory via EdgeFeatureParent (parent points to child).
	h.graph.AddEdge(&Edge{
		From:      catID,
		To:        id,
		Type:      EdgeFeatureParent,
		Weight:    1.0,
		UpdatedAt: time.Now(),
	})

	return id
}

// ClassifyFile determines the area and category for a file based on its path.
// It uses directory structure:
//
//	"cli/watch.go"           -> area="cli",     category="watch"
//	"store/gob.go"           -> area="store",   category="gob"
//	"indexer/chunker.go"     -> area="indexer",  category="chunker"
//	"main.go"                -> area="root",     category="main"
//	"internal/foo/bar.go"    -> area="internal", category="foo"
//	"a/b/c/deep.go"          -> area="a",        category="b"
//
// Returns (areaName, categoryName).
func (h *HierarchyBuilder) ClassifyFile(filePath string) (string, string) {
	// Normalize to forward slashes and clean the path.
	cleaned := filepath.ToSlash(filepath.Clean(filePath))

	// Strip any leading "./" or "/".
	cleaned = strings.TrimPrefix(cleaned, "./")
	cleaned = strings.TrimPrefix(cleaned, "/")

	parts := strings.Split(cleaned, "/")

	switch len(parts) {
	case 0:
		return "root", "unknown"
	case 1:
		// Top-level file like "main.go".
		stem := fileNameStem(parts[0])
		return "root", stem
	default:
		// At least one directory level.
		areaName := parts[0]
		if len(parts) == 2 {
			// e.g., "cli/watch.go" -> area="cli", category="watch"
			catName := fileNameStem(parts[1])
			return areaName, catName
		}
		// Deeper paths: area is first dir, category is second dir.
		// e.g., "internal/foo/bar.go" -> area="internal", category="foo"
		catName := parts[1]
		return areaName, catName
	}
}

// ClassifySymbol determines the subcategory for a symbol based on its feature label.
// It uses the first word (verb) of the feature as the subcategory grouping.
//
//	"handle-request"     -> "handle"
//	"validate-token"     -> "validate"
//	"parse-config"       -> "parse"
//	"operate-server"     -> "operate"
//	"unknown"            -> "general"
//	""                   -> "general"
func (h *HierarchyBuilder) ClassifySymbol(feature string) string {
	if feature == "" {
		return "general"
	}

	// The feature label is kebab-case: "verb-object" or "verb-object@receiver".
	// Strip the receiver suffix if present.
	label := feature
	if atIdx := strings.Index(label, "@"); atIdx >= 0 {
		label = label[:atIdx]
	}

	// Split on hyphens and take the first word as subcategory.
	parts := strings.SplitN(label, "-", 2)
	verb := strings.TrimSpace(parts[0])
	if verb == "" {
		return "general"
	}

	return verb
}

// fileNameStem returns the file name without its extension.
// e.g., "chunker.go" -> "chunker", "config.yaml" -> "config".
func fileNameStem(name string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return name
	}
	return strings.TrimSuffix(name, ext)
}

// EnrichLabels enriches area and category nodes with semantic summaries
// derived from their descendant symbol features. This adds semantic depth
// to directory-based hierarchy labels by populating the SemanticLabel field.
//
// Example: area "cli" with descendants [handle-search, handle-trace, run-watch]
// becomes "cli [handle, run]" providing semantic context about what the area does.
func (h *HierarchyBuilder) EnrichLabels() {
	areas := h.graph.GetNodesByKind(KindArea)
	sort.Slice(areas, func(i, j int) bool {
		return areas[i].ID < areas[j].ID
	})
	for _, area := range areas {
		verbs := h.collectDescendantVerbs(area.ID)
		if len(verbs) > 0 {
			area.SemanticLabel = area.Feature + " [" + strings.Join(topN(verbs, 3), ", ") + "]"
		}
	}

	cats := h.graph.GetNodesByKind(KindCategory)
	sort.Slice(cats, func(i, j int) bool {
		return cats[i].ID < cats[j].ID
	})
	for _, cat := range cats {
		verbs := h.collectDescendantVerbs(cat.ID)
		if len(verbs) > 0 {
			cat.SemanticLabel = cat.Feature + " [" + strings.Join(topN(verbs, 3), ", ") + "]"
		}
	}
}

// collectDescendantVerbs collects feature verbs from all descendant symbol nodes.
func (h *HierarchyBuilder) collectDescendantVerbs(nodeID string) map[string]int {
	verbCounts := make(map[string]int)
	visited := make(map[string]bool)
	h.walkDescendants(nodeID, visited, verbCounts)
	return verbCounts
}

// walkDescendants recursively collects feature verbs from descendants.
func (h *HierarchyBuilder) walkDescendants(nodeID string, visited map[string]bool, verbCounts map[string]int) {
	if visited[nodeID] {
		return
	}
	visited[nodeID] = true

	// Traverse outgoing edges (parent points to children).
	outgoing := h.graph.GetOutgoing(nodeID)
	sort.Slice(outgoing, func(i, j int) bool {
		return outgoing[i].To < outgoing[j].To
	})

	for _, e := range outgoing {
		if e.Type != EdgeFeatureParent && e.Type != EdgeContains {
			continue
		}

		child := h.graph.GetNode(e.To)
		if child == nil {
			continue
		}

		if child.Kind == KindSymbol {
			for _, feature := range getNodeAtomicFeatures(child) {
				key := h.extractClusterKey(feature)
				if key != "" && key != "misc" {
					verbCounts[key]++
				}
			}
			continue
		}
		h.walkDescendants(child.ID, visited, verbCounts)
	}
}

// topN returns the top N entries from a frequency map, sorted by count descending.
func topN(counts map[string]int, n int) []string {
	type entry struct {
		key   string
		count int
	}
	entries := make([]entry, 0, len(counts))
	for k, v := range counts {
		entries = append(entries, entry{k, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}
		return entries[i].key < entries[j].key
	})
	result := make([]string, 0, n)
	for i := 0; i < len(entries) && i < n; i++ {
		result = append(result, entries[i].key)
	}
	return result
}

// Helper to extract the first word of a feature string
func firstWord(s string) string {
	f := func(c rune) bool {
		return c == '-' || c == '_' || c == '/' || c == ' ' || c == '@'
	}
	parts := strings.FieldsFunc(s, f)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}
