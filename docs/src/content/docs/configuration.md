---
title: Configuration
description: Configure grepai for your needs
---

## Config File Location

grepai stores its configuration in `.grepai/config.yaml` in your project root.

Run `grepai init` to create a default configuration.

## Full Configuration Reference

```yaml
# Embedder configuration
embedder:
  # Provider: "ollama" (local), "lmstudio" (local), or "openai" (cloud)
  provider: ollama

  # Ollama settings
  ollama:
    url: http://localhost:11434
    model: nomic-embed-text

  # LM Studio settings (if using lmstudio provider)
  lmstudio:
    url: http://127.0.0.1:1234
    model: text-embedding-nomic-embed-text-v1.5

  # OpenAI settings (if using openai provider)
  openai:
    api_key: ${OPENAI_API_KEY}  # Use environment variable
    model: text-embedding-3-small

# Vector store configuration
store:
  # Backend: "gob" (file-based) or "postgres" (PostgreSQL with pgvector)
  backend: gob

  # GOB settings
  gob:
    path: .grepai/index.gob

  # PostgreSQL settings (if using postgres backend)
  postgres:
    connection_string: postgres://user:pass@localhost:5432/grepai

# Chunking configuration
chunking:
  # Maximum tokens per chunk
  size: 512
  # Overlap between chunks (for context continuity)
  overlap: 50

# Scanner configuration
scanner:
  # Patterns to ignore (in addition to .gitignore)
  ignore:
    - "*.min.js"
    - "*.min.css"
    - "vendor/"
    - "node_modules/"
    - ".git/"
```

## Embedder Options

### Ollama (Local - Recommended)

```yaml
embedder:
  provider: ollama
  ollama:
    url: http://localhost:11434
    model: nomic-embed-text
```

Available models:
- `nomic-embed-text` - Good balance of speed and quality
- `mxbai-embed-large` - Higher quality, slower
- `all-minilm` - Fast, lower quality

### LM Studio (Local)

```yaml
embedder:
  provider: lmstudio
  model: text-embedding-nomic-embed-text-v1.5
  endpoint: http://127.0.0.1:1234
```

Available models (depends on what you load in LM Studio):
- `nomic-embed-text-v1.5` - Good general purpose (768 dims)
- `bge-small-en-v1.5` - Fast, smaller (384 dims)
- `bge-large-en-v1.5` - Higher quality (1024 dims)

### OpenAI (Cloud)

```yaml
embedder:
  provider: openai
  openai:
    api_key: ${OPENAI_API_KEY}
    model: text-embedding-3-small
```

Available models:
- `text-embedding-3-small` - Fast, cost-effective
- `text-embedding-3-large` - Higher quality

## Storage Options

### GOB (File-based - Default)

```yaml
store:
  backend: gob
  gob:
    path: .grepai/index.gob
```

Best for:
- Single-developer projects
- Quick setup
- No external dependencies

### PostgreSQL with pgvector

```yaml
store:
  backend: postgres
  postgres:
    connection_string: postgres://user:pass@localhost:5432/grepai
```

Best for:
- Team environments
- Large codebases
- Advanced querying needs

Setup:
```sql
CREATE EXTENSION vector;
```

## Chunking Tuning

```yaml
chunking:
  size: 512    # Tokens per chunk
  overlap: 50  # Overlap for context
```

- **Larger chunks**: Better context, fewer results, slower
- **Smaller chunks**: More precise matches, more results, faster
- **More overlap**: Better continuity, larger index

## Environment Variables

You can use environment variables in config:

```yaml
openai:
  api_key: ${OPENAI_API_KEY}
```

Or override config via environment:

```bash
export GREPAI_EMBEDDER_PROVIDER=openai
export GREPAI_STORE_BACKEND=postgres
```
