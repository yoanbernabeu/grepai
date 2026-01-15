// Package daemon provides lifecycle management for the grepai watch daemon.
//
// This package handles PID file management, process spawning, and process
// lifecycle operations for running grepai watch in background mode.
//
// # Basic Usage
//
// Start a background process:
//
//	logDir, _ := daemon.GetDefaultLogDir()
//	pid, err := daemon.SpawnBackground(logDir, []string{"watch"})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Started with PID %d\n", pid)
//
// Check if the process is running:
//
//	pid, err := daemon.GetRunningPID(logDir)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if pid > 0 {
//	    fmt.Printf("Watcher is running (PID %d)\n", pid)
//	}
//
// Stop the process:
//
//	daemon.StopProcess(pid)
//	daemon.RemovePIDFile(logDir)
//
// # PID File Format
//
// The PID file contains a single line with the process ID as a decimal integer.
// This format is stable and will not change in future versions. If additional
// metadata is needed, it will be stored in separate files (e.g., grepai-watch.meta).
//
// # Platform Support
//
// Cross-platform support for Unix-like systems (Linux, macOS) and Windows.
// Platform-specific behavior is implemented in daemon_unix.go and daemon_windows.go.
//
// # Thread Safety
//
// PID file writes use file locking (flock) to prevent race conditions when
// multiple processes attempt to start simultaneously.
package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	pidFileName   = "grepai-watch.pid"
	logFileName   = "grepai-watch.log"
	readyFileName = "grepai-watch.ready"
)

// GetDefaultLogDir returns the OS-specific default log directory.
//
// Platform-specific defaults:
//   - Linux:   $XDG_STATE_HOME/grepai/logs or ~/.local/state/grepai/logs
//   - macOS:   ~/Library/Logs/grepai
//   - Windows: %LOCALAPPDATA%\grepai\logs
//
// This function is typically called once at startup to determine where PID files
// and logs should be stored. Use the --log-dir flag to override the default.
//
// Returns an absolute path to the log directory. The directory may not exist yet;
// callers should create it with os.MkdirAll if needed.
func GetDefaultLogDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir, "Library", "Logs", "grepai"), nil
	case "windows":
		if base := os.Getenv("LOCALAPPDATA"); base != "" {
			return filepath.Join(base, "grepai", "logs"), nil
		}
		return filepath.Join(homeDir, "AppData", "Local", "grepai", "logs"), nil
	default: // Linux and other Unix-like systems
		if base := os.Getenv("XDG_STATE_HOME"); base != "" {
			return filepath.Join(base, "grepai", "logs"), nil
		}
		return filepath.Join(homeDir, ".local", "state", "grepai", "logs"), nil
	}
}

// WritePIDFile writes the current process ID to the PID file.
// Uses file locking to prevent race conditions when multiple processes
// attempt to start simultaneously. The lock is held for the lifetime of
// the process (released by the OS on exit).
//
// PID file format: single line containing process ID as decimal integer.
// This format is stable and will not change. If additional metadata is needed
// in the future, it will be stored in a separate file (e.g., grepai-watch.meta).
func WritePIDFile(logDir string) error {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	pidPath := filepath.Join(logDir, pidFileName)
	lockPath := pidPath + ".lock"

	// Create/open lock file
	lockFh, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to create lock file: %w", err)
	}

	// Try to acquire exclusive lock (non-blocking)
	// This prevents multiple processes from starting simultaneously
	if err := lockFile(lockFh); err != nil {
		lockFh.Close()
		return fmt.Errorf("another grepai watch process is starting (lock held)")
	}

	// Write PID atomically using temp file + rename
	pid := os.Getpid()
	content := fmt.Sprintf("%d\n", pid)
	tmpPath := pidPath + ".tmp"

	if err := os.WriteFile(tmpPath, []byte(content), 0600); err != nil {
		lockFh.Close()
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	if err := os.Rename(tmpPath, pidPath); err != nil {
		os.Remove(tmpPath)
		lockFh.Close()
		return fmt.Errorf("failed to rename PID file: %w", err)
	}

	// Keep lock file open and locked for the lifetime of this process.
	// The OS will automatically release the lock when the process exits.
	// We intentionally don't close lockFh here.

	return nil
}

// ReadPIDFile reads the process ID from the PID file in the given logDir.
//
// Return values:
//   - (0, nil):     No PID file exists (watcher not running or not started yet)
//   - (pid, nil):   PID file exists and contains a valid process ID
//   - (0, error):   PID file exists but is corrupt, unreadable, or has wrong permissions
//
// Note: This function does NOT check if the process is actually running. Use
// GetRunningPID() for automatic stale PID detection and cleanup, or call
// IsProcessRunning(pid) to check the process status manually.
func ReadPIDFile(logDir string) (int, error) {
	pidPath := filepath.Join(logDir, pidFileName)

	data, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read PID file: %w", err)
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID in file: %w", err)
	}

	return pid, nil
}

// RemovePIDFile removes the PID file and its associated lock file.
func RemovePIDFile(logDir string) error {
	pidPath := filepath.Join(logDir, pidFileName)
	lockPath := pidPath + ".lock"

	// Remove lock file first (best effort, ignore errors)
	_ = os.Remove(lockPath)

	// Remove PID file
	if err := os.Remove(pidPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove PID file: %w", err)
	}
	return nil
}

// GetRunningPID returns the PID of the running watcher process, or 0 if not running.
// Automatically cleans up stale PID files (where the process no longer exists).
// This is a convenience function that combines ReadPIDFile, IsProcessRunning, and
// stale PID cleanup in one call.
func GetRunningPID(logDir string) (int, error) {
	pid, err := ReadPIDFile(logDir)
	if err != nil {
		return 0, err
	}

	if pid == 0 {
		return 0, nil
	}

	// Check if process is actually running
	if !IsProcessRunning(pid) {
		// Stale PID file - clean it up (best effort, ignore errors)
		_ = RemovePIDFile(logDir)
		return 0, nil
	}

	return pid, nil
}

// WriteReadyFile writes the ready marker file to indicate the daemon has
// successfully initialized and is ready to serve. This should be called
// after all initialization is complete (embedder, store, initial scan).
func WriteReadyFile(logDir string) error {
	readyPath := filepath.Join(logDir, readyFileName)
	content := fmt.Sprintf("ready\n%d\n", os.Getpid())
	if err := os.WriteFile(readyPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write ready file: %w", err)
	}
	return nil
}

// RemoveReadyFile removes the ready marker file.
func RemoveReadyFile(logDir string) error {
	readyPath := filepath.Join(logDir, readyFileName)
	if err := os.Remove(readyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove ready file: %w", err)
	}
	return nil
}

// IsReady checks if the ready marker file exists.
func IsReady(logDir string) bool {
	readyPath := filepath.Join(logDir, readyFileName)
	_, err := os.Stat(readyPath)
	return err == nil
}

// IsProcessRunning checks if a process with the given PID is running.
// Platform-specific implementations are in daemon_unix.go and daemon_windows.go.

// SpawnBackground re-executes the current binary as a background process.
//
// The function spawns a detached child process with:
//   - stdout/stderr redirected to logDir/grepai-watch.log
//   - stdin set to nil (no input)
//   - GREPAI_BACKGROUND=1 environment variable set
//   - process group detachment (Unix only)
//
// The parent process does NOT wait for the child - the child runs independently
// and will be reaped by the OS when it exits.
//
// Args should be the command-line arguments to pass to the child process
// (e.g., []string{"watch"} for "grepai watch").
//
// Returns the child process PID on success. The caller should verify the child
// started successfully by polling IsReady() or checking IsProcessRunning().
func SpawnBackground(logDir string, args []string) (int, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Get the current executable path
	executable, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("failed to get executable path: %w", err)
	}

	// Open log file
	logPath := filepath.Join(logDir, logFileName)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 0, fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	// Prepare command
	cmd := exec.Command(executable, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil

	// Set environment variable to indicate background mode
	cmd.Env = append(os.Environ(), "GREPAI_BACKGROUND=1")

	// Platform-specific process attributes (e.g., detach from parent process group on Unix)
	cmd.SysProcAttr = sysProcAttr()

	// Start the process
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start background process: %w", err)
	}

	// Don't wait for the process - we're detaching completely.
	// The OS will reap the child when it exits.

	return cmd.Process.Pid, nil
}

// StopProcess sends an interrupt signal to the process with the given PID.
//
// This sends SIGINT (Unix) or os.Interrupt (Windows) to request graceful shutdown.
// The target process should have signal handlers installed to catch the interrupt
// and clean up (persist state, close connections, etc.) before exiting.
//
// This function returns immediately after sending the signal. It does NOT wait
// for the process to exit. Callers should poll IsProcessRunning() to verify
// the process has stopped.
//
// Returns an error if the PID is invalid (<= 0) or if the signal cannot be sent
// (process doesn't exist, insufficient permissions, etc.).
func StopProcess(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid PID: %d", pid)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	// Send interrupt signal (SIGINT on Unix, CTRL_BREAK on Windows)
	if err := process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("failed to send interrupt signal: %w", err)
	}

	return nil
}
