package trace

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestPostgresSymbolSchemaV1V2UpgradeBackfillsMutationTime(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			cfg := isolatedSymbolSchemaConfig(t)
			store := newIsolatedSchemaStore(t, cfg)
			want := time.Date(2021, time.Month(version), 3, 4, 5, 6, 0, time.UTC)
			if err := store.SaveFile(context.Background(), "legacy.go", nil, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := store.pool.Exec(context.Background(), `UPDATE symbol_files SET mod_time=$1 WHERE project_id=$2`, want, identityBytes(store.projectID)); err != nil {
				t.Fatal(err)
			}
			if _, err := store.pool.Exec(context.Background(), `ALTER TABLE symbol_migrations DROP COLUMN last_mutation_at`); err != nil {
				t.Fatal(err)
			}
			if _, err := store.pool.Exec(context.Background(), `DROP INDEX `+symbolMigrationStateIndexName); err != nil {
				t.Fatal(err)
			}
			if _, err := store.pool.Exec(context.Background(), `UPDATE symbol_store_meta SET value=$1 WHERE key='schema_version'`, version); err != nil {
				t.Fatal(err)
			}
			reopened, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), cfg.Copy(), store.projectID, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { reopened.Close() })
			if got := storedSchemaVersion(t, reopened); got != 3 {
				t.Fatalf("schema version=%d", got)
			}
			if got := projectMutationTime(t, reopened); !got.Equal(want) {
				t.Fatalf("backfilled mutation time=%v, want %v", got, want)
			}
			var unique bool
			if err := reopened.pool.QueryRow(context.Background(), `SELECT indisunique FROM pg_index WHERE indexrelid=$1::regclass`, symbolMigrationStateIndexName).Scan(&unique); err != nil || !unique {
				t.Fatalf("migration state index unique=%v, %v", unique, err)
			}
		})
	}
}

type legacyMetadataWriterTracer struct {
	attempted chan struct{}
	release   chan struct{}
	once      sync.Once
}

func (tr *legacyMetadataWriterTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "INSERT INTO symbols") {
		tr.once.Do(func() {
			close(tr.attempted)
			select {
			case <-tr.release:
			case <-ctx.Done():
			}
		})
	}
	return ctx
}

func (*legacyMetadataWriterTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestPostgresSymbolSchemaLocksLegacyMetadataBeforeData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	cfg := isolatedSymbolSchemaConfig(t)
	seed := newIsolatedSchemaStore(t, cfg)
	if _, err := seed.pool.Exec(ctx, `ALTER TABLE symbol_migrations DROP COLUMN last_mutation_at; UPDATE symbol_store_meta SET value=2 WHERE key='schema_version'`); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.pool.Exec(ctx, `DROP INDEX `+symbolMigrationStateIndexName); err != nil {
		t.Fatal(err)
	}

	writerTracer := &legacyMetadataWriterTracer{attempted: make(chan struct{}), release: make(chan struct{})}
	writerCfg := cfg.Copy()
	writerCfg.ConnConfig.Tracer = writerTracer
	writerPool := openSchemaPool(t, writerCfg)
	wtx, err := writerPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wtx.Exec(ctx, `SELECT state FROM symbol_migrations WHERE project_id=$1 FOR SHARE`, identityBytes(seed.projectID)); err != nil {
		t.Fatal(err)
	}
	writerDone := make(chan error, 1)
	releaseWriter := sync.OnceFunc(func() { close(writerTracer.release) })
	writerFinished := false
	go func() {
		_, err := wtx.Exec(ctx, `INSERT INTO symbols(project_id,name,file,line,kind) VALUES($1,$2,$3,1,'')`, identityBytes(seed.projectID), identityBytes("LegacyWriter"), identityBytes("writer.go"))
		writerDone <- err
	}()
	defer func() {
		releaseWriter()
		if !writerFinished {
			<-writerDone
		}
		_ = wtx.Rollback(context.Background())
	}()
	select {
	case <-writerTracer.attempted:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	repairTracer := &schemaRepairPIDTracer{captured: make(chan struct{})}
	repairCfg := cfg.Copy()
	repairCfg.ConnConfig.Tracer = repairTracer
	repairDone := make(chan error, 1)
	repairJoined := false
	go func() {
		store, err := newPostgresSymbolStoreWithPoolConfig(ctx, repairCfg, seed.projectID, t.TempDir())
		if store != nil {
			store.Close()
		}
		repairDone <- err
	}()
	defer func() {
		releaseWriter()
		cancel()
		if !repairJoined {
			<-repairDone
		}
	}()
	select {
	case <-repairTracer.captured:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitForUngrantedTableLock(t, ctx, seed.pool, repairTracer.pid.Load(), seed.schema, "symbol_migrations", "AccessExclusiveLock")
	var dataLocks int
	if err := seed.pool.QueryRow(ctx, `SELECT count(*) FROM pg_locks l JOIN pg_class c ON c.oid=l.relation JOIN pg_namespace n ON n.oid=c.relnamespace WHERE l.pid=$1 AND n.nspname=$2 AND c.relname=ANY($3) AND l.granted`, int32(repairTracer.pid.Load()), seed.schema, []string{"symbols", "refs", "call_edges", "symbol_files"}).Scan(&dataLocks); err != nil {
		t.Fatal(err)
	}
	if dataLocks != 0 {
		t.Fatalf("schema repair held %d data locks while waiting for metadata", dataLocks)
	}
	releaseWriter()
	if err := <-writerDone; err != nil {
		t.Fatal(err)
	}
	writerFinished = true
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	repairErr := <-repairDone
	repairJoined = true
	if repairErr != nil {
		t.Fatalf("metadata upgrade deadlocked: %v", repairErr)
	}
}
