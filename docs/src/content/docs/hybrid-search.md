---
title: Hybrid Search
description: Merge vector search with keyword matching using Reciprocal Rank Fusion to find exact identifiers in grepai search results.
---

Hybrid search merges two result lists: vector similarity and a simple keyword match on chunk text. It combines them with Reciprocal Rank Fusion (RRF), so chunks that rank well in both lists come first. It helps when queries contain exact identifiers such as function or variable names.

Hybrid search is disabled by default. Enable it in `.grepai/config.yaml`:

```yaml
search:
  hybrid:
    enabled: true
    k: 60   # RRF constant (default: 60)
```

Then search as usual. No re-indexing is needed:

```bash
grepai search "handleAuth token refresh"
```

## How It Works

```text
Query: "handleAuth authentication"
        │
        ├─► Vector Search ──► [chunk_a, chunk_b, chunk_c]
        │                      rank 1    rank 2    rank 3
        │
        └─► Text Search ────► [chunk_b, chunk_d, chunk_a]
                               rank 1    rank 2    rank 3
        │
        ▼
    RRF Fusion: score = Σ 1/(k + rank)
        │
        ▼
    [chunk_b, chunk_a, chunk_c, chunk_d]
     (appears in both lists = higher score)
```

1. **Vector search**: the usual semantic search through the store.
2. **Text search**: keyword matching over every indexed chunk.
3. **RRF fusion**: both rankings are merged into one list.
4. **Boost and dedup**: [structural boost](/grepai/search-boost/) and `search.dedup` are then applied to the fused list, as in a normal search.

Each list holds up to 2x `--limit` candidates (4x when `search.dedup` is enabled). The `--path` filter applies to both lists.

## Configuration

| Key | Default | Description |
|-----|---------|-------------|
| `search.hybrid.enabled` | `false` | Turn hybrid search on |
| `search.hybrid.k` | `60` | RRF constant. A value of `0` or less falls back to 60 |

`grepai search --workspace` ignores this setting: workspace searches always run without hybrid search.

### The `k` parameter

The RRF formula is `score(doc) = Σ 1/(k + rank_i)`, with ranks starting at 1.

- **Higher k** (e.g., 100): rank differences matter less, so appearing in both lists counts more than being first in one.
- **Lower k** (e.g., 30): the top results of each list weigh more.
- **Default (60)**: the value commonly used for RRF.

## When to Enable

Enable hybrid search when:

- Queries often include exact function or class names
- You mix natural language with identifiers (e.g., "handleAuth function")
- Vector-only search misses obvious keyword matches

## Examples

| Query | Vector-only | Hybrid |
|-------|-------------|--------|
| `handleUserLogin` | May miss if the embedding does not capture the identifier | Chunks containing `handleuserlogin` (case-insensitive) rank higher |
| `authentication flow` | Good semantic match | Similar results |
| `validateEmail function in user module` | Partial match | Combines semantic and keyword signals |

## Performance Note

On every query, hybrid search loads all chunks from the store (`GetAllChunks`) and scans their text in memory. On large indexes, or with a remote PostgreSQL or Qdrant store, this adds latency and memory use.

Consider keeping it disabled for:
- Very large monorepos
- Purely semantic queries (no identifiers)
- Latency-sensitive use, such as an agent that runs many searches

## Technical Details

### Text Search

Simple keyword matching, no stemming or ranking model:
- The query is lowercased and split on whitespace. Words shorter than 2 characters are dropped.
- A chunk matches a word if its lowercased content contains that word as a substring.
- Score = number of matched words / number of query words. Chunks with no match are excluded.
- Results are sorted by score.

### RRF Fusion

[Reciprocal Rank Fusion](https://plg.uwaterloo.ca/~gvcormac/cormacksigir09-rrf.pdf) merges ranked lists:

```text
score(doc) = Σ 1/(k + rank_i) for each list containing doc
```

Benefits:
- No need to normalize scores between the two sources
- Robust to outliers

Because the final score is an RRF value, hybrid results show small scores (at most about 0.033 with `k: 60`, before boosting) instead of similarity values.
