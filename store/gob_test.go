package store

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestGOBStore_SaveAndSearchChunks(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.gob")

	store := NewGOBStore(indexPath)
	ctx := context.Background()

	// Create test chunks with vectors
	chunks := []Chunk{
		{
			ID:        "chunk1",
			FilePath:  "test.go",
			StartLine: 1,
			EndLine:   10,
			Content:   "func main() {}",
			Vector:    []float32{1.0, 0.0, 0.0},
			Hash:      "abc123",
			UpdatedAt: time.Now(),
		},
		{
			ID:        "chunk2",
			FilePath:  "test.go",
			StartLine: 11,
			EndLine:   20,
			Content:   "func helper() {}",
			Vector:    []float32{0.0, 1.0, 0.0},
			Hash:      "def456",
			UpdatedAt: time.Now(),
		},
	}

	// Save chunks
	err := store.SaveChunks(ctx, chunks)
	if err != nil {
		t.Fatalf("failed to save chunks: %v", err)
	}

	// Search with a query vector similar to first chunk
	queryVector := []float32{0.9, 0.1, 0.0}
	results, err := store.Search(ctx, queryVector, 10, SearchOptions{})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// First result should be chunk1 (most similar)
	if results[0].Chunk.ID != "chunk1" {
		t.Errorf("expected chunk1 as first result, got %s", results[0].Chunk.ID)
	}
}

func TestGOBStore_DeleteByFile(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.gob")

	store := NewGOBStore(indexPath)
	ctx := context.Background()

	// Save document metadata
	doc := Document{
		Path:     "test.go",
		Hash:     "abc123",
		ModTime:  time.Now(),
		ChunkIDs: []string{"chunk1", "chunk2"},
	}
	err := store.SaveDocument(ctx, doc)
	if err != nil {
		t.Fatalf("failed to save document: %v", err)
	}

	// Save chunks
	chunks := []Chunk{
		{ID: "chunk1", FilePath: "test.go", Vector: []float32{1.0, 0.0}},
		{ID: "chunk2", FilePath: "test.go", Vector: []float32{0.0, 1.0}},
	}
	err = store.SaveChunks(ctx, chunks)
	if err != nil {
		t.Fatalf("failed to save chunks: %v", err)
	}

	// Delete by file
	err = store.DeleteByFile(ctx, "test.go")
	if err != nil {
		t.Fatalf("failed to delete by file: %v", err)
	}

	// Search should return no results
	results, err := store.Search(ctx, []float32{1.0, 0.0}, 10, SearchOptions{})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results after delete, got %d", len(results))
	}
}

func TestGOBStore_PersistAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.gob")

	// Create and populate store
	store1 := NewGOBStore(indexPath)
	ctx := context.Background()

	chunks := []Chunk{
		{ID: "chunk1", FilePath: "test.go", Content: "test content", Vector: []float32{1.0, 0.0}},
	}
	err := store1.SaveChunks(ctx, chunks)
	if err != nil {
		t.Fatalf("failed to save chunks: %v", err)
	}

	doc := Document{Path: "test.go", Hash: "abc", ChunkIDs: []string{"chunk1"}}
	err = store1.SaveDocument(ctx, doc)
	if err != nil {
		t.Fatalf("failed to save document: %v", err)
	}

	// Persist to disk
	err = store1.Persist(ctx)
	if err != nil {
		t.Fatalf("failed to persist: %v", err)
	}

	// Create new store and load
	store2 := NewGOBStore(indexPath)
	err = store2.Load(ctx)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	// Verify data is loaded
	results, err := store2.Search(ctx, []float32{1.0, 0.0}, 10, SearchOptions{})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	if results[0].Chunk.Content != "test content" {
		t.Errorf("expected content 'test content', got '%s'", results[0].Chunk.Content)
	}
}

func TestGOBStore_PersistCreatesMissingParentDir(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "missing", ".grepai", "index.gob")

	s := NewGOBStore(indexPath)
	if err := s.Persist(context.Background()); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("expected persisted index file at %s: %v", indexPath, err)
	}
}

func TestGOBStore_ListDocuments(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.gob")

	store := NewGOBStore(indexPath)
	ctx := context.Background()

	// Save multiple documents
	docs := []Document{
		{Path: "file1.go", Hash: "a"},
		{Path: "file2.go", Hash: "b"},
		{Path: "file3.go", Hash: "c"},
	}

	for _, doc := range docs {
		err := store.SaveDocument(ctx, doc)
		if err != nil {
			t.Fatalf("failed to save document: %v", err)
		}
	}

	// List documents
	paths, err := store.ListDocuments(ctx)
	if err != nil {
		t.Fatalf("failed to list documents: %v", err)
	}

	if len(paths) != 3 {
		t.Errorf("expected 3 documents, got %d", len(paths))
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
	}{
		{
			name:     "identical vectors",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{1.0, 0.0, 0.0},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{0.0, 1.0, 0.0},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{-1.0, 0.0, 0.0},
			expected: -1.0,
		},
		{
			name:     "different lengths",
			a:        []float32{1.0, 0.0},
			b:        []float32{1.0, 0.0, 0.0},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineSimilarity(tt.a, tt.b)
			if abs(result-tt.expected) > 0.0001 {
				t.Errorf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func TestGOBStore_GetStats(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.gob")

	store := NewGOBStore(indexPath)
	ctx := context.Background()

	// Add some test data
	chunks := []Chunk{
		{ID: "1", FilePath: "file1.go", Content: "test1", UpdatedAt: time.Now()},
		{ID: "2", FilePath: "file1.go", Content: "test2", UpdatedAt: time.Now()},
		{ID: "3", FilePath: "file2.go", Content: "test3", UpdatedAt: time.Now()},
	}
	err := store.SaveChunks(ctx, chunks)
	if err != nil {
		t.Fatalf("failed to save chunks: %v", err)
	}

	err = store.SaveDocument(ctx, Document{Path: "file1.go", ChunkIDs: []string{"1", "2"}})
	if err != nil {
		t.Fatalf("failed to save document: %v", err)
	}
	err = store.SaveDocument(ctx, Document{Path: "file2.go", ChunkIDs: []string{"3"}})
	if err != nil {
		t.Fatalf("failed to save document: %v", err)
	}

	stats, err := store.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalFiles != 2 {
		t.Errorf("expected 2 files, got %d", stats.TotalFiles)
	}
	if stats.TotalChunks != 3 {
		t.Errorf("expected 3 chunks, got %d", stats.TotalChunks)
	}
}

func TestGOBStore_ListFilesWithStats(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.gob")

	store := NewGOBStore(indexPath)
	ctx := context.Background()

	err := store.SaveDocument(ctx, Document{Path: "a.go", ChunkIDs: []string{"1", "2"}})
	if err != nil {
		t.Fatalf("failed to save document: %v", err)
	}
	err = store.SaveDocument(ctx, Document{Path: "b.go", ChunkIDs: []string{"3"}})
	if err != nil {
		t.Fatalf("failed to save document: %v", err)
	}

	files, err := store.ListFilesWithStats(ctx)
	if err != nil {
		t.Fatalf("ListFilesWithStats failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}

	// Check chunk counts
	for _, f := range files {
		if f.Path == "a.go" && f.ChunkCount != 2 {
			t.Errorf("expected 2 chunks for a.go, got %d", f.ChunkCount)
		}
		if f.Path == "b.go" && f.ChunkCount != 1 {
			t.Errorf("expected 1 chunk for b.go, got %d", f.ChunkCount)
		}
	}
}

func TestGOBStore_GetChunksForFile(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.gob")

	store := NewGOBStore(indexPath)
	ctx := context.Background()

	chunks := []Chunk{
		{ID: "1", FilePath: "file.go", StartLine: 1, EndLine: 10, Content: "chunk1"},
		{ID: "2", FilePath: "file.go", StartLine: 11, EndLine: 20, Content: "chunk2"},
	}
	err := store.SaveChunks(ctx, chunks)
	if err != nil {
		t.Fatalf("failed to save chunks: %v", err)
	}

	err = store.SaveDocument(ctx, Document{Path: "file.go", ChunkIDs: []string{"1", "2"}})
	if err != nil {
		t.Fatalf("failed to save document: %v", err)
	}

	result, err := store.GetChunksForFile(ctx, "file.go")
	if err != nil {
		t.Fatalf("GetChunksForFile failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(result))
	}

	// Test non-existent file
	result, err = store.GetChunksForFile(ctx, "nonexistent.go")
	if err != nil {
		t.Fatalf("GetChunksForFile failed: %v", err)
	}
	if result != nil {
		t.Error("expected nil for non-existent file")
	}
}

func TestGOBStore_LookupByContentHash(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "index.gob")
	store := NewGOBStore(indexPath)
	ctx := context.Background()

	// Add chunks with content hashes
	store.SaveChunks(ctx, []Chunk{
		{
			ID:          "chunk-1",
			FilePath:    "main.go",
			Content:     "func main() {}",
			Vector:      []float32{0.1, 0.2, 0.3},
			ContentHash: "abc123",
		},
	})

	// Lookup existing hash
	vec, found, err := store.LookupByContentHash(ctx, "abc123")
	if err != nil {
		t.Fatalf("LookupByContentHash failed: %v", err)
	}
	if !found {
		t.Fatal("Expected to find chunk by content hash")
	}
	if len(vec) != 3 || vec[0] != 0.1 {
		t.Errorf("Unexpected vector: %v", vec)
	}

	// Lookup non-existent hash
	_, found, err = store.LookupByContentHash(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("LookupByContentHash failed: %v", err)
	}
	if found {
		t.Fatal("Expected not found for non-existent hash")
	}
}

func TestGOBStore_FileLocking(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "index.gob")
	ctx := context.Background()

	// Create and populate a store
	s1 := NewGOBStore(indexPath)
	s1.SaveChunks(ctx, []Chunk{
		{ID: "c1", FilePath: "a.go", Content: "hello", Vector: []float32{1, 2, 3}},
	})
	if err := s1.Persist(ctx); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	// Verify lock file was created
	lockPath := indexPath + ".lock"
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatal("Expected lock file to be created")
	}

	// Load from another store instance (simulates concurrent access)
	s2 := NewGOBStore(indexPath)
	if err := s2.Load(ctx); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	chunks, err := s2.GetAllChunks(ctx)
	if err != nil {
		t.Fatalf("GetAllChunks failed: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].ID != "c1" {
		t.Errorf("Expected chunk ID c1, got %s", chunks[0].ID)
	}
}

func TestGOBStore_LoadGarbageIndexRecovers(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.gob")

	if err := os.WriteFile(indexPath, []byte("not a gob file"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	store := NewGOBStore(indexPath)
	if err := store.Load(context.Background()); err != nil {
		t.Fatalf("Load should recover from a corrupted index, got: %v", err)
	}

	numDocs, numChunks := store.Stats()
	if numDocs != 0 || numChunks != 0 {
		t.Fatalf("Expected empty store after recovery, got %d docs / %d chunks", numDocs, numChunks)
	}

	if _, err := os.Stat(indexPath); !os.IsNotExist(err) {
		t.Fatal("Expected corrupted index file to be moved aside")
	}
	if _, err := os.Stat(indexPath + ".corrupt"); err != nil {
		t.Fatalf("Expected quarantined .corrupt file to exist: %v", err)
	}
}

func TestGOBStore_LoadTruncatedIndexRecovers(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.gob")
	ctx := context.Background()

	s1 := NewGOBStore(indexPath)
	if err := s1.SaveChunks(ctx, []Chunk{
		{ID: "c1", FilePath: "a.go", Content: "hello", Vector: []float32{1, 2, 3}},
	}); err != nil {
		t.Fatalf("SaveChunks failed: %v", err)
	}
	if err := s1.Persist(ctx); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	// Simulate a torn index file — what pre-fix versions left behind after an
	// unclean shutdown mid-write (the atomic persist now prevents grepai itself
	// from producing one, but existing installs and external truncation can).
	info, err := os.Stat(indexPath)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if err := os.Truncate(indexPath, info.Size()/2); err != nil {
		t.Fatalf("Truncate failed: %v", err)
	}

	s2 := NewGOBStore(indexPath)
	if err := s2.Load(ctx); err != nil {
		t.Fatalf("Load should recover from a truncated index, got: %v", err)
	}

	numDocs, numChunks := s2.Stats()
	if numDocs != 0 || numChunks != 0 {
		t.Fatalf("Expected empty store after recovery, got %d docs / %d chunks", numDocs, numChunks)
	}

	// A recovered (empty) store must be able to persist and reload cleanly.
	if err := s2.Persist(ctx); err != nil {
		t.Fatalf("Persist after recovery failed: %v", err)
	}
	s3 := NewGOBStore(indexPath)
	if err := s3.Load(ctx); err != nil {
		t.Fatalf("Load after recovery persist failed: %v", err)
	}
}

func TestGOBStore_PersistLeavesNoTempFiles(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.gob")
	ctx := context.Background()

	store := NewGOBStore(indexPath)
	if err := store.SaveChunks(ctx, []Chunk{
		{ID: "c1", FilePath: "a.go", Content: "v1", Vector: []float32{1}},
	}); err != nil {
		t.Fatalf("SaveChunks failed: %v", err)
	}
	if err := store.Persist(ctx); err != nil {
		t.Fatalf("First persist failed: %v", err)
	}

	// Overwrite an existing index (the rename-over path).
	if err := store.SaveChunks(ctx, []Chunk{
		{ID: "c2", FilePath: "b.go", Content: "v2", Vector: []float32{2}},
	}); err != nil {
		t.Fatalf("SaveChunks failed: %v", err)
	}
	if err := store.Persist(ctx); err != nil {
		t.Fatalf("Second persist failed: %v", err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "index.gob" && e.Name() != "index.gob.lock" {
			t.Errorf("Unexpected leftover file after persist: %s", e.Name())
		}
	}

	loaded := NewGOBStore(indexPath)
	if err := loaded.Load(ctx); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if _, numChunks := loaded.Stats(); numChunks != 2 {
		t.Fatalf("Expected 2 chunks after reload, got %d", numChunks)
	}
}

func TestGOBStore_LoadZeroByteIndexRecovers(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.gob")

	// A zero-byte index is the most common artifact of the pre-fix truncation bug.
	if err := os.WriteFile(indexPath, nil, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	store := NewGOBStore(indexPath)
	if err := store.Load(context.Background()); err != nil {
		t.Fatalf("Load should recover from a zero-byte index, got: %v", err)
	}
	if numDocs, numChunks := store.Stats(); numDocs != 0 || numChunks != 0 {
		t.Fatalf("Expected empty store after recovery, got %d docs / %d chunks", numDocs, numChunks)
	}
	if _, err := os.Stat(indexPath + ".corrupt"); err != nil {
		t.Fatalf("Expected quarantined .corrupt file to exist: %v", err)
	}
}

func TestGOBStore_LoadCorruptIndexReplacesStaleQuarantine(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.gob")
	corruptPath := indexPath + ".corrupt"

	if err := os.WriteFile(corruptPath, []byte("stale quarantine"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte("fresh garbage"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	store := NewGOBStore(indexPath)
	if err := store.Load(context.Background()); err != nil {
		t.Fatalf("Load should recover when a stale .corrupt already exists, got: %v", err)
	}

	got, err := os.ReadFile(corruptPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(got) != "fresh garbage" {
		t.Fatalf("Expected quarantine to hold the latest corrupt bytes, got %q", got)
	}
}

func TestGOBStore_ConcurrentLoadsOfCorruptIndexAllRecover(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows denies os.Open on a file another process is renaming
		// (ERROR_SHARING_VIOLATION), so a concurrent reader can fail to open
		// index.gob while the quarantine rename is in flight. Single-reader
		// recovery is covered by TestGOBStore_LoadTruncatedIndexRecovers and
		// TestGOBStore_LoadGarbageIndexRecovers, which both pass on Windows.
		t.Skip("Skipping on Windows: cannot open a file while another process renames it")
	}

	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.gob")

	if err := os.WriteFile(indexPath, []byte("not a gob file"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Load holds only a shared lock, so several readers (watch daemon retry
	// loop + a search / MCP call) can hit the recovery branch at once — the
	// exact issue #178 scenario. Losers of the quarantine rename must adopt
	// the winner's recovery instead of failing, and must not delete the
	// winner's .corrupt evidence.
	const readers = 8
	start := make(chan struct{})
	errs := make(chan error, readers)
	for i := 0; i < readers; i++ {
		go func() {
			<-start
			errs <- NewGOBStore(indexPath).Load(context.Background())
		}()
	}
	close(start)
	for i := 0; i < readers; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("Concurrent Load should recover, got: %v", err)
		}
	}
	if _, err := os.Stat(indexPath + ".corrupt"); err != nil {
		t.Fatalf("Expected .corrupt evidence to survive concurrent recovery: %v", err)
	}
}

func TestGOBStore_FailedPersistPreservesPreviousIndex(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory write-permission semantics differ on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}

	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.gob")
	ctx := context.Background()

	s1 := NewGOBStore(indexPath)
	if err := s1.SaveChunks(ctx, []Chunk{
		{ID: "c1", FilePath: "a.go", Content: "v1", Vector: []float32{1}},
	}); err != nil {
		t.Fatalf("SaveChunks failed: %v", err)
	}
	if err := s1.Persist(ctx); err != nil {
		t.Fatalf("First persist failed: %v", err)
	}

	// Make the directory unwritable so the next persist fails at CreateTemp —
	// the previously persisted index must survive a failed write untouched.
	if err := os.Chmod(tmpDir, 0o555); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(tmpDir, 0o755) })

	if err := s1.SaveChunks(ctx, []Chunk{
		{ID: "c2", FilePath: "b.go", Content: "v2", Vector: []float32{2}},
	}); err != nil {
		t.Fatalf("SaveChunks failed: %v", err)
	}
	if err := s1.Persist(ctx); err == nil {
		t.Fatal("Persist into a read-only directory should fail")
	}

	if err := os.Chmod(tmpDir, 0o755); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}
	s2 := NewGOBStore(indexPath)
	if err := s2.Load(ctx); err != nil {
		t.Fatalf("Load after failed persist failed: %v", err)
	}
	chunks, err := s2.GetAllChunks(ctx)
	if err != nil {
		t.Fatalf("GetAllChunks failed: %v", err)
	}
	if len(chunks) != 1 || chunks[0].ID != "c1" {
		t.Fatalf("Expected only the pre-failure chunk c1 to survive, got %+v", chunks)
	}
}
