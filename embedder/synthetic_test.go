package embedder

import (
	"testing"
)

// Test SyntheticEmbedder options
func TestNewSyntheticEmbedder_Defaults(t *testing.T) {
	// Set API key for testing
	t.Setenv("SYNTHETIC_API_KEY", "test-key")

	e, err := NewSyntheticEmbedder()
	if err != nil {
		t.Fatalf("failed to create SyntheticEmbedder: %v", err)
	}

	if e.endpoint != defaultSyntheticEndpoint+defaultSyntheticPath {
		t.Errorf("expected endpoint %s, got %s", defaultSyntheticEndpoint+defaultSyntheticPath, e.endpoint)
	}

	if e.model != defaultSyntheticModel {
		t.Errorf("expected model %s, got %s", defaultSyntheticModel, e.model)
	}

	if e.dimensions != syntheticEmbedDimensions {
		t.Errorf("expected dimensions %d, got %d", syntheticEmbedDimensions, e.dimensions)
	}
}

func TestNewSyntheticEmbedder_WithOptions(t *testing.T) {
	customEndpoint := "https://custom.synthetic.new/openai/v1"
	customModel := "custom-model"
	customKey := "sk-custom-key"
	customDimensions := 1024

	e, err := NewSyntheticEmbedder(
		WithSyntheticEndpoint(customEndpoint),
		WithSyntheticModel(customModel),
		WithSyntheticKey(customKey),
		WithSyntheticDimensions(customDimensions),
	)
	if err != nil {
		t.Fatalf("failed to create SyntheticEmbedder: %v", err)
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

	if e.dimensions != customDimensions {
		t.Errorf("expected dimensions %d, got %d", customDimensions, e.dimensions)
	}
}

func TestNewSyntheticEmbedder_RequiresAPIKey(t *testing.T) {
	// Ensure no API key is set
	t.Setenv("SYNTHETIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	_, err := NewSyntheticEmbedder()
	if err == nil {
		t.Fatal("expected error when API key is not set")
	}
}

func TestNewSyntheticEmbedder_UsesEnvAPIKey(t *testing.T) {
	envKey := "sk-env-test-key"
	t.Setenv("SYNTHETIC_API_KEY", envKey)

	e, err := NewSyntheticEmbedder()
	if err != nil {
		t.Fatalf("failed to create SyntheticEmbedder: %v", err)
	}

	if e.apiKey != envKey {
		t.Errorf("expected apiKey from env %s, got %s", envKey, e.apiKey)
	}
}

func TestNewSyntheticEmbedder_FallsBackToOpenAIKey(t *testing.T) {
	// Ensure SYNTHETIC_API_KEY is not set
	t.Setenv("SYNTHETIC_API_KEY", "")
	openAIKey := "sk-openai-key"
	t.Setenv("OPENAI_API_KEY", openAIKey)

	e, err := NewSyntheticEmbedder()
	if err != nil {
		t.Fatalf("failed to create SyntheticEmbedder: %v", err)
	}

	if e.apiKey != openAIKey {
		t.Errorf("expected apiKey from OPENAI_API_KEY %s, got %s", openAIKey, e.apiKey)
	}
}

func TestNewSyntheticEmbedder_ExplicitKeyOverridesEnv(t *testing.T) {
	t.Setenv("SYNTHETIC_API_KEY", "env-key")
	explicitKey := "sk-explicit-key"

	e, err := NewSyntheticEmbedder(WithSyntheticKey(explicitKey))
	if err != nil {
		t.Fatalf("failed to create SyntheticEmbedder: %v", err)
	}

	if e.apiKey != explicitKey {
		t.Errorf("expected explicit apiKey %s, got %s", explicitKey, e.apiKey)
	}
}

func TestSyntheticEmbedder_Dimensions(t *testing.T) {
	t.Setenv("SYNTHETIC_API_KEY", "test-key")

	tests := []struct {
		name       string
		dimensions int
	}{
		{"default", syntheticEmbedDimensions},
		{"custom 512", 512},
		{"custom 1024", 1024},
		{"custom 1536", 1536},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e *SyntheticEmbedder
			var err error
			if tt.dimensions == syntheticEmbedDimensions {
				e, err = NewSyntheticEmbedder()
			} else {
				e, err = NewSyntheticEmbedder(WithSyntheticDimensions(tt.dimensions))
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

func TestSyntheticEmbedder_Close(t *testing.T) {
	t.Setenv("SYNTHETIC_API_KEY", "test-key")

	e, err := NewSyntheticEmbedder()
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}
	if err := e.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestSyntheticEmbedder_EndpointVariants(t *testing.T) {
	t.Setenv("SYNTHETIC_API_KEY", "test-key")

	tests := []struct {
		name     string
		endpoint string
	}{
		{"default", "https://api.synthetic.new/openai/v1"},
		{"custom subdomain", "https://custom.synthetic.new/openai/v1"},
		{"local proxy", "http://localhost:8080/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := NewSyntheticEmbedder(WithSyntheticEndpoint(tt.endpoint))
			if err != nil {
				t.Fatalf("failed to create embedder: %v", err)
			}
			if e.endpoint != tt.endpoint {
				t.Errorf("expected endpoint %s, got %s", tt.endpoint, e.endpoint)
			}
		})
	}
}
