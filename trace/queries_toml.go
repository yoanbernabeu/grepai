package trace

// tomlQueries captures both table headers ([section]) and top-level
// key/value pairs. Nested keys inside tables are intentionally not
// surfaced — the table header is the right navigation target. Array-of-
// tables ([[arr]]) are handled by the same table query in this grammar.
var tomlQueries = []NamedQuery{
	{Kind: "table", Query: `(table (bare_key) @name)`},
	{Kind: "table", Query: `(table (dotted_key) @name)`},
	{Kind: "key", Query: `(pair (bare_key) @name)`},
}
