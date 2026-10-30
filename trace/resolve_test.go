package trace

import (
	"context"
	"path/filepath"
	"testing"
)

func extractAndSaveForTest(t *testing.T, s *GOBSymbolStore, path, content string) {
	t.Helper()
	ctx := context.Background()
	e := NewRegexExtractor()
	syms, err := e.ExtractSymbols(ctx, path, content)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := e.ExtractReferences(ctx, path, content)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveFile(ctx, path, syms, refs); err != nil {
		t.Fatal(err)
	}
}

const orderActionPHP = `<?php
class OrderAction {
    public function execute($order) {
        $this->validate($order);
        $this->repository->save($order);
    }
}
`

const invoiceActionPHP = `<?php
class InvoiceAction {
    public function execute($invoice) {
        $this->mailer->send($invoice);
    }
}
`

func calleeNames(t *testing.T, s *GOBSymbolStore, name, file string) map[string]int {
	t.Helper()
	callees, err := s.LookupCallees(context.Background(), name, file)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]int{}
	for _, c := range callees {
		names[c.SymbolName]++
	}
	return names
}

// Property reads/writes also produce call-graph edges; they are not callees,
// even when they come first (#372).
func TestLookupCallees_should_skip_property_read_and_write_edges(t *testing.T) {
	s := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	ctx := context.Background()
	syms := []Symbol{{Name: "Run", Kind: KindFunction, File: "a.go", Line: 1, Language: "go"}}
	refs := []Reference{
		{SymbolName: "count", Kind: RefKindRead, File: "a.go", Line: 2, CallerName: "Run", CallerFile: "a.go", CallerLine: 1},
		{SymbolName: "Save", Kind: RefKindCall, File: "a.go", Line: 3, CallerName: "Run", CallerFile: "a.go", CallerLine: 1},
		{SymbolName: "total", Kind: RefKindWrite, File: "a.go", Line: 4, CallerName: "Run", CallerFile: "a.go", CallerLine: 1},
	}
	if err := s.SaveFile(ctx, "a.go", syms, refs); err != nil {
		t.Fatal(err)
	}

	callees, err := s.LookupCallees(ctx, "Run", "a.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(callees) != 1 || callees[0].SymbolName != "Save" || callees[0].Kind != RefKindCall {
		t.Fatalf("expected only the Save call, got %+v", callees)
	}
}

// Legacy indexes may hold references without a kind: they are calls.
func TestLookupCallees_should_keep_references_without_kind(t *testing.T) {
	s := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	ctx := context.Background()
	refs := []Reference{
		{SymbolName: "Save", File: "a.go", Line: 3, CallerName: "Run", CallerFile: "a.go", CallerLine: 1, Context: "Save()"},
		{SymbolName: "Load", File: "a.go", Line: 4, CallerName: "Run", CallerFile: "a.go", CallerLine: 1, Context: "Load()"},
	}
	if err := s.SaveFile(ctx, "a.go", []Symbol{{Name: "Run", File: "a.go", Line: 1}}, refs); err != nil {
		t.Fatal(err)
	}
	names := calleeNames(t, s, "Run", "a.go")
	if names["Save"] != 1 || names["Load"] != 1 || len(names) != 2 {
		t.Fatalf("expected Save and Load once each, got %v", names)
	}
}

func TestLookupCallees_should_keep_two_calls_on_the_same_line(t *testing.T) {
	s := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	extractAndSaveForTest(t, s, "main.go", "package main\n\nfunc Run() {\n\tSave(Load())\n}\n\nfunc Save(x int) {}\n\nfunc Load() int { return 1 }\n")

	names := calleeNames(t, s, "Run", "main.go")
	if names["Save"] != 1 || names["Load"] != 1 {
		t.Fatalf("expected Save and Load once each, got %v", names)
	}
}

// The declaration line of a function is not a call to itself (consistent
// with GetCallGraph), but a real recursive call is kept.
func TestLookupCallees_should_drop_declaration_line_and_keep_recursion(t *testing.T) {
	s := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	extractAndSaveForTest(t, s, "main.go", "package main\n\nfunc Walk(n int) {\n\tif n > 0 {\n\t\tWalk(n - 1)\n\t}\n}\n")

	callees, err := s.LookupCallees(context.Background(), "Walk", "main.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(callees) != 1 || callees[0].SymbolName != "Walk" || callees[0].Line != 5 {
		t.Fatalf("expected only the recursive call on line 5, got %+v", callees)
	}
}

// Callees stay merged across same-name definitions (#372 option B).
func TestLookupCallees_should_merge_same_name_definitions(t *testing.T) {
	s := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	extractAndSaveForTest(t, s, "src/OrderAction.php", orderActionPHP)
	extractAndSaveForTest(t, s, "src/InvoiceAction.php", invoiceActionPHP)

	names := calleeNames(t, s, "execute", "src/OrderAction.php")
	for _, want := range []string{"validate", "save", "send"} {
		if names[want] != 1 {
			t.Errorf("expected callee %q once, got %v", want, names)
		}
	}
	if names["execute"] != 0 {
		t.Errorf("declaration lines reported as callees: %v", names)
	}
}

// Re-saving a file (a watch event) must not change which definition comes
// first (#212).
func TestLookupSymbol_should_not_depend_on_index_insertion_order(t *testing.T) {
	ctx := context.Background()
	s1 := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	extractAndSaveForTest(t, s1, "src/OrderAction.php", orderActionPHP)
	extractAndSaveForTest(t, s1, "src/InvoiceAction.php", invoiceActionPHP)

	s2 := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	extractAndSaveForTest(t, s2, "src/OrderAction.php", orderActionPHP)
	extractAndSaveForTest(t, s2, "src/InvoiceAction.php", invoiceActionPHP)
	extractAndSaveForTest(t, s2, "src/OrderAction.php", orderActionPHP)

	a, _ := s1.LookupSymbol(ctx, "execute")
	b, _ := s2.LookupSymbol(ctx, "execute")
	if len(a) != 2 || len(b) != 2 {
		t.Fatalf("expected 2 definitions, got %d and %d", len(a), len(b))
	}
	for i := range a {
		if a[i].File != b[i].File || a[i].Line != b[i].Line {
			t.Fatalf("definition order depends on history: %v vs %v", a, b)
		}
	}
	if a[0].File != "src/InvoiceAction.php" {
		t.Fatalf("expected definitions ordered by file, got %s first", a[0].File)
	}

	g1, _ := s1.GetCallGraph(ctx, "execute", 1)
	g2, _ := s2.GetCallGraph(ctx, "execute", 1)
	if g1.Nodes["execute"].File != g2.Nodes["execute"].File {
		t.Fatalf("graph root depends on history: %s vs %s", g1.Nodes["execute"].File, g2.Nodes["execute"].File)
	}
}

func TestPickTargetSymbol_prefersSameFileReferences(t *testing.T) {
	candidates := []Symbol{
		{Name: "isAdmin", File: "src/components/Navigation.vue", Line: 10},
		{Name: "isAdmin", File: "src/views/Home.vue", Line: 20},
	}
	refs := []Reference{
		{SymbolName: "isAdmin", File: "src/views/Home.vue", CallerFile: "src/views/Home.vue", Line: 49},
	}

	picked := PickTargetSymbol(candidates, refs)
	if picked == nil || picked.File != "src/views/Home.vue" {
		t.Fatalf("expected Home.vue symbol, got %+v", picked)
	}
}

func TestPickTargetSymbol_should_break_ties_by_file_then_line(t *testing.T) {
	a := Symbol{Name: "execute", File: "b.php", Line: 3}
	b := Symbol{Name: "execute", File: "a.php", Line: 9}
	c := Symbol{Name: "execute", File: "a.php", Line: 3}
	refs := []Reference{
		{SymbolName: "execute", File: "a.php", CallerFile: "a.php"},
		{SymbolName: "execute", File: "b.php", CallerFile: "b.php"},
	}
	for _, candidates := range [][]Symbol{{a, b, c}, {c, b, a}, {b, a, c}} {
		picked := PickTargetSymbol(candidates, refs)
		if picked.File != "a.php" || picked.Line != 3 {
			t.Fatalf("expected a.php:3 for %v, got %s:%d", candidates, picked.File, picked.Line)
		}
		picked = PickTargetSymbol(candidates, nil)
		if picked.File != "a.php" || picked.Line != 3 {
			t.Fatalf("expected a.php:3 without refs for %v, got %s:%d", candidates, picked.File, picked.Line)
		}
	}
}

func TestPickSymbolForFile_prefersCallerFile(t *testing.T) {
	candidates := []Symbol{
		{Name: "__vue_template_reads__", File: "src/App.vue", Line: 100},
		{Name: "__vue_template_reads__", File: "src/views/Home.vue", Line: 200},
	}

	picked := PickSymbolForFile(candidates, "src/views/Home.vue")
	if picked == nil || picked.File != "src/views/Home.vue" {
		t.Fatalf("expected Home.vue caller symbol, got %+v", picked)
	}
}

func TestPickSymbolForFile_should_not_depend_on_candidate_order(t *testing.T) {
	x := Symbol{Name: "handle", File: "z.php", Line: 1}
	y := Symbol{Name: "handle", File: "m.php", Line: 8}
	z := Symbol{Name: "handle", File: "m.php", Line: 2}
	for _, candidates := range [][]Symbol{{x, y, z}, {z, y, x}} {
		if p := PickSymbolForFile(candidates, "m.php"); p.Line != 2 {
			t.Fatalf("expected m.php:2, got %s:%d", p.File, p.Line)
		}
		if p := PickSymbolForFile(candidates, "other.php"); p.File != "m.php" || p.Line != 2 {
			t.Fatalf("expected m.php:2 fallback, got %s:%d", p.File, p.Line)
		}
	}
	if PickSymbolForFile(nil, "a.php") != nil {
		t.Fatal("expected nil for no candidates")
	}
}

// trace graph does not show property reads/writes as call edges either.
func TestGetCallGraph_should_skip_property_read_and_write_edges(t *testing.T) {
	s := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	ctx := context.Background()
	syms := []Symbol{
		{Name: "Run", Kind: KindFunction, File: "a.go", Line: 1, Language: "go"},
		{Name: "count", Kind: KindFunction, File: "a.go", Line: 10, Language: "go"},
	}
	refs := []Reference{
		{SymbolName: "count", Kind: RefKindRead, File: "a.go", Line: 2, CallerName: "Run", CallerFile: "a.go", CallerLine: 1},
		{SymbolName: "Save", Kind: RefKindCall, File: "a.go", Line: 3, CallerName: "Run", CallerFile: "a.go", CallerLine: 1},
		{SymbolName: "Run", Kind: RefKindWrite, File: "a.go", Line: 11, CallerName: "count", CallerFile: "a.go", CallerLine: 10},
	}
	if err := s.SaveFile(ctx, "a.go", syms, refs); err != nil {
		t.Fatal(err)
	}

	graph, err := s.GetCallGraph(ctx, "Run", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Edges) != 1 || graph.Edges[0].Callee != "Save" {
		t.Fatalf("expected only Run->Save, got %+v", graph.Edges)
	}
	if _, ok := graph.Nodes["count"]; ok {
		t.Fatalf("graph walked into a property access: %+v", graph.Nodes)
	}
}
