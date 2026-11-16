# Background progress: live task list in the web session chat

## Problem Statement

When an agent waits on a **Background task**, a subagent or a long foreground call, the web session chat tells the person almost nothing. Today it shows:

- `BackgroundStrip`: one "kind: label" chip per outstanding task.
- The Design page's Working row: "waiting on 1 shell, 2 subagents".
- The Working row: a foreground call in flight as "running shell: make test (+2 more)".

The person cannot see how long a task has run, what it has done so far, which tasks finished and which the agent still waits on, or whether a finished task succeeded. The Agents page's Working row shows no background tasks at all. The start time is already on the wire (`started_at`) and the UI ignores it.

## Solution

One quiet, read-only task list under the Working row, on both the Design page and the Agents page (they share one chat component). It covers background shells, Monitors, async subagents and foreground calls in flight, for Claude and Codex sessions. Per task it shows how long it has run, what it has done so far and whether it finished.

- A closed-by-default summary line: "3 of 7 tasks done", a thin progress bar and "longest 10m 40s". It opens into one row per task.
- A row has a status mark (pulsing dot while running, a check when done, a cross when failed), the kind, a label and the elapsed time. Running rows come first. Finished rows are dimmed below, in launch order.
- An expanded row adds a details line, and for a shell or foreground call the command, lightly highlighted and capped at about five lines with "Show all (N lines)".
- A **Task group** starts when a task starts while nothing else runs. Finished tasks stay listed with duration and outcome until the group's last task finishes, then the group clears.
- The browser ticks the elapsed time every second from `started_at`. The server reads task files only when the chat is read, and caches each file by size and change time. There is no new polling loop.
- The list replaces `BackgroundStrip` and the "waiting on N" text. The TUI keeps its "+bg" badge. The paused-ticket background-task card does not change.

## User Stories

1. As a person watching an agent, I want to see how long each background shell has run, so that I can tell a slow build from a stuck one.
2. As a person watching an agent, I want to see how long each async subagent has run, so that I know how much of the wait is behind us.
3. As a person watching an agent, I want to see how long each Monitor has run, so that I can tell a fresh watcher from an old one.
4. As a person watching an agent, I want a foreground call in flight listed in the same place as background tasks, so that I have one place to look for what the agent waits on.
5. As a person, I want a summary line reading "N of M tasks done" under the Working row, so that I see progress at a glance without reading every row.
6. As a person, I want the list closed by default, so that a long task list does not push the conversation off screen.
7. As a person, I want to open the summary line to see one row per task, so that I can check a single task when I care.
8. As a person, I want each row to show a status mark, the kind, a label and the elapsed time, so that rows scan quickly.
9. As a person, I want a pulsing dot on a running task, a check on a successful one and a cross on a failed one, so that I read the outcome without text.
10. As a person, I want running rows first and finished rows dimmed below them, so that what the agent still waits on stands out.
11. As a person, I want finished tasks to stay in the list with their duration and outcome until the whole group is done, so that I can see what already ran.
12. As a person, I want the list to clear when the group's last task finishes, so that old tasks do not pile up on screen.
13. As a person, I want a new group to start when a task starts while nothing else runs, so that each wave of waiting is shown on its own.
14. As a person, I want a failed shell row to read "exit 2 · 12s" and a killed row to read "killed · 12s", so that I see the exit code or the kill and how long it ran together.
15. As a person, I want an expanded shell row to show the output size and the last output line, so that I can tell whether it is still producing output.
16. As a person, I want an expanded shell or foreground row to show its command with light highlighting for bash and python, so that a long command is readable.
17. As a person, I want a long command capped at about five lines with "Show all (N lines)", so that one command cannot take over the chat.
18. As a person, I want an expanded subagent row to show its model, token count, tool-call count and the call it runs now, so that I can tell what it is doing.
19. As a person, I want a subagent's own background tasks not nested or counted in "N of M", so that the list stays short and the numbers stay about the agent I am watching.
20. As a person, I want a running subagent to keep the group open even while its own tasks run unseen, so that the group does not clear while work continues.
21. As a person, I want an expanded Monitor row to show the watched command, "N events" and how long ago the last event was, so that I can tell whether the watch is alive.
22. As a person, I want a persistent Monitor to stay listed as a running row without holding the group open, so that a watcher that never ends does not keep the list on screen forever.
23. As a person, I want a task that was moved to the background after a timeout to show the time it really started, so that its elapsed time is not short by the timeout.
24. As a person, I want a task with no recorded end to show as finished without an outcome only by a stated rule (a Claude shell whose output file has an exit trailer, a subagent or Monitor past the age-out cap, a Codex or foreground task by the rules under Groups and states), so that gx never shows a made-up success or failure and never ends a task that still runs.
25. As a person using Codex 0.144.1 or later, I want the same list with fewer fields (label, time, exit code where known), so that Codex sessions are not left out.
26. As a person using an older Codex, I want the in-flight labels I have today, so that nothing gets worse.
27. As a person using Codex, I want a Codex subagent row to show its model and token total from its own rollout, so that I can see what it costs.
28. As a person on the Agents page, I want the same list as on the Design page, so that the two pages never disagree.
29. As a person on a phone, I want the list to fit a 360px screen without sideways scrolling, so that I can check progress anywhere.
30. As a person with a single task running, I want the row alone with no summary line, so that one task does not need "1 of 1 tasks done".
31. As a person, I want the elapsed time to tick every second without a network request, so that the page stays live and cheap.
32. As a gx operator, I want task files read only when a chat is read, so that a session nobody has open costs nothing.
33. As a gx operator, I want each task file parsed again only when its size or change time moves, so that watching 50 tasks costs a stat each, not a parse each.
34. As a gx operator, I want a finished task's file never parsed again, so that a long session does not get slower as tasks pile up.
35. As a person, I want the list read-only, so that I cannot stop a task or open its output by accident.
36. As a person on the TUI, I want the "+bg" badge and the paused-ticket card unchanged, so that nothing I rely on there moves.

## Implementation Decisions

**Wire and web**
- The chat read response carries one ordered task list for the session. It replaces the old background list and the in-flight tool list for the web. Each entry has the kind (shell, monitor, subagent, foreground call), the label, `started_at`, the finish time and outcome when known, and the kind's detail fields. The read returns only the open group's tasks plus a running persistent Monitor row, so an entry carries no group field. The exact field names are the implementer's choice.
- Detail fields per kind. Shell: output size, last output line, exit code, command. Foreground call: command, exit code. Subagent: model, token total, tool-call count, current call. Monitor: watched command, event count, last-event time. Codex rows carry only the fields the rollout has: label, time, command, exit code where the end line has it.
- The browser derives the summary ("N of M tasks done", progress bar, "longest") and the row order from the list. "Longest" is the longest elapsed time among running tasks. M counts every listed task except a running persistent Monitor, which is also left out of N, the bar and "longest". The bar is done over total. A persistent Monitor row shows after the group's rows, or alone.
- The browser ticks elapsed time every second from `started_at` for running rows. Finished rows show a fixed duration (finish minus start). The tick re-renders only; it sends no request.
- The component replaces `BackgroundStrip` and the Working row's "waiting on N" text. The list holds foreground shell and Agent calls only. The Working row keeps the in-flight label of the newest other foreground call (for example "Working · Edit store.go"), as in the mock, so one-row groups do not flicker on every Read or Edit. Older Codex versions keep today's full in-flight text. Both pages get the list through the shared chat component.
- Look follows the approved mock (see Mock states). The summary line is closed by default. One task alone renders as a row with no summary line. A failed row's time reads "exit N · duration" and a killed row's reads "killed · duration", both with the cross. A task finished with no outcome shows a muted neutral mark that is neither a check nor a cross, and no outcome text. The implementer picks the glyph.
- Command highlighting covers bash and python only, with a cap of about five lines and "Show all (N lines)". The language is the implementer's call from the command text (python for a `python -c` or heredoc script, otherwise bash).
- Read-only: no stop button, no output link.

**Groups and states (derived, no stored state)**
- A group is derived from the task start and finish times on every read. It starts when a task starts while no other task runs. It ends when its last task finishes. Finished tasks of the open group stay listed. When the group ends the list clears.
- A persistent Monitor does not hold a group open and does not count as "something running" for the group-start rule. With only a persistent Monitor left, its running row stays listed on its own (no summary line). It stays until the agent stops it, or until its session's agent process has exited: then every task of that session shows as finished with no outcome. How a persistent Monitor looks and ends is not yet known (research ticket 08); this section is amended when it closes.
- A running subagent holds the group open. Its own background tasks are not listed or counted.
- Finish time and duration come from the transcript. Claude background task: the transcript time of the matching `<task-notification>` line. Foreground call: the time of its `tool_result` line. Duration is finish minus start. The output file's exit trailer is used for the exit code and output details, never for time.
- No recorded end, one rule for both harnesses. A foreground call with no result shows as finished with no outcome when the turn ends, with its duration measured to the turn end. Every other task (Claude or Codex) with no matched end stays running until the age-out cap for outstanding tasks, then shows as finished with no outcome and no duration. Exceptions: a Claude shell whose output file has the exit trailer (`[exited with code N]`, `[killed]`) counts as finished even with no notification, with the outcome from the trailer and the finish time from the file's change time. A persistent Monitor never ages out. Reason: finishing a task that still runs would state a falsehood, and Codex unified-exec processes outlive their cell and the turn. This deviates from map tickets 04 and 06, which finished such tasks at turn end.
- Claude end lines: only a final notification with a `<status>`, or a `TaskStop` result matched by its structured task id, ends a task. The finish time is the `enqueue` line time of that notification. A Monitor event notification and a subagent interim notification carry the task id but only count; they never end it. `killed` and `stopped` show the cross with the text "killed".
- The age-out and gating of the shared outstanding-task reader (used by ralph-loop finish gating, `harness/claudepid.go` and `backgroundtask`) do not change. The persistent Monitor exemption applies only to the list.
- Outcome: success, failure and killed come from the notification status, the exit code or the `is_error` flag of a `tool_result`. Failed and killed rows show the cross.

**Claude reader**
- Start time is the transcript time of the launching `tool_use` line. This holds for a call moved to the background on timeout. The late result line is never used for start. A subagent starts at its Agent launch `tool_use` line. A foreground (sync) Agent call also shows as a subagent row, with label and time only until a parent-transcript line names its agent id, when its details line appears. A Monitor starts at its `tool_use` line.
- A shell's output file is found by the exact path in the launching result or notification, never a glob on the session id (the directory can carry the id the task launched under, which differs after a resume).
- A shell row reads its output file for size, last line and the exit trailer (`[exited with code N]`, `[killed]`).
- A Monitor row has no output file. Its event count is the number of its notification lines. The row shows no output size.
- A subagent row reads the subagent transcript at the exact path from the notification or result. It takes the model, the token total (usage counted once per unique message id, since parallel calls repeat it), the tool-call count and the call in flight. When the final notification carries the totals, they win.
- The subagent transcripts are sidechain files that the current readers skip. A new reader reads them; the existing ones keep skipping.
- Parent-derived data (starts, finishes, notifications, results) comes from the parse the chat read already does. It adds no second parse of the parent transcript.

**Codex reader**
- Source: the session rollout only. No app-server connection and no process-tree check.
- Codex 0.144.1 and later (read from the rollout's own recorded Codex version) get the full list from the `exec` / `wait` shape. Older versions keep today's in-flight labels and get no list.
- A row starts at the timestamp of its tool call line. A yielded `exec` cell that returns no `session_id` finishes at its `wait` poll's "Script completed" line or at the `item_completed` line where the version has one (0.147.0 and later), whichever is earlier. Exit code and duration show only where an end line carries them.
- Up to a few commands overlap, so a start is matched to its end by cell or process id where it exists, and otherwise by command text and time.
- A command that returns a `session_id` is still alive: it ends at the `write_stdin` poll output carrying its `exit_code`, or at `item_completed`. A Codex subagent ends at its `SubAgentActivity` `completed` event or a `wait_agent` output with `completed`.
- A subagent row reads the child rollout for its model and token total.
- The in-flight reader learns the new shape so that the Working row and the list agree.

**Cost**
- Every per-file task reader (shell output file, subagent transcript, Codex child rollout) goes through the existing size-and-mtime file cache. A finished task's file is not parsed again.
- There is no timer, watcher or background read. Task files are read only inside a chat read.

**Unchanged**
- The TUI Agents tab "+bg" badge, the ticket detail paused-ticket background-task card and its process match, and the glossary terms **Background task** and **Task group** keep their meaning.

## Testing Decisions

- A good test checks external behaviour: what the chat read returns, what the page shows, how many parses happened. It does not check internals or timing. No timing assertions and no benchmarks in CI.
- Highest seam, server: the chat read for a session, driven by fixture transcripts and rollouts. It is where list, groups, times and outcomes are asserted for Claude and Codex.
- Cost seam: the file cache with an injected counting compute function. Prior art: the existing test that checks the cache reuses a value until the file changes. Assertions:
  - Two chat reads of unchanged files parse each task file once.
  - After one file grows, only that file parses again.
  - A finished task's file is not parsed again.
  - With 50 tasks, an unchanged chat read parses nothing beyond the cache's stat calls.
  - With no chat read, no task file is read.
- Reader fixtures cover each awkward case: a call moved to the background on timeout (start from the `tool_use` line), a resumed session (exact path), a persistent Monitor, a subagent with parallel calls sharing a message id, a task with no recorded end, an old Codex shape, a Codex shape with and without `item_completed`, and a Codex subagent.
- Web seam: the shared chat component with fixture responses, for the summary line, row order, marks, the single-task case, the Monitor and subagent detail lines, the command cap and the phone width. A web test checks that the one-second tick re-renders and sends no request.
- E2E: one flow in the existing catalog style on a new fixture scenario for the list (the Design page and the Agents page both), at 1280px and 390px. The flow budget rules of the e2e skill apply.
- Prior art for the web e2e: the Working row flow on the `design-working` scenario and the in-flight popover checks on `agents-tools`.

## Verification

Entry N verifies story N unless a heading names a group. Flow names are catalog flows; "new" items become implementation tickets. The new fixture scenario is called `bg-progress` here; the implementer picks the final name and its shapes (Claude, Codex, old Codex, single task, only a persistent Monitor, no-end tasks, 50 tasks). "Chat read" is the session's chat read route on the temporary server, called with the session id of a fixture session.

1. New flow on `bg-progress` (Claude shape): a running shell's row shows an elapsed time that grows. Chat read: the shell entry has `started_at` equal to its launching line time.
2. Same flow: a running subagent row shows an elapsed time that grows.
3. Same flow: the Monitor row shows an elapsed time that grows.
4. Same flow: a foreground call in flight is a row in the same list. The old "running shell: ..." text is gone from the Working row. Existing flow "75. Read a session" and "14. Working row" are updated for the removed text.
5. Mock states "collapsed" and "expanded": the summary line reads "N of M tasks done". Chat read: the list has M entries, N finished.
6. Mock state "collapsed": the list is closed on first load.
7. Mock state "expanded": a click on the summary line opens the rows.
8. Mock state "expanded": each row has the status, kind, label and elapsed parts.
9. Mock state "expanded": a running row shows the pulsing dot, a successful one the check, a failed one the cross.
10. Mock state "expanded": running rows come before finished rows, and finished rows are dimmed.
11. New flow: a task finishes while another runs; its row stays in the list with a fixed duration and its outcome.
12. New flow: the last task finishes and the list clears. Chat read: no tasks.
13. New flow: after a group clears, two tasks start and a new group opens with "0 of 2" counted anew. A separate one-task case shows no summary line.
14. Mock state "expanded": the failed shell row reads "exit 2 · 12s" and a killed row reads "killed · 12s". Chat read: the failed entry has exit code 2.
15. Mock state "expanded": the expanded shell row shows the output size and last line. New fixture: an output file that grows between two reads and the details line follows.
16. Mock state "expanded": the command shows lightly highlighted, bash and python.
17. Mock state "expanded": a 16-line command shows five lines and "Show all (16 lines)", which expands to all lines.
18. Mock state "expanded": the subagent row details line reads model, tokens, tool calls and current call. Chat read: the same fields.
19. New flow: a fixture subagent with its own background task. The list has one subagent row and "N of M" does not count the inner task.
20. Same flow: with only the subagent running, the group stays open, then clears when the subagent finishes.
21. Mock state "expanded": the Monitor row reads the watched command, "N events" and last-event age.
22. New flow: a persistent Monitor with every other task finished. The group clears and the Monitor row stays alone, running, without a summary line.
23. New fixture: a shell moved to the background after a timeout. Chat read: `started_at` is the launching line time, not the result line time.
24. New fixture: a foreground call with no result and a finished turn. The row is finished with a neutral mark and no outcome text. Chat read: no outcome, duration to the turn end. Same fixture group: a Claude background task past the age-out cap, a Codex task with no matched end that stays running, and a Claude shell whose output file has the exit trailer but whose notification is missing (finished, outcome from the trailer).
25. Mock state "codex-fewer-fields": the Codex rows show label, time and exit code only where known. Chat read on a fixture Codex 0.144.1 or later rollout: the same list.
26. New fixture: a Codex rollout from a version before 0.144.1. Chat read: no list entries. The Working row keeps today's in-flight label (existing flow "75. Read a session" shape on Codex).
27. New fixture: a Codex subagent with a child rollout. Chat read: the entry has model and token total.
28. New flow: the same fixture session opened on the Design page and on the Agents page shows the same rows and summary.
29. Mock state "expanded-phone": at 360px the list fits with no sideways scroll. The existing phone flow's no-sideways-scroll check runs at 390px with the list open.
30. Mock state "single-task": one row, no summary line.
31. New web test and flow step: over 3 seconds the elapsed time moves and the browser sends no request (the existing "no `/v1` request" check style).
32. Server test (Go): with no chat read, no task file is opened. CLI check: `go test` for the package passes (run with the process cap script).
33. Server test (Go): two chat reads of unchanged files parse each task file once; after one file grows only it re-parses; with 50 tasks an unchanged read parses nothing beyond stats.
34. Same server test: a finished task's file is not parsed on later reads.
35. Mock state "expanded": the rows have no stop button and no output link.
36. TUI Agents tab (`gx` TUI or its existing e2e): the "+bg" badge shows for a session with an outstanding task. Ticket detail page flow: the paused-ticket card is unchanged.

**Mock states**

- `$(gx tickets root)/gx/bg-progress/mocks/03-task-list-look.html`, state `collapsed`, 640px, fixture scenario `bg-progress` (Claude shape, 3 of 7 tasks done); parts: working-row, summary.
- `$(gx tickets root)/gx/bg-progress/mocks/03-task-list-look.html`, state `expanded`, 640px, fixture scenario `bg-progress` (Claude shape, 3 of 7 tasks done, one row open); parts: working-row, summary, task-list, task-row, status, kind, label, elapsed, details, command.
- `$(gx tickets root)/gx/bg-progress/mocks/03-task-list-look.html`, state `expanded-phone`, 360px, fixture scenario `bg-progress` (Claude shape, 3 of 7 tasks done); parts: working-row, summary, task-list, task-row, status, kind, label, elapsed, details, command.
- `$(gx tickets root)/gx/bg-progress/mocks/03-task-list-look.html`, state `codex-fewer-fields`, 640px, fixture scenario `bg-progress` (Codex shape, 1 of 3 tasks done); parts: working-row, summary, task-list, task-row, status, kind, label, elapsed, details.
- `$(gx tickets root)/gx/bg-progress/mocks/03-task-list-look.html`, state `single-task`, 640px, fixture scenario `bg-progress` (single-task shape); parts: working-row, task-list, task-row, status, kind, label, elapsed, details, command.

New scenarios to write: `bg-progress` with the shapes named above, for the Claude fixture transcripts (shell, Monitor, persistent Monitor, subagent, foreground call, timed-out call, no-end call) and the Codex fixture rollouts (new shape with and without `item_completed`, old shape, subagent). These become implementation tickets.

## Self-decided design questions

- Persistent Monitor in the counts: left out of N, M, the bar, "longest" and the group-start rule. Reason: it never ends, so counting it would stop the bar filling and the group clearing.
- Codex and Claude no-end rules: one shared rule. Reason: Codex processes outlive the turn, so ending them at turn end would be wrong.

## Out of Scope

- The TUI Agents tab (it keeps "+bg") and the ticket detail paused-ticket background-task card.
- Actions on a task: stop, open its full output. These may be a follow-up.
- Any polling, watching or reading for a session that has no chat open.
- A Codex app-server connection and a process-tree check. Codex versions before 0.144.1 get no list.
- Nesting a subagent's own tasks or tool calls under its row.

## Further Notes

- Why rollout and transcript only: it keeps the cost model simple (read on chat read, cache by size and change time) and needs nothing from Codex's experimental app-server.
- Why start times come from the launching line: the result line of a call moved to the background arrives late by the timeout, so using it would make the task look younger than it is.
- A transcript line's time can occasionally be earlier than the process start. The list uses transcript time throughout, so rows stay consistent with each other.
- Research behind this spec: the Claude and Codex task facts on branches `research/bg-progress-claude-task-facts` and `research/bg-progress-codex-task-facts`.
