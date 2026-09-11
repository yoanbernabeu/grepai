package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/trace"
)

type compoundCLIStore struct {
	trace.SymbolStore
	callerResult trace.CallerLookupResult
	calleeResult trace.CalleeLookupResult
	refsResult   trace.RefsLookupResult
	callerCalls  int
	calleeCalls  int
	refsCalls    int
}

func (s *compoundCLIStore) LookupCallerResult(context.Context, string) (trace.CallerLookupResult, error) {
	s.callerCalls++
	return s.callerResult, nil
}

func (s *compoundCLIStore) LookupCalleeResult(context.Context, string, string) (trace.CalleeLookupResult, error) {
	s.calleeCalls++
	return s.calleeResult, nil
}

func (s *compoundCLIStore) LookupRefsResult(context.Context, string) (trace.RefsLookupResult, error) {
	s.refsCalls++
	return s.refsResult, nil
}

func TestTraceCallersStoreLookupRoutesThroughCompoundCapability(t *testing.T) {
	store := &compoundCLIStore{callerResult: trace.CallerLookupResult{
		Symbols: map[string][]trace.Symbol{
			"Target": {{Name: "Target", File: "target.go"}},
			"Caller": {{Name: "Caller", File: "caller.go"}},
		},
		References: []trace.Reference{{SymbolName: "Target", CallerName: "Caller", CallerFile: "caller.go", File: "use.go", Line: 2}},
	}}
	target, callers, err := lookupCallersFromStore(context.Background(), store, "Target")
	if err != nil || store.callerCalls != 1 || target == nil || target.File != "target.go" || len(callers) != 1 || callers[0].Symbol.File != "caller.go" {
		t.Fatalf("compound route target=%#v callers=%#v called=%d err=%v", target, callers, store.callerCalls, err)
	}
}

func TestTraceCalleesStoreLookupRoutesThroughCompoundCapability(t *testing.T) {
	store := &compoundCLIStore{calleeResult: trace.CalleeLookupResult{
		Symbols: map[string][]trace.Symbol{
			"Target": {{Name: "Target", File: "target.go"}},
			"Callee": {{Name: "Callee", File: "callee.go"}},
		},
		References: []trace.Reference{{SymbolName: "Callee", File: "use.go", Line: 3}},
	}}
	target, callees, err := lookupCalleesFromStore(context.Background(), store, "Target")
	if err != nil || store.calleeCalls != 1 || target == nil || target.File != "target.go" || len(callees) != 1 || callees[0].Symbol.File != "callee.go" {
		t.Fatalf("compound route target=%#v callees=%#v called=%d err=%v", target, callees, store.calleeCalls, err)
	}
}

func TestRefsStoreLookupRoutesGraphAndKindThroughCompoundCapability(t *testing.T) {
	store := &compoundCLIStore{refsResult: trace.RefsLookupResult{
		Symbols: map[string][]trace.Symbol{"Reader": {{Name: "Reader", File: "reader.go"}}, "Writer": {{Name: "Writer", File: "writer.go"}}},
		References: []trace.Reference{
			{Kind: trace.RefKindRead, CallerName: "Reader", CallerFile: "reader.go"},
			{Kind: trace.RefKindWrite, CallerName: "Writer", CallerFile: "writer.go"},
		},
	}}
	graph := lookupRefsFromStores(context.Background(), []trace.SymbolStore{store}, "uid", true, true)
	readers := lookupRefsFromStores(context.Background(), []trace.SymbolStore{store}, "uid", true, false)
	writers := lookupRefsFromStores(context.Background(), []trace.SymbolStore{store}, "uid", false, true)
	if store.refsCalls != 3 || len(graph.Readers) != 1 || len(graph.Writers) != 1 || len(readers.Readers) != 1 || len(readers.Writers) != 0 || len(writers.Readers) != 0 || len(writers.Writers) != 1 {
		t.Fatalf("compound refs calls=%d graph=%#v readers=%#v writers=%#v", store.refsCalls, graph, readers, writers)
	}
}

func captureCommandStdout(t *testing.T, run func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := run()
	_ = w.Close()
	os.Stdout = old
	output, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	return string(output), runErr
}

func setTraceCommandFlags(t *testing.T, workspace, project string, compact bool) {
	t.Helper()
	oldWorkspace, oldProject := traceWorkspace, traceProject
	oldJSON, oldTOON, oldCompact, oldUI := traceJSON, traceTOON, traceCompact, traceUI
	t.Cleanup(func() {
		traceWorkspace, traceProject = oldWorkspace, oldProject
		traceJSON, traceTOON, traceCompact, traceUI = oldJSON, oldTOON, oldCompact, oldUI
	})
	traceWorkspace, traceProject = workspace, project
	traceJSON, traceTOON, traceCompact, traceUI = true, false, compact, false
}

func writeTraceProject(t *testing.T, root string, symbols []trace.Symbol, refs []trace.Reference) {
	t.Helper()
	cfg := config.DefaultConfig()
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	store := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(root))
	if err := store.SaveFile(context.Background(), "index.go", symbols, refs); err != nil {
		t.Fatal(err)
	}
	if err := store.Persist(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTraceCommandsProjectResolveBatchAndFallback(t *testing.T) {
	const (
		childMarker = "GREPAI_TEST_TRACE_PROJECT_RESOLVE_CHILD"
		rootEnv     = "GREPAI_TEST_TRACE_PROJECT_RESOLVE_ROOT"
	)
	if os.Getenv(childMarker) == "1" {
		testTraceCommandsProjectResolveBatchAndFallback(t, os.Getenv(rootEnv))
		return
	}

	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTraceCommandsProjectResolveBatchAndFallback$")
	cmd.Env = append(os.Environ(), childMarker+"=1", rootEnv+"="+root)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("trace project resolve child helper failed: %v\n%s", err, output)
	}
}

func testTraceCommandsProjectResolveBatchAndFallback(t *testing.T, root string) {
	t.Helper()
	// Given a real project GOB containing resolved and unresolved references.
	writeTraceProject(t, root, []trace.Symbol{
		{Name: "Target", File: "target.go", Line: 1},
		{Name: "Caller", File: "caller.go", Line: 2},
		{Name: "Callee", File: "callee.go", Line: 3},
	}, []trace.Reference{
		{SymbolName: "Target", Kind: trace.RefKindCall, File: "calls.go", Line: 10, CallerName: "Caller", CallerFile: "caller.go", CallerLine: 2},
		{SymbolName: "Target", Kind: trace.RefKindCall, File: "missing.go", Line: 11, CallerName: "MissingCaller", CallerFile: "missing.go", CallerLine: 9},
		{SymbolName: "Callee", Kind: trace.RefKindCall, File: "target.go", Line: 12, CallerName: "Target", CallerFile: "target.go", CallerLine: 1},
		{SymbolName: "MissingCallee", Kind: trace.RefKindCall, File: "target.go", Line: 13, CallerName: "Target", CallerFile: "target.go", CallerLine: 1},
	})
	oldWD, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	setTraceCommandFlags(t, "", "", false)

	// When both real command paths execute.
	callersJSON, err := captureCommandStdout(t, func() error { return runTraceCallers(nil, []string{"Target"}) })
	if err != nil {
		t.Fatal(err)
	}
	calleesJSON, err := captureCommandStdout(t, func() error { return runTraceCallees(nil, []string{"Target"}) })
	if err != nil {
		t.Fatal(err)
	}

	// Then batch resolution and missing-symbol fallback are visible in output.
	var callers, callees trace.TraceResult
	if err := json.Unmarshal([]byte(callersJSON), &callers); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(calleesJSON), &callees); err != nil {
		t.Fatal(err)
	}
	if len(callers.Callers) != 2 || callers.Callers[0].Symbol.File != "caller.go" || callers.Callers[1].Symbol.Name != "MissingCaller" {
		t.Fatalf("callers = %#v", callers.Callers)
	}
	if len(callees.Callees) != 2 || callees.Callees[0].Symbol.File != "callee.go" || callees.Callees[1].Symbol.Name != "MissingCallee" {
		t.Fatalf("callees = %#v", callees.Callees)
	}
}

func TestTraceCommandsWorkspacePreserveDuplicateNameProvenanceCompact(t *testing.T) {
	// Given two projects defining the same caller and callee names.
	home := t.TempDir()
	cleanup := setTestHomeDirCLI(t, home)
	defer cleanup()
	projects := make([]config.ProjectEntry, 0, 2)
	for _, name := range []string{"one", "two"} {
		root := filepath.Join(home, name)
		callerFile, calleeFile := name+"/caller.go", name+"/callee.go"
		writeTraceProject(t, root, []trace.Symbol{{Name: "Target", File: name + "/target.go", Line: 1}, {Name: "SharedCaller", File: callerFile, Line: 2}, {Name: "SharedCallee", File: calleeFile, Line: 3}}, []trace.Reference{{SymbolName: "Target", Kind: trace.RefKindCall, File: name + "/use.go", Line: 10, CallerName: "SharedCaller", CallerFile: callerFile, CallerLine: 2}, {SymbolName: "SharedCallee", Kind: trace.RefKindCall, File: name + "/target.go", Line: 11, CallerName: "Target"}})
		projects = append(projects, config.ProjectEntry{Name: name, Path: root})
	}
	wsCfg := config.DefaultWorkspaceConfig()
	wsCfg.AddWorkspace(config.Workspace{Name: "batch", Projects: projects})
	if err := config.SaveWorkspaceConfig(wsCfg); err != nil {
		t.Fatal(err)
	}
	setTraceCommandFlags(t, "batch", "", true)

	// When workspace callers and callees execute in compact JSON mode.
	callersJSON, err := captureCommandStdout(t, func() error { return runTraceCallers(nil, []string{"Target"}) })
	if err != nil {
		t.Fatal(err)
	}
	calleesJSON, err := captureCommandStdout(t, func() error { return runTraceCallees(nil, []string{"Target"}) })
	if err != nil {
		t.Fatal(err)
	}

	// Then each duplicate resolves to its originating project.
	if !containsAll(callersJSON, "one/caller.go", "two/caller.go") || !containsAll(calleesJSON, "one/callee.go", "two/callee.go") {
		t.Fatalf("workspace outputs lost provenance:\n%s\n%s", callersJSON, calleesJSON)
	}
}

func TestTraceCommandRejectsConfiguredBackendError(t *testing.T) {
	root := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Trace.StoreBackend = "broken"
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	oldWD, _ := os.Getwd()
	_ = os.Chdir(root)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	setTraceCommandFlags(t, "", "", false)
	if err := runTraceCallers(nil, []string{"Target"}); err == nil {
		t.Fatal("expected configured backend error")
	}
}

func TestTraceCommandsProjectReturnEmptyResultForMissingSymbol(t *testing.T) {
	// Given a non-empty index that does not define the requested symbol.
	root := t.TempDir()
	writeTraceProject(t, root, []trace.Symbol{{Name: "Other", File: "other.go", Line: 1}}, nil)
	oldWD, _ := os.Getwd()
	_ = os.Chdir(root)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	setTraceCommandFlags(t, "", "", false)

	// When both project commands query a missing symbol.
	for _, run := range []func() error{
		func() error { return runTraceCallers(nil, []string{"Missing"}) },
		func() error { return runTraceCallees(nil, []string{"Missing"}) },
	} {
		output, err := captureCommandStdout(t, run)
		if err != nil {
			t.Fatal(err)
		}
		var result trace.TraceResult
		if err := json.Unmarshal([]byte(output), &result); err != nil || result.Symbol != nil || result.Query != "Missing" {
			t.Fatalf("missing result = %#v, %v", result, err)
		}
	}
}

func TestTraceCommandsProjectRejectEmptyIndex(t *testing.T) {
	// Given a valid project with an empty symbol index.
	root := t.TempDir()
	writeTraceProject(t, root, nil, nil)
	oldWD, _ := os.Getwd()
	_ = os.Chdir(root)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	setTraceCommandFlags(t, "", "", false)

	// When callers and callees run, both fail before lookup.
	for _, run := range []func() error{
		func() error { return runTraceCallers(nil, []string{"Missing"}) },
		func() error { return runTraceCallees(nil, []string{"Missing"}) },
	} {
		if _, err := captureCommandStdout(t, run); err == nil {
			t.Fatal("empty index unexpectedly accepted")
		}
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
