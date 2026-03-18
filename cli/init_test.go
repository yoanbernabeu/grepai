package cli

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/internal/managedassets"
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

func setInitTestHome(t *testing.T, dir string) func() {
	t.Helper()
	originalHome := os.Getenv("HOME")
	if runtime.GOOS == "windows" {
		originalProfile := os.Getenv("USERPROFILE")
		_ = os.Setenv("USERPROFILE", dir)
		return func() {
			_ = os.Setenv("USERPROFILE", originalProfile)
		}
	}
	_ = os.Setenv("HOME", dir)
	return func() {
		_ = os.Setenv("HOME", originalHome)
	}
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

func TestRunInit_LlamaCPPDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	withInitTestState(t, tmpDir, func() {
		initProvider = "llamacpp"
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
	if cfg.Embedder.Provider != "llamacpp" {
		t.Fatalf("provider = %q, want llamacpp", cfg.Embedder.Provider)
	}
	if cfg.Embedder.Model != config.DefaultLlamaCPPEmbeddingModel {
		t.Fatalf("model = %q, want %q", cfg.Embedder.Model, config.DefaultLlamaCPPEmbeddingModel)
	}
	if cfg.Embedder.Endpoint != config.DefaultLlamaCPPEndpoint {
		t.Fatalf("endpoint = %q, want %q", cfg.Embedder.Endpoint, config.DefaultLlamaCPPEndpoint)
	}
}

func TestRunInit_LlamaCPPExplicitModelHonored(t *testing.T) {
	tmpDir := t.TempDir()
	withInitTestState(t, tmpDir, func() {
		initProvider = "llamacpp"
		initModel = "nomic-embed-text-v1.5-q8_0"
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
	if cfg.Embedder.Model != "nomic-embed-text-v1.5-q8_0" {
		t.Fatalf("model = %q, want nomic-embed-text-v1.5-q8_0", cfg.Embedder.Model)
	}
	if cfg.Embedder.Dimensions == nil || *cfg.Embedder.Dimensions != 768 {
		t.Fatalf("dimensions = %v, want 768", cfg.Embedder.Dimensions)
	}
}

func TestResolveInteractiveLlamaCPPModelSelectsInstalledModel(t *testing.T) {
	tmpDir := t.TempDir()
	cleanup := setInitTestHome(t, tmpDir)
	defer cleanup()

	modelDef, err := managedassets.LookupModel("nomic-embed-text-v1.5-q8_0")
	if err != nil {
		t.Fatalf("LookupModel failed: %v", err)
	}
	modelPath := filepath.Join(tmpDir, modelDef.FileName)
	if err := os.WriteFile(modelPath, []byte("stub"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := managedassets.SaveInstalledModels([]managedassets.InstalledModel{
		{
			ID:         managedassets.DefaultModelID,
			FileName:   "bge-small-en-v1.5-q8_0.gguf",
			Path:       filepath.Join(tmpDir, "bge-small-en-v1.5-q8_0.gguf"),
			SourceURL:  "https://example.com/bge",
			SizeBytes:  36685152,
			Dimensions: 384,
		},
		{
			ID:         modelDef.ID,
			FileName:   modelDef.FileName,
			Path:       modelPath,
			SourceURL:  modelDef.URL,
			SizeBytes:  modelDef.SizeBytes,
			Dimensions: modelDef.Dimensions,
		},
	}); err != nil {
		t.Fatalf("SaveInstalledModels failed: %v", err)
	}

	var out bytes.Buffer
	reader := bufio.NewReader(strings.NewReader("2\n"))
	selected := resolveInteractiveLlamaCPPModel(reader, &out, "")
	if selected != modelDef.ID {
		t.Fatalf("selected = %q, want %q", selected, modelDef.ID)
	}
	if !strings.Contains(out.String(), "Select managed local model") {
		t.Fatalf("expected prompt output, got %q", out.String())
	}
}

func TestHasInstalledManagedModel(t *testing.T) {
	tmpDir := t.TempDir()
	cleanup := setInitTestHome(t, tmpDir)
	defer cleanup()

	modelDef, err := managedassets.LookupModel("nomic-embed-text-v1.5-q8_0")
	if err != nil {
		t.Fatalf("LookupModel failed: %v", err)
	}
	if err := managedassets.SaveInstalledModels([]managedassets.InstalledModel{{
		ID:         modelDef.ID,
		FileName:   modelDef.FileName,
		Path:       filepath.Join(tmpDir, modelDef.FileName),
		SourceURL:  modelDef.URL,
		SizeBytes:  modelDef.SizeBytes,
		Dimensions: modelDef.Dimensions,
	}}); err != nil {
		t.Fatalf("SaveInstalledModels failed: %v", err)
	}

	if !hasInstalledManagedModel(modelDef.ID) {
		t.Fatalf("expected model %q to be reported as installed", modelDef.ID)
	}
	if hasInstalledManagedModel("missing-model") {
		t.Fatal("expected missing model to be reported as not installed")
	}
}
