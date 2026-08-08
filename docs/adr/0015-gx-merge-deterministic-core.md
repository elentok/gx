# `gx merge` owns only the deterministic half of a merge

`gx-merge` (the skill) merges a branch/worktree onto main ff-only, rebasing and resolving conflicts
when needed. We considered putting the whole flow — including running `git rebase` and stopping on
conflicts — into a new `gx merge` Go command, so the skill would just call it once. We instead split
it: `gx merge <branch>` only resolves the arg (worktree-dir name or literal branch) to a real branch
+ base via `git.Worktree`, and attempts `git merge --ff-only`; on failure it reports
`{status, branch, base, worktree_path}` and does nothing else. The skill runs `git rebase` itself and
hands off to `gx-resolving-merge-conflicts` on conflicts, then re-invokes `gx merge` for the final
merge. This keeps the Go command judgment-free (never leaves the repo mid-rebase) and gives
`gx-cleanup`'s existing ff-only-merge step the same command to call instead of duplicating the
resolve-and-attempt logic inline.
