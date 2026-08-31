package trace

// rustQueries covers top-level definition kinds in Rust. Method-like
// function_items nested inside impl_item blocks are picked up by the same
// (function_item …) pattern because tree-sitter queries match anywhere in
// the tree. Same for trait methods inside trait_item blocks.
var rustQueries = []NamedQuery{
	{Kind: "function", Query: `(function_item name: (identifier) @name)`},
	{Kind: "struct", Query: `(struct_item name: (type_identifier) @name)`},
	{Kind: "enum", Query: `(enum_item name: (type_identifier) @name)`},
	{Kind: "trait", Query: `(trait_item name: (type_identifier) @name)`},
	{Kind: "type", Query: `(type_item name: (type_identifier) @name)`},
	{Kind: "constant", Query: `(const_item name: (identifier) @name)`},
	{Kind: "static", Query: `(static_item name: (identifier) @name)`},
}
