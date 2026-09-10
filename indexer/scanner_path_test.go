package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScannerShouldIndexPathNormalizesForIgnoreMatching(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".grepaiignore"), []byte("target.go\n!target.go/keep.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(root, ignore)

	for _, path := range []string{"target.go", filepath.Join(root, "target.go")} {
		if !scanner.SupportsPath(path) {
			t.Fatalf("SupportsPath(%q) = false; extension support must remain independent of ignore policy", path)
		}
		if scanner.ShouldIndexPath(path) {
			t.Fatalf("ShouldIndexPath(%q) = true, want ignored", path)
		}
	}
	for _, path := range []string{filepath.Join("target.go", "keep.go"), filepath.Join(root, "target.go", "keep.go")} {
		if !scanner.ShouldIndexPath(path) {
			t.Fatalf("ShouldIndexPath(%q) = false, want re-included", path)
		}
	}
	if scanner.ShouldIndexPath(filepath.Join(root, "..", "outside.go")) {
		t.Fatal("path outside scanner root was accepted")
	}
}
