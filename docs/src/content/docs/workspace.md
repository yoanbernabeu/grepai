---
title: Workspace Management
description: Manage multi-project workspaces for cross-project indexing and search
---

Workspaces allow you to index and search across multiple projects using a shared vector store. This is useful when working on microservices architectures, monorepos, or related projects that you want to search together.

## Prerequisites

Workspaces require a shared vector store backend. **GOB backend is not supported** because it's file-based and cannot be shared across projects.

Supported backends for workspaces:

- **PostgreSQL** with pgvector (recommended for production)
- **Qdrant** (recommended for advanced vector search)

## Quick Start

### 1. Create a Workspace

**Interactive mode:**

```bash
grepai workspace create my-fullstack
```

This will interactively prompt you to configure the storage backend and embedding provider.

**Non-interactive mode (for automation/CI):**

```bash
# Qdrant + Ollama with defaults
grepai workspace create my-fullstack --backend qdrant --provider ollama --yes

# Qdrant + specific model
grepai workspace create my-fullstack --backend qdrant --provider ollama --model bge-m3 --yes

# Qdrant + OpenAI
grepai workspace create my-fullstack --backend qdrant --provider openai --model text-embedding-3-small --yes

# PostgreSQL + Ollama
grepai workspace create my-fullstack --backend postgres --provider ollama --dsn "postgres://grepai:grepai@localhost:5432/grepai" --yes

# Custom Qdrant endpoint
grepai workspace create my-fullstack --backend qdrant --qdrant-endpoint my-qdrant-host --qdrant-port 6334 --provider ollama --yes

# From a YAML config file
grepai workspace create my-fullstack --from workspace-config.yaml
```

**Non-interactive flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--backend` | Storage backend (`qdrant` or `postgres`) | Required (or `--yes`) |
| `--provider` | Embedding provider (`ollama`, `openai`, `lmstudio`) | `ollama` with `--yes` |
| `--model` | Embedding model name | Provider default |
| `--endpoint` | Embedder endpoint URL | Provider default |
| `--dsn` | PostgreSQL connection string | Required for postgres |
| `--qdrant-endpoint` | Qdrant hostname | `http://localhost` |
| `--qdrant-port` | Qdrant gRPC port | `6334` |
| `--collection` | Qdrant collection name | `workspace_{name}` |
| `--yes` | Accept all defaults | - |
| `--from` | Load config from YAML/JSON file | - |

**`--yes` defaults:** Qdrant backend, Ollama provider, nomic-embed-text model (768 dims).

**`--from` file format (YAML):**

```yaml
store:
  backend: qdrant
  qdrant:
    endpoint: http://localhost
    port: 6334
embedder:
  provider: openai
  model: text-embedding-3-small
  api_key: sk-...
  dimensions: 1536
  parallelism: 16
```

### 2. Add Projects

```bash
grepai workspace add my-fullstack /path/to/frontend
grepai workspace add my-fullstack /path/to/backend
grepai workspace add my-fullstack /path/to/shared-lib
```

### 3. Start Indexing

```bash
# Foreground mode
grepai watch --workspace my-fullstack

# Background mode
grepai watch --workspace my-fullstack --background
```

### 4. Search Across Projects

```bash
# Search all projects in workspace
grepai search --workspace my-fullstack "authentication flow"

# Search specific project only
grepai search --workspace my-fullstack --project frontend "React components"
```

## Configuration File

Workspace configuration is stored in `~/.grepai/workspace.yaml`:

```yaml
version: 1
workspaces:
  my-fullstack:
    name: my-fullstack
    store:
      backend: postgres
      postgres:
        dsn: postgres://localhost:5432/grepai
    embedder:
      provider: ollama
      model: nomic-embed-text
      endpoint: http://localhost:11434
      dimensions: 768
    projects:
      - name: frontend
        path: /path/to/frontend
      - name: backend
        path: /path/to/backend
      - name: shared-lib
        path: /path/to/shared-lib
```

## CLI Commands

### Workspace Management

```bash
# List all workspaces
grepai workspace list

# Show workspace details
grepai workspace show my-fullstack

# Check workspace status
grepai workspace status my-fullstack

# Delete a workspace
grepai workspace delete my-fullstack
```

### Project Management

```bash
# Add project to workspace
grepai workspace add my-fullstack /path/to/project

# Remove project from workspace
grepai workspace remove my-fullstack project-name
```

### Watch Commands

```bash
# Start workspace watcher (foreground)
grepai watch --workspace my-fullstack

# Start workspace watcher (background)
grepai watch --workspace my-fullstack --background

# Check watcher status
grepai watch --workspace my-fullstack --status

# Stop background watcher
grepai watch --workspace my-fullstack --stop
```

### Search Commands

```bash
# Search all projects in workspace
grepai search --workspace my-fullstack "query"

# Search specific projects
grepai search --workspace my-fullstack --project frontend "query"
grepai search --workspace my-fullstack --project frontend --project backend "query"

# Filter by path prefix (searches only files in matching paths)
grepai search --workspace my-fullstack --path src/ "query"
grepai search --workspace my-fullstack --project frontend --path components/ "React hooks"

# JSON output for AI agents
grepai search --workspace my-fullstack "query" --json
grepai search --workspace my-fullstack "query" --json --compact
```

## MCP Integration

### Workspace-Aware MCP Server

Start the MCP server with the `--workspace` flag to enable automatic cross-project search:

```bash
# Claude Code
claude mcp add grepai -- grepai mcp-serve --workspace my-fullstack

# Or in .mcp.json
{
  "mcpServers": {
    "grepai": {
      "command": "grepai",
      "args": ["mcp-serve", "--workspace", "my-fullstack"]
    }
  }
}
```

When `--workspace` is set, the MCP server **auto-injects** the workspace into every search request. AI agents can call `grepai_search` with just a query — no need to pass `workspace` or `projects` parameters. Cross-project search works by default.

### Manual Workspace Parameters

Without the `--workspace` flag, agents can still search workspaces by passing parameters explicitly:

```json
{
  "name": "grepai_search",
  "arguments": {
    "query": "authentication flow",
    "workspace": "my-fullstack",
    "projects": "frontend,backend"
  }
}
```

### Trace Tools in Workspace Mode

The trace tools (`grepai_trace_callers`, `grepai_trace_callees`, `grepai_trace_graph`) and `grepai_index_status` fully support workspace mode. When the MCP server is started with `--workspace`, trace tools automatically search across all projects in the workspace. You can also pass a `project` parameter to limit the trace to a specific project.

Each project in a workspace maintains its own symbol index in `.grepai/symbols.gob`, regardless of the vector store backend (Qdrant or PostgreSQL). Symbols are built automatically during `grepai watch --workspace`.

```json
{
  "name": "grepai_trace_callers",
  "arguments": {
    "symbol": "HandleRequest",
    "workspace": "my-fullstack",
    "project": "backend"
  }
}
```

## How It Works

### File Path Prefixing

When indexing workspace projects, file paths are stored with workspace and project prefixes:

- Original: `/path/to/frontend/src/App.tsx`
- Stored: `my-workspace/frontend/src/App.tsx`

This format (`workspaceName/projectName/relativePath`) allows:

- Filtering search results by project name
- Multiple workspaces with same project names (e.g., `workspace1/frontend` vs `workspace2/frontend`)
- Clear identification of which workspace/project a result belongs to

### Configuration Priority

When indexing via workspace, settings come from different sources:

| Setting | Source | Notes |
|---------|--------|-------|
| `store` (backend, DSN) | **Workspace** | Shared store required |
| `embedder` (provider, model) | **Workspace** | Ensures compatible vectors |
| `chunking` (size, overlap) | **Project** | Can vary per project/language |
| `ignore` patterns | **Project** | Project-specific exclusions |
| `external_gitignore` | **Project** | Project-specific gitignore |

**Why this design?**

- **Store & Embedder from workspace**: All projects must use the same embedder to produce compatible vectors, and the same store for cross-project search.
- **Chunking & Ignore from project**: These can safely differ between projects (e.g., larger chunks for documentation, smaller for code).

## Migration from Single-Project

Existing single-project setups continue to work unchanged. Workspaces are an additional feature that doesn't affect existing functionality.

### GOB Projects (Default)

GOB projects store their index locally in `.grepai/index.gob`. These are completely separate from workspace indexes and will continue to work.

### PostgreSQL Projects

If your projects already use PostgreSQL with individual indexes, be aware that:

1. **Existing chunks** are stored without workspace/project prefix (e.g., `src/App.tsx`)
2. **Workspace chunks** are stored with full prefix (e.g., `my-workspace/frontend/src/App.tsx`)
3. **Both can coexist** in the same database but are treated as different files

**Recommended migration steps:**

```bash
# 1. Create workspace pointing to the SAME PostgreSQL database
grepai workspace create my-workspace
# Use the same DSN as your existing projects

# 2. Add projects
grepai workspace add my-workspace /path/to/frontend
grepai workspace add my-workspace /path/to/backend

# 3. (Optional) Clean old non-prefixed chunks from database
# Old chunks don't have the workspace prefix pattern
psql -d your_db -c "DELETE FROM chunks WHERE file_path NOT LIKE '%/%/%';"

# 4. Re-index via workspace
grepai watch --workspace my-workspace
```

**Note:** If you don't clean old chunks, searches may return duplicate results (one with prefix, one without). This only affects the transition period.

## Troubleshooting

### "GOB backend not supported"

Workspaces require a shared backend. Configure PostgreSQL or Qdrant:

```bash
# Create workspace with PostgreSQL
grepai workspace create my-workspace
# Select PostgreSQL when prompted
```

### "Workspace not found"

Check workspace configuration:

```bash
grepai workspace list
grepai workspace show my-workspace
```

### "Project path does not exist"

Ensure project paths are absolute and exist:

```bash
grepai workspace status my-workspace
```

### Background watcher not starting

Check logs:

```bash
# Default log location
cat ~/Library/Logs/grepai/grepai-workspace-my-workspace.log  # macOS
cat ~/.local/state/grepai/logs/grepai-workspace-my-workspace.log  # Linux
```
