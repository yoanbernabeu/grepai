package indexer

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/store"
)

// mockStore implements store.VectorStore for testing
type mockStore struct {
	documents        map[string]store.Document
	chunks           map[string]store.Chunk
	listFilesStats   []store.FileStats
	listDocsCalled   bool
	getDocCalled     bool
	saveDocCalled    bool
	saveChunksCalled bool
	delByFileCalled  bool
	delDocCalled     bool
}

func newMockStore() *mockStore {
	return &mockStore{
		documents: make(map[string]store.Document),
		chunks:    make(map[string]store.Chunk),
	}
}

func (m *mockStore) SaveChunks(ctx context.Context, chunks []store.Chunk) error {
	m.saveChunksCalled = true
	for _, chunk := range chunks {
		m.chunks[chunk.ID] = chunk
	}
	return nil
}

func (m *mockStore) DeleteByFile(ctx context.Context, filePath string) error {
	m.delByFileCalled = true
	doc, ok := m.documents[filePath]
	if !ok {
		return nil
	}
	for _, chunkID := range doc.ChunkIDs {
		delete(m.chunks, chunkID)
	}
	return nil
}

func (m *mockStore) Search(ctx context.Context, queryVector []float32, limit int) ([]store.SearchResult, error) {
	results := make([]store.SearchResult, 0, len(m.chunks))
	for _, chunk := range m.chunks {
		results = append(results, store.SearchResult{
			Chunk: chunk,
			Score: 1.0,
		})
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (m *mockStore) GetDocument(ctx context.Context, filePath string) (*store.Document, error) {
	m.getDocCalled = true
	doc, ok := m.documents[filePath]
	if !ok {
		return nil, nil
	}
	return &doc, nil
}

func (m *mockStore) SaveDocument(ctx context.Context, doc store.Document) error {
	m.saveDocCalled = true
	m.documents[doc.Path] = doc
	return nil
}

func (m *mockStore) DeleteDocument(ctx context.Context, filePath string) error {
	m.delDocCalled = true
	delete(m.documents, filePath)
	return nil
}

func (m *mockStore) ListDocuments(ctx context.Context) ([]string, error) {
	m.listDocsCalled = true
	paths := make([]string, 0, len(m.documents))
	for path := range m.documents {
		paths = append(paths, path)
	}
	return paths, nil
}

func (m *mockStore) Load(ctx context.Context) error {
	return nil
}

func (m *mockStore) Persist(ctx context.Context) error {
	return nil
}

func (m *mockStore) Close() error {
	return nil
}

func (m *mockStore) GetStats(ctx context.Context) (*store.IndexStats, error) {
	return &store.IndexStats{
		TotalFiles:  len(m.documents),
		TotalChunks: len(m.chunks),
	}, nil
}

func (m *mockStore) ListFilesWithStats(ctx context.Context) ([]store.FileStats, error) {
	stats := make([]store.FileStats, 0, len(m.documents))
	for _, doc := range m.documents {
		stats = append(stats, store.FileStats{
			Path:       doc.Path,
			ChunkCount: len(doc.ChunkIDs),
			ModTime:    doc.ModTime,
		})
	}
	// If listFilesStats is set, use that instead (for testing)
	if len(m.listFilesStats) > 0 {
		return m.listFilesStats, nil
	}
	return stats, nil
}

func (m *mockStore) GetChunksForFile(ctx context.Context, filePath string) ([]store.Chunk, error) {
	doc, ok := m.documents[filePath]
	if !ok {
		return nil, nil
	}
	chunks := make([]store.Chunk, 0, len(doc.ChunkIDs))
	for _, id := range doc.ChunkIDs {
		if chunk, ok := m.chunks[id]; ok {
			chunks = append(chunks, chunk)
		}
	}
	return chunks, nil
}

func (m *mockStore) GetAllChunks(ctx context.Context) ([]store.Chunk, error) {
	chunks := make([]store.Chunk, 0, len(m.chunks))
	for _, chunk := range m.chunks {
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

// mockEmbedder implements embedder.Embedder for testing
type mockEmbedder struct {
	embedCalled bool
}

func newMockEmbedder() *mockEmbedder {
	return &mockEmbedder{}
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	m.embedCalled = true
	return []float32{0.1, 0.2, 0.3}, nil
}

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	m.embedCalled = true
	vectors := make([][]float32, len(texts))
	for i := range texts {
		vectors[i] = []float32{0.1, 0.2, 0.3}
	}
	return vectors, nil
}

func (m *mockEmbedder) Dimensions() int {
	return 3
}

func (m *mockEmbedder) Close() error {
	return nil
}

// TestIndexAllWithProgress_UnchangedFilesSkipped tests that files with matching ModTimes are skipped
func TestIndexAllWithProgress_UnchangedFilesSkipped(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.go")
	content := "package main\n\nfunc main() {}"
	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Get file ModTime
	fileInfo, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("failed to get file info: %v", err)
	}
	fileModTime := time.Unix(fileInfo.ModTime().Unix(), 0)

	// Create mock store with existing file that has matching ModTime
	mockStore := newMockStore()
	mockStore.documents["test.go"] = store.Document{
		Path:     "test.go",
		Hash:     "hash123",
		ModTime:  fileModTime,
		ChunkIDs: []string{"chunk1"},
	}

	// Create indexer with lastIndexTime set to now to enable ModTime-based skipping
	mockEmbedder := newMockEmbedder()
	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}
	scanner := NewScanner(tmpDir, ignoreMatcher)
	chunker := NewChunker(512, 50)
	indexer := NewIndexer(tmpDir, mockStore, mockEmbedder, chunker, scanner, time.Now())

	// Index with progress
	stats, err := indexer.IndexAllWithProgress(context.Background(), nil)
	if err != nil {
		t.Fatalf("IndexAllWithProgress failed: %v", err)
	}

	// Assert: No files should be indexed (all skipped by ModTime)
	if stats.FilesIndexed != 0 {
		t.Errorf("expected 0 files indexed (ModTime match), got %d", stats.FilesIndexed)
	}

	// Assert: No chunks should be created
	if stats.ChunksCreated != 0 {
		t.Errorf("expected 0 chunks created, got %d", stats.ChunksCreated)
	}

	// Assert: No documents should be retrieved (skipped before GetDocument call)
	if mockStore.getDocCalled {
		t.Error("GetDocument should not be called for files with matching ModTime")
	}

	// Assert: No documents should be saved
	if mockStore.saveDocCalled {
		t.Error("SaveDocument should not be called for unchanged files")
	}
}

// TestIndexAllWithProgress_ChangedFilesIndexed tests that files with different ModTimes are re-indexed
func TestIndexAllWithProgress_ChangedFilesIndexed(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.go")
	content := "package main\n\nfunc main() {}"
	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Get current file ModTime
	fileInfo, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("failed to get file info: %v", err)
	}
	currentModTime := time.Unix(fileInfo.ModTime().Unix(), 0)

	// Create mock store with OLD ModTime (1 hour ago)
	oldModTime := currentModTime.Add(-1 * time.Hour)
	mockStore := newMockStore()
	mockStore.documents["test.go"] = store.Document{
		Path:     "test.go",
		Hash:     "oldHash",
		ModTime:  oldModTime,
		ChunkIDs: []string{"oldChunk1"},
	}

	// Create indexer
	mockEmbedder := newMockEmbedder()
	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}
	scanner := NewScanner(tmpDir, ignoreMatcher)
	chunker := NewChunker(512, 50)
	indexer := NewIndexer(tmpDir, mockStore, mockEmbedder, chunker, scanner, time.Time{})

	// Index with progress
	stats, err := indexer.IndexAllWithProgress(context.Background(), nil)
	if err != nil {
		t.Fatalf("IndexAllWithProgress failed: %v", err)
	}

	// Assert: File should be indexed (ModTime differs)
	if stats.FilesIndexed != 1 {
		t.Errorf("expected 1 file indexed (ModTime changed), got %d", stats.FilesIndexed)
	}

	// Assert: Chunks should be created
	if stats.ChunksCreated == 0 {
		t.Error("expected chunks to be created for changed file")
	}

	// Assert: Embedder should be called
	if !mockEmbedder.embedCalled {
		t.Error("EmbedBatch should be called for changed file")
	}

	// Assert: Document should be saved with new ModTime
	if !mockStore.saveDocCalled {
		t.Error("SaveDocument should be called for changed file")
	}

	savedDoc, ok := mockStore.documents["test.go"]
	if !ok {
		t.Error("document should exist in store")
	} else {
		expectedModTime := time.Unix(currentModTime.Unix(), 0)
		if !savedDoc.ModTime.Equal(expectedModTime) {
			t.Errorf("expected ModTime %v, got %v", expectedModTime, savedDoc.ModTime)
		}
	}
}

// TestIndexAllWithProgress_NewFilesIndexed tests that new files are indexed
func TestIndexAllWithProgress_NewFilesIndexed(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "newfile.go")
	content := "package main\n\nfunc main() {}"
	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create mock store WITHOUT the new file (empty store)
	mockStore := newMockStore()

	// Create indexer
	mockEmbedder := newMockEmbedder()
	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}
	scanner := NewScanner(tmpDir, ignoreMatcher)
	chunker := NewChunker(512, 50)
	indexer := NewIndexer(tmpDir, mockStore, mockEmbedder, chunker, scanner, time.Time{})

	// Index with progress
	stats, err := indexer.IndexAllWithProgress(context.Background(), nil)
	if err != nil {
		t.Fatalf("IndexAllWithProgress failed: %v", err)
	}

	// Assert: New file should be indexed
	if stats.FilesIndexed != 1 {
		t.Errorf("expected 1 file indexed (new file), got %d", stats.FilesIndexed)
	}

	// Assert: Chunks should be created
	if stats.ChunksCreated == 0 {
		t.Error("expected chunks to be created for new file")
	}

	// Assert: Document should be saved
	if !mockStore.saveDocCalled {
		t.Error("SaveDocument should be called for new file")
	}

	// Verify the document exists
	_, ok := mockStore.documents["newfile.go"]
	if !ok {
		t.Error("document should exist in store")
	}
}

// TestIndexAllWithProgress_DeletedFilesRemoved tests that deleted files are removed from index
func TestIndexAllWithProgress_DeletedFilesRemoved(t *testing.T) {
	tmpDir := t.TempDir()

	// Create only file A (file B is deleted)
	fileA := filepath.Join(tmpDir, "fileA.go")
	contentA := "package main\n\nfunc A() {}"
	err := os.WriteFile(fileA, []byte(contentA), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create mock store with files A and B
	mockStore := newMockStore()
	mockStore.documents["fileA.go"] = store.Document{
		Path:     "fileA.go",
		Hash:     "hashA",
		ModTime:  time.Now().Add(-1 * time.Hour),
		ChunkIDs: []string{"chunkA"},
	}
	mockStore.documents["fileB.go"] = store.Document{
		Path:     "fileB.go",
		Hash:     "hashB",
		ModTime:  time.Now().Add(-1 * time.Hour),
		ChunkIDs: []string{"chunkB"},
	}
	// Add chunks so DeleteByFile can find them
	mockStore.chunks["chunkA"] = store.Chunk{ID: "chunkA"}
	mockStore.chunks["chunkB"] = store.Chunk{ID: "chunkB"}

	// Create indexer
	mockEmbedder := newMockEmbedder()
	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}
	scanner := NewScanner(tmpDir, ignoreMatcher)
	chunker := NewChunker(512, 50)
	indexer := NewIndexer(tmpDir, mockStore, mockEmbedder, chunker, scanner, time.Time{})

	// Index with progress
	stats, err := indexer.IndexAllWithProgress(context.Background(), nil)
	if err != nil {
		t.Fatalf("IndexAllWithProgress failed: %v", err)
	}

	// Assert: One file should be removed (fileB)
	if stats.FilesRemoved != 1 {
		t.Errorf("expected 1 file removed (fileB deleted), got %d", stats.FilesRemoved)
	}

	// Assert: fileB should be deleted from store
	if _, ok := mockStore.documents["fileB.go"]; ok {
		t.Error("fileB.go should be deleted from store")
	}

	// Assert: fileB's chunks should be deleted
	if _, ok := mockStore.chunks["chunkB"]; ok {
		t.Error("chunkB should be deleted from store")
	}

	// Assert: fileA should still exist
	if _, ok := mockStore.documents["fileA.go"]; !ok {
		t.Error("fileA.go should still exist in store")
	}
}

// mockBatchEmbedder implements embedder.BatchEmbedder for testing progress tracking
type mockBatchEmbedder struct {
	embedCalled bool
	delay       time.Duration // Optional delay per batch for testing concurrency
}

func newMockBatchEmbedder() *mockBatchEmbedder {
	return &mockBatchEmbedder{}
}

func (m *mockBatchEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	m.embedCalled = true
	return []float32{0.1, 0.2, 0.3}, nil
}

func (m *mockBatchEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	m.embedCalled = true
	vectors := make([][]float32, len(texts))
	for i := range texts {
		vectors[i] = []float32{0.1, 0.2, 0.3}
	}
	return vectors, nil
}

func (m *mockBatchEmbedder) Dimensions() int {
	return 3
}

func (m *mockBatchEmbedder) Close() error {
	return nil
}

func (m *mockBatchEmbedder) EmbedBatches(ctx context.Context, batches []embedder.Batch, progress embedder.BatchProgress) ([]embedder.BatchResult, error) {
	m.embedCalled = true

	// Calculate total chunks for progress reporting
	totalChunks := 0
	for _, batch := range batches {
		totalChunks += batch.Size()
	}

	var completedChunks int
	results := make([]embedder.BatchResult, len(batches))

	for _, batch := range batches {
		if m.delay > 0 {
			time.Sleep(m.delay)
		}

		// Create mock embeddings
		embeddings := make([][]float32, batch.Size())
		for i := range embeddings {
			embeddings[i] = []float32{0.1, 0.2, 0.3}
		}

		completedChunks += batch.Size()

		// Report progress
		if progress != nil {
			progress(batch.Index, len(batches), completedChunks, totalChunks, false, 0)
		}

		results[batch.Index] = embedder.BatchResult{
			BatchIndex: batch.Index,
			Embeddings: embeddings,
		}
	}

	return results, nil
}

// TestProgressTracking_AccurateChunkProgress tests that progress accurately reflects chunk completion
func TestProgressTracking_AccurateChunkProgress(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple test files to generate multiple chunks
	for i := 0; i < 3; i++ {
		testFile := filepath.Join(tmpDir, "file"+string(rune('a'+i))+".go")
		// Create content that will generate multiple chunks
		content := "package main\n\n// This is a test file with multiple lines\nfunc main() {\n\t// Line 1\n\t// Line 2\n\t// Line 3\n}"
		if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	mockStore := newMockStore()
	mockEmb := newMockBatchEmbedder()
	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}
	scanner := NewScanner(tmpDir, ignoreMatcher)
	chunker := NewChunker(512, 50)
	indexer := NewIndexer(tmpDir, mockStore, mockEmb, chunker, scanner, time.Time{})

	// Track progress updates
	var progressUpdates []BatchProgressInfo
	var mu sync.Mutex

	_, err = indexer.IndexAllWithBatchProgress(context.Background(), nil,
		func(info BatchProgressInfo) {
			mu.Lock()
			progressUpdates = append(progressUpdates, info)
			mu.Unlock()
		})
	if err != nil {
		t.Fatalf("IndexAllWithBatchProgress failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// Verify progress was reported
	if len(progressUpdates) == 0 {
		t.Fatal("expected progress updates, got none")
	}

	// Verify final progress shows 100%
	finalProgress := progressUpdates[len(progressUpdates)-1]
	if finalProgress.CompletedChunks != finalProgress.TotalChunks {
		t.Errorf("final progress should show all chunks completed: got %d/%d",
			finalProgress.CompletedChunks, finalProgress.TotalChunks)
	}

	// Verify total chunks is accurate
	if finalProgress.TotalChunks == 0 {
		t.Error("total chunks should be greater than 0")
	}
}

// TestProgressTracking_MonotonicallyIncreasing tests that progress is monotonically increasing
func TestProgressTracking_MonotonicallyIncreasing(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple test files
	for i := 0; i < 5; i++ {
		testFile := filepath.Join(tmpDir, "file"+string(rune('a'+i))+".go")
		content := "package main\n\nfunc main() { /* chunk content */ }"
		if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	mockStore := newMockStore()
	mockEmb := newMockBatchEmbedder()
	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}
	scanner := NewScanner(tmpDir, ignoreMatcher)
	chunker := NewChunker(512, 50)
	indexer := NewIndexer(tmpDir, mockStore, mockEmb, chunker, scanner, time.Time{})

	// Track progress updates
	var completedChunksCounts []int
	var mu sync.Mutex

	_, err = indexer.IndexAllWithBatchProgress(context.Background(), nil,
		func(info BatchProgressInfo) {
			mu.Lock()
			completedChunksCounts = append(completedChunksCounts, info.CompletedChunks)
			mu.Unlock()
		})
	if err != nil {
		t.Fatalf("IndexAllWithBatchProgress failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// Verify progress is monotonically increasing
	for i := 1; i < len(completedChunksCounts); i++ {
		if completedChunksCounts[i] < completedChunksCounts[i-1] {
			t.Errorf("progress decreased: %d at index %d < %d at index %d",
				completedChunksCounts[i], i, completedChunksCounts[i-1], i-1)
		}
	}
}

// TestProgressTracking_ConcurrentBatches tests that concurrent batch completion doesn't cause race conditions
func TestProgressTracking_ConcurrentBatches(t *testing.T) {
	tmpDir := t.TempDir()

	// Create many test files to generate multiple batches
	for i := 0; i < 10; i++ {
		testFile := filepath.Join(tmpDir, "file"+string(rune('a'+i))+".go")
		content := "package main\n\nfunc main() { /* test content */ }"
		if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	mockStore := newMockStore()
	mockEmb := newMockBatchEmbedder()
	mockEmb.delay = 10 * time.Millisecond // Add delay to create concurrency opportunity

	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}
	scanner := NewScanner(tmpDir, ignoreMatcher)
	chunker := NewChunker(512, 50)
	indexer := NewIndexer(tmpDir, mockStore, mockEmb, chunker, scanner, time.Time{})

	// Track progress updates with atomic counter
	var progressCallCount atomic.Int32
	var maxCompleted atomic.Int32
	var totalChunksReported atomic.Int32

	_, err = indexer.IndexAllWithBatchProgress(context.Background(), nil,
		func(info BatchProgressInfo) {
			progressCallCount.Add(1)

			// Track max completed and total
			if int32(info.CompletedChunks) > maxCompleted.Load() {
				maxCompleted.Store(int32(info.CompletedChunks))
			}
			totalChunksReported.Store(int32(info.TotalChunks))
		})
	if err != nil {
		t.Fatalf("IndexAllWithBatchProgress failed: %v", err)
	}

	// Verify progress was reported
	if progressCallCount.Load() == 0 {
		t.Fatal("expected progress updates, got none")
	}

	// Verify final max equals total (all chunks completed)
	if maxCompleted.Load() != totalChunksReported.Load() {
		t.Errorf("max completed %d should equal total chunks %d",
			maxCompleted.Load(), totalChunksReported.Load())
	}
}

// TestProgressTracking_NoFilesToIndex tests that progress works correctly when there are no files to index
func TestProgressTracking_NoFilesToIndex(t *testing.T) {
	tmpDir := t.TempDir()

	// Create empty directory (no files)
	mockStore := newMockStore()
	mockEmb := newMockBatchEmbedder()
	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}
	scanner := NewScanner(tmpDir, ignoreMatcher)
	chunker := NewChunker(512, 50)
	indexer := NewIndexer(tmpDir, mockStore, mockEmb, chunker, scanner, time.Time{})

	var progressCalled bool

	_, err = indexer.IndexAllWithBatchProgress(context.Background(), nil,
		func(info BatchProgressInfo) {
			progressCalled = true
		})
	if err != nil {
		t.Fatalf("IndexAllWithBatchProgress failed: %v", err)
	}

	// No progress callback should be called when there are no files
	// (batch embedder is not used when there are no chunks)
	if progressCalled {
		t.Error("progress should not be called when there are no files to index")
	}
}

// TestTimeEqualBehavior verifies time.Equal handles precision correctly
func TestTimeEqualBehavior(t *testing.T) {
	tests := []struct {
		name     string
		t1       time.Time
		t2       time.Time
		expected bool
	}{
		{
			name:     "Equal timestamps",
			t1:       time.Unix(1640995200, 0),
			t2:       time.Unix(1640995200, 0),
			expected: true,
		},
		{
			name:     "1 second difference",
			t1:       time.Unix(1640995201, 0),
			t2:       time.Unix(1640995200, 0),
			expected: false,
		},
		{
			name:     "Sub-second difference",
			t1:       time.Unix(1640995200, 500000000),
			t2:       time.Unix(1640995200, 0),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.t1.Equal(tt.t2)
			if result != tt.expected {
				t.Errorf("time.Equal(%v, %v) = %v, expected %v", tt.t1, tt.t2, result, tt.expected)
			}
		})
	}
}
