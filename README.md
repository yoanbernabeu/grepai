# grepai

[![Go](https://github.com/yoanbernabeu/grepai/actions/workflows/ci.yml/badge.svg)](https://github.com/yoanbernabeu/grepai/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/yoanbernabeu/grepai)](https://goreportcard.com/report/github.com/yoanbernabeu/grepai)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

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
grepai init                    # Initialize in your project
grepai watch                   # Start background indexing daemon
grepai search "error handling" # Search semantically
```

## Commands

| Command                  | Description                            |
|--------------------------|----------------------------------------|
| `grepai init`            | Initialize grepai in current directory |
| `grepai watch`           | Start real-time file watcher daemon    |
| `grepai search <query>`  | Search codebase with natural language  |
| `grepai status`          | Browse index state interactively       |
| `grepai agent-setup`     | Configure AI agents integration        |

```bash
grepai search "authentication" -n 5  # Limit results (default: 10)
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

## Configuration

Stored in `.grepai/config.yaml`:

```yaml
embedder:
  provider: ollama          # ollama | lmstudio | openai
  model: nomic-embed-text
store:
  backend: gob              # gob | postgres
chunking:
  size: 512
  overlap: 50
```

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

## Requirements

- Ollama, LM Studio, or OpenAI API key (for embeddings)
- Go 1.22+ (only for building from source)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

[MIT License](LICENSE) - Yoan Bernabeu 2026
