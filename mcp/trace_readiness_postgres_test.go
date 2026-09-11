package mcp

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	mark3 "github.com/mark3labs/mcp-go/mcp"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/trace"
)

// mcpReadinessPG isolates one test run in its own schema. statement_timeout
// makes any lingering full GetStats call fail fast instead of hanging the
// suite while symbol_files is locked below.
type mcpReadinessPG struct {
	admin  *pgxpool.Pool
	schema string
}

func newMCPReadinessPG(t *testing.T) *mcpReadinessPG {
	t.Helper()
	dsn := os.Getenv("GREPAI_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("GREPAI_POSTGRES_TEST_DSN is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("grepai_mcp_readiness_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+pgx.Identifier{schema}.Sanitize()+` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	return &mcpReadinessPG{admin: admin, schema: schema}
}

func (r *mcpReadinessPG) isolatedDSN(t *testing.T) string {
	t.Helper()
	u, err := url.Parse(os.Getenv("GREPAI_POSTGRES_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", r.schema)
	q.Set("statement_timeout", "2000")
	u.RawQuery = q.Encode()
	return u.String()
}

func (r *mcpReadinessPG) seedProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Trace.StoreBackend = "postgres"
	cfg.Trace.Postgres.DSN = r.isolatedDSN(t)
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store, err := trace.NewSymbolStore(ctx, cfg, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Load(ctx); err != nil {
		store.Close()
		t.Fatal(err)
	}
	symbols := []trace.Symbol{
		{Name: "Target", File: "target.go", Line: 1},
		{Name: "Caller", File: "caller.go", Line: 2},
		{Name: "Callee", File: "callee.go", Line: 3},
		{Name: "uidConsumer", File: "consumer.go", Line: 4},
	}
	refs := []trace.Reference{
		{SymbolName: "Target", Kind: trace.RefKindCall, File: "calls.go", Line: 10, CallerName: "Caller", CallerFile: "caller.go", CallerLine: 2},
		{SymbolName: "Callee", Kind: trace.RefKindCall, File: "target.go", Line: 12, CallerName: "Target", CallerFile: "target.go", CallerLine: 1},
		{SymbolName: "uid", Kind: trace.RefKindRead, File: "consumer.go", Line: 14, CallerName: "uidConsumer", CallerFile: "consumer.go", CallerLine: 4},
		{SymbolName: "uid", Kind: trace.RefKindWrite, File: "consumer.go", Line: 15, CallerName: "uidConsumer", CallerFile: "consumer.go", CallerLine: 4},
	}
	if err := store.SaveFile(ctx, "seed.go", symbols, refs); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return root
}

func (r *mcpReadinessPG) emptyProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Trace.StoreBackend = "postgres"
	cfg.Trace.Postgres.DSN = r.isolatedDSN(t)
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store, err := trace.NewSymbolStore(ctx, cfg, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Load(ctx); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return root
}

// lockSymbolFiles blocks GetStats' file aggregation until statement_timeout.
func (r *mcpReadinessPG) lockSymbolFiles(t *testing.T) func() {
	t.Helper()
	ctx := context.Background()
	tx, err := r.admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stmt := `LOCK TABLE ` + pgx.Identifier{r.schema, "symbol_files"}.Sanitize() + ` IN ACCESS EXCLUSIVE MODE`
	if _, err := tx.Exec(ctx, stmt); err != nil {
		tx.Rollback(context.Background())
		t.Fatal(err)
	}
	return func() { _ = tx.Rollback(context.Background()) }
}

func TestMCPTraceReadinessPostgresAvoidsFullStats(t *testing.T) {
	handlers := map[string]struct {
		handler func(*Server, context.Context, mark3.CallToolRequest) (*mark3.CallToolResult, error)
		symbol  string
	}{
		"callers":      {(*Server).handleTraceCallers, "Target"},
		"callees":      {(*Server).handleTraceCallees, "Target"},
		"graph":        {(*Server).handleTraceGraph, "Target"},
		"refs_readers": {(*Server).handleRefsReaders, "uid"},
		"refs_writers": {(*Server).handleRefsWriters, "uid"},
		"refs_graph":   {(*Server).handleRefsGraph, "uid"},
	}
	names := []string{"callers", "callees", "graph", "refs_readers", "refs_writers", "refs_graph"}
	for _, name := range names {
		tc := handlers[name]
		t.Run(name, func(t *testing.T) {
			pg := newMCPReadinessPG(t)
			root := pg.seedProject(t)
			release := pg.lockSymbolFiles(t)
			defer release()

			result, err := tc.handler(&Server{projectRoot: root}, context.Background(), traceHandlerRequest(map[string]any{
				"symbol": tc.symbol,
				"format": "json",
			}))
			if err != nil {
				t.Fatal(err)
			}
			payload := textResultPayload(t, result)
			if result.IsError {
				t.Fatalf("%s readiness failed while symbol_files was locked (full GetStats still on this path): %s", name, payload)
			}
			if !strings.Contains(payload, tc.symbol) {
				t.Fatalf("%s output missing symbol payload:\n%s", name, payload)
			}
		})
	}
}

func TestMCPTraceReadinessPostgresPerRequestRootOverride(t *testing.T) {
	pg := newMCPReadinessPG(t)
	root := pg.seedProject(t)
	release := pg.lockSymbolFiles(t)
	defer release()

	// Server bound to an unrelated directory; the root parameter exercises the
	// per-request override path through the same readiness seam.
	result, err := (&Server{projectRoot: t.TempDir()}).handleTraceCallers(context.Background(), traceHandlerRequest(map[string]any{
		"symbol": "Target",
		"format": "json",
		"root":   root,
	}))
	if err != nil {
		t.Fatal(err)
	}
	payload := textResultPayload(t, result)
	if result.IsError {
		t.Fatalf("root-override readiness failed while symbol_files was locked (full GetStats still on this path): %s", payload)
	}
	if !strings.Contains(payload, "Target") {
		t.Fatalf("root-override output missing symbol payload:\n%s", payload)
	}
}

func TestMCPTraceReadinessPostgresEmptyIndexMessages(t *testing.T) {
	cases := []struct {
		name    string
		handler func(*Server, context.Context, mark3.CallToolRequest) (*mark3.CallToolResult, error)
		symbol  string
	}{
		{"callers", (*Server).handleTraceCallers, "Target"},
		{"refs_readers", (*Server).handleRefsReaders, "uid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pg := newMCPReadinessPG(t)
			root := pg.emptyProject(t)
			result, err := tc.handler(&Server{projectRoot: root}, context.Background(), traceHandlerRequest(map[string]any{
				"symbol": tc.symbol,
				"format": "json",
			}))
			if err != nil {
				t.Fatal(err)
			}
			payload := textResultPayload(t, result)
			if !result.IsError || !strings.Contains(payload, "symbol index is empty. Run 'grepai watch' first to build the index") {
				t.Fatalf("empty Postgres index result = (isError=%v) %q, want preserved empty-index error", result.IsError, payload)
			}
		})
	}
}
