package embedder

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitForTokenBucket_NilBucket(t *testing.T) {
	e := &OpenAIEmbedder{tokenBucket: nil}
	err := e.waitForTokenBucket(context.Background(), 1000)
	if err != nil {
		t.Errorf("expected nil error for nil bucket, got %v", err)
	}
}

func TestWaitForTokenBucket_NoWaitNeeded(t *testing.T) {
	bucket := NewTokenBucket(100000) // Large limit, no wait needed
	e := &OpenAIEmbedder{tokenBucket: bucket}

	start := time.Now()
	err := e.waitForTokenBucket(context.Background(), 100)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if elapsed > 50*time.Millisecond {
		t.Errorf("expected no wait, but waited %v", elapsed)
	}
}

func TestWaitForTokenBucket_ContextCanceled(t *testing.T) {
	bucket := NewTokenBucket(1) // Tiny limit to force wait
	bucket.AddTokens(1000)      // Fill it up to force wait
	e := &OpenAIEmbedder{tokenBucket: bucket}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := e.waitForTokenBucket(ctx, 1000)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestCalculateRetryDelay_WithRetryAfterHeader(t *testing.T) {
	e := &OpenAIEmbedder{
		retryPolicy: RetryPolicy{BaseDelay: 100 * time.Millisecond, Multiplier: 2.0},
	}

	retryErr := &RetryableError{
		RateLimitHeaders: &RateLimitHeaders{RetryAfter: 5 * time.Second},
	}

	delay := e.calculateRetryDelay(0, retryErr)
	if delay != 5*time.Second {
		t.Errorf("expected 5s from Retry-After header, got %v", delay)
	}
}

func TestCalculateRetryDelay_RetryAfterCapped(t *testing.T) {
	e := &OpenAIEmbedder{
		retryPolicy: RetryPolicy{BaseDelay: 100 * time.Millisecond, Multiplier: 2.0},
	}

	retryErr := &RetryableError{
		RateLimitHeaders: &RateLimitHeaders{RetryAfter: 120 * time.Second}, // Exceeds 60s cap
	}

	delay := e.calculateRetryDelay(0, retryErr)
	if delay != 60*time.Second {
		t.Errorf("expected 60s cap, got %v", delay)
	}
}

func TestCalculateRetryDelay_FallbackToExponentialBackoff(t *testing.T) {
	policy := RetryPolicy{BaseDelay: 100 * time.Millisecond, Multiplier: 2.0, MaxDelay: 10 * time.Second}
	e := &OpenAIEmbedder{retryPolicy: policy}

	// No Retry-After header
	retryErr := &RetryableError{RateLimitHeaders: nil}

	delay := e.calculateRetryDelay(0, retryErr)
	// Retry policy includes jitter, so check it's within expected range
	// BaseDelay is 100ms, jitter adds up to 100% so expect 100-200ms range
	if delay < 100*time.Millisecond || delay > 200*time.Millisecond {
		t.Errorf("expected delay in range [100ms, 200ms], got %v", delay)
	}
}

func TestCalculateRetryDelay_ZeroRetryAfterFallsBack(t *testing.T) {
	policy := RetryPolicy{BaseDelay: 100 * time.Millisecond, Multiplier: 2.0}
	e := &OpenAIEmbedder{retryPolicy: policy}

	retryErr := &RetryableError{
		RateLimitHeaders: &RateLimitHeaders{RetryAfter: 0},
	}

	delay := e.calculateRetryDelay(0, retryErr)
	expected := policy.Calculate(0)
	if delay != expected {
		t.Errorf("expected %v from exponential backoff, got %v", expected, delay)
	}
}

func TestReportBatchSuccess_UpdatesAll(t *testing.T) {
	var completed atomic.Int64
	var progressCalled bool
	var progressBatchIndex, progressTotalBatches, progressCompleted, progressTotal int

	bucket := NewTokenBucket(100000)
	limiter := NewAdaptiveRateLimiter(4)

	e := &OpenAIEmbedder{
		tokenBucket: bucket,
		rateLimiter: limiter,
	}

	batch := Batch{
		Index: 2,
		Entries: []BatchEntry{
			{Content: "a"},
			{Content: "b"},
		},
	}

	progress := func(batchIndex, totalBatches, completedChunks, totalChunks int, retrying bool, attempt int, statusCode int) {
		progressCalled = true
		progressBatchIndex = batchIndex
		progressTotalBatches = totalBatches
		progressCompleted = completedChunks
		progressTotal = totalChunks
	}

	e.reportBatchSuccess(batch, 5, 100, &completed, 500, progress)

	if !progressCalled {
		t.Error("expected progress callback to be called")
	}
	if progressBatchIndex != 2 {
		t.Errorf("expected batchIndex 2, got %d", progressBatchIndex)
	}
	if progressTotalBatches != 5 {
		t.Errorf("expected totalBatches 5, got %d", progressTotalBatches)
	}
	if progressCompleted != 2 {
		t.Errorf("expected completedChunks 2, got %d", progressCompleted)
	}
	if progressTotal != 100 {
		t.Errorf("expected totalChunks 100, got %d", progressTotal)
	}
	if completed.Load() != 2 {
		t.Errorf("expected completed counter 2, got %d", completed.Load())
	}
}

func TestReportBatchSuccess_NilProgress(t *testing.T) {
	var completed atomic.Int64
	limiter := NewAdaptiveRateLimiter(4)

	e := &OpenAIEmbedder{
		tokenBucket: nil,
		rateLimiter: limiter,
	}

	batch := Batch{
		Index:   0,
		Entries: []BatchEntry{{Content: "a"}},
	}

	// Should not panic with nil progress
	e.reportBatchSuccess(batch, 1, 1, &completed, 0, nil)

	if completed.Load() != 1 {
		t.Errorf("expected completed counter 1, got %d", completed.Load())
	}
}

func TestEstimateBatchTokens_NilBucket(t *testing.T) {
	e := &OpenAIEmbedder{tokenBucket: nil}
	tokens := e.estimateBatchTokens([]string{"hello world", "foo bar"})
	if tokens != 0 {
		t.Errorf("expected 0 tokens when bucket is nil, got %d", tokens)
	}
}

func TestEstimateBatchTokens_WithBucket(t *testing.T) {
	bucket := NewTokenBucket(100000)
	e := &OpenAIEmbedder{tokenBucket: bucket}

	contents := []string{"hello world", "foo bar baz"}
	tokens := e.estimateBatchTokens(contents)

	// Should be sum of EstimateTokens for each string
	expected := int64(EstimateTokens("hello world") + EstimateTokens("foo bar baz"))
	if tokens != expected {
		t.Errorf("expected %d tokens, got %d", expected, tokens)
	}
}

func TestBuildEmbedHTTPRequest(t *testing.T) {
	dim := 512
	e := &OpenAIEmbedder{
		endpoint:   "https://api.example.com/v1",
		model:      "text-embedding-3-small",
		apiKey:     "test-api-key",
		dimensions: &dim,
	}

	ctx := context.Background()
	req, err := e.buildEmbedHTTPRequest(ctx, []string{"hello", "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check URL
	expectedURL := "https://api.example.com/v1/embeddings"
	if req.URL.String() != expectedURL {
		t.Errorf("expected URL %s, got %s", expectedURL, req.URL.String())
	}

	// Check method
	if req.Method != http.MethodPost {
		t.Errorf("expected POST method, got %s", req.Method)
	}

	// Check headers
	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", req.Header.Get("Content-Type"))
	}
	if req.Header.Get("Authorization") != "Bearer test-api-key" {
		t.Errorf("expected Authorization header, got %s", req.Header.Get("Authorization"))
	}

	// Check body can be decoded
	var body openAIEmbedRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode request body: %v", err)
	}
	if body.Model != "text-embedding-3-small" {
		t.Errorf("expected model text-embedding-3-small, got %s", body.Model)
	}
	if len(body.Input) != 2 {
		t.Errorf("expected 2 inputs, got %d", len(body.Input))
	}
	if body.Dimensions == nil || *body.Dimensions != 512 {
		t.Errorf("expected dimensions 512, got %v", body.Dimensions)
	}
}

func TestHandleEmbedErrorResponse_Basic(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{},
	}
	body := []byte(`{"error": {"message": "Invalid request", "type": "invalid_request_error"}}`)

	err := handleEmbedErrorResponse(resp, body)

	retryErr, ok := err.(*RetryableError)
	if !ok {
		t.Fatalf("expected *RetryableError, got %T", err)
	}
	if retryErr.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", retryErr.StatusCode)
	}
	if retryErr.Retryable {
		t.Error("expected 400 to not be retryable")
	}
	if retryErr.RateLimitHeaders != nil {
		t.Error("expected no rate limit headers for 400")
	}
}

func TestHandleEmbedErrorResponse_RateLimit(t *testing.T) {
	header := http.Header{}
	header.Set("Retry-After", "5")
	header.Set("X-RateLimit-Limit-Requests", "100")
	header.Set("X-RateLimit-Remaining-Requests", "0")

	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     header,
	}
	body := []byte(`{"error": {"message": "Rate limit exceeded", "type": "rate_limit_error"}}`)

	err := handleEmbedErrorResponse(resp, body)

	retryErr, ok := err.(*RetryableError)
	if !ok {
		t.Fatalf("expected *RetryableError, got %T", err)
	}
	if retryErr.StatusCode != 429 {
		t.Errorf("expected status 429, got %d", retryErr.StatusCode)
	}
	if !retryErr.Retryable {
		t.Error("expected 429 to be retryable")
	}
	if retryErr.RateLimitHeaders == nil {
		t.Fatal("expected rate limit headers to be parsed")
	}
	if retryErr.RateLimitHeaders.RetryAfter != 5*time.Second {
		t.Errorf("expected RetryAfter 5s, got %v", retryErr.RateLimitHeaders.RetryAfter)
	}
}

func TestHandleEmbedErrorResponse_InvalidJSON(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Header:     http.Header{},
	}
	body := []byte(`not valid json`)

	err := handleEmbedErrorResponse(resp, body)

	retryErr, ok := err.(*RetryableError)
	if !ok {
		t.Fatalf("expected *RetryableError, got %T", err)
	}
	// Should fall back to raw body as message
	if retryErr.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", retryErr.StatusCode)
	}
	if retryErr.RateLimitHeaders != nil {
		t.Error("expected no rate limit headers for 500")
	}
}

func TestHandleEmbedErrorResponse_ContextLengthError(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{},
	}
	body := []byte(`{"error": {"message": "This model's maximum context length is 8191 tokens", "type": "invalid_request_error"}}`)

	err := handleEmbedErrorResponse(resp, body)

	ctxErr, ok := err.(*ContextLengthError)
	if !ok {
		t.Fatalf("expected *ContextLengthError for context limit exceeded, got %T", err)
	}
	if ctxErr.MaxTokens != 8191 {
		t.Errorf("expected MaxTokens 8191, got %d", ctxErr.MaxTokens)
	}
}

func TestParseEmbeddingsResponse_Success(t *testing.T) {
	response := openAIEmbedResponse{
		Data: []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}{
			{Embedding: []float32{0.1, 0.2, 0.3}, Index: 0},
			{Embedding: []float32{0.4, 0.5, 0.6}, Index: 1},
		},
	}
	body, _ := json.Marshal(response)

	embeddings, err := parseEmbeddingsResponse(body, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(embeddings) != 2 {
		t.Errorf("expected 2 embeddings, got %d", len(embeddings))
	}
	if embeddings[0][0] != 0.1 {
		t.Errorf("expected first embedding[0] = 0.1, got %f", embeddings[0][0])
	}
	if embeddings[1][0] != 0.4 {
		t.Errorf("expected second embedding[0] = 0.4, got %f", embeddings[1][0])
	}
}

func TestParseEmbeddingsResponse_OutOfOrderIndex(t *testing.T) {
	// OpenAI may return embeddings out of order
	response := openAIEmbedResponse{
		Data: []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}{
			{Embedding: []float32{0.4, 0.5, 0.6}, Index: 1}, // Second input first
			{Embedding: []float32{0.1, 0.2, 0.3}, Index: 0}, // First input second
		},
	}
	body, _ := json.Marshal(response)

	embeddings, err := parseEmbeddingsResponse(body, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be reordered by index
	if embeddings[0][0] != 0.1 {
		t.Errorf("expected embeddings[0][0] = 0.1 (reordered), got %f", embeddings[0][0])
	}
	if embeddings[1][0] != 0.4 {
		t.Errorf("expected embeddings[1][0] = 0.4 (reordered), got %f", embeddings[1][0])
	}
}

func TestParseEmbeddingsResponse_CountMismatch(t *testing.T) {
	response := openAIEmbedResponse{
		Data: []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}{
			{Embedding: []float32{0.1, 0.2, 0.3}, Index: 0},
		},
	}
	body, _ := json.Marshal(response)

	_, err := parseEmbeddingsResponse(body, 2) // Expected 2, got 1
	if err == nil {
		t.Error("expected error for count mismatch")
	}
}

func TestParseEmbeddingsResponse_InvalidJSON(t *testing.T) {
	body := []byte(`not valid json`)

	_, err := parseEmbeddingsResponse(body, 1)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// TestOpenAIEmbedRequest_OmitsNilDimensions verifies that the dimensions field
// is omitted from JSON when nil (the fix for issue #91).
func TestOpenAIEmbedRequest_OmitsNilDimensions(t *testing.T) {
	req := openAIEmbedRequest{
		Model:      "text-embedding-3-small",
		Input:      []string{"test"},
		Dimensions: nil, // nil should be omitted
	}

	jsonBytes, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	// Verify "dimensions" key is not present in JSON
	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if _, exists := result["dimensions"]; exists {
		t.Error("dimensions key should be omitted when nil")
	}
}

// TestOpenAIEmbedRequest_IncludesExplicitDimensions verifies that the dimensions
// field is included when explicitly set.
func TestOpenAIEmbedRequest_IncludesExplicitDimensions(t *testing.T) {
	dim := 512
	req := openAIEmbedRequest{
		Model:      "text-embedding-3-small",
		Input:      []string{"test"},
		Dimensions: &dim,
	}

	jsonBytes, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	// Verify "dimensions" key is present with correct value
	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	dimVal, exists := result["dimensions"]
	if !exists {
		t.Fatal("dimensions key should be present when explicitly set")
	}

	if int(dimVal.(float64)) != 512 {
		t.Errorf("expected dimensions 512, got %v", dimVal)
	}
}

// TestOpenAIEmbedder_DimensionsMethod verifies the Dimensions() method behavior.
func TestOpenAIEmbedder_DimensionsMethod(t *testing.T) {
	tests := []struct {
		name       string
		dimensions *int
		expected   int
	}{
		{
			name:       "nil returns default (1536)",
			dimensions: nil,
			expected:   1536,
		},
		{
			name:       "explicit 512 returns 512",
			dimensions: intPtr(512),
			expected:   512,
		},
		{
			name:       "explicit 3072 returns 3072",
			dimensions: intPtr(3072),
			expected:   3072,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &OpenAIEmbedder{
				dimensions: tt.dimensions,
			}

			if e.Dimensions() != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, e.Dimensions())
			}
		})
	}
}

// TestBuildEmbedHTTPRequest_NilDimensions verifies that buildEmbedHTTPRequest
// does not include dimensions when nil.
func TestBuildEmbedHTTPRequest_NilDimensions(t *testing.T) {
	e := &OpenAIEmbedder{
		endpoint:   "https://api.example.com/v1",
		model:      "text-embedding-3-small",
		apiKey:     "test-api-key",
		dimensions: nil, // Should not be included in request
	}

	ctx := context.Background()
	req, err := e.buildEmbedHTTPRequest(ctx, []string{"hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body openAIEmbedRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode request body: %v", err)
	}

	if body.Dimensions != nil {
		t.Errorf("expected nil dimensions, got %v", body.Dimensions)
	}
}

func intPtr(v int) *int {
	return &v
}
