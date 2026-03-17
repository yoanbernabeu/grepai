package framework

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVueProcessorFallback_ExtractsScriptBlocksAndMapsLines(t *testing.T) {
	p := NewVueProcessor("node")
	source := `<template><div>Hello</div></template>

<script lang="ts">
export function a() {
  return 1
}
</script>

<script setup lang="ts">
const b = 2
</script>`

	res, err := p.fallback("Comp.vue", source)
	if err != nil {
		t.Fatalf("fallback failed: %v", err)
	}
	if !strings.Contains(res.Text, "export function a") {
		t.Fatalf("missing first script block in fallback text: %q", res.Text)
	}
	if !strings.Contains(res.Text, "const b = 2") {
		t.Fatalf("missing script setup block in fallback text: %q", res.Text)
	}
	if len(res.GeneratedToSourceLine) == 0 {
		t.Fatal("expected generated->source map")
	}
	if got := RemapLine(res.GeneratedToSourceLine, 1); got < 3 {
		t.Fatalf("expected mapped line to point into script block, got %d", got)
	}
}

func TestExtractStyleVBindRefs(t *testing.T) {
	style := `.a { color: v-bind(color); background: v-bind(getBg()); }
.b { border-color: v-bind("borderColor"); }`
	refs := extractStyleVBindRefs(style, 12)
	if len(refs) != 3 {
		t.Fatalf("expected 3 refs, got %d", len(refs))
	}
	if refs[0].expr != "color" || refs[0].line != 12 {
		t.Fatalf("unexpected first ref: %+v", refs[0])
	}
	if refs[1].expr != "getBg()" || refs[1].line != 12 {
		t.Fatalf("unexpected second ref: %+v", refs[1])
	}
	if refs[2].expr != "borderColor" || refs[2].line != 13 {
		t.Fatalf("unexpected third ref: %+v", refs[2])
	}
}

func TestAppendTemplateCtxReadCalls(t *testing.T) {
	input := "const _s = 1\nreturn _ctx.isAdmin && _ctx.store && _ctx.$slots.default && _ctx._hidden"
	mapping := []int{3, 8}

	out, outMap := appendTemplateCtxReadCalls(input, mapping)
	if !strings.Contains(out, "function __vue_template_reads__() {") {
		t.Fatalf("missing synthetic template caller function: %q", out)
	}
	if !strings.Contains(out, "isAdmin();") {
		t.Fatalf("missing isAdmin synthetic read caller: %q", out)
	}
	if !strings.Contains(out, "store();") {
		t.Fatalf("missing store synthetic read caller: %q", out)
	}
	if strings.Contains(out, "$slots();") || strings.Contains(out, "_hidden();") {
		t.Fatalf("unexpected internal ctx symbols emitted: %q", out)
	}
	if len(outMap) <= len(mapping) {
		t.Fatalf("expected extended line map, got %d <= %d", len(outMap), len(mapping))
	}
}

func TestVueProcessorTransform_ResolvesCompilerFromFileHierarchy(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	nodeModuleDir := filepath.Join(projectDir, "node_modules", "@vue", "compiler-sfc")
	componentPath := filepath.Join(projectDir, "src", "Comp.vue")

	if err := os.MkdirAll(nodeModuleDir, 0o755); err != nil {
		t.Fatalf("mkdir node module dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(componentPath), 0o755); err != nil {
		t.Fatalf("mkdir component dir: %v", err)
	}

	packageJSON := `{"name":"@vue/compiler-sfc","type":"module","exports":"./index.mjs"}`
	if err := os.WriteFile(filepath.Join(nodeModuleDir, "package.json"), []byte(packageJSON), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	moduleSource := `export function parse() {
  return {
    descriptor: {
      script: {
        content: "export const resolved = 1",
        loc: { start: { line: 1 } }
      },
      scriptSetup: null,
      template: null,
      styles: []
    },
    errors: []
  };
}

export function compileTemplate() {
  return { code: "", errors: [] };
}`
	if err := os.WriteFile(filepath.Join(nodeModuleDir, "index.mjs"), []byte(moduleSource), 0o644); err != nil {
		t.Fatalf("write compiler module: %v", err)
	}

	p := NewVueProcessor("node")
	res, err := p.TransformForEmbedding(context.Background(), componentPath, "<template><div/></template>")
	if err != nil {
		t.Fatalf("transform failed: %v", err)
	}
	if !strings.Contains(res.Text, "export const resolved = 1") {
		t.Fatalf("expected resolved compiler output, got %q", res.Text)
	}
}
