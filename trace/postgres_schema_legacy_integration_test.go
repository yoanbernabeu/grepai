package trace

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const expectedLegacyPostgresActivationSource = "legacy-postgres-no-gob"

func createAuthoritativeLegacySchema(t *testing.T, cfgSchema string, exec func(string) error) {
	t.Helper()
	q := func(name string) string { return pgx.Identifier{cfgSchema, name}.Sanitize() }
	statements := []string{
		`CREATE TABLE ` + q("symbol_files") + ` (project_id TEXT NOT NULL, path TEXT NOT NULL, content_hash TEXT NOT NULL DEFAULT '', extractor_version TEXT NOT NULL DEFAULT '', mod_time TIMESTAMPTZ NOT NULL, PRIMARY KEY(project_id,path))`,
		`CREATE TABLE ` + q("symbols") + ` (project_id TEXT NOT NULL, name TEXT NOT NULL, file TEXT NOT NULL, line INTEGER NOT NULL, end_line INTEGER NOT NULL DEFAULT 0, kind TEXT NOT NULL, signature TEXT NOT NULL DEFAULT '', receiver TEXT NOT NULL DEFAULT '', package_name TEXT NOT NULL DEFAULT '', exported BOOLEAN NOT NULL DEFAULT FALSE, language TEXT NOT NULL DEFAULT '', docstring TEXT NOT NULL DEFAULT '', feature_path TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE ` + q("refs") + ` (project_id TEXT NOT NULL, symbol_name TEXT NOT NULL, file TEXT NOT NULL, line INTEGER NOT NULL, col INTEGER NOT NULL DEFAULT 0, ref_type TEXT NOT NULL DEFAULT '', context TEXT NOT NULL DEFAULT '', caller TEXT NOT NULL DEFAULT '', caller_file TEXT NOT NULL DEFAULT '', caller_line INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE ` + q("call_edges") + ` (project_id TEXT NOT NULL, caller TEXT NOT NULL, callee TEXT NOT NULL, file TEXT NOT NULL, line INTEGER NOT NULL, call_type TEXT NOT NULL DEFAULT '')`,
		`CREATE INDEX idx_symbols_project_name ON ` + q("symbols") + `(project_id,name)`,
		`CREATE INDEX idx_symbols_project_file ON ` + q("symbols") + `(project_id,file)`,
		`CREATE INDEX idx_refs_project_name ON ` + q("refs") + `(project_id,symbol_name)`,
		`CREATE INDEX idx_refs_project_file ON ` + q("refs") + `(project_id,file)`,
		`CREATE INDEX idx_call_edges_project_caller ON ` + q("call_edges") + `(project_id,caller)`,
		`CREATE INDEX idx_call_edges_project_callee ON ` + q("call_edges") + `(project_id,callee)`,
		`CREATE INDEX idx_call_edges_project_file ON ` + q("call_edges") + `(project_id,file)`,
	}
	for _, statement := range statements {
		if err := exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}

func seedLegacyProjects(t *testing.T, pool schemaTestPool) map[string]time.Time {
	t.Helper()
	times := map[string]time.Time{
		"legacy-one": time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC),
		"legacy-two": time.Date(2021, 2, 3, 4, 5, 6, 0, time.UTC),
	}
	for project, modTime := range times {
		file := project + ".go"
		if _, err := pool.Exec(context.Background(), `INSERT INTO symbol_files(project_id,path,mod_time) VALUES($1,$2,$3)`, project, file, modTime); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(context.Background(), `INSERT INTO symbols(project_id,name,file,line,kind) VALUES($1,$2,$3,1,'function')`, project, project, file); err != nil {
			t.Fatal(err)
		}
	}
	return times
}

type schemaTestPool interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func TestPostgresSymbolSchemaAdoptsAllPreActivationProjects(t *testing.T) {
	cfg := isolatedSymbolSchemaConfig(t)
	schema := schemaNameFromConfig(t, cfg)
	pool := openSchemaPool(t, cfg)
	createAuthoritativeLegacySchema(t, schema, func(query string) error {
		_, err := pool.Exec(context.Background(), query)
		return err
	})
	wantTimes := seedLegacyProjects(t, pool)

	one, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), cfg.Copy(), "legacy-one", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { one.Close() })
	for project, wantTime := range wantTimes {
		var state string
		var source, digest []byte
		var sourceSize *int64
		var gotTime time.Time
		err := one.pool.QueryRow(context.Background(), `SELECT state,source_path,source_digest,source_size,last_mutation_at FROM symbol_migrations WHERE project_id=$1`, identityBytes(project)).Scan(&state, &source, &digest, &sourceSize, &gotTime)
		if err != nil || state != "completed" || string(source) != expectedLegacyPostgresActivationSource || digest != nil || sourceSize != nil || !gotTime.Equal(wantTime) {
			t.Fatalf("activation %q: state=%q source=%q digest=%x size=%v time=%v err=%v", project, state, source, digest, sourceSize, gotTime, err)
		}
	}
	if err := one.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, err := one.LookupSymbol(context.Background(), "legacy-one"); err != nil || len(got) != 1 {
		t.Fatalf("first legacy data=%#v, %v", got, err)
	}
	if err := one.SaveFile(context.Background(), "one-new.go", []Symbol{{Name: "OneNew", File: "one-new.go", Line: 1}}, nil); err != nil {
		t.Fatal(err)
	}

	two, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), cfg.Copy(), "legacy-two", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { two.Close() })
	if err := two.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, err := two.LookupSymbol(context.Background(), "legacy-two"); err != nil || len(got) != 1 {
		t.Fatalf("second legacy data=%#v, %v", got, err)
	}
	if err := two.SaveFile(context.Background(), "two-new.go", []Symbol{{Name: "TwoNew", File: "two-new.go", Line: 1}}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresSymbolSchemaLegacyActivationBackfillRollsBackWithDDL(t *testing.T) {
	cfg := isolatedSymbolSchemaConfig(t)
	schema := schemaNameFromConfig(t, cfg)
	pool := openSchemaPool(t, cfg)
	createAuthoritativeLegacySchema(t, schema, func(query string) error {
		_, err := pool.Exec(context.Background(), query)
		return err
	})
	seedLegacyProjects(t, pool)
	store := &PostgresSymbolStore{pool: pool, schema: schema, projectID: "legacy-one", projectRoot: t.TempDir()}
	backfillRan := false
	store.schemaDDLHook = func(_ int, query string) error {
		if backfillRan {
			return errors.New("fail after legacy activation backfill")
		}
		backfillRan = strings.Contains(query, expectedLegacyPostgresActivationSource)
		return nil
	}
	if err := store.ensureSchema(context.Background()); err == nil || !backfillRan {
		t.Fatalf("schema failure=%v backfillRan=%v", err, backfillRan)
	}
	var migrations *string
	if err := pool.QueryRow(context.Background(), `SELECT to_regclass($1)::text`, schema+".symbol_migrations").Scan(&migrations); err != nil || migrations != nil {
		t.Fatalf("rolled-back migration table=%v, %v", migrations, err)
	}
	var project string
	if err := pool.QueryRow(context.Background(), `SELECT project_id FROM symbol_files ORDER BY project_id LIMIT 1`).Scan(&project); err != nil || project != "legacy-one" {
		t.Fatalf("legacy data after rollback=%q, %v", project, err)
	}
}

func TestPostgresSymbolSchemaAdoptsAuthoritativeTextLegacy(t *testing.T) {
	cfg := isolatedSymbolSchemaConfig(t)
	schema := schemaNameFromConfig(t, cfg)
	pool := openSchemaPool(t, cfg)
	createAuthoritativeLegacySchema(t, schema, func(query string) error {
		_, err := pool.Exec(context.Background(), query)
		return err
	})
	if _, err := pool.Exec(context.Background(), `INSERT INTO symbols(project_id,name,file,line,kind) VALUES('legacy-project','Keep','keep.go',7,'function')`); err != nil {
		t.Fatal(err)
	}

	store, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), cfg.Copy(), "legacy-project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if got := storedSchemaVersion(t, store); got != currentSymbolSchemaVersion {
		t.Fatalf("adopted version=%d", got)
	}
	var name, file []byte
	var line int
	if err := store.pool.QueryRow(context.Background(), `SELECT name,file,line FROM symbols WHERE project_id=$1`, []byte("legacy-project")).Scan(&name, &file, &line); err != nil {
		t.Fatal(err)
	}
	if string(name) != "Keep" || string(file) != "keep.go" || line != 7 {
		t.Fatalf("legacy row changed: name=%q file=%q line=%d", name, file, line)
	}
	requireRefsCallerIndex(t, store)
}

func TestPostgresSymbolSchemaFreshFailureRollsBackAndRetries(t *testing.T) {
	cfg := isolatedSymbolSchemaConfig(t)
	schema := schemaNameFromConfig(t, cfg)
	pool := openSchemaPool(t, cfg)
	store := &PostgresSymbolStore{pool: pool, schema: schema, projectID: "retry", projectRoot: t.TempDir()}
	store.schemaDDLHook = func(index int, _ string) error {
		if index == 2 {
			return errors.New("stop after partial DDL")
		}
		return nil
	}
	if err := store.ensureSchema(context.Background()); err == nil {
		t.Fatal("expected injected fresh-schema failure")
	}
	assertNoSymbolTables(t, pool, schema)
	store.schemaDDLHook = nil
	if err := store.ensureSchema(context.Background()); err != nil {
		t.Fatalf("retry after rolled-back initialization: %v", err)
	}
	if got := storedSchemaVersion(t, store); got != currentSymbolSchemaVersion {
		t.Fatalf("retry version=%d", got)
	}
}

func TestPostgresSymbolSchemaVersionOneRejectsWrongIndexDefinition(t *testing.T) {
	cfg := isolatedSymbolSchemaConfig(t)
	store := newIsolatedSchemaStore(t, cfg)
	ctx := context.Background()
	if _, err := store.pool.Exec(ctx, `DROP INDEX idx_symbols_project_name`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `CREATE INDEX idx_symbols_project_name ON symbols(file,project_id)`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE symbol_store_meta SET value=1 WHERE key='schema_version'`); err != nil {
		t.Fatal(err)
	}

	reopened, err := newPostgresSymbolStoreWithPoolConfig(ctx, cfg.Copy(), "schema-project", t.TempDir())
	if reopened != nil {
		reopened.Close()
		t.Fatal("constructor accepted wrong reserved index definition")
	}
	if err == nil {
		t.Fatal("constructor did not validate current-version index definition")
	}
	if got := storedSchemaVersion(t, store); got != 1 {
		t.Fatalf("marker changed after index rejection: %d", got)
	}
}
