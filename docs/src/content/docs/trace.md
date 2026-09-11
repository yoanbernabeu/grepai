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
- **Two extraction modes**: Fast (regex) and Precise (tree-sitter AST)
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

When `--workspace` is specified without `--project`, results are aggregated from all projects. Each project has its own namespaced symbol index, using the configured trace storage backend.

| Flag | Description |
|------|-------------|
| `--workspace` | Workspace name for cross-project trace |
| `--project` | Specific project within the workspace (requires `--workspace`) |

### Extraction Modes

#### Fast Mode (default)

Uses regex patterns for quick symbol extraction. Best for:
- Large codebases where speed matters
- Most common use cases
- No additional dependencies

```bash
grepai trace callers "MyFunction" --mode fast
```

#### Precise Mode

Uses tree-sitter AST parsing for accurate extraction. Best for:
- Complex code patterns
- Edge cases not handled by regex
- When accuracy is critical

```bash
grepai trace callers "MyFunction" --mode precise
```

> **Note**: Precise mode requires building with the `treesitter` build tag and installs CGO dependencies.

### Supported Languages

| Language | Extensions | Extraction Quality |
|----------|------------|-------------------|
| Go | `.go` | Excellent |
| TypeScript | `.ts`, `.tsx` | Excellent |
| JavaScript | `.js`, `.jsx` | Excellent |
| Python | `.py` | Good |
| PHP | `.php` | Good |
| Lua | `.lua` | Good |
| Java | `.java` | Good |
| C | `.c`, `.h` | Good |
| C++ | `.cpp`, `.hpp`, `.cc`, `.cxx`, `.hxx` | Good |
| Zig | `.zig` | Good |
| Rust | `.rs` | Good |
| C# | `.cs` | Good |
| F# | `.fs`, `.fsx`, `.fsi` | Good |
| Pascal/Delphi | `.pas`, `.dpr` | Good |

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
  mode: fast                    # fast | precise

  # Optional. GOB remains the default.
  store_backend: postgres       # gob | postgres
  postgres:
    dsn: postgres://localhost:5432/grepai

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

The Postgres symbol backend is independent of `store.backend`, which controls vector storage. Its DSN is resolved from `trace.postgres.dsn`, then the workspace's `store.postgres.dsn` (in workspace mode), then the project's `store.postgres.dsn`, and finally `postgres://localhost:5432/grepai`.

Symbol storage is pinned to the DSN's first effective schema. Prefer a dedicated database or schema when sharing PostgreSQL with other applications. Initialization checks reserved tables and indexes before changing them, accepts only recognized grepai schema layouts, and rejects ambiguous collisions. Schema creation and upgrades commit atomically; a failure does not leave partially applied schema changes.

Adoption of the recognized original PostgreSQL layout also activates its indexed projects in that transaction. This records that their data came from PostgreSQL, not from a GOB import; existing migration records and orphan-only data are not silently reclassified.

The layout is validated even when its version marker is already current, using catalog reads rather than locking symbol data tables. Missing required indexes in an otherwise valid current layout are repaired under the schema lock; a current marker over incompatible identity types is rejected rather than silently accepted. Library callers must activate a new PostgreSQL project with `Load` before its first mutation; writes and deletes reject missing or incomplete activation markers rather than committing data that a later load cannot recover.

When Postgres is enabled for a project that already has `.grepai/symbols.gob`, grepai imports that index automatically. Migration is serialized with project-scoped Postgres and exclusive GOB file locks, and the complete import commits atomically. An interruption before commit leaves no partial project data and retains the GOB file for retry. After a successful import, the original file is renamed to `.grepai/symbols.gob.migrated.bak`; if commit succeeds but archiving is interrupted, the next startup completes the archive only after verifying the locked file's SHA-256 fingerprint. A different or newly-created GOB file is never archived as though it were the imported snapshot.

Stop the old watcher before switching its symbol backend. Readers without a caller-supplied deadline wait up to 30 seconds for a contending writer to finish migration, then return guidance to stop or restart that watcher. This contention limit does not shorten the actual import after the reader acquires the lock; caller-supplied deadlines still apply.

Only migrate trusted local GOB caches. The fixed cache format is not a resource-hardened interchange format: a malicious or corrupted snapshot can exhaust memory during decoding. Do not import GOB files supplied by an untrusted repository or download; regenerate those caches instead. File locks and recovery fingerprints ensure consistency, not authenticity.

Migration decodes the source GOB in memory, but regroups row values only for the current batch of up to 500 files and releases consumed entries. It does not keep extra whole-index symbol and reference copies. The file-count batch limit is not a fixed byte limit; large individual files and the decoded source still require memory. Batch-wise rescanning trades some CPU time for lower peak memory.

Symbol schema v3 records per-project mutation time in the same transaction as successful file changes. Deleting the newest or final file therefore keeps an accurate freshness timestamp; failed changes and deletion of a nonexistent file do not advance it. Statistics are read from one database snapshot. Upgrades backfill the best available saved-file or activation time, but cannot reconstruct historical deletion times that older schemas never recorded.

Postgres stores project, path, filename, and symbol identity values as raw bytes. This preserves unusual filesystem names exactly; invalid UTF-8 is replaced only in display-oriented text such as signatures, documentation, and reference context.

### How It Works

1. **Symbol Indexing**: During `grepai watch`, symbols (functions, methods, classes) are extracted from source files
2. **Reference Tracking**: Function calls are identified and linked to their callers
3. **Call Graph**: A graph is built mapping caller → callee relationships
   Duplicate caller → callee edges use a stable canonical source location across storage backends.
   PostgreSQL caller results, callee results, read/write reference graphs and complete graph traversals use read-only snapshots so concurrent file updates do not mix generations within a store's result, including its resolved definitions.
4. **Persistent Storage**: Symbols are stored in `.grepai/symbols.gob` by default, or written incrementally to Postgres when configured

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
