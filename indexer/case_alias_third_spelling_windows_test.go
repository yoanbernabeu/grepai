//go:build windows

package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanRetireCaseAliasRejectsThirdActualSpelling(t *testing.T) {
	root := t.TempDir()
	actualPath := filepath.Join(root, "FOO.go")
	if err := os.WriteFile(actualPath, []byte("package third\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	retire, err := CanRetireCaseAlias(root, "Foo.go", "foo.go")
	if err == nil {
		t.Fatalf("retire=%v err=nil, want changed-spelling error", retire)
	}
	if retire {
		t.Fatal("third actual spelling accepted as retired alias")
	}
	if _, statErr := os.Stat(actualPath); statErr != nil {
		t.Fatalf("third-spelling file changed during validation: %v", statErr)
	}
}
