# gx:agent-launch-and-recovery/13 - Stopping one background task of a live agent

Claude Code 2.1.319, Codex CLI 0.166.0, macOS, 2026-11-07. Real runs: a throwaway interactive `claude` and a
throwaway `codex` in tmux (private socket) in a scratch directory. Every process killed was one the run started.
Transcripts and probe test are under the session scratchpad (`run13*.jsonl`, `probe/`), not in the repo.
`transcript/background_task.go` was copied unchanged into the scratchpad and `ReadBackgroundTasks` run over
the real transcripts.

## Summary

- Verified: `kill -TERM` on a background shell's process, from outside, makes claude write its own
  `<task-notification>` (`status` failed, "failed with exit code 143") and start a new turn on its own. gx
  sends nothing. Same for a Monitor script. The probe then reads the marker as `resolved`, `AwaitingTurnEnd`
  false once the woken turn ends.
- gx has no task-id to pid link. Neither the pid file, the transcript nor the `.output` file holds a pid. gx
  must find the process in claude's descendants and match it by command text.
- An async Agent has no process of its own. "Stop task" for it is not a kill (see below).
- Codex: the background terminal has a real process, but killing it wakes nobody and writes no
  notification-shaped line. Not a "Stop task" candidate for the same spec; Claude only.

## Q1. How gx identifies the task's process

What exists (verified on the run):

- Claude's pid file `~/.claude/sessions/<claude pid>.json` is keyed by claude's own pid, with `status`
  (`shell` while a background shell ran, `idle` after) and no task data (run: pid 68087, `"status":"shell"`).
  gx reads it in `harness/claudepid.go:182` (`<pid>.json`). It is the claude pid, never the task's.
- gx's own owner pids: native runner reads claude (and holder) pid from the agent's `meta.json`
  (`nativerunner/owner.go:13-31`); the herdr runner returns the pane shell pid (`agentrunner/herdrrunner/owner.go:13-40`).
  Native children are `Setsid` (`nativerunner/launch.go:54`, `holder/holder.go:163`), so claude leads its
  own session/group.
- `descendantsOf(shell)` in `agentrunner/herdrrunner/leftovers.go:25-52` walks the process table by ppid from an
  owner pid and keeps `(pid, start time)` pairs; `signalAlive` rechecks both before each signal
  (:68-103). It is used only at `Stop` (herdrrunner.go:550) to reap a whole tab. Reusable for one task: take
  claude's pid, list descendants, pick one.

Process tree seen in the real run (claude 68087 in a tmux pane, no gx wrapper):

```
claude 68087
 +- caffeinate -i -t 300                (claude's own helper, same group as claude)
 +- /bin/zsh -c source <snapshot> ... && eval 'sleep 612' < /dev/null && pwd -P >| /tmp/claude-...-cwd
     (pid 70948, its own process group = its pid)
     +- sleep 612                        (pid 70949, same group)
```

- Each background shell is a direct child of claude, in its own process group. `kill -TERM -<pgid>` takes
  the shell and its children together; `kill -TERM <sleep pid>` works too (both tried).
- The command line of the wrapper shell contains `eval '<the exact command>'`. That is the only link to a
  marker. The task id (`bux2iq67r`) was not in any process argument that I could see.
- Marker `Label` is the launching call's `description` ("Sleep for 612 seconds"), else the first 60 characters
  of the command (`transcript/background_task.go:48-69`), so for a call with a description it does not match
  the process. gx must take the command from the launching `tool_use` input (transcript line before the
  `backgroundTaskId` result) and match `eval '<command>'`. Identical commands in two tasks are ambiguous;
  then the later start time wins or the button must refuse. Unverified: how claude quotes commands with
  single quotes.
- Monitor: the script is also a direct child of claude in its own group (`sleep 615`, pid 76388, wrapper
  pid 76387). Same match rule, with the Monitor's `command`.
- async Agent: no process. The subagent runs inside claude; its Bash tool calls are direct children of
  claude (`sleep 616` ppid was claude 68087), indistinguishable from the parent's tasks except by command text.
  The agent task had finished by itself in 8 s in my run, so I could not test stopping a live one.
  Stopping one needs claude's own TaskStop tool, so an agent prompt, not a signal. Unverified.
- Whole-agent ownership: only claude's descendants count. Matching by cwd is not safe (leftovers.go:22-24).

## Q2. Does killing make claude write its notification and wake itself?

Yes, in both kill shapes. Timeline of run 1 (`kill -TERM <sleep pid>` at 21:59:37, claude idle, pid file
`shell`):

- 21:59:37.646 `queue-operation enqueue` with `<task-id>bux2iq67r</task-id> ... <status>failed</status>
  <summary>Background command "Sleep for 612 seconds" failed with exit code 143</summary>`, 3 ms later `dequeue`.
- 21:59:37.658 a new `api-request`, then a `user` line holding the same `<task-notification>`, then an assistant
  reply ("terminated early with exit code 143") with `stop_reason: end_turn`, 4 s turn.
- Pid file went `shell` to `idle`. The `.output` file holds `[exited with code 143]`.
- I never typed into the pane. The footer ghost text ("run it again") is a suggestion, not input.

Run 2 (`kill -TERM -<pgid>`, task `bt9pgwpq7`): same, `failed with exit code 144`. Run 3 (Monitor `b521gl76c`,
group TERM): `Monitor "sleep 615 then done" script failed (exit 144)`, same wake. Exit codes differ (143 vs 144)
but the status is `failed` every time. SIGKILL alone was not run; the notice comes from claude seeing its
child exit, so it should be the same (unverified). The 2 s SIGTERM-then-SIGKILL grace is only a fallback for a
child that ignores TERM.

## Q3. What the probe reads after the kill

Run over the real transcript with the repo's `ReadBackgroundTasks`:

- Before the notice (first 40 lines): `bux2iq67r` `outstanding-fresh`, `Kind: shell`.
- After: `bux2iq67r` `resolved`, `AwaitingTurnEnd: false` (the woken turn ended in the same transcript).
  Two shells and a Monitor killed in sequence all read `resolved`.
- The resolving line is the `queue-operation` (type on the `reportsOnTasks` allowlist), so the existing probe
  resolves it with no change. `AwaitingTurnEnd` is true only between the notice and the end of the woken turn
  (`background_task.go:136-149`); it stuck nowhere in the run.
- Caveat for the TaskStop echo bug (note 08): that bug resolves the wrong id from a TaskStop result. Stopping
  by signal does not go through TaskStop, so it does not hit that path.

## Q4. Codex

Real run: `codex` in tmux, "Started sleep 617 in background terminal session 69053", footer
"1 background terminal running · /ps to view · /stop to close".

- Process: `/bin/zsh -lc 'sleep 617'` ran as `sleep 617` (pid 79954), child of
  `codex app-server --listen unix://` (pid 78662), itself a child of the pane's codex process (78598). No pid
  in the rollout: its `process_id: "69053"` is codex's own id, not an OS pid
  (`~/.codex/sessions/2026/11/08/rollout-...01a1b809-....jsonl`, line 25).
- `kill -TERM 79954`: footer line "1 background terminal running" disappeared, rollout got one
  `item_completed` with `status: "failed"` for that exec, and no new turn: the agent was not woken, no
  notification text. The probe has no Codex transcript anyway (`transcript` returns `unsupported`).
- The app server is a daemon (`--listen unix://`); another codex process was running a similar one. Unverified:
  whether a second pane reuses it, which would make "descendant of this pane" unsafe for Codex.
- Statement for the spec: Codex has a findable process only by walking descendants and matching command text,
  and killing it would leave the agent unaware. "Stop task" is Claude only. Codex has `/stop` in its own TUI
  (documented in its footer), which is the alternative if a Codex verb is ever wanted: not tested here.

## Q5. What the web "Stop task" verb must send afterwards

- Claude, shell or Monitor, after a kill: nothing. Claude writes the notice and wakes itself (Q2). A prompt
  from gx would arrive on top of the notice turn and double the work.
- Fallback: if the process is not found (already exited, or no match), return "not found" to the web and send
  nothing; the notice either exists already or never will.
- Only if no `task-notification` shows up for that id within a few seconds of the kill (claude hung, or a
  child that survived SIGKILL) should gx send a one-line prompt naming the task. Never observed in three
  runs, so this is a guard, not a requirement.
- async Agent: gx sends a prompt telling the agent to run its TaskStop on the id (no process to kill).
  Unverified.

## Not verified

SIGKILL-only, commands containing quotes, two identical commands, stopping a live async Agent, a
shared Codex app server across panes, and the gx-managed path (agent-holder wrapper, Setsid) which I did not
launch: the holder sits above claude, so claude's own children keep the tree shown above.
