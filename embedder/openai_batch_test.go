package embedder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockEmbeddingResponse creates a valid OpenAI embedding response
func mockEmbeddingResponse(numInputs int) openAIEmbedResponse {
	resp := openAIEmbedResponse{}
	resp.Data = make([]struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	}, numInputs)

	for i := 0; i < numInputs; i++ {
		// Create a simple deterministic embedding based on index
		embedding := make([]float32, 3)
		embedding[0] = float32(i)
		embedding[1] = float32(i * 2)
		embedding[2] = float32(i * 3)
		resp.Data[i].Embedding = embedding
		resp.Data[i].Index = i
	}
	resp.Usage.PromptTokens = numInputs * 10
	resp.Usage.TotalTokens = numInputs * 10

	return resp
}

func TestOpenAIEmbedder_EmbedBatches_ParallelismLimit(t *testing.T) {
	var (
		maxConcurrent int32
		current       int32
		mu            sync.Mutex
		requestCount  int32
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Track concurrent requests
		c := atomic.AddInt32(&current, 1)
		defer atomic.AddInt32(&current, -1)

		mu.Lock()
		if c > maxConcurrent {
			maxConcurrent = c
		}
		mu.Unlock()

		atomic.AddInt32(&requestCount, 1)

		// Simulate some processing time to overlap requests
		time.Sleep(50 * time.Millisecond)

		// Parse request to get input count
		var req openAIEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp := mockEmbeddingResponse(len(req.Input))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	parallelism := 2
	e, err := NewOpenAIEmbedder(
		WithOpenAIKey("test-key"),
		WithOpenAIEndpoint(server.URL),
		WithOpenAIParallelism(parallelism),
		WithOpenAIDimensions(3),
	)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}

	// Create 4 batches to test parallelism limit
	batches := make([]Batch, 4)
	for i := range batches {
		batches[i] = Batch{
			Index: i,
			Entries: []BatchEntry{
				{FileIndex: i, ChunkIndex: 0, Content: "test content"},
			},
		}
	}

	ctx := context.Background()
	results, err := e.EmbedBatches(ctx, batches, nil)
	if err != nil {
		t.Fatalf("EmbedBatches failed: %v", err)
	}

	// Verify all batches processed
	if len(results) != len(batches) {
		t.Errorf("expected %d results, got %d", len(batches), len(results))
	}

	// Verify parallelism was respected
	if maxConcurrent > int32(parallelism) {
		t.Errorf("max concurrent %d exceeded parallelism limit %d", maxConcurrent, parallelism)
	}

	// Verify all requests were made
	if atomic.LoadInt32(&requestCount) != int32(len(batches)) {
		t.Errorf("expected %d requests, got %d", len(batches), requestCount)
	}
}

func TestOpenAIEmbedder_EmbedBatches_ResultMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openAIEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp := mockEmbeddingResponse(len(req.Input))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	e, err := NewOpenAIEmbedder(
		WithOpenAIKey("test-key"),
		WithOpenAIEndpoint(server.URL),
		WithOpenAIDimensions(3),
	)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}

	// Create batches from multiple files
	batches := []Batch{
		{
			Index: 0,
			Entries: []BatchEntry{
				{FileIndex: 0, ChunkIndex: 0, Content: "file0 chunk0"},
				{FileIndex: 0, ChunkIndex: 1, Content: "file0 chunk1"},
				{FileIndex: 1, ChunkIndex: 0, Content: "file1 chunk0"},
			},
		},
		{
			Index: 1,
			Entries: []BatchEntry{
				{FileIndex: 1, ChunkIndex: 1, Content: "file1 chunk1"},
				{FileIndex: 2, ChunkIndex: 0, Content: "file2 chunk0"},
			},
		},
	}

	ctx := context.Background()
	results, err := e.EmbedBatches(ctx, batches, nil)
	if err != nil {
		t.Fatalf("EmbedBatches failed: %v", err)
	}

	// Verify result count
	if len(results) != len(batches) {
		t.Errorf("expected %d results, got %d", len(batches), len(results))
	}

	// Verify each result has correct batch index and embedding count
	for _, result := range results {
		expectedCount := len(batches[result.BatchIndex].Entries)
		if len(result.Embeddings) != expectedCount {
			t.Errorf("batch %d: expected %d embeddings, got %d",
				result.BatchIndex, expectedCount, len(result.Embeddings))
		}
	}

	// Test MapResultsToFiles
	fileEmbeddings := MapResultsToFiles(batches, results, 3)
	if len(fileEmbeddings) != 3 {
		t.Errorf("expected 3 file embeddings, got %d", len(fileEmbeddings))
	}

	// File 0 should have 2 chunks
	if len(fileEmbeddings[0]) != 2 {
		t.Errorf("file 0: expected 2 chunks, got %d", len(fileEmbeddings[0]))
	}

	// File 1 should have 2 chunks
	if len(fileEmbeddings[1]) != 2 {
		t.Errorf("file 1: expected 2 chunks, got %d", len(fileEmbeddings[1]))
	}

	// File 2 should have 1 chunk
	if len(fileEmbeddings[2]) != 1 {
		t.Errorf("file 2: expected 1 chunk, got %d", len(fileEmbeddings[2]))
	}
}

func TestOpenAIEmbedder_EmbedBatches_RetryOn429(t *testing.T) {
	var requestCount int32
	rateLimitUntil := int32(2) // First 2 requests return 429

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)

		if count <= rateLimitUntil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"message": "Rate limit exceeded",
					"type":    "rate_limit_error",
				},
			})
			return
		}

		var req openAIEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp := mockEmbeddingResponse(len(req.Input))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Use a fast retry policy for testing
	fastRetryPolicy := RetryPolicy{
		BaseDelay:   10 * time.Millisecond,
		Multiplier:  2.0,
		MaxDelay:    100 * time.Millisecond,
		MaxAttempts: 5,
	}

	e, err := NewOpenAIEmbedder(
		WithOpenAIKey("test-key"),
		WithOpenAIEndpoint(server.URL),
		WithOpenAIParallelism(1), // Sequential to make retry counting predictable
		WithOpenAIRetryPolicy(fastRetryPolicy),
		WithOpenAIDimensions(3),
	)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}

	batches := []Batch{
		{
			Index:   0,
			Entries: []BatchEntry{{FileIndex: 0, ChunkIndex: 0, Content: "test"}},
		},
	}

	var retryCount int32
	progress := func(batchIndex, totalBatches int, retrying bool, attempt int) {
		if retrying {
			atomic.AddInt32(&retryCount, 1)
		}
	}

	ctx := context.Background()
	results, err := e.EmbedBatches(ctx, batches, progress)
	if err != nil {
		t.Fatalf("EmbedBatches failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	// Should have retried twice (2 rate limits)
	if atomic.LoadInt32(&retryCount) != 2 {
		t.Errorf("expected 2 retries, got %d", retryCount)
	}

	// Total requests should be 3 (2 failures + 1 success)
	if atomic.LoadInt32(&requestCount) != 3 {
		t.Errorf("expected 3 requests, got %d", requestCount)
	}
}

func TestOpenAIEmbedder_EmbedBatches_FailOn4xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"message": "Invalid API key",
				"type":    "invalid_request_error",
			},
		})
	}))
	defer server.Close()

	e, err := NewOpenAIEmbedder(
		WithOpenAIKey("invalid-key"),
		WithOpenAIEndpoint(server.URL),
		WithOpenAIDimensions(3),
	)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}

	batches := []Batch{
		{
			Index:   0,
			Entries: []BatchEntry{{FileIndex: 0, ChunkIndex: 0, Content: "test"}},
		},
	}

	ctx := context.Background()
	_, err = e.EmbedBatches(ctx, batches, nil)
	if err == nil {
		t.Fatal("expected error for 401 response")
	}

	// Verify it's identified as non-retryable
	retryErr, ok := err.(*RetryableError)
	if !ok {
		t.Fatalf("expected RetryableError, got %T", err)
	}
	if retryErr.Retryable {
		t.Error("401 error should not be retryable")
	}
}

func TestOpenAIEmbedder_EmbedBatches_ContextCancellation(t *testing.T) {
	requestStarted := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		// Simulate slow response
		time.Sleep(5 * time.Second)

		var req openAIEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := mockEmbeddingResponse(len(req.Input))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	e, err := NewOpenAIEmbedder(
		WithOpenAIKey("test-key"),
		WithOpenAIEndpoint(server.URL),
		WithOpenAIDimensions(3),
	)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}

	batches := []Batch{
		{
			Index:   0,
			Entries: []BatchEntry{{FileIndex: 0, ChunkIndex: 0, Content: "test"}},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	errChan := make(chan error, 1)
	go func() {
		_, err := e.EmbedBatches(ctx, batches, nil)
		errChan <- err
	}()

	// Wait for request to start, then cancel
	<-requestStarted
	cancel()

	// Should get context cancellation error
	select {
	case err := <-errChan:
		if err == nil {
			t.Error("expected error after context cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for cancellation")
	}
}

func TestOpenAIEmbedder_EmbedBatches_EmptyInput(t *testing.T) {
	e, err := NewOpenAIEmbedder(
		WithOpenAIKey("test-key"),
		WithOpenAIDimensions(3),
	)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}

	ctx := context.Background()
	results, err := e.EmbedBatches(ctx, nil, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for empty input, got %v", results)
	}

	results, err = e.EmbedBatches(ctx, []Batch{}, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for empty input, got %v", results)
	}
}

func TestOpenAIEmbedder_EmbedBatches_ProgressCallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openAIEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := mockEmbeddingResponse(len(req.Input))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	e, err := NewOpenAIEmbedder(
		WithOpenAIKey("test-key"),
		WithOpenAIEndpoint(server.URL),
		WithOpenAIParallelism(1), // Sequential for predictable progress
		WithOpenAIDimensions(3),
	)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}

	batches := make([]Batch, 3)
	for i := range batches {
		batches[i] = Batch{
			Index:   i,
			Entries: []BatchEntry{{FileIndex: i, ChunkIndex: 0, Content: "test"}},
		}
	}

	type progressInfo struct {
		batchIndex   int
		totalBatches int
		retrying     bool
		attempt      int
	}
	var progressCalls []progressInfo
	var mu sync.Mutex
	progress := func(batchIndex, totalBatches int, retrying bool, attempt int) {
		mu.Lock()
		progressCalls = append(progressCalls, progressInfo{
			batchIndex:   batchIndex,
			totalBatches: totalBatches,
			retrying:     retrying,
			attempt:      attempt,
		})
		mu.Unlock()
	}

	ctx := context.Background()
	_, err = e.EmbedBatches(ctx, batches, progress)
	if err != nil {
		t.Fatalf("EmbedBatches failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// Should have 3 progress calls (one per batch completion)
	if len(progressCalls) != 3 {
		t.Errorf("expected 3 progress calls, got %d", len(progressCalls))
	}

	// All should report totalBatches = 3 and not retrying
	for _, call := range progressCalls {
		if call.totalBatches != 3 {
			t.Errorf("expected totalBatches=3, got %d", call.totalBatches)
		}
		if call.retrying {
			t.Error("unexpected retry flag")
		}
	}
}

func TestOpenAIEmbedder_WithParallelism(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")

	tests := []struct {
		name        string
		parallelism int
		expected    int
	}{
		{"default", 0, defaultParallelism},
		{"explicit 1", 1, 1},
		{"explicit 2", 2, 2},
		{"explicit 8", 8, 8},
		{"negative ignored", -1, defaultParallelism},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e *OpenAIEmbedder
			var err error
			if tt.parallelism == 0 {
				e, err = NewOpenAIEmbedder()
			} else {
				e, err = NewOpenAIEmbedder(WithOpenAIParallelism(tt.parallelism))
			}
			if err != nil {
				t.Fatalf("failed to create embedder: %v", err)
			}

			if e.parallelism != tt.expected {
				t.Errorf("expected parallelism %d, got %d", tt.expected, e.parallelism)
			}
		})
	}
}

func TestOpenAIEmbedder_EmbedBatches_RetryOn5xx(t *testing.T) {
	var requestCount int32
	serverErrorUntil := int32(2) // First 2 requests return 503

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)

		if count <= serverErrorUntil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"message": "Service temporarily unavailable",
					"type":    "server_error",
				},
			})
			return
		}

		var req openAIEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp := mockEmbeddingResponse(len(req.Input))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Use a fast retry policy for testing
	fastRetryPolicy := RetryPolicy{
		BaseDelay:   10 * time.Millisecond,
		Multiplier:  2.0,
		MaxDelay:    100 * time.Millisecond,
		MaxAttempts: 5,
	}

	e, err := NewOpenAIEmbedder(
		WithOpenAIKey("test-key"),
		WithOpenAIEndpoint(server.URL),
		WithOpenAIParallelism(1),
		WithOpenAIRetryPolicy(fastRetryPolicy),
		WithOpenAIDimensions(3),
	)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}

	batches := []Batch{
		{
			Index:   0,
			Entries: []BatchEntry{{FileIndex: 0, ChunkIndex: 0, Content: "test"}},
		},
	}

	var retryCount int32
	progress := func(batchIndex, totalBatches int, retrying bool, attempt int) {
		if retrying {
			atomic.AddInt32(&retryCount, 1)
		}
	}

	ctx := context.Background()
	results, err := e.EmbedBatches(ctx, batches, progress)
	if err != nil {
		t.Fatalf("EmbedBatches failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	// Should have retried twice (2 server errors)
	if atomic.LoadInt32(&retryCount) != 2 {
		t.Errorf("expected 2 retries, got %d", retryCount)
	}

	// Total requests should be 3 (2 failures + 1 success)
	if atomic.LoadInt32(&requestCount) != 3 {
		t.Errorf("expected 3 requests, got %d", requestCount)
	}
}

func TestOpenAIEmbedder_EmbedBatches_MaxRetryLimit(t *testing.T) {
	var requestCount int32

	// Server always returns 429
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"message": "Rate limit exceeded",
				"type":    "rate_limit_error",
			},
		})
	}))
	defer server.Close()

	// Use a fast retry policy with 3 max attempts for testing
	fastRetryPolicy := RetryPolicy{
		BaseDelay:   5 * time.Millisecond,
		Multiplier:  2.0,
		MaxDelay:    50 * time.Millisecond,
		MaxAttempts: 3,
	}

	e, err := NewOpenAIEmbedder(
		WithOpenAIKey("test-key"),
		WithOpenAIEndpoint(server.URL),
		WithOpenAIParallelism(1),
		WithOpenAIRetryPolicy(fastRetryPolicy),
		WithOpenAIDimensions(3),
	)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}

	batches := []Batch{
		{
			Index:   0,
			Entries: []BatchEntry{{FileIndex: 0, ChunkIndex: 0, Content: "test"}},
		},
	}

	ctx := context.Background()
	_, err = e.EmbedBatches(ctx, batches, nil)
	if err == nil {
		t.Fatal("expected error after max retries")
	}

	// Should have made MaxAttempts+1 requests (initial attempt + retries)
	expectedRequests := int32(fastRetryPolicy.MaxAttempts + 1)
	if atomic.LoadInt32(&requestCount) != expectedRequests {
		t.Errorf("expected %d requests (1 initial + %d retries), got %d",
			expectedRequests, fastRetryPolicy.MaxAttempts, requestCount)
	}

	// Error message should indicate batch failure
	if !strings.Contains(err.Error(), "batch 0 failed") {
		t.Errorf("expected error to mention batch failure, got: %v", err)
	}
}

func TestOpenAIEmbedder_EmbedBatches_ParallelBatchFailure(t *testing.T) {
	var requestCount int32

	// Server fails on batch 1 (second batch)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)

		var req openAIEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Fail on requests that look like they're from batch 1 (content contains "batch1")
		for _, input := range req.Input {
			if strings.Contains(input, "batch1") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]string{
						"message": "Invalid API key",
						"type":    "invalid_request_error",
					},
				})
				return
			}
		}

		// Add small delay to ensure batches overlap
		if count == 1 {
			time.Sleep(20 * time.Millisecond)
		}

		resp := mockEmbeddingResponse(len(req.Input))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	e, err := NewOpenAIEmbedder(
		WithOpenAIKey("test-key"),
		WithOpenAIEndpoint(server.URL),
		WithOpenAIParallelism(2), // Process 2 batches in parallel
		WithOpenAIDimensions(3),
	)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}

	batches := []Batch{
		{
			Index:   0,
			Entries: []BatchEntry{{FileIndex: 0, ChunkIndex: 0, Content: "batch0 content"}},
		},
		{
			Index:   1,
			Entries: []BatchEntry{{FileIndex: 1, ChunkIndex: 0, Content: "batch1 content"}},
		},
	}

	ctx := context.Background()
	_, err = e.EmbedBatches(ctx, batches, nil)
	if err == nil {
		t.Fatal("expected error when batch fails")
	}

	// Verify the error is from the failed batch
	retryErr, ok := err.(*RetryableError)
	if !ok {
		t.Fatalf("expected RetryableError, got %T: %v", err, err)
	}
	if retryErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", retryErr.StatusCode)
	}
}
