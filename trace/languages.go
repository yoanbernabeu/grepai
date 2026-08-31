package trace

import (
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/bash"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/csharp"
	"github.com/smacker/go-tree-sitter/elm"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/hcl"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/kotlin"
	"github.com/smacker/go-tree-sitter/lua"
	"github.com/smacker/go-tree-sitter/php"
	"github.com/smacker/go-tree-sitter/protobuf"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/scala"
	"github.com/smacker/go-tree-sitter/sql"
	"github.com/smacker/go-tree-sitter/swift"
	"github.com/smacker/go-tree-sitter/toml"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/yoanbernabeu/grepai/elisp"
	"github.com/yoanbernabeu/grepai/fsharp"
)

// LangSpec describes one tree-sitter-backed language: its name, the file
// extensions it owns, the grammar constructor, and an optional set of
// S-expression queries used by the query-based extraction path.
//
// When Queries is non-empty, TreeSitterExtractor.ExtractSymbols runs the
// queries against the parsed tree and emits one Symbol per @name capture
// with Kind taken from the corresponding NamedQuery.Kind.
//
// When Queries is nil/empty, the extractor falls back to its hand-written
// walkNodeForSymbols switch — the legacy path that the original nine
// languages (Go, JS/JSX, TS/TSX, Python, PHP, C#, F#) use.
type LangSpec struct {
	Name        string
	Extensions  []string                // lowercase, leading dot
	GetLanguage func() *sitter.Language // tree-sitter grammar constructor
	Queries     []NamedQuery            // optional; nil ⇒ use walk-based path

	// CallNodes lists the node types this grammar uses for a call. A nil
	// slice means "no call-graph support": declarative languages (SQL,
	// Protobuf, HCL, Elm, TOML) have no calls, and in Emacs Lisp a call is
	// an ordinary `list`, indistinguishable from `if`/`let`/`when`, so
	// extracting them would be mostly false positives.
	CallNodes []string

	// CalleeFields lists, in priority order, the field names holding the
	// called expression on a CallNodes node. All children carrying the
	// first field that matches are concatenated, which is what Lua needs
	// (`t.helper(1)` is three `prefix` children). When no field matches,
	// the walker falls back to the node's first named child — Kotlin and
	// Swift `call_expression` nodes carry no fields at all.
	CalleeFields []string

	// FunctionNodes lists node types that can lexically contain a call —
	// the candidates findContainingFunction walks up to when attributing
	// a call to its caller. Empty ⇒ defaultFunctionNodes.
	FunctionNodes []string

	// FunctionNameFields lists, in priority order, the field names holding
	// a FunctionNodes node's name. Empty ⇒ {"name"}. C and C++ nest the
	// name under `declarator`, hence the resolver descending for the first
	// identifier rather than taking the field's text verbatim.
	FunctionNameFields []string
}

// defaultCallNodes / defaultFunctionNodes / defaultFunctionNameFields are
// what a LangSpec that declares nothing gets. They are the node types the
// original walk-based languages used before the fields above existed.
var (
	defaultCallNodes          = []string{"call_expression", "invocation_expression"}
	defaultCalleeFields       = []string{"function", "expression"}
	defaultFunctionNodes      = []string{"function_declaration", "method_declaration", "constructor_declaration", "function_definition", "local_function_statement"}
	defaultFunctionNameFields = []string{"name"}
)

// callNodesFor and friends resolve a spec's reference-extraction settings,
// falling back to the defaults. ext is already lowercase.
func callNodesFor(spec *LangSpec) []string {
	if spec == nil || len(spec.CallNodes) == 0 {
		return defaultCallNodes
	}
	return spec.CallNodes
}

func calleeFieldsFor(spec *LangSpec) []string {
	if spec == nil || len(spec.CalleeFields) == 0 {
		return defaultCalleeFields
	}
	return spec.CalleeFields
}

func functionNodesFor(spec *LangSpec) []string {
	if spec == nil || len(spec.FunctionNodes) == 0 {
		return defaultFunctionNodes
	}
	return spec.FunctionNodes
}

func functionNameFieldsFor(spec *LangSpec) []string {
	if spec == nil || len(spec.FunctionNameFields) == 0 {
		return defaultFunctionNameFields
	}
	return spec.FunctionNameFields
}

// SupportsCallGraph reports whether references can be extracted for ext.
// A registered grammar with no CallNodes (SQL, TOML, Emacs Lisp, ...) still
// yields symbols; it just has no call graph.
func SupportsCallGraph(ext string) bool {
	spec := langSpecByExt(strings.ToLower(ext))
	if spec == nil {
		return false
	}
	return len(spec.CallNodes) > 0
}

// NamedQuery binds a tree-sitter S-expression query string to a free-form
// Kind label. The query must include a (@name) capture; the kind is
// propagated verbatim to Symbol.Kind. Use whatever kind string makes sense
// for the language — "method", "struct", "trait", "defun", "module", etc.
//
// Multiple queries per language are encouraged: one per logical symbol
// kind keeps each query simple and the extracted Kind precise.
type NamedQuery struct {
	Kind  string
	Query string
}

// treeSitterLanguages is the single source of truth for tree-sitter-backed
// languages. Adding a language is one entry below: import its grammar
// package, append a LangSpec, optionally provide Queries. Symbol extraction
// is then automatic via the query path; no edits to walkNodeForSymbols
// required.
//
// The first block contains the legacy nine — they keep the existing
// hand-walked extractGoSymbol/etc. behavior. PR 2 adds the second block
// of languages, all on the query-based path.
var treeSitterLanguages = []LangSpec{
	// --- Legacy walk-based languages (extractor_ts.go has hand-written walks).
	{Name: "go", Extensions: []string{".go"}, GetLanguage: golang.GetLanguage,
		CallNodes: []string{"call_expression"}, CalleeFields: []string{"function"},
		FunctionNodes: []string{"function_declaration", "method_declaration"}},
	{Name: "javascript", Extensions: []string{".js", ".jsx", ".mjs", ".cjs"}, GetLanguage: javascript.GetLanguage,
		CallNodes: []string{"call_expression"}, CalleeFields: []string{"function"},
		FunctionNodes: []string{"function_declaration", "method_definition"}},
	{Name: "typescript", Extensions: []string{".ts", ".tsx", ".mts", ".cts"}, GetLanguage: typescript.GetLanguage,
		CallNodes: []string{"call_expression"}, CalleeFields: []string{"function"},
		FunctionNodes: []string{"function_declaration", "method_definition"}},
	{Name: "python", Extensions: []string{".py"}, GetLanguage: python.GetLanguage,
		CallNodes: []string{"call"}, CalleeFields: []string{"function"},
		FunctionNodes: []string{"function_definition"}},
	{Name: "php", Extensions: []string{".php"}, GetLanguage: php.GetLanguage,
		CallNodes:     []string{"function_call_expression", "member_call_expression", "scoped_call_expression"},
		CalleeFields:  []string{"function", "name"},
		FunctionNodes: []string{"function_definition", "method_declaration"}},
	{Name: "csharp", Extensions: []string{".cs"}, GetLanguage: csharp.GetLanguage,
		CallNodes: []string{"invocation_expression"}, CalleeFields: []string{"function"},
		FunctionNodes: []string{"method_declaration", "constructor_declaration", "local_function_statement"}},
	// F# keeps its bespoke walk (walkFSharpCalls and the .fs branch of
	// findContainingFunction); CallNodes is declared only so that
	// SupportsCallGraph reports the truth for it.
	{Name: "fsharp", Extensions: []string{".fs", ".fsx", ".fsi"}, GetLanguage: fsharp.GetLanguage,
		CallNodes: []string{"application_expression"}},

	// --- Query-based languages (PR 2 additions).
	{Name: "ruby", Extensions: []string{".rb"}, GetLanguage: ruby.GetLanguage, Queries: rubyQueries,
		// A bare `helper` with no receiver and no parentheses parses as
		// `identifier` (it is ambiguous with a local variable), so only
		// calls with parentheses or a receiver are visible.
		CallNodes: []string{"call"}, CalleeFields: []string{"method"},
		FunctionNodes: []string{"method", "singleton_method"}},
	{Name: "rust", Extensions: []string{".rs"}, GetLanguage: rust.GetLanguage, Queries: rustQueries,
		CallNodes: []string{"call_expression"}, CalleeFields: []string{"function"},
		FunctionNodes: []string{"function_item"}},
	{Name: "java", Extensions: []string{".java"}, GetLanguage: java.GetLanguage, Queries: javaQueries,
		CallNodes: []string{"method_invocation"}, CalleeFields: []string{"name"},
		FunctionNodes: []string{"method_declaration", "constructor_declaration"}},
	{Name: "scala", Extensions: []string{".scala", ".sc", ".mill"}, GetLanguage: scala.GetLanguage, Queries: scalaQueries,
		CallNodes: []string{"call_expression"}, CalleeFields: []string{"function"},
		FunctionNodes: []string{"function_definition"}},

	// Medium priority (PR 2).
	{Name: "c", Extensions: []string{".c", ".h"}, GetLanguage: c.GetLanguage, Queries: cQueries,
		CallNodes: []string{"call_expression"}, CalleeFields: []string{"function"},
		// The name sits under `declarator` (a function_declarator), not
		// under a `name` field.
		FunctionNodes: []string{"function_definition"}, FunctionNameFields: []string{"declarator"}},
	{Name: "cpp", Extensions: []string{".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx"}, GetLanguage: cpp.GetLanguage, Queries: cppQueries,
		CallNodes: []string{"call_expression"}, CalleeFields: []string{"function"},
		FunctionNodes: []string{"function_definition"}, FunctionNameFields: []string{"declarator"}},
	{Name: "bash", Extensions: []string{".sh", ".bash", ".zsh"}, GetLanguage: bash.GetLanguage, Queries: bashQueries,
		// Every command is a "call"; a shell cannot tell a function
		// invocation from /usr/bin/foo without resolving $PATH.
		CallNodes: []string{"command"}, CalleeFields: []string{"name"},
		FunctionNodes: []string{"function_definition"}},
	{Name: "lua", Extensions: []string{".lua"}, GetLanguage: lua.GetLanguage, Queries: luaQueries,
		// `t.helper(1)` is three sibling `prefix` children (t, ., helper),
		// which is why CalleeFields concatenates rather than taking one.
		CallNodes: []string{"function_call"}, CalleeFields: []string{"prefix"},
		FunctionNodes: []string{"function_statement"}},
	{Name: "kotlin", Extensions: []string{".kt", ".kts"}, GetLanguage: kotlin.GetLanguage, Queries: kotlinQueries,
		// Neither call_expression nor function_declaration carries any
		// field in this grammar, so both resolve via first-named-child.
		CallNodes:     []string{"call_expression"},
		FunctionNodes: []string{"function_declaration"}},
	{Name: "swift", Extensions: []string{".swift"}, GetLanguage: swift.GetLanguage, Queries: swiftQueries,
		CallNodes:     []string{"call_expression"},
		FunctionNodes: []string{"function_declaration"}},

	// Long-tail (PR 2). Minimal queries; grow organically.
	// The five declarative languages below and elisp extract symbols but no
	// references; see LangSpec.CallNodes for why.
	{Name: "sql", Extensions: []string{".sql"}, GetLanguage: sql.GetLanguage, Queries: sqlQueries},
	{Name: "protobuf", Extensions: []string{".proto"}, GetLanguage: protobuf.GetLanguage, Queries: protobufQueries},
	{Name: "hcl", Extensions: []string{".hcl", ".tf"}, GetLanguage: hcl.GetLanguage, Queries: hclQueries},
	{Name: "elm", Extensions: []string{".elm"}, GetLanguage: elm.GetLanguage, Queries: elmQueries},
	{Name: "toml", Extensions: []string{".toml"}, GetLanguage: toml.GetLanguage, Queries: tomlQueries},

	// Vendored grammar (PR 2). See elisp/README.md for provenance.
	{Name: "elisp", Extensions: []string{".el"}, GetLanguage: elisp.GetLanguage, Queries: elispQueries},
}

// langSpecByExt returns the LangSpec covering ext, or nil if no
// tree-sitter grammar is registered for it. ext should be lowercase
// (callers normalize before lookup).
func langSpecByExt(ext string) *LangSpec {
	for i := range treeSitterLanguages {
		for _, e := range treeSitterLanguages[i].Extensions {
			if e == ext {
				return &treeSitterLanguages[i]
			}
		}
	}
	return nil
}

// HasTreeSitterGrammar reports whether the given file extension is backed
// by a compiled-in tree-sitter grammar in this build. ext should include
// the leading dot (e.g., ".go"); case is normalized internally.
func HasTreeSitterGrammar(ext string) bool {
	return langSpecByExt(strings.ToLower(ext)) != nil
}

// TreeSitterExtensions returns a sorted snapshot of every extension that
// has a compiled-in tree-sitter grammar.
func TreeSitterExtensions() []string {
	var out []string
	for _, spec := range treeSitterLanguages {
		out = append(out, spec.Extensions...)
	}
	sort.Strings(out)
	return out
}
