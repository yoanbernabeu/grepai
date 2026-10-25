---
title: Workspace Management
description: Group several projects in a grepai workspace to index them into one shared PostgreSQL or Qdrant store and search or trace across all of them.
---

A workspace groups several projects (microservices, a frontend and its API, shared libraries) so you can index them into one shared vector store and search or trace across all of them at once.

## Prerequisites

Workspaces need a vector store that several projects can share:

- **PostgreSQL** with pgvector
- **Qdrant**

The GOB backend is not supported: it is a local file per project, so it cannot be shared.

## Quick Start

```bash
# 1. Create the workspace (Qdrant + Ollama defaults)
grepai workspace create my-fullstack --yes

# 2. Add projects
grepai workspace add my-fullstack ~/code/frontend
grepai workspace add my-fullstack ~/code/backend

# 3. Index and watch every project
grepai watch --workspace my-fullstack

# 4. Search across projects
grepai search --workspace my-fullstack "authentication flow"
```

### 1. Create a Workspace

Run without flags for interactive prompts (backend, then embedding provider), or add `--ui` for the full-screen wizard:

```bash
grepai workspace create my-fullstack
```

Pass any flag below to skip the prompts (useful in scripts and CI):

```bash
# Qdrant + Ollama, all defaults
grepai workspace create my-fullstack --yes

# Qdrant + Ollama with another model
grepai workspace create my-fullstack --backend qdrant --provider ollama --model bge-m3

# Qdrant + OpenAI (the API key is read from OPENAI_API_KEY)
grepai workspace create my-fullstack --backend qdrant --provider openai --model text-embedding-3-small

# PostgreSQL + Ollama
grepai workspace create my-fullstack --backend postgres --dsn "postgres://grepai:grepai@localhost:5432/grepai"

# Remote Qdrant
grepai workspace create my-fullstack --backend qdrant --qdrant-endpoint http://my-qdrant-host --qdrant-port 6334

# From a YAML or JSON file
grepai workspace create my-fullstack --from workspace-config.yaml
```

| Flag | Description | Default |
|------|-------------|---------|
| `--backend` | `qdrant` or `postgres` | Required, unless `--yes` (then `qdrant`) |
| `--provider` | `ollama`, `openai` or `lmstudio` | `ollama` |
| `--model` | Embedding model | `text-embedding-3-small` for `openai`, `nomic-embed-text` otherwise |
| `--endpoint` | Embedder endpoint | `http://localhost:11434` (Ollama), `http://127.0.0.1:1234` (LM Studio), `https://api.openai.com/v1` (OpenAI) |
| `--dsn` | PostgreSQL connection string | `postgres://localhost:5432/grepai` |
| `--qdrant-endpoint` | Qdrant host | `http://localhost` |
| `--qdrant-port` | Qdrant gRPC port | `6334` |
| `--collection` | Qdrant collection name | `workspace_<name>` |
| `--yes` | Use defaults for every value you did not set | |
| `--from` | Read `store` and `embedder` from a YAML or JSON file (other flags are ignored) | |
| `--ui` | Interactive full-screen wizard | |

With Ollama or LM Studio, the workspace records 768 dimensions. With OpenAI, it uses 4 parallel embedding workers.

Example `--from` file:

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
grepai workspace add my-fullstack ./backend
```

Relative paths are converted to absolute paths. The project name is the directory name (`frontend`, `backend`), and must be unique in the workspace. A project does not need its own `grepai init`: without a `.grepai/config.yaml`, it uses default settings.

### 3. Start Indexing

```bash
# Index, then keep watching (foreground)
grepai watch --workspace my-fullstack

# Same, in the background
grepai watch --workspace my-fullstack --background

# Index once and exit
grepai index --workspace my-fullstack
```

Chunks go to the shared store. Each project's symbol index (and RPG graph, if enabled) is written to `<project>/.grepai/`.

### 4. Search Across Projects

```bash
# All projects
grepai search --workspace my-fullstack "authentication flow"

# One project
grepai search --workspace my-fullstack --project frontend "React components"
```

## Configuration File

Workspaces are stored in `~/.grepai/workspace.yaml` (file mode `0600`, since it may hold API keys):

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
    chunking:
      custom_extensions: [".tpl"]   # optional, merged with each project's list
    projects:
      - name: frontend
        path: /path/to/frontend
      - name: backend
        path: /path/to/backend
```

## CLI Commands

### Workspace Management

```bash
grepai workspace list                     # list workspaces
grepai workspace show my-fullstack        # backend, embedder, projects
grepai workspace status my-fullstack      # checks that each project path exists
grepai workspace status                   # same, for every workspace
grepai workspace delete my-fullstack      # remove from workspace.yaml
```

`workspace delete` only removes the entry from `workspace.yaml`. Indexed data stays in PostgreSQL or Qdrant.

### Project Management

```bash
grepai workspace add my-fullstack /path/to/project
grepai workspace remove my-fullstack project-name
```

`remove` takes the project name (as shown by `workspace show`), not its path.

### Watch Commands

```bash
grepai watch --workspace my-fullstack                # foreground
grepai watch --workspace my-fullstack --background   # background
grepai watch --workspace my-fullstack --status       # background watcher status
grepai watch --workspace my-fullstack --stop         # stop the background watcher
```

### Search Commands

```bash
# All projects
grepai search --workspace my-fullstack "query"

# One or several projects (repeat --project)
grepai search --workspace my-fullstack --project frontend "query"
grepai search --workspace my-fullstack --project frontend --project backend "query"

# Path prefix inside one project
grepai search --workspace my-fullstack --project frontend --path components/ "React hooks"

# Path prefix without --project: start with the project name
grepai search --workspace my-fullstack --path frontend/src/ "query"

# Absolute path: grepai finds the matching project
grepai search --workspace my-fullstack --path /path/to/frontend/src "query"

# JSON output for AI agents
grepai search --workspace my-fullstack "query" --json
grepai search --workspace my-fullstack "query" --json --compact
```

A relative `--path` is matched against stored paths (`<workspace>/<project>/<file>`). With exactly one `--project`, it is relative to that project. Otherwise it is relative to the workspace, so it must start with a project name.

Workspace search uses the default search boost and does not use hybrid search or RPG enrichment.

`grepai trace` and `grepai refs` also accept `--workspace` and `--project`. See [Call Graph Analysis](/grepai/trace/#workspace-mode).

## MCP Integration

### Workspace-Aware MCP Server

Start the MCP server with `--workspace` so that every tool with a `workspace` parameter defaults to that workspace. No local `.grepai/` is needed in this mode.

```bash
claude mcp add grepai -- grepai mcp-serve --workspace my-fullstack
```

Or in `.mcp.json`:

```json
{
  "mcpServers": {
    "grepai": {
      "command": "grepai",
      "args": ["mcp-serve", "--workspace", "my-fullstack"]
    }
  }
}
```

Agents can then call `grepai_search` with only a `query`: the workspace is filled in for them.

### Manual Workspace Parameters

Without `--workspace`, agents pass the workspace explicitly. `grepai_list_workspaces` and `grepai_list_projects` help them discover valid names.

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

The trace and refs tools (`grepai_trace_callers`, `grepai_trace_callees`, `grepai_trace_graph`, `grepai_refs_readers`, `grepai_refs_writers`, `grepai_refs_graph`) and `grepai_index_status` accept a `workspace` parameter, and default to the server's `--workspace`. Trace and refs tools search all projects unless you pass `project`.

Each project keeps its own symbol index in `<project>/.grepai/symbols.gob`, whatever the vector store. `grepai watch --workspace` builds it.

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

Workspace chunks are stored with the workspace and project name in front of the relative path:

- Original: `/path/to/frontend/src/App.tsx`
- Stored: `my-fullstack/frontend/src/App.tsx`

This `workspaceName/projectName/relativePath` format lets grepai filter by project, keeps two workspaces with a project of the same name apart, and shows where each result comes from.

The workspace data is also scoped as a whole: PostgreSQL rows use the project ID `workspace:<name>`, and Qdrant uses the collection `workspace_<name>` unless you set `--collection`.

### Configuration Priority

| Setting | Source | Notes |
|---------|--------|-------|
| `store` | Workspace | One shared store |
| `embedder` | Workspace | Every project must produce compatible vectors |
| `chunking` (size, overlap, strategy) | Project | Can differ per project |
| `chunking.custom_extensions` | Workspace + project | Both lists are merged |
| `ignore` | Project | |
| `external_gitignore` | Project | |
| `trace`, `rpg`, `framework` | Project | |

A project without `.grepai/config.yaml` uses the defaults for the project-level settings.

## Migration from Single-Project

Existing single-project setups keep working. A workspace does not read or change their indexes.

### GOB Projects (Default)

GOB projects keep their local `.grepai/index.gob`. It is independent from the workspace index and keeps working with plain `grepai search`.

### PostgreSQL Projects

Single-project and workspace data can live in the same database without conflict:

- Single-project chunks use the absolute project root as project ID, with paths like `src/App.tsx`.
- Workspace chunks use `workspace:<name>`, with paths like `my-fullstack/frontend/src/App.tsx`.

Searches never mix the two, so there are no duplicate results.

```bash
# 1. Create a workspace on the same database
grepai workspace create my-workspace --backend postgres --dsn "postgres://localhost:5432/grepai"

# 2. Add projects
grepai workspace add my-workspace /path/to/frontend
grepai workspace add my-workspace /path/to/backend

# 3. Index through the workspace
grepai watch --workspace my-workspace

# 4. (Optional) Free space used by an old single-project index
psql -d grepai -c "DELETE FROM chunks WHERE project_id = '/path/to/frontend';"
psql -d grepai -c "DELETE FROM documents WHERE project_id = '/path/to/frontend';"
```

> **Note:** only run step 4 if you no longer use plain `grepai search` in that project. The project ID is the absolute project root with symlinks resolved.

## Troubleshooting

### Windows workspace restart re-embeds everything

On Windows, the filesystem scanner used to return backslash paths while the
workspace store used forward slashes. A `watch` restart then treated every file
as deleted and rebuilt the index. The scanner now always emits forward-slash
paths. Legacy single-project indexes that still store backslash paths are cleaned
up on the next index pass (those old keys no longer match a scanned path, so they
are removed as orphans). If you still see a mass re-embed after upgrading, stop
the watcher and run a fresh workspace index once.

### "GOB backend not supported"

The actual error reads `workspace "<name>" uses GOB backend which is not supported for multi-project workspaces; use 'postgres' or 'qdrant' instead`. Recreate the workspace with a shared backend:

```bash
grepai workspace delete my-workspace
grepai workspace create my-workspace --backend qdrant --yes
```

### "Workspace not found"

The error is `workspace "<name>" not found`. Check the exact name:

```bash
grepai workspace list
grepai workspace show my-workspace
```

### "Project path does not exist"

`workspace add` fails with `path does not exist: <path>` when the directory is missing. For existing workspaces, `status` flags moved or deleted projects with `✗ (path not found)`:

```bash
grepai workspace status my-workspace
```

`grepai watch --workspace` skips such a project with a warning (`skipping project <name>, its path does not exist`) and watches the others; it fails only when no project folder exists. Remove the stale project with `grepai workspace remove` and add it again from its new location.

### Background watcher not starting

Check the workspace log (`grepai-workspace-<name>.log` in the log directory):

```bash
cat ~/Library/Logs/grepai/grepai-workspace-my-workspace.log         # macOS
cat ~/.local/state/grepai/logs/grepai-workspace-my-workspace.log    # Linux ($XDG_STATE_HOME/grepai/logs if set)
type %LOCALAPPDATA%\grepai\logs\grepai-workspace-my-workspace.log   # Windows
```

If you started the watcher with `--log-dir`, look there instead.
