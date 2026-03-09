---
title: Docker
description: Run grepai in a Docker container
---

The official grepai Docker image is published on GitHub Container Registry (GHCR). It uses a `FROM scratch` image (~30MB) containing only the static binary and CA certificates.

## Prerequisites

- Docker 20.10+
- An embedding provider: [Ollama](https://ollama.ai) running locally, or an OpenAI/OpenRouter/Synthetic API key

## Quick Start

### With Ollama (default)

```bash
docker run -v /path/to/project:/workspace \
  -e GREPAI_PROVIDER=ollama \
  -e GREPAI_ENDPOINT=http://host.docker.internal:11434 \
  ghcr.io/yoanbernabeu/grepai
```

> `host.docker.internal` allows the container to reach Ollama running on your host machine.

### With OpenAI

```bash
docker run -v /path/to/project:/workspace \
  -e GREPAI_PROVIDER=openai \
  -e GREPAI_API_KEY=sk-... \
  ghcr.io/yoanbernabeu/grepai
```

### With OpenRouter

```bash
docker run -v /path/to/project:/workspace \
  -e GREPAI_PROVIDER=openrouter \
  -e GREPAI_API_KEY=sk-or-... \
  ghcr.io/yoanbernabeu/grepai
```

### Running ad-hoc commands

Since the image has no shell, override the entrypoint to run other grepai commands:

```bash
docker run --rm --entrypoint /grepai ghcr.io/yoanbernabeu/grepai version
```

## Environment Variables

All configuration is done via environment variables. On first run, the `--auto-init` flag generates `.grepai/config.yaml` from these variables. If the config already exists, it is **not** overwritten.

### Embedder

| Variable | Config field | Default |
|---|---|---|
| `GREPAI_PROVIDER` | `embedder.provider` | `ollama` |
| `GREPAI_MODEL` | `embedder.model` | *(auto per provider)* |
| `GREPAI_ENDPOINT` | `embedder.endpoint` | *(auto per provider)* |
| `GREPAI_API_KEY` | `embedder.api_key` | *(empty)* |
| `GREPAI_DIMENSIONS` | `embedder.dimensions` | *(auto per provider)* |
| `GREPAI_PARALLELISM` | `embedder.parallelism` | `4` |

Provider defaults:

| Provider | Model | Endpoint | Dimensions |
|---|---|---|---|
| `ollama` | `nomic-embed-text` | `http://localhost:11434` | `768` |
| `lmstudio` | `text-embedding-nomic-embed-text-v1.5` | `http://127.0.0.1:1234` | `768` |
| `openai` | `text-embedding-3-small` | `https://api.openai.com/v1` | *(native)* |
| `synthetic` | `hf:nomic-ai/nomic-embed-text-v1.5` | `https://api.synthetic.new/openai/v1` | `768` |
| `openrouter` | `openai/text-embedding-3-small` | `https://openrouter.ai/api/v1` | *(native)* |

### Store

| Variable | Config field | Default |
|---|---|---|
| `GREPAI_BACKEND` | `store.backend` | `gob` |
| `GREPAI_POSTGRES_DSN` | `store.postgres.dsn` | `postgres://localhost:5432/grepai` |
| `GREPAI_QDRANT_ENDPOINT` | `store.qdrant.endpoint` | `localhost` |
| `GREPAI_QDRANT_PORT` | `store.qdrant.port` | `6334` |
| `GREPAI_QDRANT_COLLECTION` | `store.qdrant.collection` | *(auto)* |
| `GREPAI_QDRANT_API_KEY` | `store.qdrant.api_key` | *(empty)* |
| `GREPAI_QDRANT_USE_TLS` | `store.qdrant.use_tls` | `false` |

### Chunking

| Variable | Config field | Default |
|---|---|---|
| `GREPAI_CHUNKING_SIZE` | `chunking.size` | `512` |
| `GREPAI_CHUNKING_OVERLAP` | `chunking.overlap` | `50` |

## Docker Compose

The project includes a `compose.yaml` with a `watch` profile:

```bash
docker compose --profile=watch up
```

This mounts the current directory and connects to Ollama on the host. Customize with a `.env` file or override the environment variables:

```yaml
services:
  grepai:
    image: ghcr.io/yoanbernabeu/grepai:latest
    environment:
      - GREPAI_PROVIDER=ollama
      - GREPAI_ENDPOINT=http://host.docker.internal:11434
    volumes:
      - ./:/workspace
    profiles:
      - watch
```

### With Qdrant

```yaml
services:
  grepai:
    image: ghcr.io/yoanbernabeu/grepai:latest
    environment:
      - GREPAI_PROVIDER=ollama
      - GREPAI_ENDPOINT=http://host.docker.internal:11434
      - GREPAI_BACKEND=qdrant
      - GREPAI_QDRANT_ENDPOINT=qdrant
    volumes:
      - ./:/workspace
    depends_on:
      - qdrant

  qdrant:
    image: qdrant/qdrant:latest
    ports:
      - "6333:6333"
      - "6334:6334"
```

### With PostgreSQL

```yaml
services:
  grepai:
    image: ghcr.io/yoanbernabeu/grepai:latest
    environment:
      - GREPAI_PROVIDER=ollama
      - GREPAI_ENDPOINT=http://host.docker.internal:11434
      - GREPAI_BACKEND=postgres
      - GREPAI_POSTGRES_DSN=postgres://grepai:grepai@postgres:5432/grepai
    volumes:
      - ./:/workspace
    depends_on:
      - postgres

  postgres:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_USER: grepai
      POSTGRES_PASSWORD: grepai
      POSTGRES_DB: grepai
```

## Data Persistence

The index is stored inside the mounted volume at `/workspace/.grepai/`. As long as you mount the same project directory, the index persists across container restarts.

For GOB backend (default), all data is in `/workspace/.grepai/index.gob`. For Qdrant or PostgreSQL backends, the vector data is stored externally in the respective database.

## Build Locally

```bash
# Build the image
docker build -t grepai .

# Run it
docker run -v /path/to/project:/workspace \
  -e GREPAI_PROVIDER=ollama \
  -e GREPAI_ENDPOINT=http://host.docker.internal:11434 \
  grepai

# Multi-arch build
docker buildx build --platform linux/amd64,linux/arm64 -t grepai .
```
