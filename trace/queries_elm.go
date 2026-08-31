package trace

// elmQueries captures Elm's three top-level definition kinds: type
// aliases, sum types (union types), and value declarations (functions
// + constants). The grammar uses upper_case_identifier for type names
// and lower_case_identifier for value names, which makes the queries
// unambiguous.
var elmQueries = []NamedQuery{
	{Kind: "type_alias", Query: `(type_alias_declaration (upper_case_identifier) @name)`},
	{Kind: "type", Query: `(type_declaration (upper_case_identifier) @name)`},
	{Kind: "function", Query: `(value_declaration (function_declaration_left (lower_case_identifier) @name))`},
}
