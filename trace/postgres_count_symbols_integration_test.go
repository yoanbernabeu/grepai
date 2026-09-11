package trace

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

type countSymbolsQueryTracer struct {
	mu      sync.Mutex
	queries []string
}

func (t *countSymbolsQueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	t.mu.Lock()
	t.queries = append(t.queries, data.SQL)
	t.mu.Unlock()
	return ctx
}

func (*countSymbolsQueryTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestPostgresCountSymbolsUsesOneProjectScopedCount(t *testing.T) {
	poolConfig := isolatedSymbolSchemaConfig(t)
	tracer := &countSymbolsQueryTracer{}
	poolConfig.ConnConfig.Tracer = tracer
	store := newIsolatedSchemaStore(t, poolConfig)
	other := &PostgresSymbolStore{pool: store.pool, projectID: "other-project"}
	activateSymbolProject(t, other)
	ctx := context.Background()
	if err := store.SaveFile(ctx, "one.go", []Symbol{{Name: "One", File: "one.go"}, {Name: "Two", File: "one.go"}}, []Reference{{SymbolName: "One", File: "one.go"}}); err != nil {
		t.Fatal(err)
	}
	if err := other.SaveFile(ctx, "other.go", []Symbol{{Name: "Other", File: "other.go"}}, nil); err != nil {
		t.Fatal(err)
	}

	tracer.mu.Lock()
	tracer.queries = nil
	tracer.mu.Unlock()
	got, err := store.CountSymbols(ctx)
	if err != nil || got != 2 {
		t.Fatalf("CountSymbols = %d, %v", got, err)
	}
	tracer.mu.Lock()
	queries := append([]string(nil), tracer.queries...)
	tracer.mu.Unlock()
	if len(queries) != 1 {
		t.Fatalf("queries = %#v", queries)
	}
	query := strings.ToLower(queries[0])
	if !strings.Contains(query, "count(*) from symbols where project_id=$1") {
		t.Fatalf("query is not project-scoped symbol count: %q", queries[0])
	}
	for _, forbidden := range []string{"pg_column_size", "refs", "call_edges", "symbol_files"} {
		if strings.Contains(query, forbidden) {
			t.Fatalf("query contains %q aggregation: %q", forbidden, queries[0])
		}
	}
}

func TestPostgresCountSymbolsReturnsContextError(t *testing.T) {
	store := newIsolatedSchemaStore(t, isolatedSymbolSchemaConfig(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := store.CountSymbols(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CountSymbols error = %v", err)
	}
}

var _ pgx.QueryTracer = (*countSymbolsQueryTracer)(nil)
