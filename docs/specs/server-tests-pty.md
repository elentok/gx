# Server tests on the PTY runner

Map: `gx:server-tests-pty`. Related: `gx:pty-as-default` (the production default switch), which this
epic does not wait for.

## Problem Statement

`go test ./server` is slow and heavy. On main (`df5a8cb4`) a run takes about 223s wall on the 10-core
dev Mac, with 311s user and 889s sys CPU. It makes about 16,500 calls to the fake `herdr`, and each
call re-runs the 26MB test binary. Most of those calls are status polls (`agent get`, `agent wait`,
`workspace list`), and only about 250 start a session. Adding parallel slots makes the run slower,
because the run is limited by process spawning, not by waiting.

The cause is the `servertest` harness. It never sets `Config.Runner`, and a nil runner defaults to
herdr. So every server test, apart from the few that set their own runner, drives the herdr runner
through the fake `herdr` executable, even when herdr has nothing to do with what the test checks.
295 tests reach herdr this way, and only about 14 test herdr itself.

A developer running the suite waits minutes, and the machine is busy for all of it. A developer
writing a test has to script herdr fake handlers (`RegisterLaunch`, `registerLaunch`,
`h.Herdr.Register`) to get a working agent, even for a test about merging or recovery.

## Solution

The `servertest` harness runs the PTY runner by default. Each session costs one long-lived holder
process, and status comes over the holder's socket, so the 16,000 herdr execs become a few hundred
holder starts.

- A test that sets no runner gets PTY. The agent is the TUI fake. A runner-neutral turn hook,
  `h.OnTurn`, scripts the agent's turn the same way on PTY and herdr.
- A test that fakes a runner state, or checks only what the server passed to `Start`, picks the
  in-memory `runnerfake` with `servertest.WithRunner`.
- A test whose subject is herdr picks herdr with `servertest.WithHerdr(servertest.HerdrIsTheSubject)`.
  A small set of tests that wait on missing PTY features use
  `servertest.WithHerdr(servertest.UntilPtyAsDefault)` until `gx:pty-as-default` moves them.
- In a PTY or `runnerfake` test, any call to the fake `herdr` fails the test and names the command.
- The 102 serial tests become parallel, and their fixed sleeps become waits on the expected event.

Targets:

- Under 1,000 fake `herdr` calls per run of `go test ./server` (about 16,500 today).
- `go test ./server` in 2 minutes median on a quiet machine (the 10-core dev Mac).

The 2-minute target under agent load is not in this epic. It moves to the store polling effort
(`gx:follow-ups/126`) and the `git` effort.

## User Stories

1. As a developer, I want `go test ./server` to finish in about 2 minutes on a quiet machine, so
   that I run the whole suite before each commit.
2. As a developer, I want the suite to make fewer than 1,000 fake `herdr` calls, so that the run
   spends its CPU on the server code it tests, not on re-running the test binary.
3. As a developer, I want a server test that sets no runner to run on PTY, so that new tests use
   the runner gx is moving to without extra setup.
4. As a developer writing a test, I want one turn hook, `h.OnTurn(func(*Turn))`, that works on PTY
   and herdr, so that I script an agent's turn without knowing the runner's wire protocol.
5. As a developer writing a test, I want `Turn` to give me the ticket address, the worktree and the
   prompt text, so that the agent's turn can commit in the right worktree and check the prompt.
6. As a developer writing a test, I want `Turn.Stall()`, `Turn.Block(reason)` and `Turn.Exit(code)`,
   so that I can make a real PTY agent stall, block on a permission, or exit.
7. As a developer, I want a turn control that herdr can't do to fail the test with a clear message,
   so that I never get a test that passes without doing what it says.
8. As a developer writing a reclaim test, I want a harness helper that leaves a live or dead PTY
   run behind, so that `Restart` reclaims it the way a real server restart does.
9. As a developer, I want `Restart` on a PTY harness to build a fresh PTY runner, so that reclaim
   goes through `Reattach` as it does in production.
10. As a developer writing a test that fakes a runner state, I want `servertest.WithRunner(r)`, so
    that I hand the harness a `runnerfake` without hand-written `c.Runner` and `c.RunnerFor`
    closures. For a test about per-run routing (which runner or project a run goes to), I want
    `servertest.WithRunnerFor(configured, fn)`.
11. As a developer writing a herdr test, I want `servertest.WithHerdr(servertest.HerdrIsTheSubject)`,
    so that the test says in code that herdr is its subject.
12. As the developer who builds `gx:pty-as-default`, I want each test that waits on codex routing or
    PTY `Find` to be marked `servertest.WithHerdr(servertest.UntilPtyAsDefault)`, so that deleting
    that constant makes the build fail on every test I still have to move.
13. As a developer, I want any fake `herdr` call in a PTY or `runnerfake` test to fail the test and
    name the herdr command, so that a stray herdr dependency shows up at once.
14. As a developer, I want the fake `herdr` to stay first on the server's PATH in every test, so that
    a stray call never reaches a real herdr on my machine.
15. As a developer, I want `h.Herdr` to be nil unless the test passed `WithHerdr`, so that a PTY test
    can't register herdr handlers by mistake.
16. As a developer, I want the `servertest` package doc to state when a test uses PTY, `runnerfake`
    or herdr, so that I pick the runner for a new test without asking.
17. As a developer, I want `runnerShapes` to run its cases on PTY and its `headless` shape, so that
    the shared cases cover PTY and a `runnerfake` reattach verdict.
18. As a developer, I want the herdr branch of reclaim to keep its coverage through the herdr-subject
    tests (the restart tests, the lookup-error test and one test per reclaim outcome), so that moving
    `runnerShapes` off herdr loses no coverage.
19. As a developer, I want the codex start-option tests on `runnerfake`, so that they check what the
    server passed to `Start` without a codex agent on any runner.
20. As a developer, I want the R5 and undeliverable-prompt recovery tests on `runnerfake` and the two
    R7 tests on the PTY turn fixture, so that each test runs where its subject is.
21. As a developer, I want the disk tests to wait on a runner-neutral launch signal, not on herdr
    `agent start` argv, so that they run on PTY.
22. As a developer, I want a failing `TestDiskCleanNow_UnheldDiskKeepsDispatching` to fail, not hang
    the whole run, so that one bad test can't eat my test timeout.
23. As a developer, I want the harness's PTY runner to read an in-process process table on macOS
    and Linux, so that no process lookup in a server test spawns `ps`.
24. As a developer, I want each seeded run in a test to name its runner, so that an empty runner
    never loads as herdr and calls the fake `herdr`.
25. As a developer, I want every server test that can run in parallel to call `t.Parallel`, so that
    the serial tests stop adding a fixed floor to the run.
26. As a developer, I want each fixed sleep in a server test to become a wait on the event it
    expects, so that a test takes as long as the server needs, not a fixed guess.
27. As a developer, I want an opt-in fake `herdr` call counter, so that I can check the herdr call
    target on my own machine.
28. As the person who runs the verify ticket, I want one measurement protocol, so that the time and
    herdr targets are checked once and a miss becomes a follow-up, not more tuning rounds.

## Implementation Decisions

### The runner rule

The `servertest` package doc states the rule. Every test follows it.

- **PTY** is the default. It is for a test that needs a real agent turn.
- **`runnerfake`** is for a test that sets a runner state directly: blocked, a pending permission, a
  rate limit, a prompt or answer, start or stop errors, a reattach verdict, a background task. It is
  also for a test that checks only what the server passed to `Start`. Reason: such a test checks the
  server's reaction, and `runnertest` conformance already covers each runner against the same
  contract.
- **herdr** only when herdr is the test's subject, or as `UntilPtyAsDefault` while a PTY feature is
  missing.

### Harness runner choice

- A test picks its runner with options on the existing start functions: `Start`, `StartWithStore`,
  `StartWithTCP` and `StartHerdrDown`. `Start` and `StartWithTCP` take options like the others. No
  new start functions.
- The runner options only record a choice. The harness resolves the runner after all options run,
  in one fixed order, so the order of options in a test never matters:
  1. `WithRunner` or `WithRunnerFor` sets `cfg.Runner` and `cfg.RunnerFor`.
  2. `WithHerdr` turns on the fake herdr. It makes herdr the configured runner only when step 1 set
     nothing.
  3. The PTY default builds both fields the production way only when neither is set yet.
- The harness builds `cfg.Runner` and `cfg.RunnerFor` the way production does. `serverRunner`'s
  build-by-name logic (`cmd/server_preflight.go`) moves into a package the harness can import, and
  `cmd` calls it from there. So the harness gets one `nativerunner.Pty` per project with
  `Root: AgentsRoot(stateDir, project)`, as production does. The implementer picks the package.
  This move changes no production behaviour.
- The shared builder takes:
  - the env. It uses it for herdr's CLI (`herdr.CLI{Env: env}`), `HOME`, the codex home and
    `Pty.Env`.
  - `N`, the runner wait.
  - an optional PTY override: the holder program, the agent program and `Procs`.

  Production passes its process env and no override. The harness passes `cfg.Env` (so the fake
  `herdr` is first on PATH), `RunnerWait`, and an override where the holder is the test binary, the
  agent program is the TUI fake (`agentfake`, `GX_AGENTFAKE_TUI`) and `Procs` is the harness
  process table. So a herdr run in a PTY test reaches the fake `herdr` and fails the test, never the
  real one.
- The harness builds the runners again on each `serve`, so `Restart` reattaches like a real restart.
- `servertest.WithRunner(r)` sets one runner for every run: `cfg.Runner` is `r` and `cfg.RunnerFor`
  returns `r`. It replaces every hand-written `c.Runner = fake` and single-fake `c.RunnerFor = …`
  closure in the server tests.
- `servertest.WithRunnerFor(configured, fn)` is for tests about per-run routing: it sets
  `cfg.Runner` to `configured` and `cfg.RunnerFor` to `fn`. Its users include
  `TestRunner_LaunchesAndRecordsThroughTheConfiguredRunner` (it records the `project` argument) and
  `restartUnder` in `reclaim_route_test.go` (it maps runner names to different runners).
- `servertest.WithHerdr(reason)` turns on the fake herdr: `h.Herdr` is set and fake `herdr` calls are
  allowed. It makes herdr the configured runner only when no other option set `cfg.Runner`. So
  `TestReclaim_HerdrRunUnderAHeadlessServerFinishesThroughHerdr` combines it with a renamed fake as
  the configured runner. `reason` is a typed constant with two values: `HerdrIsTheSubject` and
  `UntilPtyAsDefault`. `gx:pty-as-default` deletes `UntilPtyAsDefault`.
- `StartHerdrDown` implies herdr. Its callers are herdr-subject tests and pass
  `WithHerdr(HerdrIsTheSubject)`; the implementer picks whether `StartHerdrDown` sets it itself.
- `h.Herdr` is nil unless the test passed `WithHerdr`.
- The fake `herdr` stays first on the server's PATH in every test. In a test without `WithHerdr`,
  every call fails the test (through `t.Errorf` or equivalent from the fake's coordinator) and the
  message names the herdr verb and noun.

### The turn hook

- `h.OnTurn(func(*Turn))` replaces `RegisterLaunch`. All callers, and the server package's
  `registerLaunch` wrappers, are renamed in one mechanical change before any test moves to PTY.
- `Turn` has `Address`, `Cwd` and `Text`, the fields `Prompt` has today, plus the turn controls
  `Stall()`, `Block(reason)` and `Exit(code)`.
- The hook function is the agent's turn. A commit is a `git` commit in `Turn.Cwd` inside the hook.
  The turn finishes when the hook returns, unless a control said otherwise.
- On PTY, the hook runs in the test process. Then the harness drives the TUI fake through its side
  channel (`SendTUIControl` and the control, input and ack files) to end the turn, stall, block or
  exit. The TUI fake applies a turn to slash-command prompts too, since a launch prompt is a `/`
  command.
- On herdr, the hook works as `RegisterLaunch` does today. A control herdr can't do fails the test
  with a message that names the control and says the test should run on PTY.
- The hook also covers what the classification found tests need: holding a turn, failing or counting
  starts, counting or blocking stops, and writing a transcript. The implementer picks the method
  names on `Turn` or `Harness` for these.

### Leftover runs for reclaim tests

- A `Harness` method next to `OnTurn` leaves a live or dead PTY run for a reclaim test. The
  implementer picks the name.
- A live run is a real holder and TUI fake that the next `Restart` reattaches. A dead run is a
  `runs.json` entry and an agent directory with no live holder.
- Every helper that seeds a run (`stampRun`, `PutRun`, `leaveIteration` and the like) names the
  runner. An empty runner loads as herdr, so in a PTY test it would call the fake `herdr` and fail.
- `runnerShapes` runs its subtests as `pty` and `headless`. `runnerOf` stamps `pty`. There is no
  third herdr shape. The `headless` shape stays what it is today: a `runnerfake` (`headlessFake`,
  runner name `fake`), not `nativerunner.Headless`.

### Process table

- The table is harness-only. It lives in `servertest`, with darwin and linux build files. The
  harness gives the PTY runner this in-process `ProcessTable` (`Pty.Procs`): sysctl
  (`kern.proc.pid`, `kern.procargs2`) on macOS and `/proc` on Linux, since CI runs both. No PTY
  process lookup in a server test spawns `ps`.
- Its start time format only has to match itself. Production keeps `psTable`: the `pid_start`
  values in stored agent metas use `ps`'s `lstart` format, and another format would make live agents
  look dead at reclaim. Replacing `ps` in production is a follow-up that must handle the stored
  `lstart` values. The verify ticket files it.
- The remaining `ps` calls from `harness/claudepid.go` stay as they are.

### No production change for herdr probes

The server already probes only the configured runner at startup and in the 30s watch, and each live
run's own runner at reclaim. A PTY server never probes herdr. The harness switch alone removes the
probes. No server code, health view or UI changes for this.

### Disk test hang

The first change in this epic, before the harness switch: the fake cleaner in the disk-clean tests
unblocks on test cleanup. A failing `TestDiskCleanNow_UnheldDiskKeepsDispatching` then fails, and
`Server.Stop` no longer waits on its open request.

### Where each named test goes

**herdr, `HerdrIsTheSubject`.** These test herdr health, herdr's own reclaim branch, or values only
herdr fills:

- `TestHandshake_ReportsHerdrUnavailable`
- `TestHerdr_DownAtStartIsReportedAndRecoveryStreamsOneEventEach`
- `TestPauseFold_HerdrOutagePausesBecomeOneDigest`
- `TestReclaim_HerdrLookupErrorNeverReopens`
- `TestReclaim_HerdrDeadEmptyClaimReopens`, a herdr-subject version of `TestReclaim_DeadEmptyClaimReopens` (herdr's not-found →
  `reopenEmptyClaim` path in `server/runs.go` `reclaim`)
- `TestReclaim_HerdrLaunchingIdleAgentIsStoppedThenReopens`, a herdr-subject version of
  `TestReclaim_LaunchingIdleAgentIsStoppedThenReopens` (herdr's
  launching-and-waiting → `stopLaunching` path)
- `TestReclaim_HerdrRunUnderAHeadlessServerFinishesThroughHerdr`
- `TestRecoveryRunner_LiveCheckAgreesAcrossShapes` (its herdr rows)
- `TestRunner_RestartPausesAHandleWhoseWorktreeIsGone`
- `TestRunner_RestartReclaimsTheLiveIterationAndItFinishes`
- `TestLaunchPort_RestartReattachesAndMergesOnce`
- `TestReads_IterationsAndQueue` (it asserts `IterationInfo.Pane`, which only herdr fills)
- `TestBudget_HardLimitStopsLivePaneAndPausesBudgetKilled` (it asserts herdr's `ctrl+c` keys and tab
  close; `TestBudget_HardLimitStopsRunThroughRunner` covers the server side)

Once `runnerShapes` leaves herdr, these cover the herdr reclaim branch (`server/runs.go`): the three
restart tests cover resume (`Find` by label), `TestReclaim_HerdrLookupErrorNeverReopens` the lookup
error, and the two new herdr-subject reclaim versions cover the not-found and launching outcomes.

**Already on `runnerfake` or a renamed fake, unchanged.** `TestHerdr_ProbedThroughConfiguredRunner`
and `TestHerdr_RunOnANonConfiguredHerdrIsNotProbed` keep their current fakes and assertions. The
second one matches the production decision (no herdr watch for a live herdr run under a non-herdr
server), so it does not flip. `TestHerdr_NotProbedForRunnerWithoutHost` checks probe gating; under
the PTY default the failing fake `herdr` enforces "no probe", so it needs no `WithHerdr`.

**herdr, `UntilPtyAsDefault`.** These wait on codex routing or PTY `Find` for an agent no run
tracks. `gx:pty-as-default` moves them:

- `TestRunner_LaunchSkillFollowsTicketType` (codex)
- `TestRunner_CodeReviewTicketLaunchesUnderTheCodeReviewSkill` (codex)
- `TestLaunchPort_FailureAfterClaimPausesStuck` (codex)
- `TestFeatureBranch_FirstClaimCreatesItAtTheResolvedBase` (launches codex)
- `TestFeatureBranch_AnExistingBranchIsAdoptedAndItsMergeBaseRecorded` (launches codex)
- `TestNudge_TypesIntoLivePaneLogsAndRateLimits` (`Find`)
- `TestBudget_NudgedPausedPaneIsMeteredAndKilled` (`Find`)

**`runnerfake`.**

- The codex start-option tests: `TestRunner_ClaimsWriteTheFileBeforeTheEventAndLaunchWithTheChosenAgent`,
  `TestRunner_CodexEvolveLaunchGetsTheFollowUpsAndGxDirs`,
  `TestRunner_OneOffsRunClaudeAndCodexUnderOneCap`,
  `TestRunner_OneOffWithoutAKnownAgentUsesTheQueueItemAgent`. Each asserts the `Start` options
  (kind, add-dir flags) that `runnerfake` records.
- `TestRecovery_StalledPromptIsRetypedInFullByR5`. It stays on `runnerfake` until the R5 fix
  (`gx:follow-ups/128`) rewrites it on the PTY fixture.
- `TestRecovery_UndeliverablePromptClosesThePane` (it injects a prompt error).

**PTY turn fixture.**

- `TestRecovery_TimedOutCompactionIsReWaitedOnceThenFinishedUpByR7`: check the fake agent got the
  finish-up prompt once and never `/compact`.
- `TestRecovery_SecondCompactionTimeoutLeavesThePauseToAPerson`: keep the agent working past a short
  `CompactRewaitTimeout`.
- The five disk tests that wait on herdr `agent start` argv through `startCleanRig`:
  `TestDiskClean_HoldsWhileCleaningThenResumesWithOneCleanNotice`, `TestDiskClean_NoCleanInNormalUse`,
  `TestDiskHold_ResumeStartsTicketAndNotifiesOnce`, `TestDiskHold_CleanThresholdOffNeverHolds`,
  `TestDiskCleanNow_UnheldDiskKeepsDispatching`. They wait on a start the turn hook records.
- `TestFinish_ZeroCommitPauseRecordsRun`, `TestReclaim_LaunchingWorkingAgentIsALiveRun` and
  `TestRelaunch_StartsAFreshIterationOfAPausedTicket`. Their subject is the server's finish, reclaim
  and relaunch logic. Their herdr values (runner `herdr`, session `s1`, pane `p1`, herdr argv) become
  the PTY values (runner `pty`, the session label, the turn hook's record).

**The other harness-gap tests from the spike.** The implementer sorts each by the runner rule: a
test that sets a runner state or injects a runner error goes to `runnerfake`, and a test that needs a
real turn or a live run goes to the PTY fixture. The spike's reading, as a starting point:

- `runnerfake`: `TestReclaim_NonMismatchErrorPausesStuckWithItsReason`,
  `TestRunner_AFinishThatErrorsPausesTheTicketStuckAndFreesTheRoot`,
  `TestMergeTicket_FailedLaunchKeepsTheFeatureWorktree`.
- PTY fixture: `TestCancel_LiveTicketRefusesUntilStop`, `TestLateFinish_MergesLateCommit`,
  `TestMergeTicket_PauseKeepsTheFeatureWorktreeAndReclaimReusesIt`, the herdr shapes of
  `TestReclaim_DeadEmptyClaimReopens`, `TestReclaim_LaunchingIdleAgentIsStoppedThenReopens` and
  `TestReclaim_LaunchingWithNoAgentReopens` (through `runnerShapes`),
  `TestRecovery_R3CommitlessDoneIsProposedAndOnlyApprovalMarksItDone`,
  `TestRecovery_ZeroCommitPauseMatchesOnItsTranscriptText`,
  `TestRunner_BackfillsTheNextQueuedRootWhenASlotFrees`, `TestRunner_MergesTheIterationAndClosesTheRoot`,
  `TestSchedulingPort_PauseThenResumeReusesBranchAndMergesBothCommitsOnce`,
  `TestStop_WaitsForAMergeInFlightAndFlushesTheStoreCommit`.

A test that moves to the other side than listed says why in its commit message.

**Everything else** runs on the PTY default with `OnTurn`, unchanged in what it asserts. This
includes the 208 tests that use only the herdr launch fake and the 23 that use the fake `herdr` only
to answer the startup probe (those drop the fake). The 36 tests the classification flagged as
queueing or restarting on the default runner without herdr handlers now start a real holder if they
launch. The implementer checks each one at the switch.

### Serial tests

- The 102 top-level tests without `t.Parallel` call it. They are mostly the disk, health and
  worktree-removal tests.
- Each fixed sleep (`letWatcherRead` and the like) becomes a `servertest.WaitFor` on the event the
  test expects. A negative check ("nothing happens") waits on a positive event that comes after the
  point where the unwanted one would show, where the server offers one; otherwise it keeps a short
  bounded sleep with a comment.
- `startHealthRoute` calls `t.Setenv` for `HOME` and `XDG_STATE_HOME`, which `t.Parallel` forbids.
  The harness already puts both in `Config.Env`. The helper drops `t.Setenv`. If a server code path
  reads them from the process env instead of `Config.Env`, that path is fixed to read `Config.Env`.
- No test is known to need to stay serial. A test the implementer must leave serial gets a comment
  that names the shared process state, and the verify ticket's report lists it.

### herdr call counter

`testutil/herdrfake` gets a lasting opt-in counter: an env var naming a file, and each fake `herdr`
exec appends one line (`herdr <verb> <noun>`). The implementer picks the env var name. It replaces
the spike's throwaway counter. Off by default.

### Ticket order

1. Disk test hang fix.
2. `RegisterLaunch` to `OnTurn` rename, still on herdr.
3. Moving `serverRunner`'s build logic into a shared package, `WithRunner`, `WithRunnerFor` and
   `WithHerdr`, the in-process process table, the PTY default, the failing fake
   `herdr`, the leftover-run helper and the turn controls, with the test moves.
4. Serial tests to parallel.
5. The herdr counter can land at any point before verify.

The implementer may split step 3 by test file, as long as the suite stays green after each ticket.

## Testing Decisions

- A good test here checks the server's behaviour through the API client, the run registry and the
  files the server writes. It does not check how the harness talks to the TUI fake.
- The `servertest` harness changes get their own tests in the `servertest` package or
  `server` package: `OnTurn` on PTY commits in `Turn.Cwd` and the ticket merges; each turn control
  reaches the expected runner state on PTY; a herdr-unsupported control fails a test with the
  message (checked through a fake `testing.TB` or a subprocess test); a fake `herdr` call in a PTY
  test fails with the verb and noun in the message; `h.Herdr` is nil without `WithHerdr`;
  `WithHerdr` combined with a `WithRunner` fake keeps the fake as the configured runner and allows
  fake `herdr` calls; each order of `WithHerdr` and `WithRunner` or `WithRunnerFor` resolves the
  same runner; a herdr-named run in a PTY test reaches the fake `herdr` and fails the test with the
  verb and noun; a PTY harness with two projects builds one `Pty` per project root; the
  leftover-run helper's live run reattaches as live after `Restart` and its dead run reclaims as
  dead.
- The in-process process table gets a test on each OS that it finds the test process's own pid and
  start time, run with the existing `nativerunner` process-table tests as prior art.
- The herdr counter gets a `herdrfake` test that two calls write two lines.
- Prior art: `server/servertest` and the server tests that already use `runnerfake`
  (`liveUnder`, `newRenamedFake`), `runnertest` conformance for the runner contract,
  `testutil/agentfake` TUI tests for the side channel, and the spike branch
  `spike/server-tests-pty` (`server/servertest/ptyrunner.go`, throwaway code that shows one working
  wiring).

## Verification

This epic has no user-visible behaviour in the web UI, the CLI or the server API. The users are
developers running `go test ./server`. The verify ticket runs these checks in the epic's worktree,
with no fixture server. No flow-catalog flows apply and no new flows or scenarios are needed.

Measurement protocol (stories 1, 2, 28): on the 10-core dev Mac, run `go test -count=1 ./server/`
three times interleaved with three runs of main at the epic's base as a baseline, on a quiet machine.
Report wall, user and sys medians from `/usr/bin/time -l`, and the herdr call count from the counter.
Record one more run under agent load. The verify ticket measures once. On a missed target it records
the cause and files a follow-up instead of tuning.

1. The quiet-machine median wall time of `go test -count=1 ./server/` is 120s or less; exit code 0.
2. With the herdr counter env var set, one run writes fewer than 1,000 lines.
3. A new throwaway test that calls `servertest.Start(t)` and launches a ticket shows `pty` as the
   run's runner in the run registry. `rg -n 'cfg.Runner|c.Runner =' server --glob '*_test.go'`
   finds no hand-written runner assignment outside `WithRunner`.
4. `rg -n 'RegisterLaunch' server` finds nothing; `OnTurn` has callers. The `OnTurn` harness test
   passes on PTY and on herdr.
5. The `OnTurn` test asserts `Address`, `Cwd` and `Text` on PTY.
6. The turn-control harness tests pass: `Stall()` leaves the run working with no turn end,
   `Block(reason)` shows a pending permission, `Exit(code)` ends the session.
7. The herdr-unsupported-control test passes and its message names the control.
8. The leftover-run helper tests pass for live and dead runs.
9. A PTY restart test (`TestRunner_*Restart*` on PTY, or the helper test) reclaims through
   `Reattach`; the server log names the PTY runner for the reclaimed run.
10. `rg -n 'RunnerFor *=' server --glob '*_test.go'` finds no hand-written closure outside the
    harness. Per-run routing closures appear only as `WithRunnerFor` arguments. The harness test
    shows two projects get two `Pty` runners with different roots.
11. `rg -n 'HerdrIsTheSubject' server` lists exactly the tests in the herdr-subject list.
12. `rg -n 'UntilPtyAsDefault' server` lists exactly the seven tests in that list. Deleting the
    constant in a scratch edit makes `go vet ./server/...` fail at each of them.
13. The stray-call harness test passes and its message names the herdr verb and noun.
14. In a PTY test, the server's `Config.Env` PATH starts with the fake `herdr` directory (checked in
    the harness test).
15. The harness test asserts `h.Herdr == nil` without `WithHerdr`.
16. `go doc ./server/servertest` shows the runner rule.
17. `go test -run 'TestReclaim_' -v ./server/` lists `pty` and `headless` subtests and no `herdr`
    subtest for `runnerShapes` tests.
18. `go test -run 'TestRunner_Restart|TestLaunchPort_Restart|TestReclaim_Herdr' ./server/` passes on
    herdr, including the two new herdr-subject reclaim tests.
19. `go test -run 'TestRunner_ClaimsWrite|TestRunner_CodexEvolve|TestRunner_OneOff' ./server/` passes
    and makes no herdr calls (counter file empty for that run).
20. `go test -run 'TestRecovery_' -v ./server/` passes; the counter shows no herdr calls.
21. `go test -run 'TestDisk' ./server/` passes with no herdr calls.
22. With a scratch edit that makes `TestDiskCleanNow_UnheldDiskKeepsDispatching` fail, `go test -run
    TestDiskCleanNow ./server/` exits 1 within its own timeout and does not hang.
23. A run with a PATH wrapper that logs `ps` shows no `ps -ww -p` call. The process-table test passes
    on macOS, and CI's Linux job passes it too. `rg -n 'Procs' <shared build package> cmd` finds
    nothing, so production still uses `psTable`. The harness table's files live only under
    `server/servertest`. The follow-up for a production table is filed.
24. `rg -n 'stampRun\(|PutRun\(' server --glob '*_test.go'` shows a runner on every seeded run; the
    stray-call check (story 13) catches any empty one at run time.
25. The serial-test count script from the spec research (top-level `Test` functions in `server/`
    with no `t.Parallel`) prints 0, or only tests with a comment naming the shared state. The
    verify report lists any left.
26. `rg -n 'letWatcherRead|time\.Sleep' server --glob '*_test.go'` shows only bounded sleeps with a
    comment, each in a negative check.
27. The `herdrfake` counter test passes; with the env var unset, no file appears.
28. The verify ticket's report has the medians table, the under-load run, and either "targets met"
    or the follow-up it filed.

## Out of Scope

- Cutting the `git` calls in the server tests (about 28,500 to 33,000 per run). Different cause,
  different fix.
- The store polling fix (`gx:follow-ups/126`): an incremental index, a cached `project.json`, and
  claims woken on index changes. It owns the under-load 2-minute target.
- herdr e2e tests (`make e2e`, `e2e/`).
- Switching the production default runner to PTY (`gx:pty-as-default`).
- Codex routing by kind and PTY `Find` for an agent no run tracks. Both are in `gx:pty-as-default`,
  which moves the `UntilPtyAsDefault` tests.
- The R5 and R7 recovery fixes (`gx:follow-ups/128`, `gx:follow-ups/129`).
- Watching herdr health for a live herdr run under a non-herdr server.
- A smaller holder binary. One exec costs 4 to 7ms of CPU, so it would save only a few seconds.
- Raising the harness `PollInterval` to 1s.
- Replacing `ps` in production with an in-process process table. It needs a follow-up that handles
  the stored `lstart` values.
- More `-parallel` slots. `-parallel 20` made the run slower and flakier.

## Further Notes

- Spike data, method and per-test causes: `docs/research/server-tests-pty-spike.md` on branch
  `spike/server-tests-pty`. Classification table: `docs/research/server-tests-herdr-coupling.md` on
  branch `research/server-tests-herdr-coupling`. Recovery notes:
  `docs/research/server-tests-pty-recovery.md` on branch `research/server-tests-pty-recovery`.
- The spike measured 160s median and 2,040 herdr calls with 42 tests kept on herdr. It estimated
  about 95s quiet and 130s loaded with the PTY turn fixture, codex routing and parallel serial tests.
  This epic leaves codex routing out, so the quiet estimate is a little higher. The 2 `TestFeatureBranch_*`
  tests on herdr avoid their 10s codex wait on PTY.
- Tickets 04 and 07 count 11 tests on `UntilPtyAsDefault`: 3 codex, 2 `TestFeatureBranch_*` and 6
  `Find` tests. Tickets 05 and 06 then place four of the six `Find` tests elsewhere (R5 and
  undeliverable prompt on `runnerfake`, the two R7 tests on the PTY fixture), and ticket 06 says
  those moves stand. So the list here has seven tests. The two R7 tests no longer need `Find`
  because the PTY fixture launches their agent through the server.
- The classification note that `TestHerdr_RunOnANonConfiguredHerdrIsNotProbed` must flip is
  superseded by ticket 08: production keeps today's probe rule, and the test stays as it is.
- `TestSessionScreen_LiveReturnsHolderRows` failed once under load in the spike. It does not use the
  default runner. If it flakes in verify, file a follow-up.
- The four recovery tests seed the run log and call `PauseAs` by hand, so they test the remedy, not
  whether production reaches it. The R5 and R7 follow-ups rewrite them to drive the real stall.
