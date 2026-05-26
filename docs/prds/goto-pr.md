# PRD: `g p` — Go to PR

## Problem Statement

When working across multiple worktrees, commits, or files, developers frequently need to jump to the
GitHub pull request associated with their current context. There is no keyboard shortcut to do this
— users must leave gx, navigate to GitHub manually, and find the right PR. This friction compounds
across the four main views (worktrees, log, commit, status), each of which has enough context to
open the right PR directly.

## Solution

Add a `g p` chord binding ("go to PR") to all four views. Pressing `g p` resolves the PR URL for
the current context using the `gh` CLI and opens it in the system browser. In the log and commit
views, the binding uses a two-path strategy: merged commits (in-master) trigger a commit-hash PR
search; unmerged commits trigger a branch-level PR lookup. If no PR is found, a warning
notification is shown.

## User Stories

1. As a developer in the worktrees view, I want to press `g p` to open the GitHub PR for the
   selected worktree's branch, so that I can quickly navigate to the PR without leaving gx.
2. As a developer in the status view, I want to press `g p` to open the GitHub PR for the current
   worktree's branch, so that I can check PR status while reviewing staged changes.
3. As a developer in the log view with my cursor on an unmerged commit, I want to press `g p` to
   open the PR for the current branch, so that I can jump directly to the relevant pull request.
4. As a developer in the log view with my cursor on a merged (in-master) commit, I want to press
   `g p` to find and open the PR that originally contained that commit, so that I can review the
   context of a historical change.
5. As a developer in the commit view on an unmerged commit, I want to press `g p` to open the
   branch-level PR, so that I can see the review thread for work in progress.
6. As a developer in the commit view on a merged commit, I want to press `g p` to find and open
   the historical PR for that commit hash, so that I can trace the origin of a merged change.
7. As a developer with no open PR for the current branch, I want to see a warning when I press
   `g p`, so that I know there's no PR and am not left wondering why nothing happened.
8. As a developer, I want `g p` to be discoverable in the help overlay of every view, so that I
   don't have to remember it from documentation.

## Implementation Decisions

### New PR lookup module (`git/pr.go`)

A new file in the `git` package exposes three functions:

- **`BranchPRURL(worktreeRoot string) (string, error)`** — runs
  `gh pr view --json url -q .url` in `worktreeRoot`. Returns the URL of the open PR for the
  worktree's current branch, or an error if none is found.

- **`CommitPRURL(worktreeRoot, hash string) (string, error)`** — runs
  `gh pr list --search <hash> --state all --json url -q '.[0].url'` in `worktreeRoot`. Returns the
  URL of the first PR matching the commit hash, or `""` if the output is empty (no match). An empty
  result is not treated as an error by the caller — the view layer converts it to
  `notify.Warning("no PR found")`.

- **`IsCommitMergedToMain(worktreeRoot, hash string) (bool, error)`** — resolves the repo via
  `git.FindRepo(worktreeRoot)`, then runs `git merge-base --is-ancestor <hash> <mainBranch>`.
  Returns `true` if the commit is reachable from main. A `*RunError` with exit code 1 is not an
  error — it means "not an ancestor". Any other error is returned to the caller.

These functions call the `gh` CLI (not `git`) as an external process, similar to how the `git`
package calls `git`. They are grouped here because they are GitHub-specific complements to the
existing git remote/push infrastructure.

### Per-view binding

Each view registers `bindingGotoPR` with sequence `["g", "p"]`, category "Go to", title "open PR".
This slots naturally into the existing `g`-chord group alongside `g g`, `g o`, and `g h`.

**Worktrees view**: dispatches `BranchPRURL(selectedWorktree.Path)`. The worktree's path is used
(not the repo root) so that `gh` resolves the correct branch for that worktree.

**Status view**: dispatches `BranchPRURL(worktreeRoot)`.

**Log view**: reads `row.class` for the currently selected row.
- Empty class (in-master) → `CommitPRURL(worktreeRoot, row.commit.FullHash)`
- Any non-empty class → `BranchPRURL(worktreeRoot)`

**Commit view**: because `BranchHistoryClass` is not stored in the commit model, the binding
dispatches an async command that:
1. Calls `IsCommitMergedToMain(worktreeRoot, ref)`
2. If merged → calls `CommitPRURL(worktreeRoot, ref)`
3. If not merged → calls `BranchPRURL(worktreeRoot)`
4. Opens the URL via `ui.CmdOpenURL`, or emits `notify.Warning("no PR found")` if empty

### Error and empty-result handling

- A non-nil error from `BranchPRURL` or `CommitPRURL` is surfaced as `notify.Warning("no PR found")`.
- An empty string from `CommitPRURL` (valid JSON response with no results) is also treated as
  `notify.Warning("no PR found")`.
- `IsCommitMergedToMain` returning an error (other than the "not an ancestor" exit-1 case) surfaces
  as `notify.Error(err.Error())`.

### `g` chord cancel binding

Each view's `g`-chord group already includes `g esc` mapped to `bindingCancelChord`. The new `g p`
binding fits within this group without requiring any chord infrastructure changes.

## Testing Decisions

**What makes a good test**: test observable outputs given specific inputs. For the `git` package,
that means using `testutil.TempRepo` to create real git repos and asserting on return values — not
on internal state or which git commands were called.

**`IsCommitMergedToMain`** — testable in isolation with temp repos. Create a repo with a main
branch and a feature branch; commit to each; verify the function returns `true` for commits
reachable from main and `false` for commits only on the feature branch. Prior art:
`git/branch_test.go` uses `testutil.TempRepo` extensively for similar ancestor checks.

**`BranchPRURL` and `CommitPRURL`** — these call the live `gh` CLI which requires GitHub
credentials and a real remote; not suitable for automated unit tests. Covered by manual verification
only.

**View binding dispatch** — the existing view test files (e.g. `ui/log/model_test.go`,
`ui/worktrees/worktrees_test.go`) test key dispatch by sending key messages and asserting on
resulting commands/notifications. A test for `g p` when no PR exists should assert that
`notify.Warning` is emitted. This requires either a mock for the `gh` lookup or testing only the
path that returns an empty/error result.

## Out of Scope

- GitLab, Bitbucket, or non-GitHub PR hosts.
- Creating a PR from gx (the push flow already handles this via `ExtractPRURL`).
- Showing PR status or metadata inline in any view.
- Caching PR URLs between keypresses.
- Multi-PR selection (when `gh pr list` returns more than one result, we open the first).

## Further Notes

- The `gh` CLI must be installed and authenticated for `g p` to work. No special error handling is
  needed beyond the existing "no PR found" warning — if `gh` is missing entirely, the error message
  from the shell will surface through the error return path.
- In the log view, the in-master classification comes from `BranchHistoryClass` being empty (the
  `default` case in `commitState`). This is the same signal used for rendering — the code already
  distinguishes merged commits from pushed/unpushed/diverged ones.
- `gh pr list --search <hash>` uses GitHub's search API. For merged PRs, GitHub indexes commits,
  so this reliably finds the PR. For unmerged commits, we use the branch-level path instead, so
  the hash search is only invoked where it is known to work.
