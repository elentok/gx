# gx:agent-launch-and-recovery-2/03 - What caused prompt-nudged=18 on gx:pr-flow-impl/49

Question: were the 18 `prompt-nudged` events on `gx:pr-flow-impl/49` (sessions `61b6b20a-...`, `01142f34-...`) (a)
launch-prompt resends, or (b) stall nudges fired at an idle-looking agent that waited on a background subagent?

## Verdict

- (a). All 18 are launch-prompt resends. None are stall nudges. The retro's reading (b) is wrong.
- The agent did run a background subagent, but the nudges came before that mattered: all 18 landed within 56s of
  the relaunch (15:29:16-15:30:11), then 2 `launch-failed` killed those attempts. The run log has no `stall-nudged`
  event for 49 at all.
- Trigger differs from ticket 01. The transcript record was not `<pasted_content>`. It was a normal
  `<command-name>/gx-merge</command-name>` record. The likely cause is a relaunch into a resumed session (see
  "Why the proof failed"). Ticket 01's fix (hold resends while the agent works, bare skill command first) would
  still stop these resends, but the pasted-content match fix alone would not.
- The current code holds stall nudges while a live background task exists. The prompt does not tell agents to wait
  for the task notification instead of polling.

## 1. What `prompt-nudged` counts

- One event type, one writer. `events.PromptNudged = "prompt-nudged"` (events/events.go:63-65), "one step a runner
  took on a launch prompt the agent didn't pick up".
- It is emitted only from `claudeui.CheckLaunchDelivered`: `p.Report(NudgePrompt)` right after each `Resend()`
  (agentrunner/claudeui/launchproof.go:172-178), with `NudgePrompt = "prompt-nudged"` (launchproof.go:14).
- Resend cadence: every `ResendEvery` = 3s, for a `Grace` = 30s (launchproof.go:46-51). So 9 resends per attempt.
- Written to the run log by `logPromptNudged` (ralphloop/launch.go:144-152), fed by the `OnNudge` hook
  (ralphloop/conflict.go:370-372 for conflict agents, the same hook in the launch path).
- Every nudge kind the server can send, and its event:

| Kind | Code | Event type |
| --- | --- | --- |
| Launch-prompt resend | launchproof.go:172-178 | `prompt-nudged` |
| Stall nudge (idle, no question, no report) | ralphloop/lifecycle.go:496-511 `nudgeIfStalled` | `stall-nudged` (`Text` = the nudge) |
| Person/manual nudge through the server | events/events.go:55-56 | `nudged` |
| Classifier-outage "continue" re-send | ralphloop/lifecycle.go:~425 `classifierResendDelays` | none of the above (own pause on exhaustion) |

- The stall nudge is never logged as `prompt-nudged`. Its text is fixed: `StallNudgeText` ("If you are waiting for a
  person's reply, run `gx tickets ask` now. Otherwise continue.", ralphloop/supervise/startprompt/startandprompt.go:20),
  or the launch prompt again when the first person record is not the launch (`stallNudgeText`,
  lifecycle.go:517-535). Even that second case is logged as `stall-nudged`, not `prompt-nudged`.
- `server/recovery/summaries.go:76-78` also keeps the two apart (`case events.PromptNudged` vs
  `case events.Nudged, events.StallNudged`).
- The ticket's own decision already says it: "What does prompt-nudged count? -> launch-prompt nudges"
  (`~/.local/share/gx/tickets/gx/agent-launch-and-recovery-2/issues/03-stall-nudges-background-subagent.md`).

## 2. Evidence for ticket 49

Where the evidence lives (the epic is merged and archived, so `gx tickets show` was not needed):

- Run log: `~/.local/share/gx/tickets/gx/.archive/pr-flow-impl/run-log.jsonl` (299 lines). Read directly.
- Transcripts: `~/.claude/projects/-Users-david-dev-gx-pr-flow-impl/61b6b20a-222d-4778-ae46-5b330f604ef1.jsonl` and
  `.../01142f34-e088-4e8a-8116-9ee21a464ddb.jsonl`. Both exist. Subagent dirs exist beside them.
- The retro said "the digests hold no nudge log". The run log does have it.

Run-log lines for ticket 49 (times local, +02:00):

```
15:03:09 iteration-started  agent_session=61b6b20a  path=cold-start
15:10:38 paused             kind=self-reported "agent asked via iteration_status"
15:20:40 waiting-for-answer "every remaining ticket is paused: gx:pr-flow-impl/49"
15:29:12 deps-installed     (relaunch after the answer)
15:29:16 prompt-nudged      attempt=1      <- 9 events, 3s apart, to 15:29:40
15:29:43 launch-failed      "sending initial prompt: pane hung: screen unchanged across clear rounds and the input still holds gx's text"
15:29:47 prompt-nudged      attempt=2      <- 9 events, 3s apart, to 15:30:11
15:30:14 launch-failed      same reason
15:30:17 iteration-started  agent_session=01142f34  prompt_ns=63971147875 (64s)
15:43:31 commitless
```

- `rg -c '"type":"prompt-nudged"' run-log.jsonl` gives 18, and every one is ticket 49, attempts 1 and 2.
- `rg '"type":"(stall-nudged|nudged)"' run-log.jsonl` gives no match. Zero stall nudges in the whole epic's log.
- 2 `launch-failed` + 18 `prompt-nudged` matches the flag row `launch-failed=2 prompt-nudged=18`. The 790s and
  `slow-launch` flags follow from the 64s launch and the failed attempts. `paused-answer=1` is the 15:10 pause.
- The launch-failed screen shows what the pane held: `✢ Symbioting… (3s · ↓ 110 tokens)` (the agent working) and
  `❯ Press up to edit queued messages` (the resends sitting in Claude's queue).

Transcript `61b6b20a...jsonl`, the same minutes (UTC = local - 2h):

- 13:29:13.064 user record: `<command-name>/gx-merge</command-name>` with `<command-args>--ticket
  gx:pr-flow-impl/49\n\nEarlier iterations of this ticket left these summaries ...`. The launch was delivered, the
  skill ran, and the agent started working at once (a `git branch ...` Bash call at 13:29:15).
- 13:29:22.914 the agent started an async `Agent` subagent ("Fix failing pr-text sched test", agent id
  `a031551e250fbe72e`). 13:29:24.655 it ended the turn: "The Sonnet subagent is fixing the failing `pr-text` test in
  the background. I'll rerun the checks once it commits and report back then."
- 25 `queue-operation` records. 18 are `enqueue` of `/gx-merge --ticket gx:pr-flow-impl/49\n\nEarlier iterations ...`
  at 13:29:16, :19, :22, :25, :28, :31, ... :40 and 13:29:47 ... 13:30:11, each ~3s apart, matching the 18 run-log
  events to the second (run-log 15:29:16.064 vs enqueue 13:29:16.077, and so on). The other enqueue is the real
  `<task-notification>` for task `a031551e250fbe72e` at 13:29:43.928.
- The agent was working the whole time (tool calls, a subagent, a task notification). It never sat idle for the
  stall delay, so a stall nudge could not have fired, and none did.
- The stall nudge text ("If you are waiting for a person's reply, run `gx tickets ask` now.") appears in neither
  transcript (`rg -c` over the project dir: no match).
- Session `01142f34`: launch at 13:30:17 with the same note, delivered first time (no `prompt-nudged` after
  15:30:11). It ran to 15:43 with no stall nudge.

## 3. Why the proof failed (inference from code plus transcript, not a log line)

- `launchedBy(content, text)` is true if `content` contains the raw `<command-name>/gx-merge</command-name>` tag or
  starts with the launch text (launchproof.go:123-127).
- `firstPerson` reads the first person message through `transcript.ReadChat`, which renders a command record as
  `name + " " + args` with no tags (transcript/chat.go:379-385). So the `<command-name>` test never fires on that
  input. Only the `HasPrefix` test can.
- Session `61b6b20a` is the session of the first run (13:03). Its first person record is the original
  `/gx-merge --ticket gx:pr-flow-impl/49`. The relaunch at 13:29:13 landed in that same file as a second record. The
  relaunch text is longer (it carries the `## Iteration summaries` note, see the pane lines), so
  `HasPrefix(firstRecord, text)` is false. The proof sees "not delivered" forever and resends every 3s into a working
  agent. The queue shows the same: resends pile up behind the agent's turn.
- That is the same end state as ticket 01 (resends queued into a busy agent, then `HungPaneError`
  because the input still holds gx's text, launchproof.go:161-166) with a different trigger: resume vs paste. I did
  not run a replay to confirm the resume path; the facts above (same file, first record shorter) are what support it.
- Why the third attempt worked: it started a fresh session (`01142f34`), so the first record is the relaunch
  itself.

## 4. Current stall-nudge behavior

Does it hold nudges while a background task runs? Yes, for Claude with a session id.

- The wait runs `WaitForBackgroundTasks` before the nudge and `continue`s while it returns not-confirmed
  (ralphloop/lifecycle.go:381-392). Then `nudgeIfStalled` (lifecycle.go:395-401). In order: idle, debounce, hold on
  background tasks, then the nudge.
- `WaitForBackgroundTasks` (ralphloop/supervise/waitforfinish.go:657) returns true at once only for non-Claude, no
  reader or no session id (waitforfinish.go:658-660). A `taskLive` task holds the wait and logs
  `BackgroundTaskGateHeld` (waitforfinish.go:691-698). A held task is checked every 10 minutes
  (`BackgroundTaskCheckEvery`, ralphloop/supervise/backgroundhold.go:203), under a hold cap
  (backgroundhold.go:195-198 pauses at `holdCap()`). CHANGELOG.md:179 records the shipped fix and the 60-minute cap.
- The async-subagent marker (an `agentId` from an `async_launched` result) is part of the gate; see
  `~/.claude/skills/gx-investigate/gotchas.md:323`.
- Gap: this holds only when the transcript probe reads the subagent as live. The earlier research
  (docs/research/agent-launch-and-recovery-08-background-task-signal.md) shows probe misreads that let a nudge
  through. That is a different issue from this ticket's evidence.

Does the system prompt or launch prompt tell agents to wait instead of poll? No.

- `harness.GXSystemPrompt` (harness/systemprompt.go:15) covers only `gx tickets ask`; `GXFilesSentence`
  (systemprompt.go:18) covers `gx files url`. Neither mentions background tasks, subagents, polling or the task
  notification. `GXLoopPrompt` is the two joined (systemprompt.go:21). The same text sits between the
  `gx-system-prompt` markers in `skills/gx-local-tracker.md`, kept equal by a test.
- `skills/gx-local-tracker.md:410-415` maps "wait for a background task" to `Monitor` and says not to use a
  hand-written `sleep`/`until` loop. That tells the agent how to wait, not to wait for the notice.
- Closest rules, both skill-local: `skills/gx-verify-epic/SKILL.md:134` ("Wait for the harness to say the
  background task ended. Do not start a Monitor for it") and `skills/gx-merge/SKILL.md:178` ("poll with a timeout,
  never a fixed sleep", about a different wait). `gx-merge`, the skill 49 ran, has no wait-for-notification rule.

## Not verified

- I did not replay the 15:29 launch to confirm the resumed-session mismatch in section 3.
- The ticket-49 agent's "still running" polling (the `ListAgents` roster dumps) is in the transcripts but I did not
  count it. It is real agent behavior, separate from `prompt-nudged`.

## Recommendation (correct fix, not the quick one)

- Close ticket 03's premise: `prompt-nudged=18` was not stall nudges. Merge its cause into ticket 01 as a second
  trigger of the same bug, as ticket 03's decisions already say ("If the nudges were launch-prompt resends, merge into
  ticket 01").
- Ticket 01's fix must cover this trigger too. Do not resend while the transcript or pane shows the agent working,
  and make `launchedBy` compare the record the launch actually produced (the last person record at or after the send
  time, not the first record of a resumed session). A pasted-content match alone leaves this case broken.
- Keep the separate small prompt rule from ticket 03's decisions: add one sentence to `GXLoopPrompt` telling agents
  that after starting a background task or subagent they end the turn and wait for its task notification instead of
  polling. Update `skills/gx-local-tracker.md` in step (a test enforces equality) and keep it one line (herdr
  refuses newlines in launch arguments, harness/systemprompt.go:12-14).
- The filtered `ListAgents` mode stays out of scope (Claude Code owns the tool).
