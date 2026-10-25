package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The ready marker file holds "<state> <pid>". A background watcher writes
// "started" once its startup checks pass (config, embedder, store, locks) and
// "ready" once the initial index is done. Only "ready" counts as ready.
const (
	markerStarted = "started"
	markerReady   = "ready"
)

func readMarker(path string) (state string, pid int, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, false
	}
	fields := strings.Fields(string(data))
	if len(fields) != 2 {
		return "", 0, false
	}
	pid, err = strconv.Atoi(fields[1])
	if err != nil {
		return "", 0, false
	}
	return fields[0], pid, true
}

func readyFileMatchesPID(path string, expectedPID int) bool {
	state, pid, ok := readMarker(path)
	return ok && state == markerReady && pid == expectedPID
}

func startedFileMatchesPID(path string, expectedPID int) bool {
	state, pid, ok := readMarker(path)
	return ok && (state == markerStarted || state == markerReady) && pid == expectedPID
}

func writeStartedMarker(path string) error {
	content := fmt.Sprintf("%s\n%d\n", markerStarted, os.Getpid())
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write started marker: %w", err)
	}
	return nil
}

// WriteStartedFile marks the project watcher as started (initial index in progress).
func WriteStartedFile(logDir string) error {
	return writeStartedMarker(filepath.Join(logDir, readyFileName))
}

// WriteWorktreeStartedFile marks the worktree watcher as started.
func WriteWorktreeStartedFile(logDir, worktreeID string) error {
	return writeStartedMarker(GetWorktreeReadyFile(logDir, worktreeID))
}

// WriteWorkspaceStartedFile marks the workspace watcher as started.
func WriteWorkspaceStartedFile(logDir, workspaceName string) error {
	return writeStartedMarker(GetWorkspaceReadyFile(logDir, workspaceName))
}

// IsStartedForPID reports whether the project watcher expectedPID passed its
// startup checks (started or ready).
func IsStartedForPID(logDir string, expectedPID int) bool {
	return startedFileMatchesPID(filepath.Join(logDir, readyFileName), expectedPID)
}

// IsWorktreeStartedForPID reports whether the worktree watcher expectedPID started.
func IsWorktreeStartedForPID(logDir, worktreeID string, expectedPID int) bool {
	return startedFileMatchesPID(GetWorktreeReadyFile(logDir, worktreeID), expectedPID)
}

// IsWorkspaceStartedForPID reports whether the workspace watcher expectedPID started.
func IsWorkspaceStartedForPID(logDir, workspaceName string, expectedPID int) bool {
	return startedFileMatchesPID(GetWorkspaceReadyFile(logDir, workspaceName), expectedPID)
}

// IsReadyForPID reports whether the project marker belongs to expectedPID.
func IsReadyForPID(logDir string, expectedPID int) bool {
	return readyFileMatchesPID(filepath.Join(logDir, readyFileName), expectedPID)
}

// IsWorktreeReadyForPID reports whether the worktree marker belongs to expectedPID.
func IsWorktreeReadyForPID(logDir, worktreeID string, expectedPID int) bool {
	return readyFileMatchesPID(GetWorktreeReadyFile(logDir, worktreeID), expectedPID)
}

// IsWorkspaceReadyForPID reports whether the workspace marker belongs to expectedPID.
func IsWorkspaceReadyForPID(logDir, workspaceName string, expectedPID int) bool {
	return readyFileMatchesPID(GetWorkspaceReadyFile(logDir, workspaceName), expectedPID)
}
