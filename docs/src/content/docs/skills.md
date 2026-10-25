---
title: AI Agent Skills
description: Install grepai-skills, a set of 27 agent skills that teach Claude Code, Cursor and other AI agents how to install, configure and use grepai.
---

[grepai-skills](https://github.com/yoanbernabeu/grepai-skills) is a set of 27 skills that teach your AI agent how to install, configure and use grepai. Install them once, then ask your agent for help in plain language.

## Installation

Install all skills with the [skills CLI](https://github.com/vercel-labs/skills):

```bash
npx skills add yoanbernabeu/grepai-skills
```

The CLI detects your installed agents (Claude Code, Cursor, Codex, OpenCode, Windsurf and others) and asks where to install.

### Install specific skills

These options come from the skills CLI:

```bash
# Install one skill
npx skills add yoanbernabeu/grepai-skills --skill grepai-search-basics

# Install globally (available in all projects)
npx skills add yoanbernabeu/grepai-skills -g

# List the skills without installing
npx skills add yoanbernabeu/grepai-skills --list

# Install for specific agents
npx skills add yoanbernabeu/grepai-skills -a claude-code -a cursor

# Install everything without prompts (CI friendly)
npx skills add yoanbernabeu/grepai-skills --all -y
```

### Claude Code Plugin

The repository is also a Claude Code plugin marketplace. In Claude Code, run:

```bash
/plugin marketplace add yoanbernabeu/grepai-skills
/plugin install grepai-complete@grepai-skills
```

Replace `grepai-complete` with any pack from [Skill Packs](#skill-packs) to install only that category.

### Manual installation

Copy the `skills/` directory of the repository into your agent's skills folder, for example:

- **Project**: `.claude/skills/`
- **Global**: `~/.claude/skills/`

### Supported AI Agents

The skills CLI decides the install folder for each agent, and its list of agents changes over time. See the [skills CLI agent list](https://github.com/vercel-labs/skills/blob/main/docs/agents.md) for current folders. For Claude Code:

| Agent | Project Path | Global Path |
|-------|--------------|-------------|
| Claude Code | `.claude/skills/` | `~/.claude/skills/` |

## Usage

Once the skills are installed, ask your agent in plain language:

```text
"Help me install and configure grepai"

"Search for authentication code in this project"

"What functions call the Login function?"

"Why are my search results poor?"

"Configure grepai to use OpenAI embeddings"
```

The agent loads the matching skill and follows its instructions.

## What are Skills?

A skill is a folder with a `SKILL.md` file. It gives the agent instructions for one task. The grepai skills cover:

- Step-by-step workflows (install, init, index, search, trace)
- How to write effective search queries
- Configuration examples for each embedder and store
- Fixes for common problems

## Skill Packs

The Claude Code plugin groups the skills into packs:

| Pack | Skills | Contents |
|------|--------|----------|
| `grepai-getting-started` | 3 | Installation, Ollama setup, quickstart |
| `grepai-configuration` | 3 | Init, config reference, ignore patterns |
| `grepai-embeddings` | 3 | Ollama, OpenAI, LM Studio providers |
| `grepai-storage` | 3 | GOB, PostgreSQL, Qdrant backends |
| `grepai-indexing` | 2 | Watch daemon, chunking |
| `grepai-search` | 4 | Basics, advanced, tips, boosting |
| `grepai-trace` | 3 | Callers, callees, graphs |
| `grepai-integration` | 3 | Claude Code, Cursor, MCP tools |
| `grepai-advanced` | 3 | Workspaces, languages, troubleshooting |
| `grepai-complete` | 27 | All skills |

## Available Skills

### Getting Started

| Skill | Description |
|-------|-------------|
| `grepai-installation` | Install grepai with Homebrew, the shell script or on Windows |
| `grepai-ollama-setup` | Install and configure Ollama for local embeddings |
| `grepai-quickstart` | Go from install to first search |

### Configuration

| Skill | Description |
|-------|-------------|
| `grepai-init` | Initialize grepai in a project |
| `grepai-config-reference` | Full configuration reference |
| `grepai-ignore-patterns` | Exclude files and directories from indexing |

### Embeddings Providers

| Skill | Description |
|-------|-------------|
| `grepai-embeddings-ollama` | Use Ollama for local embeddings |
| `grepai-embeddings-openai` | Use OpenAI for cloud embeddings |
| `grepai-embeddings-lmstudio` | Use LM Studio for local embeddings |

### Storage Backends

| Skill | Description |
|-------|-------------|
| `grepai-storage-gob` | Local file storage (the default) |
| `grepai-storage-postgres` | PostgreSQL with pgvector |
| `grepai-storage-qdrant` | Qdrant vector database |

### Indexing

| Skill | Description |
|-------|-------------|
| `grepai-watch-daemon` | Run and manage the watch daemon |
| `grepai-chunking` | Tune how code is split before embedding |

### Semantic Search

| Skill | Description |
|-------|-------------|
| `grepai-search-basics` | Basic semantic code search |
| `grepai-search-advanced` | JSON output, compact mode, AI integration |
| `grepai-search-tips` | Write effective search queries |
| `grepai-search-boosting` | Rank source code above tests |

### Call Graph Analysis

| Skill | Description |
|-------|-------------|
| `grepai-trace-callers` | Find all callers of a function |
| `grepai-trace-callees` | Find all functions a function calls |
| `grepai-trace-graph` | Build a call graph around a function |

### AI Agent Integration

| Skill | Description |
|-------|-------------|
| `grepai-mcp-claude` | Connect grepai to Claude Code through MCP |
| `grepai-mcp-cursor` | Connect grepai to Cursor through MCP |
| `grepai-mcp-tools` | Reference for the MCP tools |

### Advanced

| Skill | Description |
|-------|-------------|
| `grepai-workspaces` | Manage multi-project workspaces |
| `grepai-languages` | Supported programming languages |
| `grepai-troubleshooting` | Diagnose and fix common issues |

## Resources

- **Skills repository**: [github.com/yoanbernabeu/grepai-skills](https://github.com/yoanbernabeu/grepai-skills). Contributions are welcome.
- **Skills CLI**: [github.com/vercel-labs/skills](https://github.com/vercel-labs/skills)
- **Skills directory**: [skills.sh](https://skills.sh)
