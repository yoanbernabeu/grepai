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

type graphSymbolsBarrierTracer struct {
	attempted chan struct{}
	release   chan struct{}
	onQuery   func()
	once      sync.Once
}

func (tr *graphSymbolsBarrierTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
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

func (*graphSymbolsBarrierTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

var _ pgx.QueryTracer = (*graphSymbolsBarrierTracer)(nil)

type graphOutcome struct {
	graph *CallGraph
	err   error
}

func startBarrierGraph(t *testing.T, ctx context.Context, store *PostgresSymbolStore, tracer *graphSymbolsBarrierTracer) (<-chan graphOutcome, func()) {
	t.Helper()
	result := make(chan graphOutcome, 1)
	release := sync.OnceFunc(func() { close(tracer.release) })
	go func() {
		graph, err := store.GetCallGraph(ctx, "Main", 2)
		result <- graphOutcome{graph: graph, err: err}
		close(result)
	}()
	t.Cleanup(func() {
		release()
		<-result
	})
	return result, release
}

func seedGraphGeneration(t *testing.T, ctx context.Context, store *PostgresSymbolStore) {
	t.Helper()
	for _, item := range []struct {
		file string
		sym  Symbol
		refs []Reference
	}{
		{"main.go", Symbol{Name: "Main", Kind: KindFunction, File: "main.go", Line: 1}, []Reference{{SymbolName: "Middle", Kind: RefKindCall, File: "main.go", Line: 2, CallerName: "Main"}}},
		{"middle.go", Symbol{Name: "Middle", Kind: KindFunction, File: "middle.go", Line: 1}, []Reference{{SymbolName: "OldLeaf", Kind: RefKindCall, File: "middle.go", Line: 2, CallerName: "Middle"}}},
		{"old.go", Symbol{Name: "OldLeaf", Kind: KindFunction, File: "old.go", Line: 1}, nil},
		{"new.go", Symbol{Name: "NewLeaf", Kind: KindFunction, File: "new.go", Line: 1}, nil},
	} {
		if err := store.SaveFile(ctx, item.file, []Symbol{item.sym}, item.refs); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPostgresGetCallGraphUsesOneSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	poolConfig := isolatedSymbolSchemaConfig(t)
	writer := newIsolatedSchemaStore(t, poolConfig)
	seedGraphGeneration(t, ctx, writer)

	tracer := &graphSymbolsBarrierTracer{attempted: make(chan struct{}), release: make(chan struct{})}
	poolConfig.ConnConfig.Tracer = tracer
	reader := newIsolatedSchemaStore(t, poolConfig)
	result, release := startBarrierGraph(t, ctx, reader, tracer)

	select {
	case <-tracer.attempted:
	case got := <-result:
		t.Fatalf("graph ended before symbol barrier: %v", got.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := writer.SaveFile(ctx, "middle.go", []Symbol{{Name: "Middle", Kind: KindFunction, File: "middle.go", Line: 20}}, []Reference{{SymbolName: "NewLeaf", Kind: RefKindCall, File: "middle.go", Line: 21, CallerName: "Middle"}}); err != nil {
		t.Fatal(err)
	}
	release()

	got := <-result
	if got.err != nil {
		t.Fatal(got.err)
	}
	if len(got.graph.Nodes) != 3 || len(got.graph.Edges) != 2 {
		t.Fatalf("incomplete graph: nodes=%#v edges=%#v", got.graph.Nodes, got.graph.Edges)
	}
	if got.graph.Nodes["Middle"].Line != 1 {
		t.Fatalf("graph symbol lookup leaked the new definition: %#v", got.graph.Nodes["Middle"])
	}
	if _, ok := got.graph.Nodes["OldLeaf"]; !ok {
		t.Fatalf("graph mixed generations: nodes=%#v edges=%#v", got.graph.Nodes, got.graph.Edges)
	}
	if _, ok := got.graph.Nodes["NewLeaf"]; ok || got.graph.Edges[1].Callee != "OldLeaf" {
		t.Fatalf("graph leaked new generation: nodes=%#v edges=%#v", got.graph.Nodes, got.graph.Edges)
	}
}

func TestPostgresGetCallGraphCanceledReadCleansUp(t *testing.T) {
	ctx := context.Background()
	poolConfig := isolatedSymbolSchemaConfig(t)
	callCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	tracer := &graphSymbolsBarrierTracer{attempted: make(chan struct{}), release: make(chan struct{}), onQuery: cancel}
	close(tracer.release)
	poolConfig.ConnConfig.Tracer = tracer
	store := newIsolatedSchemaStore(t, poolConfig)
	seedGraphGeneration(t, ctx, store)

	_, err := store.GetCallGraph(callCtx, "Main", 2)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled graph error=%v, want context.Canceled", err)
	}
	if acquired := store.pool.Stat().AcquiredConns(); acquired != 0 {
		t.Fatalf("canceled graph left %d connections acquired", acquired)
	}
	graph, err := store.GetCallGraph(ctx, "Main", 2)
	if err != nil || len(graph.Nodes) != 3 || len(graph.Edges) != 2 {
		t.Fatalf("store unusable after canceled graph: %#v, %v", graph, err)
	}
}
