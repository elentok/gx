# gx:agent-launch-and-recovery/14 - Stop task gaps confirmed

Claude Code 2.1.319, Codex CLI 0.166.0 (shared app-server daemon 0.167.0), macOS, 2026-11-07/08. Every case below is a real run.
Claude ran either under `gx agent-holder` (the binary gx uses) or in a private tmux socket, with `env -i` so the
parent session's `CLAUDECODE`/`CLAUDE_CODE_SESSION_ID` did not leak in (a first run that inherited them wrote no
transcript and was discarded). Scratch files: the session scratchpad (`desc/` descendant walker, `probe/` probe test,
`run14-agent-*.jsonl` transcripts), not in the repo. Every process killed was one the run started. Builds on
`docs/research/agent-launch-and-recovery-13-stop-task.md` (called "13" below).

## Summary

- SIGKILL alone works: claude writes its own `<task-notification>` and wakes. Verified.
- Duplicate or quoted commands are not a problem if gx matches by the task's `.output` file, not by command text.
  Verified: each shell task's wrapper process has stdout and stderr open on `.../tasks/<task-id>.output`. This is an
  exact task-id to pid link, which 13 said did not exist. Command-text matching is ambiguous and, for duplicates,
  cannot be fixed by "oldest wins" (shown below).
- The gx holder path works: `descendantsOf` logic finds the tasks from the holder pid. Verified.
- Async Agent: a prompt naming the id makes claude run `TaskStop`; the transcript resolves the marker. Verified.
- Codex: two panes share one app-server daemon that is not a descendant of either pane. A pane's descendants hold no
  background task, and no "Stop task" code exists. Verified.

## 1. SIGKILL alone

Holder run, claude pid 2816, three background shells. Notice timeline:

- `kill -KILL -<pgid of wrapper>` (wrapper and sleep both die, task `b5fgh4g6u`) at 00:12:04: transcript line
  `queue-operation enqueue` at 22:12:04.526 (UTC) with `<status>failed</status>`, summary `... failed with exit code 1`,
  3 ms later `dequeue`, then a `user` line with the same notice and an assistant reply ("failed with exit code 1")
  at 22:12:06. Claude woke by itself; nothing was typed. Pid file `~/.claude/sessions/2816.json` was `status: shell`.
- `kill -KILL <sleep pid only>` (wrapper survives to report, task `b9a6vefga`): same enqueue/wake, summary
  exit code 137 (the assistant said "usually means SIGKILL").

So SIGTERM first is not needed. The status is `failed` in every case, but the exit code in the summary differs
(143 on TERM to the sleep, 144 on group TERM, 137 on KILL to the sleep, 1 on group KILL): gx must not parse it.
The 2 s TERM-then-KILL grace from `herdrrunner/leftovers.go:11,65-76` stays a valid fallback.

## 2. Quoted and duplicate commands

Launched in one prompt: `sleep 711` twice (A, B) and `echo "it's q"; sleep 712` (C). Claude ran each wrapper as
`eval '<command>' < /dev/null && pwd -P >| /tmp/claude-<hex>-cwd`:

```
eval 'sleep 711' < /dev/null && ...
eval 'sleep 711' < /dev/null && ...
eval 'echo "it'"'"'s q"; sleep 712' < /dev/null && ...
```

- Quoting: standard POSIX single-quote escaping, each `'` in the command becomes `'"'"'`. A matcher on command
  text must apply that escape to the launching `tool_use` command before comparing.
- Duplicates: two wrappers carry byte-identical text. Their start times were equal at the 1 s resolution
  `syshealth.Process.StartUnix` offers (`syshealth/process.go:9`), so "oldest" cannot separate them. Pid order
  does not follow launch order either: tool_use order was A, B, C; the lower wrapper pid (3103) was task B
  (`b5fgh4g6u`) and the higher (3109) was task A (`b9a6vefga`). The results also came back out of order
  (B, C, A), so even a transcript-side ordering would be a guess.
- The exact link: `lsof -p <wrapper pid>` shows fd 1 and fd 2 open on
  `/private/tmp/claude-501/<encoded cwd>/<claude session id>/tasks/<task-id>.output`. The pid-to-task mapping
  matched the kill outcome exactly (killing the group of pid 3103 failed `b5fgh4g6u`). The notification itself
  also carries `<task-id>`, `<tool-use-id>` and `<output-file>`, so the transcript has the same path.
  Source of the fd list for the spec: `lsof` proves the data exists; the repo's `syshealth` has no fd reader today
  (`DarwinProcessReader` exposes `Read` and `Argv` only, `syshealth/darwin_proc_cgo.go:49,121`), so this needs
  new code (libproc `PROC_PIDLISTFDS` / `/proc/<pid>/fd`). I did not write it: unverified that it works from
  Go, verified that the data exists.
- Rule that picks the right process: match the wrapper whose stdout fd path ends in `/<task-id>.output`, and kill
  that wrapper's process group. Fallback when the fd read fails: match the command text, and if more than one
  wrapper matches, refuse and report which pids are ambiguous. Never pick by age or pid.
- A foreground Bash inside an async Agent also got a `.output` file with its own task id (`bzuxmow5e`, not the
  agent's id), so the same link finds a bash child of an agent but never the agent itself.

## 3. gx-managed holder path

`gx agent-holder --harness claude --trust -- claude ...` was started (the real binary, own pty, `Setsid` for
claude: `nativerunner/holder/holder.go:163`; the harness it ran was Claude 2.1.319). A copy of `descendantsOf`
(`agentrunner/herdrrunner/leftovers.go:25-52`, same code, `syshealth.DefaultProcess()` for the table) pointed at the
holder pid returned:

```
claude (ppid = holder)
 +- eval 'sleep 711' ...   +- sleep 711
 +- eval 'sleep 711' ...   +- sleep 711
 +- eval 'echo "it'"'"'s q"; sleep 712' ...   +- sleep 712
```

Same tree as the plain tmux pane in 13. Pointing it at claude's own pid (what `nativerunner/owner.go:13-31` also
returns, from `meta.json`) gives the same wrappers minus the claude row. Both owner pids work. Signals from the
parent session to the group and to the bare child worked (section 1). `descendantsOf` itself is a method on the
herdr runner, used only at tab `Stop` (`herdrrunner.go:550`), so a native/holder caller needs a shared
version of it: that is wiring, not a finding about processes.

## 4. Stopping an async Agent task

Prompt used (tmux claude, `--permission-mode auto`), after an async Agent was launched with a foreground
`ping -c 100 127.0.0.1 > /dev/null` (`sleep N` as a standalone foreground command is refused by the tool, which
cost two earlier runs; the first finished before I could stop it, so the third run is the one that counts):

> Use the TaskStop tool with task_id ae750bf9c6b0a9a9b to stop that background agent task. Then end your turn with
> the single word stopped.

Observed in the transcript:

- Claude first ran `ToolSearch select:TaskStop` (TaskStop is a deferred tool), then
  `TaskStop {"task_id":"ae750bf9c6b0a9a9b"}`; result `Successfully stopped task: ae750bf9c6b0a9a9b (Run ping
  command)`, `task_type: local_agent`.
- 12 ms later claude enqueued `<task-notification>` with `<status>killed</status>`, summary `Agent "Run ping
  command" was stopped by Claude`. A note says the same task-id can notify again if the agent is resumed.
- The agent's own child (`ping`, a direct child of claude) was gone afterwards: TaskStop reaps it. No signal was
  needed.
- Probe (`transcript.ReadBackgroundTasks` over the copied transcript, repo `HEAD`): before the stop,
  `ae750bf9c6b0a9a9b` `outstanding-fresh`, `Kind: agent`; after, `resolved`, `AwaitingTurnEnd: false`. The
  existing probe resolves it with no change.
- The wording is a normal user prompt: it needs claude to be idle (gx's existing "type into an Idle session" path).
  The id comes from the marker (`TaskID`). Note 13's caveat still applies: the TaskStop echo bug (note 08) resolves
  by id from the TaskStop result; here the id in the result equals the real id, so no wrong marker was resolved.

## 5. Second Codex pane and the shared app server

Two interactive `codex` panes in different cwds (tmux), each told to start a background `sleep` terminal
(`sleep 731`, `sleep 732`; footer "1 background terminal running" in each):

- Both sleeps were children of the same `codex app-server --listen unix://` (pid 95028, started 00:08:05 by an
  app-server daemon `pid-update-loop`, pid 78699, ppid 1). Neither pane's `codex` process (30781, 28197) had it as
  a descendant. This differs from 13's run, where the app-server was a child of the pane; with the daemon running,
  panes only connect to it.
- The descendant walk from each pane's `codex` pid returned no processes at all. A "kill the task" that walks the
  pane's descendants finds nothing; one that walks the app-server would reach every pane's tasks and, if it
  signalled the app-server itself, would end all of them.
- gx code paths that signal a codex session: holder and runner stop kill claude's/holder's own process group
  (`nativerunner/pty.go:229-234`, `nativerunner/holder/holdersock.go:491-495`); neither touches the daemon, which
  is outside the group. `commandlist/codex.go:42-80` starts its own stdio `codex app-server` child and only kills
  that child.
- There is no Stop task button or kill path today: the only hits for "Stop task" in the repo are research notes and
  `TaskStop` text in skills and `transcript/background_task.go`. The web `SessionBackgroundTask` list
  (`webui/src/generated/server.ts:2245`) is filled by `agentrunner/claudeui/background_tasks.go:35,44` from the
  Claude transcript, and the loop's reader returns early for non-Claude (`ralphloop/supervise/waitforfinish.go:644`),
  so a Codex session has no task rows to hang a button on. The spec must still state "Claude only" and the
  server must refuse a Codex session, since no code gates it today.
- Cleanup: killed only the two sleeps and the tmux server of this run; left daemon 95028 alone (shared, not mine).

## Not verified

- A Go reader of a process's open files (needed for the `.output` link); only `lsof` was run.
- Monitor scripts' fd link (13 only showed Monitor wrappers are direct children of claude): not checked for
  `.output`.
- Stopping an Agent that has live Bash children of its own via a process kill instead of TaskStop: not tried.
