---
title: Workspace Management
description: Manage multi-project workspaces for cross-project indexing and search
---

# Workspace Management

Workspaces allow you to index and search across multiple projects using a shared vector store. This is useful when working on microservices architectures, monorepos, or related projects that you want to search together.

## Prerequisites

Workspaces require a shared vector store backend. **GOB backend is not supported** because it's file-based and cannot be shared across projects.

Supported backends for workspaces:
- **PostgreSQL** with pgvector (recommended for production)
- **Qdrant** (recommended for advanced vector search)

## Quick Start

### 1. Create a Workspace

```bash
grepai workspace create my-fullstack
```

This will interactively prompt you to configure:
- Storage backend (PostgreSQL or Qdrant)
- Embedding provider (Ollama, OpenAI, or LM Studio)

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

# JSON output for AI agents
grepai search --workspace my-fullstack "query" --json
grepai search --workspace my-fullstack "query" --json --compact
```

## MCP Integration

The MCP server supports workspace parameters for cross-project search:

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
