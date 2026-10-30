package mcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/yoanbernabeu/grepai/trace"
)

// MCP trace_callers picks the target and caller definitions like the CLI
// instead of taking the first index entry (#372).
func TestHandleTraceCallersFromStores_should_resolve_symbols_like_cli(t *testing.T) {
	ctx := context.Background()
	ss := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	// b.php is indexed after a.php, so it is never first in insertion order.
	if err := ss.SaveFile(ctx, "a.php", []trace.Symbol{
		{Name: "execute", Kind: "method", File: "a.php", Line: 3, Language: "php"},
		{Name: "handle", Kind: "method", File: "a.php", Line: 10, Language: "php"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := ss.SaveFile(ctx, "b.php", []trace.Symbol{
		{Name: "execute", Kind: "method", File: "b.php", Line: 3, Language: "php"},
		{Name: "handle", Kind: "method", File: "b.php", Line: 10, Language: "php"},
	}, []trace.Reference{
		{SymbolName: "execute", Kind: "call", File: "b.php", Line: 11, CallerName: "handle", CallerFile: "b.php", CallerLine: 10},
	}); err != nil {
		t.Fatal(err)
	}

	s := &Server{}
	for _, compact := range []bool{false, true} {
		res, err := s.handleTraceCallersFromStores(ctx, "", "execute", compact, "json", []trace.SymbolStore{ss})
		if err != nil {
			t.Fatal(err)
		}
		var out struct {
			Symbol  *trace.Symbol `json:"symbol"`
			Callers []struct {
				Symbol trace.Symbol `json:"symbol"`
			} `json:"callers"`
		}
		if err := json.Unmarshal([]byte(res.Content[0].(mcp.TextContent).Text), &out); err != nil {
			t.Fatal(err)
		}
		refs, _ := ss.LookupCallers(ctx, "execute")
		syms, _ := ss.LookupSymbol(ctx, "execute")
		want := trace.PickTargetSymbol(syms, refs)
		if out.Symbol == nil || out.Symbol.File != want.File || want.File != "b.php" {
			t.Fatalf("compact=%v: target symbol %+v, CLI picks %s", compact, out.Symbol, want.File)
		}
		if len(out.Callers) != 1 || out.Callers[0].Symbol.File != "b.php" {
			t.Fatalf("compact=%v: caller symbol should be resolved in b.php, got %+v", compact, out.Callers)
		}
	}
}

// A caller defined in another workspace store is still resolved, and a
// caller without any definition keeps the reference location.
func TestCallerSymbolForRef_fallbacks(t *testing.T) {
	ctx := context.Background()
	ss1 := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	ss2 := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	if err := ss2.SaveFile(ctx, "/p2/x.go", []trace.Symbol{{Name: "Run", File: "/p2/x.go", Line: 4}}, nil); err != nil {
		t.Fatal(err)
	}
	stores := []trace.SymbolStore{ss1, ss2}

	got := callerSymbolForRef(ctx, stores, trace.Reference{CallerName: "Run", CallerFile: "/p2/x.go", CallerLine: 4})
	if got.File != "/p2/x.go" || got.Line != 4 {
		t.Fatalf("expected /p2/x.go:4, got %+v", got)
	}
	got = callerSymbolForRef(ctx, stores, trace.Reference{CallerName: "Missing", CallerFile: "/p1/y.go", CallerLine: 7})
	if got.Name != "Missing" || got.File != "/p1/y.go" || got.Line != 7 {
		t.Fatalf("expected synthetic caller symbol, got %+v", got)
	}
}
