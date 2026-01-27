package indexer

import (
	"strings"
	"testing"
)

func TestChunker_Chunk(t *testing.T) {
	chunker := NewChunker(100, 10) // Small chunks for testing

	content := strings.Repeat("line of code\n", 50)
	chunks := chunker.Chunk("test.go", content)

	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}

	// Verify chunk properties
	for i, chunk := range chunks {
		if chunk.ID == "" {
			t.Errorf("chunk %d has empty ID", i)
		}
		if chunk.FilePath != "test.go" {
			t.Errorf("chunk %d has wrong file path: %s", i, chunk.FilePath)
		}
		if chunk.StartLine < 1 {
			t.Errorf("chunk %d has invalid start line: %d", i, chunk.StartLine)
		}
		if chunk.EndLine < chunk.StartLine {
			t.Errorf("chunk %d has end line before start line", i)
		}
		if chunk.Content == "" {
			t.Errorf("chunk %d has empty content", i)
		}
	}
}

func TestChunker_ChunkEmptyContent(t *testing.T) {
	chunker := NewChunker(512, 50)
	chunks := chunker.Chunk("empty.go", "")

	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty content, got %d", len(chunks))
	}
}

func TestChunker_ChunkWhitespaceOnly(t *testing.T) {
	chunker := NewChunker(512, 50)
	chunks := chunker.Chunk("whitespace.go", "   \n\n\t\t\n   ")

	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for whitespace-only content, got %d", len(chunks))
	}
}

func TestChunker_ChunkWithContext(t *testing.T) {
	chunker := NewChunker(512, 50)
	chunks := chunker.ChunkWithContext("myfile.go", "package main\n\nfunc main() {}")

	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}

	// Verify context is added
	if !strings.Contains(chunks[0].Content, "File: myfile.go") {
		t.Error("chunk should contain file path context")
	}
}

func TestChunker_DefaultValues(t *testing.T) {
	// Test with invalid values
	chunker := NewChunker(0, -1)

	if chunker.chunkSize != DefaultChunkSize {
		t.Errorf("expected default chunk size %d, got %d", DefaultChunkSize, chunker.chunkSize)
	}

	if chunker.overlap != DefaultChunkOverlap {
		t.Errorf("expected default overlap %d, got %d", DefaultChunkOverlap, chunker.overlap)
	}
}

func TestChunker_OverlapTooLarge(t *testing.T) {
	// Overlap >= chunk size should be reduced
	chunker := NewChunker(100, 150)

	if chunker.overlap >= chunker.chunkSize {
		t.Error("overlap should be less than chunk size")
	}
}

func TestChunker_MinifiedFile(t *testing.T) {
	chunker := NewChunker(512, 50)

	// Simulate a minified file: single line of 50KB
	minifiedContent := strings.Repeat("a", 50000)

	chunks := chunker.Chunk("jquery.min.js", minifiedContent)

	// Should create multiple chunks
	if len(chunks) < 10 {
		t.Errorf("expected many chunks for large minified file, got %d", len(chunks))
	}

	// Each chunk should respect the size limit
	maxChars := 512 * CharsPerToken
	for i, chunk := range chunks {
		if len(chunk.Content) > maxChars+100 {
			t.Errorf("chunk %d exceeds max size: %d chars (max %d)", i, len(chunk.Content), maxChars)
		}
	}
}

func TestChunker_LongSingleLine(t *testing.T) {
	chunker := NewChunker(100, 10)

	// Single very long line
	longLine := strings.Repeat("x", 5000)

	chunks := chunker.Chunk("test.js", longLine)

	if len(chunks) == 0 {
		t.Fatal("expected chunks for long single line")
	}

	// Verify chunks are properly sized
	maxExpected := 100 * CharsPerToken
	for i, chunk := range chunks {
		if len(chunk.Content) > maxExpected+50 {
			t.Errorf("chunk %d too large: %d chars (max %d)", i, len(chunk.Content), maxExpected)
		}
	}
}

func TestChunker_MixedContent(t *testing.T) {
	chunker := NewChunker(100, 10)

	// Mix of short lines and one very long line
	content := "short line 1\nshort line 2\n" + strings.Repeat("x", 2000) + "\nshort line 3\n"

	chunks := chunker.Chunk("mixed.js", content)

	if len(chunks) == 0 {
		t.Fatal("expected chunks for mixed content")
	}

	// All chunks should be within limits
	maxExpected := 100 * CharsPerToken
	for i, chunk := range chunks {
		if len(chunk.Content) > maxExpected+50 {
			t.Errorf("chunk %d too large: %d chars", i, len(chunk.Content))
		}
	}
}

func TestGetLineNumber(t *testing.T) {
	content := "line1\nline2\nline3\nline4"
	lineStarts := buildLineStarts(content)

	tests := []struct {
		pos      int
		expected int
	}{
		{0, 1},  // Start of line 1
		{3, 1},  // Middle of line 1
		{6, 2},  // Start of line 2
		{12, 3}, // Start of line 3
		{18, 4}, // Start of line 4
		{22, 4}, // End of content
	}

	for _, tt := range tests {
		result := getLineNumber(lineStarts, tt.pos)
		if result != tt.expected {
			t.Errorf("getLineNumber(pos=%d) = %d, expected %d", tt.pos, result, tt.expected)
		}
	}
}
