---
name: gx-merge
description:
  Merge a branch/worktree onto the repo's trunk. Tries gx's deterministic
  ff-only merge, and on non-fast-forward, rebases, resolves conflicts, runs
  the repo's checks, then retries the merge. It never pauses for
  confirmation.
disable-model-invocation: true
---

# gx Merge

Owns the judgment half of merging a branch onto the repo's trunk
(`project.json`'s `trunk`, else the detected default) — `gx merge <branch>`
(see ADR 0015) handles the deterministic half (resolve trunk and the branch,
attempt `git merge --ff-only`) and stops the moment it needs a decision only a
human/agent should make. Always read the branch it merged onto from `target`;
never assume "main".

Run every step without asking the person anything. Each stop condition below
reports and ends the run; none waits for a reply.

## Step 1: attempt the deterministic merge

Run:

```
gx merge <branch> --json
```

Branch it three ways on `status`:

- **`"merged"`** — done. Report it (`target` names the branch it landed on) and stop.
- **`"needs_rebase"`** — continue to Step 2.
- anything else (a plain command error, e.g. trunk is missing) — report the
  error and stop. Do nothing further; trunk is untouched.

## Step 2: rebase

The result carries `branch`, `target`, and `worktree_path`. `git rebase`
requires a working tree, so this step only runs when one is checked out for
`branch`:

- **`worktree_path` is non-empty** — run `git rebase <target>` there, then
  continue below.
- **`worktree_path` is empty** — no worktree is checked out for `branch`, and
  none may be created or checked out to run the rebase in: doing so risks
  landing the rebase in trunk's own worktree, checking `branch` out there and
  leaving trunk's checkout off trunk (violating ADR 0015's guarantee that this
  flow never touches a worktree's current branch out from under it). Stop
  here, report that `<branch>` has no worktree so `gx-merge` can't rebase it,
  and leave trunk untouched. This case is currently unsupported — the caller
  (e.g. `gx-cleanup`) needs to check out the branch in its own worktree first.

If the rebase reports conflicts, invoke the
[gx-resolving-merge-conflicts](../gx-resolving-merge-conflicts/SKILL.md)
skill **inline** — same conversation/context, not a detached sub-agent,
matching `gx-cleanup`'s Step 7 convention for this same step.

## Step 3: run checks

Once the rebase is clean (conflicts resolved or none arose), go straight to
the checks. Don't pause to show the rebased diff.

Discover and run this repo's automated checks (typecheck, then tests, then
format) the same way `gx-resolving-merge-conflicts` does in its own
"automated checks" step — don't hardcode a specific check command here.

- **Checks fail**: abort. Leave trunk untouched, report the failure (with
  output) and stop. Do not attempt to fix unrelated failures or force the
  merge through.
- **Checks pass**: continue to Step 4.

## Step 4: final merge

Re-invoke `gx merge <branch>` (the original arg, not the resolved
`target`/`branch` pair) for the final ff-only merge now that the rebase brought
it up to date. Report the outcome.

Never push to a remote at any point in this flow — pushing stays an explicit,
separate action for the user.
