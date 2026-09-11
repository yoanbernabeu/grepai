package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/trace"
)

// seedCalleeResolutionProject writes a real GOB symbol index for one
// workspace project with the given definitions and Target call sites.
func seedCalleeResolutionProject(t *testing.T, root string, symbols []trace.Symbol, refs []trace.Reference) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, config.ConfigDir), 0o755); err != nil {
		t.Fatal(err)
	}
	store := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(root))
	if err := store.SaveFile(context.Background(), "index.go", symbols, refs); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestTraceCalleesWorkspaceCrossProjectFallback covers the workspace callee
// resolution contract: origin preference for duplicate names, cross-project
// fallback in deterministic loaded order when the origin store has no
// definition, and the name-only placeholder when nothing defines the callee.
func TestTraceCalleesWorkspaceCrossProjectFallback(t *testing.T) {
	// Given a real GOB-backed workspace where project "one" calls callees that
	// are defined only in project "two" (CrossCallee), defined in both
	// (Shared), and defined nowhere (Undefined). Project "three" also defines
	// CrossCallee later in loaded order to pin deterministic fallback.
	home := isolateMCPTestHome(t)
	projects := make([]config.ProjectEntry, 0, 3)
	for _, name := range []string{"one", "two", "three"} {
		projects = append(projects, config.ProjectEntry{Name: name, Path: filepath.Join(home, name)})
	}
	seedCalleeResolutionProject(t, projects[0].Path, []trace.Symbol{
		{Name: "Target", File: "one/target.go", Line: 1},
		{Name: "Shared", File: "one/shared.go", Line: 5},
	}, []trace.Reference{
		{SymbolName: "CrossCallee", Kind: trace.RefKindCall, File: "one/target.go", Line: 12, CallerName: "Target"},
		{SymbolName: "Shared", Kind: trace.RefKindCall, File: "one/target.go", Line: 13, CallerName: "Target"},
		{SymbolName: "Undefined", Kind: trace.RefKindCall, File: "one/target.go", Line: 14, CallerName: "Target"},
	})
	seedCalleeResolutionProject(t, projects[1].Path, []trace.Symbol{
		{Name: "CrossCallee", File: "two/cross.go", Line: 7},
		{Name: "Shared", File: "two/shared.go", Line: 9},
	}, nil)
	seedCalleeResolutionProject(t, projects[2].Path, []trace.Symbol{
		{Name: "CrossCallee", File: "three/cross.go", Line: 3},
	}, nil)
	workspaceConfig := config.DefaultWorkspaceConfig()
	workspaceConfig.AddWorkspace(config.Workspace{Name: "callees", Projects: projects})
	if err := config.SaveWorkspaceConfig(workspaceConfig); err != nil {
		t.Fatal(err)
	}
	server := &Server{workspaceName: "callees"}

	// When callees are requested through the real workspace handler.
	result, err := server.handleTraceCallees(context.Background(), traceHandlerRequest(map[string]any{"symbol": "Target", "format": "json"}))
	if err != nil {
		t.Fatal(err)
	}

	// Then the typed trace result resolves each callee per the contract.
	var payload trace.TraceResult
	if err := json.Unmarshal([]byte(textResultPayload(t, result)), &payload); err != nil {
		t.Fatalf("handler output is not a trace result: %v", err)
	}
	if payload.Symbol == nil || payload.Symbol.File != "one/target.go" {
		t.Fatalf("query symbol lost origin definition: %#v", payload.Symbol)
	}
	callees := make(map[string]trace.Symbol, len(payload.Callees))
	for _, callee := range payload.Callees {
		callees[callee.Symbol.Name] = callee.Symbol
	}
	if len(callees) != 3 {
		t.Fatalf("expected 3 callees, got %#v", payload.Callees)
	}
	if got := callees["CrossCallee"]; got.File != "two/cross.go" || got.Line != 7 {
		t.Fatalf("cross-project callee did not fall back to first loaded definition: %#v", got)
	}
	if got := callees["Shared"]; got.File != "one/shared.go" || got.Line != 5 {
		t.Fatalf("duplicate callee lost origin preference: %#v", got)
	}
	if got := callees["Undefined"]; got.Name != "Undefined" || got.File != "" || got.Line != 0 {
		t.Fatalf("undefined callee lost name-only fallback: %#v", got)
	}
}

// batchTrackingSymbolStore records batch vs point lookups while serving only
// the requested names from its symbol table.
type batchTrackingSymbolStore struct {
	trace.SymbolStore
	symbols      map[string][]trace.Symbol
	batchCalls   int
	pointLookups int
}

func (s *batchTrackingSymbolStore) LookupSymbolsBatch(_ context.Context, names []string) (map[string][]trace.Symbol, error) {
	s.batchCalls++
	result := make(map[string][]trace.Symbol, len(names))
	for _, name := range names {
		if defs, ok := s.symbols[name]; ok {
			result[name] = defs
		}
	}
	return result, nil
}

func (s *batchTrackingSymbolStore) LookupSymbol(context.Context, string) ([]trace.Symbol, error) {
	s.pointLookups++
	return nil, nil
}

func TestLookupMissingCalleeSymbolsFallsBackInLoadedOrder(t *testing.T) {
	one := &batchTrackingSymbolStore{symbols: map[string][]trace.Symbol{
		"Shared": {{Name: "Shared", File: "one/shared.go", Line: 5}},
	}}
	two := &batchTrackingSymbolStore{symbols: map[string][]trace.Symbol{
		"CrossCallee": {{Name: "CrossCallee", File: "two/cross.go", Line: 7}},
	}}
	refs := []storeReference{
		{ref: trace.Reference{SymbolName: "CrossCallee"}, storeIndex: 0},
		{ref: trace.Reference{SymbolName: "Shared"}, storeIndex: 0},
		{ref: trace.Reference{SymbolName: "Undefined"}, storeIndex: 1},
	}
	origin := []map[string][]trace.Symbol{
		{"Shared": one.symbols["Shared"]},
		{},
	}

	resolved := lookupMissingCalleeSymbols(context.Background(), []trace.SymbolStore{one, two}, refs, origin)

	if got := resolved[0]["CrossCallee"]; got.File != "two/cross.go" || got.Line != 7 {
		t.Fatalf("cross-project fallback = %#v", got)
	}
	if _, ok := resolved[0]["Shared"]; ok {
		t.Fatal("origin-defined name must not be re-resolved")
	}
	if _, ok := resolved[1]["Undefined"]; ok {
		t.Fatal("undefined name must stay unresolved")
	}
	if one.pointLookups != 0 || two.pointLookups != 0 {
		t.Fatalf("point lookups leaked into fallback: one=%d two=%d", one.pointLookups, two.pointLookups)
	}
	if one.batchCalls > 1 || two.batchCalls > 1 {
		t.Fatalf("fallback regressed to N+1 batches: one=%d two=%d", one.batchCalls, two.batchCalls)
	}
}

func TestLookupMissingCalleeSymbolsEvaluatesMissingnessPerOrigin(t *testing.T) {
	// The first Shared reference originates from store 0, which defines
	// Shared; a later Shared reference originates from store 1, which does
	// not. Missingness must be evaluated per origin before name dedup, so
	// the store-1 reference still earns a cross-project fallback.
	one := &batchTrackingSymbolStore{symbols: map[string][]trace.Symbol{
		"Shared": {{Name: "Shared", File: "one/shared.go", Line: 5}},
	}}
	two := &batchTrackingSymbolStore{symbols: map[string][]trace.Symbol{}}
	refs := []storeReference{
		{ref: trace.Reference{SymbolName: "Shared"}, storeIndex: 0},
		{ref: trace.Reference{SymbolName: "Shared"}, storeIndex: 1},
	}
	origin := []map[string][]trace.Symbol{
		{"Shared": one.symbols["Shared"]},
		{},
	}

	resolved := lookupMissingCalleeSymbols(context.Background(), []trace.SymbolStore{one, two}, refs, origin)

	if got := resolved[1]["Shared"]; got.File != "one/shared.go" || got.Line != 5 {
		t.Fatalf("later origin without definition lost cross-project fallback: %#v", got)
	}
	if one.pointLookups != 0 || two.pointLookups != 0 {
		t.Fatalf("point lookups leaked into fallback: one=%d two=%d", one.pointLookups, two.pointLookups)
	}
	if one.batchCalls > 1 || two.batchCalls > 1 {
		t.Fatalf("fallback regressed to N+1 batches: one=%d two=%d", one.batchCalls, two.batchCalls)
	}
}

func TestLookupMissingCalleeSymbolsSingleStoreUnchanged(t *testing.T) {
	one := &batchTrackingSymbolStore{symbols: map[string][]trace.Symbol{}}
	refs := []storeReference{{ref: trace.Reference{SymbolName: "Undefined"}, storeIndex: 0}}

	resolved := lookupMissingCalleeSymbols(context.Background(), []trace.SymbolStore{one}, refs, []map[string][]trace.Symbol{{}})

	if len(resolved[0]) != 0 || one.batchCalls != 0 || one.pointLookups != 0 {
		t.Fatalf("single-store semantics changed: resolved=%v batch=%d point=%d", resolved, one.batchCalls, one.pointLookups)
	}
}
