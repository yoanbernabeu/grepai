//go:build treesitter

package trace

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/php"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
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

	if nodeType == "call_expression" {
		funcNode := node.ChildByFieldName("function")
		if funcNode != nil {
			name := funcNode.Content(content)
			// Get just the function name (remove receiver if present)
			if idx := strings.LastIndex(name, "."); idx >= 0 {
				name = name[idx+1:]
			}

			caller := e.findContainingFunction(node, content)

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

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		e.walkNodeForCalls(child, content, filePath, ext, refs)
	}
}

func (e *TreeSitterExtractor) findContainingFunction(node *sitter.Node, content []byte) string {
	parent := node.Parent()
	for parent != nil {
		switch parent.Type() {
		case "function_declaration", "method_declaration", "function_definition":
			nameNode := parent.ChildByFieldName("name")
			if nameNode != nil {
				return nameNode.Content(content)
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
