package cli

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

type mockVectorStore struct {
	savedChunks           []store.Chunk
	deletedByFilePath     string
	searchVector          []float32
	searchLimit           int
	searchPathPrefix      string
	searchResults         []store.SearchResult
	getDocumentPath       string
	getDocumentResult     *store.Document
	savedDocument         store.Document
	deletedDocumentPath   string
	listDocumentsResult   []string
	getStatsResult        *store.IndexStats
	listFilesResult       []store.FileStats
	getChunksForFilePath  string
	getChunksForFileItems []store.Chunk
	getAllChunksItems     []store.Chunk
}

func (m *mockVectorStore) SaveChunks(_ context.Context, chunks []store.Chunk) error {
	m.savedChunks = chunks
	return nil
}

func (m *mockVectorStore) DeleteByFile(_ context.Context, filePath string) error {
	m.deletedByFilePath = filePath
	return nil
}

func (m *mockVectorStore) Search(_ context.Context, queryVector []float32, limit int, opts store.SearchOptions) ([]store.SearchResult, error) {
	m.searchVector = queryVector
	m.searchLimit = limit
	m.searchPathPrefix = opts.PathPrefix
	return m.searchResults, nil
}

func (m *mockVectorStore) GetDocument(_ context.Context, filePath string) (*store.Document, error) {
	m.getDocumentPath = filePath
	return m.getDocumentResult, nil
}

func (m *mockVectorStore) SaveDocument(_ context.Context, doc store.Document) error {
	m.savedDocument = doc
	return nil
}

func (m *mockVectorStore) DeleteDocument(_ context.Context, filePath string) error {
	m.deletedDocumentPath = filePath
	return nil
}

func (m *mockVectorStore) ListDocuments(_ context.Context) ([]string, error) {
	return m.listDocumentsResult, nil
}

func (m *mockVectorStore) Load(_ context.Context) error {
	return nil
}

func (m *mockVectorStore) Persist(_ context.Context) error {
	return nil
}

func (m *mockVectorStore) Close() error {
	return nil
}

func (m *mockVectorStore) GetStats(_ context.Context) (*store.IndexStats, error) {
	return m.getStatsResult, nil
}

func (m *mockVectorStore) ListFilesWithStats(_ context.Context) ([]store.FileStats, error) {
	return m.listFilesResult, nil
}

func (m *mockVectorStore) GetChunksForFile(_ context.Context, filePath string) ([]store.Chunk, error) {
	m.getChunksForFilePath = filePath
	return m.getChunksForFileItems, nil
}

func (m *mockVectorStore) GetAllChunks(_ context.Context) ([]store.Chunk, error) {
	return m.getAllChunksItems, nil
}

func TestDescribeRetryReason(t *testing.T) {
	tests := []struct {
		statusCode int
		want       string
	}{
		{statusCode: 429, want: "Rate limited (429)"},
		{statusCode: 500, want: "Server error (500)"},
		{statusCode: 404, want: "HTTP error (404)"},
		{statusCode: 0, want: "Error"},
	}

	for _, tc := range tests {
		got := describeRetryReason(tc.statusCode)
		if got != tc.want {
			t.Errorf("describeRetryReason(%d) = %q, want %q", tc.statusCode, got, tc.want)
		}
	}
}

func TestProjectPrefixStore_SaveChunks(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	mock := &mockVectorStore{}
	wrapped := &projectPrefixStore{
		store:         mock,
		workspaceName: "ws",
		projectName:   "proj",
		projectPath:   projectRoot,
	}

	relPath := filepath.Join("dir", "a.go")
	absPath := filepath.Join(projectRoot, "dir", "b.go")
	chunks := []store.Chunk{
		{ID: relPath + "_0", FilePath: relPath},
		{ID: "orig_1", FilePath: absPath},
		{ID: "plainid", FilePath: relPath},
	}

	if err := wrapped.SaveChunks(ctx, chunks); err != nil {
		t.Fatalf("SaveChunks failed: %v", err)
	}

	if len(mock.savedChunks) != 3 {
		t.Fatalf("saved chunk count = %d, want 3", len(mock.savedChunks))
	}

	expectedPrefixedRel := wrapped.getPrefix() + "/" + filepath.ToSlash(relPath)
	if mock.savedChunks[0].FilePath != expectedPrefixedRel {
		t.Errorf("chunk0 path = %q, want %q", mock.savedChunks[0].FilePath, expectedPrefixedRel)
	}
	if mock.savedChunks[0].ID != expectedPrefixedRel+"_0" {
		t.Errorf("chunk0 id = %q, want %q", mock.savedChunks[0].ID, expectedPrefixedRel+"_0")
	}

	absRel, err := filepath.Rel(projectRoot, absPath)
	if err != nil {
		t.Fatalf("filepath.Rel failed: %v", err)
	}
	expectedPrefixedAbs := wrapped.getPrefix() + "/" + filepath.ToSlash(absRel)
	if mock.savedChunks[1].FilePath != expectedPrefixedAbs {
		t.Errorf("chunk1 path = %q, want %q", mock.savedChunks[1].FilePath, expectedPrefixedAbs)
	}
	if mock.savedChunks[1].ID != expectedPrefixedAbs+"_1" {
		t.Errorf("chunk1 id = %q, want %q", mock.savedChunks[1].ID, expectedPrefixedAbs+"_1")
	}

	// IDs without an underscore should be left as-is.
	if mock.savedChunks[2].ID != "plainid" {
		t.Errorf("chunk2 id = %q, want %q", mock.savedChunks[2].ID, "plainid")
	}
}

func TestProjectPrefixStore_PathMappedMethods(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	mock := &mockVectorStore{getDocumentResult: &store.Document{Path: "doc"}}
	wrapped := &projectPrefixStore{
		store:         mock,
		workspaceName: "ws",
		projectName:   "proj",
		projectPath:   projectRoot,
	}

	abs := filepath.Join(projectRoot, "pkg", "x.go")
	rel, err := filepath.Rel(projectRoot, abs)
	if err != nil {
		t.Fatalf("filepath.Rel failed: %v", err)
	}
	prefixed := wrapped.getPrefix() + "/" + filepath.ToSlash(rel)

	if err := wrapped.DeleteByFile(ctx, abs); err != nil {
		t.Fatalf("DeleteByFile(abs) failed: %v", err)
	}
	if mock.deletedByFilePath != prefixed {
		t.Errorf("DeleteByFile(abs) path = %q, want %q", mock.deletedByFilePath, prefixed)
	}

	if _, err := wrapped.GetDocument(ctx, abs); err != nil {
		t.Fatalf("GetDocument(abs) failed: %v", err)
	}
	if mock.getDocumentPath != prefixed {
		t.Errorf("GetDocument(abs) path = %q, want %q", mock.getDocumentPath, prefixed)
	}

	doc := store.Document{Path: abs, ModTime: time.Now()}
	if err := wrapped.SaveDocument(ctx, doc); err != nil {
		t.Fatalf("SaveDocument(abs) failed: %v", err)
	}
	if mock.savedDocument.Path != prefixed {
		t.Errorf("SaveDocument(abs) path = %q, want %q", mock.savedDocument.Path, prefixed)
	}

	if err := wrapped.DeleteDocument(ctx, abs); err != nil {
		t.Fatalf("DeleteDocument(abs) failed: %v", err)
	}
	if mock.deletedDocumentPath != prefixed {
		t.Errorf("DeleteDocument(abs) path = %q, want %q", mock.deletedDocumentPath, prefixed)
	}
}

func TestProjectPrefixStore_PassThroughAndGetChunks(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	mock := &mockVectorStore{
		searchResults:         []store.SearchResult{{Score: 0.9}},
		listDocumentsResult:   []string{"a", "b"},
		getStatsResult:        &store.IndexStats{TotalFiles: 2},
		listFilesResult:       []store.FileStats{{Path: "p"}},
		getChunksForFileItems: []store.Chunk{{ID: "c1"}},
		getAllChunksItems:     []store.Chunk{{ID: "c2"}},
	}
	wrapped := &projectPrefixStore{
		store:         mock,
		workspaceName: "ws",
		projectName:   "proj",
		projectPath:   projectRoot,
	}

	results, err := wrapped.Search(ctx, []float32{1, 2}, 5, store.SearchOptions{})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 || results[0].Score != 0.9 {
		t.Fatalf("unexpected search result: %+v", results)
	}

	docs, err := wrapped.ListDocuments(ctx)
	if err != nil || len(docs) != 2 {
		t.Fatalf("ListDocuments failed: %v %v", docs, err)
	}
	if err := wrapped.Load(ctx); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if err := wrapped.Persist(ctx); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}
	if err := wrapped.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if _, err := wrapped.GetStats(ctx); err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if _, err := wrapped.ListFilesWithStats(ctx); err != nil {
		t.Fatalf("ListFilesWithStats failed: %v", err)
	}
	if _, err := wrapped.GetAllChunks(ctx); err != nil {
		t.Fatalf("GetAllChunks failed: %v", err)
	}

	abs := filepath.Join(projectRoot, "pkg", "x.go")
	rel, err := filepath.Rel(projectRoot, abs)
	if err != nil {
		t.Fatalf("filepath.Rel failed: %v", err)
	}
	wantPrefixed := wrapped.getPrefix() + "/" + filepath.ToSlash(rel)
	if _, err := wrapped.GetChunksForFile(ctx, abs); err != nil {
		t.Fatalf("GetChunksForFile(abs) failed: %v", err)
	}
	if mock.getChunksForFilePath != wantPrefixed {
		t.Errorf("GetChunksForFile(abs) path = %q, want %q", mock.getChunksForFilePath, wantPrefixed)
	}

	// Relative path should pass through as-is when filepath.Rel fails.
	if _, err := wrapped.GetChunksForFile(ctx, "relative.go"); err != nil {
		t.Fatalf("GetChunksForFile(rel) failed: %v", err)
	}
	if mock.getChunksForFilePath != "relative.go" {
		t.Errorf("GetChunksForFile(rel) path = %q, want %q", mock.getChunksForFilePath, "relative.go")
	}
}
