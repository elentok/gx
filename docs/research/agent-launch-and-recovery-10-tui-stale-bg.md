# gx:agent-launch-and-recovery/10 - Why the TUI Agents tab kept `working +bg` after every subagent was done

Question: which signal feeds `+bg` for a Claude PTY session, is it ever checked against the transcript probe, and
when can it stay set after the tasks finish? What are the options for deriving `Background`, and what does each
get wrong? Which web-ui row showed it?

Sources: the gx code at this branch (8f2a9bfe0), ticket 10, and
docs/research/agent-launch-and-recovery-08-background-task-signal.md (note 08). I did not run the probe on the
ticket's transcript (`5d38fd33-...`), did not read Claude's source, and did not launch a live agent (see "Not
verified").

## Summary

- `+bg` for an interactive Claude session comes from Claude's own pid-file status `shell`, and only from it. The
  watcher maps `shell` to `Working, Background: true` (harness/claudepid.go:476-477). The transcript probe is not
  read on this path at all. So the ticket's suggested fix ("recompute on every status tick") is already true:
  the pid file is re-read every 100ms (:41, :194-250). Recomputing from the same source cannot clear it.
- The transcript probe (`ReadBackgroundTasks`) feeds `Background` in one place only: the PTY runner's Idle case
  (nativerunner/pty.go:300-304, via nativerunner/pty_turn.go:117-128). That is the ralph-loop runner, not the
  TUI's session list. The TUI reads the holder's watch stream (server/sessions/sessionwatch.go:140-155) through
  `SessionStatusOf` (server/sessions/sessions.go:49-50), which copies `harness.Status.Background` as is.
- The ticket's own evidence fits one cause: gx reported what Claude reported. Claude's own UI showed the same
  subagent as running while `ListAgents` said `completed`, so Claude's task registry was stale. The pid file `shell`
  status is Claude's write of "idle with a background task running" (harness/claudepid.go:21), so a stale registry
  most likely keeps writing `shell`. That is an inference from the two stale views, not something I could confirm
  without Claude's source.
- gx adds three ways to keep a stale `shell` on screen, none of them a bug on their own (section 2). The one that
  matters: when the pid file disappears or is half-written, `poll` keeps the last status (claudepid.go:209-216,
  :253-262), and there is no age or cap on `shell`.
- Note 08 already settles where each consumer decides "live", the Codex difference and a shared `Live` shape. It does
  not settle the choice for `+bg`. This note adds the trace, the staleness paths and the option table.
- Recommendation: option C, pid file with the transcript as a veto in one direction only (section 4). Which
  web-ui row showed `+bg` cannot be told from code or from any state I could read (section 5).

## 1. What note 08 already settles (not repeated)

- The five places that decide "a task is live", the table at note 08 section 1 (lines 67-81), including the `+bg`
  row. It already corrects the ticket's stale references: `server/sessions.go:101` is now
  `server/sessions/sessions.go:49-50`, and `ui/agents/terminal.go:102` is now `ui/agents/agentstyle/agentstyle.go:82`.
- That `+bg` has no transcript cross-check on the Working path, no cap, and the Codex `Ready`-title variant
  (note 08 lines 83-110).
- That the transcript probe has its own hole (TaskStop echo resolves the wrong task) and can under-report, which is
  why "trust the probe over the pid file" is not free (note 08 section 2, lines 113-160).
- A `Live` type with `Source` and `Disagree` fields (note 08 section 4, lines 198-230), and the facts-only
  transcript-vs-pid-file tradeoff (lines 251-268). It left "which wins, and should the TUI show the disagreement"
  open (note 08 "Design questions", line 327). This note answers that one.
- Not settled by note 08, and covered below: the exact staleness paths of the pid-file signal, what happens when
  the two sources disagree in each direction for each consumer, and the evidence check on the web-ui rows.

## 2. The `+bg` path, step by step

1. Claude writes a pid status file (`status`: `busy`, `shell`, `idle`, `waiting`). `shell` is documented in gx as
   "idle with a background task running" (harness/claudepid.go:21-22).
2. `PidWatcher.poll` re-reads it every 100ms (claudepid.go:41, :209-250). It stores the status and calls `changed()`
   only when the status or session id differs (:225-227, :247-249).
3. `PidWatcher.Status` (:452-488) picks the source:
   - Claude's OSC 7501 program report wins when present (`working` gives Working, `done` gives Idle) via
     `programStatus` (:559-578).
   - Exception: OSC `done` plus a pid file that last said `shell` falls back to the pid file (:457-459). The code
     comment says why: OSC `done` cannot say a background shell outlives the turn.
   - With no OSC report, the pid file decides: `shell` gives `Working, Background: true` (:476-477).
4. The holder streams that status. `sessionwatch` runs it through `SessionStatusOf`
   (server/sessions/sessions.go:49-50, `Background: st.Background`) and stores it in the session meta
   (server/sessions/sessionwatch.go:140-155).
5. The TUI shows `+bg` when `State == Working && Background` in the sidebar (ui/agents/sidebar.go:230-231) and
   the status label (ui/agents/agentstyle/agentstyle.go:81-83).

Note the sort of staleness this path has, in order of how likely each is the cause here:

- Claude's own `shell` is stale. Claude Code's UI showed a subagent as running that `ListAgents` said had
  completed (ticket 10 Evidence). If the pid file's `shell` comes from the same registry, gx faithfully shows
  Claude's wrong answer. gx cannot see inside Claude, but it can see the transcript, which is where the probe said
  "no task".
- The pid file stops changing and gx keeps the last read. `poll` returns without touching state when the file is
  missing, unparseable, or older than the spawn (`f.StartedAt < w.notBefore`, claudepid.go:253-262, :209-216, comment
  "keeps the last status when the file is gone or half-written: claude deletes it on a clean exit and rewrites it in
  place"). A session whose Claude was replaced or whose file was rewritten wrongly would keep `shell`.
  Also `Hold()` freezes the status until `Release()` (:582-592), but that is a ralph-loop tool, not a TUI path.
- No task identity and no age. `shell` carries no task list and no start time, so nothing in gx can age it out. The
  transcript probe ages out at 2h (transcript/background_task.go:43); the pid-file path has no equivalent (note 08
  line 87).
- OSC and pid file disagreeing for a turn. The OSC `done` override applies only when the pid file last said `shell`
  (:457). If the OSC report stays `working`, `Background` is never set from the pid file; so the reverse mix
  (OSC says `done`, pid file says `shell`) is the one that produces `working +bg`. That mix is exactly a
  settled turn with a registered task, so it is consistent with Claude's registry being the stale party.

`Background` is not read anywhere to repair itself: `firstprompt.go:407` and `sessionwatch.go:149-152` consume it, and
neither compares it with the transcript.

## 3. Failure directions that matter

False `+bg` (flag set, no task): the user sees `working +bg` on a finished session and may wait or distrust the
board. In the finish wait it would hold an agent whose work is done. Today that is limited: the PTY runner's `shell`
case maps to Working with no `Background` (nativerunner/pty.go:289-307), and `Wait` does not return while the pid file
says `shell` (note 08 line 79). So a stale `shell` also keeps ticket 07's finish wait from ever seeing idle, which is
a worse form of the same bug for ralph-loop runs: the iteration waits until a guard (verify cap, long ticket) ends it.
Ticket 10 only saw it on the TUI, but the same stale `shell` would hold a ralph run.

False no-bg (flag clear, task running): the agent looks idle, so `Wait` returns, the stall nudge fires, and the
zero-commit park can run on a live agent. That is ticket 08's incident (note 08 section 2). It is the more costly
direction for ralph-loop consumers, and the probe's known hole (TaskStop echo) produces it.

So any rule that lets the transcript clear a pid-file `shell` risks reintroducing ticket 08's false no-bg wherever
the probe is wrong. Any rule that lets the pid file alone stand keeps ticket 10's false `+bg`.

## 4. Options

| Option | Code it touches | Failure |
| --- | --- | --- |
| A. Transcript probe alone | `harness.PidWatcher.Status` would need a transcript reader. `PidWatcher` already finds the transcript and `stat`s it for rate limits (claudepid.go:389-428), so it has the path and a size check. It would call `transcript.ReadBackgroundTasks` and set `Background` from `outstanding-fresh` markers; `pidFileStatus` `shell` would map to plain Working or Idle. Also `server/sessions/sessions.go:49-50` if it moves there. The PTY runner's Idle case stays (pty.go:300-304). | False no-bg where the probe is wrong: the TaskStop echo (note 08), a start shape it does not know, a Codex session (no transcript). The probe does not know a Claude-side task that never wrote a marker. For the TUI that is a missing label; for ralph consumers sharing the same function it reopens ticket 08. It would also make `shell` stop keeping `Wait` from returning, which changes the finish wait. Cost: a file read per status change. |
| B. Pid file alone (today) | Nothing. | False `+bg` when Claude's registry is stale (this ticket), and no cap or identity (no per-task age). Cheapest, no new reads, and Claude's own state. Cannot support ticket 07's report-aware rule (no start times). |
| C. Pid file, with the transcript able to override only `shell` to not-live | `PidWatcher.Status` :476-477 only: when the pid file says `shell`, ask the probe; if the probe finds no `outstanding-fresh` marker and `AwaitingTurnEnd` is false, report Idle with no `Background`. Reuse `claudeui.BackgroundTasks` (agentrunner/claudeui/background_tasks.go:22-55) rather than a second reader. Keep a log line when they disagree. Never let the probe set `Background` when the pid file says `idle`/`busy` (the PTY runner does the opposite, pty.go:300-304, and keeps doing it). | False no-bg wherever the probe wrongly resolves a live task (ticket 08 mechanism): the probe then clears a true `shell`. That is why this option needs ticket 08's structured `task_id` fix first, or it moves ticket 08's bug into the TUI. False `+bg` is cut for cases where the transcript shows all tasks resolved, which is ticket 10's evidence (probe said none). Also an unreadable transcript (error, no unique path) must keep the pid file's answer, the reverse of the error policy elsewhere (note 08 line 91-93). |
| D. Show the disagreement in the TUI | Add a third value to `wire.SessionStatus` (e.g. `Background` plus a `BackgroundDisputed` or a source), computed where C computes it; render in sidebar.go:230-231 and agentstyle.go:81-83 (for example `+bg?`). The web UI reads the same wire status. | No new false negatives (the flag stays whatever the pid file says), but it adds a wire field, a render case in two UIs and tests in each. It explains the state without fixing it, and a user still waits on a stale shell. Good as an addition to C for the cases where the probe is not trusted, not as a replacement. |

The disagreement in the other direction (probe live, pid file `idle`) is handled today only in the PTY runner,
which trusts the transcript (pty.go:300-304). `+bg` for the TUI ignores it. Option C leaves that as is.

### Recommendation (not a decision)

Option C, behind ticket 08's `task_id` fix, with the disagreement logged and no TUI change at first. Reasons:

- The one evidence case is "probe says none, pid file says `shell`, Claude's own UI also stale". C clears exactly
  that and nothing else, and it keeps Claude's own signal as the primary for every other case (including Codex,
  where there is no transcript).
- It lets the probe veto only toward "not live". That is the direction ticket 10 is about. The probe's known
  failure (a wrong resolve) lands on the same direction, so C should not ship before the structured `task_id` fix.
  Order: ticket 08's resolve fix, then C. If that order cannot hold, B plus D (show the dispute, change nothing) is
  the safe stopgap.
- A alone drops the pid file's knowledge of tasks the transcript cannot see, and it would stop `shell` from holding
  `Wait` in the PTY runner. That interacts with ticket 07's finish wait, so it should be a separate decision.
- The same one-reader change is what note 08's `Live` shape would replace later, so C does not block that
  refactor: it is one call site.

Test seam to ask for: a `PidWatcher` fed a pid file `shell`, a transcript with every marker resolved, and the same
with one fresh marker. Both assert `Status().Background`. (The seams belong to the ticket's own approval; I only name
the likely one.)

## 5. Which web-ui row showed it

I cannot tell from code, and I found no evidence in the local state I could read. Checks:

- `~/.local/share/gx/sessions/*/meta.json` for `web-ui`: no match. Sessions there are named by id; none is `b6ee5c`
  or `ee9878`, and no file under `~/.local/state/gx` mentions either id. (Ticket 10 was filed 2026-10-20 and the
  session dirs have since rotated; they are only the 17 live ones.)
- `gx server` has no read verb for session status history. `gx server agents` and `iterations` list ralph-loop
  agents, not interactive Design sessions, and the status is overwritten on each change, with no log of `Background`.

So the question for the user stays: which row showed `+bg`. Only `ee9878` (idle, started 8 days earlier) showing `+bg`
would make it a non-bug, and only if that session really had a live shell. If it was `b6ee5c`, the cause is the
section 2 path. The ticket's transcript is `5d38fd33`; if the session id of `b6ee5c` is that, the row is settled
(`gx server` session meta for `b6ee5c` would carry the conversation id, but the dir is gone).

## Not verified

- That Claude's pid file writes `shell` for a finished or completed async subagent, and whether its `shell`
  clears when the subagent's own registry entry is stale. Same gap as note 08 (its "Not verified"). The inference
  from Claude's own stale UI is the only support.
- I did not run `ReadBackgroundTasks` on `5d38fd33-...`; the ticket's run is the only evidence that the probe
  said no task.
- Whether the OSC 7501 `done` plus pid `shell` mix is what the TUI showed that day. The code says it is the only path
  that yields `working +bg` with the transcript clean, but I did not see the session's raw log (gone).
- The holder-side wiring from `PidWatcher.Status` to `StreamStatus` was read through `sessionwatch.go:140-155` and
  `SessionStatusOf`, not traced in `holder/`.
