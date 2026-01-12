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
