package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/mcp"
)

// makeTestProject creates a minimal grepai project in dir and writes cfg to it.
func makeTestProject(t *testing.T, dir string, cfg *config.Config) {
	t.Helper()
	if err := cfg.Save(dir); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}
}

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

func TestResolveRPGEnabled_ReturnsFalseWhenProjectRootEmpty(t *testing.T) {
	if resolveRPGEnabled("") {
		t.Error("expected false for empty projectRoot, got true")
	}
}

func TestResolveRPGEnabled_ReturnsFalseWhenConfigMissing(t *testing.T) {
	dir := t.TempDir() // no .grepai/ directory
	if resolveRPGEnabled(dir) {
		t.Error("expected false when config file is absent, got true")
	}
}

func TestResolveRPGEnabled_ReturnsFalseWhenRPGDisabledInConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig() // RPG.Enabled defaults to false
	makeTestProject(t, dir, cfg)

	if resolveRPGEnabled(dir) {
		t.Error("expected false when RPG is disabled in config, got true")
	}
}

func TestResolveRPGEnabled_ReturnsTrueWhenRPGEnabledInConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.RPG.Enabled = true
	makeTestProject(t, dir, cfg)

	if !resolveRPGEnabled(dir) {
		t.Error("expected true when RPG is enabled in config, got false")
	}
}

// TestIntegration_MCPServerRPGToolsReflectConfig verifies end-to-end that the MCP
// server created by runMCPServe-style logic advertises RPG tools only when the
// project config has RPG enabled.
func TestIntegration_MCPServerRPGToolsReflectConfig(t *testing.T) {
	t.Run("rpg_disabled_no_rpg_tools", func(t *testing.T) {
		dir := t.TempDir()
		cfg := config.DefaultConfig() // RPG.Enabled = false
		makeTestProject(t, dir, cfg)

		rpgEnabled := resolveRPGEnabled(dir)
		if rpgEnabled {
			t.Fatal("expected rpgEnabled=false for project with RPG disabled")
		}

		srv, err := mcp.NewServer(dir, rpgEnabled)
		if err != nil {
			t.Fatalf("mcp.NewServer error: %v", err)
		}

		for _, toolName := range []string{"grepai_rpg_search", "grepai_rpg_fetch", "grepai_rpg_explore"} {
			if _, ok := srv.ListTools()[toolName]; ok {
				t.Errorf("tool %q should not be registered when RPG is disabled", toolName)
			}
		}
	})

	t.Run("rpg_enabled_rpg_tools_present", func(t *testing.T) {
		dir := t.TempDir()
		cfg := config.DefaultConfig()
		cfg.RPG.Enabled = true
		makeTestProject(t, dir, cfg)

		rpgEnabled := resolveRPGEnabled(dir)
		if !rpgEnabled {
			t.Fatal("expected rpgEnabled=true for project with RPG enabled")
		}

		srv, err := mcp.NewServer(dir, rpgEnabled)
		if err != nil {
			t.Fatalf("mcp.NewServer error: %v", err)
		}

		for _, toolName := range []string{"grepai_rpg_search", "grepai_rpg_fetch", "grepai_rpg_explore"} {
			if _, ok := srv.ListTools()[toolName]; !ok {
				t.Errorf("tool %q should be registered when RPG is enabled", toolName)
			}
		}
	})
}
