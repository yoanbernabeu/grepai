package embedder

import (
	"testing"
	"time"

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

func TestNewFromConfig_Requesty(t *testing.T) {
	t.Setenv("REQUESTY_API_KEY", "test-key")

	cfg := &config.Config{
		Embedder: config.EmbedderConfig{
			Provider: "requesty",
			Model:    "openai/text-embedding-3-small",
			Endpoint: "https://router.requesty.ai/v1",
		},
	}

	emb, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}
	defer emb.Close()

	_, ok := emb.(*RequestyEmbedder)
	if !ok {
		t.Errorf("expected *RequestyEmbedder, got %T", emb)
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

func TestNewFromConfig_RequestTimeoutAndMaxRetries(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")

	cfg := &config.Config{
		Embedder: config.EmbedderConfig{
			Provider:              "openai",
			Model:                 "text-embedding-3-small",
			RequestTimeoutSeconds: 300,
			MaxRetries:            9,
		},
	}

	emb, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}
	defer emb.Close()

	oai, ok := emb.(*OpenAIEmbedder)
	if !ok {
		t.Fatalf("expected *OpenAIEmbedder, got %T", emb)
	}
	if got := oai.client.Timeout; got != 300*time.Second {
		t.Errorf("expected HTTP client timeout 5m, got %v", got)
	}
	if got := oai.retryPolicy.MaxAttempts; got != 9 {
		t.Errorf("expected MaxAttempts 9, got %d", got)
	}
}

func TestNewFromConfig_TimeoutDefaultsPreserved(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")

	cfg := &config.Config{
		Embedder: config.EmbedderConfig{
			Provider: "openai",
			Model:    "text-embedding-3-small",
		},
	}

	emb, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}
	defer emb.Close()

	oai, ok := emb.(*OpenAIEmbedder)
	if !ok {
		t.Fatalf("expected *OpenAIEmbedder, got %T", emb)
	}
	if got := oai.client.Timeout; got != 60*time.Second {
		t.Errorf("expected default 60s timeout to be preserved, got %v", got)
	}
	if got := oai.retryPolicy.MaxAttempts; got != 5 {
		t.Errorf("expected default MaxAttempts 5 to be preserved, got %d", got)
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
