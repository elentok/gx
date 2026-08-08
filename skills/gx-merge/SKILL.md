---
name: gx-merge
description:
  Merge a branch/worktree onto the repo's main branch. Tries gx's deterministic
  ff-only merge, and on non-fast-forward, rebases, resolves conflicts, pauses
  for review, runs the repo's checks, then retries the merge.
disable-model-invocation: true
---

# gx Merge

Owns the judgment half of merging a branch onto the repo's detected main
branch — `gx merge <branch>` (see ADR 0015) handles the deterministic half
(resolve the branch, attempt `git merge --ff-only`) and stops the moment it
needs a decision only a human/agent should make.

## Step 1: attempt the deterministic merge

Run:

```
gx merge <branch> --json
```

Branch it three ways on `status`:

- **`"merged"`** — done. Report it and stop.
- **`"needs_rebase"`** — continue to Step 2.
- anything else (a plain command error) — report the error and stop. Do
  nothing further; main is untouched.

## Step 2: rebase

The result carries `branch`, `base`, and `worktree_path`. `git rebase`
requires a working tree, so this step only runs when one is checked out for
`branch`:

- **`worktree_path` is non-empty** — run `git rebase <base>` there, then
  continue below.
- **`worktree_path` is empty** — no worktree is checked out for `branch`, and
  none may be created or checked out to run the rebase in: doing so risks
  landing the rebase in main's own worktree, checking `branch` out there and
  leaving main's checkout off main (violating ADR 0015's guarantee that this
  flow never touches a worktree's current branch out from under it). Stop
  here, report that `<branch>` has no worktree so `gx-merge` can't rebase it,
  and leave main untouched. This case is currently unsupported — the caller
  (e.g. `gx-cleanup`) needs to check out the branch in its own worktree first.

If the rebase reports conflicts, invoke the
[gx-resolving-merge-conflicts](../gx-resolving-merge-conflicts/SKILL.md)
skill **inline** — same conversation/context, not a detached sub-agent,
matching `gx-cleanup`'s Step 7 convention for this same step.

## Step 3: pause for review

Once the rebase is clean (conflicts resolved or none arose), **pause and show
the rebased diff** (the rebased branch vs `base`) before doing anything
further. Do not proceed to checks or the merge until it's been reviewed.

## Step 4: run checks

Discover and run this repo's automated checks (typecheck, then tests, then
format) the same way `gx-resolving-merge-conflicts` does in its own
"automated checks" step — don't hardcode a specific check command here.

- **Checks fail**: abort. Leave main untouched, report the failure (with
  output) and stop. Do not attempt to fix unrelated failures or force the
  merge through.
- **Checks pass**: continue to Step 5.

## Step 5: final merge

Re-invoke `gx merge <branch>` (the original arg, not the resolved
`base`/`branch` pair) for the final ff-only merge now that the rebase brought
it up to date. Report the outcome.

Never push to a remote at any point in this flow — pushing stays an explicit,
separate action for the user.
