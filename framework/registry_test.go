package framework

import (
	"context"
	"strings"
	"testing"
)

type stubProcessor struct {
	name     string
	ext      string
	result   TransformResult
	err      error
	supports bool
}

func (s *stubProcessor) Name() string { return s.name }
func (s *stubProcessor) Supports(filePath string) bool {
	if !s.supports {
		return false
	}
	return hasExt(filePath, s.ext)
}
func (s *stubProcessor) Capabilities() ProcessorCapabilities { return ProcessorCapabilities{} }
func (s *stubProcessor) TransformForEmbedding(ctx context.Context, filePath, source string) (TransformResult, error) {
	if s.result.Text == "" {
		s.result = TransformResult{Text: source, FilePath: filePath, VirtualPath: filePath}
	}
	return s.result, s.err
}
func (s *stubProcessor) TransformForTrace(ctx context.Context, filePath, source string) (TransformResult, error) {
	if s.result.Text == "" {
		s.result = TransformResult{Text: source, FilePath: filePath, VirtualPath: filePath}
	}
	return s.result, s.err
}

func TestRegistryPassthroughWhenOff(t *testing.T) {
	r := NewProcessorRegistry(RegistryConfig{Enabled: false, Mode: ModeOff})
	res, err := r.TransformForEmbedding(context.Background(), "a.vue", "hello")
	if err != nil {
		t.Fatalf("TransformForEmbedding err: %v", err)
	}
	if res.Processor != "passthrough" {
		t.Fatalf("processor=%q want passthrough", res.Processor)
	}
}

func TestRegistryAutoFallbackOnUnavailable(t *testing.T) {
	p := &stubProcessor{
		name:     "vue",
		ext:      ".vue",
		supports: true,
		result: TransformResult{
			Text:                  "compiled",
			FilePath:              "Comp.vue",
			VirtualPath:           "Comp.vue.__trace__.ts",
			GeneratedToSourceLine: []int{2},
		},
		err: ErrUnavailable,
	}
	r := NewProcessorRegistry(RegistryConfig{Enabled: true, Mode: ModeAuto, EnableVue: true}, p)
	res, err := r.TransformForTrace(context.Background(), "Comp.vue", "<script>const x=1</script>")
	if err != nil {
		t.Fatalf("TransformForTrace err: %v", err)
	}
	if res.Text != "compiled" {
		t.Fatalf("text=%q want compiled", res.Text)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected fallback warning")
	}
}

func TestRegistryRequireFailsOnUnavailable(t *testing.T) {
	p := &stubProcessor{name: "vue", ext: ".vue", supports: true, err: ErrUnavailable}
	r := NewProcessorRegistry(RegistryConfig{Enabled: true, Mode: ModeRequire, EnableVue: true}, p)
	_, err := r.TransformForEmbedding(context.Background(), "Comp.vue", "x")
	if err == nil {
		t.Fatal("expected error in require mode")
	}
}

func TestRegistryHonorsFrameworkEnableFlags(t *testing.T) {
	p := &stubProcessor{
		name:     "vue",
		ext:      ".vue",
		supports: true,
		result:   TransformResult{Text: "compiled"},
	}
	r := NewProcessorRegistry(RegistryConfig{Enabled: true, Mode: ModeAuto, EnableVue: false}, p)
	res, err := r.TransformForEmbedding(context.Background(), "Comp.vue", "source")
	if err != nil {
		t.Fatalf("TransformForEmbedding err: %v", err)
	}
	if res.Processor != "passthrough" {
		t.Fatalf("processor=%q want passthrough when vue disabled", res.Processor)
	}
}

func TestRegistryRequireFailsForScaffoldProcessor(t *testing.T) {
	r := NewProcessorRegistry(
		RegistryConfig{Enabled: true, Mode: ModeRequire, EnableSvelte: true},
		&SvelteProcessor{},
	)
	_, err := r.TransformForEmbedding(context.Background(), "Comp.svelte", "<script>let x=1</script>")
	if err == nil {
		t.Fatal("expected error for scaffold processor in require mode")
	}
}

func TestVueProcessorFallbackWithMissingNode(t *testing.T) {
	p := NewVueProcessor("node-does-not-exist")
	r := NewProcessorRegistry(RegistryConfig{Enabled: true, Mode: ModeAuto, EnableVue: true}, p)

	source := `<template><div>{{ n }}</div></template>\n<script setup lang="ts">\nconst n = 1\n</script>`
	res, err := r.TransformForTrace(context.Background(), "Comp.vue", source)
	if err != nil {
		t.Fatalf("TransformForTrace err: %v", err)
	}
	if !strings.Contains(res.Text, "const n = 1") {
		t.Fatalf("expected fallback script content, got: %q", res.Text)
	}
	if len(res.GeneratedToSourceLine) == 0 {
		t.Fatal("expected line mapping")
	}
}

func TestScaffoldProcessorsReturnFallback(t *testing.T) {
	r := NewProcessorRegistry(RegistryConfig{Enabled: true, Mode: ModeAuto, EnableSvelte: true, EnableAstro: true}, &SvelteProcessor{}, &AstroProcessor{})
	res, err := r.TransformForEmbedding(context.Background(), "A.svelte", "<script>let n=1</script>")
	if err != nil {
		t.Fatalf("svelte fallback failed: %v", err)
	}
	if res.Processor != "passthrough" {
		t.Fatalf("processor=%q want passthrough", res.Processor)
	}

	res, err = r.TransformForEmbedding(context.Background(), "A.astro", "---\nconst a = 1\n---")
	if err != nil {
		t.Fatalf("astro fallback failed: %v", err)
	}
	if res.Processor != "passthrough" {
		t.Fatalf("processor=%q want passthrough", res.Processor)
	}
}

func TestRemapLineRange(t *testing.T) {
	start, end := RemapLineRange([]int{0, 4, 5, 0}, 1, 4)
	if start != 4 || end != 5 {
		t.Fatalf("range=(%d,%d) want (4,5)", start, end)
	}
}
