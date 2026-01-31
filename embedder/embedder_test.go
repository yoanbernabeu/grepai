package embedder

import (
	"testing"
)

// Test OllamaEmbedder options
func TestNewOllamaEmbedder_Defaults(t *testing.T) {
	e := NewOllamaEmbedder()

	if e.endpoint != defaultOllamaEndpoint {
		t.Errorf("expected endpoint %s, got %s", defaultOllamaEndpoint, e.endpoint)
	}

	if e.model != defaultOllamaModel {
		t.Errorf("expected model %s, got %s", defaultOllamaModel, e.model)
	}

	if e.dimensions != nomicEmbedDimensions {
		t.Errorf("expected dimensions %d, got %d", nomicEmbedDimensions, e.dimensions)
	}
}

func TestNewOllamaEmbedder_WithOptions(t *testing.T) {
	customEndpoint := "http://custom:11434"
	customModel := "custom-model"
	customDimensions := 1024

	e := NewOllamaEmbedder(
		WithOllamaEndpoint(customEndpoint),
		WithOllamaModel(customModel),
		WithOllamaDimensions(customDimensions),
	)

	if e.endpoint != customEndpoint {
		t.Errorf("expected endpoint %s, got %s", customEndpoint, e.endpoint)
	}

	if e.model != customModel {
		t.Errorf("expected model %s, got %s", customModel, e.model)
	}

	if e.dimensions != customDimensions {
		t.Errorf("expected dimensions %d, got %d", customDimensions, e.dimensions)
	}
}

func TestOllamaEmbedder_Dimensions(t *testing.T) {
	tests := []struct {
		name       string
		dimensions int
	}{
		{"default", nomicEmbedDimensions},
		{"custom 512", 512},
		{"custom 1536", 1536},
		{"custom 3072", 3072},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e *OllamaEmbedder
			if tt.dimensions == nomicEmbedDimensions {
				e = NewOllamaEmbedder()
			} else {
				e = NewOllamaEmbedder(WithOllamaDimensions(tt.dimensions))
			}

			if e.Dimensions() != tt.dimensions {
				t.Errorf("expected Dimensions() to return %d, got %d", tt.dimensions, e.Dimensions())
			}
		})
	}
}

// Test LMStudioEmbedder options
func TestNewLMStudioEmbedder_Defaults(t *testing.T) {
	e := NewLMStudioEmbedder()

	if e.endpoint != defaultLMStudioEndpoint {
		t.Errorf("expected endpoint %s, got %s", defaultLMStudioEndpoint, e.endpoint)
	}

	if e.model != defaultLMStudioModel {
		t.Errorf("expected model %s, got %s", defaultLMStudioModel, e.model)
	}

	if e.dimensions != lmStudioNomicDimensions {
		t.Errorf("expected dimensions %d, got %d", lmStudioNomicDimensions, e.dimensions)
	}
}

func TestNewLMStudioEmbedder_WithOptions(t *testing.T) {
	customEndpoint := "http://custom:1234"
	customModel := "custom-embedding-model"
	customDimensions := 2048

	e := NewLMStudioEmbedder(
		WithLMStudioEndpoint(customEndpoint),
		WithLMStudioModel(customModel),
		WithLMStudioDimensions(customDimensions),
	)

	if e.endpoint != customEndpoint {
		t.Errorf("expected endpoint %s, got %s", customEndpoint, e.endpoint)
	}

	if e.model != customModel {
		t.Errorf("expected model %s, got %s", customModel, e.model)
	}

	if e.dimensions != customDimensions {
		t.Errorf("expected dimensions %d, got %d", customDimensions, e.dimensions)
	}
}

func TestLMStudioEmbedder_Dimensions(t *testing.T) {
	tests := []struct {
		name       string
		dimensions int
	}{
		{"default", lmStudioNomicDimensions},
		{"custom 512", 512},
		{"custom 1536", 1536},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e *LMStudioEmbedder
			if tt.dimensions == lmStudioNomicDimensions {
				e = NewLMStudioEmbedder()
			} else {
				e = NewLMStudioEmbedder(WithLMStudioDimensions(tt.dimensions))
			}

			if e.Dimensions() != tt.dimensions {
				t.Errorf("expected Dimensions() to return %d, got %d", tt.dimensions, e.Dimensions())
			}
		})
	}
}

// Test OpenAIEmbedder options
func TestNewOpenAIEmbedder_Defaults(t *testing.T) {
	// Set API key for testing
	t.Setenv("OPENAI_API_KEY", "test-key")

	e, err := NewOpenAIEmbedder()
	if err != nil {
		t.Fatalf("failed to create OpenAIEmbedder: %v", err)
	}

	if e.endpoint != defaultOpenAIEndpoint {
		t.Errorf("expected endpoint %s, got %s", defaultOpenAIEndpoint, e.endpoint)
	}

	if e.model != defaultOpenAIModel {
		t.Errorf("expected model %s, got %s", defaultOpenAIModel, e.model)
	}

	// dimensions should be nil by default (no dimensions param sent to API)
	if e.dimensions != nil {
		t.Errorf("expected nil dimensions, got %v", e.dimensions)
	}
}

func TestNewOpenAIEmbedder_WithOptions(t *testing.T) {
	customEndpoint := "https://custom-openai.example.com/v1"
	customModel := "text-embedding-3-large"
	customKey := "sk-custom-key"
	customDimensions := 3072

	e, err := NewOpenAIEmbedder(
		WithOpenAIEndpoint(customEndpoint),
		WithOpenAIModel(customModel),
		WithOpenAIKey(customKey),
		WithOpenAIDimensions(customDimensions),
	)
	if err != nil {
		t.Fatalf("failed to create OpenAIEmbedder: %v", err)
	}

	if e.endpoint != customEndpoint {
		t.Errorf("expected endpoint %s, got %s", customEndpoint, e.endpoint)
	}

	if e.model != customModel {
		t.Errorf("expected model %s, got %s", customModel, e.model)
	}

	if e.apiKey != customKey {
		t.Errorf("expected apiKey %s, got %s", customKey, e.apiKey)
	}

	if e.dimensions == nil || *e.dimensions != customDimensions {
		t.Errorf("expected dimensions %d, got %v", customDimensions, e.dimensions)
	}
}

func TestNewOpenAIEmbedder_RequiresAPIKey(t *testing.T) {
	// Ensure no API key is set
	t.Setenv("OPENAI_API_KEY", "")

	_, err := NewOpenAIEmbedder()
	if err == nil {
		t.Fatal("expected error when API key is not set")
	}
}

func TestNewOpenAIEmbedder_UsesEnvAPIKey(t *testing.T) {
	envKey := "sk-env-test-key"
	t.Setenv("OPENAI_API_KEY", envKey)

	e, err := NewOpenAIEmbedder()
	if err != nil {
		t.Fatalf("failed to create OpenAIEmbedder: %v", err)
	}

	if e.apiKey != envKey {
		t.Errorf("expected apiKey from env %s, got %s", envKey, e.apiKey)
	}
}

func TestNewOpenAIEmbedder_ExplicitKeyOverridesEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-key")
	explicitKey := "sk-explicit-key"

	e, err := NewOpenAIEmbedder(WithOpenAIKey(explicitKey))
	if err != nil {
		t.Fatalf("failed to create OpenAIEmbedder: %v", err)
	}

	if e.apiKey != explicitKey {
		t.Errorf("expected explicit apiKey %s, got %s", explicitKey, e.apiKey)
	}
}

func TestOpenAIEmbedder_Dimensions(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")

	tests := []struct {
		name       string
		dimensions int
	}{
		{"default", defaultOpenAI3SmallDimensions},
		{"custom 512", 512},
		{"custom 1024", 1024},
		{"custom 3072", 3072},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e *OpenAIEmbedder
			var err error
			if tt.dimensions == defaultOpenAI3SmallDimensions {
				e, err = NewOpenAIEmbedder()
			} else {
				e, err = NewOpenAIEmbedder(WithOpenAIDimensions(tt.dimensions))
			}
			if err != nil {
				t.Fatalf("failed to create embedder: %v", err)
			}

			if e.Dimensions() != tt.dimensions {
				t.Errorf("expected Dimensions() to return %d, got %d", tt.dimensions, e.Dimensions())
			}
		})
	}
}

// Test Close methods
func TestEmbedder_Close(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")

	t.Run("OllamaEmbedder", func(t *testing.T) {
		e := NewOllamaEmbedder()
		if err := e.Close(); err != nil {
			t.Errorf("Close() returned error: %v", err)
		}
	})

	t.Run("LMStudioEmbedder", func(t *testing.T) {
		e := NewLMStudioEmbedder()
		if err := e.Close(); err != nil {
			t.Errorf("Close() returned error: %v", err)
		}
	})

	t.Run("OpenAIEmbedder", func(t *testing.T) {
		e, err := NewOpenAIEmbedder()
		if err != nil {
			t.Fatalf("failed to create embedder: %v", err)
		}
		if err := e.Close(); err != nil {
			t.Errorf("Close() returned error: %v", err)
		}
	})
}

// Test endpoint option combinations
func TestOllamaEmbedder_EndpointVariants(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
	}{
		{"localhost", "http://localhost:11434"},
		{"127.0.0.1", "http://127.0.0.1:11434"},
		{"custom host", "http://ollama.local:11434"},
		{"custom port", "http://localhost:9999"},
		{"https", "https://ollama.example.com:11434"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewOllamaEmbedder(WithOllamaEndpoint(tt.endpoint))
			if e.endpoint != tt.endpoint {
				t.Errorf("expected endpoint %s, got %s", tt.endpoint, e.endpoint)
			}
		})
	}
}

func TestLMStudioEmbedder_EndpointVariants(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
	}{
		{"default", "http://127.0.0.1:1234"},
		{"localhost", "http://localhost:1234"},
		{"custom port", "http://127.0.0.1:8080"},
		{"custom host", "http://lmstudio.local:1234"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewLMStudioEmbedder(WithLMStudioEndpoint(tt.endpoint))
			if e.endpoint != tt.endpoint {
				t.Errorf("expected endpoint %s, got %s", tt.endpoint, e.endpoint)
			}
		})
	}
}

func TestOpenAIEmbedder_EndpointVariants(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")

	tests := []struct {
		name     string
		endpoint string
	}{
		{"default", "https://api.openai.com/v1"},
		{"azure", "https://my-resource.openai.azure.com/v1"},
		{"custom", "https://custom-openai-proxy.example.com/v1"},
		{"local proxy", "http://localhost:8080/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := NewOpenAIEmbedder(WithOpenAIEndpoint(tt.endpoint))
			if err != nil {
				t.Fatalf("failed to create embedder: %v", err)
			}
			if e.endpoint != tt.endpoint {
				t.Errorf("expected endpoint %s, got %s", tt.endpoint, e.endpoint)
			}
		})
	}
}
