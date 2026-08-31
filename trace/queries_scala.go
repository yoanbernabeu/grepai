package trace

// scalaQueries covers the standard Scala 2 & 3 definition kinds: objects
// (singletons), classes (including case classes), traits, enums (Scala 3),
// def-style functions (concrete and abstract), and top-level vals.
//
// Note Scala 3 case classes still parse as class_definition with a leading
// `case` keyword, so the same query catches both.
var scalaQueries = []NamedQuery{
	{Kind: "object", Query: `(object_definition name: (identifier) @name)`},
	{Kind: "class", Query: `(class_definition name: (identifier) @name)`},
	{Kind: "trait", Query: `(trait_definition name: (identifier) @name)`},
	{Kind: "enum", Query: `(enum_definition name: (identifier) @name)`},
	{Kind: "function", Query: `(function_definition name: (identifier) @name)`},
	{Kind: "function", Query: `(function_declaration name: (identifier) @name)`},
	{Kind: "val", Query: `(val_definition pattern: (identifier) @name)`},
}
