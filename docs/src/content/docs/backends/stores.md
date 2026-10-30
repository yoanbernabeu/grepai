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
- **Large project (more than about 50,000 chunks):** use `qdrant`. A `gob` index is loaded and scanned in full on every search, so searches slow down as the index grows; Qdrant keeps a vector index on its server. grepai prints a warning after indexing when a `gob` index passes this size.
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

### Size Limits

Each `grepai search` and each MCP tool call loads the whole `index.gob`, then compares the query with every chunk. Measured with 768-dimension vectors on a recent laptop:

| Chunks | `index.gob` size | Load + search |
|--------|------------------|---------------|
| 10,000 | ~60 MB | ~0.1 s |
| 50,000 | ~310 MB | ~0.5 s |
| 100,000 | ~625 MB | ~0.8 s |

Memory follows the same curve: on the Go source tree (about 55,000 chunks, a 494 MB `index.gob`), each search peaks at about 1.2 GB of RAM. Models with 1536 or 3072 dimensions multiply the size by 2 or 4. Above about 50,000 chunks, grepai prints a warning after indexing and recommends [Qdrant](#qdrant). Run `grepai status` to see the chunk count of your index.

### Best For

- Personal projects and single repositories (up to about 50,000 chunks)
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

### Sharing a Collection

When `collection` is set, several projects can point at the same collection. Each point then carries its repository in a `project_namespace` payload field (also part of the point ID), and every search, listing, stats count and deletion is limited to the current repository:

- **Same repository, shared index.** The namespace is the `origin` remote URL, normalized so that `git@github.com:org/app.git`, `ssh://git@github.com/org/app` and `https://github.com/org/app` match (host lowercased; scheme, user, password or token, port and `.git` dropped, so no credential from the URL is stored). Clones on other machines, CI and linked worktrees of one repository share the same points. As before, checkouts on different branches overwrite each other's version of a file, and indexing one removes the files that only exist in the other.
- **Different repositories, isolated.** Two repositories never see or change each other's points, even with identical file paths. A project in a subdirectory of a repository adds that directory to the namespace (`github.com/org/app#services/api`).
- **Without a git repository, an `origin` remote, or with a local-path `origin`** (`../app.git`, `file://`), the namespace is the project's absolute path, so the same project at another path is a separate project. grepai also uses the path, with a warning, when `git` is not installed or fails (timeout, repository refused as unsafe); processes for which git works then use the repository namespace instead, so make sure every grepai process of a project can run git on it.
- **Changing the `origin` URL** (other host or path), adding a remote to a project indexed without one, or moving a project without a remote changes the namespace. The next `grepai index` or `grepai watch` re-indexes the project from scratch: vectors stored under another namespace are not reused, and the points under the previous namespace stay in the collection until you delete them (Qdrant REST API, filter `{"must": [{"key": "project_namespace", "match": {"value": "<old namespace>"}}]}`).

Collections named after the project path (`collection` empty) and workspace collections hold a single project and have no namespace.

Collections set explicitly and indexed before this change hold points without a namespace. Until the project has points of its own, `grepai search`, `grepai status` and the MCP tools keep reading those points, as earlier versions did (in a collection shared by several projects, that includes the other projects' old points). The first `grepai index` or `grepai watch` after upgrading re-indexes the project: unchanged chunks reuse the stored vectors, and the old points of each re-indexed file are deleted. Old vectors are only reused during the initial scan of `grepai index` or `grepai watch` (not by a running watcher afterwards, and not when the embedding model changed). While that first run is in progress, or if it is interrupted, search and status only see the files migrated so far, until the migration finishes. grepai warns at startup while old points remain (files deleted since, or other projects sharing the collection). Once every project sharing the collection runs the new version and has been re-indexed, the remaining old points are orphans; remove them with the Qdrant REST API (port 6333):

```bash
curl -X POST http://localhost:6333/collections/<collection>/points/delete \
  -H 'Content-Type: application/json' \
  -d '{"filter": {"must": [{"is_empty": {"key": "project_namespace"}}]}}'
```

An earlier grepai release ignores the namespace and sees every project's points: while one project sharing the collection still runs it, that project can overwrite or delete the other projects' points, so upgrade every project that uses the collection.

### Characteristics

- **Dedicated vector database:** search runs inside Qdrant.
- **Shared index:** several machines, worktrees or projects can use the same server, with one collection per project by default.
- **Requires a Qdrant server** (Docker, a binary or Qdrant Cloud).

### Best For

- Teams that share one index
- Multi-project [workspaces](/grepai/workspace/)
- Setups that already run Docker or Qdrant

## Requesting a New Store

grepai works [issue first](/grepai/contributing/) and does not accept pull requests. To ask for another storage backend, open a [feature request](https://github.com/yoanbernabeu/grepai/issues/new?template=feature_request.yml) with the database, how you run it, and why the existing stores do not fit.
