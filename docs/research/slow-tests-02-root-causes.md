# slow-tests/02: failures and flakes sorted by root cause

Ticket: `gx:slow-tests/02`. Research only. No product code, test or follow-up was changed.
Read on 2026-11-02. Local reruns used main at `a3cad310` (the design branch tip). Anything not
established is marked **unknown**.

## Sources

- CI: `gh run list --workflow CI`, `gh run view <id> --log-failed`. 46 runs on main since the last
  green run (`40018550065`, 2026-10-23). All 46 are red. The run at `a3cad310` (`41408978536`) ended
  red while this note was written.
- Web e2e traces: artifact `web-e2e-traces` of run `41354669925` (the run with 11 web-e2e failures).
- `~/.local/share/gx/check/gx.jsonl`: 228 failing records out of 927 (2026-10-27 to 2026-11-02),
  plus each record's `test.log` for the first error line.
- `gx tickets show gx:follow-ups` and each follow-up. 36 open or draft follow-ups name a test. 16 more
  are not about tests.
- Definitions: `skills/gx-failing-tests.md` and `GLOSSARY.md` ("Flake", "Pre-existing failure",
  "Branch fault").

## 1. What makes main red today

CI per job, 46 red runs:

| Job | Failed | Notes |
| --- | --- | --- |
| test (ubuntu-latest) | 40 | 4 of them failed at the "Web types are current" step, before any test ran |
| test (macos-latest) | 5 | **Cancelled in 40 runs.** The matrix fail-fast stops macOS when ubuntu fails, so macOS is mostly unknown |
| web-e2e | 36 of 42 runs that ran it | 2m playwright time on 10-27, 33m on `a3cad310` |
| herdr-e2e | 12 | Green until 10-30 13:08 |

Main at `a3cad310` fails in every job. The red parts, each one enough to keep main red:

1. `herdr-e2e`: 5 tests. `gx server cannot start: claude is not on PATH`.
2. Go, ubuntu:
   - `cmd.TestBrief_GoTestBuildErrorIsOneUnit`.
   - Guards: `TestNoEnvReads`, `TestNoNewSleeps`, `TestNoOldWords`, `migrations.TestSchemaSnapshot`.
   - `ralphloop` package failed (the real-git hang).
   - 12 `server.TestWorktreeRemoval_*`, 2 `TestTrunkLock_*` (the 30 s ready wait, group E).
   - 9 `servertest.TestPty_*`/`TestLongTicket_*` and 9 `gxfixture` tests.
3. `web-e2e`: 34 failed in 32m35s, including flow 22 and flow 57.

## 2. Root-cause groups, ranked

Ranking measure: failing `gx check` records that contain the group ("runs"), and in brackets the
records where the group is the only failure ("alone"). Out of 228. CI counts are red CI runs out of 46.
Groups C and D are sums of several separate causes, so one fix does not remove a whole group. The
single biggest fix is named in each row.

| # | Group | gx check runs (alone) | CI red runs | Status today |
| --- | --- | --- | --- | --- |
| A | Branch faults: typecheck, build and web unit failures | 78 (39) | 3 (StatsPage vitest) | Not flakes. Branch faults. |
| D | Fake PTY session gone or late | 77 (29) | 21 | Mixed. Part fixed, part still red. |
| C | Fixed time window or deadline under load | 69 (22) | 16 | `TestBudgetEvent` fixed. Others open. |
| B | Guard counts drift | 39 (9) | 10 | Red on main now. |
| F | `gxfixture` child exits | 20 (15) | 11 | Cause unknown. Red on main now. |
| H | Host `claude` assumed on PATH | 6 (5) | 12 | Red on main now. |
| G | Herdr fake gaps | 11 (2) | 0 | Open. |
| E | 30 s ready-signal wait | 7 (0) | 1 (HEAD only) | Red on main now. |
| I | `ralphloop` real-git hang | 4 (2) | 1 (HEAD only) | Red on main now. |
| J | Clock or day dependent | 4 (1) | 0 | Likely. Not reproduced. |

Section 3 lists CI failures that fit none of these groups.

"Unclassified" in gx.jsonl is 51 runs. These are one-off failures with no shared cause I could show.

### Fix order by failures removed

By `gx check` runs, the biggest single fixes are:

1. `TestBudgetEvent_BurstCoalescesAndSummaryShowsFinalTotal`: 48 runs (9 alone). **Already fixed** by
   `ac10d50f` (window 50 ms to 300 ms, committed 05:09Z). It passed 3 of 3 at main today. The last
   failure is 11-01 22:24Z, and no 11-02 record has it.
2. Guard tests (B): 39 runs.
3. `TestChartingHandoff_*`: 23 runs. `TestAutoAdvance_EndingsThatNeverAdvance/done`: 15 runs.
4. `TestSessionPrompt_Codex*`: 14 runs. **Fixed** by `d85f00bd` (own conversation id).
5. `gxfixture` "no output line: EOF": 20 runs.

By CI red runs, the biggest are in section 3. Flow 22 (28 runs) and `TestBrief_*` (23 runs) lead.

By suite time, group E is the biggest. Follow-up 227 says its package takes about 28 minutes, because
each test waits 30 s.

### A. Branch faults (not flakes)

- 78 runs: 50 `typecheck` failures in the log, `<pkg> build failed` lines and web unit failures. Two
  runs list 59 AgentsPage tests from a half-merged branch. Two list about 50 build failures (branch
  `webui-security-impl-item-64j`).
- Real failures on the branch. No follow-up should exist for these. They are noise only if they
  are counted as test failures.

### B. Guard counts drift (`TestNoNewSleeps`, `TestNoEnvReads`, `TestNoOldWords`, `TestSchemaSnapshot`)

- Tests: `sleepguard.TestNoNewSleeps` (34 runs), `envguard.TestNoEnvReads` (17),
  `oldwords.TestNoOldWords` (16), `migrations.TestSchemaSnapshot` (2).
- Follow-ups: 226 (`TestSchemaSnapshot`).
- Evidence: the failing line names a file and a count, for example
  `cmd/agent_permission_request_test.go: allowed 0 time.Sleep call(s), found 3` and
  `codexdoctor/session.go: rule child-env allows 0, found 3`. `TestNoOldWords` failed in 4 CI runs on
  10-24 on `claudedoctor/headless.go:212: old word "park"`. All four guards fail on main today.
- Class: real failures. The guard works. It fires late, after two branches merge. 31 of the 34
  sleep, env and old-word records are `--full` or `--since` runs, not `changed` runs.

### C. Fixed time window or deadline under load

Tests assert a count or a duration that a short timer decides.

- `server.TestBudgetEvent_BurstCoalescesAndSummaryShowsFinalTotal`. The test asserted at most 2
  `budget-changed` events from 5 quick spends against a 50 ms coalescing window. Under load the spends
  spread over more than one window, so the count was 3 to 5 (31 + 10 + 7 records). `ac10d50f` widened it to
  300 ms. Rerun today: 3 of 3 pass. Follow-ups: 178, 181, 182, 184, 188, 191, 205, 209, 216 (nine
  follow-ups for one test, all still open).
- `server.TestDiskHold_*` (follow-up 224): "disk low" and "disk recovered" are 9 ms apart, so
  the test misses the window.
- `cmd.TestProjectRetryMigration_StartsAndRefuses` (190, 218): 5 runs, `refused (migrating)`. It passes alone.
- `server.TestHealthProcs_*` (196): `wait returned after 183µs, want at least the cap`. 3 or more records.
- `server.TestBudget_HardLimitInterruptsRunInParallelInsideGrace`: `stop took Ns, want about the grace`.
- `nativerunner.TestPty_LongPromptDeliveredAtNarrowWidth` (no follow-up): the keys were the prompt once,
  then `\r\r`. An extra Enter came from a nudge timer. Failed 10 of 10 CI race runs since 10-30, and
  1 of 3 locally with `-race -count=3`. That makes it a flake locally and a constant failure in CI.
- `server.TestDesignCleanup_RescanWithoutStepChangeDoesNotCheckAgain`: macOS CI, 2 runs. 3 gx.jsonl
  records. No follow-up in `gx:follow-ups`; `gx:test-flakes/03` covers it.
- Not established: `TestLimitHistoryCodex_RestartKeepsSamples` (185, 189). "lock still held after 2 s" fails
  "deterministically" per the follow-up, with and without branch changes. Whether it is a window or a real
  bug is **unknown**.

### D. Fake PTY session gone or late

Server tests that drive a fake agent through a holder.

- `TestAutoAdvance_EndingsThatNeverAdvance/done`: `dial unix .../holder.sock: no such file or directory`
  (15 runs). **Reran today: failed 3 of 3 on main.** Not a load flake. A pre-existing failure.
  Follow-ups: 193, 206, 213 (same test), 212 (`TestRunEnd_NoContinueAtBranchAndDone/done`, same
  sock/ctl signature, passed 3 of 3 today).
- `TestEpicList_JoinedCardDesignQuestionWinsNeedsYou`: `open .../<id>.ctl: device not configured`
  (6 runs). Follow-up 195. Same family as `TestRestart_AutoResumesDeadWorkingHolderOnce` (211).
- `TestChartingHandoff_NothingRestartsOutsideAStrongestDesignLaunch`: `timed out waiting for a new
  conversation id` (23 runs, 5 CI runs). Follow-ups 207, 214. Passed 3 of 3 today.
- `TestSessionPrompt_Codex*` (14 and 10 runs): two parallel tests shared one conversation id.
  **Fixed** by `d85f00bd`. Follow-up 194 is still open. Its second sighting (06:26Z) is before the fix
  commit (07:12Z).
- `TestPermissionCard_*` (13 runs; 180, 183, 217): "under load". Cause **unknown**.
- `TestCancel_LiveTicketRefusesUntilStop` (10 runs; 197, 210): `ticket not cancelled:`. Cause **unknown**.
- Event on 11-01 to 11-02: `agent-lifecycle-impl/08a` found 39 server tests failing at an untouched
  HEAD (Draft, SessionKeys, FirstPrompt, SessionPrompt, RunAction, ChartingHandoff and others). The
  stray-`i` change broke the fake's editor mode. `ac10d50f` fixed the fake. This explains a burst of
  records on 11-01 and the 08a* tickets.
- Related contributor, **unknown** weight: follow-ups 215 and 221 show the server using over 100% of a
  core, so load on the same host is partly self-made.

### E. The 30 s ready-signal wait

- Tests: the 14 to 15 `server.TestWorktreeRemoval_*`, plus `TestTrunk*` and `TestStashMerge*`. 96 failing
  entries across 7 runs. The biggest single run (`20261102T045111`, 04:51Z) has 104 failures, 37 of them
  in this group. The rest are unclassified.
- Follow-ups: 227 (covers only `WorktreeRemoval`).
- Evidence: the server log says `ready signal missing, sending the launch prompt anyway` after 30 s,
  then the test times out waiting for `root-completed`. `nativerunner/pty_launch.go:92-115`: ready means the input box on
  screen **and** the transcript on disk. `DefaultReadyWait` is 30 s. The wait was added by `6b7dbd91`
  (2026-11-02 01:02 +0200). First failure in gx.jsonl: 04:51Z the same day. Reran at main today:
  `TestWorktreeRemoval_OpenTicketKeepsTheWorktree`, `TestTrunk_DivergedNewTrunkNeedsRebase`,
  `TestStashMerge_RefusesAnEpicThatIsNotPausedOnADirtyTrunk` each failed (1 run each; follow-up 227 has
  3 of 3).
- Why the fake never meets the signal: **unknown**.
- Separate in the same run: 5 tests failed with `install repo template: rename ... file exists` (a
  shared-template race across test processes). `ac10d50f` added a `RemoveAll` for a macOS purge case. Whether
  it fixes that race is **unknown**.

### F. `gxfixture` child exits before printing

- Tests: `TestScenarios_StartAnswerAndStop/agents*`, `TestHealth_*`, `TestSpendFixture_*`,
  `TestCommandsScenario_*`, `TestAgentsScenario_*`, `TestCodexQueueScenario_*` (20 runs, 15 alone).
- Follow-up: 208. It reports `session-not-idle` and `no output line: EOF`.
- Evidence: `main_test.go:173` reads one line from the fixture's stdout. EOF means the fixture
  process died first. The test prints nothing about why. 9 `gxfixture` tests fail on ubuntu at `a3cad310`.
- Cause of the exit: **unknown**.

### G. Herdr fake gaps

- `TestBudget_HardLimitStopsLivePaneAndPausesBudgetKilled` (225): `herdrfake: unimplemented command:
  agent read`. **Reran today: failed.** The fake has no `agent read`.
- `TestPauseStuckFinishedClaims`, `TestSnapshot_ClaimedTicketCarriesLaunchTime`,
  `TestRunner_*` herdr cases: `herdrfake: unexpected herdr call: herdr agent get`.
- `servertest.go:463 Turn.Transcript is not supported on herdr` (5 tests): the test used the wrong flag.

### H. Host `claude` assumed on PATH

- `herdr-e2e` (12 CI runs): `TestServerLifecycle_*`, `TestServerTUI_ShowsServerModeAndDropsOnStop`.
  Two phases:
  1. 10-30 13:08 to 10-31: the default runner became the PTY runner. Error `pty start: holder exited:
     holder.sock never answered`. Cause unknown.
  2. From 11-01: `agent-runner preflight failed ... claude is not on PATH` since `997c4a4f` (10-31, "PATH-check
     server start"). CI has no `claude`. These tests do not set `agent-runner: herdr`.
- `server.TestClaimHold_RateLimitHoldReasonWinsOverUsageGate` (`usagesample_test.go:162`, 4 CI runs): verdict
  `harness missing ... claude is not on PATH`.
- `server.TestClaimHold_HerdrOutageHoldsHerdrTickets/*` (5 gx.jsonl runs; open item
  `follow-ups-2026-11-01/37`).
- web-e2e restart flows 129, 130, 137, 143 first fail on 11-01 17:10, after `997c4a4f`. Flow 143: `gx
  server restart --continue` -> `server did not answer within 45s`. Consistent with the same cause, **not
  proven** (the web-e2e log does not print the preflight error).
- No open follow-up in `gx:follow-ups` names herdr-e2e or the restart flows.

### I. `ralphloop` real-git hang

- `TestRun_FullEpicRealGit_TelegramNotifications_AllValidMarkdownV2`, `TestRun_ProductionRealGit_DiamondThroughFullEpic`.
- Follow-ups: 219, 220. They hang on the base commit too: the 10 min package timeout. `ralphloop package failed`
  in the CI run at `a3cad310`. Cause **unknown** (the follow-up suggests a goroutine dump).

### J. Clock or day dependent

- `server.TestUsageSummary_WeeklyCarriesChipAndForecast` (4 runs; named in 186): seeds
  samples at 01:00 of each past local day and expects `yesterday 10, today 0`. All four failures are between
  19:15Z and 23:09Z. Likely a day-boundary or time-zone dependence. **Not reproduced.**

## 3. CI-only failures with no open follow-up in `gx:follow-ups`

| Failure | Red CI runs | Type | Evidence |
| --- | --- | --- | --- |
| web-e2e flow 22 "Back link count" | 28 | **Stale test.** Deterministic | `bg rgb(242,184,69)` vs expected `rgba(0,0,0,0)`. `follow-ups-2026-11-01/19` decided the amber pill is intended and is marked `done`, yet `webui/e2e/jump.e2e.ts:91` on main still expects the old value |
| `cmd.TestBrief_GoTestBuildErrorIsOneUnit` | 23 | Deterministic in CI since 10-29 06:13. Passes locally on go 1.27.1 and 1.26.5 (reran today) | CI uses go 1.25.8. The `bad` unit is counted in "8 failed" but missing from the printed output. Cause **unknown** |
| `herdr-e2e` | 12 | Group H | |
| `TestPty_LongPromptDeliveredAtNarrowWidth` | 10 | Group C | |
| Guard tests | 10 | Group B | |
| web-e2e flow 57 "Stats, Grilling section" | about 15 | **Missing wait.** Flaky | `Expected 0, Received -1`. `stats.e2e.ts:18` runs `getByRole("region").evaluateAll` right after setting the hash, with no web-first wait. `follow-ups-2026-11-01/23` is `done` |
| `TestLateFinish_FailedConditionStaysPaused/status_changed` | 6 (10-28 to 10-29) | `status = done, want left alone`, ubuntu. Gone since 10-30 | |
| "Web types are current" step | 4 (10-25 to 10-26) | Generated `webui/src/generated/server.ts` stale. Real | |
| web-e2e mass timeouts | 5 (10-27 to 10-28) | 12 to 14 flows at 60 s each. Gone from 10-29. Cause **unknown** | |
| `StatsPage` vitest "Grilling section sits between" | 3 (10-27) | `expected 5 sections, got 4`. Test out of date | |
| `herdrrunner.TestConformance` | 2 | Cause **unknown** | |

The 11 web-e2e failures of 11-01 21:58 (the run the ticket names):

- Flows 76, 78, 88, 118, 22, 129, 130, 137, 143, 24, 57. From the trace artifact: 76, 118 and 129 hit the
  10 s expect timeout. 78, 88 and 24 hit the 60 s test timeout. 143 failed with "server did not answer
  within 45s" (group H). 22 and 57 are value errors (above). The text of 130 and 137 was not read.
- 118 "Codex queue rows": the `follows default` chip is not found. Cause **unknown**.
- Playwright runs 3 workers (`playwright.config.ts`). It took 18m54s. Passed flows rose from 11 on 10-25
  to 88 on 10-28, and 60 s timeouts add up. Time per flow was not measured here.

## 4. Open test follow-ups by group

Count: 36 in `gx:follow-ups` (178 to 227 region).

- BudgetEvent (C, fixed): 178, 181, 182, 184, 188, 191, 205, 209, 216.
- Other C: 190, 218, 196, 224. Unknown cause: 185, 189.
- D, deterministic: 193, 206, 213, 212. D, flake or unknown: 195, 211, 207, 214, 180, 183, 217, 197, 210. Fixed: 194.
- E: 227. F: 208. G: 225. I: 219, 220. B: 226. Umbrella list: 186.
- Process, not tests: 228 (template for flake tickets), 229 (flake rows print cut names).
- `follow-ups-2026-11-01` holds about 15 more open flake items for the same tests (28, 35, 37, 38, 45,
  100 to 109). `gx:test-flakes` has 6 more.
- Nine follow-ups for one flake is a dedupe failure. 229 explains part of it: the `flake:` rows cut test names,
  so the `gx-follow-up` name dedupe cannot match.

## 5. Patterns a guideline or an automated rule could catch

These are candidates. None is decided.

1. **Count or duration asserted against a short timer.** `TestBudgetEvent` (50 ms), `TestDiskHold` (9 ms),
   `TestHealthProcs` caps, the Enter nudge. A scan like `TestNoNewSleeps` could flag test config constants
   under 200 ms that an assertion depends on. A rule: assert on the final state, never on how many
   events a timer produced.
2. **A product wait with a long default and no test override.** The 30 s ready wait, the 45 s server
   start, the 2 s lock wait. A guard could fail when a test launches a fake agent without setting the wait.
3. **A read of page state without a wait.** Flow 57 (`evaluateAll` right after navigation), flow 22 (computed
   style right after `toBeVisible`). A lint on e2e files: `evaluate`/`evaluateAll`/`getComputedStyle` must
   follow an `expect` on the same locator.
4. **A host binary assumed.** `claude` (herdr-e2e, `TestClaimHold`, restart flows), `codex` on PATH. A CI
   job that runs with a stripped PATH would show every one at once. `herdrfake` already fails loudly on
   an unknown call (`unexpected herdr call`), which is the pattern that works.
5. **State shared by `t.Parallel()` tests or by test processes.** One conversation id const
   (`TestSessionPrompt_Codex*`), the shared repo template under `os.TempDir()`. A rule: ids derive from
   `t.Name()`.
6. **Exact-count ratchets fail after a merge.** `TestNoNewSleeps`, `TestNoEnvReads`, `TestNoOldWords`:
   34, 17 and 16 `gx check` records, 10 red CI runs. `changed` scope rarely catches them: 26 of 39 records are
   `--full` or `--since`. Option: run the guards in every `gx check`, they are cheap.
7. **Generated-file drift.** "Web types are current" failed 4 CI runs. It runs only in CI.
8. **A failure that hides its cause.** `no output line: EOF` (20 runs), "N more failures are listed by name
   only" in CI logs, `ticket not cancelled:` with no state. Worth a rule: a helper that drops a child's
   stderr or its reason must say why.
9. **Wall clock in a test.** `time.Now()` day math (group J), and the earlier web flow that depended on the
   hour (`c90eda8c`).
10. **A stale assertion left on main.** Flow 22's ticket is `done` while the assertion is unchanged on main.
    A `done` ticket whose fix is not on main is a process gap, not a test rule.
11. **Fail-fast hides a platform.** macOS was cancelled in 40 of 46 runs.

## 6. What I reran, and what I did not

| Command | Result |
| --- | --- |
| `-count=3` `TestAutoAdvance_EndingsThatNeverAdvance`, `TestBudgetEvent_Burst*`, `TestChartingHandoff_Nothing*`, `TestRunEnd_NoContinue*`, `./server` | AutoAdvance/done failed 3 of 3. The other three passed 3 of 3 |
| `-count=1` `TestBudget_HardLimitStopsLive*`, `TestTrunk_DivergedNewTrunkNeedsRebase`, `TestStashMerge_RefusesAnEpic*`, `TestWorktreeRemoval_OpenTicketKeepsTheWorktree` | all four failed once |
| `-count=1` `TestBrief_GoTestBuildErrorIsOneUnit`, go 1.27.1 and `GOTOOLCHAIN=go1.26.5` | passed. CI uses go 1.25.8, not tested here |
| `-race -count=3` `TestPty_LongPromptDeliveredAtNarrowWidth` | 1 failed (`\r\r`), 2 passed |

Not rerun: any web-e2e flow (the design worktree has no `node_modules`; I did not change it), CI-only
jobs, the full suite. Host load was 9 to 63 during the reruns, set by another agent timing the suite. The
loads make load-sensitive results weaker evidence than a quiet run.

## 7. Sharp questions for the map

1. Which toolchain and OS should the "reliable" suite be judged on? `TestBrief_*` fails only on go 1.25.8
   in CI. macOS CI is cancelled by fail-fast in 40 of 46 runs.
2. Should CI get a fake `claude` on PATH, or should the e2e tests pin `agent-runner: herdr`? One change
   removes the herdr-e2e job (12 runs), `TestClaimHold` (4) and probably 4 web-e2e restart flows.
3. Is the 30 s ready wait a product default the tests must always override, or should the fake meet the
   signal? It decides group E (about 28 min of suite time).
4. Does "open test follow-ups closed" cover `follow-ups-2026-11-01` and `test-flakes` as well? They hold
   about 20 more items for the same tests.
