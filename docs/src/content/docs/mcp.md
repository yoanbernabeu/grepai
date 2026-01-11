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
| `grepai_search` | Semantic code search | `query` (required), `limit` (default: 10) |
| `grepai_trace_callers` | Find callers of a symbol | `symbol` (required) |
| `grepai_trace_callees` | Find callees of a symbol | `symbol` (required) |
| `grepai_trace_graph` | Build complete call graph | `symbol` (required), `depth` (default: 2) |
| `grepai_index_status` | Check index health | none |

## Configuration

### Claude Code

Use the `claude mcp add` command to register grepai as an MCP server:

```bash
claude mcp add grepai -- grepai mcp-serve
```

This automatically configures grepai in your Claude Code settings.

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

## Usage

Once configured, AI agents can use grepai tools directly:

**Search example:**
```
Tool: grepai_search
Arguments: {"query": "user authentication flow", "limit": 5}
```

**Trace callers example:**
```
Tool: grepai_trace_callers
Arguments: {"symbol": "HandleLogin"}
```

**Index status example:**
```
Tool: grepai_index_status
Arguments: {}
```

## Prerequisites

Before using MCP mode, ensure:

1. grepai is installed and in your PATH
2. The project has been initialized (`grepai init`)
3. The index has been built (`grepai watch`)

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
