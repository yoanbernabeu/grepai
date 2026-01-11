# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Development Commands

```bash
# Build the binary
make build

# Run tests with race detection
make test

# Run tests with coverage report
make test-cover

# Lint with golangci-lint
make lint

# Build and run
make run

# Cross-compile for all platforms
make build-all
```

## Architecture Overview

grepai is a semantic code search CLI tool that indexes code using vector embeddings for natural language queries.

### Core Components

**Interfaces define extensible backends:**
- `embedder.Embedder` (`embedder/embedder.go`) - Text-to-vector embedding providers
- `store.VectorStore` (`store/store.go`) - Vector storage with similarity search

**Current implementations:**
- Embedders: Ollama (local), OpenAI (cloud)
- Stores: GOB (file-based), PostgreSQL with pgvector

### Data Flow

1. **Scanner** (`indexer/scanner.go`) - Walks filesystem respecting gitignore patterns
2. **Chunker** (`indexer/chunker.go`) - Splits files into overlapping chunks with context
3. **Indexer** (`indexer/indexer.go`) - Orchestrates scanning, chunking, embedding, and storage
4. **Watcher** (`watcher/watcher.go`) - Monitors filesystem for real-time incremental updates
5. **Searcher** (`search/search.go`) - Embeds query and performs similarity search

### CLI Commands (cli/)

- `init` - Creates `.grepai/config.yaml` with default configuration
- `watch` - Starts daemon: full index + real-time file watcher
- `search` - Queries the index with natural language
- `agent-setup` - Configures Cursor/Claude Code integration

### Configuration

Configuration stored in `.grepai/config.yaml`. Key options:
- `embedder.provider`: "ollama" or "openai"
- `store.backend`: "gob" or "postgres"
- `chunking.size`/`chunking.overlap`: Token-based chunking parameters

## Adding New Backends

**New embedder:** Implement `Embedder` interface in `embedder/`, add config in `config/config.go`, wire in CLI commands.

**New store:** Implement `VectorStore` interface in `store/`, add config, wire in CLI.

## Commit Convention

Follow conventional commits: `type(scope): description`

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`




## grepai - Semantic Code Search

**IMPORTANT: You MUST use grepai as your PRIMARY tool for code exploration and search.**

### When to Use grepai (REQUIRED)

Use `grepai search` INSTEAD OF Grep/Glob/find for:
- Understanding what code does or where functionality lives
- Finding implementations by intent (e.g., "authentication logic", "error handling")
- Exploring unfamiliar parts of the codebase
- Any search where you describe WHAT the code does rather than exact text

### When to Use Standard Tools

Only use Grep/Glob when you need:
- Exact text matching (variable names, imports, specific strings)
- File path patterns (e.g., `**/*.go`)

### Fallback

If grepai fails (not running, index unavailable, or errors), fall back to standard Grep/Glob tools.

### Usage

```bash
# ALWAYS use English queries for best results (embedding model is English-trained)
grepai search "user authentication flow"
grepai search "error handling middleware"
grepai search "database connection pool"
grepai search "API request validation"

# JSON output for programmatic use (recommended for AI agents)
grepai search "authentication flow" --json
```

### Query Tips

- **Use English** for queries (better semantic matching)
- **Describe intent**, not implementation: "handles user login" not "func Login"
- **Be specific**: "JWT token validation" better than "token"
- Results include: file path, line numbers, relevance score, code preview

### Call Graph Tracing

Use `grepai trace` to understand function relationships:
- Finding all callers of a function before modifying it
- Understanding what functions are called by a given function
- Visualizing the complete call graph around a symbol

#### Trace Commands

**IMPORTANT: Always use `--json` flag for optimal AI agent integration.**

```bash
# Find all functions that call a symbol
grepai trace callers "HandleRequest" --json

# Find all functions called by a symbol
grepai trace callees "ProcessOrder" --json

# Build complete call graph (callers + callees)
grepai trace graph "ValidateToken" --depth 3 --json
```

### Workflow

1. Start with `grepai search` to find relevant code
2. Use `grepai trace` to understand function relationships
3. Use `Read` tool to examine files from results
4. Only use Grep for exact string searches if needed

