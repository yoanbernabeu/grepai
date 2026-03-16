# AST-aware chunking via cAST: experiment results

## overview

this PR implements cAST (Zhang et al., EMNLP 2025, arXiv: 2506.15655), an AST-based code chunking strategy that recursively splits oversized AST nodes and greedily merges small siblings to respect a configurable size budget. the algorithm uses non-whitespace character count as its size metric and guarantees verbatim reconstruction of the original file from the chunk sequence.

## setup

| parameter       | value                                          |
| --------------- | ---------------------------------------------- |
| embedding model | `qwen/qwen3-embedding-8b` (via openrouter)     |
| chunk size      | 512 tokens                                     |
| overlap         | 50 tokens                                      |
| hybrid search   | enabled (RRF, k=60)                            |
| index backend   | gob (local)                                    |
| test corpus     | mixed workspace: python, go, markdown, json, html (~189 files) |

## what changed

the `ASTChunker` uses tree-sitter to parse supported files (`.go`, `.py`, `.js`, `.jsx`, `.ts`, `.tsx`) and implements cAST Algorithm 1:

1. if the entire file fits within the non-whitespace budget, emit it as a single chunk
2. otherwise, iterate over root-level AST children, greedily grouping adjacent nodes whose combined non-whitespace characters fit
3. if a single node exceeds the budget, recursively descend into its children
4. after grouping, apply a second greedy merge pass on adjacent ranges
5. fill any byte gaps between ranges to guarantee verbatim reconstruction (concatenating all chunks reproduces the original source exactly)

unsupported file types always fall back to the existing fixed-size sliding-window chunker.

configured via `chunking.strategy` in `config.yaml`:

```yaml
chunking:
  size: 512
  overlap: 50
  strategy: ast   # "fixed" (default) or "ast"
```

## queries

five queries were run against the same corpus under two conditions:

1. **fixed**: grepai with fixed-size character-window chunking (baseline)
2. **ast (cAST)**: grepai with cAST AST-aware chunking (this PR)

| id  | query                                     |
| --- | ----------------------------------------- |
| Q1  | how does brain age prediction work        |
| Q2  | visualization of MRI scan results         |
| Q3  | training loop and loss computation        |
| Q4  | data loading and preprocessing pipeline   |
| Q5  | configuration and hyperparameter settings |

## result: unique files in top-5

higher is better (more diverse results). file-level deduplication was enabled for both conditions.

| query     | fixed  | ast (cAST)    |
| --------- | ------ | ------------- |
| Q1        | 3      | 5             |
| Q2        | 2      | 5             |
| Q3        | 5      | 5             |
| Q4        | 2      | 5             |
| Q5        | 4      | 5             |
| **total** | **16** | **25** (+56%) |

cAST chunking substantially improved file diversity across all five queries.

## result: source code files in top-5

counts how many of the top-5 results point to actual source code (`.py`, `.go`, `.js`, `.ts`, `.sh`) rather than notes, config json, or html.

| query     | fixed | ast (cAST) |
| --------- | ----- | ---------- |
| Q1        | 0     | 0          |
| Q2        | 1     | 1          |
| Q3        | 0     | 0          |
| Q4        | 0     | 0          |
| Q5        | 1     | 1          |
| **total** | **2** | **2**      |

source-code surfacing remained the same: the improvement from cAST is structural (better chunk boundaries and diversity) rather than ranking-level (code vs prose discrimination). this suggests the next step for improving code-file ranking would be a reranking or file-type scoring layer.

## result: notable per-query observations

### Q2 (visualization)

the AST chunker correctly produced a single clean chunk for `bullshit-bench/src/visualize.py` capturing the full module docstring and imports, which ranked #1. the fixed chunker also found this file but the chunk boundaries cut across function definitions.

### Q5 (configuration)

the AST chunker ranked `visual/src/config.py` (a 15-line config module) as #1, because cAST emitted it as a single chunk with a coherent embedding. under fixed chunking, this file's embedding was diluted by overlap with adjacent content, and a different config file ranked #1 instead.

### Q4 (data loading pipeline)

both chunking strategies surfaced markdown notes rather than code for this query. the query phrase appears verbatim in non-code files, causing keyword-level matches to dominate. this is a reranking problem, not a chunking problem.

## conclusion

1. cAST chunking improves file diversity by ~56% (25 vs 16 unique files across five queries) and produces structurally coherent chunks aligned with function and class boundaries.
2. the improvement is especially visible on small files (Q5: `config.py`) where cAST produces a single clean chunk, and on files with many small declarations that cAST merges into semantically coherent groups.
3. the algorithm guarantees verbatim reconstruction: concatenating all chunks exactly reproduces the original source file.
4. source-code ranking (code vs prose discrimination) is not affected by chunking alone and would require a reranking or file-type weighting layer as a follow-up improvement.

## implementation details

| file                          | purpose                                                     |
| ----------------------------- | ----------------------------------------------------------- |
| `indexer/chunker_iface.go`    | defines `FileChunker` interface                             |
| `indexer/chunker_ast.go`      | `ASTChunker` implementation (build tag: `treesitter`)       |
| `indexer/chunker_ast_stub.go` | stub factory for builds without tree-sitter                 |
| `indexer/chunker_ast_test.go` | unit tests (Go, Python, fallback, oversized, reconstruction, merge) |
| `config/config.go`            | adds `Strategy` field to `ChunkingConfig`                   |
| `indexer/indexer.go`          | `Indexer.chunker` changed from `*Chunker` to `FileChunker`  |
| `cli/watch.go`                | uses `NewFileChunker(strategy, size, overlap)`              |

all existing tests pass under both `treesitter` and default build tags.

## references

- Zhang, Zhao, Wang et al. (2025). "cAST: Enhancing Code Retrieval-Augmented Generation with Structural Chunking via Abstract Syntax Tree." EMNLP 2025. arXiv: 2506.15655.
