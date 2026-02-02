---
title: Community Tools
description: Community-built tools and integrations for grepai
---

The grepai community has built tools that extend and enhance grepai's capabilities. This page showcases automation scripts, integrations, and helpers that make working with grepai easier.

## Setup Automation

### grepai-beads-helpers

**One-command setup for grepai + beads across all your projects.**

[grepai-beads-helpers](https://github.com/miqcie/grepai-beads-helpers) automates the tedious setup process for grepai (semantic code search) and beads (AI agent memory) across multiple projects.

**What it solves:**

Setting up grepai across 20+ projects means repeating the same steps: install Go, configure PATH, run init, reload shell. This automates all of it.

**Installation:**

```bash
# Clone and run setup script
git clone https://github.com/miqcie/grepai-beads-helpers.git
cd grepai-beads-helpers
./setup.sh

# Bulk index all your projects
./index-all-projects.sh
```

**What it does:**

- ✅ Installs Go if needed (macOS/Linux)
- ✅ Installs grepai and beads from source
- ✅ Configures shell PATH automatically
- ✅ Initializes beads memory system
- ✅ Creates Claude Code integration (optional)
- ✅ Bulk-indexes all projects in `~/GitHub/` and `~/projects/`

**Use cases:**

- **New machine setup** - One script installs everything
- **Bulk indexing** - Index all projects at once, not one-by-one
- **Team onboarding** - Share setup script instead of documentation
- **CI/CD** - Automate grepai setup in build environments
- **Git hooks** - Auto-index projects on clone (optional)

**Repository:** [github.com/miqcie/grepai-beads-helpers](https://github.com/miqcie/grepai-beads-helpers)

**Maintained by:** [@miqcie](https://github.com/miqcie)

---

## Contributing Your Tool

Have you built something useful for the grepai community? We'd love to showcase it!

**Good candidates for this page:**

- Setup automation and installation helpers
- IDE/editor integrations
- CI/CD pipelines using grepai
- Custom embedder or store backends
- Workflow automation scripts
- Analysis and visualization tools

**How to add your tool:**

1. Fork the [grepai repository](https://github.com/yoanbernabeu/grepai)
2. Add your tool to this page (`docs/src/content/docs/community-tools.md`)
3. Include: description, installation, use cases, repository link
4. Open a pull request with clear description

**Guidelines:**

- Keep descriptions problem-focused (what pain does it solve?)
- Provide working installation commands
- Include realistic use cases
- Link to well-documented repositories
- Maintain your tool (respond to issues, keep updated)

Questions? Open an issue or discussion in the [grepai repository](https://github.com/yoanbernabeu/grepai).
