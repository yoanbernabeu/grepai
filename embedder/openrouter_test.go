package embedder

import (
	"testing"
)

// Test OpenRouterEmbedder options
func TestNewOpenRouterEmbedder_Defaults(t *testing.T) {
	// Set API key for testing
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	e, err := NewOpenRouterEmbedder()
	if err != nil {
		t.Fatalf("failed to create OpenRouterEmbedder: %v", err)
	}

	if e.endpoint != defaultOpenRouterEndpoint {
		t.Errorf("expected endpoint %s, got %s", defaultOpenRouterEndpoint, e.endpoint)
	}

	if e.model != defaultOpenRouterModel {
		t.Errorf("expected model %s, got %s", defaultOpenRouterModel, e.model)
	}

	// dimensions should be nil by default (no dimensions param sent to API)
	if e.dimensions != nil {
		t.Errorf("expected nil dimensions, got %v", e.dimensions)
	}
}

func TestNewOpenRouterEmbedder_WithOptions(t *testing.T) {
	customEndpoint := "https://custom.openrouter.ai/api/v1"
	customModel := "openai/text-embedding-3-large"
	customKey := "sk-custom-key"
	customDimensions := 3072

	e, err := NewOpenRouterEmbedder(
		WithOpenRouterEndpoint(customEndpoint),
		WithOpenRouterModel(customModel),
		WithOpenRouterKey(customKey),
		WithOpenRouterDimensions(customDimensions),
	)
	if err != nil {
		t.Fatalf("failed to create OpenRouterEmbedder: %v", err)
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

func TestNewOpenRouterEmbedder_RequiresAPIKey(t *testing.T) {
	// Ensure no API key is set
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	_, err := NewOpenRouterEmbedder()
	if err == nil {
		t.Fatal("expected error when API key is not set")
	}
}

func TestNewOpenRouterEmbedder_UsesEnvAPIKey(t *testing.T) {
	envKey := "sk-env-test-key"
	t.Setenv("OPENROUTER_API_KEY", envKey)

	e, err := NewOpenRouterEmbedder()
	if err != nil {
		t.Fatalf("failed to create OpenRouterEmbedder: %v", err)
	}

	if e.apiKey != envKey {
		t.Errorf("expected apiKey from env %s, got %s", envKey, e.apiKey)
	}
}

func TestNewOpenRouterEmbedder_FallsBackToOpenAIKey(t *testing.T) {
	// Ensure OPENROUTER_API_KEY is not set
	t.Setenv("OPENROUTER_API_KEY", "")
	openAIKey := "sk-openai-key"
	t.Setenv("OPENAI_API_KEY", openAIKey)

	e, err := NewOpenRouterEmbedder()
	if err != nil {
		t.Fatalf("failed to create OpenRouterEmbedder: %v", err)
	}

	if e.apiKey != openAIKey {
		t.Errorf("expected apiKey from OPENAI_API_KEY %s, got %s", openAIKey, e.apiKey)
	}
}

func TestNewOpenRouterEmbedder_ExplicitKeyOverridesEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "env-key")
	explicitKey := "sk-explicit-key"

	e, err := NewOpenRouterEmbedder(WithOpenRouterKey(explicitKey))
	if err != nil {
		t.Fatalf("failed to create OpenRouterEmbedder: %v", err)
	}

	if e.apiKey != explicitKey {
		t.Errorf("expected explicit apiKey %s, got %s", explicitKey, e.apiKey)
	}
}

func TestOpenRouterEmbedder_Dimensions(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	tests := []struct {
		name       string
		dimensions int
	}{
		{"default", openRouterDimensions},
		{"custom 512", 512},
		{"custom 1024", 1024},
		{"custom 3072", 3072},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e *OpenRouterEmbedder
			var err error
			if tt.dimensions == openRouterDimensions {
				e, err = NewOpenRouterEmbedder()
			} else {
				e, err = NewOpenRouterEmbedder(WithOpenRouterDimensions(tt.dimensions))
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

func TestOpenRouterEmbedder_Close(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	e, err := NewOpenRouterEmbedder()
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}
	if err := e.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestOpenRouterEmbedder_EndpointVariants(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	tests := []struct {
		name     string
		endpoint string
	}{
		{"default", "https://openrouter.ai/api/v1"},
		{"custom subdomain", "https://custom.openrouter.ai/api/v1"},
		{"local proxy", "http://localhost:8080/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := NewOpenRouterEmbedder(WithOpenRouterEndpoint(tt.endpoint))
			if err != nil {
				t.Fatalf("failed to create embedder: %v", err)
			}
			if e.endpoint != tt.endpoint {
				t.Errorf("expected endpoint %s, got %s", tt.endpoint, e.endpoint)
			}
		})
	}
}
