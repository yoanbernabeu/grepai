package embedder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestOllamaEmbedder_EmbedBatch_sends_one_modern_request(t *testing.T) {
	// Given
	var requestCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.URL.Path != "/api/embed" {
			http.NotFound(w, r)
			return
		}

		var request struct {
			Model    string   `json:"model"`
			Input    []string `json:"input"`
			Truncate *bool    `json:"truncate"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if request.Model != "test-model" || !reflect.DeepEqual(request.Input, []string{"first", "second"}) {
			http.Error(w, "unexpected request payload", http.StatusBadRequest)
			return
		}
		if request.Truncate == nil || *request.Truncate {
			http.Error(w, "truncate must be explicitly false", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string][][]float32{
			"embeddings": {{1, 2}, {3, 4}},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	embedder := NewOllamaEmbedder(
		WithOllamaEndpoint(server.URL),
		WithOllamaModel("test-model"),
	)

	// When
	embeddings, err := embedder.EmbedBatch(context.Background(), []string{"first", "second"})

	// Then
	if err != nil {
		t.Fatalf("EmbedBatch() error = %v", err)
	}
	if requestCount.Load() != 1 {
		t.Fatalf("request count = %d, want 1", requestCount.Load())
	}
	want := [][]float32{{1, 2}, {3, 4}}
	if !reflect.DeepEqual(embeddings, want) {
		t.Fatalf("EmbedBatch() = %v, want %v", embeddings, want)
	}
}

func TestOllamaEmbedder_EmbedBatch_splits_oversized_requests(t *testing.T) {
	// Given
	var mutex sync.Mutex
	requestSizes := make([]int, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request ollamaBatchEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mutex.Lock()
		requestSizes = append(requestSizes, len(request.Input))
		mutex.Unlock()

		embeddings := make([][]float32, len(request.Input))
		for i := range request.Input {
			embeddings[i] = []float32{float32(i + 1)}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(ollamaBatchEmbedResponse{Embeddings: embeddings}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	embedder := NewOllamaEmbedder(WithOllamaEndpoint(server.URL))
	texts := make([]string, 65)
	for i := range texts {
		texts[i] = "text"
	}

	// When
	embeddings, err := embedder.EmbedBatch(context.Background(), texts)

	// Then
	if err != nil {
		t.Fatalf("EmbedBatch() error = %v", err)
	}
	mutex.Lock()
	gotSizes := append([]int(nil), requestSizes...)
	mutex.Unlock()
	if !reflect.DeepEqual(gotSizes, []int{64, 1}) {
		t.Fatalf("request sizes = %v, want [64 1]", gotSizes)
	}
	if len(embeddings) != len(texts) {
		t.Fatalf("embedding count = %d, want %d", len(embeddings), len(texts))
	}
}

func TestOllamaEmbedder_EmbedBatch_reports_the_failing_context_index(t *testing.T) {
	// Given
	var requestCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		var request ollamaBatchEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(request.Input) > 1 || request.Input[0] == "too long" {
			http.Error(w, "input exceeds the context length", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(ollamaBatchEmbedResponse{
			Embeddings: [][]float32{{1, 2}},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	embedder := NewOllamaEmbedder(WithOllamaEndpoint(server.URL))

	// When
	_, err := embedder.EmbedBatch(context.Background(), []string{"ok", "too long", "also ok"})

	// Then
	contextError := AsContextLengthError(err)
	if contextError == nil {
		t.Fatalf("EmbedBatch() error = %v, want ContextLengthError", err)
	}
	if contextError.ChunkIndex != 1 {
		t.Fatalf("context error chunk index = %d, want 1", contextError.ChunkIndex)
	}
	if contextError.EstimatedTokens != len("too long")/4 {
		t.Fatalf("estimated tokens = %d, want %d", contextError.EstimatedTokens, len("too long")/4)
	}
	if requestCount.Load() != 3 {
		t.Fatalf("request count = %d, want 3", requestCount.Load())
	}
}

func TestOllamaEmbedder_EmbedBatch_rejects_invalid_response_vectors(t *testing.T) {
	tests := []struct {
		name       string
		response   ollamaBatchEmbedResponse
		wantErrMsg string
	}{
		{
			name:       "missing vector",
			response:   ollamaBatchEmbedResponse{Embeddings: [][]float32{{1}}},
			wantErrMsg: "expected 2 embeddings, got 1",
		},
		{
			name:       "empty vector",
			response:   ollamaBatchEmbedResponse{Embeddings: [][]float32{{1}, {}}},
			wantErrMsg: "empty embedding for text 1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(test.response); err != nil {
					t.Errorf("encode response: %v", err)
				}
			}))
			t.Cleanup(server.Close)
			embedder := NewOllamaEmbedder(WithOllamaEndpoint(server.URL))

			// When
			_, err := embedder.EmbedBatch(context.Background(), []string{"first", "second"})

			// Then
			if err == nil || !strings.Contains(err.Error(), test.wantErrMsg) {
				t.Fatalf("EmbedBatch() error = %v, want message %q", err, test.wantErrMsg)
			}
		})
	}
}

func TestOllamaEmbedder_EmbedBatch_empty_input_makes_no_request(t *testing.T) {
	// Given
	var requestCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requestCount.Add(1)
	}))
	t.Cleanup(server.Close)
	embedder := NewOllamaEmbedder(WithOllamaEndpoint(server.URL))

	// When
	embeddings, err := embedder.EmbedBatch(context.Background(), nil)

	// Then
	if err != nil {
		t.Fatalf("EmbedBatch() error = %v", err)
	}
	if len(embeddings) != 0 {
		t.Fatalf("embedding count = %d, want 0", len(embeddings))
	}
	if requestCount.Load() != 0 {
		t.Fatalf("request count = %d, want 0", requestCount.Load())
	}
}

func TestIsOllamaContextLengthError_recognizes_supported_messages(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{name: "context length", message: "input exceeds the context length", want: true},
		{name: "context window", message: "input exceeds the context window", want: true},
		{name: "token count", message: "too many tokens", want: true},
		{name: "modern prompt limit", message: "prompt is too long: 3000 tokens > 2048 maximum", want: true},
		{name: "unrelated server error", message: "model runner unavailable", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			got := isOllamaContextLengthError(test.message)

			// Then
			if got != test.want {
				t.Fatalf("isOllamaContextLengthError(%q) = %v, want %v", test.message, got, test.want)
			}
		})
	}
}
