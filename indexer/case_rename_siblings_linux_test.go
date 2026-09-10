//go:build linux

package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

func TestCachedCaseRenamePreservesActualCaseSibling(t *testing.T) {
	for _, hardlink := range []bool{false, true} {
		name := "distinct"
		if hardlink {
			name = "hardlink"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			upper := filepath.Join(root, "Foo.go")
			lower := filepath.Join(root, "foo.go")
			if err := os.WriteFile(upper, []byte("package upper\nfunc Upper() {}\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if hardlink {
				if err := os.Link(upper, lower); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(lower, []byte("package lower\nfunc Lower() {}\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			ignore, err := NewIgnoreMatcher(root, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			st := store.NewGOBStore(filepath.Join(root, "index.gob"))
			for _, doc := range []store.Document{
				{Path: "Foo.go", Hash: "stale-upper", ChunkIDs: []string{"upper"}},
				{Path: "foo.go", Hash: "valid-lower", ChunkIDs: []string{"lower"}},
			} {
				if err := st.SaveChunks(ctx, []store.Chunk{{ID: doc.ChunkIDs[0], FilePath: doc.Path}}); err != nil {
					t.Fatal(err)
				}
				if err := st.SaveDocument(ctx, doc); err != nil {
					t.Fatal(err)
				}
			}
			idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), NewScanner(root, ignore), time.Time{})
			removed, reconciliation, err := idx.removeCandidatesWithRevalidation(ctx,
				map[string]store.DocumentMetadata{"Foo.go": {Path: "Foo.go"}}, nil,
				map[string]string{"Foo.go": "foo.go"},
			)
			if err != nil {
				t.Fatal(err)
			}
			stats := &IndexStats{ScannedFiles: []FileMeta{{Path: "foo.go"}}}
			if err := idx.applyRemovalReconciliation(ctx, stats, reconciliation); err != nil {
				t.Fatal(err)
			}
			if removed != 0 || len(stats.ScannedFiles) != 2 {
				t.Fatalf("removed=%d scanned=%v", removed, stats.ScannedFiles)
			}
			lowerDoc, err := st.GetDocument(ctx, "foo.go")
			if err != nil || lowerDoc == nil || lowerDoc.Hash != "valid-lower" {
				t.Fatalf("case sibling changed: doc=%v err=%v", lowerDoc, err)
			}
		})
	}
}
