//go:build !treesitter

package indexer

// newASTChunker reports that AST chunking is unavailable: it needs a build
// with the treesitter tag (and CGO).
func newASTChunker(*Chunker) FileChunker {
	return nil
}
