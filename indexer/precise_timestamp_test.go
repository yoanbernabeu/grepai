package indexer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScannerPreservesNanosecondModTime(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	if err := os.WriteFile(path, []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	requested := time.Unix(1_700_000_000, 123456789)
	if err := os.Chtimes(path, requested, requested); err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	want := stat.ModTime()
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(root, ignore)
	metas, _, err := scanner.ScanMetadata()
	if err != nil || len(metas) != 1 {
		t.Fatalf("metadata = %v, %v", metas, err)
	}
	if !metas[0].ObservedModTime.Equal(want) {
		t.Fatalf("observed mtime = %v, want %v", metas[0].ObservedModTime, want)
	}
	info, err := scanner.ScanFile("a.go")
	if err != nil {
		t.Fatal(err)
	}
	if !info.ObservedModTime.Equal(want) {
		t.Fatalf("observed mtime = %v, want %v", info.ObservedModTime, want)
	}
}
