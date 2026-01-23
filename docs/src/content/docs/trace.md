---
title: Call Graph Analysis
description: Analyze function relationships with grepai trace
---

## Call Graph Analysis

`grepai trace` provides call graph analysis for your codebase, allowing you to understand how functions relate to each other by tracking callers and callees.

### Features

- **Find callers**: Discover which functions call a specific symbol
- **Find callees**: See what functions a symbol calls
- **Build call graphs**: Visualize call relationships with configurable depth
- **Multi-language support**: Go, TypeScript/JavaScript, Python, PHP, Java, C/C++, Rust, Zig
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
| Java | `.java` | Good |
| C | `.c`, `.h` | Good |
| C++ | `.cpp`, `.hpp`, `.cc`, `.cxx`, `.hxx` | Good |
| Zig | `.zig` | Good |
| Rust | `.rs` | Good |
| C# | `.cs` | Good |
| Pascal/Delphi | `.pas`, `.dpr` | Good |

### JSON Output

For AI agents and scripts, use `--json` flag:

```bash
grepai trace callers "Login" --json
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
  enabled_languages:
    - .go
    - .js
    - .ts
    - .jsx
    - .tsx
    - .py
    - .php
    - .java
    - .c
    - .h
    - .cpp
    - .hpp
    - .cc
    - .cxx
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
