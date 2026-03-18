package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
