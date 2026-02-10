package rpg

import (
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
// Strategy:
//  1. Group files by top-level directory -> these become "areas"
//  2. Within each area, group by subdirectory or file stem -> "categories"
//  3. Within each category, group symbols by feature verb -> "subcategories"
//  4. Connect file/symbol nodes to their subcategory via EdgeFeatureParent
//  5. Connect subcategories to categories, categories to areas via EdgeContains
func (h *HierarchyBuilder) BuildHierarchy() {
	now := time.Now()

	// Process all file nodes: create area and category hierarchy, link files.
	fileNodes := h.graph.GetNodesByKind(KindFile)
	for _, fn := range fileNodes {
		areaName, catName := h.ClassifyFile(fn.Path)
		areaID := h.EnsureArea(areaName)
		catID := h.EnsureCategory(areaID, catName)

		// Link file -> category via EdgeFeatureParent.
		h.graph.AddEdge(&Edge{
			From:      fn.ID,
			To:        catID,
			Type:      EdgeFeatureParent,
			Weight:    1.0,
			UpdatedAt: now,
		})
	}

	// Process all symbol nodes: extract features, create subcategories, link symbols.
	symbolNodes := h.graph.GetNodesByKind(KindSymbol)
	for _, sn := range symbolNodes {
		// Determine the file's area/category.
		areaName, catName := h.ClassifyFile(sn.Path)
		areaID := h.EnsureArea(areaName)
		catID := h.EnsureCategory(areaID, catName)

		// Extract or use existing feature label.
		feature := sn.Feature
		if feature == "" {
			feature = h.extractor.ExtractFeature(sn.SymbolName, sn.Signature, sn.Receiver, "")
			sn.Feature = feature
		}

		// Determine subcategory from the feature label.
		subcatName := h.ClassifySymbol(feature)
		subcatID := h.EnsureSubcategory(catID, subcatName)

		// Link symbol -> subcategory via EdgeFeatureParent.
		h.graph.AddEdge(&Edge{
			From:      sn.ID,
			To:        subcatID,
			Type:      EdgeFeatureParent,
			Weight:    1.0,
			UpdatedAt: now,
		})
	}
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
// and feature path. A EdgeContains edge from category to area is created if the
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

	// Link category -> area via EdgeContains (child points to parent).
	h.graph.AddEdge(&Edge{
		From:      id,
		To:        areaID,
		Type:      EdgeContains,
		Weight:    1.0,
		UpdatedAt: time.Now(),
	})

	return id
}

// EnsureSubcategory ensures a subcategory node exists under a category and returns its ID.
// The catID is used to extract the category feature path for building the subcategory's
// node ID and feature path. A EdgeContains edge from subcategory to category is created
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

	// Link subcategory -> category via EdgeContains (child points to parent).
	h.graph.AddEdge(&Edge{
		From:      id,
		To:        catID,
		Type:      EdgeContains,
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
	for _, area := range h.graph.GetNodesByKind(KindArea) {
		verbs := h.collectDescendantVerbs(area.ID)
		if len(verbs) > 0 {
			area.SemanticLabel = area.Feature + " [" + strings.Join(topN(verbs, 3), ", ") + "]"
		}
	}
	for _, cat := range h.graph.GetNodesByKind(KindCategory) {
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

	// Check incoming edges (children point TO parents via EdgeFeatureParent/EdgeContains)
	for _, e := range h.graph.GetIncoming(nodeID) {
		if e.Type == EdgeFeatureParent || e.Type == EdgeContains {
			child := h.graph.GetNode(e.From)
			if child == nil {
				continue
			}
			if child.Kind == KindSymbol {
				// Extract verb from feature
				if child.Feature != "" {
					verb := firstWord(child.Feature)
					if verb != "" {
						verbCounts[verb]++
					}
				}
			} else {
				// Recurse into hierarchy children
				h.walkDescendants(child.ID, visited, verbCounts)
			}
		}
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
