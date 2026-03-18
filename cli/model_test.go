package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/internal/managedassets"
)

func setModelTestHome(t *testing.T, dir string) func() {
	t.Helper()
	original := os.Getenv("HOME")
	if runtime.GOOS == "windows" {
		original = os.Getenv("USERPROFILE")
		os.Setenv("USERPROFILE", dir)
		return func() { os.Setenv("USERPROFILE", original) }
	}
	os.Setenv("HOME", dir)
	return func() { os.Setenv("HOME", original) }
}

func TestModelListCommand(t *testing.T) {
	tmpDir := t.TempDir()
	cleanup := setModelTestHome(t, tmpDir)
	defer cleanup()

	models := []managedassets.InstalledModel{{
		ID:         managedassets.DefaultModelID,
		FileName:   "embedding.gguf",
		Path:       filepath.Join(tmpDir, "embedding.gguf"),
		SourceURL:  "https://example.com/embedding.gguf",
		SizeBytes:  123456,
		Dimensions: 768,
	}}
	if err := managedassets.SaveInstalledModels(models); err != nil {
		t.Fatalf("SaveInstalledModels failed: %v", err)
	}

	var buf bytes.Buffer
	modelListCmd.SetOut(&buf)
	modelListCmd.SetArgs(nil)
	defer modelListCmd.SetOut(nil)

	if err := modelListCmd.RunE(modelListCmd, nil); err != nil {
		t.Fatalf("model list failed: %v", err)
	}
	if !strings.Contains(buf.String(), managedassets.DefaultModelID) {
		t.Fatalf("expected model list output to mention %s, got %q", managedassets.DefaultModelID, buf.String())
	}
	if !strings.Contains(buf.String(), "121 KB") {
		t.Fatalf("expected model list output to include formatted size, got %q", buf.String())
	}
}

func TestModelRemoveCommand(t *testing.T) {
	tmpDir := t.TempDir()
	cleanup := setModelTestHome(t, tmpDir)
	defer cleanup()

	modelPath := filepath.Join(tmpDir, "embedding.gguf")
	if err := os.WriteFile(modelPath, []byte("stub"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := managedassets.SaveInstalledModels([]managedassets.InstalledModel{{
		ID:         managedassets.DefaultModelID,
		FileName:   "embedding.gguf",
		Path:       modelPath,
		SourceURL:  "https://example.com/embedding.gguf",
		SizeBytes:  4,
		Dimensions: 768,
	}}); err != nil {
		t.Fatalf("SaveInstalledModels failed: %v", err)
	}

	if err := modelRemoveCmd.RunE(modelRemoveCmd, []string{managedassets.DefaultModelID}); err != nil {
		t.Fatalf("model remove failed: %v", err)
	}
	models, err := managedassets.LoadInstalledModels()
	if err != nil {
		t.Fatalf("LoadInstalledModels failed: %v", err)
	}
	if len(models) != 0 {
		t.Fatalf("expected model manifest to be empty, got %+v", models)
	}
	if _, err := os.Stat(modelPath); !os.IsNotExist(err) {
		t.Fatalf("expected model file to be removed, stat err=%v", err)
	}
}

func TestModelListAvailableCommand(t *testing.T) {
	var buf bytes.Buffer
	modelListAvailableCmd.SetOut(&buf)
	modelListAvailableCmd.SetArgs(nil)
	defer modelListAvailableCmd.SetOut(nil)

	if err := modelListAvailableCmd.RunE(modelListAvailableCmd, nil); err != nil {
		t.Fatalf("model list-available failed: %v", err)
	}
	if !strings.Contains(buf.String(), managedassets.DefaultModelID) {
		t.Fatalf("expected available model output to mention %s, got %q", managedassets.DefaultModelID, buf.String())
	}
	if !strings.Contains(buf.String(), "35.0 MB") {
		t.Fatalf("expected available model output to include formatted size, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "nomic-embed-text-v1.5-q8_0") {
		t.Fatalf("expected available model output to include Nomic option, got %q", buf.String())
	}
}

func TestModelUseCommand(t *testing.T) {
	tmpDir := t.TempDir()
	cleanup := setModelTestHome(t, tmpDir)
	defer cleanup()

	projectDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	cfg := config.DefaultConfig()
	if err := cfg.Save(projectDir); err != nil {
		t.Fatalf("cfg.Save failed: %v", err)
	}

	modelDef, err := managedassets.LookupModel("nomic-embed-text-v1.5-q8_0")
	if err != nil {
		t.Fatalf("LookupModel failed: %v", err)
	}
	modelPath := filepath.Join(tmpDir, modelDef.FileName)
	if err := os.WriteFile(modelPath, []byte("stub"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := managedassets.SaveInstalledModels([]managedassets.InstalledModel{{
		ID:         modelDef.ID,
		FileName:   modelDef.FileName,
		Path:       modelPath,
		SourceURL:  modelDef.URL,
		SizeBytes:  int64(len("stub")),
		Dimensions: modelDef.Dimensions,
	}}); err != nil {
		t.Fatalf("SaveInstalledModels failed: %v", err)
	}

	prevCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(prevCwd)
	}()
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}

	var buf bytes.Buffer
	modelUseCmd.SetOut(&buf)
	modelUseCmd.SetArgs(nil)
	defer modelUseCmd.SetOut(nil)

	if err := modelUseCmd.RunE(modelUseCmd, []string{modelDef.ID}); err != nil {
		t.Fatalf("model use failed: %v", err)
	}

	updated, err := config.Load(projectDir)
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}
	if updated.Embedder.Provider != "llamacpp" {
		t.Fatalf("provider = %q, want llamacpp", updated.Embedder.Provider)
	}
	if updated.Embedder.Model != modelDef.ID {
		t.Fatalf("model = %q, want %q", updated.Embedder.Model, modelDef.ID)
	}
	if updated.Embedder.Endpoint != config.DefaultLlamaCPPEndpoint {
		t.Fatalf("endpoint = %q, want %q", updated.Embedder.Endpoint, config.DefaultLlamaCPPEndpoint)
	}
	if updated.Embedder.Dimensions == nil || *updated.Embedder.Dimensions != modelDef.Dimensions {
		t.Fatalf("dimensions = %v, want %d", updated.Embedder.Dimensions, modelDef.Dimensions)
	}
	if !strings.Contains(buf.String(), modelDef.ID) {
		t.Fatalf("expected output to mention selected model, got %q", buf.String())
	}
}
