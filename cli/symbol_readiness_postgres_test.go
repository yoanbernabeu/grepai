package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/trace"
)

// readinessPG sets up a Postgres-backed project and returns helpers that hold
// symbol_files locked. Postgres GetStats aggregates symbol_files (and the
// other tables), so any readiness check that still walks full stats blocks on
// this lock until the 2s statement_timeout baked into the isolated DSN fires;
// a count-only readiness check never touches symbol_files and completes.
type readinessPG struct {
	admin  *pgxpool.Pool
	schema string
}

func newReadinessPG(t *testing.T) *readinessPG {
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
	schema := fmt.Sprintf("grepai_cli_readiness_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+pgx.Identifier{schema}.Sanitize()+` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	return &readinessPG{admin: admin, schema: schema}
}

func (r *readinessPG) isolatedDSN(t *testing.T) string {
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

// seedProject creates a Postgres-backed project in the isolated schema,
// migrates the store (so later Loads skip the migration path), and optionally
// writes symbols, call references, and property references.
func (r *readinessPG) seedProject(t *testing.T, name string, withData bool) string {
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
	if withData {
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
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return root
}

// holdStatsTablesLocked takes ACCESS EXCLUSIVE on symbol_files. GetStats'
// file-count query blocks on it; symbol/ref-only queries do not.
func (r *readinessPG) holdStatsTablesLocked(t *testing.T) func() {
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

func chdirReadinessProject(t *testing.T, root string) {
	t.Helper()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
}

func TestTraceCommandReadinessPostgresAvoidsFullStats(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"callers", func() error { return runTraceCallers(nil, []string{"Target"}) }},
		{"callees", func() error { return runTraceCallees(nil, []string{"Target"}) }},
		{"graph", func() error { return runTraceGraph(nil, []string{"Target"}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pg := newReadinessPG(t)
			root := pg.seedProject(t, tc.name, true)
			release := pg.holdStatsTablesLocked(t)
			defer release()
			chdirReadinessProject(t, root)
			setTraceCommandFlags(t, "", "", false)

			output, err := captureCommandStdout(t, tc.run)
			if err != nil {
				t.Fatalf("%s readiness failed while symbol_files was locked (full GetStats still on this path): %v\noutput:\n%s", tc.name, err, output)
			}
			var result trace.TraceResult
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				t.Fatalf("%s output is not a trace result: %v\n%s", tc.name, err, output)
			}
			if result.Query != "Target" {
				t.Fatalf("%s result query = %q:\n%s", tc.name, result.Query, output)
			}
		})
	}
}

func TestRefsCommandReadinessPostgresAvoidsFullStats(t *testing.T) {
	originalWorkspace, originalProject := refsWorkspace, refsProject
	refsWorkspace, refsProject = "", ""
	t.Cleanup(func() { refsWorkspace, refsProject = originalWorkspace, originalProject })
	for _, readers := range []bool{true, false} {
		name := "writers"
		if readers {
			name = "readers"
		}
		t.Run(name, func(t *testing.T) {
			pg := newReadinessPG(t)
			root := pg.seedProject(t, name, true)
			release := pg.holdStatsTablesLocked(t)
			defer release()
			chdirReadinessProject(t, root)

			result, err := runRefs("uid", readers)
			if err != nil {
				t.Fatalf("refs %s readiness failed while symbol_files was locked (full GetStats still on this path): %v", name, err)
			}
			usages := result.Readers
			if !readers {
				usages = result.Writers
			}
			if len(usages) != 1 || usages[0].AccessAt.File != "consumer.go" {
				t.Fatalf("unexpected %s result: %+v", name, result)
			}
		})
	}
}

func TestTraceCommandReadinessPostgresEmptyIndexMessage(t *testing.T) {
	pg := newReadinessPG(t)
	root := pg.seedProject(t, "empty", false)
	chdirReadinessProject(t, root)
	setTraceCommandFlags(t, "", "", true)

	_, err := captureCommandStdout(t, func() error { return runTraceCallers(nil, []string{"Target"}) })
	if err == nil || !strings.Contains(err.Error(), "symbol index is empty. Run 'grepai watch' first to build the index") {
		t.Fatalf("empty Postgres index error = %v, want preserved empty-index message", err)
	}
}

func TestTraceCommandReadinessGOBEmptyIndexMessage(t *testing.T) {
	root := t.TempDir()
	if err := config.DefaultConfig().Save(root); err != nil {
		t.Fatal(err)
	}
	chdirReadinessProject(t, root)
	setTraceCommandFlags(t, "", "", true)

	_, err := captureCommandStdout(t, func() error { return runTraceCallers(nil, []string{"Target"}) })
	if err == nil || !strings.Contains(err.Error(), "symbol index is empty. Run 'grepai watch' first to build the index") {
		t.Fatalf("empty GOB index error = %v, want preserved empty-index message", err)
	}
}
