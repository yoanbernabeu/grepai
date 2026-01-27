package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	DefaultChunkSize    = 512
	DefaultChunkOverlap = 50
	CharsPerToken       = 4 // Approximation: 4 chars ≈ 1 token for code
)

type ChunkInfo struct {
	ID        string
	FilePath  string
	StartLine int
	EndLine   int
	Content   string
	Hash      string
}

type Chunker struct {
	chunkSize int
	overlap   int
}

func NewChunker(chunkSize, overlap int) *Chunker {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	if overlap < 0 {
		overlap = DefaultChunkOverlap
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 10
	}

	return &Chunker{
		chunkSize: chunkSize,
		overlap:   overlap,
	}
}

func (c *Chunker) Chunk(filePath string, content string) []ChunkInfo {
	if len(content) == 0 {
		return nil
	}

	// Use character-based chunking instead of line-based
	// This handles minified files with very long lines
	maxChars := c.chunkSize * CharsPerToken
	overlapChars := c.overlap * CharsPerToken

	var chunks []ChunkInfo
	chunkIndex := 0

	// Build line index for position -> line number mapping
	lineStarts := buildLineStarts(content)

	pos := 0
	for pos < len(content) {
		end := pos + maxChars
		if end > len(content) {
			end = len(content)
		}

		// Try to break at a newline if possible (cleaner chunks)
		if end < len(content) {
			lastNewline := strings.LastIndex(content[pos:end], "\n")
			if lastNewline > 0 {
				end = pos + lastNewline + 1
			}
		}

		chunkContent := content[pos:end]

		// Skip empty chunks
		if strings.TrimSpace(chunkContent) == "" {
			pos = end
			continue
		}

		// Calculate line numbers
		startLine := getLineNumber(lineStarts, pos)
		endLine := getLineNumber(lineStarts, end-1)

		// Generate chunk ID
		hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%s", filePath, pos, end, chunkContent)))
		chunkID := fmt.Sprintf("%s_%d", filePath, chunkIndex)

		chunks = append(chunks, ChunkInfo{
			ID:        chunkID,
			FilePath:  filePath,
			StartLine: startLine,
			EndLine:   endLine,
			Content:   chunkContent,
			Hash:      hex.EncodeToString(hash[:8]),
		})

		chunkIndex++

		// Move to next chunk with overlap
		nextPos := end - overlapChars
		if nextPos <= pos {
			nextPos = end // Prevent infinite loop
		}
		pos = nextPos
	}

	return chunks
}

// buildLineStarts returns a slice where lineStarts[i] is the byte offset of line i+1
func buildLineStarts(content string) []int {
	starts := []int{0} // Line 1 starts at position 0
	for i, r := range content {
		if r == '\n' && i+1 < len(content) {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// getLineNumber returns the 1-indexed line number for a given byte position
func getLineNumber(lineStarts []int, pos int) int {
	// Binary search for the line
	low, high := 0, len(lineStarts)-1
	for low < high {
		mid := (low + high + 1) / 2
		if lineStarts[mid] <= pos {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return low + 1 // 1-indexed
}

// ChunkWithContext adds surrounding context to improve embedding quality
func (c *Chunker) ChunkWithContext(filePath string, content string) []ChunkInfo {
	chunks := c.Chunk(filePath, content)

	// Add file path context to each chunk
	for i := range chunks {
		chunks[i].Content = fmt.Sprintf("File: %s\n\n%s", filePath, chunks[i].Content)
	}

	return chunks
}
