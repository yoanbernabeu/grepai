package framework

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
)

//go:embed scripts/vue_processor.mjs
var vueProcessorScript string

type VueProcessor struct {
	nodePath string
	mu       sync.RWMutex
	missing  string
}

func NewVueProcessor(nodePath string) *VueProcessor {
	if nodePath == "" {
		nodePath = "node"
	}
	return &VueProcessor{nodePath: nodePath}
}

func (p *VueProcessor) Name() string { return "vue" }
func (p *VueProcessor) Supports(filePath string) bool {
	return hasExt(filePath, ".vue")
}
func (p *VueProcessor) Capabilities() ProcessorCapabilities {
	return ProcessorCapabilities{Embedding: true, Trace: true, Compiled: true}
}

func (p *VueProcessor) TransformForEmbedding(ctx context.Context, filePath, source string) (TransformResult, error) {
	return p.transform(ctx, filePath, source)
}

func (p *VueProcessor) TransformForTrace(ctx context.Context, filePath, source string) (TransformResult, error) {
	return p.transform(ctx, filePath, source)
}

type vueScriptInput struct {
	FilePath string `json:"filePath"`
	Source   string `json:"source"`
}

type vueScriptOutput struct {
	EmbeddingText        string   `json:"embeddingText"`
	TraceText            string   `json:"traceText"`
	VirtualPath          string   `json:"virtualPath"`
	GeneratedToSourceMap []int    `json:"generatedToSourceLine"`
	Warnings             []string `json:"warnings"`
}

func (p *VueProcessor) transform(ctx context.Context, filePath, source string) (TransformResult, error) {
	if msg, ok := p.missingCompilerMessage(); ok {
		return p.fallbackWithCompilerWarning(filePath, source, msg)
	}

	in, _ := json.Marshal(vueScriptInput{FilePath: filePath, Source: source})
	cmd := exec.CommandContext(ctx, p.nodePath, "--input-type=module", "-e", vueProcessorScript) //nolint:gosec // nodePath is an executable path from trusted config; no shell is invoked
	cmd.Stdin = bytes.NewReader(in)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if isMissingVueCompiler(msg) {
			msg = normalizeMissingVueCompilerMessage(filePath)
			p.setMissingCompilerMessage(msg)
		}
		fallback, fbErr := p.fallback(filePath, source)
		if fbErr == nil {
			fallback.Warnings = append(fallback.Warnings,
				fmt.Sprintf("vue compiler unavailable: %s", msg))
			return fallback, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		return TransformResult{}, fmt.Errorf("%w: vue processor failed: %v (%s)", ErrUnavailable, err, strings.TrimSpace(stderr.String()))
	}

	var out vueScriptOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return TransformResult{}, fmt.Errorf("%w: invalid vue processor output: %v", ErrUnavailable, err)
	}
	text := out.EmbeddingText
	if text == "" {
		text = source
	}
	virtual := out.VirtualPath
	if virtual == "" {
		virtual = filePath + ".__trace__.ts"
	}
	text, mapping := appendTemplateCtxReadCalls(text, out.GeneratedToSourceMap)
	return TransformResult{
		Processor:             p.Name(),
		FilePath:              filePath,
		VirtualPath:           virtual,
		Text:                  text,
		GeneratedToSourceLine: mapping,
		Warnings:              out.Warnings,
		Transformed:           true,
	}, nil
}

func (p *VueProcessor) fallbackWithCompilerWarning(filePath, source, msg string) (TransformResult, error) {
	fallback, err := p.fallback(filePath, source)
	if err != nil {
		out := passthrough(filePath, source)
		out.Processor = p.Name()
		out.Warnings = append(out.Warnings, fmt.Sprintf("vue compiler unavailable: %s", msg))
		return out, fmt.Errorf("%w: vue compiler unavailable", ErrUnavailable)
	}
	fallback.Warnings = append(fallback.Warnings, fmt.Sprintf("vue compiler unavailable: %s", msg))
	return fallback, fmt.Errorf("%w: vue compiler unavailable", ErrUnavailable)
}

func isMissingVueCompiler(msg string) bool {
	return strings.Contains(msg, "@vue/compiler-sfc") &&
		(strings.Contains(msg, "Cannot find package") || strings.Contains(msg, "Cannot resolve @vue/compiler-sfc"))
}

func normalizeMissingVueCompilerMessage(filePath string) string {
	return fmt.Sprintf("Cannot resolve @vue/compiler-sfc from %s; install it in the Vue workspace to enable full SFC compilation", pathDir(filePath))
}

func pathDir(filePath string) string {
	lastSlash := strings.LastIndexAny(filePath, `/\`)
	if lastSlash < 0 {
		return "."
	}
	if lastSlash == 0 {
		return filePath[:1]
	}
	return filePath[:lastSlash]
}

func (p *VueProcessor) missingCompilerMessage() (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.missing, p.missing != ""
}

func (p *VueProcessor) setMissingCompilerMessage(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.missing == "" {
		p.missing = msg
	}
}

var scriptBlockRE = regexp.MustCompile(`(?is)<script\b[^>]*>(.*?)</script>`)
var styleBlockRE = regexp.MustCompile(`(?is)<style\b[^>]*>(.*?)</style>`)
var vueTemplateCtxIdentRE = regexp.MustCompile(`\b_ctx\.([A-Za-z_$][A-Za-z0-9_$]*)\b`)

func (p *VueProcessor) fallback(filePath, source string) (TransformResult, error) {
	matches := scriptBlockRE.FindAllStringSubmatchIndex(source, -1)
	var out strings.Builder
	mapping := make([]int, 0, len(source)/20)
	for idx, m := range matches {
		if len(m) < 4 {
			continue
		}
		start, end := m[2], m[3]
		block := source[start:end]
		if strings.TrimSpace(block) == "" {
			continue
		}
		if idx > 0 && out.Len() > 0 {
			out.WriteString("\n")
			mapping = append(mapping, 0)
		}

		sourceStart := strings.Count(source[:start], "\n") + 1
		lines := strings.Split(block, "\n")
		for i, line := range lines {
			if i > 0 {
				out.WriteString("\n")
			}
			out.WriteString(line)
			mapping = append(mapping, sourceStart+i)
		}
	}

	styleMatches := styleBlockRE.FindAllStringSubmatchIndex(source, -1)
	styleBindingIndex := 0
	for _, m := range styleMatches {
		if len(m) < 4 {
			continue
		}
		start, end := m[2], m[3]
		block := source[start:end]
		if strings.TrimSpace(block) == "" {
			continue
		}
		lineOffset := strings.Count(source[:start], "\n") + 1
		refs := extractStyleVBindRefs(block, lineOffset)
		if len(refs) == 0 {
			continue
		}
		if out.Len() > 0 {
			out.WriteString("\n")
			mapping = append(mapping, 0)
		}
		header := fmt.Sprintf("function __vue_style_bindings__%d() {", styleBindingIndex)
		out.WriteString(header)
		mapping = append(mapping, refs[0].line)
		for _, ref := range refs {
			out.WriteString("\n  __css_v_bind__(" + ref.expr + ");")
			mapping = append(mapping, ref.line)
		}
		out.WriteString("\n}")
		mapping = append(mapping, refs[len(refs)-1].line)
		styleBindingIndex++
	}

	if out.Len() == 0 {
		return TransformResult{}, fmt.Errorf("no script blocks or style v-bind expressions found")
	}
	text, mapped := appendTemplateCtxReadCalls(out.String(), mapping)

	return TransformResult{
		Processor:             p.Name(),
		FilePath:              filePath,
		VirtualPath:           filePath + ".__trace__.ts",
		Text:                  text,
		GeneratedToSourceLine: mapped,
		Transformed:           true,
	}, nil
}

type styleVBindRef struct {
	expr string
	line int
}

func extractStyleVBindRefs(styleContent string, startLine int) []styleVBindRef {
	lines := strings.Split(styleContent, "\n")
	refs := make([]styleVBindRef, 0, len(lines))
	for i, line := range lines {
		exprs := extractVBindExprsFromLine(line)
		for _, raw := range exprs {
			expr := normalizeStyleVBindExpr(raw)
			if expr == "" {
				continue
			}
			refs = append(refs, styleVBindRef{
				expr: expr,
				line: startLine + i,
			})
		}
	}
	return refs
}

func extractVBindExprsFromLine(line string) []string {
	const needle = "v-bind("
	out := make([]string, 0, 2)
	start := 0

	for start < len(line) {
		idx := strings.Index(line[start:], needle)
		if idx < 0 {
			break
		}
		openIdx := start + idx + len(needle)
		depth := 1
		i := openIdx
		for i < len(line) && depth > 0 {
			switch line[i] {
			case '(':
				depth++
			case ')':
				depth--
			}
			i++
		}
		if depth != 0 {
			break
		}
		out = append(out, line[openIdx:i-1])
		start = i
	}

	return out
}

func normalizeStyleVBindExpr(expr string) string {
	trimmed := strings.TrimSpace(expr)
	if len(trimmed) >= 2 {
		if (trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'') || (trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') {
			return strings.TrimSpace(trimmed[1 : len(trimmed)-1])
		}
	}
	return trimmed
}

func appendTemplateCtxReadCalls(text string, generatedToSource []int) (string, []int) {
	if strings.TrimSpace(text) == "" {
		return text, generatedToSource
	}

	lines := strings.Split(text, "\n")
	sourceLineByIdent := make(map[string]int)
	for i, line := range lines {
		matches := vueTemplateCtxIdentRE.FindAllStringSubmatch(line, -1)
		if len(matches) == 0 {
			continue
		}
		lineSource := 0
		if i < len(generatedToSource) {
			lineSource = generatedToSource[i]
		}
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			name := m[1]
			if strings.HasPrefix(name, "$") || strings.HasPrefix(name, "_") {
				continue
			}
			if _, ok := sourceLineByIdent[name]; !ok {
				sourceLineByIdent[name] = lineSource
			}
		}
	}

	if len(sourceLineByIdent) == 0 {
		return text, generatedToSource
	}

	idents := make([]string, 0, len(sourceLineByIdent))
	for name := range sourceLineByIdent {
		idents = append(idents, name)
	}
	sort.Strings(idents)

	baseSource := 0
	for _, name := range idents {
		if sourceLineByIdent[name] > 0 {
			baseSource = sourceLineByIdent[name]
			break
		}
	}

	var b strings.Builder
	b.Grow(len(text) + 64 + len(idents)*24)
	b.WriteString(text)
	b.WriteString("\nfunction __vue_template_reads__() {")
	updatedMap := append([]int(nil), generatedToSource...)
	updatedMap = append(updatedMap, baseSource)
	for _, name := range idents {
		b.WriteString("\n  ")
		b.WriteString(name)
		b.WriteString("();")
		updatedMap = append(updatedMap, sourceLineByIdent[name])
	}
	b.WriteString("\n}")
	updatedMap = append(updatedMap, baseSource)

	return b.String(), updatedMap
}
