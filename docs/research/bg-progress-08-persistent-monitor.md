# gx:bg-progress/08 - How a persistent Monitor looks in the transcript and how it ends

Question: research 01 saw only `persistent: false` Monitors. What does a `persistent: true` Monitor write, what
ends it, what does a dead session leave behind, and can gx tell the agent process has exited?

Sources:
- Claude Code 2.1.325 (`claude --version`). Observed behaviour, not documented contract.
- Research 01 (`docs/research/bg-progress-01-claude-task-facts.md` on `research/bg-progress-claude-task-facts`),
  section 3.
- A grep of every `*.jsonl` under `~/.claude/projects`.
- Three live probes, `claude -p --model haiku` in throwaway cwds under the session scratchpad (probes A, B, C below).
  Every process started was killed or exited. Probe A's and B's session ids: `c787f4bc-2497-4830-9d81-7660d0cf99c5`,
  `31a2064c-1c22-4dd3-a7ae-8ef7a18357b9`.
- gx code at origin/main: `harness/claudepid.go`, `harness/claude.go`, `harness/claudeaskhook.go`,
  `backgroundtask/stop.go`.

## Headline

**No `persistent: true` Monitor exists to observe on this version, in real transcripts or live.**

- Real transcripts: of all `"persistent":...` values in `~/.claude/projects`, 726 are the string `"false"` (Monitor
  `tool_use` input) and 365 are boolean `false` (`toolUseResult`). Zero are `true` or `"true"`. Searched with
  `rg -o '"persistent":[^,}]{1,8}' --glob '*.jsonl' | sort | uniq -c`.
- Live: the Monitor tool schema in 2.1.325 has no `persistent` property (section 1). A model that is told to pass
  it can send it, but Claude Code ignores it. The Monitor then expires like any other.

So the `persistent` flag in gx's data is always false, and a Monitor's lifetime is always bounded by `timeout_ms`.

## 1. The Monitor schema in 2.1.325

Loaded with the deferred-tool search in this session. Parameters:

- `command` (string), `description` (string, required), `timeout_ms` (number, required, default 300000, `minimum`
  1000, `maximum` 3600000), `ws` ({url, protocols}). `additionalProperties: false`. No `persistent`.
- Tool text: "Every monitor expires after `timeout_ms` (default 5 minutes, at most 30 minutes): it is killed and you get
  one notice with the event count." and "Deadlines above 1800000ms are capped to 1800000ms."
- The start result text says the same: `Monitor started (task b67v4e3e3, expires in 10m unless the source ends first;
  you get one notice at expiry - re-arm if you still need the watch)`.
- In probe A the haiku model refused to send the flag: "I also left out `persistent=true` because the Monitor schema
  has no such field". Older transcripts still carry the key as a string (`"persistent":"false"`, research 01 line 119
  sample), so an older tool version had the field. Which version dropped it was not determined.

## 2. Probe B: `persistent: true` sent anyway

Prompt told the model to include `persistent:true` literally. Stream-json output of the session:

- `tool_use` input: `{"description":"probe08b","command":"echo hi; sleep 600","timeout_ms":60000,"persistent":"true"}`.
  The model wrote it as the string `"true"`, and the call was accepted, not rejected as an unknown key.
- `tool_result` text: `Monitor started (task bpoun7csx, expires in 1m unless the source ends first; ...)`.
  It says "expires in 1m", the `timeout_ms` given. No persistent wording.
- `toolUseResult` in the transcript: `{"taskId":"bpoun7csx","timeoutMs":60000,"persistent":false}`. The `true`
  sent in the input came back as `false`.
- So `persistent: true` in `input` does not produce `persistent: true` in `toolUseResult`. `timeoutMs` is still set,
  so `ReadBackgroundTasks` (`transcript/background_task.go:378`, `TimeoutMs != nil`) still recognises it as a Monitor.

## 3. What a running Monitor writes (probes A, B, C)

Same shapes as research 01 section 3, confirmed on 2.1.325:

- Event notification, in the main transcript as a `queue-operation` `enqueue` line, then `dequeue` (or `remove` with
  `reason":"absorbed_mid_turn"`), then a `user` line or an `attachment` (`queued_command`, `commandMode":"task-notification"`,
  `origin":{"kind":"task-notification","producer":"session-task"}`) line:
  `<task-notification>\n<task-id>b67v4e3e3</task-id>\n<summary>Monitor event: "probe08 ticker"</summary>\n<event>tick 3</event>\n</task-notification>`.
  No `<status>`, no `<tool-use-id>`, no `<output-file>` on event notices.
- `-p` stays alive while a Monitor runs. In probe A each tick re-woke the model for a new turn (one `system`
  `init` plus an `assistant` text and a `result` per tick), until the 150 s `timeout` killed the process.
- Output file `/private/tmp/claude-501/<project-dir>/<session>/tasks/<taskId>.output` held the stdout lines.
- The stream-json output (not the transcript) also carries `system` lines `background_tasks_changed`
  (`tasks:[{task_id, run_id, task_type:"local_bash", description}]`), `task_started` (`is_backgrounded:true`,
  `task_type:"local_bash"`) and `task_updated` (`patch:{status:"killed", end_time}`). A Monitor's `task_type` is
  `local_bash`, the same as a background shell. These lines are not in the on-disk `.jsonl`.

## 4. What ends a Monitor, and what each case writes

| Ending | Final notification in transcript | Output file tail | Process |
| --- | --- | --- | --- |
| Command exits (research 01) | `<status>completed</status>` `stream ended`, with `<tool-use-id>`, `<output-file>` | `[exited with code N]` | gone |
| `timeout_ms` expiry | not probed here (research 01: one notice at expiry) | - | killed |
| TaskStop (probe B) | **none** in the on-disk transcript. `rg '<status>'` on the transcript found no line. The stream-json showed `task_updated status:killed` and `task_notification status:"stopped"` | `hi\n\n[killed]` | gone, no leftover `sleep` |
| Graceful session exit, SIGTERM (probe A, `timeout` sent SIGTERM at 150 s) | **yes**, one `queue-operation` `enqueue` line written at exit, after the last `cost-state` line: `<task-notification><task-id>b67v4e3e3</task-id><tool-use-id>toolu_01Ebcp7hEXiRBx4NE4eUxEF2</tool-use-id><output-file>.../tasks/b67v4e3e3.output</output-file><status>killed</status><summary>Monitor "probe08 ticker" stopped</summary></task-notification>`. No `dequeue` or `user` line follows | `tick 1 ... tick 6\n\n[killed]` | gone, no leftover `sleep 600` |
| Crash, SIGKILL of claude (probe C) | **none** | `hi` only, no `[exited ...]` or `[killed]` line | **survives**, see section 5 |

Notes:
- TaskStop's own `tool_result` is `{"message":"Successfully stopped task: bpoun7csx (echo hi; sleep 600)","task_id":"bpoun7csx","task_type":"local_bash","command":"echo hi; sleep 600"}`.
  The task id is in it, which is how research 01 says `ReadBackgroundTasks` resolves a stop.
- The `status:"killed"` / `stopped` summary shape matches the 5 `stopped` summaries counted in research 01.
- Whether a normal interactive exit (`/exit`, Ctrl-D) writes the same SIGTERM-style `killed` line was not probed.
  Only SIGTERM of `claude -p` was.
- The agent ending its turn does not end a Monitor. In probe A and C the model replied "done" style text and the
  Monitor kept running and kept waking the session.

## 5. What a session killed with SIGKILL leaves behind (probe C)

Setup: `claude -p` (pid 44515) ran Monitor `echo hi; sleep 613`; `kill -9 44515` after 25 s.

- Pid file `~/.claude/sessions/44515.json` stayed, still `"status":"busy"` (full keys seen on a live file:
  `pid, sessionId, cwd, startedAt, procStart, version, kind, entrypoint, status, updatedAt, statusUpdatedAt, ...`).
  It is not deleted on SIGKILL. `updatedAt` stopped changing. Probes A and B (SIGTERM / normal exit) left no file.
- The Monitor's process tree survived as an orphan: `45484 ppid 1 /bin/zsh -c source ...snapshot... eval 'echo hi; sleep 613' ...`
  and `45485 ppid 45484 sleep 613`. Before the kill they were children of 44515 (ppid 44515).
- `kill -0 44515` fails with "no such process" right after the kill.
- Transcript ended on the last `assistant` text line. No notification for the Monitor. Output file `tasks/b5v9rq2h4.output`
  held `hi` and nothing else.
- Cleanup done after the probe: killed 45484 and 45485, deleted the stale `44515.json`.
- A scan of the live `~/.claude/sessions` dir (7 files) found all pids live, so no stale file from a past crash was
  on disk to check.

## 6. How gx can tell the agent process exited: `harness/claudepid.go`

What it does, with lines from `harness/claudepid.go` at origin/main:

- It reads `sessions/<pid>.json` every 100 ms (`pidStatusPoll`, `watch`). It never checks that the pid is alive. No
  `syscall.Kill(pid, 0)`, no `os.FindProcess`, no wait in that file.
- `poll` doc comment: "keeps the last status when the file is gone or half-written: claude deletes it on a clean
  exit and rewrites it in place." When `read` fails and the file was already found, `poll` returns and leaves
  `w.live` as it was. A gone file is not turned into an exit state.
- `read` (reports false for missing, half-written, or `StartedAt < notBefore`) drops a file older than the spawn,
  which handles a crashed claude whose pid was reused. It does not handle a stale file for the watched pid itself.
- Status values the file can hold: `busy`, `shell` (idle with a background task), `idle`, `waiting`, or none
  (`ClaudeStarting`). `shell` shows as `Status{State: Working, Background: true}` (`pidFileStatus`). A SIGKILLed
  claude with a Monitor leaves the last value in the file, so it keeps being read as it was (probe C showed `busy`;
  a `shell` value on the same path was not captured but the file is not rewritten after SIGKILL).
- `pollBackground` (runs only while the file says `shell`) cross-checks the transcript with
  `transcript.ReadBackgroundTasks` and `backgroundtask.Resolve`. It tests "no outstanding task" only, not whether
  claude is alive.
- Liveness checks that do exist elsewhere: `harness/claudeaskhook.go:49` uses
  `errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)` to drop stale hook markers; `backgroundtask/stop.go:22,34` treats
  `ESRCH` as already gone. `processParents` (`ps -A -o pid=,ppid=`) is in `harness/claudepid.go` and used for descendant search.
- `harness/claude.go:45-50`: `Watch(pid, ...)` builds the watcher for the pid gx spawned and runs `go w.watch(stop)`.
  The spawned pid may be a wrapper; the claude pid is found by `findDescendant`, and `w.path` is switched to that
  claude's file. The watcher then keeps the claude pid only inside `w.path` (the number in the file name), not in a field.

Facts about each exit mode against the pid file:

| Exit | `sessions/<pid>.json` | Monitor process | Transcript |
| --- | --- | --- | --- |
| Clean or SIGTERM | deleted | killed | `killed` notice enqueued (SIGTERM case) |
| SIGKILL | stays, last status frozen | orphaned under ppid 1 | no notice |

So, from files alone: a missing pid file means a clean exit; a present file with a dead pid (`kill -0` ESRCH) means a
crash. The file's `pid` field and the file name both carry the pid, and `procStart` (a string like
`Mon Nov 16 07:37:46 2026`) is in the file.

## 7. Open points not probed

- A Monitor reaching `timeout_ms` (the expiry notice's text and its transcript shape): research 01 has only
  non-persistent ones; not re-run here.
- Interactive `/exit` or Ctrl-C of a TUI session with a running Monitor.
- Whether a version older than 2.1.319 sent `persistent: true` through to a real persistent monitor.
- A `ws` source Monitor.
