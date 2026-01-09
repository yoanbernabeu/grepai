---
title: Contributing
description: How to contribute to grepai
---

## Getting Started

1. Fork and clone the repository:
```bash
git clone https://github.com/YOUR_USERNAME/grepai.git
cd grepai
```

2. Install dependencies:
```bash
go mod download
```

3. Run tests:
```bash
make test
```

4. Build:
```bash
make build
```

## Development Commands

```bash
# Build the binary
make build

# Run tests with race detection
make test

# Run tests with coverage
make test-cover

# Format code with gofmt
make fmt

# Lint with golangci-lint
make lint

# Run ALL checks before committing (recommended)
make pre-commit

# Build and run
make run

# Cross-compile for all platforms
make build-all

# Generate CLI documentation
make docs-generate
```

## Before Committing

Always run the pre-commit checks before pushing your changes:

```bash
make pre-commit
```

This single command will:
1. **Format** all Go files with `gofmt`
2. **Vet** - detect common errors with `go vet`
3. **Lint** - run comprehensive checks with `golangci-lint`
4. **Test** - run all tests with race detection

If all checks pass, you're ready to commit!

## Project Structure

```
grepai/
├── cli/              # CLI commands (Cobra)
│   ├── root.go
│   ├── init.go
│   ├── watch.go
│   ├── search.go
│   └── status.go
├── config/           # Configuration loading
├── embedder/         # Embedding providers
│   ├── embedder.go   # Interface
│   ├── ollama.go
│   └── openai.go
├── store/            # Vector storage
│   ├── store.go      # Interface
│   ├── gob.go
│   └── postgres.go
├── indexer/          # Indexing logic
│   ├── scanner.go    # File walking
│   ├── chunker.go    # Text splitting
│   └── indexer.go    # Orchestration
├── search/           # Search logic
├── watcher/          # File watching
└── docs/             # Documentation (Astro/Starlight)
```

## Adding a New Embedder

1. Create a new file in `embedder/`:

```go
// embedder/myembedder.go
package embedder

type MyEmbedder struct {
    // ...
}

func NewMyEmbedder(config Config) (*MyEmbedder, error) {
    // ...
}

func (e *MyEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
    // ...
}

func (e *MyEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
    // ...
}

func (e *MyEmbedder) Dimensions() int {
    return 768 // or whatever your embedder produces
}
```

2. Add configuration in `config/config.go`

3. Wire it up in CLI commands

## Adding a New Store

1. Create a new file in `store/`:

```go
// store/mystore.go
package store

type MyStore struct {
    // ...
}

func NewMyStore(config Config) (*MyStore, error) {
    // ...
}

func (s *MyStore) Store(ctx context.Context, chunks []Chunk) error {
    // ...
}

func (s *MyStore) Search(ctx context.Context, embedding []float32, limit int) ([]Result, error) {
    // ...
}

func (s *MyStore) Delete(ctx context.Context, filePath string) error {
    // ...
}

func (s *MyStore) Close() error {
    // ...
}
```

2. Add configuration in `config/config.go`

3. Wire it up in CLI commands

## Commit Convention

Follow conventional commits:

```
type(scope): description

Types: feat, fix, docs, style, refactor, test, chore
```

Examples:
```
feat(embedder): add support for Cohere embeddings
fix(watcher): handle symlink loops gracefully
docs(readme): update installation instructions
```

## Pull Request Process

1. Create a feature branch:
```bash
git checkout -b feat/my-feature
```

2. Make your changes and commit

3. Run pre-commit checks:
```bash
make pre-commit
```

4. Push and create a PR:
```bash
git push origin feat/my-feature
```

5. Fill out the PR template

## Code Style

- Follow standard Go conventions
- Run `make lint` before committing
- Keep functions focused and small
- Add tests for new functionality
- Document exported types and functions
