package cli

import (
	"os"
	"path/filepath"
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
}
