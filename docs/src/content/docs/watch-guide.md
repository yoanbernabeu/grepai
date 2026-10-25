---
title: File Watching
description: Keep the grepai index up to date with grepai watch, run it in the background, or update it once with grepai index.
---

## File Watching

`grepai watch` keeps your index in sync with your code. It runs an initial scan, then watches the filesystem and re-indexes files as they change. Use `grepai index` instead when you only need a one-time update.

### Quick Start

```bash
# Create .grepai/config.yaml (once per project)
grepai init

# Index, then keep watching for changes (Ctrl+C or q to stop)
grepai watch
```

In an interactive terminal, `grepai watch` opens a monitoring UI. Use `--no-ui`, or pipe the output, to get plain text logs instead.

To keep it running without a terminal, see [Background Daemon Mode](#background-daemon-mode).

### Features

- **Initial scan**: brings the index in line with the files on disk at startup.
- **Real-time updates**: re-indexes created and modified files, removes deleted ones.
- **Incremental**: unchanged files are not re-embedded.
- **Symbol extraction**: builds the call graph data used by [`grepai trace`](/grepai/trace/).
- **Debouncing**: groups rapid changes before processing them.
- **Linked worktrees**: also watches linked git worktrees by default. See [Git Worktrees](/grepai/git-worktrees/).

### One-Time Indexing

`grepai index` runs the same initial synchronization as `grepai watch` (vector, symbol and, if enabled, RPG indexes), then exits. It does not start a watcher or daemon. Unchanged files are scanned but not re-embedded.

```bash
# Update the current project, then exit
grepai index

# Update every project in a workspace, then exit
grepai index --workspace myworkspace
```

`grepai index` and `grepai watch` both take the project's writer lock, so `grepai index` fails while a watcher is running on the same project.

Use it in Git hooks when a long-running watcher would pick up temporary files created by tests or build tools. For example, `.git/hooks/pre-commit`:

```sh
#!/bin/sh
grepai index
```

`grepai index` indexes the current working tree, including unstaged changes. It does not index only the staged snapshot.

To set up and index a new linked worktree without leaving a watcher behind:

```bash
git worktree add ../my-feature feature/my-feature
(cd ../my-feature && grepai index)
```

When the main worktree has a `.grepai/` directory, the linked worktree gets its own copy automatically. Each linked worktree keeps its own index, and a one-time index does not update other worktrees.

### How It Works

```text
+------------------+     +------------------+     +------------------+
|   Initial Scan   | --> |   File Watcher   | --> |  Index Update    |
|  (sync with disk)|     |   (fsnotify)     |     |  (per file)      |
+------------------+     +------------------+     +------------------+
                                |
                                v
                        +------------------+
                        |   Debouncing     |
                        |   (500ms)        |
                        +------------------+
```

1. **Initial scan**: lists indexable files, removes index entries for files that no longer exist, and decides which files need (re)indexing (see below).
2. **Embedding**: new and changed files are chunked and embedded. With the `openai` provider, chunks from many files are sent in batches, using `embedder.parallelism` workers (default 4). Other providers embed file by file.
3. **Symbol index**: symbols are extracted for languages listed in `trace.enabled_languages`.
4. **RPG (optional)**: when `rpg.enabled: true`, the RPG graph is built after symbol extraction.
5. **File watching**: filesystem events (create, modify, delete, rename) are collected and debounced (`watch.debounce_ms`, default 500).
6. **Per-file update**: each changed file is re-scanned. If its content hash is unchanged, it is skipped. Otherwise its chunks are replaced, so no duplicate vectors are left behind.

#### Fast restarts with `last_index_time`

The initial scan avoids reading every file on restart:

- A file whose modification time is at or before `watch.last_index_time` and that already has chunks in the index is skipped without being read.
- Other files are read and hashed. Files with an unchanged hash are not re-embedded.
- Files with no chunks yet are always re-indexed, whatever their modification time.

grepai stores `watch.last_index_time` in `.grepai/config.yaml`. It is set to the start time of the initial scan when the scan indexed something (or when no value was stored yet). While watching, it advances after each indexed file and is written to disk at most every 30 seconds and on clean shutdown. You do not need to edit it. Remove it to force every file to be re-hashed on the next start.

### What Gets Indexed

A file is indexed when it is not ignored and has a supported extension. The built-in list covers common source, markup and config formats, including:

| Group | Extensions |
|-------|------------|
| Source code | `.go`, `.js`, `.jsx`, `.ts`, `.tsx`, `.py`, `.rb`, `.java`, `.kt`, `.scala`, `.swift`, `.rs`, `.c`, `.h`, `.cpp`, `.hpp`, `.cc`, `.cxx`, `.hxx`, `.cs`, `.fs`, `.php`, `.lua`, `.dart`, `.ex`, `.exs`, `.erl`, `.clj`, `.hs`, `.ml`, `.elm`, `.nim`, `.zig`, `.r`, `.pas`, `.dpr` |
| Web | `.vue`, `.svelte`, `.html`, `.css`, `.scss`, `.less` |
| Scripts and data | `.sh`, `.bash`, `.zsh`, `.sql`, `.proto`, `.json`, `.xml`, `.yaml`, `.yml`, `.toml`, `.ini`, `.cfg`, `.conf`, `.env`, `.tf`, `.hcl` |
| Docs | `.md`, `.txt` |

To index more extensions, add them under `chunking.custom_extensions`:

```yaml
chunking:
  custom_extensions: [".tengo", ".el"]
```

### What Gets Skipped

Files are skipped when they match any of these:

1. **`.gitignore` files**: at the root and in subdirectories.
2. **`.grepaiignore` files**: same syntax as `.gitignore`, and can re-include paths with `!`.
3. **`ignore` in `.grepai/config.yaml`**: each entry is compared to the file or directory **name** (not a glob or path).
4. **`external_gitignore`**: an extra gitignore file, such as your global one.
5. **Minified files**: names ending in `.min.js`, `.min.css`, `.bundle.js` or `.bundle.css`.
6. **Large files**: over 1 MB.
7. **Binary files**: content that is not valid UTF-8 or contains null bytes.

Default `ignore` list written by `grepai init`:

```yaml
ignore:
  - .git
  - .grepai
  - node_modules
  - vendor
  - bin
  - dist
  - __pycache__
  - .venv
  - venv
  - .idea
  - .vscode
  - target
  - .zig-cache
  - zig-out
  - qdrant_storage
```

### Real-Time Updates

With `--no-ui`, each change is logged as it is processed:

```text
2026/10/25 14:02:11 Indexed src/auth/handler.go (4 chunks)
2026/10/25 14:02:11 Extracted 12 symbols from src/auth/handler.go
2026/10/25 14:02:30 Skipped unchanged src/api/routes.go
2026/10/25 14:02:45 Removed src/old/deprecated.go from index
```

Editors and coding agents often save by writing a temporary file and renaming it over the original. When a delete or rename event arrives but the file is still on disk, grepai treats it as a modification instead of removing the file from the index.

### Symbol Indexing

The watcher also builds a symbol index for call graph analysis:

- **Symbols**: functions, methods, classes and types.
- **References**: function calls between symbols.
- **Languages**: those listed in `trace.enabled_languages`.
- **Stored in**: `.grepai/symbols.gob`.

See [Call Graph Analysis](/grepai/trace/) for more details.

### Configuration

Watcher settings in `.grepai/config.yaml`:

```yaml
watch:
  debounce_ms: 500
  # Auto-discover and watch linked git worktrees (default: true)
  discover_worktrees: true
  # Only used when rpg.enabled is true
  rpg_derived_debounce_ms: 300
  rpg_persist_interval_ms: 1000
  rpg_full_reconcile_interval_sec: 300
  rpg_max_dirty_files_per_batch: 128
  # Managed by grepai, see "Fast restarts with last_index_time"
  # last_index_time: 2026-10-25T14:00:00+02:00
```

| Key | Default | Notes |
|-----|---------|-------|
| `debounce_ms` | `500` | Wait time used to group file events |
| `discover_worktrees` | `true` | Read from the main worktree's config only |
| `rpg_derived_debounce_ms` | `300` | Minimum 100 |
| `rpg_persist_interval_ms` | `1000` | Minimum 200 |
| `rpg_full_reconcile_interval_sec` | `300` | Minimum 30 |
| `rpg_max_dirty_files_per_batch` | `128` | Minimum 1 |

To watch only the main worktree, set `discover_worktrees: false` in the **main worktree's** config. See [Git Worktrees](/grepai/git-worktrees/#disabling-worktree-discovery).

### Persistence

- **Periodic save**: the vector and symbol indexes are saved every 30 seconds while watching.
- **Shutdown save**: they are saved again on Ctrl+C, `q` in the UI, SIGTERM or `grepai watch --stop`.
- **Location**: `.grepai/index.gob` and `.grepai/symbols.gob` with the GOB backend. PostgreSQL and Qdrant store vectors in the database; symbols stay in `.grepai/symbols.gob`.

Only one process may write a project's index at a time. `grepai watch` (foreground, background or workspace) and `grepai index` all take a lock on `.grepai/writer.lock`. A second writer exits immediately with an error like `cannot write project ...: another writer is active`. The lock is released when the process exits, including after a crash.

`grepai search`, `grepai trace` and the MCP server only read the index, so they run normally while a watcher is active.

#### Fatal watcher errors

If grepai cannot keep full coverage of the project, it stops instead of serving a stale index. This happens when a directory cannot be watched (at startup or when a new directory appears), when fsnotify stops or reports an error (for example an event overflow), or when the project root is removed or renamed. In that case:

- New index updates are rejected, and in-flight work is canceled.
- Derived indexes are not saved, because their state may be wrong.
- The next `grepai watch` or `grepai index` re-runs the initial scan, which reconciles the index with the disk.

In workspace and multi-worktree mode, one fatal error stops the whole watcher.

### Background Daemon Mode

Run the watcher as a background process:

```bash
# Start the watcher in background
grepai watch --background

# Check if the watcher is running
grepai watch --status

# Stop the background watcher gracefully
grepai watch --stop
```

`--background`, `--status` and `--stop` cannot be combined. The background process always logs plain text (no UI).

#### Starting the Daemon

```bash
$ grepai watch --background
Background watcher started (PID 12345)
Logs: /Users/you/Library/Logs/grepai/grepai-worktree-0adda787cae4.log
Worktree ID: 0adda787cae4

Use 'grepai watch --status' to check status
Use 'grepai watch --stop' to stop the watcher
```

The command waits up to 30 seconds (60 for a workspace) for the background process to finish its initial scan and watch every directory. If the startup checks passed (config, embedder, store) but the initial index takes longer, the command returns with `initial indexing still in progress` and the watcher keeps running: follow it with `grepai watch --status` or the log file. The command fails, and points to the log file, only if the process exits early or never gets through its startup checks.

Inside a git repository, the log, PID and ready files are named after the worktree ID (`grepai-worktree-<id>.*`), so each worktree can have its own daemon. Outside git, they are named `grepai-watch.*`.

#### Checking Status

```bash
$ grepai watch --status
Status: running
PID: 12345
Log directory: /Users/you/Library/Logs/grepai
Log file: /Users/you/Library/Logs/grepai/grepai-worktree-0adda787cae4.log
Worktree ID: 0adda787cae4
```

When no daemon is running, it prints `Status: not running` and the log directory.

#### Stopping the Daemon

```bash
$ grepai watch --stop
Stopping background watcher (PID 12345)...
Background watcher stopped
```

The daemon saves the index before exiting. `--stop` waits up to 30 seconds and prints `Waiting for graceful shutdown...` every 5 seconds meanwhile. If nothing is running, it prints `No background watcher is running`.

#### Log Locations

Logs are stored in OS-specific directories:

| Platform | Log Directory |
|----------|---------------|
| Linux | `$XDG_STATE_HOME/grepai/logs/`, or `~/.local/state/grepai/logs/` |
| macOS | `~/Library/Logs/grepai/` |
| Windows | `%LOCALAPPDATA%\grepai\logs\` |

Background log lines start with `[grepai-watch]` and a timestamp with microseconds.

Use `--log-dir` to override the directory:

```bash
grepai watch --background --log-dir /custom/path
grepai watch --status --log-dir /custom/path
grepai watch --stop --log-dir /custom/path
```

`--stop` also finds a custom log directory recorded when the daemon was started. `--status` only checks the directory you give it (or the default).

#### Log Management

Logs are not rotated automatically. To limit disk usage, truncate the file from time to time or use your system's rotation tool:

```bash
# Truncate the log file (macOS example)
: > ~/Library/Logs/grepai/grepai-worktree-<id>.log

# Or set up logrotate (Linux) / newsyslog (macOS)
```

#### PID File Management

The daemon writes a PID file and locks it, which lets grepai:

- Refuse to start a second background watcher for the same project
- Make a foreground `grepai watch` refuse to start while a background watcher runs
- Clean up stale PID files left by crashed processes

#### Alternative: Terminal Multiplexers

You can also use tmux or screen. Add `--no-ui` if you will read the output later:

```bash
# Using tmux
tmux new-session -d -s grepai 'grepai watch --no-ui'

# Using screen
screen -dmS grepai grepai watch --no-ui
```

### Foreground UI Controls

In an interactive terminal (stdin and stdout are terminals and `TERM` is set and not `dumb`), `grepai watch` shows a UI with the scan and embedding progress, an event ledger and the watcher health. It is not used with `--no-ui`, `--background` or `--workspace`.

| Key | Action |
|-----|--------|
| `q` or `Ctrl+C` | Graceful shutdown (saves the index before exit) |
| `p` | Pause or resume event ledger auto-scroll |
| `?` | Toggle help |
| `Tab` | Switch the focused session when several worktrees are watched |

With `--no-ui`, the same run prints plain text (abridged):

```text
Starting grepai watch in /path/to/project
Provider: ollama (nomic-embed-text)
Backend: gob
RPG: disabled
...
Performing initial scan...
Initial scan complete: 245 files indexed, 1842 chunks created, 0 files removed, 3 skipped (took 45.2s)
Checking symbol index...
Symbol index built: 3421 symbols extracted
...
Watching for changes... (Press Ctrl+C to stop)
```

On a restart with nothing to update, the scan line reads `Index up to date: 245 files checked, 0 reindexed (took 120ms)`. The omitted lines include session status lines such as `[RUNNING] /path/to/project - steady`.

### Troubleshooting

| Problem | Solution |
|---------|----------|
| `no grepai project found` | Run `grepai init` in the project first |
| `watcher is already running in background` | Stop it with `grepai watch --stop`, or use it as is |
| `another writer is active` | A watcher or `grepai index` already runs on this project. Stop it first |
| `cannot connect to Ollama` | Start Ollama and pull the configured model |
| High CPU usage | Look for directories with many changing files and add them to `ignore` or `.grepaiignore` |
| Missing files | Check ignore rules, file size (1 MB max) and supported extensions |
| Index not updating | Check `grepai watch --status` and the log file, then file permissions and watch limits |

### System Limits (Linux)

On Linux, large projects may need a higher inotify watch limit:

```bash
# Check current limit
cat /proc/sys/fs/inotify/max_user_watches

# Increase temporarily
sudo sysctl fs.inotify.max_user_watches=524288

# Increase permanently
echo "fs.inotify.max_user_watches=524288" | sudo tee -a /etc/sysctl.conf
```

If the watcher reports `inotify watch limit reached` (`no space left on device`, `ENOSPC`) while adding a directory, this is the per-user inotify quota, **not disk space**. The quota is shared by all your processes, so an editor, language server or another watcher can use it up after grepai starts. Raise `fs.inotify.max_user_watches` or stop other watchers, then restart `grepai watch`.

### Use Cases

#### Development Workflow

Keep the watcher running while you code:

```bash
# Terminal 1: watch for changes
grepai watch

# Terminal 2: search as needed
grepai search "current feature implementation"
```

#### CI/CD Integration

In CI, update the index once and search:

```bash
grepai index
grepai search "security vulnerabilities" --json --compact
```

The embedder configured in `.grepai/config.yaml` must be reachable from the CI runner.

### Workspace Mode

For multi-project setups, the watcher can index every project of a workspace into a shared vector store:

```bash
# Start workspace watcher (foreground)
grepai watch --workspace my-fullstack

# Start workspace watcher (background)
grepai watch --workspace my-fullstack --background

# Check workspace watcher status
grepai watch --workspace my-fullstack --status

# Stop workspace watcher
grepai watch --workspace my-fullstack --stop
```

The workspace watcher always prints plain text (no UI).

#### How Workspace Watching Differs

| Aspect | Single Project | Workspace |
|--------|---------------|-----------|
| Config source | `.grepai/config.yaml` | `~/.grepai/workspace.yaml` |
| Store/embedder | Per-project settings | Shared workspace settings (PostgreSQL or Qdrant) |
| Chunking/ignore | Per-project | Per-project (each project keeps its own) |
| File path format | `src/App.tsx` | `<workspace>/<project>/src/App.tsx` |
| Symbol storage | `.grepai/symbols.gob` | Per-project `.grepai/symbols.gob` |
| Log file | `grepai-watch.log` or `grepai-worktree-<id>.log` | `grepai-workspace-<name>.log` |

#### Coexistence

A per-project watcher and a workspace watcher both write the project's symbol index in `.grepai/`, so they take the same writer lock (`.grepai/writer.lock`). The second one to start exits with an `another writer is active` error. Run one or the other for a given project, not both at once.

#### Log Locations (Workspace)

Workspace logs use the same OS-specific directories as single-project logs:

| Platform | Log File |
|----------|----------|
| Linux | `~/.local/state/grepai/logs/grepai-workspace-<name>.log` |
| macOS | `~/Library/Logs/grepai/grepai-workspace-<name>.log` |
| Windows | `%LOCALAPPDATA%\grepai\logs\grepai-workspace-<name>.log` |

See [Workspace Management](/grepai/workspace/) for setup and configuration.

### Commands Reference

- [`grepai watch`](/grepai/commands/grepai_watch/): full CLI reference
- [`grepai index`](/grepai/commands/grepai_index/): one-time indexing
- [`grepai status`](/grepai/commands/grepai_status/): check index status
- [`grepai init`](/grepai/commands/grepai_init/): initialize configuration
