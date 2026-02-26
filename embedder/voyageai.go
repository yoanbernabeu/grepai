package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	defaultVoyageAIEndpoint   = "https://api.voyageai.com/v1"
	defaultVoyageAIModel      = "voyage-code-3"
	defaultVoyageAIDimensions = 1024
)

type VoyageAIEmbedder struct {
	endpoint    string
	model       string
	apiKey      string
	dimensions  *int
	inputType   string // optional: "query", "document", or "" (none)
	parallelism int
	retryPolicy RetryPolicy
	client      *http.Client
	rateLimiter *AdaptiveRateLimiter
	tokenBucket *TokenBucket
	tpmLimit    int64 // Tokens per minute limit (0 = disabled)
}

type voyageAIEmbedRequest struct {
	Model           string   `json:"model"`
	Input           []string `json:"input"`
	OutputDimension *int     `json:"output_dimension,omitempty"`
	InputType       string   `json:"input_type,omitempty"`
}

// voyageAIErrorResponse shares the same structure as OpenAI.
type voyageAIErrorResponse = openAIErrorResponse

type VoyageAIOption func(*VoyageAIEmbedder)

func WithVoyageAIEndpoint(endpoint string) VoyageAIOption {
	return func(e *VoyageAIEmbedder) {
		e.endpoint = endpoint
	}
}

func WithVoyageAIModel(model string) VoyageAIOption {
	return func(e *VoyageAIEmbedder) {
		e.model = model
	}
}

func WithVoyageAIKey(key string) VoyageAIOption {
	return func(e *VoyageAIEmbedder) {
		e.apiKey = key
	}
}

func WithVoyageAIDimensions(dimensions int) VoyageAIOption {
	return func(e *VoyageAIEmbedder) {
		e.dimensions = &dimensions
	}
}

// WithVoyageAIInputType sets the input_type parameter for the Voyage AI API.
// Options: "query", "document", or "" (unspecified).
func WithVoyageAIInputType(inputType string) VoyageAIOption {
	return func(e *VoyageAIEmbedder) {
		e.inputType = inputType
	}
}

func WithVoyageAIParallelism(parallelism int) VoyageAIOption {
	return func(e *VoyageAIEmbedder) {
		if parallelism > 0 {
			e.parallelism = parallelism
		}
	}
}

func WithVoyageAIRetryPolicy(policy RetryPolicy) VoyageAIOption {
	return func(e *VoyageAIEmbedder) {
		e.retryPolicy = policy
	}
}

// WithVoyageAITPMLimit sets the tokens-per-minute limit for proactive rate limiting.
// When set > 0, the embedder will pace requests to stay within this limit.
func WithVoyageAITPMLimit(tpm int64) VoyageAIOption {
	return func(e *VoyageAIEmbedder) {
		if tpm > 0 {
			e.tpmLimit = tpm
		}
	}
}

func NewVoyageAIEmbedder(opts ...VoyageAIOption) (*VoyageAIEmbedder, error) {
	e := &VoyageAIEmbedder{
		endpoint:    defaultVoyageAIEndpoint,
		model:       defaultVoyageAIModel,
		dimensions:  nil, // nil = let the model use its native dimensions
		parallelism: defaultParallelism,
		retryPolicy: DefaultRetryPolicy(),
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(e)
	}

	// Try to get API key from environment if not set
	if e.apiKey == "" {
		e.apiKey = os.Getenv("VOYAGE_API_KEY")
	}

	if e.apiKey == "" {
		return nil, fmt.Errorf("Voyage AI API key not set (use VOYAGE_API_KEY environment variable)")
	}

	// Initialize adaptive rate limiter with configured parallelism
	e.rateLimiter = NewAdaptiveRateLimiter(e.parallelism)

	// Initialize token bucket if TPM limit is set
	if e.tpmLimit > 0 {
		e.tokenBucket = NewTokenBucket(e.tpmLimit)
	}

	return e, nil
}

func (e *VoyageAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

func (e *VoyageAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := voyageAIEmbedRequest{
		Model:           e.model,
		Input:           texts,
		OutputDimension: e.dimensions,
		InputType:       e.inputType,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/embeddings", e.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.apiKey))

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to Voyage AI: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, handleVoyageAIErrorResponse(resp, body)
	}

	return parseEmbeddingsResponse(body, len(texts))
}

func (e *VoyageAIEmbedder) Dimensions() int {
	if e.dimensions == nil {
		return defaultVoyageAIDimensions
	}
	return *e.dimensions
}

func (e *VoyageAIEmbedder) Close() error {
	return nil
}

// BatchConfig returns batch limits tuned for the Voyage AI embeddings API.
func (e *VoyageAIEmbedder) BatchConfig() BatchConfig {
	return BatchConfig{
		MaxBatchSize:   500,
		MaxBatchTokens: 50000,
	}
}

// EmbedBatches implements the BatchEmbedder interface.
// It processes multiple batches concurrently using a bounded worker pool
// and retries failed requests with exponential backoff.
func (e *VoyageAIEmbedder) EmbedBatches(ctx context.Context, batches []Batch, progress BatchProgress) ([]BatchResult, error) {
	if len(batches) == 0 {
		return nil, nil
	}

	// Calculate total chunks across all batches for progress tracking
	totalChunks := 0
	for _, batch := range batches {
		totalChunks += batch.Size()
	}

	// Track completed chunks atomically for thread-safe progress updates
	var completedChunks atomic.Int64

	results := make([]BatchResult, len(batches))
	g, ctx := errgroup.WithContext(ctx)
	// Use adaptive rate limiter's current workers for dynamic parallelism
	g.SetLimit(e.rateLimiter.CurrentWorkers())

	for i := range batches {
		batch := batches[i]
		g.Go(func() error {
			embeddings, err := e.embedBatchWithRetry(ctx, batch, len(batches), totalChunks, &completedChunks, progress)
			if err != nil {
				return err
			}
			results[batch.Index] = BatchResult{
				BatchIndex: batch.Index,
				Embeddings: embeddings,
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// waitForTokenBucket waits for token budget if proactive rate limiting is enabled.
// Returns an error if the context is canceled while waiting.
func (e *VoyageAIEmbedder) waitForTokenBucket(ctx context.Context, tokens int64) error {
	if e.tokenBucket == nil {
		return nil
	}
	wait := e.tokenBucket.WaitForTokens(tokens)
	if wait <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}

// calculateRetryDelay determines the delay before the next retry attempt.
// Uses Retry-After header if available, otherwise falls back to exponential backoff.
func (e *VoyageAIEmbedder) calculateRetryDelay(attempt int, retryErr *RetryableError) time.Duration {
	if retryErr.RateLimitHeaders != nil && retryErr.RateLimitHeaders.RetryAfter > 0 {
		delay := retryErr.RateLimitHeaders.RetryAfter
		if delay > 60*time.Second {
			delay = 60 * time.Second
		}
		return delay
	}
	return e.retryPolicy.Calculate(attempt)
}

// reportBatchSuccess handles successful batch completion:
// notifies rate limiter, tracks token usage, updates progress.
func (e *VoyageAIEmbedder) reportBatchSuccess(
	batch Batch,
	totalBatches int,
	totalChunks int,
	completedChunks *atomic.Int64,
	estimatedTokens int64,
	progress BatchProgress,
) {
	e.rateLimiter.OnSuccess()

	if e.tokenBucket != nil {
		e.tokenBucket.AddTokens(estimatedTokens)
	}

	newCompleted := completedChunks.Add(int64(batch.Size()))
	if progress != nil {
		progress(batch.Index, totalBatches, int(newCompleted), totalChunks, false, 0, 0)
	}
}

// estimateBatchTokens returns the estimated token count for a batch.
func (e *VoyageAIEmbedder) estimateBatchTokens(contents []string) int64 {
	if e.tokenBucket == nil {
		return 0
	}
	var total int64
	for _, content := range contents {
		total += int64(EstimateTokens(content))
	}
	return total
}

func (e *VoyageAIEmbedder) embedBatchWithRetry(
	ctx context.Context,
	batch Batch,
	totalBatches int,
	totalChunks int,
	completedChunks *atomic.Int64,
	progress BatchProgress,
) ([][]float32, error) {
	contents := batch.Contents()
	estimatedTokens := e.estimateBatchTokens(contents)

	for attempt := 0; ; attempt++ {
		if err := e.waitForTokenBucket(ctx, estimatedTokens); err != nil {
			return nil, err
		}

		embeddings, err := e.embedBatchRequest(ctx, contents)
		if err == nil {
			e.reportBatchSuccess(batch, totalBatches, totalChunks, completedChunks, estimatedTokens, progress)
			return embeddings, nil
		}

		retryErr, isRetryable := err.(*RetryableError)
		if !isRetryable || !retryErr.Retryable {
			return nil, err
		}

		if retryErr.StatusCode == 429 {
			e.rateLimiter.OnRateLimitHit()
		}

		if !e.retryPolicy.ShouldRetry(attempt) {
			return nil, fmt.Errorf("batch %d failed after %d attempts: %w", batch.Index, attempt+1, err)
		}

		if progress != nil {
			progress(batch.Index, totalBatches, int(completedChunks.Load()), totalChunks, true, attempt+1, retryErr.StatusCode)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(e.calculateRetryDelay(attempt, retryErr)):
		}
	}
}

// buildEmbedHTTPRequest creates an HTTP request for the Voyage AI embeddings API.
func (e *VoyageAIEmbedder) buildEmbedHTTPRequest(ctx context.Context, texts []string) (*http.Request, error) {
	reqBody := voyageAIEmbedRequest{
		Model:           e.model,
		Input:           texts,
		OutputDimension: e.dimensions,
		InputType:       e.inputType,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/embeddings", e.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.apiKey))

	return req, nil
}

// handleVoyageAIErrorResponse parses an error response and returns an appropriate error.
func handleVoyageAIErrorResponse(resp *http.Response, body []byte) error {
	var errResp voyageAIErrorResponse
	msg := string(body)
	if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
		msg = errResp.Error.Message
	}

	// Check for context length error
	if resp.StatusCode == http.StatusBadRequest &&
		(strings.Contains(msg, "maximum context length") ||
			strings.Contains(msg, "too many tokens") ||
			strings.Contains(msg, "reduce the length") ||
			strings.Contains(msg, "total number of tokens")) {
		return NewContextLengthError(0, 0, 32000, msg)
	}

	retryErr := NewRetryableError(resp.StatusCode, fmt.Sprintf("Voyage AI API error (status %d): %s", resp.StatusCode, msg))
	if resp.StatusCode == http.StatusTooManyRequests {
		headers := parseRateLimitHeaders(resp.Header)
		retryErr.RateLimitHeaders = &headers
	}

	return retryErr
}

// embedBatchRequest makes a single embedding request to the Voyage AI API.
// It returns a RetryableError for HTTP errors that can be retried.
func (e *VoyageAIEmbedder) embedBatchRequest(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	req, err := e.buildEmbedHTTPRequest(ctx, texts)
	if err != nil {
		return nil, err
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to Voyage AI: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, handleVoyageAIErrorResponse(resp, body)
	}

	return parseEmbeddingsResponse(body, len(texts))
}
