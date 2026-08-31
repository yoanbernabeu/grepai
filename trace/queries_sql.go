package trace

// sqlQueries captures the common CREATE-statement targets in SQL:
// tables, views, indexes, and stored functions. Object names live under
// `object_reference` for table/view/function and `identifier` for index
// (a tree-sitter-sql idiosyncrasy).
var sqlQueries = []NamedQuery{
	{Kind: "table", Query: `(create_table (object_reference) @name)`},
	{Kind: "view", Query: `(create_view (object_reference) @name)`},
	{Kind: "function", Query: `(create_function (object_reference) @name)`},
	{Kind: "index", Query: `(create_index (identifier) @name)`},
}
