#!/usr/bin/env bash
set -euo pipefail

ROOT="/Users/mladenmihajlovic/Documents/git/github/grepai"
BIN="$ROOT/bin/grepai"

cd "$ROOT"

echo "[0/9] Ensure grepai binary exists..."
if [ ! -x "$BIN" ]; then
  echo "Building $BIN ..."
  make build
fi
"$BIN" version

echo "[1/9] Install Vue compiler (dev dep)..."
echo "  (will install into temp project after creation)"

echo "[2/9] Create test Vue file..."
rm -rf /tmp/grepai-vue-test
mkdir -p /tmp/grepai-vue-test/src
cat > /tmp/grepai-vue-test/src/UserUtils.vue <<'VUE'
<template>
  <span class="hidden">{{ getGreeting("sample") }}</span>
</template>
<script lang="ts">
export function formatUserName(name: string): string {
  return name.trim().toUpperCase()
}

export function getGreeting(name: string): string {
  return `Welcome ${formatUserName(name)}`
}
</script>
VUE

cat > /tmp/grepai-vue-test/src/ActivityPanel.vue <<'VUE'
<template>
  <section>{{ buildActivityLabel("alice", 3) }}</section>
</template>
<script lang="ts">
import { formatUserName } from "./UserUtils.vue"

export function buildActivityLabel(name: string, count: number): string {
  return formatUserName(name) + " has " + String(count) + " new alerts"
}
</script>
VUE

cat > /tmp/grepai-vue-test/src/Dashboard.vue <<'VUE'
<template>
  <main>
    <h1>{{ buildHeader("alice") }}</h1>
    <ActivityPanel />
  </main>
</template>
<script lang="ts">
import ActivityPanel from "./ActivityPanel.vue"
import { getGreeting } from "./UserUtils.vue"

export function buildHeader(name: string): string {
  return getGreeting(name)
}
</script>
VUE

cat > /tmp/grepai-vue-test/src/App.vue <<'VUE'
<template>
  <Dashboard />
</template>
<script lang="ts">
import Dashboard from "./Dashboard.vue"

export function mountApp(): string {
  return "mounted"
}
</script>
VUE

( cd /tmp/grepai-vue-test && npm init -y >/dev/null 2>&1 && npm install -D @vue/compiler-sfc >/dev/null )

echo "[3/9] Ensure grepai config exists..."
( cd /tmp/grepai-vue-test && "$BIN" init --yes >/dev/null 2>&1 || true )

CFG="/tmp/grepai-vue-test/.grepai/config.yaml"
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

# Ensure Vue is included in traced languages for symbol extraction.
if ! grep -q "\.vue" "$CFG"; then
  if grep -q '    - ".tsx"' "$CFG"; then
    sed -i '' 's/    - ".tsx"/    - ".tsx"\
    - ".vue"/' "$CFG"
  else
    cat >> "$CFG" <<'YAML'

trace:
  enabled_languages:
    - ".vue"
YAML
  fi
fi

echo "[4/9] Run watch briefly for initial index..."
( cd /tmp/grepai-vue-test && timeout 18s "$BIN" watch >/tmp/grepai-watch.log 2>&1 || true )

echo "[5/9] Run search..."
( cd /tmp/grepai-vue-test && "$BIN" search "user greeting and activity label formatting" --json ) | tee /tmp/grepai-search.json

echo "[6/9] Check search includes multiple Vue files..."
grep -q "UserUtils.vue" /tmp/grepai-search.json
grep -q "ActivityPanel.vue" /tmp/grepai-search.json
grep -q "Dashboard.vue" /tmp/grepai-search.json
echo "OK: search returned multiple Vue files."

echo "[7/9] Run trace..."
( cd /tmp/grepai-vue-test && "$BIN" trace callers formatUserName --json ) | tee /tmp/grepai-trace-callers.json || true
( cd /tmp/grepai-vue-test && "$BIN" trace callees buildHeader --json ) | tee /tmp/grepai-trace-callees.json || true
grep -q "formatUserName" /tmp/grepai-trace-callers.json
grep -q "buildActivityLabel" /tmp/grepai-trace-callers.json
grep -q "buildHeader" /tmp/grepai-trace-callees.json
grep -q "getGreeting" /tmp/grepai-trace-callees.json
echo "OK: trace captured cross-file function relationships."

echo "[8/9] Optional fallback test (remove compiler, keep auto mode)..."
( cd /tmp/grepai-vue-test && npm uninstall @vue/compiler-sfc >/dev/null ) || true
( cd /tmp/grepai-vue-test && timeout 8s "$BIN" watch >/tmp/grepai-watch-fallback.log 2>&1 || true )

echo "[9/9] Done."
echo "Artifacts:"
echo "  /tmp/grepai-search.json"
echo "  /tmp/grepai-trace-callers.json"
echo "  /tmp/grepai-trace-callees.json"
echo "  /tmp/grepai-watch.log"
echo "  /tmp/grepai-watch-fallback.log"
