package trace

// cQueries covers C definitions. Function name lives two nodes deep
// (function_definition → function_declarator → identifier), so the query
// matches the inner function_declarator anywhere — equivalent in this
// grammar because the declarator only appears under a function_definition.
//
// Typedef name extraction is intentionally omitted: typedefs in C wrap
// the alias inside a series of nested type specs whose shape varies with
// pointer / array / function-type structure. The regex extractor still
// catches these for now.
var cQueries = []NamedQuery{
	{Kind: "function", Query: `(function_definition declarator: (function_declarator declarator: (identifier) @name))`},
	{Kind: "struct", Query: `(struct_specifier name: (type_identifier) @name)`},
	{Kind: "enum", Query: `(enum_specifier name: (type_identifier) @name)`},
	{Kind: "union", Query: `(union_specifier name: (type_identifier) @name)`},
	{Kind: "macro", Query: `(preproc_def name: (identifier) @name)`},
	{Kind: "macro", Query: `(preproc_function_def name: (identifier) @name)`},
}
