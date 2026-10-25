---
title: Contributing
description: grepai works issue first. How to report a bug, request a feature, test a fix or list your tool, and how to build grepai from source.
---

grepai works **issue first**: every contribution starts as an issue, and the project does not accept pull requests. The same rules are in the repository's [CONTRIBUTING.md](https://github.com/yoanbernabeu/grepai/blob/main/CONTRIBUTING.md).

## Why Issue First

With AI coding tools, writing a fix is rarely the hard part. The hard part is knowing exactly what is wrong, on which setup, and what the right behavior is. A precise issue carries all of that. Reviewing an outside pull request, checking it does not break other setups, and reworking it to fit the rest of the code usually takes longer than implementing the fix from a good issue.

So the most useful thing you can send is a detailed issue. The maintainer implements it, tests it against the current release on real projects, and credits you in the changelog. Pull requests opened anyway are closed with a link to open an issue instead.

## Open an Issue

| You have | Where | Include |
|---|---|---|
| A bug | [Bug report form](https://github.com/yoanbernabeu/grepai/issues/new?template=bug_report.yml) | `grepai version`, OS, embedder and store from `.grepai/config.yaml` (no keys), exact commands, expected vs actual result, logs |
| A feature idea | [Feature request form](https://github.com/yoanbernabeu/grepai/issues/new?template=feature_request.yml) | The problem you hit and your real use case, then the solution you have in mind |
| A question | [Discussions](https://github.com/yoanbernabeu/grepai/discussions) | What you tried |
| A security issue | [SECURITY.md](https://github.com/yoanbernabeu/grepai/blob/main/SECURITY.md) | Not a public issue |

Search the [existing issues](https://github.com/yoanbernabeu/grepai/issues?q=is%3Aissue) first. If yours is there, add your setup and logs to it: a second reproduction is often what unblocks a fix.

## Other Ways to Help

- **Test a fix.** When a fix lands on a branch, building it and confirming it works on your project is very valuable (see below).
- **Improve the docs.** If a page is wrong, unclear or out of date, open an issue that quotes the passage and says what is wrong.
- **List your tool.** Built something around grepai? See [Community Tools](/grepai/community-tools/#contributing-your-tool).
- **Talk about it.** A star, a post or a talk helps other people find grepai.

## Building From Source

You need:

- Go 1.25.5 or later (the `go` directive in `go.mod`)
- Git
- Docker, for `make lint` (it runs golangci-lint in a container)

```bash
git clone https://github.com/yoanbernabeu/grepai.git
cd grepai
make build        # builds bin/grepai
make test         # tests with the race detector
```

To try a branch that contains a fix:

```bash
git fetch origin <branch>
git checkout <branch>
make build
./bin/grepai version
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
