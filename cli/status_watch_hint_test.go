package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWatcherRuntimeStatus_UsesHintedLogDir(t *testing.T) {
	tmpHome := t.TempDir()
	cleanupHome := setTestHomeDirCLI(t, tmpHome)
	defer cleanupHome()

	projectRoot := t.TempDir()
	customLogDir := t.TempDir()

	if err := saveWatchLogDirHint(projectRoot, customLogDir); err != nil {
		t.Fatalf("saveWatchLogDirHint() failed: %v", err)
	}

	pidPath := filepath.Join(customLogDir, "grepai-watch.pid")
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0600); err != nil {
		t.Fatalf("failed to write pid file: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalWD)
	}()
	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf("Chdir() failed: %v", err)
	}

	status := resolveWatcherRuntimeStatus(projectRoot)
	if !status.running {
		t.Fatalf("expected running status, got %+v", status)
	}
	if status.pid != os.Getpid() {
		t.Fatalf("pid = %d, want %d", status.pid, os.Getpid())
	}
	if filepath.Clean(status.logDir) != filepath.Clean(customLogDir) {
		t.Fatalf("logDir = %q, want %q", status.logDir, customLogDir)
	}
}
