package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

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
	result := trace.TraceResult{Query: symbolName, Mode: "fast"}

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
