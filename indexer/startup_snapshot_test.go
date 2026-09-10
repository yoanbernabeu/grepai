package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

type bulkCountingStore struct {
	*mockStore
	metadataCalls atomic.Int64
	pointCalls    atomic.Int64
}

func (s *bulkCountingStore) ListDocumentMetadata(context.Context) ([]store.DocumentMetadata, error) {
	s.metadataCalls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]store.DocumentMetadata, 0, len(s.documents))
	for path, doc := range s.documents {
		result = append(result, store.DocumentMetadata{Path: path, Hash: doc.Hash, HasChunks: len(doc.ChunkIDs) > 0, ModTime: doc.ModTime, HasExactModTime: doc.HasExactModTime})
	}
	return result, nil
}

func (s *bulkCountingStore) GetDocument(ctx context.Context, path string) (*store.Document, error) {
	s.pointCalls.Add(1)
	return s.mockStore.GetDocument(ctx, path)
}

func TestWarmStartUsesOneSnapshotNoContentReadsAndRetriesZeroChunks(t *testing.T) {
	root := t.TempDir()
	for i := range 6 {
		path := filepath.Join(root, fmt.Sprintf("file_%d.go", i))
		if err := os.WriteFile(path, []byte("package test\nfunc f() {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(root, ignore)
	st := &bulkCountingStore{mockStore: newMockStore()}
	for i := range 6 {
		path := fmt.Sprintf("file_%d.go", i)
		file, err := scanner.ScanFile(path)
		if err != nil {
			t.Fatal(err)
		}
		chunks := []string{"chunk"}
		if i == 2 {
			chunks = nil
		}
		st.documents[path] = store.Document{Path: path, Hash: file.Hash, ModTime: file.ObservedModTime, HasExactModTime: true, ChunkIDs: chunks}
	}
	var contentReads atomic.Int64
	original := scanner.readSnapshot
	scanner.readSnapshot = func(path, relPath string) (*FileInfo, error) {
		contentReads.Add(1)
		return original(path, relPath)
	}
	idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), scanner, time.Now().Add(time.Hour))
	stats, err := idx.IndexAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesIndexed != 1 {
		t.Fatalf("indexed=%d, want zero-chunk retry only", stats.FilesIndexed)
	}
	if st.metadataCalls.Load() != 1 || st.pointCalls.Load() != 0 {
		t.Fatalf("metadata=%d point=%d", st.metadataCalls.Load(), st.pointCalls.Load())
	}
	if contentReads.Load() != 1 {
		t.Fatalf("content reads=%d, want 1 for zero-chunk retry", contentReads.Load())
	}
}

func BenchmarkWarmStartBulkSnapshotNoContentIO(b *testing.B) {
	root := b.TempDir()
	const files = 100
	for i := range files {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("file_%04d.go", i)), []byte("package p\n"), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	ignore, _ := NewIgnoreMatcher(root, nil, "")
	scanner := NewScanner(root, ignore)
	st := &bulkCountingStore{mockStore: newMockStore()}
	seedRealHashes(b, scanner, files, st.documents)
	var reads atomic.Int64
	scanner.readSnapshot = func(string, string) (*FileInfo, error) {
		reads.Add(1)
		return nil, fmt.Errorf("unexpected content read")
	}
	idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), scanner, time.Now())
	b.ResetTimer()
	for range b.N {
		stats, err := idx.IndexAll(context.Background())
		if err != nil || stats.FilesIndexed != 0 {
			b.Fatalf("stats=%v err=%v", stats, err)
		}
	}
	b.StopTimer()
	if reads.Load() != 0 || st.pointCalls.Load() != 0 || st.metadataCalls.Load() != int64(b.N) {
		b.Fatalf("reads=%d point=%d metadata=%d", reads.Load(), st.pointCalls.Load(), st.metadataCalls.Load())
	}
}
