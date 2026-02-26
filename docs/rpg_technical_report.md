# RPG Encoder: Technical Report & Gap Analysis
> **Date:** 2026-02-17
> **Scope:** Alignment between arXiv:2602.02084 (Paper) and `grepai/rpg` (Implementation)

## 1. Architectural Alignment

The `grepai/rpg` package implements the **Repository Planning Graph (RPG)** as described in the paper, functioning as a unified Intermediate Representation (IR) for agentic reasoning.

### Core Components
| Component | Paper Spec | Implementation Status |
| :--- | :--- | :--- |
| **Graph Model** | $G=(V, E)$ with $V_H, V_L$ | **Aligned.** `NodeKind` clearly distinguishes $V_H$ (Area/Category) and $V_L$ (File/Symbol). |
| **Dual View** | Functional + Dependency | **Aligned.** Edges are typed (`EdgeFeatureParent` vs `EdgeInvokes`/`EdgeImports`). |
| **Evolution** | Incremental Update Protocol | **Aligned.** `Evolver` implements Delete/Modify/Add with orphan pruning and drift detection. |
| **Operation** | Search/Fetch/Explore Tools | **Aligned.** `QueryEngine` exposes exact tool signatures. |

## 2. Implementation Deviations & Heuristics

To ensure performance and reduce token costs, the implementation employs pragmatic heuristics over the paper's "pure LLM" approach.

### 2.1 Hierarchy Construction (Phase 2)
- **Paper:** Uses LLM to induce "Latent Functional Centroids" from the entire repository manifold.
- **Implementation:** Uses **Directory Structure** as a proxy for $V_H$ (Areas).
    - **Reasoning:** Directory structures in well-maintained projects often reflect architectural intent. This avoids the $O(N^2)$ or $O(N)$ context cost of full-repo clustering.
    - **Refinement:** Leaf categories ($V_H$ bottom layer) are clustered by **Verb-Object** atomics extracted from symbol names (e.g., `handle-*`, `new-*`), restoring functional granularity lost by directory-only grouping.

### 2.2 Semantic Lifting (Phase 1)
- **Paper:** LLM-based extraction of "atomic features" for every symbol.
- **Implementation:** **Hybrid Extraction**.
    - **Regex/AST:** Extracts signature, docstring, and symbol name.
    - **Heuristic Features:** Parses `kebab-case` or `camelCase` symbol names to synthesize feature labels (e.g., `HandleRequest` -> `handle-request`).
    - **LLM Summary:** Only applied at the File level (`Summary` field).
    - **Impact:** Drastically reduces indexing time and cost while maintaining sufficient semantic signal for search.

### 2.3 Drift Detection
- **Paper:** Implies sophisticated semantic drift analysis (potentially LLM-based).
- **Implementation:** **Jaccard Similarity** on atomic feature tokens.
    - **Threshold:** Configurable `DriftThreshold` (default 0.3).
    - **Pros:** Fast, deterministic.
    - **Cons:** Misses semantic shifts that don't change terminology (e.g., algorithmic change without renaming).

## 3. Technical Debt & Future Work

1.  **Search Scoring:**
    - Current: Jaccard similarity (Bag-of-Words).
    - Issue: Fails on synonyms (e.g., "fetch" vs "retrieve").
    - Mitigation: Integrate Vector Store embeddings for semantic edge creation and search scoring.

2.  **Hierarchy Flexibility:**
    - Current: Validated mainly on Go/Python structures where directories carry meaning.
    - Risk: Flat repositories (e.g., all files in root) will result in a degenerate hierarchy `root -> general -> [files]`.
    - Fix: Implement an optional "LLM-forced clustering" mode for unstructured repos.

3.  **Dependency Graph:**
    - Current: `trace` package (static analysis).
    - Limitation: Dynamic dispatch, reflection, and complex dependency injection are invisible to the graph.

## 4. Operational Readiness

The system is fully operational for agentic workflows.
- **SearchNode:** Ready for intent-to-code mapping.
- **FetchNode:** ready for context retrieval.
- **ExploreRPG:** Ready for topology-aware navigation.

The `Evolver` ensures the graph remains fresh without full rebuilds, satisfying the "Closed Loop" requirement of the paper.
