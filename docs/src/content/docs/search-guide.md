---
title: Semantic Search
description: Master natural language code search with grepai
---

## Semantic Search

`grepai search` enables natural language searches across your codebase using vector embeddings. Instead of exact text matching, it understands the *meaning* of your queries.

### Features

- **Natural language queries**: Search by describing what the code does
- **Vector embeddings**: Uses AI models to understand semantic meaning
- **Relevance scoring**: Results ranked by similarity (0.0-1.0)
- **Structural boosting**: Prioritizes source over tests automatically
- **JSON output**: Perfect for AI agents and automation

### Quick Start

```bash
# Ensure watch is running to index your code
grepai watch

# Search for authentication code
grepai search "user authentication flow"

# Limit results
grepai search "error handling" --limit 5

# JSON output for AI agents (--compact saves ~80% tokens)
grepai search "database queries" --json --compact
```

### How It Works

1. **Query embedding**: Your search query is converted to a vector using the configured embedder (Ollama, OpenAI, or LM Studio)
2. **Similarity search**: Cosine similarity is calculated against indexed code chunks
3. **Boost adjustment**: Scores are adjusted based on file paths (tests penalized, source boosted)
4. **Result ranking**: Results are sorted by final relevance score

### Writing Effective Queries

| Good Query | Bad Query | Why |
|------------|-----------|-----|
| "user login validation" | "login" | More context improves matches |
| "how errors are handled in API" | "error" | Describes intent, not keyword |
| "where users are saved to database" | "save user" | Natural language works best |
| "JWT token authentication" | "token" | Specific terminology |

### Query Tips

- **Use English** for best results (embedding models are trained on English)
- **Describe intent**: "handles user login" not "func Login"
- **Be specific**: "JWT token validation" better than "token"
- **Think semantically**: Describe what the code does, not how it's named

### Understanding Results

```
Score: 0.89 | middleware/auth.go:15-45
----------------------------------------
func AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        token := c.GetHeader("Authorization")
        // ... validate token
    }
}
```

- **Score**: Relevance (0.0-1.0), higher is better
- **File:lines**: Location of the matching chunk
- **Content**: Code snippet with context

### JSON Output

For AI agents and scripts, use the `--json` flag:

```bash
grepai search "authentication" --json           # Full JSON output
grepai search "authentication" --json --compact # Minimal JSON (no content field)
```

Output format:

```json
[
  {
    "file_path": "middleware/auth.go",
    "start_line": 15,
    "end_line": 45,
    "score": 0.89,
    "content": "func AuthMiddleware() gin.HandlerFunc { ... }"
  }
]
```

### Search Enhancements

grepai provides two optional search improvements:

#### Structural Boosting (enabled by default)

Automatically adjusts scores based on file paths:
- **Penalized**: Tests, mocks, fixtures, generated files, docs
- **Boosted**: Source directories (`/src/`, `/lib/`, `/app/`)

See [Search Boost](/grepai/search-boost/) for configuration.

#### Hybrid Search (disabled by default)

Combines vector similarity with text matching using Reciprocal Rank Fusion (RRF). Useful when queries contain exact identifiers.

See [Hybrid Search](/grepai/hybrid-search/) for configuration.

### Troubleshooting

| Problem | Solution |
|---------|----------|
| No results | Ensure `grepai watch` is running and index is built |
| Poor relevance | Try more descriptive queries, check embedder model |
| Missing files | Check `.grepai/config.yaml` ignore patterns |
| Slow search | Consider PostgreSQL backend for large codebases |

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

# Locate API endpoints
grepai search "REST API route handlers"
```

#### AI Agent Integration

Provide code context to AI agents:

```bash
# Get compact JSON for AI processing (~80% fewer tokens)
grepai search "payment processing" --json --compact --limit 5
```

### Commands Reference

- [`grepai search`](/grepai/commands/grepai_search/) - Full CLI reference
