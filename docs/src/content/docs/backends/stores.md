---
title: Vector Stores
description: Configure storage backends for grepai
---

Vector stores persist the embeddings and enable similarity search.

## Available Stores

| Backend | Type | Pros | Cons |
|---------|------|------|------|
| GOB | File-based | Simple, no setup | Single machine only |
| PostgreSQL | Database | Scalable, team-friendly | Requires PostgreSQL + pgvector |
| Qdrant | Vector DB | Scalable, purpose-built for vectors | Requires Docker or Qdrant Cloud |

## GOB (File-based)

The default storage backend. Stores vectors in a binary file.

### Configuration

```yaml
store:
  backend: gob
  gob:
    path: .grepai/index.gob
```

### Characteristics

- **Pros**:
  - Zero dependencies
  - Fast for small/medium codebases
  - Portable (just copy the file)

- **Cons**:
  - Single machine only
  - Loads entire index into memory
  - No concurrent access

### Best For

- Personal projects
- Quick experimentation
- CI/CD pipelines (ephemeral index)

## PostgreSQL with pgvector

Scalable vector storage using PostgreSQL and the pgvector extension.

### Setup

1. Install PostgreSQL with pgvector:

```bash
# macOS with Homebrew
brew install postgresql pgvector

# Ubuntu/Debian
sudo apt install postgresql postgresql-contrib
# Then install pgvector from source or package
```

2. Create the database and extension:

```sql
CREATE DATABASE grepai;
\c grepai
CREATE EXTENSION vector;
```

3. Create the table (auto-created by grepai, but for reference):

```sql
CREATE TABLE IF NOT EXISTS chunks (
    id SERIAL PRIMARY KEY,
    file_path TEXT NOT NULL,
    start_line INT NOT NULL,
    end_line INT NOT NULL,
    content TEXT NOT NULL,
    embedding vector(768),  -- Adjust dimensions based on embedder
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX ON chunks USING ivfflat (embedding vector_cosine_ops);
```

### Configuration

```yaml
store:
  backend: postgres
  postgres:
    connection_string: postgres://user:password@localhost:5432/grepai
```

### Environment Variable

```bash
export GREPAI_POSTGRES_URL="postgres://user:password@localhost:5432/grepai"
```

```yaml
store:
  backend: postgres
  postgres:
    connection_string: ${GREPAI_POSTGRES_URL}
```

### Characteristics

- **Pros**:
  - Scales to large codebases
  - Concurrent access (team use)
  - Advanced filtering with SQL
  - Persistent and reliable

- **Cons**:
  - Requires PostgreSQL setup
  - More complex deployment

### Best For

- Team environments
- Large codebases (100k+ lines)
- Production deployments
- When you need SQL queries on metadata

## Qdrant

Run Qdrant locally from compose.yml:
```bash
docker compose --profile=qdrant up
```

Initialize with Qdrant:
```bash
grepai init --backend qdrant
```

Configuration example:
```yaml
store:
  backend: qdrant
  qdrant:
    endpoint: "localhost"  # or "localhost" (scheme auto-detected from use_tls)
    port: 6334                 # gRPC port (default: 6334)
    use_tls: false               # Enable TLS (required for Qdrant Cloud)
    collection: "myproject"       # optional
    api_key: ""                 # optional (for Qdrant Cloud)
```

**Local Qdrant:**
```yaml
store:
  backend: qdrant
  qdrant:
    endpoint: "localhost"
    port: 6334
    use_tls: false
```

**Qdrant Cloud:**
```yaml
store:
  backend: qdrant
  qdrant:
    endpoint: "your-cluster.qdrant.io"
    port: 443
    use_tls: true
    api_key: "your-api-key"
```

Note: Collection names are automatically sanitized from the project path (replaces `/` with `_`). If no collection is specified, the sanitized project path is used.

### Characteristics

- **Pros**:
  - Purpose-built for vector search
  - High performance with HNSW indexing
  - Easy Docker setup
  - Built-in dashboard (http://localhost:6333/dashboard)
  - Supports filtering and metadata

- **Cons**:
  - Requires Docker or Qdrant Cloud
  - Additional service to manage

### Best For

- Projects needing high-performance vector search
- Docker-based development environments
- Teams already using Qdrant
- When you want a dedicated vector database

## Adding a New Store

To add a new storage backend:

1. Implement the `VectorStore` interface in `store/`:

```go
type VectorStore interface {
    Store(ctx context.Context, chunks []Chunk) error
    Search(ctx context.Context, embedding []float32, limit int) ([]Result, error)
    Delete(ctx context.Context, filePath string) error
    Close() error
}
```

2. Add configuration in `config/config.go`

3. Wire it up in the CLI commands

See [Contributing](/grepai/contributing/) for more details.
