package trace

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
	"gopkg.in/yaml.v3"
)

// setTestHomeDir sets the home directory for testing in a cross-platform way.
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

// writeWorkspaceConfig writes a workspace config YAML to the test home dir.
func writeWorkspaceConfig(t *testing.T, homeDir string, wsCfg *config.WorkspaceConfig) {
	t.Helper()
	configDir := filepath.Join(homeDir, ".grepai")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	data, err := yaml.Marshal(wsCfg)
	if err != nil {
		t.Fatalf("failed to marshal workspace config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "workspace.yaml"), data, 0o644); err != nil {
		t.Fatalf("failed to write workspace config: %v", err)
	}
}

func TestLoadWorkspaceSymbolStores_should_fail_when_no_config_exists(t *testing.T) {
	tmpDir := t.TempDir()
	cleanup := setTestHomeDir(t, tmpDir)
	defer cleanup()

	ctx := context.Background()
	_, err := LoadWorkspaceSymbolStores(ctx, "nonexistent", "")
	if err == nil {
		t.Fatal("expected error when no workspace config exists")
	}
}

func TestLoadWorkspaceSymbolStores_should_fail_when_workspace_not_found(t *testing.T) {
	tmpDir := t.TempDir()
	cleanup := setTestHomeDir(t, tmpDir)
	defer cleanup()

	wsCfg := &config.WorkspaceConfig{
		Version:    1,
		Workspaces: map[string]config.Workspace{},
	}
	writeWorkspaceConfig(t, tmpDir, wsCfg)

	ctx := context.Background()
	_, err := LoadWorkspaceSymbolStores(ctx, "nonexistent", "")
	if err == nil {
		t.Fatal("expected error when workspace not found")
	}
}

func TestLoadWorkspaceSymbolStores_should_fail_when_project_not_found(t *testing.T) {
	tmpDir := t.TempDir()
	cleanup := setTestHomeDir(t, tmpDir)
	defer cleanup()

	wsCfg := &config.WorkspaceConfig{
		Version: 1,
		Workspaces: map[string]config.Workspace{
			"test-ws": {
				Name:     "test-ws",
				Projects: []config.ProjectEntry{{Name: "proj-a", Path: "/some/path"}},
			},
		},
	}
	writeWorkspaceConfig(t, tmpDir, wsCfg)

	ctx := context.Background()
	_, err := LoadWorkspaceSymbolStores(ctx, "test-ws", "nonexistent-project")
	if err == nil {
		t.Fatal("expected error when project not found in workspace")
	}
}

func TestLoadWorkspaceSymbolStores_should_load_stores_for_all_projects(t *testing.T) {
	tmpDir := t.TempDir()
	cleanup := setTestHomeDir(t, tmpDir)
	defer cleanup()

	// Create project dirs with empty symbol stores
	projA := filepath.Join(tmpDir, "project-a")
	projB := filepath.Join(tmpDir, "project-b")
	for _, dir := range []string{projA, projB} {
		symbolDir := filepath.Join(dir, ".grepai")
		if err := os.MkdirAll(symbolDir, 0o755); err != nil {
			t.Fatalf("failed to create symbol dir: %v", err)
		}
	}

	wsCfg := &config.WorkspaceConfig{
		Version: 1,
		Workspaces: map[string]config.Workspace{
			"test-ws": {
				Name: "test-ws",
				Projects: []config.ProjectEntry{
					{Name: "proj-a", Path: projA},
					{Name: "proj-b", Path: projB},
				},
			},
		},
	}
	writeWorkspaceConfig(t, tmpDir, wsCfg)

	ctx := context.Background()
	stores, err := LoadWorkspaceSymbolStores(ctx, "test-ws", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer CloseSymbolStores(stores)

	if len(stores) != 2 {
		t.Errorf("expected 2 stores, got %d", len(stores))
	}
}

func TestLoadWorkspaceSymbolStores_should_filter_by_project_name(t *testing.T) {
	tmpDir := t.TempDir()
	cleanup := setTestHomeDir(t, tmpDir)
	defer cleanup()

	projA := filepath.Join(tmpDir, "project-a")
	projB := filepath.Join(tmpDir, "project-b")
	for _, dir := range []string{projA, projB} {
		symbolDir := filepath.Join(dir, ".grepai")
		if err := os.MkdirAll(symbolDir, 0o755); err != nil {
			t.Fatalf("failed to create symbol dir: %v", err)
		}
	}

	wsCfg := &config.WorkspaceConfig{
		Version: 1,
		Workspaces: map[string]config.Workspace{
			"test-ws": {
				Name: "test-ws",
				Projects: []config.ProjectEntry{
					{Name: "proj-a", Path: projA},
					{Name: "proj-b", Path: projB},
				},
			},
		},
	}
	writeWorkspaceConfig(t, tmpDir, wsCfg)

	ctx := context.Background()
	stores, err := LoadWorkspaceSymbolStores(ctx, "test-ws", "proj-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer CloseSymbolStores(stores)

	if len(stores) != 1 {
		t.Errorf("expected 1 store when filtering by project, got %d", len(stores))
	}
}

func TestCloseSymbolStores_should_close_all_stores(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	// Create two stores
	store1 := NewGOBSymbolStore(filepath.Join(tmpDir, "s1.gob"))
	_ = store1.Load(ctx)
	store2 := NewGOBSymbolStore(filepath.Join(tmpDir, "s2.gob"))
	_ = store2.Load(ctx)

	stores := []SymbolStore{store1, store2}
	CloseSymbolStores(stores) // Should not panic
}
