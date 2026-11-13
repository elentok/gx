# Movable clusters in `server`, `server/sessions` and `server/sessions/prompt`

Ticket: `gx:split-server-wire/01`. Design branch at `31dd2aba6`. Facts first, then a proposal. The person decides in ticket 07.

Method: counted like `sizeguard/sizeguard_test.go` (tracked `.go` files directly in the directory, all lines, `^func Test[A-Z_]` in `_test.go`). Limits at 90%: 45 files, 4,050 source lines, 4,500 test lines, 162 Test funcs. Symbol dependencies came from a declaration/use scan of each file (a script, then spot checks of every claim that matters). The scan over-reports, because exported names such as `State` or `Prompt` match by name; every hit listed as a blocker below was read by hand.

## Summary

| Package | Files | Source | Test lines | Test funcs | Above 90% now | Proposal reaches |
|---|---|---|---|---|---|---|
| `server` | 50 | 3,077 | 3,281 | 97 | files | 45 files (or 42 with extras) |
| `server/sessions` | 50 | 3,383 | 4,999 | 168 | files, test lines, Test funcs | 45 files, 4,356 test lines, 154 funcs (or 42 / 4,015 / 141) |
| `server/sessions/prompt` | 50 | 4,040 | 4,923 | 177 | files, test lines, Test funcs | 43 files, 4,040 source, 3,891 test lines, 143 funcs (or 42 / 3,773 / 138) |

Key finding for all three: the source code is a tangled core and nothing in it moves cleanly. The over-limit measures are files, test lines and Test funcs, and all of them are fixed by moving external-test files (`package X_test`) into new test-only sub-packages. That has precedent in this tree: `server/sessions/banners` ("test-only package: it has no non-test code", `.ai/index.md` line 254), `server/sessions/askflows`, and `server/sessions/prompt/promptflows`. No import cycle is possible, because the moved files only import `server`, `servertest`, `wire` and the parent package's exported API, and nothing imports a `_test` package. No `pkg.X` use outside the package changes (the moved files declare no exported symbols).

Costs shared by every new test-only package:

- Each needs a `TestMain` with `servertest.RunSessionProcesses()` and `herdrfake.RunHelperProcess()`. Without both, the servertest harness re-runs the suite in each child and fork-storms (the comment in `server/sessions/askflows/helpers_test.go:10-12`, `server/sessions/prompt/main_test.go:11-13`). Use the capped test runner (`scripts/proc-cap`) for the first run.
- Each needs its own small `helpers_test.go` with the aliases the moved files use (`var waitFor = servertest.Await`, and so on). The alias blocks are in `server/sessions/helpers_test.go:30-80` and `server/sessions/prompt/helpers_test.go:14-41`. They are one-line aliases to `servertest` and `rigs`, so this is copying, not new logic.
- `sleepguard/sleepguard_test.go` keys its allowlist by test file path. Moved files need their keys renamed: `server/devmode_failure_test.go` (line 44) and five `server/sessions/prompt/*` entries (lines 125-129).
- `.ai/index.md` should list each new package next to `banners` (line 254).

## `server`

### Measures

50 files (100%), 3,077 source lines (68% of 4,500; 76% of the 4,050 target), 3,281 test lines (73%), 97 Test funcs (54%). Only the file count is above 90%. 30 source files, 20 test files.

### Tangled core (stays)

`server.go` (824 lines), `config.go`, `harnessaccess.go`, `host.go`, `propen.go`, `prwatch.go`, `projectsinit.go`, `lifecyclewire.go`, `sessionwire.go`, `sessioninfo.go`, `herdrwire.go`, `runs.go`, `webwire.go`, `routes.go`, `rootroutes.go`, `tokenhost.go`, `workspacesdeps.go`, and the host adapters `ticketopswire.go`, `ticketwritewire.go`, `ticketwrites.go`, `budgethost.go`, `recoverywire.go`, `epicsdeps.go`, `designsdeps.go`, `explainwire.go`, `launchwire.go`, `archivewire.go`, `devmode.go`.

Why no source cluster moves: the package is the wiring hub. Every `*wire.go` or `*deps.go` file is an adapter that takes `s *Server` and reads its unexported fields (`s.herdr`, `s.events`, `s.chat`, `s.writer`, `s.cfg`, for example `herdrwire.go:14-33`, `ticketwritewire.go:31-95`, `budgethost.go:12`, `recoverywire.go:20`). The sub-packages they feed (`ticketwrite`, `recovery`, `sched`, `epics`, `designs`) are already imported by `server`. A child holding an adapter would need `*Server` and its fields, so `server` would import the child and the child would import `server`: a cycle. Making it work means exporting the `Server` fields or inverting each adapter into a host interface, which is a refactor and not a move. Not needed: source is at 68% and files are fixed by tests.

Small leaves, not worth a package: `browsercheck.go` (49 lines, uses nothing, one caller `server.go`), `settingswriter.go` (13). Moving them would need exporting `refuseBrowsers` / `newSettingsWriter` and gains 2 files at the price of 2 new exported names.

### Test clusters

The 20 test files split by package name: 13 external (`server_test`) and 7 internal (`server`).

Internal (`package server`, need unexported symbols, stay): `configreload_chat_test.go`, `logfile_test.go`, `routes_internal_test.go`, `runner_route_test.go`, `server_notify_test.go`, `snapshot_budget_test.go`, `snapshot_context_test.go`.

External, movable. The only cross-file helper links among them: `sharedhelpers_test.go` (aliases such as `reload`, `reach`, `ofType`), `server_test.go` (`testFlushInterval`) and the dev-mode group among themselves.

| Cluster | Files | Lines | Test funcs | Needs from the rest |
|---|---|---|---|---|
| Dev mode: `devmode_test.go` (528, 19), `devmode_events_test.go` (173, 7), `devmode_failure_test.go` (137, 4), `devsnapshot_test.go` (66, 2) | 4 | 904 | 32 | `devsnapshot_test.go` uses the `reload` alias (`sharedhelpers_test.go:107`, one line: `servertest.Reload`). The four files use each other (`startDevRig`, `devRig`, `devSnapshot`, `devConfig`), so they move together. Imports: `server`, `servertest`, `devvite/vitetest`, `wire`. |
| `events_heartbeat_test.go` | 1 | 72 | 2 | nothing |
| `servertest_runner_options_test.go` | 1 | 100 | 3 | nothing (its `launchOnRunners` / `startRunnerOptions` have no other user) |
| `servertest_parallel_test.go` | 1 | 47 | 1 | nothing |
| `unavailable_test.go` | 1 | 90 | 1 | `testFlushInterval` (`server_test.go:26`, one alias line) |
| `live_relaunch_test.go` | 1 | 112 | 5 | five `sharedhelpers_test.go` aliases |
| `prune_route_test.go` | 1 | 88 | 1 | `newRenamedFake` (`sharedhelpers_test.go`) |

Dead code: `designsfixture_test.go` (44 lines, 0 Test funcs) defines `startDesigns`, `designsFixture`, `sessionSnapshotOf`, which no file in `server` calls (`rg` finds the same names only in `server/epics` and `server/sessions`). Deleting it frees a file slot. That is a deletion, not a move.

Possible overlap in the dev-mode cluster (names only, bodies not diffed): `TestDevMode_KeyFlipSwapsTheUIWithoutRestart` and `TestWebDevProxy_SwapsTheUIWithoutRestartingTheListener`; `TestDevMode_ReloadAtReadyAndAtOff`, `TestDevMode_ReloadAtReadyAfterDevDirChange` and `TestDevMode_ReloadAtReadyAfterRestartVite`. No confirmed duplicate. Test lines and funcs are far below the limits, so none needs trimming.

### Proposed move set

New test-only package (suggested name `server/devmodeflows`; ticket 06/07 names it): the four dev-mode files plus `events_heartbeat_test.go`. 5 files, 976 lines, 34 funcs. Needs `TestMain`, a `reload` alias, and the sleepguard key rename.

Result: 45 files (90%), 3,077 source, 2,305 test lines, 63 funcs. The file count lands exactly on 90%, so any new file in `server` breaks the target. For headroom also move `servertest_runner_options_test.go`, `unavailable_test.go` (copy `testFlushInterval`) and `servertest_parallel_test.go`: 8 files, 1,213 lines, 39 funcs, giving 42 files (84%), 2,068 test lines, 58 funcs. Those three exercise the `servertest` harness, so ticket 08 (which owns `server/servertest`) may prefer to host them.

Conflict with ticket 02: it also decides what goes into or beside `server/servertest`. Check that it does not take these files.

## `server/sessions`

### Measures

50 files (100%), 3,383 source (84% of 4,050; 75% of 4,500), 4,999 test lines (100%), 168 Test funcs (93%). Above 90%: files, test lines, Test funcs. 16 source files, 34 test files. Sub-packages already exist: `askflows` (7 files), `banners` (9), `chat` (28), `convrecord` (2), `fixagent` (5), `prompt` (59), `prompt/promptflows` (9).

### Tangled core (stays)

`sessions.go` (687, the `Sessions` type), `sessiontypes.go`, `sessionmanager.go`, `sessionwatch.go` (409), `sessionbanner.go` (388), `sessioninfo.go`, `sessionlink.go`, `sessionfocus.go`, `sessionhandoff.go` (`Resumer`), `sessionresume.go`, `sessionstate.go`, `askhold.go`, `agentlogs.go`, `sessionwebcreate.go`, `targets.go`, `routes.go`. They are mutually dependent through `SessionManager`/`Sessions`/`SessionWatch` and its unexported state. Source is at 84% of the 90% target, so nothing needs to leave. Examples of why none moves cleanly: `agentlogs.go` (104) is used only by `sessionmanager.go`, but reads `sessiontypes.go` and `sessionwatch.go` internals; `askhold.go` (158) is read by `sessionwatch.go`, `sessionmanager.go` and `routes.go` and has an internal test (`askhold_internal_test.go`).

### Test clusters

Internal tests (`package sessions`, stay): `askhold_internal_test.go`, `openonscreen_internal_test.go`, `proccap_internal_test.go`, `sessionlaunchargs_test.go`, `sessionmeta_internal_test.go`.

External tests (`package sessions_test`) share one helper file: `helpers_test.go` (138 lines, `TestMain`, the alias blocks and a `designsFixture` wrapper) and `sharedhelpers_test.go`. Almost every external file depends only on those aliases.

| Cluster | Files | Lines | Test funcs | Needs from the rest |
|---|---|---|---|---|
| Conversations: `sessions_conversations_test.go` (153, 5), `sessions_conversations_grilling_test.go` (115, 2), `conversationsweep_test.go` (65, 1) | 3 | 333 | 8 | `helpers_test.go` aliases only: `startFakeSession[With]`, `stopFakeSession`, `readMeta`, `report`, `waitForSession`. They share `recordsByID`, `clearFakeSession`, `writeGrillingRoundsAt`, `designMapStore` among themselves. |
| Handoff: `sessionhandoff_test.go` (263, 6) | 1 | 263 | 6 | uses `recordsByID` and `writeTranscriptTurns` from the conversations files, so it moves with them. Also needs the `designsFixture` wrapper (`helpers_test.go:88-121`, about 35 lines) and ten aliases. Reaches the production code only through exported API (`server.ResumerOf(h.Server).RestartFresh`, `sessionhandoff_test.go:195`). |
| `sessionadvancing_helpers_test.go` (47, 0) | 1 | 47 | 0 | Dead: its `grillingFixture`, `startAdvancing`, `endTurn`, `autoSends`, `sessionSnapshot`, `countdownDelay` have no caller in this package (the same names live on in `prompt/helpers_test.go:14-34`). Delete or move. |
| Permission and screens: `sessionpermission_test.go` (114, 5), `sessionscreen_test.go` (60, 3), `ticketscreen_test.go` (167, 5) | 3 | 341 | 13 | `sessionscreen_test.go`: four aliases. The other two: none. |
| `sessioncommands_test.go` | 1 | 323 | 12 | `designsFixture`, `startDesigns` |
| Restore group: `sessions_restore_test.go` (179, 5), `sessions_failstart_test.go` (110, 2), `sessions_unread_test.go` (155, 4), `sharedhelpers_test.go` (82, 2), `retry_prompt_test.go` (105, 5), `sessionchatwatch_test.go` (134, 5) | 6 | 765 | 23 | chained through `sharedhelpers_test.go` (`markRead`, `waitForSnapshot`, `killWhileDown`), and `sessions_restore_test.go` needs `killHolder` from `sessions_test.go:580`. More tangled; not needed. |
| Others, small and self-contained: `launch_failure_retry_test.go` (46, 1), `sessionautoadvance_rule_test.go` (46, 1), `sessionprompter_test.go` (43, 1), `targets_test.go` (42, 1) | 4 | 177 | 4 | none or aliases only |

Not movable cheaply: `sessions_test.go` (742 lines, 30 funcs) is the biggest file. Other files call its `killHolder`, and it uses 14 aliases and `designsFixture`. Splitting it is a file split, which does not help for package totals, but moving it whole would take the most lines in one step if wanted.

Tests that look related (not duplicates; same topic, different layer): `sessioninfo_test.go` has `TestRunEndLine_MatchesAgentfakeDesignRunEnd` and `TestContinueAction_UsesAgentfakeDesignPromptPrefix`, close to `prompt/design_detector_contract_internal_test.go` (`TestAgentfakeDesignRunEnd_HasTemplateShape`, `TestGxDesignRunEndTemplate_IsContinue`) and `chat/actionable_test.go` (`TestContinueAction_ByFlowAndAgent`). `TestLinkSession_TriageSetsFlowAndKey` (`sessionhandoff_test.go`) sits beside `banners/sessions_link_test.go` (`TestLinkSession_MarksMapTicketsDesign`). `askflows/permission_card_test.go` (`TestPermissionCard_*`) and `prompt/permissioncard_test.go` (`TestPermissionCard_*`) share a name prefix. `sessions_test.go` has `TestFirstPrompt_WaitsOutBlockedThenTypedAtIdle` and `TestFirstPrompt_DroppedOnExit` next to the `prompt/firstprompt_test.go` family. I did not diff bodies, so none is confirmed redundant.

### Proposed move set

New test-only package (suggested `server/sessions/convflows`): conversations trio, `sessionhandoff_test.go`, and `sessionadvancing_helpers_test.go` (move it, or delete it as dead code). 5 files, 643 lines, 14 funcs. The package needs `TestMain`, about 35 lines of `designsFixture` wrapper plus aliases in its `helpers_test.go`. Result: 45 files (90%), 3,383 source, 4,356 test lines (87%), 154 funcs (86%).

For headroom, add a second package (or fold it in) with the permission and screens trio: 3 files, 341 lines, 13 funcs. Result: 42 files (84%), 4,015 test lines (80%), 141 funcs (78%).

## `server/sessions/prompt`

### Measures

50 files (100%), 4,040 source (99.8% of the 4,050 target; 90% of limit), 4,923 test lines (98%), 177 Test funcs (98%). Above 90%: files, test lines, Test funcs. Source is under the target by 10 lines only. 20 source files, 30 test files.

### Tangled core (stays)

All source. Every file hangs on the `Prompt` struct in `prompt.go`: methods on `*Prompt` in `action.go`, `actionable.go`, `autoadvance.go`, `claudedialog.go`, `claudekeys.go`, `codexquestion.go`, `firstprompt.go`, `keys.go`, `send.go`, `sessionanswer.go`, `sessionprompt.go`, `typing.go`. The package already uses embedded halves to dodge method-count limits: `imageStore struct{ *Prompt }` (`sessionimages.go:28-31`, embedded at `prompt.go:112,149`) and `turnSettler` (`turnsettle.go:18`). A child package cannot hold these, since it would need `*Prompt`'s unexported state while `prompt` imports it: a cycle. `autoqueue.go` / `autoqueue_countdown.go` (234 lines) look standalone but use `autoQueuer` in `prompt.go`, `begin`, `deadline` and `finish` in `autoadvance.go`, and `startclock.go`'s `Stop`. Moving source out would need `Prompt` split into exported seams first. Not needed for the target, but source has 10 lines of room, so the next 10 lines of source growth in this package fail the 90% rule. Say so in the decision ticket.

### Test clusters

Internal tests (`package prompt`, stay): `actionable_internal_test.go`, `design_detector_contract_internal_test.go` (uses `designRef` from the first), `send_internal_test.go`.

External tests: `helpers_test.go` (83 lines, aliases and `designsFixture`), `main_test.go` (`TestMain`), and `sessionprompt_test.go` (360 lines, 18 funcs) which defines `promptFixture`, `startPromptSession[WithGrace]` and `fromPrompt`. The keys, cards and first-prompt files lean on `sessionprompt_test.go`, so they are the hard-to-move part.

Clean clusters, each depending only on `helpers_test.go` aliases and on each other:

| Cluster | Files | Lines | Test funcs | Notes |
|---|---|---|---|---|
| Auto-queue: `autoqueue_test.go` (82, 3), `autoqueue_countdown_test.go` (269, 8), `autoqueue_ready_test.go` (143, 3), `autoqueue_trigger_test.go` (95, 5) | 4 | 589 | 19 | share `mapCase`, `writeMapCase`, `queued`, `startReadyMap`, `setAutoQueue` among themselves; aliases `startWebDesign`, `webSessionWrite`, `sessionToken`, `startGrillingSessionIn`, `storeWithMapStep`, `endTurn`, `writeAssistantMessage`, `runEndText` |
| Auto-advance: `autoadvance_test.go` (81, 3), `autoadvance_countdown_test.go` (272, 9), `autoadvance_guard_test.go` (90, 3) | 3 | 443 | 15 | aliases: `grillingFixture`, `startAdvancing`, `endTurn`, `autoSends`, `sessionSnapshot`, `countdownDelay`, `clearByInput`, `waitUntil`, and others |
| `triage_advance_test.go` | 1 | 118 | 5 | aliases only |

Verified by hand: the only lowercase name that looked shared with a staying file is a local variable `prompt := f.H.Server.Prompt()` (`autoqueue_countdown_test.go:213`), not `promptFixture.prompt` (`sessionprompt_test.go:48`). No staying file uses a symbol these files declare. The exported names the scan flagged (`State`, `Prompt`, `QueueEpic`, `AutoQueueNotReady`, `NewestDesignSession`, `DesignReachedBranch`, `CancelAutoAdvance`) are exported, so an external test package can use them.

Harder clusters (need a copy of `sessionprompt_test.go`'s fixture, so not proposed): keys and cards (`keys_test.go` 334/15, `keys_permission_test.go` 191/8, `keys_permission_codex_test.go` 95/3, `permissioncard_test.go` 116/5, `plancard_test.go` 126/7, `codexpermissioncard_test.go` 88/4, `questionlist_test.go` 76/2) and first prompt (`firstprompt_test.go` 181/10, `firstprompt_failure_test.go` 131/4, `firstprompt_launch_test.go` 250/4, `keys_settle_test.go` 168/3). If they are wanted later, move `promptFixture` and `startPromptSession*` with them into a shared `helpers_test.go` in the new package.

Duplicates: no confirmed duplicate. Mirror pairs that look alike but cover different switches: `TestSessionAutoAdvance_RefusesAPlainAndAnUnknownSession` / `TestSessionAutoQueue_RefusesAPlainAndAnUnknownSession`, `TestWebSessions_AutoAdvanceNeedsAFreshTokenOnADesignSession` / `TestWebSessions_AutoQueueNeedsAFreshTokenOnADesignSession`. Image tests overlap in topic across `promptflows` (`TestSessionPromptImages_PastesEachImageThenTheText`, `TestSessionAction_ReplayPastesEachImageThenTheText`) and `TestSessionPrompt_CodexPastesImagesBeforeTheText` (`promptflows/sessionprompt_codex_test.go`); these are already in `promptflows` and need no move.

### Proposed move set

New test-only package (suggested `server/sessions/prompt/autoflows`, next to `promptflows`): auto-queue (4 files) and auto-advance (3 files). 7 files, 1,032 lines, 34 funcs. Needs `TestMain` and the aliases above. Result: 43 files (86%), 4,040 source (unchanged), 3,891 test lines (78%), 143 funcs (79%).

Add `triage_advance_test.go` for more room: 42 files, 3,773 test lines, 138 funcs. Rename the five `sleepguard` keys (`autoadvance_countdown`, `autoadvance_guard`, `autoqueue_countdown`, `autoqueue_trigger`, `triage_advance`).

## Blockers and open points

- No import cycle blocks the proposal: only external-test files move.
- Source clusters that would need a bigger move: `server` (Server adapters need exported fields or host interfaces), `server/sessions/prompt` (`Prompt` split into exported seams). Neither is needed for the 90% target. `prompt` source stays within 10 lines of the target.
- `server`: files land exactly on 45. Take the three extra files for headroom, or accept zero room.
- Decide in ticket 06/07: names of the three new test-only packages, whether one package per source package or one shared `*flows` package, and whether the dead `sessionadvancing_helpers_test.go` and `designsfixture_test.go` are deleted.
- Check ticket 02/08 for overlap on `server/servertest` before moving the three `servertest_*`/`unavailable` files.
- The move has no effect on `server/wire`, `tygo.yaml` or generated types.
