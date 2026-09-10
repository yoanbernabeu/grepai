package store

import (
	"context"
	"testing"
	"time"
)

func TestGOBRefreshDocumentModTimeCAS(t *testing.T) {
	ctx := context.Background()
	st := NewGOBStore(t.TempDir() + "/index.gob")
	if err := st.SaveDocument(ctx, Document{Path: "a.go", Hash: "hash", ChunkIDs: []string{"c1"}}); err != nil {
		t.Fatal(err)
	}
	precise := time.Unix(123, 456789123)
	updated, err := st.RefreshDocumentModTime(ctx, "a.go", "hash", precise)
	if err != nil || !updated {
		t.Fatalf("refresh = %v, %v", updated, err)
	}
	doc, err := st.GetDocument(ctx, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	if !doc.HasExactModTime || !doc.ModTime.Equal(precise) {
		t.Fatalf("timestamp = %v exact=%v", doc.ModTime, doc.HasExactModTime)
	}
	if len(doc.ChunkIDs) != 1 || doc.ChunkIDs[0] != "c1" {
		t.Fatalf("chunk IDs changed: %v", doc.ChunkIDs)
	}
	if updated, err := st.RefreshDocumentModTime(ctx, "a.go", "wrong", precise.Add(time.Second)); err != nil || updated {
		t.Fatalf("wrong-hash refresh = %v, %v", updated, err)
	}
}

func TestGOBRefreshDocumentModTimeRejectsZeroChunks(t *testing.T) {
	ctx := context.Background()
	st := NewGOBStore(t.TempDir() + "/index.gob")
	if err := st.SaveDocument(ctx, Document{Path: "a.go", Hash: "hash"}); err != nil {
		t.Fatal(err)
	}
	if updated, err := st.RefreshDocumentModTime(ctx, "a.go", "hash", time.Now()); err != nil || updated {
		t.Fatalf("refresh = %v, %v", updated, err)
	}
}
