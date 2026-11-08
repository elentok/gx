# Agent launch and recovery

## Problem Statement

gx launches agents and watches them, and several faults it can detect today end as a wrong record or a useless pause. The person is told the agent chose something it did not choose.

- A fresh agent ignores keys after its first prompt is typed. The ticket pauses and the pause hides why.
- gx cannot prove a Codex launch prompt landed. Claude has a proof, Codex has none.
- A holder dial timeout under machine load pauses a healthy run, and the Retry advice can start a second agent beside the live one.
- The stuck-finish sweep pauses a ticket whose run the server still tracks, and offers a Retry the server refuses.
- A relaunched verify session re-pays its orientation and loses what the first session verified. No agent leaves a summary of what it did.
- A classifier "no verdict" outage is read as a commitless finish, and the stall nudge misfires.
- An agent that waits on its own background task is nudged, then parked as zero-commit. A hung background task keeps a finished, commitless ticket `claimed` for 2 hours with no log line. The TUI agents tab showed `working +bg` after every task was done. Three readers (finish wait, nudge, `+bg`) disagree about whether a task is live.
- The claim path and the human Retry path reserve a slot in two different ways, so a Retry during a loop pass can overshoot the epic cap by one.
- When an agent asks a question, the web shows a plainer card than a design round does. The person cannot answer with a note alone and cannot say they did not follow the question.
- gx-evolve cannot learn from design epics, and nothing records the questions people could not follow.

## Solution

gx launches every agent, Claude and Codex, so it can prove the first prompt landed, and recovers by itself from the faults it can detect. It resends, relaunches or waits. It never records a gx-side fault as the agent's choice. A pause that remains names the real cause and offers only web actions that work (ADR 0042). A relaunch never starts a second agent beside a live one and keeps the verdicts an earlier session wrote.

- A pane that stops taking keys is detected, stopped and relaunched once.
- A Codex launch prompt counts as delivered when the rollout file holds a matching user record. gx resends in the session, then relaunches once, then pauses.
- A slow holder dial is retried with backoff. A live holder is re-adopted. Only a dead holder relaunches.
- One "a background task is live" reading drives the finish wait, the stall nudge, the zero-commit park and the TUI `+bg` label. A task that stays live past the cap pauses the ticket as `background-task-gate`. The card shows the task and offers "Keep waiting" (and, for Claude, "Stop task").
- A classifier "no verdict" outage is retried by gx, and pauses as `classifier-outage` only when the retries run out.
- Verify agents record per-story verdicts in the ticket, every iteration agent leaves a short summary, and gx adds its own entry about what it did. The ticket detail view shows the latest.
- The web shows agent questions in the same card module as design rounds.
- gx-evolve runs on design epics and reads the questions people could not follow.

## User Stories

Pane that stops taking keys

1. As a person, I want gx to detect a pane that stops taking keys (the screen is identical across two clear rounds and the input still holds gx's own prompt text), so that a dead pane is not mistaken for a slow one.
2. As a person, I want gx to stop that session, confirm it is gone and relaunch once, so that the ticket recovers without me.
3. As a person, I want the relaunch never to start a second agent beside a live one, so that two agents never edit one worktree.
4. As a person, I want the same recovery for Claude and Codex, and for the launch path and the stall nudge, so that the fix has no gaps.
5. As a person, I want the last screen lines in the run log and the pause text when the input is not empty or the pane hung, so that I can see what the pane showed.
6. As a person, I want a pause worded with the real cause ("pane stopped taking keys, twice") when the relaunch also hangs, and a web Retry that ends it, so that the last resort is honest and recoverable.

Codex launch proof

7. As a person, I want a Codex launch prompt to count as delivered only when the rollout file holds a matching user record (after a resume offset for a resumed launch), so that gx proves delivery the way it does for Claude.
8. As a person, I want gx to find the right rollout by working directory, start time and a string `source` when no session id exists, so that guardian and subagent rollouts are not mistaken for the agent's.
9. As a person, I want gx to resend the launch prompt in the session, then relaunch once, then pause as `agent_prompt_stalled`, for every ticket type, so that Codex recovers like Claude.
10. As a person, I want the stall nudge to check that the first person record matches the launch prompt, and for a Codex session with no rollout to resend the launch prompt and refuse the generic nudge, so that a lost launch prompt is never papered over.
11. As a person, I want the run log to show each resend as `prompt-nudged` and the `launch-failed` reason to say "no user record after 30s", so that I can read what happened.

Holder dial and re-adoption

12. As a person, I want gx to retry a holder dial with backoff, so that machine load does not pause a healthy run.
13. As a person, I want a ticket whose dial failed but whose holder is alive to stay tracked and be re-adopted on the next tick, up to a cap on re-adoption attempts, so that no pause or relaunch happens for a briefly slow holder and a never-reachable one cannot be retried forever.
14. As a person, I want the ticket card to show "holder unreachable, retrying" while gx retries or waits to re-adopt, with a matching run-log event, so that I can see the hold.
15. As a person, I want a dead holder to relaunch once through the shared launch path (resume the session, keep verdicts) before any pause, so that the common failure recovers.
16. As a person, I want the pause, if the relaunch fails too or the re-adoption cap is reached, to be an `iteration-error` pause that names the real cause (holder dead, or dial failed N times with the holder alive), with the web buttons Retry and Start over, so that I can act on it.
17. As a person, I want Retry to attach to a live holder or agent, and Start over to stop it, confirm it is dead and then launch, so that no second agent ever starts.

Stuck-finish sweep and one launch path

18. As a person, I want the stuck-finish sweep to skip every ticket whose run the server tracks, so that it never pauses a run that is still finishing.
19. As a person, I want the web to offer no verb the server refuses, using one shared check for the offered actions and the write guard, so that a button never fails.
20. As a person, I want a paused ticket whose run is still tracked to show that a run is still finishing and offer no Retry until it settles, for the pause kinds whose verbs need an untracked run, so that Retry does not race the run. `background-task-gate` is exempt: its run is tracked by design and its card always offers its own buttons.
21. As a person, I want the claim path and the human Retry to share one claim-and-reserve step, so that a Retry during a loop pass cannot overshoot the epic cap.

Background task signal

22. As a person, I want one shared reading of "a background task is live" (the pid-file status combined with the transcript probe) to drive the finish wait, the stall nudge, the zero-commit park and the TUI `+bg` label, so that they never disagree.
23. As a person, I want a clean transcript to be able to clear a stale `shell` pid-file status, and the disagreement logged once per session, so that `working +bg` does not stay after every task ended.
24. As a person, I want a finish wait that skips background tasks started before the agent's finished report once its turn has ended, with a gate event per skipped task, so that a finished ticket is not held for hours and I can see what was skipped.
25. As a person, I want gx, after the agent has reported finished, to stop and kill each leftover task of that ticket, and to log each kill, so that no task keeps running after the run settles.
26. As a person, I want an unreported finish with a live task to be checked every 10 minutes by reading state only (process alive, notification arrived, output size), with a "still waiting" run-log line each time, so that the hold is visible and nothing is typed to the agent.
27. As a person, I want the wait to release early only when the task's process is gone and no notification arrived, never because the output file is quiet, so that a slow but healthy task is not cut off.
28. As a person, I want a task still alive at the 60 minute cap to pause the ticket as `background-task-gate`, never `zero-commit`, so that the pause names its real cause. The same cap and pause apply to every live-task hold (an unreported finish, a task started after the finished report, an open turn).
29. As a person, I want the `background-task-gate` card to say plainly that a background task is still running, and to show the task's label or command, how long it has run, whether its process is alive, the time of its last output and its last 3 output lines, so that I can judge whether to wait.
30. As a person, I want a "Keep waiting" button on that pause that puts the live run back in the finish wait with a fresh 60 minute cap, starting no new agent and killing nothing, so that I can extend the wait from the web.
31. As a person using a Claude session, I want a "Stop task" button on that pause that ends the task's process (SIGTERM, then SIGKILL after 2 seconds) and lets Claude wake itself, so that I can end a hung task from the web.
32. As a person, I want Stop task to refuse and say so when it cannot tell which process belongs to the task, and to be refused for Codex sessions, so that gx never kills the wrong process.
33. As a person, I want the stall nudge to re-read the signal right before typing and go back to the finish wait if a task is live, so that an agent waiting on its own task is not nudged.
34. As a person, I want the zero-commit park to re-read the signal once before pausing and wait if a task is live, so that a live agent is not parked.
35. As a person with a Codex session, I want the title heuristic to count as "live" with the same cap and the same pause, so that Codex gets the same safety.
36. As an agent, I want my implement prompt to allow a WIP commit before a long background run (the finishing commit still needs `gx check`) and to say to use `</dev/null` in shell loops that read files, so that my work is saved and my loops end.
37. As a person, I want the gate events to reach the run log and `gx server tickets verify`, so that a hold is never silent. The events were missing because the runners hid a live Claude task from the finish wait (see Implementation Decisions).

Classifier outage

38. As a person, I want gx to count consecutive "no verdict" tool-result errors at the end of a Claude transcript, so that an outage is recognised.
39. As a person, I want gx to re-send "continue" after 1, 2 and 4 minutes while the outage lasts, and not read a turn end as a finish meanwhile, so that no commitless finish, zero-commit park or answer pause is recorded for an outage.
40. As a person, I want the stall nudge skipped while an outage shows at the end of the transcript, so that it does not misfire.
41. As a person, I want a `classifier-outage` pause when the retries run out, saying the classifier gave no verdict, with a web Retry that re-sends and ends it, so that a provider outage is recoverable.
42. As an agent, I want my prompt to say to retry the same command once on a "no verdict" error and then report the blocker once, so that I do not loop.

Iteration verdicts and summaries

43. As a verify agent, I want to append a `## Verified` entry per story as I finish it (story, verdict, the commit checked), so that a crash or relaunch keeps what I checked.
44. As a relaunched verify session, I want to trust and skip entries whose commit equals the worktree's HEAD and re-check the rest, so that I do not repeat orientation or verification.
45. As a person, I want no Verified entry to count after Start over, shown as history only, so that discarded code is not trusted.
46. As an iteration agent of any ticket type, I want to append a short final summary (what I did, what went wrong or surprised me, decisions, what is left) to `## Iteration summaries`, so that the next session and the person know.
47. As a person, I want gx to append its own entry to the same section when it handled a fault (resends, relaunches, a re-adopted holder, a nudge), labelled as written by gx, so that a gx fault is never read as the agent's choice.
48. As a person, I want a missing agent summary never to block finishing or pause the ticket, so that the summary stays a help and not a gate.
49. As a person, I want the ticket detail view to show a "Latest summary" block with the agent's four parts and gx's own entry, so that I can read what the last iteration did.

Agent questions in the web

50. As a person, I want an agent question to render in the same card module as a design round question (letter, label, muted id and consequence line per option), so that both look and behave alike.
51. As a person, I want "Huh?" and Note on an agent question, and consequence lines on a design card, so that the two pages offer the same controls.
52. As a person, I want to answer an agent question with a note alone, and the agent to receive "no option picked" plus my note, so that I can explain without picking.
53. As a person, I want an empty choice with an empty note still refused, and Send enabled when a choice or a note exists, so that I cannot send nothing.
54. As a person, I want the agent-question card to keep its waiting border, the stale-question refetch and the agent-not-ready message, so that nothing in today's answer path regresses.
55. As a person, I want the web history to show "no option picked" for a note-only answer, so that the record is honest.

Huh? log and gx-evolve

56. As a person, I want Huh? to send "I could not follow this question. Could you explain it another way?" on both pages, so that the agent explains instead of guessing.
57. As gx-evolve, I want one record per question sent as Huh? (question text and title, page, harness, session id, time) written when the answer is sent, so that evolve can improve question phrasing.
58. As gx-evolve, I want an agent-question Huh? marked `huh: true` on its `pause.answer` event, and a design-round Huh? stored in a `huh` list on the conversation record, so that I filter on a field and never on message text.
59. As gx-evolve on a queue epic, I want `gx evolve inputs` to carry a `huh` flag and count per ticket, with the flag on at 1 or more, so that questions people could not follow are read.
60. As a person, I want gx-evolve to start automatically when a design epic closes `done`, under the project's existing evolve mode, and `gx evolve all` to include done maps, so that design work is evaluated like queue work.
61. As gx-evolve on a design epic, I want `gx evolve inputs <map>` to print one row per map ticket (type, grilling rounds, words, Huh? count, session ids) with flags `spec-round-cap`, `re-review`, `scope-changed`, `many-rounds` and `huh-cluster`, so that the deep read picks the right sessions.
62. As a person, I want `gx evolve run` and `gx evolve skip` to refuse a map that is not `done` unless `--force`, so that I cannot evolve an unfinished design by accident.
63. As a person, I want the design evolve ticket to show in the TUI and web like a queue epic's evolve, and its follow-ups to go to the `gx` project, so that I need no new view.

Added after review

64. As a person, I want `background-task-gate` to be the existing pause kind with a changed row: class answer, receiver gx, and verbs chosen by cause (live task: Keep waiting, Stop task for Claude, Start over; dirty worktree with commits and no live task: Retry and Start over), so that every way into the kind shows buttons that fit it.
65. As a person, I want a "Start over" button (stop the agent, confirm it dead, launch fresh) on every `background-task-gate` pause for every harness, so that a Codex task that never ends or a Claude task gx cannot match does not leave the ticket stuck.
66. As a person, I want recovery rule R10 to release a held gate only when the task's process is gone, and the dirty-worktree-with-commits path to keep the kind with its real cause on the card and a Retry button, so that a live healthy task is never released by an idle agent and a no-task pause does not show Keep waiting.
67. As a person, I want one 60 minute cap and one pause for every live-task hold, replacing the 2 hour aged-out cap and its expiry event, so that the wait is the same however the hold began.
68. As a person, I want a never-reachable holder to stop being re-adopted after a cap and to pause as `iteration-error` with Retry and Start over, so that no ticket is retried forever.
69. As a person, I want every stored-data change in this epic to be additive, so that tickets, conversation records and events written before the upgrade still load without a rewrite.

## Implementation Decisions

Pane that stops taking keys

- The screen-clearing step returns a typed hung-pane error that wraps the existing not-delivered error. A pane is hung when the screen is identical across both clear rounds and the input still holds gx's own prompt text. No process probe or timer. The same-pane retry for "input not empty" is removed for this case.
- The launch path and the stall nudge both treat the hung-pane error as a dead pane: stop the session, confirm it is gone, relaunch once through the shared launch path. Claude and Codex both get it.
- On "input not empty" or a hung pane the last screen lines go into the run-log event and the pause text.
- Last resort: a second hang pauses as `agent_prompt_stalled`, worded "pane stopped taking keys, twice". No new pause kind. The spec makes no claim about the slash-command menu root cause; the lost-launch-prompt research stays open.

Codex launch proof

- Proof is the rollout file's user message after the injected AGENTS.md and environment lines, plus a following skill record for a `$skill` prompt. The file is created at the first turn and flushed per line. Without a session id, match on working directory, start time at or after launch and a string `source`. A resumed launch appends to the old file, so the proof is a new user line after an offset recorded at send time.
- Reuse the existing launch-prompt seam, the start-and-prompt retry and the Codex session readers. New: a first-user-prompt reader, a Codex launched-by record, Codex branches in both runners' prompt-launch steps, resume offset handling, and a Codex user-record reader.
- The transcript record is the proof; the pane only shortens the wait. Resend in-session, then relaunch once, for all ticket types. Reuse `agent_prompt_stalled` and `prompt-nudged`; no new event kind. Record on launch that proof passed and skip the re-check.

Holder dial

- Retry the dial with backoff (about 3 tries over 15 seconds, constants the implementer picks). If the dial still fails and the holder pid is alive, keep the iteration tracked and re-adopt on the next tick. A dead holder relaunches once through the shared path, then pauses naming the cause.
- The ticket card shows "holder unreachable, retrying" with a run-log event, cleared on a successful dial or when the run ends.
- Re-adoption of a live but unreachable holder is capped (attempt count is the implementer's constant). At the cap the ticket pauses as the existing `iteration-error` kind (class repair, verbs Retry and Start over, both served by the web), worded with the holder-alive cause. Same kind and buttons when a dead holder's single relaunch fails. gx cannot recover on its own at that point, and the web buttons end the pause (ADR 0042).
- Before any relaunch, the shared launch path checks for a live holder or agent for the ticket's session. Retry attaches to one. Start over stops it, confirms it dead, then launches.

Shared claim and reserve

- One claim-and-reserve step takes the scheduler's claim lock and does claim plus reserve. The scheduler loop and the human Retry both call it; the caller picks inline or background launch. A failed Retry kicks the scheduler as a failed loop claim does. The live-holder check and the hung-pane relaunch plug in here. The claimed record embeds the tracked run and takes the launch context as a parameter. The test-only claim helper is removed or becomes a thin wrapper. Retry stays synchronous.

Sweep and offered actions

- The stuck-finish sweep skips every ticket whose run is tracked, and pauses only an untracked ticket, with a reason that keeps the finished-at time and says the server no longer tracks the run. The offered actions and the write guard use one registry check. A paused ticket with a tracked run offers no Retry until it settles. The existing `iteration-error` pause keeps its web verbs.

Background task signal

- One shared reading (tasks, awaiting-turn-end, source, disagreement flag) replaces the three transcript finders. Where the type lives, how the finders merge, the catalog predicate and the structured `task_id` fix are the implementer's choices. The `task_id` fix is required before a clean transcript may clear `shell`: today any TaskStop resolves a task and the raw-line match lets the TaskStop result of an agent's own poll mark a live task resolved.
- Precedence: the pid-file status wins; a clean transcript can clear `shell`. A disagreement is logged once per session with no TUI marker. A read error means "not live" as today.
- Finish wait: with a finished report and an ended turn, skip tasks that started before the report and log a gate event (`released`, "agent reported finished, task X ignored") each. A later task or an open turn still holds. Log write errors are returned, not dropped. Cause of the missing gate events (research ticket 19): the write path works and no event kind was renamed, but since 2026-10-09 neither runner lets the runner's wait return while a Claude background task is live, so the finish wait never reaches the one writer of the `held` event. The herdr runner forces Working for a fresh task; the PTY runner (default since 2026-10-30) ignores the pid file's `shell` status and falls through to Working. Clean fix at the cause: the runners report Idle plus Background for a live Claude task (a `ClaudeShell` case in the PTY runner's status; Background instead of forced Working in herdr), and the finish wait does the holding. The runner conformance scenario `finishWaitsForBackgroundTask` encodes the hiding as the contract and is changed to expect Idle plus Background. The silently dropped log-write errors in the gate and agent event loggers are returned too (a secondary defect).
- Leftover tasks: after the skip the run settles and the normal stop path kills them. The herdr stop snapshots the pane shell's descendants before closing the tab and sends SIGTERM, then SIGKILL after 2 seconds; the native runner kills the process group. Each kill is logged. A task that detached itself before the snapshot is out of reach.
- One cap for every live-task hold: a task started after the finished report and an open turn hold the same way as an unreported finish, with the same 60 minute cap and the same pause. The 2 hour aged-out cap and its `background-task-gate-expired` event are replaced by the 60 minute pause.
- Unreported finish with a live task: every 10 minutes read state only and write a "still waiting" line. Release early when the process is gone and no notification arrived. A quiet output file never releases. Hard cap 60 minutes, then pause as `background-task-gate`, naming the task. gx never kills a task while the agent has not reported finished; after a finished report the stop path kills leftovers.
- The pause card shows the label or command, run time, process alive, last output time (the output file's modification time) and the last 3 output lines. No CPU reader. Buttons while a task is live: "Keep waiting" (all harnesses), "Stop task" (Claude only; the server refuses it for Codex) and "Start over" (all harnesses; stops the agent, confirms it dead, launches fresh). Keep waiting and Stop task appear only while a task is live.
- Keep waiting re-arms a fresh 60 minute finish wait for the still-tracked live run, starts no agent and kills nothing. If the process is already gone, the wait releases as usual.
- Stop task: walk the descendants of the claude pid (works from the holder pid) and match each task to its process by the task's output file open as the process's stdout (read with `lsof -p`; a Go reader may replace it later). With only command text and an ambiguous match, refuse and say so. Never choose by "oldest". Send SIGTERM, then SIGKILL after 2 seconds. Types nothing; Claude writes its own failed notice and wakes. Exit codes vary and are never parsed. Stopping an async Agent task is a prompt asking Claude to run TaskStop (the prompt text is in the stop-task gaps research file). The CLI spelling of the server verb is the implementer's choice.
- Nudge and park re-read the signal right before acting and fall back to the finish wait if a task is live. The nudge text is otherwise unchanged. Codex keeps its title heuristic as "live" with the same cap and the same pause.
- Implement prompt (skill text): a WIP commit is allowed before a long background run, the finishing commit still needs `gx check`, and shell loops that read files use `</dev/null`.

Classifier outage

- Count consecutive "no verdict" tool-result errors at the end of the Claude transcript (same read route as the rate-limit error). Claude only; Codex has no classifier.
- Re-send "continue" after 1, 2 and 4 minutes in the same session. While a retry is pending, a turn end is not a finish. No relaunch.
- When retries run out, pause as `classifier-outage`, class repair, with its own event kind so the run log and stats can count outages. The web Retry re-sends and ends the pause. The stall nudge is skipped while the outage shows.
- Prompt line: retry the same command once on a "no verdict" error, then report the blocker once. The launch prompt does not carry the ticket file path.

Verdicts and summaries

- Verify tickets get a durable `## Verified` section with an entry per story (story, verdict, commit checked). The prompt tells the agent to write one after each story and to start from the section on a relaunch. A new session trusts entries whose commit equals HEAD and re-checks the rest. After Start over none count.
- Every iteration agent of any ticket type appends one entry (session id, time, what it did, what went wrong or surprised it, decisions, what is left; about 10 lines) to `## Iteration summaries`. At iteration end gx appends its own entry listing what gx did, labelled as gx. A missing agent summary never blocks or pauses. Entry format, wording and length are the implementer's choice. A relaunched session reads earlier summaries and Verified entries.
- The ticket detail view shows a "Latest summary" block: the agent's four parts and gx's own entry. The Epics-page cards do not change.

Pause kinds and ADR 0042

- `background-task-gate` is an existing kind being changed, not a new one. Today: class answer, receiver agent, verb Retry. After: class answer, receiver gx, with a changed row. Every way into it: (a) a live background task at the 60 minute cap, for any hold (unreported finish, task started after the finished report, open turn), Claude or Codex; (b) recovery rule R10, where the agent goes idle with a dirty worktree and commits ahead and no task is live.
- Verbs by cause. (a): "Keep waiting", "Stop task" (Claude only) and "Start over". (b): "Retry" and "Start over"; the card shows the real cause (uncommitted work with commits ahead), not a task. Retry continues the ticket's work, attaching to a live agent when there is one, otherwise launching a fresh one on the same branch. Keep waiting and Stop task are never offered without a live task.
- Rule R10 is narrowed: it force-releases a held gate only when the task's process is gone. It no longer releases a gate whose task process is alive just because the agent is idle.
- Why gx cannot recover alone in (a): a person must judge a task that may be healthy. Start over is the exit when Stop task is refused or cannot match the process. All buttons are served by the web.
- `classifier-outage` is a new pause reason, class repair (a provider outage needs repair, not an answer), with Retry. It has a row in the pause-to-verb table. gx cannot fix a provider outage, and Retry ends the pause from the web. The test that fails for a kind with no web verb covers it.
- The "still finishing" rule of story 20 covers only kinds whose verbs need an untracked run. `background-task-gate` (a) is exempt: its tracked run is intended and its buttons act on it.
- `agent_prompt_stalled` and `iteration-error` are reused; Retry and Start over end them. A new pause arises only after gx's own resend or relaunch has failed, which keeps pauses few. No fault above is ever recorded as `zero-commit`, a commitless finish or an answer pause.

Stored data and schema

- All changes are additive; no migration step is needed (the pause reason, class and receiver tables are code, the new fields are optional). For each, a test loads data written before the change and checks it still parses:
  - `classifier-outage` pause reason: a new enum value; old tickets never carry it. Test: the pause-reason parse test over an old fixture ticket.
  - `background-task-gate` row change (receiver, verbs): class unchanged, row derived from code and not stored. Test: an old gate-paused ticket renders with the new row.
  - `## Verified` and `## Iteration summaries`: new optional ticket body sections; a ticket without them parses as before. Test: parse an old ticket body.
  - `huh` list on the conversation record: a new optional field; absent means empty. Test: load an old conversation record.
  - `huh` on `pause.answer`: a new optional event field; absent means false. Test: read an old ticket log.
  - Evolve fields: reused unchanged.

Agent questions in the web

- One shared card module holds the question card, the progress dots and the round helpers (answer, answers, choices, is-answered). It renders cards and owns no send logic. The design page keeps its composer Send. The agent-question form wraps it as a round of one (label = question sequence, title = summary line, body = detail, kind, options and recommended unchanged), adds a "Send answer" button in the same sticky slot and keeps the stale-question refetch and the agent-not-ready message. A round of one hides the dots, the count, Prev/Next and the fold button. The waiting border stays.
- Each option row shows letter, label, muted id, then the consequence line. Yes-no buttons read Yes and No with the consequence under them.
- The server accepts an empty choice with a non-empty note for yes-no and options questions. The agent gets "Reply from the user to your question N: no option picked" with the note on the next line. `pause.answer` keeps the empty choice and the note; history shows "no option picked". Empty choice and empty note stays refused as `empty-answer`. Free-text questions do not change.
- Huh? is sent as a note-only answer with the fixed message above; the server has no special case beyond the `huh: true` field.

Huh? log and gx-evolve

- Agent question: `huh: true` on `pause.answer`. Design round: a `huh` list on the conversation record next to `grilling`, each entry holding question text, title, round message id and time; the web sends it when the round is sent. Logged on send, once per question. A later pick after the explanation is a new question. Each record holds question text and title, page, harness, session id and time.
- Queue evolve: `gx evolve inputs` gets a `huh` flag (1 or more) and count, read from `pause.answer` events. This is a new read, since evolve reads no ticket logs today.
- Design evolve: starts when a map closes `done` under the existing evolve mode; the guard that keeps maps out of the completion hook goes away; `gx evolve all` includes done maps. `gx evolve inputs <map>` detects a map and prints one row per map ticket with these flags: `spec-round-cap` (2 or more `spec-round-` tickets), `re-review` (a `## Re-review findings` section), `scope-changed` (a finding marked `(stories/scope changed)`), `many-rounds` (3 or more grilling rounds on one ticket), `huh-cluster` (2 or more Huh? on one ticket, or 3 or more across the design). A rejected self-decided question gets no flag; the deep read judges it.
- No new command. The deep read runs on design sessions through the existing digest mechanism. The evolve agent files follow-ups to the `gx` project and never edits code; targets are the gx design skills, grilling phrasing and the spec template; for each Huh? cluster it lists the question text and the skill text that wrote it. `evolved_at`, `evolve_skipped` and `evolve_ticket` are reused unchanged: no schema change, no migration. `run` and `skip` refuse a map that is not `done` without `--force`. The evolve ticket is a top-level one-off. No design-step history is added.

## Testing Decisions

- A good test drives external behaviour: what the fixture server, run log, pause, card and CLI show. It does not assert on private helpers.
- Highest seam: the gx fixture server with fake agents (the herdr-fake and PTY runners, a fake Codex with a rollout directory, a fake Claude transcript and pid file), driven through the web e2e flows and `gx server` verbs. Lower seams only where a fault cannot be produced from there: drawn-screen probe tests for the hung pane, rollout-reader tests for Codex proof, transcript-probe tests for the live reading and the classifier outage, and process tests for the leftover kill and Stop task.
- Gate-event regression test shape: run the finish wait over a real PTY runner with a fake Claude whose pid file reports `shell` plus an outstanding background-task marker; the run log must show `held`, then `released`. The same over the herdr runner. The changed runner conformance scenario also covers it.
- Required tests named in the map: a finished ticket with an outstanding shell task has its wait skipped and the task's process dead after the stop; the server finish path with an outstanding marker writes a `held` event; a pause kind with no web verb fails the pause-to-verb test; a live-holder-or-agent check prevents a second agent in every launch path; cap accounting during a Retry in a loop pass.
- Prior art: the lifecycle and no-silent-stalls specs' launch-prompt probe tests, the existing pause-reason web flow ("Every pause reason recovers"), the gate-event tests, `gx evolve inputs` tests and the question-sheet unit tests.
- Web flows must follow the e2e budget: add steps to a flow on the same scenario before adding a flow, and keep the catalog and `maxFlows` in step.
- Unverified facts the implementer confirms with real runs: SessionStart hook timing on a live Codex launch; an unknown `$skill` leaving no skill record; behaviour of `gx evolve inputs` on a map.

## Verification

Entry N verifies story N. "Pause flow" is the web e2e flow "Every pause reason recovers"; "question flow" is the design-page flow with a round answered (`design.e2e.ts`); "epic flow" is the Epic page flow in `epics.e2e.ts`. No stored mocks exist, so there is no Mock states list.

1. Probe test with two identical drawn screens and gx's text in the input: the typed hung-pane error. New test.
2. Fake agent whose pane stops taking keys: the session stops, is confirmed gone, one relaunch happens (`gx server tickets explain` and the run log). New fake knob.
3. Same scenario: `gx server` shows one live agent for the ticket at all times. New assertion.
4. Same scenario for a Codex fake and for the stall-nudge path. New scenario.
5. Same scenario: the run-log event and pause text contain the last screen lines.
6. Fake hangs twice: ticket paused `agent_prompt_stalled`, text says "pane stopped taking keys, twice"; the pause flow's Retry ends it.
7. Codex fake with a rollout holding the launch prompt: delivered; without: not delivered. Rollout-reader tests. New fixture.
8. Rollout directory with a guardian rollout and a subagent rollout beside the real one: the real one is chosen. New test.
9. Codex fake that drops the prompt: `prompt-nudged` resend, then one relaunch, then `agent_prompt_stalled`, for a normal and a merge ticket. New scenario.
10. Codex nudge test: first person record not matching the launch prompt triggers a resend of the launch prompt; no rollout means the generic nudge is refused.
11. `gx server logs` after story 9 shows `prompt-nudged` per resend and a `launch-failed` reason with "no user record after 30s".
12. Fake holder whose dial times out twice then answers: the run stays tracked and no pause. New knob on the holder fake.
13. Fake holder that never answers but whose pid is alive: re-adopted on the next tick, no pause, no relaunch.
14. Same scenario: the ticket card on the epic flow shows "holder unreachable, retrying" and clears when the dial works; the run log has the event.
15. Dead holder: one relaunch resumes the session and keeps verdicts.
16. Dead holder with failing relaunch: pause text names the cause for both variants (holder dead; dial failed N times with the holder alive).
17. Live holder then Retry attaches (no second agent); Start over stops it, confirms it dead, then launches. Check with `gx server tickets retry` and `start-over`.
18. Tracked finishing run past the sweep interval: not paused; an untracked one is paused with finished-at text. New scenario.
19. Test that every verb in `detail.actions` is accepted by the write guard for tracked and untracked states.
20. Paused ticket with a tracked run: the card says a run is still finishing and has no Retry (pause flow, new step).
21. Cap-2 epic, Retry during a loop pass: running count never exceeds 2. New test.
22. Live-reading tests: pid file `shell` with outstanding task, pid file idle with outstanding task, and the finish wait, nudge, park and `+bg` all read the same value. New table test.
23. Transcript with the agent's own TaskStop poll echoing a live task's id: task stays live (structured `task_id`). With a clean transcript and a stale `shell`, `+bg` clears and one disagreement line is logged per session.
24. Ticket with a finished report, ended turn and an older outstanding task: the wait is skipped and the gate event `released` appears in the run log and `gx server tickets verify`. Task started after the report still holds.
25. Same ticket with a real sleeping process: after the stop the process is dead and the kill is logged (herdr and native runners). A detached process is documented as out of scope.
26. Unreported finish with a live task: a "still waiting" run-log line every 10 minutes (test clock) with no text typed into the pane.
27. Task process killed with no notification: the wait releases early. Quiet output file with a live process: no release.
28. Task alive at 60 minutes (test clock): ticket paused `background-task-gate`, never `zero-commit`. Pause flow gains this row.
29. The card shows the plain statement and the task label, run time, alive flag, last output time and last 3 lines. New fixture scenario with a task output file.
30. Keep waiting on the paused card: pause lifts, same agent, no new agent, no kill, new 60 minute cap. New e2e step.
31. Claude fixture with a real child process: Stop task ends it, Claude's own notice arrives, nothing is typed. Test of the SIGTERM then SIGKILL sequence.
32. Duplicate command text with distinct output files picks the right process; command-text-only ambiguity refuses with a message; a Codex session gets a refusal and the card has only Keep waiting.
33. Nudge test: signal flips to live between the stall decision and the type step: nothing typed, back to the wait.
34. Park test: signal flips to live before the pause write: no pause.
35. Codex fake with the title heuristic: held, then paused `background-task-gate` at the cap with only Keep waiting.
36. Prompt check: `rg` over the implement skill finds the WIP commit allowance and the `</dev/null` line, and still requires `gx check` for the finishing commit.
37. Finish wait over a real PTY runner with a fake reporting pid-file `shell` plus an outstanding marker: the run log shows `held`, then `released`; the same over the herdr runner; the changed `finishWaitsForBackgroundTask` conformance scenario passes on both. A failing log write returns an error.
38. Transcript-probe test with a tail of N "no verdict" errors counts N; a non-outage tail counts 0.
39. Fake Claude transcript with an outage: "continue" is re-sent at 1, 2 and 4 minutes (test clock); no commitless finish, zero-commit park or answer pause in the run log.
40. Same scenario: no stall nudge during the outage; the unchanged nudge for another stall.
41. Retries exhausted: `classifier-outage` pause with its own event kind; the pause flow gains the row; Retry re-sends and ends it.
42. Prompt check: the implement prompt carries the retry-once-then-report line.
43. Verify ticket run: a `## Verified` entry per story appears as each finishes (fixture verify agent). New scenario.
44. Relaunch the verify session: entries at HEAD are skipped, others re-checked (fake agent's received input).
45. After Start over, entries show as history and are not trusted.
46. Fake agents of an implement, a verify and a merge ticket each append an `## Iteration summaries` entry; the next session's prompt includes it.
47. A run with a resend and a relaunch gets a gx-labelled entry listing both.
48. Agent finishes with no summary: the ticket finishes, no pause, and the gx entry is still written.
49. Ticket detail view shows the "Latest summary" block with the four parts and the gx entry; the Epics-page cards are unchanged (epic flow). New step.
50. Question flow for an agent question: letter, label, muted id and consequence per option. New step in the agent-question flow, run at 1440px and 375px.
51. Agent question shows Huh? and Note; a design card shows consequence lines.
52. Answer an agent question with a note alone: the agent's received input is "...no option picked" plus the note.
53. `gx server` answer verb with empty choice and empty note: `empty-answer`. Send is disabled until a choice or note exists.
54. Agent-question form keeps the waiting border, the stale-question refetch and the not-ready message (existing tests stay green).
55. History shows "no option picked" for the story 52 answer.
56. Clicking Huh? sends the fixed message on both pages (assert the agent's received input).
57. Huh? sent: one record with text, title, page, harness, session id and time; a second click on a new question adds a new record; no record on the click alone.
58. The `pause.answer` event has `huh: true`; the conversation record has a `huh` list. Check with the fixture's stored files.
59. `gx evolve inputs` on a queue epic with one Huh? answer: `huh` flag and count 1.
60. Close a design map `done`: an evolve ticket starts under the evolve mode; `gx evolve all` lists done maps.
61. `gx evolve inputs <map>` on a fixture map with each condition: one row per ticket and the five flags; below thresholds no flag.
62. `gx evolve run` and `skip` on an open map exit non-zero; with `--force` they proceed.
63. The evolve ticket shows in the TUI and the web as a top-level one-off (epic flow); its follow-ups land in the `gx` project.

64. Pause flow: a gate pause for a live task shows Keep waiting, Stop task (Claude) and Start over; a gate pause from the dirty-worktree path shows its real cause with Retry and Start over and no Keep waiting or Stop task. Unit check of the row (class answer, receiver gx).
65. Codex gate pause: Start over stops the agent, confirms it dead and launches fresh; same for a Claude gate whose task cannot be matched. New e2e step.
66. R10 test: idle agent, dirty worktree with commits, task process alive: not released; task process gone: released. Dirty-worktree path pauses `background-task-gate` with the cause shown.
67. Test clock: a task started after the finished report, and an open turn, each pause `background-task-gate` at 60 minutes; no `background-task-gate-expired` event at 2 hours.
68. Holder alive but never answering: after the cap the ticket pauses `iteration-error` worded with the holder-alive cause; the pause flow's Retry and Start over end it.
69. Load tests listed under "Stored data and schema" pass on fixtures written before the change; `gx tickets migrate` reports nothing to migrate.

New flows or scenarios to write (these become implementation tickets): the fake-agent knobs for a hung pane, dropped Codex prompt, holder dial delay, background task with an output file and real child process, classifier-outage transcript, verify and summary agents, and map evolve fixtures. Steps go into existing flows first, per the e2e budget.

## Out of Scope

- Finding why launch prompts are lost in the first place (the slash-command menu cause stays an open research item).
- A task that detached itself from its shell before gx snapshots descendants.
- A CPU reader for background tasks, and any release on a quiet output file.
- Stop task for Codex, and a classifier-outage fix for Codex (it has no classifier).
- A TUI marker for the pid-file/transcript disagreement.
- The holder dial cause in `gx:follow-ups/223`, and the wrong recommended value on the question card (`gx:follow-ups/39`).
- Design-step history, and any change to the Epics-page cards.

## Further Notes

- Research files: `docs/research/agent-launch-and-recovery-02-codex-launch-proof.md`, `-08-background-task-signal.md`, `-10-tui-stale-bg.md`, `-13-stop-task.md`, `-14-stop-task-gaps.md`, `-16-evolve-design-epics.md`.
- Vocabulary: "Retry" continues the ticket's work, attaching to a live agent when there is one, otherwise launching a fresh one on the same branch; this epic adds "Keep waiting" and "Stop task" as separate verbs, and never uses the retired word "relaunch" for the ticket verb.

## Self-decided design questions

- Question: what class does each pause have? Choice: `background-task-gate` stays class answer (receiver gx), only `classifier-outage` is repair. Reason: a repair class would spawn an investigation on a pause whose point is a person judging a healthy task, and class is tied to the event kind.
- Question: does Keep waiting also apply to `classifier-outage`? Choice: no, that pause offers Retry only. Reason: Keep waiting is defined as re-arming a finish wait on a live run; an outage pause has no wait to re-arm.
- Question: do the new verbs need a CLI spelling and are they CLI-visible? Choice: the server verbs exist with an implementer-picked spelling, and the web is the primary path. Reason: ADR 0042 requires the web; the CLI is only for tests and scripts.
- Question: does the hung-pane and Codex proof recovery apply to every ticket type? Choice: yes. Reason: the map decided "all ticket types" for Codex proof, and a type-specific split would leave gaps.
- Question: fix the runners or the finish wait for the missing gate events? Choice: fix the runners. Reason: the finish wait is correct; the runners hide the live task from it, so changing the wait would patch the symptom.
