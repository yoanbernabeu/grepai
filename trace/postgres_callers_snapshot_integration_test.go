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

type callerSymbolsBarrierTracer struct {
	attempted chan struct{}
	release   chan struct{}
	onQuery   func()
	once      sync.Once
}

func (tr *callerSymbolsBarrierTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "FROM symbols WHERE project_id=$1 AND name=ANY") {
		tr.once.Do(func() {
			if tr.onQuery != nil {
				tr.onQuery()
			}
			close(tr.attempted)
			select {
			case <-tr.release:
			case <-ctx.Done():
			}
		})
	}
	return ctx
}

func (*callerSymbolsBarrierTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func seedCallerGeneration(t *testing.T, ctx context.Context, store *PostgresSymbolStore) {
	t.Helper()
	if err := store.SaveFile(ctx, "generation.go", []Symbol{
		{Name: "Target", File: "generation.go", Line: 1, Docstring: "gen1"},
		{Name: "Caller", File: "generation.go", Line: 10, Docstring: "gen1"},
	}, []Reference{{SymbolName: "Target", Kind: RefKindCall, File: "generation.go", Line: 11, Context: "gen1", CallerName: "Caller", CallerFile: "generation.go", CallerLine: 10}}); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresLookupCallerResultUsesOneSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	poolConfig := isolatedSymbolSchemaConfig(t)
	writer := newIsolatedSchemaStore(t, poolConfig)
	seedCallerGeneration(t, ctx, writer)

	tracer := &callerSymbolsBarrierTracer{attempted: make(chan struct{}), release: make(chan struct{})}
	poolConfig.ConnConfig.Tracer = tracer
	reader := newIsolatedSchemaStore(t, poolConfig)
	type outcome struct {
		result CallerLookupResult
		err    error
	}
	result := make(chan outcome, 1)
	release := sync.OnceFunc(func() { close(tracer.release) })
	go func() {
		defer close(result)
		got, err := reader.LookupCallerResult(ctx, "Target")
		result <- outcome{got, err}
	}()
	t.Cleanup(func() { release(); <-result })

	select {
	case <-tracer.attempted:
	case got := <-result:
		t.Fatalf("lookup ended before symbol barrier: %v", got.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := writer.SaveFile(ctx, "generation.go", []Symbol{
		{Name: "Target", File: "generation.go", Line: 2, Docstring: "gen2"},
		{Name: "MovedCaller", File: "generation.go", Line: 20, Docstring: "gen2"},
	}, []Reference{{SymbolName: "Target", Kind: RefKindCall, File: "generation.go", Line: 21, Context: "gen2", CallerName: "MovedCaller", CallerFile: "generation.go", CallerLine: 20}}); err != nil {
		t.Fatal(err)
	}
	release()

	got := <-result
	if got.err != nil {
		t.Fatal(got.err)
	}
	targets, callers := got.result.Symbols["Target"], got.result.Symbols["Caller"]
	if len(got.result.References) != 1 || got.result.References[0].Context != "gen1" || len(targets) != 1 || targets[0].Docstring != "gen1" || len(callers) != 1 || callers[0].Docstring != "gen1" {
		t.Fatalf("caller result mixed generations: %#v", got.result)
	}
}

func TestPostgresLookupCallerResultCanceledReadCleansUp(t *testing.T) {
	ctx := context.Background()
	poolConfig := isolatedSymbolSchemaConfig(t)
	callCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	tracer := &callerSymbolsBarrierTracer{attempted: make(chan struct{}), release: make(chan struct{}), onQuery: cancel}
	close(tracer.release)
	poolConfig.ConnConfig.Tracer = tracer
	store := newIsolatedSchemaStore(t, poolConfig)
	seedCallerGeneration(t, ctx, store)

	_, err := store.LookupCallerResult(callCtx, "Target")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled caller result error = %v, want context.Canceled", err)
	}
	if acquired := store.pool.Stat().AcquiredConns(); acquired != 0 {
		t.Fatalf("canceled caller result left %d connections acquired", acquired)
	}
	got, err := store.LookupCallerResult(ctx, "Target")
	if err != nil || len(got.References) != 1 {
		t.Fatalf("store unusable after cancellation: %#v, %v", got, err)
	}
}
