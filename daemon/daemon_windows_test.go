//go:build windows
// +build windows

package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStopProcessWritesStopFile(t *testing.T) {
	// Override GetDefaultLogDir by using the actual function and verifying the file lands there.
	pid := os.Getpid()

	path, err := stopFilePath(pid)
	if err != nil {
		t.Fatalf("stopFilePath() error: %v", err)
	}

	// Clean up before and after test.
	_ = os.Remove(path)
	defer os.Remove(path)

	// StopProcess checks IsProcessRunning, so use our own PID.
	if err := StopProcess(pid); err != nil {
		t.Fatalf("StopProcess() error: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("stop file was not created at %s", path)
	}
}

func TestStopChannelDetectsStopFile(t *testing.T) {
	pid := os.Getpid()

	path, err := stopFilePath(pid)
	if err != nil {
		t.Fatalf("stopFilePath() error: %v", err)
	}

	// Clean up any stale file before starting the channel.
	_ = os.Remove(path)

	ch := StopChannel()

	// Verify channel has not fired yet.
	select {
	case <-ch:
		t.Fatal("StopChannel fired before stop file was written")
	case <-time.After(100 * time.Millisecond):
		// expected
	}

	// Write the stop file.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d\n", pid)), 0600); err != nil {
		t.Fatalf("failed to write stop file: %v", err)
	}

	// Channel should fire within a reasonable time.
	select {
	case <-ch:
		// success
	case <-time.After(3 * time.Second):
		t.Fatal("StopChannel did not fire after stop file was written")
	}

	// Stop file should be cleaned up.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("stop file was not removed after detection")
	}
}

func TestStopChannelCleansStaleFile(t *testing.T) {
	pid := os.Getpid()

	path, err := stopFilePath(pid)
	if err != nil {
		t.Fatalf("stopFilePath() error: %v", err)
	}

	// Pre-create a stale stop file (simulating a leftover from a previous run
	// that happened to reuse the same PID).
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("stale\n"), 0600); err != nil {
		t.Fatalf("failed to write stale stop file: %v", err)
	}

	// StopChannel should remove the stale file on init.
	ch := StopChannel()

	// Give the goroutine a moment to start and clean up.
	time.Sleep(100 * time.Millisecond)

	// Stale file should be gone.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("stale stop file was not cleaned up on init")
	}

	// Channel should NOT have fired (the stale file was removed before polling started).
	select {
	case <-ch:
		t.Fatal("StopChannel should not fire after cleaning stale file")
	case <-time.After(stopPollInterval + 200*time.Millisecond):
		// expected — channel remains open
	}
}
