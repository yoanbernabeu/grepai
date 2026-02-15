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

	// Scan all files (metadata-only first pass)
	fileMetas, skipped, err := idx.scanner.ScanMetadata()
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
	filesToIndex := make([]FileInfo, 0, len(fileMetas))
	for i, fileMeta := range fileMetas {
		// Report progress for scanning phase
		if onProgress != nil {
			onProgress(ProgressInfo{
				Current:     i + 1,
				Total:       len(fileMetas),
				CurrentFile: fileMeta.Path,
			})
		}

		// Fetch the document once — used by both the mod-time gate and hash check.
		doc, err := idx.store.GetDocument(ctx, fileMeta.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to get document %s: %w", fileMeta.Path, err)
		}

		// Skip files modified before lastIndexTime — but only if they have chunks.
		// Files with no chunks need re-indexing even if their mod_time is old
		// (e.g., a prior indexing run created the document but failed to embed).
		if !idx.lastIndexTime.IsZero() && doc != nil && len(doc.ChunkIDs) > 0 {
			fileModTime := time.Unix(fileMeta.ModTime, 0)
			if fileModTime.Before(idx.lastIndexTime) || fileModTime.Equal(idx.lastIndexTime) {
				stats.FilesSkipped++
				delete(existingMap, fileMeta.Path)
				continue
			}
		}

		// Load file content and hash only after metadata filtering.
		file, err := idx.scanner.ScanFile(fileMeta.Path)
		if err != nil {
			log.Printf("Failed to scan %s: %v", fileMeta.Path, err)
			stats.FilesSkipped++
			delete(existingMap, fileMeta.Path)
			continue
		}
		if file == nil {
			stats.FilesSkipped++
			delete(existingMap, fileMeta.Path)
			continue
		}

		if doc != nil && doc.Hash == file.Hash && len(doc.ChunkIDs) > 0 {
			delete(existingMap, fileMeta.Path)
			continue // File unchanged and has chunks
		}

		filesToIndex = append(filesToIndex, *file)
		delete(existingMap, fileMeta.Path)
	}

	// Index files using batch processing if available, otherwise sequentially
	if batchEmbedder, ok := idx.embedder.(embedder.BatchEmbedder); ok && len(filesToIndex) > 0 {
		indexed, chunks, err := idx.indexFilesBatched(ctx, filesToIndex, batchEmbedder, onBatchProgress)
		if err != nil {
			return nil, err
		}
		stats.FilesIndexed = indexed
		stats.ChunksCreated = chunks
	} else if len(filesToIndex) > 0 {
		// Sequential indexing for non-batch embedders (e.g., Ollama)
		total := len(filesToIndex)
		for i, file := range filesToIndex {
			if onBatchProgress != nil {
				onBatchProgress(BatchProgressInfo{
					BatchIndex:      i,
					TotalBatches:    total,
					CompletedChunks: i,
					TotalChunks:     total,
				})
			}
			chunks, err := idx.IndexFile(ctx, file)
			if err != nil {
				log.Printf("Failed to index %s: %v", file.Path, err)
				continue
			}
			stats.FilesIndexed++
			stats.ChunksCreated += chunks
		}
		if onBatchProgress != nil {
			onBatchProgress(BatchProgressInfo{
				BatchIndex:      total,
				TotalBatches:    total,
				CompletedChunks: total,
				TotalChunks:     total,
			})
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
			ID:          info.ID,
			FilePath:    info.FilePath,
			StartLine:   info.StartLine,
			EndLine:     info.EndLine,
			Content:     info.Content,
			Vector:      embeddings[i],
			Hash:        info.Hash,
			ContentHash: info.ContentHash,
			UpdatedAt:   now,
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

	// Check embedding cache for content-addressed deduplication
	cache, hasCache := idx.store.(store.EmbeddingCache)
	var totalCacheHits int

	// Pre-fill cached embeddings and filter out fully-cached files
	type preFilled struct {
		fdIndex   int
		vectors   [][]float32
		allCached bool
	}

	var preFilledFiles []preFilled
	var remainingFileData []fileChunkData
	var remainingFileChunks []embedder.FileChunks

	for i, fd := range fileData {
		if !hasCache {
			remainingFileData = append(remainingFileData, fd)
			remainingFileChunks = append(remainingFileChunks, fileChunks[i])
			continue
		}

		vecs := make([][]float32, len(fd.chunkInfos))
		allCached := true
		for j, chunk := range fd.chunkInfos {
			if chunk.ContentHash == "" {
				allCached = false
				continue
			}
			vec, found, err := cache.LookupByContentHash(ctx, chunk.ContentHash)
			if err != nil {
				log.Printf("Warning: cache lookup failed: %v", err)
				allCached = false
				continue
			}
			if found {
				vecs[j] = vec
				totalCacheHits++
			} else {
				allCached = false
			}
		}

		if allCached {
			preFilledFiles = append(preFilledFiles, preFilled{fdIndex: i, vectors: vecs, allCached: true})
		} else {
			remainingFileData = append(remainingFileData, fd)
			remainingFileChunks = append(remainingFileChunks, fileChunks[i])
		}
	}

	if totalCacheHits > 0 {
		log.Printf("Reused %d cached embeddings across %d files", totalCacheHits, len(preFilledFiles))
	}

	// Save fully-cached files immediately
	now := time.Now()
	for _, pf := range preFilledFiles {
		fd := fileData[pf.fdIndex]
		chunks, chunkIDs := createStoreChunks(fd.chunkInfos, pf.vectors, now)
		if err := idx.saveFileData(ctx, fd, chunks, chunkIDs); err != nil {
			return filesIndexed, chunksCreated, err
		}
		filesIndexed++
		chunksCreated += len(chunks)
	}

	// Embed remaining (non-cached) files
	if len(remainingFileChunks) > 0 {
		batches := embedder.FormBatches(remainingFileChunks)
		results, err := batchEmb.EmbedBatches(ctx, batches, wrapBatchProgress(onProgress))
		if err != nil {
			return filesIndexed, chunksCreated, fmt.Errorf("failed to embed batches: %w", err)
		}

		fileEmbeddings := embedder.MapResultsToFiles(batches, results, len(files))

		for _, fd := range remainingFileData {
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

	// Check embedding cache for content-addressed deduplication
	cachedVectors, cacheHits := idx.lookupCachedEmbeddings(ctx, chunkInfos)
	if cacheHits > 0 {
		log.Printf("Reused %d cached embeddings for %s", cacheHits, file.Path)
	}

	// Separate cached and uncached chunks
	var uncachedChunks []ChunkInfo
	for i, chunk := range chunkInfos {
		if _, ok := cachedVectors[i]; !ok {
			uncachedChunks = append(uncachedChunks, chunk)
		}
	}

	// Embed only uncached chunks
	var uncachedVectors [][]float32
	var finalUncachedChunks []ChunkInfo
	if len(uncachedChunks) > 0 {
		var err error
		uncachedVectors, finalUncachedChunks, err = idx.embedWithReChunking(ctx, uncachedChunks)
		if err != nil {
			return 0, fmt.Errorf("failed to embed chunks: %w", err)
		}
	}

	// Merge cached and freshly embedded results
	// If re-chunking happened, the final chunks may differ from original
	// In that case, we use the re-chunked results plus the cached ones
	var vectors [][]float32
	var finalChunks []ChunkInfo

	if cacheHits == 0 {
		// No cache hits - use embedding results directly
		vectors = uncachedVectors
		finalChunks = finalUncachedChunks
	} else if len(uncachedChunks) == 0 {
		// All cached - build vectors and chunks from cache
		vectors = make([][]float32, len(chunkInfos))
		for i := range chunkInfos {
			vectors[i] = cachedVectors[i]
		}
		finalChunks = chunkInfos
	} else {
		// Mix of cached and uncached - merge results
		// Note: if re-chunking changed uncached chunks, we can't easily merge
		// with the original indices. Fall back to simple merge.
		vectors = make([][]float32, 0, len(chunkInfos))
		finalChunks = make([]ChunkInfo, 0, len(chunkInfos))

		uncachedIdx := 0
		for i, chunk := range chunkInfos {
			if vec, ok := cachedVectors[i]; ok {
				vectors = append(vectors, vec)
				finalChunks = append(finalChunks, chunk)
			} else {
				// Check if re-chunking happened (uncachedVectors may have different length)
				if uncachedIdx < len(uncachedVectors) && uncachedIdx < len(finalUncachedChunks) {
					vectors = append(vectors, uncachedVectors[uncachedIdx])
					finalChunks = append(finalChunks, finalUncachedChunks[uncachedIdx])
					uncachedIdx++
				}
			}
		}
		// If re-chunking produced extra sub-chunks, append them
		for ; uncachedIdx < len(uncachedVectors); uncachedIdx++ {
			vectors = append(vectors, uncachedVectors[uncachedIdx])
			finalChunks = append(finalChunks, finalUncachedChunks[uncachedIdx])
		}
	}

	// Create store chunks
	now := time.Now()
	chunks := make([]store.Chunk, len(finalChunks))
	chunkIDs := make([]string, len(finalChunks))

	for i, info := range finalChunks {
		chunks[i] = store.Chunk{
			ID:          info.ID,
			FilePath:    info.FilePath,
			StartLine:   info.StartLine,
			EndLine:     info.EndLine,
			Content:     info.Content,
			Vector:      vectors[i],
			Hash:        info.Hash,
			ContentHash: info.ContentHash,
			UpdatedAt:   now,
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

// lookupCachedEmbeddings checks if the store implements EmbeddingCache and returns
// cached vectors for chunks with matching content hashes. The returned map maps
// chunk index to cached vector. Chunks not in the map need fresh embedding.
func (idx *Indexer) lookupCachedEmbeddings(ctx context.Context, chunks []ChunkInfo) (map[int][]float32, int) {
	cache, ok := idx.store.(store.EmbeddingCache)
	if !ok {
		return nil, 0
	}

	cached := make(map[int][]float32)
	for i, chunk := range chunks {
		if chunk.ContentHash == "" {
			continue
		}
		vec, found, err := cache.LookupByContentHash(ctx, chunk.ContentHash)
		if err != nil {
			log.Printf("Warning: cache lookup failed for content hash %s: %v", chunk.ContentHash[:8], err)
			continue
		}
		if found {
			cached[i] = vec
		}
	}

	return cached, len(cached)
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

	// Reindex if hash changed OR if document has no chunks (prior indexing failed)
	if doc.Hash != hash || len(doc.ChunkIDs) == 0 {
		return true, nil
	}

	return false, nil
}
