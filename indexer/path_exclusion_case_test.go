package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectExistingPathUsesActualSpellingForPolicies(t *testing.T) {
	root := t.TempDir()
	actualPath := filepath.Join(root, "foo.go")
	if err := os.WriteFile(actualPath, []byte("package ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	actualInfo, err := os.Lstat(actualPath)
	if err != nil {
		t.Fatal(err)
	}
	ignore, err := NewIgnoreMatcher(root, []string{"foo.go"}, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(root, ignore)
	readCalled := false
	filesystem := pathInspectionFS{
		lstat: func(path string) (os.FileInfo, error) {
			if path == filepath.Join(root, "Foo.go") || path == actualPath {
				return actualInfo, nil
			}
			return nil, os.ErrNotExist
		},
		readDir: os.ReadDir,
		readSnapshot: func(string, string) (*FileInfo, error) {
			readCalled = true
			return nil, nil
		},
	}
	file, reason, err := scanner.inspectExistingPathWith("Foo.go", filesystem)
	if err != nil {
		t.Fatal(err)
	}
	if file != nil || reason != PathExcludedIgnored || readCalled {
		t.Fatalf("file=%v reason=%q readCalled=%v", file, reason, readCalled)
	}
}

func TestInspectExistingPathKeepsDistinctCasePolicyInputs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Foo.go"), []byte("package upper\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "foo.go"), []byte("package lower\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := NewIgnoreMatcher(root, []string{"foo.go"}, "")
	if err != nil {
		t.Fatal(err)
	}
	file, reason, err := NewScanner(root, ignore).InspectExistingPath("Foo.go")
	if err != nil || file == nil || reason != "" || file.Path != "Foo.go" {
		t.Fatalf("file=%v reason=%q err=%v", file, reason, err)
	}
}

func TestInspectExistingPathReturnsActualNativeSpelling(t *testing.T) {
	root := t.TempDir()
	actualRelative := filepath.Join("nested", "foo.go")
	actualPath := filepath.Join(root, actualRelative)
	if err := os.MkdirAll(filepath.Dir(actualPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(actualPath, []byte("package actual\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	actualInfo, err := os.Lstat(actualPath)
	if err != nil {
		t.Fatal(err)
	}
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	filesystem := pathInspectionFS{
		lstat:   func(string) (os.FileInfo, error) { return actualInfo, nil },
		readDir: os.ReadDir,
		readSnapshot: func(_ string, relative string) (*FileInfo, error) {
			return &FileInfo{Path: relative}, nil
		},
	}
	file, reason, err := NewScanner(root, ignore).inspectExistingPathWith(filepath.Join("nested", "Foo.go"), filesystem)
	if err != nil || reason != "" || file == nil || file.Path != actualRelative {
		t.Fatalf("file=%v reason=%q err=%v, want actual path %q", file, reason, err, actualRelative)
	}
}
