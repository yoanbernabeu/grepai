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
//	pid, exitCh, err := daemon.SpawnBackground(logDir, []string{"watch"})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Started with PID %d\n", pid)
//	// exitCh receives when child exits (detects early failures)
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
	pidFileName         = "grepai-watch.pid"
	logFileName         = "grepai-watch.log"
	readyFileName       = "grepai-watch.ready"
	worktreePIDPrefix   = "grepai-worktree-"
	worktreePIDSuffix   = ".pid"
	worktreeLogPrefix   = "grepai-worktree-"
	worktreeLogSuffix   = ".log"
	worktreeReadyPrefix = "grepai-worktree-"
	worktreeReadySuffix = ".ready"
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
// Args should be the command-line arguments to pass to the child process
// (e.g., []string{"watch"} for "grepai watch").
//
// Returns the child PID and an exit channel. The exit channel receives when
// the child process terminates, enabling callers to detect early failures
// without relying on kill(0) which cannot distinguish zombie processes.
func SpawnBackground(logDir string, args []string) (int, <-chan struct{}, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return 0, nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	logPath := filepath.Join(logDir, logFileName)
	return spawnBackgroundWithLog(logDir, logPath, args)
}

// StopProcess sends a stop signal to the process with the given PID.
//
// On Unix, this sends SIGINT to request graceful shutdown.
// On Windows, this writes a sentinel stop file that the daemon polls for.
//
// This function returns immediately after sending the signal. It does NOT wait
// for the process to exit. Callers should poll IsProcessRunning() to verify
// the process has stopped.
//
// Returns an error if the PID is invalid (<= 0) or if the signal cannot be sent
// (process doesn't exist, insufficient permissions, etc.).
//
// Platform-specific implementations are in daemon_unix.go and daemon_windows.go.

// StopChannel returns a channel that is closed when a stop signal is detected.
//
// On Unix, this returns a channel that never fires (signals are handled via
// os/signal). On Windows, this polls for a sentinel stop file written by
// StopProcess and closes the channel when detected.
//
// Callers should select on the returned channel alongside other shutdown
// mechanisms (e.g., os/signal) to support graceful shutdown on all platforms.
//
// Platform-specific implementations are in daemon_unix.go and daemon_windows.go.

// GetWorktreePIDFile returns the path to the PID file for a worktree.
func GetWorktreePIDFile(logDir, worktreeID string) string {
	return filepath.Join(logDir, worktreePIDPrefix+worktreeID+worktreePIDSuffix)
}

// GetWorktreeLogFile returns the path to the log file for a worktree.
func GetWorktreeLogFile(logDir, worktreeID string) string {
	return filepath.Join(logDir, worktreeLogPrefix+worktreeID+worktreeLogSuffix)
}

// GetWorktreeReadyFile returns the path to the ready file for a worktree.
func GetWorktreeReadyFile(logDir, worktreeID string) string {
	return filepath.Join(logDir, worktreeReadyPrefix+worktreeID+worktreeReadySuffix)
}

// WriteWorktreePIDFile writes the current process ID to the worktree PID file.
func WriteWorktreePIDFile(logDir, worktreeID string) error {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	pidPath := GetWorktreePIDFile(logDir, worktreeID)
	lockPath := pidPath + ".lock"

	// Create/open lock file
	lockFh, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to create lock file: %w", err)
	}

	// Try to acquire exclusive lock (non-blocking)
	if err := lockFile(lockFh); err != nil {
		lockFh.Close()
		return fmt.Errorf("another grepai worktree watch process is starting (lock held)")
	}

	// BUG FIX: Close lock file after PID write completes to prevent handle leak.
	// The lock is only needed to serialize PID file writes; the PID file itself
	// provides the liveness signal.
	defer lockFh.Close()

	// Write PID atomically using temp file + rename
	pid := os.Getpid()
	content := fmt.Sprintf("%d\n", pid)
	tmpPath := pidPath + ".tmp"

	if err := os.WriteFile(tmpPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	if err := os.Rename(tmpPath, pidPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename PID file: %w", err)
	}

	return nil
}

// ReadWorktreePIDFile reads the process ID from the worktree PID file.
func ReadWorktreePIDFile(logDir, worktreeID string) (int, error) {
	pidPath := GetWorktreePIDFile(logDir, worktreeID)

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

// RemoveWorktreePIDFile removes the worktree PID file and its lock file.
func RemoveWorktreePIDFile(logDir, worktreeID string) error {
	pidPath := GetWorktreePIDFile(logDir, worktreeID)
	lockPath := pidPath + ".lock"

	_ = os.Remove(lockPath)

	if err := os.Remove(pidPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove PID file: %w", err)
	}
	return nil
}

// GetRunningWorktreePID returns the PID of the running worktree watcher, or 0 if not running.
func GetRunningWorktreePID(logDir, worktreeID string) (int, error) {
	pid, err := ReadWorktreePIDFile(logDir, worktreeID)
	if err != nil {
		return 0, err
	}

	if pid == 0 {
		return 0, nil
	}

	// Check if process is actually running
	if !IsProcessRunning(pid) {
		_ = RemoveWorktreePIDFile(logDir, worktreeID)
		return 0, nil
	}

	return pid, nil
}

// WriteWorktreeReadyFile writes the ready marker file for a worktree.
func WriteWorktreeReadyFile(logDir, worktreeID string) error {
	readyPath := GetWorktreeReadyFile(logDir, worktreeID)
	content := fmt.Sprintf("ready\n%d\n", os.Getpid())
	if err := os.WriteFile(readyPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write ready file: %w", err)
	}
	return nil
}

// RemoveWorktreeReadyFile removes the ready marker file for a worktree.
func RemoveWorktreeReadyFile(logDir, worktreeID string) error {
	readyPath := GetWorktreeReadyFile(logDir, worktreeID)
	if err := os.Remove(readyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove ready file: %w", err)
	}
	return nil
}

// IsWorktreeReady checks if the worktree ready marker file exists.
func IsWorktreeReady(logDir, worktreeID string) bool {
	readyPath := GetWorktreeReadyFile(logDir, worktreeID)
	_, err := os.Stat(readyPath)
	return err == nil
}

// SpawnWorktreeBackground re-executes the current binary for worktree watch in background.
func SpawnWorktreeBackground(logDir, worktreeID string, extraArgs []string) (int, <-chan struct{}, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return 0, nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Use worktree-specific log file
	logPath := GetWorktreeLogFile(logDir, worktreeID)

	return spawnBackgroundWithLog(logDir, logPath, extraArgs)
}

// spawnBackgroundWithLog spawns a background process with a custom log file.
// Returns the child PID and a channel that is closed when the child exits.
// Uses platform-specific liveness detection (pipe on Unix, polling on Windows).
func spawnBackgroundWithLog(logDir, logPath string, args []string) (int, <-chan struct{}, error) {
	executable, err := os.Executable()
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get executable path: %w", err)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to open log file: %w", err)
	}

	liveness, err := newLivenessCheck()
	if err != nil {
		logFile.Close()
		return 0, nil, err
	}

	cmd := exec.Command(executable, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.Env = append(os.Environ(), "GREPAI_BACKGROUND=1")
	cmd.SysProcAttr = sysProcAttr()
	liveness.configureCmd(cmd)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		liveness.cleanup()
		return 0, nil, fmt.Errorf("failed to start background process: %w", err)
	}

	logFile.Close()
	exitCh := liveness.start(cmd.Process.Pid)

	return cmd.Process.Pid, exitCh, nil
}
