---
title: "grepai 1.0: ten months, 46 contributors, and a stable release"
description: "grepai turns 1.0, almost ten months after its first commit. A thank-you to the 46 contributors and everyone who reported bugs, a look at the numbers, and why the feedback made it time for a stable release."
pubDate: 2026-10-25
author: Yoan Bernabeu
tags:
  - release
  - announcement
  - community
---

On January 9, 2026, I pushed the first commit of a small tool to search code by meaning instead of by exact text. Almost ten months later, grepai reaches 1.0.0.

This post is mostly a thank-you. grepai got here because people used it on real codebases, every day, and took the time to tell us when it broke.

```bash
grepai update
# or: brew upgrade grepai
```

[Release notes on GitHub](https://github.com/yoanbernabeu/grepai/releases/tag/v1.0.0) · [Full changelog](https://github.com/yoanbernabeu/grepai/blob/main/CHANGELOG.md)

---

## Ten months in numbers

| | |
|---|---|
| GitHub stars | 1,922 |
| Forks | 160 |
| Contributors with code in grepai | 46 |
| Binary downloads from GitHub releases | 25,000+ |
| Releases | 56 |
| Pull requests opened / merged | 212 / 147 |
| Community pull requests (excluding mine and Dependabot) | 124 from 54 people, 77 merged |
| Issues opened / closed | 115 / 93 |

The download count only covers the release binaries (Homebrew and the install scripts fetch those). Builds from source, `go install` and Nix are not counted.

## Thank you

**To the contributors.** 46 people have code in grepai, me included. Some added a language or a provider, some fixed a bug that only showed up on their machine, some rewrote a whole part of the indexer. In alphabetical order, thank you to:

[@3em0](https://github.com/3em0), [@AkosLukacs](https://github.com/AkosLukacs), [@anyeloamt](https://github.com/anyeloamt), [@ariel-frischer](https://github.com/ariel-frischer), [@cmdaltctr](https://github.com/cmdaltctr), [@Don-Yin](https://github.com/Don-Yin), [@drizzt](https://github.com/drizzt), [@freemanpolys](https://github.com/freemanpolys), [@garitar](https://github.com/garitar), [@greenham13](https://github.com/greenham13), [@Greite](https://github.com/Greite), [@hansipie](https://github.com/hansipie), [@harumiWeb](https://github.com/harumiWeb), [@jedisct1](https://github.com/jedisct1), [@jeremyakers](https://github.com/jeremyakers), [@JohnDoeXX1333](https://github.com/JohnDoeXX1333), [@jugrajsingh](https://github.com/jugrajsingh), [@justinkatzman](https://github.com/justinkatzman), [@kryptt](https://github.com/kryptt), [@liskoX](https://github.com/liskoX), [@Lisito11](https://github.com/Lisito11), [@Logonz](https://github.com/Logonz), [@luuuc](https://github.com/luuuc), [@maarekj](https://github.com/maarekj), [@MaxFreedomPollard](https://github.com/MaxFreedomPollard), [@mholtzscher](https://github.com/mholtzscher), [@mika76](https://github.com/mika76), [@miqcie](https://github.com/miqcie), [@mirandaguillaume](https://github.com/mirandaguillaume), [@p1ng0o](https://github.com/p1ng0o), [@pentiminax](https://github.com/pentiminax), [@pingtimeout](https://github.com/pingtimeout), [@rusel95](https://github.com/rusel95), [@sethbrasile](https://github.com/sethbrasile), [@Slicit](https://github.com/Slicit), [@tadki](https://github.com/tadki), [@TangoMan75](https://github.com/TangoMan75), [@teelicht](https://github.com/teelicht), [@Third-Thing](https://github.com/Third-Thing), [@Thibaultjaigu](https://github.com/Thibaultjaigu), [@tinker495](https://github.com/tinker495), [@tintop2k](https://github.com/tintop2k), [@wasup-yash](https://github.com/wasup-yash), [@WillEhrendreich](https://github.com/WillEhrendreich) and [@wwylon](https://github.com/wwylon).

**To everyone who opened an issue.** 115 issues so far, many of them with logs, configs and exact steps to reproduce. Most of the fixes in this release started as one of those reports. A detailed bug report is a contribution too.

**To everyone who starred, shared, or wrote about grepai**, and to the people who installed it on a Friday afternoon and told us on Monday what went wrong.

## Why 1.0, and why now

For most of these ten months, grepai moved fast: new embedding providers, new storage backends, workspaces, call graph tracing, an MCP server for AI agents. That speed came with breakage, and the feedback told us exactly where.

By this autumn, the reports had changed. Fewer people asked for something new; more told us about the cases where the index slowly drifted away from the code: after a crash, after switching models, after moving a folder, on Windows. That was the signal. We had enough real-world feedback to know which problems stood between grepai and something you can leave running all day and trust.

So before tagging 1.0, I went through every open bug report and fixed the ones where search could return outdated or wrong results without telling you. That is what 1.0 is: not a big new feature, but a commitment. Commands, the config file and the MCP tools stay compatible across 1.x; breaking changes wait for a 2.0.

## What changed in 1.0

**An index you can trust**

- **No stale results after a crash.** The last-index timestamp was saved before the index itself, so after a crash or a `kill`, the next start skipped files whose new content was never saved. It is now written only after the index is saved, failed files are retried, and shutdown waits up to 30 seconds (was 2) for the final save.
- **Changing the embedding model rebuilds the index.** Vectors from two models are not comparable, yet switching models kept the old ones and search silently returned wrong results. grepai now records which embedder built each index and rebuilds it after a change. Existing indexes are adopted as they are.
- **Folders moved or deleted while watching.** On gin, moving `binding/` out of the project used to leave 15 stale results; it now leaves none, and moving it back indexes its 30 files.
- **Windows.** A workspace watch restart could wipe the index because of backslash paths. Paths are now always slash-separated, and deletes and renames are applied on Windows too.
- **Qdrant.** Reads now page through the whole collection, so deleted files no longer linger on large collections. An unreachable Qdrant now gets a clear error with the address and how to fix it.
- **Config files left alone.** Your edits to `.grepai/config.yaml` are no longer overwritten while watching, and an invalid workspace project config is reported instead of being replaced by the defaults.

**Faster re-indexing.** Re-indexing a modified file used to re-embed all of it. Unchanged chunks now reuse their vectors. On gin's `context.go` (27 chunks), a one-line edit now embeds 1 chunk instead of 27, with identical search results.

**Trace.** `grepai trace` now supports Ruby (0 to 1,145 symbols on sinatra) and GDScript for Godot (0 to 471 symbols on the Godot demo projects), and a comment with an apostrophe no longer hides Python calls.

**Agents.** `grepai agent-setup` tells agents to prefer the grepai MCP tools when they are configured, and MCP search in workspaces now handles `path` like the CLI.

**Docs.** The documentation site was rewritten: a clearer landing page, guides checked against the real commands, and a new look.

## Upgrading

```bash
grepai update
```

Your config and index are kept. On the first start, the symbol index is re-extracted once, a Windows single-project index that stored backslash paths is rebuilt, and an index built with a different embedding model than your config is rebuilt. Four settings that never had any effect are no longer written, and `grepai trace --mode` prints a deprecation warning.

## Contributing from now on: issue first

One thing changes with 1.0: grepai now works **issue first**, and no longer accepts pull requests.

This is not a step back from the community; it is the opposite. With AI coding tools, writing a fix is rarely the hard part anymore. The hard part is knowing exactly what is wrong, on which setup, and what the right behavior is, and that is what a good issue carries. Reviewing an outside pull request, checking that it does not break someone else's setup, and reworking it to fit the rest of the code often took longer than writing the fix from a precise report.

So from now on:

- **Found a bug?** Open an issue with your version, OS, config (without keys), the commands you ran and the logs.
- **Want a feature?** Open an issue that starts with the problem you hit and your real use case.
- **Want to help more?** Test fix branches on your projects, point out what is unclear in the docs, add your setup to an existing issue.

The fix is implemented and tested on real projects, and the issue author is credited in the changelog. A pull request opened anyway is closed with a link to open an issue; its description and diff still help write the fix. The [contributing guide](/grepai/contributing/) has the details.

## What's next

1.0 is a starting line, not a finish line. The open issues and pull requests are where the next steps get decided, and the most requested ideas, such as hybrid search and reranking, are already being discussed there. If grepai is part of your workflow, the best way to shape it is to [open an issue](https://github.com/yoanbernabeu/grepai/issues), or to leave a star on [GitHub](https://github.com/yoanbernabeu/grepai) so more people find it.

Thank you for these ten months.
