package trace

import "context"

// SaveFile persists symbols and references for a file.
func (s *GOBSymbolStore) SaveFile(ctx context.Context, filePath string, symbols []Symbol, refs []Reference) error {
	return s.saveFile(ctx, filePath, "", nil, symbols, refs, nil)
}

// SaveFileWithContentHash persists symbols/references for a file and tracks
// the current file content hash for future cache checks. The extractor
// version stays whatever was already stored (or empty) — call
// SaveFileWithSignature to update both fingerprints at once.
func (s *GOBSymbolStore) SaveFileWithContentHash(ctx context.Context, filePath string, contentHash string, symbols []Symbol, refs []Reference) error {
	return s.saveFile(ctx, filePath, contentHash, nil, symbols, refs, nil)
}

// saveFile publishes one file's graph data and fingerprints under one lock.
// A nil extractorVersion preserves legacy behavior; a non-nil empty value
// explicitly clears the stored version. beforeVersion is a private test seam
// used to verify that snapshots cannot observe the mutation mid-publication.
func (s *GOBSymbolStore) saveFile(ctx context.Context, filePath, contentHash string, extractorVersion *string, symbols []Symbol, refs []Reference, beforeVersion func()) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mutationGeneration++
	return s.saveFileLocked(ctx, filePath, contentHash, extractorVersion, symbols, refs, beforeVersion)
}

func (s *GOBSymbolStore) saveFileLocked(_ context.Context, filePath, contentHash string, extractorVersion *string, symbols []Symbol, refs []Reference, beforeVersion func()) error {
	// Remove old entries for this file first.
	s.deleteFileUnlocked(filePath)

	for _, sym := range symbols {
		s.index.Symbols[sym.Name] = append(s.index.Symbols[sym.Name], sym)
	}
	for _, ref := range refs {
		s.index.References[ref.SymbolName] = append(s.index.References[ref.SymbolName], ref)
	}
	for _, ref := range refs {
		if ref.CallerName != "" && ref.CallerName != "<top-level>" {
			s.index.CallGraph = append(s.index.CallGraph, CallEdge{
				Caller: ref.CallerName, Callee: ref.SymbolName, File: ref.File,
				Line: ref.Line, CallType: "direct",
			})
		}
	}

	s.fileIndex[filePath] = true
	if contentHash != "" {
		s.fileContentHashes[filePath] = contentHash
	} else {
		delete(s.fileContentHashes, filePath)
	}
	if beforeVersion != nil {
		beforeVersion()
	}
	if extractorVersion != nil {
		if *extractorVersion != "" {
			s.fileExtractorVersions[filePath] = *extractorVersion
		} else {
			delete(s.fileExtractorVersions, filePath)
		}
	}
	return nil
}

// SaveFileWithSignature persists symbols/references for a file and records both
// the content hash and extractor version that produced them.
func (s *GOBSymbolStore) SaveFileWithSignature(ctx context.Context, filePath string, contentHash, extractorVersion string, symbols []Symbol, refs []Reference) error {
	return s.saveFile(ctx, filePath, contentHash, &extractorVersion, symbols, refs, nil)
}
