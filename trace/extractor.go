package trace

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// RegexExtractor implements SymbolExtractor using regex patterns.
type RegexExtractor struct {
	patterns map[string]*LanguagePatterns
}

// NewRegexExtractor creates a new regex-based symbol extractor.
func NewRegexExtractor() *RegexExtractor {
	return &RegexExtractor{
		patterns: languagePatterns,
	}
}

// Mode returns the extraction mode.
func (e *RegexExtractor) Mode() string {
	return "fast"
}

// SupportedLanguages returns list of supported file extensions.
func (e *RegexExtractor) SupportedLanguages() []string {
	langs := make([]string, 0, len(e.patterns))
	for ext := range e.patterns {
		langs = append(langs, ext)
	}
	return langs
}

// ExtractSymbols extracts all symbol definitions from a file.
func (e *RegexExtractor) ExtractSymbols(ctx context.Context, filePath string, content string) ([]Symbol, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	patterns := e.patterns[ext]
	if patterns == nil {
		return nil, nil
	}

	var symbols []Symbol

	// Extract functions
	for _, re := range patterns.Functions {
		symbols = append(symbols, e.extractMatches(re, content, filePath, patterns.Language, KindFunction)...)
	}

	// Extract methods
	for _, re := range patterns.Methods {
		methodSymbols := e.extractMethodMatches(re, content, filePath, patterns.Language)
		symbols = append(symbols, methodSymbols...)
	}

	// Extract classes
	for _, re := range patterns.Classes {
		symbols = append(symbols, e.extractMatches(re, content, filePath, patterns.Language, KindClass)...)
	}

	// Extract interfaces
	for _, re := range patterns.Interfaces {
		symbols = append(symbols, e.extractMatches(re, content, filePath, patterns.Language, KindInterface)...)
	}

	// Extract types
	for _, re := range patterns.Types {
		symbols = append(symbols, e.extractMatches(re, content, filePath, patterns.Language, KindType)...)
	}

	return symbols, nil
}

// extractMatches extracts symbols from regex matches.
func (e *RegexExtractor) extractMatches(re *regexp.Regexp, content string, filePath string, lang string, kind SymbolKind) []Symbol {
	var symbols []Symbol
	matches := re.FindAllStringSubmatchIndex(content, -1)

	for _, match := range matches {
		if len(match) >= 4 {
			name := content[match[2]:match[3]]
			line := countLines(content[:match[0]]) + 1
			sig := extractSignature(content, match[0], match[1])

			symbols = append(symbols, Symbol{
				Name:      name,
				Kind:      kind,
				File:      filePath,
				Line:      line,
				Signature: sig,
				Exported:  isExported(name, lang),
				Language:  lang,
			})
		}
	}
	return symbols
}

// extractMethodMatches extracts method symbols including receiver info for Go.
func (e *RegexExtractor) extractMethodMatches(re *regexp.Regexp, content string, filePath string, lang string) []Symbol {
	var symbols []Symbol
	matches := re.FindAllStringSubmatchIndex(content, -1)

	for _, match := range matches {
		var name, receiver string

		switch lang {
		case "go":
			// Go method: groups are receiver_type, method_name
			if len(match) >= 6 {
				receiver = content[match[2]:match[3]]
				name = content[match[4]:match[5]]
			}
		default:
			// Other languages: first group is method name
			if len(match) >= 4 {
				name = content[match[2]:match[3]]
			}
		}

		if name != "" {
			line := countLines(content[:match[0]]) + 1
			sig := extractSignature(content, match[0], match[1])

			symbols = append(symbols, Symbol{
				Name:      name,
				Kind:      KindMethod,
				File:      filePath,
				Line:      line,
				Signature: sig,
				Receiver:  receiver,
				Exported:  isExported(name, lang),
				Language:  lang,
			})
		}
	}
	return symbols
}

// ExtractReferences extracts all symbol references from a file.
func (e *RegexExtractor) ExtractReferences(ctx context.Context, filePath string, content string) ([]Reference, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	patterns := e.patterns[ext]
	if patterns == nil {
		return nil, nil
	}

	var refs []Reference
	lines := strings.Split(content, "\n")

	// Build function boundaries for caller detection
	functionBoundaries := e.buildFunctionBoundaries(content, patterns)

	// Extract function calls
	if patterns.FunctionCall != nil {
		matches := patterns.FunctionCall.FindAllStringSubmatchIndex(content, -1)
		for _, match := range matches {
			if len(match) >= 4 {
				name := content[match[2]:match[3]]

				// Skip keywords
				if IsKeyword(name, patterns.Language) {
					continue
				}

				pos := match[0]
				line := countLines(content[:pos]) + 1
				caller := findContainingFunction(pos, functionBoundaries)

				refs = append(refs, Reference{
					SymbolName: name,
					File:       filePath,
					Line:       line,
					Context:    getLineContext(lines, line-1, 0),
					CallerName: caller.Name,
					CallerFile: filePath,
					CallerLine: caller.Line,
				})
			}
		}
	}

	// Extract method calls
	if patterns.MethodCall != nil {
		matches := patterns.MethodCall.FindAllStringSubmatchIndex(content, -1)
		for _, match := range matches {
			if len(match) >= 4 {
				name := content[match[2]:match[3]]
				pos := match[0]
				line := countLines(content[:pos]) + 1
				caller := findContainingFunction(pos, functionBoundaries)

				refs = append(refs, Reference{
					SymbolName: name,
					File:       filePath,
					Line:       line,
					Context:    getLineContext(lines, line-1, 0),
					CallerName: caller.Name,
					CallerFile: filePath,
					CallerLine: caller.Line,
				})
			}
		}
	}

	return refs, nil
}

// ExtractAll extracts both symbols and references in one pass.
func (e *RegexExtractor) ExtractAll(ctx context.Context, filePath string, content string) ([]Symbol, []Reference, error) {
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

// functionBoundary tracks function positions for caller detection.
type functionBoundary struct {
	Name     string
	StartPos int
	EndPos   int
	Line     int
}

// buildFunctionBoundaries finds all function positions in the content.
func (e *RegexExtractor) buildFunctionBoundaries(content string, patterns *LanguagePatterns) []functionBoundary {
	var boundaries []functionBoundary

	for _, re := range patterns.Functions {
		matches := re.FindAllStringSubmatchIndex(content, -1)
		for _, match := range matches {
			if len(match) >= 4 {
				name := content[match[2]:match[3]]
				line := countLines(content[:match[0]]) + 1
				endPos := findFunctionEnd(content, match[1], patterns.Language)

				boundaries = append(boundaries, functionBoundary{
					Name:     name,
					StartPos: match[0],
					EndPos:   endPos,
					Line:     line,
				})
			}
		}
	}

	for _, re := range patterns.Methods {
		matches := re.FindAllStringSubmatchIndex(content, -1)
		for _, match := range matches {
			var name string
			switch patterns.Language {
			case "go":
				if len(match) >= 6 {
					name = content[match[4]:match[5]]
				}
			default:
				if len(match) >= 4 {
					name = content[match[2]:match[3]]
				}
			}

			if name != "" {
				line := countLines(content[:match[0]]) + 1
				endPos := findFunctionEnd(content, match[1], patterns.Language)

				boundaries = append(boundaries, functionBoundary{
					Name:     name,
					StartPos: match[0],
					EndPos:   endPos,
					Line:     line,
				})
			}
		}
	}

	return boundaries
}

// findFunctionEnd finds the end position of a function body.
func findFunctionEnd(content string, start int, lang string) int {
	switch lang {
	case "go", "javascript", "typescript", "php":
		// Count braces to find function end
		braceCount := 0
		inString := false
		stringChar := byte(0)

		for i := start; i < len(content); i++ {
			c := content[i]

			// Handle string literals
			if !inString && (c == '"' || c == '\'' || c == '`') {
				inString = true
				stringChar = c
				continue
			}
			if inString {
				if c == stringChar && (i == 0 || content[i-1] != '\\') {
					inString = false
				}
				continue
			}

			if c == '{' {
				braceCount++
			} else if c == '}' {
				braceCount--
				if braceCount == 0 {
					return i + 1
				}
			}
		}
	case "python":
		// Python uses indentation - find next line with same or less indentation
		startLine := countLines(content[:start])
		lines := strings.Split(content, "\n")
		if startLine >= len(lines) {
			return len(content)
		}

		baseIndent := getIndentation(lines[startLine])
		for i := startLine + 1; i < len(lines); i++ {
			line := lines[i]
			if strings.TrimSpace(line) == "" {
				continue
			}
			if getIndentation(line) <= baseIndent && strings.TrimSpace(line) != "" {
				// Calculate position
				pos := 0
				for j := 0; j < i; j++ {
					pos += len(lines[j]) + 1
				}
				return pos
			}
		}
	}

	return len(content)
}

// getIndentation returns the number of leading whitespace characters.
func getIndentation(line string) int {
	count := 0
	for _, c := range line {
		if c == ' ' {
			count++
		} else if c == '\t' {
			count += 4
		} else {
			break
		}
	}
	return count
}

// findContainingFunction finds which function contains the given position.
func findContainingFunction(pos int, boundaries []functionBoundary) functionBoundary {
	var best functionBoundary
	for _, b := range boundaries {
		if b.StartPos <= pos && pos < b.EndPos {
			if b.StartPos > best.StartPos {
				best = b
			}
		}
	}
	if best.Name == "" {
		best.Name = "<top-level>"
	}
	return best
}

// Helper functions

// countLines counts the number of newlines in a string.
func countLines(s string) int {
	return strings.Count(s, "\n")
}

// isExported determines if a symbol is exported based on language conventions.
func isExported(name string, lang string) bool {
	if len(name) == 0 {
		return false
	}
	switch lang {
	case "go":
		return unicode.IsUpper(rune(name[0]))
	case "python":
		return !strings.HasPrefix(name, "_")
	default:
		return true
	}
}

// extractSignature extracts the function/method signature from content.
func extractSignature(content string, start, end int) string {
	// Extend to find the full signature (up to opening brace or colon)
	sig := content[start:end]

	// Find the end of the signature line
	lineEnd := strings.Index(content[start:], "\n")
	if lineEnd > 0 && lineEnd < 200 {
		sig = content[start : start+lineEnd]
	}

	// Clean up the signature
	sig = strings.TrimSpace(sig)
	if len(sig) > 150 {
		sig = sig[:150] + "..."
	}

	return sig
}

// getLineContext returns the line at the given index with optional context lines.
func getLineContext(lines []string, lineIdx int, contextLines int) string {
	if lineIdx < 0 || lineIdx >= len(lines) {
		return ""
	}

	start := lineIdx - contextLines
	if start < 0 {
		start = 0
	}
	end := lineIdx + contextLines + 1
	if end > len(lines) {
		end = len(lines)
	}

	result := strings.Join(lines[start:end], "\n")
	if len(result) > 200 {
		result = result[:200] + "..."
	}
	return strings.TrimSpace(result)
}
