---
title: Quick Start
description: Index your first project with grepai and run a natural language code search in a few minutes.
---

This page takes you from an installed `grepai` binary to your first semantic search. It assumes you have [installed grepai](/grepai/installation/) and that Ollama is running with the `nomic-embed-text` model (the default setup).

The whole flow:

```bash
cd /path/to/your/project
grepai init --yes        # create .grepai/config.yaml with the defaults
grepai index             # build the index once
grepai search "error handling middleware"
```

The steps below explain each command and the options you have.

## 1. Initialize grepai

From the root of your project, run:

```bash
cd /path/to/your/project
grepai init
```

grepai asks you to pick an embedding provider (Ollama is the default) and a storage backend (a local GOB file is the default). Press Enter to accept each default. Other ways to run it:

```bash
grepai init --yes                      # accept all defaults, no prompts
grepai init --ui                       # interactive wizard in the terminal
grepai init -p openai -b gob --yes     # pick provider and backend with flags
```

This creates `.grepai/config.yaml`. If the project has a `.gitignore`, grepai adds `.grepai/` to it. See [Configuration](/grepai/configuration/) to change settings later, and [grepai init](/grepai/commands/grepai_init/) for all flags.

## 2. Start the Indexing Daemon

```bash
grepai watch
```

`grepai watch` does two things:

1. It runs an initial scan: it splits each supported file into chunks, embeds them, and stores the vectors. Files that have not changed since the last run are skipped.
2. It keeps running and re-indexes files as you create, edit, rename or delete them.

In an interactive terminal, `grepai watch` opens a live dashboard. Use `--no-ui` for plain log output (for scripts or CI), or run it in the background:

```bash
grepai watch --no-ui        # plain output
grepai watch --background   # detach and write logs to a file
grepai watch --status       # check the background watcher
grepai watch --stop         # stop it
```

**Don't need a daemon?** `grepai index` runs the same scan once and exits. Run it again whenever you want to refresh the index:

```bash
grepai index
```

See the [File Watching guide](/grepai/watch-guide/) for details.

## 3. Search Your Code

In another terminal (or after `grepai index` finishes):

```bash
# Describe what the code does
grepai search "user authentication"
grepai search "how errors are handled"

# Return 5 results instead of the default 10
grepai search "database queries" --limit 5

# Only search under a path
grepai search "REST API routes" --path internal/api

# JSON output for scripts and AI agents (--compact drops the code content)
grepai search "authentication" --json --compact
```

See [grepai search](/grepai/commands/grepai_search/) for all flags, including the token-efficient `--toon` format.

## Example Output

```
$ grepai search "error handling middleware" --limit 2
Found 2 results for: "error handling middleware"

─── Result 1 (score: 0.6864) ───
File: middleware/error.go:1-15

   1 │ package middleware
   2 │
   3 │ import "net/http"
   4 │
   5 │ // ErrorHandler recovers from panics and returns a 500 response.
   6 │ func ErrorHandler(next http.Handler) http.Handler {
   7 │ 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
   ...

─── Result 2 (score: 0.5687) ───
File: handlers/user.go:1-11

   1 │ package handlers
   2 │
   3 │ import "errors"
   ...
```

Each result shows its rank, its similarity score, the file and line range, and up to 15 lines of the chunk. Scores are relative: compare them within one search rather than across searches.

With `--json --compact`, you get only the location and score:

```json
[
  {
    "file_path": "middleware/error.go",
    "start_line": 1,
    "end_line": 15,
    "score": 0.68637824
  }
]
```

## 4. Check Index Status

```bash
grepai status
```

In an interactive terminal, this opens a browser where you can list indexed files and inspect their chunks. For a plain summary, use `--no-ui`:

```
$ grepai status --no-ui
grepai index status
Files indexed: 2
Total chunks: 2
Index size: 10.6 KB
Last updated: 2026-10-25 12:29:27
Provider: ollama (nomic-embed-text)
Watcher: not running
Watcher log: /Users/you/Library/Logs/grepai/grepai-worktree-3890120fa89c.log
```

## 5. Connect to Your AI Agent

Your AI agent does not know about the index yet. The recommended way to connect it is **[MCP](/grepai/mcp/)**: `grepai mcp-serve` exposes search, trace and index status as native tools for any MCP-compatible agent.

If your agent does not support MCP, run `grepai agent-setup`. It appends instructions for using the grepai CLI to the agent files it finds in your project (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `.cursorrules`, `.cursor/rules`, `.windsurfrules`). You do not need it if you use MCP.

Claude Code users can also add the optional **[subagent](/grepai/subagent/)** (`grepai agent-setup --with-subagent`), which gives exploration agents grepai access in their own context.

## Tips for Better Searches

- **Be descriptive**: "user login validation" works better than "login".
- **Describe intent**: say what the code does, not how it is written ("where are users saved" rather than "func saveUser").
- **Write queries in English**: the default model, `nomic-embed-text`, is trained mainly on English.
- **Keep grep for exact matches**: to find a known identifier or string, `grep` or `ripgrep` is still the right tool.

## Next Steps

- [Semantic Search Guide](/grepai/search-guide/): write better queries
- [File Watching Guide](/grepai/watch-guide/): how the daemon keeps the index fresh
- [Call Graph Analysis](/grepai/trace/): find callers and callees with `grepai trace`
- [Configuration](/grepai/configuration/): change the embedder, storage, chunking and ignore rules
- [Commands Reference](/grepai/commands/grepai/): every command and flag
