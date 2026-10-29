# Spike: the server tests on the pty runner

Ticket `gx:server-tests-pty/01`. Branch `spike/server-tests-pty`, based on main `df5a8cb4`. Throwaway
code, never merged.

## Answer

- With pty as the default runner, 42 top-level tests fail. 7 need codex, which the pty runner
  cannot start. 11 assert on herdr itself. 24 drive a live agent through herdr fake handlers and
  need a pty way to do the same.
- With those 42 tests kept on herdr ("pty-dest"), the suite is green. Median wall time is 160s
  against 223s for main on the same machine in the same session (-28%). System CPU is 548s against
  889s (-38%). herdr fake calls drop from 16,495 to 2,040.
- Both targets are missed. 2 minutes was reached once (124.5s, in the quietest round). The herdr
  call target (<1,000) needs most of the 24 harness-gap tests moved to pty.
- The pty runner is not the cost. Holders and TUI fakes take about 15 CPU-s per run. Most of the
  rest is the test process itself (in-process servers polling the store every 100 to 200ms, and
  their GC) and about 28,500 `git` calls.

## Method

- Machine: 10-core Mac (darwin/arm64), go 1.27.1. Other agents shared the machine, and its load
  average went from about 9 to 17 during the session. Runs of one kind vary by up to 50% because of
  this, so the three configs ran interleaved: base, pty-all, pty-dest, three rounds.
- Command per run: `/usr/bin/time -l go test -count=1 -json ./server/`, killed after 600s. Wall,
  user and sys come from `time`. They include the build (cached) and vet, and every reaped child.
- Baseline: a second worktree at `df5a8cb4` plus only the counting commits (no runner change).
- Counting: each re-run of the test binary appends one line to `$GX_SPIKE_COUNT_FILE`
  (`testutil/spikecount`). The herdr fake helper writes `herdr <verb> <noun>` and its harness
  socket. Holders, TUI fakes and fake programs write their kind. No counter existed for the old
  ~16,600 figure, so this one counts each exec of the fake `herdr` once, which is the same unit.
- `git` and `ps` were counted in separate untimed runs with a PATH wrapper that logs each call and
  its CPU. The wrapper adds an exec per call, so those runs are not used for timing.
- CPU per process kind: each test-binary process writes its own `getrusage(RUSAGE_SELF)` at exit.
- Profile: one pty-dest run with `-cpuprofile` (the test process only).

### Harness changes on the branch

- `server/servertest/ptyrunner.go`: the harness builds a `nativerunner.Pty` per `serve` (so
  `Restart` reattaches like a real restart). The holder is the test binary (`servertest-agent-holder`),
  claude is the TUI fake, the env is the server's env plus `GX_AGENTFAKE_TUI=1`.
- Process table: `sysctlProcs` reads `kern.proc.pid` and `kern.procargs2` in process. The count
  runs saw no `ps -ww -p` call, so the runner made no `ps` calls. The 9 to 15 `ps` calls left come
  from `harness/claudepid.go` (`ps -A`) and `ps -o lstart=`, as on main.
- `RegisterLaunch` works on pty: the wrapper runs the test's agent func in process, then arms a fast
  turn on the TUI fake. The TUI fake now applies a fast turn to slash prompts too.
- `StopAll` stops every holder under the runner root, so cleanup does not wait out `WaitSettled`.
- Opt-outs: `GX_SPIKE_HERDR=1` keeps herdr for all tests. `GX_SPIKE_HERDR_TESTS=<file>` keeps
  herdr for the listed top-level tests (the pty-dest config).

## Results

Medians of the three interleaved rounds:

| config | wall s | user s | sys s | herdr calls | holders | TUI fakes | fake programs | top-level fails |
|---|---|---|---|---|---|---|---|---|
| base (main) | 223.4 | 310.9 | 889.4 | 16,495 | 303 | 0 | 297 | 0 |
| pty-all | 272.2 | 172.1 | 401.4 | 46 | 561 | 258 | 293 | 41 (+1 skipped, hangs) |
| pty-dest (42 tests on herdr) | 160.5 | 205.7 | 548.1 | 2,040 | 524 | 221 | 295 | 0 |

`git` calls (separate count runs): base 26,831 and 29,488, pty-dest 28,539. The pty change does not
touch them.

Paired by round, pty-dest against base:

| round | base wall | pty-dest wall | base sys | pty-dest sys |
|---|---|---|---|---|
| 1 | 150.2 | 124.5 (-17%) | 544.6 | 442.9 (-19%) |
| 2 | 223.4 | 160.5 (-28%) | 889.4 | 548.1 (-38%) |
| 3 | 226.5 | 174.0 (-23%) | 913.5 | 583.8 (-36%) |

pty-all is slower on the clock than base because failing tests sit out their timeouts (10s to 30s
each, `TestRunner_LaunchSkillFollowsTicketType` alone takes 150s). Its CPU is the lowest.

### Raw runs

```
run            wall   user    sys     herdr  holder tui program git    ps  fails exit
base-t1        150.2  200.88  544.58  13857  303    0   301     -      -   0     0
base-t2        223.4  310.91  889.44  16495  303    0   294     -      -   0     0
base-t3        226.5  311.17  913.46  16534  303    0   297     -      -   0     0
base-count     170.4  255.03  643.96  14504  303    0   301     26831  6   0     0   (PATH wrapper)
base-cnt2      237.2  359.83  903.63  16819  303    0   298     29488  6   0     0   (PATH wrapper)
base-cpu       214.4  290.54  927.84  15473  303    0   298     -      -   1     1   (flake: TestDerivedActionable_RunEndAfterGrilling)
ptyall-t1      312.9  140.24  334.67  46     561    258 302     -      -   41    1
ptyall-t2      249.3  172.12  401.44  48     561    258 290     -      -   41    1
ptyall-t3      272.2  189.64  508.94  46     562    259 293     -      -   41    1
ptydest-t1     124.5  153.03  442.93  1755   524    221 298     -      -   0     0
ptydest-t2     160.5  205.71  548.13  2040   526    223 295     -      -   0     0
ptydest-t3     174.0  223.64  583.81  2177   524    221 295     -      -   0     0
ptydest-p10ctl 165.3  218.29  558.97  1952   524    221 301     -      -   0     0
ptydest-cpu    229.3  253.30  611.10  2304   527    224 296     -      -   1     1   (flake: TestFirstPrompt_PersonInputAfterTypingStopsTheNudge)
ptydest-cnt2   205.0  280.61  689.35  2113   524    221 297     28539  15  0     0   (PATH wrapper)
ptydest-p20    284.9  390.07  1251.1  3823   519    216 296     -      -   18    1   (-parallel 20)
base-p20       356.0  469.66  1890.2  24358  303    0   294     -      -   38    1   (-parallel 20)
ptydest-p32    65.6   62.02   132.65  802    183    33  149     -      -   4     1   (-parallel 32, aborted by a runnerfake panic)
```

herdr calls by verb, base-t1: `agent get` 9,370, `agent wait` 1,860, `workspace list` 1,434,
`workspace create` 262, `tab create` / `agent start` / `agent prompt` 251 each. 243 top-level tests
make herdr calls other than the `workspace list` probe; 203 of them pass on pty unchanged.

## Failing tests on pty

Each line names the test and what it needs. "herdr assertion" means the test checks herdr argv,
herdr trace, pane ids or herdr health. "harness gap" means the test makes a live or dead agent
through herdr fake handlers (`h.Herdr.Register`, `registerLaunch`, `stampRun(..., "herdr")`,
`PutRun` with runner `herdr`) and pty has no fixture for that yet.

### Missing pty feature: codex (7)

The pty runner ignores `StartOptions.Kind` and always starts claude (`ptyHarness` in
`nativerunner/pty.go`). A codex launch dies before `holder.sock` answers. This also affects
production once pty is the default (`gx:pty-as-default`): nothing routes codex tickets to another
runner.

- `TestRunner_ClaimsWriteTheFileBeforeTheEventAndLaunchWithTheChosenAgent`: wants an agent of kind codex.
- `TestRunner_CodexEvolveLaunchGetsTheFollowUpsAndGxDirs`: checks codex `--add-dir` args.
- `TestRunner_LaunchSkillFollowsTicketType`: checks codex `$gx-...` prompts (150s of timeouts).
- `TestRunner_CodeReviewTicketLaunchesUnderTheCodeReviewSkill`: codex prompt.
- `TestRunner_OneOffsRunClaudeAndCodexUnderOneCap`: a codex one-off.
- `TestRunner_OneOffWithoutAKnownAgentUsesTheQueueItemAgent`: a codex queue item.
- `TestLaunchPort_FailureAfterClaimPausesStuck`: counts codex `agent start` calls.

`TestFeatureBranch_*` (2 tests) pass but launch codex, so each waits its full 10s context on pty
(0.5s on herdr).

### herdr assertion (11)

- `TestHandshake_ReportsHerdrUnavailable`: herdr health. Stays on herdr by design.
- `TestHerdr_DownAtStartIsReportedAndRecoveryStreamsOneEventEach`: herdr health. Stays on herdr.
- `TestBudget_HardLimitStopsLivePaneAndPausesBudgetKilled`: wants herdr `send-keys ctrl+c` and a tab close.
- `TestFinish_ZeroCommitPauseRecordsRun`: wants runner `herdr` and session `s1` in the pause event.
- `TestReclaim_LaunchingWorkingAgentIsALiveRun`: wants pane `p1` in runs.json.
- `TestRelaunch_StartsAFreshIterationOfAPausedTicket`: reads herdr `agent start`/`agent prompt` argv.
- `TestDiskClean_HoldsWhileCleaningThenResumesWithOneCleanNotice`: waits on herdr `agent start` argv (`startCleanRig`).
- `TestDiskClean_NoCleanInNormalUse`: same.
- `TestDiskHold_ResumeStartsTicketAndNotifiesOnce`: same.
- `TestDiskHold_CleanThresholdOffNeverHolds`: same.
- `TestDiskCleanNow_UnheldDiskKeepsDispatching`: same, and then hangs forever: its fake cleaner
  never unblocks after the failed wait, so `Server.Stop` waits on the open request. Skipped in the
  pty-all runs.

The 5 disk tests are serial and wait 15s each before failing, so they add about 75s to the
pty-all wall time.

### Harness gap (24)

- `TestBudget_NudgedPausedPaneIsMeteredAndKilled`: fakes a live herdr agent; nudge finds no live iteration.
- `TestCancel_LiveTicketRefusesUntilStop`: `PutRun` with runner herdr; cancel returns 500.
- `TestNudge_TypesIntoLivePaneLogsAndRateLimits`: fakes a live herdr agent.
- `TestReads_IterationsAndQueue`: fakes a live herdr agent; iterations list is empty.
- `TestLateFinish_MergesLateCommit` (herdr shape): live agent faked in herdr.
- `TestLaunchPort_RestartReattachesAndMergesOnce`: run stamped as herdr, then restart.
- `TestMergeTicket_FailedLaunchKeepsTheFeatureWorktree`: makes the launch fail through herdr handlers.
- `TestMergeTicket_PauseKeepsTheFeatureWorktreeAndReclaimReusesIt`: herdr-shaped reclaim.
- `TestReclaim_DeadEmptyClaimReopens` (herdr shape): `stampRun("herdr")`.
- `TestReclaim_LaunchingIdleAgentIsStoppedThenReopens` (herdr shape): herdr-shaped agent.
- `TestReclaim_LaunchingWithNoAgentReopens` (herdr shape): `stampRun("herdr")`.
- `TestReclaim_NonMismatchErrorPausesStuckWithItsReason`: herdr lookup error injected.
- `TestRecovery_R3CommitlessDoneIsProposedAndOnlyApprovalMarksItDone`: turn scripted in herdr handlers.
- `TestRecovery_SecondCompactionTimeoutLeavesThePauseToAPerson`: counts herdr `agent wait` calls.
- `TestRecovery_StalledPromptIsRetypedInFullByR5`: stall faked in herdr; reads typed text from herdr.
- `TestRecovery_TimedOutCompactionIsReWaitedOnceThenFinishedUpByR7`: same, for R7.
- `TestRecovery_UndeliverablePromptClosesThePane`: same, wants a tab close.
- `TestRecovery_ZeroCommitPauseMatchesOnItsTranscriptText`: turn scripted in herdr handlers.
- `TestRunner_AFinishThatErrorsPausesTheTicketStuckAndFreesTheRoot`: the finish error comes from a herdr handler.
- `TestRunner_BackfillsTheNextQueuedRootWhenASlotFrees`: agent turns scripted in herdr.
- `TestRunner_MergesTheIterationAndClosesTheRoot`: the commit happens in a herdr prompt handler.
- `TestRunner_RestartReclaimsTheLiveIterationAndItFinishes`: live herdr agent across a restart.
- `TestSchedulingPort_PauseThenResumeReusesBranchAndMergesBothCommitsOnce`: turns scripted in herdr.
- `TestStop_WaitsForAMergeInFlightAndFlushesTheStoreCommit`: merge started from a herdr handler.

The 4 R5/R7/undeliverable recovery tests may also hide a real difference: the pty runner nudges and
retypes inside `Prompt`, so the server's own R5/R7 remedies may never see the stall. This spike did
not check that.

`TestSessionScreen_LiveReturnsHolderRows` failed once under load in the first pty run and passed in
every later run and alone. It does not use the default runner; it is a timing flake.

## Where the time goes

### CPU by process kind

From the `getrusage` lines (user / sys seconds). "git" is from the PATH-wrapper runs.

| kind | base user / sys | pty-dest user / sys |
|---|---|---|
| test process (servers, test code) | 47 / 469 | 38 / 350 |
| `git` (~28,500 calls) | 176 / 222 | 168 / 213 |
| herdr fake helpers | 74 / 89 (16,809 calls) | 9 / 11 (2,112 calls) |
| holders | 3 / 3 (303) | 7 / 6 (518) |
| TUI fakes and fake programs | under 1 | under 1 |

- The pty runner saves about 150 CPU-s of herdr helpers and about 120 sys-s in the test process.
- Holder start-up is cheap. One exec of the 25MB test binary costs 2.2ms user + 2.0ms sys run
  serially, and about 3.4 + 4.1ms with 10 in parallel (1,000-exec micro-benchmark). A `git
  rev-parse` costs about the same. 221 pty holders plus 221 TUI fakes cost under 4 CPU-s to start;
  holder polling adds about 10 CPU-s. A smaller holder binary would save a few seconds at most.
- The fork itself is cheap on the parent side: 0.3ms sys serial, 0.6ms at 10 in parallel, the same
  for a 20MB and a 1GB heap.

### The test process (profile of one pty-dest run, 340 CPU-s sampled)

- `syscall.rawsyscalln` 93s flat: file opens, `readdir`, `unlink`, process start and wait.
- `Server.keepFresh` / `rescanTree` 41.5s: each test server re-reads the whole ticket store every
  200ms (the harness `PollInterval`).
- `Server.keepClaiming` 38s: re-reads `project.json` and the project dirs on each tick.
- `Server.keepBudgetPolled` 20s: the 100ms budget poll.
- `os/exec.(*Cmd).Run` 40.5s, mostly `git` (`git.IdentifyDir`/`FindRepo` 13s, `storecommit` 13s).
- `runtime.mallocgc` 67s and GC mark 24.5s, spread over the same polling loops.
- `Pty.Status` 4.9s; the herdr fake watchdog 4.7s.

### Wall time

- Wall time is not CPU-bound. A pty-dest run uses about 4.7 of 10 cores.
- go test runs 102 non-parallel tests first: a fixed 36s floor in every config. They are the disk,
  health and worktree-removal tests, and many sleep fixed intervals (`letWatcherRead`).
- After that, wall time is about (sum of parallel test durations) / 10 slots. pty-dest round 1:
  36s + (918s - 36s) / 10 = 124s, which matches the 124.5s measured.
- `-parallel 20` made both sides slower and flakier on this loaded machine (pty-dest 285s and 18
  failures, base 356s and 38 failures). The spawn and syscall load is still too high to add slots.

## Fixes for the missed targets

| cause | fix (clean) | estimated gain |
|---|---|---|
| 24 harness-gap tests stay on herdr (~1,500 of the 2,040 herdr calls) | Give the harness a pty fixture: start a real pty agent and script its turn through TUI fake controls (commit, finish, stall, block), and a helper to leave a dead or live pty run for reclaim tests. Port the tests to it. | herdr calls to ~400 to 500 (codex and herdr-health tests only), meets <1,000 |
| codex has no pty path (7 tests, plus 10s waits in 2 passing tests) | Route codex launches by kind: the pty runner refuses a non-claude kind, and the server picks headless (or herdr) for codex. Needed in production too. | removes ~360 herdr calls and ~20s of waits |
| 102 serial tests, 36s floor | Make them parallel; replace fixed sleeps with waits on the event they expect. | up to ~30s wall |
| Store rescans and claim polls every 100 to 200ms in every test server | Make the index incremental: rescan only epics whose dir mtime changed, cache `project.json` by mtime, wake `keepClaiming` from index changes. Production gains too. | ~100 CPU-s of the test process (~30%), plus its GC share |
| ~28,500 `git` calls | Out of scope for this map (separate effort). Biggest callers: `git.IdentifyDir`/`FindRepo`, `storecommit`. | up to ~380 CPU-s |
| 5 disk tests and `TestDiskCleanNow_UnheldDiskKeepsDispatching` | Port to the pty fixture; make the fake cleaner unblock on test cleanup so a failure cannot hang the run. | removes a hang |

With the first three fixes, pty-dest round 1 (124.5s) would land near 95s, and the loaded rounds
(160s to 174s) near 130s. Meeting 2 minutes on a loaded machine also needs the polling fix or the
git work.

Quicker workaround, for reference only: raise the harness `PollInterval` to 1s. It cuts the
rescan cost but slows every test that waits for a tick.
