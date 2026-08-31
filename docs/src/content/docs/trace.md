---
title: Call Graph Analysis
description: Analyze function relationships with grepai trace
---

## Call Graph Analysis

`grepai trace` provides call graph analysis for your codebase, allowing you to understand how functions relate to each other by tracking callers and callees.

### Trace vs Refs

Use `trace` for call relationships, and `refs` for property/state usage.

```bash
# Call graph (functions/methods)
grepai trace callers "isAdmin"

# Property/state usage (reads/writes)
grepai refs readers "uid"
grepai refs writers "uid"
grepai refs graph "uid"
```

`grepai refs` is especially useful in Vue/Pinia code where state keys (for example `store.uid`) are read and written without direct function calls.

### Features

- **Find callers**: Discover which functions call a specific symbol
- **Find callees**: See what functions a symbol calls
- **Build call graphs**: Visualize call relationships with configurable depth
- **Multi-language support**: Go, TypeScript/JavaScript, Python, PHP, Java, C/C++, Rust, Zig, C#, F#
- **Three extraction modes**: Auto (tree-sitter where available, default), Fast (regex) and Precise (tree-sitter AST)
- **JSON output**: Perfect for AI agents and automation

### Quick Start

```bash
# Ensure watch is running to index symbols
grepai watch

# Find all functions that call "Login"
grepai trace callers "Login"

# Find all functions called by "HandleRequest"
grepai trace callees "HandleRequest"

# Build a call graph with depth 3
grepai trace graph "ProcessOrder" --depth 3
```

### Workspace Mode

Trace commands support cross-project analysis in workspace mode:

```bash
# Trace within a specific project
grepai trace callers "HandleRequest" --workspace my-fullstack --project backend

# Trace across ALL projects in a workspace
grepai trace callers "HandleRequest" --workspace my-fullstack

# Cross-project call graph
grepai trace graph "ProcessOrder" --workspace my-fullstack --depth 3
```

When `--workspace` is specified without `--project`, results are aggregated from all projects. Each project maintains its own symbol index in `.grepai/symbols.gob`, regardless of the workspace's vector store backend.

| Flag | Description |
|------|-------------|
| `--workspace` | Workspace name for cross-project trace |
| `--project` | Specific project within the workspace (requires `--workspace`) |

### Extraction Modes

#### Auto Mode (default)

Uses tree-sitter where a grammar is compiled in for the file's extension and
falls back to regex for everything else. This is what you want unless you have
a specific reason not to.

```bash
grepai trace callers "MyFunction" --mode auto
```

#### Fast Mode

Forces regex extraction for every file. Best for:
- Deterministic per-file timing
- Working around a misbehaving grammar

```bash
grepai trace callers "MyFunction" --mode fast
```

#### Precise Mode

Forces tree-sitter AST parsing. Files whose extension has no compiled-in
grammar are skipped with a warning. Best for:
- Complex code patterns
- Edge cases not handled by regex
- When accuracy is critical

```bash
grepai trace callers "MyFunction" --mode precise
```

#### Mode is normally a `watch`-time setting

`grepai trace` answers from the symbol index that `grepai watch` built, and
that index contains whatever the extractor configured at watch time produced.
The mode that matters, therefore, is `trace.mode` in `.grepai/config.yaml`:

```yaml
trace:
  mode: auto
```

Passing `--mode` to a trace command still works, and it is honest about it: if
the requested mode differs from the one the index was built with, grepai
re-extracts the project on the spot with the requested extractor and answers
from that. It prints a note to stderr when it does, because the cost is
proportional to the size of the repository:

```console
$ grepai trace callers validate --mode precise
Note: index was built in "fast" mode; re-extracting /path/to/repo in "precise" mode
      (set trace.mode and re-run `grepai watch` to make this permanent)
```

When `--mode` is omitted, or when it names the mode the index already used,
nothing is re-extracted. The `mode` field in `--json` output always names the
extractor the answer actually came from, not the flag you passed.

### Supported Languages

Every extension below is extracted by default — `trace.enabled_languages` in
`.grepai/config.yaml` ships covering all of them. "Call graph" says whether
`trace callers` / `trace callees` work for the language, or whether it gets
symbol extraction only.

| Language | Extensions | Symbols | Call graph |
|----------|------------|---------|------------|
| Go | `.go` | tree-sitter | yes |
| TypeScript | `.ts`, `.tsx`, `.mts`, `.cts` | tree-sitter | yes |
| JavaScript | `.js`, `.jsx`, `.mjs`, `.cjs` | tree-sitter | yes |
| Python | `.py` | tree-sitter | yes |
| PHP | `.php` | tree-sitter | yes |
| C# | `.cs` | tree-sitter | yes |
| F# | `.fs`, `.fsx`, `.fsi` | tree-sitter | yes |
| Ruby | `.rb` | tree-sitter | yes (parenthesized or receiver calls only) |
| Rust | `.rs` | tree-sitter | yes |
| Java | `.java` | tree-sitter | yes |
| Scala | `.scala`, `.sc`, `.mill` | tree-sitter | yes |
| C | `.c`, `.h` | tree-sitter | yes |
| C++ | `.cpp`, `.cc`, `.cxx`, `.hpp`, `.hh`, `.hxx` | tree-sitter | yes |
| Bash | `.sh`, `.bash`, `.zsh` | tree-sitter | yes (every command counts as a call) |
| Lua | `.lua` | tree-sitter | yes |
| Kotlin | `.kt`, `.kts` | tree-sitter | yes |
| Swift | `.swift` | tree-sitter | yes |
| SQL | `.sql` | tree-sitter | no |
| Protobuf | `.proto` | tree-sitter | no |
| HCL / Terraform | `.hcl`, `.tf` | tree-sitter | no |
| Elm | `.elm` | tree-sitter | no |
| TOML | `.toml` | tree-sitter | no |
| Emacs Lisp | `.el` | tree-sitter | no |
| Vue | `.vue` | regex | no |
| Zig | `.zig` | regex | limited |
| Pascal/Delphi | `.pas`, `.dpr` | regex | limited |

Languages marked "no" for the call graph are either declarative (SQL, HCL,
Protobuf, TOML, Elm) or, in the case of Emacs Lisp, use a syntax where a call
is indistinguishable from `if`/`let`/`when` — extracting them would be mostly
false positives. They still contribute symbol definitions, so `grepai search`
and symbol lookup work.

### JSON Output

For AI agents and scripts, use `--json` or `--toon`. Add `--compact` to omit verbose context fields while keeping symbol, file, and line data:

```bash
grepai trace callers "Login" --json
grepai trace callers "Login" --json --compact
grepai refs readers "uid" --toon --compact
```

Output format:

```json
{
  "query": "Login",
  "mode": "callers",
  "count": 3,
  "results": [
    {
      "file": "handlers/auth.go",
      "line": 42,
      "caller": "HandleAuth",
      "context": "user.Login(ctx, credentials)"
    }
  ]
}
```

### Configuration

Configure trace behavior in `.grepai/config.yaml`:

```yaml
trace:
  mode: auto                    # auto | fast | precise
  enabled_languages:
    - .go
    - .js
    - .ts
    - .jsx
    - .tsx
    - .py
    - .php
    - .lua
    - .java
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
    - .pas
    - .dpr
  exclude_patterns:
    - "*_test.go"
    - "*.spec.ts"
```

### How It Works

1. **Symbol Indexing**: During `grepai watch`, symbols (functions, methods, classes) are extracted from source files
2. **Reference Tracking**: Function calls are identified and linked to their callers
3. **Call Graph**: A graph is built mapping caller → callee relationships
4. **Persistent Storage**: Symbols are stored in `.grepai/symbols.gob`

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
# Full dependency chain for a critical function
grepai trace graph "DatabaseConnect" --depth 4
```

#### AI Agent Integration

Provide call graph context to AI agents:

```bash
# Get JSON for AI processing
grepai trace graph "AuthMiddleware" --depth 2 --json
```

### Commands Reference

- [`grepai trace callers`](/grepai/commands/grepai_trace_callers/) - Find functions that call a symbol
- [`grepai trace callees`](/grepai/commands/grepai_trace_callees/) - Find functions called by a symbol
- [`grepai trace graph`](/grepai/commands/grepai_trace_graph/) - Build complete call graph
- [`grepai refs readers`](/grepai/commands/grepai_refs_readers/) - Find property/state readers
- [`grepai refs writers`](/grepai/commands/grepai_refs_writers/) - Find property/state writers
- [`grepai refs graph`](/grepai/commands/grepai_refs_graph/) - Build property usage graph
