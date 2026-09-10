package store

import "context"

func (s *GOBStore) GetCompleteDocument(ctx context.Context, filePath string) (*Document, error) {
	return s.GetCompleteDocumentWithPrefix(ctx, filePath, "")
}

func (s *GOBStore) GetCompleteDocumentWithPrefix(_ context.Context, filePath, chunkIDPrefix string) (*Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	doc, ok := s.documents[filePath]
	if !ok || len(doc.ChunkIDs) == 0 {
		return nil, nil
	}
	for _, id := range doc.ChunkIDs {
		if !s.hasValidChunkForReference(id, filePath, chunkIDPrefix) {
			return nil, nil
		}
	}

	doc.ChunkIDs = append([]string(nil), doc.ChunkIDs...)
	return &doc, nil
}

// hasValidChunkForReference reports whether any chunk stored under one of the
// reference's candidate IDs (see ChunkReferenceCandidates) belongs to
// filePath and carries a vector. An exact-ID chunk with the wrong owner or an
// empty vector does not block a valid prefixed mapping.
func (s *GOBStore) hasValidChunkForReference(id, filePath, chunkIDPrefix string) bool {
	for _, candidate := range ChunkReferenceCandidates(id, chunkIDPrefix) {
		if chunk, ok := s.chunks[candidate]; ok && chunk.FilePath == filePath && len(chunk.Vector) > 0 {
			return true
		}
	}
	return false
}

var _ CompleteDocumentSource = (*GOBStore)(nil)
var _ PrefixedCompleteDocumentSource = (*GOBStore)(nil)
