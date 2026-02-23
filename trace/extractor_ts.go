//go:build treesitter

package trace

import (
	"context"
	"fmt"
	"path/filepath"
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

// TreeSitterExtractor implements SymbolExtractor using tree-sitter AST parsing.
type TreeSitterExtractor struct {
	parsers map[string]*sitter.Parser
}

// NewTreeSitterExtractor creates a new tree-sitter based extractor.
func NewTreeSitterExtractor() (*TreeSitterExtractor, error) {
	ext := &TreeSitterExtractor{
		parsers: make(map[string]*sitter.Parser),
	}

	languages := map[string]*sitter.Language{
		".go":  golang.GetLanguage(),
		".js":  javascript.GetLanguage(),
		".jsx": javascript.GetLanguage(),
		".ts":  typescript.GetLanguage(),
		".tsx": typescript.GetLanguage(),
		".py":  python.GetLanguage(),
		".php": php.GetLanguage(),
		".cs":  csharp.GetLanguage(),
		".fs":  fsharp.GetLanguage(),
		".fsx": fsharp.GetLanguage(),
		".fsi": fsharp.GetLanguage(),
	}

	for extension, lang := range languages {
		parser := sitter.NewParser()
		parser.SetLanguage(lang)
		ext.parsers[extension] = parser
	}

	return ext, nil
}

// Mode returns the extraction mode.
func (e *TreeSitterExtractor) Mode() string {
	return "precise"
}

// SupportedLanguages returns list of supported file extensions.
func (e *TreeSitterExtractor) SupportedLanguages() []string {
	langs := make([]string, 0, len(e.parsers))
	for ext := range e.parsers {
		langs = append(langs, ext)
	}
	return langs
}

// ExtractSymbols extracts all symbol definitions from a file using tree-sitter.
func (e *TreeSitterExtractor) ExtractSymbols(ctx context.Context, filePath string, content string) ([]Symbol, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	parser, ok := e.parsers[ext]
	if !ok {
		return nil, nil
	}

	tree, err := parser.ParseCtx(ctx, nil, []byte(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}
	defer tree.Close()

	var symbols []Symbol
	root := tree.RootNode()

	e.walkNodeForSymbols(root, []byte(content), filePath, ext, &symbols)

	return symbols, nil
}

func (e *TreeSitterExtractor) walkNodeForSymbols(node *sitter.Node, content []byte, filePath string, ext string, symbols *[]Symbol) {
	nodeType := node.Type()

	switch ext {
	case ".go":
		e.extractGoSymbol(node, nodeType, content, filePath, symbols)
	case ".js", ".jsx":
		e.extractJSSymbol(node, nodeType, content, filePath, "javascript", symbols)
	case ".ts", ".tsx":
		e.extractJSSymbol(node, nodeType, content, filePath, "typescript", symbols)
	case ".py":
		e.extractPythonSymbol(node, nodeType, content, filePath, symbols)
	case ".php":
		e.extractPHPSymbol(node, nodeType, content, filePath, symbols)
	case ".cs":
		e.extractCSharpSymbol(node, nodeType, content, filePath, symbols)
	case ".fs", ".fsx", ".fsi":
		e.extractFSharpSymbol(node, nodeType, content, filePath, symbols)
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		e.walkNodeForSymbols(child, content, filePath, ext, symbols)
	}
}

func (e *TreeSitterExtractor) extractGoSymbol(node *sitter.Node, nodeType string, content []byte, filePath string, symbols *[]Symbol) {
	switch nodeType {
	case "function_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			name := nameNode.Content(content)
			*symbols = append(*symbols, Symbol{
				Name:      name,
				Kind:      KindFunction,
				File:      filePath,
				Line:      int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
				Signature: truncateSignature(string(content[node.StartByte():node.EndByte()])),
				Exported:  isExported(name, "go"),
				Language:  "go",
			})
		}

	case "method_declaration":
		nameNode := node.ChildByFieldName("name")
		receiverNode := node.ChildByFieldName("receiver")
		if nameNode != nil {
			name := nameNode.Content(content)
			var receiver string
			if receiverNode != nil {
				for i := 0; i < int(receiverNode.ChildCount()); i++ {
					child := receiverNode.Child(i)
					if child.Type() == "type_identifier" || child.Type() == "pointer_type" {
						receiver = child.Content(content)
						break
					}
				}
			}
			*symbols = append(*symbols, Symbol{
				Name:     name,
				Kind:     KindMethod,
				File:     filePath,
				Line:     int(node.StartPoint().Row) + 1,
				EndLine:  int(node.EndPoint().Row) + 1,
				Receiver: receiver,
				Exported: isExported(name, "go"),
				Language: "go",
			})
		}

	case "type_declaration":
		for i := 0; i < int(node.ChildCount()); i++ {
			spec := node.Child(i)
			if spec.Type() == "type_spec" {
				nameNode := spec.ChildByFieldName("name")
				typeNode := spec.ChildByFieldName("type")
				if nameNode != nil {
					name := nameNode.Content(content)
					kind := KindType
					if typeNode != nil {
						switch typeNode.Type() {
						case "interface_type":
							kind = KindInterface
						case "struct_type":
							kind = KindClass
						}
					}
					*symbols = append(*symbols, Symbol{
						Name:     name,
						Kind:     kind,
						File:     filePath,
						Line:     int(spec.StartPoint().Row) + 1,
						Exported: isExported(name, "go"),
						Language: "go",
					})
				}
			}
		}
	}
}

func (e *TreeSitterExtractor) extractJSSymbol(node *sitter.Node, nodeType string, content []byte, filePath string, lang string, symbols *[]Symbol) {
	switch nodeType {
	case "function_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			name := nameNode.Content(content)
			*symbols = append(*symbols, Symbol{
				Name:     name,
				Kind:     KindFunction,
				File:     filePath,
				Line:     int(node.StartPoint().Row) + 1,
				EndLine:  int(node.EndPoint().Row) + 1,
				Language: lang,
			})
		}

	case "class_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			name := nameNode.Content(content)
			*symbols = append(*symbols, Symbol{
				Name:     name,
				Kind:     KindClass,
				File:     filePath,
				Line:     int(node.StartPoint().Row) + 1,
				EndLine:  int(node.EndPoint().Row) + 1,
				Language: lang,
			})
		}

	case "interface_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			name := nameNode.Content(content)
			*symbols = append(*symbols, Symbol{
				Name:     name,
				Kind:     KindInterface,
				File:     filePath,
				Line:     int(node.StartPoint().Row) + 1,
				Language: lang,
			})
		}

	case "lexical_declaration", "variable_declaration":
		for i := 0; i < int(node.ChildCount()); i++ {
			decl := node.Child(i)
			if decl.Type() == "variable_declarator" {
				nameNode := decl.ChildByFieldName("name")
				valueNode := decl.ChildByFieldName("value")
				if nameNode != nil && valueNode != nil {
					valueType := valueNode.Type()
					if valueType == "arrow_function" || valueType == "function" {
						name := nameNode.Content(content)
						*symbols = append(*symbols, Symbol{
							Name:     name,
							Kind:     KindFunction,
							File:     filePath,
							Line:     int(node.StartPoint().Row) + 1,
							Language: lang,
						})
					}
				}
			}
		}
	}
}

func (e *TreeSitterExtractor) extractPythonSymbol(node *sitter.Node, nodeType string, content []byte, filePath string, symbols *[]Symbol) {
	switch nodeType {
	case "function_definition":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			name := nameNode.Content(content)
			kind := KindFunction
			// Check if it's a method (inside a class)
			parent := node.Parent()
			if parent != nil && parent.Type() == "block" {
				grandparent := parent.Parent()
				if grandparent != nil && grandparent.Type() == "class_definition" {
					kind = KindMethod
				}
			}
			*symbols = append(*symbols, Symbol{
				Name:     name,
				Kind:     kind,
				File:     filePath,
				Line:     int(node.StartPoint().Row) + 1,
				EndLine:  int(node.EndPoint().Row) + 1,
				Exported: !strings.HasPrefix(name, "_"),
				Language: "python",
			})
		}

	case "class_definition":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			name := nameNode.Content(content)
			*symbols = append(*symbols, Symbol{
				Name:     name,
				Kind:     KindClass,
				File:     filePath,
				Line:     int(node.StartPoint().Row) + 1,
				EndLine:  int(node.EndPoint().Row) + 1,
				Exported: !strings.HasPrefix(name, "_"),
				Language: "python",
			})
		}
	}
}

func (e *TreeSitterExtractor) extractPHPSymbol(node *sitter.Node, nodeType string, content []byte, filePath string, symbols *[]Symbol) {
	switch nodeType {
	case "function_definition":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			name := nameNode.Content(content)
			*symbols = append(*symbols, Symbol{
				Name:     name,
				Kind:     KindFunction,
				File:     filePath,
				Line:     int(node.StartPoint().Row) + 1,
				EndLine:  int(node.EndPoint().Row) + 1,
				Language: "php",
			})
		}

	case "method_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			name := nameNode.Content(content)
			*symbols = append(*symbols, Symbol{
				Name:     name,
				Kind:     KindMethod,
				File:     filePath,
				Line:     int(node.StartPoint().Row) + 1,
				EndLine:  int(node.EndPoint().Row) + 1,
				Language: "php",
			})
		}

	case "class_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			name := nameNode.Content(content)
			*symbols = append(*symbols, Symbol{
				Name:     name,
				Kind:     KindClass,
				File:     filePath,
				Line:     int(node.StartPoint().Row) + 1,
				EndLine:  int(node.EndPoint().Row) + 1,
				Language: "php",
			})
		}

	case "interface_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			name := nameNode.Content(content)
			*symbols = append(*symbols, Symbol{
				Name:     name,
				Kind:     KindInterface,
				File:     filePath,
				Line:     int(node.StartPoint().Row) + 1,
				Language: "php",
			})
		}
	}
}

func (e *TreeSitterExtractor) extractCSharpSymbol(node *sitter.Node, nodeType string, content []byte, filePath string, symbols *[]Symbol) {
	switch nodeType {
	case "class_declaration", "struct_declaration", "record_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			name := nameNode.Content(content)
			*symbols = append(*symbols, Symbol{
				Name:     name,
				Kind:     KindClass,
				File:     filePath,
				Line:     int(node.StartPoint().Row) + 1,
				EndLine:  int(node.EndPoint().Row) + 1,
				Language: "csharp",
			})
		}

	case "interface_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			name := nameNode.Content(content)
			*symbols = append(*symbols, Symbol{
				Name:     name,
				Kind:     KindInterface,
				File:     filePath,
				Line:     int(node.StartPoint().Row) + 1,
				EndLine:  int(node.EndPoint().Row) + 1,
				Language: "csharp",
			})
		}

	case "method_declaration", "constructor_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			name := nameNode.Content(content)
			*symbols = append(*symbols, Symbol{
				Name:     name,
				Kind:     KindMethod,
				File:     filePath,
				Line:     int(node.StartPoint().Row) + 1,
				EndLine:  int(node.EndPoint().Row) + 1,
				Language: "csharp",
			})
		}
	}
}

func (e *TreeSitterExtractor) extractFSharpSymbol(node *sitter.Node, nodeType string, content []byte, filePath string, symbols *[]Symbol) {
	switch nodeType {
	case "value_declaration":
		// Module-level let bindings: let add x y = ...
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "function_or_value_defn" {
				for j := 0; j < int(child.ChildCount()); j++ {
					gc := child.Child(j)
					if gc.Type() == "function_declaration_left" {
						nameNode := findChildByType(gc, "identifier")
						if nameNode != nil {
							name := nameNode.Content(content)
							*symbols = append(*symbols, Symbol{
								Name:     name,
								Kind:     KindFunction,
								File:     filePath,
								Line:     int(node.StartPoint().Row) + 1,
								EndLine:  int(node.EndPoint().Row) + 1,
								Language: "fsharp",
							})
						}
						return
					}
				}
			}
		}

	case "type_definition":
		// Find the inner type defn node
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			childType := child.Type()

			switch childType {
			case "union_type_defn", "record_type_defn", "enum_type_defn", "type_abbrev_defn":
				nameNode := findChildByType(child, "type_name")
				if nameNode != nil {
					idNode := findChildByType(nameNode, "identifier")
					if idNode != nil {
						*symbols = append(*symbols, Symbol{
							Name:     idNode.Content(content),
							Kind:     KindType,
							File:     filePath,
							Line:     int(node.StartPoint().Row) + 1,
							EndLine:  int(node.EndPoint().Row) + 1,
							Language: "fsharp",
						})
					}
				}

			case "anon_type_defn":
				nameNode := findChildByType(child, "type_name")
				if nameNode != nil {
					idNode := findChildByType(nameNode, "identifier")
					if idNode != nil {
						name := idNode.Content(content)
						kind := KindClass
						// Check if it's an interface (all members are abstract with no constructor args)
						if hasOnlyAbstractMembers(child) && findChildByType(child, "primary_constr_args") == nil {
							kind = KindInterface
						}
						*symbols = append(*symbols, Symbol{
							Name:     name,
							Kind:     kind,
							File:     filePath,
							Line:     int(node.StartPoint().Row) + 1,
							EndLine:  int(node.EndPoint().Row) + 1,
							Language: "fsharp",
						})
					}
				}
			}
		}

	case "member_defn":
		// Instance/static/override/abstract members
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			switch child.Type() {
			case "method_or_prop_defn":
				propIdent := findChildByType(child, "property_or_ident")
				if propIdent != nil {
					// Last identifier is the method name (first might be "this")
					var lastName string
					for j := 0; j < int(propIdent.ChildCount()); j++ {
						gc := propIdent.Child(j)
						if gc.Type() == "identifier" {
							lastName = gc.Content(content)
						}
					}
					if lastName != "" {
						*symbols = append(*symbols, Symbol{
							Name:     lastName,
							Kind:     KindMethod,
							File:     filePath,
							Line:     int(node.StartPoint().Row) + 1,
							EndLine:  int(node.EndPoint().Row) + 1,
							Language: "fsharp",
						})
					}
				}
				return
			case "member_signature":
				// abstract member Log: string -> unit
				idNode := findChildByType(child, "identifier")
				if idNode != nil {
					*symbols = append(*symbols, Symbol{
						Name:     idNode.Content(content),
						Kind:     KindMethod,
						File:     filePath,
						Line:     int(node.StartPoint().Row) + 1,
						EndLine:  int(node.EndPoint().Row) + 1,
						Language: "fsharp",
					})
				}
				return
			}
		}

	case "module_defn":
		idNode := findChildByType(node, "identifier")
		if idNode != nil {
			*symbols = append(*symbols, Symbol{
				Name:     idNode.Content(content),
				Kind:     KindClass,
				File:     filePath,
				Line:     int(node.StartPoint().Row) + 1,
				EndLine:  int(node.EndPoint().Row) + 1,
				Language: "fsharp",
			})
		}
	}
}

func findChildByType(node *sitter.Node, typeName string) *sitter.Node {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == typeName {
			return child
		}
	}
	return nil
}

func hasOnlyAbstractMembers(node *sitter.Node) bool {
	hasMembers := false
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_extension_elements" {
			for j := 0; j < int(child.ChildCount()); j++ {
				gc := child.Child(j)
				if gc.Type() == "member_defn" {
					hasMembers = true
					hasAbstract := false
					for k := 0; k < int(gc.ChildCount()); k++ {
						if gc.Child(k).Type() == "abstract" {
							hasAbstract = true
							break
						}
					}
					if !hasAbstract {
						return false
					}
				}
			}
		}
	}
	return hasMembers
}

// ExtractReferences extracts all symbol references from a file.
func (e *TreeSitterExtractor) ExtractReferences(ctx context.Context, filePath string, content string) ([]Reference, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	parser, ok := e.parsers[ext]
	if !ok {
		return nil, nil
	}

	tree, err := parser.ParseCtx(ctx, nil, []byte(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}
	defer tree.Close()

	var refs []Reference
	root := tree.RootNode()

	e.walkNodeForCalls(root, []byte(content), filePath, ext, &refs)

	return refs, nil
}

func (e *TreeSitterExtractor) walkNodeForCalls(node *sitter.Node, content []byte, filePath string, ext string, refs *[]Reference) {
	nodeType := node.Type()

	switch ext {
	case ".fs", ".fsx", ".fsi":
		e.walkFSharpCalls(node, nodeType, content, filePath, refs)
	default:
		if nodeType == "call_expression" || nodeType == "invocation_expression" {
			funcNode := node.ChildByFieldName("function")
			if funcNode == nil {
				funcNode = node.ChildByFieldName("expression")
			}
			if funcNode != nil {
				name := funcNode.Content(content)
				if idx := strings.LastIndex(name, "."); idx >= 0 {
					name = name[idx+1:]
				}

				caller := e.findContainingFunction(node, content, ext)

				*refs = append(*refs, Reference{
					SymbolName: name,
					File:       filePath,
					Line:       int(node.StartPoint().Row) + 1,
					Column:     int(node.StartPoint().Column),
					Context:    truncateContext(string(content[node.StartByte():node.EndByte()])),
					CallerName: caller,
					CallerFile: filePath,
				})
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		e.walkNodeForCalls(child, content, filePath, ext, refs)
	}
}

func (e *TreeSitterExtractor) walkFSharpCalls(node *sitter.Node, nodeType string, content []byte, filePath string, refs *[]Reference) {
	if nodeType != "application_expression" {
		return
	}

	// In F#, application_expression's first child is the function being called.
	// For `helper input` → first child is long_identifier_or_op "helper"
	// For `String.Format(...)` → first child is long_identifier_or_op "String.Format"
	// For nested curried calls like `sprintf "%s" name`, the outer application_expression
	// has an inner application_expression as first child. We only want the innermost function name.

	firstChild := node.Child(0)
	if firstChild == nil {
		return
	}

	// Skip if first child is itself an application_expression (curried call — outer layer)
	if firstChild.Type() == "application_expression" {
		return
	}

	name := extractFSharpCallName(firstChild, content)
	if name == "" {
		return
	}

	// Filter keywords
	if isFSharpKeyword(name) {
		return
	}

	caller := e.findContainingFunction(node, content, ".fs")

	*refs = append(*refs, Reference{
		SymbolName: name,
		File:       filePath,
		Line:       int(node.StartPoint().Row) + 1,
		Column:     int(node.StartPoint().Column),
		Context:    truncateContext(string(content[node.StartByte():node.EndByte()])),
		CallerName: caller,
		CallerFile: filePath,
	})
}

func extractFSharpCallName(node *sitter.Node, content []byte) string {
	text := node.Content(content)
	// For dotted access like "String.Format" or "Logger.log", take the last part
	if idx := strings.LastIndex(text, "."); idx >= 0 {
		return text[idx+1:]
	}
	return text
}

func isFSharpKeyword(name string) bool {
	switch name {
	case "let", "in", "if", "then", "else", "elif", "match", "with", "for", "while", "do",
		"try", "finally", "raise", "yield", "return", "fun", "function", "not", "true",
		"false", "null", "new", "module", "namespace", "open", "type", "and", "or", "when",
		"as", "of", "mutable", "rec", "inline", "private", "public", "internal",
		"begin", "end", "upcast", "downcast", "lazy", "assert", "base", "this",
		"async", "task", "use", "failwith", "failwithf", "sprintf", "printfn", "printf":
		return true
	}
	return false
}

func (e *TreeSitterExtractor) findContainingFunction(node *sitter.Node, content []byte, ext string) string {
	parent := node.Parent()
	for parent != nil {
		switch ext {
		case ".fs", ".fsx", ".fsi":
			switch parent.Type() {
			case "function_or_value_defn":
				// Look for function_declaration_left child which has the function name
				for i := 0; i < int(parent.ChildCount()); i++ {
					child := parent.Child(i)
					if child.Type() == "function_declaration_left" {
						nameNode := findChildByType(child, "identifier")
						if nameNode != nil {
							return nameNode.Content(content)
						}
					}
				}
			case "method_or_prop_defn":
				propIdent := findChildByType(parent, "property_or_ident")
				if propIdent != nil {
					var lastName string
					for i := 0; i < int(propIdent.ChildCount()); i++ {
						gc := propIdent.Child(i)
						if gc.Type() == "identifier" {
							lastName = gc.Content(content)
						}
					}
					if lastName != "" {
						return lastName
					}
				}
			}
		default:
			switch parent.Type() {
			case "function_declaration", "method_declaration", "constructor_declaration", "function_definition", "local_function_statement":
				nameNode := parent.ChildByFieldName("name")
				if nameNode != nil {
					return nameNode.Content(content)
				}
			}
		}
		parent = parent.Parent()
	}
	return "<top-level>"
}

// ExtractAll extracts both symbols and references in one pass.
func (e *TreeSitterExtractor) ExtractAll(ctx context.Context, filePath string, content string) ([]Symbol, []Reference, error) {
	symbols, err := e.ExtractSymbols(ctx, filePath, content)
	if err != nil {
		return nil, nil, err
	}
	refs, err := e.ExtractReferences(ctx, filePath, content)
	if err != nil {
		return nil, nil, err
	}
	return symbols, refs, nil
}

func truncateSignature(s string) string {
	if idx := strings.Index(s, "{"); idx > 0 {
		s = strings.TrimSpace(s[:idx])
	}
	if len(s) > 150 {
		s = s[:150] + "..."
	}
	return s
}

func truncateContext(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 100 {
		s = s[:100] + "..."
	}
	return s
}
