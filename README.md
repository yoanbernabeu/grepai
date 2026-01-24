<div align="center">

# grepai

### grep for the AI era

[![GitHub stars](https://img.shields.io/github/stars/yoanbernabeu/grepai?style=flat&logo=github)](https://github.com/yoanbernabeu/grepai/stargazers)
[![Downloads](https://img.shields.io/github/downloads/yoanbernabeu/grepai/total?style=flat&logo=github)](https://github.com/yoanbernabeu/grepai/releases)
[![Go](https://github.com/yoanbernabeu/grepai/actions/workflows/ci.yml/badge.svg)](https://github.com/yoanbernabeu/grepai/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/yoanbernabeu/grepai)](https://goreportcard.com/report/github.com/yoanbernabeu/grepai)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**Search code by meaning, not just text.**

[Documentation](https://yoanbernabeu.github.io/grepai/) · [Installation](#installation) · [Quick Start](#quick-start)

</div>

---

`grepai` is a privacy-first CLI for semantic code search. It uses vector embeddings to understand code meaning, enabling natural language queries that find relevant code—even when naming conventions vary.

**Drastically reduces AI agent input tokens** by providing relevant context instead of raw search results.

## Features

- **Search by intent** — Ask "authentication logic" and find `handleUserSession`
- **Trace call graphs** — Know who calls a function before you change it
- **100% local** — Your code never leaves your machine
- **Always up-to-date** — File watcher keeps the index fresh automatically
- **AI agent ready** — Works with Claude Code, Cursor, Windsurf out of the box
- **MCP server** — Your AI agent can call grepai directly as a tool

## Installation

```bash
curl -sSL https://raw.githubusercontent.com/yoanbernabeu/grepai/main/install.sh | sh
```

### Windows (PowerShell)

Run the following command in your PowerShell terminal to install `grepai` automatically:

```powershell
irm https://raw.githubusercontent.com/yoanbernabeu/grepai/main/install.ps1 | iex
```

Requires an embedding provider — [Ollama](https://ollama.ai) (default), [LM Studio](https://lmstudio.ai), or OpenAI:

```bash
# With Ollama (local, privacy-first)
ollama pull nomic-embed-text
```

## Quick Start

```bash
grepai init                        # Initialize in your project
grepai watch                       # Start indexing daemon
grepai search "error handling"     # Search semantically
grepai trace callers "Login"       # Find who calls a function
```

## What developers say

> *"I just hit my limit and it took 13% of my max5 plan just to read my codebase. I am very, very excited about your new tool."*
> — u/911pleasehold on [r/ClaudeAI](https://www.reddit.com/r/ClaudeAI/comments/1qiv0d3/open_source_i_reduced_claude_code_input_tokens_by/) (280K+ views)

> *"It works great! Takes 5 minutes to install. Crazy!"*
> — [@LesSaleGeek](https://x.com/LesSaleGeek/status/2010335803604611124) on X

> *"The results are incredible!"*
> — [Kenny Nguyen](https://www.linkedin.com/feed/update/urn:li:activity:7419451883293061120?commentUrn=urn%3Ali%3Acomment%3A%28activity%3A7419451883293061120%2C7419464457388453888%29) on LinkedIn

## Why grepai?

`grep` was built in 1973 for exact text matching. Modern codebases need semantic understanding.

| | `grep` / `ripgrep` | `grepai` |
|---|---|---|
| **Search** | Exact text / regex | Semantic understanding |
| **Query** | `"func.*Login"` | `"user authentication flow"` |
| **Finds** | Pattern matches | Conceptually related code |

## Documentation

Full docs, guides, and blog:

- **[Documentation](https://yoanbernabeu.github.io/grepai/)** — Configuration, AI agents, MCP setup
- **[Blog](https://yoanbernabeu.github.io/grepai/blog/)** — Benchmarks, tutorials, release notes

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

[MIT License](LICENSE) - Yoan Bernabeu 2026
