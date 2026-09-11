package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/trace"
)

func TestIndexStatusWorkspacePostgresAvoidsFullSymbolStats(t *testing.T) {
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
	schema := fmt.Sprintf("grepai_mcp_status_%d", time.Now().UnixNano())
	schemaSQL := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schemaSQL); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schemaSQL+` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	isolatedDSN := dsnWithSearchPath(t, dsn, schema)
	workspaceStore := config.StoreConfig{Backend: "postgres", Postgres: config.PostgresConfig{DSN: isolatedDSN}}

	home := isolateMCPTestHome(t)
	projects := make([]config.ProjectEntry, 0, 3)
	for i, name := range []string{"one", "two", "three"} {
		root := filepath.Join(home, name)
		cfg := config.DefaultConfig()
		cfg.Trace.StoreBackend = "postgres"
		if err := cfg.Save(root); err != nil {
			t.Fatal(err)
		}
		store, err := trace.NewSymbolStoreWithWorkspace(ctx, cfg, root, &workspaceStore)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Load(ctx); err != nil {
			store.Close()
			t.Fatal(err)
		}
		symbols := make([]trace.Symbol, i+1)
		for j := range symbols {
			symbols[j] = trace.Symbol{Name: fmt.Sprintf("%s-%d", name, j), File: name + ".go"}
		}
		if err := store.SaveFile(ctx, name+".go", symbols, nil); err != nil {
			store.Close()
			t.Fatal(err)
		}
		store.Close()
		projects = append(projects, config.ProjectEntry{Name: name, Path: root})
	}
	workspaceConfig := config.DefaultWorkspaceConfig()
	workspaceConfig.AddWorkspace(config.Workspace{
		Name:     "status-postgres",
		Store:    workspaceStore,
		Projects: projects,
	})
	if err := config.SaveWorkspaceConfig(workspaceConfig); err != nil {
		t.Fatal(err)
	}

	lock, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(context.Background())
	refsTable := pgx.Identifier{schema, "refs"}.Sanitize()
	if _, err := lock.Exec(ctx, `LOCK TABLE `+refsTable+` IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	statusCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := (&Server{workspaceName: "status-postgres"}).handleIndexStatus(statusCtx, traceHandlerRequest(map[string]any{"format": "json"}))
	if err != nil {
		t.Fatal(err)
	}
	var status WorkspaceIndexStatus
	if err := json.Unmarshal([]byte(textResultPayload(t, result)), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Projects) != 3 {
		t.Fatalf("projects = %#v", status.Projects)
	}
	for i, project := range status.Projects {
		if !project.SymbolsReady || project.TotalSymbols != i+1 {
			t.Fatalf("project %d status = %#v", i, project)
		}
	}
}

func dsnWithSearchPath(t *testing.T, dsn, schema string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") {
		query := u.Query()
		query.Set("search_path", schema)
		u.RawQuery = query.Encode()
		return u.String()
	}
	return dsn + " search_path=" + schema
}
