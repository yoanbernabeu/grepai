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

// ReChunk splits a single chunk into smaller sub-chunks when it exceeds the embedder's context limit.
// It uses half the original chunk size to ensure the new chunks fit within limits.
// The parentIndex is used to generate unique sub-chunk IDs (e.g., "file.go_0_0", "file.go_0_1").
func (c *Chunker) ReChunk(parent ChunkInfo, parentIndex int) []ChunkInfo {
	// Strip the file context prefix if present (we'll re-add it later)
	content := parent.Content
	filePrefix := fmt.Sprintf("File: %s\n\n", parent.FilePath)
	hasContext := strings.HasPrefix(content, filePrefix)
	if hasContext {
		content = strings.TrimPrefix(content, filePrefix)
	}

	if len(content) == 0 {
		return nil
	}

	// Use half the original chunk size for re-chunking
	halfSize := c.chunkSize / 2
	if halfSize < 64 {
		halfSize = 64 // Minimum reasonable chunk size
	}
	halfOverlap := c.overlap / 2

	// Create a temporary chunker with smaller settings
	subChunker := NewChunker(halfSize, halfOverlap)

	// Build line index for the original chunk content
	lineStarts := buildLineStarts(content)
	maxChars := halfSize * CharsPerToken
	overlapChars := halfOverlap * CharsPerToken

	var subChunks []ChunkInfo
	subIndex := 0
	pos := 0

	for pos < len(content) {
		end := pos + maxChars
		if end > len(content) {
			end = len(content)
		}

		// Try to break at a newline if possible
		if end < len(content) {
			lastNewline := strings.LastIndex(content[pos:end], "\n")
			if lastNewline > 0 {
				end = pos + lastNewline + 1
			}
		}

		chunkContent := content[pos:end]

		// Skip empty sub-chunks
		if strings.TrimSpace(chunkContent) == "" {
			pos = end
			continue
		}

		// Calculate line numbers relative to the parent chunk
		subStartLine := getLineNumber(lineStarts, pos)
		subEndLine := getLineNumber(lineStarts, end-1)

		// Adjust to absolute line numbers
		absoluteStartLine := parent.StartLine + subStartLine - 1
		absoluteEndLine := parent.StartLine + subEndLine - 1

		// Generate sub-chunk ID: file.go_parentIndex_subIndex
		hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%d:%s", parent.FilePath, parentIndex, subIndex, pos, chunkContent)))
		subChunkID := fmt.Sprintf("%s_%d_%d", parent.FilePath, parentIndex, subIndex)

		// Re-add file context if it was present in the parent
		finalContent := chunkContent
		if hasContext {
			finalContent = fmt.Sprintf("File: %s\n\n%s", parent.FilePath, chunkContent)
		}

		subChunks = append(subChunks, ChunkInfo{
			ID:        subChunkID,
			FilePath:  parent.FilePath,
			StartLine: absoluteStartLine,
			EndLine:   absoluteEndLine,
			Content:   finalContent,
			Hash:      hex.EncodeToString(hash[:8]),
		})

		subIndex++

		// Move to next sub-chunk with overlap
		nextPos := end - overlapChars
		if nextPos <= pos {
			nextPos = end // Prevent infinite loop
		}
		pos = nextPos
	}

	_ = subChunker // Mark as used (we might use it in the future for more complex scenarios)

	return subChunks
}

// ChunkSize returns the configured chunk size (for testing and re-chunking decisions)
func (c *Chunker) ChunkSize() int {
	return c.chunkSize
}

// Overlap returns the configured overlap (for testing)
func (c *Chunker) Overlap() int {
	return c.overlap
}
