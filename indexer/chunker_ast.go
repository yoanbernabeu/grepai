//go:build treesitter

package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

type byteRange struct {
	start, end int
}

// ASTChunker implements cAST (Zhang et al., EMNLP 2025) recursive split-then-merge.
type ASTChunker struct {
	maxSize   int
	fallback  *Chunker
	languages map[string]*sitter.Language
}

// NewASTChunker creates a chunker that respects AST structure.
func NewASTChunker(fallback *Chunker) *ASTChunker {
	return &ASTChunker{
		maxSize:  fallback.ChunkSize() * CharsPerToken,
		fallback: fallback,
		languages: map[string]*sitter.Language{
			".go":  golang.GetLanguage(),
			".js":  javascript.GetLanguage(),
			".jsx": javascript.GetLanguage(),
			".ts":  typescript.GetLanguage(),
			".tsx": typescript.GetLanguage(),
			".py":  python.GetLanguage(),
		},
	}
}

// NewFileChunker selects a chunker based on the configured strategy.
func NewFileChunker(strategy string, size, overlap int) FileChunker {
	base := NewChunker(size, overlap)
	if strategy == "ast" {
		return NewASTChunker(base)
	}
	return base
}

func buildNWSCumSum(content string) []int {
	cumsum := make([]int, len(content)+1)
	for i := 0; i < len(content); i++ {
		cumsum[i+1] = cumsum[i]
		b := content[i]
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '\f' && b != '\v' {
			cumsum[i+1]++
		}
	}
	return cumsum
}

func nwsInRange(cumsum []int, start, end int) int {
	return cumsum[end] - cumsum[start]
}

func allChildren(node *sitter.Node) []*sitter.Node {
	count := int(node.ChildCount())
	children := make([]*sitter.Node, 0, count)
	for i := 0; i < count; i++ {
		children = append(children, node.Child(i))
	}
	return children
}

func (a *ASTChunker) ChunkWithContext(filePath, content string) []ChunkInfo {
	if len(content) == 0 {
		return nil
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	lang, ok := a.languages[ext]
	if !ok {
		return a.fallback.ChunkWithContext(filePath, content)
	}

	parser := sitter.NewParser()
	parser.SetLanguage(lang)
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(content))
	if err != nil {
		return a.fallback.ChunkWithContext(filePath, content)
	}
	defer tree.Close()

	cumsum := buildNWSCumSum(content)

	// cAST Alg.1 line 5: if file fits in budget, return single chunk
	if nwsInRange(cumsum, 0, len(content)) <= a.maxSize {
		return a.makeSingleChunk(filePath, content)
	}

	// cAST Alg.1 line 8: recursive split-then-merge on root children
	ranges := a.chunkNodes(allChildren(tree.RootNode()), content, cumsum)
	if len(ranges) == 0 {
		return a.fallback.ChunkWithContext(filePath, content)
	}

	ranges = fillGaps(ranges, len(content))
	return a.rangesToChunks(filePath, content, ranges)
}

// chunkNodes implements cAST Algorithm 1 CHUNKNODES with greedy merge.
func (a *ASTChunker) chunkNodes(nodes []*sitter.Node, content string, cumsum []int) []byteRange {
	if len(nodes) == 0 {
		return nil
	}

	var groups []byteRange
	groupStart, groupEnd := -1, -1
	groupSize := 0

	flush := func() {
		if groupStart >= 0 {
			groups = append(groups, byteRange{groupStart, groupEnd})
			groupStart, groupEnd = -1, -1
			groupSize = 0
		}
	}

	for _, node := range nodes {
		nStart := int(node.StartByte())
		nEnd := int(node.EndByte())
		s := nwsInRange(cumsum, nStart, nEnd)

		if groupSize+s > a.maxSize {
			flush()
			if s > a.maxSize {
				children := allChildren(node)
				if len(children) > 0 {
					groups = append(groups, a.chunkNodes(children, content, cumsum)...)
				} else {
					groups = append(groups, byteRange{nStart, nEnd})
				}
				continue
			}
		}

		if groupStart < 0 {
			groupStart = nStart
		}
		groupEnd = nEnd
		groupSize += s
	}

	flush()
	return a.mergeAdjacentRanges(groups, cumsum)
}

// mergeAdjacentRanges greedily merges adjacent ranges whose combined NWS count fits.
func (a *ASTChunker) mergeAdjacentRanges(groups []byteRange, cumsum []int) []byteRange {
	if len(groups) <= 1 {
		return groups
	}

	merged := make([]byteRange, 0, len(groups))
	merged = append(merged, groups[0])
	mergedNWS := nwsInRange(cumsum, groups[0].start, groups[0].end)

	for i := 1; i < len(groups); i++ {
		gNWS := nwsInRange(cumsum, groups[i].start, groups[i].end)
		if mergedNWS+gNWS <= a.maxSize {
			merged[len(merged)-1].end = groups[i].end
			mergedNWS += gNWS
		} else {
			merged = append(merged, groups[i])
			mergedNWS = gNWS
		}
	}

	return merged
}

// fillGaps makes ranges contiguous over [0, contentLen) for verbatim reconstruction.
func fillGaps(ranges []byteRange, contentLen int) []byteRange {
	if len(ranges) == 0 {
		return nil
	}
	ranges[0].start = 0
	for i := 0; i < len(ranges)-1; i++ {
		ranges[i].end = ranges[i+1].start
	}
	ranges[len(ranges)-1].end = contentLen
	return ranges
}

func (a *ASTChunker) makeSingleChunk(filePath, content string) []ChunkInfo {
	lineStarts := buildLineStarts(content)
	endPos := len(content) - 1
	if endPos < 0 {
		endPos = 0
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:0:%d:%s", filePath, len(content), content)))
	contentHash := sha256.Sum256([]byte(content))
	return []ChunkInfo{{
		ID:          fmt.Sprintf("%s_0", filePath),
		FilePath:    filePath,
		StartLine:   1,
		EndLine:     getLineNumber(lineStarts, endPos),
		Content:     fmt.Sprintf("File: %s\n\n%s", filePath, content),
		Hash:        hex.EncodeToString(hash[:8]),
		ContentHash: hex.EncodeToString(contentHash[:]),
	}}
}

func (a *ASTChunker) rangesToChunks(filePath, content string, ranges []byteRange) []ChunkInfo {
	lineStarts := buildLineStarts(content)
	chunks := make([]ChunkInfo, 0, len(ranges))

	for i, r := range ranges {
		text := content[r.start:r.end]
		if strings.TrimSpace(text) == "" {
			continue
		}
		endPos := r.end - 1
		if endPos < r.start {
			endPos = r.start
		}
		hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%s", filePath, r.start, r.end, text)))
		contentHash := sha256.Sum256([]byte(text))
		chunks = append(chunks, ChunkInfo{
			ID:          fmt.Sprintf("%s_%d", filePath, i),
			FilePath:    filePath,
			StartLine:   getLineNumber(lineStarts, r.start),
			EndLine:     getLineNumber(lineStarts, endPos),
			Content:     fmt.Sprintf("File: %s\n\n%s", filePath, text),
			Hash:        hex.EncodeToString(hash[:8]),
			ContentHash: hex.EncodeToString(contentHash[:]),
		})
	}

	return chunks
}

func (a *ASTChunker) ReChunk(parent ChunkInfo, parentIndex int) []ChunkInfo {
	return a.fallback.ReChunk(parent, parentIndex)
}
