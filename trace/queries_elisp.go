package trace

// elispQueries covers the standard Emacs Lisp definition macros. The
// grammar (Wilfred/tree-sitter-elisp) gives `defun` its own
// function_definition node and `defmacro` its own macro_definition node;
// `defvar` lives under special_form with a leading `defvar` keyword
// node. Everything else (`defcustom`, `defface`, `define-minor-mode`,
// `define-derived-mode`, `cl-defmethod`, `cl-defun`, `cl-defgeneric`)
// parses as a generic `list` whose first child is a `symbol` naming the
// macro — we discriminate via #match? predicates and tag with
// human-readable Kinds.
//
// The match-vs-equal predicate choice matters: #match? takes a regex,
// #eq? takes a literal string. Using #match? with anchored alternations
// keeps the rule set short while still being precise.
var elispQueries = []NamedQuery{
	{Kind: "defun", Query: `(function_definition (symbol) @name)`},
	{Kind: "defmacro", Query: `(macro_definition (symbol) @name)`},
	// `defvar` is an anonymous token (named:false in node-types.json) so it
	// must be matched as a string literal, not as a named node type.
	{Kind: "defvar", Query: `(special_form "defvar" . (symbol) @name)`},

	// defconst is also a special_form (per grammar.json).
	{Kind: "defconst", Query: `(special_form "defconst" . (symbol) @name)`},
	// defcustom and defface aren't special_forms in this grammar — they
	// parse as generic lists. Discriminate via #eq?.
	{Kind: "defcustom", Query: `(list . (symbol) @kind . (symbol) @name (#eq? @kind "defcustom"))`},
	{Kind: "defface", Query: `(list . (symbol) @kind . (symbol) @name (#eq? @kind "defface"))`},
	// defalias is intentionally omitted: its second argument is typically
	// a quote ('name) rather than a bare symbol, so a clean query that
	// captures both quoted and unquoted forms needs grammar work beyond
	// the scope of this PR. Regex still picks it up.

	// define-*-mode family — all match the same pattern.
	{Kind: "define-mode", Query: `(list . (symbol) @kind . (symbol) @name (#match? @kind "^define-(minor|derived|generic|globalized-minor)-mode$"))`},

	// cl-* generic-function machinery.
	{Kind: "cl-defun", Query: `(list . (symbol) @kind . (symbol) @name (#eq? @kind "cl-defun"))`},
	{Kind: "cl-defmethod", Query: `(list . (symbol) @kind . (symbol) @name (#eq? @kind "cl-defmethod"))`},
	{Kind: "cl-defgeneric", Query: `(list . (symbol) @kind . (symbol) @name (#eq? @kind "cl-defgeneric"))`},
}
