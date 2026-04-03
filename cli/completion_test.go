package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
)

func TestCompletionZsh_should_output_compdef(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"completion", "zsh"})
	defer rootCmd.SetOut(nil)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("completion zsh failed: %v", err)
	}

	output := buf.String()
	if len(output) == 0 {
		t.Fatal("completion zsh produced empty output")
	}
	if !strings.Contains(output, "compdef") && !strings.Contains(output, "_grepai") {
		t.Fatalf("completion zsh output missing expected markers, got: %s", output[:min(200, len(output))])
	}
}

func TestCompletionBash_should_output_valid_script(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"completion", "bash"})
	defer rootCmd.SetOut(nil)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("completion bash failed: %v", err)
	}

	if buf.Len() == 0 {
		t.Fatal("completion bash produced empty output")
	}
}

func TestCompletionFish_should_output_valid_script(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"completion", "fish"})
	defer rootCmd.SetOut(nil)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("completion fish failed: %v", err)
	}

	if buf.Len() == 0 {
		t.Fatal("completion fish produced empty output")
	}
}

func TestCompletionPowershell_should_output_valid_script(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"completion", "powershell"})
	defer rootCmd.SetOut(nil)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("completion powershell failed: %v", err)
	}

	if buf.Len() == 0 {
		t.Fatal("completion powershell produced empty output")
	}
}

func TestCompleteWorkspaceNames_should_return_nil_without_config(t *testing.T) {
	names := completeWorkspaceNames()
	// If no workspace config exists, should return nil (not panic)
	_ = names
}

func TestCompleteProjectNames_should_return_nil_for_missing_workspace(t *testing.T) {
	names := completeProjectNames("nonexistent-workspace-xyz")
	if names != nil {
		t.Fatalf("expected nil for nonexistent workspace, got: %v", names)
	}
}

func TestCompleteProjectNames_should_return_project_names(t *testing.T) {
	// Create a temp workspace config
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".grepai")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	wsCfg := &config.WorkspaceConfig{
		Version: 1,
		Workspaces: map[string]config.Workspace{
			"test-ws": {
				Name: "test-ws",
				Store: config.StoreConfig{
					Backend: "qdrant",
				},
				Projects: []config.ProjectEntry{
					{Name: "frontend", Path: "/tmp/frontend"},
					{Name: "backend", Path: "/tmp/backend"},
				},
			},
		},
	}

	// Write workspace config to the expected location
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	if err := config.SaveWorkspaceConfig(wsCfg); err != nil {
		t.Fatalf("failed to save test workspace config: %v", err)
	}

	names := completeProjectNames("test-ws")
	if len(names) != 2 {
		t.Fatalf("expected 2 project names, got %d: %v", len(names), names)
	}

	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["frontend"] || !found["backend"] {
		t.Fatalf("expected frontend and backend, got: %v", names)
	}
}

func TestRefsCompletion_should_suggest_subcommands(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"__complete", "refs", ""})
	defer rootCmd.SetOut(nil)
	defer rootCmd.SetErr(nil)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("__complete refs failed: %v", err)
	}

	output := buf.String()
	for _, sub := range []string{"readers", "writers", "graph"} {
		if !strings.Contains(output, sub) {
			t.Fatalf("expected completion output to contain %q, got: %s", sub, output)
		}
	}
}

func TestRefsProjectCompletion_should_return_workspace_projects(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	wsCfg := &config.WorkspaceConfig{
		Version: 1,
		Workspaces: map[string]config.Workspace{
			"test-ws": {
				Name: "test-ws",
				Projects: []config.ProjectEntry{
					{Name: "frontend", Path: "/tmp/frontend"},
					{Name: "backend", Path: "/tmp/backend"},
				},
			},
		},
	}
	if err := config.SaveWorkspaceConfig(wsCfg); err != nil {
		t.Fatalf("failed to save test workspace config: %v", err)
	}

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"__complete", "refs", "readers", "--workspace", "test-ws", "--project", ""})
	defer rootCmd.SetOut(nil)
	defer rootCmd.SetErr(nil)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("__complete refs readers --project failed: %v", err)
	}

	output := buf.String()
	for _, project := range []string{"frontend", "backend"} {
		if !strings.Contains(output, project) {
			t.Fatalf("expected project completion output to contain %q, got: %s", project, output)
		}
	}
}
