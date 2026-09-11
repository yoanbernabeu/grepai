package trace

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestPostgresSymbolSchemaCurrentMarkerRejectsIncompleteCollider(t *testing.T) {
	cfg := isolatedSymbolSchemaConfig(t)
	pool := openSchemaPool(t, cfg)
	if _, err := pool.Exec(context.Background(), `
		CREATE TABLE symbol_store_meta(key TEXT PRIMARY KEY,value INTEGER NOT NULL);
		INSERT INTO symbol_store_meta VALUES('schema_version',2);
		CREATE TABLE symbols(sentinel TEXT);
		INSERT INTO symbols VALUES('keep')`); err != nil {
		t.Fatal(err)
	}
	store, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), cfg.Copy(), "collider", t.TempDir())
	if store != nil {
		store.Close()
		t.Fatal("current marker accepted an incomplete colliding schema")
	}
	if err == nil {
		t.Fatal("current marker collider was not rejected")
	}
	var sentinel string
	if err := pool.QueryRow(context.Background(), `SELECT sentinel FROM symbols`).Scan(&sentinel); err != nil || sentinel != "keep" {
		t.Fatalf("collider sentinel=%q, %v", sentinel, err)
	}
}

func TestPostgresSymbolSchemaCurrentMarkerRejectsAlteredShape(t *testing.T) {
	cfg := isolatedSymbolSchemaConfig(t)
	store := newIsolatedSchemaStore(t, cfg)
	if _, err := store.pool.Exec(context.Background(), `ALTER TABLE symbols ALTER COLUMN signature SET DEFAULT 'wrong'`); err != nil {
		t.Fatal(err)
	}
	before := columnDefault(t, store, "symbols", "signature")
	reopened, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), cfg.Copy(), "schema-project", t.TempDir())
	if reopened != nil {
		reopened.Close()
		t.Fatal("current marker accepted an altered schema")
	}
	if err == nil {
		t.Fatal("altered current schema was not rejected")
	}
	if after := columnDefault(t, store, "symbols", "signature"); after != before {
		t.Fatalf("default changed from %q to %q", before, after)
	}
}

func TestPostgresSymbolSchemaCurrentValidationAvoidsTableLocks(t *testing.T) {
	for _, table := range []string{"symbols", "symbol_files"} {
		t.Run(table, func(t *testing.T) {
			cfg := isolatedSymbolSchemaConfig(t)
			store := newIsolatedSchemaStore(t, cfg)
			tx, err := store.pool.Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			if _, err := tx.Exec(context.Background(), `LOCK TABLE `+pgx.Identifier{table}.Sanitize()+` IN ACCESS EXCLUSIVE MODE`); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			reopened, err := newPostgresSymbolStoreWithPoolConfig(ctx, cfg.Copy(), "schema-project", t.TempDir())
			if err != nil {
				t.Fatalf("catalog validation waited for %s: %v", table, err)
			}
			reopened.Close()
		})
	}
}

func TestPostgresSymbolSchemaCurrentMarkerRejectsLegacyIdentityType(t *testing.T) {
	cfg := isolatedSymbolSchemaConfig(t)
	store := newIsolatedSchemaStore(t, cfg)
	if _, err := store.pool.Exec(context.Background(), `ALTER TABLE symbols ALTER COLUMN project_id TYPE TEXT USING convert_from(project_id,'UTF8')`); err != nil {
		t.Fatal(err)
	}
	reopened, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), cfg.Copy(), "schema-project", t.TempDir())
	if reopened != nil {
		reopened.Close()
		t.Fatal("current marker accepted a legacy TEXT identity used by BYTEA queries")
	}
	if err == nil {
		t.Fatal("current schema did not reject the wrong identity type")
	}
}
