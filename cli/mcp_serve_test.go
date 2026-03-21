package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/mcp"
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

		if err := os.MkdirAll(filepath.Join(tmpDir, "pipeline", "src"), 0o755); err != nil {
			t.Fatalf("failed to create pipeline dir: %v", err)
		}
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to get cwd: %v", err)
		}
		defer func() { _ = os.Chdir(wd) }()
		if err := os.Chdir(filepath.Join(tmpDir, "pipeline", "src")); err != nil {
			t.Fatalf("failed to chdir into workspace project: %v", err)
		}

		projectRoot, wsName, err := resolveMCPTarget("", "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if wsName != "test" {
			t.Errorf("expected workspace test, got %s", wsName)
		}
		if projectRoot != filepath.Join(tmpDir, "pipeline") {
			t.Fatalf("expected projectRoot %q, got %q", filepath.Join(tmpDir, "pipeline"), projectRoot)
		}
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

func TestRunMCPServe_RefreshesWorkspaceProjectBeforeServe(t *testing.T) {
	oldResolve := mcpResolveTargetRunner
	oldRefresh := mcpRefreshStartupRunner
	oldNewServer := mcpNewServerRunner
	oldNewServerWithWorkspace := mcpNewServerWithWorkspaceRunner
	oldServe := mcpServeRunner
	defer func() {
		mcpResolveTargetRunner = oldResolve
		mcpRefreshStartupRunner = oldRefresh
		mcpNewServerRunner = oldNewServer
		mcpNewServerWithWorkspaceRunner = oldNewServerWithWorkspace
		mcpServeRunner = oldServe
	}()

	cmd := &cobra.Command{}
	cmd.Flags().String("workspace", "", "")
	if err := cmd.Flags().Set("workspace", "test"); err != nil {
		t.Fatalf("failed to set workspace flag: %v", err)
	}

	var resolvedExplicitPath, resolvedWorkspace string
	mcpResolveTargetRunner = func(explicitPath, workspaceFlag string) (string, string, error) {
		resolvedExplicitPath = explicitPath
		resolvedWorkspace = workspaceFlag
		return "/tmp/project", "test", nil
	}

	var refreshProjectRoot, refreshWorkspace string
	mcpRefreshStartupRunner = func(_ context.Context, projectRoot, workspaceName string) error {
		refreshProjectRoot = projectRoot
		refreshWorkspace = workspaceName
		return nil
	}

	var serverProjectRoot, serverWorkspace string
	mcpNewServerWithWorkspaceRunner = func(projectRoot, workspaceName string) (*mcp.Server, error) {
		serverProjectRoot = projectRoot
		serverWorkspace = workspaceName
		return &mcp.Server{}, nil
	}
	mcpNewServerRunner = func(projectRoot string) (*mcp.Server, error) {
		t.Fatalf("unexpected non-workspace server creation for projectRoot %q", projectRoot)
		return nil, nil
	}

	served := false
	mcpServeRunner = func(_ *mcp.Server) error {
		served = true
		return nil
	}

	if err := runMCPServe(cmd, nil); err != nil {
		t.Fatalf("runMCPServe failed: %v", err)
	}
	if resolvedExplicitPath != "" || resolvedWorkspace != "test" {
		t.Fatalf("resolve args = (%q, %q), want (\"\", \"test\")", resolvedExplicitPath, resolvedWorkspace)
	}
	if refreshProjectRoot != "/tmp/project" || refreshWorkspace != "test" {
		t.Fatalf("refresh args = (%q, %q), want (%q, %q)", refreshProjectRoot, refreshWorkspace, "/tmp/project", "test")
	}
	if serverProjectRoot != "/tmp/project" || serverWorkspace != "test" {
		t.Fatalf("workspace server args = (%q, %q), want (%q, %q)", serverProjectRoot, serverWorkspace, "/tmp/project", "test")
	}
	if !served {
		t.Fatal("expected MCP server to be served")
	}
}

func TestRunMCPServe_ContinuesWhenRefreshFails(t *testing.T) {
	oldResolve := mcpResolveTargetRunner
	oldRefresh := mcpRefreshStartupRunner
	oldNewServer := mcpNewServerRunner
	oldNewServerWithWorkspace := mcpNewServerWithWorkspaceRunner
	oldServe := mcpServeRunner
	defer func() {
		mcpResolveTargetRunner = oldResolve
		mcpRefreshStartupRunner = oldRefresh
		mcpNewServerRunner = oldNewServer
		mcpNewServerWithWorkspaceRunner = oldNewServerWithWorkspace
		mcpServeRunner = oldServe
	}()

	cmd := &cobra.Command{}

	mcpResolveTargetRunner = func(explicitPath, workspaceFlag string) (string, string, error) {
		return "/tmp/project", "", nil
	}
	mcpRefreshStartupRunner = func(_ context.Context, projectRoot, workspaceName string) error {
		if projectRoot != "/tmp/project" || workspaceName != "" {
			t.Fatalf("refresh args = (%q, %q), want (%q, %q)", projectRoot, workspaceName, "/tmp/project", "")
		}
		return errors.New("refresh failed")
	}
	mcpNewServerRunner = func(projectRoot string) (*mcp.Server, error) {
		if projectRoot != "/tmp/project" {
			t.Fatalf("project server root = %q, want %q", projectRoot, "/tmp/project")
		}
		return &mcp.Server{}, nil
	}
	mcpNewServerWithWorkspaceRunner = func(projectRoot, workspaceName string) (*mcp.Server, error) {
		t.Fatalf("unexpected workspace server creation for (%q, %q)", projectRoot, workspaceName)
		return nil, nil
	}

	served := false
	mcpServeRunner = func(_ *mcp.Server) error {
		served = true
		return nil
	}

	if err := runMCPServe(cmd, nil); err != nil {
		t.Fatalf("runMCPServe failed: %v", err)
	}
	if !served {
		t.Fatal("expected MCP server to be served even when refresh fails")
	}
}
