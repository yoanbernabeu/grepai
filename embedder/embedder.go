package embedder

import "context"

// Embedder defines the interface for text embedding providers
type Embedder interface {
	// Embed converts text into a vector embedding
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch converts multiple texts into vector embeddings
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions returns the vector dimension size for this embedder
	Dimensions() int

	// Close cleanly shuts down the embedder
	Close() error
}

// BatchProgress is a callback for reporting batch embedding progress.
// It receives the batch index, total batches, chunk progress info, and optional retry information.
// completedChunks and totalChunks track overall progress across all batches.
// statusCode is the HTTP status code when retrying (429 = rate limited, 5xx = server error).
type BatchProgress func(batchIndex, totalBatches, completedChunks, totalChunks int, retrying bool, attempt int, statusCode int)

// BatchEmbedder extends Embedder with cross-file batch embedding capabilities.
// Providers that support advanced batching (like OpenAI) implement this interface
// to enable parallel processing of multiple batches.
type BatchEmbedder interface {
	Embedder

	// EmbedBatches processes multiple batches of chunks concurrently.
	// It returns results mapped back to their source files, or an error if any batch fails.
	// The progress callback is called for each batch completion or retry attempt.
	EmbedBatches(ctx context.Context, batches []Batch, progress BatchProgress) ([]BatchResult, error)
}
