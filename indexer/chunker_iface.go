package indexer

import "log"

// Chunking strategies accepted by NewFileChunker (chunking.strategy in the
// configuration file).
const (
	strategyFixed = "fixed"
	strategyAST   = "ast"
)

// FileChunker splits file content into embeddable chunks.
type FileChunker interface {
	ChunkWithContext(filePath, content string) []ChunkInfo
	ReChunk(parent ChunkInfo, parentIndex int) []ChunkInfo
}

// NewFileChunker returns the chunker for the configured strategy. An empty
// strategy means "fixed". An unknown strategy, or "ast" in a build without
// tree-sitter support (see chunker_ast.go), logs a warning and falls back to
// the fixed-size chunker.
func NewFileChunker(strategy string, size, overlap int) FileChunker {
	base := NewChunker(size, overlap)

	switch strategy {
	case "", strategyFixed:
		return base
	case strategyAST:
		if ast := newASTChunker(base); ast != nil {
			return ast
		}
		log.Printf("chunking strategy %q requires a build with the treesitter tag, falling back to fixed", strategy)
	default:
		log.Printf("unknown chunking strategy %q, falling back to fixed", strategy)
	}
	return base
}
