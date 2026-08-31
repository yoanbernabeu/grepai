package trace

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// callEdge is one (callee, caller) pair from ExtractReferences, which is the
// only thing `grepai trace callers`/`callees` actually consumes.
type callEdge struct {
	callee string
	caller string
}

func extractCallEdges(t *testing.T, ext, source string) []callEdge {
	t.Helper()
	ex, err := NewTreeSitterExtractor()
	if err != nil {
		t.Fatalf("NewTreeSitterExtractor: %v", err)
	}
	refs, err := ex.ExtractReferences(context.Background(), "sample"+ext, source)
	if err != nil {
		t.Fatalf("ExtractReferences(%s): %v", ext, err)
	}
	var out []callEdge
	for _, r := range refs {
		if r.Kind != RefKindCall {
			continue
		}
		out = append(out, callEdge{callee: r.SymbolName, caller: r.CallerName})
	}
	return out
}

func hasCallEdge(edges []callEdge, want callEdge) bool {
	for _, e := range edges {
		if e == want {
			return true
		}
	}
	return false
}

func formatEdges(edges []callEdge) string {
	seen := make([]string, 0, len(edges))
	for _, e := range edges {
		seen = append(seen, fmt.Sprintf("%s <- %s", e.callee, e.caller))
	}
	sort.Strings(seen)
	return strings.Join(seen, "\n  ")
}

// TestExtractReferences_CallGraphPerLanguage is the regression net the review
// of PR #260 asked for: every language whose LangSpec claims call-graph
// support must attribute a call to the function that contains it. Before the
// node types moved into LangSpec, the walker only knew `call_expression` /
// `invocation_expression` and five function node types, so Java calls
// (`method_invocation`) were invisible and Rust callers (`function_item`) came
// back as `<top-level>`.
func TestExtractReferences_CallGraphPerLanguage(t *testing.T) {
	tests := []struct {
		ext    string
		source string
		want   []callEdge
	}{
		{
			ext:    ".go",
			source: "package p\n\nfunc helper() {}\n\nfunc run() {\n\thelper()\n\tother.helper(1)\n}\n",
			want:   []callEdge{{"helper", "run"}},
		},
		{
			ext:    ".js",
			source: "function helper() {}\nfunction run() { helper(); }\nclass C { m() { helper(); } }\n",
			want:   []callEdge{{"helper", "run"}, {"helper", "m"}},
		},
		{
			ext:    ".ts",
			source: "function helper(): void {}\nfunction run(): void { helper(); }\nclass C { m(): void { helper(); } }\n",
			want:   []callEdge{{"helper", "run"}, {"helper", "m"}},
		},
		{
			// Python's call node is `call`, not `call_expression`; before
			// this change no Python reference was ever extracted.
			ext:    ".py",
			source: "def helper():\n    pass\n\ndef run():\n    helper()\n    obj.helper(1)\n",
			want:   []callEdge{{"helper", "run"}},
		},
		{
			// Same story for PHP: function_call_expression /
			// member_call_expression, neither of which the old walker knew.
			ext:    ".php",
			source: "<?php\nfunction helper() {}\nfunction run() {\n  helper();\n  $o->helper(1);\n}\n",
			want:   []callEdge{{"helper", "run"}},
		},
		{
			ext:    ".cs",
			source: "class A { void Helper() {} void Run() { Helper(); this.Helper(); } }\n",
			want:   []callEdge{{"Helper", "Run"}},
		},
		{
			// A bare `helper` with no receiver and no parens parses as an
			// identifier in Ruby, so only the parenthesized call is seen.
			ext:    ".rb",
			source: "def helper\n  1\nend\n\ndef run\n  helper()\n  other.helper(2)\nend\n",
			want:   []callEdge{{"helper", "run"}},
		},
		{
			// The maintainer's Rust repro: the caller used to come back as
			// `<top-level>` because the function node is `function_item`.
			ext:    ".rs",
			source: "fn helper() {}\n\nfn main() {\n    helper();\n    foo::bar(1);\n}\n",
			want:   []callEdge{{"helper", "main"}, {"bar", "main"}},
		},
		{
			// The maintainer's Java repro: `method_invocation` was invisible,
			// so `trace callers helperMethod` returned zero callers.
			ext:    ".java",
			source: "class A {\n  void helperMethod() {}\n  void run() {\n    helperMethod();\n    this.helperMethod();\n  }\n  A() { helperMethod(); }\n}\n",
			want:   []callEdge{{"helperMethod", "run"}, {"helperMethod", "A"}},
		},
		{
			ext:    ".scala",
			source: "object A {\n  def helper(): Unit = {}\n  def run(): Unit = { helper(); other.helper(1) }\n}\n",
			want:   []callEdge{{"helper", "run"}},
		},
		{
			ext:    ".c",
			source: "void helper(void) {}\n\nint main(void) {\n  helper();\n  return 0;\n}\n",
			want:   []callEdge{{"helper", "main"}},
		},
		{
			ext:    ".cpp",
			source: "void helper() {}\n\nint main() { helper(); return 0; }\n",
			want:   []callEdge{{"helper", "main"}},
		},
		{
			// In a shell every command is a call; there is no way to tell a
			// function invocation from a binary on $PATH.
			ext:    ".sh",
			source: "helper() {\n  echo hi\n}\n\nrun() {\n  helper\n}\n",
			want:   []callEdge{{"helper", "run"}, {"echo", "helper"}},
		},
		{
			ext:    ".lua",
			source: "local function helper() end\n\nfunction run()\n  helper()\n  t.helper(1)\nend\n",
			want:   []callEdge{{"helper", "run"}},
		},
		{
			// Kotlin's call_expression and function_declaration carry no
			// fields at all, so both names resolve via first-named-child.
			ext:    ".kt",
			source: "fun helper() {}\n\nfun run() {\n    helper()\n    other.helper(1)\n}\n",
			want:   []callEdge{{"helper", "run"}},
		},
		{
			ext:    ".swift",
			source: "func helper() {}\n\nfunc run() {\n    helper()\n    other.helper(1)\n}\n",
			want:   []callEdge{{"helper", "run"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			edges := extractCallEdges(t, tt.ext, tt.source)
			for _, want := range tt.want {
				if !hasCallEdge(edges, want) {
					t.Errorf("missing call edge %s <- %s; got:\n  %s", want.callee, want.caller, formatEdges(edges))
				}
			}
			for _, e := range edges {
				if e.caller == "<top-level>" {
					continue // module-level calls legitimately have no caller
				}
				if e.callee == "" || e.caller == "" {
					t.Errorf("incomplete call edge %+v in:\n  %s", e, formatEdges(edges))
				}
			}
		})
	}
}

// TestSupportsCallGraph_MatchesRegistry keeps the "which languages have a
// call graph" answer honest: a language that declares CallNodes must produce
// at least one reference for a call, and one that declares none must produce
// none. Adding a grammar without wiring its call nodes then shows up here
// rather than silently returning an empty call graph to users.
func TestSupportsCallGraph_MatchesRegistry(t *testing.T) {
	declarative := []string{".sql", ".proto", ".hcl", ".tf", ".elm", ".toml", ".el", ".ex", ".exs"}
	for _, ext := range declarative {
		if SupportsCallGraph(ext) {
			t.Errorf("SupportsCallGraph(%q) = true, want false: %s has no call-graph node types", ext, ext)
		}
	}
	for _, spec := range treeSitterLanguages {
		for _, ext := range spec.Extensions {
			want := len(spec.CallNodes) > 0
			if got := SupportsCallGraph(ext); got != want {
				t.Errorf("SupportsCallGraph(%q) = %v, want %v (language %s)", ext, got, want, spec.Name)
			}
		}
	}
}

// TestExtractReferences_DeclarativeLanguagesYieldNoCalls pins the other half:
// declaring no CallNodes must actually produce no call references, so a future
// grammar addition cannot quietly start emitting noise.
func TestExtractReferences_DeclarativeLanguagesYieldNoCalls(t *testing.T) {
	tests := []struct{ ext, source string }{
		{".sql", "CREATE TABLE t (id INT);\nSELECT count(*) FROM t;\n"},
		{".toml", "[server]\nhost = \"localhost\"\n"},
		{".el", "(defun helper () 1)\n\n(defun run ()\n  (helper))\n"},
		// Elixir: `def`/`defmodule` and a real call share the `call` node
		// type, so nothing is emitted rather than a reference to "def" per
		// definition. See the elixir LangSpec.
		{".ex", "defmodule A do\n  def helper, do: 1\n  def run do\n    helper()\n  end\nend\n"},
	}
	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			if edges := extractCallEdges(t, tt.ext, tt.source); len(edges) != 0 {
				t.Errorf("expected no call references for %s, got:\n  %s", tt.ext, formatEdges(edges))
			}
		})
	}
}

func TestBaseSymbolName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"helper", "helper"},
		{"other.helper", "helper"},
		{"foo::bar", "bar"},
		{"p->m", "m"},
		{"a.b.c", "c"},
		{"", ""},
		{"foo(bar)", ""},
		{"foo bar", ""},
		{"multi\nline", ""},
	}
	for _, tt := range tests {
		if got := baseSymbolName(tt.in); got != tt.want {
			t.Errorf("baseSymbolName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
