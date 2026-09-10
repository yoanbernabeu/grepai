package cli

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

func (p *projectPrefixStore) ListDocumentMetadata(ctx context.Context) ([]store.DocumentMetadata, error) {
	source, ok := p.store.(store.DocumentMetadataSource)
	if !ok {
		// Hide this wrapper's bulk capability so the generic fallback uses its
		// prefix-scoped ListDocuments and GetDocument methods.
		fallback := struct{ store.VectorStore }{VectorStore: p}
		return store.LoadDocumentMetadata(ctx, &fallback)
	}
	all, err := source.ListDocumentMetadata(ctx)
	if err != nil {
		return nil, err
	}
	prefix := p.getPrefix() + "/"
	out := make([]store.DocumentMetadata, 0, len(all))
	for _, metadata := range all {
		if strings.HasPrefix(metadata.Path, prefix) {
			metadata.Path = filepath.FromSlash(strings.TrimPrefix(metadata.Path, prefix))
			out = append(out, metadata)
		}
	}
	return out, nil
}

func (p *projectPrefixStore) RefreshDocumentModTime(ctx context.Context, path, expectedHash string, modTime time.Time) (bool, error) {
	refresher, ok := p.store.(store.DocumentModTimeRefresher)
	if !ok {
		return false, store.ErrRefreshUnsupported
	}
	return refresher.RefreshDocumentModTime(ctx, p.getPrefix()+"/"+p.toRelSlash(path), expectedHash, modTime)
}

func (p *projectPrefixStore) GetCompleteDocument(ctx context.Context, path string) (*store.Document, error) {
	relPath := p.toRelSlash(path)
	prefixedPath := p.getPrefix() + "/" + relPath

	var doc *store.Document
	var err error
	if source, ok := p.store.(store.PrefixedCompleteDocumentSource); ok {
		// The backend may store chunks under prefixed IDs while documents keep
		// referencing the raw relative IDs, so hand it this wrapper's fixed
		// prefix (no trailing slash) for reference resolution.
		doc, err = source.GetCompleteDocumentWithPrefix(ctx, prefixedPath, p.getPrefix())
	} else if source, ok := p.store.(store.CompleteDocumentSource); ok {
		doc, err = source.GetCompleteDocument(ctx, prefixedPath)
	} else {
		return nil, store.ErrCompleteDocumentUnsupported
	}
	if err != nil || doc == nil {
		return nil, err
	}
	if doc.Path != prefixedPath {
		return nil, nil
	}

	detached := *doc
	detached.Path = filepath.FromSlash(relPath)
	detached.ChunkIDs = append([]string(nil), doc.ChunkIDs...)
	return &detached, nil
}

var _ store.DocumentMetadataSource = (*projectPrefixStore)(nil)
var _ store.DocumentModTimeRefresher = (*projectPrefixStore)(nil)
var _ store.CompleteDocumentSource = (*projectPrefixStore)(nil)
