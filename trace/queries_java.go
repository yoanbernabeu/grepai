package trace

// javaQueries covers the standard Java definition kinds: classes,
// interfaces, enums, records (Java 16+), methods, and constructors.
// Field-level extraction is deferred to a follow-up — the field name
// lives two nodes deep (field_declaration → variable_declarator →
// identifier) and the query for it is noisier than is worth shipping
// today.
var javaQueries = []NamedQuery{
	{Kind: "class", Query: `(class_declaration name: (identifier) @name)`},
	{Kind: "interface", Query: `(interface_declaration name: (identifier) @name)`},
	{Kind: "enum", Query: `(enum_declaration name: (identifier) @name)`},
	{Kind: "record", Query: `(record_declaration name: (identifier) @name)`},
	{Kind: "method", Query: `(method_declaration name: (identifier) @name)`},
	{Kind: "constructor", Query: `(constructor_declaration name: (identifier) @name)`},
}
