package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/daemon"
)

func TestWaitForBackgroundReadyChecksChildExitFirst(t *testing.T) {
	logDir := t.TempDir()
	const childPID = 4242
	readyPath := daemon.GetWorkspaceReadyFile(logDir, "ws")
	if err := os.WriteFile(readyPath, []byte("ready\n4242\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	exitCh := make(chan struct{})
	close(exitCh)

	err := waitForBackgroundReady(exitCh, childPID, func(pid int) bool {
		return daemon.IsWorkspaceReadyForPID(logDir, "ws", pid)
	}, time.Minute, time.Minute)
	if err == nil {
		t.Fatal("stale ready marker won over exited child")
	}
}

func TestWaitForBackgroundReadyAcceptsMatchingLiveChildMarker(t *testing.T) {
	exitCh := make(chan struct{})
	if err := waitForBackgroundReady(exitCh, 42, func(pid int) bool { return pid == 42 }, time.Minute, time.Minute); err != nil {
		t.Fatalf("waitForBackgroundReady() error = %v", err)
	}
}

func TestRegistrationStartupClearsStaleReadyMarkers(t *testing.T) {
	logDir := t.TempDir()
	if err := daemon.WriteReadyFile(logDir); err != nil {
		t.Fatal(err)
	}
	if err := removeProjectReadyMarker(logDir, ""); err != nil {
		t.Fatal(err)
	}
	if daemon.IsReady(logDir) {
		t.Fatal("project ready marker survived startup preparation")
	}

	if err := daemon.WriteWorktreeReadyFile(logDir, "wt"); err != nil {
		t.Fatal(err)
	}
	if err := removeProjectReadyMarker(logDir, "wt"); err != nil {
		t.Fatal(err)
	}
	if daemon.IsWorktreeReady(logDir, "wt") {
		t.Fatal("worktree ready marker survived startup preparation")
	}
}

func TestStartBackgroundWatchRejectsStaleReadyWhenChildFails(t *testing.T) {
	logDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(logDir, "grepai-watch.pid"), []byte("-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "grepai-watch.ready"), []byte("ready\n4242\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	original := watchSpawnBackground
	originalStop := watchStopProcess
	t.Cleanup(func() { watchSpawnBackground = original; watchStopProcess = originalStop })
	watchStopProcess = func(int) error { return errors.New("child already exited") }
	watchSpawnBackground = func(dir string, _ []string) (int, <-chan struct{}, error) {
		if daemon.IsReady(dir) {
			t.Fatal("stale ready marker was not removed before spawn")
		}
		exited := make(chan struct{})
		close(exited) // Simulates registration ENOSPC terminating the child.
		return 4242, exited, nil
	}

	if err := startBackgroundWatch(logDir, ""); err == nil {
		t.Fatal("startBackgroundWatch() accepted stale readiness after child failure")
	}
}

func TestStartBackgroundWorkspaceRejectsStaleReadyWhenChildFails(t *testing.T) {
	logDir := t.TempDir()
	ws := &config.Workspace{Name: "ws"}
	if err := os.WriteFile(daemon.GetWorkspacePIDFile(logDir, ws.Name), []byte("-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(daemon.GetWorkspaceReadyFile(logDir, ws.Name), []byte("ready\n4242\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	original := watchSpawnWorkspace
	originalStop := watchStopProcess
	t.Cleanup(func() { watchSpawnWorkspace = original; watchStopProcess = originalStop })
	watchStopProcess = func(int) error { return errors.New("child already exited") }
	watchSpawnWorkspace = func(dir, name string, _ []string) (int, <-chan struct{}, error) {
		if daemon.IsWorkspaceReady(dir, name) {
			t.Fatal("stale workspace ready marker was not removed before spawn")
		}
		exited := make(chan struct{})
		close(exited)
		return 4242, exited, nil
	}

	if err := startBackgroundWorkspaceWatch(logDir, ws); err == nil {
		t.Fatal("startBackgroundWorkspaceWatch() accepted stale readiness after child failure")
	}
}

func TestStartBackgroundWatchMatchingReadyPathUnaffected(t *testing.T) {
	logDir := t.TempDir()
	original := watchSpawnBackground
	t.Cleanup(func() { watchSpawnBackground = original })
	watchSpawnBackground = func(dir string, _ []string) (int, <-chan struct{}, error) {
		if err := daemon.WriteReadyFile(dir); err != nil {
			t.Fatal(err)
		}
		return os.Getpid(), make(chan struct{}), nil
	}
	if err := startBackgroundWatch(logDir, ""); err != nil {
		t.Fatalf("startBackgroundWatch() error = %v", err)
	}
}

func TestStartBackgroundWatchTimeoutStopsChildAndCleansMarkers(t *testing.T) {
	logDir := t.TempDir()
	exitCh := make(chan struct{})
	originalSpawn := watchSpawnBackground
	originalWait := watchWaitForReady
	originalStop := watchStopProcess
	t.Cleanup(func() {
		watchSpawnBackground = originalSpawn
		watchWaitForReady = originalWait
		watchStopProcess = originalStop
	})
	watchSpawnBackground = func(dir string, _ []string) (int, <-chan struct{}, error) {
		if err := os.WriteFile(filepath.Join(dir, "grepai-watch.pid"), []byte("4242\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		// Stale marker from an earlier run: this child never reported progress.
		if err := os.WriteFile(filepath.Join(dir, "grepai-watch.ready"), []byte("ready\n9999\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return 4242, exitCh, nil
	}
	watchWaitForReady = func(<-chan struct{}, int, func(int) bool, time.Duration, time.Duration) error {
		return errors.New("startup timeout")
	}
	stopCalled := false
	watchStopProcess = func(pid int) error {
		stopCalled = pid == 4242
		close(exitCh)
		return nil
	}

	if err := startBackgroundWatch(logDir, ""); err == nil {
		t.Fatal("startBackgroundWatch() succeeded after timeout")
	}
	if !stopCalled {
		t.Fatal("timed-out child was not stopped")
	}
	if _, err := os.Stat(filepath.Join(logDir, "grepai-watch.pid")); !os.IsNotExist(err) {
		t.Fatalf("PID marker remains: %v", err)
	}
	if daemon.IsReady(logDir) {
		t.Fatal("ready marker remains after timeout")
	}
}

func TestStartBackgroundWorkspaceTimeoutStopsChildAndCleansMarkers(t *testing.T) {
	logDir := t.TempDir()
	ws := &config.Workspace{Name: "ws"}
	exitCh := make(chan struct{})
	originalSpawn := watchSpawnWorkspace
	originalWait := watchWaitForReady
	originalStop := watchStopProcess
	t.Cleanup(func() {
		watchSpawnWorkspace = originalSpawn
		watchWaitForReady = originalWait
		watchStopProcess = originalStop
	})
	watchSpawnWorkspace = func(dir, name string, _ []string) (int, <-chan struct{}, error) {
		if err := os.WriteFile(daemon.GetWorkspacePIDFile(dir, name), []byte("4242\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		// Stale marker from an earlier run: this child never reported progress.
		if err := os.WriteFile(daemon.GetWorkspaceReadyFile(dir, name), []byte("ready\n9999\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return 4242, exitCh, nil
	}
	watchWaitForReady = func(<-chan struct{}, int, func(int) bool, time.Duration, time.Duration) error {
		return errors.New("startup timeout")
	}
	stopCalled := false
	watchStopProcess = func(int) error {
		stopCalled = true
		close(exitCh)
		return nil
	}

	if err := startBackgroundWorkspaceWatch(logDir, ws); err == nil {
		t.Fatal("startBackgroundWorkspaceWatch() succeeded after timeout")
	}
	if !stopCalled {
		t.Fatal("timed-out workspace child was not stopped")
	}
	if _, err := os.Stat(daemon.GetWorkspacePIDFile(logDir, ws.Name)); !os.IsNotExist(err) {
		t.Fatalf("workspace PID marker remains: %v", err)
	}
	if daemon.IsWorkspaceReady(logDir, ws.Name) {
		t.Fatal("workspace ready marker remains after timeout")
	}
}

// A child that passed its startup checks ("started") but is still building
// its initial index when the deadline expires keeps running (#218).
func TestStartBackgroundWatchTimeoutKeepsStartedChild(t *testing.T) {
	logDir := t.TempDir()
	exitCh := make(chan struct{})
	originalSpawn := watchSpawnBackground
	originalWait := watchWaitForReady
	originalStop := watchStopProcess
	t.Cleanup(func() {
		watchSpawnBackground = originalSpawn
		watchWaitForReady = originalWait
		watchStopProcess = originalStop
	})
	watchSpawnBackground = func(dir string, _ []string) (int, <-chan struct{}, error) {
		if err := os.WriteFile(filepath.Join(dir, "grepai-watch.pid"), []byte("4242\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "grepai-watch.ready"), []byte("started\n4242\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return 4242, exitCh, nil
	}
	watchWaitForReady = func(<-chan struct{}, int, func(int) bool, time.Duration, time.Duration) error {
		return errors.New("startup timeout")
	}
	watchStopProcess = func(int) error {
		t.Fatal("a started child must not be stopped")
		return nil
	}

	if err := startBackgroundWatch(logDir, ""); err != nil {
		t.Fatalf("startBackgroundWatch() error = %v", err)
	}
	if !daemon.IsStartedForPID(logDir, 4242) || daemon.IsReadyForPID(logDir, 4242) {
		t.Fatal("started marker should be kept and not count as ready")
	}
}
