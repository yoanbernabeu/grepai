---
title: "Introducing Multi-Project Workspaces: Search Across Your Entire Stack"
description: "grepai now supports workspaces for indexing and searching multiple projects together. Perfect for microservices, monorepos, and full-stack development."
pubDate: 2026-01-24
author: Yoan Bernabeu
tags:
  - feature
  - workspace
  - multi-project
---

## TL;DR

grepai now supports **workspaces** - a way to index and search across multiple projects using a shared vector store. Search your frontend, backend, and shared libraries together with a single query.

**Key features:**
- Cross-project semantic search
- Project-scoped filtering
- 100% backward compatible with single-project mode
- Full MCP integration for AI agents

[Read the full documentation](/grepai/workspace/)

---

## The Problem: Scattered Codebases

Modern development rarely involves a single repository. You might have:
- A React frontend
- A Go/Node backend API
- Shared libraries or packages
- Infrastructure code
- Documentation sites

When you're debugging an authentication issue, you need to search across all of these. Traditional tools force you to run separate searches in each project, mentally stitching together the results.

## The Solution: Workspaces

A workspace groups multiple projects under a shared index. One search, all results, ranked by relevance.

```
┌─────────────────────────────────────────────────┐
│                 my-fullstack                     │
│              (PostgreSQL Store)                  │
├─────────────────────────────────────────────────┤
│  frontend/     │  backend/     │  shared-lib/   │
│  (React/TS)    │  (Go API)     │  (Go pkg)      │
└─────────────────────────────────────────────────┘
```

---

## Real-World Example

Let's walk through a realistic scenario: a development team working on multiple products that share common infrastructure.

### The Setup

**Product A** - E-commerce platform:
- `ecommerce-frontend` (Next.js)
- `ecommerce-api` (Go)
- `shared-auth` (Go library)

**Product B** - Admin dashboard:
- `admin-frontend` (React)
- `admin-api` (Go)
- `shared-auth` (same library)

**Legacy project** - Old internal tool (still using GOB backend, not migrated)

### Creating Two Workspaces

```bash
# Workspace for Product A
grepai workspace create product-a
# Select: PostgreSQL, Ollama with nomic-embed-text

grepai workspace add product-a ~/projects/ecommerce-frontend
grepai workspace add product-a ~/projects/ecommerce-api
grepai workspace add product-a ~/projects/shared-auth

# Workspace for Product B
grepai workspace create product-b
# Select: PostgreSQL (same or different DSN), Ollama

grepai workspace add product-b ~/projects/admin-frontend
grepai workspace add product-b ~/projects/admin-api
grepai workspace add product-b ~/projects/shared-auth
```

Notice that `shared-auth` appears in both workspaces. This is fine - each workspace maintains its own index with unique prefixes:
- `product-a/shared-auth/auth.go`
- `product-b/shared-auth/auth.go`

### The Legacy Project (GOB)

The old internal tool doesn't need to migrate. It continues working exactly as before:

```bash
cd ~/projects/legacy-tool
grepai watch --background
grepai search "user permissions"
```

GOB backend stores everything locally in `.grepai/index.gob`. No changes needed.

### Searching Across Products

Now the magic happens:

```bash
# Search for authentication logic in Product A
grepai search --workspace product-a "JWT token validation"

# Results from all three projects, ranked by relevance:
# - product-a/shared-auth/jwt.go (score: 0.89)
# - product-a/ecommerce-api/middleware/auth.go (score: 0.76)
# - product-a/ecommerce-frontend/hooks/useAuth.ts (score: 0.71)
```

Need to focus on the backend only?

```bash
grepai search --workspace product-a --project ecommerce-api "rate limiting"
```

### Running the Watchers

For active development, start workspace watchers:

```bash
# Product A - background daemon
grepai watch --workspace product-a --background

# Check status
grepai watch --workspace product-a --status

# Product B - foreground for debugging
grepai watch --workspace product-b

# Legacy tool - unchanged
cd ~/projects/legacy-tool && grepai watch --background
```

---

## Migration Path

### From Single-Project GOB (Default)

**No migration needed.** GOB projects continue working. You can optionally create a workspace later if you want cross-project search.

### From Single-Project PostgreSQL

If your projects already use PostgreSQL individually:

1. **Existing data stays intact** - old chunks use paths like `src/App.tsx`
2. **Workspace creates new entries** - with full prefix `workspace/project/src/App.tsx`
3. **Both coexist** - but you may see duplicates in searches

**Recommended clean migration:**

```bash
# 1. Create workspace with same PostgreSQL DSN
grepai workspace create my-stack
grepai workspace add my-stack ~/projects/frontend
grepai workspace add my-stack ~/projects/backend

# 2. Clean old non-workspace chunks (optional)
psql -d grepai -c "DELETE FROM chunks WHERE file_path NOT LIKE '%/%/%';"

# 3. Re-index via workspace
grepai watch --workspace my-stack
```

---

## Essential Commands

### Workspace Management

```bash
# Create a new workspace (interactive)
grepai workspace create <name>

# List all workspaces
grepai workspace list

# Show workspace details
grepai workspace show <name>

# Add/remove projects
grepai workspace add <name> /path/to/project
grepai workspace remove <name> project-name

# Check project paths status
grepai workspace status <name>

# Delete a workspace
grepai workspace delete <name>
```

### Indexing

```bash
# Start workspace watcher (foreground)
grepai watch --workspace <name>

# Background daemon
grepai watch --workspace <name> --background

# Check daemon status
grepai watch --workspace <name> --status

# Stop daemon
grepai watch --workspace <name> --stop
```

### Searching

```bash
# Search all projects in workspace
grepai search --workspace <name> "query"

# Filter to specific projects
grepai search --workspace <name> --project frontend "query"
grepai search --workspace <name> --project frontend --project backend "query"

# JSON output for scripts/AI agents
grepai search --workspace <name> "query" --json --compact
```

---

## MCP Integration

AI agents like Claude Code can use workspace search via MCP:

```json
{
  "name": "grepai_search",
  "arguments": {
    "query": "authentication middleware",
    "workspace": "product-a",
    "projects": "ecommerce-api,shared-auth",
    "limit": "10"
  }
}
```

This enables AI-assisted development across your entire stack with a single tool call.

---

## Configuration Priority

When using workspaces, settings come from different sources:

| Setting | Source | Why |
|---------|--------|-----|
| Store (backend, DSN) | **Workspace** | Shared store required for cross-project search |
| Embedder (model) | **Workspace** | All vectors must be compatible |
| Chunking (size, overlap) | **Project** | Can vary per language/project |
| Ignore patterns | **Project** | Project-specific exclusions |

This means you can have different chunking strategies per project while maintaining a unified search index.

---

## Requirements

Workspaces require a **shared vector store backend**:
- **PostgreSQL** with pgvector (recommended)
- **Qdrant**

**GOB is not supported for workspaces** because it's file-based and cannot be shared across projects. However, GOB projects continue working perfectly in single-project mode.

---

## What's Next?

- Try creating a workspace with your existing projects
- [Read the full documentation](/grepai/workspace/)
- Join the discussion on [GitHub](https://github.com/yoanbernabeu/grepai)

Happy searching across your stack!
