//go:build !treesitter

package indexer

// NewFileChunker returns a fixed-size chunker when tree-sitter is not available.
func NewFileChunker(strategy string, size, overlap int) FileChunker {
	return NewChunker(size, overlap)
}
