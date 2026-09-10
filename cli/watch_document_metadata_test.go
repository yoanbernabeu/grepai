package cli

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/yoanbernabeu/grepai/store"
)

type countingNonBulkStore struct {
	*mockVectorStore
	documents map[string]store.Document
	mu        sync.Mutex
	gets      []string
}

func (s *countingNonBulkStore) GetDocument(_ context.Context, path string) (*store.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gets = append(s.gets, path)
	doc, ok := s.documents[path]
	if !ok {
		return nil, nil
	}
	return &doc, nil
}

func TestProjectPrefixMetadataFallbackReadsOnlyCurrentProject(t *testing.T) {
	nested := filepath.ToSlash(filepath.Join("nested", "pkg", "a.go"))
	backend := &countingNonBulkStore{
		mockVectorStore: &mockVectorStore{listDocumentsResult: []string{
			"workspace/one/" + nested,
			"workspace/one/b.go",
			"workspace/two/foreign.go",
		}},
		documents: map[string]store.Document{
			"workspace/one/" + nested:  {Path: "workspace/one/" + nested, Hash: "a"},
			"workspace/one/b.go":       {Path: "workspace/one/b.go", Hash: "b"},
			"workspace/two/foreign.go": {Path: "workspace/two/foreign.go", Hash: "foreign"},
		},
	}
	prefixed := &projectPrefixStore{store: backend, workspaceName: "workspace", projectName: "one"}
	metadata, err := prefixed.ListDocumentMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	backend.mu.Lock()
	gets := append([]string(nil), backend.gets...)
	backend.mu.Unlock()
	if len(metadata) != 2 || len(gets) != 2 {
		t.Fatalf("metadata=%v GetDocument calls=%v", metadata, gets)
	}
	paths := map[string]bool{}
	for _, item := range metadata {
		paths[item.Path] = true
	}
	if !paths[filepath.Join("nested", "pkg", "a.go")] {
		t.Fatalf("nested metadata path not scanner-native: %v", metadata)
	}
	for _, path := range gets {
		if path == "workspace/two/foreign.go" {
			t.Fatalf("foreign project point-read: %v", gets)
		}
	}
}

type bulkNestedMetadataStore struct {
	*mockVectorStore
	metadata []store.DocumentMetadata
}

func (s *bulkNestedMetadataStore) ListDocumentMetadata(context.Context) ([]store.DocumentMetadata, error) {
	return append([]store.DocumentMetadata(nil), s.metadata...), nil
}

func TestProjectPrefixBulkMetadataUsesScannerNativeNestedPaths(t *testing.T) {
	backend := &bulkNestedMetadataStore{
		mockVectorStore: &mockVectorStore{},
		metadata: []store.DocumentMetadata{
			{Path: "workspace/one/nested/pkg/a.go", Hash: "a"},
			{Path: "workspace/two/nested/pkg/foreign.go", Hash: "foreign"},
		},
	}
	prefixed := &projectPrefixStore{store: backend, workspaceName: "workspace", projectName: "one"}
	metadata, err := prefixed.ListDocumentMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("nested", "pkg", "a.go")
	if len(metadata) != 1 || metadata[0].Path != want {
		t.Fatalf("metadata=%v, want path %q", metadata, want)
	}
}
