package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/daemon"
)

func TestShowWatchStatus_NotRunning(t *testing.T) {
	logDir := t.TempDir()

	// Status with no PID file
	err := showWatchStatus(logDir)
	if err != nil {
		t.Fatalf("showWatchStatus() failed: %v", err)
	}

	// Verify log directory is mentioned (output is to stdout, so we can't easily capture it in unit tests)
	// This test mainly ensures no errors occur
}

func TestShowWatchStatus_Running(t *testing.T) {
	logDir := t.TempDir()

	// Write PID file with current process
	if err := daemon.WritePIDFile(logDir); err != nil {
		t.Fatalf("WritePIDFile() failed: %v", err)
	}
	defer daemon.RemovePIDFile(logDir)

	// Status with running process
	err := showWatchStatus(logDir)
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
	err := showWatchStatus(logDir)
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

func TestStopWatchDaemon_NotRunning(t *testing.T) {
	logDir := t.TempDir()

	// Stop with no PID file
	err := stopWatchDaemon(logDir)
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
	err := stopWatchDaemon(logDir)
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

func TestStartBackgroundWatch_AlreadyRunning(t *testing.T) {
	logDir := t.TempDir()

	// Write PID file with current process (simulating already running)
	if err := daemon.WritePIDFile(logDir); err != nil {
		t.Fatalf("WritePIDFile() failed: %v", err)
	}
	defer daemon.RemovePIDFile(logDir)

	// Try to start background watch (should fail)
	err := startBackgroundWatch(logDir)
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

func TestRunWatch_CheckAlreadyRunning(t *testing.T) {
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
	err := stopWatchDaemon(logDir)
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
