---
title: Contributing
description: Set up a grepai development environment, run the checks, and add embedders, stores or framework processors before you open a pull request.
---

This page explains how to build grepai from source, run the checks, and extend it. The repository's [CONTRIBUTING.md](https://github.com/yoanbernabeu/grepai/blob/main/CONTRIBUTING.md) covers the same workflow.

## Getting Started

You need:

- Go 1.25.5 or later (the `go` directive in `go.mod`)
- Git
- Docker, for `make lint` and `make pre-commit` (they run golangci-lint in a container)

Then:

1. Fork the repository and clone your fork:

   ```bash
   git clone https://github.com/YOUR_USERNAME/grepai.git
   cd grepai
   ```

2. Download dependencies:

   ```bash
   go mod download
   ```

3. Run the tests:

   ```bash
   make test
   ```

4. Build the binary into `bin/grepai`:

   ```bash
   make build
   ```

## Development Commands

| Command | What it does |
|---------|--------------|
| `make build` | Build `bin/grepai` |
| `make install` | Install `grepai` with `go install` |
| `make run` | Build, then run `bin/grepai` |
| `make test` | Run all tests with the race detector (`go test -v -race ./...`) |
| `make test-cover` | Run tests and write `coverage.out` and `coverage.html` |
| `make fmt` | Format all Go files with `gofmt -w .` |
| `make lint` | Run golangci-lint v2.13.2 in Docker |
| `make lint-local` | Run your locally installed `golangci-lint` |
| `make pre-commit` | Format, vet, lint and test (see below) |
| `make build-all` | Cross-compile for Linux, macOS and Windows into `bin/` |
| `make docs-generate` | Regenerate the CLI reference pages in `docs/src/content/docs/commands/` |
| `make docs-dev` | Regenerate the CLI pages, then start the docs dev server |
| `make clean` | Remove `bin/` and coverage files |

The default build has no AST chunking or tree-sitter extraction. To build with them, add the `treesitter` build tag (this needs CGO and a C compiler):

```bash
go build -tags treesitter -o bin/grepai ./cmd/grepai
```

## Before Committing

Run the pre-commit checks before you push:

```bash
make pre-commit
```

It runs, in order:

1. **Format**: `gofmt -w .`
2. **Vet**: `go vet ./...`
3. **Lint**: golangci-lint v2.13.2 in Docker, with the rules in `.golangci.yml`
4. **Test**: `go test -race ./...`

CI runs the same lint and tests on pull requests, with tests on Linux, macOS and Windows.

## Project Structure

```text
grepai/
├── cmd/
│   ├── grepai/       # CLI entry point
│   └── gendocs/      # Generates the CLI reference pages
├── cli/              # Cobra commands (init, watch, index, search, trace, refs, rpg, mcp-serve, ...)
├── config/           # Project and workspace configuration
├── daemon/           # Background watch daemon
├── embedder/         # Embedding providers and factory.go
├── framework/        # Framework processors (Vue, Svelte, Astro, Solid)
├── git/              # Git and worktree detection
├── indexer/          # Scanner, chunker and indexing orchestration
├── mcp/              # MCP server and tools
├── rpg/              # Repository Planning Graph
├── search/           # Search, hybrid search, boosting
├── stats/            # Token savings stats
├── store/            # Vector stores (GOB, PostgreSQL, Qdrant)
├── trace/            # Symbol extraction and call graphs
├── updater/          # Self-update
├── watcher/          # File watching
└── docs/             # Documentation site (Astro)
```

## Adding a New Embedder

1. Create a file in `embedder/` that implements the `Embedder` interface from `embedder/embedder.go`:

   ```go
   type Embedder interface {
       // Embed converts text into a vector embedding
       Embed(ctx context.Context, text string) ([]float32, error)

       // EmbedBatch converts multiple texts into vector embeddings
       EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

       // Dimensions returns the vector dimension size for this embedder
       Dimensions() int

       // Close cleanly shuts down the embedder
       Close() error
   }
   ```

   Providers that support parallel batching can also implement `BatchEmbedder` from the same file.

2. Add the provider's configuration in `config/config.go`.
3. Add a `case` for the provider in `embedder/factory.go` (`NewFromConfig` and `NewFromWorkspaceConfig`).
4. Add the provider to the `grepai init` prompts and flags in `cli/init.go`, and to shell completion in `cli/completion.go`.
5. Add tests and update the [embedders page](/grepai/backends/embedders/).

## Adding a New Store

1. Create a file in `store/` that implements the `VectorStore` interface from `store/store.go`. It covers chunks (`SaveChunks`, `DeleteByFile`, `Search`, `GetChunksForFile`, `GetAllChunks`), documents (`GetDocument`, `SaveDocument`, `DeleteDocument`, `ListDocuments`), lifecycle (`Load`, `Persist`, `Close`) and stats (`GetStats`, `ListFilesWithStats`). Read the interface in `store/store.go` for the exact signatures.

   A store can also implement `EmbeddingCache` (same file) to reuse embeddings for identical content.

2. Add the backend's configuration in `config/config.go`.
3. Create the store everywhere the other backends are created. Search the code for `case "qdrant"`: today that finds `cli/watch.go`, `cli/search.go`, `cli/status.go`, `cli/workspace.go`, `mcp/server.go` and the `init` and workspace prompts (`cli/tui_init.go`, `cli/tui_workspace.go`).
4. Add tests and update the [stores page](/grepai/backends/stores/).

## Adding a Framework Processor

Framework processors turn framework files (Vue, Svelte, Astro, ...) into text that indexing and tracing can handle, while keeping a mapping back to the source lines.

### 1. Implement the processor contract

Create a file under `framework/` that implements `FrameworkProcessor` from `framework/types.go`:

- `Name() string`
- `Supports(filePath string) bool`
- `Capabilities() ProcessorCapabilities`
- `TransformForEmbedding(ctx, filePath, source)`
- `TransformForTrace(ctx, filePath, source)`

Return `ErrNotImplemented` from scaffold placeholders, and `ErrUnavailable` when a runtime or compiler dependency is missing (both in `framework/errors.go`).

### 2. Respect mapping invariants

- `GeneratedToSourceLine` is 1-based.
- Use `0` for generated lines with no reliable source line.
- Persisted symbols and chunks must always reference the original source path.
- Keep the embedding text and the displayed snippet separate when the transformed output differs from the source.

### 3. Register processor and config flags

- Add the processor to `buildFrameworkRegistry` in `cli/watch.go`.
- Add an enable toggle in `config/config.go`, under `framework_processing.frameworks`.
- Honor the global mode: `auto`, `require` or `off`.

### 4. Required tests

- Registry routing by file extension
- `auto` fallback behavior
- `require` failure behavior
- Correct source line remapping
- Regression tests for non-framework files

### 5. Scaffold examples

These files are placeholders that show the expected structure. Their transform methods return `ErrNotImplemented`:

- `framework/svelte_processor.go`
- `framework/astro_processor.go`
- `framework/solid_processor.go`

`framework/vue_processor.go` is the working implementation.

## Commit Convention

Use [conventional commits](https://www.conventionalcommits.org/):

```text
type(scope): description
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`.

Examples:

```text
feat(embedder): add support for Cohere embeddings
fix(watcher): handle symlink loops gracefully
docs(readme): update installation instructions
```

## Pull Request Process

1. Create a branch from `main`:

   ```bash
   git checkout -b feat/my-feature
   ```

2. Make your changes and commit them.
3. Run the checks:

   ```bash
   make pre-commit
   ```

4. Push the branch and open a pull request against `main`:

   ```bash
   git push origin feat/my-feature
   ```

5. Fill in the pull request template. Update `CHANGELOG.md` for user-facing changes.

## Code Style

- Follow standard Go conventions.
- Run `make lint` before committing.
- Keep functions small and focused.
- Add tests for new behavior.
- Document exported types and functions.
