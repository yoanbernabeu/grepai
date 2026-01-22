package mcp

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/server"
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

// TestRegisterTools_IndexStatusSchema verifies that the grepai_index_status tool
// is registered with a non-empty schema (regression for empty schema error).
func TestRegisterTools_IndexStatusSchema(t *testing.T) {
	s := &Server{
		projectRoot: "/tmp/test-project",
	}

	// initialize minimal MCP server like NewServer would
	s.mcpServer = server.NewMCPServer("grepai-test", "1.0.0")

	s.registerTools()

	tools := s.mcpServer.ListTools()
	indexStatus, ok := tools["grepai_index_status"]
	if !ok {
		t.Fatalf("grepai_index_status tool not registered")
	}

	schema := indexStatus.Tool.InputSchema
	if schema.Type != "object" {
		t.Fatalf("expected schema type object, got %q", schema.Type)
	}

	prop, ok := schema.Properties["verbose"]
	if !ok {
		t.Fatalf("expected verbose property in schema")
	}

	propMap, ok := prop.(map[string]any)
	if !ok {
		t.Fatalf("verbose property is not an object, got %T", prop)
	}

	if propMap["type"] != "boolean" {
		t.Fatalf("expected verbose type boolean, got %v", propMap["type"])
	}
}
