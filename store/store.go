package store

import (
	"context"
	"time"
)

// Chunk represents a piece of code with its vector embedding
type Chunk struct {
	ID          string    `json:"id"`
	FilePath    string    `json:"file_path"`
	StartLine   int       `json:"start_line"`
	EndLine     int       `json:"end_line"`
	Content     string    `json:"content"`
	Vector      []float32 `json:"vector"`
	Hash        string    `json:"hash"`
	ContentHash string    `json:"content_hash"` // SHA256 of raw content (path-independent)
	UpdatedAt   time.Time `json:"updated_at"`
}

// Document represents a file with its chunks
type Document struct {
	Path     string    `json:"path"`
	Hash     string    `json:"hash"`
	ModTime  time.Time `json:"mod_time"`
	ChunkIDs []string  `json:"chunk_ids"`
}

// SearchResult represents a search match with its relevance score
type SearchResult struct {
	Chunk Chunk   `json:"chunk"`
	Score float32 `json:"score"`
}

// SearchOptions contains optional filters for vector search queries.
type SearchOptions struct {
	PathPrefix string
}

// IndexStats contains statistics about the index
type IndexStats struct {
	TotalFiles  int       `json:"total_files"`
	TotalChunks int       `json:"total_chunks"`
	IndexSize   int64     `json:"index_size"` // bytes
	LastUpdated time.Time `json:"last_updated"`
}

// FileStats contains statistics for a single file
type FileStats struct {
	Path       string    `json:"path"`
	ChunkCount int       `json:"chunk_count"`
	ModTime    time.Time `json:"mod_time"`
}

// VectorStore defines the interface for vector storage backends
type VectorStore interface {
	// SaveChunks stores multiple chunks atomically
	SaveChunks(ctx context.Context, chunks []Chunk) error

	// DeleteByFile removes all chunks for a given file path
	DeleteByFile(ctx context.Context, filePath string) error

	// Search finds the most similar chunks to a query vector
	Search(ctx context.Context, queryVector []float32, limit int, opts SearchOptions) ([]SearchResult, error)

	// GetDocument retrieves document metadata by path
	GetDocument(ctx context.Context, filePath string) (*Document, error)

	// SaveDocument stores document metadata
	SaveDocument(ctx context.Context, doc Document) error

	// DeleteDocument removes document metadata
	DeleteDocument(ctx context.Context, filePath string) error

	// ListDocuments returns all indexed document paths
	ListDocuments(ctx context.Context) ([]string, error)

	// Load reads the store from persistent storage
	Load(ctx context.Context) error

	// Persist writes the store to persistent storage
	Persist(ctx context.Context) error

	// Close cleanly shuts down the store
	Close() error

	// GetStats returns index statistics
	GetStats(ctx context.Context) (*IndexStats, error)

	// ListFilesWithStats returns all files with their chunk counts
	ListFilesWithStats(ctx context.Context) ([]FileStats, error)

	// GetChunksForFile returns all chunks for a specific file
	GetChunksForFile(ctx context.Context, filePath string) ([]Chunk, error)

	// GetAllChunks returns all chunks in the store (used for text search)
	GetAllChunks(ctx context.Context) ([]Chunk, error)
}

// EmbeddingCache is an optional interface that VectorStore implementations can
// provide to enable content-addressed embedding deduplication. When a store
// implements this interface, the indexer will look up existing embeddings by
// content hash before calling the embedder, avoiding redundant API calls for
// identical content (e.g., across git worktrees).
type EmbeddingCache interface {
	// LookupByContentHash returns the embedding vector for the given content hash.
	// Returns (vector, true, nil) if found, (nil, false, nil) if not found.
	LookupByContentHash(ctx context.Context, contentHash string) ([]float32, bool, error)
}
