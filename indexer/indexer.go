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

// BatchProgressInfo contains progress information for batch embedding
type BatchProgressInfo struct {
	BatchIndex      int  // Current batch index (0-indexed)
	TotalBatches    int  // Total number of batches
	CompletedChunks int  // Number of chunks completed so far
	TotalChunks     int  // Total number of chunks to embed
	Retrying        bool // True if this is a retry attempt
	Attempt         int  // Retry attempt number (1-indexed, 0 if not retrying)
	StatusCode      int  // HTTP status code when retrying (429 = rate limited, 5xx = server error)
}

// BatchProgressCallback is called for batch embedding progress and retry visibility
type BatchProgressCallback func(info BatchProgressInfo)

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
	return idx.IndexAllWithBatchProgress(ctx, onProgress, nil)
}

// IndexAllWithBatchProgress performs a full index with both file and batch progress reporting.
// When the embedder implements BatchEmbedder, files are processed in parallel using cross-file batching.
func (idx *Indexer) IndexAllWithBatchProgress(ctx context.Context, onProgress ProgressCallback, onBatchProgress BatchProgressCallback) (*IndexStats, error) {
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

	// Filter files that need indexing
	filesToIndex := make([]FileInfo, 0, len(files))
	for _, file := range files {
		// Report progress for scanning phase
		if onProgress != nil {
			onProgress(ProgressInfo{
				Current:     len(filesToIndex) + 1,
				Total:       len(files),
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

		filesToIndex = append(filesToIndex, file)
		delete(existingMap, file.Path)
	}

	// Index files using batch processing if available, otherwise sequentially
	if batchEmbedder, ok := idx.embedder.(embedder.BatchEmbedder); ok && len(filesToIndex) > 0 {
		indexed, chunks, err := idx.indexFilesBatched(ctx, filesToIndex, batchEmbedder, onBatchProgress)
		if err != nil {
			return nil, err
		}
		stats.FilesIndexed = indexed
		stats.ChunksCreated = chunks
	} else {
		// Fall back to sequential indexing
		for _, file := range filesToIndex {
			chunks, err := idx.IndexFile(ctx, file)
			if err != nil {
				log.Printf("Failed to index %s: %v", file.Path, err)
				continue
			}
			stats.FilesIndexed++
			stats.ChunksCreated += chunks
		}
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

// fileChunkData holds chunking information for a single file during batch processing.
type fileChunkData struct {
	fileIndex  int // Index in the files slice (for result mapping)
	file       FileInfo
	chunkInfos []ChunkInfo
}

// prepareFileChunks processes files by deleting existing chunks and creating new chunks.
// Returns the file data for storage and the file chunks for embedding.
func (idx *Indexer) prepareFileChunks(
	ctx context.Context,
	files []FileInfo,
) ([]fileChunkData, []embedder.FileChunks, error) {
	fileData := make([]fileChunkData, 0, len(files))
	fileChunks := make([]embedder.FileChunks, 0, len(files))

	for i, file := range files {
		if err := idx.store.DeleteByFile(ctx, file.Path); err != nil {
			return nil, nil, fmt.Errorf("failed to delete existing chunks for %s: %w", file.Path, err)
		}

		chunkInfos := idx.chunker.ChunkWithContext(file.Path, file.Content)
		if len(chunkInfos) == 0 {
			continue
		}

		contents := make([]string, len(chunkInfos))
		for j, c := range chunkInfos {
			contents[j] = c.Content
		}

		fileData = append(fileData, fileChunkData{
			fileIndex:  i,
			file:       file,
			chunkInfos: chunkInfos,
		})

		fileChunks = append(fileChunks, embedder.FileChunks{
			FileIndex: i,
			Chunks:    contents,
		})
	}

	return fileData, fileChunks, nil
}

// createStoreChunks creates store.Chunk objects from chunk info and embeddings.
func createStoreChunks(chunkInfos []ChunkInfo, embeddings [][]float32, now time.Time) ([]store.Chunk, []string) {
	chunks := make([]store.Chunk, len(chunkInfos))
	chunkIDs := make([]string, len(chunkInfos))

	for i, info := range chunkInfos {
		chunks[i] = store.Chunk{
			ID:        info.ID,
			FilePath:  info.FilePath,
			StartLine: info.StartLine,
			EndLine:   info.EndLine,
			Content:   info.Content,
			Vector:    embeddings[i],
			Hash:      info.Hash,
			UpdatedAt: now,
		}
		chunkIDs[i] = info.ID
	}

	return chunks, chunkIDs
}

// saveFileData saves chunks and document metadata for a single file.
func (idx *Indexer) saveFileData(ctx context.Context, fd fileChunkData, chunks []store.Chunk, chunkIDs []string) error {
	if err := idx.store.SaveChunks(ctx, chunks); err != nil {
		return fmt.Errorf("failed to save chunks for %s: %w", fd.file.Path, err)
	}

	doc := store.Document{
		Path:     fd.file.Path,
		Hash:     fd.file.Hash,
		ModTime:  time.Unix(fd.file.ModTime, 0),
		ChunkIDs: chunkIDs,
	}

	if err := idx.store.SaveDocument(ctx, doc); err != nil {
		return fmt.Errorf("failed to save document for %s: %w", fd.file.Path, err)
	}

	return nil
}

// wrapBatchProgress creates an embedder.BatchProgress callback from BatchProgressCallback.
func wrapBatchProgress(onProgress BatchProgressCallback) embedder.BatchProgress {
	if onProgress == nil {
		return nil
	}
	return func(batchIndex, totalBatches, completedChunks, totalChunks int, retrying bool, attempt int, statusCode int) {
		onProgress(BatchProgressInfo{
			BatchIndex:      batchIndex,
			TotalBatches:    totalBatches,
			CompletedChunks: completedChunks,
			TotalChunks:     totalChunks,
			Retrying:        retrying,
			Attempt:         attempt,
			StatusCode:      statusCode,
		})
	}
}

// indexFilesBatched indexes multiple files using cross-file batch embedding.
// It collects chunks from all files, forms batches, embeds them in parallel,
// then maps results back and stores them.
func (idx *Indexer) indexFilesBatched(
	ctx context.Context,
	files []FileInfo,
	batchEmb embedder.BatchEmbedder,
	onProgress BatchProgressCallback,
) (filesIndexed int, chunksCreated int, err error) {
	fileData, fileChunks, err := idx.prepareFileChunks(ctx, files)
	if err != nil {
		return 0, 0, err
	}

	if len(fileChunks) == 0 {
		return 0, 0, nil
	}

	batches := embedder.FormBatches(fileChunks)

	results, err := batchEmb.EmbedBatches(ctx, batches, wrapBatchProgress(onProgress))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to embed batches: %w", err)
	}

	fileEmbeddings := embedder.MapResultsToFiles(batches, results, len(files))

	now := time.Now()
	for _, fd := range fileData {
		embeddings := fileEmbeddings[fd.fileIndex]

		if len(embeddings) != len(fd.chunkInfos) {
			log.Printf("Warning: embedding count mismatch for %s: got %d, expected %d",
				fd.file.Path, len(embeddings), len(fd.chunkInfos))
			continue
		}

		chunks, chunkIDs := createStoreChunks(fd.chunkInfos, embeddings, now)

		if err := idx.saveFileData(ctx, fd, chunks, chunkIDs); err != nil {
			return filesIndexed, chunksCreated, err
		}

		filesIndexed++
		chunksCreated += len(chunks)
	}

	return filesIndexed, chunksCreated, nil
}

// maxReChunkAttempts is the maximum number of times we'll try to re-chunk
// before giving up on a file.
const maxReChunkAttempts = 3

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

	// Generate embeddings with automatic re-chunking on context limit errors
	vectors, finalChunks, err := idx.embedWithReChunking(ctx, chunkInfos)
	if err != nil {
		return 0, fmt.Errorf("failed to embed chunks: %w", err)
	}

	// Create store chunks
	now := time.Now()
	chunks := make([]store.Chunk, len(finalChunks))
	chunkIDs := make([]string, len(finalChunks))

	for i, info := range finalChunks {
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

// embedWithReChunking attempts to embed chunks, automatically re-chunking
// any chunks that exceed the embedder's context limit.
func (idx *Indexer) embedWithReChunking(ctx context.Context, chunks []ChunkInfo) ([][]float32, []ChunkInfo, error) {
	currentChunks := chunks
	var allVectors [][]float32
	var finalChunks []ChunkInfo

	for attempt := 0; attempt < maxReChunkAttempts; attempt++ {
		contents := make([]string, len(currentChunks))
		for i, c := range currentChunks {
			contents[i] = c.Content
		}

		vectors, err := idx.embedder.EmbedBatch(ctx, contents)
		if err == nil {
			// Success! Append all results
			allVectors = append(allVectors, vectors...)
			finalChunks = append(finalChunks, currentChunks...)
			return allVectors, finalChunks, nil
		}

		// Check if it's a context length error
		ctxErr := embedder.AsContextLengthError(err)
		if ctxErr == nil {
			// Not a context length error, return the original error
			return nil, nil, err
		}

		// Re-chunk the problematic chunk
		failedIndex := ctxErr.ChunkIndex
		if failedIndex < 0 || failedIndex >= len(currentChunks) {
			return nil, nil, fmt.Errorf("invalid chunk index %d from context length error", failedIndex)
		}

		failedChunk := currentChunks[failedIndex]
		log.Printf("Re-chunking %s chunk %d (attempt %d/%d): context limit exceeded",
			failedChunk.FilePath, failedIndex, attempt+1, maxReChunkAttempts)

		// Embed all chunks before the failed one (they should work)
		if failedIndex > 0 {
			beforeContents := make([]string, failedIndex)
			for i := 0; i < failedIndex; i++ {
				beforeContents[i] = currentChunks[i].Content
			}
			beforeVectors, err := idx.embedder.EmbedBatch(ctx, beforeContents)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to embed chunks before failed index: %w", err)
			}
			allVectors = append(allVectors, beforeVectors...)
			finalChunks = append(finalChunks, currentChunks[:failedIndex]...)
		}

		// Re-chunk the failed chunk
		subChunks := idx.chunker.ReChunk(failedChunk, failedIndex)
		if len(subChunks) == 0 {
			return nil, nil, fmt.Errorf("re-chunking produced no chunks for %s", failedChunk.FilePath)
		}

		log.Printf("Split chunk into %d sub-chunks", len(subChunks))

		// Prepare for next iteration: sub-chunks + remaining chunks
		currentChunks = append(subChunks, currentChunks[failedIndex+1:]...)
	}

	return nil, nil, fmt.Errorf("exceeded maximum re-chunk attempts (%d) for file", maxReChunkAttempts)
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
