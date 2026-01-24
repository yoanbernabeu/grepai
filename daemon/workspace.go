// Package daemon provides workspace daemon management for multi-project indexing.
package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	workspacePIDPrefix   = "grepai-workspace-"
	workspacePIDSuffix   = ".pid"
	workspaceLogPrefix   = "grepai-workspace-"
	workspaceLogSuffix   = ".log"
	workspaceReadyPrefix = "grepai-workspace-"
	workspaceReadySuffix = ".ready"
)

// GetWorkspacePIDFile returns the path to the PID file for a workspace.
func GetWorkspacePIDFile(logDir, workspaceName string) string {
	return filepath.Join(logDir, workspacePIDPrefix+workspaceName+workspacePIDSuffix)
}

// GetWorkspaceLogFile returns the path to the log file for a workspace.
func GetWorkspaceLogFile(logDir, workspaceName string) string {
	return filepath.Join(logDir, workspaceLogPrefix+workspaceName+workspaceLogSuffix)
}

// GetWorkspaceReadyFile returns the path to the ready file for a workspace.
func GetWorkspaceReadyFile(logDir, workspaceName string) string {
	return filepath.Join(logDir, workspaceReadyPrefix+workspaceName+workspaceReadySuffix)
}

// WriteWorkspacePIDFile writes the current process ID to the workspace PID file.
func WriteWorkspacePIDFile(logDir, workspaceName string) error {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	pidPath := GetWorkspacePIDFile(logDir, workspaceName)
	lockPath := pidPath + ".lock"

	// Create/open lock file
	lockFh, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to create lock file: %w", err)
	}

	// Try to acquire exclusive lock (non-blocking)
	if err := lockFile(lockFh); err != nil {
		lockFh.Close()
		return fmt.Errorf("another grepai workspace watch process is starting (lock held)")
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

	// Keep lock file open and locked for the lifetime of this process
	return nil
}

// ReadWorkspacePIDFile reads the process ID from the workspace PID file.
func ReadWorkspacePIDFile(logDir, workspaceName string) (int, error) {
	pidPath := GetWorkspacePIDFile(logDir, workspaceName)

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

// RemoveWorkspacePIDFile removes the workspace PID file and its lock file.
func RemoveWorkspacePIDFile(logDir, workspaceName string) error {
	pidPath := GetWorkspacePIDFile(logDir, workspaceName)
	lockPath := pidPath + ".lock"

	_ = os.Remove(lockPath)

	if err := os.Remove(pidPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove PID file: %w", err)
	}
	return nil
}

// GetRunningWorkspacePID returns the PID of the running workspace watcher, or 0 if not running.
func GetRunningWorkspacePID(logDir, workspaceName string) (int, error) {
	pid, err := ReadWorkspacePIDFile(logDir, workspaceName)
	if err != nil {
		return 0, err
	}

	if pid == 0 {
		return 0, nil
	}

	// Check if process is actually running
	if !IsProcessRunning(pid) {
		_ = RemoveWorkspacePIDFile(logDir, workspaceName)
		return 0, nil
	}

	return pid, nil
}

// WriteWorkspaceReadyFile writes the ready marker file for a workspace.
func WriteWorkspaceReadyFile(logDir, workspaceName string) error {
	readyPath := GetWorkspaceReadyFile(logDir, workspaceName)
	content := fmt.Sprintf("ready\n%d\n", os.Getpid())
	if err := os.WriteFile(readyPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write ready file: %w", err)
	}
	return nil
}

// RemoveWorkspaceReadyFile removes the ready marker file for a workspace.
func RemoveWorkspaceReadyFile(logDir, workspaceName string) error {
	readyPath := GetWorkspaceReadyFile(logDir, workspaceName)
	if err := os.Remove(readyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove ready file: %w", err)
	}
	return nil
}

// IsWorkspaceReady checks if the workspace ready marker file exists.
func IsWorkspaceReady(logDir, workspaceName string) bool {
	readyPath := GetWorkspaceReadyFile(logDir, workspaceName)
	_, err := os.Stat(readyPath)
	return err == nil
}

// SpawnWorkspaceBackground re-executes the current binary for workspace watch in background.
func SpawnWorkspaceBackground(logDir, workspaceName string, extraArgs []string) (int, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Build args for background process
	args := []string{"watch", "--workspace", workspaceName}
	args = append(args, extraArgs...)

	// Use workspace-specific log file
	logPath := GetWorkspaceLogFile(logDir, workspaceName)

	return spawnBackgroundWithLog(logDir, logPath, args)
}

// spawnBackgroundWithLog spawns a background process with a custom log file.
func spawnBackgroundWithLog(logDir, logPath string, args []string) (int, error) {
	executable, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("failed to get executable path: %w", err)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 0, fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(executable, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.Env = append(os.Environ(), "GREPAI_BACKGROUND=1")
	cmd.SysProcAttr = sysProcAttr()

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start background process: %w", err)
	}

	return cmd.Process.Pid, nil
}
