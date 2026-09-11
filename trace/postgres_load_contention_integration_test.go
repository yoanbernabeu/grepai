package trace

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/internal/fileutil"
)

// Regression tests for the Load contention wait: a caller without a deadline
// must never poll forever behind a stale GOB watcher that holds the lifetime
// project writer lock but can no longer publish the PG migration marker.

func TestPostgresLoadNoDeadlineBoundedByDefaultBudget(t *testing.T) {
	root := t.TempDir()
	writeMigrationGOB(t, root, 1)
	store := newIntegrationSymbolStore(t, "load-budget-default", root)
	truncateSymbolTablesUnactivated(t, store)

	// A stale writer holds the lifetime lock and never finishes the marker.
	held, err := fileutil.AcquireProjectWriterLock(root)
	if err != nil {
		t.Fatal(err)
	}
	// Close is idempotent, so this backstop releases the lock on the success
	// path too, where the join defer below is a no-op.
	t.Cleanup(func() { _ = held.Close() })
	errCh := make(chan error, 1)
	start := time.Now()
	// The caller passes a deadline-free context, as CLI Load callers do.
	go func() { errCh <- store.Load(context.Background()) }()
	joined := false
	// Always release the lock and join the goroutine, even on failure: once
	// the lock is released a stuck Load migrates and returns, so the drain
	// terminates. The join bound below is generous enough for the correct
	// 30s budget, so a regression to unbounded polling fails here rather
	// than at the go test timeout.
	defer func() {
		if !joined {
			_ = held.Close()
			<-errCh
		}
	}()
	select {
	case err = <-errCh:
		joined = true
	case <-time.After(2 * migrationWriterWaitBudget):
		t.Fatalf("Load did not return within twice the %s default wait budget", migrationWriterWaitBudget)
	}
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Load with a never-completing writer returned nil")
	}
	var activeErr *fileutil.ProjectWriterActiveError
	if !errors.As(err, &activeErr) {
		t.Fatalf("Load error = %T %v, want errors.As(*ProjectWriterActiveError)", err, err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Load error = %v, want errors.Is(context.DeadlineExceeded)", err)
	}
	if !strings.Contains(err.Error(), "watcher") {
		t.Fatalf("Load error = %q, want actionable stop/restart watcher guidance", err)
	}
	if elapsed < migrationWriterWaitBudget {
		t.Fatalf("Load returned after %s, before the %s default budget", elapsed, migrationWriterWaitBudget)
	}
	// The bounded wait must not have imported anything or touched the GOB.
	var rows int
	if qerr := store.pool.QueryRow(context.Background(), `SELECT (SELECT COUNT(*) FROM symbols WHERE project_id=$1)+(SELECT COUNT(*) FROM symbol_files WHERE project_id=$1)`, identityBytes(store.projectID)).Scan(&rows); qerr != nil || rows != 0 {
		t.Fatalf("bounded wait imported %d rows: %v", rows, qerr)
	}
	if _, serr := os.Stat(config.GetSymbolIndexPath(root)); serr != nil {
		t.Fatalf("source GOB missing after bounded wait: %v", serr)
	}
}

func TestPostgresLoadReaderSkipsLifetimeWriterAfterMigration(t *testing.T) {
	root := t.TempDir()
	writeMigrationGOB(t, root, 2)
	writer := newIntegrationSymbolStore(t, "load-steady-reader", root)
	truncateSymbolTablesUnactivated(t, writer)
	if err := writer.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	// A live PG watcher then owns the lifetime writer lock forever.
	held, err := fileutil.AcquireProjectWriterLock(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = held.Close() })

	// A steady-state reader with a completed migration returns immediately,
	// without ever waiting on the watcher or arming the default budget.
	reader := newIntegrationSymbolStore(t, "load-steady-reader", root)
	start := time.Now()
	if err := reader.Load(context.Background()); err != nil {
		t.Fatalf("steady-state reader Load: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("steady-state reader waited %s behind a completed migration", elapsed)
	}
}

func TestPostgresLoadWaiterFreedByConcurrentMigrationCompletion(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeMigrationGOB(t, root, 2)
	reader := newIntegrationSymbolStore(t, "load-wait-freed", root)
	writer := newIntegrationSymbolStore(t, "load-wait-freed", root)
	truncateSymbolTablesUnactivated(t, reader)

	// The writer holds the lifetime lock like a live watcher that will still
	// run its initial migration via LoadWithProjectWriterLockHeld.
	held, err := fileutil.AcquireProjectWriterLock(root)
	if err != nil {
		t.Fatal(err)
	}
	// Backstop release for fatals after the goroutine joined but before the
	// explicit Close below; Close is idempotent, so this never double-fails.
	t.Cleanup(func() { _ = held.Close() })
	errCh := make(chan error, 1)
	go func() { errCh <- reader.Load(context.Background()) }()
	joined := false
	defer func() {
		if !joined {
			_ = held.Close()
			<-errCh
		}
	}()

	// The reader waits for the writer instead of failing fast.
	select {
	case err := <-errCh:
		joined = true
		t.Fatalf("reader Load returned while writer lock held: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	// Once the concurrent writer completes the migration marker, the reader
	// finishes without ever acquiring the lifetime lock or hitting a budget.
	if err := writer.LoadWithProjectWriterLockHeld(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		joined = true
		if err != nil {
			t.Fatalf("reader Load after concurrent migration: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("reader still blocked after concurrent migration completed")
	}
	if err := held.Close(); err != nil {
		t.Fatal(err)
	}
	if stats, err := reader.GetStats(ctx); err != nil || stats.TotalFiles != 2 || stats.TotalSymbols != 2 {
		t.Fatalf("reader data after concurrent migration = %#v, %v", stats, err)
	}
}
