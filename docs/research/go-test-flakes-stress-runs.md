# go-test-flakes: stress runs under `gx check --full`

Date: 2026-11-22. Worktree: go-test-flakes-design (HEAD 2a8b45b98). Question for every ticket of gx:go-test-flakes except 06 and 18: does the test still flake while a `gx check --full` runs next to it?

## Method

- A loop ran `gx check --full` in this worktree. Check run 1: `~/.local/share/gx/check/20261122T150053.362515000` (17:00:53 to 17:10:14). Runs 2 and 3 (`...T151015.073794000`, `...T151929.364658000`) followed; run 3 was stopped early.
- Every stress run below started between 17:01 and 17:09, so all of them overlapped check run 1. Run 1 passed every Go test; it failed on `format` (a Biome diff in a webui test) and on 6 Playwright flows. Neither is a Go test.
- Stress run: `gx brief go-test -timeout 90m -count=20 -run '^(<tests>)$' <pkgs>`, at most 3 at once (xargs -P3). The `-timeout 90m` is mine: designflows at 20 x 70 s would pass the default 10m.
- The installed `gx` (v0.51.11) rejects this checkout's `.gx/checks.json` (`"always" must be a list of command strings`). I built `gx` from this worktree into the scratchpad and put it first on PATH for the check and the stress runs.
- `gx brief go-test` takes no `-exec` flag of its own and applies its own process cap (see the header of `scripts/proc-cap`), so I did not wrap it.
- Process count: baseline 684 for the uid. During the check it sat at 730 to 1000 (check plus 3 stress runs plus other sessions). That is past baseline+150, the kill line in my brief, from the check itself. The proc-cap hard cap is about 1480, so I kept going and raised my own alarm to 1250. It never reached it. The caller should know about this.

## Results

passes/20 is the iteration count of the one stress run per ticket. "Log" is the `gx brief` log under `~/.local/share/gx/brief/`.

| Ticket | Test | Package | Result | Log |
|---|---|---|---|---|
| 01 | TestStallNudge_HungPaneTwicePausesPromptStalled | server/sched/launchtest | 20/20 | 20261122T150546.050863000-go-test.log |
| 02 | TestLateFinish_ResumeAndRelaunchAdoptFirst (4 subtests) + TestKeepWorktreeScenario_TheAskedFileSurvivesTheAnswerAndASecondTicketRuns | server/merge, testutil/gxfixture/designflows | 20/20 each (160 test lines) | 20261122T150151.814720000-go-test.log |
| 03 | TestRestartAgents_CountsTheRunningAgent | testutil/gxfixture/limitsflows | 20/20 | 20261122T150413.012922000-go-test.log |
| 04 | TestBudget_HardLimitStopWaitsForSlowInterrupts | server/budget | 20/20 | 20261122T150731.949250000-go-test.log |
| 05 | TestStallNudge_HungPaneOnceIsRelaunchedOnce | server/sched/launchtest | 20/20 | 20261122T150622.695529000-go-test.log |
| 07 | TestVimLaunch_PersistentFooterWithAHangEndsPromptStalledAndRetryWorks | server/sched/launchtest | 20/20 | 20261122T150623.812809000-go-test.log |
| 08 | TestClaim_ResumesOrColdStarts (all subtests, incl. answered_pty_resumes) | server/sched/launchtest | 20/20 | 20261122T150646.806076000-go-test.log |
| 09 | TestPty_HoldKeepsTheRunWorkingUntilReleased | server/servertest | 20/20 | 20261122T150811.475787000-go-test.log |
| 10 | TestPty_PersonInputAfterGxTypedEndsNudgesWithoutError | nativerunner | 20/20 | 20261122T150838.363035000-go-test.log |
| 11 | TestConflict_ThreeMergeTicketsThenNeedsRebase | server/prflow | 20/20 | 20261122T150844.245759000-go-test.log |
| 12 | TestRecovery_UnmatchedPauseForksAQueuedInvestigateChildAndNeverRecurses | server/recovery | 20/20 | 20261122T150845.995617000-go-test.log |
| 13 | TestDiskCleanNow_HoldEndsInTheResponse + TestBudget_HardLimitInterruptsRunInParallelInsideGrace | server/disk, server/budget | 20/20 each | 20261122T150747.863738000-go-test.log |
| 14 | TestWorkspacesScenario_SeedsFixAgentsAndAnInvestigateAgentInOtherAndUnderEpics | testutil/gxfixture/designflows | 20/20 | 20261122T150133.065692000-go-test.log |
| 15 | TestBlockedScenario_EachKindOfQuestionListsItsCard | testutil/gxfixture/designflows | **0/20** (20 failures, one cause) | 20261122T150133.067606000-go-test.log |
| 16 | TestStoreWatchSeesNewFilesAndSkipsGit, TestColdStoreWave_SixTicketsOfOneEpicInstallOneAtATime, TestLaunchFailScenario_TheTicketPausesWithTheErrorAndNothingReadsLaunching (the 4th, TestWorkspacesScenario_Seeds..., is ticket 14's test and was not run again) | server/index, server/sched/launchtest, testutil/gxfixture/designflows | 20/20 each (60 test lines) | 20261122T150258.269129000-go-test.log |
| 17 | TestDraftHistoryScenario_StartsWithPagesAndIdleSessions | testutil/gxfixture | 20/20 | 20261122T150437.238624000-go-test.log |
| 19 | TestToolsScenario_ChatListsTheCallsInFlight | testutil/gxfixture/designflows | 20/20 | 20261122T150133.066042000-go-test.log |
| 20 | TestHealthProcs_WindowRunsFromTheReadNotTheLastCall | server/health | 20/20 | 20261122T150854.195600000-go-test.log |

Ticket 15 reruns while checks 1 and 2 ran, with 3 designflows runs started in the same second:

| Run | Result | Log |
|---|---|---|
| `-count=1` alone-ish (check running) | pass | 20261122T150914.241518000-go-test.log |
| `-count=3` x 2 at once | pass, pass | 20261122T151008.003056000 and ...003040000-go-test.log |
| 3 designflows tests at once, `-count=2`, 3 rounds | round 1 FAIL 2/2, rounds 2 and 3 pass | round 1: 20261122T151046.421607000-go-test.log |

## TestBlockedScenario_EachKindOfQuestionListsItsCard (ticket 15): FAILS under load

Failure text (all 20 iterations of the stress run, identical):

```
questionscenarios_test.go:43: question session 4 lists [], want a permission card
```

The fixture stderr is empty, so there is no server log. The `gx brief` log has only this line, 20 times, one every 20 s (the test's `Poll` timeout).

### Why it fails 20 of 20, not 1 of 20

`agents-blocked` is one shared fixture. `TestMain` starts it once per `go test` run (`sharedBlocked` in `commandsscenarios_test.go:107`). `-count=20` reuses it. A wrong state left by seeding is permanent for the whole run. The same thing explains the ticket's "failed 2 of 4 runs": the unit of randomness is a fixture start, not a test iteration.

### Reproduction without the test

I built the fixture binaries and started `gxfixture agents-blocked` 24 times in 8 rounds of 3 at once, with the check running. Then I read `gx agents list --json` 3 s after it printed its output line (`scratchpad/s/trial.sh`).

- 6 of 24 starts showed a session that the spec seeds as Blocked as `idle` (3 in each batch of 12). In the one I inspected in full (the saved list of start t71), it is the Plan session, `{"state":"idle"}` with no `failure`, where the spec asks for `Failure: blocked` and the seeder waited for it. In the other 5 the Idle slot was a permission or question session; I did not inspect those further.
- In t71 the Idle session has `unread: {unread: true}`. The seeder marks each session read once. A raised Unread after that means the state changed after seeding finished.
- The server was Blocked at seeding time (the seeder's wait passed) and Idle 3 s later.

### Root cause (read from code; the mechanism matches every observation but I did not trace one run's events)

The seeder races the fake agent's own delayed turn end.

1. `settleSession` (`testutil/gxfixture/sessions.go:150`) sends prompt "hello". The Claude TUI fake runs `chatTurn` (`testutil/agentfake/tuiclaude/design.go:80`): set pid file Busy, append the agent reply to the transcript, then start a goroutine that sleeps `DesignTurnTime` (500 ms) and sends `ActionFinish` on the control FIFO.
2. The seeder waits for an agent message in the chat, then for `si.Status.State == Idle` (sessions.go:165). The chat reader sees the reply the moment it is on disk. The server's status can still be the pre-turn Idle, because the pid-file watcher has not yet seen Busy (or Busy and the reply landed in the same read). Under load the pid watcher lags. The Idle wait passes on the stale Idle without the turn being over.
3. The seeder sends the Block control (`ActionBlock` with the plan dialog, or the permission dialog). The fake sets the pid file to Waiting. The server shows Blocked and the seeder's `failure blocked` wait passes.
4. At 500 ms the fake's goroutine sends `ActionFinish`. The fake sets the pid file to Idle (`handleControl`, `tui_state.go:63`). The session is Idle with no dialog on its pid file. The server raises Unread. Nothing in the fixture ever blocks it again.
5. `Cards` (`server/sessions/chat/permissioncard.go:83`) returns a permission card only for `State == Blocked && Kind == "permission"`. The Idle session returns none, so `cardSources` is `[]` for every poll.

At low load the seeder's Idle wait runs after the fake's Finish, so the order is the intended one.

### Root-cause fix (not implemented)

Wait on the turn edge, not on Idle alone. Two ways; the first is the smaller:

- In `settleSession`, before sending any control, wait until the turn has ended: observe Working and then Idle (or compare `status_changed_at` with the agent message time), instead of a bare `State == Idle`. Do this for every session that gets a control after a prompt.
- Or make the fake's turn end not race controls: have `chatTurn` end the turn through the same serialized path as the seeder's controls (no delayed self-sent `ActionFinish`), so a Block control cannot be overwritten by an older Finish.

A longer Poll, a sleep between prompt and control, or a larger `DesignTurnTime` are not fixes.

The same race should hit every `agents-blocked`-style seed that sends a control right after a chat prompt (`TestBlockedScenario_BlockedSessionsSettleBlocked` in the same fixture, and other `settleSession` users). The ticket text of 15 lists that test as having flaked too.

### Other tickets

No failure in any of the other 17 stress runs, with the check running next to them. Tickets 14, 16 and 19 start designflows fixtures too (workspaces, launch-fail, tools scenarios) and passed 20/20 each, but they do not seed a Blocked session after a chat turn, so the race does not reach them.

## Files

- Check summary and logs: `~/.local/share/gx/check/20261122T150053.362515000/` (run 1).
- Stress run output is in the `gx brief` logs listed above; each log path is the last line `gx brief` prints.

## Ticket 18: why the `test` kind took 1128 s (run 20261121T172314.661376000)

The slowdown is machine load. No one package grew.

- The `test` kind runs go, server, web, e2e and burst in parallel. The server part took 1089 s against 616 s in the 2026-11-18 baseline run (20261118T152906). go took 577 s against 595 s.
- I compared per-package times from the `gx brief` go-test logs of both runs. 69 packages ran in both and took over 5 s. The median ratio is 1.41, and the sum went from 1974 s to 3025 s. The growth is spread over most packages (promptflows +137 s, merge +75 s, designs +61 s, epics +59 s, gxfixture +59 s, ...). No single package explains it.
- The promptflows growth is partly the nine image tests that failed for real (fixed by follow-ups-20261121/84). They are out of scope here.
- `server/drafthistory` (48 s) is new since the baseline, so it adds about 5% at most.
- The same run failed six load flakes that all passed on rerun. 20/20 stress runs on 2026-11-22 found none of them except ticket 15.
- `load_start` 6.14 and `load_end` 2.68 are two point samples on 12 cores, so they do not show the load in the middle of the run.
- Earlier slow runs show the same spikes: 1132 s (2026-11-18 17:52), 933 s (2026-11-17), 885 to 919 s (2026-11-14). Quiet runs on 2026-11-22 took 558 s and 552 s.

Verdict: no package to fix. The load preflight (gx:follow-ups/624) covers the slow runs.
