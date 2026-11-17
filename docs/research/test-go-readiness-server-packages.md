# gx:test-suite-stability/13 - Go readiness timeouts in the scheduler, launchtest, disk, budget and session tests

Question: why do the scheduler, launchtest, disk, budget and session tests time out under load but pass alone, and what is the fix at the cause?

Date of runs: 2026-11-17, worktree `test-suite-stability-design`. All load runs used `GX_PROC_HEADROOM=600`, so fork-limit errors (ticket 09) do not mask other causes. Load average is recorded per run below. `TestHerdrBanner_HiddenWhileNoTicketNeedsHerdr` is skipped (ticket 09 and 05). `server/disk` has no ticket in this list that still fails: `test-flakes/15` (disk hold) was closed in the sibling report (`test-go-readiness-gxfixture.md`).

## Answer in short

- The 17 tickets in this scope fall into 4 causes. Two are proven by a deterministic reproduction, one by logs plus a code read, and one by a code read plus injection of a stall. One group has no cause: nothing has failed in the retained logs and it does not fail under load.
- **A. A launch that "starts no turn" (proven, new).** The `launchtest` nudge and launch tests run a fake agent that never starts a turn for the launch prompt (no `OnTurn` hook). The launch passes only because the holder's first read of the pid file (starting to idle) counts as turn 1, and only if that read comes after the launch took its baseline. Under load the baseline comes later, and the launch fails with `started no turn` three times, then pauses `agent_prompt_stalled`. In the 16 retained logs from Nov 9 to Nov 15 where `TestStallNudge_*` failed, 15 carry exactly this text and none carries a fork error (the 16th, Nov 15 15:16, ends in `status = claimed, want paused` after 30 s with no signature text). My 2 loaded failures carry it too. Fix: let the fake run a real turn (`h.OnTurn(func(*servertest.Turn){})`). With a 400 ms delay injected before the baseline, the current test fails 2 of 2 and the fixed test passes 2 of 2. Tickets: test-health/01, /10 and, by the same text, `TestVimLaunch_...` and `TestLaunch_PastedCommand...` in the same package.
- **B. The budget test polls while a run is still `Launching` (proven).** `startBudgetRuns` waits until the fake runner lists the sessions. The budget kill skips runs in state `Launching`, and the test never polls again. Reproduction: 9 failures in 60 runs when the test polls at once, 0 in 60 when it first waits for `Budget().LaunchingRuns() == 0` (a seam that already exists). Ticket: test-health/07.
- **C. A cleanup that panics aborts the whole package binary (proven from logs).** `t.Cleanup(func(){ fake.SetState(label, ...) })` panics when the launch never made the session (`runnerfake: no live session`). In 4 logs this turned one failed test into the failure of 2 to 3 tests and ended the run. This is why test-health/13, /14 and /15 "failed together" (Nov 15), and it spoiled my own run today. About 16 call sites.
- **D. The fork limit (ticket 09).** follow-ups/514 (the four `server/sched` tests), test-health/13 and /14 (their first failure), and the `DerivedActionable` failures of Nov 16 all carry `fork/exec ...: resource temporarily unavailable` in their logs. Fix and validation are in `test-fork-errors-full-suite.md`.
- **E. Countdown test (mechanism proven by injection, natural trigger not seen).** `TestAutoAdvance_RunEndSendsContinueAfterTheCountdown` waits for a state that exists for 1 s (the countdown). A test goroutine that is stalled 1.5 s misses it and the wait runs to its 20 s limit. Not seen failing today (0 in 25 under load).
- **F. No cause found:** `TestUsageGate_HoldsClaudeClaimsOnly`, `TestLateFinish_FailedConditionStaysPaused/status_changed`, `TestDerivedActionable_PageAnswerClearsGrilling` (not the fork-error runs). 0 failures in 25 runs each under load; no failing log since Nov 6 except fork errors; the reports are from Oct 29 to Nov 4.

## Method

For each ticket: read the wait and the code it waits for, read the failing logs in `~/.local/share/gx/brief`, run alone, run in the package, run under load with repeats, then inject the suspected delay to make a rare failure deterministic.

- Failing logs: `scan.py` and `classify.py` in the scratchpad read every `*go-test*.log` (21 676 files) and list, for the test names, the failing tests with the signatures `started no turn`, `resource temporarily unavailable`/`process limit hit`, `panic:`. Logs older than Nov 6 are gone.
- Alone: `gx brief go-test -count=1 -run ... ./server/sched/launchtest`.
- Load: 24 to 36 busy loops (`sh -c 'while :; do :; done'`) on 12 cores on top of the load from other sessions, `-count` 6 to 60. Plain `go test` runs used `-exec scripts/proc-cap` and headroom 600.
- Injection: a temporary edit of `server/servertest/ptyrunner.go` (a sleep before the prompt is sent, read from an env var) and temporary test files. All were removed; `git status` shows no change under `server/` or `testutil/`.
- Scratchpad: `/private/tmp/claude-501/-Users-david-dev-gx-test-suite-stability-design/d1536974-9324-48ad-950d-7804404bd1b2/scratchpad/` (`tmp_diag_test.go.txt`, `tmp_budget_diag_test.go.txt`, `tmp_countdown_diag_test.go.txt`, `scan.py`, `classify.py`, `testout.py`, `burn.sh`, `diag*.out`, `ph2.out`).

## Facts

### Runs

| Run (time, load average at start) | What | Result |
| --- | --- | --- |
| 08:35, 46 | launchtest: StallNudge x2, VimLaunch (3), Claim, ColdStoreWave, alone | 16 pass, 24.9 s |
| 08:36, 39 | same set, `-count=10`, no burners | 140 pass |
| 08:40, 36 | same set `-count=6`, 24 burners (load 36 to 44) | 1 fail: `TestStallNudge_HungPaneOnceIsRelaunchedOnce`, `started no turn` x2 attempts, 30 s |
| 08:43, 28 | diag copy of the Once test x25, 24 burners | 25 pass |
| 08:44, 34 | StallNudge, diag Once, Claim, ColdStoreWave, `TestLaunch_*` `-count=8`, 24 burners | diag Once fails (`started no turn`); `TestLaunch_RelaunchPromptListsEarlierSummaries/implement` times out after 22 s, then the cleanup panic ends the run |
| 08:49, 29 | same group `-count=10`, 24 burners | 400 pass |
| 09:00, 3 then 35 | same group `-count=15`, 28 burners | 3 fail, all `started no turn`: Twice test, diag Once, `TestLaunch_PastedCommand...` (9m12s) |
| 09:10, 16 | 4 diag tests (base/fixed x Once/Twice) `-count=40`, 36 burners | 160 pass (so the rate is too low to compare base and fix this way) |
| 09:14, 28 | injected 400 ms delay before the prompt, base and fixed Once x2 | base fails 2/2, fixed passes 2/2 |
| 09:15, 17 | budget, limits, merge, sessions/prompt, promptflows (UsageGate, LateFinish, DerivedActionable x4, AutoAdvance, 8 budget tests) `-count=25`, 28 burners, load up to 48 | 875 pass, 0 fail |
| 09:20, 36 | budget diag: poll as soon as the fake lists 2 sessions, x15 then x60 | 1/15 then 9/60 fail, all with `launching=1` at poll time |
| 09:21, 20 | budget diag: wait for `LaunchingRuns()==0` first, x15 and x60 | 0/15 and 0/60 fail |
| 09:23, 5 | countdown diag: stall 0 / 600 ms / 1500 ms between ending the turn and reading the session | pass / pass / `no session-changed event ...: context deadline exceeded` after 20 s |

### Group A: `started no turn` in the launch tests

Facts:

- The failing logs: `TestStallNudge_*` failed in 18 logs from Nov 9 on (16 retained from before today, 2 from my runs). 17 of the 18 carry `started no turn`; none carries `resource temporarily unavailable` or `process limit hit`. The 18th (Nov 15 15:16, Twice test) ends `status = claimed, want paused` after 30 s. Dates: Nov 9 x3, Nov 10 x2, Nov 11 x3, Nov 12 x2, Nov 13, Nov 15 x5, Nov 17 x2. The old name `TestLaunch_HungPane*` (Nov 8 to Nov 15) shows the same text on its `/claude` subtests. The one fork-error failure in the package (Nov 15 22:36) is a different test (`TestVimLaunch_...`).
- A failing run, from the run log: the first launch (or the relaunch) logs `launch-failed ... started no turn` at attempts 1 and 2, 5.2 s apart (the 5 s `ptyPromptGrace` of `servertest/ptyrunner.go`), then `paused ... agent_prompt_stalled`. The Once test then waits its whole 30 s for a second `iteration-started`.
- Timeline of a failing run (diag test, polling the holder every 10 ms): `09:03:49.220 pid file idle`, `09:03:49.286 holder status=idle turn=1`, then no change for 5 s. The pid file never shows `busy`, no OSC report arrives, `turn` stays 1.
- Timeline of a passing run: `pid file idle`, `+90 to 100 ms holder status=idle turn=1`. `launch-timed` shows `PromptStep` 72 to 74 ms for the first launch and 147 to 149 ms for the relaunch, in every passing run (8 runs). So the launch is "proven" at the moment of the holder's first pid read, with no turn. The pid file stays `idle` throughout.
- The code: `ptyTurn.awaitNotBusy` (`nativerunner/pty_turn.go:59`) returns the current turn count as the baseline as soon as the pid file is not `busy`, including before the file was ever read (`claude == nil`). `PidWatcher.poll` (`harness/claudepid.go:241-247`) counts a first read past `starting` as a change ("a first read past starting still counts the change a poll missed"). `HolderAnswerer.WaitTurn` (`nativerunner/holder/holderanswer.go:77`) passes when `turn > baseline`. `PromptLaunch` (`nativerunner/pty_launch.go:116`) fails with `started no turn` when it does not.
- The fake: `ptyRunner.prompt` (`servertest/ptyrunner.go:109`) arms the fake with `ActionFast` (working then done by OSC, counted without a poll) only when an `OnTurn` hook is set. `nudgeHang` in `hung_nudge_test.go` sets none. Without it the typed `/gx-implement ...` is a plain slash command in the fake and starts no turn (`tui_prompt.go:366`, "starts no turn").
- Reproduction: `DIAG_PROMPT_DELAY=400ms` (sleep before `awaitNotBusy` takes its baseline): the current Once test fails 2 of 2 (`launch-failed` x2, paused, 30 s); the same test with `h.OnTurn(func(*servertest.Turn){})` passes 2 of 2.

Hypotheses (not proven):

- The race is "baseline after the holder's first read". It needs the launcher to take the baseline more than about 100 ms after the fake wrote its pid file. I did not measure what stretches it under load (goroutine delay in the server, the 5 s `awaitEmptyInput`, or a slow holder start).
- `awaitNotBusy` returning a baseline before the first pid read means a real launch of a real claude whose turn never starts could be "proven" by the startup count. Not tested; it would show in production as a missed stall, not as a flake.

### Group B: the budget test

Facts:

- Failing log (Nov 14 22:26, run 20261114T222647): `limits_test.go:526: timed out waiting for no live sessions`, 6.07 s (about 1 s of setup and the 5 s wait). The server log has no `budget stop signal` or `budget kill` line.
- `KillableRuns` (`server/budget/budget.go:279-287`) leaves out every run in state `Launching` ("a later check stops it once it runs"). The test sets `BudgetPollInterval = time.Hour`, so no later check comes.
- `startBudgetRuns` waits for `fake.List("epic-a")` to hold n sessions (`limits_test.go:408`). The run leaves `Launching` only after the prompt step, later than the session appears in the fake.
- The seam for this already exists: `Budget().LaunchingRuns()` (`budget/testseams.go:44`), used by `servertest/ticketwrite.go:95` and `rigs/blockedclaimed.go:82` for the same wait.
- Reproduction: 9 of 60 failures when the test polls as soon as the fake lists the sessions; every failure printed `launching=1`. 0 of 60 when it first waits for `LaunchingRuns() == 0`. The test as written passed 25 of 25 under load 48 (its wait is coarser than my spin).

### Group C: the cleanup panic

- `runnerfake.Runner.byLabel` panics for a missing label (`testutil/runnerfake/runnerfake.go:115`), and `SetState` uses it.
- 16 `t.Cleanup(func() { x.SetState(label, ...) })` call sites have no guard (`server/sched/claimhold_test.go:120,157`, `launchtest/claim_resume_test.go:86`, `iteration_summaries_test.go:52`, `launch_note_lifecycle_test.go:68`, `launch_note_files_test.go:44`, `verify_resume_test.go:64`, `launch_timed_test.go:23,54`, `reclaim_launching_test.go:90`, `stale_holder_test.go:37`, `server/rest/agentsunderrest_test.go:44`, `server/harness/harness_missing_test.go:47`, `server/sched/finish_test.go:381`, `ratelimit_hold_run_test.go:40`, `ratelimit_claim_hold_test.go:32`); `cold_store_wave_test.go:49-51` loops over six labels. `usage_gate_test.go` guards its cleanup with `len(pty.Prompts(label)) > 0` and does not have the problem.
- A panic in a cleanup ends the test binary. Seen in 4 logs: Nov 15 22:36 (`TestClaim_ResumesOrColdStarts/answered_pty_resumes` and `TestHerdrBanner...` both end in `panic: runnerfake: no live session`), Nov 16 10:54 (`TestColdStoreWave_...`, `panic` after `launch-timed = 5, want 6`), Nov 17 08:49 (`TestLaunch_RelaunchPromptListsEarlierSummaries/implement`, my run).
- In those logs the first failure is the real cause (a fork error in the three Nov logs). The panic then makes every test still running in the package fail too, which is why the tickets list tests that "failed together".

### Group D: fork errors

From the logs (see `test-fork-errors-full-suite.md` for the cause):

- Nov 15 22:36 (test-health/13, /14, /15): `TestVimLaunch_...` failed at 3.4 s with `fork/exec .../launchtest.test: resource temporarily unavailable` and `git worktree remove ...: fork/exec /opt/homebrew/bin/git: resource temporarily unavailable`. `TestClaim_ResumesOrColdStarts/answered_pty_resumes` and `TestHerdrBanner...` ended in the cleanup panic (group C).
- Nov 16 10:54 (follow-ups/514): `PauseThenResume` failed with `git [branch --list ralph-loop/*]: fork/exec /opt/homebrew/bin/git: resource temporarily unavailable`; `ABlocksBAndC` and `CodeReviewTicketCreatesSubtickets` with `git rev-list ...: fork/exec ...` then `no root-completed event ... context deadline exceeded` after 30 s; `TestColdStoreWave_...` with `pnpm install ...: fork: Resource temporarily unavailable`, then the cleanup panic.
- Nov 16 14:17: 8 `TestDerivedActionable_*` subtests failed in 0.03 to 0.19 s with `start session holder: fork/exec ...`. The other `DerivedActionable` rows of test-flakes/12 are Oct 29 and Nov 1 and are not in any retained log.

### Group E: the countdown test

Facts:

- `CountdownDelay = time.Second` (`servertest/rigs/advancing.go:12`). `WaitForSession` (`servertest/sessionwait.go:43`) checks a snapshot, then waits for a `session-changed` event. The predicate `AutoAdvanceAt != nil` is true for 1 s; after that the server sends `/clear` and the field is nil again.
- The test then asserts `got.AutoAdvanceAt.After(time.Now().Add(-time.Second))`, a wall-clock check that holds only if the session was read within 2 s of the countdown start.
- Injection: a 1.5 s stall between `endTurn` and the wait made `waitForSession` fail with `no session-changed event for "...": context deadline exceeded` after 20 s. A 600 ms stall passed.
- The Nov 4 triage decision (ticket text) already says the same: "add a test-controlled countdown trigger".
- Not seen failing under load today (0 in 25). Its last sightings are Oct 30 to Nov 1, in packages that have only been moved since (commits `33d768c42` Nov 13, `a019da984` Nov 9).

Hypothesis: the Oct 30 to Nov 1 failures came from a test or server goroutine stalled for 1 s or more. Not tested at that rate.

### Group F: no evidence of a cause

- `TestUsageGate_HoldsClaudeClaimsOnly`: runnerfake-based; no wall-clock in the test (the `SeenAt` timestamp is not read as a freshness limit: `rg SeenAt server/limits/*.go` shows only de-duplication against an older sample). 0 failures in 25 under load. 6 failure reports on Nov 3 to Nov 4; the package was created on Nov 4 (`0742a1911`). No log of those runs remains.
- `TestLateFinish_FailedConditionStaysPaused`: `StaysPaused` (`rigs/zerocommit_paused.go:137`) is a 200 ms negative wait (it cannot fail from slowness; it can only miss a late adoption). A failure would be the server adopting a ticket it should not. 0 in 25 under load. Report of Oct 29.
- `TestDerivedActionable_PageAnswerClearsGrilling`: 0 in 25 under load. Report of Oct 29. The test sends the first `report(... working)` after the holder is up, the shape the sibling report found for `TestSessionScreen_LiveReturnsHolderRows` (a socket or file is read as "agent ready"); I did not test it.

## Table

| Ticket | Cause | Evidence | Fix | Validation |
| --- | --- | --- | --- | --- |
| test-health/01 `TestStallNudge_HungPaneTwicePausesPromptStalled` | A: the fake starts no turn for the launch prompt (no `OnTurn`); the launch passes only if its baseline is taken before the holder's first pid read | 18 failing logs of the StallNudge tests Nov 9 to Nov 17, 17 with `started no turn`, 0 with a fork error; timeline `turn=1` with no `busy`; 400 ms injection: fail 2/2 | In `nudgeHang` call `h.OnTurn(func(*servertest.Turn){})` before `Queue.Add`, so the launch prompt runs a real fast turn (OSC `working` and `done`, counted without a poll). Do not raise `ptyPromptGrace`: ticket 01 already saw a 20 s grace fail differently | Fixed variant passes 2/2 under the 400 ms injection and in all quiet and loaded runs (160 runs). Rate too low to compare (base 0/80 in that run); the injection is the proof. Not applied |
| test-health/10 `TestStallNudge_HungPaneOnceIsRelaunchedOnce` | A | same 18 logs; 08:40 run; Nov 15 09:53 log shows `launch-failed started no turn` at attempts 0, 1, 2 then `paused` | Same fix. The 30 s against the 20 s slow limit is the test waiting its whole poll after a pause: not slowness | As above; also base fails 2/2, fixed passes 2/2 under injection |
| test-health/13 `TestVimLaunch_PersistentFooterWithAHang...` | D then C: fork error in the holder start and `git worktree remove`; the cleanup panic ended the package | Nov 15 22:36 log: `fork/exec .../launchtest.test: resource temporarily unavailable` at 3.4 s | Ticket 09 (headroom). Same `OnTurn` rule applies to the other `vimLaunch` tests that expect a launch to succeed (`vim_launch_test.go` has no hook) | Not run under load: it passed in all 4 groups of runs today (0 failures) |
| test-health/14 `TestClaim_ResumesOrColdStarts/answered_pty_resumes` | C: after a fork error nothing launched; `t.Cleanup(fake.SetState(...))` panicked `runnerfake: no live session` | Nov 15 22:36 log: `start attempts carried resume ids []`, then `panic: runnerfake: no live session "epic-a-iter-01"` at `claim_resume_test.go:86` | Make the cleanup tolerate a missing session (see C below). The ticket's question "fail fast when the events channel closes" is separate: `claimOpen` loops `for ev := range evs` and ends when the channel closes | Not run; the panic is visible in the log text |
| test-health/15 `TestHerdrBanner...` | skipped (ticket 09) | Nov 15 22:36 log shows the same panic at `claimhold_test.go:157` | n/a | n/a |
| test-health/07 `TestBudget_HardLimitStopWaitsForSlowInterrupts` | B: the budget poll runs while a run is `Launching`; the kill skips it; no later poll (interval 1 h) | Nov 14 log: 6.07 s, `timed out waiting for no live sessions`; spin variant 9/60 fails, all `launching=1` | In `startBudgetRuns` wait for `h.Server.Budget().LaunchingRuns() == 0` after the sessions appear (the helper in `ticketwrite.go:95` does the same). Do not widen the 5 s wait | Variant with that wait: 0/60 (same load). Spin variant: 9/60. Not applied |
| follow-ups/514 `TestSchedulingPort_*` (3), `TestColdStoreWave_...` | D (fork errors) and C (panic after `ColdStoreWave`) | Nov 16 10:54 log: `git rev-list`, `git branch --list`, `pnpm install` fork errors; `launch-timed = 5, want 6`; panic | Ticket 09. C for the panic | Not run; the sched tests were not in a failing state today (the 08:35 and 08:49 runs pass `ColdStoreWave` 11 of 11 times) |
| test-flakes/12 `TestLateFinish_FailedConditionStaysPaused/status_changed`, `TestDerivedActionable_PageAnswerClearsGrilling` | Unknown. `DerivedActionable` also failed with fork errors on Nov 16 14:17 | 0 failures in 25 runs each under load 48; the reports are from Oct 29 and are not in any retained log | None proven. If it returns, print the ticket file and the server log in the failure message | 25 runs under load each |
| test-flakes/17 `TestUsageGate_HoldsClaudeClaimsOnly` | Unknown | 0 in 25 under load; no log since Nov 6 | None proven | 25 runs under load |
| follow-ups-2026-11-01/29 `TestAutoAdvance_RunEndSendsContinueAfterTheCountdown` | E: a 1 s state observed with a poll; `AutoAdvanceAt` check against the wall clock | Injection: a 1.5 s stall gives a 20 s timeout; 600 ms passes; 0 in 25 under load | The ticket's triage decision: a test-controlled countdown (hold the timer, let the test observe the raised Continue, then release it) and drop the wall-clock assertion. Not a longer countdown | Not applied. The injection test (stall 1.5 s) should pass after the fix |
| Found while testing: `TestLaunch_PastedCommandRelaunchesOnceThenPausesAndRetryEndsIt` | A | 09:00 run: `retry: ... prompt not delivered ... started no turn` after Retry (the launch after Retry does not paste and no hook is set), 11.6 s | Same `OnTurn` fix | Not run |
| Found while testing: `TestLaunch_RelaunchPromptListsEarlierSummaries/implement` | Open. The test timed out after 22 s with no `startup step` line in the server log (a normal start logs `first-rescan` within 40 ms); then C ended the run | 08:49 run (one occurrence, `-count=8`, 24 burners); the 10 s `WaitFor` ran out, but the test took 22 s, so the harness start or `Queue.Add` was slow too | None proven | Not seen again in the 400 runs of 08:49, the 160 of 09:10 or the 875 of 09:15 (those did not include this test except the 400) |

## Groups by shared cause

- **A. The fake does not start a turn that the launch waits for:** test-health/01, /10; `TestVimLaunch_*`, `TestLaunch_PastedCommand...`, and the other no-hook launch tests in `launchtest` (`hung_pane_test.go`, `launch_inflight_test.go`, `codex_dropped_prompt_test.go`: not run, they use the same harness without a hook).
- **B. Test acts on the runner's view, the server's state is not ready:** test-health/07. Same family as the sibling report (a socket or file read as "ready").
- **C. Cleanup that panics:** every launch test with an unguarded `SetState` cleanup. It does not cause a failure; it multiplies one (test-health/13, /14, /15, follow-ups/514).
- **D. Fork limit:** follow-ups/514, test-health/13 first failure, `DerivedActionable` Nov 16.
- **E. Short transient state:** follow-ups-2026-11-01/29. Related shape: any test waiting for `AutoAdvanceAt != nil`, a banner, or a countdown.
- **F. Unknown:** test-flakes/12, /17.

## Fix list for the implementation epic

1. Give the `launchtest` fakes a real turn: `h.OnTurn(func(*servertest.Turn){})` in `nudgeHang`, `vimLaunch` and the other no-hook launch tests. Better, make it a harness default: `ptyRunner.prompt` arms `ActionFast` for a launch prompt unless the test asked for a stall (`TUIHangLaunchesEnv`, `Turn.Stall`).
2. Make `awaitNotBusy`/`WaitStart` robust to the first read, or document it: a baseline taken before the pid file was read should not let the startup count as a turn. This is a product change; keep it separate from the test fix and check how a real claude behaves (hypothesis only above).
3. `startBudgetRuns`: wait for `Budget().LaunchingRuns() == 0`.
4. `runnerfake`: add a non-panicking `Finish(label)` (or let `SetState` ignore a missing label when called from a cleanup) and use it in the 16 cleanups, so a failed launch fails one test.
5. `TestAutoAdvance_*` countdown: hold the countdown from the test; drop the wall-clock assertion.
6. Fork errors: ticket 09's headroom change.
7. Do not retry and do not lengthen a timeout. Group A fails after a fixed 5 s grace that is not slowness, B fails after a 5 s wait for a poll that never comes, E fails after the state is gone.

## Hypotheses (not proven)

- H1: the delay that puts the launch baseline after the holder's first pid read comes from CPU starvation of the test process (the server side), since the holder's read is event-timed (100 ms poll) in a separate process. Support: the failures need 24 or more burners and a parallel package; 0 of 25 with burners but only the Once test. Not measured.
- H2: the `TestLaunch_RelaunchPromptListsEarlierSummaries` 22 s start stall is the same slow server start that made `startup step` lines late. One occurrence; no log of what blocked it.
- H3: the Oct 29 to Nov 4 sightings of group F came from the pre-Nov 6 conditions (no launch cap, no parallel cap; see the sibling report). Support: 0 in 25 under load now. Against: nothing found.

## Lessons for the Go skill

- If the success signal is a counter ("a turn started"), check that the fake really produces the event. A counter that moves on startup lets a test pass by accident, and the same test then fails when the baseline moves. Make the fake do the work and assert it.
- When a wait compares against a baseline taken earlier, ask when the baseline is taken, relative to the first observation. A baseline taken before the first read and one taken after it differ by exactly the thing being counted.
- Wait on the state the code under test reads, not on the runner's view of it. The budget kill reads the run registry; the test waited for the runner's session list. Look for an existing seam (`LaunchingRuns`) before adding a wait.
- A cleanup that can panic is a failure multiplier: it ends the binary, and every test still running fails with it. Cleanups tolerate "already gone" and never `panic` for a missing fixture.
- A state that lasts about as long as a scheduling delay (a 1 s countdown) cannot be observed by polling. Hold the timer in the test, or observe an event.
- Make a rare failure deterministic by injecting the suspected delay at the seam (a sleep before the baseline, a stall before the read, a spin poll instead of a coarse one). A 1 in 100 flake becomes 2 of 2 and the fix 0 of 2. Repeats under load are for finding the shape, not for proving the fix.
- Classify failures from logs by signature before reading code: here 17 of 18 logs said `started no turn` and none said `resource temporarily unavailable`, which ruled the fork limit out for that family in one script.
- Comparing base and fix by repeats needs a failure rate high enough to see: 0 of 80 against 0 of 80 proved nothing. Say so in the report instead of counting it as a pass.
- A `timeout` fix is wrong when the failure is a missed event: a 30 s Once test that fails after a pause is waiting for a state that cannot happen any more.
