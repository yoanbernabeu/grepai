# AGENTS.md

This file provides guidance to AI coding agents (Claude Code, Codex, Cursor, etc.) when working with code in this repository. `CLAUDE.md` only imports this file.

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

## Issue Workflow

When working on a GitHub issue, **ALWAYS** follow this workflow:

1. **Create a feature branch** from `main`:
   ```bash
   git checkout main
   git pull origin main
   git checkout -b <type>/<issue-number>-<short-description>
   # Example: feat/42-add-new-embedder
   ```

2. **Implement the changes** and commit following the commit convention

3. **Push the branch and create a Pull Request**:
   ```bash
   git push -u origin <branch-name>
   gh pr create --title "<type>(scope): description" --body "Closes #<issue-number>"
   ```

4. **Before merging**, ensure:
   - CI pipeline passes (all checks green)
   - Code has been reviewed if required
   - No merge conflicts with `main`

**IMPORTANT:** Never push directly to `main`. Always use branches and PRs.

## V1 Stability Rules

grepai is now 1.x: users rely on existing configs, indexes and CLI/MCP outputs. Every change MUST follow these rules:

1. **Always test backward compatibility.** Before opening a PR, verify that:
   - an existing `.grepai/config.yaml` (without any newly added keys) still loads and behaves as before, with defaults matching the previous behavior;
   - an index built by the previous release (gob, Postgres, Qdrant, symbols, RPG) is still readable, or is migrated/rebuilt transparently;
   - CLI flags, JSON output fields and MCP tool parameters/results are not removed or renamed (additions only).
   Add a regression test whenever possible (e.g. load a config or index fixture produced by the previous version).
2. **Always test locally on a real, representative project**, not only with unit tests:
   - build the binary (`make build`) and run `./bin/grepai` (index, watch, search, trace, MCP…) on an actual project;
   - pick a project whose language matches the feature under work (e.g. PHP project for PHP trace scoping, SvelteKit project for `.svelte` extraction, Go project for Go-specific chunking); the grepai repository itself is fine for Go and generic features;
   - for backend-specific changes, test against that backend (e.g. a local Postgres/pgvector or Qdrant instance).
3. **Report both** in the PR "Test Plan": the backward-compatibility checks performed and the real project used (language, size, commands run, observed results).


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

# JSON output for AI agents (--compact saves ~80% tokens by omitting content)
grepai search "authentication flow" --json --compact
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
