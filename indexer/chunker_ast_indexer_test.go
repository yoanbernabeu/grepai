//go:build treesitter

package indexer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/embedder"
)

// recordingBatchEmbedder implements embedder.BatchEmbedder and records the
// text of every chunk it is asked to embed.
type recordingBatchEmbedder struct {
	mu    sync.Mutex
	texts []string
}

func (r *recordingBatchEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	r.record(text)
	return []float32{0.1, 0.2, 0.3}, nil
}

func (r *recordingBatchEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		r.record(text)
		vectors[i] = []float32{0.1, 0.2, 0.3}
	}
	return vectors, nil
}

func (r *recordingBatchEmbedder) EmbedBatches(ctx context.Context, batches []embedder.Batch, progress embedder.BatchProgress) ([]embedder.BatchResult, error) {
	results := make([]embedder.BatchResult, len(batches))
	for _, batch := range batches {
		vectors := make([][]float32, batch.Size())
		for i, entry := range batch.Entries {
			r.record(entry.Content)
			vectors[i] = []float32{0.1, 0.2, 0.3}
		}
		results[batch.Index] = embedder.BatchResult{BatchIndex: batch.Index, Embeddings: vectors}
	}
	return results, nil
}

func (r *recordingBatchEmbedder) Dimensions() int { return 3 }

func (r *recordingBatchEmbedder) Close() error { return nil }

func (r *recordingBatchEmbedder) record(text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.texts = append(r.texts, text)
}

func (r *recordingBatchEmbedder) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.texts...)
}

func newASTTestIndexer(t *testing.T, dir string, emb embedder.Embedder, size, overlap int) (*Indexer, *mockStore) {
	t.Helper()

	ignoreMatcher, err := NewIgnoreMatcher(dir, []string{}, "")
	if err != nil {
		t.Fatalf("failed to create ignore matcher: %v", err)
	}
	st := newMockStore()
	chunker := NewFileChunker("ast", size, overlap)
	if _, ok := chunker.(*ASTChunker); !ok {
		t.Fatalf("NewFileChunker(\"ast\") = %T, want *ASTChunker", chunker)
	}
	return NewIndexer(dir, st, emb, chunker, NewScanner(dir, ignoreMatcher), time.Time{}), st
}

// TestIndexAll_ASTChunksAreEmbedded checks that batch indexing (used by
// embedders that implement BatchEmbedder, e.g. OpenAI) sends the chunk text to
// the embedder rather than an empty string.
func TestIndexAll_ASTChunksAreEmbedded(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(goFuncs(8)), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	emb := &recordingBatchEmbedder{}
	idx, st := newASTTestIndexer(t, dir, emb, 16, 2)

	stats, err := idx.IndexAllWithBatchProgress(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("IndexAllWithBatchProgress failed: %v", err)
	}

	if stats.ChunksCreated < 2 {
		t.Fatalf("expected the file to be split into several chunks, got %d", stats.ChunksCreated)
	}
	texts := emb.recorded()
	if len(texts) != stats.ChunksCreated {
		t.Fatalf("embedder received %d texts for %d chunks", len(texts), stats.ChunksCreated)
	}
	for i, text := range texts {
		if !strings.HasPrefix(text, "File: main.go\n\n") || !strings.Contains(text, "func f") {
			t.Errorf("text %d sent to the embedder is not chunk content: %q", i, text)
		}
	}
	for id, chunk := range st.chunks {
		if !strings.Contains(chunk.Content, "func f") {
			t.Errorf("stored chunk %s has no source: %q", id, chunk.Content)
		}
	}
}

// TestIndexAll_ASTChunkReChunkedOnContextLengthError checks that a chunk the
// embedder rejects as too large is split by ReChunk and indexing still
// succeeds.
func TestIndexAll_ASTChunkReChunkedOnContextLengthError(t *testing.T) {
	dir := t.TempDir()
	// Every root-level node is a short comment, so the AST chunker packs
	// about 2000 non-whitespace characters into each chunk, well over the
	// 1000 characters the mock embedder accepts.
	src := "package main\n\n" + strings.Repeat("// This is a very long comment line that repeats many times\n", 200)
	if err := os.WriteFile(filepath.Join(dir, "large.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	emb := newMockContextLimitEmbedder(1000)
	emb.failOnce = true
	idx, _ := newASTTestIndexer(t, dir, emb, 512, 50)

	stats, err := idx.IndexAllWithProgress(context.Background(), nil)
	if err != nil {
		t.Fatalf("IndexAllWithProgress failed: %v", err)
	}

	if stats.FilesIndexed != 1 {
		t.Errorf("expected 1 file indexed, got %d", stats.FilesIndexed)
	}
	if stats.ChunksCreated == 0 {
		t.Error("expected chunks to be created")
	}
	if !emb.rechunkExpected {
		t.Error("expected the embedder to reject an oversized chunk, which should have been re-chunked")
	}
}
