---
title: Git Worktrees
description: Use grepai seamlessly across git worktrees
---

## Git Worktree Support

grepai automatically detects [git worktrees](https://git-scm.com/docs/git-worktree) and provides zero-config setup for linked worktrees. If you work on multiple branches in parallel using `git worktree add`, grepai handles everything transparently.

### How It Works

When you run any grepai command (`search`, `trace`, `watch`) from a **linked worktree**, grepai:

1. **Detects** that you're in a linked worktree (not the main repository)
2. **Locates** the main worktree's `.grepai/` directory
3. **Auto-initializes** a local `.grepai/` by copying `config.yaml`, `index.gob`, and `symbols.gob` from the main worktree
4. **Adds** `.grepai/` to the worktree's `.gitignore`

This means `search` and `trace` work immediately in any worktree, without re-indexing.

### Quick Start

```bash
# Your main project already has grepai set up
cd /path/to/my-project
grepai watch  # index is built here

# Create a linked worktree for a feature branch
git worktree add ../my-project-feature feature-branch

# grepai works immediately in the linked worktree
cd ../my-project-feature
grepai search "authentication flow"   # uses copied index
grepai trace callers "HandleRequest"   # uses copied symbols
```

No `grepai init` or `grepai watch` required in the linked worktree — it just works.

### Explicit Initialization with `--inherit`

If you prefer to explicitly initialize a worktree (or want to customize the config afterwards), use the `--inherit` flag:

```bash
cd /path/to/my-worktree
grepai init --inherit
```

This will:
- Detect the main worktree automatically
- Copy its configuration
- Display backend information
- Let you proceed to `grepai watch` for incremental updates

You can also combine it with `--yes` for fully non-interactive setup:

```bash
grepai init --inherit --yes
```

### Backend Behavior

Each worktree gets its own `.grepai/` directory for isolation. The behavior differs depending on your storage backend:

| Backend | Behavior |
|---------|----------|
| **GOB** | Index is copied as a seed. Each worktree maintains an **independent index**. Changes in one worktree don't affect the other. |
| **PostgreSQL / Qdrant** | Config is inherited. Each worktree scopes its data within the **shared store**. Embeddings can be reused across worktrees. |

For teams using multiple worktrees, PostgreSQL or Qdrant backends are recommended for shared indexing.

### Worktree Identification

Each repository is identified by a stable **Worktree ID**: a 12-character hex string derived from the SHA-256 hash of the git common directory. This ID is the same for the main worktree and all linked worktrees of the same repository.

```bash
grepai init --inherit
# Output:
# Git worktree detected.
#   Main worktree: /path/to/main-repo
#   Worktree ID:   0adda787cae4
#   Backend:       gob
```

### Running `grepai watch` in a Worktree

After auto-init, you can optionally run `grepai watch` in the linked worktree to get **incremental updates** for worktree-specific changes:

```bash
cd /path/to/my-worktree
grepai watch
```

This re-indexes only the files that differ from the copied seed index, keeping your worktree's index up to date.

### What Gets Copied

| File | Purpose | Required |
|------|---------|----------|
| `config.yaml` | Embedder/store configuration | Yes |
| `index.gob` | Vector index (search seed) | No (optional) |
| `symbols.gob` | Symbol index (trace seed) | No (optional) |

If `config.yaml` is missing from the main worktree, auto-init will not proceed.

### Troubleshooting

| Problem | Solution |
|---------|----------|
| Auto-init doesn't trigger | Verify the main worktree has `.grepai/config.yaml`. Run `grepai init` in the main worktree first. |
| Search returns stale results | Run `grepai watch` in the linked worktree to update the index with worktree-specific changes. |
| "not a git repository" error | Ensure `git` is installed and the directory is a valid git worktree. |
| Want shared indexing | Switch to `postgres` or `qdrant` backend for cross-worktree index sharing. |
