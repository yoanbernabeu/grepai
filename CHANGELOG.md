# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Adaptive Rate Limiting for OpenAI**: Intelligent rate limit handling that automatically optimizes parallelism
  - **Automatic parallelism adjustment**: Halves parallelism after consecutive 429 responses, gradually restores after successful requests
  - **Retry-After header support**: Uses OpenAI's Retry-After header for optimal retry timing when present
  - **Proactive token pacing**: Optional TPM limit (`WithOpenAITPMLimit`) to pace requests and avoid hitting rate limits
  - **Enhanced visibility**: Logs parallelism adjustments with old/new values for debugging and monitoring
  - Addresses the 16% performance regression observed with parallelism=4 under rate limiting conditions
  - New `embedder/rate_limiter.go` with thread-safe `AdaptiveRateLimiter` and `TokenBucket` implementations
  - All rate limiting code passes race detector tests

- **Parallel OpenAI Embedding**: Cross-file batch embedding with parallel API requests for 3x+ faster indexing
  - Batches chunks from multiple files into single API requests (up to 2000 inputs per batch)
  - Parallel batch processing with configurable worker count (default: 4 workers)
  - New `embedder.parallelism` configuration option to tune concurrency for your API tier
  - Automatic retry with exponential backoff (1s base, 2x multiplier, max 32s) on rate limits (429) and server errors (5xx)
  - Immediate failure on non-retryable client errors (400, 401, 403)
  - Jitter added to backoff to prevent thundering herd
  - Progress percentage now reflects chunk completion across all batches
  - Retry attempts displayed to user: "Retrying batch N (attempt X/5)..."
  - Atomic indexing: all batches succeed or entire operation fails cleanly
  - Ollama embedder unchanged (local, already fast)

## [0.23.0] - 2026-01-25

### Added

- **Windows PowerShell Installation**: Native PowerShell installation script for Windows users (#73) - @Lisito11
  - Simple one-liner: `irm https://grepai.dev/install.ps1 | iex`
  - Automatic PATH configuration
  - No external dependencies required

### Fixed

- **MCP Server Project Path**: Add optional `project-path` argument to `mcp-serve` command (#76) - @yoanbernabeu
  - Fixes "failed to find project root" error when launched via Cursor/MCP on Windows
  - Configuration: `grepai mcp-serve /path/to/your/project`
  - Fully backward compatible: without argument, uses existing behavior

## [0.22.0] - 2026-01-24

### Added

- **Multi-Project Workspace Support**: Index and search across multiple projects with shared vector store (#75) - @yoanbernabeu
  - New `grepai workspace` command for managing workspaces:
    - `workspace create <name>` - Create a new workspace with store/embedder configuration
    - `workspace add <workspace> <path>` - Add a project to a workspace
    - `workspace remove <workspace> <project>` - Remove a project from a workspace
    - `workspace list` - List all configured workspaces
    - `workspace show <name>` - Show workspace details and projects
    - `workspace status <name>` - Show indexing status per project
    - `workspace delete <name>` - Delete a workspace
  - Extended `grepai watch` with `--workspace` flag for multi-project indexing
    - Background daemon mode: `grepai watch --workspace <name> --background`
    - Status check: `grepai watch --workspace <name> --status`
    - Stop daemon: `grepai watch --workspace <name> --stop`
  - Extended `grepai search` with `--workspace` and `--project` flags
    - Cross-project search: `grepai search --workspace <name> "query"`
    - Scoped search: `grepai search --workspace <name> --project frontend "query"`
  - Extended MCP server with `workspace` and `projects` parameters for `grepai_search`
  - Global workspace configuration stored in `~/.grepai/workspace.yaml`
  - Path prefixing format: `workspaceName/projectName/relativePath` for isolation
  - Requires PostgreSQL or Qdrant backend (GOB not supported for shared storage)
  - 100% backward compatible: existing single-project workflows unchanged

### Documentation

- New workspace management documentation page
- Blog post announcing multi-project workspace feature

## [0.21.0] - 2026-01-23

### Added

- **Pascal/Delphi Language Support for Trace**: Symbol extraction and call graph analysis now supports Pascal/Delphi (#71) - @yoanbernabeu
  - Functions: `function FunctionName(params): ReturnType;`
  - Procedures: `procedure ProcedureName(params);`
  - Class methods: `function TClassName.MethodName` / `procedure TClassName.MethodName`
  - Classes: `TClassName = class(TParent)` / `TClassName = class`
  - Interfaces: `IInterfaceName = interface`
  - Types: records, packed records, enums, type aliases, arrays
  - Pascal keywords added to filter out false positives
  - `.pas` and `.dpr` added to default traced languages and supported extensions

- **Claude Code Release Skill**: New skill for automated release process
  - Checks CI status before proceeding
  - Determines version type (major/minor/patch) based on changes
  - Updates CHANGELOG and documentation version
  - Credits contributors automatically

## [0.20.1] - 2026-01-23

### Fixed

- **MCP Index Status Schema**: Added `verbose` parameter to `grepai_index_status` tool to fix empty schema issue with strict MCP clients like Copilot/GPT5-Codex-Max (#66)
  - Some MCP clients require a non-empty input schema for all tools
  - Added regression test to prevent future schema-related issues

## [0.20.0] - 2026-01-23

### Added

- **MCP Compact Mode**: New `compact` parameter for MCP tools to reduce token usage (#61)
  - `grepai_search`: When `compact=true`, omits the `content` field (~80% token savings)
  - `grepai_trace_callers`: When `compact=true`, omits the `context` field from call sites
  - `grepai_trace_callees`: When `compact=true`, omits the `context` field from call sites
  - Default is `false` for full backward compatibility
  - Ideal for AI agents that only need file locations to then read files directly

### Documentation

- Added Opencode MCP configuration example

## [0.19.0] - 2026-01-22

### Added

- **Watcher Performance Optimization**: Skip unchanged files on subsequent launches (#62)
  - New `last_index_time` field in configuration tracks last indexing timestamp
  - Files with ModTime before `last_index_time` are skipped, avoiding unnecessary embeddings
  - Config write throttling (30s) prevents file system overload during active development
  - Significantly faster subsequent `grepai watch` launches (~1ms vs ~100ms for unchanged codebases)
  - Fully backward compatible: old configs work normally, optimization kicks in after first watch

### Changed

- `Indexer` now accepts `lastIndexTime` parameter for ModTime-based file skipping
- `runInitialScan` returns `IndexStats` to enable conditional config updates

## [0.18.0] - 2026-01-21

### Added

- **Qdrant Vector Store Backend**: New storage backend using Qdrant vector database (#57)
  - Support for local Qdrant (Docker) and Qdrant Cloud
  - gRPC connection with TLS support
  - Automatic collection creation and management
  - Docker Compose profile for easy local setup: `docker compose --profile=qdrant up`
  - Configuration options: endpoint, port, TLS, API key, collection name

### Fixed

- **Qdrant Backend Improvements**: Various fixes and improvements
  - Fixed default port display in `grepai init` prompt (6333 → 6334 for gRPC)
  - Added UTF-8 sanitization to prevent indexing errors on files with invalid characters
  - Added `qdrant_storage` to default ignore patterns
  - Updated CLI help to include qdrant in backend options
  - Fixed typo in compose.yaml ("Optionnal" → "Optional")

## [0.17.0] - 2026-01-21

### Added

- **Cursor Rules Support**: `grepai agent-setup` now supports `.cursor/rules` configuration file (#59)
  - `.cursor/rules` (Cursor's current standard) takes priority over deprecated `.cursorrules`
  - Backwards compatibility maintained for existing `.cursorrules` files
  - Both files are configured if present (idempotence handled by marker detection)

## [0.16.1] - 2026-01-18

### Fixed

- **CLI Error Display**: Commands now properly display error messages on stderr (#52, #53)
  - Previously errors were silenced by Cobra's `SilenceErrors: true` setting
  - Permission errors in `update` command now show user-friendly message with sudo suggestion

## [0.16.0] - 2026-01-16

### Added

- **Background Daemon Mode**: New flags for `grepai watch` to run as a background process
  - `grepai watch --background`: Start watcher as a detached daemon
  - `grepai watch --status`: Check if background watcher is running (shows PID and log location)
  - `grepai watch --stop`: Gracefully stop the background watcher (with 30s timeout)
  - `--log-dir`: Override default log directory
  - OS-specific default log directories:
    - Linux: `~/.local/state/grepai/logs/` (or `$XDG_STATE_HOME`)
    - macOS: `~/Library/Logs/grepai/`
    - Windows: `%LOCALAPPDATA%\grepai\logs\`
  - PID file management with file locking to prevent race conditions
  - Automatic stale PID detection and cleanup
  - Ready signaling: parent waits for child to fully initialize before returning
  - Graceful shutdown with index persistence on SIGINT/SIGTERM
- **New `daemon` package**: Cross-platform process lifecycle management
  - Platform-specific implementations for Unix and Windows
  - File locking (flock on Unix, LockFileEx on Windows)
  - Process detection and signal handling

## [0.15.1] - 2026-01-16

### Added

- **External Gitignore Support**: New `external_gitignore` configuration option to specify a path to an external gitignore file (e.g., `~/.config/git/ignore`) (#50)
  - Supports `~` expansion for home directory paths
  - External patterns are respected during indexing alongside project-level `.gitignore` files
  - If the file doesn't exist, a warning is logged but indexing continues normally

## [0.15.0] - 2026-01-14

### Added

- **C# Language Support for Trace**: Symbol extraction and call graph analysis now supports C# (#48)
  - Classes (with inheritance, generics, sealed/abstract/static/partial modifiers)
  - Structs (including readonly and ref structs)
  - Records (record, record class, record struct)
  - Interfaces (including generic interfaces)
  - Methods (with all modifiers: public, private, protected, internal, static, virtual, override, abstract, async, etc.)
  - Constructors
  - Expression-bodied members
  - C# keywords added to filter out false positives
  - `.cs` added to default traced languages
  - Tree-sitter support for precise symbol extraction

## [0.14.0] - 2026-01-12

### Added

- **Java Language Support for Trace**: Symbol extraction and call graph analysis now supports Java (#32)
  - Classes (with extends/implements, generics, sealed/non-sealed)
  - Inner and nested classes
  - Interfaces (including generic interfaces)
  - Annotations (`@interface`)
  - Enums (top-level and inner, with methods)
  - Records (Java 14+)
  - Methods with all modifiers (public, protected, private, static, final, abstract, synchronized, native, strictfp)
  - Constructors
  - Default interface methods (Java 8+)
  - Abstract methods
  - Java keywords added to filter out false positives
  - `.java` added to default traced languages

## [0.13.0] - 2026-01-12

### Added

- **Self-Update Command**: New `grepai update` command for automatic updates (#42)
  - `grepai update --check`: Check for available updates without installing
  - `grepai update`: Download and install the latest version from GitHub releases
  - `grepai update --force`: Force update even if already on latest version
  - Automatic platform detection (linux/darwin/windows, amd64/arm64)
  - SHA256 checksum verification before installation
  - Progress bar during download
  - Graceful error handling for network issues, rate limits, and permission errors

### Changed

- **Makefile**: Uses Docker for consistent linting with golangci-lint v1.64.2

## [0.12.0] - 2026-01-12

### Fixed

- **Custom OpenAI Endpoint**: Fixed `embedder.endpoint` config not being used for OpenAI provider (#35)
  - Enables Azure OpenAI and Microsoft Foundry support
  - Custom endpoints now correctly passed to the OpenAI embedder

### Added

- **Configurable Vector Dimensions**: New `embedder.dimensions` config option (#35)
  - Allows specifying vector dimensions per embedding model
  - PostgreSQL vector column automatically resizes to match configured dimensions
  - Backward compatible: old configs without `dimensions` use sensible defaults per provider

## [0.11.0] - 2026-01-12

### Added

- **Nested `.gitignore` Support**: Each subdirectory can now have its own `.gitignore` file (#40)
  - Patterns in nested `.gitignore` files apply only to their directory and subdirectories
  - Matches git's native behavior for hierarchical ignore rules
  - Example: `src/.gitignore` with `generated/` only ignores `src/generated/`, not `docs/generated/`

### Fixed

- **Directory Pattern Matching**: Patterns with trailing slash (e.g., `build/`) now correctly match the directory itself
  - Previously only matched contents inside the directory
  - Now triggers `filepath.SkipDir` for better performance on large repositories
  - Significantly improves indexing speed when ignoring `node_modules/`, `vendor/`, etc.

## [0.10.0] - 2026-01-11

### Added

- **Compact JSON Output**: New `--compact`/`-c` flag for `grepai search` command (#33)
  - Outputs minimal JSON without `content` field for ~80% token savings
  - Requires `--json` flag (returns error if used alone)
  - Recommended format for AI agents: `grepai search "query" --json --compact`
  - All agent setup templates updated to use `--json --compact` by default

## [0.9.0] - 2026-01-11

### Added

- **Claude Code Subagent**: New `--with-subagent` flag for `grepai agent-setup` (#17)
  - Creates `.claude/agents/deep-explore.md` for Claude Code
  - Provides a specialized exploration agent with grepai search and trace access
  - Uses `model: inherit` to match user's current model
  - Subagents operate in isolated context, ensuring grepai tools are available during exploration

## [0.8.1] - 2026-01-11

### Documentation
- Simplify Claude Code MCP setup: use `claude mcp add` command instead of manual JSON configuration

## [0.8.0] - 2026-01-11

### Added
- **MCP Server Mode**: New `grepai mcp-serve` command for Model Context Protocol integration (#18)
  - Exposes grepai as native MCP tools for AI agents (Claude Code, Cursor, Windsurf, etc.)
  - Available tools: `grepai_search`, `grepai_trace_callers`, `grepai_trace_callees`, `grepai_trace_graph`, `grepai_index_status`
  - Uses stdio transport for local MCP server communication
  - Structured JSON responses by default
  - Works automatically in subagents without explicit configuration

## [0.7.2] - 2026-01-11

### Documentation
- **Sidebar Reorganization**: Moved "Search Boost" and "Hybrid Search" from Configuration to Features section
- **Configuration Reference**: Updated full configuration reference with correct field names
  - Added missing options: `version`, `watch.debounce_ms`, `trace.mode`, `trace.enabled_languages`, `trace.exclude_patterns`
  - Fixed `scanner.ignore` → `ignore` (root level)
  - Fixed `store.postgres.connection_string` → `dsn`
  - Removed `store.gob.path` (handled automatically)
- **Trace Documentation**: Added missing supported languages (C, C++, Zig, Rust) to the languages table

## [0.7.1] - 2026-01-11

### Added
- **Agent Setup Trace Instructions**: Updated `grepai agent-setup` to include trace command documentation (#16)
  - Added "Call Graph Tracing" section with `trace callers`, `trace callees`, `trace graph` examples
  - All trace examples include `--json` flag for optimal AI agent integration
  - Updated workflow to include trace as step 2 for understanding function relationships

## [0.7.0] - 2026-01-10

### Added
- **Extended Language Support for Trace**: Symbol extraction now supports additional languages
  - C (`.c`, `.h`) - functions, structs, enums, typedefs
  - Zig (`.zig`) - functions, methods (inside structs/enums), inline/export/extern functions, structs, unions, enums, error sets, opaque types, nested types
  - Rust (`.rs`) - functions, methods, structs, enums, traits, type aliases
  - C++ (`.cpp`, `.hpp`, `.cc`, `.cxx`, `.hxx`) - functions, methods, classes, structs, enums
- **Default ignore patterns** for Zig and Rust build directories: `target`, `.zig-cache`, `zig-out`

## [0.6.0] - 2026-01-10

### Added
- **Search JSON Output**: New `--json`/`-j` flag for `grepai search` command
  - Machine-readable JSON output optimized for AI agents
  - Excludes internal fields (vector, hash, updated_at) to minimize token usage
  - Error handling outputs JSON format when flag is used
  - Closes #13

## [0.5.0] - 2026-01-10

### Added
- **Call Graph Tracing**: New `grepai trace` command for code navigation
  - `trace callers <symbol>` - find all functions calling a symbol
  - `trace callees <symbol>` - find all functions called by a symbol
  - `trace graph <symbol>` - build call graph with configurable depth
- Regex-based symbol extraction (fast mode) for Go, JS/TS, Python, PHP
- Tree-sitter integration (precise mode) with build tag `treesitter`
- Separate symbol index stored in `.grepai/symbols.gob`
- JSON output for AI agent integration (`--json` flag)
- Automatic symbol indexing during `grepai watch`

## [0.4.0] - 2026-01-10

### Added
- **LM Studio Provider**: New local embedding provider using LM Studio
  - Supports OpenAI-compatible API format
  - Configurable endpoint and model selection
  - Privacy-first alternative for local embeddings

## [0.3.0] - 2026-01-09

### Added
- **Search Boost**: Configurable score multipliers based on file paths
  - Penalize tests, mocks, fixtures, generated files, and docs
  - Boost source directories (`/src/`, `/lib/`, `/app/`)
  - Language-agnostic patterns, enabled by default
- **Hybrid Search**: Combine vector similarity with text matching
  - Uses Reciprocal Rank Fusion (RRF) algorithm
  - Configurable k parameter (default: 60)
  - Optional, disabled by default
- `GetAllChunks()` method to VectorStore interface for text search
- Dedicated documentation pages for Search Boost and Hybrid Search
- Feature cards on docs homepage

### Changed
- Searcher now accepts full SearchConfig instead of just BoostConfig

## [0.2.0] - 2026-01-09

### Added
- Initial release of grepai
- `grepai init` command for project initialization
- `grepai watch` command for real-time file indexing
- `grepai search` command for semantic code search
- `grepai agent-setup` command for AI agent integration
- Ollama embedding provider (local, privacy-first)
- OpenAI embedding provider
- GOB file storage backend (default)
- PostgreSQL with pgvector storage backend
- Gitignore support
- Binary file detection and exclusion
- Configurable chunk size and overlap
- Debounced file watching
- Cross-platform support (macOS, Linux, Windows)

### Security
- Privacy-first design with local embedding option
- No telemetry or data collection

## [0.1.0] - 2026-01-09

### Added
- Initial public release

[Unreleased]: https://github.com/yoanbernabeu/grepai/compare/v0.23.0...HEAD
[0.23.0]: https://github.com/yoanbernabeu/grepai/compare/v0.22.0...v0.23.0
[0.22.0]: https://github.com/yoanbernabeu/grepai/compare/v0.21.0...v0.22.0
[0.21.0]: https://github.com/yoanbernabeu/grepai/compare/v0.20.1...v0.21.0
[0.20.1]: https://github.com/yoanbernabeu/grepai/compare/v0.20.0...v0.20.1
[0.20.0]: https://github.com/yoanbernabeu/grepai/compare/v0.19.0...v0.20.0
[0.19.0]: https://github.com/yoanbernabeu/grepai/compare/v0.18.0...v0.19.0
[0.18.0]: https://github.com/yoanbernabeu/grepai/compare/v0.17.0...v0.18.0
[0.17.0]: https://github.com/yoanbernabeu/grepai/compare/v0.16.1...v0.17.0
[0.16.1]: https://github.com/yoanbernabeu/grepai/compare/v0.16.0...v0.16.1
[0.16.0]: https://github.com/yoanbernabeu/grepai/compare/v0.15.1...v0.16.0
[0.15.1]: https://github.com/yoanbernabeu/grepai/compare/v0.15.0...v0.15.1
[0.15.0]: https://github.com/yoanbernabeu/grepai/compare/v0.14.0...v0.15.0
[0.14.0]: https://github.com/yoanbernabeu/grepai/compare/v0.13.0...v0.14.0
[0.13.0]: https://github.com/yoanbernabeu/grepai/compare/v0.12.0...v0.13.0
[0.12.0]: https://github.com/yoanbernabeu/grepai/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/yoanbernabeu/grepai/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/yoanbernabeu/grepai/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/yoanbernabeu/grepai/compare/v0.8.1...v0.9.0
[0.8.1]: https://github.com/yoanbernabeu/grepai/compare/v0.8.0...v0.8.1
[0.8.0]: https://github.com/yoanbernabeu/grepai/compare/v0.7.2...v0.8.0
[0.7.2]: https://github.com/yoanbernabeu/grepai/compare/v0.7.1...v0.7.2
[0.7.1]: https://github.com/yoanbernabeu/grepai/compare/v0.7.0...v0.7.1
[0.7.0]: https://github.com/yoanbernabeu/grepai/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/yoanbernabeu/grepai/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/yoanbernabeu/grepai/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/yoanbernabeu/grepai/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/yoanbernabeu/grepai/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/yoanbernabeu/grepai/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/yoanbernabeu/grepai/releases/tag/v0.1.0
