---
description: Commit, tag, and push a release (pass major, minor, or patch)
---

I need to create a new release for this project. The release pipeline is triggered by pushing a git tag matching `v*`.

First, check the current state:
!`git log --oneline -5`
!`git tag --sort=-v:refname | head -5`
!`git status --short`
!`git diff --stat`

The user wants a **$ARGUMENTS** release (should be one of: major, minor, or patch).

Follow these steps:

1. Determine the latest git tag (e.g., v0.3.4)
2. Parse the semantic version and bump the appropriate component:
   - `patch`: v0.3.4 → v0.3.5
   - `minor`: v0.3.4 → v0.4.0
   - `major`: v0.3.4 → v1.0.0
3. If there are uncommitted changes (staged, unstaged, or untracked files):
   a. Review all changes with `git diff` and `git diff --cached` and any untracked files
   b. Stage ALL changes: `git add -A`
   c. Write a clear, descriptive commit message summarizing everything being released
   d. Commit the changes
   e. Verify the commit succeeded
4. Check if the current branch is `main`. If not, warn the user but proceed if they confirm.
5. Push the commit to origin: `git push origin main`
6. Show the user what the new tag will be and what commits are included since the last tag using `git log <last-tag>..HEAD --oneline`
7. Create the git tag: `git tag -a vX.Y.Z -m "vX.Y.Z"`
8. Push the tag: `git push origin vX.Y.Z`
9. Confirm the release pipeline has been triggered and provide the GitHub Actions URL: https://github.com/rmkohlman/MaestroVault/actions

IMPORTANT: This command handles the FULL release flow — committing, pushing, tagging, everything. The user expects a single command to go from dirty working tree to released version.
