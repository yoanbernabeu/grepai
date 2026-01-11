---
title: Claude Code Subagent
description: Use grepai as a specialized exploration agent in Claude Code
---

Claude Code uses subagents for specialized tasks. grepai provides a ready-to-use exploration subagent that leverages semantic search and call graph tracing.

## Why a Subagent?

When Claude Code spawns an "Explore" subagent:

- The subagent operates in an isolated context
- It doesn't inherit CLAUDE.md instructions
- It uses standard Grep/Glob instead of semantic search

The grepai deep-explore subagent solves this by providing direct access to grepai tools.

## Installation

```bash
grepai agent-setup --with-subagent
```

This creates `.claude/agents/deep-explore.md` in your project.

## What It Does

The deep-explore subagent provides:

| Capability | Tool | Description |
|------------|------|-------------|
| Semantic Search | `grepai search` | Find code by intent, not just text |
| Call Graph | `grepai trace` | Understand function relationships |
| Standard Tools | Grep, Glob, Read | Available as fallback |

## Usage

Claude Code automatically selects the deep-explore agent when:

- Exploring unfamiliar code areas
- Understanding code architecture
- Finding implementations by intent
- Analyzing function relationships

You can also explicitly request it:

> "Use the deep-explore agent to understand the authentication flow"

## Subagent vs MCP

| Feature | Subagent | MCP |
|---------|----------|-----|
| Setup | `--with-subagent` flag | Separate MCP config |
| Access | Bash commands | Native tools |
| Context | Isolated | Shared |
| Use case | Exploration tasks | All tasks |

Both approaches complement each other. MCP provides native tool access, while the subagent ensures exploration tasks use grepai.

## Manual Setup

If you prefer manual setup, create `.claude/agents/deep-explore.md`:

```yaml
---
name: deep-explore
description: Deep codebase exploration using grepai semantic search and call graph tracing.
tools: Read, Grep, Glob, Bash
model: inherit
---

## Instructions

You are a specialized code exploration agent with access to grepai.

### Primary Tools

Use `grepai search` for semantic code search:
- grepai search "authentication flow"
- grepai search "error handling"

Use `grepai trace` for call graph analysis:
- grepai trace callers "Login"
- grepai trace callees "HandleRequest"
- grepai trace graph "ProcessOrder" --depth 3

### Workflow

1. Start with grepai search to find relevant code
2. Use grepai trace to understand function relationships
3. Use Read to examine files in detail
4. Synthesize findings into a clear summary
```

## Troubleshooting

### Subagent not appearing

- Verify the file exists: `cat .claude/agents/deep-explore.md`
- Restart Claude Code after creating the file
- Ensure the YAML frontmatter is valid

### grepai commands failing in subagent

- Ensure grepai is in your PATH
- Verify the index is built: `grepai status`
- Run `grepai watch` to build/update the index
