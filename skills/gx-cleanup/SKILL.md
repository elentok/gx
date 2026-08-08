---
name: gx-cleanup
description:
  Scan this repo's epics and branches for cleanup opportunities (archivable done epics,
  mergeable done-but-unmerged epics, deletable landed branches/worktrees), investigate the
  cases the deterministic scan can't resolve on its own, present one confirmable plan, and
  on confirm execute the safe half (archive, safe deletes, review-ticket stubs,
  housekeeping) plus the ff-only merges, in the same run, off the same confirm.
disable-model-invocation: true
---

# gx Cleanup

## Background

A repo driven by ralph-loop accumulates epics under `.scratch/` and the branches/worktrees it
created for them long after they stop being useful: epics fully done and already merged, epics
fully done but never merged, and iteration/feature/other branches whose commits already landed
elsewhere. `gx cleanup scan` classifies all of this deterministically from git and ticket state.
This skill turns that classification into a report you can act on: it investigates the handful of
cases the scan can't resolve on its own, then stops at a single confirm. On confirm, the
mechanically-safe actions — archiving, deleting cleanly-landed branches/worktrees, stamping out
review-ticket stubs, and housekeeping — run immediately (see "Step 6" below), and the ff-only
merge candidates run immediately after, in the same invocation (see "Step 7" below).

## Step 1: scan

Run:

```
gx cleanup scan --json
```

The result has three parts:

- **`epics`** — one entry per non-`.archive` directory under `.scratch/`: `all_done`,
  `merged_to_main`, `has_code_review_ticket`, `code_review_done`.
- **`worktrees`** — one entry per local branch other than the main branch: `kind`
  (`"iteration"` | `"feature"` | `"other"`), `active` (recent commit or uncommitted changes —
  never propose an action against this one), `epic`/`ticket_id`/`ticket_done`/`landed` (iteration
  branches only), `merged_to_main` (feature/other branches only), and `recommendation`
  (`"delete"`, `"merge"`, or `""` when the scan itself can't call it — always `""` when `active`
  is true).
- **`housekeeping`** — tracked files that leaked under `.scratch` and whether `.gitignore` covers
  it.

## Step 2: classify

**Epics:**

| condition | verdict |
|---|---|
| `all_done && merged_to_main` | archive — unambiguous |
| `!all_done` | still in progress — report only, no action |
| `all_done && !merged_to_main && has_code_review_ticket && !code_review_done` | waiting on review — report only, no action yet |
| `all_done && !merged_to_main && has_code_review_ticket && code_review_done` | merge candidate, review already satisfied ("case 2.1") — unambiguous |
| `all_done && !merged_to_main && !has_code_review_ticket` | **ambiguous ("case 2.2")** — needs both investigation (Step 3) and your pick between adding a review ticket or merging ff-only skipping review (Step 4) |

**Worktrees/branches:**

| condition | verdict |
|---|---|
| `active` | in-progress work — report only, no action, regardless of `recommendation` |
| `!active && recommendation == "delete"` | safe delete — unambiguous |
| `!active && recommendation == ""` | **ambiguous** — not cleanly cherry-picked/merged; needs investigation |

## Step 3: investigate ambiguous items

For every ambiguous item from Step 2 (case-2.2 epics, and non-active branches the scan left
unrecommended), dispatch one investigation sub-agent per item — independent items, so run them in
parallel. Give each sub-agent:

- the item's scan fields from Step 1
- for a case-2.2 epic: the diff between the epic's feature branch and the main branch
- for a branch: the diff between the branch and its landing target — the epic's feature branch
  for `kind == "iteration"`, the main branch for `kind == "feature"`/`"other"` — plus its last
  commit's age
- any related ticket/run-log state under `<root>/<epic>/` (`gx tickets root` for `<root>`; see
  gx-investigate's "Where state lives" section for `run-log.jsonl`'s shape)

Require it to return exactly one of:

- `safe to delete`
- `needs manual rebase/cherry-pick` — plus the drafted git commands (never run them)
- `unclear — needs your eyes`

plus 1-2 sentences of evidence backing that call. A bare "needs more investigation" is not a valid
answer — if a sub-agent returns one, send it back once telling it to commit to one of the three.

## Step 4: two-way choice for case-2.2 epics

For every case-2.2 epic (done, unmerged, no code-review ticket), ask: add a code-review ticket
(`gx tickets ensure-code-review <epic>`) before merging, or merge ff-only and skip review. Collect
every pick in this same round, alongside the rest of the report — don't interleave one confirm per
epic with the summary table in Step 5.

## Step 5: report and confirm

Build:

- **An inline summary table**, one line per proposed action: item name, action
  (archive/merge/delete/rebase/skip), a confidence/risk tag (`safe` / `needs-rebase` / `unclear`),
  and for case-2.2 epics the picked path from Step 4.
- **Full investigation detail** — every sub-agent's evidence, diffs, and drafted commands —
  written to a scratch file *outside* the `.scratch` tracker root (it isn't a ticket and
  `gx cleanup scan` would otherwise pick up a stray directory there as an "epic"); a plain temp
  file is fine. Reference it by path in the summary — never inline the detail.

Present the table plus the scratch file path and get one confirm covering the whole plan.

## Step 6: execute the safe half

On confirm, run the mechanically-safe actions below directly — no further per-item prompting. The
ff-only-merge path (case-2.1 merge candidates and the "merge, skip review" pick for case-2.2) is
**not** executed here — it's queued and executed next, in Step 7, off this same confirm.

- **Archive** (`all_done && merged_to_main` epics): `mv .scratch/<epic> .scratch/.archive/<epic>`
  — a plain filesystem move, no git staging or commit, since `.scratch` is gitignored/untracked.
- **Safe deletes** (branches/worktrees with `recommendation == "delete"`, or investigated as "safe
  to delete" in Step 3): run `git branch -d <branch>`, then `git worktree remove <path>` if the
  entry has a `path`. Never pass `--force` to either. If git refuses (not fully merged — the
  scan's or an investigation's classification was wrong), stop for that item only: leave it as-is,
  and add it to an end-of-run failure list with the equivalent force command
  (`git branch -D <branch>` / `git worktree remove --force <path>`) printed for you to run
  manually — never run the force command yourself.
- **Case-2.2 "add review ticket" picks**: run `gx tickets ensure-code-review <epic>`, then flag
  the epic as needs-review in the end-of-run report. Do not attempt to merge it in this run.
- **Housekeeping**: if `housekeeping.TrackedFiles` is non-empty, `git rm --cached <file>` for each.
  If `housekeeping.GitignoreHasScratch` is false, append a `.scratch/` entry to `.gitignore`.

Work through archive, safe-delete, case-2.2, then housekeeping, collecting failures as you go
rather than stopping the whole run on the first one. End with a short report: what ran, the failure
list with its manual force commands (if any), then proceed to Step 7 to run the queued merges.

## Step 7: execute — ff-only merge path

On confirm, execute the merge candidates from Step 6 — case-2.1 epics (already have a done
code-review ticket) and case-2.2 epics where the Step 4 pick was "merge ff-only" — one at a time,
in order. A rebase/conflict or check failure on one epic aborts only that epic's merge; report it
and move on to the next.

For each merge-candidate epic:

1. Try `git merge --ff-only <epic-branch>` directly. If it succeeds, the epic is merged — record it
   as done and move to the next epic. Git's own refusal on a non-fast-forward *is* the check for
   whether a rebase is needed; don't pre-check ancestry yourself.
2. If it fails (non-fast-forward), invoke the `gx-resolving-merge-conflicts` skill inline — same
   conversation/context, not a detached sub-agent — to rebase the epic branch onto main and resolve
   any conflicts.
3. Once that skill finishes, call `gx notify` with a short summary of the epic and what's pending
   review, then **pause and show the resolved diff** (the rebased branch vs main) before doing
   anything further. Do not proceed to checks or the merge until you've reviewed it with the user.
4. After the pause, run this repo's checks on the rebased branch — discover and run them the same
   way `gx-resolving-merge-conflicts` does in its own "automated checks" step; don't hardcode a
   specific check command here.
   - Checks pass: do the `git merge --ff-only <epic-branch>` merge into main.
   - Checks fail: abort this epic's merge — leave main untouched — and record the failure (with the
     check output) for the end-of-run summary. Do not attempt to fix unrelated failures or force the
     merge through.

Never push to a remote at any point in this step — pushing stays an explicit, separate action for
the user.

Once every queued epic has been processed, report an end-of-run summary: which epics merged
cleanly, which needed a rebase (and merged after review), and which were aborted and why.
