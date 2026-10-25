---
title: Semantic Search
description: Search your codebase in natural language with grepai search, filter by path, and get JSON or TOON output for AI agents.
---

## Semantic Search

`grepai search` finds code by meaning instead of exact text. You describe what the code does, and grepai returns the chunks whose embeddings are closest to your query.

### Quick Start

You need an index first. Build it with `grepai watch` (keeps it up to date) or `grepai index` (one-shot), then search:

```bash
# Build or update the index once
grepai index

# Search in natural language
grepai search "user authentication flow"

# Return 5 results instead of the default 10
grepai search "error handling" --limit 5

# Only search under a directory (relative to the project root)
grepai search "input validation" --path src/middleware/

# Structured output for scripts and AI agents
grepai search "database queries" --json --compact
```

### Features

- **Natural language queries**: describe what the code does.
- **Path filter**: restrict results to a directory with `--path`.
- **Ranking controls**: structural boost, optional hybrid keyword matching, and one result per file.
- **Structured output**: JSON or TOON, with a `--compact` mode that drops code content.
- **Workspaces**: search several projects at once with `--workspace`.

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--limit` | `-n` | `10` | Maximum number of results |
| `--path` | | | Only return files whose path starts with this prefix |
| `--json` | `-j` | `false` | Output JSON |
| `--toon` | `-t` | `false` | Output TOON (cannot be combined with `--json`) |
| `--compact` | `-c` | `false` | Omit the `content` field (requires `--json` or `--toon`) |
| `--workspace` | | | Search a workspace instead of the current project |
| `--project` | | | Limit a workspace search to one or more projects (repeatable, requires `--workspace`) |

See the [`grepai search`](/grepai/commands/grepai_search/) reference for the generated flag list.

### Filtering by Path

`--path` keeps only results whose file path starts with the given prefix:

- A relative prefix is matched against paths relative to the **project root**, not your current directory. `--path src/api/` matches `src/api/handler.go`.
- An absolute path is converted to a project-relative prefix. It must be inside the project.
- Matching is a plain prefix test, so `--path src/api` also matches `src/api_old/`. Add a trailing `/` to match a directory only.

In workspace mode, an absolute path also selects the matching project.

### How It Works

1. **Query embedding**: grepai converts your query to a vector with the embedder configured in `.grepai/config.yaml`.
2. **Similarity search**: the store returns the closest chunks. It fetches more candidates than `--limit` (2x, or 4x when `search.dedup` is enabled) so the next steps have room to reorder.
3. **Hybrid fusion (optional)**: if `search.hybrid.enabled` is true, vector results are merged with keyword matches. See [Hybrid Search](/grepai/hybrid-search/).
4. **Boost**: scores are multiplied by path-based factors (tests and docs down, some source directories up). See [Search Boost](/grepai/search-boost/).
5. **Deduplication**: if `search.dedup.enabled` is true, only the best chunk per file is kept.
6. **Limit**: the top `--limit` results are returned.

### Writing Effective Queries

| Good Query | Bad Query | Why |
|------------|-----------|-----|
| "user login validation" | "login" | More context improves matches |
| "how errors are handled in API" | "error" | Describes intent, not keyword |
| "where users are saved to database" | "save user" | Natural language works best |
| "JWT token authentication" | "token" | Specific terminology |

### Query Tips

- **Use English**: most embedding models work best with English queries.
- **Describe intent**: "handles user login" rather than "func Login".
- **Be specific**: "JWT token validation" is better than "token".
- **Looking for an exact identifier?** Enable [hybrid search](/grepai/hybrid-search/) or use a text search tool such as `grep`.

### Understanding Results

The default output looks like this:

```text
Found 2 results for: "user authentication flow"

─── Result 1 (score: 0.8125) ───
File: middleware/auth.go:15-45

  15 │ func AuthMiddleware() gin.HandlerFunc {
  16 │     return func(c *gin.Context) {
  17 │         token := c.GetHeader("Authorization")
...
     │ ... (16 more lines)

─── Result 2 (score: 0.7500) ───
File: handlers/login.go:10-32
...
```

- **score**: higher is better. Without hybrid search it is the vector similarity multiplied by the boost factors, so a boosted result can exceed 1.0. With hybrid search it is an RRF score (small values, around 0.03 at most before boosting).
- **File**: path relative to the project root, with the chunk's start and end lines.
- **Content**: up to 15 lines of the chunk, then a count of the remaining lines.
- If [RPG](/grepai/rpg/) is enabled, `Feature:` and `Symbol:` lines may appear under `File:`.

If nothing matches, grepai prints `No results found.`

### Structured Output

For AI agents and scripts, use `--json` or `--toon`. Add `--compact` to drop the `content` field and return locations only.

```bash
grepai search "authentication" --json             # JSON with content
grepai search "authentication" --json --compact   # JSON without content
grepai search "authentication" --toon             # TOON with content
grepai search "authentication" --toon --compact   # TOON without content
```

#### JSON Format

`--json` prints an indented array:

```json
[
  {
    "file_path": "middleware/auth.go",
    "start_line": 15,
    "end_line": 45,
    "score": 0.8125,
    "content": "File: middleware/auth.go\n\nfunc AuthMiddleware() gin.HandlerFunc {\n..."
  }
]
```

- `content` is the indexed chunk text, which starts with a `File: <path>` header.
- `--compact` removes `content`.
- `feature_path` and `symbol_name` are added when RPG provides them.
- On error, the output is `{"error": "..."}` instead of an array.

#### TOON Format

TOON (Token-Oriented Object Notation) is a tabular format that repeats field names once per array instead of once per result, which makes it shorter than JSON. Example with `--toon --compact`:

```text
[2]{end_line,"feature_path,omitempty",file_path,score,start_line,"symbol_name,omitempty"}:
  45,"",middleware/auth.go,0.8125,15,""
  32,"",handlers/login.go,0.75,10,""
```

The header lists the fields in alphabetical order. The `feature_path` and `symbol_name` columns are always present in TOON output and are empty when RPG is off.

**When to use TOON:**
- AI agents with tight token budgets
- High-volume automated searches

**When to use JSON:**
- Integration with existing JSON tooling (for example `jq`)
- Debugging and inspection

### Search Enhancements

Three settings under `search:` in `.grepai/config.yaml` change ranking. A new `grepai init` writes them with these defaults:

```yaml
search:
  boost:
    enabled: true    # path-based score multipliers
  hybrid:
    enabled: false   # vector + keyword fusion
    k: 60
  dedup:
    enabled: true    # keep only the best chunk per file
```

> **Note:** these values come from the config file. If a key is missing (for example in a config created by an older version), the feature is off. Add the key to turn it on.

#### Structural Boosting (enabled by default)

Multiplies scores by factors based on the file path:
- **Penalized**: tests, mocks, fixtures, generated files, Markdown and docs
- **Boosted**: paths containing `/src/`, `/lib/` or `/app/`

See [Search Boost](/grepai/search-boost/) for the rules and how they match.

#### Hybrid Search (disabled by default)

Merges vector results with keyword matches using Reciprocal Rank Fusion (RRF). Useful when queries contain exact identifiers.

See [Hybrid Search](/grepai/hybrid-search/) for configuration.

#### Deduplication (enabled by default)

With `search.dedup.enabled: true`, each file appears at most once in the results (its highest-scoring chunk). Set it to `false` to see several chunks from the same file.

#### Workspace searches

`grepai search --workspace` does not read the project's `search:` settings. It always uses the default boost rules, with hybrid search and deduplication off.

### Troubleshooting

| Problem | Solution |
|---------|----------|
| `no grepai project found` | Run `grepai init` in the project first |
| No results | Build the index with `grepai index` or `grepai watch`, then check it with `grepai status` |
| No results with `--path` | The prefix is relative to the project root, not your current directory |
| Poor relevance | Use a more descriptive query, or check the embedder model |
| Missing files | Check `ignore` in `.grepai/config.yaml`, `.gitignore` and `.grepaiignore` |
| Slow search on a large index | The GOB backend scans every vector in memory. Consider [PostgreSQL or Qdrant](/grepai/backends/stores/) |
| `--compact flag requires --json or --toon flag` | Add `--json` or `--toon` |

### Use Cases

#### Finding Implementation

```bash
# Where is authentication handled?
grepai search "user authentication logic"

# How are errors processed?
grepai search "error handling middleware"
```

#### Understanding Codebase

```bash
# Find database interactions
grepai search "database connection and queries"

# Locate API endpoints, only under internal/
grepai search "REST API route handlers" --path internal/
```

#### AI Agent Integration

Give an agent file locations, then let it read the files it needs:

```bash
grepai search "payment processing" --json --compact --limit 5
grepai search "payment processing" --toon --compact --limit 5
```

### Commands Reference

- [`grepai search`](/grepai/commands/grepai_search/): full CLI reference
- [`grepai index`](/grepai/commands/grepai_index/): build or update the index once
- [`grepai watch`](/grepai/commands/grepai_watch/): keep the index up to date
