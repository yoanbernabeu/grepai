package mcp

import (
	"testing"

	"github.com/yoanbernabeu/grepai/config"
)

// TestServerCreateEmbedder_AppliesConfiguredDimensions verifies that createEmbedder
// passes the configured dimension into each embedder constructor.
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
