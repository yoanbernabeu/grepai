package trace

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// schemaWriterRefsBarrierTracer pauses a file-mutation transaction at the
// exact moment pgx is about to send the refs DELETE: the writer then holds
// ROW EXCLUSIVE on symbols (its first mutation table, per deleteFileTx) but
// has not yet touched refs. PoolConfig.ConnConfig.Tracer makes the pause
// deterministic without production test hooks.
type schemaWriterRefsBarrierTracer struct {
	attempted chan struct{}
	release   chan struct{}
	once      sync.Once
}

func (tr *schemaWriterRefsBarrierTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "DELETE FROM refs WHERE project_id=$1 AND file=$2") {
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

func (*schemaWriterRefsBarrierTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {
}

var _ pgx.QueryTracer = (*schemaWriterRefsBarrierTracer)(nil)

// schemaRepairPIDTracer captures the backend PID of the connection running
// the schema advisory lock, which is the same connection ensureSchema uses
// for the whole schema transaction. pg_locks can then be queried for that
// transaction's exact lock state.
type schemaRepairPIDTracer struct {
	pid      atomic.Uint32
	captured chan struct{}
	once     sync.Once
}

func (tr *schemaRepairPIDTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "SELECT pg_advisory_lock(") {
		tr.once.Do(func() {
			tr.pid.Store(conn.PgConn().PID())
			close(tr.captured)
		})
	}
	return ctx
}

func (*schemaRepairPIDTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

var _ pgx.QueryTracer = (*schemaRepairPIDTracer)(nil)

// waitForUngrantedTableLock polls pg_locks until pid holds an ungranted
// (waiting) lock request in mode on schema.table, or fails on ctx. Polling
// an explicit lock state is a barrier, not an arbitrary sleep: the test
// cannot proceed until the schema transaction is provably blocked there.
func waitForUngrantedTableLock(t *testing.T, ctx context.Context, pool *pgxpool.Pool, pid uint32, schema, table, mode string) {
	t.Helper()
	query := `SELECT EXISTS(
		SELECT 1 FROM pg_locks l
		JOIN pg_class c ON c.oid=l.relation
		JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE l.pid=$1 AND n.nspname=$2 AND c.relname=$3 AND l.mode=$4 AND NOT l.granted)`
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, query, int32(pid), schema, table, mode).Scan(&waiting); err != nil {
			t.Fatalf("pg_locks probe failed: %v", err)
		}
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("schema transaction never waited for %s on %s.%s: %v", mode, schema, table, ctx.Err())
		case <-ticker.C:
		}
	}
}

// TestPostgresSymbolSchemaRepairLocksInMutationOrder reproduces the deadlock
// from review: a writer paused between its symbols and refs mutations while a
// missing-index repair locks the owned tables. Mutation order is
// symbols->refs->call_edges->symbol_files; the repair must lock in that same
// order. The old alphabetical order locked refs/call_edges first, so the
// repair held refs while waiting on symbols and the writer held symbols while
// waiting on refs — a lock cycle Postgres must abort. With a
// mutation-consistent order the repair waits on symbols holding no
// later-mutated table, so the writer commits and the repair succeeds.
func TestPostgresSymbolSchemaRepairLocksInMutationOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	poolConfig := isolatedSymbolSchemaConfig(t)
	seed := newIsolatedSchemaStore(t, poolConfig)
	for _, file := range []struct {
		path, symbol, target string
	}{
		{"main.go", "Main", "MainTarget"},
		{"keep.go", "Keep", "KeepTarget"},
	} {
		refs := []Reference{{SymbolName: file.target, Kind: RefKindCall, File: file.path, Line: 2, CallerName: file.symbol, CallerFile: file.path, CallerLine: 1}}
		if err := seed.SaveFile(ctx, file.path, []Symbol{{Name: file.symbol, Kind: KindFunction, File: file.path, Line: 1}}, refs); err != nil {
			t.Fatal(err)
		}
	}

	writerTracer := &schemaWriterRefsBarrierTracer{attempted: make(chan struct{}), release: make(chan struct{})}
	writerConfig := poolConfig.Copy()
	writerConfig.ConnConfig.Tracer = writerTracer
	writer, err := newPostgresSymbolStoreWithPoolConfig(ctx, writerConfig, "schema-project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { writer.Close() })

	// Drop the index only after the writer constructed, so exactly the
	// repair constructor below takes the missing-index path.
	if _, err := seed.pool.Exec(ctx, `DROP INDEX `+refsCallerIndexName); err != nil {
		t.Fatal(err)
	}

	writerResult := make(chan error, 1)
	releaseWriter := sync.OnceFunc(func() { close(writerTracer.release) })
	go func() {
		defer close(writerResult)
		writerResult <- writer.DeleteFile(ctx, "main.go")
	}()
	t.Cleanup(func() {
		releaseWriter()
		<-writerResult
	})
	select {
	case <-writerTracer.attempted:
	case <-ctx.Done():
		t.Fatalf("writer never reached the refs-mutation barrier: %v", ctx.Err())
	}

	repairTracer := &schemaRepairPIDTracer{captured: make(chan struct{})}
	repairConfig := poolConfig.Copy()
	repairConfig.ConnConfig.Tracer = repairTracer
	type repairOutcome struct {
		store *PostgresSymbolStore
		err   error
	}
	repairResult := make(chan repairOutcome, 1)
	go func() {
		defer close(repairResult)
		store, err := newPostgresSymbolStoreWithPoolConfig(ctx, repairConfig, "schema-project", t.TempDir())
		repairResult <- repairOutcome{store, err}
	}()
	// LIFO runs this cleanup first: unblock the paused writer and cancel the
	// bounded ctx before joining the repair goroutine, so an early fatal can
	// never wait out the full 30s while the repair holds a lock wait behind
	// the writer.
	t.Cleanup(func() {
		releaseWriter()
		cancel()
		if outcome := <-repairResult; outcome.store != nil {
			outcome.store.Close()
		}
	})
	select {
	case <-repairTracer.captured:
	case <-ctx.Done():
		t.Fatalf("repair never attempted the schema advisory lock: %v", ctx.Err())
	}
	// Provable lock state before releasing the writer: the schema
	// transaction is blocked on the symbols table lock. This holds under
	// both lock orders; what differs is what the repair already holds.
	waitForUngrantedTableLock(t, ctx, seed.pool, repairTracer.pid.Load(), seed.schema, "symbols", "ShareRowExclusiveLock")

	releaseWriter()
	if err := <-writerResult; err != nil {
		t.Fatalf("writer deadlocked against schema repair: %v", err)
	}
	outcome := <-repairResult
	if outcome.err != nil {
		t.Fatalf("schema repair deadlocked against writer: %v", outcome.err)
	}
	if outcome.store == nil {
		t.Fatal("schema repair returned no store")
	}
	// The consumed store is out of the channel, so the cleanup above sees a
	// zero outcome; close it here on both the success and the fatal paths.
	defer outcome.store.Close()

	requireRefsCallerIndex(t, outcome.store)
	if got := storedSchemaVersion(t, outcome.store); got != currentSymbolSchemaVersion {
		t.Fatalf("repair changed schema version to %d", got)
	}
	if got, err := outcome.store.GetSymbolsForFile(ctx, "main.go"); err != nil || len(got) != 0 {
		t.Fatalf("deleted file symbols remain: %#v, %v", got, err)
	}
	if got, err := outcome.store.LookupCallers(ctx, "MainTarget"); err != nil || len(got) != 0 {
		t.Fatalf("deleted file refs remain: %#v, %v", got, err)
	}
	if got, err := outcome.store.GetSymbolsForFile(ctx, "keep.go"); err != nil || len(got) != 1 || got[0].Name != "Keep" {
		t.Fatalf("repair disturbed untouched file symbols: %#v, %v", got, err)
	}
	if got, err := outcome.store.LookupCallers(ctx, "KeepTarget"); err != nil || len(got) != 1 {
		t.Fatalf("repair disturbed untouched file refs: %#v, %v", got, err)
	}
	stats, err := outcome.store.GetStats(ctx)
	if err != nil || stats.TotalFiles != 1 || stats.TotalSymbols != 1 || stats.TotalReferences != 1 {
		t.Fatalf("post-repair stats = %#v, %v", stats, err)
	}
}
