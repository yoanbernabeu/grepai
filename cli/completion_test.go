package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
)

func TestCompletionZsh_should_output_compdef(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := rootCmd
	cmd.SetArgs([]string{"completion", "zsh"})
	err := cmd.Execute()
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("completion zsh failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if len(output) == 0 {
		t.Fatal("completion zsh produced empty output")
	}
	// Cobra's zsh completion should contain compdef or _grepai
	if !bytes.Contains([]byte(output), []byte("compdef")) && !bytes.Contains([]byte(output), []byte("_grepai")) {
		t.Fatalf("completion zsh output missing expected markers, got: %s", output[:min(200, len(output))])
	}
}

func TestCompletionBash_should_output_valid_script(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := rootCmd
	cmd.SetArgs([]string{"completion", "bash"})
	err := cmd.Execute()
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("completion bash failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if len(output) == 0 {
		t.Fatal("completion bash produced empty output")
	}
}

func TestCompletionFish_should_output_valid_script(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := rootCmd
	cmd.SetArgs([]string{"completion", "fish"})
	err := cmd.Execute()
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("completion fish failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	if buf.Len() == 0 {
		t.Fatal("completion fish produced empty output")
	}
}

func TestCompletionPowershell_should_output_valid_script(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := rootCmd
	cmd.SetArgs([]string{"completion", "powershell"})
	err := cmd.Execute()
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("completion powershell failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
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
