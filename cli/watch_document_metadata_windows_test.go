//go:build windows

package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/store"
)

func TestWindowsPrefixMetadataWarmStartUsesSingleNativeKey(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	relative := filepath.Join("nested", "main.go")
	absolute := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte("package nested\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := indexer.NewScanner(root, ignore)
	file, err := scanner.ScanFile(relative)
	if err != nil || file == nil {
		t.Fatalf("snapshot=%v err=%v", file, err)
	}
	backend := store.NewGOBStore(filepath.Join(root, "index.gob"))
	backendPath := "workspace/project/" + filepath.ToSlash(relative)
	if err := backend.SaveDocument(ctx, store.Document{Path: backendPath, Hash: file.Hash, ModTime: file.ObservedModTime, HasExactModTime: true, ChunkIDs: []string{"chunk"}}); err != nil {
		t.Fatal(err)
	}
	prefixed := &projectPrefixStore{store: backend, workspaceName: "workspace", projectName: "project", projectPath: root}
	idx := indexer.NewIndexer(root, prefixed, &noOpEmbedder{}, indexer.NewChunker(512, 50), scanner, time.Now())
	stats, err := idx.IndexAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesIndexed != 0 || stats.FilesRemoved != 0 {
		t.Fatalf("indexed=%d removed=%d", stats.FilesIndexed, stats.FilesRemoved)
	}
	documents, err := backend.ListDocuments(ctx)
	if err != nil || len(documents) != 1 || documents[0] != backendPath {
		t.Fatalf("backend documents=%v err=%v", documents, err)
	}
}
