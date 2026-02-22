package rpg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SearchNodeRequest is the input for SearchNode.
type SearchNodeRequest struct {
	Query             string     `json:"query"`                          // natural language or feature query (legacy)
	Scope             string     `json:"scope,omitempty"`                // area/category path to narrow search (legacy)
	Kinds             []NodeKind `json:"kinds,omitempty"`                // filter by node kind (default: symbol)
	Limit             int        `json:"limit,omitempty"`                // max results (default: 10)
	Mode              string     `json:"mode,omitempty"`                 // features | snippets | auto
	FeatureTerms      []string   `json:"feature_terms,omitempty"`        // behavior/functionality phrases
	SearchScopes      []string   `json:"search_scopes,omitempty"`        // optional scope filters
	SearchTerms       []string   `json:"search_terms,omitempty"`         // snippet-oriented query terms
	LineNums          []int      `json:"line_nums,omitempty"`            // reserved for parity (not used in graph search)
	FilePathOrPattern string     `json:"file_path_or_pattern,omitempty"` // optional file path/glob filter
}

// SearchNodeResult is a single search result.
type SearchNodeResult struct {
	Node        *Node   `json:"node"`
	Score       float64 `json:"score"`
	FeaturePath string  `json:"feature_path"` // area/category/subcategory path
}

// FetchNodeRequest is the input for FetchNode.
type FetchNodeRequest struct {
	NodeID          string   `json:"node_id,omitempty"`
	CodeEntities    []string `json:"code_entities,omitempty"`
	FeatureEntities []string `json:"feature_entities,omitempty"`
}

// FetchNodeResult contains detailed node info with context.
type FetchNodeResult struct {
	Node        *Node   `json:"node"`
	FeaturePath string  `json:"feature_path"`
	Parents     []*Node `json:"parents,omitempty"`      // hierarchy chain
	Children    []*Node `json:"children,omitempty"`     // contained nodes
	Incoming    []*Edge `json:"incoming,omitempty"`     // incoming edges
	Outgoing    []*Edge `json:"outgoing,omitempty"`     // outgoing edges
	CodePreview string  `json:"code_preview,omitempty"` // source snippet when available
}

// ExploreRequest is the input for Explore.
type ExploreRequest struct {
	StartNodeID          string     `json:"start_node_id,omitempty"`
	StartCodeEntities    []string   `json:"start_code_entities,omitempty"`
	StartFeatureEntities []string   `json:"start_feature_entities,omitempty"`
	Direction            string     `json:"direction"`                    // forward, reverse, both
	Depth                int        `json:"depth,omitempty"`              // max depth (default: 2)
	TraversalDepth       int        `json:"traversal_depth,omitempty"`    // alias for depth
	EdgeTypes            []EdgeType `json:"edge_types,omitempty"`         // filter by edge type
	EntityTypeFilter     string     `json:"entity_type_filter,omitempty"` // directory | file | class | function | method
	Limit                int        `json:"limit,omitempty"`              // max nodes returned
}

// ExploreResult contains the explored subgraph.
type ExploreResult struct {
	StartNode *Node            `json:"start_node"`
	Nodes     map[string]*Node `json:"nodes"`
	Edges     []*Edge          `json:"edges"`
	Depth     int              `json:"depth"`
}

// QueryEngine provides the 3 RPG query operations.
type QueryEngine struct {
	graph *Graph
}

// NewQueryEngine creates a QueryEngine backed by the given graph.
func NewQueryEngine(graph *Graph) *QueryEngine {
	return &QueryEngine{graph: graph}
}

// SearchNode finds nodes matching a query within optional scope.
// Scoring uses Jaccard similarity between query words and the union of a
// node's Feature label words and SymbolName words.
func (qe *QueryEngine) SearchNode(_ context.Context, req SearchNodeRequest) ([]SearchNodeResult, error) {
	if req.Limit <= 0 {
		req.Limit = 10
	}
	if len(req.Kinds) == 0 {
		req.Kinds = []NodeKind{KindSymbol}
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "legacy"
	}

	// Legacy mode preserves existing behavior.
	if mode == "legacy" {
		return qe.searchNodeInternal(req.Query, req.Scope, req.Kinds, req.Limit, req.FilePathOrPattern, nil)
	}

	switch mode {
	case "features":
		query := firstNonEmpty(strings.Join(req.FeatureTerms, " "), req.Query)
		return qe.searchNodeInternal(query, req.Scope, req.Kinds, req.Limit, req.FilePathOrPattern, req.SearchScopes)
	case "snippets":
		query := firstNonEmpty(strings.Join(req.SearchTerms, " "), req.Query)
		return qe.searchNodeInternal(query, req.Scope, req.Kinds, req.Limit, req.FilePathOrPattern, req.SearchScopes)
	case "auto":
		featureQuery := firstNonEmpty(strings.Join(req.FeatureTerms, " "), req.Query)
		results, err := qe.searchNodeInternal(featureQuery, req.Scope, req.Kinds, req.Limit, req.FilePathOrPattern, req.SearchScopes)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			return results, nil
		}
		snippetQuery := firstNonEmpty(strings.Join(req.SearchTerms, " "), req.Query)
		return qe.searchNodeInternal(snippetQuery, req.Scope, req.Kinds, req.Limit, req.FilePathOrPattern, req.SearchScopes)
	default:
		return nil, fmt.Errorf("unsupported search mode %q", req.Mode)
	}
}

func (qe *QueryEngine) searchNodeInternal(query string, scope string, kinds []NodeKind, limit int, filePathPattern string, extraScopes []string) ([]SearchNodeResult, error) {
	queryWords := normalizeWords(query)
	if len(queryWords) == 0 {
		return nil, nil
	}

	// Build the set of allowed kinds for fast lookup.
	kindSet := make(map[NodeKind]bool, len(kinds))
	for _, k := range kinds {
		kindSet[k] = true
	}

	// Collect candidate nodes.
	var candidates []*Node
	for _, kind := range kinds {
		candidates = append(candidates, qe.graph.GetNodesByKind(kind)...)
	}

	// Filter by scopes if set. Scope is matched as a prefix of the node's
	// feature path (e.g. "cli" or "cli/commands").
	scopeFilters := make([]string, 0, 1+len(extraScopes))
	if scope != "" {
		scopeFilters = append(scopeFilters, strings.ToLower(scope))
	}
	for _, s := range extraScopes {
		s = strings.TrimSpace(strings.ToLower(s))
		if s != "" {
			scopeFilters = append(scopeFilters, s)
		}
	}

	type scored struct {
		node  *Node
		score float64
		path  string
	}
	var results []scored

	for _, n := range candidates {
		if !kindSet[n.Kind] {
			continue
		}
		if !matchesFilePathPattern(n, filePathPattern) {
			continue
		}

		featurePath := qe.getFeaturePath(n.ID)
		if len(scopeFilters) > 0 && !matchesAnyScope(featurePath, scopeFilters) {
			continue
		}

		score := scoreMatch(queryWords, n)
		if score > 0 {
			results = append(results, scored{node: n, score: score, path: featurePath})
		}
	}

	// Sort by score descending.
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// Apply limit.
	if len(results) > limit {
		results = results[:limit]
	}

	out := make([]SearchNodeResult, len(results))
	for i, r := range results {
		out[i] = SearchNodeResult{
			Node:        r.node,
			Score:       r.score,
			FeaturePath: r.path,
		}
	}
	return out, nil
}

// FetchNode retrieves detailed information about a specific node.
func (qe *QueryEngine) FetchNode(_ context.Context, req FetchNodeRequest) (*FetchNodeResult, error) {
	if strings.TrimSpace(req.NodeID) == "" {
		return nil, fmt.Errorf("node ID is required")
	}
	node := qe.graph.GetNode(req.NodeID)
	if node == nil {
		return nil, nil
	}

	result := &FetchNodeResult{
		Node:        node,
		FeaturePath: qe.getFeaturePath(node.ID),
		Incoming:    qe.graph.GetIncoming(node.ID),
		Outgoing:    qe.graph.GetOutgoing(node.ID),
		CodePreview: readNodeCodePreview(node),
	}

	// Build hierarchy chain by walking upward through parent links.
	visited := make(map[string]bool)
	current := node.ID
	for {
		parentID := findParentID(qe.graph, current)
		if parentID == "" || visited[parentID] {
			break
		}
		visited[parentID] = true
		parentNode := qe.graph.GetNode(parentID)
		if parentNode == nil {
			break
		}
		result.Parents = append(result.Parents, parentNode)
		current = parentID
	}

	// Collect direct children via outgoing hierarchy/containment edges.
	for _, e := range qe.graph.GetOutgoing(node.ID) {
		if e.Type == EdgeContains || e.Type == EdgeFeatureParent {
			if child := qe.graph.GetNode(e.To); child != nil {
				result.Children = append(result.Children, child)
			}
		}
	}

	return result, nil
}

// FetchNodes retrieves details for one or more entities.
// Resolution order:
//  1. req.NodeID
//  2. req.CodeEntities (node IDs, file paths, or symbol names)
//  3. req.FeatureEntities (feature paths)
func (qe *QueryEngine) FetchNodes(ctx context.Context, req FetchNodeRequest) ([]*FetchNodeResult, error) {
	nodeIDs := qe.resolveFetchNodeIDs(req)
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	results := make([]*FetchNodeResult, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		fetchResult, err := qe.FetchNode(ctx, FetchNodeRequest{NodeID: nodeID})
		if err != nil {
			return nil, err
		}
		if fetchResult != nil {
			results = append(results, fetchResult)
		}
	}
	return results, nil
}

// Explore traverses the graph from a start node using BFS.
func (qe *QueryEngine) Explore(_ context.Context, req ExploreRequest) (*ExploreResult, error) {
	depth := req.Depth
	if depth <= 0 {
		depth = req.TraversalDepth
	}
	if depth <= 0 {
		depth = 2
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}

	startNodeID := strings.TrimSpace(req.StartNodeID)
	if startNodeID == "" {
		startNodeID = qe.resolveExploreStartNodeID(req.StartCodeEntities, req.StartFeatureEntities)
	}

	startNode := qe.graph.GetNode(startNodeID)
	if startNode == nil {
		return nil, nil
	}

	// Build edge type filter set.
	edgeTypeSet := make(map[EdgeType]bool, len(req.EdgeTypes))
	for _, et := range req.EdgeTypes {
		edgeTypeSet[et] = true
	}
	filterEdges := len(edgeTypeSet) > 0

	result := &ExploreResult{
		StartNode: startNode,
		Nodes:     make(map[string]*Node),
		Edges:     make([]*Edge, 0),
		Depth:     0,
	}
	result.Nodes[startNode.ID] = startNode

	matchesEntityType := buildEntityTypeMatcher(req.EntityTypeFilter)

	// BFS state.
	type bfsEntry struct {
		nodeID string
		depth  int
	}
	queue := []bfsEntry{{nodeID: startNode.ID, depth: 0}}
	visited := map[string]bool{startNode.ID: true}

	for len(queue) > 0 {
		entry := queue[0]
		queue = queue[1:]

		if entry.depth >= depth {
			continue
		}

		if len(result.Nodes) >= limit {
			break
		}

		var edges []*Edge

		switch req.Direction {
		case "forward":
			edges = qe.graph.GetOutgoing(entry.nodeID)
		case "reverse":
			edges = qe.graph.GetIncoming(entry.nodeID)
		default: // "both"
			edges = append(edges, qe.graph.GetOutgoing(entry.nodeID)...)
			edges = append(edges, qe.graph.GetIncoming(entry.nodeID)...)
		}

		for _, e := range edges {
			// Apply edge type filter.
			if filterEdges && !edgeTypeSet[e.Type] {
				continue
			}

			result.Edges = append(result.Edges, e)

			// Determine the neighbor ID.
			neighborID := e.To
			if neighborID == entry.nodeID {
				neighborID = e.From
			}

			if visited[neighborID] {
				continue
			}
			visited[neighborID] = true

			neighborNode := qe.graph.GetNode(neighborID)
			if neighborNode == nil {
				continue
			}
			if !matchesEntityType(neighborNode) {
				continue
			}

			result.Nodes[neighborID] = neighborNode

			if len(result.Nodes) >= limit {
				break
			}

			// Track max depth reached.
			nextDepth := entry.depth + 1
			if nextDepth > result.Depth {
				result.Depth = nextDepth
			}

			queue = append(queue, bfsEntry{nodeID: neighborID, depth: nextDepth})
		}
	}

	return result, nil
}

// getFeaturePath returns the full feature path for a node.
// Hierarchy nodes (area/category/subcategory) already store their full path in Feature.
// File nodes use incoming feature-parent links, symbol nodes inherit file path.
func (qe *QueryEngine) getFeaturePath(nodeID string) string {
	return qe.getFeaturePathWithDepth(nodeID, 0)
}

// getFeaturePathWithDepth returns the full feature path for a node with depth limiting.
func (qe *QueryEngine) getFeaturePathWithDepth(nodeID string, depth int) string {
	if depth > 4 {
		return ""
	}

	node := qe.graph.GetNode(nodeID)
	if node == nil {
		return ""
	}

	// Hierarchy nodes already store the full path in Feature
	if node.Kind == KindArea || node.Kind == KindCategory || node.Kind == KindSubcategory {
		return node.Feature
	}

	if node.Kind == KindFile {
		for _, e := range qe.graph.GetIncoming(nodeID) {
			if e.Type != EdgeFeatureParent {
				continue
			}
			parent := qe.graph.GetNode(e.From)
			if parent != nil {
				return parent.Feature
			}
		}
		return ""
	}

	if node.Kind == KindSymbol {
		for _, e := range qe.graph.GetIncoming(nodeID) {
			if e.Type != EdgeContains {
				continue
			}
			fileNode := qe.graph.GetNode(e.From)
			if fileNode != nil && fileNode.Kind == KindFile {
				return qe.getFeaturePathWithDepth(fileNode.ID, depth+1)
			}
		}
		return ""
	}

	if node.Kind == KindChunk {
		for _, e := range qe.graph.GetIncoming(nodeID) {
			if e.Type != EdgeMapsToChunk {
				continue
			}
			symNode := qe.graph.GetNode(e.From)
			if symNode != nil && symNode.Kind == KindSymbol {
				return qe.getFeaturePathWithDepth(symNode.ID, depth+1)
			}
		}
	}

	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func matchesAnyScope(featurePath string, scopes []string) bool {
	if len(scopes) == 0 {
		return true
	}
	normalized := strings.ToLower(featurePath)
	for _, scope := range scopes {
		scope = strings.TrimSpace(strings.ToLower(scope))
		if scope == "" {
			continue
		}
		if strings.HasPrefix(normalized, scope) {
			return true
		}
	}
	return false
}

func matchesFilePathPattern(node *Node, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "**/*" {
		return true
	}
	path := strings.TrimSpace(node.Path)
	if path == "" {
		return false
	}
	path = filepath.ToSlash(path)
	pattern = filepath.ToSlash(pattern)

	if ok, err := filepath.Match(pattern, path); err == nil && ok {
		return true
	}
	// Fallback for broad patterns that filepath.Match can't represent well (e.g. **/*.go).
	pattern = strings.TrimPrefix(pattern, "**/")
	pattern = strings.Trim(pattern, "*")
	if pattern == "" {
		return true
	}
	return strings.Contains(path, pattern)
}

func readNodeCodePreview(node *Node) string {
	if node == nil || strings.TrimSpace(node.Path) == "" {
		return ""
	}
	info, err := os.Stat(node.Path)
	if err != nil {
		return ""
	}
	if info.Size() > 1<<20 { // 1MB limit
		return ""
	}
	content, err := os.ReadFile(node.Path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return ""
	}

	start := node.StartLine
	end := node.EndLine
	if start <= 0 {
		start = 1
	}
	if end < start {
		// For file nodes without explicit range, return a small prefix.
		end = min(start+19, len(lines))
	}

	startIdx := start - 1
	endIdx := end
	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx > len(lines) {
		endIdx = len(lines)
	}
	if startIdx >= endIdx {
		return ""
	}
	return strings.Join(lines[startIdx:endIdx], "\n")
}

func (qe *QueryEngine) resolveFetchNodeIDs(req FetchNodeRequest) []string {
	ordered := make([]string, 0)
	seen := make(map[string]struct{})

	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		if qe.graph.GetNode(id) == nil {
			return
		}
		seen[id] = struct{}{}
		ordered = append(ordered, id)
	}

	if req.NodeID != "" {
		add(req.NodeID)
	}

	for _, entity := range req.CodeEntities {
		entity = strings.TrimSpace(entity)
		if entity == "" {
			continue
		}
		// 1) direct node id
		add(entity)

		// 2) file path
		add(MakeNodeID(KindFile, entity))

		// 3) symbol name
		for _, n := range qe.graph.GetNodesByKind(KindSymbol) {
			if n.SymbolName == entity {
				add(n.ID)
			}
		}
	}

	for _, feature := range req.FeatureEntities {
		feature = strings.TrimSpace(feature)
		if feature == "" {
			continue
		}
		for _, kind := range []NodeKind{KindArea, KindCategory, KindSubcategory} {
			for _, n := range qe.graph.GetNodesByKind(kind) {
				if n.Feature == feature {
					add(n.ID)
				}
			}
		}
	}

	return ordered
}

func (qe *QueryEngine) resolveExploreStartNodeID(codeEntities, featureEntities []string) string {
	ids := qe.resolveFetchNodeIDs(FetchNodeRequest{
		CodeEntities:    codeEntities,
		FeatureEntities: featureEntities,
	})
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func buildEntityTypeMatcher(filter string) func(*Node) bool {
	filter = strings.TrimSpace(strings.ToLower(filter))
	if filter == "" {
		return func(*Node) bool { return true }
	}

	switch filter {
	case "directory":
		return func(n *Node) bool {
			return n != nil && (n.Kind == KindArea || n.Kind == KindCategory || n.Kind == KindSubcategory)
		}
	case "file":
		return func(n *Node) bool {
			return n != nil && n.Kind == KindFile
		}
	case "class", "function", "method":
		return func(n *Node) bool {
			return n != nil && n.Kind == KindSymbol
		}
	default:
		return func(*Node) bool { return true }
	}
}

// findParentID finds the hierarchy parent of a node by looking at outgoing
// incoming EdgeFeatureParent and EdgeContains edges.
func findParentID(g *Graph, nodeID string) string {
	// Hierarchy/file parent links.
	for _, e := range g.GetIncoming(nodeID) {
		if e.Type == EdgeFeatureParent {
			return e.From
		}
	}
	// Symbol/file containment parent links.
	for _, e := range g.GetIncoming(nodeID) {
		if e.Type == EdgeContains {
			return e.From
		}
	}
	return ""
}

// scoreMatch computes Jaccard similarity between query words and the combined
// word set from node features and SymbolName.
func scoreMatch(queryWords []string, node *Node) float64 {
	// Build the node word set from primary/atomic features and symbol name.
	nodeWordSet := make(map[string]bool)
	for _, w := range normalizeWords(node.Feature) {
		nodeWordSet[w] = true
	}
	for _, feature := range node.Features {
		for _, w := range normalizeWords(feature) {
			nodeWordSet[w] = true
		}
	}
	for _, w := range normalizeWords(node.SymbolName) {
		nodeWordSet[w] = true
	}

	if len(nodeWordSet) == 0 {
		return 0
	}

	querySet := make(map[string]bool, len(queryWords))
	for _, w := range queryWords {
		querySet[w] = true
	}

	// Jaccard = |intersection| / |union|.
	union := make(map[string]bool)
	for w := range querySet {
		union[w] = true
	}
	for w := range nodeWordSet {
		union[w] = true
	}

	intersectionCount := 0
	for w := range querySet {
		if nodeWordSet[w] {
			intersectionCount++
		}
	}

	if len(union) == 0 {
		return 0
	}

	return float64(intersectionCount) / float64(len(union))
}

// normalizeWords splits a string on common separators and lowercases each
// word. Suitable for feature labels ("handle-request"), symbol names
// ("HandleRequest"), and natural language queries.
func normalizeWords(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ToLower(s)

	// Split on common delimiters: dash, underscore, slash, space, dot, @.
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-' || r == '_' || r == '/' || r == ' ' || r == '.' || r == '@'
	})

	// Deduplicate while preserving order.
	seen := make(map[string]bool, len(parts))
	var result []string
	for _, p := range parts {
		if p != "" && !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	return result
}
