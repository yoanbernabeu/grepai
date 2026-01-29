package config

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	endpoints  = "https://api.grepai.com/v1"
	dimensions = 1536
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Version != 1 {
		t.Errorf("expected version 1, got %d", cfg.Version)
	}

	if cfg.Embedder.Provider != "ollama" {
		t.Errorf("expected provider ollama, got %s", cfg.Embedder.Provider)
	}

	if cfg.Embedder.Model != "nomic-embed-text" {
		t.Errorf("expected model nomic-embed-text, got %s", cfg.Embedder.Model)
	}

	if cfg.Embedder.Dimensions != 768 {
		t.Errorf("expected dimensions 768, got %d", cfg.Embedder.Dimensions)
	}

	if cfg.Store.Backend != "gob" {
		t.Errorf("expected backend gob, got %s", cfg.Store.Backend)
	}

	if cfg.Chunking.Size != 512 {
		t.Errorf("expected chunk size 512, got %d", cfg.Chunking.Size)
	}

	if cfg.Chunking.Overlap != 50 {
		t.Errorf("expected chunk overlap 50, got %d", cfg.Chunking.Overlap)
	}

	if cfg.Watch.DebounceMs != 500 {
		t.Errorf("expected debounce 500ms, got %d", cfg.Watch.DebounceMs)
	}
}

func TestConfigSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := DefaultConfig()
	cfg.Embedder.Provider = "openai"
	cfg.Embedder.Dimensions = dimensions
	cfg.Embedder.Endpoint = endpoints
	cfg.Store.Backend = "postgres"

	err := cfg.Save(tmpDir)
	if err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Check file exists
	configPath := GetConfigPath(tmpDir)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}

	// Load config
	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.Embedder.Provider != "openai" {
		t.Errorf("expected provider openai, got %s", loaded.Embedder.Provider)
	}

	if loaded.Embedder.Dimensions != dimensions {
		t.Errorf("expected dimensions %d, got %d", dimensions, loaded.Embedder.Dimensions)
	}

	if loaded.Store.Backend != "postgres" {
		t.Errorf("expected backend postgres, got %s", loaded.Store.Backend)
	}
	if loaded.Embedder.Endpoint != endpoints {
		t.Errorf("expected endpoint %s, got %s", endpoints, loaded.Embedder.Endpoint)
	}
}

func TestConfigExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Should not exist initially
	if Exists(tmpDir) {
		t.Error("config should not exist initially")
	}

	// Create config
	cfg := DefaultConfig()
	if err := cfg.Save(tmpDir); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Should exist now
	if !Exists(tmpDir) {
		t.Error("config should exist after saving")
	}
}

func TestGetConfigDir(t *testing.T) {
	result := GetConfigDir("/test/path")
	expected := filepath.Join("/test/path", ConfigDir)

	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestGetConfigPath(t *testing.T) {
	result := GetConfigPath("/test/path")
	expected := filepath.Join("/test/path", ConfigDir, ConfigFileName)

	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestGetIndexPath(t *testing.T) {
	result := GetIndexPath("/test/path")
	expected := filepath.Join("/test/path", ConfigDir, IndexFileName)

	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

// TestParallelismConfig verifies parallelism defaults and validation
func TestParallelismConfig(t *testing.T) {
	tests := []struct {
		name                string
		configYAML          string
		expectedParallelism int
	}{
		{
			name: "default parallelism is 4 when not specified",
			configYAML: `version: 1
embedder:
  provider: openai
  model: text-embedding-3-small
  api_key: sk-test
store:
  backend: gob
`,
			expectedParallelism: 4,
		},
		{
			name: "custom parallelism is respected",
			configYAML: `version: 1
embedder:
  provider: openai
  model: text-embedding-3-small
  api_key: sk-test
  parallelism: 8
store:
  backend: gob
`,
			expectedParallelism: 8,
		},
		{
			name: "parallelism of 1 is valid",
			configYAML: `version: 1
embedder:
  provider: openai
  model: text-embedding-3-small
  api_key: sk-test
  parallelism: 1
store:
  backend: gob
`,
			expectedParallelism: 1,
		},
		{
			name: "parallelism of 0 defaults to 4",
			configYAML: `version: 1
embedder:
  provider: openai
  model: text-embedding-3-small
  api_key: sk-test
  parallelism: 0
store:
  backend: gob
`,
			expectedParallelism: 4,
		},
		{
			name: "negative parallelism defaults to 4",
			configYAML: `version: 1
embedder:
  provider: openai
  model: text-embedding-3-small
  api_key: sk-test
  parallelism: -1
store:
  backend: gob
`,
			expectedParallelism: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configDir := filepath.Join(tmpDir, ConfigDir)
			if err := os.MkdirAll(configDir, 0755); err != nil {
				t.Fatalf("failed to create config dir: %v", err)
			}

			configPath := filepath.Join(configDir, ConfigFileName)
			if err := os.WriteFile(configPath, []byte(tt.configYAML), 0600); err != nil {
				t.Fatalf("failed to write config: %v", err)
			}

			loaded, err := Load(tmpDir)
			if err != nil {
				t.Fatalf("failed to load config: %v", err)
			}

			if loaded.Embedder.Parallelism != tt.expectedParallelism {
				t.Errorf("expected parallelism %d, got %d", tt.expectedParallelism, loaded.Embedder.Parallelism)
			}
		})
	}
}

// TestFindProjectRootWithSymlink verifies that FindProjectRoot resolves symlinks correctly.
func TestFindProjectRootWithSymlink(t *testing.T) {
	// Create a real directory with grepai config
	realDir := t.TempDir()
	configDir := filepath.Join(realDir, ConfigDir)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	cfg := DefaultConfig()
	if err := cfg.Save(realDir); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Create a symlink to the real directory
	symlinkParent := t.TempDir()
	symlinkPath := filepath.Join(symlinkParent, "symlink-project")
	if err := os.Symlink(realDir, symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Save original working directory
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get original working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalWd); err != nil {
			t.Logf("warning: failed to restore working directory: %v", err)
		}
	}()

	// Change to the symlink directory
	if err := os.Chdir(symlinkPath); err != nil {
		t.Fatalf("failed to change to symlink directory: %v", err)
	}

	// Call FindProjectRoot - it should return the resolved (real) path
	projectRoot, err := FindProjectRoot()
	if err != nil {
		t.Fatalf("FindProjectRoot failed: %v", err)
	}

	// Resolve realDir for comparison (in case it contains symlinks itself)
	expectedPath, err := filepath.EvalSymlinks(realDir)
	if err != nil {
		t.Fatalf("failed to resolve expected path: %v", err)
	}

	if projectRoot != expectedPath {
		t.Errorf("expected resolved path %s, got %s", expectedPath, projectRoot)
	}

	// Verify the returned path is not the symlink path
	if projectRoot == symlinkPath {
		t.Error("FindProjectRoot returned symlink path instead of resolved path")
	}
}

// TestBackwardCompatibility verifies that old configs without dimensions/endpoint
// still work correctly by applying sensible defaults.
func TestBackwardCompatibility(t *testing.T) {
	tests := []struct {
		name               string
		configYAML         string
		expectedEndpoint   string
		expectedDimensions int
	}{
		{
			name: "ollama without endpoint or dimensions",
			configYAML: `version: 1
embedder:
  provider: ollama
  model: nomic-embed-text
store:
  backend: gob
`,
			expectedEndpoint:   "http://localhost:11434",
			expectedDimensions: 768,
		},
		{
			name: "openai without endpoint or dimensions",
			configYAML: `version: 1
embedder:
  provider: openai
  model: text-embedding-3-small
  api_key: sk-test
store:
  backend: gob
`,
			expectedEndpoint:   "https://api.openai.com/v1",
			expectedDimensions: 1536,
		},
		{
			name: "lmstudio without endpoint or dimensions",
			configYAML: `version: 1
embedder:
  provider: lmstudio
  model: text-embedding-nomic-embed-text-v1.5
store:
  backend: gob
`,
			expectedEndpoint:   "http://127.0.0.1:1234",
			expectedDimensions: 768,
		},
		{
			name: "openai with custom endpoint keeps it",
			configYAML: `version: 1
embedder:
  provider: openai
  model: text-embedding-ada-002
  endpoint: https://my-azure.openai.azure.com/v1
  api_key: sk-test
store:
  backend: gob
`,
			expectedEndpoint:   "https://my-azure.openai.azure.com/v1",
			expectedDimensions: 1536,
		},
		{
			name: "custom dimensions preserved",
			configYAML: `version: 1
embedder:
  provider: openai
  model: text-embedding-3-large
  dimensions: 3072
  api_key: sk-test
store:
  backend: gob
`,
			expectedEndpoint:   "https://api.openai.com/v1",
			expectedDimensions: 3072,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configDir := filepath.Join(tmpDir, ConfigDir)
			if err := os.MkdirAll(configDir, 0755); err != nil {
				t.Fatalf("failed to create config dir: %v", err)
			}

			configPath := filepath.Join(configDir, ConfigFileName)
			if err := os.WriteFile(configPath, []byte(tt.configYAML), 0600); err != nil {
				t.Fatalf("failed to write config: %v", err)
			}

			loaded, err := Load(tmpDir)
			if err != nil {
				t.Fatalf("failed to load config: %v", err)
			}

			if loaded.Embedder.Endpoint != tt.expectedEndpoint {
				t.Errorf("expected endpoint %s, got %s", tt.expectedEndpoint, loaded.Embedder.Endpoint)
			}

			if loaded.Embedder.Dimensions != tt.expectedDimensions {
				t.Errorf("expected dimensions %d, got %d", tt.expectedDimensions, loaded.Embedder.Dimensions)
			}
		})
	}
}
