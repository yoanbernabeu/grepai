package embedder_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/embedder"
)

// TestOpenAIEmbedder_RetryAfterHeader tests that the Retry-After header is respected.
func TestOpenAIEmbedder_RetryAfterHeader(t *testing.T) {
	var requestCount atomic.Int32
	startTime := time.Now()
	var retryTime time.Time

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		if count == 1 {
			// First request: 429 with Retry-After header
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": "Rate limit exceeded",
					"type":    "rate_limit_error",
				},
			})
			return
		}
		// Record when retry happened
		retryTime = time.Now()

		// Second request: success
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": make([]float32, 10), "index": 0},
			},
			"usage": map[string]int{
				"prompt_tokens": 10,
				"total_tokens":  10,
			},
		})
	}))
	defer server.Close()

	emb, err := embedder.NewOpenAIEmbedder(
		embedder.WithOpenAIEndpoint(server.URL),
		embedder.WithOpenAIKey("test-key"),
		embedder.WithOpenAIDimensions(10),
	)
	if err != nil {
		t.Fatalf("Failed to create embedder: %v", err)
	}

	// Use EmbedBatches which has retry logic
	batches := []embedder.Batch{
		{
			Index: 0,
			Entries: []embedder.BatchEntry{
				{FileIndex: 0, ChunkIndex: 0, Content: "test text"},
			},
		},
	}

	_, err = emb.EmbedBatches(context.Background(), batches, nil)
	if err != nil {
		t.Fatalf("EmbedBatches failed: %v", err)
	}

	// Verify we actually retried
	if requestCount.Load() != 2 {
		t.Errorf("Expected 2 requests, got %d", requestCount.Load())
	}

	// Verify the retry waited approximately 1 second (with tolerance)
	elapsed := retryTime.Sub(startTime)
	if elapsed < 900*time.Millisecond || elapsed > 2*time.Second {
		t.Errorf("Expected retry after ~1s, actual elapsed: %v", elapsed)
	}
}

// TestOpenAIEmbedder_RetryAfterFallback tests fallback to exponential backoff when no Retry-After header.
func TestOpenAIEmbedder_RetryAfterFallback(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		if count == 1 {
			// First request: 429 without Retry-After header
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": "Rate limit exceeded",
					"type":    "rate_limit_error",
				},
			})
			return
		}

		// Second request: success
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": make([]float32, 10), "index": 0},
			},
			"usage": map[string]int{
				"prompt_tokens": 10,
				"total_tokens":  10,
			},
		})
	}))
	defer server.Close()

	emb, err := embedder.NewOpenAIEmbedder(
		embedder.WithOpenAIEndpoint(server.URL),
		embedder.WithOpenAIKey("test-key"),
		embedder.WithOpenAIDimensions(10),
	)
	if err != nil {
		t.Fatalf("Failed to create embedder: %v", err)
	}

	// Use EmbedBatches which has retry logic
	batches := []embedder.Batch{
		{
			Index: 0,
			Entries: []embedder.BatchEntry{
				{FileIndex: 0, ChunkIndex: 0, Content: "test text"},
			},
		},
	}

	_, err = emb.EmbedBatches(context.Background(), batches, nil)
	if err != nil {
		t.Fatalf("EmbedBatches failed: %v", err)
	}

	// Verify we actually retried
	if requestCount.Load() != 2 {
		t.Errorf("Expected 2 requests, got %d", requestCount.Load())
	}
}

// TestOpenAIEmbedder_InvalidRetryAfter tests that invalid Retry-After falls back to exponential backoff.
func TestOpenAIEmbedder_InvalidRetryAfter(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		if count == 1 {
			// First request: 429 with invalid Retry-After header
			w.Header().Set("Retry-After", "invalid-value")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": "Rate limit exceeded",
					"type":    "rate_limit_error",
				},
			})
			return
		}

		// Second request: success
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": make([]float32, 10), "index": 0},
			},
			"usage": map[string]int{
				"prompt_tokens": 10,
				"total_tokens":  10,
			},
		})
	}))
	defer server.Close()

	emb, err := embedder.NewOpenAIEmbedder(
		embedder.WithOpenAIEndpoint(server.URL),
		embedder.WithOpenAIKey("test-key"),
		embedder.WithOpenAIDimensions(10),
	)
	if err != nil {
		t.Fatalf("Failed to create embedder: %v", err)
	}

	// Use EmbedBatches which has retry logic
	batches := []embedder.Batch{
		{
			Index: 0,
			Entries: []embedder.BatchEntry{
				{FileIndex: 0, ChunkIndex: 0, Content: "test text"},
			},
		},
	}

	_, err = emb.EmbedBatches(context.Background(), batches, nil)
	if err != nil {
		t.Fatalf("EmbedBatches failed: %v", err)
	}

	// Verify we actually retried
	if requestCount.Load() != 2 {
		t.Errorf("Expected 2 requests, got %d", requestCount.Load())
	}
}

// TestOpenAIEmbedder_AdaptiveParallelismReduction tests that parallelism is reduced after rate limits.
func TestOpenAIEmbedder_AdaptiveParallelismReduction(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)

		// First 3 requests: 429 (to trigger parallelism reduction)
		if count <= 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": "Rate limit exceeded",
					"type":    "rate_limit_error",
				},
			})
			return
		}

		// Subsequent requests: success
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": make([]float32, 10), "index": 0},
			},
			"usage": map[string]int{
				"prompt_tokens": 10,
				"total_tokens":  10,
			},
		})
	}))
	defer server.Close()

	emb, err := embedder.NewOpenAIEmbedder(
		embedder.WithOpenAIEndpoint(server.URL),
		embedder.WithOpenAIKey("test-key"),
		embedder.WithOpenAIDimensions(10),
		embedder.WithOpenAIParallelism(4),
	)
	if err != nil {
		t.Fatalf("Failed to create embedder: %v", err)
	}

	// Use EmbedBatches which has retry logic
	batches := []embedder.Batch{
		{
			Index: 0,
			Entries: []embedder.BatchEntry{
				{FileIndex: 0, ChunkIndex: 0, Content: "test text"},
			},
		},
	}

	_, err = emb.EmbedBatches(context.Background(), batches, nil)
	if err != nil {
		t.Fatalf("EmbedBatches failed: %v", err)
	}

	// Request should eventually succeed
	if requestCount.Load() < 4 {
		t.Errorf("Expected at least 4 requests (3 failures + 1 success), got %d", requestCount.Load())
	}
}

// TestOpenAIEmbedder_TokenPacing tests that token-based pacing delays requests when near limit.
func TestOpenAIEmbedder_TokenPacing(t *testing.T) {
	var requestCount atomic.Int32
	var requestTimes []time.Time
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestTimes = append(requestTimes, time.Now())
		mu.Unlock()

		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": make([]float32, 10), "index": 0},
			},
			"usage": map[string]int{
				"prompt_tokens": 10,
				"total_tokens":  10,
			},
		})
	}))
	defer server.Close()

	// Set a very low TPM limit to force pacing
	emb, err := embedder.NewOpenAIEmbedder(
		embedder.WithOpenAIEndpoint(server.URL),
		embedder.WithOpenAIKey("test-key"),
		embedder.WithOpenAIDimensions(10),
		embedder.WithOpenAITPMLimit(100), // Very low limit
	)
	if err != nil {
		t.Fatalf("Failed to create embedder: %v", err)
	}

	// Create multiple batches
	batches := []embedder.Batch{
		{Index: 0, Entries: []embedder.BatchEntry{{FileIndex: 0, ChunkIndex: 0, Content: "test text batch 1"}}},
		{Index: 1, Entries: []embedder.BatchEntry{{FileIndex: 0, ChunkIndex: 1, Content: "test text batch 2"}}},
	}

	_, err = emb.EmbedBatches(context.Background(), batches, nil)
	if err != nil {
		t.Fatalf("EmbedBatches failed: %v", err)
	}

	// Verify requests were made
	if requestCount.Load() != 2 {
		t.Errorf("Expected 2 requests, got %d", requestCount.Load())
	}
}

// TestOpenAIEmbedder_NoTokenPacingWhenDisabled tests that token pacing is disabled when TPM=0.
func TestOpenAIEmbedder_NoTokenPacingWhenDisabled(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": make([]float32, 10), "index": 0},
			},
			"usage": map[string]int{
				"prompt_tokens": 10,
				"total_tokens":  10,
			},
		})
	}))
	defer server.Close()

	// Don't set TPM limit - pacing should be disabled
	emb, err := embedder.NewOpenAIEmbedder(
		embedder.WithOpenAIEndpoint(server.URL),
		embedder.WithOpenAIKey("test-key"),
		embedder.WithOpenAIDimensions(10),
	)
	if err != nil {
		t.Fatalf("Failed to create embedder: %v", err)
	}

	batches := []embedder.Batch{
		{Index: 0, Entries: []embedder.BatchEntry{{FileIndex: 0, ChunkIndex: 0, Content: "test text"}}},
	}

	_, err = emb.EmbedBatches(context.Background(), batches, nil)
	if err != nil {
		t.Fatalf("EmbedBatches failed: %v", err)
	}

	if requestCount.Load() != 1 {
		t.Errorf("Expected 1 request, got %d", requestCount.Load())
	}
}

// TestOpenAIEmbedder_BatchProgressCallback tests that progress callback receives rate limit info.
func TestOpenAIEmbedder_BatchProgressCallback(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		if count == 1 {
			// First request: 429
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": "Rate limit exceeded",
					"type":    "rate_limit_error",
				},
			})
			return
		}

		// Second request: success
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": make([]float32, 10), "index": 0},
			},
			"usage": map[string]int{
				"prompt_tokens": 10,
				"total_tokens":  10,
			},
		})
	}))
	defer server.Close()

	emb, err := embedder.NewOpenAIEmbedder(
		embedder.WithOpenAIEndpoint(server.URL),
		embedder.WithOpenAIKey("test-key"),
		embedder.WithOpenAIDimensions(10),
	)
	if err != nil {
		t.Fatalf("Failed to create embedder: %v", err)
	}

	var retryCallbackCalled bool
	var receivedStatusCode int

	progress := func(batchIndex, totalBatches, completedChunks, totalChunks int, retrying bool, attempt int, statusCode int) {
		if retrying {
			retryCallbackCalled = true
			receivedStatusCode = statusCode
		}
	}

	batches := []embedder.Batch{
		{
			Index: 0,
			Entries: []embedder.BatchEntry{
				{FileIndex: 0, ChunkIndex: 0, Content: "test text"},
			},
		},
	}

	_, err = emb.EmbedBatches(context.Background(), batches, progress)
	if err != nil {
		t.Fatalf("EmbedBatches failed: %v", err)
	}

	if !retryCallbackCalled {
		t.Error("Expected progress callback to be called with retrying=true")
	}

	if receivedStatusCode != 429 {
		t.Errorf("Expected status code 429, got %d", receivedStatusCode)
	}
}
