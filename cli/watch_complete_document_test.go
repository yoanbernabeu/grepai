package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

type completeDocumentStore struct {
	*mockVectorStore
	path string
	doc  *store.Document
	err  error
}

func (s *completeDocumentStore) GetCompleteDocument(_ context.Context, path string) (*store.Document, error) {
	s.path = path
	return s.doc, s.err
}

func TestProjectPrefixStoreForwardsCompleteDocumentWithNativeScope(t *testing.T) {
	root := t.TempDir()
	backendDoc := &store.Document{Path: "workspace/project/nested/a.go", Hash: "hash", ChunkIDs: []string{"c1"}}
	backend := &completeDocumentStore{mockVectorStore: &mockVectorStore{}, doc: backendDoc}
	prefixed := &projectPrefixStore{
		store: backend, workspaceName: "workspace", projectName: "project", projectPath: root,
	}
	got, err := prefixed.GetCompleteDocument(context.Background(), filepath.Join(root, "nested", "a.go"))
	if err != nil || got == nil {
		t.Fatalf("GetCompleteDocument() = %+v, %v", got, err)
	}
	if backend.path != "workspace/project/nested/a.go" {
		t.Fatalf("backend path = %q", backend.path)
	}
	if got.Path != filepath.Join("nested", "a.go") || got.Hash != "hash" || len(got.ChunkIDs) != 1 || got.ChunkIDs[0] != "c1" {
		t.Fatalf("scanner-native document = %+v", got)
	}
	got.Hash = "changed"
	got.ChunkIDs[0] = "changed"
	if backendDoc.Hash != "hash" || backendDoc.ChunkIDs[0] != "c1" {
		t.Fatalf("returned document aliases backend: %+v", backendDoc)
	}
}

func TestProjectPrefixStoreRejectsCompleteDocumentOutsideExpectedPath(t *testing.T) {
	for _, backendDoc := range []*store.Document{
		nil,
		{Path: "workspace/other/a.go", ChunkIDs: []string{"foreign"}},
		{Path: "workspace/project/other.go", ChunkIDs: []string{"wrong-file"}},
	} {
		backend := &completeDocumentStore{mockVectorStore: &mockVectorStore{}, doc: backendDoc}
		prefixed := &projectPrefixStore{store: backend, workspaceName: "workspace", projectName: "project"}
		got, err := prefixed.GetCompleteDocument(context.Background(), "a.go")
		if err != nil || got != nil {
			t.Fatalf("backend document %+v returned %+v, %v", backendDoc, got, err)
		}
	}
}

func TestProjectPrefixStoreCompleteDocumentUnsupportedWithoutFallback(t *testing.T) {
	backend := &mockVectorStore{getDocumentResult: &store.Document{Path: "metadata-only.go"}}
	prefixed := &projectPrefixStore{store: backend, workspaceName: "workspace", projectName: "project"}
	got, err := prefixed.GetCompleteDocument(context.Background(), "a.go")
	if got != nil || !errors.Is(err, store.ErrCompleteDocumentUnsupported) {
		t.Fatalf("GetCompleteDocument() = %+v, %v", got, err)
	}
	if backend.getDocumentPath != "" {
		t.Fatalf("metadata fallback called GetDocument(%q)", backend.getDocumentPath)
	}
}

func TestProjectPrefixStoreForwardsCompleteDocumentError(t *testing.T) {
	wantErr := errors.New("backend failure")
	backend := &completeDocumentStore{mockVectorStore: &mockVectorStore{}, err: wantErr}
	prefixed := &projectPrefixStore{store: backend, workspaceName: "workspace", projectName: "project"}
	if _, err := prefixed.GetCompleteDocument(context.Background(), "a.go"); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

type prefixedCompleteDocumentStore struct {
	*mockVectorStore
	path       string
	prefix     string
	exactCalls int
	doc        *store.Document
	err        error
}

func (s *prefixedCompleteDocumentStore) GetCompleteDocument(_ context.Context, path string) (*store.Document, error) {
	s.exactCalls++
	s.path = path
	return s.doc, s.err
}

func (s *prefixedCompleteDocumentStore) GetCompleteDocumentWithPrefix(_ context.Context, path, prefix string) (*store.Document, error) {
	s.path = path
	s.prefix = prefix
	return s.doc, s.err
}

func TestProjectPrefixStoreInjectsOwnPrefixIntoCompleteDocument(t *testing.T) {
	root := t.TempDir()
	backendDoc := &store.Document{Path: "workspace/project/nested/a.go", Hash: "hash", ChunkIDs: []string{"nested/a.go_0"}}
	backend := &prefixedCompleteDocumentStore{mockVectorStore: &mockVectorStore{}, doc: backendDoc}
	prefixed := &projectPrefixStore{
		store: backend, workspaceName: "workspace", projectName: "project", projectPath: root,
	}
	got, err := prefixed.GetCompleteDocument(context.Background(), filepath.Join(root, "nested", "a.go"))
	if err != nil || got == nil {
		t.Fatalf("GetCompleteDocument() = %+v, %v", got, err)
	}
	if backend.path != "workspace/project/nested/a.go" {
		t.Fatalf("backend path = %q", backend.path)
	}
	if backend.prefix != "workspace/project" {
		t.Fatalf("backend chunk ID prefix = %q, want %q", backend.prefix, "workspace/project")
	}
	if backend.exactCalls != 0 {
		t.Fatalf("exact capability called %d times despite prefixed capability", backend.exactCalls)
	}
	if got.Path != filepath.Join("nested", "a.go") || got.ChunkIDs[0] != "nested/a.go_0" {
		t.Fatalf("scanner-native document = %+v", got)
	}
	got.ChunkIDs[0] = "changed"
	if backendDoc.ChunkIDs[0] != "nested/a.go_0" {
		t.Fatalf("returned document aliases backend: %+v", backendDoc)
	}
}

// TestProjectPrefixStoreCompleteDocumentRawWriterFormatGOB reproduces the real
// writer format: SaveChunks and SaveDocument run through the actual
// projectPrefixStore, which prefixes stored chunk IDs while document metadata
// keeps referencing the raw relative IDs.
func TestProjectPrefixStoreCompleteDocumentRawWriterFormatGOB(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	backend := store.NewGOBStore(filepath.Join(root, "index.gob"))
	prefixed := &projectPrefixStore{
		store: backend, workspaceName: "ws", projectName: "proj", projectPath: root,
	}
	chunks := []store.Chunk{
		{ID: "src/a.go_0", FilePath: "src/a.go", Vector: []float32{1}, Content: "one"},
		{ID: "src/a.go_1", FilePath: "src/a.go", Vector: []float32{2}, Content: "two"},
	}
	if err := prefixed.SaveChunks(ctx, chunks); err != nil {
		t.Fatal(err)
	}
	// The document references raw chunk IDs as the indexer records them; one
	// legacy reference already carries the full prefixed form.
	doc := store.Document{Path: "src/a.go", Hash: "hash", ChunkIDs: []string{"src/a.go_0", "ws/proj/src/a.go_1"}}
	if err := prefixed.SaveDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}
	got, err := prefixed.GetCompleteDocument(ctx, "src/a.go")
	if err != nil || got == nil {
		t.Fatalf("GetCompleteDocument() = %+v, %v; want complete document", got, err)
	}
	if got.Path != filepath.Join("src", "a.go") || got.Hash != "hash" ||
		len(got.ChunkIDs) != 2 || got.ChunkIDs[0] != "src/a.go_0" || got.ChunkIDs[1] != "ws/proj/src/a.go_1" {
		t.Fatalf("scanner-native document = %+v", got)
	}
	metadata, err := prefixed.GetDocument(ctx, "src/a.go")
	if err != nil || metadata == nil || metadata.Hash != "hash" {
		t.Fatalf("GetDocument metadata changed: %+v, %v", metadata, err)
	}
}

// TestProjectPrefixStoreCompleteDocumentRawWriterFormatPostgres reproduces the
// same writer format against a real Postgres backend.
func TestProjectPrefixStoreCompleteDocumentRawWriterFormatPostgres(t *testing.T) {
	dsn := os.Getenv("GREPAI_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("GREPAI_POSTGRES_TEST_DSN is not set")
	}
	ctx := context.Background()
	projectID := fmt.Sprintf("cli-complete-%d-%d", os.Getpid(), time.Now().UnixNano())
	backend, err := store.NewPostgresStore(ctx, dsn, projectID, 3)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	root := t.TempDir()
	prefixed := &projectPrefixStore{
		store: backend, workspaceName: "ws", projectName: "proj", projectPath: root,
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_ = prefixed.DeleteDocument(cleanupCtx, "src/a.go")
		_ = prefixed.DeleteByFile(cleanupCtx, "src/a.go")
	})
	chunks := []store.Chunk{
		{ID: "src/a.go_0", FilePath: "src/a.go", Vector: []float32{1, 2, 3}, Content: "one", UpdatedAt: time.Now().UTC()},
		{ID: "src/a.go_1", FilePath: "src/a.go", Vector: []float32{4, 5, 6}, Content: "two", UpdatedAt: time.Now().UTC()},
	}
	if err := prefixed.SaveChunks(ctx, chunks); err != nil {
		t.Fatal(err)
	}
	doc := store.Document{Path: "src/a.go", Hash: "hash", ChunkIDs: []string{"src/a.go_0", "ws/proj/src/a.go_1"}}
	if err := prefixed.SaveDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}
	got, err := prefixed.GetCompleteDocument(ctx, "src/a.go")
	if err != nil || got == nil {
		t.Fatalf("GetCompleteDocument() = %+v, %v; want complete document", got, err)
	}
	if got.Path != filepath.Join("src", "a.go") || len(got.ChunkIDs) != 2 || got.ChunkIDs[0] != "src/a.go_0" {
		t.Fatalf("scanner-native document = %+v", got)
	}
	metadata, err := prefixed.GetDocument(ctx, "src/a.go")
	if err != nil || metadata == nil || metadata.Hash != "hash" {
		t.Fatalf("GetDocument metadata changed: %+v, %v", metadata, err)
	}
}
