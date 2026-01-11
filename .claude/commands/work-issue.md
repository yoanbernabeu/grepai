# Work Issue Command

You are an issue implementation assistant. Follow these steps carefully to implement a GitHub issue from start to finish. **Always ask for user confirmation before critical operations.**

## Step 1: Retrieve Issue Information

Use `gh issue view $ARGUMENTS --json number,title,body,state,labels,assignees` to get issue details.

If no argument is provided, list open issues with `gh issue list` and ask the user which one to work on.

Display a clear summary of:

- Issue number and title
- Description/requirements
- Labels and priority

## Step 2: Enter Plan Mode

**CRITICAL: Enter plan mode using the EnterPlanMode tool before any implementation.**

In plan mode:

1. Analyze the issue requirements thoroughly
2. Explore the codebase using grepai search to understand related code
3. Identify files that need modification or creation
4. Design the implementation approach
5. Break down into clear, actionable steps
6. Consider edge cases and potential issues
7. Plan necessary tests

Write your plan and exit plan mode with ExitPlanMode when ready for user approval.

## Step 3: Create Feature Branch

After plan approval:

1. Ensure you're on main and up-to-date:

   ```bash
   git checkout main
   git pull origin main
   ```

2. Create a feature branch following the convention:

   ```bash
   git checkout -b <type>/<issue-number>-<short-description>
   ```

   Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`

3. Confirm branch creation to the user

## Step 4: Implement Changes

Follow your approved plan step by step:

1. Use TodoWrite to track implementation progress
2. Make focused, incremental changes
3. Follow project coding conventions (see CLAUDE.md)
4. Add/update tests as needed
5. Mark todos as completed as you progress

**After each significant change, briefly summarize what was done.**

## Step 5: Test the Implementation Locally

**CRITICAL: You must build the CLI and test it on the actual codebase, not just run unit tests.**

### 5.1 Run Linting and Unit Tests

```bash
make lint      # Check code style
make test      # Run unit tests with race detection
```

### 5.2 Build the CLI Binary

```bash
make build
```

This creates the binary at `./bin/grepai`.

### 5.3 Test the CLI on the Real Codebase

**This step is mandatory.** You must run the actual CLI binary and test your changes in real conditions:

```bash
# Run the new/modified functionality on THIS codebase
./bin/grepai <command-to-test>

# Examples:
# - New CLI command: ./bin/grepai new-command --flag
# - Search fix: ./bin/grepai search "test query"
# - Trace feature: ./bin/grepai trace callers "FunctionName"
# - Config change: ./bin/grepai init && cat .grepai/config.yaml
```

**You MUST:**

- Build the CLI binary with `make build`
- Execute the CLI binary (`./bin/grepai`) to test your changes
- Test the new/modified functionality against this actual codebase
- Verify the output is correct and the feature works as expected
- Test edge cases mentioned in your plan

**If any step fails:**

1. Display the error clearly
2. Fix the issue
3. Rebuild with `make build`
4. Re-test until everything works

**Ask user confirmation:** "Local build successful and CLI tested on the codebase. Ready to commit and push?"

## Step 6: Commit and Push

1. Stage changes:

   ```bash
   git add -A
   ```

2. Create commit following conventional commits:

   ```bash
   git commit -m "<type>(<scope>): <description>

   <body if needed>

   Closes #<issue-number>

   Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
   ```

3. Push the branch:

   ```bash
   git push -u origin <branch-name>
   ```

## Step 7: Create Draft Pull Request

Create a draft PR with:

```bash
gh pr create --draft --title "<type>(<scope>): <description>" --body "$(cat <<'EOF'
## Summary
<Brief description of changes>

## Changes
<Bulleted list of main changes>

## Test Plan
<How the changes were tested - include both unit tests AND real-world testing results>

## Related Issue
Closes #<issue-number>

---
🤖 Generated with [Claude Code](https://claude.ai/code)
EOF
)"
```

Display the PR URL to the user.

## Step 8: Wait for GitHub Actions

1. Monitor CI status:

   ```bash
   gh pr checks <pr-number> --watch
   ```

2. **If checks fail:**

   - Display the failure details
   - Investigate and fix the issue
   - Commit and push the fix
   - Wait for checks again

3. **When all checks pass:**

   - Display success message
   - Show check summary

**Ask user confirmation:** "All GitHub Actions are green. Ready to mark PR as ready for review?"

## Step 9: Mark PR Ready for Review

```bash
gh pr ready <pr-number>
```

**Ask user confirmation:** "PR is ready for review. Do you want to proceed with merging to main?"

## Step 10: Merge to Main

1. Merge the PR:

   ```bash
   gh pr merge <pr-number> --squash --delete-branch
   ```

2. Update local main:

   ```bash
   git checkout main
   git pull origin main
   ```

## Step 11: Update Documentation (if needed)

Evaluate if documentation updates are required:

**CHANGELOG.md:**

- For `feat` or `fix` changes, add entry under appropriate section
- Follow Keep a Changelog format

**README.md:**

- Update if new features affect user-facing functionality
- Update if configuration options changed

**docs/ folder:**

- Update relevant documentation files
- Add new docs for new features

**Ask user:** "Do you want me to update the documentation? (CHANGELOG, README, docs/)"

If yes:

1. Make documentation updates
2. Commit with `docs: update documentation for #<issue-number>`
3. Push directly to main (since these are doc-only changes)

## Step 12: Create Release (if applicable)

Evaluate if a release is warranted:

- `feat`: Usually warrants a release (minor version bump)
- `fix`: May warrant a release (patch version bump)
- `docs`, `chore`, `test`, `refactor`: Usually no release needed

**Ask user:** "This change type is `<type>`. Do you want to create a new release?"

If yes:

1. Get latest version:

   ```bash
   gh release list --limit 1
   ```

2. Determine new version (semver):

   - **Major (vX+1.0.0)**: Breaking changes
   - **Minor (vX.Y+1.0)**: New features (feat)
   - **Patch (vX.Y.Z+1)**: Bug fixes (fix)

3. **Ask user confirmation** with proposed version

4. Create release:

   ```bash
   gh release create v<version> --generate-notes --title "v<version>"
   ```

5. Display release URL

## Important Notes

- **Always display clear progress** at each step
- **Never skip user confirmations** for critical operations (merge, release)
- **If any error occurs**, explain clearly and ask how to proceed
- **Use grepai** for code exploration (as per project guidelines)
- **Follow commit conventions** strictly
- **Keep the user informed** of what you're doing at all times
- **Test on real codebase** - unit tests alone are not sufficient

## Confirmation Checkpoints Summary

The following operations REQUIRE explicit user confirmation:

1. Plan approval (via ExitPlanMode)
2. Ready to commit and push after local tests AND real-world testing pass
3. Mark PR as ready for review (after CI passes)
4. Merge PR to main
5. Update documentation
6. Create a new release
