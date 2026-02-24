package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
)

func TestResolveMCPWorkspace(t *testing.T) {
	t.Run("explicit_workspace_flag", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "grepai-mcp-test")
		defer os.RemoveAll(tmpDir)
		cleanup := setTestHomeDirCLI(t, tmpDir)
		defer cleanup()

		cfg := config.DefaultWorkspaceConfig()
		cfg.AddWorkspace(config.Workspace{
			Name:  "test",
			Store: config.StoreConfig{Backend: "qdrant"},
			Embedder: config.EmbedderConfig{
				Provider: "ollama",
				Model:    "nomic-embed-text",
			},
			Projects: []config.ProjectEntry{
				{Name: "pipeline", Path: filepath.Join(tmpDir, "pipeline")},
			},
		})
		config.SaveWorkspaceConfig(cfg)

		projectRoot, wsName, err := resolveMCPTarget("", "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if wsName != "test" {
			t.Errorf("expected workspace test, got %s", wsName)
		}
		_ = projectRoot
	})

	t.Run("explicit_workspace_not_found", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "grepai-mcp-test")
		defer os.RemoveAll(tmpDir)
		cleanup := setTestHomeDirCLI(t, tmpDir)
		defer cleanup()

		_, _, err := resolveMCPTarget("", "nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent workspace")
		}
	})

	t.Run("explicit_project_path", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "grepai-mcp-test")
		defer os.RemoveAll(tmpDir)

		grepaiDir := filepath.Join(tmpDir, ".grepai")
		os.MkdirAll(grepaiDir, 0755)
		os.WriteFile(filepath.Join(grepaiDir, "config.yaml"), []byte("version: 1\n"), 0644)

		projectRoot, wsName, err := resolveMCPTarget(tmpDir, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if wsName != "" {
			t.Errorf("expected empty workspace name, got %s", wsName)
		}
		if projectRoot != tmpDir {
			t.Errorf("expected projectRoot %s, got %s", tmpDir, projectRoot)
		}
	})

	t.Run("explicit_project_path_no_config", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "grepai-mcp-test")
		defer os.RemoveAll(tmpDir)

		_, _, err := resolveMCPTarget(tmpDir, "")
		if err == nil {
			t.Error("expected error when no .grepai/ at path")
		}
	})

	t.Run("no_local_project_uses_runtime_workspace_when_configured", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "grepai-mcp-test")
		defer os.RemoveAll(tmpDir)
		cleanup := setTestHomeDirCLI(t, tmpDir)
		defer cleanup()

		cfg := config.DefaultWorkspaceConfig()
		cfg.AddWorkspace(config.Workspace{
			Name:  "runtime-only",
			Store: config.StoreConfig{Backend: "qdrant"},
			Embedder: config.EmbedderConfig{
				Provider: "ollama",
				Model:    "nomic-embed-text",
			},
			Projects: []config.ProjectEntry{
				{Name: "service", Path: filepath.Join(tmpDir, "service")},
			},
		})
		if err := config.SaveWorkspaceConfig(cfg); err != nil {
			t.Fatalf("failed to save workspace config: %v", err)
		}

		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to get cwd: %v", err)
		}
		defer func() { _ = os.Chdir(wd) }()

		emptyDir := filepath.Join(tmpDir, "empty")
		if err := os.MkdirAll(emptyDir, 0o755); err != nil {
			t.Fatalf("failed to create empty dir: %v", err)
		}
		if err := os.Chdir(emptyDir); err != nil {
			t.Fatalf("failed to chdir: %v", err)
		}

		projectRoot, wsName, err := resolveMCPTarget("", "")
		if err != nil {
			t.Fatalf("expected fallback startup, got error: %v", err)
		}
		if projectRoot != "" || wsName != "" {
			t.Fatalf("expected unscoped startup (\"\", \"\"), got (%q, %q)", projectRoot, wsName)
		}
	})

	t.Run("no_local_project_and_no_workspace_config_still_errors", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "grepai-mcp-test")
		defer os.RemoveAll(tmpDir)
		cleanup := setTestHomeDirCLI(t, tmpDir)
		defer cleanup()

		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to get cwd: %v", err)
		}
		defer func() { _ = os.Chdir(wd) }()

		emptyDir := filepath.Join(tmpDir, "empty")
		if err := os.MkdirAll(emptyDir, 0o755); err != nil {
			t.Fatalf("failed to create empty dir: %v", err)
		}
		if err := os.Chdir(emptyDir); err != nil {
			t.Fatalf("failed to chdir: %v", err)
		}

		_, _, err = resolveMCPTarget("", "")
		if err == nil {
			t.Fatal("expected error when no local project and no workspace config")
		}
		if !strings.Contains(err.Error(), "no grepai project or workspace found") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
