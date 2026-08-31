package trace

import (
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/csharp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/php"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/yoanbernabeu/grepai/fsharp"
)

// treeSitterLanguages is the single source of truth for which file
// extensions are backed by a compiled-in tree-sitter grammar. The value is
// the constructor for that language's *sitter.Language — invoked once when
// TreeSitterExtractor is initialized.
//
// To add a new tree-sitter language:
//  1. Import its grammar package (smacker subpackage or a repo-root vendored
//     binding like fsharp/).
//  2. Add an entry below mapping its extension(s) to GetLanguage.
//  3. Add the symbol-extraction switch arm in
//     TreeSitterExtractor.walkNodeForSymbols.
//
// HasTreeSitterGrammar, TreeSitterExtensions, and NewTreeSitterExtractor
// all derive from this map, so there is no second list to keep in sync.
var treeSitterLanguages = map[string]func() *sitter.Language{
	".go":  golang.GetLanguage,
	".js":  javascript.GetLanguage,
	".jsx": javascript.GetLanguage,
	".ts":  typescript.GetLanguage,
	".tsx": typescript.GetLanguage,
	".py":  python.GetLanguage,
	".php": php.GetLanguage,
	".cs":  csharp.GetLanguage,
	".fs":  fsharp.GetLanguage,
	".fsx": fsharp.GetLanguage,
	".fsi": fsharp.GetLanguage,
}

// HasTreeSitterGrammar reports whether the given file extension is backed
// by a compiled-in tree-sitter grammar in this build. ext should include
// the leading dot (e.g., ".go"); case is normalized internally.
func HasTreeSitterGrammar(ext string) bool {
	_, ok := treeSitterLanguages[strings.ToLower(ext)]
	return ok
}

// TreeSitterExtensions returns a sorted snapshot of every extension that
// has a compiled-in tree-sitter grammar.
func TreeSitterExtensions() []string {
	out := make([]string, 0, len(treeSitterLanguages))
	for ext := range treeSitterLanguages {
		out = append(out, ext)
	}
	sort.Strings(out)
	return out
}
