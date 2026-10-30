package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/daemon"
)

func TestAbortBackgroundStartStopFailurePreservesMarkers(t *testing.T) {
	stopFailure := errors.New("stop denied")
	primary := errors.New("startup failed")
	exited := make(chan struct{})
	close(exited)
	cleanupCalled := false

	originalStop := watchStopProcess
	t.Cleanup(func() { watchStopProcess = originalStop })
	watchStopProcess = func(int) error { return stopFailure }

	err := abortBackgroundStart(primary, 42, exited, func() error {
		cleanupCalled = true
		return nil
	})
	if cleanupCalled {
		t.Fatal("marker cleanup ran after stop failure")
	}
	if !errors.Is(err, primary) || !errors.Is(err, stopFailure) {
		t.Fatalf("abortBackgroundStart() error = %v, want joined primary and stop errors", err)
	}
}

func TestAbortBackgroundStartExitTimeoutPreservesMarkers(t *testing.T) {
	primary := errors.New("startup failed")
	cleanupCalled := false
	originalStop := watchStopProcess
	originalTimeout := backgroundExitTimeout
	t.Cleanup(func() {
		watchStopProcess = originalStop
		backgroundExitTimeout = originalTimeout
	})
	watchStopProcess = func(int) error { return nil }
	backgroundExitTimeout = 10 * time.Millisecond

	err := abortBackgroundStart(primary, 42, make(chan struct{}), func() error {
		cleanupCalled = true
		return nil
	})
	if cleanupCalled {
		t.Fatal("marker cleanup ran before child exit")
	}
	if !errors.Is(err, primary) || !strings.Contains(err.Error(), "did not exit") {
		t.Fatalf("abortBackgroundStart() error = %v, want joined primary and timeout errors", err)
	}
}

func TestAbortBackgroundStartConfirmedExitRemovesMarkers(t *testing.T) {
	primary := errors.New("startup failed")
	exited := make(chan struct{})
	cleanupFailure := errors.New("cleanup failed")
	cleanupCalled := false
	originalStop := watchStopProcess
	t.Cleanup(func() { watchStopProcess = originalStop })
	watchStopProcess = func(int) error {
		close(exited)
		return nil
	}

	err := abortBackgroundStart(primary, 42, exited, func() error {
		cleanupCalled = true
		return cleanupFailure
	})
	if !cleanupCalled {
		t.Fatal("marker cleanup did not run after confirmed child exit")
	}
	if !errors.Is(err, primary) || !errors.Is(err, cleanupFailure) {
		t.Fatalf("abortBackgroundStart() error = %v, want joined primary and cleanup errors", err)
	}
}

// Regression test for #357: when two background starts race, the losing child
// exits because the winner holds the PID lock. Aborting the losing start must
// not delete the winner's PID and ready markers, or the winner becomes
// invisible to --status/--stop and a later start spawns a second daemon.
func TestStartBackgroundWatchAbortKeepsOtherDaemonMarkers(t *testing.T) {
	for _, worktreeID := range []string{"", "wt-race"} {
		name := "main"
		if worktreeID != "" {
			name = "worktree"
		}
		t.Run(name, func(t *testing.T) {
			logDir := t.TempDir()
			winnerPID := os.Getpid()
			const loserPID = 999999

			pidPath := filepath.Join(logDir, "grepai-watch.pid")
			readyPath := filepath.Join(logDir, "grepai-watch.ready")
			if worktreeID != "" {
				pidPath = daemon.GetWorktreePIDFile(logDir, worktreeID)
				readyPath = daemon.GetWorktreeReadyFile(logDir, worktreeID)
			}

			// The winner registers itself while our child is starting; our
			// child then exits because it cannot take the lock.
			spawn := func(string) (int, <-chan struct{}, error) {
				if err := os.WriteFile(pidPath, []byte(strconv.Itoa(winnerPID)+"\n"), 0600); err != nil {
					t.Fatalf("failed to write winner PID file: %v", err)
				}
				if err := os.WriteFile(readyPath, []byte("ready\n"+strconv.Itoa(winnerPID)+"\n"), 0600); err != nil {
					t.Fatalf("failed to write winner ready file: %v", err)
				}
				exited := make(chan struct{})
				close(exited)
				return loserPID, exited, nil
			}

			originalSpawn := watchSpawnBackground
			originalSpawnWorktree := watchSpawnWorktree
			originalStop := watchStopProcess
			t.Cleanup(func() {
				watchSpawnBackground = originalSpawn
				watchSpawnWorktree = originalSpawnWorktree
				watchStopProcess = originalStop
			})
			watchSpawnBackground = func(dir string, _ []string) (int, <-chan struct{}, error) { return spawn(dir) }
			watchSpawnWorktree = func(dir, _ string, _ []string) (int, <-chan struct{}, error) { return spawn(dir) }
			watchStopProcess = func(int) error { return nil }

			if err := startBackgroundWatch(logDir, worktreeID); err == nil {
				t.Fatal("startBackgroundWatch() succeeded although the child exited")
			}

			data, err := os.ReadFile(pidPath)
			if err != nil {
				t.Fatalf("winner PID file was removed by the aborted start: %v", err)
			}
			if got := strings.TrimSpace(string(data)); got != strconv.Itoa(winnerPID) {
				t.Fatalf("PID file = %q, want winner PID %d", got, winnerPID)
			}
			if _, err := os.Stat(readyPath); err != nil {
				t.Fatalf("winner ready file was removed by the aborted start: %v", err)
			}
		})
	}
}

// The aborted start still removes the markers its own child left behind.
func TestStartBackgroundWatchAbortRemovesOwnMarkers(t *testing.T) {
	logDir := t.TempDir()
	worktreeID := "wt-own"
	const childPID = 999998
	pidPath := daemon.GetWorktreePIDFile(logDir, worktreeID)
	readyPath := daemon.GetWorktreeReadyFile(logDir, worktreeID)

	originalSpawnWorktree := watchSpawnWorktree
	originalStop := watchStopProcess
	t.Cleanup(func() {
		watchSpawnWorktree = originalSpawnWorktree
		watchStopProcess = originalStop
	})
	watchSpawnWorktree = func(string, string, []string) (int, <-chan struct{}, error) {
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(childPID)+"\n"), 0600); err != nil {
			t.Fatalf("failed to write child PID file: %v", err)
		}
		if err := os.WriteFile(readyPath, []byte("started\n"+strconv.Itoa(childPID)+"\n"), 0600); err != nil {
			t.Fatalf("failed to write child started file: %v", err)
		}
		exited := make(chan struct{})
		close(exited)
		return childPID, exited, nil
	}
	watchStopProcess = func(int) error { return nil }

	if err := startBackgroundWatch(logDir, worktreeID); err == nil {
		t.Fatal("startBackgroundWatch() succeeded although the child exited")
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("child PID file still exists after abort: %v", err)
	}
	if _, err := os.Stat(readyPath); !os.IsNotExist(err) {
		t.Fatalf("child ready file still exists after abort: %v", err)
	}
}

// Same race for workspace daemons: the aborted start keeps the winner's markers.
func TestStartBackgroundWorkspaceAbortKeepsOtherDaemonMarkers(t *testing.T) {
	logDir := t.TempDir()
	ws := &config.Workspace{Name: "ws-race"}
	winnerPID := os.Getpid()
	pidPath := daemon.GetWorkspacePIDFile(logDir, ws.Name)
	readyPath := daemon.GetWorkspaceReadyFile(logDir, ws.Name)

	originalSpawn := watchSpawnWorkspace
	originalStop := watchStopProcess
	t.Cleanup(func() {
		watchSpawnWorkspace = originalSpawn
		watchStopProcess = originalStop
	})
	watchSpawnWorkspace = func(string, string, []string) (int, <-chan struct{}, error) {
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(winnerPID)+"\n"), 0600); err != nil {
			t.Fatalf("failed to write winner PID file: %v", err)
		}
		if err := os.WriteFile(readyPath, []byte("ready\n"+strconv.Itoa(winnerPID)+"\n"), 0600); err != nil {
			t.Fatalf("failed to write winner ready file: %v", err)
		}
		exited := make(chan struct{})
		close(exited)
		return 999997, exited, nil
	}
	watchStopProcess = func(int) error { return nil }

	if err := startBackgroundWorkspaceWatch(logDir, ws); err == nil {
		t.Fatal("startBackgroundWorkspaceWatch() succeeded although the child exited")
	}
	if pid, err := daemon.ReadWorkspacePIDFile(logDir, ws.Name); err != nil || pid != winnerPID {
		t.Fatalf("workspace PID file = %d, %v; want winner PID %d", pid, err, winnerPID)
	}
	if !daemon.IsWorkspaceReadyForPID(logDir, ws.Name, winnerPID) {
		t.Fatal("winner workspace ready file was removed by the aborted start")
	}
}

// An unreadable PID file cannot be attributed to the aborted child, so it is kept.
func TestRemoveDaemonMarkersOwnedByKeepsUnreadablePIDFile(t *testing.T) {
	removedPID, removedReady := false, false
	err := removeDaemonMarkersOwnedBy(42,
		func() (int, error) { return 0, errors.New("permission denied") },
		func() error { removedPID = true; return nil },
		func() bool { return false },
		func() error { removedReady = true; return nil },
	)
	if err != nil || removedPID || removedReady {
		t.Fatalf("removeDaemonMarkersOwnedBy() err=%v removedPID=%v removedReady=%v, want nothing removed", err, removedPID, removedReady)
	}
}
