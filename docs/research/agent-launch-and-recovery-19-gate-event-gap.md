# gx:agent-launch-and-recovery/19 - Why no server run has written a finish-wait gate event since 2026-10-08

Question: the draft spec says no server run has written a finish-wait gate event (`background-task-gate-held`,
`-released`, `-expired`) since 2026-10-08. Is the write path unreachable, the kind renamed, the error swallowed, or
the branch no longer run? What changed, what are the last events, and what is the smallest clean fix?

Sources: code at branch `agent-launch-and-recovery-design` (791e2d0b4), git history of this repo, every
`run-log.jsonl` under `~/.local/share/gx/tickets` (live and `.archive`, project `gx` and the sibling
`orchestrator-daemon-5-recovery`), the ticket files named below, and one real Claude pid file under
`~/.claude/sessions`. Companion reading: ticket 12 and
`docs/research/agent-launch-and-recovery-08-background-task-signal.md`. I did not launch a live agent.

## Summary

- The write path is reachable and the kinds are intact. The cause is upstream of it: since 2026-10-09 neither
  runner lets `Runner.Wait` return while a Claude background task is live, so `WaitIterationFinished` never reaches
  `WaitForBackgroundTasks`, the only writer of the `held` event. The gate was built to sit behind an idle that
  includes the task; both runners now hide the task from that idle.
- Herdr: commit `5afbb0f70` (2026-10-09 11:22 +03:00) made `status` turn herdr idle/done into `Working` whenever the
  transcript probe sees a fresh task (`agentrunner/herdrrunner/herdrrunner.go:741-760`). That is 22 hours after the
  last events (13:35-13:37 on 2026-10-08).
- PTY (the default runner since `b6b6050d1`, 2026-10-30): `Pty.Status` handles only pid-file `idle` and `waiting`
  (`nativerunner/pty.go:299-318`). Claude's pid-file status for a live background task is `shell`
  (`harness/claudepid.go:21-22`), which falls through to the initial `StateWorking`. Until `41b59e949` (2026-11-06)
  even `idle` plus a task was forced to `Working`. That commit fixed `idle`, not `shell`.
- Run-log writing works: the same logs have 8,000+ lines through 2026-11-08, and no gate event kind was renamed.
  Only the swallowed write error (`_ =` in `LogGateEvent`) is a secondary defect, not the cause of this gap.
- Real case: `gx:limit-burn-rate-impl/26b17` (2026-11-02). The agent reported finished with two shells live, the run
  log has no gate event at all, and the server paused the ticket 5.5 minutes later.
- Recommended fix: make the runners report Idle plus `Background` for a live task (PTY: add a `ClaudeShell` case;
  Herdr: set `Background` instead of forcing `Working`), so `Wait` returns and the gate decides. Test it at
  `WaitIterationFinished` against a real-shaped runner status and assert the run log.

## 1. The write path is intact

- `LogGateEvent` (`ralphloop/supervise/launch.go:111-116`) builds the event and calls `runlog.AppendEvent`, and
  discards the error with `_ =` (:114-115, the same in `LogAgentEvent` :105-107). Defect, not cause.
- The only `held` writer is `WaitForBackgroundTasks` (`ralphloop/supervise/waitforfinish.go:667`), `released` at
  :672 and :683, `expired` at :677. It starts with `if p.Agent != agent.AgentClaude || d.ReadBackgroundTasks == nil
  || sessionID == "" { return true, nil }` (:644). Claude is the only agent that reaches it; `ReadBackgroundTasks`
  is wired by `server/sched/run.go` via `deps` (ralphloop/deps/deps.go:356-362). I did not find a wiring gap.
- It is called from `WaitIterationFinished` (`ralphloop/lifecycle.go:356`) only after `d.Runner.Wait(s, waitStates,
  ...)` returned an idle/done/blocked state and `ConfirmFinished` agreed (lifecycle.go:303-355). That ordering is the
  crux: if `Wait` does not return, the gate never runs.
- Event kinds: `events/events.go:30-32` define the three kinds. `git log -S'background-task-gate-held' -- events`
  shows only the creation, `affabddbf` (2026-10-06). Not renamed or dropped. `server/sched/gatewatch.go:94-96` and
  `recovery/catalog.go:153` (R10) still match the same names.
- The finish path did get rewired: `ab11215b3` (2026-10-08 11:00) made the server's `finishRun` call
  `WaitIterationFinished`, and the in-process `Run` loop was deleted the same day (orchestrator-daemon-3-cutover
  commits, e.g. `498e0e26e`) and finally in `e003338e4` (2026-11-04). The events up to 10-06 came from the
  in-process loop; 10-08 13:35 was the first server-path run to write one.

## 2. Last gate events found

| Time | Log | Ticket | Event |
| --- | --- | --- | --- |
| 2026-10-08 13:35:42 +03 | `~/.local/share/gx/tickets/orchestrator-daemon-5-recovery/run-log.jsonl` | 06 | `held`, task `bwpvxw6ko` |
| 2026-10-08 13:37:12 +03 | same | 06 | `released`, task `bwpvxw6ko` (1.5 min hold) |
| 2026-10-06 12:47-12:52 | `.archive/orchestrator-daemon-1-foundation/run-log.jsonl` | 15 | `held`/`released`, `b0e1ooxr4` |
| 2026-10-06 11:36-11:39 | same | 05 | `held`/`released`, `bjy4fx0cs` |
| 2026-08-20 | `.archive/support-herdr-082` | | last of the older cluster (`released`) |

Totals across every run log: 16 `held`, 8 `released`, 1 `expired`, none after 2026-10-08 13:37 +03. The epic run
logs for 10-09 through 11-08 contain no `background-task-gate-*` line (`grep` over all `run-log.jsonl`). No ticket
`.jsonl` log or session file carries one either. The 10-08 event is the last one, on a run that began before `5afbb0f70`
existed. Only the `smart-zone-gate-released` kind (50 lines) keeps appearing; it is written by a different branch
(waitforfinish.go:329).

## 3. What changed around 2026-10-08

All commits are on this repo; times are local.

| Commit | When | Change |
| --- | --- | --- |
| `ab11215b3` | 10-08 11:00 | Server `finishRun` uses `WaitIterationFinished`: the gate becomes reachable on the server path |
| `490b00ea8`..`e13a7be17`, `36f85326d`, `5dbe98850` | 10-08 | R10 (catalog entry, release verbs, watchdog) and `TaskID` on gate events. `gatewatch.go` reads only the `held` event |
| `orchestrator-daemon-3-cutover/*` (`498e0e26e` et al.) | 10-08 | The TUI/in-process loop is removed; the server becomes the only runner of tickets |
| `5afbb0f70` | 10-09 11:22 | Herdr runner reports `Working` while the transcript has a fresh task (`agent-runner-phase1-impl/02a4`). The gate is now hidden from the finish wait |
| `3b98236cd` | 10-11 | Gate also holds while a notification-woken turn runs (`AwaitingTurnEnd`) |
| `a565e80da` | 10-18 | R10 enabled (release a held gate through the runner). Needs a `held` event, which no longer appears |
| `b6b6050d1` | 10-30 | `auto` picks PTY; PTY status hid an idle agent's task as `Working` |
| `67f4a3e2f` | 10-29 | Herdr ignores a finished agent's tasks (`Finished` check at herdrrunner.go:757) |
| `41b59e949` | 11-06 | PTY reports `Idle` plus `Background` for pid-file `idle` + transcript task (follow-ups-2026-11-04/08) |
| `764932d83`, `d16d7f907` | 11-05 | Move into `ralphloop/supervise` (path change only; no behaviour change found) |

I found no commit that touches the `held` branch itself between 10-08 and now other than moves and the
`AwaitingTurnEnd` addition.

## 4. Why the finish wait never runs the branch

### Herdr (5afbb0f70 on 10-09, still current)

`herdrrunner.go:741-760`: for herdr `Idle`/`Done`, if `BackgroundTasks(probed)` is true and the ticket is not
finished, the status becomes `Working`. `Wait` for idle/done then times out and `WaitIterationFinished` loops through
its guards (`ErrTimeout` branch, lifecycle.go:307-322). Only when the task resolves does herdr's idle pass through,
and then `WaitForBackgroundTasks` finds no outstanding marker and `gated` is false, so it returns true with no
event (waitforfinish.go:694-698). Where the ticket is already `finished`, the override is off (since `67f4a3e2f`),
so `Wait` returns and the gate could hold and log: that is the only herdr-path way to see a `held` event today, and
no such event has been recorded since 10-08 (my reading: herdr runs after 10-30 are rare, unconfirmed).

### PTY (default since 10-30)

- `nativerunner/pty.go:299-318`: `st` starts as `StateWorking`; only `ClaudeIdle` and `ClaudeWaiting` change it.
  `ClaudeShell` (`harness/claudepid.go:21`, "idle with a background task running") has no case, so a live task
  stays `Working`, `Background` unset.
- I confirmed Claude really writes `shell`: `~/.claude/sessions/30560.json` (the running interactive session that
  hosts this research) holds `"status":"shell"` while it has a background task. Of 21 pid files on this machine,
  1 was `shell`, 18 `idle`, 2 `busy`.
- Before `41b59e949`, even pid `idle` plus a transcript task was `Working` (the old comment: "A running background
  task will wake claude again."). `41b59e949` fixed that case only, and its tests drive the fake, which marks a
  task in the transcript and leaves the pid file at `idle` (`testutil/agentfake/tui_state.go:158-176`). The fake never
  writes `shell`. So the shape that the real Claude produces is not covered by any test I found, and the fix is
  unlikely to have revived the events. Unconfirmed against a live run.
- `Pty.Wait` (pty.go:352-364) simply polls `Status` for the requested states, so `Working` means it times out
  forever (bounded by the guards, the 2h cap in the probe is irrelevant because the gate is never entered).
- The pid-file-level view agrees: `harness.PidWatcher.Status` returns `Working, Background: true` for `shell`
  (claudepid.go:476-477), which is what the TUI shows as `+bg`. The finish wait cannot see that.

### Unreported finish versus reported finish

With the herdr override, a finished report turns the override off, so `Wait` returns. With PTY there is no such
check: a finished agent whose claude sits in `shell` keeps the ticket `claimed`, and `PauseStuckFinishedClaims`
(`server/sched/finishedclaims.go:31-58`, grace 5 min at :16) pauses it with `iteration-error`, "agent reported finished
at ... but the ticket was never processed". That is the observed failure shape.

## 5. One real case: a finished ticket held for live tasks with no event

`gx:limit-burn-rate-impl/26b17`, 2026-11-02 (runner PTY):

- Run log `~/.local/share/gx/tickets/gx/limit-burn-rate-impl/run-log.jsonl` lines 165-169: `iteration-started`
  07:32:19, `model-mismatch` 08:07:12, then `paused` 08:17:11 (`iteration-error`, "agent reported finished at
  2026-11-02T06:11:43Z but the ticket was never processed"). No `iteration-finished`, no `background-task-gate-*`.
  The whole 203-line log has none.
- Per ticket `follow-ups-2026-11-04/08` (the bug report that led to `41b59e949`, written from
  `limit-burn-rate-impl/26b18`): the agent's screen read "done 8:11 AM - 2 shells still running". The shells were
  `until rg -q '^real' cap2.txt; do sleep 15; done` loops that never exit (tasks `bjq63t271`, `botschewe`). That
  ticket's own diagnosis, "PTY ... reads as Working, `Wait` never returns ... only writer of `held`", matches
  section 4. Note: 26b18's finding ("machine load starved the wait", "Not proven") is not what that follow-up
  concluded; the follow-up has the better evidence.
- Not verified: the pid-file value for that session at the time (the screen says shells running, so `shell` is
  consistent). I did not open its transcript.

The 5.5-minute pause is the observable cost. Because the finish wait never entered the gate, R10 (which needs a
`held` event) never ran, and there was no gate event in `gx server tickets verify`.

## 6. Smallest clean fix (recommendation, not a decision)

At the cause: runners must return an idle state with `Background` set, and leave the hold to the finish wait.

1. PTY: add `case ClaudeShell:` in `Pty.Status` (nativerunner/pty.go near :300) setting `st.State = Idle` and
   `st.Background = true`. Keep the existing `ClaudeIdle` branch. `Status.Background` already exists
   (`agentrunner/agentrunner.go:77-79`).
2. Herdr: replace the `Working` override at herdrrunner.go:757 with `st.Background = true` (state unchanged). The
   `Finished` check can then go: ticket 12's report-aware rule belongs in the finish wait, not the runner.
3. Check consumers of `Working` for Background (`holderPrompter.submitKey` firstprompt.go:407, `sessionwatch.go:149`,
   TUI `+bg`) read the flag, not the state; research 08 section 1 lists them. Most already do.
4. Separately, make the gate's two log helpers return their error (`launch.go:105-116`), as the spec already says.

This is small (two switch arms) and it is the same direction ticket 12 chose: one signal, the finish wait owns the
decision, and the hold is logged. It does not replace the pending "structured `task_id`" fix in the probe.

### A test shape that would have caught it

Cover the seam where the bug lives, not either side of it:

- `WaitIterationFinished` (or `WaitForBackgroundTasks`) test over a real `Pty` driven by a fake whose pid file reports
  `shell` and whose transcript carries an outstanding `backgroundTaskId` marker, with a fake clock. Assert the run
  log (read through `runlog`) gets a `background-task-gate-held` event for that task id, then a `released` event when
  the task notification is appended. The fake needs a `shell` pid-status action alongside `writeBackgroundMarker`
  (`testutil/agentfake/tui_state.go`), because today it cannot express the real state.
- Equivalent herdr test with a fake herdr reporting `idle` and a transcript marker.
- A runner conformance scenario (`agentrunner/runnertest/runnertest.go:267`, `finishWaitsForBackgroundTask`) that
  today asserts `Working` + `ErrTimeout`. That scenario encodes the hiding as the contract; it should assert idle
  plus `Background`, then a finish-wait scenario for the run log. Odd observation, unexplained: that scenario still
  passes against `Pty` after `41b59e949` (I ran `go test ./nativerunner -run TestPty_Conformance`), although
  `Status` now returns `Idle`. I did not find out why; treat the PTY conformance wiring as unverified.

The existing finish-wait tests that mock `d.ReadBackgroundTasks` (waitforfinish topic test files, cut by `d16d7f907`)
exercise the branch with the runner stubbed, which is how it stayed green while unreachable in production.

## Not verified

- No live agent run: the `shell`-falls-through claim rests on code reading plus one real pid file, not a run that
  shows the missing event end to end.
- Whether any herdr run after 10-09 reached the gate (finished-report path). None logged an event.
- Whether `41b59e949` is reachable only for pid `idle` in practice; I assume real Claude reports `shell`, not `idle`,
  for a live task (the pid file I read says `shell`).
- The unexplained pass of `TestPty_Conformance/FinishWaitsForBackgroundTask` after the 11-06 change.
- Whether `ReadBackgroundTasks` is wired on every server path (it is in `server/sched/run.go` paths I read; I did not
  read every constructor).
