package indexer

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

// TestIndexAll_RemovesLegacyBackslashDocuments checks that a store key using
// Windows backslashes is treated as an orphan once the scanner emits
// forward-slash paths, so IndexAll removes the legacy entry instead of leaving
// duplicates. Meaningful on all OSes when the store is pre-seeded with "\".
func TestIndexAll_RemovesLegacyBackslashDocuments(t *testing.T) {
	tmpDir := t.TempDir()
	nested := filepath.Join(tmpDir, "pkg", "util")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "util.go"), []byte("package util\n"), 0644); err != nil {
		t.Fatal(err)
	}

	legacyPath := `pkg\util\util.go`
	slashPath := "pkg/util/util.go"

	mockStore := newMockStore()
	mockStore.documents[legacyPath] = store.Document{
		Path:     legacyPath,
		Hash:     "legacy",
		ChunkIDs: []string{"legacy-chunk"},
	}

	ignoreMatcher, err := NewIgnoreMatcher(tmpDir, []string{}, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(tmpDir, ignoreMatcher)
	chunker := NewChunker(512, 50)
	idx := NewIndexer(tmpDir, mockStore, newMockBatchEmbedder(), chunker, scanner, time.Time{})

	stats, err := idx.IndexAllWithBatchProgress(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesRemoved < 1 {
		t.Fatalf("FilesRemoved = %d, want at least 1 (legacy backslash orphan)", stats.FilesRemoved)
	}

	docs, err := mockStore.ListDocuments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range docs {
		if d == legacyPath || strings.Contains(d, `\`) {
			t.Fatalf("legacy backslash document still present: %q (docs=%v GOOS=%s)", d, docs, runtime.GOOS)
		}
	}
	foundSlash := false
	for _, d := range docs {
		if d == slashPath {
			foundSlash = true
			break
		}
	}
	if !foundSlash {
		t.Fatalf("expected forward-slash document %q after index, got %v", slashPath, docs)
	}
}
