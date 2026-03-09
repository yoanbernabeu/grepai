package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGenerateFromEnv_Defaults(t *testing.T) {
	dir := t.TempDir()

	// Clear all GREPAI_* env vars
	clearGrepaiEnv(t)

	if err := GenerateFromEnv(dir); err != nil {
		t.Fatalf("GenerateFromEnv() error = %v", err)
	}

	cfg := loadTestConfig(t, dir)

	if cfg.Embedder.Provider != "ollama" {
		t.Errorf("provider = %q, want %q", cfg.Embedder.Provider, "ollama")
	}
	if cfg.Embedder.Model != DefaultOllamaEmbeddingModel {
		t.Errorf("model = %q, want %q", cfg.Embedder.Model, DefaultOllamaEmbeddingModel)
	}
	if cfg.Embedder.Endpoint != DefaultOllamaEndpoint {
		t.Errorf("endpoint = %q, want %q", cfg.Embedder.Endpoint, DefaultOllamaEndpoint)
	}
	if cfg.Embedder.Dimensions == nil || *cfg.Embedder.Dimensions != 768 {
		t.Errorf("dimensions = %v, want 768", cfg.Embedder.Dimensions)
	}
	if cfg.Store.Backend != "gob" {
		t.Errorf("backend = %q, want %q", cfg.Store.Backend, "gob")
	}
	if cfg.Chunking.Size != 512 {
		t.Errorf("chunking.size = %d, want 512", cfg.Chunking.Size)
	}
	if cfg.Chunking.Overlap != 50 {
		t.Errorf("chunking.overlap = %d, want 50", cfg.Chunking.Overlap)
	}
}

func TestGenerateFromEnv_ExplicitVars(t *testing.T) {
	dir := t.TempDir()

	clearGrepaiEnv(t)
	t.Setenv("GREPAI_PROVIDER", "openai")
	t.Setenv("GREPAI_MODEL", "text-embedding-3-large")
	t.Setenv("GREPAI_API_KEY", "sk-test-key")
	t.Setenv("GREPAI_BACKEND", "postgres")
	t.Setenv("GREPAI_POSTGRES_DSN", "postgres://user:pass@db:5432/test")
	t.Setenv("GREPAI_PARALLELISM", "8")
	t.Setenv("GREPAI_CHUNKING_SIZE", "1024")
	t.Setenv("GREPAI_CHUNKING_OVERLAP", "100")

	if err := GenerateFromEnv(dir); err != nil {
		t.Fatalf("GenerateFromEnv() error = %v", err)
	}

	cfg := loadTestConfig(t, dir)

	if cfg.Embedder.Provider != "openai" {
		t.Errorf("provider = %q, want %q", cfg.Embedder.Provider, "openai")
	}
	if cfg.Embedder.Model != "text-embedding-3-large" {
		t.Errorf("model = %q, want %q", cfg.Embedder.Model, "text-embedding-3-large")
	}
	if cfg.Embedder.APIKey != "sk-test-key" {
		t.Errorf("api_key = %q, want %q", cfg.Embedder.APIKey, "sk-test-key")
	}
	if cfg.Embedder.Endpoint != DefaultOpenAIEndpoint {
		t.Errorf("endpoint = %q, want %q", cfg.Embedder.Endpoint, DefaultOpenAIEndpoint)
	}
	if cfg.Embedder.Parallelism != 8 {
		t.Errorf("parallelism = %d, want 8", cfg.Embedder.Parallelism)
	}
	if cfg.Store.Backend != "postgres" {
		t.Errorf("backend = %q, want %q", cfg.Store.Backend, "postgres")
	}
	if cfg.Store.Postgres.DSN != "postgres://user:pass@db:5432/test" {
		t.Errorf("postgres.dsn = %q, want %q", cfg.Store.Postgres.DSN, "postgres://user:pass@db:5432/test")
	}
	if cfg.Chunking.Size != 1024 {
		t.Errorf("chunking.size = %d, want 1024", cfg.Chunking.Size)
	}
	if cfg.Chunking.Overlap != 100 {
		t.Errorf("chunking.overlap = %d, want 100", cfg.Chunking.Overlap)
	}
}

func TestGenerateFromEnv_Idempotent(t *testing.T) {
	dir := t.TempDir()

	// Create existing config
	configDir := filepath.Join(dir, ConfigDir)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	existing := []byte("version: 1\ncustom: true\n")
	if err := os.WriteFile(filepath.Join(configDir, ConfigFileName), existing, 0600); err != nil {
		t.Fatal(err)
	}

	clearGrepaiEnv(t)
	t.Setenv("GREPAI_PROVIDER", "openai")

	if err := GenerateFromEnv(dir); err != nil {
		t.Fatalf("GenerateFromEnv() error = %v", err)
	}

	// Verify the file was NOT overwritten
	data, err := os.ReadFile(filepath.Join(configDir, ConfigFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(existing) {
		t.Errorf("config was overwritten, got:\n%s\nwant:\n%s", string(data), string(existing))
	}
}

func TestGenerateFromEnv_ProviderDefaults(t *testing.T) {
	tests := []struct {
		provider      string
		wantModel     string
		wantEndpoint  string
		wantDimNil    bool
		wantDimension int
	}{
		{"ollama", DefaultOllamaEmbeddingModel, DefaultOllamaEndpoint, false, 768},
		{"openai", DefaultOpenAIEmbeddingModel, DefaultOpenAIEndpoint, true, 0},
		{"lmstudio", DefaultLMStudioEmbeddingModel, DefaultLMStudioEndpoint, false, 768},
		{"synthetic", DefaultSyntheticEmbeddingModel, DefaultSyntheticEndpoint, false, 768},
		{"openrouter", DefaultOpenRouterEmbeddingModel, DefaultOpenRouterEndpoint, true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			dir := t.TempDir()
			clearGrepaiEnv(t)
			t.Setenv("GREPAI_PROVIDER", tt.provider)

			if err := GenerateFromEnv(dir); err != nil {
				t.Fatalf("GenerateFromEnv() error = %v", err)
			}

			cfg := loadTestConfig(t, dir)

			if cfg.Embedder.Provider != tt.provider {
				t.Errorf("provider = %q, want %q", cfg.Embedder.Provider, tt.provider)
			}
			if cfg.Embedder.Model != tt.wantModel {
				t.Errorf("model = %q, want %q", cfg.Embedder.Model, tt.wantModel)
			}
			if cfg.Embedder.Endpoint != tt.wantEndpoint {
				t.Errorf("endpoint = %q, want %q", cfg.Embedder.Endpoint, tt.wantEndpoint)
			}
			if tt.wantDimNil {
				if cfg.Embedder.Dimensions != nil {
					t.Errorf("dimensions = %v, want nil", *cfg.Embedder.Dimensions)
				}
			} else {
				if cfg.Embedder.Dimensions == nil {
					t.Errorf("dimensions = nil, want %d", tt.wantDimension)
				} else if *cfg.Embedder.Dimensions != tt.wantDimension {
					t.Errorf("dimensions = %d, want %d", *cfg.Embedder.Dimensions, tt.wantDimension)
				}
			}
		})
	}
}

func TestGenerateFromEnv_QdrantVars(t *testing.T) {
	dir := t.TempDir()

	clearGrepaiEnv(t)
	t.Setenv("GREPAI_BACKEND", "qdrant")
	t.Setenv("GREPAI_QDRANT_ENDPOINT", "qdrant-server")
	t.Setenv("GREPAI_QDRANT_PORT", "6335")
	t.Setenv("GREPAI_QDRANT_COLLECTION", "my-collection")
	t.Setenv("GREPAI_QDRANT_API_KEY", "qdrant-key")
	t.Setenv("GREPAI_QDRANT_USE_TLS", "true")

	if err := GenerateFromEnv(dir); err != nil {
		t.Fatalf("GenerateFromEnv() error = %v", err)
	}

	cfg := loadTestConfig(t, dir)

	if cfg.Store.Backend != "qdrant" {
		t.Errorf("backend = %q, want %q", cfg.Store.Backend, "qdrant")
	}
	if cfg.Store.Qdrant.Endpoint != "qdrant-server" {
		t.Errorf("qdrant.endpoint = %q, want %q", cfg.Store.Qdrant.Endpoint, "qdrant-server")
	}
	if cfg.Store.Qdrant.Port != 6335 {
		t.Errorf("qdrant.port = %d, want 6335", cfg.Store.Qdrant.Port)
	}
	if cfg.Store.Qdrant.Collection != "my-collection" {
		t.Errorf("qdrant.collection = %q, want %q", cfg.Store.Qdrant.Collection, "my-collection")
	}
	if cfg.Store.Qdrant.APIKey != "qdrant-key" {
		t.Errorf("qdrant.api_key = %q, want %q", cfg.Store.Qdrant.APIKey, "qdrant-key")
	}
	if !cfg.Store.Qdrant.UseTLS {
		t.Error("qdrant.use_tls = false, want true")
	}
}

func TestGenerateFromEnv_InvalidValues(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"invalid dimensions", map[string]string{"GREPAI_DIMENSIONS": "abc"}},
		{"invalid parallelism", map[string]string{"GREPAI_PARALLELISM": "xyz"}},
		{"invalid qdrant port", map[string]string{"GREPAI_QDRANT_PORT": "not-a-port"}},
		{"invalid chunking size", map[string]string{"GREPAI_CHUNKING_SIZE": "big"}},
		{"invalid chunking overlap", map[string]string{"GREPAI_CHUNKING_OVERLAP": "lots"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			clearGrepaiEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if err := GenerateFromEnv(dir); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

// clearGrepaiEnv unsets all GREPAI_* environment variables for test isolation.
func clearGrepaiEnv(t *testing.T) {
	t.Helper()
	envVars := []string{
		"GREPAI_PROVIDER", "GREPAI_MODEL", "GREPAI_ENDPOINT",
		"GREPAI_API_KEY", "GREPAI_DIMENSIONS", "GREPAI_PARALLELISM",
		"GREPAI_BACKEND", "GREPAI_POSTGRES_DSN",
		"GREPAI_QDRANT_ENDPOINT", "GREPAI_QDRANT_PORT",
		"GREPAI_QDRANT_COLLECTION", "GREPAI_QDRANT_API_KEY",
		"GREPAI_QDRANT_USE_TLS",
		"GREPAI_CHUNKING_SIZE", "GREPAI_CHUNKING_OVERLAP",
	}
	for _, key := range envVars {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

// loadTestConfig reads and parses the config file from the test directory.
func loadTestConfig(t *testing.T, dir string) *Config {
	t.Helper()
	data, err := os.ReadFile(GetConfigPath(dir))
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}
	return &cfg
}
