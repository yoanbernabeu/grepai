package trace

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func projectMutationTime(t *testing.T, store *PostgresSymbolStore) time.Time {
	t.Helper()
	var value time.Time
	if err := store.pool.QueryRow(context.Background(), `SELECT last_mutation_at FROM symbol_migrations WHERE project_id=$1`, identityBytes(store.projectID)).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func setProjectMutationTime(t *testing.T, store *PostgresSymbolStore, value time.Time) {
	t.Helper()
	if _, err := store.pool.Exec(context.Background(), `UPDATE symbol_migrations SET last_mutation_at=$2 WHERE project_id=$1`, identityBytes(store.projectID), value); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresMutationTimestampSurvivesDeletes(t *testing.T) {
	store := newIsolatedSchemaStore(t, isolatedSymbolSchemaConfig(t))
	ctx := context.Background()
	for _, file := range []string{"old.go", "new.go"} {
		if err := store.SaveFile(ctx, file, []Symbol{{Name: file, File: file, Line: 1}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	baseline := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	setProjectMutationTime(t, store, baseline)
	if _, err := store.pool.Exec(ctx, `UPDATE symbol_files SET mod_time=CASE WHEN path=$2 THEN '2030-01-01'::timestamptz ELSE '2010-01-01'::timestamptz END WHERE project_id=$1`, identityBytes(store.projectID), identityBytes("new.go")); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteFile(ctx, "new.go"); err != nil {
		t.Fatal(err)
	}
	afterNewest := projectMutationTime(t, store)
	if !afterNewest.After(baseline) {
		t.Fatalf("newest-file delete timestamp=%v, want after %v", afterNewest, baseline)
	}
	stats, err := store.GetStats(ctx)
	if err != nil || !stats.LastUpdated.Equal(afterNewest) || stats.TotalFiles != 1 {
		t.Fatalf("stats after newest delete=%#v, %v", stats, err)
	}
	if err := store.DeleteFile(ctx, "old.go"); err != nil {
		t.Fatal(err)
	}
	afterFinal := projectMutationTime(t, store)
	if !afterFinal.After(afterNewest) {
		t.Fatalf("final delete moved timestamp backward: %v -> %v", afterNewest, afterFinal)
	}
	stats, err = store.GetStats(ctx)
	if err != nil || stats.TotalFiles != 0 || !stats.LastUpdated.Equal(afterFinal) {
		t.Fatalf("stats after final delete=%#v, %v", stats, err)
	}
	if err := store.DeleteFile(ctx, "absent.go"); err != nil {
		t.Fatal(err)
	}
	if got := projectMutationTime(t, store); !got.Equal(afterFinal) {
		t.Fatalf("absent delete changed timestamp: %v -> %v", afterFinal, got)
	}
}

func TestPostgresMutationTimestampRollbackAndTenantIsolation(t *testing.T) {
	cfg := isolatedSymbolSchemaConfig(t)
	one := newIsolatedSchemaStore(t, cfg)
	two, err := newPostgresSymbolStoreWithPoolConfig(context.Background(), cfg.Copy(), "other-project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { two.Close() })
	if err := two.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	baseline := time.Date(2020, 2, 2, 0, 0, 0, 0, time.UTC)
	setProjectMutationTime(t, one, baseline)
	setProjectMutationTime(t, two, baseline)
	one.mutationHook = func(string, string) error { return errors.New("fail before mutation") }
	if err := one.SaveFile(context.Background(), "failed.go", nil, nil); err == nil {
		t.Fatal("expected mutation hook failure")
	}
	one.mutationHook = nil
	if got := projectMutationTime(t, one); !got.Equal(baseline) {
		t.Fatalf("failed save changed timestamp: %v", got)
	}
	if err := one.SaveFile(context.Background(), "ok.go", nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := projectMutationTime(t, one); !got.After(baseline) {
		t.Fatalf("successful save timestamp=%v", got)
	}
	if got := projectMutationTime(t, two); !got.Equal(baseline) {
		t.Fatalf("other tenant timestamp changed: %v", got)
	}
}

type statsQueryTracer struct {
	mu      sync.Mutex
	selects int
}

func (tr *statsQueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.HasPrefix(strings.TrimSpace(strings.ToUpper(data.SQL)), "SELECT") {
		tr.mu.Lock()
		tr.selects++
		tr.mu.Unlock()
	}
	return ctx
}

func (*statsQueryTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestPostgresGetStatsUsesOneSnapshotStatement(t *testing.T) {
	cfg := isolatedSymbolSchemaConfig(t)
	tracer := &statsQueryTracer{}
	cfg.ConnConfig.Tracer = tracer
	store := newIsolatedSchemaStore(t, cfg)
	tracer.mu.Lock()
	tracer.selects = 0
	tracer.mu.Unlock()
	stats, err := store.GetStats(context.Background())
	if err != nil || !stats.LastUpdated.After(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("activated empty project stats=%#v, %v", stats, err)
	}
	tracer.mu.Lock()
	defer tracer.mu.Unlock()
	if tracer.selects != 1 {
		t.Fatalf("GetStats SELECT statements=%d, want 1", tracer.selects)
	}
}

func TestPostgresDifferentFilesMutateConcurrentlyUntilMetadataCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := isolatedSymbolSchemaConfig(t)
	one := newIsolatedSchemaStore(t, cfg)
	two, err := newPostgresSymbolStoreWithPoolConfig(ctx, cfg.Copy(), one.projectID, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer two.Close()
	attempted := make(chan string, 2)
	release := make(chan struct{})
	releaseAll := sync.OnceFunc(func() { close(release) })
	hook := func(_ string, file string) error {
		attempted <- file
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	one.mutationHook = hook
	two.mutationHook = hook
	results := make(chan error, 2)
	var workers sync.WaitGroup
	start := func(store *PostgresSymbolStore, file string) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			results <- store.SaveFile(ctx, file, nil, nil)
		}()
	}
	defer func() {
		releaseAll()
		cancel()
		workers.Wait()
	}()
	start(one, "one.go")
	select {
	case <-attempted:
	case <-ctx.Done():
		t.Fatal("first mutation did not reach data barrier")
	}
	start(two, "two.go")
	select {
	case <-attempted:
	case <-ctx.Done():
		t.Fatal("different-file mutation was serialized before data work")
	}
	releaseAll()
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

func TestPostgresMutationTimestampRollsBackAfterMetadataUpdate(t *testing.T) {
	store := newIsolatedSchemaStore(t, isolatedSymbolSchemaConfig(t))
	baseline := time.Date(2020, 3, 3, 0, 0, 0, 0, time.UTC)
	setProjectMutationTime(t, store, baseline)
	tx, err := store.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := store.requireProjectActivation(context.Background(), tx, "save"); err != nil {
		t.Fatal(err)
	}
	if err := store.saveFileTx(context.Background(), tx, "rollback.go", "", nil, []Symbol{{Name: "Rollback", File: "rollback.go", Line: 1}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.recordProjectMutation(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	var inside time.Time
	if err := tx.QueryRow(context.Background(), `SELECT last_mutation_at FROM symbol_migrations WHERE project_id=$1`, identityBytes(store.projectID)).Scan(&inside); err != nil {
		t.Fatal(err)
	}
	if !inside.After(baseline) {
		t.Fatalf("transaction did not update metadata: %v", inside)
	}
	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := projectMutationTime(t, store); !got.Equal(baseline) {
		t.Fatalf("rolled-back metadata persisted: %v", got)
	}
	if store.IsFileIndexed("rollback.go") {
		t.Fatal("rolled-back file data persisted")
	}
}
