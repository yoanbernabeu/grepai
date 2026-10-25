package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadyMarkersValidateExpectedPID(t *testing.T) {
	logDir := t.TempDir()
	if err := WriteReadyFile(logDir); err != nil {
		t.Fatal(err)
	}
	if !IsReadyForPID(logDir, os.Getpid()) {
		t.Fatal("current process ready marker was rejected")
	}
	if IsReadyForPID(logDir, os.Getpid()+1) {
		t.Fatal("ready marker for another PID was accepted")
	}

	if err := WriteWorktreeReadyFile(logDir, "wt"); err != nil {
		t.Fatal(err)
	}
	if !IsWorktreeReadyForPID(logDir, "wt", os.Getpid()) {
		t.Fatal("worktree ready marker PID was rejected")
	}
	if err := WriteWorkspaceReadyFile(logDir, "ws"); err != nil {
		t.Fatal(err)
	}
	if !IsWorkspaceReadyForPID(logDir, "ws", os.Getpid()) {
		t.Fatal("workspace ready marker PID was rejected")
	}
}

func TestStalePIDCleanupAlsoRemovesReadyMarkers(t *testing.T) {
	logDir := t.TempDir()
	tests := []struct {
		name      string
		pidPath   string
		readyPath string
		cleanup   func() (int, error)
	}{
		{
			name:      "project",
			pidPath:   filepath.Join(logDir, pidFileName),
			readyPath: filepath.Join(logDir, readyFileName),
			cleanup:   func() (int, error) { return GetRunningPID(logDir) },
		},
		{
			name:      "worktree",
			pidPath:   GetWorktreePIDFile(logDir, "wt"),
			readyPath: GetWorktreeReadyFile(logDir, "wt"),
			cleanup:   func() (int, error) { return GetRunningWorktreePID(logDir, "wt") },
		},
		{
			name:      "workspace",
			pidPath:   GetWorkspacePIDFile(logDir, "ws"),
			readyPath: GetWorkspaceReadyFile(logDir, "ws"),
			cleanup:   func() (int, error) { return GetRunningWorkspacePID(logDir, "ws") },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(tc.pidPath, []byte("-1\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(tc.readyPath, []byte("ready\n-1\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			pid, err := tc.cleanup()
			if err != nil || pid != 0 {
				t.Fatalf("cleanup = (%d, %v), want (0, nil)", pid, err)
			}
			if _, err := os.Stat(tc.readyPath); !os.IsNotExist(err) {
				t.Fatalf("stale ready marker remains: %v", err)
			}
		})
	}
}

func TestStartedMarkerIsNotReady(t *testing.T) {
	logDir := t.TempDir()
	if err := WriteStartedFile(logDir); err != nil {
		t.Fatal(err)
	}
	if IsReadyForPID(logDir, os.Getpid()) {
		t.Fatal("a started marker must not count as ready")
	}
	if !IsStartedForPID(logDir, os.Getpid()) {
		t.Fatal("started marker was not recognized")
	}
	if err := WriteReadyFile(logDir); err != nil {
		t.Fatal(err)
	}
	if !IsStartedForPID(logDir, os.Getpid()) || !IsReadyForPID(logDir, os.Getpid()) {
		t.Fatal("a ready watcher also counts as started")
	}
	if IsStartedForPID(logDir, os.Getpid()+1) {
		t.Fatal("started marker for another PID was accepted")
	}
}
