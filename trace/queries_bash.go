package trace

// bashQueries captures shell function definitions in both their forms:
//
//	greet() { … }
//	function greet { … }
//
// plus top-level variable assignments (often used as named constants).
var bashQueries = []NamedQuery{
	{Kind: "function", Query: `(function_definition name: (word) @name)`},
	{Kind: "variable", Query: `(variable_assignment name: (variable_name) @name)`},
}
