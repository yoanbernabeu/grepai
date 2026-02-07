---
title: MCP Integration
description: Use grepai as a native MCP tool for AI agents
---

grepai includes a built-in MCP (Model Context Protocol) server that allows AI agents to use semantic code search as a native tool.

## What is MCP?

[Model Context Protocol (MCP)](https://modelcontextprotocol.io/) is an open standard for AI tool integration, supported by:

- Claude Code
- Cursor
- Windsurf
- Continue
- Other MCP-compatible AI tools

## Benefits

- **Native tool access**: AI models see grepai as a first-class tool, not a shell command
- **Subagent inheritance**: MCP tools are automatically available to subagents
- **Structured data**: JSON responses by default, no parsing required
- **Tool discovery**: MCP tools are automatically discovered by AI models

## Available Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `grepai_search` | Semantic code search | `query` (required), `limit` (default: 10), `compact` (default: false) |
| `grepai_trace_callers` | Find callers of a symbol | `symbol` (required), `workspace`, `project`, `compact` (default: false) |
| `grepai_trace_callees` | Find callees of a symbol | `symbol` (required), `workspace`, `project`, `compact` (default: false) |
| `grepai_trace_graph` | Build complete call graph | `symbol` (required), `workspace`, `project`, `depth` (default: 2) |
| `grepai_index_status` | Check index health | `verbose` (optional, default: false), `workspace` |

## Configuration

### Claude Code

Use the `claude mcp add` command to register grepai as an MCP server:

```bash
# Single project (auto-detects from current directory)
claude mcp add grepai -- grepai mcp-serve

# Explicit project path
claude mcp add grepai -- grepai mcp-serve /path/to/project

# Workspace mode (auto-searches all workspace projects)
claude mcp add grepai -- grepai mcp-serve --workspace my-fullstack
```

**Scope options:**

```bash
# User-level (available in all Claude Code sessions)
claude mcp add grepai -s user -- grepai mcp-serve --workspace my-fullstack

# Project-level (this project only)
claude mcp add grepai -s project -- grepai mcp-serve
```

### Project `.mcp.json`

For team-shareable configuration, create `.mcp.json` at the project root:

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

With workspace mode:

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

This file can be committed to git so the entire team shares the same MCP configuration.

### Cursor

Add to `.cursor/mcp.json` in your project:

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

### Windsurf

Add to your Windsurf MCP configuration:

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

Add to `.config/opencode/opencode.jsonc` MCP section, or `[YourProject]/opencode.json`

```json
{"mcp" : {
  "grepai": {
    "type": "local",
    "enabled": true,
    "command": ["grepai", "mcp-serve"]
    }
  }
}
```

## Workspace Mode

When started with the `--workspace` flag, the MCP server automatically injects the workspace into search requests. This means AI agents can use `grepai_search` without specifying the `workspace` parameter — cross-project search works by default.

```bash
grepai mcp-serve --workspace my-fullstack
```

**How auto-detection works:**

Without `--workspace`, the MCP server resolves its target in this order:

1. Explicit project path argument (if provided)
2. Walk upward from current directory looking for `.grepai/config.yaml`
3. Auto-detect workspace by checking if current directory is within a workspace project

With `--workspace`, the server skips auto-detection and uses the specified workspace directly. The `grepai_search` tool will search across all workspace projects without the agent needing to pass `workspace` or `projects` parameters.

## Usage

Once configured, AI agents can use grepai tools directly:

**Search example:**

```text
Tool: grepai_search
Arguments: {"query": "user authentication flow", "limit": 5}
```

**Trace callers example:**

```text
Tool: grepai_trace_callers
Arguments: {"symbol": "HandleLogin"}
```

**Index status example:**

```text
Tool: grepai_index_status
Arguments: {}
```

## Prerequisites

Before using MCP mode, ensure:

1. grepai is installed and in your PATH
2. The project has been initialized (`grepai init`) or a workspace is configured (`grepai workspace create`)
3. The index has been built (`grepai watch` or `grepai watch --workspace my-fullstack`)

## Troubleshooting

### Tool not appearing

- Verify grepai is in your PATH: `which grepai`
- Check MCP configuration file syntax
- Restart your AI tool after configuration changes

### Search returns no results

- Ensure the index is built: `grepai status`
- Run `grepai watch` to build/update the index

### Connection errors

- MCP server uses stdio transport (local process communication)
- No network ports are opened
- Check that `grepai mcp-serve` runs without errors when invoked directly
