package embedder

import (
	"encoding/json"
	"strings"
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

	t.Run("VoyageAIEmbedder", func(t *testing.T) {
		t.Setenv("VOYAGE_API_KEY", "test-key")
		e, err := NewVoyageAIEmbedder()
		if err != nil {
			t.Fatalf("failed to create embedder: %v", err)
		}
		if err := e.Close(); err != nil {
			t.Errorf("Close() returned error: %v", err)
		}
	})
}

// Test VoyageAIEmbedder options
func TestNewVoyageAIEmbedder_Defaults(t *testing.T) {
	t.Setenv("VOYAGE_API_KEY", "test-key")

	e, err := NewVoyageAIEmbedder()
	if err != nil {
		t.Fatalf("failed to create VoyageAIEmbedder: %v", err)
	}

	if e.endpoint != defaultVoyageAIEndpoint {
		t.Errorf("expected endpoint %s, got %s", defaultVoyageAIEndpoint, e.endpoint)
	}

	if e.model != defaultVoyageAIModel {
		t.Errorf("expected model %s, got %s", defaultVoyageAIModel, e.model)
	}

	// dimensions should be nil by default (no output_dimension param sent to API)
	if e.dimensions != nil {
		t.Errorf("expected nil dimensions, got %v", e.dimensions)
	}
}

func TestNewVoyageAIEmbedder_WithOptions(t *testing.T) {
	customEndpoint := "https://custom-voyage.example.com/v1"
	customModel := "voyage-3"
	customKey := "va-custom-key"
	customDimensions := 512

	e, err := NewVoyageAIEmbedder(
		WithVoyageAIEndpoint(customEndpoint),
		WithVoyageAIModel(customModel),
		WithVoyageAIKey(customKey),
		WithVoyageAIDimensions(customDimensions),
		WithVoyageAIInputType("document"),
	)
	if err != nil {
		t.Fatalf("failed to create VoyageAIEmbedder: %v", err)
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

	if e.inputType != "document" {
		t.Errorf("expected inputType 'document', got %s", e.inputType)
	}
}

func TestNewVoyageAIEmbedder_RequiresAPIKey(t *testing.T) {
	t.Setenv("VOYAGE_API_KEY", "")

	_, err := NewVoyageAIEmbedder()
	if err == nil {
		t.Fatal("expected error when API key is not set")
	}
}

func TestNewVoyageAIEmbedder_UsesEnvAPIKey(t *testing.T) {
	envKey := "va-env-test-key"
	t.Setenv("VOYAGE_API_KEY", envKey)

	e, err := NewVoyageAIEmbedder()
	if err != nil {
		t.Fatalf("failed to create VoyageAIEmbedder: %v", err)
	}

	if e.apiKey != envKey {
		t.Errorf("expected apiKey from env %s, got %s", envKey, e.apiKey)
	}
}

func TestNewVoyageAIEmbedder_ExplicitKeyOverridesEnv(t *testing.T) {
	t.Setenv("VOYAGE_API_KEY", "env-key")
	explicitKey := "va-explicit-key"

	e, err := NewVoyageAIEmbedder(WithVoyageAIKey(explicitKey))
	if err != nil {
		t.Fatalf("failed to create VoyageAIEmbedder: %v", err)
	}

	if e.apiKey != explicitKey {
		t.Errorf("expected explicit apiKey %s, got %s", explicitKey, e.apiKey)
	}
}

func TestVoyageAIEmbedder_Dimensions(t *testing.T) {
	t.Setenv("VOYAGE_API_KEY", "test-key")

	tests := []struct {
		name       string
		dimensions int
	}{
		{"default", defaultVoyageAIDimensions},
		{"custom 256", 256},
		{"custom 512", 512},
		{"custom 2048", 2048},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e *VoyageAIEmbedder
			var err error
			if tt.dimensions == defaultVoyageAIDimensions {
				e, err = NewVoyageAIEmbedder()
			} else {
				e, err = NewVoyageAIEmbedder(WithVoyageAIDimensions(tt.dimensions))
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

func TestVoyageAIEmbedder_RequestUsesOutputDimension(t *testing.T) {
	// Verify the JSON request body uses "output_dimension" not "dimensions"
	dimensions := 512
	req := voyageAIEmbedRequest{
		Model:           "voyage-code-3",
		Input:           []string{"test"},
		OutputDimension: &dimensions,
		InputType:       "document",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	jsonStr := string(data)

	// Must contain "output_dimension", not "dimensions"
	if !strings.Contains(jsonStr, `"output_dimension"`) {
		t.Errorf("expected JSON to contain 'output_dimension', got: %s", jsonStr)
	}

	// Must NOT contain bare "dimensions" key (only "output_dimension")
	// Parse as generic map to check exact keys
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if _, ok := parsed["dimensions"]; ok {
		t.Errorf("JSON should not contain 'dimensions' key, got: %s", jsonStr)
	}

	if val, ok := parsed["output_dimension"]; !ok {
		t.Errorf("JSON should contain 'output_dimension' key, got: %s", jsonStr)
	} else if int(val.(float64)) != dimensions {
		t.Errorf("expected output_dimension=%d, got %v", dimensions, val)
	}
}

func TestVoyageAIEmbedder_RequestOmitsNilDimension(t *testing.T) {
	// When dimensions is nil, output_dimension should be omitted (omitempty)
	req := voyageAIEmbedRequest{
		Model: "voyage-code-3",
		Input: []string{"test"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if _, ok := parsed["output_dimension"]; ok {
		t.Errorf("output_dimension should be omitted when nil, got: %s", string(data))
	}
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

func TestVoyageAIEmbedder_EndpointVariants(t *testing.T) {
	t.Setenv("VOYAGE_API_KEY", "test-key")

	tests := []struct {
		name     string
		endpoint string
	}{
		{"default", "https://api.voyageai.com/v1"},
		{"custom", "https://custom-voyage-proxy.example.com/v1"},
		{"local proxy", "http://localhost:8080/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := NewVoyageAIEmbedder(WithVoyageAIEndpoint(tt.endpoint))
			if err != nil {
				t.Fatalf("failed to create embedder: %v", err)
			}
			if e.endpoint != tt.endpoint {
				t.Errorf("expected endpoint %s, got %s", tt.endpoint, e.endpoint)
			}
		})
	}
}
