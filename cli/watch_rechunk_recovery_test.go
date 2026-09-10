package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/store"
)

// contextLimitCountingEmbedder counts embedded texts and fails oversized ones
// with a model context-length error so the indexer re-chunks them.
type contextLimitCountingEmbedder struct {
	maxChars int
	calls    int
}

func (e *contextLimitCountingEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	e.calls++
	if len(text) > e.maxChars {
		return nil, embedder.NewContextLengthError(0, len(text)/4, e.maxChars/4, "input exceeds context length")
	}
	return []float32{1, 2, 3}, nil
}

func (e *contextLimitCountingEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	e.calls += len(texts)
	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		if len(text) > e.maxChars {
			return nil, embedder.NewContextLengthError(i, len(text)/4, e.maxChars/4, "input exceeds context length")
		}
		vectors[i] = []float32{1, 2, 3}
	}
	return vectors, nil
}

func (*contextLimitCountingEmbedder) Dimensions() int { return 3 }
func (*contextLimitCountingEmbedder) Close() error    { return nil }

// setupPrefixedRecoveryProject writes one file under a symlinked directory so
// startup reconciliation treats it as a recovered (remaining) candidate, and
// returns a GOB backend behind the real projectPrefixStore writer boundary.
func setupPrefixedRecoveryProject(t *testing.T, name, content string) (context.Context, string, string, *indexer.Scanner, *store.GOBStore, *projectPrefixStore, *indexer.FileInfo) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	target := t.TempDir()
	relative := filepath.Join("linked", name)
	absolute := filepath.Join(target, name)
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore)
	snapshot, err := scanner.ScanFile(relative)
	if err != nil || snapshot == nil {
		t.Fatalf("snapshot=%v err=%v", snapshot, err)
	}
	backend := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	prefixed := &projectPrefixStore{store: backend, workspaceName: "ws", projectName: "proj", projectPath: root}
	return ctx, root, relative, scanner, backend, prefixed, snapshot
}

func backendFileChunks(t *testing.T, backend *store.GOBStore, prefixedPath string) []store.Chunk {
	t.Helper()
	all, err := backend.GetAllChunks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []store.Chunk
	for _, chunk := range all {
		if chunk.FilePath == prefixedPath {
			out = append(out, chunk)
		}
	}
	return out
}

// TestProjectPrefixStoreCompleteDocumentReChunkWriterFormatGOB runs the actual
// writer with a nested native path and true ReChunk multi-underscore IDs: the
// stored IDs preserve the entire raw ID under the fixed prefix, multiple
// sub-chunks never collide, and the complete read accepts the new writer's
// raw references.
func TestProjectPrefixStoreCompleteDocumentReChunkWriterFormatGOB(t *testing.T) {
	ctx, _, relative, _, backend, prefixed, _ := setupPrefixedRecoveryProject(t, filepath.Join("src", "nested", "a.go"), "package nested\n")
	relSlash := filepath.ToSlash(relative)
	rawIDs := []string{relSlash + "_0_0", relSlash + "_0_1", relSlash + "_1"}
	chunks := make([]store.Chunk, len(rawIDs))
	for i, id := range rawIDs {
		chunks[i] = store.Chunk{ID: id, FilePath: relative, Vector: []float32{1, 2, 3}, Content: id}
	}
	if err := prefixed.SaveChunks(ctx, chunks); err != nil {
		t.Fatal(err)
	}
	if err := prefixed.SaveDocument(ctx, store.Document{Path: relative, Hash: "hash", ChunkIDs: rawIDs}); err != nil {
		t.Fatal(err)
	}
	stored := backendFileChunks(t, backend, "ws/proj/"+relSlash)
	if len(stored) != len(rawIDs) {
		t.Fatalf("stored chunks=%v, want %d", stored, len(rawIDs))
	}
	seen := make(map[string]bool, len(stored))
	for _, chunk := range stored {
		if seen[chunk.ID] {
			t.Fatalf("collided stored chunk ID %q", chunk.ID)
		}
		seen[chunk.ID] = true
	}
	for _, id := range rawIDs {
		if !seen["ws/proj/"+id] {
			t.Fatalf("stored IDs %v missing preserved raw ID %q", seen, id)
		}
	}
	got, err := prefixed.GetCompleteDocument(ctx, relative)
	if err != nil || got == nil || len(got.ChunkIDs) != len(rawIDs) || got.ChunkIDs[0] != rawIDs[0] {
		t.Fatalf("GetCompleteDocument() = %+v, %v; want complete raw references", got, err)
	}
}

// TestProjectPrefixStoreCompleteDocumentWindowsLegacyMetadataGOB reproduces
// metadata recorded by a Windows host: raw references carry native backslash
// separators while the writer stored slash-normalized IDs. The prefixed
// namespace lookup must still resolve them.
func TestProjectPrefixStoreCompleteDocumentWindowsLegacyMetadataGOB(t *testing.T) {
	ctx, _, relative, _, backend, prefixed, _ := setupPrefixedRecoveryProject(t, filepath.Join("src", "nested", "a.go"), "package nested\n")
	const legacyRef = `linked\src\nested\a.go_0`
	stored := store.Chunk{ID: "ws/proj/linked/src/nested/a.go_0", FilePath: "ws/proj/linked/src/nested/a.go", Vector: []float32{1, 2, 3}}
	if err := backend.SaveChunks(ctx, []store.Chunk{stored}); err != nil {
		t.Fatal(err)
	}
	doc := store.Document{Path: relative, Hash: "hash", ChunkIDs: []string{legacyRef}}
	if err := prefixed.SaveDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}
	got, err := prefixed.GetCompleteDocument(ctx, relative)
	if err != nil || got == nil {
		t.Fatalf("GetCompleteDocument() = %+v, %v; want complete document", got, err)
	}
	if len(got.ChunkIDs) != 1 || got.ChunkIDs[0] != legacyRef {
		t.Fatalf("chunk IDs = %v, want verbatim legacy reference %q", got.ChunkIDs, legacyRef)
	}
	metadata, err := prefixed.GetDocument(ctx, relative)
	if err != nil || metadata == nil || metadata.Hash != "hash" {
		t.Fatalf("GetDocument metadata changed: %+v, %v", metadata, err)
	}
}

// TestMiswrittenLegacyReChunkRecordRebuildsOnceThenStaysQuiet documents the
// repair path for old rechunk metadata whose stored chunk IDs were truncated
// by the previous writer: the complete read reports the record incomplete, the
// file is rebuilt exactly once through the new writer, and the next startup
// reuses the repaired record without embedding again.
func TestMiswrittenLegacyReChunkRecordRebuildsOnceThenStaysQuiet(t *testing.T) {
	ctx, root, relative, scanner, backend, prefixed, snapshot := setupPrefixedRecoveryProject(t, "legacy.go", "package legacy\n")
	relSlash := filepath.ToSlash(relative)
	// Old writer state: metadata references the raw rechunk ID while the
	// chunk was stored under a last-underscore-truncated key.
	miswritten := store.Chunk{ID: "ws/proj/" + relSlash + "_1", FilePath: "ws/proj/" + relSlash, Vector: []float32{9, 9, 9}}
	if err := backend.SaveChunks(ctx, []store.Chunk{miswritten}); err != nil {
		t.Fatal(err)
	}
	doc := store.Document{Path: relative, Hash: snapshot.Hash, ModTime: snapshot.ObservedModTime, HasExactModTime: true, ChunkIDs: []string{relSlash + "_0_1"}}
	if err := prefixed.SaveDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}

	first := &remainingCountingEmbedder{}
	idx := indexer.NewIndexer(root, prefixed, first, indexer.NewChunker(512, 50), scanner, time.Unix(1, 0))
	stats, err := idx.IndexAll(ctx)
	if err != nil || stats.FilesIndexed != 1 || first.calls == 0 {
		t.Fatalf("first pass: indexed=%d embedded=%d err=%v", stats.FilesIndexed, first.calls, err)
	}

	second := &remainingCountingEmbedder{}
	idx = indexer.NewIndexer(root, prefixed, second, indexer.NewChunker(512, 50), scanner, time.Unix(1, 0))
	stats, err = idx.IndexAll(ctx)
	if err != nil || stats.FilesIndexed != 0 || second.calls != 0 {
		t.Fatalf("second pass: indexed=%d embedded=%d err=%v", stats.FilesIndexed, second.calls, err)
	}
}

// TestModelErrorReChunkIDsSurviveWriterAndStayQuiet drives a real rechunk
// through a model context-length error: the generated multi-underscore
// sub-chunk IDs must survive the writer intact and non-colliding, and the next
// startup must not repeat embeddings.
func TestModelErrorReChunkIDsSurviveWriterAndStayQuiet(t *testing.T) {
	content := "package large\n\n" + strings.Repeat("// a fairly long comment line for padding\n", 90)
	ctx, root, relative, scanner, backend, prefixed, snapshot := setupPrefixedRecoveryProject(t, "large.go", content)
	relSlash := filepath.ToSlash(relative)

	chunker := indexer.NewChunker(512, 50)
	first := &contextLimitCountingEmbedder{maxChars: 1500}
	idx := indexer.NewIndexer(root, prefixed, first, chunker, scanner, time.Unix(1, 0))
	if _, err := idx.IndexFile(ctx, *snapshot); err != nil {
		t.Fatalf("IndexFile failed: %v", err)
	}
	if first.calls == 0 {
		t.Fatal("expected embeddings during initial index")
	}
	stored := backendFileChunks(t, backend, "ws/proj/"+relSlash)
	if len(stored) < 2 {
		t.Fatalf("stored chunks=%v", stored)
	}
	seen := make(map[string]bool, len(stored))
	multiUnderscore := false
	for _, chunk := range stored {
		if seen[chunk.ID] {
			t.Fatalf("collided stored chunk ID %q", chunk.ID)
		}
		seen[chunk.ID] = true
		if strings.Count(strings.TrimPrefix(chunk.ID, "ws/proj/"), "_") >= 2 {
			multiUnderscore = true
		}
	}
	if !multiUnderscore {
		t.Fatalf("expected rechunk multi-underscore IDs, got %v", seen)
	}
	got, err := prefixed.GetCompleteDocument(ctx, relative)
	if err != nil || got == nil {
		t.Fatalf("GetCompleteDocument() = %+v, %v; want complete new-writer document", got, err)
	}

	second := &contextLimitCountingEmbedder{maxChars: 1500}
	idx = indexer.NewIndexer(root, prefixed, second, chunker, scanner, time.Unix(1, 0))
	stats, err := idx.IndexAll(ctx)
	if err != nil || stats.FilesIndexed != 0 || second.calls != 0 {
		t.Fatalf("second pass: indexed=%d embedded=%d err=%v", stats.FilesIndexed, second.calls, err)
	}
}
