package trace

// kotlinQueries covers the standard Kotlin definition kinds. The grammar
// distinguishes between class_declaration (for class / interface / data
// class / enum class) and object_declaration (for singletons). Companion
// objects are not exposed as object_declarations by this grammar — they
// surface as companion_object children of class_declaration, which we
// leave for a follow-up.
var kotlinQueries = []NamedQuery{
	{Kind: "class", Query: `(class_declaration (type_identifier) @name)`},
	{Kind: "object", Query: `(object_declaration (type_identifier) @name)`},
	{Kind: "function", Query: `(function_declaration (simple_identifier) @name)`},
	// Properties / fields. Kotlin nests the name inside a
	// variable_declaration that lives directly under
	// property_declaration. Captures `val`, `var`, and `const val`
	// properties — both class-level and top-level.
	{Kind: "property", Query: `(property_declaration (variable_declaration (simple_identifier) @name))`},
}
