package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/yoanbernabeu/grepai/daemon"
)

func removeProjectReadyMarker(logDir, worktreeID string) error {
	if worktreeID != "" {
		return daemon.RemoveWorktreeReadyFile(logDir, worktreeID)
	}
	return daemon.RemoveReadyFile(logDir)
}

// removeProjectDaemonMarkers removes the PID and ready markers left by the
// daemon ownerPID. Markers written by another daemon are kept: when two starts
// race, the losing child exits on the PID lock while the winner keeps running,
// and deleting the winner's markers would hide it from --status and --stop.
func removeProjectDaemonMarkers(logDir, worktreeID string, ownerPID int) error {
	if worktreeID != "" {
		return removeDaemonMarkersOwnedBy(ownerPID,
			func() (int, error) { return daemon.ReadWorktreePIDFile(logDir, worktreeID) },
			func() error { return daemon.RemoveWorktreePIDFile(logDir, worktreeID) },
			func() bool { return daemon.IsWorktreeStartedForPID(logDir, worktreeID, ownerPID) },
			func() error { return daemon.RemoveWorktreeReadyFile(logDir, worktreeID) },
		)
	}
	return removeDaemonMarkersOwnedBy(ownerPID,
		func() (int, error) { return daemon.ReadPIDFile(logDir) },
		func() error { return daemon.RemovePIDFile(logDir) },
		func() bool { return daemon.IsStartedForPID(logDir, ownerPID) },
		func() error { return daemon.RemoveReadyFile(logDir) },
	)
}

// removeWorkspaceDaemonMarkers is removeProjectDaemonMarkers for a workspace.
func removeWorkspaceDaemonMarkers(logDir, workspaceName string, ownerPID int) error {
	return removeDaemonMarkersOwnedBy(ownerPID,
		func() (int, error) { return daemon.ReadWorkspacePIDFile(logDir, workspaceName) },
		func() error { return daemon.RemoveWorkspacePIDFile(logDir, workspaceName) },
		func() bool { return daemon.IsWorkspaceStartedForPID(logDir, workspaceName, ownerPID) },
		func() error { return daemon.RemoveWorkspaceReadyFile(logDir, workspaceName) },
	)
}

func removeDaemonMarkersOwnedBy(ownerPID int, readPID func() (int, error), removePID func() error, readyOwned func() bool, removeReady func() error) error {
	// A PID file naming ownerPID is removed with the ready marker as before.
	// Otherwise (another PID, no PID file, or one we cannot read) another
	// daemon may own the slot, so only a ready marker written by ownerPID is
	// removed.
	if pid, err := readPID(); err == nil && pid == ownerPID {
		return errors.Join(removeReady(), removePID())
	}
	if readyOwned() {
		return removeReady()
	}
	return nil
}

var errNotReady = errors.New("not ready")

// stillIndexingAfterTimeout reports whether a background child that missed the
// readiness deadline is alive and past its startup checks, i.e. still building
// its initial index. Such a child is kept running instead of being stopped.
func stillIndexingAfterTimeout(exitCh <-chan struct{}, childPID int, isStarted func(int) bool) bool {
	select {
	case <-exitCh:
		return false
	default:
	}
	return isStarted(childPID)
}

var backgroundExitTimeout = 5 * time.Second

func waitForBackgroundReady(exitCh <-chan struct{}, expectedPID int, isReady func(int) bool, timeout, pollInterval time.Duration) error {
	check := func() error {
		select {
		case <-exitCh:
			return errors.New("background process exited before becoming ready")
		default:
		}
		if isReady(expectedPID) {
			return nil
		}
		return errNotReady
	}
	if err := check(); !errors.Is(err, errNotReady) {
		return err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-exitCh:
			return errors.New("background process exited before becoming ready")
		case <-timer.C:
			return fmt.Errorf("timeout waiting for background process readiness after %v", timeout)
		case <-ticker.C:
			if err := check(); !errors.Is(err, errNotReady) {
				return err
			}
		}
	}
}

func waitForBackgroundExit(exitCh <-chan struct{}, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-exitCh:
		return nil
	case <-timer.C:
		return fmt.Errorf("background process did not exit within %v", timeout)
	}
}

func abortBackgroundStart(primary error, childPID int, exitCh <-chan struct{}, cleanup func() error) error {
	stopErr := watchStopProcess(childPID)
	exitErr := waitForBackgroundExit(exitCh, backgroundExitTimeout)
	var cleanupErr error
	if stopErr == nil && exitErr == nil {
		cleanupErr = cleanup()
	}
	if stopErr != nil {
		stopErr = fmt.Errorf("stop failed background process %d: %w", childPID, stopErr)
	}
	if cleanupErr != nil {
		cleanupErr = fmt.Errorf("remove failed background markers: %w", cleanupErr)
	}
	return errors.Join(primary, stopErr, exitErr, cleanupErr)
}
