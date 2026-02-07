package embedder

import (
	"testing"

	"github.com/yoanbernabeu/grepai/config"
)

func TestNewFromConfig_Ollama(t *testing.T) {
	cfg := &config.Config{
		Embedder: config.EmbedderConfig{
			Provider: "ollama",
			Model:    "nomic-embed-text",
			Endpoint: "http://localhost:11434",
		},
	}

	emb, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}
	defer emb.Close()

	ollamaEmb, ok := emb.(*OllamaEmbedder)
	if !ok {
		t.Errorf("expected *OllamaEmbedder, got %T", emb)
	}

	if ollamaEmb.model != "nomic-embed-text" {
		t.Errorf("expected model nomic-embed-text, got %s", ollamaEmb.model)
	}
}

func TestNewFromConfig_OpenAI(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")

	cfg := &config.Config{
		Embedder: config.EmbedderConfig{
			Provider:    "openai",
			Model:       "text-embedding-3-small",
			Endpoint:    "https://api.openai.com/v1",
			Parallelism: 4,
		},
	}

	emb, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}
	defer emb.Close()

	_, ok := emb.(*OpenAIEmbedder)
	if !ok {
		t.Errorf("expected *OpenAIEmbedder, got %T", emb)
	}
}

func TestNewFromConfig_LMStudio(t *testing.T) {
	cfg := &config.Config{
		Embedder: config.EmbedderConfig{
			Provider: "lmstudio",
			Model:    "nomic-embed-text",
			Endpoint: "http://127.0.0.1:1234",
		},
	}

	emb, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}
	defer emb.Close()

	_, ok := emb.(*LMStudioEmbedder)
	if !ok {
		t.Errorf("expected *LMStudioEmbedder, got %T", emb)
	}
}

func TestNewFromConfig_Synthetic(t *testing.T) {
	t.Setenv("SYNTHETIC_API_KEY", "test-key")

	cfg := &config.Config{
		Embedder: config.EmbedderConfig{
			Provider: "synthetic",
			Model:    "hf:nomic-ai/nomic-embed-text-v1.5",
			Endpoint: "https://api.synthetic.new/openai/v1",
		},
	}

	emb, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}
	defer emb.Close()

	_, ok := emb.(*SyntheticEmbedder)
	if !ok {
		t.Errorf("expected *SyntheticEmbedder, got %T", emb)
	}
}

func TestNewFromConfig_OpenRouter(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	cfg := &config.Config{
		Embedder: config.EmbedderConfig{
			Provider: "openrouter",
			Model:    "openai/text-embedding-3-small",
			Endpoint: "https://openrouter.ai/api/v1",
		},
	}

	emb, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}
	defer emb.Close()

	_, ok := emb.(*OpenRouterEmbedder)
	if !ok {
		t.Errorf("expected *OpenRouterEmbedder, got %T", emb)
	}
}

func TestNewFromConfig_UnknownProvider(t *testing.T) {
	cfg := &config.Config{
		Embedder: config.EmbedderConfig{
			Provider: "unknown",
		},
	}

	_, err := NewFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestNewFromConfig_WithDimensions(t *testing.T) {
	dimensions := 1024
	cfg := &config.Config{
		Embedder: config.EmbedderConfig{
			Provider:   "ollama",
			Model:      "nomic-embed-text",
			Endpoint:   "http://localhost:11434",
			Dimensions: &dimensions,
		},
	}

	emb, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}
	defer emb.Close()

	if emb.Dimensions() != 1024 {
		t.Errorf("expected dimensions 1024, got %d", emb.Dimensions())
	}
}

func TestNewFromWorkspaceConfig(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")

	ws := &config.Workspace{
		Name: "test-workspace",
		Embedder: config.EmbedderConfig{
			Provider: "openai",
			Model:    "text-embedding-3-small",
		},
	}

	emb, err := NewFromWorkspaceConfig(ws)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}
	defer emb.Close()

	_, ok := emb.(*OpenAIEmbedder)
	if !ok {
		t.Errorf("expected *OpenAIEmbedder, got %T", emb)
	}
}
