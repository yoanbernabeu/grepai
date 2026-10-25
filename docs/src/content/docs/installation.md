---
title: Installation
description: Install grepai on macOS, Linux or Windows (Homebrew, install script, Nix or source), set up Ollama, and enable shell completion.
---

This page shows how to install the `grepai` binary and an embedding provider. It takes a few minutes. When you are done, continue with the [Quick Start](/grepai/quickstart/).

Short version for macOS with local embeddings:

```bash
brew install yoanbernabeu/tap/grepai
brew install ollama
ollama serve &              # skip if Ollama already runs as an app or service
ollama pull nomic-embed-text
grepai version
```

## Prerequisites

grepai needs an embedding provider to turn code into vectors. Pick one:

- **Ollama** (default, local): free, and your code stays on your machine. See [Install Ollama](#install-ollama-recommended) below.
- **LM Studio** (local): uses the OpenAI-compatible server built into LM Studio.
- **A cloud API**: OpenAI, OpenRouter, Synthetic or Requesty. You need an API key.

See [Embedders](/grepai/backends/embedders/) for details on each provider.

Prebuilt binaries are available for Linux, macOS and Windows on `amd64` and `arm64`.

## Homebrew (macOS)

```bash
brew install yoanbernabeu/tap/grepai
```

## Shell Script (Linux/macOS)

```bash
curl -sSL https://raw.githubusercontent.com/yoanbernabeu/grepai/main/install.sh | sh
```

The script downloads the latest release for your OS and architecture and installs it to `/usr/local/bin` (it uses `sudo` if that directory is not writable). To install somewhere else, set `INSTALL_DIR`:

```bash
curl -sSL https://raw.githubusercontent.com/yoanbernabeu/grepai/main/install.sh | INSTALL_DIR="$HOME/.local/bin" sh
```

You can also download an archive directly from [GitHub Releases](https://github.com/yoanbernabeu/grepai/releases). Each release includes a `checksums.txt` file.

## Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/yoanbernabeu/grepai/main/install.ps1 | iex
```

The script installs `grepai.exe` to `%LOCALAPPDATA%\Programs\grepai` and adds that folder to your user `PATH`. It installs the `amd64` build. On Windows ARM, download the `windows_arm64` archive from [GitHub Releases](https://github.com/yoanbernabeu/grepai/releases).

## Nix

The repository provides a Nix flake:

```bash
# Run without installing
nix run github:yoanbernabeu/grepai -- version

# Install into your profile
nix profile install github:yoanbernabeu/grepai
```

## Install from Source

Requires **Go 1.25.5** or later (see `go.mod`).

```bash
git clone https://github.com/yoanbernabeu/grepai.git
cd grepai
make build                         # creates ./bin/grepai
sudo mv ./bin/grepai /usr/local/bin/
```

Or install directly into your `GOBIN` with:

```bash
go install github.com/yoanbernabeu/grepai/cmd/grepai@latest
```

Source builds report a placeholder version: `0.1.0` with `make build` (override it with `make build VERSION=x.y.z`) and `dev` with `go install`.

> **Note:** Release binaries are built without CGO. The experimental `ast` chunking strategy needs a build with the `treesitter` tag and CGO enabled: `CGO_ENABLED=1 go build -tags treesitter ./cmd/grepai`. See [Chunking Strategy](/grepai/configuration/#chunking-strategy).

## Install Ollama (Recommended)

Ollama runs the embedding model locally. grepai uses `nomic-embed-text` by default.

```bash
# macOS
brew install ollama

# Linux
curl -fsSL https://ollama.com/install.sh | sh

# Start the server (skip if Ollama already runs as an app or service)
ollama serve

# In another terminal, download the default embedding model
ollama pull nomic-embed-text
```

grepai expects Ollama at `http://localhost:11434`. You can change this with `embedder.endpoint` in the [configuration](/grepai/configuration/).

## Verify Installation

```bash
grepai version
# grepai version 0.39.0

# If you use Ollama: check that the server answers and lists nomic-embed-text
curl http://localhost:11434/api/tags
```

## Shell Completion

grepai can generate completion scripts for bash, zsh, fish and PowerShell. Completion covers commands, flags, and values such as providers, backends and workspace names.

```bash
# Zsh (add to ~/.zshrc)
eval "$(grepai completion zsh)"

# Bash on Linux
grepai completion bash > /etc/bash_completion.d/grepai

# Bash on macOS (requires bash-completion@2)
grepai completion bash > $(brew --prefix)/etc/bash_completion.d/grepai

# Fish
grepai completion fish > ~/.config/fish/completions/grepai.fish
```

```powershell
# PowerShell
grepai completion powershell | Out-String | Invoke-Expression
```

Run `grepai completion --help` for more options, such as an Oh My Zsh plugin. See also [grepai completion](/grepai/commands/grepai_completion/).

## Updating

If you installed with Homebrew:

```bash
brew upgrade grepai
```

If you installed with the shell script, the PowerShell script or a downloaded archive:

```bash
grepai update --check   # only check whether a newer version exists
grepai update           # download and install the latest version
grepai update --force   # reinstall even if you are already up to date
```

`grepai update` downloads the release archive for your platform from GitHub, verifies it against `checksums.txt`, and replaces the current binary. Restart any running `grepai watch` afterwards.

If you installed with Nix or from source, update the same way you installed.

## Next Steps

- [Quick Start](/grepai/quickstart/): initialize a project and run your first search
- [Configuration](/grepai/configuration/): choose an embedder and a storage backend
