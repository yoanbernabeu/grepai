package trace

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func schemaNameFromConfig(t *testing.T, cfg *pgxpool.Config) string {
	t.Helper()
	return strings.Trim(cfg.ConnConfig.RuntimeParams["search_path"], `"`)
}

func openSchemaPool(t *testing.T, cfg *pgxpool.Config) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg.Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func assertOnlyReservedRelation(t *testing.T, pool *pgxpool.Pool, schema, want string) {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname=$1 AND c.relname=ANY($2) ORDER BY c.relname`, schema, reservedSymbolTables)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("reserved relations after rejection = %v, want only %q", got, want)
	}
}

func TestPostgresSymbolSchemaRejectsReservedTableCollisions(t *testing.T) {
	for _, table := range reservedSymbolTables {
		t.Run(table, func(t *testing.T) {
			cfg := isolatedSymbolSchemaConfig(t)
			schema := schemaNameFromConfig(t, cfg)
			pool := openSchemaPool(t, cfg)
			name := pgx.Identifier{schema, table}.Sanitize()
			if _, err := pool.Exec(context.Background(), `CREATE TABLE `+name+` (sentinel TEXT)`); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(context.Background(), `INSERT INTO `+name+` VALUES ('keep')`); err != nil {
				t.Fatal(err)
			}

			store, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), cfg.Copy(), "collision", t.TempDir())
			if store != nil {
				store.Close()
				t.Fatal("constructor accepted unrelated reserved table")
			}
			if err == nil {
				t.Fatal("constructor did not reject unrelated reserved table")
			}
			var sentinel string
			if scanErr := pool.QueryRow(context.Background(), `SELECT sentinel FROM `+name).Scan(&sentinel); scanErr != nil || sentinel != "keep" {
				t.Fatalf("sentinel after rejection = %q, %v", sentinel, scanErr)
			}
			assertOnlyReservedRelation(t, pool, schema, table)
		})
	}
}

func TestPostgresSymbolSchemaRejectsWrongIdentityTypeWithoutMutation(t *testing.T) {
	cfg := isolatedSymbolSchemaConfig(t)
	schema := schemaNameFromConfig(t, cfg)
	pool := openSchemaPool(t, cfg)
	if _, err := pool.Exec(context.Background(), `CREATE TABLE symbols(project_id UUID, sentinel TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO symbols VALUES(gen_random_uuid(),'keep')`); err != nil {
		t.Fatal(err)
	}
	store, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), cfg.Copy(), "collision", t.TempDir())
	if store != nil {
		store.Close()
	}
	if err == nil {
		t.Fatal("constructor accepted UUID identity column")
	}
	assertOnlyReservedRelation(t, pool, schema, "symbols")
}

func TestPostgresSymbolSchemaRejectsWrongRelationKind(t *testing.T) {
	cfg := isolatedSymbolSchemaConfig(t)
	schema := schemaNameFromConfig(t, cfg)
	pool := openSchemaPool(t, cfg)
	if _, err := pool.Exec(context.Background(), `CREATE VIEW refs AS SELECT 'keep'::text AS sentinel`); err != nil {
		t.Fatal(err)
	}
	store, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), cfg.Copy(), "collision", t.TempDir())
	if store != nil {
		store.Close()
		t.Fatal("constructor accepted a view with a reserved table name")
	}
	if err == nil {
		t.Fatal("constructor did not reject reserved relation kind")
	}
	assertOnlyReservedRelation(t, pool, schema, "refs")
}

func TestPostgresSymbolSchemaRejectsWrongIndexOwner(t *testing.T) {
	cfg := isolatedSymbolSchemaConfig(t)
	schema := schemaNameFromConfig(t, cfg)
	pool := openSchemaPool(t, cfg)
	if _, err := pool.Exec(context.Background(), `CREATE TABLE app_rows(project_id BYTEA, name BYTEA, sentinel TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `CREATE INDEX idx_symbols_project_name ON app_rows(project_id,name)`); err != nil {
		t.Fatal(err)
	}
	store, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), cfg.Copy(), "collision", t.TempDir())
	if store != nil {
		store.Close()
	}
	if err == nil {
		t.Fatal("constructor accepted reserved index owned by another table")
	}
	var owner string
	if err := pool.QueryRow(context.Background(), `SELECT tablename FROM pg_indexes WHERE schemaname=$1 AND indexname='idx_symbols_project_name'`, schema).Scan(&owner); err != nil || owner != "app_rows" {
		t.Fatalf("reserved index owner = %q, %v", owner, err)
	}
	assertNoSymbolTables(t, pool, schema)
}

func assertNoSymbolTables(t *testing.T, pool *pgxpool.Pool, schema string) {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname=ANY($2)`, schema, reservedSymbolTables).Scan(&count); err != nil || count != 0 {
		t.Fatalf("created %d reserved tables after rejection: %v", count, err)
	}
}

func TestPostgresSymbolSchemaPinsFirstSearchPathSchema(t *testing.T) {
	targetCfg := isolatedSymbolSchemaConfig(t)
	laterCfg := isolatedSymbolSchemaConfig(t)
	later := newIsolatedSchemaStore(t, laterCfg)
	target := schemaNameFromConfig(t, targetCfg)
	laterSchema := schemaNameFromConfig(t, laterCfg)
	pool := openSchemaPool(t, targetCfg)
	if _, err := pool.Exec(context.Background(), `CREATE TABLE symbols(sentinel TEXT)`); err != nil {
		t.Fatal(err)
	}
	targetCfg.ConnConfig.RuntimeParams["search_path"] = fmt.Sprintf("%s,%s", pgx.Identifier{target}.Sanitize(), pgx.Identifier{laterSchema}.Sanitize())
	store, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), targetCfg.Copy(), "collision", t.TempDir())
	if store != nil {
		store.Close()
	}
	if err == nil {
		t.Fatal("later schema marker blessed first-schema collision")
	}
	assertOnlyReservedRelation(t, pool, target, "symbols")
	if got := storedSchemaVersion(t, later); got != currentSymbolSchemaVersion {
		t.Fatalf("later schema marker changed to %d", got)
	}
}
