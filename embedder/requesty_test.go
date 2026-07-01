package embedder

import (
	"testing"
)

// Test RequestyEmbedder options
func TestNewRequestyEmbedder_Defaults(t *testing.T) {
	// Set API key for testing
	t.Setenv("REQUESTY_API_KEY", "test-key")

	e, err := NewRequestyEmbedder()
	if err != nil {
		t.Fatalf("failed to create RequestyEmbedder: %v", err)
	}

	if e.endpoint != defaultRequestyEndpoint {
		t.Errorf("expected endpoint %s, got %s", defaultRequestyEndpoint, e.endpoint)
	}

	if e.model != defaultRequestyModel {
		t.Errorf("expected model %s, got %s", defaultRequestyModel, e.model)
	}

	// dimensions should be nil by default (no dimensions param sent to API)
	if e.dimensions != nil {
		t.Errorf("expected nil dimensions, got %v", e.dimensions)
	}
}

func TestNewRequestyEmbedder_WithOptions(t *testing.T) {
	customEndpoint := "https://custom.requesty.ai/v1"
	customModel := "openai/text-embedding-3-large"
	customKey := "***"
	customDimensions := 3072

	e, err := NewRequestyEmbedder(
		WithRequestyEndpoint(customEndpoint),
		WithRequestyModel(customModel),
		WithRequestyKey(customKey),
		WithRequestyDimensions(customDimensions),
	)
	if err != nil {
		t.Fatalf("failed to create RequestyEmbedder: %v", err)
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

func TestNewRequestyEmbedder_RequiresAPIKey(t *testing.T) {
	// Ensure no API key is set
	t.Setenv("REQUESTY_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	_, err := NewRequestyEmbedder()
	if err == nil {
		t.Fatal("expected error when API key is not set")
	}
}

func TestNewRequestyEmbedder_UsesEnvAPIKey(t *testing.T) {
	envKey := "***"
	t.Setenv("REQUESTY_API_KEY", envKey)

	e, err := NewRequestyEmbedder()
	if err != nil {
		t.Fatalf("failed to create RequestyEmbedder: %v", err)
	}

	if e.apiKey != envKey {
		t.Errorf("expected apiKey from env %s, got %s", envKey, e.apiKey)
	}
}

func TestNewRequestyEmbedder_FallsBackToOpenAIKey(t *testing.T) {
	// Ensure REQUESTY_API_KEY is not set
	t.Setenv("REQUESTY_API_KEY", "")
	openAIKey := "***"
	t.Setenv("OPENAI_API_KEY", openAIKey)

	e, err := NewRequestyEmbedder()
	if err != nil {
		t.Fatalf("failed to create RequestyEmbedder: %v", err)
	}

	if e.apiKey != openAIKey {
		t.Errorf("expected apiKey from OPENAI_API_KEY %s, got %s", openAIKey, e.apiKey)
	}
}

func TestNewRequestyEmbedder_ExplicitKeyOverridesEnv(t *testing.T) {
	t.Setenv("REQUESTY_API_KEY", "env-key")
	explicitKey := "***"

	e, err := NewRequestyEmbedder(WithRequestyKey(explicitKey))
	if err != nil {
		t.Fatalf("failed to create RequestyEmbedder: %v", err)
	}

	if e.apiKey != explicitKey {
		t.Errorf("expected explicit apiKey %s, got %s", explicitKey, e.apiKey)
	}
}

func TestRequestyEmbedder_Dimensions(t *testing.T) {
	t.Setenv("REQUESTY_API_KEY", "test-key")

	tests := []struct {
		name       string
		dimensions int
	}{
		{"default", requestyDimensions},
		{"custom 512", 512},
		{"custom 1024", 1024},
		{"custom 3072", 3072},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e *RequestyEmbedder
			var err error
			if tt.dimensions == requestyDimensions {
				e, err = NewRequestyEmbedder()
			} else {
				e, err = NewRequestyEmbedder(WithRequestyDimensions(tt.dimensions))
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

func TestRequestyEmbedder_Close(t *testing.T) {
	t.Setenv("REQUESTY_API_KEY", "test-key")

	e, err := NewRequestyEmbedder()
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}
	if err := e.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestRequestyEmbedder_EndpointVariants(t *testing.T) {
	t.Setenv("REQUESTY_API_KEY", "test-key")

	tests := []struct {
		name     string
		endpoint string
	}{
		{"default", "https://router.requesty.ai/v1"},
		{"custom subdomain", "https://custom.requesty.ai/v1"},
		{"local proxy", "http://localhost:8080/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := NewRequestyEmbedder(WithRequestyEndpoint(tt.endpoint))
			if err != nil {
				t.Fatalf("failed to create embedder: %v", err)
			}
			if e.endpoint != tt.endpoint {
				t.Errorf("expected endpoint %s, got %s", tt.endpoint, e.endpoint)
			}
		})
	}
}
