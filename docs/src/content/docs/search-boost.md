---
title: Search Boost
description: Rank source code above tests, mocks and docs by multiplying search scores with path-based factors.
---

Structural boosting multiplies each search score by factors based on the file path. By default it pushes tests, mocks, generated code and docs down, and gives a small bonus to paths containing `/src/`, `/lib/` or `/app/`.

Boost is enabled by default and configured under `search.boost` in `.grepai/config.yaml`. To demote a directory, append a rule to the existing list:

```yaml
search:
  boost:
    enabled: true
    penalties:
      # ...existing default rules...
      - pattern: "/examples/"
        factor: 0.5
```

Changes apply to the next `grepai search`. No re-indexing is needed.

## How It Works

After the similarity search (and hybrid fusion, if enabled), grepai multiplies each result's score by every rule that matches its path, then re-sorts the results:

- **Penalties** (`factor < 1.0`) lower the score.
- **Bonuses** (`factor > 1.0`) raise the score.
- **Cumulative**: when several patterns match, their factors are multiplied together.

Example: `pkg/user/handler_test.go` matches `_test.` (×0.5). `services/api/tests/auth_test.py` matches `/tests/` (×0.5) and `_test.` (×0.5), so its final factor is 0.25.

Boost only reorders the candidates the store returned (2x `--limit`, or 4x with `search.dedup` enabled). It does not bring in files that were not among them.

## Pattern Matching

Patterns are plain, case-sensitive substring matches on the file path. No globs or regular expressions.

The path is relative to the project root, with no leading slash (for example `src/auth/handler.go`). This matters for patterns that start with `/`:

- `/tests/` matches `services/api/tests/auth.py`, but **not** a top-level `tests/auth.py`.
- `/src/` matches `packages/web/src/app.ts`, but **not** a top-level `src/app.ts`.
- `_test.` matches `auth_test.go` and `user_test.py`.
- `.spec.` matches `auth.spec.ts` and `user.spec.js`.

The leading `/` avoids false positives (`/tests/` does not match `contests/`). To also match a top-level directory, add a second rule without the leading slash, keeping in mind it will match anywhere in the path:

```yaml
search:
  boost:
    penalties:
      - pattern: "/tests/"
        factor: 0.5
      - pattern: "tests/"   # also matches top-level tests/ (and contests/)
        factor: 0.5
```

In workspace searches, stored paths start with `<workspace>/<project>/`, so a pattern like `/src/` also matches each project's top-level `src/`.

## Default Configuration

`grepai init` writes these rules to `.grepai/config.yaml`:

```yaml
search:
  boost:
    enabled: true
    penalties:
      # Test files (multi-language)
      - pattern: "/tests/"
        factor: 0.5
      - pattern: "/test/"
        factor: 0.5
      - pattern: "__tests__"
        factor: 0.5
      - pattern: "_test."
        factor: 0.5
      - pattern: ".test."
        factor: 0.5
      - pattern: ".spec."
        factor: 0.5
      - pattern: "test_"
        factor: 0.5
      # Mocks
      - pattern: "/mocks/"
        factor: 0.4
      - pattern: "/mock/"
        factor: 0.4
      - pattern: ".mock."
        factor: 0.4
      # Fixtures & test data
      - pattern: "/fixtures/"
        factor: 0.4
      - pattern: "/testdata/"
        factor: 0.4
      # Generated code
      - pattern: "/generated/"
        factor: 0.4
      - pattern: ".generated."
        factor: 0.4
      - pattern: ".gen."
        factor: 0.4
      # Documentation
      - pattern: ".md"
        factor: 0.6
      - pattern: "/docs/"
        factor: 0.6
    bonuses:
      # Entry points (multi-language)
      - pattern: "/src/"
        factor: 1.1
      - pattern: "/lib/"
        factor: 1.1
      - pattern: "/app/"
        factor: 1.1
```

> **Note:** grepai reads boost rules from the config file only. If the `search.boost` block is missing, boost is off. Copy the block above to restore the defaults.

## Default Rules Summary

| Category | Patterns | Factor |
|----------|----------|--------|
| Tests | `/tests/`, `/test/`, `__tests__`, `_test.`, `.test.`, `.spec.`, `test_` | ×0.5 |
| Mocks | `/mocks/`, `/mock/`, `.mock.` | ×0.4 |
| Fixtures | `/fixtures/`, `/testdata/` | ×0.4 |
| Generated | `/generated/`, `.generated.`, `.gen.` | ×0.4 |
| Docs | `.md`, `/docs/` | ×0.6 |
| Source | `/src/`, `/lib/`, `/app/` | ×1.1 |

Because matching is by substring, some defaults are broad: `.md` also matches `.mdx`, and `test_` also matches names like `contest_results.go`.

## Customization

### Add custom patterns

Your `penalties` and `bonuses` lists replace the defaults, so keep the default rules you still want and append yours:

```yaml
search:
  boost:
    enabled: true
    penalties:
      # ...default rules you want to keep...
      - pattern: "/examples/"
        factor: 0.5
      - pattern: "/third_party/"
        factor: 0.3
    bonuses:
      - pattern: "/core/"
        factor: 1.2
```

### Disable boosting

```yaml
search:
  boost:
    enabled: false
```

### Workspace searches

`grepai search --workspace` (and the MCP workspace search) always applies the built-in default rules. It ignores the `search.boost` block of each project.
