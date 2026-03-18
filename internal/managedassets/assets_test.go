package managedassets

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func setTestHomeDir(t *testing.T, dir string) func() {
	t.Helper()
	if runtime.GOOS == "windows" {
		original := os.Getenv("USERPROFILE")
		os.Setenv("USERPROFILE", dir)
		return func() { os.Setenv("USERPROFILE", original) }
	}
	original := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	return func() { os.Setenv("HOME", original) }
}

func TestManagedPaths(t *testing.T) {
	tmpDir := t.TempDir()
	cleanup := setTestHomeDir(t, tmpDir)
	defer cleanup()

	binDir, err := GetManagedBinDir()
	if err != nil {
		t.Fatalf("GetManagedBinDir failed: %v", err)
	}
	modelDir, err := GetManagedModelsDir()
	if err != nil {
		t.Fatalf("GetManagedModelsDir failed: %v", err)
	}
	if filepath.Base(binDir) != "bin" {
		t.Fatalf("expected bin dir, got %s", binDir)
	}
	if filepath.Base(modelDir) != "models" {
		t.Fatalf("expected models dir, got %s", modelDir)
	}
}

func TestSaveAndLoadInstalledModels(t *testing.T) {
	tmpDir := t.TempDir()
	cleanup := setTestHomeDir(t, tmpDir)
	defer cleanup()

	models := []InstalledModel{{
		ID:         DefaultModelID,
		FileName:   "test.gguf",
		Path:       filepath.Join(tmpDir, "test.gguf"),
		SourceURL:  "https://example.com/test.gguf",
		Dimensions: 768,
	}}
	if err := SaveInstalledModels(models); err != nil {
		t.Fatalf("SaveInstalledModels failed: %v", err)
	}
	loaded, err := LoadInstalledModels()
	if err != nil {
		t.Fatalf("LoadInstalledModels failed: %v", err)
	}
	if len(loaded) != 1 || loaded[0].ID != DefaultModelID {
		t.Fatalf("unexpected loaded models: %+v", loaded)
	}
}

func TestLookupCurrentRuntime(t *testing.T) {
	if _, err := LookupCurrentRuntime(); err != nil {
		t.Fatalf("LookupCurrentRuntime failed for %s/%s: %v", runtime.GOOS, runtime.GOARCH, err)
	}
}

func TestLookupRuntime_KnownCrossPlatformTargets(t *testing.T) {
	targets := [][2]string{
		{"darwin", "arm64"},
		{"darwin", "amd64"},
		{"linux", "amd64"},
		{"windows", "amd64"},
	}

	for _, target := range targets {
		def, err := LookupRuntime(target[0], target[1])
		if err != nil {
			t.Fatalf("LookupRuntime(%s, %s) failed: %v", target[0], target[1], err)
		}
		if def.URL == "" || def.Binary == "" {
			t.Fatalf("incomplete runtime definition for %s/%s: %+v", target[0], target[1], def)
		}
	}
}

func TestRuntimeStateRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	cleanup := setTestHomeDir(t, tmpDir)
	defer cleanup()

	state := RuntimeState{
		Version:  DefaultRuntimeVersion,
		Platform: "darwin",
		Arch:     "arm64",
		Binary:   "/tmp/llama-server",
		Endpoint: DefaultSidecarEndpoint(),
		PID:      12345,
	}
	if err := SaveRuntimeState(state); err != nil {
		t.Fatalf("SaveRuntimeState failed: %v", err)
	}
	loaded, err := LoadRuntimeState()
	if err != nil {
		t.Fatalf("LoadRuntimeState failed: %v", err)
	}
	if loaded == nil || loaded.PID != state.PID || loaded.Endpoint != state.Endpoint {
		t.Fatalf("unexpected runtime state: %+v", loaded)
	}
	if err := ClearRuntimeState(); err != nil {
		t.Fatalf("ClearRuntimeState failed: %v", err)
	}
	loaded, err = LoadRuntimeState()
	if err != nil {
		t.Fatalf("LoadRuntimeState after clear failed: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected nil runtime state after clear, got %+v", loaded)
	}
}
