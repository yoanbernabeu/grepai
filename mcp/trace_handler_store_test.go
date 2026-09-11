package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mark3 "github.com/mark3labs/mcp-go/mcp"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/trace"
)

func seedTraceHandlerProject(t *testing.T, root, prefix string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, config.ConfigDir), 0o755); err != nil {
		t.Fatal(err)
	}
	store := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(root))
	if err := store.SaveFile(context.Background(), prefix+"/index.go", []trace.Symbol{
		{Name: "Target", File: prefix + "/target.go", Line: 1},
		{Name: "SharedCaller", File: prefix + "/caller.go", Line: 2},
		{Name: "SharedCallee", File: prefix + "/callee.go", Line: 3},
	}, []trace.Reference{
		{SymbolName: "Target", Kind: trace.RefKindCall, File: prefix + "/use.go", Line: 10, CallerName: "SharedCaller", CallerFile: prefix + "/caller.go", CallerLine: 2},
		{SymbolName: "Target", Kind: trace.RefKindCall, File: prefix + "/missing.go", Line: 11, CallerName: "MissingCaller", CallerFile: prefix + "/missing.go", CallerLine: 9},
		{SymbolName: "SharedCallee", Kind: trace.RefKindCall, File: prefix + "/target.go", Line: 12, CallerName: "Target"},
		{SymbolName: "MissingCallee", Kind: trace.RefKindCall, File: prefix + "/target.go", Line: 13, CallerName: "Target"},
		{SymbolName: "uid", Kind: trace.RefKindRead, File: prefix + "/caller.go", Line: 14, CallerName: "SharedCaller", CallerFile: prefix + "/caller.go", CallerLine: 2},
		{SymbolName: "uid", Kind: trace.RefKindWrite, File: prefix + "/caller.go", Line: 15, CallerName: "SharedCaller", CallerFile: prefix + "/caller.go", CallerLine: 2},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func traceHandlerRequest(arguments map[string]any) mark3.CallToolRequest {
	return mark3.CallToolRequest{Params: mark3.CallToolParams{Arguments: arguments}}
}

func isolateMCPTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	workspacePath, err := config.GetWorkspaceConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(home, workspacePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("workspace config escaped isolated home: home=%q path=%q rel=%q err=%v", home, workspacePath, relative, err)
	}
	return home
}

func TestTraceHandlersProjectUseBatchAndFallback(t *testing.T) {
	// Given a real project symbol index.
	root := t.TempDir()
	seedTraceHandlerProject(t, root, "project")
	server := &Server{projectRoot: root}

	// When the real project handlers run.
	callersResult, err := server.handleTraceCallers(context.Background(), traceHandlerRequest(map[string]any{"symbol": "Target", "format": "json"}))
	if err != nil {
		t.Fatal(err)
	}
	calleesResult, err := server.handleTraceCallees(context.Background(), traceHandlerRequest(map[string]any{"symbol": "Target", "format": "json", "compact": true}))
	if err != nil {
		t.Fatal(err)
	}

	// Then resolved and fallback symbols are returned.
	callersText, calleesText := textResultPayload(t, callersResult), textResultPayload(t, calleesResult)
	if !containsMCPParts(callersText, "project/caller.go", "MissingCaller") || !containsMCPParts(calleesText, "project/callee.go", "MissingCallee") {
		t.Fatalf("unexpected handler output:\n%s\n%s", callersText, calleesText)
	}
}

func TestTraceHandlersWorkspacePreserveOriginatingProject(t *testing.T) {
	// Given a workspace with duplicate symbol names in two projects.
	home := isolateMCPTestHome(t)
	projects := make([]config.ProjectEntry, 0, 2)
	for _, name := range []string{"one", "two"} {
		root := filepath.Join(home, name)
		seedTraceHandlerProject(t, root, name)
		projects = append(projects, config.ProjectEntry{Name: name, Path: root})
	}
	workspaceConfig := config.DefaultWorkspaceConfig()
	workspaceConfig.AddWorkspace(config.Workspace{Name: "handlers", Projects: projects})
	if err := config.SaveWorkspaceConfig(workspaceConfig); err != nil {
		t.Fatal(err)
	}
	server := &Server{workspaceName: "handlers"}

	// When callers and callees are requested through their public handler surface.
	callersResult, err := server.handleTraceCallers(context.Background(), traceHandlerRequest(map[string]any{"symbol": "Target", "format": "json", "compact": true}))
	if err != nil {
		t.Fatal(err)
	}
	calleesResult, err := server.handleTraceCallees(context.Background(), traceHandlerRequest(map[string]any{"symbol": "Target", "format": "json"}))
	if err != nil {
		t.Fatal(err)
	}

	// Then both originating definitions survive duplicate names.
	refsResult, err := server.handleRefsGraph(context.Background(), traceHandlerRequest(map[string]any{"symbol": "uid", "format": "json", "compact": true}))
	if err != nil {
		t.Fatal(err)
	}
	callersText, calleesText, refsText := textResultPayload(t, callersResult), textResultPayload(t, calleesResult), textResultPayload(t, refsResult)
	if !containsMCPParts(callersText, "one/caller.go", "two/caller.go") || !containsMCPParts(calleesText, "one/callee.go", "two/callee.go") || !containsMCPParts(refsText, "one/caller.go", "two/caller.go") {
		t.Fatalf("workspace provenance lost:\n%s\n%s\n%s", callersText, calleesText, refsText)
	}
}

func TestTraceCallersRequiresSymbol(t *testing.T) {
	server := &Server{}
	result, err := server.handleTraceCallers(context.Background(), traceHandlerRequest(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if got := textResultPayload(t, result); got != "symbol parameter is required" {
		t.Fatalf("missing-symbol error = %q", got)
	}
}

func TestTraceCalleesRejectsUnsupportedFormat(t *testing.T) {
	server := &Server{}
	result, err := server.handleTraceCallees(context.Background(), traceHandlerRequest(map[string]any{"symbol": "Target", "format": "xml"}))
	if err != nil {
		t.Fatal(err)
	}
	if got := textResultPayload(t, result); got != "format must be 'json' or 'toon'" {
		t.Fatalf("unsupported-format error = %q", got)
	}
}

type countingBatchSymbolStore struct {
	trace.SymbolStore
	symbols     map[string][]trace.Symbol
	batchCalls  int
	lookupCalls int
	batchNames  []string
}

type compoundMCPStore struct {
	trace.SymbolStore
	callerResult trace.CallerLookupResult
	calleeResult trace.CalleeLookupResult
	refsResult   trace.RefsLookupResult
	callerCalls  int
	calleeCalls  int
	refsCalls    int
}

func (s *compoundMCPStore) LookupCallerResult(context.Context, string) (trace.CallerLookupResult, error) {
	s.callerCalls++
	return s.callerResult, nil
}

func (s *compoundMCPStore) LookupCalleeResult(context.Context, string, string) (trace.CalleeLookupResult, error) {
	s.calleeCalls++
	return s.calleeResult, nil
}

func (s *compoundMCPStore) LookupRefsResult(context.Context, string) (trace.RefsLookupResult, error) {
	s.refsCalls++
	return s.refsResult, nil
}

func TestTraceCallersHandlerRoutesThroughCompoundCapability(t *testing.T) {
	store := &compoundMCPStore{callerResult: trace.CallerLookupResult{
		Symbols: map[string][]trace.Symbol{
			"Target": {{Name: "Target", File: "target.go"}},
			"Caller": {{Name: "Caller", File: "caller.go"}},
		},
		References: []trace.Reference{{SymbolName: "Target", CallerName: "Caller", CallerFile: "caller.go", File: "use.go", Line: 2}},
	}}
	server := &Server{}
	result, err := server.handleTraceCallersFromStores(context.Background(), "Target", false, "json", []trace.SymbolStore{store})
	if err != nil || store.callerCalls != 1 {
		t.Fatalf("compound calls=%d err=%v", store.callerCalls, err)
	}
	if output := textResultPayload(t, result); !containsMCPParts(output, "target.go", "caller.go", "use.go") {
		t.Fatalf("compound handler output = %s", output)
	}
}

func TestCalleeAndRefsHandlersRouteThroughCompoundCapabilities(t *testing.T) {
	store := &compoundMCPStore{
		calleeResult: trace.CalleeLookupResult{
			Symbols:    map[string][]trace.Symbol{"Target": {{Name: "Target", File: "target.go"}}, "Callee": {{Name: "Callee", File: "callee.go"}}},
			References: []trace.Reference{{SymbolName: "Callee", File: "use.go", Line: 2}},
		},
		refsResult: trace.RefsLookupResult{
			Symbols:    map[string][]trace.Symbol{"Reader": {{Name: "Reader", File: "reader.go"}}, "Writer": {{Name: "Writer", File: "writer.go"}}},
			References: []trace.Reference{{Kind: trace.RefKindRead, CallerName: "Reader", CallerFile: "reader.go"}, {Kind: trace.RefKindWrite, CallerName: "Writer", CallerFile: "writer.go"}},
		},
	}
	server := &Server{}
	calleeOutput, err := server.handleTraceCalleesFromStores(context.Background(), "Target", true, "json", []trace.SymbolStore{store})
	if err != nil {
		t.Fatal(err)
	}
	graphOutput, err := server.handleRefsGraphFromStores(context.Background(), "uid", false, "json", []trace.SymbolStore{store})
	if err != nil {
		t.Fatal(err)
	}
	readerOutput, err := server.handleRefsFromStores(context.Background(), "uid", trace.RefKindRead, true, "json", []trace.SymbolStore{store})
	if err != nil {
		t.Fatal(err)
	}
	writerOutput, err := server.handleRefsFromStores(context.Background(), "uid", trace.RefKindWrite, false, "json", []trace.SymbolStore{store})
	if err != nil {
		t.Fatal(err)
	}
	if store.calleeCalls != 1 || store.refsCalls != 3 || !containsMCPParts(textResultPayload(t, calleeOutput), "callee.go") || !containsMCPParts(textResultPayload(t, graphOutput), "reader.go", "writer.go") || !containsMCPParts(textResultPayload(t, readerOutput), "reader.go") || !containsMCPParts(textResultPayload(t, writerOutput), "writer.go") {
		t.Fatalf("compound routes callee=%d refs=%d", store.calleeCalls, store.refsCalls)
	}
}

func (s *countingBatchSymbolStore) LookupSymbolsBatch(_ context.Context, names []string) (map[string][]trace.Symbol, error) {
	s.batchCalls++
	s.batchNames = append([]string(nil), names...)
	return s.symbols, nil
}

func (s *countingBatchSymbolStore) LookupSymbol(context.Context, string) ([]trace.Symbol, error) {
	s.lookupCalls++
	return nil, nil
}

func TestLookupSymbolsByOriginBatchesOncePerStoreWithoutPointLookups(t *testing.T) {
	one := &countingBatchSymbolStore{symbols: map[string][]trace.Symbol{"Shared": {{Name: "Shared", File: "one.go"}}}}
	two := &countingBatchSymbolStore{symbols: map[string][]trace.Symbol{"Shared": {{Name: "Shared", File: "two.go"}}}}
	refs := []storeReference{
		{ref: trace.Reference{CallerName: "Shared", CallerFile: "one.go"}, storeIndex: 0},
		{ref: trace.Reference{CallerName: "Shared", CallerFile: "one.go"}, storeIndex: 0},
		{ref: trace.Reference{CallerName: "Missing", CallerFile: "missing.go", CallerLine: 9}, storeIndex: 0},
		{ref: trace.Reference{CallerName: "Shared", CallerFile: "two.go"}, storeIndex: 1},
	}

	resolved := lookupSymbolsByOrigin(context.Background(), []trace.SymbolStore{one, two}, refs, true, "caller")

	if one.batchCalls != 1 || two.batchCalls != 1 || one.lookupCalls != 0 || two.lookupCalls != 0 {
		t.Fatalf("calls: one batch/lookup=%d/%d two=%d/%d", one.batchCalls, one.lookupCalls, two.batchCalls, two.lookupCalls)
	}
	if len(one.batchNames) != 2 || len(two.batchNames) != 1 {
		t.Fatalf("batch names were not unique: one=%v two=%v", one.batchNames, two.batchNames)
	}
	if got := resolveRefCallerSymbol(resolved[1], refs[3].ref); got.File != "two.go" {
		t.Fatalf("originating store resolution = %#v", got)
	}
	if got := resolveRefCallerSymbol(resolved[0], refs[2].ref); got.Name != "Missing" || got.File != "missing.go" || got.Line != 9 {
		t.Fatalf("missing-symbol fallback = %#v", got)
	}
}

func containsMCPParts(value string, parts ...string) bool {
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return false
	}
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
