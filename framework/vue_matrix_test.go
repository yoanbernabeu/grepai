package framework

import (
	"context"
	"strings"
	"testing"
)

func TestVueProcessorFallback_Matrix(t *testing.T) {
	p := NewVueProcessor("node")

	tests := []struct {
		name         string
		source       string
		wantContains []string
		wantErr      bool
	}{
		{
			name: "script-only",
			source: `<template><div/></template>
<script lang="ts">
export function one() { return 1 }
</script>`,
			wantContains: []string{"export function one"},
		},
		{
			name: "script-lang-js",
			source: `<template><div/></template>
<script lang="js">
export function jsFn() { return 11 }
</script>`,
			wantContains: []string{"export function jsFn"},
		},
		{
			name: "script-no-lang",
			source: `<template><div/></template>
<script>
export function plainFn() { return 12 }
</script>`,
			wantContains: []string{"export function plainFn"},
		},
		{
			name: "script-setup-only",
			source: `<template><div/></template>
<script setup lang="ts">
const count = 1
</script>`,
			wantContains: []string{"const count = 1"},
		},
		{
			name: "script-and-script-setup",
			source: `<template><div/></template>
<script lang="ts">
export function two() { return 2 }
</script>
<script setup lang="ts">
const count = 2
</script>`,
			wantContains: []string{"export function two", "const count = 2"},
		},
		{
			name:    "template-only",
			source:  `<template><button>{{ msg }}</button></template>`,
			wantErr: true,
		},
		{
			name: "style-heavy",
			source: `<template><div class="a">x</div></template>
<style scoped>
.a { color: v-bind(color); background: v-bind(getBg()); }
</style>
<script setup lang="ts">
const msg = 'ok'
const color = '#f00'
function getBg() { return '#fff' }
</script>`,
			wantContains: []string{"const msg = 'ok'", "__css_v_bind__(color)", "__css_v_bind__(getBg())"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := p.fallback("Comp.vue", tt.source)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got result: %+v", res)
				}
				return
			}
			if err != nil {
				t.Fatalf("fallback failed: %v", err)
			}
			for _, s := range tt.wantContains {
				if !strings.Contains(res.Text, s) {
					t.Fatalf("fallback text missing %q in %q", s, res.Text)
				}
			}
			if len(res.GeneratedToSourceLine) == 0 {
				t.Fatal("expected generated->source line map")
			}
		})
	}
}

func TestVueProcessorTransform_CompilerUnavailableFallsBackViaRegistryAuto(t *testing.T) {
	p := NewVueProcessor("node-does-not-exist")
	r := NewProcessorRegistry(RegistryConfig{Enabled: true, Mode: ModeAuto, EnableVue: true}, p)

	source := `<template><div/></template>
<script setup lang="ts">
const value = 123
</script>`

	res, err := r.TransformForEmbedding(context.Background(), "Comp.vue", source)
	if err != nil {
		t.Fatalf("auto mode should not fail on unavailable compiler: %v", err)
	}
	if !strings.Contains(res.Text, "const value = 123") {
		t.Fatalf("expected fallback script content, got: %q", res.Text)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected warning about compiler fallback")
	}
}

func TestVueProcessorTransform_CompilerUnavailableRequireFails(t *testing.T) {
	p := NewVueProcessor("node-does-not-exist")
	r := NewProcessorRegistry(RegistryConfig{Enabled: true, Mode: ModeRequire, EnableVue: true}, p)

	source := `<template><div/></template>
<script setup lang="ts">
const value = 123
</script>`

	_, err := r.TransformForTrace(context.Background(), "Comp.vue", source)
	if err == nil {
		t.Fatal("expected require mode failure when compiler unavailable")
	}
}
