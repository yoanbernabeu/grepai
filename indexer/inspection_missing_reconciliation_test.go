package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

func TestInspectionENOENTRemovesExcludedOwnership(t *testing.T) {
	for _, cachedRename := range []bool{false, true} {
		name := "ordinary-exclusion"
		if cachedRename {
			name = "cached-reversal"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			path := filepath.Join(root, "Foo.go")
			if err := os.WriteFile(path, []byte("package old\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			ignore, err := NewIgnoreMatcher(root, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			scanner := NewScanner(root, ignore)
			scanner.readSnapshot = func(snapshotPath, _ string) (*FileInfo, error) {
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				return nil, &os.PathError{Op: "open", Path: snapshotPath, Err: os.ErrNotExist}
			}
			st := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
			if err := st.SaveChunks(ctx, []store.Chunk{{ID: "old", FilePath: "Foo.go"}}); err != nil {
				t.Fatal(err)
			}
			if err := st.SaveDocument(ctx, store.Document{Path: "Foo.go", Hash: "old", ChunkIDs: []string{"old"}}); err != nil {
				t.Fatal(err)
			}
			caseRenames := map[string]string(nil)
			exclusions := map[string]string{"Foo.go": "cached exclusion"}
			wantRemoved := 1
			if cachedRename {
				caseRenames = map[string]string{"Foo.go": "foo.go"}
				exclusions = nil
				if err := st.SaveChunks(ctx, []store.Chunk{{ID: "alias", FilePath: "foo.go"}}); err != nil {
					t.Fatal(err)
				}
				if err := st.SaveDocument(ctx, store.Document{Path: "foo.go", Hash: "alias", ChunkIDs: []string{"alias"}}); err != nil {
					t.Fatal(err)
				}
				wantRemoved = 2
			}
			idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), scanner, time.Time{})
			removed, _, err := idx.removeCandidatesWithRevalidation(ctx,
				map[string]store.DocumentMetadata{"Foo.go": {Path: "Foo.go"}}, exclusions, caseRenames)
			if err != nil {
				t.Fatal(err)
			}
			if removed != wantRemoved {
				t.Fatalf("removed=%d, want %d", removed, wantRemoved)
			}
			if documents, err := st.ListDocuments(ctx); err != nil || len(documents) != 0 {
				t.Fatalf("documents=%v err=%v", documents, err)
			}
			_, chunks := st.Stats()
			if chunks != 0 {
				t.Fatalf("orphan chunks=%d", chunks)
			}
		})
	}
}

func TestInspectionENOENTPreservesOwnershipWhenRootIsLost(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	if err := os.WriteFile(path, []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(root, ignore)
	scanner.readSnapshot = func(snapshotPath, _ string) (*FileInfo, error) {
		if err := os.RemoveAll(root); err != nil {
			t.Fatal(err)
		}
		return nil, &os.PathError{Op: "open", Path: snapshotPath, Err: os.ErrNotExist}
	}
	st := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	if err := st.SaveDocument(ctx, store.Document{Path: "a.go", Hash: "old", ChunkIDs: []string{"old"}}); err != nil {
		t.Fatal(err)
	}
	idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), scanner, time.Time{})
	if _, _, err := idx.removeCandidatesWithRevalidation(ctx,
		map[string]store.DocumentMetadata{"a.go": {Path: "a.go"}}, map[string]string{"a.go": "excluded"}, nil); err == nil {
		t.Fatal("root loss returned successful cleanup")
	}
	if doc, err := st.GetDocument(ctx, "a.go"); err != nil || doc == nil {
		t.Fatalf("ownership lost with root: doc=%v err=%v", doc, err)
	}
}

func TestInspectionENOENTFailsWhenPathReappears(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	if err := os.WriteFile(path, []byte("package old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(root, ignore)
	scanner.readSnapshot = func(snapshotPath, _ string) (*FileInfo, error) {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package reappeared\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return nil, &os.PathError{Op: "open", Path: snapshotPath, Err: os.ErrNotExist}
	}
	st := store.NewGOBStore(filepath.Join(t.TempDir(), "index.gob"))
	if err := st.SaveDocument(ctx, store.Document{Path: "a.go", Hash: "old", ChunkIDs: []string{"old"}}); err != nil {
		t.Fatal(err)
	}
	idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), scanner, time.Time{})
	if _, _, err := idx.removeCandidatesWithRevalidation(ctx,
		map[string]store.DocumentMetadata{"a.go": {Path: "a.go"}}, map[string]string{"a.go": "excluded"}, nil); err == nil {
		t.Fatal("reappeared path returned successful stale reconciliation")
	}
	if doc, err := st.GetDocument(ctx, "a.go"); err != nil || doc == nil {
		t.Fatalf("ownership changed after reappearance: doc=%v err=%v", doc, err)
	}
}
