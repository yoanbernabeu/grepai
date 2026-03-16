package indexer

// FileChunker splits file content into embeddable chunks.
type FileChunker interface {
	ChunkWithContext(filePath, content string) []ChunkInfo
	ReChunk(parent ChunkInfo, parentIndex int) []ChunkInfo
}
