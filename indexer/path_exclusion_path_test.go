package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectExistingPathPreservesScannerNativeNestedPath(t *testing.T) {
	root := t.TempDir()
	relative := filepath.Join("nested", "pkg", "current.go")
	absolute := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte("package current\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(root, ignore)
	file, reason, err := scanner.InspectExistingPath(relative)
	if err != nil {
		t.Fatal(err)
	}
	if reason != "" || file == nil {
		t.Fatalf("snapshot=%v reason=%q", file, reason)
	}
	if file.Path != relative {
		t.Fatalf("snapshot path=%q, want scanner-native input %q", file.Path, relative)
	}
}
