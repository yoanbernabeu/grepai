package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/rpg"
	"github.com/yoanbernabeu/grepai/trace"
)

// TestRegisterTools_rootParameterOnProjectTools verifies that tools operating
// on a single project advertise the optional "root" parameter, and that
// workspace/RPG/stats tools do not.
func TestRegisterTools_rootParameterOnProjectTools(t *testing.T) {
	s := &Server{projectRoot: "/tmp/test-project"}
	s.mcpServer = server.NewMCPServer("grepai-test", "1.0.0")
	s.registerTools()

	toolsWithRoot := []string{
		"grepai_search",
		"grepai_trace_callers",
		"grepai_trace_callees",
		"grepai_trace_graph",
		"grepai_refs_readers",
		"grepai_refs_writers",
		"grepai_refs_graph",
		"grepai_index_status",
	}
	for _, name := range toolsWithRoot {
		tool, ok := s.mcpServer.ListTools()[name]
		if !ok {
			t.Fatalf("%s tool not registered", name)
		}
		propRaw, ok := tool.Tool.InputSchema.Properties["root"]
		if !ok {
			t.Errorf("%s: expected root property in schema", name)
			continue
		}
		propMap, ok := propRaw.(map[string]any)
		if !ok {
			t.Errorf("%s: root property is not an object, got %T", name, propRaw)
			continue
		}
		if desc, _ := propMap["description"].(string); !strings.Contains(desc, "Absolute project root path") {
			t.Errorf("%s: unexpected root description: %q", name, desc)
		}
	}

	toolsWithoutRoot := []string{
		"grepai_list_workspaces",
		"grepai_list_projects",
		"grepai_rpg_search",
		"grepai_rpg_fetch",
		"grepai_rpg_explore",
		"grepai_stats",
	}
	for _, name := range toolsWithoutRoot {
		tool, ok := s.mcpServer.ListTools()[name]
		if !ok {
			t.Fatalf("%s tool not registered", name)
		}
		if _, ok := tool.Tool.InputSchema.Properties["root"]; ok {
			t.Errorf("%s: unexpected root property in schema", name)
		}
	}
}

// TestNewServer_advertises_tool_capabilities verifies that the initialize
// response advertises tool capabilities, which some MCP clients require before
// discovering server features.
func TestNewServer_advertises_tool_capabilities(t *testing.T) {
	for name, newServer := range map[string]func() (*Server, error){
		"NewServer":              func() (*Server, error) { return NewServer("") },
		"NewServerWithWorkspace": func() (*Server, error) { return NewServerWithWorkspace("", "test") },
	} {
		t.Run(name, func(t *testing.T) {
			s, err := newServer()
			if err != nil {
				t.Fatalf("server construction failed: %v", err)
			}

			initRequest := map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "initialize",
				"params": map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{},
					"clientInfo":      map[string]any{"name": "test-client", "version": "0.0.1"},
				},
			}
			raw, err := json.Marshal(initRequest)
			if err != nil {
				t.Fatalf("failed to marshal initialize request: %v", err)
			}

			response := s.mcpServer.HandleMessage(context.Background(), raw)
			wire, err := json.Marshal(response)
			if err != nil {
				t.Fatalf("failed to marshal initialize response: %v", err)
			}

			var parsed struct {
				Result struct {
					Capabilities struct {
						Tools *struct {
							ListChanged bool `json:"listChanged"`
						} `json:"tools"`
					} `json:"capabilities"`
				} `json:"result"`
			}
			if err := json.Unmarshal(wire, &parsed); err != nil {
				t.Fatalf("failed to decode initialize response: %v (%s)", err, wire)
			}
			if parsed.Result.Capabilities.Tools == nil {
				t.Fatalf("expected tools capability in initialize response, got: %s", wire)
			}
		})
	}
}

// TestResolveProjectContext covers the workspace/root resolution rules.
func TestResolveProjectContext(t *testing.T) {
	existingDir := t.TempDir()
	existingFile := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(existingFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	t.Run("no_params_returns_empty_context", func(t *testing.T) {
		s := &Server{}
		workspace, root, err := s.resolveProjectContext(refsTestRequest(map[string]any{}), "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if workspace != "" || root != "" {
			t.Fatalf("expected empty workspace/root, got %q/%q", workspace, root)
		}
	})

	t.Run("auto_injects_server_workspace", func(t *testing.T) {
		s := &Server{workspaceName: "acme"}
		workspace, root, err := s.resolveProjectContext(refsTestRequest(map[string]any{}), "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if workspace != "acme" || root != "" {
			t.Fatalf("expected auto-injected workspace, got %q/%q", workspace, root)
		}
	})

	t.Run("explicit_root_wins_over_server_workspace", func(t *testing.T) {
		s := &Server{workspaceName: "acme"}
		workspace, root, err := s.resolveProjectContext(refsTestRequest(map[string]any{
			"root": existingDir,
		}), "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if workspace != "" {
			t.Fatalf("expected no workspace when root override is used, got %q", workspace)
		}
		if root != existingDir {
			t.Fatalf("root = %q, want %q", root, existingDir)
		}
	})

	t.Run("explicit_workspace_and_root_conflict", func(t *testing.T) {
		s := &Server{}
		_, _, err := s.resolveProjectContext(refsTestRequest(map[string]any{
			"workspace": "acme",
			"root":      existingDir,
		}), "acme")
		if err == nil || !strings.Contains(err.Error(), "cannot specify both workspace and root parameters") {
			t.Fatalf("expected workspace/root conflict error, got %v", err)
		}
	})

	t.Run("relative_root_is_resolved_to_absolute", func(t *testing.T) {
		s := &Server{}
		want, err := filepath.Abs(".")
		if err != nil {
			t.Fatalf("filepath.Abs failed: %v", err)
		}
		_, root, err := s.resolveProjectContext(refsTestRequest(map[string]any{
			"root": ".",
		}), "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if root != want {
			t.Fatalf("root = %q, want %q", root, want)
		}
	})

	t.Run("missing_root_is_rejected", func(t *testing.T) {
		s := &Server{}
		_, _, err := s.resolveProjectContext(refsTestRequest(map[string]any{
			"root": filepath.Join(t.TempDir(), "does-not-exist"),
		}), "")
		if err == nil || !strings.Contains(err.Error(), "does not exist") {
			t.Fatalf("expected missing-root error, got %v", err)
		}
	})

	t.Run("file_root_is_rejected", func(t *testing.T) {
		s := &Server{}
		_, _, err := s.resolveProjectContext(refsTestRequest(map[string]any{
			"root": existingFile,
		}), "")
		if err == nil || !strings.Contains(err.Error(), "must be a directory") {
			t.Fatalf("expected not-a-directory error, got %v", err)
		}
	})
}

// TestHandleTraceCallers_rootOverride_loads_index_from_root verifies that a
// server started with no project context can still answer trace queries when
// the client passes an explicit root (per-launch project detection).
func TestHandleTraceCallers_rootOverride_loads_index_from_root(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, config.ConfigDir), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	symbolStore := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(projectRoot))
	if err := symbolStore.SaveFile(ctx, "internal/auth/login.go", []trace.Symbol{
		{Name: "Login", Kind: "function", File: "internal/auth/login.go", Line: 10, Language: "go"},
	}, []trace.Reference{
		{SymbolName: "Login", File: "internal/auth/handler.go", Line: 20, CallerName: "HandleAuth", CallerFile: "internal/auth/handler.go", CallerLine: 15},
	}); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}
	if err := symbolStore.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// No startup project and no workspace: only the root parameter can resolve
	// the project context.
	s := &Server{}

	result, err := s.handleTraceCallers(ctx, refsTestRequest(map[string]any{
		"symbol": "Login",
		"format": "json",
		"root":   projectRoot,
	}))
	if err != nil {
		t.Fatalf("handleTraceCallers returned error: %v", err)
	}

	payload := textResultPayload(t, result)
	if !strings.Contains(payload, "HandleAuth") {
		t.Fatalf("expected caller HandleAuth in root-overridden trace, got %q", payload)
	}
}

// TestHandleTraceCallers_without_context_still_errors verifies the error
// message when neither a startup project nor a root is available.
func TestHandleTraceCallers_without_context_still_errors(t *testing.T) {
	s := &Server{}

	result, err := s.handleTraceCallers(context.Background(), refsTestRequest(map[string]any{
		"symbol": "Login",
		"format": "json",
	}))
	if err != nil {
		t.Fatalf("handleTraceCallers returned error: %v", err)
	}

	if got := textResultPayload(t, result); !strings.Contains(got, "trace requires a project context") {
		t.Fatalf("expected project-context error, got %q", got)
	}
}

// TestHandleRefsReaders_rootOverride verifies that refs tools load the symbol
// index from the root parameter when the server has no startup project.
func TestHandleRefsReaders_rootOverride(t *testing.T) {
	projectRoot := seedRefsTestStore(t)
	s := &Server{}

	result, err := s.handleRefsReaders(context.Background(), refsTestRequest(map[string]any{
		"symbol": "uid",
		"format": "json",
		"root":   projectRoot,
	}))
	if err != nil {
		t.Fatalf("handleRefsReaders returned error: %v", err)
	}

	var payload struct {
		Readers []RefUsage `json:"readers"`
	}
	if err := json.Unmarshal([]byte(textResultPayload(t, result)), &payload); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	if len(payload.Readers) != 1 {
		t.Fatalf("expected 1 reader via root override, got %d", len(payload.Readers))
	}
}

// TestHandleSearch_rejects_workspace_and_root_combination verifies the guard
// against mixing the two project contexts in a single request.
func TestHandleSearch_rejects_workspace_and_root_combination(t *testing.T) {
	s := &Server{}

	result, err := s.handleSearch(context.Background(), refsTestRequest(map[string]any{
		"query":     "authentication",
		"workspace": "acme",
		"root":      t.TempDir(),
	}))
	if err != nil {
		t.Fatalf("handleSearch returned error: %v", err)
	}

	if got := textResultPayload(t, result); !strings.Contains(got, "cannot specify both workspace and root parameters") {
		t.Fatalf("expected workspace/root conflict error, got %q", got)
	}
}

// TestCreateStoreForRoot_gobBackend verifies the store helper resolves the
// index path from the explicit root, not from the server startup project.
func TestCreateStoreForRoot_gobBackend(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, config.ConfigDir), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	s := &Server{projectRoot: "/somewhere/else"}
	cfg := config.DefaultConfig()
	cfg.Store.Backend = "gob"

	st, err := s.createStoreForRoot(ctx, cfg, projectRoot)
	if err != nil {
		t.Fatalf("createStoreForRoot returned error: %v", err)
	}
	defer st.Close()

	if st == nil {
		t.Fatal("expected non-nil store")
	}
}

// setupTraceProjectWithRPG creates a project with RPG enabled, a symbol index
// containing Login (called by HandleAuth) and an RPG graph placing
// internal/auth/login.go under the given feature path.
func setupTraceProjectWithRPG(t *testing.T, featurePath string) string {
	t.Helper()
	ctx := context.Background()
	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, config.ConfigDir), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.RPG.Enabled = true
	if err := cfg.Save(projectRoot); err != nil {
		t.Fatalf("save config: %v", err)
	}

	symbolStore := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(projectRoot))
	if err := symbolStore.SaveFile(ctx, "internal/auth/login.go", []trace.Symbol{
		{Name: "Login", Kind: "function", File: "internal/auth/login.go", Line: 10, Language: "go"},
	}, []trace.Reference{
		{SymbolName: "Login", File: "internal/auth/handler.go", Line: 20, CallerName: "HandleAuth", CallerFile: "internal/auth/handler.go", CallerLine: 15},
	}); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}
	if err := symbolStore.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	rpgStore := rpg.NewGOBRPGStore(config.GetRPGIndexPath(projectRoot))
	g := rpgStore.GetGraph()
	area := &rpg.Node{ID: "area:" + featurePath, Kind: rpg.KindArea, Feature: featurePath}
	file := &rpg.Node{ID: "file:internal/auth/login.go", Kind: rpg.KindFile, Path: "internal/auth/login.go"}
	sym := &rpg.Node{ID: "sym:internal/auth/login.go:Login", Kind: rpg.KindSymbol, Path: "internal/auth/login.go", SymbolName: "Login"}
	g.AddNode(area)
	g.AddNode(file)
	g.AddNode(sym)
	g.AddEdge(&rpg.Edge{From: area.ID, To: file.ID, Type: rpg.EdgeFeatureParent})
	g.AddEdge(&rpg.Edge{From: file.ID, To: sym.ID, Type: rpg.EdgeContains})
	if err := rpgStore.Persist(ctx); err != nil {
		t.Fatalf("persist RPG graph: %v", err)
	}

	return projectRoot
}

// TestTraceTools_rootOverride_enrich_from_root_RPG verifies that RPG
// enrichment reads the graph of the root override, not the startup project.
func TestTraceTools_rootOverride_enrich_from_root_RPG(t *testing.T) {
	ctx := context.Background()
	startupRoot := setupTraceProjectWithRPG(t, "startup-feature")
	overrideRoot := setupTraceProjectWithRPG(t, "override-feature")

	s := &Server{projectRoot: startupRoot}

	tools := []struct {
		name    string
		handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
	}{
		{name: "callers", handler: s.handleTraceCallers},
		{name: "callees", handler: s.handleTraceCallees},
		{name: "graph", handler: s.handleTraceGraph},
	}

	for _, tool := range tools {
		t.Run(tool.name, func(t *testing.T) {
			for _, tt := range []struct {
				root string
				want string
				not  string
			}{
				{root: overrideRoot, want: "override-feature", not: "startup-feature"},
				{root: "", want: "startup-feature", not: "override-feature"},
			} {
				args := map[string]any{"symbol": "Login", "format": "json"}
				if tt.root != "" {
					args["root"] = tt.root
				}
				result, err := tool.handler(ctx, refsTestRequest(args))
				if err != nil {
					t.Fatalf("handler returned error: %v", err)
				}
				payload := textResultPayload(t, result)
				if !strings.Contains(payload, tt.want) || strings.Contains(payload, tt.not) {
					t.Fatalf("root=%q: expected feature path %q (not %q), got %s", tt.root, tt.want, tt.not, payload)
				}
			}
		})
	}
}
