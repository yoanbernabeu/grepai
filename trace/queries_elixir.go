package trace

// elixirQueries discriminates the Elixir definition macros by matching
// against the leading identifier of a `call` node. The grammar parses
// every macro invocation (def/defp/defmodule/...) as `call` with shape:
//
//	call
//	  identifier   ← the macro name ("def", "defmodule", …)
//	  arguments    ← positional args (the defined name lives here)
//	  do_block?    ← optional body
//
// The defined name is buried in `arguments` and its node type varies:
//
//	defmodule Greeter do ...  → arguments > alias
//	def hello(name), do: ...  → arguments > call > identifier  (parens form)
//	def hello, do: ...        → arguments > identifier         (paren-less form)
//
// We use anchored captures (`.`) plus #match? predicates to map each
// shape to a precise Kind.
var elixirQueries = []NamedQuery{
	// Module-shaped definitions whose argument is an alias.
	{Kind: "module", Query: `
		(call
		  (identifier) @kind
		  (arguments (alias) @name)
		  (#match? @kind "^def(module|protocol|impl|struct|exception|record)$"))`},

	// Function/macro definitions written with parentheses:
	//   def foo(a, b), do: ...    or    def foo(a, b) do ... end
	{Kind: "function", Query: `
		(call
		  (identifier) @kind
		  (arguments . (call (identifier) @name))
		  (#match? @kind "^def(p?|macrop?)$"))`},

	// Paren-less form:  def foo, do: 1
	{Kind: "function", Query: `
		(call
		  (identifier) @kind
		  (arguments . (identifier) @name)
		  (#match? @kind "^def(p?|macrop?)$"))`},
}
