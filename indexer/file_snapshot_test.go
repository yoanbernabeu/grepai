package indexer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func replaceSnapshotPath(path, content string) error {
	tmp := path + ".replacement"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func TestReadFileSnapshotRetriesPathReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.go")
	if err := os.WriteFile(path, []byte("package old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := readFileSnapshotWithHook(path, "a.go", func(attempt int) error {
		if attempt == 0 {
			return replaceSnapshotPath(path, "package new\n")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if info == nil || info.Content != "package new\n" {
		t.Fatalf("snapshot = %+v", info)
	}
}

func TestReadFileSnapshotBoundsGrowthAfterStat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.go")
	if err := os.WriteFile(path, []byte("package small\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := readFileSnapshotWithHooks(path, "a.go", snapshotHooks{afterStat: func(int) error { return os.WriteFile(path, []byte(strings.Repeat("x", maxFileSize+2)), 0o644) }})
	if err != nil {
		t.Fatal(err)
	}
	if info != nil {
		t.Fatalf("grew-past-limit snapshot accepted: size=%d", info.Size)
	}
}

func TestReadFileSnapshotRejectsContinuallyReplacedPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.go")
	if err := os.WriteFile(path, []byte("package zero\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := readFileSnapshotWithHook(path, "a.go", func(attempt int) error { return replaceSnapshotPath(path, strings.Repeat("x", attempt+1)+"\n") })
	if err == nil || info != nil {
		t.Fatalf("unstable snapshot = %+v, %v", info, err)
	}
}
