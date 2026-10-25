//go:build !treesitter

package indexer

import (
	"strings"
	"testing"
)

func TestNewFileChunker_ASTWithoutTreeSitterWarnsAndFallsBack(t *testing.T) {
	logs := captureLogs(t)

	fc := NewFileChunker("ast", 512, 50)

	if _, ok := fc.(*Chunker); !ok {
		t.Fatalf("NewFileChunker(\"ast\") = %T, want *Chunker", fc)
	}
	got := logs()
	for _, want := range []string{`"ast"`, "treesitter", "falling back to fixed"} {
		if !strings.Contains(got, want) {
			t.Errorf("log output %q does not contain %q", got, want)
		}
	}
}
