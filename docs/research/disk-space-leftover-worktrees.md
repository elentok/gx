# Disk space: leftover worktrees and other growth

Research for ticket gx:disk-space/02. Measured on 2026-10-27, about 23:00 to 23:25 IST, with gx
v0.39.0-65-g9cb5e8fa.

Measurement note. While this research ran, another session ran a cleanup on `~/dev/gx`. The
worktree count dropped from 133 to 55, and the scan's epic count dropped from 211 to 78. The
"before" numbers below come from the first scan. They match the state right before that cleanup.

## Summary

- gx never removes an epic's feature worktree, not even after the epic merges into trunk. It
  removes only iteration worktrees, design worktrees and start-over worktrees.
- The gx-cleanup skill is the only way to remove feature worktrees and merged branches. It runs
  only when a person types `/gx-cleanup` (`disable-model-invocation: true`).
- Before the cleanup, the scan recommended deleting 79 worktrees. `du` put them at 2.7 GiB. Most of
  that is `node_modules` (42 `node_modules` dirs across all worktrees used 10.1 GiB).
- The largest leak is not worktrees. `$TMPDIR` held 32 GiB, and 28 GiB of it came from 425
  `gx-e2e-bin-*` dirs. The Playwright global setup creates one per e2e run and never removes it.
  The Go test helper `claudeenv.Isolate` leaks another 2 GiB, because its cleanup cannot delete
  the read-only Go module cache that tests download into the fake `HOME`.

## 1. Which worktrees gx leaves behind, and why

### Where gx creates and removes worktrees

| Kind | Created | Removed |
|---|---|---|
| Iteration (`<epic>-item-<id>`) | `ralphloop/iteration.go:63` | `finishCleanup`, `ralphloop/iteration.go:711-735`, after the ticket merges. Also on startup repair, `ralphloop/reconcile_repair.go:79`, and on start-over, `repair/startover.go:136`. |
| Feature (epic, `<epic>`) | `ralphloop/loop.go:473`, `ralphloop/feature.go:19`, `ralphloop/servermerge.go:141`, `server/epicmerge.go:326` | Never. |
| Design (map) | `server/designs.go:202-214` | `server/designs.go:120-170`, once the map is past the Branch step, no design session is live, the worktree is clean, and every design commit is in `<map>-impl`. |
| Session worktree (web "new session") | `server/sessions.go:256-276` | Never. `POST /v1/sessions/remove` does not touch the worktree. |
| TUI worktree view | `ui/worktrees/worktree_create.go:25` | By hand, `ui/worktrees/delete.go:33-46` (force-removes the worktree and force-deletes the branch). |

Notes:

- `finishCleanup` keeps the iteration branch when a ticket pauses for an answer
  (`ralphloop/iteration.go:702-705`). If that ticket is never resumed, the branch stays.
- `PrepareFeature` marks the feature worktree `keep: true` (`ralphloop/feature.go:38`).
  `FinishMerge` says "The feature worktree always stays" (`ralphloop/feature.go:47`).

### What happens when an epic merges into trunk

`completeEpic` (`server/epicmerge_root.go:57`) calls `mergeEpic`
(`server/epicmerge_root.go:173`). On success it stamps `merged_at` and `completed_at`
(`server/epicmerge_root.go:189`, `:213`), dequeues the root (`:223`), sends notices and files the
retro. Nothing in this path removes the feature worktree or deletes the epic branch. The
fast-forward itself (`git/merge.go:26`) only moves the target ref.

So every queued epic leaves one full worktree, with its `node_modules`, until a person cleans up.
Before the cleanup, 58 feature worktrees were already merged and idle.

### Why gx-cleanup does not keep up

- The skill has `disable-model-invocation: true` (`skills/gx-cleanup/SKILL.md:9`). Only a person
  can start it. No server code, timer or post-merge hook runs it.
- `gx cleanup` has a single subcommand, `scan` (`cmd/commands.go:474-494`). Every action (archive,
  `git worktree remove`, `git branch -d`) is a shell command the agent runs from the skill text
  (`skills/gx-cleanup/SKILL.md:135-145`).
- Each run needs an agent and a confirm round, because the skill mixes the safe actions with
  investigations and merges.

### Gaps in the scan itself

- Archived epics are invisible. `runCleanupScan` loads only live epics (`cmd/cleanup_scan.go:84`).
  An iteration branch of an archived epic gets `ticket_done=false`, so it is never recommended
  for deletion (`cmd/cleanup_scan_worktrees.go:119-134`). Example:
  `ralph-loop/drain-queue-item-06` (epic `drain-queue` is in `.archive`).
- Once the epic branch is gone, the patch-id check is skipped. `iterationMerged` compares patch
  ids only against the epic branch (`cmd/cleanup_scan_worktrees.go:194-211`). Against trunk it
  checks only ancestry (`:214`), and cherry-picked commits are never ancestors. Example:
  `ralph-loop/orchestrator-daemon-5-recovery-item-06` had `ticket_done=true` but `merged=false`.
- Detached-HEAD worktrees are skipped. The scan lists branches, not worktrees
  (`cmd/cleanup_scan_worktrees.go:55-81`). Two detached worktrees existed.
- The standing `one-offs` epic shows up as "done, unmerged, no review" (case 2.2). It never merges
  (`server/epicmerge_root.go:28-31`), so the skill should skip it.

After the concurrent cleanup, 15 idle worktrees remain that the scan does not recommend. Seven are
iteration worktrees from archived epics (the gaps above). The rest are `prototype/*`,
`research/*`, `fix-session-cost` and the unmerged `idle-cost` epic.

## 2. When gx may remove a worktree safely

### What the scan said (before the concurrent cleanup)

Branches (160 local, trunk excluded):

| Kind | Active | Recommendation | Has worktree | Count |
|---|---|---|---|---|
| feature | no | delete | yes | 58 |
| feature | no | none | yes | 2 |
| feature | yes | none | yes | 16 |
| iteration | no | delete | yes | 5 |
| iteration | no | delete | no | 1 |
| iteration | no | none | yes | 9 |
| iteration | no | none | no | 8 |
| iteration | yes | none | yes | 2 |
| other | no | delete | yes | 16 |
| other | no | delete | no | 1 |
| other | no | none | yes | 7 |
| other | no | none | no | 16 |
| other | yes | none | yes | 15 |
| other | yes | none | no | 4 |

Totals: 81 "delete" (79 with a worktree), 42 unresolved, 37 active. 3 attic refs.

Epics (211): 134 archive (done and merged), 71 in progress, 1 merge candidate with a done review
(case 2.1), 5 case 2.2 (`better-tests`, `flaky-session-test`, `idle-cost`, `test-logging`,
`test-suite-performance`, plus `one-offs`, which is a false positive).

After the cleanup: 78 epics, 87 branches, 0 "delete" recommendations, 37 active.

### The scan's rules

- `active`: the branch tip is younger than 48 hours, or the worktree has staged, unstaged or
  untracked changes (`cmd/cleanup_scan_worktrees.go:17`, `:219-236`). Ignored files do not count.
- Iteration branch: delete when not active, the ticket is done, and the commits are in the epic
  branch (ancestor or same patch ids) or in trunk (`:119-135`, `:193-215`).
- Feature or other branch: delete when not active and the tip is an ancestor of trunk
  (`:144-153`, `git/pr.go:230`).

### Recommended rule

The disk cost is in worktrees, not branches. A branch costs almost nothing, and it holds every
commit. So split the rule in two.

Remove a worktree when all of these hold:

1. It is not the trunk checkout.
2. It has no staged, unstaged or untracked changes (`git worktree remove` without `--force`
   refuses otherwise, so git enforces this).
3. No live agent session or pane uses it (the server knows this. The design cleanup already
   checks it, `server/designs.go:131-134`).
4. It is not the feature worktree of an epic that is queued or has open tickets. gx recreates a
   missing feature worktree (`ralphloop/feature.go:11-12`, `server/epicmerge.go:315-326`), but
   removing it mid-run costs a reinstall.
5. Its branch is merged (the scan's rule above), or the person confirms it by hand.

Rule 5 protects nothing on disk, because the branch survives. It only stops the person from
losing the place they were working in. With rules 1 to 4, removing an unmerged worktree loses
only ignored files. Most are rebuildable (`node_modules`, `dist`, `*.tsbuildinfo`, the `gx`
binary), but an ignored notes file would be lost. The button should list any ignored files outside
a known rebuildable set before it removes the worktree.

Delete a branch only when it is merged:

- feature or other: the tip is an ancestor of trunk, so `git branch -d` works.
- iteration: the patch ids are in the epic branch or in trunk. `git branch -d` refuses
  cherry-picked branches, so the server should not use `-D` blindly. The clean option is to move
  the branch to `ralph-loop/attic/` like start-over does (`repair/startover.go:149`). That
  keeps the commits and clears the branch list.

Clean fix for the main leak: remove the feature worktree in `mergeEpic` right after a successful
merge, with the same checks (no live session, clean, not `--force`). Keep the epic branch there.
The design cleanup still reads `<map>-impl` (`server/designs.go:148`), and the retro may look at
it. Branch deletion stays with the cleanup section.

## 3. A web cleanup section

### What can be a button

| Action | Backing code today | Needed |
|---|---|---|
| List done epics, merged branches, worktrees | `gx cleanup scan` logic in `cmd/cleanup_scan.go` and `cmd/cleanup_scan_worktrees.go` | Move it out of `cmd` into a package the server can call. Add `GET /v1/cleanup?project=`. Add size per worktree. |
| Archive done and merged epics | The skill's `mv` (`skills/gx-cleanup/SKILL.md:135`). `tickets.ArchiveDir` (`tickets/archive.go:12`). | A `tickets` function that moves an epic under its lock. `POST /v1/cleanup/archive`. Refuse if queued. |
| Remove worktrees | `git.RemoveWorktree` (`git/worktree.go:66`) | `POST /v1/cleanup/worktrees` with the rule in section 2. Never `force`. Take the ralph-loop worktree lock. |
| Delete merged branches | `git.DeleteLocalBranch` (`git/branch.go:88`). Attic rename in `repair/startover.go`. | Same endpoint or a sibling. `-d` for ancestors, attic for cherry-picked iteration branches. |
| Disk use | none | Free bytes on the volume, and per-worktree size. Size needs a slow `du`, so compute it in the background and cache it. |

Each row should show why it is safe (merged into what, last commit age, clean). A row that git
refuses should show git's reason and stay. The UI should never offer `--force`.

The existing write routes are in `server/server.go:601-678`. The web-exposed list is in
`server/web.go:112-136`. A cleanup write route goes in both.

### What needs an agent

- Unresolved branches (42 before the cleanup). The skill sends an agent to read the diff and say
  "safe to delete", "needs rebase" or "unclear" (`skills/gx-cleanup/SKILL.md` Step 3).
- Case 2.2 epics: done, unmerged, no review. Someone has to choose between a review ticket and a
  merge.
- The ff-only merges. They run gx-merge with its checks and fixer (Step 7).

The web section can start that half through the one-off path, the same way the retro does
(`server/oneoff.go:184`, `server/epicmerge_root.go:313`). The button would file a one-off that runs
`/gx-cleanup` on the unresolved items only. `POST /v1/oneoff` already exists
(`server/server.go:625`).

## 4. Sizes

### Worktrees

- `~/dev/gx` total: 13 GiB before, 9.8 GiB after the concurrent cleanup. The bare repo is 167 MiB.
- The 79 worktrees the scan recommended deleting: 2.68 GiB by `du -sk` per dir, 2.68 GiB with
  `du -skc` (hard links counted once). The largest was `e2es-impl` at 458 MiB (`web/node_modules`
  252 MiB, `webui` 185 MiB).
- 42 `node_modules` dirs across all worktrees: 10.1 GiB. Typical sizes are 166 MiB for
  `webui/node_modules` and 380 MiB for `web/node_modules`.
- The 55 remaining worktrees: 9.1 GiB. Most are active.
- APFS caveat. Both `web` and `webui` use pnpm (`pnpm-lock.yaml`), and gx runs
  `pnpm install --frozen-lockfile` (`ralphloop/install.go:39-40`). The pnpm store is 1.7 GiB at
  `~/Library/pnpm/store/v11`. On APFS, pnpm can clone files from the store. `du` then counts every
  clone in full, so the real gain is smaller than `du` says. Evidence: the concurrent cleanup removed
  worktrees with 2.7 GiB of `du`, but `df` free space rose only from 29 to 30 GiB. Other writers
  ran at the same time, so this is a rough signal, not a measurement.

### Other growth

| Item | Size | Cause |
|---|---|---|
| `$TMPDIR/gx-e2e-bin-*` | 28 GiB, 425 dirs of 67 MiB | `webui/e2e/global-setup.ts:19` builds `gx`, `claude` and `gxfixture` into a new temp dir per run. There is no global teardown. 174 dirs on 10-26 and 208 on 10-27. |
| `$TMPDIR/gx-claudeenv*` | 2.0 GiB, 176 dirs | `testutil/claudeenv/claudeenv.go:20` makes a temp `HOME`. Tests that run `go` there fill `$HOME/go/pkg/mod` and `$HOME/Library/Caches/go-build` (one dir: 119 MiB plus 306 MiB). Go makes module dirs read-only (`dr-xr-xr-x`), so the cleanup `os.RemoveAll(home)` (`claudeenv.go:46`) fails and nobody checks the error. Each test run also downloads the modules again. |
| `$TMPDIR/go-build*` | 0.9 GiB, 236 dirs | Go's own work dirs, left by killed builds. |
| `$TMPDIR` other | about 1 GiB | `node-compile-cache` 605 MiB, `gx-agentfake` 180 MiB, `playwright-transform-cache` 112 MiB, 3042 `gxs*` dirs (`server/servertest/servertest.go:440`) at 25 MiB total. |
| `$TMPDIR` total | 32 GiB, 17,153 entries | |
| `~/.npm` | 7.1 GiB | npm cache. Not from gx (gx uses pnpm). |
| `~/Library/Caches/ms-playwright` | 2.6 GiB | Browsers. |
| `~/.claude/projects` | 3.0 GiB | Claude Code transcripts. |
| `~/Library/pnpm/store/v11` | 1.7 GiB | pnpm store. |
| `~/go/pkg/mod` | 1.4 GiB | Go module cache. |
| `~/Library/Caches/go-build` | 1.2 GiB | Go build cache (already cleaned after the 60 GiB incident). |
| `~/.local/share/gx` | 328 MiB | `brief` 235 MiB (1081 logs, `brief/brief.go:94`, no pruning found), `tickets` 52 MiB, `sessions` 33 MiB. |
| `~/.local/state/gx` | 43 MiB | `agents` 30 MiB, logs 9 MiB. |

Volume at the time: 461 GiB, 431 to 433 GiB used, 29 to 30 GiB free (94%).

Clean fixes for the temp leaks:

- Playwright: add a `globalTeardown` that removes the bin dir. Better, build into a fixed
  per-worktree path (for example under the worktree's ignored build dir), so a killed run leaves
  at most one copy.
- `claudeenv.Isolate`: before it swaps `HOME`, read `go env GOCACHE GOMODCACHE` and set both env
  vars to the real paths. Tests stop downloading modules into the temp `HOME`, and the leftover
  becomes a few small files. Also run `chmod -R u+w` before `RemoveAll`, or use
  `go clean -modcache` with that `HOME`, and report the error.
- The disk watcher's per-project cleanup command could also sweep `$TMPDIR/gx-*` dirs older than a
  day. That is a backstop, not the fix.

## Open questions for a person

1. Should gx remove the feature worktree automatically after a merge, or only offer it in the web
   section? The research favors automatic removal with the checks in section 2.
2. For cherry-picked iteration branches: delete with `-D` after the server checks patch ids, or
   move them to the attic? The attic keeps commits but nothing prunes it
   (`cmd/cleanup_scan.go:41-43`).
3. Should the web section remove an unmerged, clean, idle worktree (branch kept)? It frees the
   most space, but the person loses the checkout and its ignored files.
4. Should the scan load archived epics and check patch ids against trunk, so it resolves the
   leftover iteration branches itself?
5. Should the disk watcher count `$TMPDIR` growth from gx tests? Today it is the largest single
   consumer.
