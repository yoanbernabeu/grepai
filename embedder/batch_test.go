package embedder

import (
	"testing"
)

func TestBatchSize(t *testing.T) {
	batch := Batch{
		Entries: []BatchEntry{
			{FileIndex: 0, ChunkIndex: 0, Content: "chunk1"},
			{FileIndex: 0, ChunkIndex: 1, Content: "chunk2"},
			{FileIndex: 1, ChunkIndex: 0, Content: "chunk3"},
		},
		Index: 0,
	}

	if batch.Size() != 3 {
		t.Errorf("expected Size() = 3, got %d", batch.Size())
	}
}

func TestBatchContents(t *testing.T) {
	batch := Batch{
		Entries: []BatchEntry{
			{FileIndex: 0, ChunkIndex: 0, Content: "hello"},
			{FileIndex: 0, ChunkIndex: 1, Content: "world"},
			{FileIndex: 1, ChunkIndex: 0, Content: "foo"},
		},
		Index: 0,
	}

	contents := batch.Contents()
	expected := []string{"hello", "world", "foo"}

	if len(contents) != len(expected) {
		t.Fatalf("expected %d contents, got %d", len(expected), len(contents))
	}

	for i, c := range contents {
		if c != expected[i] {
			t.Errorf("contents[%d] = %q, expected %q", i, c, expected[i])
		}
	}
}

func TestFormBatches_EmptyInput(t *testing.T) {
	// Empty files slice
	batches := FormBatches(nil)
	if batches != nil {
		t.Errorf("expected nil batches for nil input, got %v", batches)
	}

	batches = FormBatches([]FileChunks{})
	if batches != nil {
		t.Errorf("expected nil batches for empty input, got %v", batches)
	}

	// Files with no chunks
	batches = FormBatches([]FileChunks{
		{FileIndex: 0, Chunks: []string{}},
		{FileIndex: 1, Chunks: nil},
	})
	if batches != nil {
		t.Errorf("expected nil batches for files with no chunks, got %v", batches)
	}
}

func TestFormBatches_SingleFileFewChunks(t *testing.T) {
	files := []FileChunks{
		{FileIndex: 0, Chunks: []string{"chunk1", "chunk2", "chunk3"}},
	}

	batches := FormBatches(files)

	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}

	batch := batches[0]
	if batch.Index != 0 {
		t.Errorf("expected batch.Index = 0, got %d", batch.Index)
	}
	if len(batch.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(batch.Entries))
	}

	// Verify entries maintain correct file/chunk indices
	for i, entry := range batch.Entries {
		if entry.FileIndex != 0 {
			t.Errorf("entry[%d].FileIndex = %d, expected 0", i, entry.FileIndex)
		}
		if entry.ChunkIndex != i {
			t.Errorf("entry[%d].ChunkIndex = %d, expected %d", i, entry.ChunkIndex, i)
		}
	}
}

func TestFormBatches_SingleFileManyChunks(t *testing.T) {
	// Create file with more than MaxBatchSize chunks
	chunks := make([]string, MaxBatchSize+500)
	for i := range chunks {
		chunks[i] = "chunk"
	}

	files := []FileChunks{
		{FileIndex: 0, Chunks: chunks},
	}

	batches := FormBatches(files)

	if len(batches) != 2 {
		t.Fatalf("expected 2 batches for %d chunks, got %d", len(chunks), len(batches))
	}

	// First batch should be full
	if len(batches[0].Entries) != MaxBatchSize {
		t.Errorf("first batch should have %d entries, got %d", MaxBatchSize, len(batches[0].Entries))
	}
	if batches[0].Index != 0 {
		t.Errorf("first batch.Index = %d, expected 0", batches[0].Index)
	}

	// Second batch should have remaining
	if len(batches[1].Entries) != 500 {
		t.Errorf("second batch should have 500 entries, got %d", len(batches[1].Entries))
	}
	if batches[1].Index != 1 {
		t.Errorf("second batch.Index = %d, expected 1", batches[1].Index)
	}

	// Verify chunk indices are correct across batches
	for i, entry := range batches[0].Entries {
		if entry.ChunkIndex != i {
			t.Errorf("batch[0].entry[%d].ChunkIndex = %d, expected %d", i, entry.ChunkIndex, i)
		}
	}
	for i, entry := range batches[1].Entries {
		expectedIdx := MaxBatchSize + i
		if entry.ChunkIndex != expectedIdx {
			t.Errorf("batch[1].entry[%d].ChunkIndex = %d, expected %d", i, entry.ChunkIndex, expectedIdx)
		}
	}
}

func TestFormBatches_MultipleFilesCombined(t *testing.T) {
	files := []FileChunks{
		{FileIndex: 0, Chunks: []string{"file0-chunk0", "file0-chunk1"}},
		{FileIndex: 1, Chunks: []string{"file1-chunk0", "file1-chunk1", "file1-chunk2"}},
		{FileIndex: 2, Chunks: []string{"file2-chunk0"}},
	}

	batches := FormBatches(files)

	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}

	batch := batches[0]
	if len(batch.Entries) != 6 {
		t.Fatalf("expected 6 entries, got %d", len(batch.Entries))
	}

	// Verify file/chunk index tracking
	expected := []struct {
		fileIdx  int
		chunkIdx int
		content  string
	}{
		{0, 0, "file0-chunk0"},
		{0, 1, "file0-chunk1"},
		{1, 0, "file1-chunk0"},
		{1, 1, "file1-chunk1"},
		{1, 2, "file1-chunk2"},
		{2, 0, "file2-chunk0"},
	}

	for i, exp := range expected {
		entry := batch.Entries[i]
		if entry.FileIndex != exp.fileIdx {
			t.Errorf("entry[%d].FileIndex = %d, expected %d", i, entry.FileIndex, exp.fileIdx)
		}
		if entry.ChunkIndex != exp.chunkIdx {
			t.Errorf("entry[%d].ChunkIndex = %d, expected %d", i, entry.ChunkIndex, exp.chunkIdx)
		}
		if entry.Content != exp.content {
			t.Errorf("entry[%d].Content = %q, expected %q", i, entry.Content, exp.content)
		}
	}
}

func TestFormBatches_MultipleFilesBatchBoundary(t *testing.T) {
	// Create files that will span batch boundaries
	file1Chunks := make([]string, MaxBatchSize-100)
	for i := range file1Chunks {
		file1Chunks[i] = "file1"
	}
	file2Chunks := make([]string, 200) // This will cross the boundary
	for i := range file2Chunks {
		file2Chunks[i] = "file2"
	}

	files := []FileChunks{
		{FileIndex: 0, Chunks: file1Chunks},
		{FileIndex: 1, Chunks: file2Chunks},
	}

	batches := FormBatches(files)

	if len(batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(batches))
	}

	// First batch: all of file1 (1900) + first 100 of file2
	if len(batches[0].Entries) != MaxBatchSize {
		t.Errorf("first batch should have %d entries, got %d", MaxBatchSize, len(batches[0].Entries))
	}

	// Second batch: remaining 100 of file2
	if len(batches[1].Entries) != 100 {
		t.Errorf("second batch should have 100 entries, got %d", len(batches[1].Entries))
	}

	// Verify file indices in second batch are correct
	for _, entry := range batches[1].Entries {
		if entry.FileIndex != 1 {
			t.Errorf("second batch entry.FileIndex = %d, expected 1", entry.FileIndex)
		}
	}

	// Verify chunk indices for file2 are preserved across batches
	file2InBatch1 := 0
	for _, entry := range batches[0].Entries {
		if entry.FileIndex == 1 {
			if entry.ChunkIndex != file2InBatch1 {
				t.Errorf("file2 chunk in batch1: ChunkIndex = %d, expected %d", entry.ChunkIndex, file2InBatch1)
			}
			file2InBatch1++
		}
	}

	for i, entry := range batches[1].Entries {
		expectedChunkIdx := file2InBatch1 + i
		if entry.ChunkIndex != expectedChunkIdx {
			t.Errorf("file2 chunk in batch2[%d]: ChunkIndex = %d, expected %d", i, entry.ChunkIndex, expectedChunkIdx)
		}
	}
}

func TestFormBatches_ExactlyMaxBatchSize(t *testing.T) {
	chunks := make([]string, MaxBatchSize)
	for i := range chunks {
		chunks[i] = "chunk"
	}

	files := []FileChunks{
		{FileIndex: 0, Chunks: chunks},
	}

	batches := FormBatches(files)

	if len(batches) != 1 {
		t.Errorf("expected 1 batch for exactly %d chunks, got %d", MaxBatchSize, len(batches))
	}
	if len(batches[0].Entries) != MaxBatchSize {
		t.Errorf("batch should have %d entries, got %d", MaxBatchSize, len(batches[0].Entries))
	}
}

func TestFormBatches_ExactlyMaxBatchSizePlusOne(t *testing.T) {
	chunks := make([]string, MaxBatchSize+1)
	for i := range chunks {
		chunks[i] = "chunk"
	}

	files := []FileChunks{
		{FileIndex: 0, Chunks: chunks},
	}

	batches := FormBatches(files)

	if len(batches) != 2 {
		t.Errorf("expected 2 batches for %d chunks, got %d", MaxBatchSize+1, len(batches))
	}
	if len(batches[0].Entries) != MaxBatchSize {
		t.Errorf("first batch should have %d entries, got %d", MaxBatchSize, len(batches[0].Entries))
	}
	if len(batches[1].Entries) != 1 {
		t.Errorf("second batch should have 1 entry, got %d", len(batches[1].Entries))
	}
}

func TestMapResultsToFiles(t *testing.T) {
	batches := []Batch{
		{
			Index: 0,
			Entries: []BatchEntry{
				{FileIndex: 0, ChunkIndex: 0, Content: "f0c0"},
				{FileIndex: 0, ChunkIndex: 1, Content: "f0c1"},
				{FileIndex: 1, ChunkIndex: 0, Content: "f1c0"},
			},
		},
		{
			Index: 1,
			Entries: []BatchEntry{
				{FileIndex: 1, ChunkIndex: 1, Content: "f1c1"},
				{FileIndex: 2, ChunkIndex: 0, Content: "f2c0"},
			},
		},
	}

	results := []BatchResult{
		{
			BatchIndex: 0,
			Embeddings: [][]float32{
				{0.1, 0.2}, // f0c0
				{0.3, 0.4}, // f0c1
				{0.5, 0.6}, // f1c0
			},
		},
		{
			BatchIndex: 1,
			Embeddings: [][]float32{
				{0.7, 0.8}, // f1c1
				{0.9, 1.0}, // f2c0
			},
		},
	}

	fileEmbeddings := MapResultsToFiles(batches, results, 3)

	if len(fileEmbeddings) != 3 {
		t.Fatalf("expected 3 files, got %d", len(fileEmbeddings))
	}

	// File 0: 2 chunks
	if len(fileEmbeddings[0]) != 2 {
		t.Errorf("file 0 should have 2 chunks, got %d", len(fileEmbeddings[0]))
	}
	assertEmbedding(t, fileEmbeddings[0][0], []float32{0.1, 0.2}, "file0-chunk0")
	assertEmbedding(t, fileEmbeddings[0][1], []float32{0.3, 0.4}, "file0-chunk1")

	// File 1: 2 chunks
	if len(fileEmbeddings[1]) != 2 {
		t.Errorf("file 1 should have 2 chunks, got %d", len(fileEmbeddings[1]))
	}
	assertEmbedding(t, fileEmbeddings[1][0], []float32{0.5, 0.6}, "file1-chunk0")
	assertEmbedding(t, fileEmbeddings[1][1], []float32{0.7, 0.8}, "file1-chunk1")

	// File 2: 1 chunk
	if len(fileEmbeddings[2]) != 1 {
		t.Errorf("file 2 should have 1 chunk, got %d", len(fileEmbeddings[2]))
	}
	assertEmbedding(t, fileEmbeddings[2][0], []float32{0.9, 1.0}, "file2-chunk0")
}

func assertEmbedding(t *testing.T, got, expected []float32, desc string) {
	t.Helper()
	if len(got) != len(expected) {
		t.Errorf("%s: expected embedding len %d, got %d", desc, len(expected), len(got))
		return
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("%s: expected embedding[%d] = %f, got %f", desc, i, expected[i], got[i])
		}
	}
}

func TestMapResultsToFiles_EmptyInput(t *testing.T) {
	fileEmbeddings := MapResultsToFiles(nil, nil, 0)
	if len(fileEmbeddings) != 0 {
		t.Errorf("expected 0 files for empty input, got %d", len(fileEmbeddings))
	}

	fileEmbeddings = MapResultsToFiles([]Batch{}, []BatchResult{}, 3)
	if len(fileEmbeddings) != 3 {
		t.Errorf("expected 3 files, got %d", len(fileEmbeddings))
	}
	for i, fe := range fileEmbeddings {
		if fe != nil {
			t.Errorf("file %d should have nil embeddings, got %v", i, fe)
		}
	}
}
