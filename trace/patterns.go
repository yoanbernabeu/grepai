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
	".go":  goPatterns,
	".js":  jsPatterns,
	".ts":  tsPatterns,
	".jsx": jsxPatterns,
	".tsx": tsxPatterns,
	".py":  pythonPatterns,
	".php": phpPatterns,
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
	Methods:  jsPatterns.Methods,
	Classes:  jsPatterns.Classes,
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
}

// IsKeyword checks if a name is a language keyword.
func IsKeyword(name string, lang string) bool {
	if kw, ok := languageKeywords[lang]; ok {
		return kw[name]
	}
	return false
}
