# gx:bg-progress/01 - What Claude Code records live about each running task

Question: while a Claude Code session runs, what can gx read cheaply, without the agent's help, about each task the
agent waits on? Four kinds: background shell, Monitor, async subagent, foreground tool call in flight. Fields
while running and when finished, where each lives, and what a read costs.

Sources:
- Claude Code 2.1.325 (the version running this session). Older transcripts in `~/.claude/projects` carry
  versions down to 2.1.319, and the shapes below held across all of them unless a section says otherwise.
- Live probes run in this session (`6c75ba65-ed46-43ca-93e3-8b9d86bdac80`, project dir
  `-Users-david-dev-gx-bg-progress-design`). The probes were launched from an async subagent, so the probe
  lines sit in `subagents/agent-a7f2ec6cab6cae52b.jsonl`.
- Real transcripts: 6866 main files and 3527 subagent files under `~/.claude/projects`, and 963 regular task
  output files plus 1460 subagent-output symlinks under `/private/tmp/claude-501/*/*/tasks/`.
- gx code at this branch: `transcript/background_task.go`, `transcript/inflight.go`, `backgroundtask/process.go`,
  `backgroundtask/facts.go`, `server/sessions/chat/read.go`.
- The Claude Code binary and its docs were not read. Everything here is observed behaviour, not documented
  contract. Statements marked "observed once" rest on a single probe.

Line numbers below count lines in the named JSONL file, starting at 1.

## 1. Where the data lives

| Place | Path | Written by Claude Code |
| --- | --- | --- |
| Main transcript | `~/.claude/projects/<project-dir>/<session>.jsonl` | one JSON object per line, appended |
| Subagent transcript | `~/.claude/projects/<project-dir>/<session>/subagents/agent-<agentId>.jsonl` | one per async subagent, appended from launch |
| Subagent metadata | `.../subagents/agent-<agentId>.meta.json` | small JSON, rewritten in place |
| Task output | `/private/tmp/claude-<uid>/<project-dir>/<session>/tasks/<taskId>.output` | shell and Monitor stdout+stderr |
| Subagent "output" | `.../tasks/<agentId>.output` | a symlink to the subagent's `agent-<agentId>.jsonl` |
| Large tool results | `~/.claude/projects/<project-dir>/<session>/tool-results/<id>.txt` | oversize tool output, see section 8 |
| Live-session registry | `~/.claude/sessions/<claude pid>.json` | one object per running `claude`, see section 9 |
| Process tree | children of the `claude` pid | one `zsh -c` per shell or Monitor, see section 7 |

Facts about the layout:

- The `tasks/` directory is keyed by the session id the task was launched under. After a resume that session id
  can differ from the transcript's `sessionId`. Example: `/private/tmp/claude-501/-Users-david-dev-gx-better-notifications-design/c0f6c673-83c0-4d83-b7c1-0ee65693ff6b/tasks/a22d96cd9845ebf4d.output`
  is a symlink to `~/.claude/projects/-Users-david-dev-gx-better-notifications-design/5c6e9246-d1cb-468d-8512-c6a19dedb1e7/subagents/agent-a22d96cd9845ebf4d.jsonl`.
  The transcript `5c6e9246-....jsonl` has no `sessionId` of `c0f6c673`, but its `<output-file>` path names `c0f6c673`.
  `backgroundtask.OutputPath(base, sessionID, taskID)` (backgroundtask/facts.go:30) globs
  `<base>/*/<sessionID>/tasks/<taskID>.output` with the transcript's session id, so it finds nothing for this case.
- The exact output path is written in the transcript in three places: the shell start `tool_result` text
  ("Output is being written to: ..."), the async-agent start `toolUseResult.outputFile`, and every
  `<task-notification>` `<output-file>`.
- Subagents launched from inside a subagent are written flat in the same `subagents/` directory of the top
  session, with `parentAgentId` and `spawnDepth: 2` in their `.meta.json` (observed: `agent-a894329cb6539eab0.meta.json`).
  A subagent's own background shells write their `.output` into the same top-session `tasks/` directory
  (observed: `br06j3i2z.output` next to `a7f2ec6cab6cae52b.output`).
- On disk the output files expire. The oldest regular `.output` file is from 2026-11-12 10:11. Directories from
  sessions of 2026-11-07 are empty (`.../9804ad59-a6c6-4452-985f-3977c37c0ede/tasks/` has 0 entries).
  Transcripts under `~/.claude/projects` are not expired by this.

## 2. Background shell

Launch (Bash with `run_in_background: true`).

- Assistant line, `tool_use` block: `input = {command, description, run_in_background: true}`. `description` is
  optional (183 of 1944 background Bash calls in all transcripts have one). Example, session
  `9804ad59-a6c6-4452-985f-3977c37c0ede` (project `-private-tmp-claude-501--Users-david-dev-gx-agent-launch-and-recovery-design-db1c7d82-08cf-4431-82df-074fbc2d81cd-scratchpad-work`) line 31:
  `{"command":"echo \"it's q\"; sleep 712","description":"Echo text then sleep 712 seconds (C)","run_in_background":true}`.
- User line, `tool_result` (line 33 in that file): `message.content[0].content` is the text
  `Command running in background with ID: bustb3oh9. Output is being written to: /private/tmp/claude-501/.../tasks/bustb3oh9.output. You will be notified when it completes ...`,
  and `toolUseResult = {"stdout":"","stderr":"","interrupted":false,"isImage":false,"noOutputExpected":false,"backgroundTaskId":"bustb3oh9"}`.
  Some results also carry `backgroundCwdHint` (651 of the 1241+651 results with a `backgroundTaskId`).
- The `tool_result` line's timestamp is 1.5 s after the `tool_use` line in that example
  (22:10:45.111 and 22:10:46.612). The two never carry a start time of their own beyond these timestamps.
- Several parallel calls in one assistant message become one assistant line per `tool_use` block. They share
  `message.id` (`msg_011CgpLHXawv8JE1mQgLRJ4N`, lines 29 to 31) and each carries `stop_reason: "tool_use"`. The three
  `tool_result` lines (32 to 34) arrive in a different order than the calls (B, C, A), 5 to 8 ms apart.

Foreground call moved to the background on timeout.

- The `tool_use` has no `run_in_background`, and has `timeout` (observed: `"timeout": 400000`).
- The `tool_result` carries the output so far in `toolUseResult.stdout`, plus `backgroundTaskId` and
  `timedOutAfterMs` (174 plus 120 results in all transcripts). The text reads `Command did not complete within its 400s timeout and was moved to the background (ID: bn5vaqmtp). Output is being written to: ...`.
- Example: session `e8e4df30-d879-4e4d-b747-87502e288858` (project `-Users-david-dev-gx-bulk-actions-impl-item-39`),
  `tool_use` at 2026-11-10T23:24:28.208Z, `tool_result` at line 104, 23:31:09.440Z, 401 s later. The marker's
  start time in `ReadBackgroundTasks` (the `tool_result` line's timestamp) is 401 s after the process started.

While running.

- Process: a child of the `claude` pid, see section 7.
- Output file `tasks/<id>.output`: created at launch (birth time 17:01:34.044 for a launch logged at
  15:01:34.048Z), then grows as the command writes. Probe: a loop printing a line every 2 s showed sizes
  36, 72, 90 bytes at 17:01:37, :40, :43 (mtime moved with each line), and 108 bytes in the next listing. A Python heredoc that
  prints with `flush=True` grew the same way (10, 15, 20, 25 bytes). stdout and stderr go to the same file
  (`lsof` fd 1 and 2 of the shell both name the `.output` file).
- Nothing is added to the transcript while it runs, other than the agent's own lines.
- `toolUseResult` has no pid, no start time, no output size and no duration.

When finished.

- The `.output` file gets a trailer line: `\n[exited with code N]\n` (observed `[exited with code 0]` after 40 ticks,
  `[exited with code 3]` for a Python `sys.exit(3)`). A task stopped by `TaskStop` gets `[killed]`.
  395 of the 963 regular output files carry an `[exited with code ...]` line.
- A `<task-notification>` is enqueued, see section 6. Status and exit code are in it.
- Observed: the file mtime at exit (17:02:59.745) matches the notification line's timestamp (15:02:59.741Z).
- `TaskStop` result (line 206 of the subagent file): `{"message":"Successfully stopped task: boz79mgqc (echo started; sleep 300)","task_id":"boz79mgqc","task_type":"local_bash","command":"echo started; sleep 300"}`.
  `task_type` is `local_bash` (862 results) or `local_agent` (3). In this probe no `<task-notification>` followed
  the stop. Across all transcripts 11 notifications read `<status>killed</status>` with
  `Background command "..." was stopped`.

What the full command and its language look like.

- The full command text is in the launching `tool_use` `input.command`, untruncated. In 1944 background Bash calls
  the length is median 238 characters, p90 377, p99 541, max 997. 188 of them are multiline, 1 uses a heredoc, 2
  use `python -c`.
- Nothing records the language. The command text is the only evidence (`python3 -I - <<'PYEOF' ...`).
- The `<summary>` of the notification repeats `description` if the call had one, otherwise the command text
  (`Background command "Background python heredoc, exits 3" failed with exit code 3`).

## 3. Monitor

Launch.

- `tool_use` name `Monitor`, `input = {command, description, timeout_ms, persistent}`. Example: session `5813cc70-94d8-4f1b-bad9-2c4d8b36e57f`
  (project `-Users-david-dev-gx-better-triage-impl-item-42`) line 109:
  `{"command":"S=...; until grep -q GXF_WEB_URL $S/env1.sh ...; do sleep 2; done; echo ready","timeout_ms":180000,"description":"wait for fixture env","persistent":"false"}`.
- `tool_result` (line 110): text `Monitor started (task bi0062mc8, expires in 3m unless the source ends first; ...)`
  and `toolUseResult = {"taskId":"bi0062mc8","timeoutMs":180000,"persistent":false}`. All 347 results seen have
  `persistent: false`. `timeoutMs` values seen: 180000, 240000, 300000, 600000.
- `ReadBackgroundTasks` tells a Monitor start from other results by `toolUseResult.timeoutMs != nil`
  (transcript/background_task.go:378).

While running.

- Each stdout line of the script is one notification, and the same lines land in `tasks/<id>.output` as they
  happen (probe: after event 2 the file held `event 1\nevent 2\n`, 16 bytes at 17:02).
- Each event notification has only `<task-id>`, `<summary>` and `<event>`. It has no `<tool-use-id>`, no
  `<output-file>` and no `<status>`:
  `<task-notification><task-id>bdq8u246v</task-id><summary>Monitor event: "test monitor six events"</summary><event>event 1</event></task-notification>`.
  The description is the only name in them. 190 such notifications exist in all transcripts.
- Events batch: stdout lines within 200 ms form one notification (tool description). Event count therefore is the
  number of event notifications seen for the task id, and nothing records a running total.
- In the main transcript an event shows as a `queue-operation` `enqueue` line and a later `user` line (section 6).
  In a subagent file it shows as an `attachment` line with `attachment.type: "queued_command"`
  (probe line 142 of the subagent file).
- The watched thing is `input.command` and `input.description` on the `tool_use`. A Monitor can also take a
  `ws` source instead of a `command` (tool schema). No `ws` call was seen in the transcripts.

When finished.

- Final notification has `<tool-use-id>`, `<output-file>`, `<status>` and `<summary>`:
  `<status>completed</status><summary>Monitor "test monitor six events" stream ended</summary>`. A last `<event>` may
  ride along (`<event>ready</event>` in session 5813cc70 line 127) or be absent (probe).
- Summary shapes counted across all transcripts: `stream ended` 1773, `ended without producing output` 6,
  `script failed` 5, `stopped` 5 (status `killed`).
- The output file ends with `\n[exited with code 0]\n` (observed).

## 4. Async subagent

Launch.

- In the transcripts seen, every `Agent` call is async. `tool_use` input is `{description, model, prompt}` with no
  `run_in_background`; the result is still `async_launched`. No synchronous result shape appears in any
  transcript (no `totalDurationMs` or `totalTokens` key anywhere).
- `tool_result` `toolUseResult` has these keys (3505 results): `isAsync, status:"async_launched", agentId, description,
  resolvedModel, prompt, outputFile, canReadOutputFile, canContinueAgent`. Example, this session's parent
  transcript line 72: `agentId a7f2ec6cab6cae52b`, `description "Research Claude task facts"`,
  `resolvedModel claude-sonnet-5-5`. `outputFile` is the `tasks/<agentId>.output` symlink path. In sessions
  before 2026-11-16 some results carried `outputFile` ending in `tasks/` without a file name (session `12a391b3`
  line 186).
- `ReadBackgroundTasks` marks it as `Kind: agent` when `toolUseResult.status == "async_launched"`, id from
  `toolUseResult.agentId` (transcript/background_task.go:375).

While running, in the parent transcript.

- Nothing. The parent file has no progress lines (`"type":"progress"` count in all main files: 0) and no line
  about the subagent between the launch result and the hand-back. Example: session `12a391b3-6030-4fba-b97f-d3af0d48b9c2`
  (project `-Users-david-dev-gx-web-performance-telemetry-design`): launch result at line 186 (13:43:56.541Z),
  next line about the agent at 196 (13:46:33.700Z).

While running, in `subagents/agent-<agentId>.jsonl`.

- The file exists from launch. A tiny probe agent (`agent-a894329cb6539eab0.jsonl`) was 176372 bytes and 17 lines
  3 s after launch, before its first tool result, because it starts with prompt, environment and
  `prompt_snapshot` attachment lines and `api-request-blob` lines.
- Every line has `isSidechain: true`, `agentId`, `sessionId` (the top session), `timestamp`, `cwd`, `version`.
  The first line is the `user` prompt (`parentUuid: null`).
- Line types in a running subagent file (probe, 102 assistant, 61 user, 49 api-request, 26 attachment, 19
  api-request-blob, 2 api-request-shape). Unlike a main file it has no `queue-operation` lines.
- Model: `message.model` on each assistant line (`claude-haiku-5-5`), also `requestedModel` and `effort`
  on the line. `.meta.json` has `model` as the alias asked for (`"sonnet"`, `"haiku"`), and `pickedEffortModel`.
  The parent's `resolvedModel` is the resolved id (`claude-sonnet-5-5`).
- Tokens so far: `message.usage` on each assistant line: `input_tokens, output_tokens, cache_read_input_tokens,
  cache_creation_input_tokens` (example line 17 of the probe: `2, 115, 0, 32156`). Parallel `tool_use` blocks of one
  message each get their own assistant line with the same `message.id` and the same `usage` object, so summing lines
  double-counts. Summed over unique `message.id`, one finished subagent
  (`agent-a6fcbc8a9e2eb44da`, 19 unique messages) gives output 20765, input+output 20803, all four fields
  1,539,903 (cache reads repeat every turn).
- Tool call count so far: count of `tool_use` blocks in assistant lines (28 in `a6fcbc8a9e2eb44da`, matching the
  notification's `<tool_uses>28`; the count includes the final `SubagentHandback` call).
- Current tool call: the last assistant line's `tool_use` block with no `tool_result` line after it (probe line
  17 `Bash "sleep 20; echo hi"` at 15:03:21.619, result at line 18, 15:03:42.175). `input.description`, `command`
  or `file_path` give the label.
- Last text: the last assistant line with a `text` block. Thinking blocks appear as separate assistant lines with
  `thinking` content (`signature` present, text sometimes empty).
- Its own background tasks: lines with `toolUseResult.backgroundTaskId` (shell), `taskId`+`timeoutMs`
  (Monitor) or `status:"async_launched"` (nested agent), all inside the subagent file. Their notifications are
  `attachment`/`queued_command` lines in the subagent file (probe line 187: the `br06j3i2z` completion). The
  parent file has no copy. Both gx readers skip `isSidechain` lines (transcript/background_task.go:343,
  transcript/inflight.go:96), so every line of a subagent file is skipped by them.
- Start time: the file's first line timestamp (probe agent: 15:03:18.740Z; the parent's launch result line was
  written at about the same time).

When finished.

- `.meta.json` gains `"finished": true` (3604 of 3653 meta files). Running probe: 263 bytes without it, finished:
  293 bytes with it. Other keys seen: `agentType, isBuiltIn, description, toolUseId, spawnDepth, requestShape,
  requestNonInteractive, model, pickedEffortModel, launchedWithDefaults, parentAgentId, isFork, worktreePath,
  spawnedWithWorktree, worktreeBranch, effort, stoppedByUser`. `toolUseId` is the launching `tool_use` id.
- The subagent file ends with the `SubagentHandback` `tool_use` and its result line
  (`{"success":true,"message":"Report delivered to your caller."}`).
- The parent file gets two `queue-operation` `enqueue` lines in the same 3 ms: an `<agent-message from="<agentId>">`
  carrying the report text (line 196) and the `<task-notification>` (line 197, 13:46:33.703Z). Later `user`
  lines deliver both (lines 200 and 208; 208 is 1.5 s later, 13:46:35.222Z).
- Notification body: `<status>completed</status>`, `<summary>Agent "<description>" finished</summary>`,
  a `<note>`, a `<result>` that holds the report or the sentence "This agent's report was delivered to you as a message ...",
  and `<usage><subagent_tokens>105155</subagent_tokens><tool_uses>28</tool_uses><duration_ms>157181</duration_ms></usage>`.
  An optional `<worktree><worktreePath>...<worktreeBranch>...</worktree>` follows (2 of 3,600 seen).
- `<duration_ms>` is within 8 ms of last-line minus first-line timestamp of the subagent file
  (157173 ms versus 157181 ms; probe agent 29389 ms versus 29402 ms).
- `<subagent_tokens>` equals neither the sum of per-message tokens nor the cache-inclusive sum. For
  `a6fcbc8a9e2eb44da` it was 105155, and the last assistant message's `input+cache_read+cache_creation+output`
  was 104670 (485 less). For the probe it was 40167. It is closest to the final context size.
- Other agent summaries: `Agent "X" failed: Agent terminated early due to an API error: ...` (status `failed`, 7)
  and `Agent "X" is waiting on its own background work` (status `completed`, 53; the `<note>` says the result may
  be interim and that the same task id notifies again). The reader already skips interim notices
  (`interimNotificationNote`, transcript/background_task.go:244).
- Notification shapes across all transcripts (enqueue lines only): 3591 agent
  notifications with `result` and `usage`, 1473 shell notifications with no result, 323 Monitor final notices with `event`.

## 5. Foreground tool call in flight

- Start: the assistant line's `tool_use` block, timestamp of that line. End: the user line with the matching
  `tool_result` (`tool_use_id`). The two line timestamps are the only duration source.
- Parallel calls: one assistant line per `tool_use`, shared `message.id`. Probe (3 parallel `sleep 6`): assistant
  lines at 15:06:04.503, :04.859, :05.184; results at 15:06:11.545, :11.552, :11.558, order c, b, a, 6.4 to 6.8 s
  after the calls (they ran concurrently).
- Second probe (`sleep 8`, `sleep 3`, `sleep 5; exit 7`, calls at 15:05:41.567, :41.924, :42.320). Results at
  15:05:45.770 (fast, 3 s call, 4.2 s after its line), 15:05:50.739 (slow, 8 s call, 9.2 s after) and 15:05:55.916 (the
  5 s call, 13.6 s after its line). The third call's result came 5.2 s after the second one. The cause is not known;
  a `tool_use` line timestamp can be earlier than the process start.
- Success result: `toolUseResult = {"stdout", "stderr", "interrupted", "isImage", "noOutputExpected"}`. The model-facing
  `content` is the stdout text. No exit code is stored on success (117,997 results have exactly these five keys).
- Failure result (non-zero exit): `tool_result` has `is_error: true` and content `Exit code 7`; `toolUseResult` is the
  string `"Error: Exit code 7"`, not an object (probe line 284).
- Other keys: `returnCodeInterpretation` (1478), `persistedOutputPath` + `persistedOutputSize` (604, section 8),
  `bashEditDiff`, `gitOperation`, `staleReadFileStateHint`, `dangerouslyDisableSandbox`.
- An in-flight call never has a `tool_result` yet. `ReadInFlightTools` lists the `tool_use` blocks of the current
  turn (after the last real user line) with no matching `tool_result`, newest first, label capped at 60 runes
  (transcript/inflight.go:75-124, `inFlightLabelCap = 60`). It keeps only `kind` and `label`. It drops
  the tool-use id, the timestamp and every argument beyond the label.
- An `Agent` call and a background Bash call get their `tool_result` within about 1.5 s of the `tool_use` (above), so
  they leave the in-flight list almost at once. A call moved to the background by timeout stays in flight until
  its timeout (400 s in the example) and then gets the result carrying `backgroundTaskId`.
- Foreground calls of a subagent are in the subagent file, not the parent file.

## 6. `<task-notification>` and the queue lines

- Main transcript, one notification takes three lines: `queue-operation` `enqueue` (content is the notification),
  `queue-operation` `dequeue` or `remove` (a `reason` such as `absorbed_mid_turn` can follow), and a `user` line
  whose `message.content` is the notification string (`origin.kind: "task-notification"`,
  `turnOrigin: "task_notification"`), plus an `attachment` line of type `queued_command`. Example, session `9804ad59`
  lines 48 (enqueue at 22:12:04.526), 49 (dequeue), 52 (user, 22:12:04.533). Counts across all transcripts:
  9987 `enqueue`, 4953 `dequeue`, 4963 `remove`.
- Subagent file: only the `attachment` `queued_command` line (carries `prompt`, `source_uuid`, `delivery_id`,
  `commandMode: "task-notification"`, `origin`).
- Fields: `<task-id>`, `<tool-use-id>`, `<output-file>`, `<status>`, `<summary>`, optional `<note>`, `<result>`,
  `<usage>`, `<event>`, `<worktree>`. Text inside is XML-escaped (`&gt;`, `&amp;`).
- Status values over all transcripts: `completed` 25064, `failed` 2015, `killed` 66, `stopped` 17. There is no
  field for the end time and none for the exit code. End time is the notification line's timestamp. Exit
  code is in the summary text: `completed (exit code 0)` (548) or `failed with exit code N` (160). Shell
  summary shapes: `Background command "X" completed (exit code N)`, `failed with exit code N`, `was stopped`.
- `stopped` is written for tasks of a previous session: `Background shell command didn't finish before the previous session ended` (with a `<note>`).
- Exact times in the example: failed task `b5fgh4g6u`, started 22:10:46.604, enqueued 22:12:04.526: the
  `sleep 711` command had been running 78 s, so it was killed before its time (exit 1, status `failed`).
  Nothing in the transcript says why.
- `ReadBackgroundTasks` resolves a marker on any later non-sidechain notification, attachment or `queue-operation` line
  that contains the task id token, and on a `TaskStop`/`TaskOutput` result by structured id. It records no status, exit code or end
  time (`BackgroundTaskMarker` has TaskID, Status, StartedAt, Cap, Kind, Label only). `Label` is the launching call's
  `description`, else the first 60 runes of `command` (transcript/background_task.go:75-80, 67).

## 7. Process tree

- Every background shell and every Monitor is a direct child of the `claude` pid, a `zsh` run as
  `/bin/zsh -c source <shell-snapshot>.sh 2>/dev/null || true && ... && eval '<command>' < /dev/null && pwd -P >| /tmp/claude-<4 hex>-cwd`.
  Observed in this session (`claude` pid 12178, children 14138 and 14141).
- The command text is inside the `eval '...'` argument. A newline is a real newline in argv (`ps` prints
  `\012`) and a single quote in the command becomes `'"'"'`. A heredoc command shows its whole body.
  `Procs.Argv(pid)` joined with spaces is what `ProcessFinder.Match` searches with `strings.Contains` for
  the command text (backgroundtask/process.go:75), so a command with quotes or newlines does not appear verbatim.
- Shell: fd 1 and fd 2 are the `.output` file (`lsof -a -p 14138 -d 1,2 -Fn`). The child of the shell
  (`sleep`, `python3`) also has fd 1 on the same file. `ProcessFinder.Match` matches this path first
  (process.go:56-68) and falls back to command text.
- Monitor: the `zsh` child has no `.output` file on fd 1 (fd 1 is not a path under `tasks/`), so the output-file match
  finds nothing and only the command-text fit is left. Observed for two Monitors (pids 42711, 42815).
- Subagent: no process. The agent runs inside `claude`; the child list of `claude` showed only shell
  children and `caffeinate`.
- A foreground Bash call is also a `zsh -c ... eval` child with stdout on a pipe.
- `ProcessMatch` (process.go:21-33): `Alive`, `PID`, `Ambiguous`, `LastOutput` (output-file mtime), `LastLines`
  (last 3 lines). `readOutputTail` reads the whole file with a `bufio.Scanner` (1 MiB line limit), keeps a 3-line ring
  (process.go:149-175).
- `backgroundtask.Facts` picks only the newest outstanding task of a session and calls `Match` with root pid 0
  (the whole process table) and no command text (backgroundtask/facts.go:41-55).

## 8. Output file sizes and large results

- Regular `.output` files (shell and Monitor), 963 files: total 302,529 bytes, median 48 B, p90 1179 B, p99 4004 B,
  max 10,716 B, 3 files with size 0. Largest: `.../-Users-david-dev-gx-bg-progress-design/6c75ba65-.../tasks/bfwi3xb9d.output`
  (10716 B, in this session's tasks directory), then 10103 B in
  `-Users-david-dev-gx-follow-ups-2026-11-16-item-44/f3683881-.../tasks/bgfe9h9cc.output`.
- Subagent "output" symlinks, 1460 files, all resolve: size of the target JSONL median 384,106 B, p90 966,162 B,
  p99 2,626,466 B, max 4,962,744 B.
- Main transcripts (6866): median 779,185 B, p90 1,490,618 B, p99 2,962,855 B, max 16,662,659 B
  (`-Users-david-dev-gx-main/2acb1907-8fbf-4105-8d6b-b86005f7b783.jsonl`). Subagent transcripts (3527): median
  388,295 B, p90 913,705 B, p99 2,617,024 B, max 5,232,992 B. Meta files (3653): median 246 B, max 421 B.
- `tool-results/` files (1155): median 40,101 B, p90 101,999 B, max 67,108,864 B. A Bash result over a size
  cap (this session: a 488.8 KB result) is replaced in the model's view by `<persisted-output>` text with the file
  path, and `toolUseResult` gets `persistedOutputPath` and `persistedOutputSize` (604 results).
- Part of every subagent and main file is preamble: a 3-line subagent probe was already 176 KB.
- Line sizes: Bash `command` text is stored whole. A transcript line can hold a long `tool_result` body, so a
  line can run to many KB.

## 9. Cost of a read

Measured with `transcript.ReadBackgroundTasks` and `transcript.ReadInFlightTools` from a scratch module outside the
repo (replace directive to this worktree), third of three runs per file, warm page cache, this laptop:

| Main transcript | Size | `ReadBackgroundTasks` | `ReadInFlightTools` |
| --- | --- | --- | --- |
| median | 779,185 B | 1.9 ms | 1.8 ms |
| p90 | 1,490,618 B | 3.4 ms | 3.2 ms |
| p99 | 2,962,855 B | 6.9 ms | 6.5 ms |
| max | 16,662,659 B | 24.8 ms | 29.0 ms |

- Both readers read the whole file from byte 0 through `ForEachLine` (transcript/chat.go:216, a 64 KiB
  `bufio.Reader`), parse every line as JSON, and keep no state between calls. `ForEachLine` hands a final line that
  has no newline to the callback (chat.go:229), so a half-written last line is parsed and dropped as invalid.
- No reader in `transcript/` or `backgroundtask/` keeps an offset or a cache of parsed results. The one
  partial read is `ReadAt` from the tail for the last-lines reader (transcript/transcript.go:310). `backgroundtask.Reader`
  caches only the transcript path per session id (backgroundtask/backgroundtask.go:80).
- Cost of cold reads (first read after the page cache is cold) was not measured.
- The chat read (`server/sessions/chat/read.go:75-116`) runs `c.ReadSnapshot` then `snap.Background(time.Now())` and
  `snap.InFlight` (lines 94, 102, 105). `SessionBackgroundTask` is returned by `snap.Background`; the code that builds
  it is outside the files read here. `read.go` reads no output file, no subagent file and no `.meta.json`.
  The paused-ticket card reads the output file through `backgroundtask.Facts`.
- Appending: every file in this document was only seen growing. In this session the subagent file went
  from 612,940 B (17:00) to 1,791,736 B, and the main file from 1,185,477 B. No truncation or rewrite of a
  JSONL was seen. `.meta.json` and `~/.claude/sessions/<pid>.json` are rewritten in place. So a reader can use a stored
  byte offset for the JSONL files and the `.output` files; an offset past the file size (truncation) was never observed
  and was not tested.
- File change signals: `.output` mtime changes with each write and at exit; its birth time is the start
  (macOS `stat -c %w`, GNU `stat` from coreutils). Subagent JSONL mtime changes on each appended line.
  A cheap check per task is one `stat`.
- `wc -l` or `tail` of an `.output` file costs a read of at most the file size, which is median 48 B and p99 4 KB.
  Line count of the file is not stored anywhere else.

## 10. Per kind: what exists

Cells say where the field lives. "none" means no source was found.

| Field | Background shell | Monitor | Async subagent | Foreground call |
| --- | --- | --- | --- | --- |
| Start time | `tool_use` line ts; result line ts; output file birth time | `tool_use` line ts; result line ts; output file birth time | parent launch result line ts; subagent file first line ts | `tool_use` line ts (may be earlier than process start, section 5) |
| End time | notification line ts; output file mtime at exit | final notification line ts | parent notification line ts; subagent file last line ts | `tool_result` line ts |
| Duration | none stored; end minus start | none stored; end minus start | `<duration_ms>` in notification; or first/last line ts | none stored |
| Exit code | notification summary text; `[exited with code N]` in output file | final summary text only (`script failed`, `stream ended`); output file trailer | none (status `failed` with API error text) | `is_error` + text `Exit code N` on failure only |
| Status | notification `<status>` completed/failed/killed | final notification `<status>` | notification `<status>` completed/failed; interim form | presence of `tool_result` and `is_error` |
| Full command | launching `tool_use` `input.command` | launching `tool_use` `input.command` | launching `tool_use` `input.prompt` (also in `toolUseResult.prompt`, and the subagent file's first line) | `tool_use` `input` |
| Description | `input.description` | `input.description` | `toolUseResult.description`, `.meta.json` `description` | `input.description` for Bash |
| Output size, last lines | `stat` and read of `.output` (grows while running) | `.output` (one line per event, grows) | subagent JSONL size; last assistant `text` | `tool_result` content; `persistedOutputSize` if persisted |
| Model | none | none | `resolvedModel` (parent); `message.model` (subagent lines); `.meta.json` alias | none |
| Tokens | none | none | `message.usage` per unique `message.id`; `<subagent_tokens>` at the end only | none |
| Tool call count | none | none | `tool_use` blocks in subagent file; `<tool_uses>` at the end only | none |
| Current tool call | none | none | last `tool_use` with no result in subagent file | the call itself |
| Event count | n/a | count of event notifications by `<task-id>` | n/a | n/a |
| Process | child `zsh`, stdout is the `.output` path | child `zsh`, stdout is a pipe | none | child `zsh`, stdout is a pipe |

## 11. Not verified

- Whether a subagent's `<subagent_tokens>` formula is exact. Only the two examples above were compared.
- Behaviour when two sessions resume the same task directory beyond the one example in section 1.
- Cold-cache read time, and any read while Claude Code is mid-write (no torn line was caught).
- Whether the `.output` file of a task can be truncated or rotated while it runs. Biggest file seen is 10,716 B, so
  no large-output case was exercised.
- The cause of the delayed third call in the second foreground probe (section 5).
- Why no killed-notification followed the `TaskStop` probe, while 11 appear in older transcripts.
