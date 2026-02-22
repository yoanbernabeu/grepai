#!/usr/bin/env bash
set -euo pipefail

ROOT="/Users/mladenmihajlovic/Documents/git/github/grepai"
BIN="$ROOT/bin/grepai"
TEST_ROOT="/tmp/grepai-vue-matrix"

cd "$ROOT"

echo "[0/8] Ensure binary exists"
[ -x "$BIN" ] || make build
"$BIN" version

echo "[1/8] Install compiler"
HAVE_COMPILER=1
echo "  (will install into temp project after creation)"

echo "[2/8] Create matrix project"
rm -rf "$TEST_ROOT"
mkdir -p "$TEST_ROOT/src"

cat > "$TEST_ROOT/src/ScriptOnly.vue" <<'VUE'
<template><div/></template>
<script lang="ts">
export function scriptOnlyFn() { return 1 }
</script>
VUE

cat > "$TEST_ROOT/src/ScriptJS.vue" <<'VUE'
<template><div/></template>
<script lang="js">
export function scriptJsFn() { return 11 }
</script>
VUE

cat > "$TEST_ROOT/src/ScriptNoLang.vue" <<'VUE'
<template><div/></template>
<script>
export function scriptNoLangFn() { return 12 }
</script>
VUE

cat > "$TEST_ROOT/src/ScriptSetupOnly.vue" <<'VUE'
<template><div>{{ count }}</div></template>
<script setup lang="ts">
const count = 2
function setupOnlyFn() { return count }
</script>
VUE

cat > "$TEST_ROOT/src/Mixed.vue" <<'VUE'
<template><div>{{ runMixed() }}</div></template>
<script lang="ts">
export function helperMixed() { return 3 }
</script>
<script setup lang="ts">
function runMixed() { return helperMixed() }
</script>
VUE

cat > "$TEST_ROOT/src/TemplateOnly.vue" <<'VUE'
<template><button>{{ msg }}</button></template>
VUE

cat > "$TEST_ROOT/src/StyleHeavy.vue" <<'VUE'
<template><div class="card">{{ styledFn() }}</div></template>
<style scoped>
.card {
  color: v-bind(colorVar);
  background: v-bind(styledFn());
}
</style>
<script lang="ts">
const colorVar = "#f00"
export function styledFn() { return "ok" }
</script>
VUE

if ! ( cd "$TEST_ROOT" && npm init -y >/dev/null 2>&1 && timeout 60s npm install -D @vue/compiler-sfc >/dev/null 2>&1 ); then
  HAVE_COMPILER=0
  echo "WARN: compiler install failed or timed out; continuing in fallback-only mode"
fi

( cd "$TEST_ROOT" && "$BIN" init --yes >/dev/null 2>&1 || true )
CFG="$TEST_ROOT/.grepai/config.yaml"

if ! grep -q "framework_processing:" "$CFG"; then
  cat >> "$CFG" <<'YAML'

framework_processing:
  enabled: true
  mode: auto
  node_path: node
  frameworks:
    vue:
      enabled: true
    svelte:
      enabled: false
    astro:
      enabled: false
    solid:
      enabled: false
YAML
fi

if ! grep -q "\.vue" "$CFG"; then
  sed -i '' 's/    - ".tsx"/    - ".tsx"\
    - ".vue"/' "$CFG"
fi

echo "[3/8] Index with compiler/fallback path"
( cd "$TEST_ROOT" && timeout 20s "$BIN" watch > /tmp/grepai-vue-matrix-watch.log 2>&1 || true )

echo "[4/8] Search assertions"
( cd "$TEST_ROOT" && "$BIN" search "mixed helper and styled function" --json ) > /tmp/grepai-vue-matrix-search.json
for f in ScriptOnly.vue ScriptSetupOnly.vue Mixed.vue StyleHeavy.vue; do
  grep -q "$f" /tmp/grepai-vue-matrix-search.json
  echo "  - found $f"
done
for f in ScriptJS.vue ScriptNoLang.vue; do
  grep -q "$f" /tmp/grepai-vue-matrix-search.json
  echo "  - found $f"
done

echo "[5/8] Trace assertions"
( cd "$TEST_ROOT" && "$BIN" trace callees runMixed --json ) > /tmp/grepai-vue-matrix-trace-callees.json || true
grep -q "helperMixed" /tmp/grepai-vue-matrix-trace-callees.json

( cd "$TEST_ROOT" && "$BIN" trace callers styledFn --json ) > /tmp/grepai-vue-matrix-trace-callers-style.json || true
grep -q "__vue_style_bindings__" /tmp/grepai-vue-matrix-trace-callers-style.json
echo "  - style v-bind synthetic caller detected for styledFn"

( cd "$TEST_ROOT" && "$BIN" trace callers scriptJsFn --json ) > /tmp/grepai-vue-matrix-trace-callers-js.json || true
grep -q "scriptJsFn" /tmp/grepai-vue-matrix-trace-callers-js.json
( cd "$TEST_ROOT" && "$BIN" trace callers scriptNoLangFn --json ) > /tmp/grepai-vue-matrix-trace-callers-nolang.json || true
grep -q "scriptNoLangFn" /tmp/grepai-vue-matrix-trace-callers-nolang.json
echo "  - JS and no-lang script symbols detected"

echo "[6/8] Fallback mode run"
if [ "$HAVE_COMPILER" -eq 1 ]; then
  ( cd "$TEST_ROOT" && npm uninstall @vue/compiler-sfc >/dev/null ) || true
fi
( cd "$TEST_ROOT" && timeout 12s "$BIN" watch > /tmp/grepai-vue-matrix-watch-fallback.log 2>&1 || true )

echo "[7/8] Fallback still searchable"
( cd "$TEST_ROOT" && "$BIN" search "setup only function" --json ) > /tmp/grepai-vue-matrix-search-fallback.json
grep -q "ScriptSetupOnly.vue" /tmp/grepai-vue-matrix-search-fallback.json

echo "[8/8] Done"
echo "Artifacts:"
echo "  /tmp/grepai-vue-matrix-watch.log"
echo "  /tmp/grepai-vue-matrix-search.json"
echo "  /tmp/grepai-vue-matrix-trace-callees.json"
echo "  /tmp/grepai-vue-matrix-trace-callers-style.json"
echo "  /tmp/grepai-vue-matrix-trace-callers-js.json"
echo "  /tmp/grepai-vue-matrix-trace-callers-nolang.json"
echo "  /tmp/grepai-vue-matrix-watch-fallback.log"
echo "  /tmp/grepai-vue-matrix-search-fallback.json"
