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

// In a workspace, each caller is resolved in the store its reference comes
// from, even when another project has the same relative file (#372).
func TestHandleTraceCallersFromStores_should_resolve_callers_in_their_own_store(t *testing.T) {
	ctx := context.Background()
	api := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	web := trace.NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	if err := web.SaveFile(ctx, "src/main.go", []trace.Symbol{
		{Name: "handle", Kind: "function", File: "src/main.go", Line: 3, Language: "go"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := api.SaveFile(ctx, "src/main.go", []trace.Symbol{
		{Name: "handle", Kind: "function", File: "src/main.go", Line: 40, Language: "go"},
		{Name: "save", Kind: "function", File: "src/main.go", Line: 60, Language: "go"},
	}, []trace.Reference{
		{SymbolName: "save", Kind: "call", File: "src/main.go", Line: 41, CallerName: "handle", CallerFile: "src/main.go", CallerLine: 40},
	}); err != nil {
		t.Fatal(err)
	}

	s := &Server{}
	res, err := s.handleTraceCallersFromStores(ctx, "", "save", false, "json", []trace.SymbolStore{web, api})
	if err != nil {
		t.Fatal(err)
	}
	var out trace.TraceResult
	if err := json.Unmarshal([]byte(res.Content[0].(mcp.TextContent).Text), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Callers) != 1 || out.Callers[0].Symbol.Line != 40 {
		t.Fatalf("expected caller handle at src/main.go:40 from its own store, got %+v", out.Callers)
	}
}
