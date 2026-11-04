# Splitting `tickets`, `nativerunner`, `git` and `testutil/agentfake` (gx:split-server-pkg/11)

How the four "core" packages split under the agreed limits (50 files including tests, 4,500 source
lines, 5,000 test lines, 180 `Test` funcs, 1,000 lines per file, 50 methods per type): the package
list, import direction, cycle risks, the files over 1000 lines and what each cut costs importers.
Measured on branch `research/split-core-packages` (from `main` at `e039fe8e8`) on 2026-11-04. The
per-package totals match `docs/research/split-size-baseline.md`. Paths are repo-relative.

## Method

- Dependencies come from `go/types`: a source-importer type check of each package (source plus
  internal test files). For every identifier use it records the file that declares the object.
  This resolves methods and fields exactly, so it has none of the name collisions of a plain AST
  scan. The file graph and its strongly connected components (SCCs) come from that.
- A candidate package is a list of files. The tool prints, for every pair of candidates, the
  symbols one needs from the other. A split is cycle-free when those edges form a DAG.
- Importer cost comes from a `go/parser` scan of every `.go` file in the repo (not `webui`, not
  `docs`) that imports the package. It records each `pkg.Name` selector. A selector whose symbol
  moves is one edit site. "Files touched" is the number of importer files with at least one such
  selector, tests included. The edits are mechanical (import line plus qualifier).
- Counts are `wc -l` lines and `^func Test` matches, as in the baseline. Methods are `FuncDecl`
  receivers in non-test files. None of the four packages declares methods in a test file.
- Not measured: wall time, and whether the guard counts anything differently. The proposed splits
  were not run. Every edge claim is from the current tree.

## Answer

All four fit. R = required to fit, O = optional (for margin).

| Package today | Proposed packages | Importer files touched |
|---|---|---|
| `tickets` (102 files, 9,033 src lines) | `tickets` core, `tickets/ticketlog`, `ticketmove`, `ticketspec`, `ticketmigrate` (all R), plus one tiny test-helper package | 114 of 387 |
| `nativerunner` (56 files, 6,623 test lines, `pty.go` 1,022, `Pty` 55 methods) | `nativerunner/holder` (R); `pty.go` file split (R); `Pty` registry extraction (R) | 53 of 103 (the holder cut; the other two cost 0) |
| `git` (67 files, 247 tests) | `git/pr` (R), `git/stage` (R), `git/history` (O, but see below) | 40 of 230 (R only); 93 with `history` |
| `testutil/agentfake` (4,694 src lines, `tui.go` 1,168) | `testutil/agentfake/codex` (R); `tui.go` file split (R) | 10 of 96 |

- `tickets` has one knot (the model, loader, status and block graph, 13 files in one SCC). It
  stays in one package. It fits at 47 files, 4,399 source lines (98% of the limit) and 168 tests.
  It is the tightest result here.
- `nativerunner` fits after the holder cut, but the remaining runner package has about 4,863 test
  lines (97% of the limit). `Pty` and `pty.go` are fixed inside the package with no importer change.
- `git` has no gx imports, so no cycle can come from outside. Cutting only `pr` and `stage` fits by
  one file and two tests (49 files, 178 tests), so `history` is the cut that gives real margin.
- `agentfake` is over by only 194 source lines. The one clean cut is the fake Codex (1,168 lines),
  which sits on top of the fake Claude TUI.
- Behaviour does not change anywhere. The work is moving declarations between files and packages,
  exporting identifiers a new package needs, and extracting one helper type.

## 1. `tickets`

### Today

55 source files, 47 test files, 9,033 source lines, 5,678 test lines, 252 tests. The sub-package
`tickets/schema` (1,849 lines) is separate and not part of this split. No file is over 1000 lines
(largest: `loader.go` 706, `status_test.go` 833).

Two measures force at least three packages: 9,033 source lines cannot fit in two packages of
4,500, and 102 files cannot fit in two of 50.

Non-test gx imports: `config`, `conversations`, `events`, `fsutil`, `git`, `projectlock`,
`tickets/schema`. None of them imports `tickets`, so no cycle can come from outside.

### The knot

The file graph (type-checked, source files only) has three SCCs:

- 13 files: `archive, blockedby, blockedby_cycles, design_step, epic, epic_add, epic_set, loader,
  migrate, original, parentgraph, status, ticket`. This is the model: `Ticket`, `Epic`, status
  derivation, the blocked-by graph, the loader.
- 3 files: `log, migrate_question, pausedetail`.
- 2 files: `address, store`.

`migrate.go` is in the first SCC only because `design_step.go` uses `MigrationChange`, a type
defined in `migrate.go`. Moving that one type into a core file takes `migrate.go` out. What stays
is intrinsically coupled: `Epic` has methods in `epic.go`, `blockedby*.go`, `parentgraph.go` and
`status.go`, and Go allows methods only in the type's own package. Cutting the knot means turning
methods into functions, which breaks the API. Not recommended.

### Proposed packages (all R)

| Package | Files | Src/test files | Src lines | Test lines | Tests |
|---|---|--:|--:|--:|--:|
| `tickets` (core) | `address, archive, basederive, blockedby, blockedby_cycles, design_step, epic, epic_add, epic_set, epic_stats, format, iterbranch, loader, original, parentgraph, status, store, ticket, trunk, trunk_cache, validate, warnings` + 25 tests | 22 / 25 = 47 | 4,399 | 3,492 | 168 |
| `tickets/ticketlog` | `log, question, rounds, answered, startover, pausedetail` + 6 tests | 6 / 6 = 12 | 872 | 803 | 31 |
| `tickets/ticketmove` | `move, allocate, refs` + 4 tests (with `original_test`) | 3 / 4 = 7 | 850 | 498 | 21 |
| `tickets/ticketspec` | `lintclaims, lintclaims_extract, bundled, verifyplan, verifybodies, manifest, specread, publish_checks, estimates` + 5 tests | 9 / 5 = 14 | 1,229 | 419 | 20 |
| `tickets/ticketmigrate` | `migrate, migrate_destination, migrate_pause, migrate_question, migrate_store, backfill*, storewalk, map_done, birthtime_*` + 7 tests (with `epic_stats_test`, `project_file_test`) | 15 / 7 = 22 | 1,683 | 466 | 12 |
| Sum | | 102 | 9,033 | 5,678 | 252 |

Package names are working names. The file lists are what was measured. After the `ValidateRefs`
tests move (see the cycle table) the core is 4 tests and about 135 test lines smaller than listed.
The core is the only package near a limit: 47 of 50 files, 4,399 of 4,500 lines, 168 of 180 tests.

### Import direction

```
ticketspec    -> tickets (core)
ticketlog     -> tickets
ticketmove    -> tickets, ticketlog      (LogPath)
ticketmigrate -> tickets, ticketlog      (AppendLogInner, pause events)
```

Measured symbol needs (source only): `ticketlog` needs 12 core symbols, `ticketmove` 36 core and 1
`ticketlog`, `ticketspec` 3 core (`Ticket`, `ActualContextWindow`, `PrefixTokens`),
`ticketmigrate` 58 core and 19 `ticketlog`. The core needs nothing from the other four once the two
back edges below are fixed.

### Cycle risks and fixes

| Risk | Where | Fix |
|---|---|---|
| core -> `ticketmigrate` | `design_step.go` uses `MigrationChange` (`Notes`, `Path`), defined in `migrate.go` | Put `type MigrationChange` in a core file. `ticketmigrate` returns `[]tickets.MigrationChange`. Callers already write `tickets.MigrationChange` (5 `cmd` files, 7 `migrations` files, 4 `server` tests), so no importer change |
| `ticketlog` -> `ticketmigrate` | `log.go` calls `clipRunes`, defined in `migrate_question.go` | Move `clipRunes` into `log.go` (or `fsutil`); `migrate_question.go` imports it |
| Shared test helpers | `log_test.go` defines `requireWaitsForProjectMigration` and `storeProjectWithEpic` (and the core's `writeFile`); core, `ticketlog` and `ticketmove` tests call them | Move them to a tiny non-test helper package that imports only `projectlock` and `testing` (for example `tickets/tickettest`, about 60 lines). It must not import `tickets`, or the core's internal tests would cycle |
| Core tests that need an upper layer | `original_test.go` uses `PlanMove`, `ApplyMove`; `epic_stats_test.go` uses `BackfillFriction`; `project_file_test.go` uses `MigrateIntoStore`; `blockedby_epic_test.go` has 4 tests (`TestValidateRefs_*`, about 135 lines) that call `ValidateRefs` in `refs.go` | The first three files move with their subject (already in the table). Move the 4 `ValidateRefs` tests into `ticketmove`. An internal core test cannot import a package that imports the core |
| `log_test.go` pause-migration tests | use `ApplyPauseMigration`, `PauseStep` from `migrate_pause.go` | Move those tests into `ticketmigrate` |

Every other source edge is a DAG edge. Only test files have upward edges, and they are fixed above.

### Importer-change cost

| Cut | Distinct symbols | Importer files (non-test / test) | By package |
|---|--:|---|---|
| `ticketlog` | 50 | 36 / 37 | `server` 44, `cmd` 11, `ui/tickets` 6, `ralphloop` 4, `repair` 3, `migrations` 2, `gxfixture` 2, `grillround` 1 |
| `ticketmove` | 11 | 12 / 0 | `cmd` 6, `server` 4, `ralphloop` 1, `recovery` 1 |
| `ticketspec` | 19 | 5 / 2 | `cmd` 6, `skills` 1 |
| `ticketmigrate` | 21 | 17 / 10 | `server` 14, `migrations` 7, `cmd` 5, `ralphloop` 1 |

Union: 114 importer files (66 non-test, 48 test) of the 387 that import `tickets` today (219
non-test, 168 test). By package: `server` 62, `cmd` 24, `migrations` 8, `ralphloop` 6, `ui/tickets`
6, `repair` 3, `gxfixture` 2, one each in `grillround`, `recovery`, `skills`. The other 273 files use
only core symbols. `LogEvent` and `AppendLog` are most of the cost: `ticketlog` alone is 73 files.

### Optional cuts

- O1, not recommended: extract `address, store, trunk, trunk_cache` and their 4 tests as a lower
  layer. It is a true leaf (nothing in it uses another core file). The core would drop to 39 files,
  3,810 source lines and 144 tests. But `Address`, `ParseAddress`, `RootIn`, `ProjectDirs` and
  `Trunk` are used in 162 importer files in 14 packages (`server` 82, `cmd` 44, `ralphloop` 12,
  `ui/tickets` 9). The cost is far above the margin it buys.
- O2: merge `ticketlog`, `ticketmove` and `ticketspec` into one package. It is 33 files, 2,951 source
  lines, 1,720 test lines and 72 tests, so it fits, and gives 3 packages in total instead of 5. It
  joins unrelated concerns (event log, ticket moves, publish checks).
- O3: move `validate.go` and `warnings.go` (221 lines, 2 files; nothing else in the core uses
  them) into a small `ticketcheck` package. Gains 2 files and 221 lines of margin for one more
  package. Low value.

## 2. `nativerunner`

### Today

25 source files, 31 test files, 5,998 source lines, 6,623 test lines, 192 tests. Over 1000
lines: `pty.go` (1,022). Methods: `Pty` 55 (limit 50), `Headless` 34, `HolderClient` 28,
`frameHub` 19, `holderAnswerer` 14. Non-test gx imports: `agentrunner` (plus `claudeui`, `codexui`,
`screen`), `harness`, `syshealth`, `transcript`. 29 of 31 test files are external
(`nativerunner_test`); `export_test.go` (92 lines) exposes internals.

Source lines, test lines and tests are all over, so at least two packages are needed.

### The seam: the holder

The non-test file graph has one SCC of 12 files, mostly through shared constants and a few
helpers. By role it separates into clean layers:

- `holder`: the process that owns the pty and serves `holder.sock`, its client and wire protocol.
  Files `holder, holderanswer, holderattach, holderframe, holdersock, codexapproval, holdererr`
  (7 files, 2,521 lines). Tests `holder_test, holderanswer_test, holderattach_test, holdersock_test,
  holderwatch_test` (5 files, 1,705 lines, 52 tests).
- `nativerunner`: everything else (`Pty`, `Headless`, `OpenDir`, `OwnerPIDs`, meta, process table,
  usage, version).

### Proposed packages (R)

| Package | Files (src/test) | Src lines | Test lines | Tests |
|---|--:|--:|--:|--:|
| `nativerunner/holder` | 12 (7 / 5), plus a new `export_test.go` | 2,521 | about 1,765 | 52 |
| `nativerunner` | 44 (18 / 26) | 3,477 | about 4,863 | 140 |
| Sum | 56 | 5,998 | 6,623 | 192 |

`export_test.go` splits in two. The holder half (`AnswerClaude`, `AnswerCodex`, `RunTestHolder`,
`RunTestSessionHolder`, `SetHolderOptionalOps`, `HolderOptionalOps`, `HolderDenyTimeout`,
`SetHolderAfterExit`, `SetHolderDenyTimeout`, `FrameInterval`, `SetFramePacing`,
`ArmedFrameTimers`, `SendOversizedHeader`, about 60 lines) goes with the holder. `Died`,
`LaunchCommand` and `PS` stay. The test-line figures are estimates for that file only.

The runner package is at 97% of the test-line limit. Its `Pty` tests are about 2,700 lines, so
growth lands there. See O4.

### Import direction

```
nativerunner -> nativerunner/holder -> harness, agentrunner, ...
```

Measured: runner needs 56 holder symbols (`DialHolder`, `HolderClient`, `ErrHolderDenyBusy`,
`HolderSocketFile`, `ExitCodeFile`, `Screen`, `SendKeys`, `Watch`, ...). Holder needs from the
runner 3 exported symbols (`ClaudeIdle`, `ClaudeWaiting`, `PidStatus`) and 4 unexported ones
(`claudeTyping`, `permissionPrompt`, `startupScrollback`, `trustKeyRepeat`).

### Cycle risks and fixes

| Edge | Fix |
|---|---|
| holder -> runner: `ClaudeIdle`, `ClaudeWaiting`, `PidStatus` | These are aliases in `pidstatus.go` for `harness.PidStatus` and its constants. `holder` imports `harness` directly. The aliases stay in `nativerunner` for the 33 importer files that use them |
| holder -> runner: `claudeTyping`, `permissionPrompt`, `startupScrollback`, `trustKeyRepeat` | Define them in `holder` (constants and one var for the holder child's screen rules) and export what `Pty` still reads (`pty.go` lines 257, 265, 618, `pty_personinput.go`) |
| runner -> holder: unexported helpers | `createHolderErr` and `holderErrText` (in `holdererr.go`) move with the holder and become exported. `Pty` also uses `holderAnswerer`, `statusReply`, `waitTurn`, `statusWithProgram`, `watched`, `codexScreenDialog`, `holderStopGrace`: export those it needs |
| `agentdir.go` | It names `Pty`, `Headless` and the holder file names (`HolderSocketFile`, `RawLogFile`, `ScreenFile`). It stays in `nativerunner` and imports `holder` |
| Tests | Holder tests import `nativerunner` for `PS` and a few aliases. An external test package may import the package that imports it, so there is no cycle. `agentfake` tests import the holder and `nativerunner` tests import `agentfake`: still no cycle, since `holder` does not import `agentfake` |

### `pty.go` (1,022 lines)

The launch code is contiguous: `launch`, `holderExitedErr`, `watchExit`, `startupScreen`,
`noPidFileErr`, `launchTick`, `spawn`, `launchCommand` (lines 170 to 502, apart from the exported
`Reattach`), plus the consts `trustKeyRepeat` and `startupScrollback`. Moving them to
`pty_launch.go` (139 lines now) takes about 320 lines out of `pty.go` (to about 700) and puts
`pty_launch.go` at about 460. It is a pure move: no behaviour or API change (R).

### `Pty` has 55 methods (limit 50)

A file move does not change the count. 19 of the 55 are exported (the `agentrunner.Runner` surface
and `Cleaner`) and cannot move. The unexported methods group by the state they touch (measured by
the fields each one reads):

| Group | Methods | State |
|---|---|---|
| session registry | `reserve`, `release`, `live`, `markDied`, `stopFailed`, `tracked`, `trackSilent` | `mu`, `sessions`, `starting` only |
| timing defaults | `pollInterval`, `promptGrace`, `startTimeout`, `launchCaps` (`WaitN` is exported) | `PollInterval`, `PromptGrace`, `StartTimeout`, `LaunchCaps` only |

R: extract the registry as an unexported `ptyRegistry` type that owns `mu`, `sessions` and
`starting` with those 7 methods. `Pty` holds one and calls `p.reg.reserve(...)`. `Pty` goes from 55
to 48 methods. The places that lock `p.mu` directly (`Start`, `adopt`, `Stop`, `Find`, `List`)
change to use it. It is behaviour-preserving but it touches lock code, so run the pty tests under
`-race` (`pty_test.go`, `pty_start_adopt_test.go`).

O4a: also fold the 4 timing getters into a small `ptyTimings` value. `Pty` goes to 44 (12% below the
limit instead of 4%).

The same shape exists already: `ptyPrompter` (11 methods) and `holderAnswerer` (14) are extracted
helpers of this kind.

### Importer-change cost

| Cut | Distinct symbols | Importer files (non-test / test) | By package |
|---|--:|---|---|
| `holder` | 39 | 28 / 25 | `server` 20, `cmd` 11, `ui/agents` 8, `claudedoctor` 4, `agentfake` 5 (tests), `gxfixture` 3, `agentlog` 1, `servertest` 1 |
| `pty.go` file split | 0 | 0 | none |
| `Pty` registry | 0 | 0 | none (all unexported) |

53 of the 103 files that import `nativerunner`. Names are kept, so each edit is an import line
plus the qualifier (`nativerunner.DialHolder` to `holder.DialHolder`). Renaming to drop the stutter
(`holder.HolderClient`) would raise the cost and is left out.

### Optional cut

- O4: move `Headless` (752 lines), `events.go`, `reattach.go`, `stop.go` and their tests
  (`headless_test`, `headless_permission_test`, `events_test`, `answer_test`, `reattach_test`,
  `stop_test`, about 1,400 test lines) into `nativerunner/headless`. It takes the runner's test
  lines from about 4,863 to about 3,500. It is more than a file move: `reattach.go` also holds 3
  `Headless` methods plus the shared `Verdict`, `awaitExit`, `pidAlive`, `readMeta` and
  `signalGroup` that `Pty` uses (8 symbols); `isSlashCommand` is used by `Headless` but defined in
  the `Pty` files; `agentdir.go` and `owner.go` still name both runners. Cost: 17 `server` files plus
  4 others use `Headless`, `Verdict`, `StatusTrail`. Take it only if the test-line margin matters.

## 3. `git`

### Today

35 source files, 32 test files, 4,643 source lines, 5,354 test lines, 247 tests, 67 files. No file
is over 1000 lines (largest `cherry_pick_test.go` 505). No type has more than 8 methods. Non-test
imports of gx packages: none. Test imports: `testutil`, which itself imports no gx package. So
`git` cannot be in a cycle with anything, today or after a split.

Over: files (67), source lines (3%), test lines (7%), tests (247, 37% over).

### Layers (non-test, measured)

```
run.go                                   Run, RunWith, RunRaw, RunError, ...
repo.go ref.go head.go                   -> run
branch.go remote.go worktree.go          -> repo, run
status.go log.go log_parse.go commit.go  -> branch, repo   (log and log_parse are one SCC)
tag stash clone doctor gitdirs configenv tracked_files file_origin dirtyland push_divergence
stage.go                                 -> run
delta.go commit_diff.go                  -> stage
amend.go -> stage;  reword.go -> amend;  cherry_pick.go -> reword
treeid.go merge.go                       -> cherry_pick (merge also branch, worktree)
pr.go pr_*.go                            -> repo, run
```

### Proposed packages

| Package | Files | Src/test files | Src lines | Test lines | Tests | |
|---|---|--:|--:|--:|--:|---|
| `git/pr` | `pr, pr_closed, pr_comments, pr_errors, pr_graphql, pr_list` + 4 tests | 6 / 4 = 10 | 847 | 534 | 24 | R |
| `git/stage` | `stage, delta, commit_diff` + `stage_test, delta_test, commit_diff_test, diff_lines_test, repo_relative_path_test`; `StagedFiles` (about 8 lines) moves in from `amend.go` | 3 / 5 = 8 | about 766 | 940 | 45 | R |
| `git/history` | `amend, reword, cherry_pick, treeid, merge` + 5 tests | 5 / 5 = 10 | about 676 | 1,075 | 44 | O |
| `git` (core) | the rest | 21 / 18 = 39 | 2,354 | 2,805 | 134 | |

What fits:

- With only `pr` and `stage` cut, the core keeps `history`: 49 files, 3,030 source lines, 3,880 test
  lines, 178 tests. Every limit holds, but the margin is 1 file and 2 tests. That is why `history` is
  the recommended third cut even though it is not needed to fit.
- With all three cuts the core is 39 files (78%), 2,354 source lines (52%), 2,805 test lines (56%)
  and 134 tests (74%).

### Import direction

```
git/pr      -> git
git/stage   -> git
git/history -> git, git/stage
```

Measured needs: `pr` needs 9 core symbols and `run`; `stage` needs 12 (`RunWith`, `RunOptions`,
`NonInteractiveEnv`, `AllowExit`, 4 unexported run helpers, ...); `history` needs 15 core symbols
and 3 from `stage` (`HasStagedChanges`, `ListStageFiles`, `Path`).

### Cycle risks and fixes

| Risk | Where | Fix |
|---|---|---|
| Unexported run helpers | `run.go` has `run`, `runEnv`, `runAllowFail`, `runNoOptionalLocks`, `runRawAllowFail`, `joinOutput`. `stage.go` uses three, `amend.go` and `reword.go` use `joinOutput`, `treeid.go` and `dirtyland.go` use `runEnv`, `pr.go` uses `run` | `run(dir, args)` is a thin wrapper of the exported `Run(dir, nil, args...)`, so the new packages can call that. That leaves 5 new exports: `RunEnv`, `RunAllowFail`, `RunNoOptionalLocks`, `RunRawAllowFail`, `JoinOutput`. They become `git` API. The alternative is a `git/internal/run` package that `git` re-exports with aliases (no importer change, two spellings) |
| `StagedFiles` | defined in `amend.go`, used by `stage_test.go` | Move it into `stage.go`. Then `stage` does not need `history` at all |
| Internal tests across the new boundary | `stash_test.go` (internal) calls `StagePath` from `stage.go`; `repo_relative_path_test.go` (internal) tests `RepoRelativePath`, also in `stage.go` | Move `repo_relative_path_test.go` to `stage`. Give `stash_test.go` a setup that stages with `Run(dir, nil, "add", ...)`, or make it an external test. An internal test of `git` may not import `git/stage`, because `stage` imports `git` |
| Internal tests of moved code | `amend_test, pr_closed_test, pr_graphql_test, pr_state_test, reword_test, stage_test, delta_test, treeid_test` are internal | They move with their subject. Check they use only that package's own unexported names plus the 5 helpers above |

### Importer-change cost

| Cut | Distinct symbols | Importer files (non-test / test) | By package |
|---|--:|---|---|
| `pr` | 34 | 12 (10 / 2) | `ui/prs` 6, `cmd` 2, `ui/commit`, `ui/log`, `ui/status`, `ui/worktrees` 1 each |
| `stage` | 24 | 27 (18 / 9) | `ui/status` 16, `ui/commit` 7, `cmd` 2, `ui/stash` 1, `ui/diffview/diffcore` 1 |
| `history` | 38 | 55 (31 / 24) | `server` 20, `ralphloop` 15, `cmd` 9, `check` 2, `repair` 2, `ui/log` 2, one each in `brief`, `migrations`, `ui/amend`, `ui/commit`, `ui/reword` |

`pr` and `stage` together touch 40 files. All three touch 93 of the 230 files that import `git`.
`tickets` uses only core symbols (`FindRepo`, `ListWorktrees`, `RefExists`, `RemoteDefaultBranch`,
`Repo`, `ValidBranchName`), so the `tickets` split and the `git` split do not interact.

### Optional cut

- O5: instead of `history` (55 files, 44 tests), take `stash.go` (137 lines, 14 tests, 12 importer
  files) into `git/stash`. Files 47 (with `pr` and `stage` cut), tests 164 (91%). Cheaper (12 files
  instead of 55) but a package of one 137-line file is thin and `stash` is not a distinct layer.
  `history` has the better shape; `stash` is cheaper.

## 4. `testutil/agentfake`

### Today

14 source files, 15 test files, 4,694 source lines (4% over), 3,056 test lines, 119 tests. Over
1000 lines: `tui.go` (1,168). Methods: `codexFake` 36, `tui` 29, `program` 10, `Harness` 8. Non-test
imports: `agentrunner`, `codexui`, `screen`, `check`, `claudedoctor`, `harness`, `transcript`. All
15 test files are internal. Five `cmd/*` programs use it (`agentfake`, `agentfaketui`,
`agentfakeworker`, `fakeclaude`, `fakecodex`).

### Structure (measured)

```
agentfake.go claude.go worker.go        leaf layer, 737 lines: Options, Run, Control, Action*,
                                        Behaviour*, SendControl, WorkerConfig, RunWorker
probe.go editormode.go                  leaf, 263 lines
tui.go design.go statusline.go program.go programinput.go harness.go
                                        the fake Claude TUI (one SCC of 5)
codex.go codexhooks.go codexinput.go    the fake Codex TUI (one SCC of 3), 1,168 lines
```

The fake Codex is built on the Claude TUI. `codex*.go` needs 45 package-level symbols from the TUI
files. In the other direction the TUI files need only the constant `CodexEnv` (read by `probe.go`)
and, in tests, `TUICodex`.

### `tui.go` (1,168 lines): a file split (R)

`tui.go` is five contiguous parts (line numbers today):

| Range | Content | Lines | New file |
|---|---|--:|---|
| 27 to 245 | env names, `PidFile`, path helpers, `ReadTUIArgs`, `SendTUIControl`, dialogs, action constants | about 220 | `tui_control.go` |
| 247 to 417 | `TUIClaude`, `tuiOptions`, `parseTUIArgs`, `runTUIClaude` | about 170 | `tui_options.go` |
| 418 to 692 | `tui` struct, `run`, event reader, key handlers, `startSession` | about 275 | `tui.go` |
| 693 to 936 | `submit`, `skillPromptKnobs`, `classify`, `workerPrompt`, `work`, `fastTurn`, `slashCommand` | about 245 | `tui_prompt.go` |
| 937 to 1168 | `interrupt`, `cancel`, `handleControl`, status, pid, exit, marker, `colored` | about 235 | kept in `tui.go` or `tui_state.go` |

A pure move within the package. No API change. `tui.go` ends near 500 to 700 lines.

### Package cut (R): the fake Codex

| Package | Files (src/test) | Src lines | Test lines | Tests |
|---|--:|--:|--:|--:|
| `testutil/agentfake` | 23 (11 / 12) | 3,526 | 2,199 | 95 |
| `testutil/agentfake/codex` | 6 (3 / 3) | 1,168 | 857 | 24 |

`codex` holds `codex.go, codexhooks.go, codexinput.go` and `codex_test.go, codexhooks_test.go,
codexdesign_test.go`. Direction: `agentfake/codex -> agentfake`. The `agentfake` package keeps its
name, so the 42 leaf-layer symbols that importers use (`Control`, `Action*`, `WorkerConfig`, ...) do
not change.

What the cut costs:

- Export about 28 names from `agentfake` that `codex*.go` uses today: `inputBox`, `newInputBox`,
  `syncBuffer`, `designState`, `pendingWork`, `tuiEvents`, `ptyClosed`, `draw`, `ctrlC`, `escKey`,
  `waitFor`, `waitScreen`, `waitStatus`, `holdProgram`, `programDir`, `programReadyPath`,
  `programConversationPath`, `openProgramChannel`, `newConversationID`, `testDir`,
  `isDesignPrompt`, `designSlowTime`, `designTurnTime`, `turnScript`, `writeAppReport`,
  `logSubmitted`, and from the leaf layer `commitCount` and `workerEnv`. That widens a `testutil`
  fixture's API, which is acceptable. The cleaner shape is a small `agentfake/tuikit` package for
  these shared pieces (O6); it is a bigger first step and not needed for the limits.
- `probe.go` reads `CodexEnv`, defined in `codex.go`. Move the constant into `probe.go`.
  `codex.go` also calls `runCodexProbe`, so export it or move it.
- `cmd/fakecodex/main.go` changes from `agentfake.TUICodex()` to `codex.TUICodex()`.
- Tests: the codex tests are internal today and use TUI helpers. After the cut they use the
  exported names above.

### Importer-change cost

The `codex` symbols used outside are `CodexEnv`, `CodexArgsPath`, `CodexIDEnv`, `TUICodex`, the
`Codex*Env` hook switches and `ReadCodexHookLog`. 10 importer files: `nativerunner` 3 (tests),
`server` 3, `servertest` 1, `codexdoctor` 1, `gxfixture` 1, `cmd/fakecodex` 1. 96 files import
`agentfake` today.

### Optional cuts

- O6: the `agentfake/tuikit` package above.
- O7: if the codex cut is not wanted, the other cut that fits is the leaf layer (`agentfake.go,
  claude.go, worker.go`, 737 lines). It gives 3,957 source lines but touches 64 importer files
  (54 of them in `server`), because it holds the shared `Control` and `Action*` names. Cutting
  `worker.go` (274 lines) alone gives 4,420 lines (98% of the limit), which is too tight.

## Limits after the cuts

| Package | Files | Src lines | Test lines | Tests | Largest type |
|---|--:|--:|--:|--:|---|
| `tickets` | 47 | 4,399 | 3,492 | 168 | below 50 methods |
| `tickets/ticketlog` | 12 | 872 | 803 | 31 | |
| `tickets/ticketmove` | 7 | 850 | 498 | 21 | |
| `tickets/ticketspec` | 14 | 1,229 | 419 | 20 | |
| `tickets/ticketmigrate` | 22 | 1,683 | 466 | 12 | |
| `nativerunner` | 44 | 3,477 | about 4,863 | 140 | `Pty` 48 (44 with O4a) |
| `nativerunner/holder` | 13 | 2,521 | about 1,765 | 52 | `HolderClient` 28 |
| `git` | 39 (49 without O) | 2,354 | 2,805 | 134 | below 50 methods |
| `git/pr` | 10 | 847 | 534 | 24 | |
| `git/stage` | 8 | about 766 | 940 | 45 | |
| `git/history` (O) | 10 | about 676 | 1,075 | 44 | |
| `testutil/agentfake` | 23 | 3,526 | 2,199 | 95 | `tui` 29 |
| `testutil/agentfake/codex` | 6 | 1,168 | 857 | 24 | `codexFake` 36 |

`nativerunner/pty.go` and `testutil/agentfake/tui.go`, the two files over 1000 lines, are both fixed
by moving whole functions between files of the same package.

## Open questions

- Package names. `ticketlog`, `ticketmove`, `ticketspec`, `ticketmigrate` and `holder` are working
  names. A name that echoes the directory (`tickets/log`) clashes with the standard `log` in
  importers, hence the prefix.
- The 5 new run-helper exports in `git` become public API. If that is unwanted, `git/internal/run`
  with aliases avoids it, at the price of two spellings.
- The `tickets` core uses 98% of the source-line limit after the cuts. If the guard needs more
  margin, the only candidates are O1 (162 importer files) and turning `Epic` methods into functions
  (an API break). Neither is recommended.
- Does the guard count methods from test files too? It makes no difference for these four packages.
