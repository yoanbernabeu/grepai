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

type readResultSymbolsBarrier struct {
	attempted chan struct{}
	release   chan struct{}
	onQuery   func()
	once      sync.Once
}

func (tr *readResultSymbolsBarrier) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
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
func (*readResultSymbolsBarrier) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestPostgresLookupCalleeResultKeepsDefinitionsWithReferences(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	cfg := isolatedSymbolSchemaConfig(t)
	writer := newIsolatedSchemaStore(t, cfg)
	if err := writer.SaveFile(ctx, "main.go", []Symbol{{Name: "Main", File: "main.go", Docstring: "old"}, {Name: "Callee", File: "old.go", Docstring: "old"}}, []Reference{{SymbolName: "Callee", Kind: RefKindCall, CallerName: "Main", File: "main.go", Line: 2, Context: "old"}}); err != nil {
		t.Fatal(err)
	}
	barrier := &readResultSymbolsBarrier{attempted: make(chan struct{}), release: make(chan struct{})}
	cfg.ConnConfig.Tracer = barrier
	reader := newIsolatedSchemaStore(t, cfg)
	type outcome struct {
		result CalleeLookupResult
		err    error
	}
	done := make(chan outcome, 1)
	release := sync.OnceFunc(func() { close(barrier.release) })
	go func() {
		defer close(done)
		got, err := reader.LookupCalleeResult(ctx, "Main", "")
		done <- outcome{got, err}
	}()
	t.Cleanup(func() { cancel(); release(); <-done })
	select {
	case <-barrier.attempted:
	case got := <-done:
		t.Fatalf("lookup ended before barrier: %v", got.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := writer.SaveFile(ctx, "main.go", []Symbol{{Name: "Main", File: "main.go", Docstring: "new"}, {Name: "Callee", File: "new.go", Docstring: "new"}}, []Reference{{SymbolName: "Callee", Kind: RefKindCall, CallerName: "Main", File: "main.go", Line: 2, Context: "new"}}); err != nil {
		t.Fatal(err)
	}
	release()
	got := <-done
	if got.err != nil || len(got.result.References) != 1 || got.result.References[0].Context != "old" || got.result.Symbols["Main"][0].Docstring != "old" || got.result.Symbols["Callee"][0].File != "old.go" {
		t.Fatalf("callee result mixed generations: %#v, %v", got.result, got.err)
	}
}

func TestPostgresLookupRefsResultKeepsKindsWithCallerDefinitions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	cfg := isolatedSymbolSchemaConfig(t)
	writer := newIsolatedSchemaStore(t, cfg)
	if err := writer.SaveFile(ctx, "owner.go", []Symbol{{Name: "Owner", File: "old.go", Docstring: "old"}}, []Reference{{SymbolName: "uid", Kind: RefKindRead, CallerName: "Owner", CallerFile: "old.go", Context: "old"}}); err != nil {
		t.Fatal(err)
	}
	barrier := &readResultSymbolsBarrier{attempted: make(chan struct{}), release: make(chan struct{})}
	cfg.ConnConfig.Tracer = barrier
	reader := newIsolatedSchemaStore(t, cfg)
	type outcome struct {
		result RefsLookupResult
		err    error
	}
	done := make(chan outcome, 1)
	release := sync.OnceFunc(func() { close(barrier.release) })
	go func() { defer close(done); got, err := reader.LookupRefsResult(ctx, "uid"); done <- outcome{got, err} }()
	t.Cleanup(func() { cancel(); release(); <-done })
	select {
	case <-barrier.attempted:
	case got := <-done:
		t.Fatalf("lookup ended before barrier: %v", got.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := writer.SaveFile(ctx, "owner.go", []Symbol{{Name: "MovedOwner", File: "new.go", Docstring: "new"}}, []Reference{{SymbolName: "uid", Kind: RefKindWrite, CallerName: "MovedOwner", CallerFile: "new.go", Context: "new"}}); err != nil {
		t.Fatal(err)
	}
	release()
	got := <-done
	if got.err != nil || len(got.result.References) != 1 || got.result.References[0].Kind != RefKindRead || got.result.References[0].Context != "old" || got.result.Symbols["Owner"][0].File != "old.go" || len(got.result.Symbols["MovedOwner"]) != 0 {
		t.Fatalf("refs result mixed generations: %#v, %v", got.result, got.err)
	}
}

func TestPostgresLookupRefsResultCanceledReadCleansUp(t *testing.T) {
	ctx := context.Background()
	cfg := isolatedSymbolSchemaConfig(t)
	callCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	barrier := &readResultSymbolsBarrier{attempted: make(chan struct{}), release: make(chan struct{}), onQuery: cancel}
	close(barrier.release)
	cfg.ConnConfig.Tracer = barrier
	store := newIsolatedSchemaStore(t, cfg)
	if err := store.SaveFile(ctx, "owner.go", []Symbol{{Name: "Owner", File: "owner.go"}}, []Reference{{SymbolName: "uid", Kind: RefKindRead, CallerName: "Owner"}}); err != nil {
		t.Fatal(err)
	}
	_, err := store.LookupRefsResult(callCtx, "uid")
	if !errors.Is(err, context.Canceled) || store.pool.Stat().AcquiredConns() != 0 {
		t.Fatalf("canceled refs result error=%v acquired=%d", err, store.pool.Stat().AcquiredConns())
	}
}

func TestPostgresLookupCalleeResultCanceledReadCleansUp(t *testing.T) {
	ctx := context.Background()
	cfg := isolatedSymbolSchemaConfig(t)
	callCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	barrier := &readResultSymbolsBarrier{attempted: make(chan struct{}), release: make(chan struct{}), onQuery: cancel}
	close(barrier.release)
	cfg.ConnConfig.Tracer = barrier
	store := newIsolatedSchemaStore(t, cfg)
	if err := store.SaveFile(ctx, "main.go", []Symbol{{Name: "Main", File: "main.go"}, {Name: "Callee", File: "callee.go"}}, []Reference{{SymbolName: "Callee", Kind: RefKindCall, CallerName: "Main", File: "main.go", Line: 2}}); err != nil {
		t.Fatal(err)
	}
	_, err := store.LookupCalleeResult(callCtx, "Main", "")
	if !errors.Is(err, context.Canceled) || store.pool.Stat().AcquiredConns() != 0 {
		t.Fatalf("canceled callee result error=%v acquired=%d", err, store.pool.Stat().AcquiredConns())
	}
}
