package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanMetadataScopeStaysWithinScopeAndDoesNotFollowDirectorySymlinks(t *testing.T) {
	root := t.TempDir()
	scope := filepath.Join(root, "src")
	outside := t.TempDir()
	if err := os.MkdirAll(scope, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		filepath.Join(scope, "new.go"):      "package src",
		filepath.Join(scope, "custom.xyz"):  "custom",
		filepath.Join(outside, "secret.go"): "package outside",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(scope, "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.go"), filepath.Join(scope, "linked.go")); err != nil {
		t.Skipf("file symlink unavailable: %v", err)
	}
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(root, ignore).WithCustomExtensions([]string{".xyz"})

	files, _, err := scanner.ScanMetadataScope("src")
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool, len(files))
	for _, file := range files {
		got[file.Path] = true
	}
	if !got[filepath.Join("src", "new.go")] || !got[filepath.Join("src", "custom.xyz")] || len(got) != 2 {
		t.Fatalf("scoped files = %#v", got)
	}
	legacy, _, err := scanner.ScanMetadata()
	if err != nil {
		t.Fatal(err)
	}
	legacyPaths := make(map[string]bool, len(legacy))
	for _, file := range legacy {
		legacyPaths[file.Path] = true
	}
	if !legacyPaths[filepath.Join("src", "linked.go")] {
		t.Fatalf("legacy metadata scan no longer includes file symlink: %#v", legacyPaths)
	}
}

func TestScanMetadataScopeRejectsUnavailableAndOutsideScopes(t *testing.T) {
	root := t.TempDir()
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(root, ignore)
	for _, scope := range []string{"missing", "..", filepath.Join("..", "outside")} {
		if files, skipped, err := scanner.ScanMetadataScope(scope); err == nil || files != nil || skipped != nil {
			t.Fatalf("scope %q: files=%#v skipped=%#v err=%v", scope, files, skipped, err)
		}
	}
}
