package cli

import (
	"fmt"
	"os"
	"os/exec"
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

func TestSaveWatchLogDirHint_StoresAbsolutePath(t *testing.T) {
	projectRoot := t.TempDir()
	baseDir := t.TempDir()

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalWD)
	}()
	if err := os.Chdir(baseDir); err != nil {
		t.Fatalf("Chdir() failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "logs"), 0755); err != nil {
		t.Fatalf("MkdirAll() failed: %v", err)
	}

	if err := saveWatchLogDirHint(projectRoot, "./logs"); err != nil {
		t.Fatalf("saveWatchLogDirHint() failed: %v", err)
	}

	hinted, err := readWatchLogDirHint(projectRoot)
	if err != nil {
		t.Fatalf("readWatchLogDirHint() failed: %v", err)
	}

	expected := filepath.Join(baseDir, "logs")
	if canonicalPath(hinted) != canonicalPath(expected) {
		t.Fatalf("hinted log dir = %q, want %q", hinted, expected)
	}
	if !filepath.IsAbs(hinted) {
		t.Fatalf("hinted log dir should be absolute: %q", hinted)
	}
}

func TestResolveWatcherRuntimeStatus_WorktreeFallsBackToLegacyPID(t *testing.T) {
	tmpHome := t.TempDir()
	cleanupHome := setTestHomeDirCLI(t, tmpHome)
	defer cleanupHome()

	projectRoot := t.TempDir()
	customLogDir := t.TempDir()

	initGitRepoCmd := exec.Command("git", "init", projectRoot)
	if output, err := initGitRepoCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v (%s)", err, string(output))
	}

	if err := saveWatchLogDirHint(projectRoot, customLogDir); err != nil {
		t.Fatalf("saveWatchLogDirHint() failed: %v", err)
	}

	pidPath := filepath.Join(customLogDir, "grepai-watch.pid")
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0600); err != nil {
		t.Fatalf("failed to write legacy pid file: %v", err)
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
		t.Fatalf("expected running status from legacy pid fallback, got %+v", status)
	}
	if status.pid != os.Getpid() {
		t.Fatalf("pid = %d, want %d", status.pid, os.Getpid())
	}
	if filepath.Clean(status.logFile) != filepath.Join(filepath.Clean(customLogDir), "grepai-watch.log") {
		t.Fatalf("logFile = %q, want %q", status.logFile, filepath.Join(customLogDir, "grepai-watch.log"))
	}
}
