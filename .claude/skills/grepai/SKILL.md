---
name: grepai
description: "MANDATORY: Replaces Grep/Glob for semantic code searches. You MUST invoke this skill BEFORE using Grep or Glob for intent-based searches. Use `grepai search` for natural language queries and `grepai trace` for call graph analysis."
---

## CRITICAL: Tool Override for Semantic Searches

This skill **REPLACES** built-in Grep/Glob tools for semantic code exploration.

**WRONG**:
- Using built-in `Grep` to find "where authentication happens"
- Using built-in `Glob` to explore "error handling code"
- Searching by intent with regex patterns

**CORRECT**:
- Invoke this skill, then use `grepai search "authentication flow"` for semantic search
- Invoke this skill, then use `grepai trace callers "FunctionName"` for call graph
- Use built-in Grep/Glob ONLY for exact text matches (variable names, imports)

## When to Invoke This Skill

Invoke this skill **IMMEDIATELY** when:

- User asks to find code by **intent** (e.g., "where is authentication handled?")
- User asks to understand **what code does** (e.g., "how does the indexer work?")
- User asks to explore **functionality** (e.g., "find error handling logic")
- You need to understand **code relationships** (e.g., "what calls this function?")
- User asks about **implementation details** (e.g., "how are vectors stored?")

**DO NOT** use built-in Grep/Glob for intent-based searches. Use grepai instead.

## When to Use Built-in Tools

Use Grep/Glob **ONLY** for:

- Exact text matching: `Grep "func NewIndexer"` (find exact function name)
- Specific imports: `Grep "import.*cobra"` (find import statements)
- File patterns: `Glob "**/*.go"` (find files by extension)
- Variable references: `Grep "configPath"` (find exact variable name)

## How to Use This Skill

### Semantic Search

Use `grepai search` to find code by **describing what it does**:

```bash
# Search with natural language (ALWAYS use English for best results)
grepai search "user authentication flow"
grepai search "error handling middleware"
grepai search "database connection pooling"
grepai search "API request validation"

# JSON output for programmatic use (recommended)
grepai search "authentication flow" --json

# Limit results
grepai search "error handling" -n 5
```

### Call Graph Tracing

Use `grepai trace` to understand **function relationships**:

```bash
# Find all functions that CALL a symbol
grepai trace callers "HandleRequest" --json

# Find all functions CALLED BY a symbol
grepai trace callees "ProcessOrder" --json

# Build complete call graph (both directions)
grepai trace graph "ValidateToken" --depth 3 --json
```

### Query Best Practices

**Do:**
```bash
grepai search "How are file chunks created and stored?"
grepai search "Vector embedding generation process"
grepai search "Configuration loading and validation"
grepai trace callers "Search" --json
```

**Don't:**
```bash
grepai search "func"           # Too vague
grepai search "error"          # Too generic
grepai search "HandleRequest"  # Use Grep for exact matches
```

## Recommended Workflow

1. **Start with `grepai search`** to find relevant code semantically
2. **Use `grepai trace`** to understand function relationships
3. **Use `Read` tool** to examine files from search results
4. **Use `Grep`** only for exact string searches if needed

## Fallback

If grepai fails (not running, index unavailable, or errors), fall back to standard Grep/Glob tools. Common issues:

- Index not built: Run `grepai watch` to build/update the index
- Embedder not available: Check that Ollama is running or OpenAI API key is set

## Keywords

semantic search, code search, natural language search, find code, explore codebase,
call graph, callers, callees, function relationships, code understanding,
intent search, grep replacement, code exploration
