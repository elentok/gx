# gx test suite: time and flakes (October 2026)

Answers `gx:better-tests/01`. Measured on `main` at `c892f6e6`, on the developer Mac (10 cores,
darwin/arm64, Go 1.27.1), in a fresh worktree, with the default `TMPDIR`.

## Summary

- One full run (`make test` pieces plus `make web-test`) takes **6.1 to 7.0 minutes**. `make web-e2e`
  adds 2 to 3 minutes.
- **No run was green.** All 6 Go rounds and all 6 web rounds failed. Every Go round had 2 to 4
  failures that pass on rerun. Every web round failed `StatsPage`, which is not a flake
  (`gx:follow-ups/11`).
- The unsharded `go test ./server` took **11.3 minutes**. That is over the 10-minute timeout that
  `.ai/index.md` and CI use. Sharded (`make test-server`) it takes 2.5 to 3 minutes.
- Proposed targets: **N = 4 minutes** for the full suite without e2e, and **zero failures in
  M = 20 runs in a row**.

## Method

The script ran each step in turn, timed it, and kept all output:

1. `go test -race -count=1 -json $(RACE_PKGS) -timeout=10m` (the `test-race` target).
2. `go test -count=1 -json <every package but ./server> -timeout=10m`.
3. `./server` sharded the same way as `scripts/test-shards.sh` (5 processes, round-robin by test
   index), with `-test.v` added so each test's time is logged.
4. `make web-test` (pnpm install, `tsc --noEmit`, `vitest run`).

It ran 6 rounds of these steps. Round 1 includes compiling. After that it ran one unsharded
`go test ./server -timeout=20m`, then `make web-e2e` 3 times. The herdr e2e tests (`make e2e`)
were skipped. Every failure was then rerun alone (`-test.run '^Name$' -test.count=10`, a single
vitest file 10 times, or `playwright --repeat-each=3` on the failing files).

Machine load: other agents were running the whole time. The 1-minute load average was 5 to 30
during the Go and web steps, and peaked at 112 during the first e2e run. `fseventsd` used 100% of
one core at one point. Treat the times as "a busy dev Mac", which is the real working condition
for agents.

## Wall time

Seconds per step, per round:

| Round | race | go (no server) | server (5 shards) | web-test | total |
|---|---|---|---|---|---|
| 1 | 80 | 102 | 150 | 36 | 368 |
| 2 | 78 | 101 | 163 | 35 | 377 |
| 3 | 80 | 103 | 178 | 34 | 395 |
| 4 | 81 | 112 | 173 | 36 | 402 |
| 5 | 81 | 114 | 176 | 36 | 407 |
| 6 | 81 | 123 | 178 | 37 | 419 |

Other runs:

- Unsharded `go test ./server`: 678 s (11.3 min), 975 tests.
- `make web-e2e`: 187 s, 139 s and 114 s (84 flows, 3 workers). The first run includes `web-build`.
  Playwright reports 1.8 to 3.0 minutes of that.
- `vitest`: 668 tests in 55 files. Vitest reports a 31 s duration, and `tsc` plus install take the
  rest.

### Slowest Go packages (without `./server`)

Median of 6 rounds, in seconds. Packages run side by side, so the step takes about as long as the
slowest package.

| Package | median | min | max | slowest test |
|---|---|---|---|---|
| testutil/gxfixture | 94 | 84 | 113 | `TestScenarios_StartAnswerAndStop` 24 s |
| cmd | 89 | 72 | 102 | `TestBrief_SweepRemovesOldLogsAndIndexLines` 11 s |
| nativerunner | 66 | 60 | 72 | `TestFakeclaude_PtyReadsScreenAndAnswersPermission` 17 s |
| ralphloop | 66 | 53 | 68 | `TestRun_ProductionRealGit_DiamondThroughFullEpic` 9 s |
| ui/app | 59 | 51 | 64 | |
| ui/log | 46 | 35 | 51 | |
| ui/status | 46 | 35 | 53 | |
| e2e | 29 | 25 | 34 | `TestAgentsTerminal_ShowsTheAgentsColors` 18 s |
| ui/commit | 26 | 20 | 29 | |
| oldwords | 26 | 20 | 30 | `TestNoOldWords` 19 s |

Under `-race`, `nativerunner` takes 78 s (`TestPty_Conformance` 18 s) and the other five race
packages take under 13 s each. So `test-race` is about 80 s of nativerunner alone.

### Server package

- 975 tests, about 690 s of test time in total. 205 tests take 1 s or more, and they account for
  473 s of it.
- The 5 shards are balanced (133 to 173 s of test time each), so the server step lasts as long as
  one shard plus about 10 s of compiling.
- Slowest tests (median of 6 rounds): `TestLateFinish_FailedConditionStaysPaused` 21 s,
  `TestRunner_BackfillsTheNextQueuedRootWhenASlotFrees` 13 s,
  `TestConfigReload_MaxAgentsAppliesWithoutKillingAgents` 11 s, the two budget tests below 10 s
  each (their timeout), `TestLateFinish_ResumeAndRelaunchAdoptFirst` 9 s,
  `TestRecovery_R10ReleasesAnIdleAgentsGateAndLetsTheFinishRun` 8 s.

## Failures

"Suite" counts the 6 sharded rounds plus the 1 unsharded run (7 server runs), 6 vitest runs and 3
e2e runs. "Alone" is the rerun on its own.

### 1. Store watch misses or delays events (FSEvents)

| Test | Suite | Alone | Message |
|---|---|---|---|
| `TestStoreWatchSeesNewFilesAndSkipsGit` | 6 of 7 | 0 of 10 | `storewatch_test.go:44: no event for the new ticket file` |
| `TestBudget_HardLimitInterruptsRunInParallelInsideGrace` | 6 of 7 | 1 of 10 | `budget_limits_test.go:423: live sessions = 1, want 3` |
| `TestBudget_HardLimitStopsDespiteInterruptError` | 4 of 7 | 1 of 10 | `budget_limits_test.go:458: live sessions = 1, want 2` |
| `TestBudget_HardLimitStopWaitsForSlowInterrupts` | 1 of 7 | 1 of 10 | `budget_limits_test.go:442: live sessions = 1, want 2` |

- `TestStoreWatchSeesNewFilesAndSkipsGit` fails in every sharded round and passed in the unsharded
  run. Shard 0's tests run alone in one process (no other shards) still fail it. So it depends on
  the tests that run before it in the same process, not on the other shards. On macOS
  `NewStoreWatch` uses FSEvents (`server/storewatch.go`). Commit `581f848b` ("Keep writing until the
  store watch fires") worked around the same miss in `ui/tickets`.
- The budget tests wait up to 10 s for 2 or 3 fake sessions to start (`startBudgetRuns`,
  `server/budget_limits_test.go:401`). Alone, passing runs took 3, 5, 7 or 9 s: steps of 2 s, the
  store poll interval (`defaultPollInterval`, `server/watch.go:17`). So each extra launch seems to
  wait for a poll tick instead of a watch event. When the ticks fall badly, 10 s is not enough.
  This link to group 1 is a guess from the timing. The test logs do not show it.
- Follow-up `gx:follow-ups/06` saw the same two tests fail (its tickets 29 and 33a).

### 2. Races with the fake agent

| Test | Suite | Alone | Message |
|---|---|---|---|
| `TestDraft_ActionDraftBeforeLaterStepIsCleared` | 2 of 7 | 0 of 10 | `sessions_draft_test.go:70: submitted = "\"first-step\"\n\"xsecond-step\"\n", want "\"first-step\"\n\"second-step\"\n"` (each failure took 10 s) |
| `TestDesignCleanup_RescanWithoutStepChangeDoesNotCheckAgain` | 1 of 7 | 1 of 10 | `designcleanup_test.go:208: an unchanged step ran the cleanup check again` |
| `TestSessionName_FallsBackToDisplayNameThenProjectN` | 1 of 7 | 0 of 10 | `sessions_test.go:416: open .../T/gx-agentfake/<uuid>.ctl: device not configured` |

- The `device not configured` error (ENXIO) comes from opening the fake program's control FIFO
  before the program has opened it for reading.
- Side finding: `testutil/agentfake` keeps its FIFOs in a fixed `$TMPDIR/gx-agentfake` directory
  (`testutil/agentfake/program.go:49`). It is shared by every test process on the machine and is
  never cleaned. It held 116,678 files after these runs. That breaks the "tests never read live
  machine state" rule in `.ai/index.md`.

### 3. Not a flake

- `webui/src/stats/StatsPage.test.tsx` "the Grilling section sits between Cost and effort and
  Friction" failed 6 of 6 suite runs and 1 of 1 alone. This is `gx:follow-ups/11`. It alone keeps
  `make web-test` red.

### 4. Vitest under load

- `webui/src/design/ChatLog.test.tsx` "a successful send leaves focus mode; a refused send keeps it
  and shows the error" failed 1 of 6 (`ChatLog.test.tsx:477`, `expected false to be true` on
  `inFocus()`), and 0 of 10 alone. Round 2 had a load average near 30.

### 5. Playwright under load

`retries: 0` and an `expect` timeout of 10 s (`webui/e2e/playwright.config.ts`).

| Flow | Suite (3 runs) | Alone (3 runs) | Error |
|---|---|---|---|
| `phone.e2e.ts` 24. Phone, Design page | 2 | 2 | `phone.e2e.ts:151` popover bottom 169 is not below the sheet top 412 |
| `design.e2e.ts` 45. Delete design | 2 | 0 | `Delete design` button not visible in 10 s |
| `design.e2e.ts` 47. Send popover | 1 | 0 | `Unanswered questions` alertdialog still open after 10 s |
| `epics.e2e.ts` 2. Epic looks right | 1 | 0 | `getByText('4/7')` not visible in 10 s |
| `phone.e2e.ts` 23. Phone, Epics page | 1 | 0 | `← Epics · 9 more` link not visible in 10 s |
| `phone.e2e.ts` 53. Phone, epic page, full-width tap targets | 0 | 1 | |

- Four of the six failures were in run 2, under the highest load. Most are 10 s visibility
  timeouts.
- Flow 24 failed 4 of 6 times. Its error is a layout position, not a timeout, so it may be a real
  layout bug that only some timings show.

### Known failures that did not show up

- `TestRecovery_TimedOutCompactionIsReWaitedOnceThenFinishedUpByR7` (`gx:follow-ups/06`, ticket 02)
  passed in all 7 server runs.

## Notes on the known follow-ups

- `gx:follow-ups/06`: confirmed and worse. The unsharded server run is now 11.3 min, over the
  10-minute timeout, and its flakes are groups 1 and 2 above. Sharding cuts the time to about
  3 min but does not remove the flakes.
- `gx:follow-ups/07`: when any shard fails, `scripts/test-shards.sh` prints every shard's output.
  That includes each failing test's full server log and herdrfake dump. The last line is
  `FAIL ./server (sharded x5)`, with no list of failing test names. So `| tail` shows no names,
  which matches what the follow-up saw.
- `gx:follow-ups/11`: confirmed, deterministic. See group 3.

## Proposed targets

### Full-suite time: N = 4 minutes

N is the wall time of `make test` plus `make web-test` on this Mac under normal agent load. Today it
is 6.1 to 7.0 minutes. E2E gets its own limit of 2 minutes (today 1.8 to 3.0 minutes).

Why 4:

- The steps run one after another. The two largest are the server shards (150 to 178 s) and the
  other Go packages (100 to 123 s, set by `testutil/gxfixture` and `cmd` at about 90 s each).
- In the server package, 205 tests take 473 s of its 690 s. Many wait on fixed timers: the 2 s
  store poll, 10 s waits. Waiting for an event instead of a tick shortens both the flaky tests and
  the slow ones.
- `nativerunner` under `-race` is 80 s by itself and sets the race step.
- Halving the server step and the two largest packages gets to about 4 minutes without new
  hardware or more shards. A lower target would need structural change (running the steps side by
  side, more shards), which is a design question for later tickets.

### Flakes: zero failures in M = 20 runs in a row

One run is the full suite above plus `make web-e2e`, on this Mac, with other agents running.

Why 20:

- Today 0 of 6 runs pass, so any M is a big step. The deterministic `StatsPage` failure has to be
  fixed first.
- With 20 clean runs in a row, a failure rate above about 14% per run is ruled out at 95%
  confidence (0.86^20 is about 0.05). 10 runs only rule out about 26%. That is too loose, because
  most agents run the suite more than once per ticket.
- 20 runs at about 8 minutes each fit in under 3 hours, so one unattended evening can check the
  target.
- The runs must be under real load. Most of the failures above show up only with other agents
  running, so a quiet-machine run would pass and still leave agents with flakes.

## Top items to fix

1. Store-watch misses (group 1): fails almost every sharded run and slows the budget tests.
2. `./server` unsharded time of 11.3 min, over the 10-minute timeout. Its slowest tests are the
   21 s `TestLateFinish_FailedConditionStaysPaused` and the 10 to 13 s runner and config-reload
   tests.
3. `StatsPage` grilling-order test: red every run (`gx:follow-ups/11`).
4. `testutil/gxfixture` and `cmd`, about 90 s each, which set the non-server Go step.
5. Playwright flow 24 "Phone, Design page" (4 of 6 failures) and the other e2e timeouts under load.
