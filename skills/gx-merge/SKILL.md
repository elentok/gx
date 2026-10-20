---
name: gx-merge
description:
  Merge a branch/worktree onto the repo's trunk. Tries gx's deterministic
  ff-only merge, and on non-fast-forward, rebases, resolves conflicts, runs
  the repo's checks (one fixer pass, flaky-test triage), then retries the
  merge. With `--ticket <addr>` it runs as a merge ticket, never runs
  `gx merge`, and leaves the ff to the server. It never pauses for confirmation.
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

## Modes

- **Manual** — `/gx-merge <branch>`. Runs Steps 1–5, including the final
  `gx merge`.
- **Ticket** — `/gx-merge --ticket <addr>`, launched by the server for a
  `type: merge` ticket. The branch is the epic named in the address
  (`<project>:<epic>/<id>` → `<epic>`); don't look for it anywhere else. Same
  steps, but Step 1 is an ancestry check instead of `gx merge`, and Step 5 is
  reporting `finished`. Ticket mode never runs `gx merge`: the server owns
  trunk, so it does the ff under its own per-trunk lock and dirty-trunk check.
  Never edit the ticket's `status`; report through `--iteration-status` only.

## Stopping

Every stop below says **stop: `<reason>`**. How to stop depends on the mode:

- **Manual** — report the reason (with any output it names) and end the run.
- **Ticket** — follow [gx-local-tracker.md](../gx-local-tracker.md)'s
  announce-and-stop rule:
  1. `gx tickets ask <addr> --body -` — the `## Question` opens with the
     reason as a one-line summary, then the detail (check output, file list)
     and what a person can do about it.
  2. The `## Handoff` in the same body states the feature worktree (rebase
     aborted, or rebased branch kept and at which commit), any fixer commit,
     and any follow-ups already filed.
  3. End your turn.

Leave trunk untouched at every stop. Never push to a remote at any point in
this flow — pushing stays an explicit, separate action for the user.

## Step 1: attempt the deterministic merge

### Manual

Run:

```
gx merge <branch> --json
```

Branch it three ways on `status`:

- **`"merged"`** — done. Report it (`target` names the branch it landed on)
  and stop.
- **`"needs_rebase"`** — continue to Step 2.
- anything else (a plain command error, e.g. trunk is missing) — **stop:
  `gx merge failed: <error>`**.

### Ticket

Don't run `gx merge`. Instead:

1. Read `target` from the ticket: `gx tickets show <addr>`. Its title reads
   `Merge <branch> onto <target>`. If no target is there, **stop: `merge
   ticket names no target`**.
2. `worktree_path` is the current worktree (`git rev-parse --show-toplevel`);
   the server launches the agent in the feature worktree.
3. Run `git merge-base --is-ancestor <target> <branch>`:
   - **exit 0** — the branch already holds `target`, so the server can ff it.
     Go to Step 5.
   - **exit 1** — continue to Step 2.
   - anything else (e.g. an unknown ref) — **stop: `ancestry check failed:
     <error>`**.

## Step 2: check the worktree

Manual: the result carries `branch`, `target`, and `worktree_path`. Ticket:
Step 1 set them. `git rebase` requires a working tree, so the rest of the flow
only runs when one is checked out for `branch`:

- **`worktree_path` is empty** — no worktree is checked out for `branch`, and
  none may be created or checked out to run the rebase in: doing so risks
  landing the rebase in trunk's own worktree, checking `branch` out there and
  leaving trunk's checkout off trunk (violating ADR 0015's guarantee that this
  flow never touches a worktree's current branch out from under it). **Stop:
  `<branch> has no worktree, so gx-merge can't rebase it`.** The caller (e.g.
  `gx-cleanup`) needs to check out the branch in its own worktree first. In
  ticket mode the server recreates the feature worktree before launch, so
  this should not happen there; if the current worktree is not on `branch`
  (`git branch --show-current`), stop with the same reason.
- **`rtk proxy git status --porcelain` in `worktree_path` is non-empty** — **stop:
  `feature worktree <worktree_path> is dirty: <files>`**. Never commit,
  stash or discard someone else's changes.
- Otherwise continue.

## Step 3: rebase

Run `git rebase <target>` in `worktree_path`.

If it reports conflicts, resolve them **inline** with the
[gx-resolving-merge-conflicts](../gx-resolving-merge-conflicts/SKILL.md)
skill — same conversation/context, not a detached sub-agent, matching
`gx-cleanup`'s Step 7 convention for this same step. Two overrides:

- Skip its checks step; Step 4 below runs them once the rebase is done.
- When a hunk can't be resolved without inventing behaviour, don't force it:
  note the conflicting files (`rtk proxy git diff --name-only --diff-filter=U`), run
  `git rebase --abort` **first** so the worktree is clean on the pre-rebase
  branch, then **stop: `unresolvable rebase conflict in <files>`**.

## Step 4: run checks

Once the rebase is clean, go straight to the checks. Don't pause to show the
rebased diff.

Discover this repo's automated checks the same way
`gx-resolving-merge-conflicts` does — don't hardcode a check command here —
and run them in order: typecheck, tests, lint/format. Each failure kind has
its own path; all paths run in both modes. Run format checks through `rtk proxy`, or only on the
files the branch changed (`rtk proxy git diff --name-only <target>...HEAD`): a repo-wide
`gofmt -l` lists files the branch never touched.

### Typecheck or lint failure: one fixer

The first time typecheck or lint/format fails in this run, launch **one**
fixer subagent in `worktree_path` with the failing output. It fixes only what
the output names, without changing behaviour, and commits on the rebased
branch with the subject
`<branch>: Fix typecheck/lint after rebase onto <target>`. The commit lands
with the branch, so a merge ticket stays commitless.

Then rerun **all** checks from the start. The fixer runs once per run: if
typecheck or lint/format fails again, **stop: `checks still fail after the
fixer: <check>`** with the output. Keep the rebased branch and the fixer
commit.

### Test failure: flaky-test triage

Launch a triage subagent in `worktree_path` with the failing tests and their
output. For each failing test it:

1. Reruns **only that test** 3 times (e.g. Go: `go test -count=1 -run
   '^TestName$' ./pkg`).
2. Lists the branch's diff: `git diff --name-only <target>...HEAD`.
3. Calls it **flaky** only if it passed at least once **and** the diff touches
   neither the test file nor any non-test file in the test's package (Go: the
   test's directory). Anything else — failed all 3 times, the diff touches
   that code, or unsure — is **the branch's fault**.
4. For a branch-fault test only: runs it on `<target>` in a temporary
   detached worktree (`git worktree add --detach <tmp> <target>`, removed
   after), and says whether it **fails on trunk too**.

It returns one verdict per test, with the rerun results and the diff evidence.

- **Any branch-fault test** — keep the rebased branch and **stop: `tests
  fail after rebase onto <target>: <tests>`** with the check output. If any
  of them fails on trunk too, the reason says so: `… (fails on trunk too)`.
- **All flaky** — continue with the remaining checks. File one follow-up per
  flaky test through [gx-follow-up](../gx-follow-up/SKILL.md) (it dedupes):
  the finding is the test, its rerun results and output; Source names the
  branch and, in ticket mode, `<addr>`; pass `unattended` and `category: bug`.
  Note the address it returns for each.

### All checks pass

Continue to Step 5.

## Step 5: finish

- **Manual** — re-invoke `gx merge <branch>` (the original arg, not the
  resolved `target`/`branch` pair) for the final ff-only merge now that the
  rebase brought it up to date. Report the outcome and any flaky-test
  follow-ups.
- **Ticket** — skip `gx merge`. If any flaky-test follow-ups were filed, list
  their addresses with `gx tickets section <addr> Resolution -`. Then run
  `gx tickets set <addr> --iteration-status finished`; the server checks the
  rebase finished and `<target>` is an ancestor of the branch, then does the
  ff.

Finish with zero background shells: read and stop (TaskStop) any that are
still open before reporting.
