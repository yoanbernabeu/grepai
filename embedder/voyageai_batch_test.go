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

func TestVoyageAIEmbedder_EmbedBatches_ParallelismLimit(t *testing.T) {
	var (
		maxConcurrent int32
		current       int32
		mu            sync.Mutex
		requestCount  int32
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := atomic.AddInt32(&current, 1)
		defer atomic.AddInt32(&current, -1)

		mu.Lock()
		if c > maxConcurrent {
			maxConcurrent = c
		}
		mu.Unlock()

		atomic.AddInt32(&requestCount, 1)

		time.Sleep(50 * time.Millisecond)

		var req voyageAIEmbedRequest
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
	e, err := NewVoyageAIEmbedder(
		WithVoyageAIKey("test-key"),
		WithVoyageAIEndpoint(server.URL),
		WithVoyageAIParallelism(parallelism),
		WithVoyageAIDimensions(3),
	)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}

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

	if len(results) != len(batches) {
		t.Errorf("expected %d results, got %d", len(batches), len(results))
	}

	if maxConcurrent > int32(parallelism) {
		t.Errorf("max concurrent %d exceeded parallelism limit %d", maxConcurrent, parallelism)
	}

	if atomic.LoadInt32(&requestCount) != int32(len(batches)) {
		t.Errorf("expected %d requests, got %d", len(batches), requestCount)
	}
}

func TestVoyageAIEmbedder_EmbedBatches_RequestFormat(t *testing.T) {
	// Verify that the actual HTTP request uses "output_dimension" not "dimensions"
	var capturedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		capturedBody = body

		resp := mockEmbeddingResponse(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	dims := 512
	e, err := NewVoyageAIEmbedder(
		WithVoyageAIKey("test-key"),
		WithVoyageAIEndpoint(server.URL),
		WithVoyageAIDimensions(dims),
		WithVoyageAIInputType("document"),
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
	if err != nil {
		t.Fatalf("EmbedBatches failed: %v", err)
	}

	// Parse the captured request body
	var parsed map[string]interface{}
	if err := json.Unmarshal(capturedBody, &parsed); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}

	// Must have "output_dimension", not "dimensions"
	if _, ok := parsed["dimensions"]; ok {
		t.Errorf("request body should NOT contain 'dimensions', got: %s", string(capturedBody))
	}

	if val, ok := parsed["output_dimension"]; !ok {
		t.Errorf("request body should contain 'output_dimension', got: %s", string(capturedBody))
	} else if int(val.(float64)) != dims {
		t.Errorf("expected output_dimension=%d, got %v", dims, val)
	}

	// Check input_type is set
	if val, ok := parsed["input_type"]; !ok {
		t.Errorf("request body should contain 'input_type', got: %s", string(capturedBody))
	} else if val != "document" {
		t.Errorf("expected input_type='document', got %v", val)
	}
}

func TestVoyageAIEmbedder_EmbedBatches_ResultMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req voyageAIEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp := mockEmbeddingResponse(len(req.Input))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	e, err := NewVoyageAIEmbedder(
		WithVoyageAIKey("test-key"),
		WithVoyageAIEndpoint(server.URL),
		WithVoyageAIDimensions(3),
	)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}

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

	if len(results) != len(batches) {
		t.Errorf("expected %d results, got %d", len(batches), len(results))
	}

	for _, result := range results {
		expectedCount := len(batches[result.BatchIndex].Entries)
		if len(result.Embeddings) != expectedCount {
			t.Errorf("batch %d: expected %d embeddings, got %d",
				result.BatchIndex, expectedCount, len(result.Embeddings))
		}
	}

	fileEmbeddings := MapResultsToFiles(batches, results, 3)
	if len(fileEmbeddings) != 3 {
		t.Errorf("expected 3 file embeddings, got %d", len(fileEmbeddings))
	}

	if len(fileEmbeddings[0]) != 2 {
		t.Errorf("file 0: expected 2 chunks, got %d", len(fileEmbeddings[0]))
	}

	if len(fileEmbeddings[1]) != 2 {
		t.Errorf("file 1: expected 2 chunks, got %d", len(fileEmbeddings[1]))
	}

	if len(fileEmbeddings[2]) != 1 {
		t.Errorf("file 2: expected 1 chunk, got %d", len(fileEmbeddings[2]))
	}
}

func TestVoyageAIEmbedder_EmbedBatches_RetryOn429(t *testing.T) {
	var requestCount int32
	rateLimitUntil := int32(2)

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

		var req voyageAIEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp := mockEmbeddingResponse(len(req.Input))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fastRetryPolicy := RetryPolicy{
		BaseDelay:   10 * time.Millisecond,
		Multiplier:  2.0,
		MaxDelay:    100 * time.Millisecond,
		MaxAttempts: 5,
	}

	e, err := NewVoyageAIEmbedder(
		WithVoyageAIKey("test-key"),
		WithVoyageAIEndpoint(server.URL),
		WithVoyageAIParallelism(1),
		WithVoyageAIRetryPolicy(fastRetryPolicy),
		WithVoyageAIDimensions(3),
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
	progress := func(batchIndex, totalBatches, completedChunks, totalChunks int, retrying bool, attempt int, statusCode int) {
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

	if atomic.LoadInt32(&retryCount) != 2 {
		t.Errorf("expected 2 retries, got %d", retryCount)
	}

	if atomic.LoadInt32(&requestCount) != 3 {
		t.Errorf("expected 3 requests, got %d", requestCount)
	}
}

func TestVoyageAIEmbedder_EmbedBatches_FailOn4xx(t *testing.T) {
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

	e, err := NewVoyageAIEmbedder(
		WithVoyageAIKey("invalid-key"),
		WithVoyageAIEndpoint(server.URL),
		WithVoyageAIDimensions(3),
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

	retryErr, ok := err.(*RetryableError)
	if !ok {
		t.Fatalf("expected RetryableError, got %T", err)
	}
	if retryErr.Retryable {
		t.Error("401 error should not be retryable")
	}
}

func TestVoyageAIEmbedder_EmbedBatches_ContextCancellation(t *testing.T) {
	requestStarted := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		time.Sleep(5 * time.Second)

		var req voyageAIEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := mockEmbeddingResponse(len(req.Input))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	e, err := NewVoyageAIEmbedder(
		WithVoyageAIKey("test-key"),
		WithVoyageAIEndpoint(server.URL),
		WithVoyageAIDimensions(3),
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

	<-requestStarted
	cancel()

	select {
	case err := <-errChan:
		if err == nil {
			t.Error("expected error after context cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for cancellation")
	}
}

func TestVoyageAIEmbedder_EmbedBatches_EmptyInput(t *testing.T) {
	e, err := NewVoyageAIEmbedder(
		WithVoyageAIKey("test-key"),
		WithVoyageAIDimensions(3),
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

func TestVoyageAIEmbedder_EmbedBatches_ProgressCallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req voyageAIEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := mockEmbeddingResponse(len(req.Input))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	e, err := NewVoyageAIEmbedder(
		WithVoyageAIKey("test-key"),
		WithVoyageAIEndpoint(server.URL),
		WithVoyageAIParallelism(1),
		WithVoyageAIDimensions(3),
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
		batchIndex      int
		totalBatches    int
		completedChunks int
		totalChunks     int
		retrying        bool
		attempt         int
	}
	var progressCalls []progressInfo
	var mu sync.Mutex
	progress := func(batchIndex, totalBatches, completedChunks, totalChunks int, retrying bool, attempt int, statusCode int) {
		mu.Lock()
		progressCalls = append(progressCalls, progressInfo{
			batchIndex:      batchIndex,
			totalBatches:    totalBatches,
			completedChunks: completedChunks,
			totalChunks:     totalChunks,
			retrying:        retrying,
			attempt:         attempt,
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

	if len(progressCalls) != 3 {
		t.Errorf("expected 3 progress calls, got %d", len(progressCalls))
	}

	for _, call := range progressCalls {
		if call.totalBatches != 3 {
			t.Errorf("expected totalBatches=3, got %d", call.totalBatches)
		}
		if call.retrying {
			t.Error("unexpected retry flag")
		}
	}
}

func TestVoyageAIEmbedder_WithParallelism(t *testing.T) {
	t.Setenv("VOYAGE_API_KEY", "test-key")

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
			var e *VoyageAIEmbedder
			var err error
			if tt.parallelism == 0 {
				e, err = NewVoyageAIEmbedder()
			} else {
				e, err = NewVoyageAIEmbedder(WithVoyageAIParallelism(tt.parallelism))
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

func TestVoyageAIEmbedder_EmbedBatches_RetryOn5xx(t *testing.T) {
	var requestCount int32
	serverErrorUntil := int32(2)

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

		var req voyageAIEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp := mockEmbeddingResponse(len(req.Input))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fastRetryPolicy := RetryPolicy{
		BaseDelay:   10 * time.Millisecond,
		Multiplier:  2.0,
		MaxDelay:    100 * time.Millisecond,
		MaxAttempts: 5,
	}

	e, err := NewVoyageAIEmbedder(
		WithVoyageAIKey("test-key"),
		WithVoyageAIEndpoint(server.URL),
		WithVoyageAIParallelism(1),
		WithVoyageAIRetryPolicy(fastRetryPolicy),
		WithVoyageAIDimensions(3),
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
	progress := func(batchIndex, totalBatches, completedChunks, totalChunks int, retrying bool, attempt int, statusCode int) {
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

	if atomic.LoadInt32(&retryCount) != 2 {
		t.Errorf("expected 2 retries, got %d", retryCount)
	}

	if atomic.LoadInt32(&requestCount) != 3 {
		t.Errorf("expected 3 requests, got %d", requestCount)
	}
}

func TestVoyageAIEmbedder_EmbedBatches_MaxRetryLimit(t *testing.T) {
	var requestCount int32

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

	fastRetryPolicy := RetryPolicy{
		BaseDelay:   5 * time.Millisecond,
		Multiplier:  2.0,
		MaxDelay:    50 * time.Millisecond,
		MaxAttempts: 3,
	}

	e, err := NewVoyageAIEmbedder(
		WithVoyageAIKey("test-key"),
		WithVoyageAIEndpoint(server.URL),
		WithVoyageAIParallelism(1),
		WithVoyageAIRetryPolicy(fastRetryPolicy),
		WithVoyageAIDimensions(3),
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

	expectedRequests := int32(fastRetryPolicy.MaxAttempts + 1)
	if atomic.LoadInt32(&requestCount) != expectedRequests {
		t.Errorf("expected %d requests (1 initial + %d retries), got %d",
			expectedRequests, fastRetryPolicy.MaxAttempts, requestCount)
	}

	if !strings.Contains(err.Error(), "batch 0 failed") {
		t.Errorf("expected error to mention batch failure, got: %v", err)
	}
}

func TestVoyageAIEmbedder_EmbedBatches_ParallelBatchFailure(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)

		var req voyageAIEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

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

		if count == 1 {
			time.Sleep(20 * time.Millisecond)
		}

		resp := mockEmbeddingResponse(len(req.Input))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	e, err := NewVoyageAIEmbedder(
		WithVoyageAIKey("test-key"),
		WithVoyageAIEndpoint(server.URL),
		WithVoyageAIParallelism(2),
		WithVoyageAIDimensions(3),
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

	retryErr, ok := err.(*RetryableError)
	if !ok {
		t.Fatalf("expected RetryableError, got %T: %v", err, err)
	}
	if retryErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", retryErr.StatusCode)
	}
}
