package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

func createGoFixtureFiles(tb testing.TB, root string, fileCount int) {
	tb.Helper()

	content := "package main\n\n" + strings.Repeat("func f() int { return 1 }\n", 80)
	for i := range fileCount {
		filePath := filepath.Join(root, fmt.Sprintf("file_%04d.go", i))
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			tb.Fatalf("failed to create fixture file %s: %v", filePath, err)
		}
	}
}

func TestIndexAllWithProgress_BranchSwitchSkipsBulkWithoutLookupOrEmbedding(t *testing.T) {
	tmpDir := t.TempDir()
	createGoFixtureFiles(t, tmpDir, 200)

	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	mockStore := newMockStore()
	// Seed documents with ChunkIDs so the lastIndexTime gate can skip them.
	// The new logic requires doc != nil && len(doc.ChunkIDs) > 0 to skip.
	for i := range 200 {
		path := fmt.Sprintf("file_%04d.go", i)
		mockStore.documents[path] = store.Document{
			Path:     path,
			Hash:     "seeded",
			ChunkIDs: []string{"c1"},
		}
	}
	mockEmbedder := newMockEmbedder()
	scanner := NewScanner(tmpDir, ignoreMatcher)
	chunker := NewChunker(512, 50)

	// Simulate watcher restart after latest changes: all fixture files are older than cutoff.
	lastIndexTime := time.Now().Add(1 * time.Hour)
	idx := NewIndexer(tmpDir, mockStore, mockEmbedder, chunker, scanner, lastIndexTime)

	stats, err := idx.IndexAllWithProgress(context.Background(), nil)
	if err != nil {
		t.Fatalf("IndexAllWithProgress failed: %v", err)
	}

	if stats.FilesIndexed != 0 {
		t.Fatalf("expected 0 indexed files, got %d", stats.FilesIndexed)
	}
	if stats.ChunksCreated != 0 {
		t.Fatalf("expected 0 created chunks, got %d", stats.ChunksCreated)
	}
	if stats.FilesSkipped < 200 {
		t.Fatalf("expected at least 200 skipped files, got %d", stats.FilesSkipped)
	}
	if mockEmbedder.embedCalled {
		t.Fatal("embedder should not be called when all files are skipped")
	}
}

func BenchmarkIndexAllWithProgress_BranchSwitchScenario(b *testing.B) {
	ctx := context.Background()
	tmpDir := b.TempDir()
	createGoFixtureFiles(b, tmpDir, 800)

	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		b.Fatalf("failed to create ignore matcher: %v", err)
	}

	mockStore := newMockStore()
	mockEmbedder := newMockEmbedder()
	scanner := NewScanner(tmpDir, ignoreMatcher)
	chunker := NewChunker(512, 50)
	lastIndexTime := time.Now().Add(1 * time.Hour)
	idx := NewIndexer(tmpDir, mockStore, mockEmbedder, chunker, scanner, lastIndexTime)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		stats, err := idx.IndexAllWithProgress(ctx, nil)
		if err != nil {
			b.Fatalf("IndexAllWithProgress failed: %v", err)
		}
		if stats.FilesIndexed != 0 {
			b.Fatalf("expected 0 indexed files, got %d", stats.FilesIndexed)
		}
	}
}

func TestIndexAllWithProgress_BinaryFileSkippedAfterMetadataPass(t *testing.T) {
	tmpDir := t.TempDir()

	// Binary-like content: metadata pass includes this file, ScanFile should drop it.
	binaryPath := filepath.Join(tmpDir, "binary.go")
	if err := os.WriteFile(binaryPath, []byte("package main\x00"), 0644); err != nil {
		t.Fatalf("failed to create binary fixture: %v", err)
	}

	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	mockStore := newMockStore()
	mockEmbedder := newMockEmbedder()
	scanner := NewScanner(tmpDir, ignoreMatcher)
	chunker := NewChunker(512, 50)
	idx := NewIndexer(tmpDir, mockStore, mockEmbedder, chunker, scanner, time.Time{})

	stats, err := idx.IndexAllWithProgress(context.Background(), nil)
	if err != nil {
		t.Fatalf("IndexAllWithProgress failed: %v", err)
	}

	if stats.FilesIndexed != 0 {
		t.Fatalf("expected 0 indexed files, got %d", stats.FilesIndexed)
	}
	if !mockStore.listDocsCalled {
		t.Fatal("expected ListDocuments to be called")
	}
	if mockEmbedder.embedCalled {
		t.Fatal("expected embedder to not be called for binary file")
	}
}

func TestIndexAllWithProgress_UnreadableFileSkippedAfterMetadataPass(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission behavior differs on windows")
	}

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "restricted.go")
	if err := os.WriteFile(srcPath, []byte("package main\n\nfunc x() {}\n"), 0644); err != nil {
		t.Fatalf("failed to create fixture file: %v", err)
	}
	if err := os.Chmod(srcPath, 0o000); err != nil {
		t.Fatalf("failed to chmod fixture file: %v", err)
	}
	defer func() {
		_ = os.Chmod(srcPath, 0o644)
	}()

	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}

	mockStore := newMockStore()
	mockEmbedder := newMockEmbedder()
	scanner := NewScanner(tmpDir, ignoreMatcher)
	chunker := NewChunker(512, 50)
	idx := NewIndexer(tmpDir, mockStore, mockEmbedder, chunker, scanner, time.Time{})

	stats, err := idx.IndexAllWithProgress(context.Background(), nil)
	if err != nil {
		t.Fatalf("IndexAllWithProgress failed: %v", err)
	}

	if stats.FilesIndexed != 0 {
		t.Fatalf("expected 0 indexed files, got %d", stats.FilesIndexed)
	}
	if mockEmbedder.embedCalled {
		t.Fatal("expected embedder to not be called for unreadable file")
	}
}
