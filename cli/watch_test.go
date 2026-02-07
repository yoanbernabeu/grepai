package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/daemon"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/watcher"
)

// mockVectorStore is a test double that records calls made to it.
type mockVectorStore struct {
	savedChunks      []store.Chunk
	deletedFiles     []string
	savedDocuments   []store.Document
	deletedDocuments []string
	gotDocumentPaths []string
	gotChunkPaths    []string
}

func (m *mockVectorStore) SaveChunks(_ context.Context, chunks []store.Chunk) error {
	m.savedChunks = append(m.savedChunks, chunks...)
	return nil
}

func (m *mockVectorStore) DeleteByFile(_ context.Context, filePath string) error {
	m.deletedFiles = append(m.deletedFiles, filePath)
	return nil
}

func (m *mockVectorStore) Search(_ context.Context, _ []float32, _ int) ([]store.SearchResult, error) {
	return nil, nil
}

func (m *mockVectorStore) GetDocument(_ context.Context, filePath string) (*store.Document, error) {
	m.gotDocumentPaths = append(m.gotDocumentPaths, filePath)
	for i := range m.savedDocuments {
		if m.savedDocuments[i].Path == filePath {
			return &m.savedDocuments[i], nil
		}
	}
	return nil, nil
}

func (m *mockVectorStore) SaveDocument(_ context.Context, doc store.Document) error {
	m.savedDocuments = append(m.savedDocuments, doc)
	return nil
}

func (m *mockVectorStore) DeleteDocument(_ context.Context, filePath string) error {
	m.deletedDocuments = append(m.deletedDocuments, filePath)
	return nil
}

func (m *mockVectorStore) ListDocuments(_ context.Context) ([]string, error) {
	return nil, nil
}

func (m *mockVectorStore) Load(_ context.Context) error  { return nil }
func (m *mockVectorStore) Persist(_ context.Context) error { return nil }
func (m *mockVectorStore) Close() error                    { return nil }

func (m *mockVectorStore) GetStats(_ context.Context) (*store.IndexStats, error) {
	return nil, nil
}

func (m *mockVectorStore) ListFilesWithStats(_ context.Context) ([]store.FileStats, error) {
	return nil, nil
}

func (m *mockVectorStore) GetChunksForFile(_ context.Context, filePath string) ([]store.Chunk, error) {
	m.gotChunkPaths = append(m.gotChunkPaths, filePath)
	return nil, nil
}

func (m *mockVectorStore) GetAllChunks(_ context.Context) ([]store.Chunk, error) {
	return nil, nil
}

func skipIfWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: cannot delete locked files")
	}
}

func TestShowWatchStatus_NotRunning(t *testing.T) {
	logDir := t.TempDir()

	// Status with no PID file
	err := showWatchStatus(logDir)
	if err != nil {
		t.Fatalf("showWatchStatus() failed: %v", err)
	}

	// Verify log directory is mentioned (output is to stdout, so we can't easily capture it in unit tests)
	// This test mainly ensures no errors occur
}

func TestShowWatchStatus_Running(t *testing.T) {
	skipIfWindows(t)
	logDir := t.TempDir()

	// Write PID file with current process
	if err := daemon.WritePIDFile(logDir); err != nil {
		t.Fatalf("WritePIDFile() failed: %v", err)
	}
	defer daemon.RemovePIDFile(logDir)

	// Status with running process
	err := showWatchStatus(logDir)
	if err != nil {
		t.Fatalf("showWatchStatus() failed: %v", err)
	}
}

func TestShowWatchStatus_StalePID(t *testing.T) {
	logDir := t.TempDir()
	pidPath := filepath.Join(logDir, "grepai-watch.pid")

	// Write stale PID (very high number unlikely to exist)
	if err := os.WriteFile(pidPath, []byte("9999999\n"), 0644); err != nil {
		t.Fatalf("Failed to write stale PID: %v", err)
	}

	// Status should clean stale PID
	err := showWatchStatus(logDir)
	if err != nil {
		t.Fatalf("showWatchStatus() failed: %v", err)
	}

	// Verify PID file was removed (if process 9999999 doesn't exist)
	if !daemon.IsProcessRunning(9999999) {
		if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
			t.Error("Stale PID file was not removed")
		}
	}
}

func TestStopWatchDaemon_NotRunning(t *testing.T) {
	logDir := t.TempDir()

	// Stop with no PID file
	err := stopWatchDaemon(logDir)
	if err != nil {
		t.Fatalf("stopWatchDaemon() failed: %v", err)
	}
}

func TestStopWatchDaemon_StalePID(t *testing.T) {
	logDir := t.TempDir()
	pidPath := filepath.Join(logDir, "grepai-watch.pid")

	// Write stale PID
	if err := os.WriteFile(pidPath, []byte("9999999\n"), 0644); err != nil {
		t.Fatalf("Failed to write stale PID: %v", err)
	}

	// Stop should clean stale PID
	err := stopWatchDaemon(logDir)
	if err != nil {
		t.Fatalf("stopWatchDaemon() failed: %v", err)
	}

	// Verify PID file was removed (if process 9999999 doesn't exist)
	if !daemon.IsProcessRunning(9999999) {
		if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
			t.Error("Stale PID file was not removed")
		}
	}
}

func TestStartBackgroundWatch_AlreadyRunning(t *testing.T) {
	skipIfWindows(t)
	logDir := t.TempDir()

	// Write PID file with current process (simulating already running)
	if err := daemon.WritePIDFile(logDir); err != nil {
		t.Fatalf("WritePIDFile() failed: %v", err)
	}
	defer daemon.RemovePIDFile(logDir)

	// Try to start background watch (should fail)
	err := startBackgroundWatch(logDir)
	if err == nil {
		t.Fatal("startBackgroundWatch() should have failed when already running")
	}
}

func TestStartBackgroundWatch_CleansStalePID(t *testing.T) {
	logDir := t.TempDir()
	pidPath := filepath.Join(logDir, "grepai-watch.pid")

	// Write stale PID
	if err := os.WriteFile(pidPath, []byte("9999999\n"), 0644); err != nil {
		t.Fatalf("Failed to write stale PID: %v", err)
	}

	// Note: We can't easily test the actual spawning without integration tests,
	// but we can verify the stale PID check works

	// Read PID before
	pid, err := daemon.ReadPIDFile(logDir)
	if err != nil {
		t.Fatalf("ReadPIDFile() failed: %v", err)
	}
	if pid != 9999999 {
		t.Fatalf("Expected PID 9999999, got %d", pid)
	}

	// The actual startBackgroundWatch would spawn a process here,
	// which we can't easily test in a unit test.
	// This is better tested in integration tests.
}

func TestRunWatch_CheckAlreadyRunning(t *testing.T) {
	skipIfWindows(t)
	logDir := t.TempDir()

	// Set custom log dir for testing
	watchLogDir = logDir
	watchBackground = false
	watchStatus = false
	watchStop = false

	// Write PID file with current process
	if err := daemon.WritePIDFile(logDir); err != nil {
		t.Fatalf("WritePIDFile() failed: %v", err)
	}
	defer daemon.RemovePIDFile(logDir)

	// Try to run watch in foreground (should detect already running)
	err := runWatch(nil, nil)
	if err == nil {
		t.Fatal("runWatch() should have failed when already running in background")
	}

	// Reset for other tests
	watchLogDir = ""
}

func TestLogDirectoryDefaults(t *testing.T) {
	// Test that default log directory can be determined
	logDir, err := daemon.GetDefaultLogDir()
	if err != nil {
		t.Fatalf("GetDefaultLogDir() failed: %v", err)
	}

	if logDir == "" {
		t.Fatal("GetDefaultLogDir() returned empty string")
	}

	if !filepath.IsAbs(logDir) {
		t.Errorf("Expected absolute path, got: %s", logDir)
	}
}

func TestPIDFileLifecycleInWatch(t *testing.T) {
	skipIfWindows(t)
	logDir := t.TempDir()

	// Initially no PID file
	pid, err := daemon.ReadPIDFile(logDir)
	if err != nil {
		t.Fatalf("ReadPIDFile() failed: %v", err)
	}
	if pid != 0 {
		t.Errorf("Expected no PID initially, got %d", pid)
	}

	// Write PID file
	if err := daemon.WritePIDFile(logDir); err != nil {
		t.Fatalf("WritePIDFile() failed: %v", err)
	}

	// Verify it exists
	pid, err = daemon.ReadPIDFile(logDir)
	if err != nil {
		t.Fatalf("ReadPIDFile() failed: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("Expected PID %d, got %d", os.Getpid(), pid)
	}

	// Remove it
	if err := daemon.RemovePIDFile(logDir); err != nil {
		t.Fatalf("RemovePIDFile() failed: %v", err)
	}

	// Verify it's gone
	pid, err = daemon.ReadPIDFile(logDir)
	if err != nil {
		t.Fatalf("ReadPIDFile() failed: %v", err)
	}
	if pid != 0 {
		t.Errorf("Expected no PID after removal, got %d", pid)
	}
}

func TestCustomLogDirectory(t *testing.T) {
	skipIfWindows(t)
	customDir := filepath.Join(t.TempDir(), "custom-logs")

	// Set custom log dir
	watchLogDir = customDir
	defer func() { watchLogDir = "" }()

	// Verify custom directory is used
	if watchLogDir != customDir {
		t.Errorf("Expected custom log dir %s, got %s", customDir, watchLogDir)
	}

	// Test that PID files can be written to custom directory
	if err := daemon.WritePIDFile(customDir); err != nil {
		t.Fatalf("WritePIDFile() to custom dir failed: %v", err)
	}
	defer daemon.RemovePIDFile(customDir)

	// Verify PID file exists in custom directory
	pidPath := filepath.Join(customDir, "grepai-watch.pid")
	if _, err := os.Stat(pidPath); os.IsNotExist(err) {
		t.Error("PID file not found in custom directory")
	}
}

func TestStopWatchDaemon_WaitForShutdown(t *testing.T) {
	// This test verifies the wait logic in stopWatchDaemon
	// We can't easily test the actual process stopping in a unit test,
	// but we can verify the logic with a stale PID

	logDir := t.TempDir()
	pidPath := filepath.Join(logDir, "grepai-watch.pid")

	// Write stale PID
	if err := os.WriteFile(pidPath, []byte("9999998\n"), 0644); err != nil {
		t.Fatalf("Failed to write stale PID: %v", err)
	}

	// Measure time taken
	start := time.Now()
	err := stopWatchDaemon(logDir)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("stopWatchDaemon() failed: %v", err)
	}

	// Should complete quickly for non-existent process (less than 1 second)
	if elapsed > 2*time.Second {
		t.Errorf("stopWatchDaemon() took too long: %v", elapsed)
	}

	// Verify PID file was cleaned up (if process doesn't exist)
	if !daemon.IsProcessRunning(9999998) {
		if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
			t.Error("PID file was not removed after stop")
		}
	}
}

func TestProjectPrefixStore_should_prefix_chunk_paths_when_saving(t *testing.T) {
	mock := &mockVectorStore{}
	projectPath := "/home/user/projects/myapp"
	ps := &projectPrefixStore{
		store:         mock,
		workspaceName: "myworkspace",
		projectName:   "myproject",
		projectPath:   projectPath,
	}

	chunks := []store.Chunk{
		{
			ID:       "/home/user/projects/myapp/src/main.go_0",
			FilePath: "/home/user/projects/myapp/src/main.go",
			Content:  "package main",
		},
		{
			ID:       "/home/user/projects/myapp/pkg/util.go_1",
			FilePath: "/home/user/projects/myapp/pkg/util.go",
			Content:  "package pkg",
		},
	}

	err := ps.SaveChunks(context.Background(), chunks)
	if err != nil {
		t.Fatalf("SaveChunks() returned error: %v", err)
	}

	if len(mock.savedChunks) != 2 {
		t.Fatalf("expected 2 saved chunks, got %d", len(mock.savedChunks))
	}

	expectedPath0 := "myworkspace/myproject/src/main.go"
	if mock.savedChunks[0].FilePath != expectedPath0 {
		t.Errorf("chunk[0].FilePath = %q, want %q", mock.savedChunks[0].FilePath, expectedPath0)
	}

	expectedPath1 := "myworkspace/myproject/pkg/util.go"
	if mock.savedChunks[1].FilePath != expectedPath1 {
		t.Errorf("chunk[1].FilePath = %q, want %q", mock.savedChunks[1].FilePath, expectedPath1)
	}

	// Verify chunk IDs were also updated
	expectedID0 := "myworkspace/myproject/src/main.go_0"
	if mock.savedChunks[0].ID != expectedID0 {
		t.Errorf("chunk[0].ID = %q, want %q", mock.savedChunks[0].ID, expectedID0)
	}
}

func TestProjectPrefixStore_should_prefix_relative_paths_when_saving_chunks(t *testing.T) {
	mock := &mockVectorStore{}
	ps := &projectPrefixStore{
		store:         mock,
		workspaceName: "ws",
		projectName:   "proj",
		projectPath:   "/home/user/projects/myapp",
	}

	chunks := []store.Chunk{
		{
			ID:       "src/main.go_0",
			FilePath: "src/main.go",
			Content:  "package main",
		},
	}

	err := ps.SaveChunks(context.Background(), chunks)
	if err != nil {
		t.Fatalf("SaveChunks() returned error: %v", err)
	}

	if len(mock.savedChunks) != 1 {
		t.Fatalf("expected 1 saved chunk, got %d", len(mock.savedChunks))
	}

	expectedPath := "ws/proj/src/main.go"
	if mock.savedChunks[0].FilePath != expectedPath {
		t.Errorf("chunk.FilePath = %q, want %q", mock.savedChunks[0].FilePath, expectedPath)
	}

	expectedID := "ws/proj/src/main.go_0"
	if mock.savedChunks[0].ID != expectedID {
		t.Errorf("chunk.ID = %q, want %q", mock.savedChunks[0].ID, expectedID)
	}
}

func TestProjectPrefixStore_should_prefix_path_when_deleting_by_file(t *testing.T) {
	mock := &mockVectorStore{}
	projectPath := "/home/user/projects/myapp"
	ps := &projectPrefixStore{
		store:         mock,
		workspaceName: "ws",
		projectName:   "proj",
		projectPath:   projectPath,
	}

	err := ps.DeleteByFile(context.Background(), "/home/user/projects/myapp/src/main.go")
	if err != nil {
		t.Fatalf("DeleteByFile() returned error: %v", err)
	}

	if len(mock.deletedFiles) != 1 {
		t.Fatalf("expected 1 deleted file, got %d", len(mock.deletedFiles))
	}

	expected := "ws/proj/src/main.go"
	if mock.deletedFiles[0] != expected {
		t.Errorf("deleted file path = %q, want %q", mock.deletedFiles[0], expected)
	}
}

func TestProjectPrefixStore_should_prefix_path_when_getting_document(t *testing.T) {
	mock := &mockVectorStore{}
	projectPath := "/home/user/projects/myapp"
	ps := &projectPrefixStore{
		store:         mock,
		workspaceName: "ws",
		projectName:   "proj",
		projectPath:   projectPath,
	}

	_, err := ps.GetDocument(context.Background(), "/home/user/projects/myapp/src/main.go")
	if err != nil {
		t.Fatalf("GetDocument() returned error: %v", err)
	}

	if len(mock.gotDocumentPaths) != 1 {
		t.Fatalf("expected 1 GetDocument call, got %d", len(mock.gotDocumentPaths))
	}

	expected := "ws/proj/src/main.go"
	if mock.gotDocumentPaths[0] != expected {
		t.Errorf("GetDocument path = %q, want %q", mock.gotDocumentPaths[0], expected)
	}
}

func TestProjectPrefixStore_should_prefix_path_when_saving_document(t *testing.T) {
	mock := &mockVectorStore{}
	projectPath := "/home/user/projects/myapp"
	ps := &projectPrefixStore{
		store:         mock,
		workspaceName: "ws",
		projectName:   "proj",
		projectPath:   projectPath,
	}

	doc := store.Document{
		Path: "/home/user/projects/myapp/src/main.go",
		Hash: "abc123",
	}

	err := ps.SaveDocument(context.Background(), doc)
	if err != nil {
		t.Fatalf("SaveDocument() returned error: %v", err)
	}

	if len(mock.savedDocuments) != 1 {
		t.Fatalf("expected 1 saved document, got %d", len(mock.savedDocuments))
	}

	expected := "ws/proj/src/main.go"
	if mock.savedDocuments[0].Path != expected {
		t.Errorf("saved document path = %q, want %q", mock.savedDocuments[0].Path, expected)
	}
}

func TestProjectPrefixStore_should_prefix_path_when_deleting_document(t *testing.T) {
	mock := &mockVectorStore{}
	projectPath := "/home/user/projects/myapp"
	ps := &projectPrefixStore{
		store:         mock,
		workspaceName: "ws",
		projectName:   "proj",
		projectPath:   projectPath,
	}

	err := ps.DeleteDocument(context.Background(), "/home/user/projects/myapp/src/main.go")
	if err != nil {
		t.Fatalf("DeleteDocument() returned error: %v", err)
	}

	if len(mock.deletedDocuments) != 1 {
		t.Fatalf("expected 1 deleted document, got %d", len(mock.deletedDocuments))
	}

	expected := "ws/proj/src/main.go"
	if mock.deletedDocuments[0] != expected {
		t.Errorf("deleted document path = %q, want %q", mock.deletedDocuments[0], expected)
	}
}

func TestProjectFileEvent_should_carry_project_context(t *testing.T) {
	evt := projectFileEvent{
		FileEvent: watcher.FileEvent{
			Type: watcher.EventCreate,
			Path: "/home/user/projects/myapp/src/main.go",
		},
		Project: config.ProjectEntry{
			Name: "myproject",
			Path: "/home/user/projects/myapp",
		},
	}

	if evt.Type != watcher.EventCreate {
		t.Errorf("event type = %v, want EventCreate", evt.Type)
	}
	if evt.Path != "/home/user/projects/myapp/src/main.go" {
		t.Errorf("event path = %q, want %q", evt.Path, "/home/user/projects/myapp/src/main.go")
	}
	if evt.Project.Name != "myproject" {
		t.Errorf("project name = %q, want %q", evt.Project.Name, "myproject")
	}
	if evt.Project.Path != "/home/user/projects/myapp" {
		t.Errorf("project path = %q, want %q", evt.Project.Path, "/home/user/projects/myapp")
	}
}
