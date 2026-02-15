package cli

import (
	"strings"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
)

func TestRunWorkspaceStatus_UINamedWorkspaceStillValidatesName(t *testing.T) {
	tmpDir := t.TempDir()
	cleanup := setTestHomeDirCLI(t, tmpDir)
	defer cleanup()

	wsCfg := config.DefaultWorkspaceConfig()
	ws, err := buildWorkspaceFromFlags("demo", "qdrant", "ollama", "", "", "", "", 0, "", true)
	if err != nil {
		t.Fatalf("buildWorkspaceFromFlags() failed: %v", err)
	}
	if err := wsCfg.AddWorkspace(*ws); err != nil {
		t.Fatalf("AddWorkspace() failed: %v", err)
	}
	if err := config.SaveWorkspaceConfig(wsCfg); err != nil {
		t.Fatalf("SaveWorkspaceConfig() failed: %v", err)
	}

	originalUI := workspaceStatusUI
	originalSelector := workspaceStatusUISelector
	originalRunner := workspaceStatusUIRunner
	defer func() {
		workspaceStatusUI = originalUI
		workspaceStatusUISelector = originalSelector
		workspaceStatusUIRunner = originalRunner
	}()

	workspaceStatusUI = true
	workspaceStatusUISelector = func(isTTY, noUI bool) bool { return true }
	uiCalled := false
	workspaceStatusUIRunner = func(cfg *config.WorkspaceConfig, args []string) error {
		uiCalled = true
		return nil
	}

	err = runWorkspaceStatus(nil, []string{"missing"})
	if err == nil {
		t.Fatal("expected missing workspace error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
	if uiCalled {
		t.Fatal("UI runner should not be called for a missing named workspace")
	}
}
