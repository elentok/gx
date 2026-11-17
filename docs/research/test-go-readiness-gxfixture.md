# gx:test-suite-stability/12 - Go readiness timeouts in gxfixture, pty runner and codex tests

Question: why do the gxfixture, pty runner and codex tests time out under load but pass alone, and what is the fix at the cause?

Date of runs: 2026-11-17, worktree `test-suite-stability-design`. All load runs used `GX_PROC_HEADROOM=600` unless stated, so fork-limit errors (ticket 09) do not mask other causes.

## Answer in short

- Of the 8 tickets, 3 have a cause shown by a reproduction or a read of the code. 4 could not be reproduced today and 1 (the `holder.sock never answered` / `claude exited (code 1)` family) has no proven cause.
- The fork limit explains none of the four signatures in the ticket. In today's full run with headroom 200, fork errors hit about 40 designflows tests, and all of them read `process limit hit (EAGAIN)` or `fork/exec ...: resource temporarily unavailable`. None read `holder.sock never answered`, `already running (pid )`, `claude exited (code 1)` or `timed out waiting for a new conversation id` (0 matches in the go-test logs of ticket 09 runs 1 and 2 that I searched).
- Reproduced under CPU load, no fork errors: `TestSessionScreen_LiveReturnsHolderRows` (ticket test-flakes/15, second test). Cause: the test types keys when only the holder socket is ready, not the agent. Fix validated: 0 failures in 300 runs, against 5 in 300 for the current test.
- Proven by a code read and a race experiment: `already running (pid )` has a real window in `AcquireLock` (go-flaky-tests/07). Who the second lock contender is in the fixture is not proven.
- New find while validating: `webflows.TestBulkActions_*` failed once with `want one new control file, got [two files]`. The seeder diffs a directory after a 500 ms quiet window (a clock), not by session id.

## Method

For each ticket: read the wait and the code it waits for, run the test alone, run its package, then run under load.

- Alone: `gx brief go-test -count=1 -run '^...$' <pkg>`, headroom 600.
- Package: `gx brief go-test -count=1 <pkg>` for `testutil/gxfixture`, `limitsflows`, `webflows`, `server/servertest`, `convflows`, `askflows`, `server/disk`, `nativerunner`.
- Full-suite load: reused the two full `gx check --full --kind test` runs of ticket 09 (run 1 headroom 200, run 2 headroom 600) and read the per-test results from their go-test logs (script in the scratchpad: `res.py`).
- Synthetic load: 24 busy loops (`sh -c 'while :; do :; done'`) on 12 cores, load average 30 to 57, plus the five fixture packages started together. Repeats with `-count`. Headroom 600, so no fork errors.
- Scratchpad: `/private/tmp/claude-501/-Users-david-dev-gx-test-suite-stability-design/d1536974-9324-48ad-950d-7804404bd1b2/scratchpad/` (`res.py`, `cpuload-server.out`, `cpuload-server2.out`, `screen-base.out`, `screen-fixed.out`, `fx1.out` to `fx4.out`, `tmp_screenready_test.go.txt`).

## Facts

### Where the tests stand today

| Run | Result for the 11 named tests |
| --- | --- |
| Alone, headroom 600 | all pass (Hold 1.5 s, ChatSessions 13.5 s with the package build, DeadRunEmptyReopens + DeadMany 9.7 s, Charting 3.1 s, DiskHold 1.9 s, SessionScreen 1.4 s, Codex blocked 57 s including the five-binary build in TestMain). |
| Packages, headroom 600 | `gxfixture` 91 pass (2m), `limitsflows` 46 pass, `servertest`, `convflows`, `askflows`, `disk`, `nativerunner` (230) pass. `webflows`: 1 fail, not a named ticket (see "New find"). |
| Ticket 09 run 1 (headroom 200, with fork errors) | all pass except `TestCodexBlockedScenario_PermissionRequestShowsAPendingCard`, which failed in 0.00 s with `process limit hit (EAGAIN) ... fork/exec .../gxfixture: resource temporarily unavailable` (at 07:27:22 and 07:29:36, matching the sampler). |
| Ticket 09 run 2 (headroom 600) | all 11 pass. Times: DeadRunEmptyReopens 3.9 s, DeadMany 7.2 s, ChatSessions 6.6 s, Hold 1.65 s. |
| CPU load 24 loops: Hold x100, Charting x60 (300 subtests), DiskHold x100 | 0 failures. |
| CPU load: `TestSessionScreen_LiveReturnsHolderRows` x20 / x150 / x300 | 2 / 2 / 5 failures. Each failure is at 0.14 s, not a timeout. |
| Five fixture packages together under load 50: DeadRun, DeadMany, SchedulesRestartShape, MigrateHeld, ChatSessions, Codex blocked, x6 each | 0 failures. |

### Already in place (so older failures predate it)

- Launch slots: `acquireLaunchSlot`, 4 flock slots across all test binaries (`fixturetest.go`, commit `9fca28c49` Nov 12, `410b41cee` Nov 14).
- `-test.parallel` capped at 4 in fixture packages (`ccb0d3e13`, Nov 16).
- `explainStartErr` names EAGAIN (`9fca28c49`, Nov 12).
- Fixture failures print the tail of `holder.err` (`testutil/gxfixture/sessions.go:456`).
- `AcquireLock` retries 2 s when the lock file names this process (`server/lifecycle/lock.go`).
- Every failure signature in `gx brief` logs for `holder.sock never answered` is dated Nov 6 to Nov 15, before or during these fixes.

### Fact: the signatures in the ticket

- `holder.sock never answered`: `ptyLauncher.launchTick` (`nativerunner/pty_launch.go:373`). The text is `holder exited: <holder.err>`, or the fallback `holder exited: holder.sock never answered` when the holder exited and `holder.err` was empty. So in those logs the holder died with nothing on stderr. A fork failure would have left `fork/exec ... resource temporarily unavailable` in `holder.err`.
- `claude exited (code 1) before it turned idle`: `holderExitedErr`. The holder saved exit code 1 of the fake claude. The fake exits 1 on any startup error and prints the error to its tty (`tuiclaude.TUIClaude`), which is not captured in the test log.
- `already running (pid )`: `AcquireLock` read `server.lock` while empty (see experiment below).
- `timed out waiting for a new conversation id`: `rigs.HandoffFixture.NewConversation`, a 10 s `WaitFor` on `meta["conversation_id"]` changing after the fake handles `/clear`.

## Table

| Ticket | Cause | Evidence | Fix | Validation |
| --- | --- | --- | --- | --- |
| test-flakes/15 (test 2), `TestSessionScreen_LiveReturnsHolderRows` | Proven. The test waits for `holder.sock` (the holder answering), then sends keys. The fake claude may not yet be in raw mode and reading. The tty echoes the keys, so `WaitForScreen("typed-on-screen")` passes on the echo. The fake then starts, clears the screen (`\x1b[2J` in `tui_state.go:367`) and redraws as it reads the queued keys one at a time. The server's second `DialHolder().Screen(0)` reads a frame from that redraw: all blank, or `type`. | Under load: rows `"\n\n...\n"` and `"type\n\n..."`; failures at 0.14 s, not at a timeout. 2/20, 2/150, 5/300. Sibling tests (`TestRestartFresh_*`) already send a control to the fake first. | Wait for the agent, not the socket: `servertest.Report(t, conv, agentfake.Control{State: "idle"})` before `SendKeys` (the control is acked, so the fake reads input). Better still: poll `Sessions.Screen` until the rows hold the text. | Variant with `Report(... idle)` first: 0 failures in 300 under load 30, current test 5 in 300 in the same conditions (`screen-fixed.out`, `screen-base.out`). Not applied to the tree. |
| test-flakes/15 (test 1), `TestDiskHold_TwoProjectsOnOneDiskGiveOneEntryAndOneNotice` | Ticket text (Oct 31): `waitFor` 15 s cap at `diskhold_test.go:275`, hold ended by the automatic clean racing the resume. Commit `877bd2872` (Nov 9) now waits for `!Cleaning` before recovering. The wait is a state poll, no clock gate. | 100/100 pass at load 40 to 57, 1 s each. | Already fixed by `877bd2872`. No timeout change. | 100 runs under load. |
| test-flakes/14, `TestChartingHandoff_...` | Not reproduced. The wait (`NewConversation`, 10 s) is a poll of `meta.conversation_id` after the fake `/clear` (fake, then holder watcher, then meta write). The last report is Nov 1. | 5 subtests x 60 runs under load 40 to 57: 0 failures; 0.3 to 0.5 s each. | None proven needed. If it returns: read `meta` and the holder watch state in the failure message. Hypothesis only: the same startup-race shape as the SessionScreen row (a key or `/clear` sent before the fake is reading). | 300 runs under load. |
| go-flaky-tests/01, `TestPty_HoldKeepsTheRunWorkingUntilReleased` | Not reproduced. The release is event-based: `SendTUIControl` waits for the control FIFO and for the fake's ack (up to 5 s) before returning, then the test polls `liveIterations` for 30 s. The Nov 9 failure (30.9 s) ran before the launch cap (Nov 14) and the parallel cap (Nov 16), at load 12.7; whether `proc-cap` was in force is not known. | 100/100 pass at load 30 to 57 in 1.5 s each. | None proven. The 30 s wait is long enough that the only stuck path is a lost ack or a stalled reap; the failure message should include the iteration list. | 100 runs under load. |
| go-flaky-tests/06, `TestChatSessionsScenario_...` | Partly proven (code). The test takes the fixture start (up to 40 s, `startTimeout`), a launch-slot wait with no limit, then `waitForParts` (60 s poll). Their sum is above the 60 s hang limit of `gx brief`, so when the start is slow the test is killed (`HUNG ... ran longer than 1m0s`) before `waitForParts` can print its diagnostic. That explains "hangs 60 s, passes in 6 to 13 s". It does not say why the start was slow. | Passes 6.6 s in loaded full run 2, 6 of 6 under load 50 (about 10 s each). The ticket's chosen diagnostic ("fail at about 40 s and print the missing parts") is not in the code. | Make the nested budgets fit under the hang limit: poll to about 30 s, print the missing parts and capture lines; keep the launch-slot wait out of the poll. Do not raise any timeout. | Not applied. A run under a lowered limit would show the new message. |
| go-flaky-tests/07 and test-flakes/09 (DeadRunEmptyReopens) | Two parts. Proven: `AcquireLock` truncates the file, then writes the pid (`lock.go`), so a second process reading in between reports `already running (pid )`. Not proven: who the second process is in the fixture. Other signatures in these tickets (`holder.sock never answered`, `claude exited (code 1)`) are the unresolved family below. | Experiment: 6 processes calling `AcquireLock` in a loop on one file gave 268 to 1009 `(pid )` reads per 3000 attempts. The Nov 11 failure was 0.88 s into the first fixture start of the test. The test passes today: 4 of 4 in full runs 1 and 2 (3.9 to 4.8 s), 12 of 12 under load 50, 7 of 7 with headroom 3 to 150 (the cap did not bite in these runs, so this is not a fork test). | Remove the window in `AcquireLock`: write the pid in one write of fixed width and never truncate to empty, or make the loser retry the read while empty (the ticket's decision). Find the second contender by logging the lock path and the loser's `ppid` in the fixture. | Experiment above shows the window. Not fixed in the tree. A repeat of the experiment after the fix should give 0 empty reads. |
| test-flakes/09 (the gxfixture package), `TestCodexBlocked...`, `TestLimitBurnFixture_...`, `TestDeadMany` | Mostly the fork limit, plus an old overload. Proven for today: `TestCodexBlockedScenario_PermissionRequestShowsAPendingCard` fails with EAGAIN at the fixture's own fork in run 1 (headroom 200) and passes at headroom 600. Not proven for the `holder.sock never answered` / `claude exited (code 1)` runs of Nov 6 to 14 (see below). The suggested fix (cap parallel server starts) is already in the tree. | Run 1 fail at 07:27:22 and 07:29:36 with named EAGAIN, the burst test starting at 07:29:35. Run 2: pass 1.7 s. 6 of 6 under load. | Ticket 09's fix (headroom 600, burst test in its own step). Nothing else for this test. | Run 2 (0 fork errors, test passes). |
| test-health/09, `TestDeadMany` over the 20 s slow limit | Not reproduced. The test does real work (several server restarts and holder kills). It took 7.2 to 7.5 s in all three loaded full runs, 9.7 s with its sibling alone. The 20.9 s of Nov 15 predates the launch cap (Nov 14) and parallel cap (Nov 16). | 7.4 s, 7.5 s, 7.2 s. | None. Do not raise the limit. | n/a |
| follow-ups/533, `TestSchedulesRestartShape_...`, `TestMigrateHeld_...` | Not reproduced. Both only call `gx server status` through `r.Gx`, which fails the test when the CLI call fails (for example on a fork error), and `waitUntil` polls 30 s. The Nov 16 failure was a loaded full run with the old default headroom. | 1.0 s and 1.6 s in full runs; 12 of 12 under load 50. | Covered by ticket 09 (headroom). If it returns, show the `gx server status` error, not only `timed out waiting for`. | Passes under load and in both full runs. |

### Group A: the agent is not ready, the test acts anyway

- `TestSessionScreen_LiveReturnsHolderRows` (proven).
- Hypothesis for `TestChartingHandoff_*` (`/clear` before the fake reads) and the `TUIClaude` exit-1 family. Not tested.
- Shared cause: a socket, FIFO or pid file that exists is read as "the agent is ready". The agent has a later readiness point (pty raw, first idle report, an acked control).

### Group B: fork limit (ticket 09)

- `TestCodexBlockedScenario_PermissionRequestShowsAPendingCard` in run 1 and everything else that fails with `resource temporarily unavailable` or `process limit hit (EAGAIN)`.
- Not a readiness bug; fix and validation are in `test-fork-errors-full-suite.md`.

### Group C: clock where a signal exists

- `webflows` seeder `settledControlFiles` (new find below), `waitForParts` budget, lock write window.

### Group D: not reproduced, no evidence of a cause

- `holder.sock never answered`, `claude exited (code 1) before it turned idle`, `timed out waiting for a new conversation id`, `TestDeadMany` slow, the `limitsflows` pair, `TestPty_Hold...`.
- Seen in logs of Nov 6 to Nov 15. Zero matches in all of today's logs (alone, package, two full runs, load, repeats).

## New find (not a named ticket)

`webflows.TestBulkActions_SeedsCardsAgentsWorkspacesAndArchivedEpics` failed once in a package run at headroom 600. The test log only says `no output line: EOF`. The cause was on the fixture's stderr, which the fixture inherits from the test binary, so the line landed under the neighbouring test: `gxfixture: session in demo: session ac29e1: want one new control file, got [3b6629f9-....ctl 618018c4-....ctl]`.

- Fact: `seedSession` snapshots the control files with `settledControlFiles` (waits for 5 polls of 100 ms with no change), creates the session, and expects exactly one new file (`sessions.go:315`).
- Fact: the code comment says Queue, Fix and Design agents start their fakes after the server accepts them.
- Hypothesis: a fake started more than 500 ms after the last file, so its file counted as the new session's. A relaunch of the same session would give the same message.
- Fix to try: the session's conversation id (`agents chat --json`, already used in `recordTranscript`) is the fake's `--session-id`, and the FIFO is `<id>.ctl`. Wait for that file instead of diffing. Validation not run.

## Fix list for the implementation epic

1. Wait on the agent's readiness, not the socket, in `TestSessionScreen_LiveReturnsHolderRows` (and audit every test that calls `waitForHolder` or `WaitForHolder` and then types). Validated above.
2. `AcquireLock`: stop truncating the pid to empty (write a fixed-width pid in one write), or retry the empty read. Validate with the six-process loop (0 empty reads).
3. `webflows`/`gxfixture` seeding: look the control file up by session id.
4. Fixture child stderr: capture each fixture's stderr in the `Running` and attach it to the failure (`no output line: EOF` hid the cause above, and the line was printed under another test).
5. `waitForParts` and the fixture start: make the sum of nested waits smaller than the hang limit so the diagnostic prints.
6. Ticket 09's headroom and burst-step changes cover the fork-limit family.
7. Do not retry any of these and do not lengthen a timeout. The three proven causes fail at 0.14 s or at the first read, not by slowness.

## Hypotheses (not proven)

- H1: the old `holder.sock never answered` and `claude exited (code 1)` failures came from process or load pressure before the launch slots and parallel cap (Nov 12 to 16). Support: they are all in logs from Nov 6 to Nov 15, many fixtures started at once, and zero occurrences today with those caps. Against: a fork error in the holder would write `fork/exec ... resource temporarily unavailable` to `holder.err`, and the logs show an empty `holder.err`. Not tested: I did not find a way to make these signatures appear.
- H2: the second `gx server` that causes `already running (pid )` is a server the fixture starts twice (for example a `serverctl start` racing the first). Not tested.
- H3: the Charting and Hold timeouts of Oct 30 to Nov 9 came from the same pre-cap load as H1. They do not reproduce at load average 57.

## Lessons for the Go skill

- Wait on the signal that means "ready for this action", not the first thing that exists. A listening socket is not an agent reading input; a pid file is not a finished write. Name the readiness point and wait for it (an acked control, a status event).
- A tty echoes input before the program reads it. A screen check that passes on the echo proves nothing. Poll the surface the test is about (here the server endpoint), after the program has acked input.
- `Truncate` then `Write` is a window readers can see. Write a state file in one write, or by temp file and `rename`, so a reader never sees it empty.
- A pid or lock file read by a loser is a diagnosis and a race at once. If it can be empty, retry the read, and print the path.
- Do not use a quiet window ("no change for 500 ms") to say that async work has finished. Identify the thing by its id.
- Nested waits must fit the outer limit. If a start timeout, a slot wait and a poll add up to more than the hang limit, the diagnostic never prints.
- A child process's stderr goes to the parent's stderr and lands under whichever test is running. Capture it per child and attach it to that test's failure.
- Classify fork errors where they occur (`process limit hit (EAGAIN)`). Then a mass failure reads as one cause, and an unexplained exit 1 stands out as something else.
- Reproduce with load and repeats, with the fork cap lifted, before reading a wait. A failure at 0.14 s under load is a race, not slowness, and a longer timeout hides it.
- Compare base and fix under the same load with the same `-count`: 5 of 300 against 0 of 300 is evidence; one passing run is not.
