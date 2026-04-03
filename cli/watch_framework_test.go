package cli

import (
	"context"
	"testing"

	"github.com/yoanbernabeu/grepai/framework"
	"github.com/yoanbernabeu/grepai/trace"
)

type mockSymbolExtractor struct {
	symbols []trace.Symbol
	refs    []trace.Reference
}

func (m *mockSymbolExtractor) ExtractSymbols(ctx context.Context, filePath string, content string) ([]trace.Symbol, error) {
	return m.symbols, nil
}
func (m *mockSymbolExtractor) ExtractReferences(ctx context.Context, filePath string, content string) ([]trace.Reference, error) {
	return m.refs, nil
}
func (m *mockSymbolExtractor) ExtractAll(ctx context.Context, filePath string, content string) ([]trace.Symbol, []trace.Reference, error) {
	return m.symbols, m.refs, nil
}
func (m *mockSymbolExtractor) SupportedLanguages() []string { return []string{".ts"} }
func (m *mockSymbolExtractor) Mode() string                 { return "fast" }

type mockFrameworkProcessor struct{}

func (m *mockFrameworkProcessor) Name() string { return "vue" }
func (m *mockFrameworkProcessor) Supports(filePath string) bool {
	return true
}
func (m *mockFrameworkProcessor) Capabilities() framework.ProcessorCapabilities {
	return framework.ProcessorCapabilities{Embedding: true, Trace: true}
}
func (m *mockFrameworkProcessor) TransformForEmbedding(ctx context.Context, filePath, source string) (framework.TransformResult, error) {
	return framework.TransformResult{FilePath: filePath, VirtualPath: filePath, Text: source}, nil
}
func (m *mockFrameworkProcessor) TransformForTrace(ctx context.Context, filePath, source string) (framework.TransformResult, error) {
	return framework.TransformResult{
		FilePath:              filePath,
		VirtualPath:           filePath + ".__trace__.ts",
		Text:                  "transformed",
		GeneratedToSourceLine: []int{10, 20, 30},
	}, nil
}

func TestExtractSymbolsWithFramework_RemapsFileAndLines(t *testing.T) {
	ex := &mockSymbolExtractor{
		symbols: []trace.Symbol{{Name: "fn", File: "virtual.ts", Line: 2, EndLine: 3}},
		refs:    []trace.Reference{{SymbolName: "callee", File: "virtual.ts", Line: 3, CallerFile: "virtual.ts", CallerLine: 1}},
	}

	reg := framework.NewProcessorRegistry(
		framework.RegistryConfig{Enabled: true, Mode: framework.ModeAuto, EnableVue: true},
		&mockFrameworkProcessor{},
	)

	symbols, refs, err := extractSymbolsWithFramework(context.Background(), ex, "src/Test.vue", "original", reg)
	if err != nil {
		t.Fatalf("extractSymbolsWithFramework failed: %v", err)
	}
	if len(symbols) != 1 || len(refs) != 1 {
		t.Fatalf("unexpected symbols/refs counts: %d/%d", len(symbols), len(refs))
	}

	if symbols[0].File != "src/Test.vue" {
		t.Fatalf("symbol file = %q, want src/Test.vue", symbols[0].File)
	}
	if symbols[0].Line != 20 || symbols[0].EndLine != 30 {
		t.Fatalf("symbol lines = %d-%d, want 20-30", symbols[0].Line, symbols[0].EndLine)
	}

	if refs[0].File != "src/Test.vue" || refs[0].CallerFile != "src/Test.vue" {
		t.Fatalf("reference file remap failed: file=%q caller=%q", refs[0].File, refs[0].CallerFile)
	}
	if refs[0].Line != 30 || refs[0].CallerLine != 10 {
		t.Fatalf("reference lines = %d/%d, want 30/10", refs[0].Line, refs[0].CallerLine)
	}
}
