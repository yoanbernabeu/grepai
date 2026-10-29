---
name: triage
description: "Process grepai GitHub issues and pull requests one at a time under the V1 feature freeze: verify the claim, judge relevance, propose a backward-compatible fix, test before/after on a real project, open a draft PR, review in a loop, and explain the result. Use whenever the user wants to go through, handle or work on issues or PRs (\"des PR ou des issues à traiter ?\", \"traitons les issues\", \"on passe à la #353\", \"regarde la PR de X\"), or names an issue or PR number to work on."
---

# Issue and PR triage for grepai

grepai is 1.x and under a **feature freeze**. This skill is the only workflow for handling issues and external pull requests. Talk to the user in French; everything written on GitHub (comments, commits, PR titles and bodies) is in English.

Whenever you show the user a draft of any text to be posted on GitHub (comment, review, closing message, PR body), show the English version followed by a French translation, labeled as being for the user only. Only the English version is ever posted.

Work on **one issue or PR at a time**. Never start the next one before the current one is closed, merged or explicitly parked by the user.

## Scope rule (feature freeze)

Accepted:
- bug fixes;
- stabilization: crashes, hangs, deadlocks, races, index corruption or data loss, memory leaks;
- security fixes;
- performance fixes only when pathological (OOM, latency blow-up, quadratic behavior on real projects);
- a new **provider** (embedder or vector store) that only implements an existing interface (`embedder.Embedder`, `store.VectorStore`) without touching the core. Weigh its usefulness: real user demand, maintenance cost, whether it duplicates an existing provider (e.g. an OpenAI-compatible endpoint already covers it).

Refused:
- new features, CLI commands, flags, config keys or MCP tools, unless a fix strictly requires one;
- refactors that do not fix a bug (a refactor is allowed only as the minimum needed by a fix).

## Step 0: List

When no number is given, list open PRs and issues (`gh pr list`, `gh issue list --limit 200`), group them (data integrity, crash/hang, wrong results, security, performance, out of scope), and recommend an order: data loss/corruption first, then security, crash/hang, wrong results, performance. Ask which one to start with.

## Step 1: Is it true?

- Read `gh issue view <n>` (or `gh pr view <n>` + `gh pr diff <n>`), including comments.
- Trace the described code path on current `main` (grepai search/trace, then Read). Issues from automated audits often contain false claims: check every one.
- Whenever possible, write a **regression test that reproduces the bug and fails on `main`**.
- Verdict, always with evidence (code excerpt with `file:line`, failing test, command output):
  - **Confirmed**: reproduced;
  - **Plausible**: code path exists but not reproduced (say why);
  - **False**: the claim does not hold;
  - **Already fixed**: give the commit/PR;
  - **Duplicate** of #X.

## Step 2: Is it relevant?

- Reachable in normal use? Who is affected (gob / Postgres / Qdrant, workspace, MCP, watch, a given OS)?
- Severity: data loss or corruption > crash or hang > wrong results > performance > cosmetic.
- Cost and risk of the fix versus the benefit.
- Out of scope per the freeze rule?
- Decision proposed: **fix**, **postpone**, or **close**.

## Step 3: Present and wait for "go"

Present in French, as plain markdown (no AskUserQuestion after a long text): what the issue says, verdict and evidence, relevance, proposed solution with real alternatives and trade-offs. Then ask in plain text. **No code before the user's "go".**

If the decision is to close (false, already fixed, duplicate, out of scope): draft the English comment, show it, and on the user's OK post it and close (`gh issue close <n> --comment ...`, or `gh pr close`). Stay polite and thank the reporter; for out-of-scope requests explain that grepai 1.x is in a stabilization phase and only accepts bug fixes, stability, security and providers.

## Step 4: Backward-compatible solution (V1)

The fix must not affect an existing 1.x user. Check each point:
- a `.grepai/config.yaml` without any new key loads and behaves exactly as before;
- an index built by the previous release (gob, Postgres, Qdrant, symbols, RPG) is read without forced reindex; if a migration is unavoidable, it is transparent and explained;
- no CLI flag, JSON output field, or MCP parameter/result is removed or renamed;
- default results do not change, unless they are precisely the bug;
- if the on-disk format changes, also check the downgrade: what does the previous binary do with the new index?
- minimal diff, no opportunistic refactor, code that reads like the surrounding code.

## Step 5: Implement and test

1. Branch from up-to-date `main`: `fix/<n>-<slug>` (or `feat/` only for a provider).
2. Regression test red before, green after. `make lint` and `make test` pass.
3. **Before/after on a real project**, mandatory:
   - build the previous release binary (`git worktree add` on the latest tag, `make build`) and the new one (`make build`);
   - index the project with the **previous** binary;
   - run the **new** binary on that same index and config without reindexing; compare `search`, `status`, `trace` (and MCP when relevant): same results except the bug;
   - reproduce the bug scenario with both binaries: present before, gone after;
   - pick a project matching the issue (language, size, backend); the grepai repo is fine for Go and generic topics. For Postgres/Qdrant use a local Docker container.
   - record exact commands and observed results for the PR.
4. Clean up: worktrees, containers, background processes, temp indexes.

## Step 6: Draft PR

Commit with conventional commits (`fix(scope): description`, `Closes #<n>`), push, then `gh pr create --draft` with this body:

```
## Problem
## Root cause
## Fix
## Backward compatibility
<each V1 check from step 4 and how it was verified>
## Test plan
<unit/regression tests + real project: name, language, size, backend, commands run before/after, observed results>

Closes #<n>
```

## Step 7: Demanding review, in a loop

- Run `/code-review high` on the branch diff, plus your own adversarial pass: edge cases, concurrency, crash in the middle of the operation, error paths, compatibility, test quality.
- Fix confirmed findings, rerun tests (and the real-project check if behavior changed), push.
- Repeat until no confirmed finding remains and CI is green (`gh pr checks <n> --watch`).

## Step 8: Teaching summary, then ask to merge

Explain in French, for someone who wants to understand, not just approve:
- the problem in one simple sentence (an analogy if it helps);
- why it happened;
- what changed;
- what does **not** change for an existing V1 user;
- how it was proven (tests + before/after on the real project);
- what remains to decide.

Then ask whether to merge. On the user's "go" and with green CI: `gh pr ready <n>`, `gh pr merge <n> --squash --delete-branch`, `git checkout main && git pull`. Never merge without that go. Releases are a separate step (`release` skill), only when the user asks.

## External pull requests

Same steps applied to the PR: scope rule first, then verify the bug it fixes is real (step 1-2), review the diff against step 4, run the before/after test on its branch, then present. If changes are needed, draft an English review comment and post it on the user's OK; otherwise ask to merge as in step 8.
