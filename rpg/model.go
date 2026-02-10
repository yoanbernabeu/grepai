package rpg

import (
	"fmt"
	"strings"
	"time"
)

// NodeKind represents the type of node in the RPG graph.
type NodeKind string

const (
	KindArea        NodeKind = "area"        // functional area (top level)
	KindCategory    NodeKind = "category"    // category within an area
	KindSubcategory NodeKind = "subcategory" // subcategory within a category
	KindFile        NodeKind = "file"        // source file
	KindSymbol      NodeKind = "symbol"      // function/method/class/type
	KindChunk       NodeKind = "chunk"       // vector chunk reference
)

// EdgeType represents the type of relationship between nodes.
type EdgeType string

const (
	EdgeFeatureParent EdgeType = "feature_parent" // child -> parent in hierarchy
	EdgeContains      EdgeType = "contains"       // file contains symbol
	EdgeInvokes       EdgeType = "invokes"        // symbol calls symbol
	EdgeImports       EdgeType = "imports"        // file imports file/package
	EdgeMapsToChunk   EdgeType = "maps_to_chunk"  // symbol maps to vector chunk
	EdgeSemanticSim   EdgeType = "semantic_sim"   // symbols with similar features/co-call patterns
)

// Node represents a node in the RPG graph.
type Node struct {
	ID            string    `json:"id"`
	Kind          NodeKind  `json:"kind"`
	Feature       string    `json:"feature"`               // semantic feature label (verb-object)
	Path          string    `json:"path,omitempty"`        // file path (for file/symbol/chunk nodes)
	SymbolName    string    `json:"symbol_name,omitempty"` // symbol name (for symbol nodes)
	Receiver      string    `json:"receiver,omitempty"`    // Go receiver type
	Language      string    `json:"language,omitempty"`    // programming language
	StartLine     int       `json:"start_line,omitempty"`
	EndLine       int       `json:"end_line,omitempty"`
	Signature     string    `json:"signature,omitempty"`      // function signature
	ChunkID       string    `json:"chunk_id,omitempty"`       // linked vector chunk ID
	SemanticLabel string    `json:"semantic_label,omitempty"` // enriched label with semantic context
	UpdatedAt     time.Time `json:"updated_at"`
}

// Edge represents a directed edge in the RPG graph.
type Edge struct {
	From      string    `json:"from"`             // source node ID
	To        string    `json:"to"`               // target node ID
	Type      EdgeType  `json:"type"`             // relationship type
	Weight    float64   `json:"weight,omitempty"` // edge weight/confidence
	UpdatedAt time.Time `json:"updated_at"`
}

// Graph is the in-memory RPG graph with fast lookup indexes.
type Graph struct {
	Nodes map[string]*Node `json:"nodes"`
	Edges []*Edge          `json:"edges"`

	// Indexes for fast lookup (not serialized)
	byKind        map[NodeKind][]*Node
	byFile        map[string][]*Node // path -> nodes in that file
	byFeaturePath map[string]*Node   // feature path -> hierarchy node
	adjForward    map[string][]*Edge // from -> outgoing edges
	adjReverse    map[string][]*Edge // to -> incoming edges
}

// GraphStats holds graph statistics.
type GraphStats struct {
	TotalNodes  int              `json:"total_nodes"`
	TotalEdges  int              `json:"total_edges"`
	NodesByKind map[NodeKind]int `json:"nodes_by_kind"`
	EdgesByType map[EdgeType]int `json:"edges_by_type"`
	LastUpdated time.Time        `json:"last_updated"`
}

// NewGraph creates an empty graph with initialized indexes.
func NewGraph() *Graph {
	return &Graph{
		Nodes:         make(map[string]*Node),
		Edges:         make([]*Edge, 0),
		byKind:        make(map[NodeKind][]*Node),
		byFile:        make(map[string][]*Node),
		byFeaturePath: make(map[string]*Node),
		adjForward:    make(map[string][]*Edge),
		adjReverse:    make(map[string][]*Edge),
	}
}

// AddNode adds or updates a node and maintains indexes.
// If a node with the same ID already exists, old index entries are removed first
// to prevent stale reference accumulation.
func (g *Graph) AddNode(n *Node) {
	// If node already exists, remove old index entries first
	if old, exists := g.Nodes[n.ID]; exists {
		if nodes, ok := g.byKind[old.Kind]; ok {
			for i, node := range nodes {
				if node.ID == n.ID {
					g.byKind[old.Kind] = append(nodes[:i], nodes[i+1:]...)
					break
				}
			}
		}
		if old.Path != "" {
			if nodes, ok := g.byFile[old.Path]; ok {
				for i, node := range nodes {
					if node.ID == n.ID {
						g.byFile[old.Path] = append(nodes[:i], nodes[i+1:]...)
						break
					}
				}
			}
		}
		if old.Kind == KindArea || old.Kind == KindCategory || old.Kind == KindSubcategory {
			delete(g.byFeaturePath, old.Feature)
		}
	}

	g.Nodes[n.ID] = n

	// Update byKind index
	g.byKind[n.Kind] = append(g.byKind[n.Kind], n)

	// Update byFile index for nodes that have a path
	if n.Path != "" {
		g.byFile[n.Path] = append(g.byFile[n.Path], n)
	}

	// Update byFeaturePath index for hierarchy nodes
	if n.Kind == KindArea || n.Kind == KindCategory || n.Kind == KindSubcategory {
		g.byFeaturePath[n.Feature] = n
	}
}

// RemoveNode removes a node and all its edges, updating indexes.
func (g *Graph) RemoveNode(id string) {
	n, ok := g.Nodes[id]
	if !ok {
		return
	}

	// Remove from byKind index
	if nodes, exists := g.byKind[n.Kind]; exists {
		filtered := make([]*Node, 0, len(nodes))
		for _, node := range nodes {
			if node.ID != id {
				filtered = append(filtered, node)
			}
		}
		if len(filtered) == 0 {
			delete(g.byKind, n.Kind)
		} else {
			g.byKind[n.Kind] = filtered
		}
	}

	// Remove from byFile index
	if n.Path != "" {
		if nodes, exists := g.byFile[n.Path]; exists {
			filtered := make([]*Node, 0, len(nodes))
			for _, node := range nodes {
				if node.ID != id {
					filtered = append(filtered, node)
				}
			}
			if len(filtered) == 0 {
				delete(g.byFile, n.Path)
			} else {
				g.byFile[n.Path] = filtered
			}
		}
	}

	// Remove from byFeaturePath index
	if n.Kind == KindArea || n.Kind == KindCategory || n.Kind == KindSubcategory {
		delete(g.byFeaturePath, n.Feature)
	}

	// Remove all edges referencing this node
	filtered := make([]*Edge, 0, len(g.Edges))
	for _, e := range g.Edges {
		if e.From != id && e.To != id {
			filtered = append(filtered, e)
		}
	}
	g.Edges = filtered

	// Remove from adjacency indexes
	delete(g.adjForward, id)
	delete(g.adjReverse, id)

	// Clean up edges referencing this node from other adjacency lists
	for nodeID, edges := range g.adjForward {
		cleaned := make([]*Edge, 0, len(edges))
		for _, e := range edges {
			if e.To != id {
				cleaned = append(cleaned, e)
			}
		}
		if len(cleaned) == 0 {
			delete(g.adjForward, nodeID)
		} else {
			g.adjForward[nodeID] = cleaned
		}
	}
	for nodeID, edges := range g.adjReverse {
		cleaned := make([]*Edge, 0, len(edges))
		for _, e := range edges {
			if e.From != id {
				cleaned = append(cleaned, e)
			}
		}
		if len(cleaned) == 0 {
			delete(g.adjReverse, nodeID)
		} else {
			g.adjReverse[nodeID] = cleaned
		}
	}

	// Remove the node itself
	delete(g.Nodes, id)
}

// AddEdge adds an edge and updates adjacency indexes.
func (g *Graph) AddEdge(e *Edge) {
	g.Edges = append(g.Edges, e)
	g.adjForward[e.From] = append(g.adjForward[e.From], e)
	g.adjReverse[e.To] = append(g.adjReverse[e.To], e)
}

// RemoveEdgesBetween removes all edges between two nodes.
func (g *Graph) RemoveEdgesBetween(from, to string) {
	// Remove from main edge list
	filtered := make([]*Edge, 0, len(g.Edges))
	for _, e := range g.Edges {
		if !(e.From == from && e.To == to) {
			filtered = append(filtered, e)
		}
	}
	g.Edges = filtered

	// Remove from forward adjacency
	if edges, ok := g.adjForward[from]; ok {
		cleaned := make([]*Edge, 0, len(edges))
		for _, e := range edges {
			if e.To != to {
				cleaned = append(cleaned, e)
			}
		}
		if len(cleaned) == 0 {
			delete(g.adjForward, from)
		} else {
			g.adjForward[from] = cleaned
		}
	}

	// Remove from reverse adjacency
	if edges, ok := g.adjReverse[to]; ok {
		cleaned := make([]*Edge, 0, len(edges))
		for _, e := range edges {
			if e.From != from {
				cleaned = append(cleaned, e)
			}
		}
		if len(cleaned) == 0 {
			delete(g.adjReverse, to)
		} else {
			g.adjReverse[to] = cleaned
		}
	}
}

// RemoveEdgesIf removes edges that match the predicate and rebuilds edge indexes.
func (g *Graph) RemoveEdgesIf(predicate func(*Edge) bool) {
	if predicate == nil {
		return
	}

	filtered := make([]*Edge, 0, len(g.Edges))
	removed := false
	for _, e := range g.Edges {
		if predicate(e) {
			removed = true
			continue
		}
		filtered = append(filtered, e)
	}

	if !removed {
		return
	}

	g.Edges = filtered
	g.RebuildIndexes()
}

// NodePath returns the file path for a node ID when present.
func (g *Graph) NodePath(id string) (string, bool) {
	n := g.GetNode(id)
	if n == nil || n.Path == "" {
		return "", false
	}
	return n.Path, true
}

// GetNode returns a node by ID.
func (g *Graph) GetNode(id string) *Node {
	return g.Nodes[id]
}

// GetNodesByKind returns all nodes of a given kind.
func (g *Graph) GetNodesByKind(kind NodeKind) []*Node {
	return g.byKind[kind]
}

// GetNodesByFile returns all nodes for a given file path.
func (g *Graph) GetNodesByFile(path string) []*Node {
	return g.byFile[path]
}

// GetOutgoing returns all outgoing edges from a node.
func (g *Graph) GetOutgoing(nodeID string) []*Edge {
	return g.adjForward[nodeID]
}

// GetIncoming returns all incoming edges to a node.
func (g *Graph) GetIncoming(nodeID string) []*Edge {
	return g.adjReverse[nodeID]
}

// GetNeighbors returns neighbor node IDs in a given direction ("forward", "reverse", "both").
func (g *Graph) GetNeighbors(nodeID string, direction string) []string {
	seen := make(map[string]bool)
	var result []string

	if direction == "forward" || direction == "both" {
		for _, e := range g.adjForward[nodeID] {
			if !seen[e.To] {
				seen[e.To] = true
				result = append(result, e.To)
			}
		}
	}

	if direction == "reverse" || direction == "both" {
		for _, e := range g.adjReverse[nodeID] {
			if !seen[e.From] {
				seen[e.From] = true
				result = append(result, e.From)
			}
		}
	}

	return result
}

// RebuildIndexes rebuilds all in-memory indexes from Nodes and Edges.
// Called after deserialization.
func (g *Graph) RebuildIndexes() {
	g.byKind = make(map[NodeKind][]*Node)
	g.byFile = make(map[string][]*Node)
	g.byFeaturePath = make(map[string]*Node)
	g.adjForward = make(map[string][]*Edge)
	g.adjReverse = make(map[string][]*Edge)

	for _, n := range g.Nodes {
		g.byKind[n.Kind] = append(g.byKind[n.Kind], n)

		if n.Path != "" {
			g.byFile[n.Path] = append(g.byFile[n.Path], n)
		}

		if n.Kind == KindArea || n.Kind == KindCategory || n.Kind == KindSubcategory {
			g.byFeaturePath[n.Feature] = n
		}
	}

	for _, e := range g.Edges {
		g.adjForward[e.From] = append(g.adjForward[e.From], e)
		g.adjReverse[e.To] = append(g.adjReverse[e.To], e)
	}
}

// Stats returns basic graph statistics.
func (g *Graph) Stats() GraphStats {
	nodesByKind := make(map[NodeKind]int)
	for _, n := range g.Nodes {
		nodesByKind[n.Kind]++
	}

	edgesByType := make(map[EdgeType]int)
	for _, e := range g.Edges {
		edgesByType[e.Type]++
	}

	var lastUpdated time.Time
	for _, n := range g.Nodes {
		if n.UpdatedAt.After(lastUpdated) {
			lastUpdated = n.UpdatedAt
		}
	}
	for _, e := range g.Edges {
		if e.UpdatedAt.After(lastUpdated) {
			lastUpdated = e.UpdatedAt
		}
	}

	return GraphStats{
		TotalNodes:  len(g.Nodes),
		TotalEdges:  len(g.Edges),
		NodesByKind: nodesByKind,
		EdgesByType: edgesByType,
		LastUpdated: lastUpdated,
	}
}

// MakeNodeID creates a deterministic node ID.
// For symbols: "sym:<path>:<receiver>.<name>" or "sym:<path>:<name>"
// For files: "file:<path>"
// For hierarchy: "area:<name>", "cat:<parent>/<name>", "subcat:<parent>/<name>"
// For chunks: "chunk:<chunkID>"
func MakeNodeID(kind NodeKind, parts ...string) string {
	switch kind {
	case KindSymbol:
		// parts: path, name OR path, receiver, name
		if len(parts) == 3 && parts[1] != "" {
			return fmt.Sprintf("sym:%s:%s.%s", parts[0], parts[1], parts[2])
		}
		if len(parts) >= 2 {
			return fmt.Sprintf("sym:%s:%s", parts[0], parts[1])
		}
		return fmt.Sprintf("sym:%s", strings.Join(parts, ":"))
	case KindFile:
		if len(parts) >= 1 {
			return fmt.Sprintf("file:%s", parts[0])
		}
		return "file:"
	case KindArea:
		if len(parts) >= 1 {
			return fmt.Sprintf("area:%s", parts[0])
		}
		return "area:"
	case KindCategory:
		if len(parts) >= 2 {
			return fmt.Sprintf("cat:%s/%s", parts[0], parts[1])
		}
		if len(parts) == 1 {
			return fmt.Sprintf("cat:%s", parts[0])
		}
		return "cat:"
	case KindSubcategory:
		if len(parts) >= 2 {
			return fmt.Sprintf("subcat:%s/%s", parts[0], parts[1])
		}
		if len(parts) == 1 {
			return fmt.Sprintf("subcat:%s", parts[0])
		}
		return "subcat:"
	case KindChunk:
		if len(parts) >= 1 {
			return fmt.Sprintf("chunk:%s", parts[0])
		}
		return "chunk:"
	default:
		return strings.Join(parts, ":")
	}
}
