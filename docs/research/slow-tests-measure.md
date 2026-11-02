# Measuring the test suite (gx:slow-tests/01)

Question: where does the time in `gx check --full` go, and why did some recorded runs take 10 to 30 minutes?

Measured on 2026-11-02 on the dev Mac (10 cores, 32 GB), which is shared with live agents. Branch `research/slow-tests-measure`, based on main at `a3cad310`. No product code or tests were changed.

## How to read the numbers

- The machine was never quiet. Load average (1, 5, 15 minutes) is next to every timing. Load went from 4 to 63 during one run. No idle-machine timing exists. Treat every wall time as "under normal agent load".
- `gx check --full` is `scripts/test.sh && make web-e2e` (`.gx/checks.json`). `&&` stops at the first failure, so the e2e step probably did not run in most recorded full runs. See section 1.
- Per-test times come from `go test -json` logs that `gx brief` keeps in `~/.local/share/gx/brief/` and from Playwright's list reporter.
- "Leaf seconds" means the sum of the elapsed time of tests that have no subtests. With `t.Parallel` it is CPU-time-like work, not wall time. Parent tests with parallel subtests show a long elapsed time that is only queue time, so they are left out.
- Anything not established is marked **unknown**.

## Summary

- A full run on current main does not pass on this machine. Both runs I did failed in `race`, `go` and `server`. `web` passed.
- Most of the wall time is failing or hanging tests that wait for a timeout, not slow passing tests. In my run, the 26 failing `./server` tests used 792 of 1126 leaf seconds, and the package then hit the 10 minute `go test` timeout.
- Three separate causes produce the 10 to 30 minute `gx check` runs:
  1. `./server` needs about 5 minutes with the `GOMAXPROCS=2` cap on a moderately busy machine, so it sits near the 10 minute `go test` timeout. When the machine is busy, its leaf seconds double and the timeout kills it. This explains 26 of the 32 recorded runs of 10 minutes or more.
  2. A test in `./ralphloop` hangs until the package timeout (10 minutes). It fails on a `git commit` inside its fake and then waits forever. 4 of the 32 runs.
  3. On code with the PTY ready-wait change, every PTY agent launch in `server`, `servertest` and `gxfixture` tests waits 30 seconds. The 30 minute `--since` run is this plus a broken branch: four packages ran to about 10 minutes each, two at a time.
- No evidence of waiting on a lock. The time is inside `go test`.
- The suite grew. Go tests +25 percent and `./server` tests +56 percent in 5 days, Playwright tests x4.4.
- A 4 minute full check is not reachable by one fix. Even in a healthy state, `race`, `server` and `web` each take about 4 to 5 minutes on their own, and e2e comes after them. See section 6.

## 1. What the recorded history says

Source: `~/.local/share/gx/check/gx.jsonl` (927 records, 2026-10-27 to 2026-11-02).

Test-step duration for runs with a test step that did not skip, by scope and day (UTC dates):

| scope | day | runs | passed | median s | max s | runs >= 600 s |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| changed | 10-28 | 74 | 58 | 33 | 652 | 3 |
| changed | 10-29 | 191 | 170 | 160 | 336 | 0 |
| changed | 10-30 | 96 | 83 | 45 | 304 | 0 |
| changed | 10-31 | 146 | 121 | 55 | 375 | 0 |
| changed | 11-01 | 215 | 140 | 97 | 760 | 23 |
| changed | 11-02 | 18 | 7 | 465 | 621 | 5 |
| full | 10-28 to 11-02 | 37 | 2 | 240 to 333 | 640 | 1 |
| since | 11-01 | 17 | 6 | 378 | 605 | 1 |
| since | 11-02 | 2 | 0 | 1215 | 1828 | 1 |

- The 28 `changed` runs of 10 to 19 minutes on Nov 1-2 are confirmed (23 on Nov 1, 5 on Nov 2). Three more happened on Oct 28.
- Recorded `full` runs take 4 to 6 minutes (three took 9 to 10:40). 35 of 37 failed. The `&&` skips `make web-e2e` when `test.sh` fails, and the failure lists name Go tests, so most of these runs probably never reached web-e2e. The recorded full times say little about web-e2e.
- The 1828 second `--since` run (limit-burn-rate-impl, 2026-11-02 07:12 UTC): `go-test` 28m36s plus `vitest` 1m52s = 1828 s. No gap, so no waiting outside the test commands.

## 2. Step wall times, side by side

`scripts/test.sh` runs `race`, `go`, `server` and `web` as parallel processes sharing a CPU budget (half the cores, so `GOMAXPROCS=2` for `go` and `server`, 1 for `race`, one vitest worker for `web`).

| run | commit | wall | race | go | server | web | load at start / during / end |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| A: `scripts/test.sh` | main `a3cad310` | 1264 s (21:04) | 509 s FAIL | 1257 s FAIL | 629 s FAIL | 662 s PASS | 14.4 / 63 then 25 to 40 / 4.3 |
| B: `scripts/test.sh` | `df88a571` (parent of the ready-wait commit) | 948 s (15:48) | 328 s FAIL | 942 s FAIL | 612 s FAIL | 680 s PASS | 6.2 / 36 to 51 / 17.6 |
| `make test` on 2026-10-28 (better-tests ticket 38b) | before the GOMAXPROCS cap | 217 s | 53 s | 85 s | 212 s | 92 s | not recorded |
| CI, ubuntu, main `a3cad310` (run 41408978536) | | 34 min job | 256 s PASS | 2039 s FAIL | 628 s FAIL | 297 s PASS | n/a |

Run B is not a clean baseline: that commit sits in the middle of an epic and fails four guard tests (envguard, migrations schema snapshot, oldwords, sleepguard). They fail in under a second, so they do not change the timings.

Where the time went inside each step in run A (main):

- `race` (509 s): `./nativerunner` 307.7 s, one failing test (`TestPty_LongPromptDeliveredAtNarrowWidth`, 6.6 s). Its 279 passing leaf tests add up to 300 leaf seconds under `GOMAXPROCS=1`. The Codex approval tests are the slow ones (21.6, 21.6, 15.8, 11.4, 11.4 s). The other five race packages take 24 s or less.
- `go` (1257 s): `testutil/gxfixture` 628 s (hit 10 m timeout, 7 failed leaves = 417 leaf seconds), `ralphloop` 600 s (hang, section 3), `server/servertest` 339 s (10 failed leaves = 460 leaf seconds; passing PTY tests take 31 s each), `cmd` 180 s (passes, 1076 leaves, 222 leaf seconds), `git` 50 s, `oldwords` 29 s (fail), `ui/app` 28 s. All other packages are under 25 s. The step is `go test` of about 100 packages with `-p 2`, so the 10 minute packages run two at a time and the step ends only after the last one.
- `server` (629 s): killed by the 10 minute timeout. 708 leaf tests passed (334 leaf seconds), 26 failed (792 leaf seconds, 24 of them took 30 to 62 s each). Only 734 of about 1890 leaf tests had finished.
- `web` (662 s): `tsc` plus `vitest --maxWorkers=1`, 1301 tests. In history, the same vitest run with default workers took 130 to 324 s (Nov 1-2). CI took 297 s.

Run B insights: `server` again hit 10 minutes with only 4 failures (53 s). 1514 passing leaf tests added up to 1144 leaf seconds, double the 530 to 630 leaf seconds that quiet runs show for the same tests (section 3, cause 1). `servertest` passed in 83 s there, against 339 s (with failures) on main.

### Fastest single checks I made

PTY launch cost, one test, alone, no other load of mine (load 5):

| test | `df88a571` | main `a3cad310` |
| --- | ---: | ---: |
| `servertest.TestPty_TheRunRecordsThePtyRunner` | 0.64 s | 30.48 s |
| `servertest.TestPty_TurnStallLeavesTheRunWorkingWithNoTurnEnd` | 0.75 s | 30.67 s |

## 3. Why some `gx check` runs took 10 to 30 minutes

Every one of the 32 recorded runs with a test step of 595 to 700 s, plus the 1828 s run, was spent inside `go test`, and each has a package at about 600 s. Counting by the package that ran to the timeout (from the `gx brief` go-test logs):

| package at about 600 s | runs |
| --- | ---: |
| `./server` alone | 24 |
| `./server` and `./ralphloop` | 2 |
| `./ralphloop` alone | 2 |
| `./testutil/gxfixture` | 2 |
| `./server/servertest` | 1 |
| no brief log kept (the 640 s full run of Oct 30) | 1 |

### Cause 1: `./server` is near the 10 minute timeout and doubles under load

- `go test -json` logs of full `./server` runs on Nov 1:
  - 10:00 to 17:00 UTC: package time 275 to 390 s, passing leaf seconds 530 to 770, about 1800 to 1900 leaf tests, slowest tests 10 to 11 s.
  - 19:00 to 23:00 UTC: package time 480 to 607 s (many hit 600), passing leaf seconds 950 to 1200, about the same leaf count and the same slowest tests.
  - So the same tests cost about twice the time in the evening. No test got slower by itself. This fits CPU contention. Machine load was not recorded then, so the cause is **unknown**. `gx brief go-test` runs overlapped by an average of 1.8 to 2.0 at 09:00 to 12:00 UTC and 1.5 to 2.7 at 19:00 to 22:00 UTC, which does not explain it alone.
- Package time of full `./server` runs (median of runs of 60 s or more, UTC): Oct 25 405 s, Oct 26 465 s, Oct 27 549 s, Oct 28 211 s (after better-tests), Oct 29 249 s, Oct 30 245 s, Oct 31 279 s, Nov 1 309 s, Nov 2 351 s.
- `scripts/test.sh` and `gx brief go-test` cap `GOMAXPROCS` at two fifths of half the cores. On 10 cores that is 2, so `go test -parallel` is 2. Before the cap (commit `391c9b88`, Oct 28 23:39 +02) `go test ./server` ran at -parallel 10 and took 138.6 s on a quiet run (ticket 38b). After it the median is 245 to 351 s. I did not measure `./server` alone without the cap, so the exact cost of the cap is **unknown**; the numbers above suggest about 100 to 150 s.
- Many `-run` outputs show `TestReclaim_DeadClaimWithWorkPauses` (4 to 7 minutes) as "still running" at the timeout. That is a false lead. Its 4 subtests take about 1 s each. The parent test shows a long elapsed time only because its parallel subtests queue behind about 1000 other paused tests for the two parallel slots.

### Cause 2: a `ralphloop` test hangs for the whole timeout

- `TestRun_FullEpicRealGit_TelegramNotifications_AllValidMarkdownV2` is the test running at 9m52s when `./ralphloop` times out. It appears in all 4 `ralphloop` timeouts since Nov 1 22:41 UTC and in both of my runs.
- What happens (from my run A log): the fake's `agent prompt` handler calls `commitIterationWork`, which fails with `git commit: nothing to commit, working tree clean`. The handler reports an error, and `Run()` at `ralphloop/full_epic_notifications_e2e_test.go:177` never returns.
- The watchdog `realGitTimeoutWatchdog` fires after 30 s and calls `t.Fatalf` from its own goroutine. That only ends the watchdog goroutine. The test goroutine keeps waiting, so the package waits for the `go test` timeout.
- Why the `git commit` finds nothing to commit is **unknown**. It started on Nov 1 around 22:41 UTC.
- `brief/gotest.go` now defaults to `-timeout=30m` (commit `4f2e4f5d`, Nov 2 08:11 +02). A hang like this one now costs up to 30 minutes, not 10. `scripts/test.sh` still passes `-timeout=10m`.

### Cause 3: PTY launches wait 30 seconds

- Commit `6b7dbd91` (agent-lifecycle-impl/22, Nov 1 23:02 UTC; it is on main as of `a3cad310`, the date it reached main is unknown) added `awaitReady` in `nativerunner/pty_launch.go`. It waits up to `DefaultReadyWait` (30 s) for the input box on screen and exactly one transcript file, then sends the prompt anyway.
- With the fake claude used by `servertest`, `gxfixture` and `server`, the signal never arrives, so every PTY launch waits the full 30 s. Measured in section 2: 0.64 s before, 30.5 s after. The log says `ready signal missing, sending the launch prompt anyway`.
- In run A, failing tests that wait on a PTY launch used 30 to 62 s each. On branches with this commit, a package of such tests takes as long as the sum of those waits divided by 2.
- Why the signal is missing with the fake claude is **unknown**.
- The 30 minute `--since` run shows the same warning (`ready signal missing`, waited 30 s) in its log, so that branch had this commit.
- Not all slow gxfixture tests come from it: `gxfixture` also hit 641 s in run B at `df88a571`, which does not have the ready-wait. Its failing scenario tests took 33 to 78 s each. The cause there is **unknown**.

### The 30 minute `--since` run

- Command: `gx brief go-test ./brief ./cmd ./codexsession ./migrations ./ralphloop ./server ./server/servertest ./testutil/gxfixture ./tokenledger` (9 packages), then vitest.
- With `-p 2` and a 10 minute timeout per package: `server` 600 s, `ralphloop` 600 s, `gxfixture` 624 s, `servertest` 371 s (120 s and 90 s tests that waited on 30 s timeouts), `cmd` 85 s, plus compile time. About 2 x 14 minutes. Together with 1m52s of vitest that is the recorded 1828 s.
- The branch was also broken: `migrations.TestSchemaSnapshot` failed and 13 worktree-removal tests failed at 30 s each. Those are failing tests waiting for timeouts, not slow passes.

### Waiting versus running

- Lock waiting: none seen. The check's test duration matches the sum of its `go test` and `vitest` durations to the second for the long runs I compared (603.93 s against `go-test` 10m3.91s; 1828 s against 28m36s plus 1m52s).
- The one lock in play, `gx-gocache.lock` (`gocachegate`), is taken shared by `gx brief` builds and exclusive by cache cleaners. I found no run where it delayed a test. Whether a long cache clean ever blocked a build is **unknown**.
- Slow passes: `./server` (cause 1) is the only package that is both slow and mostly passing. Everything else in the 10 to 30 minute runs is a hang (cause 2) or failing tests waiting for timeouts (cause 3 and the broken branches).

## 4. How the suite grew since better-tests (2026-10-28)

better-tests ticket 38b measured `go test ./server` at 2:21 and `make test` at 3:37 (race 53, go 85, server 212, web 92 s) on Oct 28.

Counts at main on each day (`git grep`):

| main at | Go `func Test` (all) | in `server/` | Playwright `test(` | e2e files | vitest files |
| --- | ---: | ---: | ---: | ---: | ---: |
| 10-28 12:00 | 5850 | 987 | 42 | 11 | 58 |
| 10-29 12:00 | 6168 | 1114 | 56 | 14 | 65 |
| 10-30 12:00 | 6436 | 1286 | 101 | 24 | 88 |
| 10-31 12:00 | 6558 | 1332 | 105 | 24 | 90 |
| 11-01 12:00 | 6930 | 1452 | 135 | 30 | 108 |
| 11-02 08:00 | 7306 | 1540 | 183 | 37 | 115 |

- Go tests +25 percent, `./server` +56 percent, Playwright x4.4 (42 to 183 `test(` calls; 320 cases in the current run list), vitest tests 962 (Oct 29) to 1301 (Nov 2).
- Playwright full-suite runs recorded by `gx brief` (list the whole suite): Oct 28 112 tests in 73 to 82 s (all pass); Oct 29 14:28 UTC 86 to 92 s; Oct 29 23:09 165 s; Oct 30 403 s, 346 s, 320 s, 370 s; Nov 1 15:49 UTC 294 tests in 591 s (failing).
- Vitest full runs: 37 to 86 s for 962 tests (Oct 29) to 130 to 324 s for 1301 tests (Nov 1-2).
- CI job times on main (`gh run view`):

| main commit date | web-e2e | test (ubuntu) |
| --- | ---: | ---: |
| 10-28 21:36 UTC | 148 s pass | 343 s |
| 10-29 14:05 | 207 s fail | 317 s |
| 10-30 21:21 | 413 s fail | 461 s |
| 10-31 20:35 | 550 s fail | 533 s |
| 11-01 17:10 | 1032 s fail | 859 s |
| 11-01 21:58 | 1185 s fail (11 failed, 18m54 in Playwright) | 791 s |
| 11-02 07:51 (HEAD) | 2009 s fail (34 failed, 32m35 in Playwright) | 2061 s |

## 5. Playwright (web-e2e)

- Config: `webui/e2e/playwright.config.ts`: 3 workers, 60 s test timeout, no retries, each flow starts its own fixture server. Global setup builds the Go binaries once.
- I started `make web-e2e`'s Playwright step alone on main (load 4.4 at start, 320 tests). It ran too slowly to finish inside the time box, so I stopped it after about 3 minutes with 11 tests done. Every agent flow took 39 to 43 s (`agents-commands-output`, `agents-commands-send`), and two of the 11 failed (`154. Commands in a design session`, `/clear and /compact` 50.4 s and `keys and Type` 26.4 s).
- The same agent flows take 37 to 43 s at `df88a571`, which is before the ready-wait commit. So the 40 s per agent flow is not explained by cause 3. Where it comes from is **unknown**.
- The wall time of the whole web-e2e step on main is **unknown**. The best data are the 591 s run of Nov 1 (294 tests, failing), the CI jobs above (up to 2009 s with failures), and the 75 to 90 s run of Oct 28 (112 tests).
- Playwright flows that fail wait for the 60 s test timeout. CI failures of Nov 1 are mostly 10 s `expect` timeouts and 60 s test timeouts.

## 6. What a 4 minute `gx check --full` would need

Today's `gx check --full` is `test.sh` (the slowest of four parallel steps), then the `web-build` (about 13 s) and Playwright. Wall is roughly the slowest step plus build plus web-e2e. All time estimates below are my estimates from the data above, not measurements of the change, and they assume the machine load seen in the history.

Healthy-state timings (no failures, no hangs) for each step, best evidence:

| step | healthy time with today's settings | source |
| --- | ---: | --- |
| race | 270 s | `nativerunner` 269 s in run B |
| go | about 150 to 250 s, **unknown** | `cmd` 126 to 180 s, `servertest` 83 s; `gxfixture` and `ralphloop` never passed in my runs |
| server | 245 to 350 s | package time on quiet to normal days |
| web | 300 to 680 s | CI 297 s; 662 to 680 s locally with 1 worker |
| web-e2e | **unknown**, 75 to 90 s for 112 tests on Oct 28; 591 s for 294 tests on Nov 1 | history |

So even with every failure fixed, `race`, `server` and `web` are each 4.5 to 11 minutes, and e2e comes after them. A 4 minute target needs each of these cut by about half, plus e2e at 60 to 90 s or run side by side.

Options, with an estimated saving and what each depends on:

| # | change | where | saving (estimate) | basis |
| --- | --- | --- | --- | --- |
| 1 | Make the hung `ralphloop` Telegram test fail fast (stop `Run` when the watchdog fires; fix the `git commit` that finds nothing) | go | 0 s on a green day; 600 s (or 1800 s with the new 30m default) on every run that includes `ralphloop` | 4 of 32 long runs, both of my runs |
| 2 | Remove the 30 s ready-wait from the fake claude path (make the fake emit the signal, or give tests a short `ReadyWait`) | go, server | `servertest` 339 s to about 83 s (measured on commits); `server` 100 to 400 s when failing tests wait on launches | 0.64 s against 30.5 s per launch |
| 3 | Find out why `gxfixture` scenario tests wait 30 to 78 s (fails in run B too) | go | up to 400 s of the `go` step | 7 failed leaves = 417 leaf seconds in run A |
| 4 | Lift the `GOMAXPROCS=2` cap for `server`, e.g. 4 | server | 100 to 150 s when the machine has idle cores; small under heavy load | leaf seconds 530 to 600 at parallel 2 on quiet days |
| 5 | Run vitest with 2 to 4 workers in `test.sh` (now 1) | web | 150 to 400 s locally | 662 s against 130 to 324 s with default workers |
| 6 | Shard or trim the slow Codex approval tests in `nativerunner` (5 tests, about 82 s) or drop `-race` for them | race | 60 to 120 s | per-test times in run A |
| 7 | Raise Playwright workers above 3 on the dev Mac; find why one agent flow takes 40 s | web-e2e | 30 to 60 percent of e2e wall time, **unknown** until the 40 s is explained | 320 cases at 3 workers |
| 8 | Start `make web-e2e` at the same time as `scripts/test.sh` instead of after it | whole check | up to the shorter of the two (about 5 minutes) minus the extra contention; **unknown** under load | the steps are independent |
| 9 | Use a per-test timeout (or `-timeout` at 2 to 3 minutes per package) so a hang cannot cost 10 to 30 minutes | go | caps a hang at the new limit; no change on green runs | cause 2, 30m default |
| 10 | Trim duplicate or low-value tests (`server` has about 1890 leaf tests, `cmd` 1076) | all | proportional: 10 percent of leaf tests saves about 10 percent of the step; **unknown** which tests | needs ticket 3 of the map |

Sizing: items 1 to 3 restore the passing state and remove the 10 to 30 minute tail. Items 4 to 8 are what cut the healthy 4.5 to 11 minute steps toward 4 minutes. Which of them to do belongs to the person.

## 7. Unknowns

- An idle-machine timing for any step.
- The wall time of the whole web-e2e step on main.
- Why the Telegram test's `git commit` finds nothing to commit.
- Why the fake claude never gives the ready signal, and why gxfixture scenarios wait 30 to 78 s at `df88a571`.
- Why every agent e2e flow takes about 40 s at both `df88a571` and main.
- The cause of the evening slowdown on Nov 1 (machine load was not recorded).
- The cost of the `GOMAXPROCS` cap measured directly (`./server` with and without it).
- Whether `gocachegate` ever delayed a build.

## 8. Sources and commands

- `~/.local/share/gx/check/gx.jsonl` and `<run>/summary.txt`, `<run>/test.log`.
- `~/.local/share/gx/brief/<run>-go-test.log` (full `go test -json`), `index.jsonl` (per command durations).
- My runs: `scripts/test.sh` at main and at `df88a571`, written per step into the brief logs `20261102T081110.*` (A) and `20261102T083717.*` (B).
- `gh run view 41408978536`, `41354669925`, `41337974141`, `41330125466`, `41239641240`, `41130384107`, `40888072207`, `40773288533` for CI.
- `git grep -E '^func Test'` at the main commit before each date.
- Analysis was ad hoc Python over those JSON logs (leaf-test sums: tests with no subtests, grouped by package); the scripts are not kept.
