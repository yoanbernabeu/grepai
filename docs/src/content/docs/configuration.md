---
title: Configuration
description: Reference for .grepai/config.yaml - embedder, vector store, chunking, ignore rules (.gitignore, .grepaiignore), search tuning and API keys.
---

This page documents `.grepai/config.yaml`, the per-project configuration file. Read it when you want to change the embedding provider, the storage backend, what gets indexed, or how results are ranked.

## Config File Location

grepai stores its configuration in `.grepai/config.yaml` at your project root. `grepai init` creates it with defaults. Commands look for `.grepai/` in the current directory and then in each parent directory, so you can run them from any subfolder.

The most common change is switching the embedder. For example, to use OpenAI instead of Ollama:

```yaml
embedder:
  provider: openai
  model: text-embedding-3-small
  endpoint: https://api.openai.com/v1
```

```bash
export OPENAI_API_KEY="sk-..."
grepai index
```

> **Note:** Changing the embedder model or dimensions makes existing vectors incompatible. Delete the index (`rm .grepai/index.gob` for the GOB store) and re-index. The same applies to chunking changes: files already indexed are not re-chunked until they change.

Multi-project workspaces use a separate file, `~/.grepai/workspace.yaml`. See [Workspace Management](/grepai/workspace/).

## Full Configuration Reference

This is the file that `grepai init --yes` generates, shortened and annotated. Keys you omit fall back to defaults when the file is loaded, except where noted.

```yaml
version: 1

embedder:
  provider: ollama              # ollama | lmstudio | openai | openrouter | synthetic | requesty
  model: nomic-embed-text
  endpoint: http://localhost:11434
  dimensions: 768               # vector size; see "Embedder Options"
  # api_key: ""                 # cloud providers; prefer an environment variable
  # parallelism: 4              # concurrent requests (openai only), default 4
  # request_timeout_seconds: 60 # HTTP timeout per request; default 60 (90 for synthetic), ignored by requesty
  # max_retries: 5              # retries on HTTP 429/5xx (openai only)
  # max_batch_size: 2000        # inputs per request (openai only)
  # max_batch_tokens: 280000    # estimated tokens per request (openai only)

store:
  backend: gob                  # gob | postgres | qdrant
  # postgres:
  #   dsn: postgres://user:pass@localhost:5432/grepai
  # qdrant:
  #   endpoint: localhost
  #   port: 6334
  #   use_tls: false
  #   collection: ""            # default: derived from the project path
  #   api_key: ""               # for Qdrant Cloud

chunking:
  size: 512                     # chunk budget in tokens (estimated as 4 characters per token)
  overlap: 50                   # tokens shared by neighboring chunks
  strategy: fixed               # fixed | ast (experimental)
  # custom_extensions:          # extra file extensions to index
  #   - .tengo
  #   - .prisma

framework_processing:           # pre-processing of framework single-file components
  enabled: true
  mode: auto                    # auto | require | off
  node_path: node
  frameworks:
    vue:
      enabled: true
    svelte:
      enabled: false
    astro:
      enabled: false
    solid:
      enabled: false

watch:
  debounce_ms: 500              # wait time before processing a burst of file events
  # discover_worktrees: true    # also watch linked git worktrees (read from the main worktree only)
  # last_index_time: ...        # written by grepai watch/index, do not edit
  rpg_persist_interval_ms: 1000 # RPG settings, see the RPG page
  rpg_derived_debounce_ms: 300
  rpg_full_reconcile_interval_sec: 300
  rpg_max_dirty_files_per_batch: 128

search:
  boost:
    enabled: true
    penalties:                  # 17 default rules (tests, mocks, fixtures, generated code, docs)
      - pattern: _test.
        factor: 0.5
    bonuses:                    # 3 default rules (/src/, /lib/, /app/)
      - pattern: /src/
        factor: 1.1
  hybrid:
    enabled: false
    k: 60
  dedup:
    enabled: true               # keep only the best chunk per file

trace:
  enabled_languages:            # extensions included in the symbol index
    - .go
    - .js
    - .ts
    # ... .jsx .tsx .vue .py .php .lua .gd .rb .c .h .cpp .hpp .cc .cxx .hxx
    #     .rs .zig .cs .java .fs .fsx .fsi .pas .dpr

rpg:
  enabled: false                # Repository Planning Graph, see the RPG page
  feature_mode: local
  drift_threshold: 0.35
  max_traversal_depth: 3
  llm_provider: ollama
  llm_endpoint: http://localhost:11434/v1
  llm_timeout_ms: 8000
  feature_group_strategy: sample

ignore:                         # directory or file names skipped everywhere
  - .git
  - .grepai
  - node_modules
  - vendor
  - bin
  - dist
  - __pycache__
  - .venv
  - venv
  - .idea
  - .vscode
  - target
  - .zig-cache
  - zig-out
  - qdrant_storage

# external_gitignore: "~/.config/git/ignore"
```

Notes on specific keys:

- `ignore` replaces the default list when you set it. Copy the defaults if you want to keep them.
- `search` has no load-time defaults. If your config has no `search:` section (for example, an older file), boost and deduplication are off. Run `grepai init` in a scratch directory to get the current defaults.
- Older config files may still contain `trace.mode`, `trace.exclude_patterns`, `update.check_on_startup` or `rpg.store_path`. These keys never had any effect and were removed; grepai ignores them and drops them the next time it saves the file.
- `framework_processing` runs a Node.js script to compile Vue single-file components before embedding and tracing. Only the Vue processor is implemented; the Svelte, Astro and Solid entries are placeholders. With `mode: auto`, grepai falls back to the raw file if Node or the Vue compiler is missing. `require` fails instead, and `off` disables processing.
- The `rpg` section and `watch.rpg_*` keys are described in [Repository Planning Graph](/grepai/rpg/).

## Embedder Options

The embedder turns code chunks and queries into vectors. If you do not set `endpoint`, grepai uses the provider's default. `dimensions` must match the vector size of your model:

| Provider | Default model | Default endpoint | Default `dimensions` | API key environment variable |
|----------|---------------|------------------|----------------------|------------------------------|
| `ollama` | `nomic-embed-text` | `http://localhost:11434` | 768 | none |
| `lmstudio` | `text-embedding-nomic-embed-text-v1.5` | `http://127.0.0.1:1234` | 768 | none |
| `openai` | `text-embedding-3-small` | `https://api.openai.com/v1` | model's native size | `OPENAI_API_KEY` |
| `openrouter` | `openai/text-embedding-3-small` | `https://openrouter.ai/api/v1` | model's native size | `OPENROUTER_API_KEY`, then `OPENAI_API_KEY` |
| `synthetic` | `hf:nomic-ai/nomic-embed-text-v1.5` | `https://api.synthetic.new/openai/v1` | 768 | `SYNTHETIC_API_KEY`, then `OPENAI_API_KEY` |
| `requesty` | `openai/text-embedding-3-small` | `https://router.requesty.ai/v1` | model's native size | `REQUESTY_API_KEY`, then `OPENAI_API_KEY` |

For `openai`, `openrouter` and `requesty`, leave `dimensions` unset to use the model's native size: 1536 for `text-embedding-3-small`, 3072 for `text-embedding-3-large`, 4096 for `qwen/qwen3-embedding-8b`. For local providers, set `dimensions` yourself when you use a model other than the default.

See [Embedders](/grepai/backends/embedders/) for setup details for each provider.

### Ollama (Local - Recommended)

```yaml
embedder:
  provider: ollama
  model: nomic-embed-text
  endpoint: http://localhost:11434
  dimensions: 768
```

Pull the model first with `ollama pull nomic-embed-text`. To use another model, pull it and set `model` and `dimensions` to match. Common choices:

| Model | Dimensions | Notes |
|-------|-----------|-------|
| `nomic-embed-text` | 768 | Default, English |
| `nomic-embed-text-v2-moe` | 768 | Multilingual |
| `bge-m3` | 1024 | Multilingual |
| `mxbai-embed-large` | 1024 | Larger, slower |
| `all-minilm` | 384 | Small and fast |

For example, for a codebase with comments in several languages:

```yaml
embedder:
  provider: ollama
  model: nomic-embed-text-v2-moe
  endpoint: http://localhost:11434
  dimensions: 768
```

### LM Studio (Local)

Start the LM Studio local server and load an embedding model, then:

```yaml
embedder:
  provider: lmstudio
  model: text-embedding-nomic-embed-text-v1.5
  endpoint: http://127.0.0.1:1234
  dimensions: 768
```

Set `model` to the identifier LM Studio shows for the loaded model, and `dimensions` to its vector size.

### OpenAI (Cloud)

```yaml
embedder:
  provider: openai
  model: text-embedding-3-small   # or text-embedding-3-large
  endpoint: https://api.openai.com/v1
  parallelism: 4                  # concurrent requests, lowered automatically on rate limits
```

Set your key in the environment with `export OPENAI_API_KEY="sk-..."`. You can also put the key in `api_key`, but then it is stored in plain text in `config.yaml`.

### Azure OpenAI / Microsoft Foundry

The `openai` provider works with any endpoint that implements the OpenAI embeddings API. grepai sends `POST <endpoint>/embeddings` with an `Authorization: Bearer <api_key>` header.

```yaml
embedder:
  provider: openai
  model: text-embedding-ada-002     # your deployment's model name
  endpoint: https://YOUR-RESOURCE.openai.azure.com/v1
  dimensions: 1536
```

Provide the key with `OPENAI_API_KEY` or `api_key`.

## Storage Options

The store keeps the vectors and runs similarity search.

### GOB (File-based - Default)

```yaml
store:
  backend: gob
```

The index is a single file, `.grepai/index.gob`, next to the symbol index `.grepai/symbols.gob`. No server is needed. This is the right choice for most single-developer projects.

### PostgreSQL with pgvector

```yaml
store:
  backend: postgres
  postgres:
    dsn: postgres://user:pass@localhost:5432/grepai
```

On startup, grepai runs `CREATE EXTENSION IF NOT EXISTS vector` and creates its tables. The database user needs permission to do this, or an administrator must create the `vector` extension first. A shared database lets several projects or worktrees use one index.

> **Note:** `grepai init -b postgres --yes` writes the default DSN `postgres://localhost:5432/grepai`. Change `store.postgres.dsn` if your database uses another host, user or name.

### Qdrant

```yaml
store:
  backend: qdrant
  qdrant:
    endpoint: localhost
    port: 6334          # gRPC port
    use_tls: false
```

To start Qdrant locally with Docker:

```bash
docker run -p 6333:6333 -p 6334:6334 qdrant/qdrant
```

For Qdrant Cloud, set `use_tls: true` and `api_key`. If `collection` is empty, grepai derives a collection name from the project path.

See [Vector Stores](/grepai/backends/stores/) for detailed configuration options.

## Ignoring Files

grepai indexes files with a supported source extension (Go, Python, JavaScript/TypeScript, Java, C/C++, Rust, PHP, Ruby, Markdown and more). It skips binary files, minified files (such as `*.min.js`) and files larger than 1 MB. To index another extension, add it to `chunking.custom_extensions` (with the leading dot).

To exclude files, grepai combines these sources:

1. **`.gitignore`** files at any level of the project.
2. **`ignore`** in `config.yaml`: names or gitignore-style patterns applied from the project root.
3. **`.grepaiignore`** files at any level of the project. They use gitignore syntax and take precedence over `.gitignore`.
4. **`external_gitignore`**: an extra gitignore file outside the project (see [External Gitignore](#external-gitignore)).

Use `.grepaiignore` to exclude files from grepai without touching `.gitignore`, or to index files that Git ignores:

```gitignore
# .grepaiignore
# Do not index generated fixtures
testdata/golden/

# Index this file even though .gitignore excludes it
!config/local.example.go
```

A negation (`!pattern`) in `.grepaiignore` overrides a `.gitignore` at the same level or above. A `.gitignore` in a deeper directory still wins.

## Chunking Tuning

```yaml
chunking:
  size: 512    # tokens per chunk
  overlap: 50  # tokens shared between neighboring chunks
```

grepai estimates tokens as 4 characters per token.

- **Larger chunks**: more context per result, fewer chunks to embed.
- **Smaller chunks**: more precise matches, more chunks to embed.
- **More overlap**: better continuity across chunk boundaries, larger index.

### Chunking Strategy

> **Experimental**: the `ast` strategy may change in future releases, which can require re-indexing. `fixed` is the default and recommended strategy.

```yaml
chunking:
  strategy: ast  # "fixed" (default) or "ast"
```

- **`fixed`** (default): splits files into windows of `chunking.size` tokens, with `chunking.overlap` tokens shared between neighboring chunks.
- **`ast`**: parses the file with tree-sitter and splits it along syntax boundaries (functions, classes, declarations) using the [cAST](https://arxiv.org/abs/2506.15655) algorithm. Small neighboring nodes are merged up to the size budget, and nodes larger than the budget are split into their children. A chunk is usually a whole function or a group of related declarations.

With `ast`, `chunking.size` is still the budget, counted in non-whitespace characters (4 per token), and `chunking.overlap` is not used. Supported files are `.go`, `.py`, `.js`, `.jsx`, `.ts` and `.tsx`. Other files, and files nested too deeply to split, use the fixed chunker.

> **Note:** `ast` requires a build with the `treesitter` build tag and CGO (`go build -tags treesitter ./cmd/grepai`). The prebuilt release binaries do not include it. Without it, or with an unknown strategy name, grepai logs a warning and uses `fixed`.

Changing the strategy does not re-chunk files that are already indexed. To apply it to the whole project, delete the index (`rm .grepai/index.gob` for the default GOB store) and run `grepai index` or `grepai watch` again.

### Automatic Re-chunking

If a chunk is larger than the embedder's context limit, grepai splits it and retries:

1. grepai sends the chunk to the embedder.
2. If the embedder returns a context-length error, grepai splits that chunk into smaller pieces (half the size).
3. It retries, with up to 3 re-chunking attempts in total.

You will see log messages like:

```
Re-chunking large_file.go chunk 0 (attempt 1/3): context limit exceeded
Split chunk into 2 sub-chunks
```

This is a safety net: with the default `size: 512`, you should rarely see it.

## Search Options

These settings change how results are ranked. They apply to `grepai search` and the MCP search tool in single-project mode. Workspace searches use the built-in defaults.

### Search Boost (enabled by default)

Multiplies each result's score by a factor based on its file path. Patterns are case-sensitive substring matches on the project-relative path. When several rules match, their factors are multiplied. The default rules lower the score of tests, mocks, fixtures, generated code and docs, and raise `/src/`, `/lib/` and `/app/`.

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

Combines vector similarity with text matching using Reciprocal Rank Fusion (RRF). `k` is the RRF constant.

```yaml
search:
  hybrid:
    enabled: true
    k: 60
```

See [Hybrid Search](/grepai/hybrid-search/) for full documentation.

### Result Deduplication (enabled by default)

When `search.dedup.enabled` is `true`, grepai keeps only the highest-scoring chunk for each file, so one large file cannot fill all the results.

```yaml
search:
  dedup:
    enabled: true
```

## External Gitignore

Use `external_gitignore` to apply a gitignore file that lives outside the project, such as your global Git ignore file:

```yaml
external_gitignore: "~/.config/git/ignore"
```

Common locations:

- `~/.config/git/ignore` (XDG default)
- `~/.gitignore_global` (older convention)

A leading `~` expands to your home directory. Patterns apply from the project root. If the file does not exist, grepai logs a warning and continues.

## Environment Variables

grepai does **not** expand variables inside `config.yaml`. A value such as `api_key: ${OPENAI_API_KEY}` is sent literally as the key. Instead, leave `api_key` empty and set the provider's environment variable:

| Provider | Variables read (in order) |
|----------|---------------------------|
| `openai` | `OPENAI_API_KEY` |
| `openrouter` | `OPENROUTER_API_KEY`, `OPENAI_API_KEY` |
| `synthetic` | `SYNTHETIC_API_KEY`, `OPENAI_API_KEY` |
| `requesty` | `REQUESTY_API_KEY`, `OPENAI_API_KEY` |

```bash
export OPENAI_API_KEY="sk-..."
grepai watch
```

An `api_key` set in `config.yaml` takes precedence over the environment. Other settings, such as the provider or the backend, cannot be overridden with environment variables: edit `config.yaml` instead.
