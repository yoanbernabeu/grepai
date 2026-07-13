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

	// Pre-seed documents so every b.N iteration hits the mtime-gate fast path,
	// same as TestIndexAllWithProgress_BranchSwitchSkipsBulkWithoutLookupOrEmbedding.
	// Without this, the first iteration would actually index all 800 files
	// (populating mockStore as a side effect), and only iterations after the
	// first would take the fast path -- making the benchmark's own assertion
	// fail whenever b.N > 1 (e.g. under -benchtime=Nx, or whenever Go's default
	// calibration decides more than one iteration is needed), independent of
	// anything being measured.
	for i := range 800 {
		path := fmt.Sprintf("file_%04d.go", i)
		mockStore.documents[path] = store.Document{
			Path:     path,
			Hash:     "seeded",
			ChunkIDs: []string{"c1"},
		}
	}

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

// BenchmarkIndexAllWithProgress_FullHashRescan exercises the path this
// change targets directly: every file's mtime looks new (e.g. after a
// watcher restart, git checkout, or CI clone where mtimes don't reflect
// history), so every file must be read and hashed to discover that its
// content actually hasn't changed. This is the phase that was previously
// fully sequential; see decideFileScan/scanWorkerLimit in indexer.go.
func BenchmarkIndexAllWithProgress_FullHashRescan(b *testing.B) {
	ctx := context.Background()
	tmpDir := b.TempDir()
	createGoFixtureFiles(b, tmpDir, 800)

	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		b.Fatalf("failed to create ignore matcher: %v", err)
	}

	scanner := NewScanner(tmpDir, ignoreMatcher)
	chunker := NewChunker(512, 50)

	// Pre-seed the store with the *real* content hash for each file so the
	// hash comparison in decideFileScan finds them unchanged, matching what
	// happens on a watcher restart against an already fully-indexed project.
	mockStore := newMockStore()
	for i := range 800 {
		path := fmt.Sprintf("file_%04d.go", i)
		info, err := scanner.ScanFile(path)
		if err != nil || info == nil {
			b.Fatalf("failed to pre-hash fixture %s: %v", path, err)
		}
		mockStore.documents[path] = store.Document{
			Path:     path,
			Hash:     info.Hash,
			ChunkIDs: []string{"c1"},
		}
	}

	mockEmbedder := newMockEmbedder()
	// lastIndexTime is intentionally zero so the mtime fast-path gate never
	// applies -- every file must go through ScanFile + hash comparison.
	idx := NewIndexer(tmpDir, mockStore, mockEmbedder, chunker, scanner, time.Time{})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		stats, err := idx.IndexAllWithProgress(ctx, nil)
		if err != nil {
			b.Fatalf("IndexAllWithProgress failed: %v", err)
		}
		if stats.FilesIndexed != 0 {
			b.Fatalf("expected 0 indexed files (all unchanged), got %d", stats.FilesIndexed)
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
