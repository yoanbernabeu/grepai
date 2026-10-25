---
title: MCP Integration
description: Run grepai as an MCP server so Claude Code, Cursor, Windsurf or OpenCode can call semantic search, call tracing and index status as native tools.
---

`grepai mcp-serve` starts a local MCP server. Your AI agent can then call grepai search, trace and index tools directly instead of running shell commands.

## Quick start

1. Index your project (see [Prerequisites](#prerequisites)):

   ```bash
   grepai init
   grepai watch
   ```

2. Register the server with your agent. For Claude Code:

   ```bash
   claude mcp add grepai -- grepai mcp-serve
   ```

3. Restart the agent and ask a question such as "Where is user authentication handled?". The agent calls `grepai_search`.

For other agents, see [Configuration](#configuration).

## What is MCP?

[Model Context Protocol (MCP)](https://modelcontextprotocol.io/) is an open standard that lets AI agents call external tools. grepai works with any MCP client. This page shows the setup for Claude Code, Cursor, Windsurf and OpenCode.

## Benefits

- **Native tools**: the agent sees typed tools with documented parameters, not a shell command to guess.
- **Structured output**: tools return JSON by default, or TOON (a more compact format) with `format: "toon"`.
- **Local only**: the server talks to the agent over stdio. It opens no network port.

## Available Tools

The server registers 14 tools.

| Tool | What it does | Parameters |
|------|--------------|------------|
| `grepai_search` | Semantic code search with natural language | `query` (required), `limit` (default 10), `compact` (default false), `format`, `path`, `workspace`, `projects`, `root` |
| `grepai_trace_callers` | Find the functions that call a symbol | `symbol` (required), `compact` (default false), `format`, `workspace`, `project`, `root` |
| `grepai_trace_callees` | Find the functions a symbol calls | `symbol` (required), `compact` (default false), `format`, `workspace`, `project`, `root` |
| `grepai_trace_graph` | Build a call graph (callers and callees) around a symbol | `symbol` (required), `depth` (default 2), `format`, `workspace`, `project`, `root` |
| `grepai_refs_readers` | Find where a property or state symbol is read | `symbol` (required), `compact` (default false), `format`, `workspace`, `project`, `root` |
| `grepai_refs_writers` | Find where a property or state symbol is written | `symbol` (required), `compact` (default false), `format`, `workspace`, `project`, `root` |
| `grepai_refs_graph` | Combine readers and writers into a usage graph | `symbol` (required), `compact` (default false), `format`, `workspace`, `project`, `root` |
| `grepai_index_status` | Report index health: files, chunks, size, provider, model, symbol and RPG status | `verbose`, `format`, `workspace`, `root` |
| `grepai_list_workspaces` | List workspace names | `format` |
| `grepai_list_projects` | List the projects in a workspace | `workspace`, `format` |
| `grepai_rpg_search` | Search [RPG](/grepai/rpg/) nodes | `query` (required), `scope`, `kinds` (default `symbol`), `limit` (default 10), `format` |
| `grepai_rpg_fetch` | Fetch one RPG node with its hierarchy and edges | `node_id` (required), `format` |
| `grepai_rpg_explore` | Walk the RPG graph from a node | `start_node_id` (required), `direction` (default `both`), `depth` (default 2), `edge_types`, `limit` (default 100), `format` |
| `grepai_stats` | Summarize token savings recorded in `.grepai/stats.json` | `history` (default false), `limit` (default 30 days, used with `history`) |

### Parameter reference

- `format`: `json` (default) or `toon`. Any other value returns an error.
- `compact`: omit code content (search) or call-site context (trace, refs) to save tokens.
- `path` (search only): keep results whose path starts with this prefix. With `projects`, the prefix is relative to each project root, not the workspace root.
- `workspace`: run against a [workspace](/grepai/workspace/) instead of the local project. Use `grepai_list_workspaces` to find names.
- `projects` (search) and `project` (trace, refs): limit a workspace query to some projects. `projects` is a comma-separated list. Both require `workspace`.
- `root`: absolute path of a project directory. The tool loads that project's config and index instead of the one the server started in, so one server can serve several projects without a restart. You cannot pass both `root` and `workspace`.
- `kinds` (`grepai_rpg_search`): comma-separated list of `area`, `category`, `subcategory`, `file`, `symbol`, `chunk`.
- `direction` (`grepai_rpg_explore`): `forward`, `reverse` or `both`.
- `edge_types` (`grepai_rpg_explore`): comma-separated list of `feature_parent`, `contains`, `invokes`, `imports`, `maps_to_chunk`, `semantic_sim`.
- `verbose` (`grepai_index_status`): accepted, but it does not change the output today.

The RPG tools and `grepai_stats` always use the project the server started in. The RPG tools also need RPG enabled in that project (see [RPG](/grepai/rpg/)).

## Configuration

All clients run the same command: `grepai mcp-serve [project-path] [--workspace <name>]`. See the [`mcp-serve` reference](/grepai/commands/grepai_mcp-serve/).

### Claude Code

Register the server with `claude mcp add`:

```bash
# Use the project found from the current directory
claude mcp add grepai -- grepai mcp-serve

# Use an explicit project path
claude mcp add grepai -- grepai mcp-serve /path/to/project

# Search all projects of a workspace by default
claude mcp add grepai -- grepai mcp-serve --workspace my-fullstack
```

Pick a scope with `-s`:

```bash
# User scope: available in all your Claude Code sessions
claude mcp add grepai -s user -- grepai mcp-serve --workspace my-fullstack

# Project scope: written to .mcp.json in the project
claude mcp add grepai -s project -- grepai mcp-serve
```

### Project `.mcp.json`

To share the setup with your team, commit a `.mcp.json` file at the project root:

```json
{
  "mcpServers": {
    "grepai": {
      "command": "grepai",
      "args": ["mcp-serve"]
    }
  }
}
```

For workspace mode, add the flag to `args`:

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

### Cursor

Add this to `.cursor/mcp.json` in your project:

```json
{
  "mcpServers": {
    "grepai": {
      "command": "grepai",
      "args": ["mcp-serve"]
    }
  }
}
```

On Windows, pass the project path explicitly:

```json
{
  "mcpServers": {
    "grepai": {
      "command": "grepai",
      "args": ["mcp-serve", "C:\\path\\to\\your\\project"]
    }
  }
}
```

### Windsurf

Add the same block to your Windsurf MCP configuration file:

```json
{
  "mcpServers": {
    "grepai": {
      "command": "grepai",
      "args": ["mcp-serve"]
    }
  }
}
```

### Opencode

Add a `mcp` entry to `opencode.json` at the project root, or to your global OpenCode config in `~/.config/opencode/`:

```json
{
  "mcp": {
    "grepai": {
      "type": "local",
      "enabled": true,
      "command": ["grepai", "mcp-serve"]
    }
  }
}
```

## Workspace Mode

Start the server with `--workspace` to search across all projects of a workspace by default:

```bash
grepai mcp-serve --workspace my-fullstack
```

The server then fills in `workspace` for every tool that accepts it. The agent can call `grepai_search` with only a `query`. The workspace must exist in `~/.grepai/workspace.yaml`. If the current directory also holds a grepai project, the server keeps it for `grepai_stats` and the RPG tools.

**How the server picks its target:**

1. `--workspace <name>`, if set.
2. The `project-path` argument, if given. It must contain `.grepai/config.yaml`.
3. The nearest `.grepai/config.yaml`, walking up from the current directory.
4. A workspace that contains the current directory.
5. If none match but `~/.grepai/workspace.yaml` defines at least one workspace, the server starts with no default target. The agent then passes `workspace` in each request.

If nothing matches, `mcp-serve` exits with an error asking you to run `grepai init` or use `--workspace`.

## Usage

Once the server is configured, the agent calls the tools itself. Example calls:

**Search:**

```text
Tool: grepai_search
Arguments: {"query": "user authentication flow", "limit": 5}
```

**Search in some workspace projects, under a path:**

```text
Tool: grepai_search
Arguments: {"query": "request validation", "workspace": "my-fullstack", "projects": "backend", "path": "api/"}
```

**Trace callers:**

```text
Tool: grepai_trace_callers
Arguments: {"symbol": "HandleLogin"}
```

**RPG search:**

```text
Tool: grepai_rpg_search
Arguments: {"query": "command line search flow", "kinds": "symbol,file", "limit": 5}
```

**Index status:**

```text
Tool: grepai_index_status
Arguments: {}
```

## Prerequisites

1. `grepai` is installed and on your `PATH`.
2. The project is initialized (`grepai init`), or a workspace exists (`grepai workspace create`).
3. The index is built: `grepai watch` (or `grepai watch --workspace my-fullstack`). Trace and refs tools also need the symbol index that `grepai watch` builds.

## Troubleshooting

### Tool not appearing

- Check that grepai is on your `PATH`: `which grepai`.
- Check the syntax of your MCP configuration file.
- Restart the agent after changing the configuration.

### Search returns no results

- Check the index: `grepai status`.
- Run `grepai watch` to build or update it.
- Trace and refs tools return `symbol index is empty` until `grepai watch` has run.

### Connection errors

- The server uses stdio, so there is no port or network setting to check.
- Run `grepai mcp-serve` in a terminal from the project directory. It should start and wait for input without printing an error. Press Ctrl+C to stop it.
