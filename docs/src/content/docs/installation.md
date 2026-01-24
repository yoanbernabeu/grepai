---
title: Installation
description: How to install grepai
---

## Prerequisites

- **Ollama** (for local embeddings) or an **OpenAI API key** (for cloud embeddings)

## Quick Install (Recommended)

```bash
curl -sSL https://raw.githubusercontent.com/yoanbernabeu/grepai/main/install.sh | sh
```

Or download directly from [Releases](https://github.com/yoanbernabeu/grepai/releases).

## Install from Source

Requires **Go 1.24+**.

```bash
# Clone the repository
git clone https://github.com/yoanbernabeu/grepai.git
cd grepai

# Build the binary
make build

# The binary is created at ./bin/grepai
# Move it to your PATH
sudo mv ./bin/grepai /usr/local/bin/
```

## Install on Windows (PowerShell)

Run the following command in your PowerShell terminal to install `grepai` automatically:

```powershell
irm https://raw.githubusercontent.com/yoanbernabeu/grepai/main/install.ps1 | iex
```

## Install Ollama (Recommended)

For privacy-first local embeddings, install Ollama:

```bash
# macOS
brew install ollama

# Linux
curl -fsSL https://ollama.com/install.sh | sh

# Start Ollama
ollama serve

# Pull the embedding model
ollama pull nomic-embed-text
```

## Verify Installation

```bash
# Check grepai is installed
grepai version

# Check Ollama is running (if using local embeddings)
curl http://localhost:11434/api/tags
```

## Updating

Keep grepai up to date with the built-in update command:

```bash
# Check for available updates
grepai update --check

# Download and install the latest version
grepai update
```

The update command will:
- Fetch the latest release from GitHub
- Download the appropriate binary for your platform
- Verify checksum integrity
- Replace the current binary automatically

## Next Steps

- [Quick Start](/grepai/quickstart/) - Initialize and start using grepai
- [Configuration](/grepai/configuration/) - Configure embedders and storage backends
