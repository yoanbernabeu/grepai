package embedder

// MaxBatchSize is the maximum number of inputs per OpenAI embedding API call.
// OpenAI allows 2048, but we use 2000 as a safety margin.
const MaxBatchSize = 2000

// MaxBatchTokens is the maximum total tokens per OpenAI embedding API batch.
// OpenAI has a 300,000 token limit. We use 280,000 for safety margin.
const MaxBatchTokens = 280000

// BatchLimits bounds the size of a single embedding request.
// Zero or negative values fall back to MaxBatchSize / MaxBatchTokens.
type BatchLimits struct {
	MaxSize   int
	MaxTokens int
}

// DefaultBatchLimits returns the limits matching the OpenAI API.
func DefaultBatchLimits() BatchLimits {
	return BatchLimits{MaxSize: MaxBatchSize, MaxTokens: MaxBatchTokens}
}

// withDefaults replaces unset limits with the OpenAI defaults.
func (l BatchLimits) withDefaults() BatchLimits {
	if l.MaxSize <= 0 {
		l.MaxSize = MaxBatchSize
	}
	if l.MaxTokens <= 0 {
		l.MaxTokens = MaxBatchTokens
	}
	return l
}

// BatchLimiter is implemented by embedders whose request size limits are
// configurable. Embedders that do not implement it use DefaultBatchLimits.
type BatchLimiter interface {
	BatchLimits() BatchLimits
}

// EstimateTokens estimates the token count for a text string.
// Uses a conservative estimate of ~4 characters per token for English text.
// This is intentionally conservative to avoid hitting API limits.
func EstimateTokens(text string) int {
	// Rough estimate: 1 token ≈ 4 characters for English text
	// Use 3.5 to be more conservative
	return (len(text) + 3) / 4 // Round up
}

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

// batchBuilder accumulates chunks into batches.
type batchBuilder struct {
	batches       []Batch
	current       Batch
	currentTokens int
	limits        BatchLimits
}

func newBatchBuilder(estimatedBatches int, limits BatchLimits) *batchBuilder {
	return &batchBuilder{
		batches: make([]Batch, 0, estimatedBatches),
		current: Batch{
			Index:   0,
			Entries: make([]BatchEntry, 0, min(limits.MaxSize, MaxBatchSize)),
		},
		limits: limits,
	}
}

func (b *batchBuilder) isFull(additionalTokens int) bool {
	if len(b.current.Entries) >= b.limits.MaxSize {
		return true
	}
	if len(b.current.Entries) > 0 && b.currentTokens+additionalTokens > b.limits.MaxTokens {
		return true
	}
	return false
}

func (b *batchBuilder) finalizeCurrent() {
	b.batches = append(b.batches, b.current)
	b.current = Batch{
		Index:   len(b.batches),
		Entries: make([]BatchEntry, 0, min(b.limits.MaxSize, MaxBatchSize)),
	}
	b.currentTokens = 0
}

func (b *batchBuilder) add(fileIdx, chunkIdx int, content string, tokens int) {
	b.current.Entries = append(b.current.Entries, BatchEntry{
		FileIndex:  fileIdx,
		ChunkIndex: chunkIdx,
		Content:    content,
	})
	b.currentTokens += tokens
}

func (b *batchBuilder) build() []Batch {
	if len(b.current.Entries) > 0 {
		b.batches = append(b.batches, b.current)
	}
	return b.batches
}

// FormBatches splits chunks from multiple files into batches respecting both
// MaxBatchSize (input count) and MaxBatchTokens (token limit).
// Chunks maintain their file/chunk index tracking for result mapping.
func FormBatches(files []FileChunks) []Batch {
	return FormBatchesWithLimits(files, DefaultBatchLimits())
}

// FormBatchesWithLimits is FormBatches with custom request size limits.
// A single chunk larger than MaxTokens still gets its own batch.
func FormBatchesWithLimits(files []FileChunks, limits BatchLimits) []Batch {
	totalChunks := countTotalChunks(files)
	if totalChunks == 0 {
		return nil
	}

	limits = limits.withDefaults()
	estimatedBatches := (totalChunks + limits.MaxSize - 1) / limits.MaxSize
	builder := newBatchBuilder(estimatedBatches, limits)

	for _, file := range files {
		for chunkIdx, chunk := range file.Chunks {
			tokens := EstimateTokens(chunk)
			if builder.isFull(tokens) {
				builder.finalizeCurrent()
			}
			builder.add(file.FileIndex, chunkIdx, chunk, tokens)
		}
	}

	return builder.build()
}

func countTotalChunks(files []FileChunks) int {
	total := 0
	for _, f := range files {
		total += len(f.Chunks)
	}
	return total
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
	chunkCounts := countChunksPerFile(batches, numFiles)
	fileEmbeddings := allocateFileEmbeddings(chunkCounts)
	populateEmbeddings(fileEmbeddings, batches, results)
	return fileEmbeddings
}

func countChunksPerFile(batches []Batch, numFiles int) []int {
	counts := make([]int, numFiles)
	for _, batch := range batches {
		for _, entry := range batch.Entries {
			if entry.ChunkIndex+1 > counts[entry.FileIndex] {
				counts[entry.FileIndex] = entry.ChunkIndex + 1
			}
		}
	}
	return counts
}

func allocateFileEmbeddings(chunkCounts []int) [][][]float32 {
	embeddings := make([][][]float32, len(chunkCounts))
	for i, count := range chunkCounts {
		if count > 0 {
			embeddings[i] = make([][]float32, count)
		}
	}
	return embeddings
}

func populateEmbeddings(fileEmbeddings [][][]float32, batches []Batch, results []BatchResult) {
	for _, result := range results {
		batch := batches[result.BatchIndex]
		for i, entry := range batch.Entries {
			if i < len(result.Embeddings) {
				fileEmbeddings[entry.FileIndex][entry.ChunkIndex] = result.Embeddings[i]
			}
		}
	}
}
