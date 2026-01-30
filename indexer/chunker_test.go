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

func TestChunker_ReChunk(t *testing.T) {
	chunker := NewChunker(200, 20) // Original chunk size

	t.Run("splits large chunk into smaller pieces", func(t *testing.T) {
		// Create a large chunk that would exceed context limit
		largeContent := strings.Repeat("x", 2000)
		parentChunk := ChunkInfo{
			ID:        "test.go_0",
			FilePath:  "test.go",
			StartLine: 1,
			EndLine:   50,
			Content:   largeContent,
			Hash:      "abc123",
		}

		subChunks := chunker.ReChunk(parentChunk, 0)

		// Should create multiple sub-chunks
		if len(subChunks) < 2 {
			t.Errorf("expected at least 2 sub-chunks, got %d", len(subChunks))
		}

		// Verify sub-chunk properties
		for i, subChunk := range subChunks {
			// ID should follow pattern file.go_parentIndex_subIndex
			expectedIDPrefix := "test.go_0_"
			if !strings.HasPrefix(subChunk.ID, expectedIDPrefix) {
				t.Errorf("sub-chunk %d ID should start with %q, got %q", i, expectedIDPrefix, subChunk.ID)
			}

			// FilePath should be preserved
			if subChunk.FilePath != "test.go" {
				t.Errorf("sub-chunk %d FilePath should be test.go, got %s", i, subChunk.FilePath)
			}

			// Content should not be empty
			if len(subChunk.Content) == 0 {
				t.Errorf("sub-chunk %d has empty content", i)
			}

			// Line numbers should be within parent range
			if subChunk.StartLine < parentChunk.StartLine {
				t.Errorf("sub-chunk %d StartLine %d is before parent StartLine %d",
					i, subChunk.StartLine, parentChunk.StartLine)
			}
		}
	})

	t.Run("preserves file context prefix", func(t *testing.T) {
		content := strings.Repeat("code line\n", 100)
		contentWithContext := "File: myfile.go\n\n" + content

		parentChunk := ChunkInfo{
			ID:        "myfile.go_0",
			FilePath:  "myfile.go",
			StartLine: 1,
			EndLine:   100,
			Content:   contentWithContext,
			Hash:      "abc123",
		}

		subChunks := chunker.ReChunk(parentChunk, 0)

		for i, subChunk := range subChunks {
			if !strings.HasPrefix(subChunk.Content, "File: myfile.go\n\n") {
				t.Errorf("sub-chunk %d should have file context prefix", i)
			}
		}
	})

	t.Run("handles empty content", func(t *testing.T) {
		parentChunk := ChunkInfo{
			ID:        "empty.go_0",
			FilePath:  "empty.go",
			StartLine: 1,
			EndLine:   1,
			Content:   "",
			Hash:      "abc123",
		}

		subChunks := chunker.ReChunk(parentChunk, 0)

		if len(subChunks) != 0 {
			t.Errorf("expected 0 sub-chunks for empty content, got %d", len(subChunks))
		}
	})

	t.Run("handles whitespace-only content", func(t *testing.T) {
		parentChunk := ChunkInfo{
			ID:        "whitespace.go_0",
			FilePath:  "whitespace.go",
			StartLine: 1,
			EndLine:   1,
			Content:   "   \n\n\t\t   ",
			Hash:      "abc123",
		}

		subChunks := chunker.ReChunk(parentChunk, 0)

		if len(subChunks) != 0 {
			t.Errorf("expected 0 sub-chunks for whitespace-only content, got %d", len(subChunks))
		}
	})

	t.Run("generates unique sub-chunk IDs", func(t *testing.T) {
		content := strings.Repeat("x", 2000)
		parentChunk := ChunkInfo{
			ID:        "test.go_5",
			FilePath:  "test.go",
			StartLine: 100,
			EndLine:   200,
			Content:   content,
			Hash:      "abc123",
		}

		subChunks := chunker.ReChunk(parentChunk, 5)

		idSet := make(map[string]bool)
		for _, subChunk := range subChunks {
			if idSet[subChunk.ID] {
				t.Errorf("duplicate sub-chunk ID: %s", subChunk.ID)
			}
			idSet[subChunk.ID] = true
		}
	})
}

func TestChunker_ChunkSize(t *testing.T) {
	chunker := NewChunker(256, 32)

	if chunker.ChunkSize() != 256 {
		t.Errorf("ChunkSize() = %d, expected 256", chunker.ChunkSize())
	}
}

func TestChunker_Overlap(t *testing.T) {
	chunker := NewChunker(256, 32)

	if chunker.Overlap() != 32 {
		t.Errorf("Overlap() = %d, expected 32", chunker.Overlap())
	}
}
