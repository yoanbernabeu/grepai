package trace

import (
	"context"
	"strings"
	"testing"
)

const refsCallerIndexName = "idx_refs_project_caller"

func requireRefsCallerIndex(t *testing.T, store *PostgresSymbolStore) {
	t.Helper()
	var valid bool
	var definition string
	err := store.pool.QueryRow(context.Background(), `
		SELECT i.indisvalid AND i.indisready, pg_get_indexdef(i.indexrelid)
		FROM pg_index i
		JOIN pg_class c ON c.oid=i.indexrelid
		JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname=current_schema() AND c.relname=$1`, refsCallerIndexName).Scan(&valid, &definition)
	if err != nil {
		t.Fatalf("caller index lookup: %v", err)
	}
	if !valid || !strings.HasSuffix(definition, "USING btree (project_id, caller)") {
		t.Fatalf("caller index valid=%v definition=%q", valid, definition)
	}
}

func TestPostgresSymbolSchemaFreshCreatesRefsCallerIndex(t *testing.T) {
	store := newIsolatedSchemaStore(t, isolatedSymbolSchemaConfig(t))
	requireRefsCallerIndex(t, store)
}

func TestPostgresSymbolSchemaVersionOneAddsRefsCallerIndex(t *testing.T) {
	poolConfig := isolatedSymbolSchemaConfig(t)
	store := newIsolatedSchemaStore(t, poolConfig)
	ctx := context.Background()
	if _, err := store.pool.Exec(ctx, `DROP INDEX IF EXISTS `+refsCallerIndexName); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE symbol_store_meta SET value=1 WHERE key='schema_version'`); err != nil {
		t.Fatal(err)
	}

	upgraded, err := newPostgresSymbolStoreWithPoolConfig(ctx, poolConfig.Copy(), "schema-project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { upgraded.Close() })
	if got := storedSchemaVersion(t, upgraded); got != currentSymbolSchemaVersion {
		t.Fatalf("upgraded schema version=%d, want %d", got, currentSymbolSchemaVersion)
	}
	requireRefsCallerIndex(t, upgraded)
}

func TestPostgresSymbolSchemaCurrentVersionRepairsMissingIndex(t *testing.T) {
	cfg := isolatedSymbolSchemaConfig(t)
	store := newIsolatedSchemaStore(t, cfg)
	ctx := context.Background()
	if _, err := store.pool.Exec(ctx, `DROP INDEX `+refsCallerIndexName); err != nil {
		t.Fatal(err)
	}

	reopened, err := newPostgresSymbolStoreWithPoolConfig(ctx, cfg.Copy(), "schema-project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	requireRefsCallerIndex(t, reopened)
	if got := storedSchemaVersion(t, reopened); got != currentSymbolSchemaVersion {
		t.Fatalf("index repair changed schema version to %d", got)
	}
}
