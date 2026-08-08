---
name: gx-cleanup
description:
  Scan this repo's epics and branches for cleanup opportunities (archivable done epics,
  mergeable done-but-unmerged epics, deletable landed branches/worktrees), investigate the
  cases the deterministic scan can't resolve on its own, and present one confirmable plan.
  Read-only — this half of the skill never archives, deletes, or merges anything itself.
disable-model-invocation: true
---

# gx Cleanup

## Background

A repo driven by ralph-loop accumulates epics under `.scratch/` and the branches/worktrees it
created for them long after they stop being useful: epics fully done and already merged, epics
fully done but never merged, and iteration/feature/other branches whose commits already landed
elsewhere. `gx cleanup scan` classifies all of this deterministically from git and ticket state.
This skill turns that classification into a report you can act on: it investigates the handful of
cases the scan can't resolve on its own, then stops at a single confirm. Nothing is archived,
deleted, or merged here — that's a separate, later skill invocation (see "Step 6" below).

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

## Step 6: dry-run output

On confirm, print exactly what would run — nothing here executes it:

- `mv .scratch/<epic> .scratch/.archive/<epic>` for each done+merged archive
- `git branch -d <branch>` and `git worktree remove <path>` for each safe delete
- `gx tickets ensure-code-review <epic>` for each case-2.2 epic where you picked "add review
  ticket"
- `git merge --ff-only` (or the rebase-then-merge sequence) for each merge candidate
- `git rm --cached <file>` / the missing `.gitignore` entry, if `housekeeping` flagged either

This ticket's scope ends here — actually running any of the above happens on a later invocation of
this skill's execution half.
