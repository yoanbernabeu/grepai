package store

import (
	"context"
	"errors"
	"strings"
)

// ErrCompleteDocumentUnsupported indicates that a store cannot atomically
// validate document metadata and all of its referenced chunks.
var ErrCompleteDocumentUnsupported = errors.New("complete document lookup unsupported")

// CompleteDocumentSource optionally provides a coherent store observation of
// document metadata and every chunk it references. The path is interpreted in
// the store's namespace. Missing or incomplete documents return (nil, nil), and
// successful reads return a detached Document. This contract does not imply an
// atomic observation of the filesystem represented by the store.
type CompleteDocumentSource interface {
	GetCompleteDocument(ctx context.Context, filePath string) (*Document, error)
}

// PrefixedCompleteDocumentSource optionally extends the complete-document
// contract for callers whose stored documents reference chunk IDs relative to
// a fixed namespace prefix while chunks may be stored under prefixed IDs. Each
// reference is satisfied by any valid chunk (same file, non-empty vector)
// whose ID equals the reference or, when chunkIDPrefix is non-empty,
// chunkIDPrefix+"/"+reference, with backslashes in the reference additionally
// normalized to slashes both bare and under the prefix (see
// ChunkReferenceCandidates). An empty prefix matches exact IDs only.
type PrefixedCompleteDocumentSource interface {
	GetCompleteDocumentWithPrefix(ctx context.Context, filePath, chunkIDPrefix string) (*Document, error)
}

// ChunkReferenceCandidates returns the stored chunk IDs that may satisfy a
// document chunk reference, in preference order:
//
//  1. the literal reference (covers unprefixed stores and references that
//     already carry the full stored ID);
//  2. when chunkIDPrefix is non-empty, chunkIDPrefix+"/"+reference (covers raw
//     relative references written next to prefix-stored chunks);
//  3. additionally, with backslashes converted to slashes: the normalized
//     reference itself (covers already-prefixed Windows references without
//     adding the prefix a second time) and chunkIDPrefix+"/"+normalized
//     (covers raw Windows references), so references recorded by a Windows
//     writer resolve to the slash-normalized IDs the writer stores.
//
// The literal candidate is never normalized: an empty prefix matches exact
// IDs only, and hosts whose file names legitimately contain backslashes keep
// resolving literally. Every candidate must still satisfy the strict
// same-project, same-file, non-null-vector validation of the backing store;
// normalization widens the namespace lookup only, never the ownership check.
func ChunkReferenceCandidates(id, chunkIDPrefix string) []string {
	if chunkIDPrefix == "" {
		return []string{id}
	}
	candidates := []string{id, chunkIDPrefix + "/" + id}
	if slash := strings.ReplaceAll(id, `\`, "/"); slash != id {
		candidates = append(candidates, slash, chunkIDPrefix+"/"+slash)
	}
	return candidates
}
