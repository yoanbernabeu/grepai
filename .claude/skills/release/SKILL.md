---
name: release
description: "Create a new release for grepai. Checks CI, determines version type, updates CHANGELOG and documentation, credits contributors, and creates GitHub release."
---

# Release Process for grepai

This skill guides you through creating a new release. **All output must be in English.**

## When to Use This Skill

Invoke this skill when:
- User asks to "create a release"
- User asks to "publish a new version"
- User asks to "bump the version"
- User mentions "release" in the context of versioning

## Release Workflow

### Step 1: Verify CI on Main

Check that all CI checks pass on main:

```bash
gh run list --branch main --limit 1 --json status,conclusion,name
```

**STOP if CI is not green.** Ask the user to fix issues first.

### Step 2: Get Current Version and Changes

Get the latest release:

```bash
gh release list --limit 1
```

List merged PRs since last release:

```bash
gh pr list --state merged --base main --json number,title,author,mergedAt,labels --limit 50
```

Display a summary of changes categorized by type (feat, fix, docs, etc.).

### Step 3: Determine Version Type

| Change Type | Version Bump |
|-------------|--------------|
| Breaking changes | **Major** (vX+1.0.0) |
| New features (`feat`) | **Minor** (vX.Y+1.0) |
| Bug fixes (`fix`) | **Patch** (vX.Y.Z+1) |
| Docs/chore only | Usually no release needed |

**Ask user confirmation** before proceeding.

### Step 4: Update Version Files

#### 4.1 CHANGELOG.md

Add new section after `## [Unreleased]`:

```markdown
## [X.Y.Z] - YYYY-MM-DD

### Added
- **Feature Name**: Description (#PR) - @author

### Fixed
- **Fix Name**: Description (#PR) - @author
```

Update comparison links at bottom:

```markdown
[Unreleased]: https://github.com/yoanbernabeu/grepai/compare/vX.Y.Z...HEAD
[X.Y.Z]: https://github.com/yoanbernabeu/grepai/compare/vPREVIOUS...vX.Y.Z
```

#### 4.2 Documentation Hero

Update version in `docs/src/components/homepage/HeroAnimated.astro` line 6:

```astro
const { version = "X.Y.Z" } = Astro.props;
```

#### 4.3 Nix Flake

Update `flake.nix`:

1. Update `version` (line 13):

```nix
version = "X.Y.Z";
```

2. Update `vendorHash`:
   - Set `vendorHash = "";` temporarily in `flake.nix`
   - Run `make nix-hash` to compute the correct hash (requires Docker)
   - Replace with the new hash: `vendorHash = "sha256-XXXXX";`
   - If Docker is not available, ask the user to provide the hash or skip this step

### Step 5: Commit and Push

```bash
git add CHANGELOG.md docs/src/components/homepage/HeroAnimated.astro flake.nix
git commit -m "chore(release): bump version to X.Y.Z

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
git push origin main
```

### Step 6: Wait for CI

```bash
gh run list --branch main --limit 1 --json status,conclusion --watch
```

### Step 7: Create GitHub Release

```bash
gh release create vX.Y.Z --generate-notes --title "vX.Y.Z"
```

### Step 8: Verify

```bash
gh release view vX.Y.Z
```

## Contributor Credits

**IMPORTANT:** Always credit PR authors in CHANGELOG:

```markdown
- **Pascal/Delphi Support**: Add `.pas` and `.dpr` file support (#71) - @yoanbernabeu
```

GitHub `--generate-notes` automatically credits contributors in release notes.

## Checklist

- [ ] CI green on main before starting
- [ ] Version type matches changes (major/minor/patch)
- [ ] CHANGELOG.md updated with all PRs
- [ ] All contributors credited with @username
- [ ] Hero version updated in docs
- [ ] Nix flake version and vendorHash updated in flake.nix
- [ ] CI passed after version commit
- [ ] GitHub release created

## Keywords

release, version, bump, publish, changelog, semver, semantic versioning
