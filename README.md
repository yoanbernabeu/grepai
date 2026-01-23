# grepai

[![Go](https://github.com/yoanbernabeu/grepai/actions/workflows/ci.yml/badge.svg)](https://github.com/yoanbernabeu/grepai/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/yoanbernabeu/grepai)](https://goreportcard.com/report/github.com/yoanbernabeu/grepai)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Documentation](https://img.shields.io/badge/docs-yoanbernabeu.github.io%2Fgrepai-blue)](https://yoanbernabeu.github.io/grepai/)

> **[Full documentation available here](https://yoanbernabeu.github.io/grepai/)** — Installation guides, configuration options, AI agent integration, and more.

**A privacy-first, CLI-native way to semantically search your codebase.**

Search code by *what it does*, not just what it's called. `grepai` indexes the meaning of your code using vector embeddings, enabling natural language queries that find conceptually related code—even when naming conventions vary.

## Why grepai?

`grep` was built in 1973 for exact text matching. Modern codebases need semantic understanding.

|                      | `grep` / `ripgrep`           | `grepai`                          |
|----------------------|------------------------------|-----------------------------------|
| **Search type**      | Exact text / regex           | Semantic understanding            |
| **Query**            | `"func.*Login"`              | `"user authentication flow"`      |
| **Finds**            | Exact pattern matches        | Conceptually related code         |
| **AI Agent context** | Requires many searches       | Fewer, more relevant results      |

### Built for AI Agents

grepai is designed to provide **high-quality context** to AI coding assistants. By returning semantically relevant code chunks, your agents spend less time searching and more time coding.

## Getting Started

### Installation

```bash
curl -sSL https://raw.githubusercontent.com/yoanbernabeu/grepai/main/install.sh | sh
```

Or download from [Releases](https://github.com/yoanbernabeu/grepai/releases).

### Quick Start

```bash
grepai init                        # Initialize in your project
grepai watch                       # Start background indexing daemon
grepai search "error handling"     # Search semantically
grepai trace callers "Login"       # Find who calls a function
```

## Commands

| Command                  | Description                            |
|--------------------------|----------------------------------------|
| `grepai init`            | Initialize grepai in current directory |
| `grepai watch`           | Start real-time file watcher daemon    |
| `grepai search <query>`  | Search codebase with natural language  |
| `grepai trace <cmd>`     | Analyze call graph (callers/callees)   |
| `grepai status`          | Browse index state interactively       |
| `grepai agent-setup`     | Configure AI agents integration        |
| `grepai update`          | Update grepai to the latest version    |

```bash
grepai search "authentication" -n 5       # Limit results (default: 10)
grepai search "authentication" --json     # JSON output for AI agents
grepai search "authentication" --json -c  # Compact JSON (~80% fewer tokens)
```

### Background Daemon

Run the watcher as a background process:

```bash
grepai watch --background    # Start in background
grepai watch --status        # Check if running
grepai watch --stop          # Stop gracefully
```

Logs are stored in OS-specific directories:

| Platform | Log Directory |
|----------|---------------|
| Linux    | `~/.local/state/grepai/logs/` |
| macOS    | `~/Library/Logs/grepai/` |
| Windows  | `%LOCALAPPDATA%\grepai\logs\` |

Use `--log-dir /custom/path` to override (must be passed to all commands):

```bash
grepai watch --background --log-dir /custom/path    # Start in background
grepai watch --status --log-dir /custom/path        # Check if running
grepai watch --stop --log-dir /custom/path          # Stop gracefully
```

### Self-Update

Keep grepai up to date:

```bash
grepai update --check    # Check for available updates
grepai update            # Download and install latest version
grepai update --force    # Force update even if already on latest
```

The update command:
- Fetches the latest release from GitHub
- Verifies checksum integrity
- Replaces the binary automatically
- Works on all supported platforms (Linux, macOS, Windows)

### Call Graph Analysis

Find function relationships in your codebase:

```bash
grepai trace callers "Login"           # Who calls Login?
grepai trace callees "HandleRequest"   # What does HandleRequest call?
grepai trace graph "ProcessOrder" --depth 3  # Full call graph
```

Output as JSON for AI agents:
```bash
grepai trace callers "Login" --json
```

## AI Agent Integration

grepai integrates natively with popular AI coding assistants. Run `grepai agent-setup` to auto-configure.

| Agent        | Configuration File                     |
|--------------|----------------------------------------|
| Cursor       | `.cursorrules`                         |
| Windsurf     | `.windsurfrules`                       |
| Claude Code  | `CLAUDE.md` / `.claude/settings.md`    |
| Gemini CLI   | `GEMINI.md`                            |
| OpenAI Codex | `AGENTS.md`                            |

### MCP Server Mode

grepai can run as an MCP (Model Context Protocol) server, making it available as a native tool for AI agents:

```bash
grepai mcp-serve    # Start MCP server (stdio transport)
```

Configure in your AI tool's MCP settings:

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

Available MCP tools:
- `grepai_search` — Semantic code search
- `grepai_trace_callers` — Find function callers
- `grepai_trace_callees` — Find function callees
- `grepai_trace_graph` — Build call graph
- `grepai_index_status` — Check index health

### Claude Code Subagent

For enhanced exploration capabilities in Claude Code, create a specialized subagent:

```bash
grepai agent-setup --with-subagent
```

This creates `.claude/agents/deep-explore.md` with:
- Semantic search via `grepai search`
- Call graph tracing via `grepai trace`
- Workflow guidance for code exploration

Claude Code automatically uses this agent for deep codebase exploration tasks.

## Configuration

Stored in `.grepai/config.yaml`:

```yaml
embedder:
  provider: ollama          # ollama | lmstudio | openai
  model: nomic-embed-text
  endpoint: http://localhost:11434  # Custom endpoint (for Azure OpenAI, etc.)
  dimensions: 768           # Vector dimensions (depends on model)
store:
  backend: gob              # gob | postgres
chunking:
  size: 512
  overlap: 50
search:
  boost:
    enabled: true           # Structural boosting for better relevance
trace:
  mode: fast                # fast (regex) | precise (tree-sitter)
external_gitignore: ""      # Path to external gitignore (e.g., ~/.config/git/ignore)
```

> **Note**: Old configs without `endpoint` or `dimensions` are automatically updated with sensible defaults.

### Search Boost (enabled by default)

grepai automatically adjusts search scores based on file paths. Patterns are language-agnostic:

| Category | Patterns | Factor |
|----------|----------|--------|
| Tests | `/tests/`, `/test/`, `__tests__`, `_test.`, `.test.`, `.spec.` | ×0.5 |
| Mocks | `/mocks/`, `/mock/`, `.mock.` | ×0.4 |
| Fixtures | `/fixtures/`, `/testdata/` | ×0.4 |
| Generated | `/generated/`, `.generated.`, `.gen.` | ×0.4 |
| Docs | `.md`, `/docs/` | ×0.6 |
| Source | `/src/`, `/lib/`, `/app/` | ×1.1 |

Customize or disable in `.grepai/config.yaml`. See [documentation](https://yoanbernabeu.github.io/grepai/configuration/) for details.

### Hybrid Search (optional)

Enable hybrid search to combine vector similarity with text matching:

```yaml
search:
  hybrid:
    enabled: true
    k: 60
```

Uses [Reciprocal Rank Fusion](https://plg.uwaterloo.ca/~gvcormac/cormacksigir09-rrf.pdf) to merge results. Useful when queries contain exact identifiers.

### Embedding Providers

**Ollama (Default)** — Privacy-first, runs locally:

```bash
ollama pull nomic-embed-text
```

**LM Studio** — Local, OpenAI-compatible API:

```bash
# Start LM Studio and load an embedding model
# Default endpoint: http://127.0.0.1:1234
```

**OpenAI** — Cloud-based:

```bash
export OPENAI_API_KEY=sk-...
```

### Storage Backends

- **GOB (Default)**: File-based, zero config
- **PostgreSQL + pgvector**: For large monorepos
- **Qdrant**: Docker-based vector database

## Requirements

- Ollama, LM Studio, or OpenAI API key (for embeddings)
- Go 1.22+ (only for building from source)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

[MIT License](LICENSE) - Yoan Bernabeu 2026
