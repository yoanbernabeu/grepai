---
title: Repository Planning Graph
description: Enable and query grepai's Repository Planning Graph (RPG), a feature-level graph of files, symbols and chunks, from the CLI or MCP.
---

The Repository Planning Graph (RPG) groups your code into a feature hierarchy (areas, categories, subcategories) and links it to files, symbols, vector chunks and call dependencies. Use it when search finds relevant code and you also want to know which feature it belongs to and what sits around it.

## Prerequisites

RPG is disabled by default. Enable it in `.grepai/config.yaml` (the `grepai init --ui` wizard also offers it):

```yaml
rpg:
  enabled: true
```

Then build the index:

```bash
grepai index     # one-time build, then exit
# or
grepai watch     # same initial build, then keeps the graph up to date
```

The graph is stored in `.grepai/rpg.gob`. It is built from the symbol index used by [`grepai trace`](/grepai/trace/) and from the vector chunks, so it covers the languages listed in `trace.enabled_languages`.

Quick check:

```bash
grepai rpg search "command line search flow"
```

If RPG is disabled or the graph is empty, the command fails with `RPG is not enabled or index is empty`. If the graph was written by an incompatible version, it fails with `RPG index is outdated; run 'grepai watch' to rebuild` (`grepai index` rebuilds it too).

## Search RPG Nodes

`grepai rpg search` finds nodes whose feature labels or symbol names share words with your query. Scoring is word overlap (Jaccard), not embeddings, so use words that appear in feature or symbol names.

```bash
grepai rpg search "command line search flow"
grepai rpg search "command line search flow" --kinds symbol,file --scope cli --limit 5
grepai rpg search "command line search flow" --json
```

| Flag | Description | Default |
|------|-------------|---------|
| `--kinds` | Comma-separated node kinds: `area`, `category`, `subcategory`, `file`, `symbol`, `chunk` | `symbol` |
| `--scope` | Keep only nodes whose feature path starts with this prefix, such as `cli` or `rpg/query` | none |
| `--limit`, `-n` | Maximum number of results | `10` |
| `--json` | JSON output | |
| `--toon`, `-t` | Token-efficient TOON output | |

Each result shows the node name, kind, score, node ID, feature path and file location. Pass the node ID to `fetch` or `explore`.

Node IDs follow this format:

| Kind | ID format | Example |
|------|-----------|---------|
| Symbol | `sym:<path>:<name>` or `sym:<path>:<Receiver>.<name>` | `sym:cli/search.go:runSearch` |
| File | `file:<path>` | `file:cli/search.go` |
| Area | `area:<name>` | |
| Category | `cat:<area>/<name>` | |
| Subcategory | `subcat:<parent>/<name>` | |
| Chunk | `chunk:<chunk-id>` | |

## Fetch Node Context

`fetch` shows one node with its hierarchy and edges:

```bash
grepai rpg fetch "sym:cli/search.go:runSearch"
grepai rpg fetch "sym:cli/search.go:runSearch" --json
```

The text output lists the node ID, kind, symbol, feature, feature path, file and line, then the number of parents, children, incoming edges and outgoing edges, followed by a code preview when one is available. Use `--json` or `--toon` to get the full parent, child and edge lists.

## Explore Neighborhoods

`explore` walks the graph breadth-first from a starting node:

```bash
grepai rpg explore "sym:cli/search.go:runSearch"
grepai rpg explore "sym:cli/search.go:runSearch" --direction forward --depth 1 --edge-types invokes --json
```

| Flag | Description | Default |
|------|-------------|---------|
| `--direction` | `forward` (outgoing edges), `reverse` (incoming edges) or `both` | `both` |
| `--depth`, `-d` | Maximum traversal depth | `2` |
| `--edge-types` | Comma-separated edge types to follow (see below) | all |
| `--limit`, `-n` | Maximum number of nodes returned | `100` |
| `--json` | JSON output | |
| `--toon`, `-t` | Token-efficient TOON output | |

Edge types:

| Edge type | Meaning |
|-----------|---------|
| `feature_parent` | Hierarchy node to child node (area, category, subcategory, file) |
| `contains` | File to the symbols it defines |
| `invokes` | Symbol calls symbol |
| `imports` | File imports file or package |
| `maps_to_chunk` | Symbol to the vector chunk that holds its code |
| `semantic_sim` | Symbols with similar features or call patterns |

## Configuration

All keys live under `rpg:` in `.grepai/config.yaml`. Defaults:

| Key | Default | Description |
|-----|---------|-------------|
| `enabled` | `false` | Build and maintain the graph. |
| `feature_mode` | `local` | `local` derives verb-object feature labels (such as `validate token`) from symbol names with heuristics. `llm` and `hybrid` ask an LLM instead (both behave the same today) and fall back to `local` on errors. |
| `llm_provider` | `ollama` | Label for the LLM backend. Requests use the OpenAI-compatible `/chat/completions` API. |
| `llm_endpoint` | `http://localhost:11434/v1` | Base URL of the OpenAI-compatible API. |
| `llm_model` | empty | Model name. If `llm_model` or `llm_endpoint` is empty, grepai uses `local`. |
| `llm_api_key` | empty | API key, if the endpoint needs one. |
| `llm_timeout_ms` | `8000` | Timeout per LLM request. |
| `drift_threshold` | `0.35` | During incremental updates, a file moves to a new place in the hierarchy only when its features change by at least this much (`0.0` to `1.0`). |
| `feature_group_strategy` | `sample` | When building `semantic_sim` edges, how oversized groups of similar symbols are reduced: `sample` (random sample) or `split` (by directory). |

When RPG is enabled, `grepai trace` and `grepai search` results also show the feature path of each symbol or chunk.

## MCP Parity

MCP clients get the same features as these tools:

- `grepai_rpg_search` (`query`, `scope`, `kinds`, `limit`)
- `grepai_rpg_fetch` (`node_id`)
- `grepai_rpg_explore` (`start_node_id`, `direction`, `depth`, `edge_types`, `limit`)

Defaults match the CLI. The CLI uses positional arguments and `--json`/`--toon`; MCP tools take named parameters and a `format` parameter (`json` by default, or `toon`). See [MCP Integration](/grepai/mcp/).
