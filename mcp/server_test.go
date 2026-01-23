package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
)

// TestServerCreateEmbedder_AppliesConfiguredDimensions verifies that createEmbedder
// passes configured dimension into each embedder constructor.
func TestServerCreateEmbedder_AppliesConfiguredDimensions(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		dimensions int
		apiKey     string
	}{
		{name: "ollama", provider: "ollama", dimensions: 768},
		{name: "lmstudio", provider: "lmstudio", dimensions: 768},
		{name: "openai-1536", provider: "openai", dimensions: 1536, apiKey: "sk-test"},
		{name: "openai-3072", provider: "openai", dimensions: 3072, apiKey: "sk-test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{}
			cfg := config.DefaultConfig()
			cfg.Embedder.Provider = tt.provider
			cfg.Embedder.Dimensions = tt.dimensions
			cfg.Embedder.APIKey = tt.apiKey

			emb, err := s.createEmbedder(cfg)
			if err != nil {
				t.Fatalf("createEmbedder returned error: %v", err)
			}

			if emb.Dimensions() != tt.dimensions {
				t.Fatalf("expected dimensions %d, got %d", tt.dimensions, emb.Dimensions())
			}
		})
	}
}

// TestCompactStructDefinitions verifies compact struct definitions.
func TestCompactStructDefinitions(t *testing.T) {
	t.Run("SearchResultCompact has no Content field", func(t *testing.T) {
		compact := SearchResultCompact{
			FilePath:  "test.go",
			StartLine: 10,
			EndLine:   20,
			Score:     0.95,
		}

		jsonBytes, err := json.Marshal(compact)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		jsonStr := string(jsonBytes)
		if strings.Contains(jsonStr, "content") {
			t.Errorf("Compact struct should not contain 'content' field, got: %s", jsonStr)
		}
		if !strings.Contains(jsonStr, "file_path") {
			t.Errorf("Compact struct should contain 'file_path' field, got: %s", jsonStr)
		}
	})

	t.Run("CallSiteCompact has no Context field", func(t *testing.T) {
		compact := CallSiteCompact{
			File: "test.go",
			Line: 10,
		}

		jsonBytes, err := json.Marshal(compact)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		jsonStr := string(jsonBytes)
		if strings.Contains(jsonStr, "context") {
			t.Errorf("Compact struct should not contain 'context' field, got: %s", jsonStr)
		}
		if !strings.Contains(jsonStr, "file") {
			t.Errorf("Compact struct should contain 'file' field, got: %s", jsonStr)
		}
	})
}

// TestCompactStructMarshaling verifies JSON marshaling of compact structs.
func TestCompactStructMarshaling(t *testing.T) {
	t.Run("SearchResult vs SearchResultCompact", func(t *testing.T) {
		full := SearchResult{
			FilePath:  "test.go",
			StartLine: 10,
			EndLine:   20,
			Score:     0.95,
			Content:   "line 1\nline 2\nline 3",
		}

		compact := SearchResultCompact{
			FilePath:  "test.go",
			StartLine: 10,
			EndLine:   20,
			Score:     0.95,
		}

		fullJSON, err := json.Marshal(full)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		compactJSON, err := json.Marshal(compact)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		if len(compactJSON) >= len(fullJSON) {
			t.Errorf("Compact JSON should be shorter than full JSON, got compact=%d, full=%d", len(compactJSON), len(fullJSON))
		}

		if !strings.Contains(string(fullJSON), "content") {
			t.Errorf("Full JSON should contain 'content' field")
		}

		if strings.Contains(string(compactJSON), "content") {
			t.Errorf("Compact JSON should not contain 'content' field")
		}
	})
}

// TestNonCompactSearchResult verifies that the full SearchResult struct
// includes all expected fields when NOT in compact mode.
func TestNonCompactSearchResult(t *testing.T) {
	result := SearchResult{
		FilePath:  "example/test.go",
		StartLine: 42,
		EndLine:   50,
		Score:     0.87,
		Content:   "func example() {\n\treturn true\n}",
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)

	// Verify all required fields are present
	expectedFields := []string{
		"file_path",
		"start_line",
		"end_line",
		"score",
		"content",
	}

	for _, field := range expectedFields {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("Non-compact JSON should contain '%s' field, got: %s", field, jsonStr)
		}
	}

	// Verify content field has correct value
	if !strings.Contains(jsonStr, "func example()") {
		t.Errorf("Non-compact JSON should contain full content, got: %s", jsonStr)
	}

	// Verify score is present and non-zero
	if !strings.Contains(jsonStr, "0.87") {
		t.Errorf("Non-compact JSON should contain score value, got: %s", jsonStr)
	}
}

// TestServerCreateStore_GOBBackend tests createStore with gob backend
func TestServerCreateStore_GOBBackend(t *testing.T) {
	s := &Server{
		projectRoot: "/tmp/test-project",
	}

	cfg := config.DefaultConfig()
	cfg.Store.Backend = "gob"

	ctx := context.Background()
	store, err := s.createStore(ctx, cfg)

	if err != nil {
		t.Fatalf("createStore returned error: %v", err)
	}

	if store == nil {
		t.Error("expected non-nil store")
	}

	_ = store.Close()
}

// TestServerCreateStore_UnknownBackend tests that createStore returns error for unknown backend
func TestServerCreateStore_UnknownBackend(t *testing.T) {
	s := &Server{
		projectRoot: "/tmp/test-project",
	}

	cfg := config.DefaultConfig()
	cfg.Store.Backend = "unknown-backend"

	ctx := context.Background()
	_, err := s.createStore(ctx, cfg)

	if err == nil {
		t.Fatal("expected error for unknown backend, got nil")
	}

	expected := "unknown storage backend: unknown-backend"
	if err.Error() != expected {
		t.Errorf("expected error message %s, got %s", expected, err.Error())
	}
}
