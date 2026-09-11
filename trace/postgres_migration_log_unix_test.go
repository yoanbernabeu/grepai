//go:build unix

package trace

import (
	"bytes"
	"context"
	"log"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostgresMigrationEscapesArchivePathInLog(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache\nforged-line")
	writeMigrationGOB(t, root, 1)
	store := newIntegrationSymbolStore(t, "migration-log-path", root)
	truncateSymbolTablesUnactivated(t, store)
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })

	if err := store.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "cache\nforged-line") {
		t.Fatalf("archive path forged a log line: %q", output.String())
	}
	if !strings.Contains(output.String(), `cache\nforged-line`) {
		t.Fatalf("escaped archive path missing: %q", output.String())
	}
}
