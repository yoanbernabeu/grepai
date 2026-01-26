package embedder

// MaxBatchSize is the maximum number of inputs per OpenAI embedding API call.
// OpenAI allows 2048, but we use 2000 as a safety margin.
const MaxBatchSize = 2000

// BatchEntry represents a single chunk with metadata for tracking its source.
type BatchEntry struct {
	// FileIndex is the index of the source file in the files slice
	FileIndex int
	// ChunkIndex is the index of the chunk within the file's chunks
	ChunkIndex int
	// Content is the text content to embed
	Content string
}

// Batch represents a collection of chunks to be embedded in a single API call.
type Batch struct {
	// Entries contains chunks with source file tracking
	Entries []BatchEntry
	// Index is the batch number for progress reporting (0-indexed)
	Index int
}

// Size returns the number of entries in the batch.
func (b *Batch) Size() int {
	return len(b.Entries)
}

// Contents returns the text contents of all entries for embedding.
func (b *Batch) Contents() []string {
	contents := make([]string, len(b.Entries))
	for i, entry := range b.Entries {
		contents[i] = entry.Content
	}
	return contents
}

// FileChunks represents chunks from a single file for batch formation.
type FileChunks struct {
	// FileIndex is the index of this file in the original files slice
	FileIndex int
	// Chunks is the list of text chunks from this file
	Chunks []string
}

// FormBatches splits chunks from multiple files into batches of max MaxBatchSize.
// Chunks maintain their file/chunk index tracking for result mapping.
func FormBatches(files []FileChunks) []Batch {
	if len(files) == 0 {
		return nil
	}

	// Count total chunks to pre-allocate
	totalChunks := 0
	for _, f := range files {
		totalChunks += len(f.Chunks)
	}

	if totalChunks == 0 {
		return nil
	}

	// Estimate number of batches needed
	estimatedBatches := (totalChunks + MaxBatchSize - 1) / MaxBatchSize
	batches := make([]Batch, 0, estimatedBatches)

	var currentBatch Batch
	currentBatch.Index = 0
	currentBatch.Entries = make([]BatchEntry, 0, MaxBatchSize)

	for _, file := range files {
		for chunkIdx, chunk := range file.Chunks {
			// If current batch is full, finalize it and start a new one
			if len(currentBatch.Entries) >= MaxBatchSize {
				batches = append(batches, currentBatch)
				currentBatch = Batch{
					Index:   len(batches),
					Entries: make([]BatchEntry, 0, MaxBatchSize),
				}
			}

			currentBatch.Entries = append(currentBatch.Entries, BatchEntry{
				FileIndex:  file.FileIndex,
				ChunkIndex: chunkIdx,
				Content:    chunk,
			})
		}
	}

	// Add the last batch if it has entries
	if len(currentBatch.Entries) > 0 {
		batches = append(batches, currentBatch)
	}

	return batches
}

// BatchResult contains the embeddings for a batch with file/chunk index mapping.
type BatchResult struct {
	// BatchIndex is the index of the batch this result belongs to
	BatchIndex int
	// Embeddings contains the embedding vectors in the same order as batch entries
	Embeddings [][]float32
}

// MapResultsToFiles maps batch results back to per-file embeddings.
// Returns a slice where each index corresponds to a file, containing embeddings for that file's chunks.
func MapResultsToFiles(batches []Batch, results []BatchResult, numFiles int) [][][]float32 {
	// First, figure out how many chunks each file has
	chunkCounts := make([]int, numFiles)
	for _, batch := range batches {
		for _, entry := range batch.Entries {
			if entry.ChunkIndex+1 > chunkCounts[entry.FileIndex] {
				chunkCounts[entry.FileIndex] = entry.ChunkIndex + 1
			}
		}
	}

	// Allocate result slices for each file
	fileEmbeddings := make([][][]float32, numFiles)
	for i, count := range chunkCounts {
		if count > 0 {
			fileEmbeddings[i] = make([][]float32, count)
		}
	}

	// Map embeddings back to their source files
	for _, result := range results {
		batch := batches[result.BatchIndex]
		for i, entry := range batch.Entries {
			if i < len(result.Embeddings) {
				fileEmbeddings[entry.FileIndex][entry.ChunkIndex] = result.Embeddings[i]
			}
		}
	}

	return fileEmbeddings
}
