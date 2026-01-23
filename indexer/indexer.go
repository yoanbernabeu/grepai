package indexer

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/store"
)

type Indexer struct {
	root          string
	store         store.VectorStore
	embedder      embedder.Embedder
	chunker       *Chunker
	scanner       *Scanner
	lastIndexTime time.Time
}

type IndexStats struct {
	FilesIndexed  int
	FilesSkipped  int
	ChunksCreated int
	FilesRemoved  int
	Duration      time.Duration
}

// ProgressInfo contains progress information for indexing
type ProgressInfo struct {
	Current     int    // Current file number (1-indexed)
	Total       int    // Total number of files
	CurrentFile string // Path of current file being processed
}

// ProgressCallback is called for each file during indexing
type ProgressCallback func(info ProgressInfo)

func NewIndexer(
	root string,
	st store.VectorStore,
	emb embedder.Embedder,
	chunker *Chunker,
	scanner *Scanner,
	lastIndexTime time.Time,
) *Indexer {
	return &Indexer{
		root:          root,
		store:         st,
		embedder:      emb,
		chunker:       chunker,
		scanner:       scanner,
		lastIndexTime: lastIndexTime,
	}
}

// IndexAll performs a full index of the project (no progress reporting)
func (idx *Indexer) IndexAll(ctx context.Context) (*IndexStats, error) {
	return idx.IndexAllWithProgress(ctx, nil)
}

// IndexAllWithProgress performs a full index with progress reporting
func (idx *Indexer) IndexAllWithProgress(ctx context.Context, onProgress ProgressCallback) (*IndexStats, error) {
	start := time.Now()
	stats := &IndexStats{}

	// Scan all files
	files, skipped, err := idx.scanner.Scan()
	if err != nil {
		return nil, fmt.Errorf("failed to scan files: %w", err)
	}
	stats.FilesSkipped = len(skipped)

	// Get existing documents
	existingDocs, err := idx.store.ListDocuments(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list documents: %w", err)
	}

	existingMap := make(map[string]bool)
	for _, doc := range existingDocs {
		existingMap[doc] = true
	}

	total := len(files)

	// Index new/modified files
	for i, file := range files {
		// Report progress
		if onProgress != nil {
			onProgress(ProgressInfo{
				Current:     i + 1,
				Total:       total,
				CurrentFile: file.Path,
			})
		}

		// Skip files modified before lastIndexTime
		if !idx.lastIndexTime.IsZero() {
			fileModTime := time.Unix(file.ModTime, 0)
			if fileModTime.Before(idx.lastIndexTime) || fileModTime.Equal(idx.lastIndexTime) {
				stats.FilesSkipped++
				delete(existingMap, file.Path)
				continue
			}
		}

		// Check if file needs reindexing
		doc, err := idx.store.GetDocument(ctx, file.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to get document %s: %w", file.Path, err)
		}

		if doc != nil && doc.Hash == file.Hash {
			delete(existingMap, file.Path)
			continue // File unchanged
		}

		// Index the file
		chunks, err := idx.IndexFile(ctx, file)
		if err != nil {
			log.Printf("Failed to index %s: %v", file.Path, err)
			continue
		}

		stats.FilesIndexed++
		stats.ChunksCreated += chunks

		delete(existingMap, file.Path)
	}

	// Remove deleted files
	for path := range existingMap {
		if err := idx.RemoveFile(ctx, path); err != nil {
			log.Printf("Failed to remove %s: %v", path, err)
			continue
		}
		stats.FilesRemoved++
	}

	stats.Duration = time.Since(start)
	return stats, nil
}

// IndexFile indexes a single file
func (idx *Indexer) IndexFile(ctx context.Context, file FileInfo) (int, error) {
	// Remove existing chunks for this file
	if err := idx.store.DeleteByFile(ctx, file.Path); err != nil {
		return 0, fmt.Errorf("failed to delete existing chunks: %w", err)
	}

	// Chunk the file
	chunkInfos := idx.chunker.ChunkWithContext(file.Path, file.Content)
	if len(chunkInfos) == 0 {
		return 0, nil
	}

	// Generate embeddings
	contents := make([]string, len(chunkInfos))
	for i, c := range chunkInfos {
		contents[i] = c.Content
	}

	vectors, err := idx.embedder.EmbedBatch(ctx, contents)
	if err != nil {
		return 0, fmt.Errorf("failed to embed chunks: %w", err)
	}

	// Create store chunks
	now := time.Now()
	chunks := make([]store.Chunk, len(chunkInfos))
	chunkIDs := make([]string, len(chunkInfos))

	for i, info := range chunkInfos {
		chunks[i] = store.Chunk{
			ID:        info.ID,
			FilePath:  info.FilePath,
			StartLine: info.StartLine,
			EndLine:   info.EndLine,
			Content:   info.Content,
			Vector:    vectors[i],
			Hash:      info.Hash,
			UpdatedAt: now,
		}
		chunkIDs[i] = info.ID
	}

	// Save chunks
	if err := idx.store.SaveChunks(ctx, chunks); err != nil {
		return 0, fmt.Errorf("failed to save chunks: %w", err)
	}

	// Save document metadata
	doc := store.Document{
		Path:     file.Path,
		Hash:     file.Hash,
		ModTime:  time.Unix(file.ModTime, 0),
		ChunkIDs: chunkIDs,
	}

	if err := idx.store.SaveDocument(ctx, doc); err != nil {
		return 0, fmt.Errorf("failed to save document: %w", err)
	}

	return len(chunks), nil
}

// RemoveFile removes a file from the index
func (idx *Indexer) RemoveFile(ctx context.Context, path string) error {
	if err := idx.store.DeleteByFile(ctx, path); err != nil {
		return fmt.Errorf("failed to delete chunks: %w", err)
	}

	if err := idx.store.DeleteDocument(ctx, path); err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	return nil
}

// NeedsReindex checks if a file needs reindexing
func (idx *Indexer) NeedsReindex(ctx context.Context, path string, hash string) (bool, error) {
	doc, err := idx.store.GetDocument(ctx, path)
	if err != nil {
		return false, err
	}

	if doc == nil {
		return true, nil
	}

	return doc.Hash != hash, nil
}
