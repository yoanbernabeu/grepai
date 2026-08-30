---
name: grepai
description: "Semantic code search and call-graph tracing. Use for intent questions ('where is authentication handled?'), exploring unfamiliar code, or tracing callers/callees — as a ranking layer on top of exact-match Grep, not a replacement for it."
---

## grepai: ranked semantic search (not a Grep replacement)

`grepai search` answers a natural-language query with ~10 scored chunks in one
call — a ranked starting point at a fraction of the tokens of dumping raw grep
output (`--json --compact` alone saves ~80%). It is a **ranking layer, not an
exhaustive one**: a vector top-10 can miss a relevant file that keyword grep
finds trivially. The rules below save tokens without losing recall.

## Tool Choice

| Query | Tool |
|---|---|
| Exact identifiers, imports, string literals | built-in Grep / `git grep` — fastest, exhaustive |
| Intent with a canonical syntax anchor (`@main`, `func main(`, `class AppDelegate`) | Grep the anchor — many "intent" questions are exact-match queries in disguise |
| Intent with no obvious anchor ("where are errors handled?") | recall-safe combo below |
| Function relationships (callers/callees) | `grepai trace` — grep has no equivalent |
| File patterns (`**/*.go`) | Glob |

## Recall-Safe Combo (cheap AND exhaustive)

grep's token cost is in dumping content lines; its *recall* is nearly free when
you ask for file names only. For an intent query, run both cheap layers:

```bash
# 1. Ranking: ~10 scored chunks, one call
grepai search "where errors are handled and logged" --json --compact

# 2. Recall: exhaustive candidate checklist — file NAMES only, ~zero tokens
git grep -ilE 'error|handl|logg' | head -50
```

Read grepai's top hits first, then scan the checklist for relevant-looking
files grepai did not rank — read those too. Never dump full grep content
output for an intent query; the file list gives you grep's recall at ~1% of
the tokens.

If grepai's top hits are docs/reports instead of code: scope with
`grepai search "<query>" --path <srcdir>`, or add generated content to a
`.grepaiignore`.

## How to Use This Skill

### Semantic Search

Use `grepai search` to find code by **describing what it does**:

```bash
# Search with natural language (ALWAYS use English for best results)
grepai search "user authentication flow"
grepai search "error handling middleware"
grepai search "database connection pooling"
grepai search "API request validation"

# JSON output for AI agents (--compact saves ~80% tokens)
grepai search "authentication flow" --json --compact

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

1. **Start with `grepai search`** for ranked starting points
2. **Add `git grep -ilE '<keywords>'`** for the exhaustive file checklist (names only)
3. **Use `grepai trace`** to understand function relationships
4. **Use `Read`** on ranked hits first, then on relevant checklist files grepai did not rank

## Fallback

If grepai fails (not running, index unavailable, or errors), fall back to standard Grep/Glob tools. Common issues:

- Index not built: Run `grepai watch` to build/update the index
- Embedder not available: Check that Ollama is running or OpenAI API key is set

## Keywords

semantic search, code search, natural language search, find code, explore codebase,
call graph, callers, callees, function relationships, code understanding,
intent search, code exploration, recall, token savings
