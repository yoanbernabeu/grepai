package trace

import "regexp"

// LanguagePatterns holds regex patterns for a specific language.
type LanguagePatterns struct {
	Extension    string
	Language     string
	Functions    []*regexp.Regexp
	Methods      []*regexp.Regexp
	Classes      []*regexp.Regexp
	Interfaces   []*regexp.Regexp
	Types        []*regexp.Regexp
	FunctionCall *regexp.Regexp
	MethodCall   *regexp.Regexp
}

// GetPatternsForLanguage returns patterns for a file extension.
func GetPatternsForLanguage(ext string) *LanguagePatterns {
	return languagePatterns[ext]
}

// SupportedExtensions returns all supported file extensions.
func SupportedExtensions() []string {
	exts := make([]string, 0, len(languagePatterns))
	for ext := range languagePatterns {
		exts = append(exts, ext)
	}
	return exts
}

var languagePatterns = map[string]*LanguagePatterns{
	".go":   goPatterns,
	".js":   jsPatterns,
	".ts":   tsPatterns,
	".jsx":  jsxPatterns,
	".tsx":  tsxPatterns,
	".py":   pythonPatterns,
	".php":  phpPatterns,
	".c":    cPatterns,
	".h":    cPatterns,
	".zig":  zigPatterns,
	".rs":   rustPatterns,
	".cpp":  cppPatterns,
	".hpp":  cppPatterns,
	".cc":   cppPatterns,
	".cxx":  cppPatterns,
	".hxx":  cppPatterns,
	".java": javaPatterns,
	".cs":   csharpPatterns,
}

// Go patterns
var goPatterns = &LanguagePatterns{
	Extension: ".go",
	Language:  "go",
	Functions: []*regexp.Regexp{
		// func FunctionName(params) returns
		regexp.MustCompile(`(?m)^func\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	},
	Methods: []*regexp.Regexp{
		// func (r *Receiver) MethodName(params) returns
		regexp.MustCompile(`(?m)^func\s+\(\w+\s+\*?([A-Za-z_][A-Za-z0-9_]*)\)\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	},
	Interfaces: []*regexp.Regexp{
		// type InterfaceName interface {
		regexp.MustCompile(`(?m)^type\s+([A-Z][A-Za-z0-9_]*)\s+interface\s*\{`),
	},
	Types: []*regexp.Regexp{
		// type TypeName struct {
		regexp.MustCompile(`(?m)^type\s+([A-Z][A-Za-z0-9_]*)\s+struct\s*\{`),
		// type TypeName other
		regexp.MustCompile(`(?m)^type\s+([A-Z][A-Za-z0-9_]*)\s+[^=\s{]+`),
	},
	FunctionCall: regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	MethodCall:   regexp.MustCompile(`\.([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
}

// JavaScript patterns
var jsPatterns = &LanguagePatterns{
	Extension: ".js",
	Language:  "javascript",
	Functions: []*regexp.Regexp{
		// function name(params)
		regexp.MustCompile(`(?m)(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`),
		// const name = function(params)
		regexp.MustCompile(`(?m)(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:async\s+)?function\s*\(`),
		// const name = (params) =>
		regexp.MustCompile(`(?m)(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:async\s+)?\([^)]*\)\s*=>`),
		// const name = async param =>
		regexp.MustCompile(`(?m)(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*async\s+[A-Za-z_$][A-Za-z0-9_$]*\s*=>`),
	},
	Methods: []*regexp.Regexp{
		// methodName(params) { inside class
		regexp.MustCompile(`(?m)^\s+(?:async\s+)?([A-Za-z_$][A-Za-z0-9_$]*)\s*\([^)]*\)\s*\{`),
		// static methodName(params)
		regexp.MustCompile(`(?m)^\s+static\s+(?:async\s+)?([A-Za-z_$][A-Za-z0-9_$]*)\s*\([^)]*\)\s*\{`),
	},
	Classes: []*regexp.Regexp{
		// class ClassName
		regexp.MustCompile(`(?m)(?:export\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)(?:\s+extends\s+[A-Za-z_$][A-Za-z0-9_$]*)?\s*\{`),
	},
	FunctionCall: regexp.MustCompile(`\b([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`),
	MethodCall:   regexp.MustCompile(`\.([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`),
}

// TypeScript patterns (extends JS patterns)
var tsPatterns = &LanguagePatterns{
	Extension: ".ts",
	Language:  "typescript",
	Functions: append(jsPatterns.Functions,
		// function name<T>(params): ReturnType
		regexp.MustCompile(`(?m)(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*<[^>]*>\s*\(`),
	),
	Methods: jsPatterns.Methods,
	Classes: jsPatterns.Classes,
	Interfaces: []*regexp.Regexp{
		// interface InterfaceName
		regexp.MustCompile(`(?m)(?:export\s+)?interface\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>]*>)?\s*(?:extends\s+[^{]+)?\{`),
	},
	Types: []*regexp.Regexp{
		// type TypeName = ...
		regexp.MustCompile(`(?m)(?:export\s+)?type\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>]*>)?\s*=`),
	},
	FunctionCall: jsPatterns.FunctionCall,
	MethodCall:   jsPatterns.MethodCall,
}

// JSX patterns (same as JS)
var jsxPatterns = &LanguagePatterns{
	Extension:    ".jsx",
	Language:     "javascript",
	Functions:    jsPatterns.Functions,
	Methods:      jsPatterns.Methods,
	Classes:      jsPatterns.Classes,
	FunctionCall: jsPatterns.FunctionCall,
	MethodCall:   jsPatterns.MethodCall,
}

// TSX patterns (same as TS)
var tsxPatterns = &LanguagePatterns{
	Extension:    ".tsx",
	Language:     "typescript",
	Functions:    tsPatterns.Functions,
	Methods:      tsPatterns.Methods,
	Classes:      tsPatterns.Classes,
	Interfaces:   tsPatterns.Interfaces,
	Types:        tsPatterns.Types,
	FunctionCall: tsPatterns.FunctionCall,
	MethodCall:   tsPatterns.MethodCall,
}

// Python patterns
var pythonPatterns = &LanguagePatterns{
	Extension: ".py",
	Language:  "python",
	Functions: []*regexp.Regexp{
		// def function_name(params):
		regexp.MustCompile(`(?m)^def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
		// async def function_name(params):
		regexp.MustCompile(`(?m)^async\s+def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	},
	Methods: []*regexp.Regexp{
		// def method_name(self, params): inside class (indented)
		regexp.MustCompile(`(?m)^[ \t]+def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(self`),
		// async def method_name(self, params):
		regexp.MustCompile(`(?m)^[ \t]+async\s+def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(self`),
	},
	Classes: []*regexp.Regexp{
		// class ClassName(Base):
		regexp.MustCompile(`(?m)^class\s+([A-Za-z_][A-Za-z0-9_]*)(?:\s*\([^)]*\))?\s*:`),
	},
	FunctionCall: regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	MethodCall:   regexp.MustCompile(`\.([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
}

// PHP patterns
var phpPatterns = &LanguagePatterns{
	Extension: ".php",
	Language:  "php",
	Functions: []*regexp.Regexp{
		// function name(params)
		regexp.MustCompile(`(?m)^function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	},
	Methods: []*regexp.Regexp{
		// public/protected/private function methodName(params)
		regexp.MustCompile(`(?m)^\s+(?:public|protected|private)\s+(?:static\s+)?function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	},
	Classes: []*regexp.Regexp{
		// class ClassName
		regexp.MustCompile(`(?m)^(?:abstract\s+)?class\s+([A-Za-z_][A-Za-z0-9_]*)(?:\s+extends\s+[A-Za-z_][A-Za-z0-9_]*)?(?:\s+implements\s+[^{]+)?\s*\{`),
	},
	Interfaces: []*regexp.Regexp{
		// interface InterfaceName
		regexp.MustCompile(`(?m)^interface\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`),
	},
	FunctionCall: regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	MethodCall:   regexp.MustCompile(`(?:->|::)([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
}

// Language keywords to filter out from function calls.
var languageKeywords = map[string]map[string]bool{
	"go": {
		"if": true, "for": true, "range": true, "switch": true, "select": true,
		"go": true, "defer": true, "return": true, "make": true, "new": true,
		"append": true, "len": true, "cap": true, "panic": true, "recover": true,
		"close": true, "delete": true, "copy": true, "print": true, "println": true,
		"complex": true, "real": true, "imag": true,
	},
	"javascript": {
		"if": true, "for": true, "while": true, "switch": true, "return": true,
		"new": true, "typeof": true, "instanceof": true, "await": true, "yield": true,
		"throw": true, "try": true, "catch": true, "finally": true, "delete": true,
		"void": true, "import": true, "export": true, "require": true,
	},
	"typescript": {
		"if": true, "for": true, "while": true, "switch": true, "return": true,
		"new": true, "typeof": true, "instanceof": true, "await": true, "yield": true,
		"throw": true, "try": true, "catch": true, "finally": true, "delete": true,
		"void": true, "import": true, "export": true, "require": true, "keyof": true,
	},
	"python": {
		"if": true, "for": true, "while": true, "with": true, "return": true,
		"yield": true, "raise": true, "assert": true, "print": true, "len": true,
		"range": true, "enumerate": true, "zip": true, "map": true, "filter": true,
		"list": true, "dict": true, "set": true, "tuple": true, "str": true,
		"int": true, "float": true, "bool": true, "type": true, "isinstance": true,
		"hasattr": true, "getattr": true, "setattr": true, "delattr": true,
		"open": true, "input": true, "super": true,
	},
	"php": {
		"if": true, "for": true, "foreach": true, "while": true, "switch": true,
		"return": true, "new": true, "echo": true, "print": true, "isset": true,
		"empty": true, "array": true, "unset": true, "include": true, "require": true,
		"include_once": true, "require_once": true, "die": true, "exit": true,
	},
	"c": {
		"if": true, "for": true, "while": true, "switch": true, "return": true,
		"sizeof": true, "typeof": true, "goto": true, "break": true, "continue": true,
		"malloc": true, "calloc": true, "realloc": true, "free": true, "printf": true,
		"fprintf": true, "sprintf": true, "scanf": true, "memcpy": true, "memset": true,
		"strlen": true, "strcmp": true, "strcpy": true, "strcat": true,
	},
	"cpp": {
		"if": true, "for": true, "while": true, "switch": true, "return": true,
		"new": true, "delete": true, "sizeof": true, "typeof": true, "typeid": true,
		"throw": true, "try": true, "catch": true, "static_cast": true, "dynamic_cast": true,
		"const_cast": true, "reinterpret_cast": true, "decltype": true, "noexcept": true,
	},
	"zig": {
		"if": true, "for": true, "while": true, "switch": true, "return": true,
		"break": true, "continue": true, "unreachable": true, "defer": true, "errdefer": true,
		"try": true, "catch": true, "orelse": true, "comptime": true, "inline": true,
		"assert": true, "expect": true, "expectEqual": true, "expectError": true,
	},
	"rust": {
		"if": true, "for": true, "while": true, "loop": true, "match": true, "return": true,
		"break": true, "continue": true, "panic": true, "assert": true, "assert_eq": true,
		"vec": true, "Box": true, "Rc": true, "Arc": true, "Some": true, "None": true,
		"Ok": true, "Err": true, "println": true, "print": true, "format": true,
	},
	"java": {
		// Control flow
		"if": true, "else": true, "for": true, "while": true, "do": true,
		"switch": true, "case": true, "default": true, "break": true, "continue": true,
		"return": true, "throw": true, "try": true, "catch": true, "finally": true,
		// Object creation/checking
		"new": true, "instanceof": true, "this": true, "super": true,
		// Assertions and synchronization
		"assert": true, "synchronized": true,
		// Common built-in methods (to filter out noise)
		"println": true, "print": true, "printf": true,
		"valueOf": true, "toString": true, "equals": true, "hashCode": true,
		"length": true, "size": true, "get": true, "set": true, "add": true, "remove": true,
		"isEmpty": true, "contains": true, "containsKey": true, "containsValue": true,
		"put": true, "clear": true, "toArray": true,
	},
	"csharp": {
		"if": true, "else": true, "for": true, "foreach": true, "while": true,
		"do": true, "switch": true, "case": true, "default": true, "break": true,
		"continue": true, "return": true, "throw": true, "try": true, "catch": true,
		"finally": true, "new": true, "nameof": true, "typeof": true, "using": true,
		"get": true, "set": true, "init": true, "value": true, "await": true,
		"yield": true, "lock": true, "this": true, "base": true,
		"add": true, "remove": true, "toString": true, "equals": true, "getHashCode": true,
	},
}

// C patterns
var cPatterns = &LanguagePatterns{
	Extension: ".c",
	Language:  "c",
	Functions: []*regexp.Regexp{
		// return_type function_name(params) - standard C function
		regexp.MustCompile(`(?m)^(?:static\s+)?(?:inline\s+)?(?:const\s+)?(?:unsigned\s+)?(?:signed\s+)?(?:struct\s+)?(?:enum\s+)?[A-Za-z_][A-Za-z0-9_]*(?:\s*\*+)?\s+([A-Za-z_][A-Za-z0-9_]*)\s*\([^;]*\)\s*\{`),
		// void/int/char etc. function_name(params)
		regexp.MustCompile(`(?m)^(?:void|int|char|short|long|float|double|size_t|ssize_t|bool|_Bool)\s+([A-Za-z_][A-Za-z0-9_]*)\s*\([^;]*\)\s*\{`),
	},
	Types: []*regexp.Regexp{
		// typedef struct { ... } TypeName;
		regexp.MustCompile(`(?m)^typedef\s+(?:struct|union|enum)\s*\{[^}]*\}\s*([A-Za-z_][A-Za-z0-9_]*)\s*;`),
		// typedef existing_type new_type;
		regexp.MustCompile(`(?m)^typedef\s+[A-Za-z_][A-Za-z0-9_\s\*]+\s+([A-Za-z_][A-Za-z0-9_]*)\s*;`),
		// struct StructName {
		regexp.MustCompile(`(?m)^struct\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`),
		// enum EnumName {
		regexp.MustCompile(`(?m)^enum\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`),
	},
	FunctionCall: regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	MethodCall:   regexp.MustCompile(`(?:->|\.)\s*([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
}

// Zig patterns
var zigPatterns = &LanguagePatterns{
	Extension: ".zig",
	Language:  "zig",
	Functions: []*regexp.Regexp{
		// Top-level: pub fn function_name(params) ReturnType {
		regexp.MustCompile(`(?m)^(?:pub\s+)?fn\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
		// inline fn / export fn / extern fn
		regexp.MustCompile(`(?m)^(?:pub\s+)?(?:inline\s+|export\s+|extern\s+)fn\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	},
	Methods: []*regexp.Regexp{
		// Methods inside structs/enums (indented): pub fn method_name(self, ...) or fn method_name(...)
		regexp.MustCompile(`(?m)^[ \t]+(?:pub\s+)?(?:inline\s+)?fn\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	},
	Types: []*regexp.Regexp{
		// const TypeName = struct { (any case for type name)
		regexp.MustCompile(`(?m)^(?:pub\s+)?const\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:packed\s+|extern\s+)?struct\s*(?:\([^)]*\))?\s*\{`),
		// const TypeName = union { (any case for type name)
		regexp.MustCompile(`(?m)^(?:pub\s+)?const\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:packed\s+|extern\s+)?union\s*(?:\([^)]*\))?\s*\{`),
		// const TypeName = enum { (any case for type name)
		regexp.MustCompile(`(?m)^(?:pub\s+)?const\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*enum\s*(?:\([^)]*\))?\s*\{`),
		// const ErrorName = error {
		regexp.MustCompile(`(?m)^(?:pub\s+)?const\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*error\s*\{`),
		// const TypeName = opaque {};
		regexp.MustCompile(`(?m)^(?:pub\s+)?const\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*opaque\s*\{`),
		// Nested types inside structs (indented): pub const NestedType = struct/enum/union {
		regexp.MustCompile(`(?m)^[ \t]+(?:pub\s+)?const\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:packed\s+|extern\s+)?(?:struct|enum|union)\s*(?:\([^)]*\))?\s*\{`),
	},
	FunctionCall: regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	MethodCall:   regexp.MustCompile(`\.([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
}

// Rust patterns
var rustPatterns = &LanguagePatterns{
	Extension: ".rs",
	Language:  "rust",
	Functions: []*regexp.Regexp{
		// fn function_name(params) -> ReturnType { (top-level)
		regexp.MustCompile(`(?m)^(?:pub\s+)?(?:async\s+)?(?:unsafe\s+)?(?:const\s+)?(?:extern\s+"[^"]*"\s+)?fn\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?:<[^>]*>)?\s*\(`),
	},
	Methods: []*regexp.Regexp{
		// Methods inside impl blocks (indented): pub fn / pub const fn / pub async fn
		regexp.MustCompile(`(?m)^[ \t]+(?:pub\s+)?(?:async\s+)?(?:unsafe\s+)?(?:const\s+)?fn\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?:<[^>]*>)?\s*\(`),
	},
	Types: []*regexp.Regexp{
		// struct StructName { or struct StructName; or struct StructName(...)
		regexp.MustCompile(`(?m)^(?:pub\s+)?struct\s+([A-Za-z_][A-Za-z0-9_]*)(?:<[^>]*>)?\s*(?:where\s+[^{;(]+)?(?:\{|;|\()`),
		// enum EnumName {
		regexp.MustCompile(`(?m)^(?:pub\s+)?enum\s+([A-Za-z_][A-Za-z0-9_]*)(?:<[^>]*>)?\s*(?:where\s+[^{]+)?\{`),
		// type TypeName = ... (type alias)
		regexp.MustCompile(`(?m)^(?:pub\s+)?type\s+([A-Za-z_][A-Za-z0-9_]*)(?:<[^>]*>)?\s*=`),
	},
	Interfaces: []*regexp.Regexp{
		// trait TraitName { (with optional bounds)
		regexp.MustCompile(`(?m)^(?:pub\s+)?(?:unsafe\s+)?trait\s+([A-Za-z_][A-Za-z0-9_]*)(?:<[^>]*>)?\s*(?::\s*[^{]+)?\s*\{`),
	},
	FunctionCall: regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*(?:!?\s*)?\(`),
	MethodCall:   regexp.MustCompile(`\.([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
}

// C++ patterns
var cppPatterns = &LanguagePatterns{
	Extension: ".cpp",
	Language:  "cpp",
	Functions: []*regexp.Regexp{
		// return_type function_name(params) - standard C++ function
		regexp.MustCompile(`(?m)^(?:static\s+)?(?:inline\s+)?(?:virtual\s+)?(?:const\s+)?(?:constexpr\s+)?(?:unsigned\s+)?(?:signed\s+)?[A-Za-z_][A-Za-z0-9_:<>]*(?:\s*[&*]+)?\s+([A-Za-z_][A-Za-z0-9_]*)\s*\([^;]*\)\s*(?:const\s*)?(?:noexcept\s*)?(?:override\s*)?(?:final\s*)?\{`),
		// void/int/char etc. function_name(params)
		regexp.MustCompile(`(?m)^(?:void|int|char|short|long|float|double|bool|auto)\s+([A-Za-z_][A-Za-z0-9_]*)\s*\([^;]*\)\s*\{`),
		// template<typename T> return_type function_name (top-level)
		regexp.MustCompile(`(?m)^template\s*<[^>]*>\s*(?:inline\s+)?[A-Za-z_][A-Za-z0-9_:<>]*(?:\s*[&*]+)?\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
		// Functions inside namespaces (indented)
		regexp.MustCompile(`(?m)^[ \t]+(?:static\s+)?(?:inline\s+)?(?:void|int|char|short|long|float|double|bool|auto|[A-Za-z_][A-Za-z0-9_:<>]*)\s+([A-Za-z_][A-Za-z0-9_]*)\s*\([^;)]*\)\s*\{`),
	},
	Methods: []*regexp.Regexp{
		// ClassName::MethodName(params) - out of class definition
		regexp.MustCompile(`(?m)^[A-Za-z_][A-Za-z0-9_:<>]*(?:\s*[&*]+)?\s+[A-Za-z_][A-Za-z0-9_]*::([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
		// Methods inside class body (indented): return_type method_name(params) { or = default/delete
		regexp.MustCompile(`(?m)^[ \t]+(?:virtual\s+)?(?:static\s+)?(?:inline\s+)?(?:constexpr\s+)?(?:explicit\s+)?(?:void|int|char|bool|auto|size_t|[A-Za-z_][A-Za-z0-9_:<>]*(?:\s*[&*]+)?)\s+([A-Za-z_][A-Za-z0-9_]*)\s*\([^;)]*\)\s*(?:const\s*)?(?:noexcept\s*)?(?:override\s*)?(?:final\s*)?(?:\{|=)`),
		// Constructor/Destructor: ClassName(params) or ~ClassName()
		regexp.MustCompile(`(?m)^[ \t]+(?:explicit\s+)?(?:virtual\s+)?(~?[A-Z][A-Za-z0-9_]*)\s*\([^;)]*\)\s*(?:noexcept\s*)?(?::\s*[^{]+)?\{`),
	},
	Classes: []*regexp.Regexp{
		// class ClassName { (with optional inheritance)
		regexp.MustCompile(`(?m)^(?:template\s*<[^>]*>\s*)?class\s+([A-Za-z_][A-Za-z0-9_]*)(?:\s*:\s*(?:public|protected|private)\s+[A-Za-z_][A-Za-z0-9_:<>,\s]*)?\s*\{`),
		// struct StructName { (with optional inheritance)
		regexp.MustCompile(`(?m)^(?:template\s*<[^>]*>\s*)?struct\s+([A-Za-z_][A-Za-z0-9_]*)(?:\s*:\s*(?:public|protected|private)\s+[A-Za-z_][A-Za-z0-9_:<>,\s]*)?\s*\{`),
	},
	Types: []*regexp.Regexp{
		// using TypeName = ...
		regexp.MustCompile(`(?m)^using\s+([A-Za-z_][A-Za-z0-9_]*)\s*=`),
		// typedef ... TypeName;
		regexp.MustCompile(`(?m)^typedef\s+[A-Za-z_][A-Za-z0-9_\s\*<>:,]+\s+([A-Za-z_][A-Za-z0-9_]*)\s*;`),
		// enum class EnumName { or enum EnumName {
		regexp.MustCompile(`(?m)^enum\s+(?:class\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*(?::\s*[A-Za-z_][A-Za-z0-9_]*)?\s*\{`),
	},
	FunctionCall: regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	MethodCall:   regexp.MustCompile(`(?:->|\.|\:\:)([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
}

// Java patterns
var javaPatterns = &LanguagePatterns{
	Extension: ".java",
	Language:  "java",
	// Note: Java has no standalone functions - all are methods within classes
	Functions: []*regexp.Regexp{},
	Methods: []*regexp.Regexp{
		// Standard method: [modifiers] returnType methodName(params) [throws] {
		// Handles: public, protected, private, static, final, abstract, synchronized, native, strictfp
		// Handles generic return types like List<String>, Map<K,V>
		regexp.MustCompile(`(?m)^\s+(?:(?:public|protected|private)\s+)?(?:static\s+)?(?:final\s+)?(?:abstract\s+)?(?:synchronized\s+)?(?:native\s+)?(?:strictfp\s+)?(?:<[^>]+>\s+)?[A-Za-z_][A-Za-z0-9_<>,\[\]\s]*\s+([a-z][A-Za-z0-9_]*)\s*\([^)]*\)\s*(?:throws\s+[A-Za-z_][A-Za-z0-9_,\s]*)?\s*\{`),
		// Constructor: [modifiers] ClassName(params) [throws] {
		regexp.MustCompile(`(?m)^\s+(?:(?:public|protected|private)\s+)?([A-Z][A-Za-z0-9_]*)\s*\([^)]*\)\s*(?:throws\s+[A-Za-z_][A-Za-z0-9_,\s]*)?\s*\{`),
		// Abstract method (no body): [modifiers] returnType methodName(params);
		regexp.MustCompile(`(?m)^\s+(?:(?:public|protected|private)\s+)?(?:static\s+)?(?:abstract\s+)?(?:<[^>]+>\s+)?[A-Za-z_][A-Za-z0-9_<>,\[\]\s]*\s+([a-z][A-Za-z0-9_]*)\s*\([^)]*\)\s*;`),
		// Interface default method: default returnType methodName(params) {
		regexp.MustCompile(`(?m)^\s+default\s+(?:<[^>]+>\s+)?[A-Za-z_][A-Za-z0-9_<>,\[\]\s]*\s+([a-z][A-Za-z0-9_]*)\s*\([^)]*\)\s*\{`),
	},
	Classes: []*regexp.Regexp{
		// class ClassName [extends ...] [implements ...] {
		// Handles: public, abstract, final, strictfp, sealed, non-sealed modifiers
		// Handles generics: class Container<T> or class Pair<K, V>
		regexp.MustCompile(`(?m)^(?:public\s+)?(?:abstract\s+)?(?:final\s+)?(?:sealed\s+)?(?:non-sealed\s+)?(?:strictfp\s+)?class\s+([A-Z][A-Za-z0-9_]*)(?:<[^>]*>)?(?:\s+extends\s+[A-Za-z_][A-Za-z0-9_<>,\s]*)?(?:\s+implements\s+[A-Za-z_][A-Za-z0-9_<>,\s]*)?(?:\s+permits\s+[A-Za-z_][A-Za-z0-9_,\s]*)?\s*\{`),
		// Inner/nested class (indented)
		regexp.MustCompile(`(?m)^\s+(?:(?:public|protected|private)\s+)?(?:static\s+)?(?:abstract\s+)?(?:final\s+)?(?:sealed\s+)?(?:non-sealed\s+)?class\s+([A-Z][A-Za-z0-9_]*)(?:<[^>]*>)?(?:\s+extends\s+[A-Za-z_][A-Za-z0-9_<>,\s]*)?(?:\s+implements\s+[A-Za-z_][A-Za-z0-9_<>,\s]*)?\s*\{`),
		// Enum: [public] enum EnumName [implements ...] {
		regexp.MustCompile(`(?m)^(?:public\s+)?enum\s+([A-Z][A-Za-z0-9_]*)(?:\s+implements\s+[A-Za-z_][A-Za-z0-9_<>,\s]*)?\s*\{`),
		// Record (Java 14+): [public] record RecordName(params) [implements ...] {
		regexp.MustCompile(`(?m)^(?:public\s+)?record\s+([A-Z][A-Za-z0-9_]*)(?:<[^>]*>)?\s*\([^)]*\)(?:\s+implements\s+[A-Za-z_][A-Za-z0-9_<>,\s]*)?\s*\{`),
	},
	Interfaces: []*regexp.Regexp{
		// interface InterfaceName [extends ...] {
		// Handles generics: interface Comparable<T>
		// Handles sealed interfaces (Java 17+)
		regexp.MustCompile(`(?m)^(?:public\s+)?(?:sealed\s+)?interface\s+([A-Z][A-Za-z0-9_]*)(?:<[^>]*>)?(?:\s+extends\s+[A-Za-z_][A-Za-z0-9_<>,\s]*)?(?:\s+permits\s+[A-Za-z_][A-Za-z0-9_,\s]*)?\s*\{`),
		// @interface (annotation type): [public] @interface AnnotationName {
		regexp.MustCompile(`(?m)^(?:public\s+)?@interface\s+([A-Z][A-Za-z0-9_]*)\s*\{`),
	},
	Types: []*regexp.Regexp{
		// Inner enum (indented)
		regexp.MustCompile(`(?m)^\s+(?:(?:public|protected|private)\s+)?(?:static\s+)?enum\s+([A-Z][A-Za-z0-9_]*)\s*\{`),
	},
	FunctionCall: regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	MethodCall:   regexp.MustCompile(`\.([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
}

// C# patterns
var csharpPatterns = &LanguagePatterns{
	Extension: ".cs",
	Language:  "csharp",
	Functions: []*regexp.Regexp{},
	Methods: []*regexp.Regexp{
		// Method with return type and modifiers
		regexp.MustCompile(`(?m)^\s*(?:(?:public|private|protected|internal|static|virtual|override|abstract|sealed|async|extern|new|unsafe|partial)\s+)*[A-Za-z_][A-Za-z0-9_<>,\[\]\s\?]*\s+([A-Za-z_][A-Za-z0-9_]*)\s*\([^;{)]*\)\s*(?:where\s+[^\r\n{;]+)?\s*(?:\{|;|=>)`),
		// Constructor (no return type)
		regexp.MustCompile(`(?m)^\s*(?:(?:public|private|protected|internal|static|extern|unsafe|partial)\s+)*([A-Z][A-Za-z0-9_]*)\s*\([^;{)]*\)\s*(?:\{|;|=>)`),
	},
	Classes: []*regexp.Regexp{
		// class ClassName ...
		regexp.MustCompile(`(?m)^(?:\s*(?:public|private|protected|internal)?\s*(?:abstract|sealed|static|partial)?\s*)class\s+([A-Z][A-Za-z0-9_]*)(?:<[^>]*>)?(?:\s*:\s*[^\{]+)?\s*\{`),
		// struct StructName ...
		regexp.MustCompile(`(?m)^(?:\s*(?:public|private|protected|internal)?\s*(?:readonly|ref|partial)?\s*)struct\s+([A-Z][A-Za-z0-9_]*)(?:<[^>]*>)?(?:\s*:\s*[^\{]+)?\s*\{`),
		// record RecordName ... or record struct RecordName ...
		regexp.MustCompile(`(?m)^(?:\s*(?:public|private|protected|internal)?\s*(?:sealed|abstract|partial)?\s*)record\s+(?:class\s+|struct\s+)?([A-Z][A-Za-z0-9_]*)(?:<[^>]*>)?(?:\s*\([^\)]*\))?(?:\s*:\s*[^\{]+)?\s*\{`),
	},
	Interfaces: []*regexp.Regexp{
		// interface InterfaceName ...
		regexp.MustCompile(`(?m)^(?:\s*(?:public|private|protected|internal)?\s*(?:partial)?\s*)interface\s+([A-Z][A-Za-z0-9_]*)(?:<[^>]*>)?(?:\s*:\s*[^\{]+)?\s*\{`),
	},
	FunctionCall: regexp.MustCompile(`\b(?:new\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*(?:<[^>]*>)?\s*\(`),
	MethodCall:   regexp.MustCompile(`(?:\.|\?\.|::)\s*([A-Za-z_][A-Za-z0-9_]*)\s*(?:<[^>]*>)?\s*\(`),
}

// IsKeyword checks if a name is a language keyword.
func IsKeyword(name string, lang string) bool {
	if kw, ok := languageKeywords[lang]; ok {
		return kw[name]
	}
	return false
}
