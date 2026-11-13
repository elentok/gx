# Movable clusters in `server/epics`, `server/sched`, `server/merge` and `server/servertest`

Ticket: `gx:split-server-wire/02`. Measured on the design branch at `31dd2aba6`, the way
`sizeguard/sizeguard_test.go` counts: tracked `.go` files of one directory (no recursion), all lines;
Test funcs are `^func Test[A-Z_]` in `_test.go` (`sizeguard_test.go:164-212`). Targets (90%): at most
45 files, 4,050 source lines, 4,500 test lines, 162 Test funcs.

Method: a throwaway `go/parser` script (kept in the session scratchpad, not committed) listed, per file, the
package-level symbols it uses from other files of the same package. It skips selector names and struct keys.
Method calls (`x.foo()`) are invisible to it, so every cluster below was re-checked by hand for unexported
fields and methods of its parent. `rg` gave the cross-package use counts.

## Summary

| Package | Above 90% on | Proposal | Result |
|---|---|---|---|
| `server/epics` | files (50), test lines (4,999) | Move 6 end-to-end test files to a new test-only package `server/epics/epicflows`. No source moves | 44 files, 3,669 test lines, 104 funcs. With the optional second group: 40 files, 2,934 lines, 78 funcs |
| `server/sched` | files (49) | Move 5 end-to-end test files to the existing test-only `server/sched/launchtest` | 44 files, 3,854 test lines, 94 funcs |
| `server/merge` | files (49), test lines (4,549) | Move the epic-merge and merge-ticket test group (7 files) to a new test-only `server/merge/mergeflows`. About 125 lines of shared test helpers must be copied (not a pure move). The source cannot move without a design change | 42 files, about 2,773 test lines, 79 funcs |
| `server/servertest` | source lines (4,493) | Move the designs, grilling, advancing, triage and prompt-session fixtures (5 files, 561 lines) into the existing sub-package `server/servertest/rigs` | 37 files, 3,932 source lines (under the 4,050 target) |

Cross-cutting facts:

- Test-only sub-packages are an established pattern here: `server/sched/launchtest` (22 files),
  `server/sessions/banners`, `askflows`, `server/sessions/prompt/promptflows`, `testutil/gxfixture/*flows`
  (`.ai/index.md:254`, `:310`). Each has its own `main_test.go` with the two `TestMain` hooks
  (`testmainguard`) and its own alias block for the `servertest` helpers.
- The end-to-end tests of `epics`, `sched` and `merge` are `package x_test` files that drive the whole server
  through `servertest.Harness`. They use only the exported API of the package, so they can sit in any package
  that does not create an import cycle. A test-only package imports its parent, so no cycle is possible.
- Source pressure is low in `epics`, `sched` and `merge` (71%, 70% and 72% of 4,500 lines). Only `servertest`
  needs a source move. In `epics`, `sched` and `merge` the pressure is on file count and test lines, which test
  moves fix with no change to any import in other packages.

## 1. `server/epics`

Current: 50 files, 3,206 source lines (71% of 4,500), 4,999 test lines (111% of the 4,500 target), 138 Test
funcs (85% of 162). Above 90%: files (50 vs 45) and test lines. Needs at least 5 fewer files and 499 fewer test
lines. Only `server/epics/prstate` exists as a child.

External use: 17 distinct `epics.X` symbols, about 20 uses, in 7 Go files outside the package (`epics.Epics` 3,
`Deps` 2, `BackgroundTask` 2, `StatePaused`, `StateOf`, `StateCycle`, `CycleReason`, `PausedReason`,
`PausedTickets`, `EpicStateFor`, `EpicMergeBlocked`, `RecentlyFinished`, `OpenSeen`, `NeedsYouCounts`,
`EpicSummaries`, `Routes`, `New`).

### Source: tangled core and leaf clusters

Almost every user-facing function is a method on `*Epics` (`deps.go:126`), and `Epics` embeds `Deps`
(`deps.go:81`, 11 interfaces and 9 callbacks). A file with a method on `*Epics` cannot move to a child unless the
method becomes a function (a refactor, not a move).

Tangled core (keep): `deps.go`, `detail.go`, `epiclist.go`, `epicdetail.go`, `epiccard.go`, `epicneeds.go`,
`oneoffcard.go`, `designcard.go`, `mappicker.go`, `epicpr.go`. They call each other in loops: `epiclist` ->
`designcard`, `epiccard`, `oneoffcard`; `designcard` -> `epiccard`, `epiclist.lastActivity`,
`epicneeds.PillQuestion`, `mappicker`; `epicdetail` -> `designcard`, `detail.actionInfo`; `epicsummary` ->
`epicdetail.mergeBlockActions`.

Source leaves. None is needed to hit the target, because the test moves below already do it. They are the
options if more room is wanted:

| Cluster | Files and lines | Outside use | Parent internals it needs |
|---|---|---|---|
| State rules | `state.go` 113 (+ `state_test.go` 67 lines, 2 funcs) | 6 external `epics.X` uses (`StateOf`, `StateCycle`, `StatePaused`, `CycleReason`, `PausedReason`, `PausedTickets`); used by several files inside the package | none (imports only `queue` and `tickets`) |
| Epic summary rules | `epicsummary.go:81-252` (about 172 lines: `answeredLatest`, `summarizeEpic`, `EpicStateFor`, `EpicMergeBlocked`, helpers) + `mergeBlockActions` (`epicdetail.go:19-27`) + `epicsummary_test.go` (394 lines, 8 funcs, internal) | 2 external uses (`EpicStateFor`, `EpicMergeBlocked`) | `summarizeEpic` and `mergeBlockActions` are unexported and used by the parent (`epicsummary.go:40`, `epicdetail.go:83`), so they need exporting. The `EpicSummaries`, `launchStep`, `capLine`, `needsApproval` methods (`epicsummary.go:18-80`) stay |
| Seen store | `seen.go:1-123` (`SeenStore`, `OpenSeen`, `seenStateOf`) + `seen_test.go` (95 lines, 5 funcs, internal) | 1 external use (`OpenSeen`); parent uses at `deps.go:100`, `epiclist.go:156` | `MarkSeen` (`seen.go:125-139`) is a method on `*Epics` and calls `cardList`; it stays in the parent (move it to `epiclist.go`) |
| Recently finished | `recentlyfinished.go` 76 (its test, `recentlyfinished_test.go`, is end-to-end) | 1 external use; `oneoffcard.go` uses `finishedOneOffs` | export `finishedOneOffs` |
| Map progress | `mapprogress.go` 117, no deps at all | none external; used by `designcard.go`, `epicdetail.go` | export `mapSteps`, `mapTicketFlags`, `designProgressSteps` |

State + summary + seen + recently-finished together would give 43 files and 4,289 test lines, but need about 8
renames to exported names and touch every `State*` use inside `epics`. They are optional.

### Tests (the proposal)

6 test files are internal (`deps_test`, `epiclist_state_test`, `epicpr_test`, `epicprstate_test`,
`epicsummary_test`, `seen_test`; 1,021 lines) and stay with the source. The other 24 are `package epics_test`
end-to-end files that use `servertest` and `rigs`.

Dependency groups among the external test files (helpers defined in one file and used by others):

- Epic detail group: `epicdetail_test.go` (514 lines, 17 funcs) defines `getEpicDetail`, `rowOf`,
  `rowAddresses`; `mapdetail_test.go` (134, 2) and `launchstep_test.go` (111, 3) use them;
  `joinedcard_test.go` (228, 4) uses `getEpicDetail` and defines `joinedMapMD`; `workspaces_test.go` (208, 5)
  uses `joinedMapMD` and defines `startWorkspaceRig`, `needsYou`; `followupcount_test.go` (135, 3) uses
  `startWorkspaceRig`. Closed set: 6 files, 1,330 lines, 34 funcs. Nothing outside it uses a helper defined in
  it (checked with `rg`). It uses `sharedhelpers_test.go` only for `repairToken`, `readMeta`, `report`,
  `waitForSession`, `writeAssistantMessage`, and the exported `epics.PillQuestion` (`joinedcard_test.go:154`).
- Ticket detail group: `detail_test.go` (261, 9), `detailerror_test.go` (94, 3), `detailpayload_test.go`
  (167, 5), `detailruns_test.go` (213, 9; uses `fetchDetail`, `withRepo` from `detailpayload_test.go`). Closed
  set: 4 files, 735 lines, 26 funcs. Uses `sharedhelpers` for `answerAddr`, `optionsBlock`,
  `pauseWithQuestion`, `startWaitingRestartFake` and one more.
- Everything else is single files that use only `sharedhelpers_test.go`.

New package `server/epics/epicflows` (test-only): the 6-file group, its own `main_test.go` (20 lines, the two
hooks) and a short alias file for the 5 helpers.

- Required move (epic detail group): `server/epics` ends at 44 files (88% of 50; under the 45 target), 3,669
  test lines (under 4,500), 104 Test funcs, 3,206 source lines unchanged. `epicflows` gets 8 files, about
  1,365 test lines, 35 funcs.
- Optional second group (ticket detail): `server/epics` ends at 40 files, 2,934 test lines, 78 funcs.
  `epicflows` (or a second package `ticketflows`) then holds about 2,100 test lines, 12 files.
- No Go file outside `server/epics` changes. No import in `server/epics` changes. No tygo or `wire` change.
- Risk: the new package needs its own `TestMain` (the `testmainguard` scan), and the alias block is partly
  copied (5 names, about 15 lines) unless the tests call `servertest.X` directly.

Possible duplicate tests (judged by name and a read of two; not proven redundant):

- `TestPausedReasonNamesPausedTickets` (`state_test.go`) and the first half of `TestPausedAndCycleReason`
  (`epicsummary_test.go:360`) both assert `PausedReason`.
- `TestStateOf` (`state_test.go`) and `TestSummarizeEpic_DeadlockedAgreesWithStateOf` overlap on the state rules.
- `TestEpicDetail_MapProgress` (`mapdetail_test.go:14`), `TestEpicDetail_MapRingProgress`
  (`epicdetail_test.go:260`) and `TestEpicList_MapProgress` (`mapprogress_test.go`) all read map progress
  through the server.

## 2. `server/sched`

Current: 49 files, 3,133 source lines (70%), 4,307 test lines (under the 4,500 target), 107 Test funcs.
Above 90%: files only (49 vs 45). Needs at least 4 fewer files. Existing child: `launchtest` (22 files, 2,301
test lines, 53 funcs).

External use: 20 distinct `sched.X` symbols, mostly one use each, in the root server wiring (`sched.Scheduler`
3, `Explainer` 3, `DefaultProgressInterval` 3).

### Source

Tangled core (keep): `scheduler.go`, `claim.go`, `run.go`, `launch.go`, `deadlock.go`, `gatewatch.go`,
`finishedclaims.go`, `baseref.go`, `explainer.go`, `verdicts.go`, `pending.go`, `iterations.go`. Almost all are
methods on `*Scheduler`, `*Launcher` or `*Explainer`, and `scheduler.go` builds `Explainer`, `Launcher` and
`HerdrWatch`.

Leaf clusters (use no other file of the package):

| Cluster | Files and lines | Who uses it | Parent internals needed |
|---|---|---|---|
| Herdr watch | `herdrwatch.go` 196 + `herdrwatch_test.go` 103 (3 funcs, internal) | `explainer.go`, `scheduler.go`; 3 external `sched.` uses (`HerdrWatch`, `HerdrHost`, `NewHerdrWatch`) | none |
| Reclaimer | `reclaim.go` 385 (no test file of its own; its flows are end-to-end) | 3 external `sched.` uses (`Reclaimer`, `ReclaimHost`, `NewReclaimer`) | none |
| Progress timer | `progress.go` 158 (+ end-to-end `progress_test.go`, 24 lines) | `launch.go` (`ProgressTimer`); about 6 external uses | none |
| Pause marker | `pausemarker.go` 29 | `run.go` (`markedIterationReason`) | export one function |

Herdr watch + Reclaimer + Progress (4 files) reach exactly 45 files (no headroom) and change about 10 external
call sites. Not needed if the test move is chosen.

### Tests (the proposal)

Only 3 test files are internal (`epicmerge_cache_test`, `herdrwatch_test`, `launch_internal_test`; 257 lines).
The other 28 are end-to-end. The reclaim and holder flows belong with the 4 `reclaim_*` files that already sit
in `launchtest`.

Closed group (uses nothing from other test files except `sharedhelpers_test.go`; nothing else uses it):
`holder_dead_test.go` (85 lines, 2 funcs), `holder_readopt_test.go` (107, 2), `holder_retry_test.go` (57, 2;
uses `readoptLabel`, `startUnreachable` from `holder_readopt`), `reattach_test.go` (104, 3),
`headless_block_reclaim_test.go` (100, 4). Total 453 lines, 13 funcs. `holder_readopt_test.go:17,100` imports
`sched` for `sched.HolderReadoptCap`, which is exported, so the move works.

- `server/sched`: 44 files (under 45), 3,854 test lines, 94 funcs, source unchanged.
- `launchtest`: 27 files, about 2,770 test lines, 66 funcs. Its alias file (`launchtest/sharedhelpers_test.go`,
  45 lines) lacks `assertPaused`, `chatRecorder`, `markWaiting`, `pauseBlocked`, `waitForChat`; add them (about
  10 lines).
- No Go file outside `server/sched` changes.

## 3. `server/merge`

Current: 49 files (21 source, 28 test), 3,227 source lines (72%), 4,549 test lines (above the 4,500 target), 135
Test funcs. Above 90%: files and test lines. Needs at least 4 fewer files and 49 fewer test lines. No children.

External use of `merge.X` is small: `New`, `Deps`, `Host`, `Routes`, `Merger`, `Reconciler`, `Landing` and
`IterationMode` from the root wiring.

### Source: three types that point at each other

`Merger` (`merger.go`), `Reconciler` (`reconciler.go`) and `Landing` (`dirtyland.go`) are built together in `New`
(`deps.go`) and hold pointers to each other: `Merger.rec`, `Merger.landing`, `Reconciler.merger`,
`Landing.merger`. A child package cannot hold any of them without the parent importing the child and the child
importing the parent.

| Cluster | Files and lines | Parent internals it needs (the cycle) |
|---|---|---|
| Reconciler | `reconciler.go` 60, `handmerged.go` 103, `latefinish.go` 299, `missingtrunk.go` 145, `parentdefect.go` 68, `worktree_removal.go` 152 (827 lines, 6 files) | calls `Merger` methods `EpicRefs`, `CompleteRootIfDone` (exported) and unexported `notifyEpicOutcome`, `stampMergedEpic`, `StampEpic` (`handmerged.go:63,74,90,91`, `latefinish.go:267`, `worktree_removal.go:56`); uses `mergeLockKey` (`epicmerge_root.go`). `Merger` calls back `rec.RemoveMergedWorktree` and `rec.enterTrunkMissing` (`epicmerge_root.go:379,534`) |
| Landing | `dirtyland.go` 134, `stashmerge.go` 203, `trunkconflict_finish.go` 73, `trunkconflict_startover.go` 53 (463 lines, 4 files) | calls unexported `Merger` methods `loadMergeEpic`, `actOnMerge`, `retryTrunkUpdate`, `epicMergeLocked`, `clearMergeBlock`, `ensureRootQueued` (`stashmerge.go:25,49,64,144,168,189`) and the helpers `listFiles`, `clashingBetween`, `mergeDecision`, `checkoutDirFor`. `Merger` calls `landing.landOverDirtyTrunk` (`epicmerge_root.go:507`); `iterationmode.go` uses `Landing` and `filesWithMarkers` |
| Merger core | `merger.go`, `epicmerge.go` 398, `epicmerge_root.go` 585, `mergefork.go`, `markmerged.go`, `mergerecover.go`, `prtrunk.go`, `iterationmode.go`, `deps.go`, `seams.go`, `routes.go` | tangled core, stays |

Moving the Reconciler or Landing cluster to a child is correct only with a seam: an interface in the child for the
5 to 8 `Merger` methods, and an interface in the parent for `rec` and `landing`. That is a refactor, not a move.
It is not needed for the target, and a hack move would not compile. Say so in the follow-up grilling.

### Tests

The end-to-end suite is one connected blob. 4 files are internal (`ensure_worktree_test` 41/1,
`epicmerge_repo_test` 50/2, `hostfakes_test` 64/3, `mergefork_internal_test` 49/2; 204 lines, 8 funcs). The other
24 files (4,345 lines, 127 funcs, with `main_test` and `sharedhelpers_test`) are `merge_test`. They share small
helper methods on the test `schedFixture` wrapper (`sharedhelpers_test.go`), defined across 9 other files:

| Helper | Defined in |
|---|---|
| `pauseRoot` (24 lines), `assertRootQueued` (15), `commitFiles` (12) | `mergeerror_test.go:20,44,59` |
| `epicMD` (10), `assertStamps` (42), `finishCommitless` (18) | `mergeroot_test.go:121,131,173` |
| `rev` (5), `runEpic` (18) | `trunk_merging_test.go:23,28` |
| `settle` (9), `assertWorktreeRemoved`, `holdWorktreeLock` | `worktree_removal_test.go:48,...` |
| `read`, `backupRefs` | `dirtyland_test.go:44,53` |

Options:

- A, pure move: move all 24 end-to-end files into a new test-only `server/merge/mergeflows`. No helper is
  copied. `server/merge` ends at 25 files, 204 test lines, 8 funcs. But `mergeflows` ends at 4,345 test lines
  (under the 4,500 target by 3%, and 87% of the real 5,000 limit). It meets the target and leaves no room. Not
  recommended alone.
- B, recommended: split along the merge-verb seam. Group G2 is closed except for helper inputs: `epicmerge_test`
  (625, 20), `mergefork_test` (279, 5), `merge_ticket_test` (191, 4), `merge_finish_test` (207, 9),
  `frictionstamp_test` (36, 1), `worktree_removal_test` (270, 10), `worktree_removal_live_test` (177, 7). That is
  1,785 lines, 56 funcs, 7 files. Nothing outside G2 uses G2's definitions except `settle` (used by
  `latefinish_test` and `trunklock_test`). G2 uses 6 hub helpers from the other group: `pauseRoot`,
  `assertRootQueued`, `epicMD`, `assertStamps`, `finishCommitless`, `rev` (about 115 lines). Those must be
  copied into `mergeflows` (with the `sharedhelpers` wrapper, 75 lines), and `settle` (9 lines) copied back.
  This is not a pure move; the duplication is the price. Alternative: export the helpers from a small non-test
  helper package. That is a bigger design step.

Result of B: `server/merge` 42 files, about 2,773 test lines, 79 funcs, source unchanged. `mergeflows` about 9
files (7 + `main_test` + helpers), about 2,000 test lines, about 57 funcs. No Go file outside `server/merge`
changes.

Possible duplicate tests (by name only, not read in full):

- `TestHandMerged_FrictionIncludesACancelledTicket` and `TestEpicMerge_FrictionIncludesACancelledTicket`.
- `TestHandMerged_StampsSummedTotalsFromTheEpicsTickets` and
  `TestEpicMerge_StampsSummedTotalsWhenEveryDoneTicketIsStamped`.
- `TestMergeFinish_TrunkMovedPastTheClaimTipStillFinishesDone` and
  `TestLateFinish_TrunkMovedPastTheClaimTipStillClosesDone`.
- `TestMergeFork_NonFFForksAMergeTicketWhoseFinishMergesTheEpic` and
  `TestEpicMerge_NonFFForksAManualTicketWhoseFinishMergesTheEpic`.
- `TestEpicMerge_DirtyTrunkPausesTheRootAndLeavesItQueued`, `TestMergeError_DirtyTrunkPausesTheRootWithACappedFileList`
  and `TestMergeFork_DirtyTrunkAfterAMergeTicketPausesWithoutAnother` all touch the dirty-trunk pause.

## 4. `server/servertest`

Current: 42 files (38 source, 4 test), 4,493 source lines (111% of the 4,050 target), 797 test lines, 32 Test
funcs. Above 90%: source lines only. Needs at least 443 fewer source lines. Existing child:
`server/servertest/rigs` (27 files, 1,738 lines). `rigs` imports `servertest` and `server`; nothing in
`servertest` imports `rigs`.

External use: `servertest.X` is used everywhere (`store.go` symbols 1,042 uses in 252 files, `wait.go` 321 in
120, `projects.go` 256 in 144). Test packages mostly reach it through alias blocks (`sharedhelpers_test.go`), so
the real edit count for a moved file is the alias lines plus direct uses.

### Tangled core (keep)

`servertest.go` (the `Harness` type), `start.go`, `turn.go`, `store.go`, `sessions.go`, `wait.go`, `runner.go`,
`web.go`, `projects.go`, `ptyrunner.go` + `leftover.go` (they use unexported `holderArg`,
`requireSessionProcesses`, `stopSessionHolders`, `newTurn`, `ptyRunner` and add methods on `Harness`),
`fakedisks.go`, `fakeprocesses.go`, `fakesystem.go`, `notify.go`, `programs.go` (fields of `Harness`, built in
`servertest.go` and `start.go`; `fakedisks.go` uses `WaitFor` from `wait.go`, which would be a cycle if it
moved). `Harness` has unexported fields (`cfg`, `herdrDown`, `starts`, ...), so every method on `Harness`
stays.

### Leaf clusters

Cluster M1, recommended: the session fixtures.

- `designs.go` 153 (`DesignsFixture`, `PromptFixture`, `StartDesigns*`, `StartPromptSession*`, `RepairToken`),
  `grilling.go` 129 (`GrillingFixture`, `StartGrillingSessionIn`, `EndTurn`, `StoreWithMapStep`,
  `WriteAssistantMessage`, ...), `advancing.go` 52 (`StartAdvancing`, `CountdownDelay`), `triagesession.go` 47,
  `promptsession.go` 180. Total 561 lines, 5 files.
- It uses only exported `servertest` symbols (`Harness`, the `With*` options, `Report`, `WaitForSession`,
  `StartWithStore`, `WriteTicket`, `Login`, `JSONPost`, `ReadMeta`, `WithAlerts`, `WaitForScreen`,
  `FakeCodexDir`). No method on `Harness` and no unexported field is used (checked with `rg`).
- Nothing else in `servertest` uses it. No name collides with an existing `rigs` name.
- Outside use: 223 `servertest.X` uses of 27 symbols in 46 files (`server/sessions/prompt` 93, `server/sessions`
  39, `server/triage` 21, `server` 11, `server/epics` 10, `promptflows` 10, `server/workspaces` 10, others under
  6). Most go through alias blocks (`server/epics/sharedhelpers_test.go`, `server/sessions/helpers_test.go`,
  `server/sessions/prompt/helpers_test.go`, `server/sharedhelpers_test.go`, ...).
- Result: `servertest` 37 files, 3,932 source lines (under the 4,050 target); `rigs` 32 files, about 2,299 source
  lines.
- Overlap: `server/sessions`, `server/sessions/prompt` and `server` are in ticket 01's packages. Their alias
  blocks change here and again if that ticket moves tests. Order the work.

Cluster M2, optional (headroom): `pausedrun.go` 201, `headlessfake.go` 99, `schedfixture.go` 180,
`mergehelpers.go` about 106 of its 125 lines (about 586 lines, 4 files). `mergehelpers.go:85` is a method on
`Harness` that reads `h.cfg` (unexported), so that method (`PutRunIn`, about 19 lines) must stay in the parent.
About 232 outside uses in about 45 files. With M1 and M2: 33 files, about 3,346 source lines.

Not an option: `ptyrunner_test.go` (600 lines) is internal (`ptyRunner`, `ptyPollInterval`), so it stays with
`ptyrunner.go`. Test lines are only 797, so there is no pressure.

## Blockers and open questions for the decide ticket (`08`)

- `server/merge`: no source move is possible without a design change (interfaces between `Merger`,
  `Reconciler`, `Landing`). Option B for tests copies about 125 lines of helpers. Decide copy vs shared helper
  package.
- `server/epics`, `server/sched`: the test-only package convention already exists. Confirm the names
  (`epicflows`; reuse `launchtest`).
- `server/servertest` M1 changes alias blocks in `server/sessions`, `server/sessions/prompt` and `server`, which
  are in ticket 01's scope.
- tygo and `generated/server.ts` are not touched by any move here (no `wire` type moves).
