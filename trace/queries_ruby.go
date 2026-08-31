package trace

// rubyQueries extracts the top-level definition kinds in Ruby:
// module, class (including its name regardless of <superclass>),
// method (def …), and singleton_method (def self.…).
//
// Each query captures the symbol identifier with @name. The Kind on the
// NamedQuery propagates verbatim to Symbol.Kind.
var rubyQueries = []NamedQuery{
	{Kind: "module", Query: `(module name: (constant) @name)`},
	{Kind: "class", Query: `(class name: (constant) @name)`},
	{Kind: "method", Query: `(method name: (identifier) @name)`},
	{Kind: "singleton_method", Query: `(singleton_method name: (identifier) @name)`},
}
