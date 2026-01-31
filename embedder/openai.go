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
	defaultOpenAIEndpoint         = "https://api.openai.com/v1"
	defaultOpenAIModel            = "text-embedding-3-small"
	defaultOpenAI3SmallDimensions = 1536
	defaultParallelism            = 4
)

type OpenAIEmbedder struct {
	endpoint    string
	model       string
	apiKey      string
	dimensions  *int
	parallelism int
	retryPolicy RetryPolicy
	client      *http.Client
	rateLimiter *AdaptiveRateLimiter
	tokenBucket *TokenBucket
	tpmLimit    int64 // Tokens per minute limit (0 = disabled)
}

type openAIEmbedRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions *int     `json:"dimensions,omitempty"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

type OpenAIOption func(*OpenAIEmbedder)

func WithOpenAIEndpoint(endpoint string) OpenAIOption {
	return func(e *OpenAIEmbedder) {
		e.endpoint = endpoint
	}
}

func WithOpenAIModel(model string) OpenAIOption {
	return func(e *OpenAIEmbedder) {
		e.model = model
	}
}

func WithOpenAIKey(key string) OpenAIOption {
	return func(e *OpenAIEmbedder) {
		e.apiKey = key
	}
}
func WithOpenAIDimensions(dimensions int) OpenAIOption {
	return func(e *OpenAIEmbedder) {
		e.dimensions = &dimensions
	}
}

func WithOpenAIParallelism(parallelism int) OpenAIOption {
	return func(e *OpenAIEmbedder) {
		if parallelism > 0 {
			e.parallelism = parallelism
		}
	}
}

func WithOpenAIRetryPolicy(policy RetryPolicy) OpenAIOption {
	return func(e *OpenAIEmbedder) {
		e.retryPolicy = policy
	}
}

// WithOpenAITPMLimit sets the tokens-per-minute limit for proactive rate limiting.
// When set > 0, the embedder will pace requests to stay within this limit.
// Default: 0 (disabled). OpenAI Tier 1 limit is 1,000,000 TPM for embeddings.
func WithOpenAITPMLimit(tpm int64) OpenAIOption {
	return func(e *OpenAIEmbedder) {
		if tpm > 0 {
			e.tpmLimit = tpm
		}
	}
}

func NewOpenAIEmbedder(opts ...OpenAIOption) (*OpenAIEmbedder, error) {
	e := &OpenAIEmbedder{
		endpoint:    defaultOpenAIEndpoint,
		model:       defaultOpenAIModel,
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
		e.apiKey = os.Getenv("OPENAI_API_KEY")
	}

	if e.apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key not set (use OPENAI_API_KEY environment variable)")
	}

	// Initialize adaptive rate limiter with configured parallelism
	e.rateLimiter = NewAdaptiveRateLimiter(e.parallelism)

	// Initialize token bucket if TPM limit is set
	if e.tpmLimit > 0 {
		e.tokenBucket = NewTokenBucket(e.tpmLimit)
	}

	return e, nil
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

func (e *OpenAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := openAIEmbedRequest{
		Model:      e.model,
		Input:      texts,
		Dimensions: e.dimensions,
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
		return nil, fmt.Errorf("failed to send request to OpenAI: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp openAIErrorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("OpenAI API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("OpenAI returned status %d: %s", resp.StatusCode, string(body))
	}

	var result openAIEmbedResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Data) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(result.Data))
	}

	// Sort by index to maintain order
	embeddings := make([][]float32, len(texts))
	for _, item := range result.Data {
		embeddings[item.Index] = item.Embedding
	}

	return embeddings, nil
}

func (e *OpenAIEmbedder) Dimensions() int {
	if e.dimensions == nil {
		return defaultOpenAI3SmallDimensions
	}
	return *e.dimensions
}

func (e *OpenAIEmbedder) Close() error {
	return nil
}

// EmbedBatches implements the BatchEmbedder interface.
// It processes multiple batches concurrently using a bounded worker pool
// and retries failed requests with exponential backoff.
func (e *OpenAIEmbedder) EmbedBatches(ctx context.Context, batches []Batch, progress BatchProgress) ([]BatchResult, error) {
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

// embedBatchWithRetry embeds a single batch with retry logic for retryable errors.
// waitForTokenBucket waits for token budget if proactive rate limiting is enabled.
// Returns an error if the context is canceled while waiting.
func (e *OpenAIEmbedder) waitForTokenBucket(ctx context.Context, tokens int64) error {
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
func (e *OpenAIEmbedder) calculateRetryDelay(attempt int, retryErr *RetryableError) time.Duration {
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
func (e *OpenAIEmbedder) reportBatchSuccess(
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
func (e *OpenAIEmbedder) estimateBatchTokens(contents []string) int64 {
	if e.tokenBucket == nil {
		return 0
	}
	var total int64
	for _, content := range contents {
		total += int64(EstimateTokens(content))
	}
	return total
}

func (e *OpenAIEmbedder) embedBatchWithRetry(
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

// buildEmbedHTTPRequest creates an HTTP request for the OpenAI embeddings API.
func (e *OpenAIEmbedder) buildEmbedHTTPRequest(ctx context.Context, texts []string) (*http.Request, error) {
	reqBody := openAIEmbedRequest{
		Model:      e.model,
		Input:      texts,
		Dimensions: e.dimensions,
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

// handleEmbedErrorResponse parses an error response and returns an appropriate error.
// Returns a ContextLengthError for context limit exceeded, or a RetryableError for other cases.
func handleEmbedErrorResponse(resp *http.Response, body []byte) error {
	var errResp openAIErrorResponse
	msg := string(body)
	if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
		msg = errResp.Error.Message
	}

	// Check for context length error (OpenAI returns 400 with specific message)
	// Common patterns: "maximum context length", "too many tokens"
	if resp.StatusCode == http.StatusBadRequest &&
		(strings.Contains(msg, "maximum context length") ||
			strings.Contains(msg, "too many tokens") ||
			strings.Contains(msg, "reduce the length")) {
		// OpenAI typically includes "8191 tokens" or similar in the message
		// For simplicity, we don't parse the exact limit from the message
		return NewContextLengthError(0, 0, 8191, msg)
	}

	retryErr := NewRetryableError(resp.StatusCode, fmt.Sprintf("OpenAI API error (status %d): %s", resp.StatusCode, msg))
	if resp.StatusCode == http.StatusTooManyRequests {
		headers := parseRateLimitHeaders(resp.Header)
		retryErr.RateLimitHeaders = &headers
	}

	return retryErr
}

// parseEmbeddingsResponse extracts embeddings from a successful API response.
func parseEmbeddingsResponse(body []byte, expectedCount int) ([][]float32, error) {
	var result openAIEmbedResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Data) != expectedCount {
		return nil, fmt.Errorf("expected %d embeddings, got %d", expectedCount, len(result.Data))
	}

	embeddings := make([][]float32, expectedCount)
	for _, item := range result.Data {
		embeddings[item.Index] = item.Embedding
	}

	return embeddings, nil
}

// embedBatchRequest makes a single embedding request to the OpenAI API.
// It returns a RetryableError for HTTP errors that can be retried.
func (e *OpenAIEmbedder) embedBatchRequest(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	req, err := e.buildEmbedHTTPRequest(ctx, texts)
	if err != nil {
		return nil, err
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to OpenAI: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, handleEmbedErrorResponse(resp, body)
	}

	return parseEmbeddingsResponse(body, len(texts))
}
