---
title: Introduction
description: What grepai is, how it works, and when to use it instead of grep. Start here if you are new to grepai.
---

grepai is a command-line tool that lets you search a codebase by describing what the code does, in plain English. This page explains what it is and how it works. If you just want to try it, go to the [Quick Start](/grepai/quickstart/).

## What is grepai?

`grep` and `ripgrep` find exact text. grepai finds code by meaning. It splits your files into chunks, turns each chunk into a vector embedding, and stores the vectors in an index. When you search, grepai embeds your query the same way and returns the closest chunks.

```bash
grepai search "where are users saved to the database"
```

This can find a function called `persistAccount` even though none of the query words appear in it.

By default, embeddings are computed locally with [Ollama](https://ollama.com), so your code does not leave your machine. Cloud providers are available if you prefer.

## Why grepai?

### The Problem

Text search works well when you already know the name of what you are looking for. It does not help when you only know what the code is supposed to do. AI coding agents have the same problem: they often spend many tool calls (and tokens) running `grep` and reading files before they find the right place.

### The Solution

grepai keeps a searchable index of your project:

1. **Index**: `grepai watch` scans your project and embeds every supported file. Files ignored by `.gitignore` or `.grepaiignore` are skipped.
2. **Stay up to date**: the watcher re-indexes files as you edit them. If you prefer not to run a daemon, `grepai index` updates the index once and exits.
3. **Search**: `grepai search` returns the most relevant chunks with file paths, line numbers and scores.

grepai also builds a symbol index, so `grepai trace` can list the callers and callees of a function.

## Example Searches

```bash
# Find code without knowing function names
grepai search "user login validation"

# Ask about patterns
grepai search "how are errors handled in API requests"

# Machine-readable output for scripts and AI agents
grepai search "where are users saved to the database" --json --compact

# Find every function that calls Login
grepai trace callers "Login"
```

## Architecture

```
┌──────────────┐     ┌────────────────────────┐     ┌──────────────────┐
│  Your code   │ ──► │  Embedder              │ ──► │  Vector store    │
│  (files)     │     │  Ollama, LM Studio,    │     │  GOB file,       │
└──────────────┘     │  OpenAI, OpenRouter,   │     │  PostgreSQL,     │
                     │  Synthetic, Requesty   │     │  Qdrant          │
                     └────────────────────────┘     └──────────────────┘
                                                              │
                                                              ▼
                                       ┌──────────────────────────────────┐
                                       │  grepai search "auth flow"       │
                                       │  → ranked chunks                 │
                                       └──────────────────────────────────┘
```

- **Embedder**: turns text into vectors. See [Embedders](/grepai/backends/embedders/).
- **Vector store**: keeps the vectors. The default GOB store is a single file in `.grepai/`. See [Vector Stores](/grepai/backends/stores/).
- **Configuration**: each project has its own `.grepai/config.yaml`. See [Configuration](/grepai/configuration/).

## Using grepai with AI agents

grepai includes an MCP server (`grepai mcp-serve`) that exposes search, trace and index status as tools for MCP-compatible agents such as Claude Code or Cursor. See [MCP Integration](/grepai/mcp/).

## Next Steps

- [Installation](/grepai/installation/): install grepai and an embedding provider
- [Quick Start](/grepai/quickstart/): index a project and run your first search
- [Configuration](/grepai/configuration/): change the embedder, store, chunking and ignore rules
