package store

import (
	"context"
	"testing"
	"time"
)

func TestGOBStoreListDocumentMetadataIncludesZeroChunksAndExactTime(t *testing.T) {
	ctx := context.Background()
	st := NewGOBStore(t.TempDir() + "/index.gob")
	precise := time.Unix(1_700_000_000, 123456789)
	chunks := []string{"c1"}
	if err := st.SaveDocument(ctx, Document{Path: "b.go", Hash: "b", ModTime: precise, HasExactModTime: true, ChunkIDs: chunks}); err != nil {
		t.Fatal(err)
	}
	chunks[0] = "mutated"
	if err := st.SaveDocument(ctx, Document{Path: "a.go", Hash: "a"}); err != nil {
		t.Fatal(err)
	}
	metadata, err := st.ListDocumentMetadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(metadata) != 2 || metadata[0].Path != "a.go" || metadata[0].HasChunks {
		t.Fatalf("metadata=%+v", metadata)
	}
	if metadata[1].Path != "b.go" || !metadata[1].HasChunks || !metadata[1].HasExactModTime || !metadata[1].ModTime.Equal(precise) {
		t.Fatalf("metadata=%+v", metadata[1])
	}
	doc, _ := st.GetDocument(ctx, "b.go")
	if doc.ChunkIDs[0] != "c1" {
		t.Fatalf("stored chunk IDs aliased caller: %v", doc.ChunkIDs)
	}
}

type metadataFallbackStore struct{ VectorStore }

func TestLoadDocumentMetadataFallbackIncludesZeroChunkDocuments(t *testing.T) {
	ctx := context.Background()
	base := NewGOBStore(t.TempDir() + "/index.gob")
	if err := base.SaveDocument(ctx, Document{Path: "empty.go", Hash: "h"}); err != nil {
		t.Fatal(err)
	}
	metadata, err := LoadDocumentMetadata(ctx, &metadataFallbackStore{VectorStore: base})
	if err != nil {
		t.Fatal(err)
	}
	if len(metadata) != 1 || metadata[0].Path != "empty.go" || metadata[0].HasChunks {
		t.Fatalf("metadata=%+v", metadata)
	}
}
