---
title: AI Agent Skills
description: Install ready-to-use skills to help your AI agent master grepai
---

AI coding agents like Claude Code, Cursor, or Windsurf work better when they understand the tools at their disposal. **grepai-skills** is a collection of 27 skills that teach your AI agent how to use grepai effectively.

## What are Skills?

Skills are knowledge modules that AI agents can load to understand how to use specific tools. Instead of the agent guessing how to use grepai, skills provide:

- Step-by-step instructions for common workflows
- Best practices for writing effective search queries
- Troubleshooting guides for common issues
- Configuration examples for different use cases

## Installation

Install all 27 skills with a single command:

```bash
npx skills add yoanbernabeu/grepai-skills
```

This works with Claude Code, Cursor, Codex, OpenCode, Windsurf, and 30+ other AI agents.

### Install specific skills

```bash
# Install only search-related skills
npx skills add yoanbernabeu/grepai-skills --skill grepai-search-basics

# Install globally (available in all projects)
npx skills add yoanbernabeu/grepai-skills -g

# List all available skills
npx skills add yoanbernabeu/grepai-skills --list

# Install to specific agents
npx skills add yoanbernabeu/grepai-skills -a claude-code -a cursor

# Non-interactive (CI/CD friendly)
npx skills add yoanbernabeu/grepai-skills --all -y
```

### Claude Code Plugin

```bash
/plugin marketplace add yoanbernabeu/grepai-skills
/plugin install grepai-complete@grepai-skills
```

### Manual installation

Copy the `skills/` directory from the repository to:
- **Global**: `~/.claude/skills/` (or `~/.cursor/skills/`, etc.)
- **Project**: `.claude/skills/` (or `.cursor/skills/`, etc.)

### Supported AI Agents

| Agent | Project Path | Global Path |
|-------|--------------|-------------|
| Claude Code | `.claude/skills/` | `~/.claude/skills/` |
| Cursor | `.cursor/skills/` | `~/.cursor/skills/` |
| Codex | `.codex/skills/` | `~/.codex/skills/` |
| OpenCode | `.opencode/skill/` | `~/.config/opencode/skill/` |
| Windsurf | `.windsurf/skills/` | `~/.windsurf/skills/` |

## Skill Packs

Install skills by category:

| Pack | Skills | Description |
|------|--------|-------------|
| `grepai-getting-started` | 3 | Installation, Ollama setup, quickstart |
| `grepai-configuration` | 3 | Init, config reference, ignore patterns |
| `grepai-embeddings` | 3 | Ollama, OpenAI, LM Studio providers |
| `grepai-storage` | 3 | GOB, PostgreSQL, Qdrant backends |
| `grepai-indexing` | 2 | Watch daemon, chunking |
| `grepai-search` | 4 | Basics, advanced, tips, boosting |
| `grepai-trace` | 3 | Callers, callees, graphs |
| `grepai-integration` | 3 | Claude Code, Cursor, MCP tools |
| `grepai-advanced` | 3 | Workspaces, languages, troubleshooting |
| **`grepai-complete`** | **27** | **All skills — complete toolkit** |

## Available Skills

### Getting Started
| Skill | Description |
|-------|-------------|
| `grepai-installation` | Multi-platform installation (Homebrew, shell, Windows) |
| `grepai-ollama-setup` | Install and configure Ollama for local embeddings |
| `grepai-quickstart` | Get searching in 5 minutes |

### Configuration
| Skill | Description |
|-------|-------------|
| `grepai-init` | Initialize grepai in a project |
| `grepai-config-reference` | Complete configuration reference |
| `grepai-ignore-patterns` | Exclude files and directories from indexing |

### Embeddings Providers
| Skill | Description |
|-------|-------------|
| `grepai-embeddings-ollama` | Configure Ollama for local, private embeddings |
| `grepai-embeddings-openai` | Configure OpenAI for cloud embeddings |
| `grepai-embeddings-lmstudio` | Configure LM Studio with GUI interface |
| `grepai-embeddings-voyageai` | Configure Voyage AI for code-optimized cloud embeddings |

### Storage Backends
| Skill | Description |
|-------|-------------|
| `grepai-storage-gob` | Local file storage (default, simple) |
| `grepai-storage-postgres` | PostgreSQL + pgvector for teams |
| `grepai-storage-qdrant` | Qdrant for high-performance search |

### Indexing
| Skill | Description |
|-------|-------------|
| `grepai-watch-daemon` | Configure and manage the watch daemon |
| `grepai-chunking` | Optimize how code is split for embedding |

### Semantic Search
| Skill | Description |
|-------|-------------|
| `grepai-search-basics` | Basic semantic code search |
| `grepai-search-advanced` | JSON output, compact mode, AI integration |
| `grepai-search-tips` | Write effective search queries |
| `grepai-search-boosting` | Prioritize source code over tests |

### Call Graph Analysis
| Skill | Description |
|-------|-------------|
| `grepai-trace-callers` | Find all callers of a function |
| `grepai-trace-callees` | Find all functions called by a function |
| `grepai-trace-graph` | Build complete dependency graphs |

### AI Agent Integration
| Skill | Description |
|-------|-------------|
| `grepai-mcp-claude` | Integrate with Claude Code via MCP |
| `grepai-mcp-cursor` | Integrate with Cursor IDE via MCP |
| `grepai-mcp-tools` | Reference for all MCP tools |

### Advanced
| Skill | Description |
|-------|-------------|
| `grepai-workspaces` | Multi-project workspace management |
| `grepai-languages` | Supported programming languages |
| `grepai-troubleshooting` | Diagnose and fix common issues |

## Usage

Once installed, just ask your AI agent naturally:

```
"Help me install and configure grepai"

"Search for authentication code in this project"

"What functions call the Login function?"

"Why are my search results poor?"

"Configure grepai to use OpenAI embeddings"
```

The agent will automatically use the relevant skills to provide accurate guidance.

## Resources

- **Skills Repository**: [github.com/yoanbernabeu/grepai-skills](https://github.com/yoanbernabeu/grepai-skills) — Contributions welcome!
- **Skills CLI**: [github.com/vercel-labs/skills](https://github.com/vercel-labs/skills) — The open agent skills tool
- **Skills Directory**: [skills.sh](https://skills.sh) — Browse all available skills
