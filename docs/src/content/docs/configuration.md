---
title: Configuration
description: Configure grepai for your needs
---

## Config File Location

grepai stores its configuration in `.grepai/config.yaml` in your project root.

Run `grepai init` to create a default configuration.

## Full Configuration Reference

```yaml
# Config file version
version: 1

# Embedder configuration
embedder:
  # Provider: "ollama" (local), "lmstudio" (local), or "openai" (cloud)
  provider: ollama
  # Model name (depends on provider)
  model: nomic-embed-text
  # Endpoint URL (depends on provider, supports Azure OpenAI)
  endpoint: http://localhost:11434
  # API key (for OpenAI provider, use environment variable)
  api_key: ${OPENAI_API_KEY}
  # Vector dimensions (depends on model, auto-detected if not set)
  dimensions: 768

# Vector store configuration
store:
  # Backend: "gob" (file-based) or "postgres" (PostgreSQL with pgvector)
  backend: gob

  # PostgreSQL settings (if using postgres backend)
  postgres:
    dsn: postgres://user:pass@localhost:5432/grepai

# Chunking configuration
chunking:
  # Maximum tokens per chunk
  size: 512
  # Overlap between chunks (for context continuity)
  overlap: 50

# File watching configuration
watch:
  # Debounce delay in milliseconds
  debounce_ms: 500

# Call graph tracing configuration
trace:
  # Extraction mode: "fast" (regex) or "precise" (tree-sitter)
  mode: fast
  # File extensions to index for symbols
  enabled_languages:
    - .go
    - .js
    - .ts
    - .jsx
    - .tsx
    - .py
    - .php
    - .c
    - .h
    - .cpp
    - .hpp
    - .cc
    - .cxx
    - .rs
    - .zig
  # Patterns to exclude from symbol indexing
  exclude_patterns:
    - "*_test.go"
    - "*.spec.ts"

# Patterns to ignore (in addition to .gitignore)
ignore:
  - ".git"
  - ".grepai"
  - "node_modules"
  - "vendor"
  - "target"
  - ".zig-cache"
  - "zig-out"
```

## Embedder Options

### Ollama (Local - Recommended)

```yaml
embedder:
  provider: ollama
  model: nomic-embed-text
  endpoint: http://localhost:11434
  dimensions: 768
```

Available models:
- `nomic-embed-text` - Good balance of speed and quality
- `mxbai-embed-large` - Higher quality, slower
- `all-minilm` - Fast, lower quality
- `qwen3-embedding:0.6b` - State-of-the-art, multilingual (including programming languages)
- `embeddinggemma` - Google's model optimized for efficiency

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
  model: text-embedding-3-small
  api_key: ${OPENAI_API_KEY}
  dimensions: 1536
```

Available models:
- `text-embedding-3-small` - 1536 dimensions, fast, cost-effective
- `text-embedding-3-large` - 3072 dimensions, higher quality

### Azure OpenAI / Microsoft Foundry

Use a custom endpoint for Azure OpenAI or other OpenAI-compatible providers:

```yaml
embedder:
  provider: openai
  model: text-embedding-ada-002
  endpoint: https://YOUR-RESOURCE.openai.azure.com/v1
  api_key: ${AZURE_OPENAI_API_KEY}
  dimensions: 1536
```

## Storage Options

### GOB (File-based - Default)

```yaml
store:
  backend: gob
```

Best for:
- Single-developer projects
- Quick setup
- No external dependencies

The index is stored automatically in `.grepai/index.gob`.

### PostgreSQL with pgvector

```yaml
store:
  backend: postgres
  postgres:
    dsn: postgres://user:pass@localhost:5432/grepai
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

## Search Options

grepai provides two optional search enhancements:

### Search Boost (enabled by default)

Adjusts scores based on file paths. Test files are penalized, source directories are boosted.

```yaml
search:
  boost:
    enabled: true
    penalties:
      - pattern: "_test."
        factor: 0.5
    bonuses:
      - pattern: "/src/"
        factor: 1.1
```

See [Search Boost](/grepai/search-boost/) for full documentation.

### Hybrid Search (disabled by default)

Combines vector similarity with text matching using RRF.

```yaml
search:
  hybrid:
    enabled: true
    k: 60
```

See [Hybrid Search](/grepai/hybrid-search/) for full documentation.

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
