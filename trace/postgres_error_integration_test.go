package trace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
)

func TestPostgresClosedPoolOperationsReturnErrors(t *testing.T) {
	// Given a fully initialized store whose pool is then closed.
	store := newIntegrationSymbolStore(t, "closed-pool", t.TempDir())
	store.pool.Close()
	ctx := context.Background()

	// When every remote operation is attempted, each returns an error rather
	// than stale or partial data.
	checks := []struct {
		name string
		run  func() error
	}{
		{"save", func() error { return store.SaveFile(ctx, "file.go", nil, nil) }},
		{"delete", func() error { return store.DeleteFile(ctx, "file.go") }},
		{"lookup symbol", func() error { _, err := store.LookupSymbol(ctx, "Name"); return err }},
		{"lookup batch", func() error { _, err := store.LookupSymbolsBatch(ctx, []string{"Name", "Name"}); return err }},
		{"lookup callers", func() error { _, err := store.LookupCallers(ctx, "Name"); return err }},
		{"lookup callees", func() error { _, err := store.LookupCallees(ctx, "Name", "file.go"); return err }},
		{"lookup readers", func() error { _, err := store.LookupReaders(ctx, "Name"); return err }},
		{"lookup writers", func() error { _, err := store.LookupWriters(ctx, "Name"); return err }},
		{"symbols for file", func() error { _, err := store.GetSymbolsForFile(ctx, "file.go"); return err }},
		{"call edges", func() error { _, err := store.GetCallEdges(ctx); return err }},
		{"call graph", func() error { _, err := store.GetCallGraph(ctx, "Name", 2); return err }},
		{"stats", func() error { _, err := store.GetStats(ctx); return err }},
		{"load", func() error { return store.Load(ctx) }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.run(); err == nil {
				t.Fatal("operation unexpectedly succeeded on closed pool")
			}
		})
	}
	if store.IsFileIndexed("file.go") {
		t.Fatal("closed store reported indexed file")
	}
	if _, ok := store.GetFileContentHash("file.go"); ok {
		t.Fatal("closed store returned content hash")
	}
	if _, ok := store.GetFileExtractorVersion("file.go"); ok {
		t.Fatal("closed store returned extractor version")
	}
}

func TestPostgresMigrationRejectsCorruptGOBWithoutMutation(t *testing.T) {
	// Given a corrupt legacy source in an otherwise empty project.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, config.ConfigDir), 0o755); err != nil {
		t.Fatal(err)
	}
	path := config.GetSymbolIndexPath(root)
	if err := os.WriteFile(path, []byte("not-a-gob"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := newIntegrationSymbolStore(t, "corrupt-migration", root)
	truncateSymbolTablesUnactivated(t, store)

	// When migration loads the locked snapshot, decoding fails.
	err := store.Load(context.Background())

	// Then the source remains and no project data or completed marker exists.
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode failure, got %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("corrupt source was moved: %v", err)
	}
	var rows int
	if err := store.pool.QueryRow(context.Background(), `SELECT (SELECT COUNT(*) FROM symbols WHERE project_id=$1)+(SELECT COUNT(*) FROM refs WHERE project_id=$1)+(SELECT COUNT(*) FROM call_edges WHERE project_id=$1)+(SELECT COUNT(*) FROM symbol_files WHERE project_id=$1)`, identityBytes(store.projectID)).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("corrupt migration rows=%d err=%v", rows, err)
	}
	var markerRows int
	if err := store.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM symbol_migrations WHERE project_id=$1 AND state IN ('migrating','completed')`, identityBytes(store.projectID)).Scan(&markerRows); err != nil || markerRows != 0 {
		t.Fatalf("corrupt migration marker rows=%d err=%v", markerRows, err)
	}
}

func TestPostgresCanceledContextOperationsReturnErrors(t *testing.T) {
	// Given a live store and an already-canceled context.
	store := newIntegrationSymbolStore(t, "canceled-context", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When representative read/write paths execute, cancellation propagates.
	for name, run := range map[string]func() error{
		"save":         func() error { return store.SaveFile(ctx, "file.go", nil, nil) },
		"lookup batch": func() error { _, err := store.LookupSymbolsBatch(ctx, []string{"Name"}); return err },
		"graph":        func() error { _, err := store.GetCallGraph(ctx, "Name", 1); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(); err == nil {
				t.Fatal("canceled operation unexpectedly succeeded")
			}
		})
	}
}
