# Contributing to grepai

Thank you for your interest in contributing to grepai! This document provides guidelines and instructions for contributing.

## Getting Started

### Prerequisites

- Go 1.22 or later
- Ollama (for local testing with embeddings)
- Git

### Setup

1. Fork the repository
2. Clone your fork:
   ```bash
   git clone https://github.com/YOUR_USERNAME/grepai.git
   cd grepai
   ```
3. Add the upstream remote:
   ```bash
   git remote add upstream https://github.com/yoanbernabeu/grepai.git
   ```
4. Install dependencies:
   ```bash
   go mod download
   ```

## Development Workflow

### Building

```bash
make build
# or
go build ./cmd/grepai
```

### Running Tests

```bash
make test
# or
go test -v -race ./...
```

### Linting

We use golangci-lint for code quality:

```bash
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run linter
make lint
```

### Before Committing

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

### Code Style

- Follow standard Go conventions and idioms
- Use `gofmt` or `goimports` to format your code
- Write meaningful commit messages
- Add tests for new functionality
- Update documentation as needed

## Making Changes

### Branch Naming

- `feat/description` - New features
- `fix/description` - Bug fixes
- `docs/description` - Documentation changes
- `refactor/description` - Code refactoring
- `test/description` - Test additions or modifications

### Commit Messages

We follow conventional commits:

```
type(scope): description

[optional body]

[optional footer]
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

Examples:
- `feat(search): add fuzzy matching option`
- `fix(watcher): handle file rename events correctly`
- `docs(readme): update installation instructions`

### Pull Request Process

1. Create a feature branch from `main`
2. Make your changes
3. Ensure all tests pass
4. Run the linter and fix any issues
5. Update CHANGELOG.md if applicable
6. Submit a pull request

### Pull Request Checklist

- [ ] Tests added/updated
- [ ] Documentation updated
- [ ] CHANGELOG.md updated (for user-facing changes)
- [ ] Code follows project style guidelines
- [ ] All CI checks pass

## Project Structure

```
grepai/
├── cmd/grepai/      # CLI entry point
├── cli/             # Cobra commands
├── config/          # Configuration management
├── embedder/        # Embedding providers (Ollama, OpenAI)
├── store/           # Vector storage backends (GOB, Postgres)
├── indexer/         # File scanning and chunking
├── watcher/         # File system watcher
└── search/          # Search functionality
```

## Adding New Features

### New Embedding Provider

1. Create a new file in `embedder/` (e.g., `embedder/newprovider.go`)
2. Implement the `Embedder` interface
3. Add configuration options in `config/config.go`
4. Update CLI initialization in `cli/watch.go` and `cli/search.go`
5. Add tests in `embedder/newprovider_test.go`
6. Update documentation

### New Storage Backend

1. Create a new file in `store/` (e.g., `store/newbackend.go`)
2. Implement the `VectorStore` interface
3. Add configuration options in `config/config.go`
4. Update CLI initialization
5. Add tests
6. Update documentation

## Getting Help

- Open an issue for bugs or feature requests
- Start a discussion for questions
- Check existing issues and discussions first

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
