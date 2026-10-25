---
title: Vector Stores
description: Choose and configure where grepai stores its vectors, a local GOB file, PostgreSQL with pgvector, or Qdrant.
---

A vector store keeps the embeddings of your code and finds the chunks closest to a search query. This page helps you pick a backend and shows how to configure each one.

## Available Stores

| Backend | Where data lives | Setup | Shared index | Workspaces |
|---------|------------------|-------|--------------|------------|
| `gob` (default) | `.grepai/index.gob` in the project | None | No | No |
| `postgres` | PostgreSQL database with pgvector | PostgreSQL server | Yes | Yes |
| `qdrant` | Qdrant collection | Qdrant server or Qdrant Cloud | Yes | Yes |

How to choose:

- **One developer, one project:** keep `gob`. There is nothing to install.
- **Index shared by several machines, worktrees or projects, or [workspaces](/grepai/workspace/):** use `postgres` or `qdrant`. Workspaces reject the GOB backend.
- **You already run PostgreSQL:** use `postgres`. **You prefer a dedicated vector database:** use `qdrant`.

Pick the backend during `grepai init`, or pass it directly:

```bash
grepai init --backend qdrant
```

All three backends reuse a stored vector when a chunk with identical content is already indexed, which saves embedding calls.

## GOB (File-based)

The default backend. grepai keeps the whole index in memory and saves it to `.grepai/index.gob` with Go's `gob` encoding.

### Configuration

```yaml
store:
  backend: gob
```

There are no other options. The file path is always `.grepai/index.gob` in the project root.

### Characteristics

- **No dependencies:** nothing to install or run.
- **In memory:** the full index is loaded into RAM, and each search compares the query with every chunk.
- **Safe writes:** grepai writes to a temporary file and then replaces `index.gob`, so an interrupted save does not leave a half-written index.
- **Shared between grepai processes on one machine:** a lock file (`index.gob.lock`) lets `grepai watch` and `grepai search` use the same index at the same time.
- **Self-healing:** if `index.gob` cannot be read, grepai moves it to `index.gob.corrupt`, starts from an empty index and rebuilds it on the next scan.
- **Local only:** the file cannot be shared across machines, and each git worktree has its own index.

### Best For

- Personal projects and single repositories
- Quick experiments
- CI jobs that build a throwaway index

## PostgreSQL with pgvector

Stores vectors in PostgreSQL using the [pgvector](https://github.com/pgvector/pgvector) extension. Several projects can share one database: each row is tagged with the project's absolute path (or `workspace:<name>` for a [workspace](/grepai/workspace/)).

### Setup

The simplest way to get PostgreSQL with pgvector is the `compose.yaml` file at the root of the grepai repository:

```bash
docker compose --profile postgres up -d
```

It starts `pgvector/pgvector:pg16` on port 5432 with user, password and database all set to `grepai`.

To use your own server, install pgvector and create a database:

```sql
CREATE DATABASE grepai;
```

On first connection, grepai creates everything else itself: the `vector` extension (`CREATE EXTENSION IF NOT EXISTS vector`, which needs sufficient privileges), the `chunks` and `documents` tables, and their indexes. The vector column is sized from the embedder's [dimensions](/grepai/backends/embedders/#changing-embedding-models).

### Configuration

In interactive mode, `grepai init` asks for the PostgreSQL DSN. With `grepai init --backend postgres`, set it in `.grepai/config.yaml`:

```yaml
store:
  backend: postgres
  postgres:
    dsn: postgres://grepai:grepai@localhost:5432/grepai
```

The DSN accepts any connection string supported by [pgx](https://github.com/jackc/pgx), for example `postgres://user:password@host:5432/dbname?sslmode=require`.

### Environment Variable

grepai does not read the DSN from an environment variable, and it does not expand `${VAR}` placeholders in `config.yaml`. Write the DSN directly in the `dsn` key.

### Characteristics

- **Shared index:** several machines, worktrees and projects can use the same database.
- **Exact search:** grepai does not create a vector index (such as HNSW or IVFFlat). Each search ranks all chunks of the project by cosine distance.
- **One vector size per database:** all projects in a database share the same `vector` column. Projects that use embedders with different dimensions need separate databases.
- **Requires a running PostgreSQL server.**

### Best For

- Teams that share one index
- Multi-project [workspaces](/grepai/workspace/)
- Environments that already run PostgreSQL

### Re-indexing After a Model Change

grepai detects an embedding model change and rebuilds the project's rows on the next `grepai index` or `grepai watch` (see [Re-indexing After Model Change](/grepai/backends/embedders/#re-indexing-after-model-change)). To clear a project by hand anyway, use its absolute path:

```sql
DELETE FROM chunks    WHERE project_id = '/absolute/path/to/project';
DELETE FROM documents WHERE project_id = '/absolute/path/to/project';
```

## Qdrant

Stores vectors in a [Qdrant](https://qdrant.tech) collection. grepai talks to Qdrant over gRPC (port 6334 by default).

### Setup

Start Qdrant with the `compose.yaml` file at the root of the grepai repository:

```bash
docker compose --profile qdrant up -d
```

It exposes the HTTP API and dashboard on port 6333 (`http://localhost:6333/dashboard`) and gRPC on port 6334.

Then initialize grepai:

```bash
grepai init --backend qdrant
```

With `--backend qdrant`, grepai connects to `localhost:6334`. In interactive mode, `grepai init` asks for the endpoint, port, TLS, collection name and API key.

### Configuration

```yaml
store:
  backend: qdrant
  qdrant:
    endpoint: localhost      # host name only
    port: 6334               # gRPC port (default: 6334)
    use_tls: false           # true for Qdrant Cloud
    collection: myproject    # optional
    api_key: ""              # optional, for Qdrant Cloud
```

| Key | Default | Description |
|-----|---------|-------------|
| `endpoint` | `localhost` | Qdrant host. A scheme (`http://`, `https://`), port or path in this value is ignored |
| `port` | `6334` | gRPC port |
| `use_tls` | `false` | Enables TLS. This is the only setting that turns TLS on |
| `collection` | project path | Collection name. If empty, grepai uses the absolute project path with `/`, `\` and `:` replaced by `_` (`workspace_<name>` for a workspace) |
| `api_key` | empty | API key sent to Qdrant |

**Local Qdrant:**

```yaml
store:
  backend: qdrant
  qdrant:
    endpoint: localhost
    port: 6334
    use_tls: false
```

**Qdrant Cloud:**

```yaml
store:
  backend: qdrant
  qdrant:
    endpoint: your-cluster.cloud.qdrant.io
    port: 6334
    use_tls: true
    api_key: your-api-key
```

When the collection does not exist, grepai creates it with cosine distance and the embedder's dimensions. An existing collection is never resized: after changing to a model with different dimensions, delete the collection (from the dashboard or the Qdrant API) or set a new `collection` name, then run `grepai index` or `grepai watch`.

### Characteristics

- **Dedicated vector database:** search runs inside Qdrant.
- **Shared index:** several machines, worktrees or projects can use the same server, with one collection per project by default.
- **Requires a Qdrant server** (Docker, a binary or Qdrant Cloud).

### Best For

- Teams that share one index
- Multi-project [workspaces](/grepai/workspace/)
- Setups that already run Docker or Qdrant

## Adding a New Store

To add a storage backend:

1. Implement the `VectorStore` interface from `store/store.go`. It covers saving and deleting chunks and documents, similarity search (with an optional path prefix filter), loading and persisting, and index statistics.
2. Optionally implement `EmbeddingCache` (same file) to let the indexer reuse vectors for identical content.
3. Add the backend's options to `StoreConfig` in `config/config.go`.
4. Wire it into the commands that open a store (`cli/`, `mcp/`) and into the prompts in `cli/init.go`.

See [Contributing](/grepai/contributing/) for more details.
