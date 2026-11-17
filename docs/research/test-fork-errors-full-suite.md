# gx:test-suite-stability/09 - Full-suite fork errors

Question: what uses the process headroom during a full `gx check`, and what causes `fork/exec ...: resource temporarily unavailable`?

## Answer in short

- Reproduced on the first full capped run. Cause: `scripts/proc-cap` fixes each test binary's limit as (uid process count at that binary's start) + 200. The count rises 200 to 450 during a normal run, and the `gxfixture` burst test adds 240 more at once. When the uid count passes a binary's frozen limit, every fork in that binary fails.
- The 200 headroom is too small for a full run. `GX_PROC_HEADROOM=600` gave zero fork errors in a second full run that also contained a burst.
- Fix for the impl epic: raise the default headroom to 600 (cap stays on), and stop one test package from spending the shared headroom of its siblings (see "Fix").

## Method

- Sampler: `/private/tmp/claude-501/-Users-david-dev-gx-test-suite-stability-design/d1536974-9324-48ad-950d-7804404bd1b2/scratchpad/sampler.sh`. Once a second: `ps -u $UID -o pid,ppid,comm`, uid process count, `git` count, top 5 process names. It copies the full `ps` listing whenever the count sets a new peak.
- Run: `gx check --full --kind test` (the `test` kind of `.gx/checks.json`: `scripts/test.sh` with `GX_TEST_E2E=1`, so race, go, server, vitest and Playwright side by side). `gx check` applies the cap itself; no extra wrapper. Worktree `test-suite-stability-design`, 2026-11-17.
- Fork errors counted with `rg "resource temporarily"` in the check `test.log` and the `go test -json` brief logs, then matched to the sampler by timestamp.
- Machine: 12 cores, other sessions running (see run 1). `kern.maxprocperuid` 4000, `kern.maxproc` 6000.

## Facts

### Run 1: default headroom (200)

| Item | Value |
|---|---|
| Start count (first sample) | 584 |
| Count range | 471 to 939; median 571, p90 663 |
| Peak | 939 at 07:29:48, of which 252 were `sleep`, all children of one `sh` (pid 30166, 240 `sleep` under it) |
| Samples above 800 | 11 |
| Top names at peak | sleep 252, gx 193, claude 106, codex 39, WebKit.WebContent 15 |
| `git` processes | 0 to 2 at any sample; never a factor |
| Wall time | 799 s; check reports `fail test 792s` |
| Fork errors | present: 9 lines in the check `test.log`; in the `go test -json` logs of my run, 101 lines in the go step (`designflows`) and 35 in the server step. The other session's runs logged 100 and 81 more |
| Packages hit | `server/prflow` (13 tests), `server/recovery` (R6, R3), `server/triage` (5 tests), `server/ticketops` (2), `testutil/gxfixture/designflows` (about 40 tests) |

Fork-error times, matched to the sampler (local time):

| Error time | Package(s) hit | Uid count at that second | What was running |
|---|---|---|---|
| 07:27:22 | `designflows` (other session, `bg-progress-impl`, run started 07:26:05) | 701 to 722 (count was 507 at 07:26:05, so its cap was about 707) | no burst; the count ramped from 497 to 728 over two minutes |
| 07:29:36 to 07:29:37 | `designflows` (other session, started 07:27:48 at count 623, cap about 823), `designflows` and `prflow` and `recovery` (my run) | 717 -> 902 -> 920 within two seconds | `TestProcessBurstUnderDefaultLimitsIsNotKilledAndTheTicketFinishes` in `testutil/gxfixture` of my run: `run` at 07:29:35.19, `pass` at 07:29:49.46 |
| 07:34:30 to 07:34:31 | `triage`, `ticketops` (my run) | 518 -> 738 within two seconds | the same burst test in the other session's run (`bg-progress-impl`, `./testutil/gxfixture`: `run` at 07:34:29.56, `pass` at 07:34:43.60) |

- The `sleep` processes come from `agentfake.BehaviourBurst`: `BurstDescendants = 240` processes held for `BurstHold = 13 s` (`testutil/agentfake/worker.go`). The scenario `ProcessBurst` (`testutil/gxfixture/spec/storm.go`) exists to prove a 240-process tree is not killed by the default watchdog.
- That test calls `raiseProcessCap(t, 400)` (`testutil/gxfixture/storm_test.go`) so its own tree fits. The raise applies to that binary only. Uid-wide, the 240 new processes still count against every other binary's frozen limit.
- All fork errors in the run are at or just after these two burst starts and the one ramp crossing. The error text is from tests in packages with no process-heavy work of their own (`git init --bare`, `git worktree add`, `fork/exec gxfixture`).
- Each failed test then waits on its timeouts, so the log line time of the failure can be 15 s to 60 s after the fork error (`07:29:37` in the embedded log, `07:30:37` as the test failure time).
- The same 10 deterministic Codex and debounce failures as ticket 05 appear in every run (`TestConfigReload_StoreCommitDebounceApplies`, 7 `TestSessionAnswer_CodexQuestion*` tests and 2 subtests). They are not fork errors (ticket 06).
- Leftover processes were not checked after the run; the sampler shows the `sleep` count dropping right after each burst.

### Run 2: headroom 600 (one change), default `-p`

Command: `GX_PROC_HEADROOM=600 gx check --full --kind test`. Start count 522; no other session running a go test (the index shows only my run).

| Item | Value |
|---|---|
| Count range | 422 to 818; median 513, p90 580 |
| Peak | 818 at 07:50:26 with 243 `sleep` (the burst ran in this run too, 07:43:35) |
| Fork errors | 0 (check `test.log` and all three go-test logs) |
| Failing tests | only the 10 deterministic ones listed above |
| Wall time | 710 s |

The `-p` change was not run. The ticket asks for it only if headroom 600 still shows fork errors.

Samples to compare, run 1 vs run 2:

- Run 1 limits: base at binary start + 200 = about 700 to 800. Count peaked at 939. Errors.
- Run 2 limits: base at binary start + 600 = about 1020 to 1200. Count peaked at 818. No errors.

## Hypotheses (not proven)

- H1 (well supported): a fork error happens when the uid count passes a test binary's frozen limit. Evidence: three error bursts, each at the second the count crossed (or came within one sample of) the limit estimated from the count at that binary's start; none with headroom 600 and the same burst present.
- H2 (supported): a normal full run needs about 250 headroom on its own (count 497 to 728 in two minutes at 07:26 to 07:28 with no burst). With the 200 default, any binary that starts in a trough (count about 500) fails once the other steps ramp up. That explains the 07:27:22 errors with no burst.
- H3 (supported): the burst test adds a fixed 240 on top, and it does so to any session on the uid. Two sessions running `gx check` at once is the Nov 16 shape: one session's burst failed the other's `prflow`, `sched` and `triage` tests.
- H4 (not checked): other sessions' agents add to the count outside tests. The sampler shows `claude` 33 to 112 and `codex` 21 to 44 processes; those belong to the fixture harness, but I did not trace each to its parent session.
- Not the cause: a `git` leak (0 to 2 `git` processes at any sample), the `-p` setting (not changed), the package set (the four Nov 16 packages alone never hit the cap: ticket 05).

## Fix for the impl epic

1. Default headroom 200 -> 600 in `scripts/proc-cap` and wherever `gx brief go-test` and `gx check` set their default (same value, one constant). Keep the cap on. The cap is still far below the 4,658 fork storm the wrapper exists for, and below `kern.maxprocperuid` (4000). Run 2 shows 600 holds with a burst inside the run.
2. Make the headroom derive from what the suite needs, not a round number: `BurstDescendants` (240) + the normal ramp (about 250) + one other session's burst (240) = 730 as an upper bound. 600 passed here; if a validation run on a busier machine shows errors, raise to 800 before touching `-p`.
3. Stop the burst test from starving its siblings in the same run: `TestProcessBurst...` in `testutil/gxfixture` should run in its own `scripts/test.sh` step after the other steps finish (or under a uid-wide lock that the other packages' fixture starts also wait on). Fixing only the headroom leaves the 240-process spike inside the uid; fixing only the scheduling leaves the +250 ramp. Do both, the headroom first.
4. Do not retry the failing packages and do not raise test timeouts: the fork errors are not slow tests.
5. The 10 deterministic failures are ticket 06's.

## Validation (for the impl epic)

- Pass condition: `gx check --full --kind test` with the new default shows 0 `resource temporarily unavailable` lines in the check log and the `go test -json` logs, twice in a row, once while another session runs a go-test job and a `testutil/gxfixture` run.
- Evidence so far: run 2 above (headroom 600, burst present, 0 errors). It was one run with no other heavy job, so a second run under another session's load is still open.
- Count to watch: the sampler's peak minus the lowest binary start count. Fail the impl if it passes 80 % of the new headroom.

## Lessons for the Go skill

- `ulimit -u` on macOS counts every process of the uid, across all sessions, and the limit is fixed at each binary's start. A cap of "count + N" is a promise about that moment only. Size N from a measured peak of the whole run, not from one package.
- A test that starts many processes (a burst, a storm) charges them to every concurrent test binary and every other session, even if it raises its own limit. Run such tests in their own step, or lock them uid-wide.
- A fork error in a package with no process-heavy work is a sign of someone else's spike: read the uid count at that second before reading the failing test.
- Sample the process count once a second during any full run you tune a cap for. A table of counts by second answered this ticket; the failing test names alone did not.
- Do not retry or lengthen timeouts for `resource temporarily unavailable`. The error is deterministic once the count crosses the limit.
- A fixture helper that raises `RLIMIT_NPROC` for itself (`raiseProcessCap`) must be paired with a note that its siblings do not get the raise.

## Raw files

Scratchpad `/private/tmp/claude-501/-Users-david-dev-gx-test-suite-stability-design/d1536974-9324-48ad-950d-7804404bd1b2/scratchpad/`: `run1.csv`, `run2.csv` (epoch, uid count, git count, top 5 names), `run1.peak`, `run2.peak` (full `ps` at peak), `run1.out`, `run2.out`. Logs: `check/20261117T052352.910130000/test.log`, `check/20261117T053848.730073000/test.log`, and the `brief/20261117T052358.49*-go-test.log`, `20261117T052605.872837000`, `20261117T052748.237583000`, `20261117T053324.341682000`, `20261117T0538*` files.
