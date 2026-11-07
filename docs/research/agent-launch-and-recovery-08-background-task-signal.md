# gx:agent-launch-and-recovery/08 - Why a stall nudge parks an agent that waits on its own background task

Question: why does gx pause an agent zero-commit 9-18s after a stall nudge when the agent is alive and waiting
on its own background test run? Where does each consumer decide that "a background task is live", and what would
one shared signal look like? Also gathers the code facts for ticket 10 (`+bg`) and ticket 07's open
unreported-finish case.

Sources: the gx code at this branch (88f80d800), the tickets named above, and three real Claude transcripts
under `~/.claude/projects` (the session of ticket 04 itself, `087fea7d-...`, and one other TaskStop transcript,
`f62f73d4-...`). I did not run the probe against the transcripts and did not launch a live agent (see "Not
verified").

## Summary

- The nudge and the park are not separate detectors. They sit in one wait, `ralphloop.WaitIterationFinished`
  (ralphloop/lifecycle.go:291-372): idle, debounce, `WaitForBackgroundTasks`, then the nudge, then
  `finishIteration`, which parks zero-commit when no commit landed (ralphloop/finish.go:140). The nudge runs
  only after the transcript probe said "no live task". The park re-reads nothing. So a nudge on an agent
  with a live task means the probe was wrong, not that the nudge ignores it.
- Root cause, confirmed on ticket 04's own transcript: the probe resolves a still-running task when
  another task's TaskStop result echoes its id. Line 397 of session `087fea7d` is the TaskStop of the agent's
  `until [ -s .../tasks/bdosnqwwk.output ]` poll loop (task `bekhk2gt6`). Claude Code's result text is
  `Successfully stopped task: bekhk2gt6 (until [ -s .../tasks/bdosnqwwk.output ]; ...)`, and `bdosnqwwk` is the
  real test run. `reportsOnTasks` lets any TaskStop result through, and `containsTaskIDToken` then matches the
  id inside that text. Every incident in ticket 08 (04, 07, webui-stats 15, test-and-worktree-health 01) also
  ran that poll loop and killed it with TaskStop.
- The hole is not fixed. `reportsOnTasks` still accepts a tool_result on `StoppedTaskID != ""`
  (transcript/background_task.go:204), and the resolution loop still searches the whole raw line
  (:323-331). The ticket's added fix (resolve by the structured `task_id` only) has not landed. The existing
  test `TestReadBackgroundTasks_ResolvedViaTaskStop` (transcript/background_task_test.go:108) uses a TaskStop
  whose message names only its own id, so it does not catch this.
- Five code places decide "a task is live", and they differ (table in section 1). The finish wait and the
  stall path share the transcript probe. The `+bg` label for a Claude PTY session uses Claude's pid-file
  `shell` status, with no transcript cross-check on the Working path. Codex has no transcript probe: only a
  title heuristic (`Ready` while working) feeds `+bg`, and the finish wait skips Codex outright.
- Ticket 07's decision does not cover the unreported-finish case, and it is not implemented yet (no
  `agent reported finished` gate event exists in the code). That case today waits up to 2h
  (transcript/background_task.go:43), and the finished report is never consulted by the stall nudge's gate.
- One shared signal is a small refactor: a `BackgroundLive` reading built from the transcript probe plus the pid
  file, with one `Reading` type and per-consumer rules (section 4). The tradeoff for ticket 10 is that the
  probe is blind to a task whose start marker it does not know (Codex, a changed result shape) and can
  over- or under-resolve, while the pid file is Claude's own bookkeeping but says nothing about which task.
- The catalog has no entry for "zero-commit park while a task is live". R10 matches only a `gate-held` event
  (recovery/catalog.go:151). The zero-commit entries R2, R3 and R12 all miss, so recovery launches an
  investigate session with "No catalog entry matched this failure" (server/recovery/investigate.go:27).
- The prompt change is small: `skills/gx-implement/SKILL.md` already says "Finish with zero background shells"
  (line 83) and to use the wait tool, not an `until` loop (skills/gx-local-tracker.md:364). It has no line
  to commit the green slice before a long background run, and none for `</dev/null`.

## 1. Where each consumer decides "a task is live"

### The transcript probe (shared by three consumers)

`transcript.ReadBackgroundTasks(path, cap, now)` (transcript/background_task.go:272-396) scans a Claude JSONL
transcript:

- Start markers: a tool_result carrying `backgroundTaskId` (shell), `status: async_launched` with `agentId`
  (async Agent), or `timeoutMs` with `taskId` (Monitor; cap is `min(cap, timeout+1min)`) (:341-350). Sidechain
  lines are skipped (:316).
- Resolution: a later line for which `reportsOnTasks()` is true and that holds the task id as a token anywhere
  in the raw line (:323-331). `reportsOnTasks` accepts `queue-operation`, `attachment`, a `user` line with
  `<task-notification>`, and a `user` tool_result with `retrieval_status` or a flat `task_id` (:178-208). Interim
  subagent notices do not resolve (:213, :323).
- A marker older than its cap reads `outstanding-aged-out`. The cap is `BackgroundTaskAgedOutCap`, 2h (:43).
- `AwaitingTurnEnd`: a resolution with no assistant turn end after it (:139-143, :333-335, :376).

### The four consumers (plus the herdr status wrapper)

| Consumer | Where it decides | Input | Live means | When it is not live / cap |
| --- | --- | --- | --- | --- |
| Finish wait | `WaitForBackgroundTasks` (ralphloop/supervise/waitforfinish.go:643-715), called from `WaitIterationFinished` (ralphloop/lifecycle.go:356) and `WaitForFinish` (waitforfinish.go:126, :153). Reads via `deps.ReadBackgroundTasks` (ralphloop/deps/deps.go:356-362). | transcript probe | any `outstanding-fresh` marker, or `AwaitingTurnEnd` | resolved, aged out (2h), `GateReleased` recovery override (:683), or Claude-only: returns true at once for Codex, a nil reader, or an empty session id (:644). Probe read error: not live. Logs held/released/expired through `LogGateEvent`. |
| Stall nudge | `nudgeIfStalled` (lifecycle.go:380-419) | none of its own | it relies on the finish wait having just returned | A nudge fires only after `WaitForBackgroundTasks` returned (lifecycle.go:356-362). It checks the ticket's report (`agentReported`, :410) and a 2-min delay (config/execution_queue.go:14), nothing about tasks. It has no agent-kind check, so it also nudges Codex. |
| Zero-commit park | `finishIteration`, `ahead == 0` (ralphloop/finish.go:138-145) | the commit count | never consults tasks | It trusts that the wait ahead of it settled. Nothing re-reads between the wait and the park. |
| Runner status (herdr) | `status` (agentrunner/herdrrunner/herdrrunner.go:742-760): herdr idle/done becomes Working when `BackgroundTasks(probed)` and the ticket is not finished | `claudeui.BackgroundTasks` (agentrunner/claudeui/background_tasks.go:22-55) over the same probe; true only for `outstanding-fresh` | any fresh marker | Hides a live task from every `Wait` on idle: neither the nudge nor the finish wait fires. A `Finished` report overrides it. A read error or an ambiguous session lookup (`len(paths) != 1`) is "no task". |
| Runner status (PTY) | `Pty.Status` (nativerunner/pty.go:300-304): pid-file `idle` stays Idle with `Background = transcript probe` (same `claudeui` probe, nativerunner/pty_turn.go:117-128) | pid file + probe | the probe's fresh marker | pid-file `shell` and `busy` both map to `StateWorking` with no `Background` (the switch at :300 handles only `idle` and `waiting`), so a `shell` state keeps the wait from returning at all. |
| TUI `+bg`, interactive Claude session | `harness.PidWatcher.Status` (harness/claudepid.go:455-482): pid-file `shell` gives `Working, Background: true` (:477). OSC `done` is overridden to the pid file only when that file says `shell` (:458-461). The server maps it in `SessionStatusOf` (server/sessions/sessions.go:50); the TUI draws it at ui/agents/sidebar.go:230-231 and ui/agents/agentstyle/agentstyle.go:82 | pid file | `shell` | No transcript cross-check. The ticket's `server/sessions.go:101` and `ui/agents/terminal.go:102` references are stale: the code moved to `server/sessions/sessions.go` and `agentstyle.go`. |
| Chat background list | `chat.Loader` (server/sessions/chat/cache.go:85-101): every non-resolved marker is listed, aged out ones kept until the request's `Background(now)` filters them (:133-139) | transcript probe, Claude only (:87-89) | not resolved and within the cap at request time | Separate from `+bg`. It also feeds chat failure: `transcriptFailure` skips the stalled check when `m.Status.Background` is set (server/sessions/chat/failure.go:121-125). |

Other readers of the same flag: `holderPrompter.submitKey` uses `Working && !Background` to pick Codex's submit
key (server/sessions/prompt/firstprompt.go:407), and `sessionwatch` counts `Working+Background` as a
settled turn (server/sessions/sessionwatch.go:149-152).

### Differences, stated plainly

- Three different data sources: the transcript (finish wait, runner probes, chat list), the pid file (`+bg`, PTY
  `Wait`), and herdr's pane status (herdr `Wait` before the override). No place compares them.
- The 2h cap exists only in the transcript probe. The pid-file path has no cap, so `+bg` shows for as long as
  Claude says `shell`.
- Which markers count differs: the herdr and PTY runner probes count only `outstanding-fresh` and ignore
  `AwaitingTurnEnd`; the finish wait also counts `AwaitingTurnEnd` (a notification-woken turn still
  running). A runner that says Idle while a woken turn runs short tool calls is exactly what `AwaitingTurnEnd`
  was added to cover (transcript/background_task.go:139-143).
- Error policy is "not live" everywhere (read error, unreadable, ambiguous session lookup). That is the safe
  direction for the old bug (never stuck), and the wrong one for the park (a false "not live" parks a live agent).

### Codex

- Finish wait: skipped for Codex (waitforfinish.go:644). The runner probe reads a Claude transcript, so a Codex
  session id finds no path and returns "no task" (claudeui/background_tasks.go:38-47, "A Codex session has no
  such transcript").
- Status: the Codex watcher says `Working, Background: true` when the OSC state is `working` and the pane title
  is exactly `Ready` (harness/codexwatch.go:12-14, :62). The PTY runner keeps that as `StateWorking`
  and does not carry `Background` (`watchedStatus`, nativerunner/pty.go:~330-345, comment: "the server
  reads Background off the holder's watch stream"). So while a Codex background terminal runs, the finish
  wait never sees idle and neither the nudge nor the park fires. It waits without a task-level cap, bounded only
  by the guards (verify cap, long-ticket).
- Chat: no background markers for Codex (server/sessions/chat/cache.go:87-89).
- The nudge has no agent-kind check (lifecycle.go:355-362), but with the Codex status above it rarely
  reaches a background-running Codex. The Codex rows of the skill tool table ("none") mean a Codex agent
  has no TaskStop or Monitor, so a `run in the background` step waits or runs inline
  (skills/gx-local-tracker.md:356-359).

## 2. The path that produced the ticket 08 evidence

The incident is the herdr-era server path: `finishRun` (server/sched/run.go:268-282) calls
`WaitIterationFinished`, then `settleRun`.

1. Agent starts a 15-min `./server` test run in the background (task `bdosnqwwk`, line 336 of session
   `087fea7d`, 2026-10-21T18:38:30Z) and also an `until [ -s ...bdosnqwwk.output ]; do :; done; echo` poll loop
   (task `bekhk2gt6`, line 378).
2. At 18:39:16Z it kills the poll loop with TaskStop (line 397). The result's `toolUseResult` is
   `{message: "Successfully stopped task: bekhk2gt6 (until [ -s /private/tmp/.../tasks/bdosnqwwk.output ]; do :; done; echo)", task_id: "bekhk2gt6", command: "until ..."}`.
   Both the message and the `command` field carry the real test run's id.
3. `reportsOnTasks` is true for that line (flat `task_id`). The loop at transcript/background_task.go:323-331
   tests `containsTaskIDToken(raw, taskID)` for every marker, not just the stopped one. `bdosnqwwk` is flanked by
   `/` and `.`, so it matches and is marked resolved, and `awaitingTurnEnd` is set (:328) until the agent's
   next turn end. The probe now reports no live task while the 15-min run is alive.
4. The agent ends its turn to wait. The herdr status override sees no fresh marker (herdrrunner.go:757), so herdr's
   idle passes through. `WaitIterationFinished` returns from `Wait`, `ConfirmFinished` agrees, and
   `WaitForBackgroundTasks` finds no outstanding marker and no `gated`, so it returns true with no gate event
   (waitforfinish.go:694-698). That is why ticket 08 saw no `background-task-gate-held` in the run log. (It is
   also true that the gate-event writer drops its error: ticket 07, not re-verified here.)
5. After the 2-min stall delay the nudge types `StallNudgeText` (lifecycle.go:396-408). The agent answers "I'm not
   waiting on a person" and goes idle. Back in the loop the gate reads nothing again, `nudged` is true, and
   `WaitIterationFinished` returns nil (lifecycle.go:362-364).
6. `settleRun` runs `finishIteration`: 13 files dirty, 0 commits ahead, so `pauseAsking(ZeroCommit)`
   (ralphloop/finish.go:140). The nudge reply and the pause are 9-18s apart because the park needs only the debounce
   (3s) plus the confirm (2s) plus the finish path, with no further wait.

Is the stall or zero-commit path consulting the transcript probe at all? Yes, indirectly and only once, through
`WaitForBackgroundTasks` (lifecycle.go:356), and `finishIteration` reads no task state. The hole is the probe, not a
missing call. Within the gate, what protects the second idle (after the nudge) is the same probe, so the same
wrong "resolved" repeats.

Is the TaskStop/TaskOutput hole fixed now? No (see Summary). Specifically:

- TaskStop: `reportsOnTasks` lets any `user` tool_result with a flat `task_id` through (:204). The loop then
  matches every marker's id against the raw text (:324-330). Fix as the ticket says: resolve a TaskStop line only
  by `toolUseResult.task_id`, a TaskOutput line only by `toolUseResult.task.task_id`.
- TaskOutput: same shape of hole. A TaskOutput result also resolves by raw text, and it resolves regardless of
  `retrieval_status` or `task.status`. A timed-out poll (`retrieval_status` other than `success`, `task.status`
  `running`) would resolve a live task. I read this from the code only; I found no such result in the local
  transcripts (see "Not verified").
- The `attachment` and `queue-operation` types are accepted wholesale (:180-181) and also match by raw text. A
  queued user message that quotes a task's `.output` path would resolve it (the same family as archived
  follow-ups/22 and `gotchas.md` lines 315 and 322). I did not find a live case.

Also relevant: `skills/gx-implement/SKILL.md:83` tells the agent to "finish with zero background shells", and the
nudge's own text ("If you are waiting for a person's reply...") carries no information about tasks. The agent's
answer "I'm not waiting on a person" is correct: it is waiting on a task.

## 3. Ticket 07's decision and the unreported-finish case

Ticket 07 resolved: with a finished report and an ended turn, the finish wait skips tasks started before the report
and logs each as a released gate event, and the normal stop kills leftovers (SIGTERM, then SIGKILL). The code does
not have that yet (`rg "agent reported finished"` finds nothing in the gate). It left the unreported case (turn ended,
no finished report, task live) at the 2h cap and handed it to this ticket.

What the decision implies for that case:

- The finished report is the only evidence the agent concluded. Without it, a task started before the turn end is
  not abandoned work; it is the agent waiting for its notification, which is the ticket 08 pattern. Skipping such
  a task is the bug being fixed. So the unreported case must hold, and the 2h cap is the only release.
- The two cases share one distinction: was the task started before or after the last agent signal. With a report,
  "started before" means leftover. Without one, "started before" means in flight.
- A correct policy: hold while the transcript says a task started in this iteration is unresolved and its turn
  ended without a report. Release on (a) the notification arriving (the agent wakes and finishes or commits), (b) a
  finished/asking report, or (c) the cap. Do not nudge while held: the nudge asks a question the agent can only answer
  "no" to, and its reply is itself a turn that can arm the park. Ticket 08's own evidence shows it does nothing but
  wake the agent early.
- Cap: the data points at 2h being a hang guard, not an expected wait. Observed real waits are 295s, 371s and 15
  min (ticket 08 evidence). The `./server` run is about 4 min (skills/gx-implement/SKILL.md:232), the full check
  about 10 min. A cap in the 30-60 minute range keeps every observed wait and bounds a hung `awk` (ticket 07)
  without a 2h strand; the epic's verify ticket runs `gx check --full` and a soak in the background
  (`.ai/index.md`, "Testing"), so the cap must not undercut those. I did not measure a distribution; the figures above are
  the only data. A per-task cap from the launching command is not available in the transcript (`toolUseInput`
  has only the description and command text, transcript/background_task.go:71-81), except Monitor, which already
  carries its own `timeoutMs` (:345-349).
- What the 2h cap costs: a task that hangs forever keeps the ticket `claimed` for 2h, holds an active slot, keeps
  the worktree and tab alive, and shows nothing in the run log if the gate event is dropped (ticket 07). With a live agent
  it is also two hours of nothing in the queue; with a dead agent behind a stuck task (ticket 07's `awk`) it is
  pure loss. Releasing early costs the opposite: a real run is parked and a recovery session started
  (63.7k tokens, 444s in ticket 15a), and `07a` ran tests in a live worktree while late-finish removed it.
- On release without a report, the park is still the wrong action if the process is alive. The release should
  re-read the task state once before parking, and park with a reason that names the task (the existing
  `background-task-gate` pause kind exists for this, section 5), not `zero-commit`.

## 4. One shared signal

### Shape

The code facts to build on:

- Probe: `transcript.ReadBackgroundTasks` already returns what all consumers need (markers with Kind, Label,
  StartedAt, Cap, Status, plus `AwaitingTurnEnd`).
- `claudeui.BackgroundTasks` reduces it to a bool with its own path cache (claudeui/background_tasks.go:22-55);
  `deps.ReadBackgroundTasks` re-implements the path lookup differently (deps.go:356-362, `transcriptPath`
  by cwd); `chat.Loader` reads a third time (cache.go:93). Three finders, three read policies.
- Pid file: `harness.PidWatcher` holds claude's last status per session (`idle/busy/shell/waiting`,
  harness/claudepid.go:18-24, :466-480). `shell` is "idle with a background task running" and is claude's
  own count, so it covers shell and Monitor-style work but names no task.
- herdr: pane status only (idle/working/done/blocked). It knows nothing about tasks.

Proposed shape (sketch, not a decision): one function in a new `backgroundtask` package (or `transcript` plus a thin
adapter), e.g.

```go
type Live struct {
    Live       bool      // any task the agent still waits on
    Tasks      []Task    // id, kind, label, startedAt, status (fresh / aged-out)
    AwaitingTurnEnd bool // a notification woke the agent and its turn has not ended
    Source     Source    // transcript | pidfile | both | none
    Disagree   bool      // transcript and pid file differ (log it)
}
func Read(sess Session) Live
```

Inputs: transcript path (via one shared finder), the holder's pid-file status when the runner has it, and herdr status
only as the "agent is not working" precondition. Consumers get a `Live`, never a bool.

### Consumer behaviours

- Stall nudge: skip while `Live`. Reword is the alternative (ask "are you waiting on task X? say so with
  `gx tickets ask` only if on a person"), but the evidence shows the nudge adds nothing when a task is live, so skip is
  simpler. This is already the effective behaviour if the probe is right, so the fix is the signal's accuracy, plus
  a defensive recheck immediately before typing the nudge (the gate result can be 2+ minutes old, since the nudge sits
  after the 2-minute delay at lifecycle.go:383-385; nothing re-reads in that window, and a task started
  after the gate returned would be missed).
- Zero-commit park: before `pauseAsking(ZeroCommit)` (finish.go:140), re-read `Live`. If live: wait with the finish wait
  rather than pause. If a hold times out: pause with `background-task-gate` and a reason naming the task, never
  `zero-commit` (the destination says gx never records the fault as the agent's choice).
- Finish wait: `Live.Tasks` replaces the marker loop (waitforfinish.go:653-690), keeps its gate events, and gets
  ticket 07's report-aware rule. It should also log each ignored task, and log when the gate is clear but the pid file
  disagrees.
- `+bg`: render from `Live.Live`. The Idle path in the PTY runner already uses the transcript; Working+`shell` uses the
  pid file. A single `Live` makes both agree, and answers ticket 10's check of which one wins.
- Codex: no transcript, so `Source` is the title heuristic only (`Ready` + working) and the finish wait and nudge
  need a Codex clause that treats "working with no tool progress" as live (the PTY status already does, by staying
  Working). That is an implementer's choice, not something to decide here.

### The ticket 10 tradeoff (transcript alone vs pid file when they disagree)

Facts only:

- Transcript alone: it carries ids, kinds, start times and caps, so it can show "waiting on task X" and apply the
  2h/Monitor caps. It is blind to a start shape it does not know (any new tool result), and can wrongly resolve
  (this ticket) or wrongly keep a task fresh (ticket 07's hung `awk`). Ticket 10's own evidence is the probe saying no task
  while `+bg` showed: if the row was the right session, the pid file (or the probe) was the wrong one, and
  nothing in the evidence says which.
- Pid file alone: it is Claude's own state, with no cap and no task identity, so `+bg` can persist for a hung shell
  with no way to say which. It cannot gate per task (ticket 07's skip-before-report rule needs start times).
- They disagree in both directions. Transcript says live, pid file `idle`: the PTY runner already trusts the
  transcript for that (nativerunner/pty.go:300-304). Pid file `shell`, transcript says none: the runner stays Working
  with no `Background` flag, and the TUI shows `+bg`. That second row is ticket 10's report, and there is no
  cross-check there today.
- Whichever wins, the disagreement should be logged once per session, because the 4 incidents above would have
  shown as a transcript/pid mismatch had anything compared them.

## 5. Recovery catalog

- No entry exists for zero-commit while a task is live. The catalog has R10 for `background-task-gate-held`
  (recovery/catalog.go:151-165, enabled: it force-releases the gate and finishes), and the `Triggers` filter
  (recovery/trigger.go:38-39) accepts a held-gate event or an answer-class zero-commit pause. The zero-commit
  entries are R2 (unexecuted tool call, :374), R3 claimed-present and merged-elsewhere (:333, :355) and R12 (stalled
  reclaim, :214). None matches a plain park with a live agent. An unmatched zero-commit goes to investigate
  (`No catalog entry matched this failure.`, server/recovery/investigate.go:27), which is the 444s, 63.7k-token
  session in ticket 08.
- The gotchas file has the related entries (skills/gx-investigate/gotchas.md:25-33 `background-task-gate-released`
  treated as completion; :315, :322-323, :330 the earlier resolve-too-eagerly families). None names "TaskStop result
  echoes another task's id" or the poll-loop pattern.
- The existing `background-task-gate` pause kind (tickets/schema/pause.go:29, label "Background task still running",
  answer class, Retry only, tickets/schema/pause_verbs.go:137) is written by `PauseReleasedGate`
  (server/recovery/verbs.go:168-173) with the reason from `gatePauseReason` (verbs.go:154-161: "released on an idle
  agent, but the worktree has uncommitted changes"). Its retry prompt is "A person reviewed the background task
  you were waiting on and wants you to continue" (server/ticketops/repair.go:59). That is the honest pause for a
  live-task park, and it fits the epic's rule.
- What would need adding so recovery does not investigate:
  1. Make the park not happen (primary): the signal in section 4, so the zero-commit pause is never written.
  2. Add a catalog entry for the residue: a rule that matches `Paused/zero-commit` where the run log shows a
     `stall-nudged` and no gate event (the shape of all four incidents), whose remedy re-reads the agent's task
     state through the shared signal and, if live, does nothing (leave the pause open and let late-finish adopt),
     else falls through. Predicate needs a task field on the events, which the run log does not carry yet
     (`runlog.Event` has `TaskID`, used by gate events at server/sched/gatewatch.go:94-100).
  3. Record the nudge's reply and the park's task state in the pause event, so a future investigator, or a
     rule, can see "task X was live at the park" instead of losing the evidence when the worktree is cleaned
     up (15a found the worktree already gone).
  4. Add the gotchas entry for this mechanism, with the poll-loop and TaskStop pattern.
  5. A late-finish adoption already exists (`late-finish-adopted` flag rows); the recovery rule must not cancel or
     start over a ticket whose agent is still live (ticket 15a ran tests inside a worktree late-finish was
     removing).

## 6. The implement prompt

- The text is the skill `skills/gx-implement/SKILL.md`: gx's launch prompt is only `/gx-implement <address>`
  (`agent.DefaultWorkerSkill`, ralphloop/agent/agent.go:28; `stallNudgeText` rebuilds it, lifecycle.go:421-436). The
  tool-name table and the "use the wait tool, not an `until` loop" rule are in `skills/gx-local-tracker.md:344-364`.
- Already there: "Finish with zero background shells" (SKILL.md:83), "run `gx check` in the foreground, unpiped...
  600000 ms" (:229-233), and "Never run a command that can read stdin" (:77-78). The poll loop that TaskStop kills
  is written by agents anyway, three epics in a row (ticket 08), so the rule is not reaching them, or reaches them
  only after the loop is written.
- A change (a proposal, a person decides the wording): after "Finish with zero background shells", add a line that a
  slice already green is committed before a background run longer than a few minutes starts, so a park or a kill
  does not leave 13-17 files uncommitted; and that waiting for a background task is the wait tool's job, never a shell
  loop (restating `gx-local-tracker.md:364` where agents read it). Ticket 07's `</dev/null` line goes in the same
  bullet list.
- Cost of the commit-first line: `gx check` runs before every commit (SKILL.md:229), so committing a green slice
  before a long run means one extra `gx check` first, or an agreed exception for a WIP commit. That is a rule
  change, not a wording one.
- A skills test guards the text: `skills/check_rules_test.go` and `skills/gx_design_rules_test.go` hold phrase checks.
  Any new line needs its shared test (`gx-test-guidelines.md`, "never add a second file asserting the same phrase").

## Design questions that remain

- Unreported-finish release: a release cap value (shorter than 2h; the observed waits are 5-15 min) and what a
  release does when the process is alive: park as `background-task-gate`, or keep waiting once?
- Should the nudge skip a live task, or reword to name it?
- Ticket 10: transcript or pid file wins when they disagree, and should the TUI show the disagreement?
- Codex: one shared signal for Codex needs a definition of "live" (title `Ready` only?) and an expectation for how
  long it may hold; nothing in gx decides that today.
- Commit-first line: allow a WIP commit before the single `gx check`, or run `gx check` before the background run?

## Not verified

- I did not run `transcript.ReadBackgroundTasks` on session `087fea7d`. The mechanism is shown from its code and
  from the exact TaskStop line in the transcript, so a one-off probe run on that file would confirm it.
- Incident tickets other than 04: I read only 04's transcript. 07, webui-stats 15 and test-and-worktree-health 01
  rest on the ticket's evidence ("also ran an `until` loop and killed it"), not on my own reading of their transcripts or run
  logs. For 15 the ticket's own investigation names the same mechanism.
- TaskOutput and `attachment`/`queue-operation` raw-text holes: read from code; no real transcript example found in the
  local data.
- Whether Claude's pid file reports `shell` for a plain `run_in_background` Bash task, and what it says during a
  task that auto-backgrounded at the 120s limit. I read gx's mapping, not Claude's source.
- The Codex Working path is read from `watchedStatus` (nativerunner/pty.go) and `codexwatch.go`; I did not run Codex
  with a background terminal.
- That gate events are dropped on the server path (ticket 07 question) was not re-verified here.
- The cap figures (30-60 min) are my reading of the three observed waits, not a measured distribution.
