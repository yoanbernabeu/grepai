package trace

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func legacyStoreForPredicateTest(t *testing.T) (*PostgresSymbolStore, string) {
	t.Helper()
	cfg := isolatedSymbolSchemaConfig(t)
	schema := schemaNameFromConfig(t, cfg)
	pool := openSchemaPool(t, cfg)
	createAuthoritativeLegacySchema(t, schema, func(query string) error {
		_, err := pool.Exec(context.Background(), query)
		return err
	})
	return &PostgresSymbolStore{pool: pool, schema: schema, projectID: "predicate", projectRoot: t.TempDir()}, schema
}

func columnDefault(t *testing.T, store *PostgresSymbolStore, table, column string) string {
	t.Helper()
	var value string
	err := store.pool.QueryRow(context.Background(), `
		SELECT pg_get_expr(d.adbin,d.adrelid) FROM pg_attrdef d
		JOIN pg_class c ON c.oid=d.adrelid JOIN pg_namespace n ON n.oid=c.relnamespace
		JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum=d.adnum
		WHERE n.nspname=$1 AND c.relname=$2 AND a.attname=$3`, store.schema, table, column).Scan(&value)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func requireReservedTableCount(t *testing.T, store *PostgresSymbolStore, want int) {
	t.Helper()
	var count int
	if err := store.pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname=ANY($2)`, store.schema, reservedSymbolTables).Scan(&count); err != nil || count != want {
		t.Fatalf("reserved table count=%d, want %d: %v", count, want, err)
	}
}

func TestPostgresSymbolSchemaRejectsNonCanonicalEmptyDefaults(t *testing.T) {
	for _, expression := range []string{`' '::text`, `''::text COLLATE "C"`} {
		t.Run(strings.ReplaceAll(expression, " ", "_"), func(t *testing.T) {
			store, _ := legacyStoreForPredicateTest(t)
			if _, err := store.pool.Exec(context.Background(), `INSERT INTO symbols(project_id,name,file,line,kind) VALUES('sentinel','Keep','keep.go',7,'function')`); err != nil {
				t.Fatal(err)
			}
			if _, err := store.pool.Exec(context.Background(), `ALTER TABLE symbols ALTER COLUMN signature SET DEFAULT `+expression); err != nil {
				t.Fatal(err)
			}
			before := columnDefault(t, store, "symbols", "signature")
			hookCalls := 0
			store.schemaDDLHook = func(int, string) error { hookCalls++; return nil }
			if err := store.ensureSchema(context.Background()); err == nil {
				t.Fatal("non-canonical default was accepted")
			}
			if hookCalls != 0 {
				t.Fatalf("ran %d DDL hooks before rejecting default", hookCalls)
			}
			if after := columnDefault(t, store, "symbols", "signature"); after != before {
				t.Fatalf("default changed from %q to %q", before, after)
			}
			requireReservedTableCount(t, store, 4)
			var count int
			if err := store.pool.QueryRow(context.Background(), `SELECT count(*) FROM symbols WHERE project_id='sentinel'`).Scan(&count); err != nil || count != 1 {
				t.Fatalf("sentinel count=%d, %v", count, err)
			}
		})
	}
}

func TestPostgresSymbolSchemaRejectsLookalikeBuiltinDomains(t *testing.T) {
	tests := []struct {
		name, table, column, using string
	}{
		{"text", "symbols", "project_id", `project_id::text::"text"`},
		{"bytea", "symbols", "project_id", `convert_to(project_id,'UTF8')::"bytea"`},
		{"int4", "symbols", "line", `line::integer::"int4"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, schema := legacyStoreForPredicateTest(t)
			if _, err := store.pool.Exec(context.Background(), `INSERT INTO symbols(project_id,name,file,line,kind) VALUES('sentinel','Keep','keep.go',7,'function')`); err != nil {
				t.Fatal(err)
			}
			qdomain := pgx.Identifier{schema, tc.name}.Sanitize()
			if _, err := store.pool.Exec(context.Background(), `CREATE DOMAIN `+qdomain+` AS pg_catalog.`+tc.name); err != nil {
				t.Fatal(err)
			}
			alter := `ALTER TABLE symbols ALTER COLUMN ` + pgx.Identifier{tc.column}.Sanitize() + ` TYPE ` + qdomain + ` USING ` + tc.using
			if _, err := store.pool.Exec(context.Background(), alter); err != nil {
				t.Fatal(err)
			}
			beforeDefault := columnDefault(t, store, "symbols", "signature")
			hookCalls := 0
			store.schemaDDLHook = func(int, string) error { hookCalls++; return nil }
			if err := store.ensureSchema(context.Background()); err == nil {
				t.Fatal("lookalike domain was accepted")
			}
			if hookCalls != 0 {
				t.Fatalf("ran %d DDL hooks before rejecting domain", hookCalls)
			}
			var name string
			if err := store.pool.QueryRow(context.Background(), `SELECT name::text FROM symbols`).Scan(&name); err != nil || name != "Keep" {
				t.Fatalf("sentinel name=%q, %v", name, err)
			}
			var typeSchema string
			if err := store.pool.QueryRow(context.Background(), `SELECT tn.nspname FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_type t ON t.oid=a.atttypid JOIN pg_namespace tn ON tn.oid=t.typnamespace WHERE n.nspname=$1 AND c.relname=$2 AND a.attname=$3`, schema, tc.table, tc.column).Scan(&typeSchema); err != nil || typeSchema != schema {
				t.Fatalf("column type schema=%q, %v", typeSchema, err)
			}
			if after := columnDefault(t, store, "symbols", "signature"); after != beforeDefault {
				t.Fatalf("default changed from %q to %q", beforeDefault, after)
			}
			requireReservedTableCount(t, store, 4)
		})
	}
}

func TestPostgresSymbolSchemaReAdoptsAlterAppendedMigrationColumns(t *testing.T) {
	store, schema := legacyStoreForPredicateTest(t)
	migrations := pgx.Identifier{schema, "symbol_migrations"}.Sanitize()
	if _, err := store.pool.Exec(context.Background(), `CREATE TABLE `+migrations+` (project_id TEXT PRIMARY KEY,state TEXT NOT NULL,source_path TEXT NOT NULL,started_at TIMESTAMPTZ NOT NULL,completed_at TIMESTAMPTZ)`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(context.Background(), `INSERT INTO `+migrations+` VALUES('project','completed','source.gob',now(),now())`); err != nil {
		t.Fatal(err)
	}
	if err := store.ensureSchema(context.Background()); err != nil {
		t.Fatalf("initial legacy migration: %v", err)
	}
	if _, err := store.pool.Exec(context.Background(), `DROP TABLE symbol_store_meta`); err != nil {
		t.Fatal(err)
	}
	if err := store.ensureSchema(context.Background()); err != nil {
		t.Fatalf("marker-loss re-adoption: %v", err)
	}
	var state string
	if err := store.pool.QueryRow(context.Background(), `SELECT state FROM symbol_migrations WHERE project_id=$1`, []byte("project")).Scan(&state); err != nil || state != "completed" {
		t.Fatalf("migration row state=%q, %v", state, err)
	}
}

func TestPostgresSymbolSchemaConcurrentReservedCreationRollsBack(t *testing.T) {
	for _, tc := range []struct {
		name      string
		hookIndex int
		create    func(*PostgresSymbolStore) error
		verify    func(*PostgresSymbolStore)
	}{
		{"table", 0, func(s *PostgresSymbolStore) error {
			_, err := s.pool.Exec(context.Background(), `CREATE TABLE symbols(sentinel TEXT); INSERT INTO symbols VALUES('keep')`)
			return err
		}, func(s *PostgresSymbolStore) {
			var value string
			if err := s.pool.QueryRow(context.Background(), `SELECT sentinel FROM symbols`).Scan(&value); err != nil || value != "keep" {
				t.Fatalf("table sentinel=%q, %v", value, err)
			}
		}},
		{"index", 6, func(s *PostgresSymbolStore) error {
			_, err := s.pool.Exec(context.Background(), `CREATE TABLE app_rows(project_id BYTEA,name BYTEA,sentinel TEXT); INSERT INTO app_rows VALUES('a','b','keep'); CREATE INDEX idx_symbols_project_name ON app_rows(project_id,name)`)
			return err
		}, func(s *PostgresSymbolStore) {
			var value, owner string
			if err := s.pool.QueryRow(context.Background(), `SELECT sentinel FROM app_rows`).Scan(&value); err != nil || value != "keep" {
				t.Fatalf("index sentinel=%q, %v", value, err)
			}
			if err := s.pool.QueryRow(context.Background(), `SELECT tablename FROM pg_indexes WHERE schemaname=$1 AND indexname='idx_symbols_project_name'`, s.schema).Scan(&owner); err != nil || owner != "app_rows" {
				t.Fatalf("index owner=%q, %v", owner, err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := isolatedSymbolSchemaConfig(t)
			schema := schemaNameFromConfig(t, cfg)
			store := &PostgresSymbolStore{pool: openSchemaPool(t, cfg), schema: schema, projectID: "race", projectRoot: t.TempDir()}
			store.schemaDDLHook = func(index int, _ string) error {
				if index == tc.hookIndex {
					return tc.create(store)
				}
				return nil
			}
			if err := store.ensureSchema(context.Background()); err == nil {
				t.Fatal("concurrent reserved creation was adopted")
			}
			tc.verify(store)
			want := 0
			if tc.name == "table" {
				want = 1
			}
			requireReservedTableCount(t, store, want)
		})
	}
}
