---
title: Community Tools
description: Tools and integrations built by the grepai community, and how to add your own to this page.
---

This page lists tools that community members built around grepai. They are third-party projects: the grepai maintainers do not maintain them, so check each repository before you use it.

## Setup Automation

### grepai-beads-helpers

**One command to set up grepai and beads across all your projects.**

[grepai-beads-helpers](https://github.com/miqcie/grepai-beads-helpers) automates the setup of grepai (semantic code search) and beads (AI agent memory) across many projects.

**What it solves:**

Setting up grepai in 20+ projects means repeating the same steps: install Go, configure `PATH`, run init, reload the shell. This tool scripts all of it.

**Installation:**

```bash
# Clone and run the setup script
git clone https://github.com/miqcie/grepai-beads-helpers.git
cd grepai-beads-helpers
./setup.sh

# Index all your projects
./index-all-projects.sh
```

**What it does:**

- Installs Go if needed (macOS and Linux)
- Installs grepai and beads from source
- Configures your shell `PATH`
- Initializes the beads memory system
- Creates the Claude Code integration (optional)
- Indexes all projects in `~/GitHub/` and `~/projects/`

**Use cases:**

- **New machine setup**: one script installs everything.
- **Bulk indexing**: index all projects at once instead of one by one.
- **Team onboarding**: share a script instead of written steps.
- **CI/CD**: automate grepai setup in build environments.
- **Git hooks**: index projects automatically on clone (optional).

**Repository:** [github.com/miqcie/grepai-beads-helpers](https://github.com/miqcie/grepai-beads-helpers)

**Maintained by:** [@miqcie](https://github.com/miqcie)

---

## Contributing Your Tool

Built something useful around grepai? Add it to this page.

**Good candidates:**

- Setup and installation helpers
- IDE and editor integrations
- CI/CD pipelines that use grepai
- Custom embedder or store backends
- Workflow automation scripts
- Analysis and visualization tools

**How to add your tool:**

1. Fork the [grepai repository](https://github.com/yoanbernabeu/grepai).
2. Add your tool to `docs/src/content/docs/community-tools.md`.
3. Include a description, installation steps, use cases and a repository link.
4. Open a pull request that explains what the tool does.

**Guidelines:**

- Start with the problem the tool solves.
- Provide installation commands that work.
- Describe real use cases.
- Link to a documented repository.
- Keep the tool maintained (answer issues, keep it up to date).

Questions? Open an issue or a discussion in the [grepai repository](https://github.com/yoanbernabeu/grepai).
