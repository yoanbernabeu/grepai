package trace

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
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
	ignored := buildIgnoredMask(content, patterns.Language)
	scanContent := getReferenceScanContent(content, patterns, ignored)

	// Build function boundaries for caller detection
	functionBoundaries := e.buildFunctionBoundaries(content, patterns)
	appendRef := func(name string, pos int) {
		refs = append(refs, buildReference(filePath, content, lines, name, pos, functionBoundaries))
	}

	// Extract function calls
	if patterns.FunctionCall != nil {
		matches := patterns.FunctionCall.FindAllStringSubmatchIndex(scanContent, -1)
		for _, match := range matches {
			if len(match) >= 4 {
				if isDeclarationReferenceMatch(content, patterns, match[0]) {
					continue
				}
				if ignored[match[2]] {
					continue
				}
				name := content[match[2]:match[3]]

				// Skip keywords
				if IsKeyword(name, patterns.Language) {
					continue
				}

				if isDeclarationCallArtifact(content, match[0], name, patterns.Language) {
					continue
				}
				appendRef(name, match[0])
			}
		}
	}

	// Extract method calls
	if patterns.MethodCall != nil {
		matches := patterns.MethodCall.FindAllStringSubmatchIndex(scanContent, -1)
		for _, match := range matches {
			if len(match) >= 4 {
				if isDeclarationReferenceMatch(content, patterns, match[0]) {
					continue
				}
				if ignored[match[2]] {
					continue
				}
				name := content[match[2]:match[3]]
				appendRef(name, match[0])
			}
		}
	}

	refs = append(refs, e.extractLanguageSpecificReferences(filePath, content, lines, patterns, functionBoundaries)...)

	return dedupeReferences(refs), nil
}

// getReferenceScanContent returns the content view used for regex reference matching.
func getReferenceScanContent(content string, patterns *LanguagePatterns, ignored []bool) string {
	if patterns == nil {
		return content
	}
	if patterns.Language == "lua" {
		return maskedContent(content, ignored)
	}
	return content
}

// extractLanguageSpecificReferences runs supplemental reference extraction for specific languages.
func (e *RegexExtractor) extractLanguageSpecificReferences(filePath string, content string, lines []string, patterns *LanguagePatterns, functionBoundaries []functionBoundary) []Reference {
	if patterns == nil {
		return nil
	}

	switch patterns.Language {
	case "javascript", "typescript":
		return e.extractJSPropertyReferences(filePath, content, lines, functionBoundaries)
	case "lua":
		return e.extractLuaBracketKeyReferences(filePath, content, lines, patterns, functionBoundaries)
	default:
		return nil
	}
}

var (
	jsPropertyReadRe  = regexp.MustCompile(`\b(?:this\.)?(?:[A-Za-z_$][A-Za-z0-9_$]*\.)+([A-Za-z_$][A-Za-z0-9_$]*)\b`)
	jsPropertyWriteRe = regexp.MustCompile(`\b(?:this\.)?(?:[A-Za-z_$][A-Za-z0-9_$]*\.)+([A-Za-z_$][A-Za-z0-9_$]*)\s*=`)
	jsBracketReadRe   = regexp.MustCompile(`\b(?:this\.)?(?:[A-Za-z_$][A-Za-z0-9_$]*\.)*[A-Za-z_$][A-Za-z0-9_$]*\s*\[\s*["']([A-Za-z_$][A-Za-z0-9_$]*)["']\s*\]`)
	jsBracketWriteRe  = regexp.MustCompile(`\b(?:this\.)?(?:[A-Za-z_$][A-Za-z0-9_$]*\.)*[A-Za-z_$][A-Za-z0-9_$]*\s*\[\s*["']([A-Za-z_$][A-Za-z0-9_$]*)["']\s*\]\s*=`)
	jsStoreToRefsRe   = regexp.MustCompile(`\bconst\s*{\s*([^}]*)\s*}\s*=\s*(?:storeToRefs|toRefs)\s*\([^)]*\)`)
	jsSimpleAliasRe   = regexp.MustCompile(`\b(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*([A-Za-z_$][A-Za-z0-9_$]*)\.([A-Za-z_$][A-Za-z0-9_$]*)\b`)
)

var jsBuiltinRoots = map[string]bool{
	"Math": true, "JSON": true, "Object": true, "Array": true, "String": true,
	"Number": true, "Boolean": true, "Date": true, "RegExp": true, "Promise": true,
	"Reflect": true, "Intl": true, "Set": true, "Map": true, "WeakSet": true,
	"WeakMap": true, "Symbol": true, "BigInt": true, "console": true,
	"window": true, "document": true, "globalThis": true,
}

var jsVueRuntimeInternalProps = map[string]bool{
	"$el": true, "$refs": true, "$slots": true, "$attrs": true, "$listeners": true,
	"$parent": true, "$root": true, "$children": true, "$scopedSlots": true,
	"$isServer": true, "$ssrContext": true, "$vnode": true, "$props": true,
}

func (e *RegexExtractor) extractJSPropertyReferences(filePath string, content string, lines []string, functionBoundaries []functionBoundary) []Reference {
	writeMatches := jsPropertyWriteRe.FindAllStringSubmatchIndex(content, -1)
	writeStarts := make(map[int]bool, len(writeMatches))
	refs := make([]Reference, 0, len(writeMatches))

	for _, m := range writeMatches {
		if len(m) < 4 {
			continue
		}
		start := m[0]
		writeStarts[start] = true
		expr := content[m[0]:m[1]]
		name := content[m[2]:m[3]]
		if !keepJSPropertyReference(name, expr) {
			continue
		}
		refs = append(refs, buildDataReference(filePath, content, lines, name, start, RefKindWrite, functionBoundaries))
	}

	readMatches := jsPropertyReadRe.FindAllStringSubmatchIndex(content, -1)
	for _, m := range readMatches {
		if len(m) < 4 {
			continue
		}
		start := m[0]
		if writeStarts[start] {
			continue
		}
		expr := content[m[0]:m[1]]
		name := content[m[2]:m[3]]
		if !keepJSPropertyReference(name, expr) {
			continue
		}
		refs = append(refs, buildDataReference(filePath, content, lines, name, start, RefKindRead, functionBoundaries))
	}

	// Bracket property access: store["uid"], this.store['role']
	bracketWriteMatches := jsBracketWriteRe.FindAllStringSubmatchIndex(content, -1)
	for _, m := range bracketWriteMatches {
		if len(m) < 4 {
			continue
		}
		start := m[0]
		writeStarts[start] = true
		expr := content[m[0]:m[1]]
		name := content[m[2]:m[3]]
		if !keepJSPropertyReference(name, expr) {
			continue
		}
		refs = append(refs, buildDataReference(filePath, content, lines, name, start, RefKindWrite, functionBoundaries))
	}

	bracketReadMatches := jsBracketReadRe.FindAllStringSubmatchIndex(content, -1)
	for _, m := range bracketReadMatches {
		if len(m) < 4 {
			continue
		}
		start := m[0]
		if writeStarts[start] {
			continue
		}
		expr := content[m[0]:m[1]]
		name := content[m[2]:m[3]]
		if !keepJSPropertyReference(name, expr) {
			continue
		}
		refs = append(refs, buildDataReference(filePath, content, lines, name, start, RefKindRead, functionBoundaries))
	}

	refs = append(refs, extractJSAliasPropertyReferences(filePath, content, lines, functionBoundaries)...)
	return dedupeReferences(refs)
}

func keepJSPropertyReference(name string, expr string) bool {
	if name == "" || strings.HasPrefix(name, "_") {
		return false
	}
	if jsVueRuntimeInternalProps[name] {
		return false
	}
	root := extractJSRootIdentifier(expr)
	return !jsBuiltinRoots[root]
}

func extractJSRootIdentifier(expr string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ""
	}
	expr = strings.TrimPrefix(expr, "this.")
	for i, r := range expr {
		if r == '.' || r == '[' || r == '(' || r == ' ' || r == '\t' || r == '\n' {
			if i == 0 {
				return ""
			}
			return expr[:i]
		}
	}
	return expr
}

type jsAliasProperty struct {
	alias    string
	propName string
	isRef    bool
	declPos  int
}

func extractJSAliasPropertyReferences(filePath, content string, lines []string, functionBoundaries []functionBoundary) []Reference {
	aliases := collectJSAliases(content)
	if len(aliases) == 0 {
		return nil
	}

	refs := make([]Reference, 0)
	for _, a := range aliases {
		if a.isRef {
			readRe := regexp.MustCompile(`\b` + regexp.QuoteMeta(a.alias) + `\s*\.\s*value\b`)
			for _, m := range readRe.FindAllStringIndex(content, -1) {
				if m[0] == a.declPos {
					continue
				}
				refs = append(refs, buildDataReference(filePath, content, lines, a.propName, m[0], RefKindRead, functionBoundaries))
			}
			writeEqRe := regexp.MustCompile(`\b` + regexp.QuoteMeta(a.alias) + `\s*\.\s*value\s*=`)
			for _, m := range writeEqRe.FindAllStringIndex(content, -1) {
				if m[0] == a.declPos {
					continue
				}
				refs = append(refs, buildDataReference(filePath, content, lines, a.propName, m[0], RefKindWrite, functionBoundaries))
			}
			writeUpdRe := regexp.MustCompile(`\b` + regexp.QuoteMeta(a.alias) + `\s*\.\s*value\s*(?:\+\+|--)`)
			for _, m := range writeUpdRe.FindAllStringIndex(content, -1) {
				if m[0] == a.declPos {
					continue
				}
				refs = append(refs, buildDataReference(filePath, content, lines, a.propName, m[0], RefKindWrite, functionBoundaries))
			}
			continue
		}

		writeRe := regexp.MustCompile(`\b` + regexp.QuoteMeta(a.alias) + `\s*=`)
		writeStarts := map[int]bool{}
		for _, m := range writeRe.FindAllStringIndex(content, -1) {
			if m[0] == a.declPos {
				continue
			}
			writeStarts[m[0]] = true
			refs = append(refs, buildDataReference(filePath, content, lines, a.propName, m[0], RefKindWrite, functionBoundaries))
		}
		readRe := regexp.MustCompile(`\b` + regexp.QuoteMeta(a.alias) + `\b`)
		for _, m := range readRe.FindAllStringIndex(content, -1) {
			if m[0] == a.declPos || writeStarts[m[0]] {
				continue
			}
			refs = append(refs, buildDataReference(filePath, content, lines, a.propName, m[0], RefKindRead, functionBoundaries))
		}
	}

	return dedupeReferences(refs)
}

func collectJSAliases(content string) []jsAliasProperty {
	aliases := make([]jsAliasProperty, 0)

	destructured := jsStoreToRefsRe.FindAllStringSubmatchIndex(content, -1)
	for _, m := range destructured {
		if len(m) < 4 {
			continue
		}
		inner := content[m[2]:m[3]]
		parts := strings.Split(inner, ",")
		for _, p := range parts {
			part := strings.TrimSpace(p)
			if part == "" {
				continue
			}
			prop := part
			alias := part
			if strings.Contains(part, ":") {
				sides := strings.SplitN(part, ":", 2)
				prop = strings.TrimSpace(sides[0])
				alias = strings.TrimSpace(sides[1])
			}
			if prop == "" || alias == "" {
				continue
			}
			declPos := m[2] + strings.Index(inner, alias)
			aliases = append(aliases, jsAliasProperty{alias: alias, propName: prop, isRef: true, declPos: declPos})
		}
	}

	simple := jsSimpleAliasRe.FindAllStringSubmatchIndex(content, -1)
	for _, m := range simple {
		if len(m) < 8 {
			continue
		}
		alias := strings.TrimSpace(content[m[2]:m[3]])
		root := strings.TrimSpace(content[m[4]:m[5]])
		prop := strings.TrimSpace(content[m[6]:m[7]])
		if alias == "" || prop == "" {
			continue
		}
		if jsBuiltinRoots[root] || jsVueRuntimeInternalProps[prop] || strings.HasPrefix(prop, "_") {
			continue
		}
		aliases = append(aliases, jsAliasProperty{alias: alias, propName: prop, isRef: false, declPos: m[2]})
	}

	return aliases
}

// isDeclarationReferenceMatch reports whether a regex call match occurs in a declaration context.
func isDeclarationReferenceMatch(content string, patterns *LanguagePatterns, pos int) bool {
	if patterns == nil {
		return false
	}

	switch patterns.Language {
	case "lua":
		return isLuaDeclarationReferenceMatch(content, pos)
	default:
		return false
	}
}

// isLuaDeclarationReferenceMatch filters out call-like matches inside Lua function declarations.
func isLuaDeclarationReferenceMatch(content string, pos int) bool {
	lineStart := 0
	if idx := strings.LastIndexByte(content[:pos], '\n'); idx >= 0 {
		lineStart = idx + 1
	}

	lineEnd := len(content)
	if idx := strings.IndexByte(content[pos:], '\n'); idx >= 0 {
		lineEnd = pos + idx
	}

	line := content[lineStart:lineEnd]
	signatureEnd := strings.IndexByte(line, ')')
	if signatureEnd < 0 || pos >= lineStart+signatureEnd+1 {
		return false
	}

	linePrefix := strings.TrimSpace(content[lineStart:pos])
	return strings.HasPrefix(linePrefix, "function") || strings.HasPrefix(linePrefix, "local function")
}

// extractLuaBracketKeyReferences maps bracket-key calls like obj["foo"]() to foo.
// It skips call-result key accesses like obj["bar"]()["foo"]() to avoid over-linking dynamic values.
func (e *RegexExtractor) extractLuaBracketKeyReferences(filePath string, content string, lines []string, patterns *LanguagePatterns, functionBoundaries []functionBoundary) []Reference {
	if patterns == nil || patterns.BracketKeyCall == nil {
		return nil
	}

	commentMask := buildLuaCommentMask(content)
	bracketScanContent := maskedContent(content, commentMask)
	matches := patterns.BracketKeyCall.FindAllStringSubmatchIndex(bracketScanContent, -1)
	refs := make([]Reference, 0, len(matches))

	for _, match := range matches {
		if len(match) < 4 || commentMask[match[2]] {
			continue
		}
		if prevNonSpaceByte(bracketScanContent, match[0]) == ')' {
			continue
		}
		name := content[match[2]:match[3]]
		if IsKeyword(name, patterns.Language) {
			continue
		}
		refs = append(refs, buildReference(filePath, content, lines, name, match[0], functionBoundaries))
	}

	return refs
}

// buildIgnoredMask marks bytes inside strings and comments.
// Regex-based call extraction uses this mask to skip obvious artifacts.
// lang is used to handle language-specific comment syntax (e.g., F#'s (* *) block comments).
func buildIgnoredMask(content string, lang string) []bool {
	mask := make([]bool, len(content))
	const (
		stateNormal = iota
		stateLineComment
		stateBlockComment
		stateParenBlockComment // F#/OCaml (* *) block comments
		stateSingleQuote
		stateDoubleQuote
		stateBacktick
		stateLuaLongComment
		stateLuaLongString
	)

	state := stateNormal
	parenDepth := 0
	luaLongEqCount := 0
	for i := 0; i < len(content); i++ {
		ch := content[i]
		next := byte(0)
		if i+1 < len(content) {
			next = content[i+1]
		}

		switch state {
		case stateLineComment:
			mask[i] = true
			if ch == '\n' {
				state = stateNormal
			}
			continue
		case stateBlockComment:
			mask[i] = true
			if ch == '*' && next == '/' {
				mask[i+1] = true
				i++
				state = stateNormal
			}
			continue
		case stateParenBlockComment:
			mask[i] = true
			if ch == '(' && next == '*' {
				parenDepth++
				mask[i+1] = true
				i++
			} else if ch == '*' && next == ')' {
				mask[i+1] = true
				i++
				parenDepth--
				if parenDepth == 0 {
					state = stateNormal
				}
			}
			continue
		case stateSingleQuote:
			mask[i] = true
			if ch == '\\' && i+1 < len(content) {
				mask[i+1] = true
				i++
				continue
			}
			if ch == '\'' {
				state = stateNormal
			}
			continue
		case stateDoubleQuote:
			mask[i] = true
			if ch == '\\' && i+1 < len(content) {
				mask[i+1] = true
				i++
				continue
			}
			if ch == '"' {
				state = stateNormal
			}
			continue
		case stateBacktick:
			mask[i] = true
			if ch == '`' {
				state = stateNormal
			}
			continue
		case stateLuaLongComment, stateLuaLongString:
			mask[i] = true
			if closeLen := luaLongBracketCloseAt(content, i, luaLongEqCount); closeLen > 0 {
				for j := 0; j < closeLen; j++ {
					mask[i+j] = true
				}
				i += closeLen - 1
				state = stateNormal
			}
			continue
		}

		if lang == "lua" {
			if ch == '-' && next == '-' {
				mask[i] = true
				mask[i+1] = true
				if openLen, eqCount, ok := luaLongBracketOpenAt(content, i+2); ok {
					for j := 0; j < openLen; j++ {
						mask[i+2+j] = true
					}
					i += 1 + openLen
					luaLongEqCount = eqCount
					state = stateLuaLongComment
				} else {
					i++
					state = stateLineComment
				}
				continue
			}
			if openLen, eqCount, ok := luaLongBracketOpenAt(content, i); ok {
				for j := 0; j < openLen; j++ {
					mask[i+j] = true
				}
				i += openLen - 1
				luaLongEqCount = eqCount
				state = stateLuaLongString
				continue
			}
		}

		if ch == '/' && next == '/' {
			mask[i] = true
			mask[i+1] = true
			i++
			state = stateLineComment
			continue
		}
		if ch == '/' && next == '*' {
			mask[i] = true
			mask[i+1] = true
			i++
			state = stateBlockComment
			continue
		}
		if lang == "fsharp" && ch == '(' && next == '*' {
			mask[i] = true
			mask[i+1] = true
			i++
			parenDepth = 1
			state = stateParenBlockComment
			continue
		}
		if ch == '\'' {
			mask[i] = true
			state = stateSingleQuote
			continue
		}
		if ch == '"' {
			mask[i] = true
			state = stateDoubleQuote
			continue
		}
		if ch == '`' {
			mask[i] = true
			state = stateBacktick
			continue
		}
	}

	return mask
}

// isDeclarationCallArtifact filters regex call matches that are actually
// function/method declarations in JS/TS-like languages.
func isDeclarationCallArtifact(content string, pos int, name string, lang string) bool {
	if lang != "javascript" && lang != "typescript" {
		return false
	}

	lineStart := strings.LastIndex(content[:pos], "\n") + 1
	lineEndRel := strings.Index(content[pos:], "\n")
	lineEnd := len(content)
	if lineEndRel >= 0 {
		lineEnd = pos + lineEndRel
	}
	line := strings.TrimSpace(content[lineStart:lineEnd])
	if line == "" {
		return false
	}

	declPrefixes := []string{
		"function " + name + "(",
		"async function " + name + "(",
		"export function " + name + "(",
		"export async function " + name + "(",
		"export default function " + name + "(",
		"export default async function " + name + "(",
	}
	for _, p := range declPrefixes {
		if strings.HasPrefix(line, p) {
			return true
		}
	}

	// Class/object method declaration styles:
	// - methodName(args) { ... }
	// - public async methodName(args) { ... }
	// These must include "{" on the same line.
	if strings.Contains(line, "{") {
		withoutMods := stripTSMethodModifiers(line)
		if strings.HasPrefix(withoutMods, name+"(") {
			return true
		}
	}

	return false
}

func stripTSMethodModifiers(line string) string {
	out := strings.TrimSpace(line)
	mods := []string{"public ", "private ", "protected ", "static ", "readonly ", "abstract ", "override ", "async "}
	for {
		changed := false
		for _, m := range mods {
			if strings.HasPrefix(out, m) {
				out = strings.TrimSpace(strings.TrimPrefix(out, m))
				changed = true
			}
		}
		if !changed {
			return out
		}
	}
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
	case "go", "javascript", "typescript", "php", "c", "zig", "rust", "cpp":
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
	case "fsharp":
		// F# uses indentation-based scoping, same as Python
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
				pos := 0
				for j := 0; j < i; j++ {
					pos += len(lines[j]) + 1
				}
				return pos
			}
		}
	case "lua":
		// Lua functions end with 'end', but we need to handle nested functions and control structures.
		return findLuaFunctionEnd(content, start)
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
	found := false
	for _, b := range boundaries {
		if b.StartPos <= pos && pos < b.EndPos {
			if !found || b.StartPos > best.StartPos {
				best = b
				found = true
			}
		}
	}
	if !found {
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

// buildReference constructs a reference with caller and context metadata.
func buildReference(filePath, content string, lines []string, name string, pos int, functionBoundaries []functionBoundary) Reference {
	line := countLines(content[:pos]) + 1
	caller := findContainingFunction(pos, functionBoundaries)

	return Reference{
		SymbolName: name,
		Kind:       RefKindCall,
		File:       filePath,
		Line:       line,
		Context:    getLineContext(lines, line-1, 0),
		CallerName: caller.Name,
		CallerFile: filePath,
		CallerLine: caller.Line,
	}
}

func buildDataReference(filePath, content string, lines []string, name string, pos int, kind string, functionBoundaries []functionBoundary) Reference {
	line := countLines(content[:pos]) + 1
	caller := findContainingFunction(pos, functionBoundaries)
	return Reference{
		SymbolName: name,
		Kind:       kind,
		File:       filePath,
		Line:       line,
		Context:    getLineContext(lines, line-1, 0),
		CallerName: caller.Name,
		CallerFile: filePath,
		CallerLine: caller.Line,
	}
}

// dedupeReferences removes duplicate logical refs emitted by overlapping regex passes.
func dedupeReferences(refs []Reference) []Reference {
	if len(refs) < 2 {
		return refs
	}

	seen := make(map[string]bool, len(refs))
	deduped := make([]Reference, 0, len(refs))

	for _, ref := range refs {
		key := ref.SymbolName + "\x00" + ref.Kind + "\x00" + ref.File + "\x00" + ref.CallerName + "\x00" + strconv.Itoa(ref.Line) + "\x00" + strconv.Itoa(ref.CallerLine)
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, ref)
	}

	return deduped
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

// findLuaFunctionEnd locates the matching end of a Lua function body.
func findLuaFunctionEnd(content string, start int) int {
	ignored := buildIgnoredMask(content, "lua")
	depth := 1
	pendingLoopDo := 0

	for i := start; i < len(content); {
		if ignored[i] {
			i++
			continue
		}
		if isIdentStartASCII(content[i]) {
			j := i + 1
			for j < len(content) && isIdentPartASCII(content[j]) {
				j++
			}
			token := content[i:j]
			switch token {
			case "function", "if":
				depth++
			case "for", "while":
				depth++
				pendingLoopDo++
			case "do":
				if pendingLoopDo > 0 {
					pendingLoopDo--
				} else {
					depth++
				}
			case "repeat":
				depth++
			case "end", "until":
				depth--
				if depth == 0 {
					return j
				}
			}
			i = j
			continue
		}
		i++
	}

	return len(content)
}

// buildLuaCommentMask marks only Lua comment bytes (line and long comments).
// It preserves string bytes so bracket-key calls like obj["foo"]() remain matchable.
func buildLuaCommentMask(content string) []bool {
	mask := make([]bool, len(content))
	const (
		stateNormal = iota
		stateLineComment
		stateLongComment
		stateSingleQuote
		stateDoubleQuote
		stateLongString
	)

	state := stateNormal
	longEqCount := 0
	for i := 0; i < len(content); i++ {
		ch := content[i]
		next := byte(0)
		if i+1 < len(content) {
			next = content[i+1]
		}

		switch state {
		case stateLineComment:
			mask[i] = true
			if ch == '\n' {
				state = stateNormal
			}
			continue
		case stateLongComment:
			mask[i] = true
			if closeLen := luaLongBracketCloseAt(content, i, longEqCount); closeLen > 0 {
				for j := 0; j < closeLen; j++ {
					mask[i+j] = true
				}
				i += closeLen - 1
				state = stateNormal
			}
			continue
		case stateSingleQuote:
			if ch == '\\' && i+1 < len(content) {
				i++
				continue
			}
			if ch == '\'' {
				state = stateNormal
			}
			continue
		case stateDoubleQuote:
			if ch == '\\' && i+1 < len(content) {
				i++
				continue
			}
			if ch == '"' {
				state = stateNormal
			}
			continue
		case stateLongString:
			if closeLen := luaLongBracketCloseAt(content, i, longEqCount); closeLen > 0 {
				i += closeLen - 1
				state = stateNormal
			}
			continue
		}

		if ch == '-' && next == '-' {
			mask[i] = true
			mask[i+1] = true
			if openLen, eqCount, ok := luaLongBracketOpenAt(content, i+2); ok {
				for j := 0; j < openLen; j++ {
					mask[i+2+j] = true
				}
				i += 1 + openLen
				longEqCount = eqCount
				state = stateLongComment
			} else {
				i++
				state = stateLineComment
			}
			continue
		}
		if openLen, eqCount, ok := luaLongBracketOpenAt(content, i); ok {
			i += openLen - 1
			longEqCount = eqCount
			state = stateLongString
			continue
		}
		if ch == '\'' {
			state = stateSingleQuote
			continue
		}
		if ch == '"' {
			state = stateDoubleQuote
			continue
		}
	}

	return mask
}

func maskedContent(content string, ignored []bool) string {
	if len(content) == 0 || len(ignored) != len(content) {
		return content
	}

	b := []byte(content)
	for i, skip := range ignored {
		if !skip {
			continue
		}
		if b[i] == '\n' || b[i] == '\r' {
			continue
		}
		b[i] = ' '
	}
	return string(b)
}

func luaLongBracketOpenAt(content string, i int) (length int, eqCount int, ok bool) {
	if i >= len(content) || content[i] != '[' {
		return 0, 0, false
	}
	j := i + 1
	for j < len(content) && content[j] == '=' {
		j++
	}
	if j >= len(content) || content[j] != '[' {
		return 0, 0, false
	}
	return j - i + 1, j - (i + 1), true
}

func luaLongBracketCloseAt(content string, i int, eqCount int) int {
	if i >= len(content) || content[i] != ']' {
		return 0
	}
	j := i + 1
	for n := 0; n < eqCount; n++ {
		if j >= len(content) || content[j] != '=' {
			return 0
		}
		j++
	}
	if j >= len(content) || content[j] != ']' {
		return 0
	}
	return j - i + 1
}

func prevNonSpaceByte(content string, i int) byte {
	for j := i - 1; j >= 0; j-- {
		if content[j] == ' ' || content[j] == '\t' || content[j] == '\n' || content[j] == '\r' {
			continue
		}
		return content[j]
	}
	return 0
}

func isIdentStartASCII(b byte) bool {
	return b == '_' || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func isIdentPartASCII(b byte) bool {
	return isIdentStartASCII(b) || (b >= '0' && b <= '9')
}
