package store

import (
	"context"
	"time"
)

func (s *GOBStore) RefreshDocumentModTime(ctx context.Context, path, expectedHash string, modTime time.Time) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if _, ok := exactModTimeNanos(modTime); !ok {
		return false, nil
	}
	matched := false
	s.mutateDocument(path, func(doc Document, exists bool) (Document, bool) {
		if !exists || doc.Hash != expectedHash || expectedHash == "" || len(doc.ChunkIDs) == 0 {
			return doc, false
		}
		matched = true
		if doc.HasExactModTime && doc.ModTime.Equal(modTime) {
			return doc, false
		}
		doc.ModTime, doc.HasExactModTime = modTime, true
		return doc, true
	})
	return matched, nil
}

var _ DocumentModTimeRefresher = (*GOBStore)(nil)
