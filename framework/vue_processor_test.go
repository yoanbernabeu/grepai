package framework

import (
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
