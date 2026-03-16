package cli

import (
	"os"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
)

func withInitTestState(t *testing.T, dir string, configure func()) {
	t.Helper()

	prevCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir(%s): %v", dir, err)
	}

	prevProvider := initProvider
	prevModel := initModel
	prevBackend := initBackend
	prevNonInteractive := initNonInteractive
	prevInherit := initInherit
	prevUI := initUI

	initProvider = ""
	initModel = ""
	initBackend = ""
	initNonInteractive = false
	initInherit = false
	initUI = false
	configure()

	t.Cleanup(func() {
		_ = os.Chdir(prevCwd)
		initProvider = prevProvider
		initModel = prevModel
		initBackend = prevBackend
		initNonInteractive = prevNonInteractive
		initInherit = prevInherit
		initUI = prevUI
	})
}

func TestRunInit_OpenAIExplicitModelHonored(t *testing.T) {
	tmpDir := t.TempDir()
	withInitTestState(t, tmpDir, func() {
		initProvider = "openai"
		initModel = "text-embedding-3-large"
		initBackend = "gob"
		initNonInteractive = true
	})

	if err := runInit(nil, nil); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	cfg, err := config.Load(tmpDir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Embedder.Model != "text-embedding-3-large" {
		t.Fatalf("model = %q, want text-embedding-3-large", cfg.Embedder.Model)
	}
	if cfg.Embedder.Parallelism != config.DefaultOpenAIParallelism {
		t.Fatalf("parallelism = %d, want %d", cfg.Embedder.Parallelism, config.DefaultOpenAIParallelism)
	}
	if cfg.Embedder.GetDimensions() != config.DefaultOpenAILargeDimensions {
		t.Fatalf("dimensions = %d, want %d", cfg.Embedder.GetDimensions(), config.DefaultOpenAILargeDimensions)
	}
}

func TestRunInit_OpenAIDefaultsToOpenAISmallModel(t *testing.T) {
	tmpDir := t.TempDir()
	withInitTestState(t, tmpDir, func() {
		initProvider = "openai"
		initBackend = "gob"
		initNonInteractive = true
	})

	if err := runInit(nil, nil); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	cfg, err := config.Load(tmpDir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Embedder.Model != config.DefaultOpenAIEmbeddingModel {
		t.Fatalf("model = %q, want %q", cfg.Embedder.Model, config.DefaultOpenAIEmbeddingModel)
	}
	if cfg.Embedder.Parallelism != config.DefaultOpenAIParallelism {
		t.Fatalf("parallelism = %d, want %d", cfg.Embedder.Parallelism, config.DefaultOpenAIParallelism)
	}
}
