# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/yoanbernabeu/grepai/compare/v0.8.1...HEAD
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
