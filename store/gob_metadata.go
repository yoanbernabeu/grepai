package store

import (
	"context"
	"sort"
)

func (s *GOBStore) ListDocumentMetadata(ctx context.Context) ([]DocumentMetadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	metas := make([]DocumentMetadata, 0, len(s.documents))
	for path, doc := range s.documents {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		metas = append(metas, DocumentMetadata{Path: path, Hash: doc.Hash, HasChunks: len(doc.ChunkIDs) > 0, ModTime: doc.ModTime, HasExactModTime: doc.HasExactModTime})
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].Path < metas[j].Path })
	return metas, nil
}

var _ DocumentMetadataSource = (*GOBStore)(nil)
