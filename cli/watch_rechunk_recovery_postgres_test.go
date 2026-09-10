package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/store"
)

// newRechunkRecoveryPostgresBackend returns a project-isolated Postgres store
// for the recovery regression tests, skipping when no test database is set.
func newRechunkRecoveryPostgresBackend(t *testing.T) *store.PostgresStore {
	t.Helper()
	dsn := os.Getenv("GREPAI_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("GREPAI_POSTGRES_TEST_DSN is not set")
	}
	projectID := fmt.Sprintf("cli-rechunk-recovery-%d-%d", os.Getpid(), time.Now().UnixNano())
	backend, err := store.NewPostgresStore(context.Background(), dsn, projectID, 3)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	return backend
}

// setupPrefixedRecoveryProjectPG mirrors setupPrefixedRecoveryProject (file
// under a symlinked directory so startup reconciliation recovers it) with a
// real Postgres backend behind the projectPrefixStore writer boundary.
func setupPrefixedRecoveryProjectPG(t *testing.T, name, content string) (context.Context, string, string, *indexer.Scanner, *store.PostgresStore, *projectPrefixStore, *indexer.FileInfo) {
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
	backend := newRechunkRecoveryPostgresBackend(t)
	prefixed := &projectPrefixStore{store: backend, workspaceName: "ws", projectName: "proj", projectPath: root}
	return ctx, root, relative, scanner, backend, prefixed, snapshot
}

// pgFileChunkIDs returns the sorted stored chunk IDs for one prefixed file.
func pgFileChunkIDs(t *testing.T, backend *store.PostgresStore, prefixedPath string) []string {
	t.Helper()
	all, err := backend.GetAllChunks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, chunk := range all {
		if chunk.FilePath == prefixedPath {
			ids = append(ids, chunk.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// pgDocumentCount returns how many document rows exist for a prefixed path.
func pgDocumentCount(t *testing.T, backend *store.PostgresStore, prefixedPath string) int {
	t.Helper()
	paths, err := backend.ListDocuments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, path := range paths {
		if path == prefixedPath {
			count++
		}
	}
	return count
}

// TestMiswrittenLegacyReChunkRecordRebuildsOnceThenStaysQuietPostgres proves
// the repair path against the actual workspace backend: an old-writer record
// whose chunk was stored under a truncated ID is reported incomplete, rebuilt
// exactly once through the new writer, and — unlike GOB, where the orphan is
// a pre-existing key-space leftover — Postgres DeleteByFile removes the
// obsolete chunk by file_path. The next startup makes no model calls.
func TestMiswrittenLegacyReChunkRecordRebuildsOnceThenStaysQuietPostgres(t *testing.T) {
	ctx, root, relative, scanner, backend, prefixed, snapshot := setupPrefixedRecoveryProjectPG(t, "legacy.go", "package legacy\n")
	relSlash := filepath.ToSlash(relative)
	prefixedPath := "ws/proj/" + relSlash

	// Old writer state: metadata references the raw rechunk ID while the
	// chunk was stored under a last-underscore-truncated key.
	miswritten := store.Chunk{ID: prefixedPath + "_1", FilePath: prefixedPath, Vector: []float32{9, 9, 9}, UpdatedAt: time.Now().UTC()}
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

	ids := pgFileChunkIDs(t, backend, prefixedPath)
	if len(ids) != 1 || ids[0] != prefixedPath+"_0" {
		t.Fatalf("stored chunk IDs = %v, want exactly [%q]", ids, prefixedPath+"_0")
	}
	for _, id := range ids {
		if id == miswritten.ID {
			t.Fatalf("obsolete truncated chunk %q was not deleted", id)
		}
	}
	if count := pgDocumentCount(t, backend, prefixedPath); count != 1 {
		t.Fatalf("document count = %d, want 1", count)
	}
	got, err := prefixed.GetCompleteDocument(ctx, relative)
	if err != nil || got == nil {
		t.Fatalf("GetCompleteDocument() = %+v, %v; want complete repaired document", got, err)
	}

	second := &remainingCountingEmbedder{}
	idx = indexer.NewIndexer(root, prefixed, second, indexer.NewChunker(512, 50), scanner, time.Unix(1, 0))
	stats, err = idx.IndexAll(ctx)
	if err != nil || stats.FilesIndexed != 0 || second.calls != 0 {
		t.Fatalf("second pass: indexed=%d embedded=%d err=%v", stats.FilesIndexed, second.calls, err)
	}
}

// TestModelErrorReChunkNewWriterPostgres drives a real rechunk through a model
// context-length error against the Postgres workspace backend: sub-chunk IDs
// survive the new writer intact and non-colliding, and the next startup makes
// no model calls.
func TestModelErrorReChunkNewWriterPostgres(t *testing.T) {
	content := "package large\n\n" + strings.Repeat("// a fairly long comment line for padding\n", 90)
	ctx, root, relative, scanner, backend, prefixed, snapshot := setupPrefixedRecoveryProjectPG(t, "large.go", content)
	relSlash := filepath.ToSlash(relative)
	prefixedPath := "ws/proj/" + relSlash

	chunker := indexer.NewChunker(512, 50)
	first := &contextLimitCountingEmbedder{maxChars: 1500}
	idx := indexer.NewIndexer(root, prefixed, first, chunker, scanner, time.Unix(1, 0))
	if _, err := idx.IndexFile(ctx, *snapshot); err != nil {
		t.Fatalf("IndexFile failed: %v", err)
	}
	if first.calls == 0 {
		t.Fatal("expected embeddings during initial index")
	}

	ids := pgFileChunkIDs(t, backend, prefixedPath)
	if len(ids) < 2 {
		t.Fatalf("stored chunk IDs = %v, want multiple rechunked chunks", ids)
	}
	seen := make(map[string]bool, len(ids))
	multiUnderscore := false
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("collided stored chunk ID %q", id)
		}
		seen[id] = true
		if strings.Count(strings.TrimPrefix(id, "ws/proj/"), "_") >= 2 {
			multiUnderscore = true
		}
		if !strings.HasPrefix(id, prefixedPath+"_") {
			t.Fatalf("chunk ID %q is not the preserved raw ID under the fixed prefix", id)
		}
	}
	if !multiUnderscore {
		t.Fatalf("expected rechunk multi-underscore IDs, got %v", ids)
	}
	if count := pgDocumentCount(t, backend, prefixedPath); count != 1 {
		t.Fatalf("document count = %d, want 1", count)
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
