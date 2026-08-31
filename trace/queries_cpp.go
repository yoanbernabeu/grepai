package trace

// cppQueries covers C++ definitions on top of the C set. Classes,
// namespaces, and templated functions get their own kinds. Method
// definitions nested inside class_specifier blocks are picked up by the
// same function_definition query because tree-sitter queries match at
// any depth.
var cppQueries = []NamedQuery{
	{Kind: "function", Query: `(function_definition declarator: (function_declarator declarator: (identifier) @name))`},
	{Kind: "function", Query: `(function_definition declarator: (function_declarator declarator: (field_identifier) @name))`},
	{Kind: "class", Query: `(class_specifier name: (type_identifier) @name)`},
	{Kind: "struct", Query: `(struct_specifier name: (type_identifier) @name)`},
	{Kind: "union", Query: `(union_specifier name: (type_identifier) @name)`},
	{Kind: "enum", Query: `(enum_specifier name: (type_identifier) @name)`},
	{Kind: "namespace", Query: `(namespace_definition name: (namespace_identifier) @name)`},
	{Kind: "macro", Query: `(preproc_def name: (identifier) @name)`},
}
