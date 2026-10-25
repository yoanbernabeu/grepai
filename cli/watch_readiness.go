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

func removeProjectDaemonMarkers(logDir, worktreeID string) error {
	if worktreeID != "" {
		return errors.Join(
			daemon.RemoveWorktreeReadyFile(logDir, worktreeID),
			daemon.RemoveWorktreePIDFile(logDir, worktreeID),
		)
	}
	return errors.Join(daemon.RemoveReadyFile(logDir), daemon.RemovePIDFile(logDir))
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
