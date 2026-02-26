package cli

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/daemon"
	"github.com/yoanbernabeu/grepai/watcher"
)

func skipIfWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: cannot delete locked files")
	}
}

func TestShowWatchStatus_NotRunning(t *testing.T) {
	logDir := t.TempDir()

	// Status with no PID file
	err := showWatchStatus(logDir, "")
	if err != nil {
		t.Fatalf("showWatchStatus() failed: %v", err)
	}

	// Verify log directory is mentioned (output is to stdout, so we can't easily capture it in unit tests)
	// This test mainly ensures no errors occur
}

func TestShowWatchStatus_Running(t *testing.T) {
	skipIfWindows(t)
	logDir := t.TempDir()

	// Write PID file with current process
	if err := daemon.WritePIDFile(logDir); err != nil {
		t.Fatalf("WritePIDFile() failed: %v", err)
	}
	defer daemon.RemovePIDFile(logDir)

	// Status with running process
	err := showWatchStatus(logDir, "")
	if err != nil {
		t.Fatalf("showWatchStatus() failed: %v", err)
	}
}

func TestShowWatchStatus_StalePID(t *testing.T) {
	logDir := t.TempDir()
	pidPath := filepath.Join(logDir, "grepai-watch.pid")

	// Write stale PID (very high number unlikely to exist)
	if err := os.WriteFile(pidPath, []byte("9999999\n"), 0644); err != nil {
		t.Fatalf("Failed to write stale PID: %v", err)
	}

	// Status should clean stale PID
	err := showWatchStatus(logDir, "")
	if err != nil {
		t.Fatalf("showWatchStatus() failed: %v", err)
	}

	// Verify PID file was removed (if process 9999999 doesn't exist)
	if !daemon.IsProcessRunning(9999999) {
		if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
			t.Error("Stale PID file was not removed")
		}
	}
}

func TestShowWatchStatus_WorktreeNotRunning(t *testing.T) {
	logDir := t.TempDir()
	err := showWatchStatus(logDir, "wt-1")
	if err != nil {
		t.Fatalf("showWatchStatus() failed: %v", err)
	}
}

func TestShowWatchStatus_WorktreeRunning(t *testing.T) {
	logDir := t.TempDir()
	worktreeID := "wt-2"

	pidPath := daemon.GetWorktreePIDFile(logDir, worktreeID)
	content := strconv.Itoa(os.Getpid()) + "\n"
	if err := os.WriteFile(pidPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write worktree PID file: %v", err)
	}

	err := showWatchStatus(logDir, worktreeID)
	if err != nil {
		t.Fatalf("showWatchStatus() failed: %v", err)
	}
}

func TestStopWatchDaemon_NotRunning(t *testing.T) {
	logDir := t.TempDir()

	// Stop with no PID file
	_, err := stopWatchDaemon(logDir, "")
	if err != nil {
		t.Fatalf("stopWatchDaemon() failed: %v", err)
	}
}

func TestStopWatchDaemon_StalePID(t *testing.T) {
	logDir := t.TempDir()
	pidPath := filepath.Join(logDir, "grepai-watch.pid")

	// Write stale PID
	if err := os.WriteFile(pidPath, []byte("9999999\n"), 0644); err != nil {
		t.Fatalf("Failed to write stale PID: %v", err)
	}

	// Stop should clean stale PID
	_, err := stopWatchDaemon(logDir, "")
	if err != nil {
		t.Fatalf("stopWatchDaemon() failed: %v", err)
	}

	// Verify PID file was removed (if process 9999999 doesn't exist)
	if !daemon.IsProcessRunning(9999999) {
		if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
			t.Error("Stale PID file was not removed")
		}
	}
}

func TestStopWatchDaemon_WorktreeNotRunning(t *testing.T) {
	logDir := t.TempDir()
	_, err := stopWatchDaemon(logDir, "wt-3")
	if err != nil {
		t.Fatalf("stopWatchDaemon() failed: %v", err)
	}
}

func TestStartBackgroundWatch_AlreadyRunning(t *testing.T) {
	skipIfWindows(t)
	logDir := t.TempDir()

	// Write PID file with current process (simulating already running)
	if err := daemon.WritePIDFile(logDir); err != nil {
		t.Fatalf("WritePIDFile() failed: %v", err)
	}
	defer daemon.RemovePIDFile(logDir)

	// Try to start background watch (should fail)
	err := startBackgroundWatch(logDir, "")
	if err == nil {
		t.Fatal("startBackgroundWatch() should have failed when already running")
	}
}

func TestStartBackgroundWatch_CleansStalePID(t *testing.T) {
	logDir := t.TempDir()
	pidPath := filepath.Join(logDir, "grepai-watch.pid")

	// Write stale PID
	if err := os.WriteFile(pidPath, []byte("9999999\n"), 0644); err != nil {
		t.Fatalf("Failed to write stale PID: %v", err)
	}

	// Note: We can't easily test the actual spawning without integration tests,
	// but we can verify the stale PID check works

	// Read PID before
	pid, err := daemon.ReadPIDFile(logDir)
	if err != nil {
		t.Fatalf("ReadPIDFile() failed: %v", err)
	}
	if pid != 9999999 {
		t.Fatalf("Expected PID 9999999, got %d", pid)
	}

	// The actual startBackgroundWatch would spawn a process here,
	// which we can't easily test in a unit test.
	// This is better tested in integration tests.
}

func TestStartBackgroundWatch_WorktreeAlreadyRunning(t *testing.T) {
	logDir := t.TempDir()
	worktreeID := "wt-4"

	pidPath := daemon.GetWorktreePIDFile(logDir, worktreeID)
	content := strconv.Itoa(os.Getpid()) + "\n"
	if err := os.WriteFile(pidPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write worktree PID file: %v", err)
	}

	originalWatchLogDir := watchLogDir
	watchLogDir = ""
	defer func() { watchLogDir = originalWatchLogDir }()

	err := startBackgroundWatch(logDir, worktreeID)
	if err == nil {
		t.Fatal("startBackgroundWatch() should have failed when worktree watcher already running")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Fatalf("startBackgroundWatch() error = %q, want message containing %q", err.Error(), "already running")
	}
}

func TestRunWatch_CheckAlreadyRunning(t *testing.T) {
	skipIfWindows(t)
	logDir := t.TempDir()

	// Set custom log dir for testing
	watchLogDir = logDir
	watchBackground = false
	watchStatus = false
	watchStop = false

	// Write PID file with current process
	if err := daemon.WritePIDFile(logDir); err != nil {
		t.Fatalf("WritePIDFile() failed: %v", err)
	}
	defer daemon.RemovePIDFile(logDir)

	// Try to run watch in foreground (should detect already running)
	err := runWatch(nil, nil)
	if err == nil {
		t.Fatal("runWatch() should have failed when already running in background")
	}

	// Reset for other tests
	watchLogDir = ""
}

func TestRunWatchStatus_OutsideProjectRoot(t *testing.T) {
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	outside := t.TempDir()
	if err := os.Chdir(outside); err != nil {
		t.Fatalf("Chdir() failed: %v", err)
	}

	oldBackground := watchBackground
	oldStatus := watchStatus
	oldStop := watchStop
	oldWorkspace := watchWorkspace
	oldLogDir := watchLogDir
	oldNoUI := watchNoUI
	defer func() {
		watchBackground = oldBackground
		watchStatus = oldStatus
		watchStop = oldStop
		watchWorkspace = oldWorkspace
		watchLogDir = oldLogDir
		watchNoUI = oldNoUI
	}()

	watchBackground = false
	watchStatus = true
	watchStop = false
	watchWorkspace = ""
	watchNoUI = false
	watchLogDir = t.TempDir()

	if err := runWatch(nil, nil); err != nil {
		t.Fatalf("runWatch(--status) outside project should not fail: %v", err)
	}
}

func TestRunWatchStop_UsesHintedLogDirAndClearsHint(t *testing.T) {
	tmpHome := t.TempDir()
	cleanupHome := setTestHomeDirCLI(t, tmpHome)
	defer cleanupHome()

	projectRoot := t.TempDir()
	if err := config.DefaultConfig().Save(projectRoot); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}
	hintedLogDir := filepath.Join(projectRoot, "custom-logs")
	if err := saveWatchLogDirHint(projectRoot, hintedLogDir); err != nil {
		t.Fatalf("saveWatchLogDirHint() failed: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()
	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf("Chdir() failed: %v", err)
	}

	oldStopRunner := watchStopDaemonRunner
	defer func() { watchStopDaemonRunner = oldStopRunner }()
	var called []string
	watchStopDaemonRunner = func(logDir, worktreeID string) (bool, error) {
		called = append(called, filepath.Clean(logDir))
		return filepath.Clean(logDir) == filepath.Clean(hintedLogDir), nil
	}

	oldBackground := watchBackground
	oldStatus := watchStatus
	oldStop := watchStop
	oldWorkspace := watchWorkspace
	oldLogDir := watchLogDir
	oldNoUI := watchNoUI
	defer func() {
		watchBackground = oldBackground
		watchStatus = oldStatus
		watchStop = oldStop
		watchWorkspace = oldWorkspace
		watchLogDir = oldLogDir
		watchNoUI = oldNoUI
	}()
	watchBackground = false
	watchStatus = false
	watchStop = true
	watchWorkspace = ""
	watchLogDir = ""
	watchNoUI = false

	if err := runWatch(nil, nil); err != nil {
		t.Fatalf("runWatch(--stop) failed: %v", err)
	}
	if len(called) == 0 || filepath.Clean(called[0]) != filepath.Clean(hintedLogDir) {
		t.Fatalf("expected hinted log dir to be tried first, got %v", called)
	}
	hinted, err := readWatchLogDirHint(projectRoot)
	if err != nil {
		t.Fatalf("readWatchLogDirHint() failed: %v", err)
	}
	if hinted != "" {
		t.Fatalf("expected hint to be cleared after successful stop, got %q", hinted)
	}
}

func TestRunWatchStop_DoesNotClearHintWhenNothingStopped(t *testing.T) {
	tmpHome := t.TempDir()
	cleanupHome := setTestHomeDirCLI(t, tmpHome)
	defer cleanupHome()

	projectRoot := t.TempDir()
	if err := config.DefaultConfig().Save(projectRoot); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}
	hintedLogDir := filepath.Join(projectRoot, "custom-logs")
	if err := saveWatchLogDirHint(projectRoot, hintedLogDir); err != nil {
		t.Fatalf("saveWatchLogDirHint() failed: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()
	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf("Chdir() failed: %v", err)
	}

	oldStopRunner := watchStopDaemonRunner
	defer func() { watchStopDaemonRunner = oldStopRunner }()
	watchStopDaemonRunner = func(logDir, worktreeID string) (bool, error) {
		return false, nil
	}

	oldBackground := watchBackground
	oldStatus := watchStatus
	oldStop := watchStop
	oldWorkspace := watchWorkspace
	oldLogDir := watchLogDir
	oldNoUI := watchNoUI
	defer func() {
		watchBackground = oldBackground
		watchStatus = oldStatus
		watchStop = oldStop
		watchWorkspace = oldWorkspace
		watchLogDir = oldLogDir
		watchNoUI = oldNoUI
	}()
	watchBackground = false
	watchStatus = false
	watchStop = true
	watchWorkspace = ""
	watchLogDir = ""
	watchNoUI = false

	if err := runWatch(nil, nil); err != nil {
		t.Fatalf("runWatch(--stop) failed: %v", err)
	}
	hinted, err := readWatchLogDirHint(projectRoot)
	if err != nil {
		t.Fatalf("readWatchLogDirHint() failed: %v", err)
	}
	if filepath.Clean(hinted) != filepath.Clean(hintedLogDir) {
		t.Fatalf("expected hint to remain when no watcher stopped, got %q", hinted)
	}
}

func TestRunWatch_MultiWorktreeUsesUIForeground(t *testing.T) {
	oldBackground := watchBackground
	oldStatus := watchStatus
	oldStop := watchStop
	oldWorkspace := watchWorkspace
	oldLogDir := watchLogDir
	oldNoUI := watchNoUI
	defer func() {
		watchBackground = oldBackground
		watchStatus = oldStatus
		watchStop = oldStop
		watchWorkspace = oldWorkspace
		watchLogDir = oldLogDir
		watchNoUI = oldNoUI
	}()

	oldInteractive := watchIsInteractiveTerminal
	oldSelector := watchUseUISelector
	oldForeground := watchForegroundRunner
	oldForegroundUI := watchForegroundUIRunner
	defer func() {
		watchIsInteractiveTerminal = oldInteractive
		watchUseUISelector = oldSelector
		watchForegroundRunner = oldForeground
		watchForegroundUIRunner = oldForegroundUI
	}()

	watchBackground = false
	watchStatus = false
	watchStop = false
	watchWorkspace = ""
	watchNoUI = false
	watchLogDir = t.TempDir()

	watchIsInteractiveTerminal = func() bool { return true }
	watchUseUISelector = func(isTTY, noUI, background, status, stop bool, workspace string) bool { return true }

	foregroundErr := errors.New("foreground-called")
	watchForegroundRunner = func() error { return foregroundErr }
	uiErr := errors.New("ui-called")
	watchForegroundUIRunner = func() error { return uiErr }

	err := runWatch(nil, nil)
	if !errors.Is(err, uiErr) {
		t.Fatalf("expected UI path for interactive watch, got: %v", err)
	}
}

func TestLogDirectoryDefaults(t *testing.T) {
	// Test that default log directory can be determined
	logDir, err := daemon.GetDefaultLogDir()
	if err != nil {
		t.Fatalf("GetDefaultLogDir() failed: %v", err)
	}

	if logDir == "" {
		t.Fatal("GetDefaultLogDir() returned empty string")
	}

	if !filepath.IsAbs(logDir) {
		t.Errorf("Expected absolute path, got: %s", logDir)
	}
}

func TestPIDFileLifecycleInWatch(t *testing.T) {
	skipIfWindows(t)
	logDir := t.TempDir()

	// Initially no PID file
	pid, err := daemon.ReadPIDFile(logDir)
	if err != nil {
		t.Fatalf("ReadPIDFile() failed: %v", err)
	}
	if pid != 0 {
		t.Errorf("Expected no PID initially, got %d", pid)
	}

	// Write PID file
	if err := daemon.WritePIDFile(logDir); err != nil {
		t.Fatalf("WritePIDFile() failed: %v", err)
	}

	// Verify it exists
	pid, err = daemon.ReadPIDFile(logDir)
	if err != nil {
		t.Fatalf("ReadPIDFile() failed: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("Expected PID %d, got %d", os.Getpid(), pid)
	}

	// Remove it
	if err := daemon.RemovePIDFile(logDir); err != nil {
		t.Fatalf("RemovePIDFile() failed: %v", err)
	}

	// Verify it's gone
	pid, err = daemon.ReadPIDFile(logDir)
	if err != nil {
		t.Fatalf("ReadPIDFile() failed: %v", err)
	}
	if pid != 0 {
		t.Errorf("Expected no PID after removal, got %d", pid)
	}
}

func TestCustomLogDirectory(t *testing.T) {
	skipIfWindows(t)
	customDir := filepath.Join(t.TempDir(), "custom-logs")

	// Set custom log dir
	watchLogDir = customDir
	defer func() { watchLogDir = "" }()

	// Verify custom directory is used
	if watchLogDir != customDir {
		t.Errorf("Expected custom log dir %s, got %s", customDir, watchLogDir)
	}

	// Test that PID files can be written to custom directory
	if err := daemon.WritePIDFile(customDir); err != nil {
		t.Fatalf("WritePIDFile() to custom dir failed: %v", err)
	}
	defer daemon.RemovePIDFile(customDir)

	// Verify PID file exists in custom directory
	pidPath := filepath.Join(customDir, "grepai-watch.pid")
	if _, err := os.Stat(pidPath); os.IsNotExist(err) {
		t.Error("PID file not found in custom directory")
	}
}

func TestStopWatchDaemon_WaitForShutdown(t *testing.T) {
	// This test verifies the wait logic in stopWatchDaemon
	// We can't easily test the actual process stopping in a unit test,
	// but we can verify the logic with a stale PID

	logDir := t.TempDir()
	pidPath := filepath.Join(logDir, "grepai-watch.pid")

	// Write stale PID
	if err := os.WriteFile(pidPath, []byte("9999998\n"), 0644); err != nil {
		t.Fatalf("Failed to write stale PID: %v", err)
	}

	// Measure time taken
	start := time.Now()
	_, err := stopWatchDaemon(logDir, "")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("stopWatchDaemon() failed: %v", err)
	}

	// Should complete quickly for non-existent process (less than 1 second)
	if elapsed > 2*time.Second {
		t.Errorf("stopWatchDaemon() took too long: %v", elapsed)
	}

	// Verify PID file was cleaned up (if process doesn't exist)
	if !daemon.IsProcessRunning(9999998) {
		if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
			t.Error("PID file was not removed after stop")
		}
	}
}

func TestIsTracedLanguage_should_match_known_extensions(t *testing.T) {
	langs := []string{".go", ".js", ".ts", ".py"}

	if !isTracedLanguage(".go", langs) {
		t.Error("expected .go to be traced")
	}
	if !isTracedLanguage(".py", langs) {
		t.Error("expected .py to be traced")
	}
	if isTracedLanguage(".rs", langs) {
		t.Error("expected .rs to NOT be traced")
	}
	if isTracedLanguage("", langs) {
		t.Error("expected empty string to NOT be traced")
	}
}

func TestIsTracedLanguage_should_return_false_for_empty_list(t *testing.T) {
	if isTracedLanguage(".go", nil) {
		t.Error("expected false for nil languages list")
	}
	if isTracedLanguage(".go", []string{}) {
		t.Error("expected false for empty languages list")
	}
}

func TestStopWorkspaceWatchDaemon_should_handle_not_running(t *testing.T) {
	logDir := t.TempDir()
	err := stopWorkspaceWatchDaemon(logDir, "test-ws")
	if err != nil {
		t.Fatalf("stopWorkspaceWatchDaemon() failed: %v", err)
	}
}

func TestStopWorkspaceWatchDaemon_should_handle_stale_pid(t *testing.T) {
	logDir := t.TempDir()
	pidPath := daemon.GetWorkspacePIDFile(logDir, "test-ws")
	if err := os.WriteFile(pidPath, []byte("9999997\n"), 0644); err != nil {
		t.Fatalf("failed to write PID file: %v", err)
	}

	start := time.Now()
	err := stopWorkspaceWatchDaemon(logDir, "test-ws")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("stopWorkspaceWatchDaemon() failed: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Errorf("stopWorkspaceWatchDaemon() took too long: %v", elapsed)
	}
}

func TestWorkspaceWatchEvent_should_carry_project_path(t *testing.T) {
	evt := workspaceWatchEvent{
		projectPath: "/home/user/projects/myapp",
		event: watcher.FileEvent{
			Type: watcher.EventCreate,
			Path: "src/main.go",
		},
	}

	if evt.event.Type != watcher.EventCreate {
		t.Errorf("event type = %v, want EventCreate", evt.event.Type)
	}
	if evt.event.Path != "src/main.go" {
		t.Errorf("event path = %q, want %q", evt.event.Path, "src/main.go")
	}
	if evt.projectPath != "/home/user/projects/myapp" {
		t.Errorf("project path = %q, want %q", evt.projectPath, "/home/user/projects/myapp")
	}
}
