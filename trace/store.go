package trace

import (
	"context"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// GOBSymbolStore implements SymbolStore using GOB encoding.
type GOBSymbolStore struct {
	indexPath         string
	index             *SymbolIndex
	fileIndex         map[string]bool
	fileContentHashes map[string]string
	mu                sync.RWMutex
}

type gobSymbolData struct {
	Index             SymbolIndex
	FileIndex         map[string]bool
	FileContentHashes map[string]string
}

// NewGOBSymbolStore creates a new GOB-based symbol store.
func NewGOBSymbolStore(indexPath string) *GOBSymbolStore {
	return &GOBSymbolStore{
		indexPath: indexPath,
		index: &SymbolIndex{
			Symbols:    make(map[string][]Symbol),
			References: make(map[string][]Reference),
			CallGraph:  []CallEdge{},
			Version:    1,
		},
		fileIndex:         make(map[string]bool),
		fileContentHashes: make(map[string]string),
	}
}

// Load reads the index from storage.
func (s *GOBSymbolStore) Load(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to open symbol index: %w", err)
	}
	defer file.Close()

	var data gobSymbolData
	if err := gob.NewDecoder(file).Decode(&data); err != nil {
		return fmt.Errorf("failed to decode symbol index: %w", err)
	}

	s.index = &data.Index
	s.fileIndex = data.FileIndex
	s.fileContentHashes = data.FileContentHashes

	if s.index.Symbols == nil {
		s.index.Symbols = make(map[string][]Symbol)
	}
	if s.index.References == nil {
		s.index.References = make(map[string][]Reference)
	}
	if s.index.CallGraph == nil {
		s.index.CallGraph = []CallEdge{}
	}
	if s.fileIndex == nil {
		s.fileIndex = make(map[string]bool)
	}
	if s.fileContentHashes == nil {
		s.fileContentHashes = make(map[string]string)
	}

	return nil
}

// Persist writes the index to storage.
func (s *GOBSymbolStore) Persist(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := ensureParentDir(s.indexPath); err != nil {
		return fmt.Errorf("failed to prepare symbol index directory: %w", err)
	}

	file, err := os.Create(s.indexPath)
	if err != nil {
		return fmt.Errorf("failed to create symbol index file: %w", err)
	}
	defer file.Close()

	s.index.UpdatedAt = time.Now()
	data := gobSymbolData{
		Index:             *s.index,
		FileIndex:         s.fileIndex,
		FileContentHashes: s.fileContentHashes,
	}

	if err := gob.NewEncoder(file).Encode(data); err != nil {
		return fmt.Errorf("failed to encode symbol index: %w", err)
	}

	return nil
}

func ensureParentDir(filePath string) error {
	dir := filepath.Dir(filePath)
	return os.MkdirAll(dir, 0755)
}

// SaveFile persists symbols and references for a file.
func (s *GOBSymbolStore) SaveFile(ctx context.Context, filePath string, symbols []Symbol, refs []Reference) error {
	return s.SaveFileWithContentHash(ctx, filePath, "", symbols, refs)
}

// SaveFileWithContentHash persists symbols/references for a file and tracks
// the current file content hash for future cache checks.
func (s *GOBSymbolStore) SaveFileWithContentHash(ctx context.Context, filePath string, contentHash string, symbols []Symbol, refs []Reference) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove old entries for this file first
	s.deleteFileUnlocked(filePath)

	// Add new symbols
	for _, sym := range symbols {
		s.index.Symbols[sym.Name] = append(s.index.Symbols[sym.Name], sym)
	}

	// Add new references
	for _, ref := range refs {
		s.index.References[ref.SymbolName] = append(s.index.References[ref.SymbolName], ref)
	}

	// Build call graph edges
	for _, ref := range refs {
		if ref.CallerName != "" && ref.CallerName != "<top-level>" {
			s.index.CallGraph = append(s.index.CallGraph, CallEdge{
				Caller:   ref.CallerName,
				Callee:   ref.SymbolName,
				File:     ref.File,
				Line:     ref.Line,
				CallType: "direct",
			})
		}
	}

	s.fileIndex[filePath] = true
	if contentHash != "" {
		s.fileContentHashes[filePath] = contentHash
	} else {
		delete(s.fileContentHashes, filePath)
	}
	return nil
}

// DeleteFile removes all symbols and references for a file.
func (s *GOBSymbolStore) DeleteFile(ctx context.Context, filePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteFileUnlocked(filePath)
	return nil
}

func (s *GOBSymbolStore) deleteFileUnlocked(filePath string) {
	// Remove symbols from this file
	for name, symbols := range s.index.Symbols {
		filtered := make([]Symbol, 0, len(symbols))
		for _, sym := range symbols {
			if sym.File != filePath {
				filtered = append(filtered, sym)
			}
		}
		if len(filtered) == 0 {
			delete(s.index.Symbols, name)
		} else {
			s.index.Symbols[name] = filtered
		}
	}

	// Remove references from this file
	for name, refs := range s.index.References {
		filtered := make([]Reference, 0, len(refs))
		for _, ref := range refs {
			if ref.File != filePath {
				filtered = append(filtered, ref)
			}
		}
		if len(filtered) == 0 {
			delete(s.index.References, name)
		} else {
			s.index.References[name] = filtered
		}
	}

	// Remove call graph edges from this file
	filtered := make([]CallEdge, 0, len(s.index.CallGraph))
	for _, edge := range s.index.CallGraph {
		if edge.File != filePath {
			filtered = append(filtered, edge)
		}
	}
	s.index.CallGraph = filtered

	delete(s.fileIndex, filePath)
	delete(s.fileContentHashes, filePath)
}

// LookupSymbol finds symbol definitions by name.
func (s *GOBSymbolStore) LookupSymbol(ctx context.Context, name string) ([]Symbol, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	symbols := s.index.Symbols[name]
	if symbols == nil {
		return []Symbol{}, nil
	}
	return symbols, nil
}

// LookupCallers finds all references/callers of a symbol.
func (s *GOBSymbolStore) LookupCallers(ctx context.Context, symbolName string) ([]Reference, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	refs := s.index.References[symbolName]
	if refs == nil {
		return []Reference{}, nil
	}
	return refs, nil
}

// LookupCallees finds all symbols called by a function.
func (s *GOBSymbolStore) LookupCallees(ctx context.Context, symbolName string, file string) ([]Reference, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var callees []Reference
	seen := make(map[string]bool)

	for _, edge := range s.index.CallGraph {
		if edge.Caller == symbolName {
			key := fmt.Sprintf("%s:%d", edge.File, edge.Line)
			if seen[key] {
				continue
			}
			seen[key] = true

			// Find reference details
			if refs, ok := s.index.References[edge.Callee]; ok {
				for _, ref := range refs {
					if ref.CallerName == symbolName && ref.File == edge.File && ref.Line == edge.Line {
						callees = append(callees, ref)
						break
					}
				}
			}

			// If no reference found, create a minimal one
			if !seen[key] || len(callees) == 0 {
				callees = append(callees, Reference{
					SymbolName: edge.Callee,
					File:       edge.File,
					Line:       edge.Line,
					CallerName: symbolName,
				})
			}
		}
	}
	return callees, nil
}

// GetCallGraph builds a call graph from a starting symbol.
func (s *GOBSymbolStore) GetCallGraph(ctx context.Context, symbolName string, depth int) (*CallGraph, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	graph := &CallGraph{
		Root:  symbolName,
		Nodes: make(map[string]Symbol),
		Edges: []CallEdge{},
		Depth: depth,
	}

	// BFS to build graph up to depth
	visited := make(map[string]bool)
	type queueItem struct {
		name  string
		depth int
	}
	queue := []queueItem{{symbolName, 0}}
	edgeSeen := make(map[string]bool)

	shouldTraverse := func(name string, isRoot bool) bool {
		symbols := s.index.Symbols[name]
		if len(symbols) == 0 {
			return false
		}
		// Root is explicitly requested by user and may be ambiguous.
		if isRoot {
			return true
		}
		// Avoid exploding through name-collided symbols (e.g. Load, Init).
		return len(symbols) == 1
	}
	isDeclarationSelfEdge := func(edge CallEdge) bool {
		if edge.Caller != edge.Callee {
			return false
		}
		for _, sym := range s.index.Symbols[edge.Caller] {
			if sym.File == edge.File && sym.Line == edge.Line {
				return true
			}
		}
		return false
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current.name] || current.depth > depth {
			continue
		}
		visited[current.name] = true

		// Add node
		if symbols, ok := s.index.Symbols[current.name]; ok && len(symbols) > 0 {
			graph.Nodes[current.name] = symbols[0]
		}

		// Find edges (both callers and callees)
		for _, edge := range s.index.CallGraph {
			if edge.Caller == current.name {
				if isDeclarationSelfEdge(edge) {
					continue
				}
				edgeKey := fmt.Sprintf("%s->%s", edge.Caller, edge.Callee)
				if !edgeSeen[edgeKey] {
					graph.Edges = append(graph.Edges, edge)
					edgeSeen[edgeKey] = true
				}
				if !visited[edge.Callee] && shouldTraverse(edge.Callee, false) {
					queue = append(queue, queueItem{edge.Callee, current.depth + 1})
				}
			}
			if current.depth == 0 && edge.Callee == current.name {
				if isDeclarationSelfEdge(edge) {
					continue
				}
				edgeKey := fmt.Sprintf("%s->%s", edge.Caller, edge.Callee)
				if !edgeSeen[edgeKey] {
					graph.Edges = append(graph.Edges, edge)
					edgeSeen[edgeKey] = true
				}
			}
		}
	}

	return graph, nil
}

// GetSymbolsForFile returns all symbols defined in a specific file.
func (s *GOBSymbolStore) GetSymbolsForFile(ctx context.Context, filePath string) ([]Symbol, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Symbol
	for _, symbols := range s.index.Symbols {
		for _, sym := range symbols {
			if sym.File == filePath {
				result = append(result, sym)
			}
		}
	}
	return result, nil
}

// GetCallEdges returns all call graph edges.
func (s *GOBSymbolStore) GetCallEdges(ctx context.Context) ([]CallEdge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	edges := make([]CallEdge, len(s.index.CallGraph))
	copy(edges, s.index.CallGraph)
	return edges, nil
}

// Close shuts down the store.
func (s *GOBSymbolStore) Close() error {
	return s.Persist(context.Background())
}

// GetStats returns statistics about the symbol index.
func (s *GOBSymbolStore) GetStats(ctx context.Context) (*SymbolStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalSymbols := 0
	for _, syms := range s.index.Symbols {
		totalSymbols += len(syms)
	}

	totalRefs := 0
	for _, refs := range s.index.References {
		totalRefs += len(refs)
	}

	var size int64
	if info, err := os.Stat(s.indexPath); err == nil {
		size = info.Size()
	}

	return &SymbolStats{
		TotalSymbols:    totalSymbols,
		TotalReferences: totalRefs,
		TotalFiles:      len(s.fileIndex),
		IndexSize:       size,
		LastUpdated:     s.index.UpdatedAt,
	}, nil
}

// IsFileIndexed checks if a file has been indexed.
func (s *GOBSymbolStore) IsFileIndexed(filePath string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fileIndex[filePath]
}

// GetFileContentHash returns the stored content hash for a file when available.
func (s *GOBSymbolStore) GetFileContentHash(filePath string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	hash, ok := s.fileContentHashes[filePath]
	return hash, ok
}
