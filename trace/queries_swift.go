package trace

// swiftQueries covers Swift's main definition kinds. Note the
// tree-sitter-swift grammar lumps class / struct / actor / extension /
// enum all under class_declaration with the keyword as a child — we tag
// them all with Kind "class" rather than try to discriminate cheaply.
// Protocols are first-class via protocol_declaration.
var swiftQueries = []NamedQuery{
	{Kind: "class", Query: `(class_declaration (type_identifier) @name)`},
	{Kind: "protocol", Query: `(protocol_declaration (type_identifier) @name)`},
	{Kind: "function", Query: `(function_declaration (simple_identifier) @name)`},
	{Kind: "function", Query: `(protocol_function_declaration (simple_identifier) @name)`},
}
