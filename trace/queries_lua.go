package trace

// luaQueries handles the three Lua function-definition shapes the
// tree-sitter-lua grammar emits:
//
//	local function name() ...   -> function_statement with identifier child
//	function name() ...          -> function_statement with function_name child
//	function M.method() ...      -> function_statement with function_name (dotted)
//
// For the dotted form @name captures the whole qualified name (e.g.
// "M.method"); downstream tooling can split on the dot.
//
// Local declarations are also captured so module tables get found.
var luaQueries = []NamedQuery{
	{Kind: "function", Query: `(function_statement (identifier) @name)`},
	{Kind: "function", Query: `(function_statement (function_name) @name)`},
	{Kind: "variable", Query: `(variable_declaration (variable_declarator (identifier) @name))`},
}
