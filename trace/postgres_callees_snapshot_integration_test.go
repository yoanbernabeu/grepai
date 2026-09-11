package trace

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// calleeRefsBarrierTracer pauses the callee refs SELECT at the exact moment
// pgx is about to execute it, so a test can commit a new file generation
// strictly between the edges read and the refs read. PoolConfig.ConnConfig
// .Tracer makes this deterministic without production test hooks.
type calleeRefsBarrierTracer struct {
	refsAttempted chan struct{}
	release       chan struct{}
	onRefs        func()
	once          sync.Once
}

func (tr *calleeRefsBarrierTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	// Only the callee refs query: it uniquely selects from refs by caller.
	if strings.Contains(data.SQL, "FROM refs WHERE project_id=$1 AND caller=$2") {
		tr.once.Do(func() {
			if tr.onRefs != nil {
				tr.onRefs()
			}
			close(tr.refsAttempted)
			select {
			case <-tr.release:
			case <-ctx.Done():
			}
		})
	}
	return ctx
}

func (*calleeRefsBarrierTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

var _ pgx.QueryTracer = (*calleeRefsBarrierTracer)(nil)

func calleeGenRefs(generation string) []Reference {
	return []Reference{
		{SymbolName: "Shared", Kind: RefKindCall, File: "main.go", Line: 10, Context: generation, CallerName: "Main", CallerFile: "main.go", CallerLine: 1},
		{SymbolName: "OnlyOld", Kind: RefKindCall, File: "main.go", Line: 11, Context: generation, CallerName: "Main", CallerFile: "main.go", CallerLine: 1},
	}
}

// TestPostgresLookupCalleesUsesOneSnapshot commits a second generation of
// main.go strictly after the edges read and before the refs read. Both reads
// must observe one REPEATABLE READ snapshot: the result is exactly generation
// 1, never a generation-2 ref paired with a generation-1 edge fallback.
func TestPostgresLookupCalleesUsesOneSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	poolConfig := isolatedSymbolSchemaConfig(t)
	// Separate store with a separate pool (separate server connections) so
	// the interfering commit cannot share the reader's transaction state.
	writer, err := newPostgresSymbolStoreWithPoolConfig(ctx, poolConfig.Copy(), "schema-project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { writer.Close() })
	if err := writer.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if err := writer.SaveFile(ctx, "main.go", []Symbol{{Name: "Main", Kind: KindFunction, File: "main.go", Line: 1}}, calleeGenRefs("gen1")); err != nil {
		t.Fatal(err)
	}

	tracer := &calleeRefsBarrierTracer{refsAttempted: make(chan struct{}), release: make(chan struct{})}
	poolConfig.ConnConfig.Tracer = tracer
	reader := newIsolatedSchemaStore(t, poolConfig)

	type outcome struct {
		refs []Reference
		err  error
	}
	result := make(chan outcome, 1)
	releaseReader := sync.OnceFunc(func() { close(tracer.release) })
	go func() {
		defer close(result)
		refs, err := reader.LookupCallees(ctx, "Main", "main.go")
		result <- outcome{refs, err}
	}()
	t.Cleanup(func() {
		cancel()
		releaseReader()
		<-result
	})

	select {
	case <-tracer.refsAttempted:
	case got := <-result:
		t.Fatalf("lookup ended before the reference-read barrier: %v", got.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Generation 2 is durable before the refs read executes. Same site at
	// line 10 with new data; line 11 replaced by a symbol the old edges do
	// not reference.
	if err := writer.SaveFile(ctx, "main.go", []Symbol{{Name: "Main", Kind: KindFunction, File: "main.go", Line: 1}}, []Reference{
		{SymbolName: "Shared", Kind: RefKindCall, File: "main.go", Line: 10, Context: "gen2", CallerName: "Main", CallerFile: "main.go", CallerLine: 1},
		{SymbolName: "OnlyNew", Kind: RefKindCall, File: "main.go", Line: 11, Context: "gen2", CallerName: "Main", CallerFile: "main.go", CallerLine: 1},
	}); err != nil {
		t.Fatal(err)
	}
	releaseReader()

	got := <-result
	if got.err != nil {
		t.Fatal(got.err)
	}
	want := []Reference{calleeGenRefs("gen1")[0], calleeGenRefs("gen1")[1]}
	if !reflect.DeepEqual(got.refs, want) {
		t.Fatalf("LookupCallees mixed snapshots instead of one generation:\n got: %#v\nwant: %#v", got.refs, want)
	}
	for _, ref := range got.refs {
		if ref.Context == "" {
			t.Fatalf("cross-snapshot skew synthesized a fallback reference: %#v", ref)
		}
		if ref.SymbolName == "OnlyNew" {
			t.Fatalf("refs snapshot leaked past the edges snapshot: %#v", got.refs)
		}
	}
}

// TestPostgresLookupCalleesCanceledContextCleansUp cancels strictly during
// the refs read so the transaction is already open: the call must surface the
// cancellation, release the transaction/connection, and leave the store
// usable (an aborted transaction left on a pooled connection would poison
// every later query).
func TestPostgresLookupCalleesCanceledContextCleansUp(t *testing.T) {
	ctx := context.Background()
	poolConfig := isolatedSymbolSchemaConfig(t)
	callCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	tracer := &calleeRefsBarrierTracer{refsAttempted: make(chan struct{}), release: make(chan struct{})}
	tracer.onRefs = cancel
	close(tracer.release)
	poolConfig.ConnConfig.Tracer = tracer
	store := newIsolatedSchemaStore(t, poolConfig)
	if err := store.SaveFile(ctx, "main.go", []Symbol{{Name: "Main", Kind: KindFunction, File: "main.go", Line: 1}}, calleeGenRefs("gen1")); err != nil {
		t.Fatal(err)
	}

	_, err := store.LookupCallees(callCtx, "Main", "main.go")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled LookupCallees error = %v, want context.Canceled cause", err)
	}

	if acq := store.pool.Stat().AcquiredConns(); acq != 0 {
		t.Fatalf("canceled LookupCallees left %d pooled connections acquired", acq)
	}
	// A busy connection or an aborted server-side transaction would fail or
	// return "current transaction is aborted" here; the same data must come
	// back intact.
	after, err := store.LookupCallees(ctx, "Main", "main.go")
	if err != nil || len(after) != 2 || after[0].Context != "gen1" {
		t.Fatalf("store unusable after canceled LookupCallees: %#v, %v", after, err)
	}
}
