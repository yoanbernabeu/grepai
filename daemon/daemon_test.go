package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestGetDefaultLogDir(t *testing.T) {
	logDir, err := GetDefaultLogDir()
	if err != nil {
		t.Fatalf("GetDefaultLogDir() failed: %v", err)
	}

	if logDir == "" {
		t.Fatal("GetDefaultLogDir() returned empty string")
	}

	// Check platform-specific expectations
	switch runtime.GOOS {
	case "darwin":
		if !filepath.IsAbs(logDir) {
			t.Errorf("Expected absolute path, got: %s", logDir)
		}
		if !contains(logDir, "Library/Logs/grepai") {
			t.Errorf("Expected path to contain 'Library/Logs/grepai', got: %s", logDir)
		}
	case "windows":
		if !filepath.IsAbs(logDir) {
			t.Errorf("Expected absolute path, got: %s", logDir)
		}
		if !contains(logDir, "grepai") {
			t.Errorf("Expected path to contain 'grepai', got: %s", logDir)
		}
	default: // Linux and other Unix-like
		if !filepath.IsAbs(logDir) {
			t.Errorf("Expected absolute path, got: %s", logDir)
		}
		if !contains(logDir, "grepai") {
			t.Errorf("Expected path to contain 'grepai', got: %s", logDir)
		}
	}
}

func TestWriteAndReadPIDFile(t *testing.T) {
	logDir := t.TempDir()

	// Write PID file
	if err := WritePIDFile(logDir); err != nil {
		t.Fatalf("WritePIDFile() failed: %v", err)
	}

	// Verify file exists
	pidPath := filepath.Join(logDir, "grepai-watch.pid")
	if _, err := os.Stat(pidPath); os.IsNotExist(err) {
		t.Fatal("PID file was not created")
	}

	// Read PID file
	pid, err := ReadPIDFile(logDir)
	if err != nil {
		t.Fatalf("ReadPIDFile() failed: %v", err)
	}

	// Verify PID matches current process
	expectedPID := os.Getpid()
	if pid != expectedPID {
		t.Errorf("ReadPIDFile() = %d, want %d", pid, expectedPID)
	}
}

func TestReadPIDFile_NotExists(t *testing.T) {
	logDir := t.TempDir()

	// Read non-existent PID file
	pid, err := ReadPIDFile(logDir)
	if err != nil {
		t.Fatalf("ReadPIDFile() failed: %v", err)
	}

	if pid != 0 {
		t.Errorf("ReadPIDFile() = %d, want 0", pid)
	}
}

func TestReadPIDFile_InvalidContent(t *testing.T) {
	logDir := t.TempDir()
	pidPath := filepath.Join(logDir, "grepai-watch.pid")

	// Write invalid content
	if err := os.WriteFile(pidPath, []byte("not-a-number\n"), 0644); err != nil {
		t.Fatalf("Failed to write invalid PID file: %v", err)
	}

	// Read should fail
	_, err := ReadPIDFile(logDir)
	if err == nil {
		t.Fatal("ReadPIDFile() should have failed with invalid content")
	}
}

func TestRemovePIDFile(t *testing.T) {
	logDir := t.TempDir()

	// Write PID file
	if err := WritePIDFile(logDir); err != nil {
		t.Fatalf("WritePIDFile() failed: %v", err)
	}

	// Remove PID file
	if err := RemovePIDFile(logDir); err != nil {
		t.Fatalf("RemovePIDFile() failed: %v", err)
	}

	// Verify file is gone
	pidPath := filepath.Join(logDir, "grepai-watch.pid")
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatal("PID file still exists after removal")
	}

	// Removing again should not error
	if err := RemovePIDFile(logDir); err != nil {
		t.Fatalf("RemovePIDFile() failed on non-existent file: %v", err)
	}
}

func TestIsProcessRunning(t *testing.T) {
	// Test with current process (should be running)
	currentPID := os.Getpid()
	if !IsProcessRunning(currentPID) {
		t.Error("IsProcessRunning() returned false for current process")
	}

	// Test with PID 0 (invalid)
	if IsProcessRunning(0) {
		t.Error("IsProcessRunning() returned true for PID 0")
	}

	// Test with negative PID (invalid)
	if IsProcessRunning(-1) {
		t.Error("IsProcessRunning() returned true for negative PID")
	}

	// Test with very high PID (likely not running)
	// Note: We can't guarantee a specific PID won't exist, so we test with a very high number
	if IsProcessRunning(9999999) {
		t.Log("Warning: PID 9999999 appears to be running (rare but possible)")
	}
}

func TestPIDFileLifecycle(t *testing.T) {
	logDir := t.TempDir()

	// Initially, no PID file
	pid, err := ReadPIDFile(logDir)
	if err != nil {
		t.Fatalf("ReadPIDFile() failed: %v", err)
	}
	if pid != 0 {
		t.Errorf("Expected no PID, got %d", pid)
	}

	// Write PID file
	if err := WritePIDFile(logDir); err != nil {
		t.Fatalf("WritePIDFile() failed: %v", err)
	}

	// Read it back
	pid, err = ReadPIDFile(logDir)
	if err != nil {
		t.Fatalf("ReadPIDFile() failed: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("Expected PID %d, got %d", os.Getpid(), pid)
	}

	// Process should be running
	if !IsProcessRunning(pid) {
		t.Error("Current process should be running")
	}

	// Remove PID file
	if err := RemovePIDFile(logDir); err != nil {
		t.Fatalf("RemovePIDFile() failed: %v", err)
	}

	// Read again (should be 0)
	pid, err = ReadPIDFile(logDir)
	if err != nil {
		t.Fatalf("ReadPIDFile() failed: %v", err)
	}
	if pid != 0 {
		t.Errorf("Expected no PID after removal, got %d", pid)
	}
}

func TestConcurrentPIDAccess(t *testing.T) {
	logDir := t.TempDir()

	// Write initial PID
	if err := WritePIDFile(logDir); err != nil {
		t.Fatalf("WritePIDFile() failed: %v", err)
	}

	// Concurrent reads should all succeed
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			pid, err := ReadPIDFile(logDir)
			if err != nil {
				t.Errorf("Concurrent ReadPIDFile() failed: %v", err)
			}
			if pid != os.Getpid() {
				t.Errorf("Concurrent ReadPIDFile() got wrong PID: %d", pid)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent reads")
		}
	}
}

func TestRemovePIDFile_CleansUpLockFile(t *testing.T) {
	skipIfWindows(t)
	logDir := t.TempDir()

	// Write PID file (which creates lock file)
	if err := WritePIDFile(logDir); err != nil {
		t.Fatalf("WritePIDFile() failed: %v", err)
	}

	// Verify both files exist
	pidPath := filepath.Join(logDir, "grepai-watch.pid")
	lockPath := pidPath + ".lock"

	if _, err := os.Stat(pidPath); os.IsNotExist(err) {
		t.Fatal("PID file was not created")
	}
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatal("Lock file was not created")
	}

	// Remove PID file
	if err := RemovePIDFile(logDir); err != nil {
		t.Fatalf("RemovePIDFile() failed: %v", err)
	}

	// Verify both files are gone
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("PID file still exists after removal")
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("Lock file still exists after removal")
	}
}

// Helper functions

func skipIfWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: cannot delete locked files")
	}
}

func contains(s, substr string) bool {
	return strings.Contains(filepath.ToSlash(s), substr)
}
