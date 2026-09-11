package trace

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Data owned by a hypothetical newer schema generation. A correct
// future-version guard must refuse the schema without touching either the
// version marker or these rows.
const (
	newerSchemaProject = "newer-schema-owner"
	newerSchemaPath    = "newer-schema.go"
)

// schemaLockAttemptTracer records whether ensureSchema reached the schema
// advisory lock, and can mutate stored state at the exact instant the lock is
// attempted. PoolConfig.ConnConfig.Tracer coordinates the race through public
// pgx APIs, so no test-only hook is needed in production code. TraceQueryStart
// fires synchronously before pg_advisory_lock is sent, and the external
// holder's unlock is issued from the same callback, so the version bump is
// committed strictly between the cheap read and the locked re-read.
type schemaLockAttemptTracer struct {
	attempted atomic.Bool
	onLock    func()
	once      sync.Once
}

func (tr *schemaLockAttemptTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	// pg_advisory_xact_lock (file mutations) and pg_advisory_unlock must not
	// match: only the schema lock probe coordinates the race.
	if strings.Contains(data.SQL, "SELECT pg_advisory_lock(") {
		tr.once.Do(func() {
			tr.attempted.Store(true)
			if tr.onLock != nil {
				tr.onLock()
			}
		})
	}
	return ctx
}

func (*schemaLockAttemptTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

var _ pgx.QueryTracer = (*schemaLockAttemptTracer)(nil)

func seedNewerSchemaData(t *testing.T, store *PostgresSymbolStore) {
	t.Helper()
	_, err := store.pool.Exec(context.Background(),
		`INSERT INTO symbol_files(project_id,path,content_hash,extractor_version,mod_time) VALUES($1,$2,'','',$3)`,
		identityBytes(newerSchemaProject), identityBytes(newerSchemaPath), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
}

func setStoredSchemaVersion(t *testing.T, store *PostgresSymbolStore, version int) {
	t.Helper()
	if _, err := store.pool.Exec(context.Background(),
		`UPDATE symbol_store_meta SET value=$1 WHERE key='schema_version'`, version); err != nil {
		t.Fatal(err)
	}
}

func storedSchemaVersion(t *testing.T, store *PostgresSymbolStore) int {
	t.Helper()
	var version int
	if err := store.pool.QueryRow(context.Background(), symbolSchemaVersionQuery).Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

func newerSchemaSentinelExists(t *testing.T, store *PostgresSymbolStore) bool {
	t.Helper()
	var exists bool
	if err := store.pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM symbol_files WHERE project_id=$1 AND path=$2)`,
		identityBytes(newerSchemaProject), identityBytes(newerSchemaPath)).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	return exists
}

func connectSchemaLockHolder(t *testing.T) *pgx.Conn {
	t.Helper()
	dsn := os.Getenv("GREPAI_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("GREPAI_POSTGRES_TEST_DSN is not set")
	}
	holder, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		key1, key2 := schemaAdvisoryKey()
		_, _ = holder.Exec(context.Background(), `SELECT pg_advisory_unlock($1,$2)`, key1, key2)
		holder.Close(context.Background())
	})
	return holder
}

func TestPostgresEnsureSchemaRefusesFutureVersionWithoutMutation(t *testing.T) {
	store := newIsolatedSchemaStore(t, isolatedSymbolSchemaConfig(t))
	ddlCalls := 0
	store.schemaDDLHook = func(int, string) error { ddlCalls++; return nil }
	seedNewerSchemaData(t, store)
	setStoredSchemaVersion(t, store, currentSymbolSchemaVersion+1)

	// If the cheap read were not guarded, the lock path below would block on
	// this holder forever (until ctx deadline) instead of refusing.
	holder := connectSchemaLockHolder(t)
	key1, key2 := schemaAdvisoryKey()
	if _, err := holder.Exec(context.Background(), `SELECT pg_advisory_lock($1,$2)`, key1, key2); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := store.ensureSchema(ctx)

	if !errors.Is(err, ErrSymbolSchemaVersionTooNew) {
		t.Fatalf("future-version ensureSchema error = %v, want ErrSymbolSchemaVersionTooNew", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("guard reached the advisory lock path instead of refusing on the cheap read: %v", err)
	}
	if got := storedSchemaVersion(t, store); got != currentSymbolSchemaVersion+1 {
		t.Fatalf("guard overwrote the future marker: version=%d", got)
	}
	if ddlCalls != 0 {
		t.Fatalf("guard ran %d DDL statements against a newer schema", ddlCalls)
	}
	if !newerSchemaSentinelExists(t, store) {
		t.Fatal("guard mutated data owned by the newer schema")
	}
}

func TestPostgresLoadRefusesFutureVersionWithoutMutation(t *testing.T) {
	store := newIsolatedSchemaStore(t, isolatedSymbolSchemaConfig(t))
	seedNewerSchemaData(t, store)
	setStoredSchemaVersion(t, store, currentSymbolSchemaVersion+1)

	err := store.Load(context.Background())

	if !errors.Is(err, ErrSymbolSchemaVersionTooNew) {
		t.Fatalf("future-version Load error = %v, want ErrSymbolSchemaVersionTooNew", err)
	}
	if got := storedSchemaVersion(t, store); got != currentSymbolSchemaVersion+1 {
		t.Fatalf("Load overwrote the future marker: version=%d", got)
	}
	if !newerSchemaSentinelExists(t, store) {
		t.Fatal("Load mutated data owned by the newer schema")
	}
}

func TestPostgresConstructorRefusesFutureVersionFlippedUnderAdvisoryLock(t *testing.T) {
	store := newIsolatedSchemaStore(t, isolatedSymbolSchemaConfig(t))
	seedNewerSchemaData(t, store)
	// Stale marker: the cheap read must be allowed to fall through to the
	// lock path so the upgrade races the locked re-read.
	setStoredSchemaVersion(t, store, 0)
	key1, key2 := schemaAdvisoryKey()

	// A competing initializer holds the schema advisory lock while it
	// upgrades the schema past this build's supported version.
	holder := connectSchemaLockHolder(t)
	if _, err := holder.Exec(context.Background(), `SELECT pg_advisory_lock($1,$2)`, key1, key2); err != nil {
		t.Fatal(err)
	}

	tracer := &schemaLockAttemptTracer{}
	tracer.onLock = func() {
		// The store's cheap read already observed the stale version. Flip the
		// stored version to a future value, then release the lock so the
		// store's locked re-read is the first observation of it.
		setStoredSchemaVersion(t, store, currentSymbolSchemaVersion+1)
		if _, err := holder.Exec(context.Background(), `SELECT pg_advisory_unlock($1,$2)`, key1, key2); err != nil {
			t.Error(err)
		}
	}
	poolConfig := store.pool.Config()
	poolConfig.ConnConfig.Tracer = tracer

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	failed, err := newPostgresSymbolStoreWithPoolConfig(ctx, poolConfig.Copy(), "constructor-race", t.TempDir())
	if failed != nil {
		failed.Close()
		t.Fatal("constructor returned a store despite a future version appearing under the lock")
	}
	if !errors.Is(err, ErrSymbolSchemaVersionTooNew) {
		t.Fatalf("future-version constructor error = %v, want ErrSymbolSchemaVersionTooNew", err)
	}
	if !tracer.attempted.Load() {
		t.Fatal("constructor never attempted the schema advisory lock; the race window was not exercised")
	}
	if got := storedSchemaVersion(t, store); got != currentSymbolSchemaVersion+1 {
		t.Fatalf("constructor overwrote the future marker: version=%d", got)
	}
	if !newerSchemaSentinelExists(t, store) {
		t.Fatal("constructor mutated data owned by the newer schema")
	}
}
