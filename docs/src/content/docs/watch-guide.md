---
title: File Watching
description: Real-time index updates with grepai watch
---

## File Watching

`grepai watch` is the core daemon that maintains your codebase index. It performs an initial full scan and then monitors for file changes in real-time.

### Features

- **Initial indexing**: Full codebase scan on startup
- **Real-time updates**: Monitors filesystem for changes
- **Incremental sync**: Only re-indexes modified files
- **Symbol extraction**: Builds call graph data for `grepai trace`
- **Debounced processing**: Batches rapid changes efficiently

### Quick Start

```bash
# Initialize grepai in your project
grepai init

# Start the watcher daemon
grepai watch
```

Output:

```
Starting grepai watch in /path/to/project
Provider: ollama (nomic-embed-text)
Backend: gob

Performing initial scan...
Indexing [================] 100% (245/245) src/auth/handler.go
Initial scan complete: 245 files indexed, 1842 chunks created (took 45.2s)
Building symbol index...
Symbol index built: 3421 symbols extracted

Watching for changes... (Press Ctrl+C to stop)
```

### How It Works

```
+------------------+     +------------------+     +------------------+
|   Initial Scan   | --> |   File Watcher   | --> |  Index Update    |
|   (full index)   |     |   (fsnotify)     |     |  (incremental)   |
+------------------+     +------------------+     +------------------+
                                |
                                v
                        +------------------+
                        |   Debouncing     |
                        |   (500ms)        |
                        +------------------+
```

1. **Initial scan**: Compares disk state with existing index, removes obsolete entries, indexes new files
2. **File watching**: Monitors filesystem events (create, modify, delete, rename)
3. **Debouncing**: Batches rapid changes to avoid redundant indexing
4. **Atomic updates**: Prevents duplicate vectors during updates

### What Gets Indexed

The watcher indexes files with these extensions:

| Language | Extensions |
|----------|------------|
| Go | `.go` |
| JavaScript/TypeScript | `.js`, `.jsx`, `.ts`, `.tsx` |
| Python | `.py` |
| PHP | `.php` |
| Rust | `.rs` |
| C/C++ | `.c`, `.cpp`, `.h`, `.hpp`, `.cc`, `.cxx`, `.hxx` |
| Zig | `.zig` |
| Java | `.java` |
| Ruby | `.rb` |

### What Gets Skipped

Files are skipped based on:

1. **`.gitignore` patterns**: Automatically respected
2. **Config ignore patterns**: From `.grepai/config.yaml`
3. **Binary files**: Non-text files are excluded
4. **Large files**: Files exceeding size limits

Default ignore patterns:

```yaml
scanner:
  ignore:
    - "*.min.js"
    - "*.min.css"
    - "vendor/"
    - "node_modules/"
    - ".git/"
    - "target/"
    - ".zig-cache/"
    - "zig-out/"
```

### Real-Time Updates

When files change, the watcher logs updates:

```
[MODIFY] src/auth/handler.go
Indexed src/auth/handler.go (4 chunks)
Extracted 12 symbols from src/auth/handler.go

[CREATE] src/api/routes.go
Indexed src/api/routes.go (3 chunks)
Extracted 8 symbols from src/api/routes.go

[DELETE] src/old/deprecated.go
Removed src/old/deprecated.go from index
```

### Symbol Indexing

The watcher also builds a symbol index for call graph analysis:

- **Symbols extracted**: Functions, methods, classes, types
- **References tracked**: Function calls and references
- **Used by**: `grepai trace` for call graph analysis

See [Call Graph Analysis](/grepai/trace/) for more details.

### Configuration

Configure watcher behavior in `.grepai/config.yaml`:

```yaml
# Chunking parameters
chunking:
  size: 512      # Tokens per chunk
  overlap: 50    # Overlap for context

# Additional ignore patterns
scanner:
  ignore:
    - "*.generated.go"
    - "dist/"
    - "build/"
```

### Persistence

The watcher periodically saves the index:

- **Auto-save**: Automatic persistence during operation
- **Shutdown save**: Clean save on Ctrl+C or SIGTERM
- **Location**: `.grepai/index.gob` (or PostgreSQL)

### Running in Background

For long-running sessions, consider using a terminal multiplexer or process manager:

```bash
# Using tmux
tmux new-session -d -s grepai 'grepai watch'

# Using screen
screen -dmS grepai grepai watch

# Using nohup
nohup grepai watch > grepai.log 2>&1 &
```

### Troubleshooting

| Problem | Solution |
|---------|----------|
| High CPU usage | Check for too many file changes, review ignore patterns |
| Missing files | Check ignore patterns and file extensions |
| Index not updating | Check file permissions and watcher limits |
| Ollama connection failed | Ensure Ollama is running with the model loaded |

### System Limits (Linux)

On Linux, you may need to increase inotify watchers for large projects:

```bash
# Check current limit
cat /proc/sys/fs/inotify/max_user_watches

# Increase temporarily
sudo sysctl fs.inotify.max_user_watches=524288

# Increase permanently
echo "fs.inotify.max_user_watches=524288" | sudo tee -a /etc/sysctl.conf
```

### Use Cases

#### Development Workflow

Keep the watcher running in a terminal while you code:

```bash
# Terminal 1: Watch for changes
grepai watch

# Terminal 2: Search as you need
grepai search "current feature implementation"
```

#### CI/CD Integration

For CI environments, run a one-time index:

```bash
grepai watch &
sleep 60  # Wait for initial indexing
grepai search "security vulnerabilities" --json --compact
```

### Commands Reference

- [`grepai watch`](/grepai/commands/grepai_watch/) - Full CLI reference
- [`grepai status`](/grepai/commands/grepai_status/) - Check index status
- [`grepai init`](/grepai/commands/grepai_init/) - Initialize configuration
