---
title: Claude Code Subagent
description: Add a deep-explore subagent to Claude Code that explores your code with grepai search, trace and refs instead of plain text search.
---

grepai can install a Claude Code subagent named `deep-explore`. It explores code with `grepai search`, `grepai trace` and `grepai refs`, and falls back to Grep and Glob for exact text.

## Installation

Run this at the root of your project:

```bash
grepai agent-setup --with-subagent
```

This creates `.claude/agents/deep-explore.md`. If the file already defines `name: deep-explore`, grepai leaves it unchanged.

The same command also appends grepai usage instructions to any agent files that already exist in the project (`.cursor/rules`, `.cursorrules`, `.windsurfrules`, `CLAUDE.md`, `.claude/settings.md`, `GEMINI.md`, `AGENTS.md`). It does not create those files. See the [`agent-setup` reference](/grepai/commands/grepai_agent-setup/).

Restart Claude Code so it loads the new subagent.

## Why a Subagent?

Claude Code delegates exploration work to subagents. Each subagent runs in its own context, with its own instructions and tool list. A generic exploration subagent knows nothing about grepai, so it searches with plain text tools.

`deep-explore` comes with grepai instructions built in, so exploration tasks use semantic search and call tracing.

## What It Does

| Capability | Command | Use it for |
|------------|---------|------------|
| Semantic search | `grepai search` | Find code by intent, not exact text |
| Call graph | `grepai trace callers`, `callees`, `graph` | See who calls a function and what it calls |
| Property usage | `grepai refs readers`, `writers` | Find where a property or state key is read or written |
| Standard tools | `Read`, `Grep`, `Glob` | Read files, match exact strings and path patterns |

The subagent runs grepai through the `Bash` tool. Its tool list is `Read, Grep, Glob, Bash`, and it inherits the model of the main session (`model: inherit`).

## Usage

Claude Code picks a subagent from its `description`. The `deep-explore` description covers:

- Understanding code architecture
- Finding implementations by intent
- Analyzing function relationships
- Exploring unfamiliar code areas

You can also ask for it by name:

> "Use the deep-explore agent to understand the authentication flow"

## Subagent vs MCP

| | Subagent | [MCP](/grepai/mcp/) |
|---|----------|-----|
| Setup | `grepai agent-setup --with-subagent` | `claude mcp add grepai -- grepai mcp-serve` |
| How grepai is called | CLI commands through `Bash` | Native MCP tools (`grepai_search`, ...) |
| Who uses it | The `deep-explore` subagent | Any session or agent that has the MCP tools |
| Agents supported | Claude Code | Any MCP client |

You can use both. MCP gives the main session native tools. The subagent makes sure delegated exploration also goes through grepai.

## Manual Setup

To write the file yourself, create `.claude/agents/deep-explore.md`. This is the content `agent-setup` writes:

````markdown
---
name: deep-explore
description: Deep codebase exploration using grepai semantic search and call graph tracing. Use this agent for understanding code architecture, finding implementations by intent, analyzing function relationships, and exploring unfamiliar code areas.
tools: Read, Grep, Glob, Bash
model: inherit
---

## Instructions

You are a specialized code exploration agent with access to grepai semantic search and call graph tracing.

### Primary Tools

#### 1. Semantic Search: `grepai search`

Use this to find code by intent and meaning:

```bash
# Use English queries for best results (--compact saves ~80% tokens)
grepai search "authentication flow" --json --compact
grepai search "error handling middleware" --json --compact
grepai search "database connection management" --json --compact
```

#### 2. Call Graph Tracing: `grepai trace`

Use this to understand function relationships and code flow:

```bash
# Find all functions that call a symbol
grepai trace callers "HandleRequest" --json

# Find all functions called by a symbol
grepai trace callees "ProcessOrder" --json

# Build complete call graph
grepai trace graph "ValidateToken" --depth 3 --json
```

#### 3. Property/Data Usage Tracing: `grepai refs`

Use this when the target is a property/state key rather than a function call:

```bash
# Find readers of a property
grepai refs readers "uid" --json

# Find writers of a property
grepai refs writers "uid" --json
```

Use `grepai trace` when you need to:
- Find all callers of a function
- Understand the call hierarchy
- Analyze the impact of changes to a function
- Map dependencies between components

### When to use standard tools

Only fall back to Grep/Glob when:
- You need exact text matching (variable names, imports)
- grepai is not available or returns errors
- You need file path patterns

### Workflow

1. Start with `grepai search` to find relevant code semantically
2. Use `grepai trace` to understand function relationships and call graphs
3. Use `grepai refs` for property/state readers and writers
4. Use `Read` to examine promising files in detail
5. Use Grep only for exact string searches if needed
6. Synthesize findings into a clear summary
````

> **Note:** `grepai search`, `grepai trace` and `grepai refs` also accept `--toon`, a more compact output format than `--json`. You can swap it in if you want to save tokens.

## Troubleshooting

### Subagent not appearing

- Check that the file exists: `cat .claude/agents/deep-explore.md`.
- Check that the YAML frontmatter at the top of the file is valid.
- Restart Claude Code.

### grepai commands failing in subagent

- Check that grepai is on your `PATH`: `which grepai`.
- Check the index: `grepai status`.
- Run `grepai watch` to build or update the index. `grepai trace` and `grepai refs` also need the symbol index it builds.
