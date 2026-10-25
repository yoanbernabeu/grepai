---
title: Call Graph Analysis
description: Find who calls a function, what it calls, and where JS/TS properties are read or written, with grepai trace and grepai refs.
---

## Call Graph Analysis

`grepai trace` answers two questions about a function or method: who calls it, and what does it call. It reads a local symbol index, so queries never call your embedding provider.

### Quick Start

Build the index once (or keep `grepai watch` running), then query it:

```bash
# Build or update the symbol index
grepai index            # one-time run, or: grepai watch

# Who calls Login?
grepai trace callers "Login"

# What does HandleRequest call?
grepai trace callees "HandleRequest"

# Callers and callees around ProcessOrder, 3 levels deep
grepai trace graph "ProcessOrder" --depth 3
```

Example output of `grepai trace callers "Login"`:

```text
Symbol: Login (function)
File: auth/service.go:12

Callers (1):
------------------------------------------------------------

1. HandleAuth
   Defined: handlers/auth.go:30
   Calls at: handlers/auth.go:42
   Context: user.Login(ctx, credentials)
```

If the symbol index is empty, the command fails with `symbol index is empty. Run 'grepai watch' first to build the index`.

### Features

- **`trace callers`**: functions that call a symbol.
- **`trace callees`**: functions a symbol calls.
- **`trace graph`**: a call graph around a symbol, with a configurable `--depth` (default `2`).
- **`refs readers|writers|graph`**: where a JavaScript/TypeScript property is read or written (see [Trace vs Refs](#trace-vs-refs)).
- **Output formats**: text (default), `--json`, `--toon`, and `--ui` for an interactive view (trace only).
- **Workspace support**: query one project or every project of a workspace.
- **RPG feature paths**: when the [Repository Planning Graph](/grepai/rpg/) is enabled, each symbol also shows its feature path.

### Trace vs Refs

Use `trace` for call relationships. Use `refs` for data access: places where a property is read or assigned without a function call (for example `store.uid` in Vue or Pinia code).

```bash
# Call graph (functions and methods)
grepai trace callers "isAdmin"

# Property usage (reads and writes)
grepai refs readers "uid"
grepai refs writers "uid"
grepai refs graph "uid"     # readers and writers together
```

### Property References (refs)

`grepai refs` uses the same symbol index as `trace`. Read and write references are only extracted from **JavaScript and TypeScript** (`.js`, `.jsx`, `.ts`, `.tsx`) and from Vue single-file components (`.vue`), which are converted to TypeScript before extraction. Other languages produce call references only, so `refs` returns nothing for them.

What counts as an access:

| Pattern | Example | Recorded as |
|---------|---------|-------------|
| Member access | `store.uid`, `this.user.uid` | read of `uid` |
| Member assignment | `store.uid = id` | write of `uid` |
| Bracket access with a string key | `store["uid"]`, `store['uid'] = id` | read or write of `uid` |
| Destructuring from `storeToRefs()` or `toRefs()` | `const { uid } = storeToRefs(store)`, then `uid.value` | read (or write when assigned or `++`/`--`) |
| Simple alias | `const uid = store.uid`, then `uid` | read (or write when assigned) |

Accesses on built-in roots (`Math`, `JSON`, `console`, `window`, `document`, and similar), names starting with `_`, and Vue internals such as `$el` or `$refs` are ignored. An access outside any function is attributed to `<top-level>`.

Example output of `grepai refs readers "uid"`:

```text
Symbol: uid (property)

Readers (1):
------------------------------------------------------------

1. useAuth
   Defined: src/composables/useAuth.ts:5
   Access: read at src/composables/useAuth.ts:9
   Context: return store.uid
```

`refs` supports `--json`, `--toon`, `--compact`, `--workspace` and `--project`. It has no `--ui` or `--mode` flag.

### Workspace Mode

Both `trace` and `refs` can query the projects of a [workspace](/grepai/workspace/):

```bash
# One project of the workspace
grepai trace callers "HandleRequest" --workspace my-fullstack --project backend

# All projects of the workspace
grepai trace callers "HandleRequest" --workspace my-fullstack

# Cross-project call graph
grepai trace graph "ProcessOrder" --workspace my-fullstack --depth 3

# Property usage across projects
grepai refs writers "uid" --workspace my-fullstack
```

| Flag | Description |
|------|-------------|
| `--workspace` | Workspace name. Without `--project`, results from all projects are merged. |
| `--project` | Limit to one project of the workspace (requires `--workspace`). |

Each project keeps its own symbol index in `<project>/.grepai/symbols.gob`, whatever vector store the workspace uses. `grepai watch --workspace` and `grepai index --workspace` build these indexes. In workspace mode, results are not enriched with RPG feature paths.

### Supported Languages

| Language | Extensions |
|----------|------------|
| Go | `.go` |
| JavaScript | `.js`, `.jsx` |
| TypeScript | `.ts`, `.tsx` |
| Vue SFC | `.vue` (see note below) |
| Python | `.py` |
| PHP | `.php` |
| Lua | `.lua` |
| Java | `.java` |
| C | `.c`, `.h` |
| C++ | `.cpp`, `.hpp`, `.cc`, `.cxx`, `.hxx` |
| Rust | `.rs` |
| Zig | `.zig` |
| C# | `.cs` |
| F# | `.fs`, `.fsx`, `.fsi` |
| Pascal/Delphi | `.pas`, `.dpr` |

Extracted symbol kinds are functions, methods, classes, interfaces and types.

> **Note:** `.vue` files go through the Vue framework processor (`framework.frameworks.vue.enabled`, on by default). It converts the script and template to TypeScript, then extracts symbols and references with the TypeScript rules. Full compilation needs Node.js and `@vue/compiler-sfc` installed in the Vue project. Without them, grepai logs a warning and falls back to the `<script>` blocks.

Only extensions listed in `trace.enabled_languages` are indexed. A config created by an older version may lack newer entries such as `.vue` or the F# extensions: add them by hand if needed.

### Extraction Modes

#### Fast Mode (default)

Symbols and references are extracted with regular expressions, one pattern set per language. This is the extractor that `grepai watch` and `grepai index` use, and it needs no extra dependency.

#### Precise Mode

> **Note:** precise mode was never wired into indexing. `grepai watch` and `grepai index` always use the regex extractor. The `--mode` flag of `trace callers|callees|graph` is deprecated: it is still accepted so existing scripts keep working, prints a deprecation warning, and the output always reports `"mode": "fast"`.

A tree-sitter extractor exists in the source tree behind the `treesitter` build tag (CGO required) for Go, JavaScript, TypeScript, Python, PHP, C# and F#. Release binaries are built without it.

### JSON Output

Use `--json` or `--toon` (token-efficient) for scripts and AI agents. Add `--compact` to drop the `context` line of each call site (or access site for `refs`). `--compact` requires `--json` or `--toon`; `--json`, `--toon` and `--ui` are mutually exclusive.

```bash
grepai trace callers "Login" --json
grepai trace callers "Login" --json --compact
grepai refs readers "uid" --toon --compact
```

`trace callers` output (`callees` has the same shape with a `callees` array):

```json
{
  "query": "Login",
  "mode": "fast",
  "symbol": {
    "name": "Login",
    "kind": "function",
    "file": "auth/service.go",
    "line": 12,
    "signature": "func Login(ctx context.Context, creds Credentials) (*User, error) {",
    "exported": true,
    "language": "go"
  },
  "callers": [
    {
      "symbol": {
        "name": "HandleAuth",
        "kind": "function",
        "file": "handlers/auth.go",
        "line": 30,
        "signature": "func HandleAuth(w http.ResponseWriter, r *http.Request) {",
        "exported": true,
        "language": "go"
      },
      "call_site": {
        "file": "handlers/auth.go",
        "line": 42,
        "context": "user.Login(ctx, credentials)"
      }
    }
  ]
}
```

Symbols can also carry `receiver` (methods) and `feature_path` (when RPG is enabled). `trace graph` returns a `graph` object instead, with `root`, `nodes` (symbols keyed by name), `edges` (`caller`, `callee`, `file`, `line`) and `depth`.

`refs` output:

```json
{
  "query": "uid",
  "kind": "property",
  "mode": "fast",
  "readers": [
    {
      "symbol": { "name": "useAuth", "kind": "function", "file": "src/composables/useAuth.ts", "line": 5, "language": "typescript" },
      "access": "read",
      "access_at": { "file": "src/composables/useAuth.ts", "line": 9, "context": "return store.uid" }
    }
  ]
}
```

`refs writers` returns a `writers` array, and `refs graph` returns both arrays.

### Configuration

The `trace` section of `.grepai/config.yaml` (defaults shown):

```yaml
trace:
  enabled_languages:
    - .go
    - .js
    - .ts
    - .jsx
    - .tsx
    - .vue
    - .py
    - .php
    - .lua
    - .c
    - .h
    - .cpp
    - .hpp
    - .cc
    - .cxx
    - .hxx
    - .rs
    - .zig
    - .cs
    - .java
    - .fs
    - .fsx
    - .fsi
    - .pas
    - .dpr
```

| Key | Effect |
|-----|--------|
| `enabled_languages` | File extensions to extract symbols from. An empty list falls back to the defaults. |

To keep files out of every index, including the symbol index, use the top-level `ignore` list. Older configs may contain `trace.mode` or `trace.exclude_patterns`: these keys never had any effect and were removed, and grepai ignores them.

### How It Works

1. **Symbol indexing**: `grepai watch` (initial scan plus file events) and `grepai index` extract symbol definitions from every file whose extension is in `trace.enabled_languages`.
2. **Reference tracking**: each call (and, for JS/TS, each property read or write) is recorded with the function that contains it.
3. **Call graph**: caller to callee edges are derived from those references.
4. **Storage**: everything is saved in `.grepai/symbols.gob`. Unchanged files are skipped on the next run.

### Use Cases

#### Understanding Code Flow

```bash
# Where is this function used?
grepai trace callers "ValidateToken"

# What does this function depend on?
grepai trace callees "ProcessPayment"
```

#### Impact Analysis

```bash
# Dependency chain around a critical function
grepai trace graph "DatabaseConnect" --depth 4

# Every place that assigns a store field before you rename it
grepai refs writers "uid"
```

#### AI Agent Integration

```bash
grepai trace graph "AuthMiddleware" --depth 2 --json
```

The same features are exposed to MCP clients as `grepai_trace_callers`, `grepai_trace_callees`, `grepai_trace_graph`, `grepai_refs_readers`, `grepai_refs_writers` and `grepai_refs_graph` (see [MCP Integration](/grepai/mcp/)).

### Commands Reference

- [`grepai trace callers`](/grepai/commands/grepai_trace_callers/): find functions that call a symbol
- [`grepai trace callees`](/grepai/commands/grepai_trace_callees/): find functions called by a symbol
- [`grepai trace graph`](/grepai/commands/grepai_trace_graph/): build a call graph
- [`grepai refs readers`](/grepai/commands/grepai_refs_readers/): find property readers
- [`grepai refs writers`](/grepai/commands/grepai_refs_writers/): find property writers
- [`grepai refs graph`](/grepai/commands/grepai_refs_graph/): readers and writers together
