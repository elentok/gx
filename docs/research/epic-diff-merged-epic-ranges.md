# Rebuilding the range of already-merged epics

Ticket: `gx:epic-diff/03`. Tags: **[gx]** gx source in this worktree; **[git]** a read-only git command on this repo (the shared bare repo, `main` = `fc592dc12`, 6182 commits); **[store]** the real ticket store (`gx tickets root`, run logs beside the tickets). Checked on 2026-11-13. No product code, ticket or ref was changed.

## Answer in short

- **Use main's reflog.** Every fast-forward of `main` logs `old new`. For an epic that is exactly `trunkBefore` and the tip. It gave an exact range for **110 of 177** merged epics that have commits on `main`. It is the only source I found that is exact.
- **Trailers alone are not a correct range.** They give the right tip for 86 of 107 epics and the right base for 27 of 107 (both right: 25). They are good for *finding* which epic a reflog entry belongs to, not for the range.
- **Run-log `sha` is stale after a rebase.** The last `cherry-picked` sha is the tip for 50 of 107 epics. `target_tip` is an exact base (34 of 34 where present), but it exists only from 2026-11-01.
- **66 merged epics must show "No diff recorded".** They merged before the reflog starts (2026-10-12), or by a hand cherry-pick. Any range for them would be a guess.
- The reflog is local and can be pruned. **Backfill it once, now**, and verify each entry (rules below).

## Counts

Universe: the 201 archived epics that have a `run-log.jsonl` [store]. A merged epic lives in `.archive/` (`tickets/archive.go:10`); only 3 live epics have `merged_at`, all standing inboxes (`one-offs`, `one-offs-archived`, `follow-ups-archived`). I could not read `merged_at` of archived epics (see "Not confirmed").

| Group | Epics | Result |
|---|---|---|
| Archived epics with a run log | 201 | |
| of these: trailer commits on `main` | 177 | |
| recovered exactly from the reflog, one range | **110** | 107 matched by branch name in `merge <branch>: Fast-forward`, 3 matched by trailers (empty reflog message) |
| reflog gives two ranges | 1 | `agent-lifecycle-impl` landed twice: "No diff recorded" |
| no reflog entry | 66 | 65 merged before 2026-10-12, 1 (`idle-cost`) by a hand `cherry-pick`: "No diff recorded" |
| of these: no commits at all | 22 | 8 `cpu-spike-*`, 13 `evolve-*`, `gx-tickets-set-retrofit`; nothing to merge, "No changes", not "No diff recorded" |
| of these: commits not on `main` | 2 | `ralph-loop-recovery-e2e` (33 picks, none on `main`, 15 shas gone from the object store, branch `ralph-loop/ralph-loop-recovery-e2e/iter-19` still exists), `ralphloop-tui-impl` (only bare `02`, `03`, `04a` ... trailers): "No diff recorded" |
| Archived dirs without a run log | 155 | maps and legacy `epic.yaml` epics; no trailer ties them to a commit |

Cross-check of the reflog against trailers (107 single-merge epics matched by name, plus the 3 unnamed): the reflog range holds every trailer commit of the epic in all but 3 cases, and holds a commit of another epic in 1 case. Both are explained below. All 137 `merge ...: Fast-forward` entries have `old` as an ancestor of `new`.

## How the ranges are written today [gx]

- Merge is fast-forward only: `MergeFastForward` runs `git merge --ff-only` (`git/history/merge.go:115`); with no checkout it calls `fastForwardRef` (`git/history/merge.go:75`). Nothing is stored. `trunkBefore` is read at `server/merge/epicmerge_root.go:213` and used once, for the line count (`:231`).
- `fastForwardRef` runs `update-ref` with no `-m` (`git/history/merge.go:84`). That is why 3 reflog entries have an empty message and no branch name.
- Trailer: `StampMerged` amends **only HEAD** of a ticket's pick (`ralphloop/merge/merge.go:278`, `git/history/cherry_pick.go:155`). A ticket with several commits has the trailer on its last commit only. The value is `<project>:<epic>/<id>` from the ticket path at stamp time (`ralphloop/merge/merge.go:248`, `tickets/address.go:72`).
- Run log: `sha` is the feature tip right after a pick (`ralphloop/runlog/eventlog.go:116`). `resolved_base` is declared (`:119`) but appears in **0** of the run logs [store]. `target_tip` (`:123`) is on merge tickets only, from 2026-11-01 (51 events).
- PR delivery: `git/pr/read.go:66,91` can read `mergeCommit`; nothing stores it.

## Question 1: is an epic's trailer run unbroken on first-parent history?

Mostly, but not always. [git] `git log main --first-parent` with the trailer.

- Trailer spans (first to last trailer commit) in which every commit is a trailer commit or has the epic's subject prefix (`<epic>/NN`, `<design>:`): **167 of 177** (107 of 111 in the reflog era, 60 of 66 before it).
- 4 reflog-era and 6 older epics have other commits inside the span. Examples: `disk-space-impl` (span 94 commits, 74 foreign) because one of its picks, `disk-space-impl/01`, landed inside `better-tests-impl`'s fast-forward, and `agent-lifecycle-impl` (two landings, a `Merge main into agent-lifecycle-impl` commit, a backup ref `refs/gx/backups/agent-lifecycle-impl-...`).
- The trailer span is **not** the epic's range even when unbroken:
  - The first commit of the epic is often a spec commit (`title-bar: Add spec, ADRs and glossary terms`). It has no trailer, so the first trailer's parent is too late.
  - The epic often ends with a hand fix after the last pick (`... Fix typecheck/lint after rebase onto main`, 22 of 107).
  - Measured against the reflog (107 single-merge epics): base right 27, tip right 86, both right 25.

## Question 2: which signal is more reliable?

| Signal | Base | Tip | Why |
|---|---|---|---|
| main reflog `old new` | exact | exact | Written by git at the ff. 110 of 110 verified against trailers. |
| Trailers (first, last) | 27 of 107 | 86 of 107 | Only one commit per ticket; spec and hand commits have none. Survives rebase. |
| Trailers + subject-prefix walk (epic name or name without `-impl`) | 101 of 107 | 105 of 107 | A heuristic. Right 99 of 107 for both, wrong 8. It cannot be checked for epics without a reflog. |
| Run log last `cherry-picked` `sha` | none | 50 of 107 | Picks are rewritten by the rebase of a merge ticket: 0 of 33 epics with a merge ticket keep it. Only 83 of 175 last shas are still on `main`. |
| Run log last `target_tip` | 34 of 34 | none | Equals the reflog `old` every time, but only for epics with a merge ticket claimed since 2026-11-01. 0 of the 66 unrecoverable epics have it. |

Trailers are the more reliable *identity* signal (they survive the rebase and the ff); the run log's shas do not. Neither gives the range alone.

## Question 3: hand commits, merge tickets, conflict resolution

- Hand commits and conflict-resolution commits have no trailer. They sit inside the range (reflog era: untrailed commits of the epic sit inside nearly every range, mostly the earlier commits of multi-commit tickets). Only the reflog includes them.
- A merge ticket rebases the epic onto trunk before the ff. Its cherry-pick shas then stop matching `main`. The trailers still match (trailer count equals pick count in 158 of 175 epics).
- `disk-space-impl`: reflog range holds 19 of its 20 trailer commits. The 20th, a second `disk-space-impl/01`, sits in `better-tests-impl`'s range. The reflog range is still the right epic range; the trailer set is not.
- `agent-lifecycle-impl`: two ff entries (`122438f9..e3a7e2fd`, 30 commits; `2f4baa57..b55d0a41`, 15 commits). Other work landed between them. A single base and tip would cover foreign commits.

## Question 4: PR-delivery epics

- No local record. No `pr-activity.jsonl` exists anywhere in the store [store] (`server/prflow/activity.go:18` is the file name). `main` has no squash subjects ending `(#N)` and one old `Merge pull request #1`. I found no merged epic that was delivered by PR, so I cannot test a method on real data.
- A PR epic's real source is GitHub, not git: `mergeCommit` (read at `git/pr/read.go:91`, not stored). For a squash merge the range is `mergeCommit^..mergeCommit`. Untested here. Until a PR epic exists with a stamped `merge commit`, show "No diff recorded" for any epic with `pr_number` and no stamp.

## Question 5: renamed and archived epics

- A rename keeps the branch (`tickets/epic.go:154`), so the reflog says `merge <old-branch>` while trailers use the epic name at stamp time (`ralphloop/merge/merge.go:248`). In this store every trailer epic name (176, not counting the bare ones) has an archive directory with the same name, and none of the 20 non-epic branch merges in the reflog contains an epic trailer. So no rename broke a match here. This is **not proven** for future renames.
- Match a reflog entry to an epic by **trailer containment** (every trailer commit of the epic lies in `old..new`), not by branch name. It found the 3 unnamed entries and needs no name.
- Archiving moves the directory to `.archive/` with run log and tickets intact, so archived epics lose nothing.

## Recommended method

For each merged epic, in order:

1. **Reflog, verified.** Read `main`'s reflog for entries where `old` is an ancestor of `new` and the message is `merge <x>: Fast-forward` or empty. Pick the entry whose `old..new` contains **all** the epic's trailer commits. Stamp `base = old`, `tip = new`. Refuse (go to 3) when: no entry matches, two entries match, trailers are only partly inside the range, or the range holds a trailer commit of another epic that is not accounted for.
2. **Merge tickets since 2026-11-01:** `target_tip` of the last merge ticket claim is the same `old` and can confirm step 1.
3. **Otherwise show "No diff recorded".** Do not fall back to the trailer span or the subject-prefix walk.

Run the backfill once, before the reflog is pruned, as a one-off command that writes the range into each epic and logs the epics it refused.

## Cases that must show "No diff recorded" (69 epics with commits)

| Case | Epics | Why |
|---|---|---|
| Merged before 2026-10-12 | 65 | No reflog; trailers give no exact base or tip. 6 have other commits inside their span. |
| Hand `cherry-pick` into `main` | 1 (`idle-cost`) | Reflog entry is a `cherry-pick`, not a ff. |
| Landed in two parts | 1 (`agent-lifecycle-impl`) | Two ranges, other work between them. |
| Commits not on `main` | 2 (`ralph-loop-recovery-e2e`, `ralphloop-tui-impl`) | Picks rewritten or never landed; bare trailers cannot be tied to an epic. |
| Epics without a run log or trailer | 155 archived dirs | Nothing links them to commits. |
| PR-delivered epics without a stamp | none found | No local record. |

Epics with no commits at all (22) show "No changes", not "No diff recorded". The 65 older epics could show an approximate range if the page labels it. I recommend against it: the heuristic is wrong for 8 of 107 epics where it can be checked, and it cannot be checked where it would be used.

## Not confirmed

- `merged_at`, `pr_number` and `branch_name` of archived epics: archived epics cannot be shown with `gx tickets show`, and I did not read their `ticket.md` by path. "Merged" for the 201 is taken from the archive convention (`tickets/archive.go:10`).
- How long the reflog lives: `gc.reflogExpire` is unset (git default 90 days). The log starts at 2026-10-12 although commits date from August, so the file was probably recreated; I could not tell why or whether it can be pruned again.
- Reflogs are local: a clone or a second machine has none. Stamping must run on the machine that merged.
- Ticket frontmatter `resolved_base` (`ralphloop/ticketops/claim.go:41`) is not readable for archived epics; it records trunk at claim time, not at merge, so it is not the base.
- Whether the Diff page wants the spec commits that sit on the epic branch (`title-bar: Add spec ...`). The reflog range includes them.

## Commands run

- `git log main --first-parent --format=...%(trailers:key=Ralph-Loop-Ticket,valueonly)` for all trailer, span and contiguity counts; `git rev-list old..new` for each reflog range; `git merge-base --is-ancestor` for ancestry.
- `git reflog show main`, and a read of `/Users/david/dev/gx/.bare/logs/refs/heads/main` (535 lines: 390 `commit`, 137 `merge ...: Fast-forward`, 3 `cherry-pick`, 3 empty).
- `gx tickets epics`, `gx tickets show gx:<epic>` (live epics), `fd`/`rg` over the store for `run-log.jsonl` (201 archived, 3 live), `pr-activity.jsonl` (0), `epic.yaml` (34).
- A Python script in the scratchpad joined the run logs, trailers and reflog entries.
