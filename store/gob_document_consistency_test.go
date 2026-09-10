package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGOBSaveDocumentCompletesRecordAfterContextCancellation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		existing bool
	}{
		{name: "new record"},
		{name: "existing record", existing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			st := NewGOBStore(t.TempDir() + "/index.gob")
			if tc.existing {
				if err := st.SaveChunks(ctx, []Chunk{{ID: "old", FilePath: "a.go"}}); err != nil {
					t.Fatal(err)
				}
				if err := st.SaveDocument(ctx, Document{Path: "a.go", Hash: "old", ChunkIDs: []string{"old"}}); err != nil {
					t.Fatal(err)
				}
				if err := st.DeleteByFile(ctx, "a.go"); err != nil {
					t.Fatal(err)
				}
			}
			if err := st.SaveChunks(ctx, []Chunk{{ID: "new", FilePath: "a.go"}}); err != nil {
				t.Fatal(err)
			}
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			if err := st.SaveDocument(canceled, Document{Path: "a.go", Hash: "new", ChunkIDs: []string{"new"}}); err != nil {
				t.Fatalf("document half of an in-progress record was rejected: %v", err)
			}
			if err := st.DeleteByFile(ctx, "a.go"); err != nil {
				t.Fatal(err)
			}
			_, chunks := st.Stats()
			if chunks != 0 {
				t.Fatalf("orphan chunks after DeleteByFile: %d", chunks)
			}
		})
	}
}

func TestGOBRefreshDocumentModTimeStillHonorsCanceledContext(t *testing.T) {
	ctx := context.Background()
	st := NewGOBStore(t.TempDir() + "/index.gob")
	if err := st.SaveDocument(ctx, Document{Path: "a.go", Hash: "hash", ChunkIDs: []string{"chunk"}}); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	updated, err := st.RefreshDocumentModTime(canceled, "a.go", "hash", time.Now())
	if updated || !errors.Is(err, context.Canceled) {
		t.Fatalf("updated=%v err=%v", updated, err)
	}
	doc, err := st.GetDocument(ctx, "a.go")
	if err != nil || doc == nil || doc.HasExactModTime {
		t.Fatalf("canceled refresh mutated document: doc=%v err=%v", doc, err)
	}
}
