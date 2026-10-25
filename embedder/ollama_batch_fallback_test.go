package embedder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOllamaEmbedder_EmbedBatch_falls_back_once_when_modern_endpoint_rejects_the_method(t *testing.T) {
	// Given
	var modernRequests atomic.Int64
	var legacyRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embed":
			modernRequests.Add(1)
			w.WriteHeader(http.StatusMethodNotAllowed)
		case "/api/embeddings":
			legacyRequests.Add(1)
			var request ollamaEmbedRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(ollamaEmbedResponse{
				Embedding: []float32{float32(len(request.Prompt))},
			}); err != nil {
				t.Errorf("encode response: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	embedder := NewOllamaEmbedder(WithOllamaEndpoint(server.URL))

	// When
	first, firstErr := embedder.EmbedBatch(context.Background(), []string{"one", "three"})
	second, secondErr := embedder.EmbedBatch(context.Background(), []string{"second"})

	// Then
	if firstErr != nil {
		t.Fatalf("first EmbedBatch() error = %v", firstErr)
	}
	if secondErr != nil {
		t.Fatalf("second EmbedBatch() error = %v", secondErr)
	}
	if modernRequests.Load() != 1 {
		t.Fatalf("modern request count = %d, want 1", modernRequests.Load())
	}
	if legacyRequests.Load() != 3 {
		t.Fatalf("legacy request count = %d, want 3", legacyRequests.Load())
	}
	if !reflect.DeepEqual(first, [][]float32{{3}, {5}}) {
		t.Fatalf("first EmbedBatch() = %v, want [[3] [5]]", first)
	}
	if !reflect.DeepEqual(second, [][]float32{{6}}) {
		t.Fatalf("second EmbedBatch() = %v, want [[6]]", second)
	}
}

func TestOllamaEmbedder_EmbedBatch_does_not_fallback_on_server_errors(t *testing.T) {
	// Given
	var legacyRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/embeddings" {
			legacyRequests.Add(1)
		}
		http.Error(w, "server unavailable", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	embedder := NewOllamaEmbedder(WithOllamaEndpoint(server.URL))

	// When
	_, err := embedder.EmbedBatch(context.Background(), []string{"text"})

	// Then
	if err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("EmbedBatch() error = %v, want status 500", err)
	}
	if legacyRequests.Load() != 0 {
		t.Fatalf("legacy request count = %d, want 0", legacyRequests.Load())
	}
}

func TestOllamaEmbedder_EmbedBatch_limits_server_error_bodies(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte(strings.Repeat("x", 128<<10))); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	embedder := NewOllamaEmbedder(WithOllamaEndpoint(server.URL))

	// When
	_, err := embedder.EmbedBatch(context.Background(), []string{"text"})

	// Then
	if err == nil {
		t.Fatal("EmbedBatch() error = nil, want server error")
	}
	if len(err.Error()) > (64<<10)+100 {
		t.Fatalf("error length = %d, want at most %d", len(err.Error()), (64<<10)+100)
	}
}

func TestOllamaEmbedder_EmbedBatch_does_not_fallback_when_model_is_missing(t *testing.T) {
	// Given
	var legacyRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/embeddings" {
			legacyRequests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(ollamaEmbedResponse{Embedding: []float32{1}}); err != nil {
				t.Errorf("encode response: %v", err)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		if _, err := w.Write([]byte(`{"error":"model not found"}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	embedder := NewOllamaEmbedder(WithOllamaEndpoint(server.URL))

	// When
	_, err := embedder.EmbedBatch(context.Background(), []string{"text"})

	// Then
	if err == nil || !strings.Contains(err.Error(), "model not found") {
		t.Fatalf("EmbedBatch() error = %v, want model-not-found error", err)
	}
	if legacyRequests.Load() != 0 {
		t.Fatalf("legacy request count = %d, want 0", legacyRequests.Load())
	}
}

func TestOllamaEmbedder_EmbedBatch_falls_back_for_json_endpoint_not_found(t *testing.T) {
	// Given
	var modernRequests atomic.Int64
	var legacyRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/embed" {
			modernRequests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			if _, err := w.Write([]byte(`{"error":"not found"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
			return
		}
		legacyRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(ollamaEmbedResponse{Embedding: []float32{1}}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	embedder := NewOllamaEmbedder(WithOllamaEndpoint(server.URL))

	// When
	_, firstErr := embedder.EmbedBatch(context.Background(), []string{"first"})
	_, secondErr := embedder.EmbedBatch(context.Background(), []string{"second"})

	// Then
	if firstErr != nil || secondErr != nil {
		t.Fatalf("EmbedBatch() errors = (%v, %v), want nil", firstErr, secondErr)
	}
	if modernRequests.Load() != 1 {
		t.Fatalf("modern request count = %d, want 1", modernRequests.Load())
	}
	if legacyRequests.Load() != 2 {
		t.Fatalf("legacy request count = %d, want 2", legacyRequests.Load())
	}
}

func TestOllamaEmbedder_Embed_keeps_the_legacy_single_request_contract(t *testing.T) {
	// Given
	var modernRequests atomic.Int64
	var legacyRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/embed" {
			modernRequests.Add(1)
			http.NotFound(w, r)
			return
		}
		legacyRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(ollamaEmbedResponse{Embedding: []float32{1, 2}}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	embedder := NewOllamaEmbedder(WithOllamaEndpoint(server.URL))

	// When
	embedding, err := embedder.Embed(context.Background(), "query")

	// Then
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if !reflect.DeepEqual(embedding, []float32{1, 2}) {
		t.Fatalf("Embed() = %v, want [1 2]", embedding)
	}
	if modernRequests.Load() != 0 || legacyRequests.Load() != 1 {
		t.Fatalf("request counts = modern:%d legacy:%d, want modern:0 legacy:1", modernRequests.Load(), legacyRequests.Load())
	}
}
