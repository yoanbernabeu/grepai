package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspacePIDFunctions(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "grepai-daemon-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	workspaceName := "test-workspace"

	// Test GetWorkspacePIDFile
	t.Run("GetWorkspacePIDFile", func(t *testing.T) {
		pidFile := GetWorkspacePIDFile(tmpDir, workspaceName)
		expected := filepath.Join(tmpDir, "grepai-workspace-test-workspace.pid")
		if pidFile != expected {
			t.Errorf("expected %s, got %s", expected, pidFile)
		}
	})

	// Test GetWorkspaceLogFile
	t.Run("GetWorkspaceLogFile", func(t *testing.T) {
		logFile := GetWorkspaceLogFile(tmpDir, workspaceName)
		expected := filepath.Join(tmpDir, "grepai-workspace-test-workspace.log")
		if logFile != expected {
			t.Errorf("expected %s, got %s", expected, logFile)
		}
	})

	// Test GetWorkspaceReadyFile
	t.Run("GetWorkspaceReadyFile", func(t *testing.T) {
		readyFile := GetWorkspaceReadyFile(tmpDir, workspaceName)
		expected := filepath.Join(tmpDir, "grepai-workspace-test-workspace.ready")
		if readyFile != expected {
			t.Errorf("expected %s, got %s", expected, readyFile)
		}
	})

	// Test ReadWorkspacePIDFile with non-existent file
	t.Run("ReadWorkspacePIDFile_NonExistent", func(t *testing.T) {
		pid, err := ReadWorkspacePIDFile(tmpDir, "non-existent")
		if err != nil {
			t.Errorf("expected no error for non-existent file, got %v", err)
		}
		if pid != 0 {
			t.Errorf("expected pid 0 for non-existent file, got %d", pid)
		}
	})

	// Test WriteWorkspaceReadyFile and IsWorkspaceReady
	t.Run("WriteAndCheckWorkspaceReady", func(t *testing.T) {
		// Initially should not be ready
		if IsWorkspaceReady(tmpDir, workspaceName) {
			t.Error("workspace should not be ready before writing ready file")
		}

		// Write ready file
		err := WriteWorkspaceReadyFile(tmpDir, workspaceName)
		if err != nil {
			t.Errorf("failed to write ready file: %v", err)
		}

		// Now should be ready
		if !IsWorkspaceReady(tmpDir, workspaceName) {
			t.Error("workspace should be ready after writing ready file")
		}

		// Remove ready file
		err = RemoveWorkspaceReadyFile(tmpDir, workspaceName)
		if err != nil {
			t.Errorf("failed to remove ready file: %v", err)
		}

		// Should not be ready anymore
		if IsWorkspaceReady(tmpDir, workspaceName) {
			t.Error("workspace should not be ready after removing ready file")
		}
	})

	// Test GetRunningWorkspacePID with no PID file
	t.Run("GetRunningWorkspacePID_NoPID", func(t *testing.T) {
		pid, err := GetRunningWorkspacePID(tmpDir, "no-pid-workspace")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if pid != 0 {
			t.Errorf("expected pid 0, got %d", pid)
		}
	})

	// Test RemoveWorkspacePIDFile with non-existent file
	t.Run("RemoveWorkspacePIDFile_NonExistent", func(t *testing.T) {
		err := RemoveWorkspacePIDFile(tmpDir, "non-existent")
		if err != nil {
			t.Errorf("expected no error when removing non-existent PID file, got %v", err)
		}
	})

	// Test RemoveWorkspaceReadyFile with non-existent file
	t.Run("RemoveWorkspaceReadyFile_NonExistent", func(t *testing.T) {
		err := RemoveWorkspaceReadyFile(tmpDir, "non-existent")
		if err != nil {
			t.Errorf("expected no error when removing non-existent ready file, got %v", err)
		}
	})
}

func TestWorkspaceFileNaming(t *testing.T) {
	logDir := "/var/log/grepai"

	tests := []struct {
		name          string
		workspaceName string
		wantPID       string
		wantLog       string
		wantReady     string
	}{
		{
			name:          "simple_name",
			workspaceName: "myworkspace",
			wantPID:       "/var/log/grepai/grepai-workspace-myworkspace.pid",
			wantLog:       "/var/log/grepai/grepai-workspace-myworkspace.log",
			wantReady:     "/var/log/grepai/grepai-workspace-myworkspace.ready",
		},
		{
			name:          "name_with_hyphen",
			workspaceName: "my-workspace",
			wantPID:       "/var/log/grepai/grepai-workspace-my-workspace.pid",
			wantLog:       "/var/log/grepai/grepai-workspace-my-workspace.log",
			wantReady:     "/var/log/grepai/grepai-workspace-my-workspace.ready",
		},
		{
			name:          "name_with_underscore",
			workspaceName: "my_workspace",
			wantPID:       "/var/log/grepai/grepai-workspace-my_workspace.pid",
			wantLog:       "/var/log/grepai/grepai-workspace-my_workspace.log",
			wantReady:     "/var/log/grepai/grepai-workspace-my_workspace.ready",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetWorkspacePIDFile(logDir, tt.workspaceName); got != tt.wantPID {
				t.Errorf("GetWorkspacePIDFile() = %v, want %v", got, tt.wantPID)
			}
			if got := GetWorkspaceLogFile(logDir, tt.workspaceName); got != tt.wantLog {
				t.Errorf("GetWorkspaceLogFile() = %v, want %v", got, tt.wantLog)
			}
			if got := GetWorkspaceReadyFile(logDir, tt.workspaceName); got != tt.wantReady {
				t.Errorf("GetWorkspaceReadyFile() = %v, want %v", got, tt.wantReady)
			}
		})
	}
}
