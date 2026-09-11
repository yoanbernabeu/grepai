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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yoanbernabeu/grepai/internal/fileutil"
)

// loadContextTracer records, per relevant statement, whether the context the
// store actually handed to pgx carried a deadline. It distinguishes the
// bounded wait context (deadline) from the caller's original
// deadline-free context during contention and during the real CopyFrom
// import — with no sleeps and no production test hooks.
type loadContextTracer struct {
	mu                sync.Mutex
	boundedMarkerSeen chan struct{}
	seenOnce          sync.Once
	copyFromDeadline  []bool
}

func (t *loadContextTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	// The marker poll is the only SELECT against symbol_migrations issued by
	// the Load wait loop (migrationState reads it only after lock
	// acquisition, under the caller's original context).
	if strings.HasPrefix(data.SQL, "SELECT") && strings.Contains(data.SQL, "symbol_migrations") {
		if _, hasDeadline := ctx.Deadline(); hasDeadline {
			t.seenOnce.Do(func() { close(t.boundedMarkerSeen) })
		}
	}
	return ctx
}

func (t *loadContextTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (t *loadContextTracer) TraceCopyFromStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceCopyFromStartData) context.Context {
	_, hasDeadline := ctx.Deadline()
	t.mu.Lock()
	t.copyFromDeadline = append(t.copyFromDeadline, hasDeadline)
	t.mu.Unlock()
	return ctx
}

func (t *loadContextTracer) TraceCopyFromEnd(context.Context, *pgx.Conn, pgx.TraceCopyFromEndData) {}

// TestPostgresLoadBoundsMarkerPollingButNotImportContext proves two context
// contracts of the contended Load path at once:
//  1. Once contending, migration-marker re-checks run under the bounded wait
//     context (they carry the armed default-budget deadline), so a stalled
//     marker query cannot bypass the limit.
//  2. The actual GOB import — observed at the real CopyFrom calls — runs
//     under the caller's ORIGINAL deadline-free context, so the contention
//     budget never limits the import after lock acquisition.
func TestPostgresLoadBoundsMarkerPollingButNotImportContext(t *testing.T) {
	dsn := os.Getenv("GREPAI_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("GREPAI_POSTGRES_TEST_DSN is not set")
	}
	root := t.TempDir()
	writeMigrationGOB(t, root, 2)

	tracer := &loadContextTracer{boundedMarkerSeen: make(chan struct{})}
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	poolConfig.ConnConfig.Tracer = tracer
	store, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), poolConfig, "load-import-ctx", root)
	if err != nil {
		t.Fatalf("failed to create Postgres symbol store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	truncateSymbolTablesUnactivated(t, store)

	// A contending writer holds the lifetime project lock.
	held, err := fileutil.AcquireProjectWriterLock(root)
	if err != nil {
		t.Fatal(err)
	}
	// Deadline-free caller context, as CLI Loads pass.
	errCh := make(chan error, 1)
	go func() { errCh <- store.Load(context.Background()) }()
	joined := false
	defer func() {
		if !joined {
			_ = held.Close()
			<-errCh
		}
	}()

	// Deterministic barrier: the tracer fires only when a marker re-check
	// runs under the armed bounded wait context, proving Load is contending
	// and its marker polling is budget-bound (contract 1).
	select {
	case <-tracer.boundedMarkerSeen:
	case <-time.After(10 * time.Second):
		t.Fatal("no marker re-check observed under the bounded wait context")
	}
	if err := held.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		joined = true
		if err != nil {
			t.Fatalf("Load after writer release: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Load did not finish after writer release")
	}

	// Contract 2: every CopyFrom of the import ran deadline-free even though
	// the contention budget was armed — the import used the caller's
	// original context, not the bounded wait context.
	tracer.mu.Lock()
	defer tracer.mu.Unlock()
	if len(tracer.copyFromDeadline) == 0 {
		t.Fatal("no CopyFrom observed during the GOB import")
	}
	for i, hasDeadline := range tracer.copyFromDeadline {
		if hasDeadline {
			t.Fatalf("CopyFrom %d ran under a deadline-bounded context; the import must use the caller's original context", i)
		}
	}
	if stats, err := store.GetStats(context.Background()); err != nil || stats.TotalFiles != 2 {
		t.Fatalf("import data = %#v, %v", stats, err)
	}
}

// markerExpiryTracer forces one bounded migration-marker re-check to observe
// deadline expiry via the context returned from TraceQueryStart — pgx uses
// that context for the rest of the call, so expiry lands deterministically
// inside the marker query without waiting out the real 30s budget or racing
// a sleep.
type markerExpiryTracer struct {
	armedSeen  chan struct{}
	armedOnce  sync.Once
	inject     atomic.Bool
	injected   chan struct{}
	injectOnce sync.Once
	expiredCtx context.Context
}

func (t *markerExpiryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.HasPrefix(data.SQL, "SELECT") && strings.Contains(data.SQL, "symbol_migrations") {
		if _, hasDeadline := ctx.Deadline(); hasDeadline {
			t.armedOnce.Do(func() { close(t.armedSeen) })
			if t.inject.Load() {
				t.injectOnce.Do(func() { close(t.injected) })
				return t.expiredCtx
			}
		}
	}
	return ctx
}

func (t *markerExpiryTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// TestPostgresLoadMarkerRecheckDeadlineKeepsActiveWriterError covers the
// seed-1789099310812036755 flake: when the armed default budget expires
// during a marker re-check rather than in the wait timer, Load must still
// return the typed active-writer error, the context deadline cause, and the
// stop/restart watcher guidance.
func TestPostgresLoadMarkerRecheckDeadlineKeepsActiveWriterError(t *testing.T) {
	dsn := os.Getenv("GREPAI_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("GREPAI_POSTGRES_TEST_DSN is not set")
	}
	root := t.TempDir()
	writeMigrationGOB(t, root, 1)

	expiredCtx, expiredCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer expiredCancel()
	tracer := &markerExpiryTracer{armedSeen: make(chan struct{}), injected: make(chan struct{}), expiredCtx: expiredCtx}
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	poolConfig.ConnConfig.Tracer = tracer
	store, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), poolConfig, "load-marker-expiry", root)
	if err != nil {
		t.Fatalf("failed to create Postgres symbol store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	truncateSymbolTablesUnactivated(t, store)

	// Contention persists for the whole test: Load must error while the
	// stale writer still holds the lifetime lock.
	held, err := fileutil.AcquireProjectWriterLock(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = held.Close() })
	// Deadline-free (so the default budget still arms) but cancelable, so
	// the goroutine is always joined promptly even on an early fatal.
	loadCtx, loadCancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- store.Load(loadCtx) }()
	joined := false
	defer func() {
		if !joined {
			loadCancel()
			<-errCh
		}
	}()

	// Barrier: the budget is armed once a bounded marker re-check is
	// observed; only then inject the expiry into the next re-check.
	select {
	case <-tracer.armedSeen:
	case <-time.After(10 * time.Second):
		t.Fatal("no bounded marker re-check observed; budget did not arm")
	}
	tracer.inject.Store(true)
	var loadErr error
	select {
	case loadErr = <-errCh:
		joined = true
	case <-time.After(10 * time.Second):
		t.Fatal("Load did not return after injected marker deadline expiry")
	}
	select {
	case <-tracer.injected:
	default:
		t.Fatal("Load returned before the injected marker deadline expiry")
	}

	if loadErr == nil {
		t.Fatal("Load with a never-completing writer returned nil")
	}
	var activeErr *fileutil.ProjectWriterActiveError
	if !errors.As(loadErr, &activeErr) {
		t.Fatalf("Load error = %T %v, want errors.As(*ProjectWriterActiveError)", loadErr, loadErr)
	}
	if !errors.Is(loadErr, context.DeadlineExceeded) {
		t.Fatalf("Load error = %v, want errors.Is(context.DeadlineExceeded)", loadErr)
	}
	if !strings.Contains(loadErr.Error(), "watcher") {
		t.Fatalf("Load error = %q, want actionable stop/restart watcher guidance", loadErr)
	}
}
