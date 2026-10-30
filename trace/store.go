package trace

import (
	"context"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yoanbernabeu/grepai/internal/fileutil"
)

// GOBSymbolStore implements SymbolStore using GOB encoding.
type GOBSymbolStore struct {
	indexPath                 string
	lockPath                  string
	index                     *SymbolIndex
	fileIndex                 map[string]bool
	fileContentHashes         map[string]string
	fileExtractorVersions     map[string]string
	constructorPersistPending bool
	mutationGeneration        uint64
	mu                        sync.RWMutex
}

type gobSymbolData struct {
	Index             SymbolIndex
	FileIndex         map[string]bool
	FileContentHashes map[string]string
	// FileExtractorVersions records the extractor.Version() value used
	// when each file's symbols were extracted. Pairing this with the
	// content hash lets dedup invalidate cached extractions when a new
	// release ships better symbol coverage even though the source files
	// themselves haven't changed. Older gob files without this field
	// decode with a nil map; the dedup check then treats every file as
	// "extractor version unknown" and re-extracts once, which is the
	// correct behavior for a one-time upgrade.
	FileExtractorVersions map[string]string
}

// NewGOBSymbolStore creates a new GOB-based symbol store.
func NewGOBSymbolStore(indexPath string) *GOBSymbolStore {
	return &GOBSymbolStore{
		indexPath: indexPath,
		lockPath:  indexPath + ".lock",
		index: &SymbolIndex{
			Symbols:    make(map[string][]Symbol),
			References: make(map[string][]Reference),
			CallGraph:  []CallEdge{},
			Version:    1,
		},
		fileIndex:                 make(map[string]bool),
		fileContentHashes:         make(map[string]string),
		fileExtractorVersions:     make(map[string]string),
		constructorPersistPending: true,
	}
}

// Load reads the index from storage.
func (s *GOBSymbolStore) Load(ctx context.Context) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	loaded := false
	defer func() {
		if err == nil {
			s.constructorPersistPending = false
			if loaded {
				s.mutationGeneration = 0
			}
		}
	}()

	lockFile, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		loaded, err = s.loadUnlocked()
		return err
	}
	defer lockFile.Close()
	if err := fileutil.FlockShared(lockFile, false); err != nil {
		loaded, err = s.loadUnlocked()
		return err
	}
	defer func() {
		_ = fileutil.Funlock(lockFile)
	}()

	loaded, err = s.loadUnlocked()
	return err
}

func (s *GOBSymbolStore) loadUnlocked() (bool, error) {
	file, err := os.Open(s.indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to open symbol index: %w", err)
	}
	defer file.Close()

	var data gobSymbolData
	if err := gob.NewDecoder(file).Decode(&data); err != nil {
		return false, fmt.Errorf("failed to decode symbol index: %w", err)
	}

	s.index = &data.Index
	s.fileIndex = data.FileIndex
	s.fileContentHashes = data.FileContentHashes
	s.fileExtractorVersions = data.FileExtractorVersions

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
	if s.fileExtractorVersions == nil {
		s.fileExtractorVersions = make(map[string]string)
	}

	return true, nil
}

// Persist writes the index to storage.
func (s *GOBSymbolStore) Persist(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.constructorPersistPending && s.mutationGeneration == 0 {
		return nil
	}

	if err := fileutil.EnsureParentDir(s.indexPath); err != nil {
		return fmt.Errorf("failed to prepare symbol index directory: %w", err)
	}

	lockFile, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return s.persistUnlocked()
	}
	defer lockFile.Close()
	if err := fileutil.FlockExclusive(lockFile, false); err != nil {
		return s.persistUnlocked()
	}
	defer func() {
		_ = fileutil.Funlock(lockFile)
	}()

	return s.persistUnlocked()
}

func (s *GOBSymbolStore) persistUnlocked() error {
	s.index.UpdatedAt = time.Now()
	data := gobSymbolData{
		Index:                 *s.index,
		FileIndex:             s.fileIndex,
		FileContentHashes:     s.fileContentHashes,
		FileExtractorVersions: s.fileExtractorVersions,
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(s.indexPath), filepath.Base(s.indexPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create symbol index temp file: %w", err)
	}

	tmpPath := tmpFile.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := gob.NewEncoder(tmpFile).Encode(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to encode symbol index: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to sync symbol index temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close symbol index temp file: %w", err)
	}
	if err := fileutil.ReplaceFileAtomically(tmpPath, s.indexPath); err != nil {
		return fmt.Errorf("failed to replace symbol index file: %w", err)
	}
	cleanupTemp = false
	s.constructorPersistPending = false
	s.mutationGeneration = 0

	return nil
}

// SaveFile persists symbols and references for a file.
func (s *GOBSymbolStore) SaveFile(ctx context.Context, filePath string, symbols []Symbol, refs []Reference) error {
	return s.SaveFileWithContentHash(ctx, filePath, "", symbols, refs)
}

// SaveFileWithContentHash persists symbols/references for a file and tracks
// the current file content hash for future cache checks. The extractor
// version stays whatever was already stored (or empty) — call
// SaveFileWithSignature to update both fingerprints at once.
func (s *GOBSymbolStore) SaveFileWithContentHash(ctx context.Context, filePath string, contentHash string, symbols []Symbol, refs []Reference) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mutationGeneration++
	s.saveFileWithContentHashUnlocked(filePath, contentHash, symbols, refs)
	return nil
}

func (s *GOBSymbolStore) saveFileWithContentHashUnlocked(filePath string, contentHash string, symbols []Symbol, refs []Reference) {
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
}

// SaveFileWithSignature persists symbols/references for a file and
// records BOTH the content hash and the extractor version that produced
// them. Dedup callers can then ask GetFileExtractorVersion alongside
// GetFileContentHash and re-extract whenever either has drifted.
func (s *GOBSymbolStore) SaveFileWithSignature(ctx context.Context, filePath string, contentHash, extractorVersion string, symbols []Symbol, refs []Reference) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mutationGeneration++
	s.saveFileWithContentHashUnlocked(filePath, contentHash, symbols, refs)
	if extractorVersion != "" {
		s.fileExtractorVersions[filePath] = extractorVersion
	} else {
		delete(s.fileExtractorVersions, filePath)
	}
	return nil
}

// DeleteFile removes all symbols and references for a file.
func (s *GOBSymbolStore) DeleteFile(ctx context.Context, filePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := s.deleteFileUnlocked(filePath)
	if _, ok := s.fileExtractorVersions[filePath]; ok {
		delete(s.fileExtractorVersions, filePath)
		removed = true
	}
	if removed {
		s.mutationGeneration++
	}
	return nil
}

func (s *GOBSymbolStore) deleteFileUnlocked(filePath string) bool {
	removed := false
	// Remove symbols from this file
	for name, symbols := range s.index.Symbols {
		filtered := make([]Symbol, 0, len(symbols))
		for _, sym := range symbols {
			if sym.File != filePath {
				filtered = append(filtered, sym)
			} else {
				removed = true
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
			} else {
				removed = true
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
		} else {
			removed = true
		}
	}
	s.index.CallGraph = filtered

	if _, ok := s.fileIndex[filePath]; ok {
		delete(s.fileIndex, filePath)
		removed = true
	}
	if _, ok := s.fileContentHashes[filePath]; ok {
		delete(s.fileContentHashes, filePath)
		removed = true
	}
	return removed
}

// LookupSymbol finds symbol definitions by name, ordered by file then line so
// that callers taking the first one get the same definition whatever the
// order in which files were indexed.
func (s *GOBSymbolStore) LookupSymbol(ctx context.Context, name string) ([]Symbol, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	symbols := s.index.Symbols[name]
	if symbols == nil {
		return []Symbol{}, nil
	}
	result := append([]Symbol(nil), symbols...)
	sort.SliceStable(result, func(i, j int) bool { return symbolLess(result[i], result[j]) })
	return result, nil
}

// LookupCallers finds all references/callers of a symbol.
func (s *GOBSymbolStore) LookupCallers(ctx context.Context, symbolName string) ([]Reference, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	refs := s.index.References[symbolName]
	if refs == nil {
		return []Reference{}, nil
	}
	return filterByReferenceKinds(refs, RefKindCall, ""), nil
}

// LookupCallees finds all symbols called by a function. Callees of every
// same-name definition are merged.
func (s *GOBSymbolStore) LookupCallees(ctx context.Context, symbolName string, file string) ([]Reference, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var callees []Reference
	type calleeKey struct {
		callee, file string
		line         int
	}
	// Two calls on the same line (e.g. Save(Load())) are distinct callees.
	seen := make(map[calleeKey]bool)
	callRefs := s.newCallRefIndexUnlocked()

	for _, edge := range s.index.CallGraph {
		if edge.Caller != symbolName {
			continue
		}
		key := calleeKey{edge.Callee, edge.File, edge.Line}
		if seen[key] {
			continue
		}
		seen[key] = true

		if ref, ok := callRefs.lookup(edge); ok && !s.isDeclarationSelfEdgeUnlocked(edge, ref.Context) {
			callees = append(callees, ref)
		}
	}
	return callees, nil
}

// hasLaterCall reports whether name, after its first whole-identifier
// occurrence in line (the declaration), appears again followed by "(" or by
// type arguments.
func hasLaterCall(line, name string) bool {
	if name == "" {
		return false
	}
	seenFirst := false
	for i := 0; ; {
		j := strings.Index(line[i:], name)
		if j < 0 {
			return false
		}
		start := i + j
		end := start + len(name)
		i = end
		if start > 0 && isIdentByte(line[start-1]) {
			continue
		}
		if end < len(line) && isIdentByte(line[end]) {
			continue
		}
		if !seenFirst {
			seenFirst = true
			continue
		}
		// "(" or explicit type arguments: Map[T](...), execute<T>(...).
		if rest := strings.TrimLeft(line[end:], " \t"); rest != "" && strings.ContainsRune("([<", rune(rest[0])) {
			return true
		}
	}
}

// isTruncatedContext reports whether getLineContext may have cut the line
// (it appends "..." and then trims spaces, so the length varies).
func isTruncatedContext(lineContext string) bool {
	return strings.HasSuffix(lineContext, "...")
}

func isIdentByte(b byte) bool {
	return isIdentPartASCII(b) || b == '$'
}

// callRefIndex finds the call reference behind call-graph edges. Edges are
// also built for property reads/writes, which are not calls. The references
// of each callee are indexed once, so popular names stay linear.
type callRefIndex struct {
	s      *GOBSymbolStore
	byEdge map[string]map[edgeRefKey]edgeRef
}

type edgeRefKey struct {
	caller, file string
	line         int
}

type edgeRef struct {
	ref    Reference
	isCall bool
}

// newCallRefIndexUnlocked returns an empty index. Caller must hold s.mu while
// using it.
func (s *GOBSymbolStore) newCallRefIndexUnlocked() *callRefIndex {
	return &callRefIndex{s: s, byEdge: make(map[string]map[edgeRefKey]edgeRef)}
}

// lookup returns the call reference of an edge; ok is false for a property
// read/write edge. An edge without any reference details gets a minimal one.
func (x *callRefIndex) lookup(edge CallEdge) (Reference, bool) {
	refs, built := x.byEdge[edge.Callee]
	if !built {
		refs = make(map[edgeRefKey]edgeRef)
		for _, ref := range x.s.index.References[edge.Callee] {
			key := edgeRefKey{ref.CallerName, ref.File, ref.Line}
			current, seen := refs[key]
			switch {
			case isCallReference(ref):
				if !seen || !current.isCall {
					refs[key] = edgeRef{ref: ref, isCall: true}
				}
			case !seen:
				refs[key] = edgeRef{}
			}
		}
		x.byEdge[edge.Callee] = refs
	}
	match, found := refs[edgeRefKey{edge.Caller, edge.File, edge.Line}]
	if !found {
		return Reference{
			SymbolName: edge.Callee,
			File:       edge.File,
			Line:       edge.Line,
			CallerName: edge.Caller,
		}, true
	}
	return match.ref, match.isCall
}

// isDeclarationSelfEdgeUnlocked reports whether an edge is a function's own
// declaration line picked up as a call to itself. lineContext is the source
// line of the reference: when the name is called again after the declaration
// (one-line delegation to a same-name method, one-line recursion) or the line
// was truncated, the edge may be a real call and is kept. An empty lineContext always treats a
// self edge on a declaration line as the declaration. Caller must hold s.mu.
func (s *GOBSymbolStore) isDeclarationSelfEdgeUnlocked(edge CallEdge, lineContext string) bool {
	if edge.Caller != edge.Callee {
		return false
	}
	if lineContext != "" && (hasLaterCall(lineContext, edge.Callee) || isTruncatedContext(lineContext)) {
		return false
	}
	for _, sym := range s.index.Symbols[edge.Caller] {
		if sym.File == edge.File && sym.Line == edge.Line {
			return true
		}
	}
	return false
}

// LookupReaders finds property/data readers for a symbol name.
func (s *GOBSymbolStore) LookupReaders(ctx context.Context, symbolName string) ([]Reference, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	refs := s.index.References[symbolName]
	if refs == nil {
		return []Reference{}, nil
	}
	return filterByReferenceKinds(refs, RefKindRead), nil
}

// LookupWriters finds property/data writers for a symbol name.
func (s *GOBSymbolStore) LookupWriters(ctx context.Context, symbolName string) ([]Reference, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	refs := s.index.References[symbolName]
	if refs == nil {
		return []Reference{}, nil
	}
	return filterByReferenceKinds(refs, RefKindWrite), nil
}

func filterByReferenceKinds(refs []Reference, kinds ...string) []Reference {
	if len(refs) == 0 {
		return []Reference{}
	}

	allowed := make(map[string]bool, len(kinds))
	for _, kind := range kinds {
		allowed[kind] = true
	}

	filtered := make([]Reference, 0, len(refs))
	for _, ref := range refs {
		if allowed[ref.Kind] {
			filtered = append(filtered, ref)
			continue
		}
		// Backward compatibility with older indices where kind wasn't persisted.
		if ref.Kind == "" && allowed[RefKindCall] {
			filtered = append(filtered, ref)
		}
	}

	return filtered
}

func isCallReference(ref Reference) bool {
	return ref.Kind == "" || ref.Kind == RefKindCall
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

	callRefs := s.newCallRefIndexUnlocked()
	// skipEdge drops property reads/writes and every self edge on a
	// declaration line (graph behavior unchanged for one-line bodies).
	skipEdge := func(edge CallEdge) bool {
		_, isCall := callRefs.lookup(edge)
		return !isCall || s.isDeclarationSelfEdgeUnlocked(edge, "")
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current.name] || current.depth > depth {
			continue
		}
		visited[current.name] = true

		// Add node
		if sym := firstSymbol(s.index.Symbols[current.name]); sym != nil {
			graph.Nodes[current.name] = *sym
		}

		// Find edges (both callers and callees)
		for _, edge := range s.index.CallGraph {
			if edge.Caller == current.name {
				if skipEdge(edge) {
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
				if skipEdge(edge) {
					continue
				}
				edgeKey := fmt.Sprintf("%s->%s", edge.Caller, edge.Callee)
				if !edgeSeen[edgeKey] {
					graph.Edges = append(graph.Edges, edge)
					edgeSeen[edgeKey] = true
				}
				// Ensure caller node is present in the graph.
				if _, exists := graph.Nodes[edge.Caller]; !exists {
					if sym := firstSymbol(s.index.Symbols[edge.Caller]); sym != nil {
						graph.Nodes[edge.Caller] = *sym
					}
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

// GetFileExtractorVersion returns the SymbolExtractor.Version() that was
// recorded when this file's symbols were last persisted, if known.
// Symbol stores produced by older grepai releases lack this metadata; in
// that case the second return is false and callers should treat the
// cache as stale.
func (s *GOBSymbolStore) GetFileExtractorVersion(filePath string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	version, ok := s.fileExtractorVersions[filePath]
	return version, ok
}
