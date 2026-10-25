package indexer

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// captureLogs redirects the standard logger and returns a function that
// reports everything logged since the call.
func captureLogs(t *testing.T) func() string {
	t.Helper()

	var buf bytes.Buffer
	oldWriter, oldFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
	})
	return buf.String
}

func TestNewFileChunker_FixedStrategies(t *testing.T) {
	for _, strategy := range []string{"", "fixed"} {
		t.Run("strategy="+strategy, func(t *testing.T) {
			logs := captureLogs(t)

			fc := NewFileChunker(strategy, 512, 50)

			if _, ok := fc.(*Chunker); !ok {
				t.Fatalf("NewFileChunker(%q) = %T, want *Chunker", strategy, fc)
			}
			if got := logs(); strings.Contains(got, "chunking strategy") {
				t.Errorf("unexpected warning: %q", got)
			}
		})
	}
}

func TestNewFileChunker_UnknownStrategyWarnsAndFallsBack(t *testing.T) {
	logs := captureLogs(t)

	fc := NewFileChunker("typo", 512, 50)

	if _, ok := fc.(*Chunker); !ok {
		t.Fatalf("NewFileChunker(\"typo\") = %T, want *Chunker", fc)
	}
	want := `unknown chunking strategy "typo", falling back to fixed`
	if got := logs(); !strings.Contains(got, want) {
		t.Errorf("log output %q does not contain %q", got, want)
	}
}
