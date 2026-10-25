---
title: Git Worktrees
description: How grepai sets up linked git worktrees automatically by copying the main worktree's config and indexes, and how each backend handles them.
---

## Git Worktree Support

grepai detects [git worktrees](https://git-scm.com/docs/git-worktree). When you work in a linked worktree (created with `git worktree add`), it sets up the worktree from the main worktree's `.grepai/` automatically, so you do not need to run `grepai init` again.

### Quick Start

```bash
# The main worktree is already set up and indexed
cd /path/to/my-project
grepai watch

# Create a linked worktree for a feature branch
git worktree add ../my-project-feature feature-branch

# Use grepai there right away
cd ../my-project-feature
grepai search "authentication flow"    # GOB: uses the copied index
grepai trace callers "HandleRequest"   # uses the copied symbol index
```

With the default GOB backend, search and trace work immediately from the copied indexes. With PostgreSQL or Qdrant, see [Backend Behavior](#backend-behavior).

### How It Works

When a grepai command that looks up the project (such as `search`, `trace`, `refs`, `rpg`, `status` or `watch`) runs inside a linked worktree that has no `.grepai/config.yaml`, grepai:

1. **Detects** the linked worktree with `git rev-parse` (so `git` must be installed).
2. **Checks** that the main worktree has a `.grepai/config.yaml`. If not, nothing happens.
3. **Copies** `index.gob`, `symbols.gob`, then `config.yaml` from the main worktree into the linked worktree's `.grepai/`.
4. **Adds** `.grepai/` to the linked worktree's `.gitignore` (creating the file if needed).

A running `grepai watch` in the main worktree does the same for every linked worktree it discovers (see [Disabling Worktree Discovery](#disabling-worktree-discovery)).

The copy is safe against concurrent runs. grepai holds the linked worktree's `.grepai/writer.lock` for the whole copy (waiting up to 5 seconds for it). Each file is written to a synced temporary file, then moved into place atomically. `config.yaml` is copied last and serves as the completion marker: a `.grepai/` with only `writer.lock` or partial files is not considered initialized, and a watcher never loads a half-copied seed. If any step fails, the files from that attempt are removed and a later run retries.

### What Gets Copied

| File | Purpose | Required |
|------|---------|----------|
| `config.yaml` | Embedder and store configuration | Yes: auto-init stops if it is missing or invalid |
| `index.gob` | Vector index (GOB backend) | No: skipped if absent |
| `symbols.gob` | Symbol index for `trace` and `refs` | No: skipped if absent |

The RPG graph (`rpg.gob`) is not copied. If RPG is enabled, it is built the first time `grepai watch` or `grepai index` indexes the worktree.

### Backend Behavior

Each worktree gets its own `.grepai/` and its own index scope:

| Backend | Behavior |
|---------|----------|
| **GOB** | The copied `index.gob` is the worktree's starting index. Each worktree then keeps an **independent index**: changes in one do not affect the other. |
| **PostgreSQL** | The config is inherited. Data is stored under the worktree's own project ID (its absolute root path), so the worktree starts with **no vectors**: search returns nothing until a watcher has indexed it. Nothing is shared with the main worktree. |
| **Qdrant** | Same as PostgreSQL: the default collection name is derived from the worktree's root path, so the worktree starts empty. |

> **Note:** if the main config sets `store.qdrant.collection` explicitly, linked worktrees inherit that exact name and write into the same collection as the main worktree. Leave the key empty to keep worktrees separate.

The copied `symbols.gob` works with every backend, so `trace` and `refs` answer immediately in all cases.

### Running `grepai watch` in a Worktree

```bash
cd /path/to/my-worktree
grepai watch
```

The watcher starts from the copied state and updates the worktree's index. With GOB, files whose content matches the copied index are skipped. With PostgreSQL or Qdrant, this first run indexes the worktree from scratch.

You often do not need this: a `grepai watch` running in the main worktree already watches linked worktrees, unless discovery is disabled.

> **Note:** the background watcher's PID and log files are named after the repository's worktree ID (`grepai-worktree-<id>.pid` and `.log`), which is shared by all worktrees of a repository. If a background watcher is already running for the repository, `grepai watch` in a linked worktree stops with `watcher is already running in background`.

### Worktree Identification

A repository is identified by a stable **worktree ID**: the first 12 hex characters of the SHA-256 hash of the git common directory. The main worktree and all its linked worktrees share the same ID. grepai uses it to name the background watcher files and shows it in `grepai init` and `grepai watch --status`.

```text
Git worktree detected.
  Main worktree: /path/to/main-repo
  Worktree ID:   0adda787cae4
  Backend:       gob
```

### Explicit Initialization with `--inherit`

To create the worktree's config yourself (for example to edit it before the first index), run `grepai init` in the linked worktree **before** any other grepai command (otherwise auto-init has already created `.grepai/config.yaml`, and `init` reports that grepai is already initialized):

```bash
cd /path/to/my-worktree
grepai init --inherit
```

`grepai init` prints the detection block shown above, then:

- With `--inherit`, it copies the main worktree's configuration without asking.
- Without `--inherit`, it asks `Inherit configuration from main worktree? [Y/n]`.
- With `--yes` but without `--inherit`, it does not inherit and writes a default configuration instead.

`init --inherit` writes `config.yaml` only. It does not copy `index.gob` or `symbols.gob`, so run `grepai watch` (or `grepai index`) afterwards to build the worktree's indexes.

For a fully non-interactive setup:

```bash
grepai init --inherit --yes
```

### Disabling Worktree Discovery

By default, `grepai watch` in the main worktree lists linked worktrees (`git worktree list`), auto-initializes them, and watches them alongside the main project. With many short-lived worktrees (for example AI agent sessions), this multiplies indexing work and memory. To opt out, set this in the **main worktree's** `.grepai/config.yaml`:

```yaml
watch:
  discover_worktrees: false
```

The watcher re-reads this key on every discovery pass (every 3 seconds), so no restart is needed: worktrees already watched are released on the next pass. The key is only read from the main worktree's config; setting it in a linked worktree's copy has no effect. If the config cannot be parsed, discovery skips that pass and logs a warning.

This only affects discovery by the main watcher. Running a grepai command inside a linked worktree still auto-initializes it.

### Troubleshooting

| Problem | Solution |
|---------|----------|
| Auto-init does not trigger | Check that the main worktree has a valid `.grepai/config.yaml` (run `grepai init` there first) and that `git` is installed. For discovery by the main watcher, also check that `watch.discover_worktrees` is not `false`. |
| `no grepai project found (run 'grepai init' first)` in a worktree | The main worktree is not initialized, or the directory is not a git worktree. Run `grepai init` in the main worktree. |
| Search returns nothing (PostgreSQL or Qdrant) | The worktree has its own empty scope. Run `grepai watch` in the worktree, or keep a watcher running in the main worktree with discovery enabled. |
| Search returns stale results (GOB) | Run `grepai watch` in the linked worktree to index its changes. |
| `watcher is already running in background` | A background watcher already runs for this repository. Check it with `grepai watch --status`, or stop it with `grepai watch --stop`. |
| Too many worktrees indexed or high memory | Set `watch.discover_worktrees: false` in the main worktree's config, or prune old worktrees with `git worktree remove` and `git worktree prune`. |
