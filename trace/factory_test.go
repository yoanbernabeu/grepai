package trace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
)

func TestCanonicalSymbolPostgresRootResolvesSymlink(t *testing.T) {
	parent := t.TempDir()
	realRoot := filepath.Join(parent, "real")
	if err := os.Mkdir(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	aliasRoot := filepath.Join(parent, "alias")
	if err := os.Symlink(realRoot, aliasRoot); err != nil {
		t.Fatal(err)
	}

	realID, err := canonicalSymbolPostgresRoot(realRoot)
	if err != nil {
		t.Fatal(err)
	}
	aliasID, err := canonicalSymbolPostgresRoot(aliasRoot)
	if err != nil {
		t.Fatal(err)
	}
	if aliasID != realID {
		t.Fatalf("alias project ID = %q, want canonical ID %q", aliasID, realID)
	}
}

func TestCanonicalSymbolPostgresRootRejectsMissingPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	got, err := canonicalSymbolPostgresRoot(missing)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canonicalSymbolPostgresRoot(%q) = %q, %v; want not-exist error", missing, got, err)
	}
	if got != "" {
		t.Fatalf("missing path produced project ID %q", got)
	}
}

func TestPostgresSymbolStoreFactoryUsesCanonicalRootNamespace(t *testing.T) {
	dsn := os.Getenv("GREPAI_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("GREPAI_POSTGRES_TEST_DSN is not set")
	}

	parent := t.TempDir()
	realRoot := filepath.Join(parent, "real")
	if err := os.Mkdir(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	aliasRoot := filepath.Join(parent, "alias")
	if err := os.Symlink(realRoot, aliasRoot); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.Trace.StoreBackend = "postgres"
	cfg.Trace.Postgres.DSN = dsn

	realStore, err := NewSymbolStore(context.Background(), cfg, realRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer realStore.Close()
	aliasStore, err := NewSymbolStore(context.Background(), cfg, aliasRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer aliasStore.Close()
	if err := realStore.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	const file = "factory-canonical.go"
	const symbol = "FactoryCanonicalSymbol"
	if err := realStore.SaveFile(context.Background(), file, []Symbol{{Name: symbol, File: file, Line: 1}}, nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = realStore.DeleteFile(context.Background(), file) }()
	got, err := aliasStore.LookupSymbol(context.Background(), symbol)
	if err != nil || len(got) != 1 {
		t.Fatalf("lookup through symlink factory = %#v, %v; want one symbol", got, err)
	}
}

func TestResolveSymbolPostgresDSNOrder(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Store.Postgres.DSN = "project"
	workspaceStore := &config.StoreConfig{Postgres: config.PostgresConfig{DSN: "workspace"}}

	tests := []struct {
		name       string
		traceDSN   string
		workspace  *config.StoreConfig
		projectDSN string
		wantDSN    string
		wantSource string
	}{
		{"trace wins", "trace", workspaceStore, "project", "trace", "trace.postgres.dsn"},
		{"workspace precedes project", "", workspaceStore, "project", "workspace", "workspace store.postgres.dsn"},
		{"project fallback", "", nil, "project", "project", "project store.postgres.dsn"},
		{"default fallback", "", nil, "", config.DefaultPostgresDSN, "default Postgres DSN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg.Trace.Postgres.DSN = tt.traceDSN
			cfg.Store.Postgres.DSN = tt.projectDSN
			got, source := resolveSymbolPostgresDSN(cfg, tt.workspace)
			if got != tt.wantDSN || source != tt.wantSource {
				t.Fatalf("got (%q, %q), want (%q, %q)", got, source, tt.wantDSN, tt.wantSource)
			}
		})
	}
}

func TestNewSymbolStoreDefaultsToGOB(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Trace.StoreBackend = ""
	store, err := NewSymbolStore(context.Background(), cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.(*GOBSymbolStore); !ok {
		t.Fatalf("got %T, want *GOBSymbolStore", store)
	}
}

func TestNewSymbolStoreRejectsUnknownBackend(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Trace.StoreBackend = "unknown"
	if _, err := NewSymbolStore(context.Background(), cfg, t.TempDir()); err == nil {
		t.Fatal("expected unknown backend error")
	}
}

func TestSymbolStoreBackendSelection(t *testing.T) {
	for input, want := range map[string]string{"": "gob", "gob": "gob", "postgres": "postgres"} {
		got, err := symbolStoreBackend(input)
		if err != nil || got != want {
			t.Fatalf("symbolStoreBackend(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

func TestFreshSymbolSchemaQueriesAreDeterministicAndFailOnDuplicates(t *testing.T) {
	queries := freshSymbolSchemaQueries()
	if second := freshSymbolSchemaQueries(); !reflect.DeepEqual(queries, second) {
		t.Fatal("schema generation is not deterministic")
	}
	seen := make(map[string]bool, len(queries))
	for _, query := range queries {
		if query == "" {
			t.Fatal("schema query must not be empty")
		}
		if seen[query] {
			t.Fatalf("duplicate schema query: %s", query)
		}
		seen[query] = true
		if query[:6] != "CREATE" || strings.Contains(query, "IF NOT EXISTS") {
			t.Fatalf("fresh creation must duplicate-fail: %s", query)
		}
	}
}

func TestMigrationAdvisoryKeyIsStableAndProjectScoped(t *testing.T) {
	a1, a2 := migrationAdvisoryKey("project-a")
	a1Again, a2Again := migrationAdvisoryKey("project-a")
	b1, b2 := migrationAdvisoryKey("project-b")
	if a1 != a1Again || a2 != a2Again {
		t.Fatal("migration advisory key is not stable")
	}
	if a1 == b1 && a2 == b2 {
		t.Fatal("different projects must not share an advisory key")
	}
	f1, f2 := fileMutationAdvisoryKey("project-a", "file.go")
	if a1 == f1 && a2 == f2 {
		t.Fatal("migration and file mutation keys must use distinct namespaces")
	}
	if other1, other2 := fileMutationAdvisoryKey("project-a", "other.go"); f1 == other1 && f2 == other2 {
		t.Fatal("different files must not share a mutation key")
	}
}

func TestAdvisoryKeyPartPreservesSignedBitPattern(t *testing.T) {
	tests := []struct {
		name  string
		value []byte
		want  int32
	}{
		{name: "positive one", value: []byte{0, 0, 0, 1}, want: 1},
		{name: "maximum positive", value: []byte{0x7f, 0xff, 0xff, 0xff}, want: 2147483647},
		{name: "minimum negative", value: []byte{0x80, 0, 0, 0}, want: -2147483648},
		{name: "negative one", value: []byte{0xff, 0xff, 0xff, 0xff}, want: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := advisoryKeyPart(test.value); got != test.want {
				t.Fatalf("advisoryKeyPart(%x) = %d, want %d", test.value, got, test.want)
			}
		})
	}
}

func TestSymbolSchemaUsesLosslessIdentityColumnsAndMigrationState(t *testing.T) {
	schema := strings.Join(freshSymbolSchemaQueries(), "\n")
	for _, required := range []string{
		"symbol_files (project_id BYTEA", "path BYTEA",
		"symbols (project_id BYTEA", "name BYTEA", "file BYTEA",
		"refs (project_id BYTEA", "symbol_name BYTEA", "caller BYTEA", "caller_file BYTEA",
		"call_edges (project_id BYTEA", "callee BYTEA",
		"symbol_migrations (project_id BYTEA PRIMARY KEY", "source_digest BYTEA", "source_size BIGINT", "completed_at TIMESTAMPTZ",
		"caller_line INTEGER NOT NULL DEFAULT 0, ordinal INTEGER", "call_type TEXT NOT NULL DEFAULT '', ordinal INTEGER",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("symbol schema missing %q", required)
		}
	}
}

func TestCalleeQueriesUseDurableOrdinals(t *testing.T) {
	for _, query := range []string{calleeEdgesSQL, calleeRefsSQL} {
		if strings.Contains(strings.ToLower(query), "ctid") {
			t.Fatalf("callee query depends on ctid: %s", query)
		}
		if !strings.Contains(query, "ORDER BY file,line,ordinal") {
			t.Fatalf("callee query does not use durable ordinal ordering: %s", query)
		}
	}
}
