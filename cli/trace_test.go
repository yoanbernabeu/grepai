package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/trace"
)

func TestLoadWorkspaceSymbolStores_should_return_error_when_workspace_not_found(t *testing.T) {
	ctx := context.Background()

	// Use a workspace name that will never exist.
	// This should return an error regardless of whether workspace.yaml exists:
	// - If workspace.yaml doesn't exist: "failed to load workspace config" or "no workspaces configured"
	// - If workspace.yaml exists but lacks this workspace: "workspace not found"
	_, err := loadWorkspaceSymbolStores(ctx, "nonexistent-workspace-abc123xyz", "")
	if err == nil {
		t.Fatal("expected error when loading stores for nonexistent workspace, got nil")
	}
}

func TestCloseSymbolStores_should_close_all_stores(t *testing.T) {
	// Create 3 GOBSymbolStores in temp dirs, close them all, verify no panics.
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()
	tmpDir3 := t.TempDir()

	store1 := trace.NewGOBSymbolStore(filepath.Join(tmpDir1, "symbols.gob"))
	store2 := trace.NewGOBSymbolStore(filepath.Join(tmpDir2, "symbols.gob"))
	store3 := trace.NewGOBSymbolStore(filepath.Join(tmpDir3, "symbols.gob"))

	stores := []trace.SymbolStore{store1, store2, store3}

	// Should not panic
	closeSymbolStores(stores)
}

func TestCloseSymbolStores_should_handle_empty_slice(t *testing.T) {
	// Should not panic with an empty slice
	closeSymbolStores([]trace.SymbolStore{})
}

func TestLoadWorkspaceSymbolStores_should_return_error_when_project_requires_workspace(t *testing.T) {
	// Save original values and restore after test
	origProject := traceProject
	origWorkspace := traceWorkspace
	defer func() {
		traceProject = origProject
		traceWorkspace = origWorkspace
	}()

	traceProject = "some-project"
	traceWorkspace = ""

	err := runTraceCallers(nil, []string{"SomeSymbol"})
	if err == nil {
		t.Fatal("expected error when --project is set without --workspace, got nil")
	}
	if !strings.Contains(err.Error(), "--project requires --workspace") {
		t.Fatalf("expected error to contain '--project requires --workspace', got: %s", err.Error())
	}
}

func TestWorkspaceTraceCallers_should_aggregate_results_from_multiple_stores(t *testing.T) {
	ctx := context.Background()

	// Create two stores with different symbols and references
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	store1 := trace.NewGOBSymbolStore(filepath.Join(tmpDir1, "symbols.gob"))
	store2 := trace.NewGOBSymbolStore(filepath.Join(tmpDir2, "symbols.gob"))

	// Populate store1: defines "Login" and has a caller "HandleAuth" -> "Login"
	err := store1.SaveFile(ctx, "auth/login.go", []trace.Symbol{
		{Name: "Login", Kind: trace.KindFunction, File: "auth/login.go", Line: 10, Language: "go"},
	}, []trace.Reference{
		{SymbolName: "Login", File: "auth/handler.go", Line: 25, CallerName: "HandleAuth", CallerFile: "auth/handler.go", CallerLine: 20, Context: "Login()"},
	})
	if err != nil {
		t.Fatalf("store1.SaveFile() failed: %v", err)
	}

	// Populate store2: has a different caller "ProcessRequest" -> "Login"
	err = store2.SaveFile(ctx, "api/request.go", []trace.Symbol{
		{Name: "ProcessRequest", Kind: trace.KindFunction, File: "api/request.go", Line: 5, Language: "go"},
	}, []trace.Reference{
		{SymbolName: "Login", File: "api/request.go", Line: 15, CallerName: "ProcessRequest", CallerFile: "api/request.go", CallerLine: 5, Context: "Login()"},
	})
	if err != nil {
		t.Fatalf("store2.SaveFile() failed: %v", err)
	}

	// Aggregate callers from both stores (mirrors workspace mode logic in runTraceCallers)
	stores := []trace.SymbolStore{store1, store2}
	symbolName := "Login"
	var result trace.TraceResult

	for _, ss := range stores {
		symbols, _ := ss.LookupSymbol(ctx, symbolName)
		if len(symbols) > 0 && result.Symbol == nil {
			result.Symbol = &symbols[0]
		}
		refs, _ := ss.LookupCallers(ctx, symbolName)
		for _, ref := range refs {
			callerSyms, _ := ss.LookupSymbol(ctx, ref.CallerName)
			var callerSym trace.Symbol
			if len(callerSyms) > 0 {
				callerSym = callerSyms[0]
			} else {
				callerSym = trace.Symbol{Name: ref.CallerName, File: ref.CallerFile, Line: ref.CallerLine}
			}
			result.Callers = append(result.Callers, trace.CallerInfo{
				Symbol: callerSym,
				CallSite: trace.CallSite{
					File:    ref.File,
					Line:    ref.Line,
					Context: ref.Context,
				},
			})
		}
	}

	// Verify symbol was found from store1
	if result.Symbol == nil {
		t.Fatal("expected symbol to be found, got nil")
	}
	if result.Symbol.Name != "Login" {
		t.Fatalf("expected symbol name 'Login', got %q", result.Symbol.Name)
	}

	// Verify we got callers from both stores
	if len(result.Callers) != 2 {
		t.Fatalf("expected 2 callers from aggregation, got %d", len(result.Callers))
	}

	// Verify caller names
	callerNames := make(map[string]bool)
	for _, c := range result.Callers {
		callerNames[c.Symbol.Name] = true
	}
	if !callerNames["HandleAuth"] {
		t.Error("expected 'HandleAuth' in callers")
	}
	if !callerNames["ProcessRequest"] {
		t.Error("expected 'ProcessRequest' in callers")
	}

	// Cleanup
	closeSymbolStores(stores)
}

func TestTruncate_should_return_short_string_unchanged(t *testing.T) {
	got := truncate("hello", 80)
	if got != "hello" {
		t.Errorf("truncate(%q, 80) = %q, want %q", "hello", got, "hello")
	}
}

func TestTruncate_should_trim_long_string(t *testing.T) {
	long := strings.Repeat("a", 100)
	got := truncate(long, 80)
	if len(got) != 80 {
		t.Errorf("truncate() length = %d, want 80", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("truncate() should end with '...', got %q", got)
	}
}

func TestTruncate_should_replace_newlines(t *testing.T) {
	got := truncate("line1\nline2\nline3", 80)
	if strings.Contains(got, "\n") {
		t.Errorf("truncate() should replace newlines, got %q", got)
	}
	if got != "line1 line2 line3" {
		t.Errorf("truncate() = %q, want %q", got, "line1 line2 line3")
	}
}

func TestOutputJSON_should_produce_valid_json(t *testing.T) {
	result := trace.TraceResult{
		Query: "TestSymbol",
		Mode:  "fast",
		Symbol: &trace.Symbol{
			Name: "TestSymbol",
			Kind: trace.KindFunction,
			File: "main.go",
			Line: 10,
		},
		Callers: []trace.CallerInfo{
			{
				Symbol:   trace.Symbol{Name: "Caller1", Kind: trace.KindFunction, File: "caller.go", Line: 5},
				CallSite: trace.CallSite{File: "caller.go", Line: 20, Context: "TestSymbol()"},
			},
		},
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputJSON(result)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("outputJSON() failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify it's valid JSON
	var decoded trace.TraceResult
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("outputJSON() produced invalid JSON: %v\nOutput: %s", err, output)
	}
	if decoded.Query != "TestSymbol" {
		t.Errorf("decoded query = %q, want %q", decoded.Query, "TestSymbol")
	}
	if len(decoded.Callers) != 1 {
		t.Errorf("decoded callers count = %d, want 1", len(decoded.Callers))
	}
}

func TestDisplayCallersResult_should_handle_empty_callers(t *testing.T) {
	result := trace.TraceResult{
		Symbol: &trace.Symbol{Name: "Foo", Kind: trace.KindFunction, File: "foo.go", Line: 1},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := displayCallersResult(result)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("displayCallersResult() failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "No callers found") {
		t.Errorf("expected 'No callers found' in output, got: %s", output)
	}
}

func TestDisplayCallersResult_should_list_callers(t *testing.T) {
	result := trace.TraceResult{
		Symbol: &trace.Symbol{Name: "Foo", Kind: trace.KindFunction, File: "foo.go", Line: 1},
		Callers: []trace.CallerInfo{
			{
				Symbol:   trace.Symbol{Name: "Bar", File: "bar.go", Line: 5},
				CallSite: trace.CallSite{File: "bar.go", Line: 10, Context: "Foo()"},
			},
		},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := displayCallersResult(result)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("displayCallersResult() failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Bar") {
		t.Errorf("expected caller 'Bar' in output, got: %s", output)
	}
	if !strings.Contains(output, "Callers (1)") {
		t.Errorf("expected 'Callers (1)' in output, got: %s", output)
	}
}

func TestDisplayCalleesResult_should_handle_empty_callees(t *testing.T) {
	result := trace.TraceResult{
		Symbol: &trace.Symbol{Name: "Foo", Kind: trace.KindFunction, File: "foo.go", Line: 1},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := displayCalleesResult(result)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("displayCalleesResult() failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "No callees found") {
		t.Errorf("expected 'No callees found' in output, got: %s", output)
	}
}

func TestDisplayCalleesResult_should_list_callees(t *testing.T) {
	result := trace.TraceResult{
		Symbol: &trace.Symbol{Name: "Foo", Kind: trace.KindFunction, File: "foo.go", Line: 1},
		Callees: []trace.CalleeInfo{
			{
				Symbol:   trace.Symbol{Name: "Baz", File: "baz.go", Line: 15},
				CallSite: trace.CallSite{File: "foo.go", Line: 3},
			},
		},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := displayCalleesResult(result)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("displayCalleesResult() failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Baz") {
		t.Errorf("expected callee 'Baz' in output, got: %s", output)
	}
	if !strings.Contains(output, "Callees (1)") {
		t.Errorf("expected 'Callees (1)' in output, got: %s", output)
	}
}

func TestDisplayGraphResult_should_show_nodes_and_edges(t *testing.T) {
	result := trace.TraceResult{
		Query: "Main",
		Graph: &trace.CallGraph{
			Root:  "Main",
			Depth: 2,
			Nodes: map[string]trace.Symbol{
				"Main":   {Name: "Main", Kind: trace.KindFunction, File: "main.go", Line: 1},
				"Helper": {Name: "Helper", Kind: trace.KindFunction, File: "helper.go", Line: 5},
			},
			Edges: []trace.CallEdge{
				{Caller: "Main", Callee: "Helper", File: "main.go", Line: 3},
			},
		},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := displayGraphResult(result)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("displayGraphResult() failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Call Graph for: Main") {
		t.Errorf("expected graph header in output, got: %s", output)
	}
	if !strings.Contains(output, "Nodes (2)") {
		t.Errorf("expected 'Nodes (2)' in output, got: %s", output)
	}
	if !strings.Contains(output, "Edges (1)") {
		t.Errorf("expected 'Edges (1)' in output, got: %s", output)
	}
	if !strings.Contains(output, "Main -> Helper") {
		t.Errorf("expected 'Main -> Helper' edge in output, got: %s", output)
	}
}

func TestRunTraceCallees_should_require_workspace_with_project(t *testing.T) {
	origProject := traceProject
	origWorkspace := traceWorkspace
	defer func() {
		traceProject = origProject
		traceWorkspace = origWorkspace
	}()

	traceProject = "some-project"
	traceWorkspace = ""

	err := runTraceCallees(nil, []string{"SomeSymbol"})
	if err == nil {
		t.Fatal("expected error when --project without --workspace")
	}
	if !strings.Contains(err.Error(), "--project requires --workspace") {
		t.Fatalf("expected '--project requires --workspace', got: %s", err.Error())
	}
}

func TestRunTraceGraph_should_require_workspace_with_project(t *testing.T) {
	origProject := traceProject
	origWorkspace := traceWorkspace
	defer func() {
		traceProject = origProject
		traceWorkspace = origWorkspace
	}()

	traceProject = "some-project"
	traceWorkspace = ""

	err := runTraceGraph(nil, []string{"SomeSymbol"})
	if err == nil {
		t.Fatal("expected error when --project without --workspace")
	}
	if !strings.Contains(err.Error(), "--project requires --workspace") {
		t.Fatalf("expected '--project requires --workspace', got: %s", err.Error())
	}
}

func TestLoadWorkspaceSymbolStores_should_load_stores_for_valid_workspace(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	cleanup := setTestHomeDirCLI(t, tmpDir)
	defer cleanup()

	// Create project directories with symbol stores
	projectDir1 := filepath.Join(tmpDir, "proj1")
	projectDir2 := filepath.Join(tmpDir, "proj2")
	os.MkdirAll(filepath.Join(projectDir1, ".grepai"), 0755)
	os.MkdirAll(filepath.Join(projectDir2, ".grepai"), 0755)

	// Create symbol stores with data
	store1 := trace.NewGOBSymbolStore(filepath.Join(projectDir1, ".grepai", "symbols.gob"))
	store1.SaveFile(ctx, "main.go", []trace.Symbol{
		{Name: "Main", Kind: trace.KindFunction, File: "main.go", Line: 1, Language: "go"},
	}, nil)
	store1.Persist(ctx)
	store1.Close()

	store2 := trace.NewGOBSymbolStore(filepath.Join(projectDir2, ".grepai", "symbols.gob"))
	store2.SaveFile(ctx, "util.go", []trace.Symbol{
		{Name: "Helper", Kind: trace.KindFunction, File: "util.go", Line: 1, Language: "go"},
	}, nil)
	store2.Persist(ctx)
	store2.Close()

	// Create workspace config
	wsCfg := config.DefaultWorkspaceConfig()
	wsCfg.AddWorkspace(config.Workspace{
		Name:  "test-load",
		Store: config.StoreConfig{Backend: "qdrant"},
		Embedder: config.EmbedderConfig{
			Provider: "ollama",
			Model:    "nomic-embed-text",
		},
		Projects: []config.ProjectEntry{
			{Name: "proj1", Path: projectDir1},
			{Name: "proj2", Path: projectDir2},
		},
	})
	config.SaveWorkspaceConfig(wsCfg)

	// Load all stores
	stores, err := loadWorkspaceSymbolStores(ctx, "test-load", "")
	if err != nil {
		t.Fatalf("loadWorkspaceSymbolStores() failed: %v", err)
	}
	defer closeSymbolStores(stores)

	if len(stores) != 2 {
		t.Fatalf("expected 2 stores, got %d", len(stores))
	}
}

func TestLoadWorkspaceSymbolStores_should_filter_by_project(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	cleanup := setTestHomeDirCLI(t, tmpDir)
	defer cleanup()

	projectDir1 := filepath.Join(tmpDir, "proj1")
	projectDir2 := filepath.Join(tmpDir, "proj2")
	os.MkdirAll(filepath.Join(projectDir1, ".grepai"), 0755)
	os.MkdirAll(filepath.Join(projectDir2, ".grepai"), 0755)

	// Create empty symbol stores
	s1 := trace.NewGOBSymbolStore(filepath.Join(projectDir1, ".grepai", "symbols.gob"))
	s1.Persist(ctx)
	s1.Close()
	s2 := trace.NewGOBSymbolStore(filepath.Join(projectDir2, ".grepai", "symbols.gob"))
	s2.Persist(ctx)
	s2.Close()

	wsCfg := config.DefaultWorkspaceConfig()
	wsCfg.AddWorkspace(config.Workspace{
		Name:  "test-filter",
		Store: config.StoreConfig{Backend: "qdrant"},
		Embedder: config.EmbedderConfig{
			Provider: "ollama",
			Model:    "nomic-embed-text",
		},
		Projects: []config.ProjectEntry{
			{Name: "proj1", Path: projectDir1},
			{Name: "proj2", Path: projectDir2},
		},
	})
	config.SaveWorkspaceConfig(wsCfg)

	// Filter to just proj1
	stores, err := loadWorkspaceSymbolStores(ctx, "test-filter", "proj1")
	if err != nil {
		t.Fatalf("loadWorkspaceSymbolStores() failed: %v", err)
	}
	defer closeSymbolStores(stores)

	if len(stores) != 1 {
		t.Fatalf("expected 1 store when filtering by project, got %d", len(stores))
	}
}

func TestLoadWorkspaceSymbolStores_should_error_on_unknown_project(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	cleanup := setTestHomeDirCLI(t, tmpDir)
	defer cleanup()

	wsCfg := config.DefaultWorkspaceConfig()
	wsCfg.AddWorkspace(config.Workspace{
		Name:  "test-unknown",
		Store: config.StoreConfig{Backend: "qdrant"},
		Embedder: config.EmbedderConfig{
			Provider: "ollama",
			Model:    "nomic-embed-text",
		},
		Projects: []config.ProjectEntry{
			{Name: "proj1", Path: "/tmp/nonexistent"},
		},
	})
	config.SaveWorkspaceConfig(wsCfg)

	_, err := loadWorkspaceSymbolStores(ctx, "test-unknown", "nonexistent-project")
	if err == nil {
		t.Fatal("expected error for unknown project")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' in error, got: %s", err.Error())
	}
}
