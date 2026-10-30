package trace

import (
	"context"
	"path/filepath"
	"strings"
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

	picked := PickSymbolForFile(candidates, "src/views/Home.vue", 0)
	if picked == nil || picked.File != "src/views/Home.vue" {
		t.Fatalf("expected Home.vue caller symbol, got %+v", picked)
	}
}

func TestPickSymbolForFile_should_not_depend_on_candidate_order(t *testing.T) {
	x := Symbol{Name: "handle", File: "z.php", Line: 1}
	y := Symbol{Name: "handle", File: "m.php", Line: 8}
	z := Symbol{Name: "handle", File: "m.php", Line: 2}
	for _, candidates := range [][]Symbol{{x, y, z}, {z, y, x}} {
		if p := PickSymbolForFile(candidates, "m.php", 0); p.Line != 2 {
			t.Fatalf("expected m.php:2, got %s:%d", p.File, p.Line)
		}
		if p := PickSymbolForFile(candidates, "other.php", 0); p.File != "m.php" || p.Line != 2 {
			t.Fatalf("expected m.php:2 fallback, got %s:%d", p.File, p.Line)
		}
	}
	if PickSymbolForFile(nil, "a.php", 0) != nil {
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

// Several same-name definitions in one file (e.g. two PHP classes): the
// caller's declaration line picks the right one.
func TestPickSymbolForFile_should_prefer_caller_line(t *testing.T) {
	candidates := []Symbol{
		{Name: "handle", File: "x.php", Line: 20},
		{Name: "handle", File: "x.php", Line: 3},
		{Name: "handle", File: "a.php", Line: 1},
	}
	if p := PickSymbolForFile(candidates, "x.php", 20); p.Line != 20 {
		t.Fatalf("expected x.php:20, got %s:%d", p.File, p.Line)
	}
	if p := PickSymbolForFile(candidates, "x.php", 0); p.Line != 3 {
		t.Fatalf("expected x.php:3 without caller line, got %s:%d", p.File, p.Line)
	}
	if p := PickSymbolForFile(candidates, "x.php", 99); p.Line != 3 {
		t.Fatalf("expected x.php:3 for an unknown caller line, got %s:%d", p.File, p.Line)
	}
}

// The extractor records the caller's declaration line on references, which
// is what PickSymbolForFile matches against.
func TestReferenceCallerLine_should_match_caller_declaration(t *testing.T) {
	const src = `<?php
class A {
    public function handle() {
        return 1;
    }
}
class B {
    public function handle() {
        $this->execute();
    }
}
`
	s := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	extractAndSaveForTest(t, s, "x.php", src)
	ctx := context.Background()
	refs, _ := s.LookupCallers(ctx, "execute")
	if len(refs) != 1 {
		t.Fatalf("expected 1 reference, got %+v", refs)
	}
	syms, _ := s.LookupSymbol(ctx, "handle")
	p := PickSymbolForFile(syms, refs[0].CallerFile, refs[0].CallerLine)
	if p == nil || p.Line != 8 {
		t.Fatalf("expected B::handle at line 8, got %+v (caller line %d)", p, refs[0].CallerLine)
	}
}

// A one-line method that delegates to a same-name method, or recurses, keeps
// its callee: only the bare declaration line is dropped.
func TestLookupCallees_should_keep_one_line_delegation_and_recursion(t *testing.T) {
	cases := []struct {
		path, src, name string
	}{
		{"w.go", "package w\n\ntype W struct{ inner *W }\n\nfunc (w *W) Save() error { return w.inner.Save() }\n", "Save"},
		{"Decorator.php", "<?php\nclass Decorator {\n    public function execute() { return $this->inner->execute(); }\n}\n", "execute"},
		{"fact.go", "package m\n\nfunc fact(n int) int { if n < 2 { return 1 }; return n * fact(n-1) }\n", "fact"},
	}
	for _, c := range cases {
		s := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
		extractAndSaveForTest(t, s, c.path, c.src)
		ctx := context.Background()
		callees, err := s.LookupCallees(ctx, c.name, c.path)
		if err != nil {
			t.Fatal(err)
		}
		if len(callees) != 1 || callees[0].SymbolName != c.name {
			t.Errorf("%s: expected the same-name call to be kept, got %+v", c.path, callees)
		}
	}
}

// Declarations that are not written "name(" (generics, class fields) with a
// one-line delegation, and a long line cut by the extractor, keep the call.
func TestLookupCallees_should_keep_self_call_when_line_may_hold_a_real_call(t *testing.T) {
	long := "public function execute($a) { " + strings.Repeat("$this->log($a); ", 14) + "return $this->inner->execute($a); }"
	cases := []struct{ context, name string }{
		{"execute<T>(x: T) { return this.inner.execute(x); }", "execute"},
		{"func Map[T any](xs []T) []T { return Map(xs[1:]) }", "Map"},
		{long[:200] + "...", "execute"},
	}
	for _, c := range cases {
		s := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
		ctx := context.Background()
		if err := s.SaveFile(ctx, "a.ts", []Symbol{{Name: c.name, File: "a.ts", Line: 1}}, []Reference{
			{SymbolName: c.name, Kind: RefKindCall, File: "a.ts", Line: 1, CallerName: c.name, CallerFile: "a.ts", CallerLine: 1, Context: c.context},
		}); err != nil {
			t.Fatal(err)
		}
		callees, err := s.LookupCallees(ctx, c.name, "a.ts")
		if err != nil {
			t.Fatal(err)
		}
		if len(callees) != 1 {
			t.Errorf("%q: expected the self call to be kept, got %+v", c.context, callees)
		}
	}
}

func TestHasLaterCall(t *testing.T) {
	cases := []struct {
		line, name string
		want       bool
	}{
		{"func (m *MockStore) Close() error {", "Close", false},
		{"func (e *OpenAIEmbedder) BatchLimits() BatchLimits {", "BatchLimits", false},
		{"func (f *fence) cleanup(parent context.Context, cleanup func()) {", "cleanup", false},
		{"set value(value) {", "value", false},
		{"func (w *W) Save() error { return w.inner.Save() }", "Save", true},
		{"public function execute() { return $this->inner->execute (); }", "execute", true},
		{"execute<T>(x: T) { return this.inner.execute(x); }", "execute", true},
		{"func Map[T any](xs []T) []T { return Map[T](xs[1:]) }", "Map", true},
		{"execute<T>(x: T) { return this.inner.execute<T>(x); }", "execute", true},
		{"func Saver() { doSave(); SaveAll() }", "Save", false},
		{"", "Save", false},
		{"Save() Save()", "", false},
	}
	for _, c := range cases {
		if got := hasLaterCall(c.line, c.name); got != c.want {
			t.Errorf("hasLaterCall(%q, %q) = %v, want %v", c.line, c.name, got, c.want)
		}
	}
}

// A declaration whose return type or parameter repeats the name is still a
// declaration, not a callee.
func TestLookupCallees_should_drop_declaration_repeating_the_name(t *testing.T) {
	s := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	extractAndSaveForTest(t, s, "limits.go", "package e\n\ntype BatchLimits struct{}\n\ntype E struct{}\n\nfunc (e *E) BatchLimits() BatchLimits {\n\treturn BatchLimits{}\n}\n")
	callees, err := s.LookupCallees(context.Background(), "BatchLimits", "limits.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range callees {
		if c.Line == 7 {
			t.Fatalf("declaration line reported as a callee: %+v", callees)
		}
	}
}

// getLineContext trims the indentation after cutting a long line, so an
// indented truncated line is shorter than the cut length.
func TestLookupCallees_should_keep_self_call_on_indented_truncated_line(t *testing.T) {
	line := "    public function execute($a) { " + strings.Repeat("$this->log($a); ", 14) + "return $this->inner->execute($a); }"
	ctxLine := getLineContext([]string{line}, 0, 0)
	s := NewGOBSymbolStore(filepath.Join(t.TempDir(), "symbols.gob"))
	ctx := context.Background()
	if err := s.SaveFile(ctx, "a.php", []Symbol{{Name: "execute", File: "a.php", Line: 1}}, []Reference{
		{SymbolName: "execute", Kind: RefKindCall, File: "a.php", Line: 1, CallerName: "execute", CallerFile: "a.php", CallerLine: 1, Context: ctxLine},
	}); err != nil {
		t.Fatal(err)
	}
	callees, _ := s.LookupCallees(ctx, "execute", "a.php")
	if len(callees) != 1 {
		t.Fatalf("expected the delegation on a truncated line to be kept (context %d chars), got %+v", len(ctxLine), callees)
	}
}
