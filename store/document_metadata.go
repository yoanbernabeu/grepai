package store

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"golang.org/x/sync/errgroup"
)

// DocumentMetadata is the per-document state needed for scan reconciliation.
type DocumentMetadata struct {
	Path            string
	Hash            string
	HasChunks       bool
	ModTime         time.Time
	HasExactModTime bool
}

// DocumentMetadataSource optionally provides all document metadata in one read.
type DocumentMetadataSource interface {
	ListDocumentMetadata(ctx context.Context) ([]DocumentMetadata, error)
}

// LoadDocumentMetadata uses the bulk capability when available and otherwise
// falls back to bounded concurrent point reads.
func LoadDocumentMetadata(ctx context.Context, st VectorStore) ([]DocumentMetadata, error) {
	if src, ok := st.(DocumentMetadataSource); ok {
		return src.ListDocumentMetadata(ctx)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	paths, err := st.ListDocuments(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list documents: %w", err)
	}
	type result struct {
		meta    DocumentMetadata
		present bool
	}
	results := make([]result, len(paths))
	limit := runtime.GOMAXPROCS(0) * 4
	if limit > 64 {
		limit = 64
	}
	if limit < 8 {
		limit = 8
	}
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(limit)
	for i, path := range paths {
		if err := gctx.Err(); err != nil {
			_ = g.Wait()
			return nil, err
		}
		g.Go(func() error {
			doc, err := st.GetDocument(gctx, path)
			if err != nil {
				return fmt.Errorf("failed to get document %s: %w", path, err)
			}
			if doc != nil {
				results[i] = result{present: true, meta: DocumentMetadata{Path: path, Hash: doc.Hash, HasChunks: len(doc.ChunkIDs) > 0, ModTime: doc.ModTime, HasExactModTime: doc.HasExactModTime}}
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	out := make([]DocumentMetadata, 0, len(results))
	for _, result := range results {
		if result.present {
			out = append(out, result.meta)
		}
	}
	return out, nil
}
