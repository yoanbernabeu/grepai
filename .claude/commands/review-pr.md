# Review PR Command

You are a Pull Request review assistant. Follow these steps in order:

## Step 1: Retrieve PR Information

Use `gh pr view $ARGUMENTS --json number,title,body,state,author,headRefName,baseRefName,additions,deletions,changedFiles,labels` to get PR details.

If no argument is provided, list open PRs with `gh pr list` and ask the user which one to review.

## Step 2: Check CI Status

Use `gh pr checks $ARGUMENTS` to verify all GitHub Actions checks pass.

- If any checks fail, display the details and ask the user if they want to continue anyway
- If all checks pass, continue

## Step 3: Analyze the Code

1. Get the diff with `gh pr diff $ARGUMENTS`
2. Analyze the modified code:
   - Code quality
   - Adherence to project conventions
   - Potential bugs or security issues
   - Tests added/modified if relevant
3. Provide a summary of your analysis with observations

## Step 4: Determine Change Type

Analyze the PR title and modified files to determine the type:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `chore`: Maintenance
- `refactor`: Refactoring
- `test`: Tests only

## Step 5: Request Custom Message

Use the AskUserQuestion tool to ask the user:
- If they want to add a custom comment on the PR before merging
- If yes, what message to add

If the user provides a message, add it as a comment on the PR with `gh pr comment $ARGUMENTS --body "MESSAGE"`.

## Step 6: Merge the PR

1. Ask for user confirmation before merging
2. Merge with `gh pr merge $ARGUMENTS --squash --delete-branch`
3. Update local branch with `git checkout main && git pull origin main`

## Step 7: Create a Release (if applicable)

If the change type is `feat` or `fix` (not `docs`, `chore`, `test`):

1. Get the latest version with `gh release list --limit 1`
2. Parse the current version number (semver format vX.Y.Z)
3. Ask the user with AskUserQuestion:
   - For `feat`: propose a Minor version (vX.Y+1.0) or ask if it's a Major (vX+1.0.0)
   - For `fix`: propose a Patch version (vX.Y.Z+1)
4. Generate release notes based on commits since the last release
5. Create the release with `gh release create vX.Y.Z --generate-notes --title "vX.Y.Z"`

## Important Notes

- Always display a clear summary of each step
- In case of error, clearly explain the issue
- Never merge automatically without explicit user confirmation
- For Major releases, always request explicit confirmation
