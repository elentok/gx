## Problem Statement

The gx Go code has grown into a few very large units, and people and agents pay for it every day.

- `server` has 512 files, about 35k source lines, about 52k test lines and 1,570 tests. `server.Server` alone has 708 methods. `cmd` has 259 files and 899 tests. `ralphloop` has 153 files and 653 tests.
- Nine more packages are over a sensible size: `tickets`, `ui/tickets`, `nativerunner`, `git`, `testutil/agentfake`, `ui/status`, `ui/worktrees`, `ui/agents` and `ui`.
- 22 files are over 1000 lines. 11 types have more than 50 methods (`server.Server`, eight `ui/*` models, `apiclient.Client`, `nativerunner.Pty`).
- A change in a huge package forces a long build and test run, an agent has to read a lot to find the one file it needs, and nothing stops the next feature from adding to the pile.

## Solution

Split every oversized package, file and type with no behaviour change, then add a guardrail test that keeps them small.

- `server`, `cmd` and `ralphloop` become a thin root plus focused subpackages. The nine other oversized packages are split too.
- Every file over 1000 lines is cut. Every type over 50 methods becomes a small type that owns child components.
- A new Go test package, in the style of `envguard` and `playwrightguard`, measures every package, file and type against fixed limits. It fails with a message that tells the agent to split, not to raise the limit. Its exception list starts empty.
- The split runs as one implementation epic. Behaviour, routes, wire formats and the generated TypeScript types do not change.

## User Stories

1. As a gx developer, I want `server` split into focused subpackages, so that I can change one subsystem without reading or rebuilding the rest.
2. As a gx developer, I want the wire types in their own package, so that the types that `cmd`, `ui` and the web UI share do not pull in the whole server.
3. As a gx developer, I want `tygo` to keep generating a byte-identical `webui/src/generated/server.ts`, so that the web UI does not notice the move.
4. As a gx developer, I want each server subsystem (sessions, web auth, health, disk, limits, epics, merge, scheduling and the rest) to be its own type that owns its state, so that state has one owner and not 708 methods on one struct.
5. As a gx developer, I want each subsystem to reach the server only through a small interface, so that I can see and test what it needs.
6. As a gx developer, I want each subsystem to expose its own routes, so that adding a route touches the subsystem and not one giant table.
7. As a gx developer, I want the root `server` package to only wire subsystems together and run the process shell, so that startup and shutdown are easy to read.
8. As a gx developer, I want a test that fails if the `Serve` shutdown order changes, so that I can refactor the root without breaking the 13 background loops, the merge guard, the store flush and the lock release.
9. As a gx developer, I want `sessions`, `sched`, `merge` and `projects` each to hold two or more types, so that no single type passes 50 methods.
10. As a gx developer, I want the test-only seams of `server.Server` exported on each subsystem type, so that tests move with the code they test.
11. As a gx developer, I want the helpers in `server/servertest` cut by topic, so that no helper file is over 1000 lines and the server split has a smaller diff.
12. As a gx developer, I want `cmd` split into one package per command group, so that I can find and change a command in a small package.
13. As a gx developer, I want each command package to take a small `Deps` that embeds a shared base, so that a command lists exactly what it needs.
14. As a gx developer, I want each command package to export one constructor that the root registers, so that the command tree is explicit and not built by `init()` side effects.
15. As a gx developer, I want `gx tickets` to keep working as one command whose verbs come from two packages, so that users see no change.
16. As a gx developer, I want the CLI-verb parity test to stay in the `cmd` root, so that a missing verb for a route still fails.
17. As a gx developer, I want the dead `ralphloop` `Run` cluster deleted before the split, with the tests that reach the live finish path ported and not dropped, so that I do not move code nobody calls.
18. As a gx developer, I want `ralphloop` split into subpackages with the root kept as the iteration lifecycle, so that scheduling, merging, verification, supervision and notification each live in a small package.
19. As a gx developer, I want the `ralphloop` test helpers in two shared support packages, so that moved tests keep their fixtures and fakes.
20. As a gx developer, I want `apiclient.Client` split into sub-clients that share one transport, so that a tab depends on the sub-client it uses and its fake shrinks.
21. As a gx developer, I want each `ui/*` model with more than 50 methods split into child components, so that no model is a god object.
22. As a gx developer, I want `ui/tickets` split into child packages, so that it meets the file, line and test limits without an exception.
23. As a gx developer, I want `ui/gitops` and `ui/commitops` as shared packages, so that the status, log and commit tabs reuse one implementation of the git actions they now each carry.
24. As a gx developer, I want `nativerunner.Pty` and `nativerunner` split, so that the pty type is at most 50 methods and the holder code is its own package.
25. As a gx developer, I want `tickets`, `git` and `testutil/agentfake` split into packages along their real seams, so that each is under the limits.
26. As a gx developer, I want every file over 1000 lines cut by topic, so that I can read a whole file.
27. As a gx developer, I want the `gxfixture` `run()` function refactored into a small parse step plus helpers, so that its file is under the limit and the function is readable.
28. As a gx developer, I want a guardrail test that measures files, source lines, test lines and `Test` funcs per package, lines per file and methods per type, so that no package grows past the limits again.
29. As a gx developer, I want the guardrail failure message to name the package, file or type, the measure, the limit and the size found, say to split and not to raise the limit, and point at `.ai/index.md`, so that an agent that trips it knows what to do.
30. As a gx developer, I want the guardrail exception list to start empty and only shrink, so that nothing hides behind it.
31. As a ralph-loop agent, I want each move ticket to stay small (about 60 files or fewer, lifted only for a `server/wire` group that cannot be cut smaller) and keep the tree green, so that one agent session can finish it.
32. As a ralph-loop agent, I want each ticket to update the path-keyed guards (`sleepguard`, `oldwords`, `envguard`, `agentrunner` herdr list) in the same ticket, so that no guard fails after a move.
33. As a gx developer, I want `.ai/index.md` (package map, adding a route, file size, testing) and `scripts/test.sh` updated with each package they describe, so that the docs and CI match the tree.
34. As a gx developer, I want the full test time measured before and after the split, so that I know what the split did to it.
35. As a gx user, I want gx to behave exactly as before, so that the refactor costs me nothing.
36. As a maintainer, I want any behaviour bug found during the split filed as a follow-up, so that the refactor stays a pure move.
37. As a gx developer, I want `ui/status` (`stagemodal`), `ui/worktrees` (`modal`, `launch`), `ui/agents` (`terminal`, `create`, `jump`, `agentstyle`) and the root `ui` (`fuzzy`, `refrules`, `cmdoutput`, `pushstate`) each split into child packages under the limits, so that no `ui` package stays a pile.

## Implementation Decisions

### Rules for the whole epic

- No behaviour change: no route, wire format, log output or runtime behaviour changes. A bug found on the way becomes a follow-up in `gx:follow-ups`. Loggers get no new `.With("pkg", ...)` fields.
- New packages live in subdirectories of their parent. Tests move with the code they test. Importers are rewritten in the ticket that moves the code, with `gopls rename`. There are no alias shims.
- Package, type and file names below are working names. The implementer picks the final names, keeping the structure. Where a name would clash with a domain package, the root uses an import alias.
- Ticket size: one package per ticket, and one type per ticket for `sched`. A type split done in place that a package move needs is its own ticket and runs first. A ticket that would touch more than about 60 files is split further (a guide, not a hard limit). The `server/wire` move is already cut into one ticket per group of wire files; the guide is lifted only when a group cannot be cut smaller. `tickets` gets one ticket per new package.
- Cross-package copies of the same logic (go-to-PR in five packages, split-layout math in four) are not cleaned up. Each becomes a follow-up. A copy that a new shared child replaces is removed.
- The final package and file lists are checked against the guardrail limits with a count script when each ticket is written. If a measured package ends over a limit, the implementer makes the next cut (the optional cuts named below) in the same ticket.

### Guardrail

- A Go test in its own package, next to `envguard` and `playwrightguard`. It reads tracked files and measures with `go/ast`.
- Default limits for every package: 50 files (source plus test), 4,500 source lines, 5,000 test lines, 180 `Test` funcs. Lines per file: 1000, and test files count. Methods per type: 50.
- What is measured:
  - Tracked `.go` files only, grouped by directory.
  - `_test` packages count in their directory. `package main` is included.
  - A `Test` func is a top-level `func Test*` with a `*testing.T` parameter. Benchmarks, fuzz tests, examples and subtests do not count.
- Method counting: declared methods per receiver type. Promoted methods are ignored. Pointer, value and generic receivers are unwrapped. Methods in test files do not count. Test-only types do not count toward the type limit through this rule, because their methods live in test files.
- Exception list: the key is a package path, a file path or `pkg.Type`. The value is `{limit, reason}`, with the reason taken from a closed list that the test checks. The list starts empty. An entry is allowed only when a split proves infeasible.
- Entries only shrink. A limit above the current size fails with "lower the limit". An entry under the default fails with "drop the entry".
- The failure message names the package, file or type, the measure, the limit and the size found, tells the reader to split and not to raise the limit, and points at `.ai/index.md`. It emits one `t.Error` per problem.
- The guardrail lands last, blocked by every split ticket. The exact exception-list type, key syntax and the closed list of reasons are the implementer's choice, within the rules above.

### `server`

- `server/wire` is created first. It takes the 167 wire types and 373 constants, including the `Reason*`, `Event*`, `Verdict*`, `Origin*` and `Outcome*` constants. The four hybrid wire types (`TicketInfo`, `QueueRequest`, `RepairRequest`, `CreateSessionRequest`) are split in place before the move. `Config` and `Settings` stay out of `wire`.
- The move into `wire` is several tickets, one group of wire files each. Every `include_files` entry in `tygo.yaml`, and every wire constant, belongs to exactly one group. Each ticket moves the types and constants of its group and rewrites only their importers. There are no alias shims. The ~60-file guide is lifted only when a group cannot be cut smaller. The five groups:
  - Sessions: `sessionautoadvance`, `sessionchat`, `sessioncommands`, `sessionfailure`, `sessionfocus`, `sessionkeys`, `sessionpermission`, `sessionprompt`, `sessionround`, `sessionscreen`, `sessionstate`, `sessions`, `sessiontypes`.
  - Tickets: `detail`, `detailrelated`, `detailruns`, `epiccard`, `epicdetail`, `epiclist`, `epicsummary`, `index`, `pause`, `verdicts`, `recentlyfinished`, `designs`, `migrate`.
  - Health and disk: `healthreport`, `healthtypes`, `diskcleannow`, `diskwatch`, `checkstatus`, `restart`, `restartcheck`.
  - Limits and stats: `limitburn`, `limitsstore`, `limitsevents`, `usagesample`, `usagesummary`, `stats`, `statsgrilling`, `statslimitburn`, `budget`.
  - Other: `fixagent`, `harnesslabel`, `projectworktrees`, and the constants (`Reason*`, `Event*`, `Verdict*`, `Origin*`, `Outcome*`) that no other group's types use. A constant goes with the group of the type it belongs to.
  - The first wire ticket checks the list against `tygo.yaml` and the constants, and moves any file or constant that is in no group into the right one. The implementer may move a file between groups if the types require it, but each stays in exactly one.
- `tygo` keeps the wire file names in its `include_files` lists and one output file, `webui/src/generated/server.ts`. Wire types keep literal values, and cross-package types are mapped in `tygo.yaml` `type_mappings`. `git diff` must show no `any`.
- `tygo` in the middle of the chain: each group ticket adds or extends a `server/wire` package entry in `tygo.yaml` (and drops the moved files from the `server` entry). The first group ticket finds out whether `tygo` can write two packages to one output file.
  - If it can, `server.ts` stays byte-identical after every group ticket.
  - If it cannot, the generated TypeScript is written to more than one file in the middle of the chain. The per-ticket check is then a sorted diff of the TypeScript declarations across all generated files, before and after. The last group ticket returns to one `server.ts` that is byte-identical to the file before the epic.
- 26 subpackages in total, `wire` first, in this layer order (a package imports only earlier layers):
  - L0: `wire`, `broker`.
  - L1: `webtls`, `settings`, `runs`, `index`, `projects`.
  - L2: `queue`, `harness`, `webauth`, `disk`, `limits`.
  - L3: `budget`, `health`, `stats`, `sessions/chat`.
  - L4: `sessions`, `ticketwrite`, `recovery`, `merge`, `epics`, `designs`.
  - L5: `sessions/prompt`, `ticketops`.
  - L6: `sched`.
  - L7: `web`.
  - The root keeps `New`, `Serve`, route assembly, snapshot, SSE, handshake and the process shell.
- `stats` is its own package. `ticketPause`, `ticketNudge` and `ticketRelaunch` move into `ticketwrite`.
- Each subsystem is its own type that holds the state it owns and gets a small interface back to the server. The root creates and connects them.
- Hooks back to the root: `Applier` (settings), `OnRescan` (index), `Prompter` (sessions), `Source` (chat), `Launch` (ticketops). The root registers them.
- Shared state keeps one owner: `adoptMu`, the merge guard, trunk locks and refusals live in `runs`. `reloadMu` is passed in, not copied.
- `sessions`, `sched`, `merge` and `projects` each get two or more types so that none passes 50 methods. `sched` moves one type per ticket.
- Non-wire names that `cmd` uses:
  - `SocketPath`, `LogPath`, `LockHeldBy` and `DefaultTCPAddr` stay in root `server`.
  - `AgentEnv`, `PersistedActive` and `EnsureOutboxProject` go to `projects`.
  - `CPUSpikeDir` goes to `health`, `IsOneOffEpic` to `queue`, `RetroSubmit` to `ticketwrite` and `RunRepairDirect` to `ticketops`.
  - `cmd` imports the owner packages.
- Routes: each subsystem exposes `Routes() []Route` (pattern, handler, web flag). The root concatenates them and replaces `routeTable`, `webWrites` and `webOnlyRoutes`. `RoutePatterns()` stays in root. The "Adding a server route" section of `.ai/index.md` is rewritten to match.
- The test-only seams that `server.Server` has today (about 49 shims) are exported on each subsystem type, in the same ticket as the move.
- A new test fails when the `Serve` shutdown order changes (13 loops, then the merge guard, the store commit flush, then the lock release). It is the first ticket of the `server` chain, blocked by the wave-0 `servertest` cut, and comes before `server/wire`, so it guards every later move.
  - The probe is a test-only shutdown recorder owned by the root and set through `export_test.go`. `Serve` calls it for each step. Log output is not the probe.
  - A later move calls the recorder from the new owner of the step. It never edits the test's expected order.
- Move order inside `server` is a serial chain. The `servertest` file cut is in wave 0 only and is not part of the chain. The chain starts with the shutdown-order test, then `server/wire` (one ticket per group), packages in layer order, `sched` per type, then the root cleanup.
- `scripts/test.sh`: the server step runs `./server/...` and the generic Go step skips the same pattern.

### `cmd`

- Three shared packages: `cmdbase`, `ticketref` and `cmdtest`.
  - `cmdbase` holds `Env` (the 9 common fields), `ExitError`, the dial and server-read helpers, provenance and JSON output helpers.
  - `ticketref` holds ticket and project resolution.
  - `cmdtest` holds the cross-package test fixtures.
- 19 command packages: `tickets`, `ticketsflow`, `ticketsmigrate`, `serverctl`, `serverapi`, `project`, `limits`, `agent`, `claude`, `codex`, `evolve`, `config`, `tui`, `gitops`, `skills`, `doctor`, `brief`, `check` and `term`.
  - `tickets` keeps the core edit verbs: add, set, show, close, section, move, log, epics, root, validate, schema, ping, alias.
  - `ticketsflow` takes the workflow verbs: epic add, publish, follow-up, the ensure verbs, verify-plan, calibration, lint-claims, filter-run-log and finish-guard.
  - `gx tickets ask` and `gx tickets answer` live in `serverapi`. The root adds them to the `tickets` tree.
  - `serverweb` stays inside `serverapi`. It splits only if the guardrail flags `serverapi`.
  - The small packages (`codex`, `brief`, `check`, `term`) stay unmerged.
- Each package defines its own small `Deps` that embeds `cmdbase.Env` and adds only its own hooks. Fields become exported. The roughly 300 `deps{...}` literals in tests get a mechanical rename.
- Registration: each package exports `New(Deps) *cobra.Command`. A tree that spans packages takes the other packages' children as `extra ...*cobra.Command`. There is no `init()` self-registration.
- `lint-claims` gets a `Deps.CommandTree func() *cobra.Command`, supplied lazily by the root.
- `liveProbe` is exported as `serverctl.LiveProbe`. `tui` imports `serverctl` for it. `cmdbase.Env` keeps its 9 common fields and gets no probe field. `projectlock.SetWaitHook` stays global as before and is set once in root `cmd`. Tests that override it use a `cmdtest` helper.
- The root `cmd` keeps `Execute`, `newRootCmd`, the help groups, `gx recovery`, `gx version`, `routeVerbs`, `TestRouteTableHasCLIVerbs` and the tests that need the whole tree. It keeps `type ExitError = cmdbase.ExitError`, so the program entry point is untouched.

### `ralphloop`

- The dead `Run` cluster is deleted first, as the first `ralphloop` ticket: `Run`, `RunOptions`, `Permit`, `runIteration`, `reattachIteration`, the reconcile code (keeping the path-reconcile and latest-cherry-pick helpers), the spin and retry-storm and resume code, the merge-queue worker, `ChannelEventSink`, `EpicFailureReporter` and the legacy sinks. About 2.5k source lines and about 220 tests go. The 31 tests that reach the live finish path through `Run` are ported to call the finish functions directly, not dropped.
- Eleven subpackages: `agent`, `runlog`, `ticketops`, `metrics`, `deps`, `worktree`, `schedule`, `notify`, `merge`, `verify` and `supervise`. No optional second cuts are planned. If one package ends over a limit, the implementer makes the cut the boundary research names (`notify/gate`, `supervise/compact`, `land`).
- The root `ralphloop` stays as the iteration lifecycle that the server drives: `OneIteration`, `IterationWorktree`, the Prepare/Resume/Discard/Finish verbs and `WaitIterationFinished`. The long `loop` and `iteration` source files disappear. The root gets a finish file and a conflict file.
- Exported names stay (`AgentKind`, `Deps`, `Event`), so `deps.Deps` stutter is accepted. `LatestPauseEvent` moves to `runlog`, `LatestMergedSHA` to `merge`, `Gate` and `StallNudgeText` to `supervise`. The two cycle-closing edges are cut as the boundary research describes. Unexported names that tests use across package lines are exported, or the test moves.
- Two test-support packages: `ralphloop/internal/fixture` (file and transcript helpers) and `ralphloop/internal/fakes` (fake deps, runner and recording sink). Until `deps` has moved, the shared helpers are extracted into a root test-helper file.
- `agentrunner/imports_test.go` allowlists `ralphloop/deps` instead of `ralphloop`.

### Large types outside `server`

- `apiclient.Client` becomes six sub-clients (`Sessions`, `Tickets`, `Admin`, `Reads`, `Queue`, `Web`) that share one transport. Callers are rewritten to `c.Sessions.Stop(...)`. There is no embedding. Tabs take only the sub-client they use, so `SessionClient`, `ServerAPI`, `ServerClient` and their fakes shrink. About 100 call sites in `cmd` and `ui` change. This runs in wave 3, after the `server` chain.
- The eight `ui/*` models (`status`, `agents`, `commit`, `tickets.QueueModel`, `tickets.Model`, `worktrees`, `log`, `app`) split into child components. The leaf pattern is `Update(msg) (Child, tea.Cmd, Result)` plus `View(width)`. `ui/worktrees` uses a `modal` interface with one type per mode. Each model ends at 50 methods or fewer.
- `ui/gitops` and `ui/commitops` are new packages, like `ui/amend` and `ui/bump`.
- `nativerunner.Pty` gets an unexported `ptyRegistry` and helper types (launcher, turn driver, usage reader) and ends at 50 methods or fewer. The pty tests run under `-race`.
- A child package never imports its parent. The type split happens before the package move.

### `ui` packages

- `ui/tickets` is split into child packages:
  - `rowfmt` (leaf: metrics, pause labels, pause reasons, ticket status, live row, tree icon click).
  - `preview` (the preview pane).
  - `rowactions` (row menus, the row editor, the actions behind menu picks, the watch modal).
  - `queue` (`QueueModel` and its children).
  - `tabapi` (the exported names that `ui/app` and `ui/agents` use: `ServerAPI`, `ServerLink*`, the messages, queue epic and status names, `QueuedToast`, `ScratchDir`, `WatchStore`). It removes the dependency of `ui/agents` on the whole tickets tab. About 60 references change.
  - `sidebar` (the sidebar tree), because the root would otherwise sit at 93% of the test-line limit.
  - Not planned: `queue/header`. The root `ui/tickets` keeps `Model`.
- `View()` snapshot tests for a few states of `ui/tickets` are added before the render code moves. Shared test helpers move to a test-helper package under `ui/tickets` before tests move.
- `ui/status`: shared `ui/gitops` plus a stage-modal child package. The 3,154-line model test file is cut by theme into 7 or 8 files. About 19 tests that drive the whole `Model` are rewritten, not moved; their assertions stay.
- `ui/worktrees`: `modal` and `launch` child packages.
- `ui/agents`: `terminal`, `create` and `jump` child packages, with a small `agentstyle` leaf as a prerequisite. The sidebar stays in the root.
- `ui` (root): `fuzzy`, `refrules`, `cmdoutput` and `pushstate` move out.
- `ui/settings` and `ui/term` do not move: they give no limit gain and are under the default limits.
- Optional cuts named in the research (`diffarea`, `filetree`, `tableview`) are made only if the guardrail flags the parent.
- Helper and leaf packages count against the same default limits as any other package.

### Core packages

- `tickets`: new packages `ticketlog`, `ticketmove`, `ticketspec` and `ticketmigrate` beside a core that keeps the model files. Two back edges are fixed first (`MigrationChange` and `clipRunes`). The cycle of 13 model files stays whole.
- `nativerunner`: a `holder` package. The `pty` launch group moves to its own file. The `holder` ticket adds `./nativerunner/holder` to `race_pkgs` in `scripts/test.sh`. Any later split of a `race_pkgs` package does the same for its new package. Optional `headless` and timings cuts are made only if the guardrail flags the parent.
- `git`: `git/pr`, `git/stage` and `git/history`. The five run helpers go in `git/internal/run`. `StagedFiles` moves next to the stage code. The `stash` test import is fixed.
- `testutil/agentfake`: the 1,168-line TUI file is cut into 4 or 5 files, and a `codex` child package takes the codex helpers (about 28 become exported).

### Long files

- Test files cut by topic, in the same package and with no dependency on a package split: `config`, `ui/diffview`, `ui/app`, `ui/log` and `ui/commit` model tests, plus `server`'s recovery test (cut with the recovery move) and the long `ralphloop` test files (cut with the `ralphloop` split).
- `testutil/gxfixture`: the main file loses `Output`, the env helpers, seed and the binary build. `run()` gets a real refactor (an argument-parse step) in its own ticket. The main test file is cut by topic. `scenarios` is cut with it. The test-only `running` type is not counted.
- `server/servertest` is cut by topic in wave 0, before the server split. The `server` chain starts with the shutdown-order test, which this cut blocks.
- `apiclient`'s client file is cut by endpoint group in wave 0. The sub-client split fixes the method count.

### Order of work

1. Wave 0, no package boundary change.
   - First, the before-records ticket. It blocks every other wave-0 ticket. It names the base commit SHA in the epic and captures every "before" record on that commit, before any other wave-0 ticket merges. It writes them in the epic:
     - the baseline test time (command in Verification, story 34);
     - the `RoutePatterns()` dump;
     - the `gx --help` and `gx tickets --help` output;
     - the tab fake sizes (method count plus line count per fake);
     - the `gxfixture` JSON keys;
     - the output and exit codes of a fixed list of `gx server` read verbs, and of `gx tickets add`, `show` and `close`, run on a fixed `gxfixture` scenario and a temporary store.
   - Then, in parallel: the five model-test cuts, the `gxfixture` cuts (including the `run()` refactor), the `servertest` file cut, the `apiclient` file cut, and the dead `ralphloop` `Run` deletion. The `Run` deletion ticket records the test names it deletes: the diff of `go test -list` before and after its own change.
2. `server`, as the serial chain above. Its first ticket is the shutdown-order test, blocked by the wave-0 `servertest` cut.
3. `apiclient`, the core packages (`tickets`, `git`, `nativerunner`, `testutil/agentfake`) and `ralphloop`, in parallel.
4. `cmd` and the `ui` packages, in parallel.
5. The guardrail, last, with an empty exception list. The guardrail ticket runs the whole suite once, including the flow catalog, and records the final test time. The verify ticket comes after it.

### Path-keyed guards

- `sleepguard`, `oldwords` and `envguard` path entries are updated in the same ticket as each move. Their rules do not change. A split file divides its count by where each call lands. A count never goes up. A new file with no hit needs no entry.
- `agentrunner/imports_test.go` lists each package that really imports `herdr`, by exact path. An old entry is dropped when no file in it imports `herdr` any more. There is no prefix rule. If a split leaves more `herdr` importers than before, the ticket says so, so a follow-up can shrink the list.
- `.ai/index.md` and `scripts/test.sh` are updated in the ticket that creates the package they describe.

## Testing Decisions

- A good test here checks external behaviour only. The epic changes no behaviour, so the existing tests are the spec. A moved test keeps its assertions. Tests are rewritten only where a type split forces it (for example, tests that drove the whole `ui/status` `Model`), and the assertions stay.
- Seams: the existing ones. The `routeTable` and CLI parity tests, the `servertest` harness, the `tygo` output and the existing per-package tests. New seams are a `View()` snapshot test for `ui/tickets`, a `Serve` shutdown-order test and the guardrail test itself.
- The guardrail test is tested with a table of fixtures: a package over each limit, a file over 1000 lines, a type over 50 methods, an exception entry above the current size, and an entry under the default. Each case checks the failure message text. It also runs against the real tree.
- Per ticket, run: `go build` and `go vet` on the changed packages and their importers, the tests of those packages and importers, the four path-keyed guard tests (`sleepguard`, `oldwords`, `envguard`, `agentrunner`), and, when wire types change, `make web-types` with the per-ticket tygo check (see the `server` section: a sorted diff of TypeScript declarations across the generated files, or a byte-identical `server.ts` when tygo can write two packages to one file). Do not run `go test ./...` for a local change. Do not run the full suite per ticket.
- The whole suite runs once, in the guardrail ticket (`make test`, `make web-e2e` for the flow catalog, `go test ./e2e/...` as CI does, and `gx check --full`), which records the final test time. The verify ticket does not rerun the suite. It runs every per-story check that is not part of the suite, plus a smoke check of a temporary server.
- Prior art: `envguard` and `playwrightguard` for the guardrail shape. The `server_parity_test` pattern for the route-to-verb check.

## Verification

The epic has no user-visible behaviour change. Verification is mostly build, vet, the guardrail and unchanged generated output.

- The guardrail ticket runs the whole suite once (the common checks below), including the flow catalog (`make web-e2e`), and records the final test time. The flow catalog runs only there.
- The verify ticket does not rerun the suite. It runs every per-story check below that is not part of the suite: layering lists, interface method counts, `Deps` embedding, no `init()`, `rg` for deleted symbols, `gitops` and `commitops` imports, `race_pkgs`, the 100-line count, the ticket-log review, the herdr list, the `.ai/index.md` contents, the follow-ups, and the before and after diffs against the records in the epic. For stories 7 and 35 it starts a temporary gx server, opens the web UI, runs a few CLI verbs as a smoke check, and points at the guardrail ticket's flow-catalog result.

Common checks, used by several stories (run once, in the guardrail ticket):

- `gx brief go-build ./...` and `gx brief go-vet ./...` pass.
- The guardrail test passes with an empty exception list.
- `make web-types` leaves `webui/src/generated/server.ts` byte-identical (`git diff --exit-code`).
- `gx check --full` passes (run in the background and polled).
- The flow catalog (`make web-e2e`) and `go test ./e2e/...` pass with no new or changed flows (guardrail ticket only).

Per story:

1. Each `server` subpackage exists. `go list ./server/...` lists the packages above, and `go list -deps` shows no package importing a later layer. Root `server` is under the guardrail limits.
2. `server/wire` holds the wire types, moved in one ticket per group (sessions, tickets, health and disk, limits and stats, other). Every `tygo.yaml` `include_files` entry and every constant is in exactly one group. No wire type is left in root `server` (checked with `go/types` or `rg` on the exported type declarations in root `server`, against the list of `include_files` types). `cmd`, `ui` and `apiclient` import `wire`, not the root, for wire names. `tygo.yaml` is valid after each group ticket.
3. After the last group ticket, `make web-types` output is one `server.ts`, byte-identical to the file before the epic. `git diff` of the generated file is empty and contains no `any`. After each earlier group ticket, the sorted diff of TypeScript declarations across the generated files is empty (or `server.ts` is byte-identical, if tygo can write two packages to one file).
4. No type in `server` or its subpackages has more than 50 declared methods (the guardrail checks this). `server.Server` has 50 or fewer methods and none is a route handler.
5. Each named hook (`Applier`, `OnRescan`, `Prompter`, `Source`, `Launch`) and each subsystem back-interface is a named interface with at most about 10 methods. `go vet` passes. Fakes for the interface exist in the tests that use them.
6. `TestRouteTableHasCLIVerbs` and `TestWeb_EverythingElseIs404` pass. `RoutePatterns()` returns the same set of patterns as before the epic (the verify ticket diffs a new dump against the before dump recorded in the epic). The `gx server` read verbs return the same output and exit codes as the before record in the epic, on the same `gxfixture` scenario.
7. A temporary gx server starts (verify ticket), serves the web UI and answers a few CLI verbs. The flow catalog result is the guardrail ticket's (`make web-e2e`), not rerun.
8. The new shutdown-order test passes and fails when the order of two shutdown steps is swapped (checked once by hand, not committed).
9. The guardrail reports no type over 50 methods in `sessions`, `sched`, `merge` or `projects`.
10. Server tests that used `export_test.go` seams pass in their new packages. `go test ./server/...` passes for the moved packages.
11. The guardrail reports no file over 1000 lines in `server/servertest`.
12. `go list ./cmd/...` lists the shared and command packages. The guardrail passes for `cmd` and every subpackage.
13. Each command package has a `Deps` that embeds `cmdbase.Env`. `go vet ./cmd/...` passes.
14. `gx --help` lists the same commands and groups as before (diff against the before output recorded in the epic). No command package has an `init()` that registers a command.
15. `gx tickets --help` lists the same verbs as before (diff against the recorded before output), including `ask` and `answer`. `gx tickets add`, `show` and `close` run on the same `gxfixture` scenario and a temporary store with the same output and exit codes as the before record in the epic.
16. `TestRouteTableHasCLIVerbs` passes from the `cmd` root.
17. The deleted `Run` cluster has no symbol left (`rg` for `RunOptions` and `Permit` finds nothing). The 31 ported tests are present and pass. The `ralphloop` test count drop is about the 220 tests of the deleted cluster, and the list of deleted test names is recorded in the epic by the `Run` deletion ticket itself (the diff of `go test -list` before and after its own change).
18. `go list ./ralphloop/...` lists the eleven subpackages. The guardrail passes for `ralphloop` and every subpackage. A ralph-loop run on a fixture epic reaches done (the ralph-loop flows in the guardrail ticket's flow-catalog run pass).
19. `ralphloop/internal/fixture` and `ralphloop/internal/fakes` exist. No test helper is duplicated across packages (reviewed in the ticket).
20. `apiclient.Client` has at most 50 methods. Callers use sub-clients (`rg` finds no call on the old flat methods). Fake size is the method count plus the line count per fake (`SessionClient`, `ServerAPI`, `ServerClient` fakes). It passes when each fake's method count is lower than the size recorded in the epic.
21. The guardrail reports no `ui/*` model over 50 methods.
22. `ui/tickets` and each child pass the guardrail. The `View()` snapshot tests pass unchanged before and after the render move. The tickets tab flows in the catalog pass (in the guardrail ticket's run).
23. `ui/gitops` and `ui/commitops` exist and are imported by `ui/status`, `ui/log` and `ui/commit`.
24. `nativerunner.Pty` has at most 50 methods. `go test -race ./nativerunner/...` passes. The guardrail passes for `nativerunner` and `nativerunner/holder`, and `race_pkgs` in `scripts/test.sh` lists `./nativerunner/holder`.
25. `tickets`, `git` and `testutil/agentfake` and their new packages pass the guardrail.
26. The guardrail reports no file over 1000 lines anywhere in the repo.
27. `gxfixture` starts every scenario (`ready`, `mixed`, `finishing`, `edge`) and prints the same JSON keys as the record in the epic. The file is under 1000 lines, and `run()` and each new helper are under 100 lines (checked with a count).
28. The guardrail test exists in its own package and its table tests pass.
29. The guardrail table tests assert the failure message contents: name, measure, limit, size, "split", and the `.ai/index.md` pointer.
30. The exception list in the guardrail is empty at merge. The table tests cover both entry rules.
31. Every move ticket in the epic lists the packages it tested. No ticket touched more than about 60 files without being split, except a `server/wire` group that could not be cut smaller (reviewed from the ticket log).
32. `go test ./sleepguard ./oldwords ./envguard ./agentrunner` passes after the last move. The `agentrunner` herdr list contains only packages that import `herdr`.
33. `.ai/index.md` lists every new package in the package map, has the new "Adding a server route" steps, and states the guardrail limits. `scripts/test.sh` runs `./server/...` and the generic step skips it. `make test` runs.
34. Test time before and after are measured with the same command on the same machine: `make test` plus `gx brief go-test -count=1 ./server/... ./cmd/... ./ralphloop/...`. The command runs alone, with no other suite and no `gx check` running at the same time. `gx check --full` runs before or after it, not during. They are written here, in this section:
    - Before: recorded by the before-records ticket on the epic's base commit, before any other wave-0 ticket merges. To be recorded.
    - After: recorded by the guardrail ticket. To be recorded.
    There is no target.
35. The flow catalog runs with no new or changed flow, once, in the guardrail ticket (`make web-e2e`), and `gx check --full` passes there. The verify ticket starts a temporary gx server, opens the web UI, runs a few CLI verbs as a smoke check, and points at the guardrail ticket's flow-catalog result.
36. Every behaviour bug found during the split has a follow-up in `gx:follow-ups` (`gx tickets epics` and the follow-ups listing; the ticket logs name them). No fix for them is in the epic's commits.
37. `ui/status`, `ui/worktrees`, `ui/agents` and the root `ui` each have the child packages named in the story. The guardrail passes for each package and each child. The golden files of `ui/agents` and `ui/app` keep their content: `git diff --exit-code -M100% <base>` passes, and each golden shows only as a 100% rename. No `-update` run happens in the epic.

New flows or scenarios to write: none. The existing flow catalog covers the regression check.

## Self-decided design questions

These were decided while writing the spec and in the first review. The git run helpers choice was confirmed by the person.

- Include `ui/tickets/tabapi` now? Yes. It removes the dependency of `ui/agents` on the whole tickets tab, and a later add would rewrite the same references twice.
- Include `ui/tickets/sidebar`? Yes. Without it the `ui/tickets` root would sit at 93% of the test-line limit and trip the guardrail on the next feature.
- Include `git/history`? Yes. It is a real seam in `git`, and `git` is over the limits without it.
- Leave `ui/settings` and `ui/term` out? Yes. They are under the default limits, so a split gives no gain.
- Do helper and leaf packages get looser limits? No. They count against the same default limits, so a helper cannot become the next pile.
- Accept test rewrites where a type split forces them? Yes. The assertions are kept, so the tests still act as the spec.
- Where do the git run helpers go? In `git/internal/run`. Exporting five helpers on the public `git` API is the quick fix and widens that API for no user gain.
- Does `projectlock.SetWaitHook` stay global? Yes. It is global today, and moving it into `Env` would change every test that overrides it for no gain. The root sets it once and tests use a `cmdtest` helper.
- How many wire groups? Five: sessions, tickets, health and disk, limits and stats, and other. The first plan had four, but about a dozen `tygo.yaml` files (`fixagent`, `harnesslabel`, `projectworktrees` and the loose constants) fit none of them. A fifth group keeps every file and constant in exactly one group.
- Where does the shutdown-order test sit? First in the `server` chain, after the wave-0 `servertest` cut and before `server/wire`. It then guards every later move, and the cut is not repeated in the chain.
- What is the `race_pkgs` rule? Any split of a package in `race_pkgs` adds the new package to `race_pkgs` in the same ticket. Otherwise a race test would quietly leave the race run.
- How is test time measured? One command, `make test` plus `gx brief go-test -count=1 ./server/... ./cmd/... ./ralphloop/...`, run alone on the same machine, with no other suite or `gx check` running. Parallel load would make the before and after numbers incomparable.
- What are the guardrail measurement rules? Tracked `.go` files only, `go/ast`, declared methods per receiver, no promoted methods, no test-file methods, `Test` funcs only. They are cheap to run, stable, and hard to game, and they match the style of `envguard` and `playwrightguard`.
- Why a 100-line limit for `run()` and its helpers? `run()` is the one function the epic refactors on purpose. A fixed, countable limit makes the check objective, and 100 lines fits one screen of parse code.
- Why "about 10 methods" per back-interface? It is a size where a reader sees the whole need and a fake stays short. It is a guide, not a guardrail rule, so a subsystem that needs 12 is not blocked.
- How does `tui` reach `liveProbe`? Export it as `serverctl.LiveProbe` and let `tui` import `serverctl`. Putting it in `cmdbase.Env` would break the 9-common-fields rule, and a copy in `tui` would duplicate the probe.

## Out of Scope

- Behaviour changes and bug fixes found during the split. Each becomes a follow-up in `gx:follow-ups`.
- A target test time. The split records the time before and after, nothing more.
- Splitting packages that are under the default limits and that the guardrail does not flag, including `ui/settings` and `ui/term`.
- Cleaning up cross-package duplicates (go-to-PR, split-layout math), except where a new shared child replaces a copy.

## Further Notes

- Research for each area is on branches `research/split-size-baseline`, `research/split-server-boundaries`, `research/split-cmd-boundaries`, `research/split-ralphloop-boundaries`, `research/split-large-types`, `research/split-core-packages`, `research/split-ui-packages` and `research/split-long-files`. The cuts there were read from function positions and were not compiled, so each ticket re-checks them with a count script and `go/types` before moving code.
- The tree can only be this large because `server`, `cmd` and `ralphloop` grew as flat packages. The guardrail is the lasting fix: without it, the same growth returns.
- `server` goes first because its wire-type move changes imports in `cmd`. Everything else follows in parallel waves.
- Package lists are the proposal from the research as adopted in the map's decision tickets. Names that the research marked as working names are the implementer's to finalise.
