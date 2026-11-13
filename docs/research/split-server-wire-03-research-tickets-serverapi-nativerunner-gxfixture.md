# split-server-wire/03: movable clusters in tickets, cmd/serverapi, nativerunner, testutil/gxfixture

Question: which code in each package can move into new sub-packages so that every sizeguard measure ends at or below 90% of its limit (at most 45 files, 4,050 source lines, 4,500 test lines, 162 Test funcs)?

Method: sizes counted like `sizeguard/sizeguard_test.go` (tracked `.go` files directly in the package dir, all lines; Test funcs are `^func Test[A-Z_]` in `_test.go`), at design branch `31dd2aba6`. Dependencies come from a go/ast scan of which top-level names each file uses from the other files in the package, checked by hand with `rg`. External use counts are `rg` counts of `pkg.X` outside the package. A "use" is one occurrence. Counts are approximate (name-based, no type resolution).

A fact that applies to all four: moving a type's methods apart from the type is impossible, so a cluster can only move when it owns its types. Child packages may import the parent (`tickets`) as long as the parent does not import the child. Test files in `package x_test` that only use exported names can move with a cluster; internal tests move with the code they test.

## Summary

| Package | Now (files / src / test / funcs) | Over 90% | Proposed moves | After (files / src / test / funcs) |
|---|---|---|---|---|
| `tickets` | 50 / 4,500 / 3,596 / 168 | files, src, funcs (93%) | `log.go` into `tickets/ticketlog`; `oneoff.go` and `slug.go` into a new `tickets/oneoff` | 44 / 3,973 / 3,245 / 153 |
| `cmd/serverapi` | 50 / 3,956 / 3,620 / 146 | files only | the one-off verb (3 files, 3 test files) into `cmd/serverapi/oneoff` | 44 / 3,527 / 3,041 / 119 |
| `nativerunner` | 49 / 3,860 / 4,619 / 133 | files (98%), test lines (92%) | `procs.go` + `pty_exitwatch.go` into `nativerunner/proctable`; `pidstatus.go` into its own package | 43 / 3,634 / 4,479 / 129 (test lines are tight) |
| `testutil/gxfixture` | 34 / 4,338 / 1,193 / 44 | src (96%) | the scenario catalog into `testutil/gxfixture/catalog` | 34 / about 3,500 / 1,193 / 44 |

No import cycle blocks any proposal. Two need exports of internals (see nativerunner) and one changes a constructor signature (serverapi).

## 1. `tickets`

### Measures

50 files (100%), 4,500 source lines (100%), 3,596 test lines (72%), 168 Test funcs (93%).
Over 90%: files, source lines, Test funcs.
Needed: at least 5 files, 450 source lines and 6 Test funcs out of the root.

Already existing sub-packages: `archivekey`, `epicstats`, `schema` (28 files), `ticketgit`, `ticketlog` (10 files), `ticketmigrate` (25), `ticketmove` (7), `ticketspec` (17), `tickettest` (1). The root imports only `schema` (and `tickettest` from `helpers_test.go`), so child packages may import `tickets`.

### Tangled core (stays)

Almost everything else is one knot around the `Epic` and `Ticket` types and their methods, which cannot leave the package:

- `epic.go`, `epic_add.go`, `epic_set.go`, `loader.go`, `ticket.go`, `status.go` (633 lines, mostly `Epic` methods), `parentgraph.go`, `store.go`, `address.go`, `archive.go`, `original.go`, `design_step.go`.
- `blockedby.go` (270) + `blockedby_cycles.go` (433): `Epic` methods plus `projectGraph`, and `status.go`, `parentgraph.go`, `validate.go`, `warnings.go` and `basederive.go` all use its unexported `projectGraph`, `newProjectGraph`, `crossEpicRef`, `qualifiedRefs`, `nodeKey`. Moving them would need those internals exported, and `blockedby_cycles.go` also calls `epic_set.go`'s `ParseEpicAddress`, `original.go`'s `archivedFrom`, and `status.go`'s `siblingKey`, `splitBlockedByToken`: a real cycle if moved.
- `validate.go` (65), `warnings.go` (156), `basederive.go` (118 + 138 test, 3 funcs): use `newProjectGraph` / `projectGraph` / `qualifiedRefs` (unexported). Same cycle.

### Clusters that move cleanly

| Cluster | Files | Src lines | Test lines / funcs | What it uses from the rest of the package | `tickets.X` uses outside root |
|---|---|---|---|---|---|
| `log.go` + `log_test.go` (ticket event log: `LogEvent`, `ReadLog`, `AppendLog*`, rounds, `Event*`, `Source*`, `How*` constants) | 2 | 461 | 302 / 12 | Nothing. `log.go` imports only `events`, `projectlock`, `tickets/schema` (`tickets/log.go:3-16`). `log_test.go` uses `Load` and the `writeFile` helper var from `helpers_test.go` (`tickettest.WriteFile`); as a child test it imports `tickets` and `tickettest` | about 377 uses in about 95 files |
| `oneoff.go` + `oneoff_test.go` (`OneOffFile`, `IsOneOffsEpicName`, `IsOneOffTicket`, `IsOneOffEpic`, `EpicAddress`) | 2 | 43 | 30 / 2 | `Epic`, `Ticket`, and the exported consts `OneOffsEpic` / `ArchivedOneOffsEpic` (stay in `original.go`). No root file calls into `oneoff.go`. Child imports `tickets`: no cycle | about 30 uses in 21 files |
| `slug.go` + `slug_test.go` (`Slug`, `SlugFallback`) | 2 | 23 | 19 / 1 | Nothing. No other root file uses it | 7 uses in 7 files (`schedules`, `server/queue`, `server/health`, `server/cleanup`, `server/ticketwrite`, `testutil/gxfixture/spec`) |

Evidence for `log.go`: no root file other than `log.go` and `log_test.go` references any name it defines (`rg` over `tickets/*.go`). The only hits are `EventType` struct fields at `tickets/loader_test.go:165,167`, which are unrelated. `tickets/ticketlog` already imports `tickets` for `LogEvent`, `Rounds`, `PauseRound`, `EventPause*` (`tickets/ticketlog/rounds.go:3-8` and others), so moving `log.go` there turns those into local names and removes the `ticketlog` to `tickets` log coupling.

Where to put them:

- `log.go`: merge into the existing `tickets/ticketlog` (recommended; it is the log's renderer and question helpers already). It would hold 12 files (about 1,390 source lines, small). Call sites become `ticketlog.ReadLog`, `ticketlog.LogEvent`. The alternative is a new `tickets/ticketevents`, which avoids the `ticketlog.LogEvent` choice but adds a package.
- `oneoff.go` and `slug.go`: one new package `tickets/oneoff` holding `IsOneOffEpic`, `EpicAddress`, ... and `Slug`. `Slug` names one-off epics at every call site except `schedules/schedules.go:210` and `testutil/gxfixture/spec/schedules.go:80`, and `SlugFallback = "one-off"`. If the person does not want the pairing, `slug.go` has no better home among the existing packages and a package of one file is the honest alternative.

### Proposed move set and result

Move `log.go`+test, `oneoff.go`+test, `slug.go`+test (6 files).

- Files 50 to 44 (88%).
- Source lines 4,500 to 3,973 (88%).
- Test lines 3,596 to 3,245 (65%).
- Test funcs 168 to 153 (85%).

Minimum alternative: `log.go`+test alone gives 48 files and 4,039 source lines, which misses the file target. Adding `oneoff` (2 files) and one more file is needed. `format.go` (9 lines, `FormatCost`, 26 uses in 9 files) is a stray one-file candidate that has no natural home.

A different lever for files only: merging the three tiny files `format.go` (9), `legacy_headings.go` (9), `migration_change.go` (8) into one file saves 2 slots with no import change. The brief says moves only, so this is listed as an option, not proposed.

Duplicate tests: a name scan of `status_test.go` (53 funcs), `loader_test.go` (26) and `blockedby_epic_test.go` (17) found no tests that look like duplicates. The root is under the funcs target once `log_test.go` (12) leaves, so no test pruning is needed.

## 2. `cmd/serverapi`

### Measures

50 files (100%), 3,956 source lines (88%), 3,620 test lines (72%), 146 Test funcs (81%).
Over 90%: files only. Needed: at least 5 files out of the root.

### Shape

Every verb file defines `newServerXCmd(d Deps)`; `serverapi.go:46-59` (`Commands`) registers them all. Each file uses only `Deps` (`cmdbase.Env` plus `StdinIsTerminal`, `serverapi.go:38-41`) and a few shared helpers: `runTargets` (`serverapi.go:22`, used by `server_hold.go:53`, `server_workspaces.go:95`, `server_epics_archive.go:25,63`) and `webClient` / `runWebCall` (`server_web.go`, used by `server_cleanup.go`). External users of the package: `cmd/commands.go:23-24`, `cmd/cmd.go:143-144,250`, `cmd/evolve/evolve.go:187` (`serverapi.DecideOneOff`), `cmd/serverctl/server_config_status_test.go:46`. There is no unexported coupling between verb clusters, so a verb cluster can move if its constructor takes a type that the child can import. A child cannot import `serverapi.Deps` (the parent imports the child: cycle), so the constructor takes `cmdbase.Env` directly, which is what `Deps` embeds.

### Clusters

| Cluster | Files | Src lines | Test files / lines / funcs | Uses from the rest of the package | Notes |
|---|---|---|---|---|---|
| one-off: `server_oneoff.go` (143), `server_oneoff_payload.go` (88), `server_oneoff_wait.go` (198) | 3 | 429 | 3 / 579 / 27 (`server_oneoff_test.go` 227/10, `server_oneoff_payload_test.go` 145/8, `server_oneoff_wait_test.go` 207/9) | Only `Deps`. It calls `d.Dirs()` and `d.Dial()`, both `cmdbase.Env` methods (`cmdbase/env.go:29,32`). `payload` and `wait` use nothing from the package. Tests do not use `execute` / `serverParent` from `main_test.go` | `server_oneoff.go:129` exports `DecideOneOff` / `OneOffDecision`; one outside use (`cmd/evolve/evolve.go:187`) changes its import |
| web: `server_web.go` (387) | 1 | 387 | 4 / 885 / 32 | `Deps` including `StdinIsTerminal` (`server_web.go:171,304`), so the child needs its own Deps struct. `server_cleanup.go` uses its `webClient` and `runWebCall` | The tests use `execute` / `serverParent` and `webResetDeps`, so the child needs its own copy of `main_test.go` helpers |
| epics: `server_epics.go` (206), `server_epics_archive.go` (131) | 2 | 337 | 2 / 495 / 28 | `Deps`; archive uses `runTargets` | Needs `runTargets` and `notRunningMessage` exported from a shared place (for example `cmdbase`) first |
| cleanup: `server_cleanup.go` (140) | 1 | 140 | 1 / 62 / 2 | `runWebCall`, `webClient` from `server_web.go` | Moves with web |

### Tangled core (stays)

`serverapi.go` (registry, `Deps`, `runTargets`), `server_reads.go` (483: uses `tickets_ask.go`, `server_repair.go`, `server_snapshot.go`), `tickets_ask.go`, `server_repair.go`, `server_snapshot.go`, `server_hold.go`, `server_workspaces.go`, and the many small verbs (`server_agents`, `server_checks`, `server_config`, `server_disk`, `server_health`, `server_settings`, `server_stats`, `server_triage`, ...). They are independent of each other but each is small, so they are not worth moving to reach the target.

### Needed for any child package

- Constructors take `cmdbase.Env` (or the child's own Deps). `Commands` calls `oneoff.NewCmd(d.Env)`.
- Tests in a new child package need a `TestMain` that calls `cmdtest.Main(m)` (as `main_test.go:13` does). `cmdtest.Main` runs `servertest.RunSessionProcesses()` and `herdrfake.RunHelperProcess()` (`cmd/cmdtest/main.go:14-18`). The one-off tests build servers with `servertest` (`server_oneoff_test.go:24`, `dupServer`), so they need it. A child package without it would re-run the test binary as a session process (see the memory note on missing TestMain hooks causing fork storms).

### Proposed move set and result

Move the one-off cluster to `cmd/serverapi/oneoff` (6 files: 3 source, 3 test).

- Files 50 to 44 (88%).
- Source lines 3,956 to 3,527 (78%).
- Test lines 3,620 to 3,041 (61%).
- Test funcs 146 to 119 (66%).

External `serverapi.X` uses that change: 1 (`DecideOneOff`). Optional second move for more room: web + cleanup (3 files source+test count: `server_web.go`, `server_cleanup.go` and 5 test files, 8 files), which needs its own `Deps` with `StdinIsTerminal`.

## 3. `nativerunner`

### Measures

49 files (98%), 3,860 source lines (86%), 4,619 test lines (92%), 133 Test funcs (74%).
Over 90%: files, test lines. Needed: at least 4 files and 119 test lines out of the root.

### Shape

Most test files are `package nativerunner_test` (external) and share helpers: `killAgents` (`conformance_test.go`), `newHarness` (`headless_test.go`), `newPty` / `ptyControl` / `ptyHome` (`pty_test.go`), `shortTempDir` and `waitFor` (`shorttempdir_test.go`), `builtFakeclaude` (`builtfakes_test.go`), plus `export_test.go` hooks (`Died`, `LaunchCommand`, `KillLeftovers`, `PS`). `headless_test.go:35` holds the `TestMain` (fake claude, `holder.ServeTestBinary`). So test files cannot move on their own; only tests that go with an extracted piece of code can.

Heavy non-test tangle (stays): `headless.go` (765) and `pty.go` (613) with `pty_launch.go`, `pty_adopt.go`, `pty_personinput.go`, `pty_registry.go`, `pty_turn.go`, `pty_usage.go`, `pty_codexapproval.go`, `stop.go`, `owner.go`. They share `Pty`, `ptySession`, `headlessSession`, `tracker`. `agentdir.go` and `reattach.go` loop with them: `agentdir.go` uses `ptyRunnerName` (`pty.go`) and `headlessAlive` / `ptyAlive` / `readMeta` (`reattach.go`), while `pty.go` uses `layouts` / `removeAll` (`agentdir.go`) and `Verdict*` / `pidAlive` (`reattach.go`).

### Clusters that move

| Cluster | Files | Src lines | Test files / lines / funcs | What it uses from the rest | Uses outside the package |
|---|---|---|---|---|---|
| `proctable`: `procs.go` (67) + `pty_exitwatch.go` (143) | 2 | 210 | 2 / 78 / 2 (`procs_test.go` 23/1, `pty_exitwatch_test.go` 55/1) | Nothing. `procs.go` uses only the standard library. `pty_exitwatch.go` uses only `ProcessTable`, `psTable` | `Process` 25, `ProcessTable` 5, `PS` 6 = about 36 |
| `pidstatus.go` | 1 | 16 | 1 / 62 / 2 (`pidstatus_test.go`) | Nothing. It is pure aliases of `harness.PidStatus`, `harness.Claude*`, `harness.ParsePidStatus` | `PidStatus` 16, `Claude*` 21, `ParsePidStatus` 3 = about 40 |
| `usage.go` | 1 | 30 | none | Nothing | `UsageRecord` 39, `UsageWindow` 28, `UsageFile` 10, `UsageLimitPercent` 1 = about 78. In-package: `pty_usage.go`, `pty_ratelimit_test.go` |
| `version.go` + `nativerunner.go` | 2 | 41 | none | Nothing (`version.go` has no test) | `CompareVersions` 6, `MinClaudeVersion` 7, `MinCodexVersion` 4 = 17 |
| `events.go` + `events_test.go` | 2 | 290 | 1 / 54 / 2 | `tracker` (unexported) is read and written by `headless.go` and `reattach.go`, so it needs exports: fields `sessionID`, `turn`, `blocked`, `permission`, `answered`, `exited`, `resetAt`, `unknownCommands`, and methods `status`, `busy`, `apply`, `ended`, `started`, `capabilityErr`, plus `parseEvent`, `event`, `permission` (`headless.go:92,259,313,343,369,414,440-475,685,712-745`, `reattach.go:58-80,133-156`) | `StatusTrail` 2 |
| `launch.go` + `launch_test.go` | 2 | 89 | 1 / 32 / 1 | `ProcessTable` only, so it would sit above `proctable` | `Meta` 39, `MetaFile` 32, `NewUUID` 3, `WriteMeta` 4, `StartDetached` 1 and more = about 80 |

Details on the `proctable` coupling:

- `psTable.StartTimes` is defined in `pty_exitwatch.go:27`, a method on the type from `procs.go`, so the two files must move together.
- It needs exports: a real-table value in place of `psTable{}` / `orPS` (`orPS` is called at `pty.go:238,245,429`, `pty_launch.go:243`, `headless.go:274`, `pty_adopt.go:35,55,63`, `agentdir.go:100`), `ExitWatcher` with its `watch` method (`pty.go:54`, `pty_adopt.go:55`) and `exitPollInterval` (`pty.go:583`).
- `export_test.go:25` (`var PS ProcessTable = psTable{}`) becomes the package's own exported value; its other users are `headless_test.go`, `pty_test.go`, `stop_test.go`, `procs_test.go`.

Details on `pidstatus`:

- Moving it to a child package of pure aliases is a weak fix. The cleaner fix is to point the 40 users at `harness.X` and drop the file. That is a refactor, not a move, so it is flagged for the person.
- `pidstatus_test.go:55` `TestClaudeSessionsDirIn` tests `harness.ClaudeSessionsDirIn`, and no test of that function exists in `harness`. It looks misplaced and belongs in `harness`. No duplicate was found.

### Proposed move set and result

Move `proctable` (4 files) and `pidstatus` (2 files).

- Files 49 to 43 (86%).
- Source lines 3,860 to 3,634 (81%).
- Test lines 4,619 to 4,479 (89.6% of the 5,000 limit; 99.5% of the 4,500 target, so only 21 lines of room).
- Test funcs 133 to 129.

The test-line room is thin because few test files are tied to a movable cluster. If the person wants room, add:

- `version.go`+`nativerunner.go` and `usage.go` (3 files, 71 source lines, no test lines): files go to 40.
- `events` (needs the tracker exports above): test lines 4,425.
- `launch` (above `proctable`): test lines about 4,393, but about 80 `nativerunner.X` uses change.

Splitting the external test files (`headless_test.go` 707, `pty_test.go` 948, ...) out of the root dir would need the shared helpers (`newHarness`, `newPty`, `killAgents`, `TestMain`) to move too. That is a bigger move than a cluster move and is not proposed.

## 4. `testutil/gxfixture`

### Measures

34 files (68%), 4,338 source lines (96%), 1,193 test lines (24%), 44 Test funcs (24%).
Over 90%: source lines only. Needed: at least 288 source lines out of the root.

The root is `package main` (the `gxfixture` command, `main.go:1`), so only code that no `fixture` method owns can move into an importable sub-package. Existing children: `spec` (28 files, 2,753 lines; the scenario data model and builders), `limitseed`, `designflows`, `webflows`, `limitsflows`, `chatcapture`, `fixturetest`, `websignin`.

### Tangled core (stays)

All `fixture` methods and their helpers: `main.go`, `startup.go` (303), `serve.go` (438), `sessions.go` (492), `seed.go`, `health.go`, `limits.go`, `limitburn.go`, `spend.go`, `restart.go`, `migrate.go`, `shutdown.go`, `disk.go`, `devmode.go`, `fakegh.go`, `chat.go`. `env.go` (101) has no dependencies, but `env_test.go` (254, 19 funcs) uses `watchLifetime` from `main.go` and the process harness, so it cannot leave cleanly.

### Cluster that moves: the scenario catalog

`scenarios.go` (816 lines) is 750 lines of table data (`var scenarios = map[string]scenario{...}`, `scenarios.go:54-803`) with some small helpers around it. The data uses only:

- `spec.*`, `limitseed`, `servertest`, `wire`, `events`, `tickets`, `schema`, `agentfake` (all importable).
- `chatEpicDone` (`scenarios.go:22`) and `oldShapeStatus` (`:32`): used by no other file, they move with the map.
- The aliases `epic`, `ticket`, `scenario`, `sessionSpec`, `commandStep`, `restartSpec`, `recommendation`, `archivedEpic`, `webMode` and `project` (`scenarios.go:28-52`). These are definitions of `spec.*` and `limitseed.Project`, so the new package defines its own or uses `spec.X` directly.
- Six scenario values and builders defined elsewhere in `package main`: `cacheGate`, `cacheGateCustom` (and `cacheGateEpics`) in `cachegate.go:36-48`; `launchHeld`, `launchFail`, `epicAtCap` and their `addLockfile` GitSetup in `launchstate.go:29-79`; `agentsImagesScenario()` in `imagesscenarios.go:23-33`. They are pure data, kept in files that also hold `fixture` methods (`setupFakeGo`, `setupFakeNpm`, `seedDrafts`, ...). Those declarations move; the methods stay.

Remaining use of the map by `package main`: `scenarios[...]` / `scenarioNames()` in `main.go:59-61`, `args_test.go:52`, `drafts_test.go:12`; `freePort` (`scenarios.go:809`, used by `main.go:96`) stays.

No other `package main` file uses `chatEpicDone` or `oldShapeStatus`, so no unexported internal of `main` is needed by the data. There is no import cycle: `catalog` imports `spec`, `limitseed`, `servertest`; nothing under `spec` or `limitseed` imports `main`. There are no outside `gxfixture.X` uses (it is a command).

### Proposed move set and result

Add `testutil/gxfixture/catalog` exporting the map (for example `catalog.All()` and `Names()`), holding the table plus the six scenario values and `chatEpicDone` / `oldShapeStatus`. `scenarios.go` keeps the aliases, `project`, `freePort` and `scenarioNames` (about 70 lines). Moved lines: about 750 (map) + 13 (`cachegate.go` vars) + 51 (`launchstate.go`) + 13 (`imagesscenarios.go`) + 13 (helpers) = about 840.

- Files 34 (no change; the new files are in the child package).
- Source lines 4,338 to about 3,500 (78% of the limit; 86% of the target).
- Test lines 1,193 and Test funcs 44 unchanged.

Minimum variant: move only the map and leave the six scenario values in `main` (the map would then be merged with them in `main`). That gives about 3,590 lines but splits the catalog across two places, so it is not recommended.

`spec` could host the map instead of a new package, but it has 28 files and 2,753 lines already and holds the data model, so a separate `catalog` is cleaner.

## Open items for the grilling ticket

- `tickets`: put `log.go` into `ticketlog` or a new `ticketevents` package; pair `slug.go` with `oneoff.go` or not.
- `nativerunner`: accept the thin test-line room (21 lines), or also move `events` (needs tracker exports) or `launch`; retire the `pidstatus.go` aliases in favor of `harness.*` instead of moving them.
- `cmd/serverapi`: constructors take `cmdbase.Env`; the new child package needs a `TestMain` calling `cmdtest.Main`.
- `testutil/gxfixture`: confirm `catalog` as the package name and that the six scenario values move with the map.
