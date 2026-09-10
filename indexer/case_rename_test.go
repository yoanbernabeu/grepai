package indexer

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

func TestFindCaseRenameWitnessesRequiresActualSpellingAndIdentity(t *testing.T) {
	root := t.TempDir()
	actualPath := filepath.Join(root, "dir", "foo.go")
	if err := os.Mkdir(filepath.Dir(actualPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(actualPath, []byte("package foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	actualInfo, err := os.Stat(actualPath)
	if err != nil {
		t.Fatal(err)
	}
	caseInsensitiveStat := func(path string) (os.FileInfo, error) {
		if filepath.Clean(path) == filepath.Join(root, "Dir", "Foo.go") || filepath.Clean(path) == actualPath {
			return actualInfo, nil
		}
		return nil, os.ErrNotExist
	}
	filesystem := caseRenameFS{stat: caseInsensitiveStat, readDir: os.ReadDir}
	witnesses := findCaseRenameWitnessesWith(root, []string{"Dir/Foo.go"}, []FileMeta{{Path: "dir/foo.go"}}, filesystem)
	if witnesses["Dir/Foo.go"] != "dir/foo.go" {
		t.Fatalf("witnesses = %#v", witnesses)
	}

	if got := findCaseRenameWitnessesWith(root, []string{"Dir/Foo.go"}, nil, filesystem); len(got) != 0 {
		t.Fatalf("unseen existing candidate treated as rename: %#v", got)
	}
	other := filepath.Join(root, "other.go")
	if err := os.WriteFile(other, []byte("package other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	distinctStat := func(path string) (os.FileInfo, error) {
		if filepath.Clean(path) == filepath.Join(root, "Dir", "Foo.go") {
			return os.Stat(other)
		}
		return os.Stat(path)
	}
	if got := findCaseRenameWitnessesWith(root, []string{"Dir/Foo.go"}, []FileMeta{{Path: "dir/foo.go"}}, caseRenameFS{stat: distinctStat, readDir: os.ReadDir}); len(got) != 0 {
		t.Fatalf("distinct files treated as rename: %#v", got)
	}
}

func TestFindCaseRenameWitnessesRejectsLinuxHardlinkSpellings(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux permits distinct case spellings in one directory")
	}
	root := t.TempDir()
	upper := filepath.Join(root, "Foo.go")
	lower := filepath.Join(root, "foo.go")
	if err := os.WriteFile(upper, []byte("package foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(upper, lower); err != nil {
		t.Fatal(err)
	}
	if got := FindCaseRenameWitnesses(root, []string{"Foo.go"}, []FileMeta{{Path: "foo.go"}}); len(got) != 0 {
		t.Fatalf("hardlink spellings treated as case rename: %#v", got)
	}
}

func TestVectorRemovalAcceptsVerifiedCaseRenameWitness(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	actualPath := filepath.Join(root, "foo.go")
	if err := os.WriteFile(actualPath, []byte("package foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	actualInfo, err := os.Stat(actualPath)
	if err != nil {
		t.Fatal(err)
	}
	st := store.NewGOBStore(filepath.Join(root, "index.gob"))
	if err := st.SaveDocument(ctx, store.Document{Path: "Foo.go", Hash: "old"}); err != nil {
		t.Fatal(err)
	}
	idx := NewIndexer(root, st, nil, nil, nil, time.Time{})
	caseInsensitiveStat := func(path string) (os.FileInfo, error) {
		if filepath.Clean(path) == filepath.Join(root, "Foo.go") || filepath.Clean(path) == actualPath {
			return actualInfo, nil
		}
		return nil, os.ErrNotExist
	}
	witnesses := findCaseRenameWitnessesWith(root, []string{"Foo.go"}, []FileMeta{{Path: "foo.go"}}, caseRenameFS{stat: caseInsensitiveStat, readDir: os.ReadDir})
	removed, err := idx.removeMissingFilesWithWitnesses(ctx,
		map[string]store.DocumentMetadata{"Foo.go": {Path: "Foo.go"}},
		witnesses,
		func(string) (os.FileInfo, error) { return actualInfo, nil },
	)
	if err != nil || removed != 1 {
		t.Fatalf("removed=%d err=%v", removed, err)
	}
	if doc, err := st.GetDocument(ctx, "Foo.go"); err != nil || doc != nil {
		t.Fatalf("old vector spelling retained: doc=%v err=%v", doc, err)
	}
}

func TestIndexAllReconcilesOfflineCaseOnlyRename(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	oldPath := filepath.Join(root, "Foo.go")
	if err := os.WriteFile(oldPath, []byte("package foo\nfunc Current() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	temporaryPath := filepath.Join(root, "case-rename-temp.go")
	newPath := filepath.Join(root, "foo.go")
	if err := os.Rename(oldPath, temporaryPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporaryPath, newPath); err != nil {
		t.Fatal(err)
	}
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	st := store.NewGOBStore(filepath.Join(root, "index.gob"))
	if err := st.SaveDocument(ctx, store.Document{Path: "Foo.go", Hash: "old", ChunkIDs: []string{"old-chunk"}}); err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(root, ignore)
	idx := NewIndexer(root, st, newMockEmbedder(), NewChunker(512, 50), scanner, time.Time{})
	stats, err := idx.IndexAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesRemoved != 1 || stats.FilesIndexed != 1 {
		t.Fatalf("removed=%d indexed=%d", stats.FilesRemoved, stats.FilesIndexed)
	}
	if old, err := st.GetDocument(ctx, "Foo.go"); err != nil || old != nil {
		t.Fatalf("old spelling retained: doc=%v err=%v", old, err)
	}
	if current, err := st.GetDocument(ctx, "foo.go"); err != nil || current == nil {
		t.Fatalf("new spelling missing: doc=%v err=%v", current, err)
	}
}

func TestCachedCaseRenameReversalRestoresFinalVectorSpelling(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	finalPath := filepath.Join(root, "Foo.go")
	if err := os.WriteFile(finalPath, []byte("package foo\nfunc Final() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	st := store.NewGOBStore(filepath.Join(root, "index.gob"))
	for _, doc := range []store.Document{
		{Path: "Foo.go", Hash: "old", ChunkIDs: []string{"old"}},
		{Path: "foo.go", Hash: "temporary", ChunkIDs: []string{"temporary"}},
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
	if removed != 1 || len(reconciliation.reeligible) != 1 || reconciliation.reeligible[0].Path != "Foo.go" {
		t.Fatalf("removed=%d reconciliation=%v", removed, reconciliation)
	}
	stats := &IndexStats{ScannedFiles: []FileMeta{{Path: "foo.go"}}}
	if err := idx.applyRemovalReconciliation(ctx, stats, reconciliation); err != nil {
		t.Fatal(err)
	}
	if len(stats.ScannedFiles) != 1 || stats.ScannedFiles[0].Path != "Foo.go" {
		t.Fatalf("consumer metadata retained temporary alias: %v", stats.ScannedFiles)
	}
	documents, err := st.ListDocuments(ctx)
	if err != nil || len(documents) != 1 || documents[0] != "Foo.go" {
		t.Fatalf("final documents=%v err=%v", documents, err)
	}
}
