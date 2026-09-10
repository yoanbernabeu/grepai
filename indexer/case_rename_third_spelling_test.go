package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRevalidateCaseRenameWitnessRejectsThirdActualSpelling(t *testing.T) {
	root := t.TempDir()
	actualPath := filepath.Join(root, "FOO.go")
	if err := os.WriteFile(actualPath, []byte("package third\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	valid, err := RevalidateCaseRenameWitness(root, "Foo.go", "foo.go")
	if err == nil {
		t.Fatalf("valid=%v err=nil, want changed-spelling error", valid)
	}
	if valid {
		t.Fatal("third spelling accepted as cached witness")
	}
	if _, statErr := os.Stat(actualPath); statErr != nil {
		t.Fatalf("third-spelling file changed during validation: %v", statErr)
	}
}
