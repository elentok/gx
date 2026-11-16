# gx:bg-progress/09 - What a Monitor writes when it expires at `timeout_ms`

Question: what does Claude Code write when a Monitor reaches its `timeout_ms`, how does that differ from TaskStop,
and does `toolUseResult.timeoutMs` hold the requested value for a request above 1800000?

Sources:
- Claude Code 2.1.325 (`claude --version`). Observed behaviour, not documented contract.
- Research 08 (`docs/research/bg-progress-08-persistent-monitor.md` on `research/bg-progress-persistent-monitor`), probe method reused.
- Live probes: `claude -p --model haiku --verbose --output-format stream-json --allowedTools Monitor TaskStop`, throwaway cwds
  `/private/tmp/claude-501/p09/{a,b,c,d1800000,d900000}`. Session transcripts read from `~/.claude/projects/-private-tmp-claude-501-p09-*`,
  output files from `/private/tmp/claude-501/-private-tmp-claude-501-p09-*/<session>/tasks/`.
- Every probe process exited or hit its `timeout`. No leftover `sleep`, no `~/.claude/sessions/*.json` for the probe cwds.

## Headline

- Expiry is **an `<event>` line**, not a final-status notice. It has `<summary>Monitor event: "<description>"</summary>` and
  `<event>[Monitor expired after 1m with 1 event delivered. Re-arm it if you still need the watch.]</event>`. No `<status>`,
  no `<tool-use-id>`, no `<output-file>`. The on-disk transcript has no `<status>` line at all for this Monitor.
- Expiry shows up exactly like a normal event notice (same `queue-operation enqueue`, `dequeue`, `user` triple, same `origin`).
  Only the `<event>` text tells them apart. The count in the text includes the earlier `hi` event.
- TaskStop writes **no** notification line to the transcript (confirms research 08). Expiry does write one.
- The output file ends with `\n\n[killed]\n` in both cases.
- `toolUseResult.timeoutMs` is **capped to 600000 (10 minutes) on 2.1.325**, not 1800000. Requests of 900000, 1800000 and 3600000 all gave 600000.
  The tool description in this version says so. Research 08 quoted the older text (30 minutes).

## 1. Probe A: Monitor left to expire (`timeout_ms` 60000)

Setup: `Monitor {description:"probe09a", command:"echo hi; sleep 600", timeout_ms:60000}`. `claude -p` stayed alive past expiry
(it was woken by the event, then by the expiry notice, and ended after answering). Session `87fabef8-...`, Monitor task `b1o8fmpyd`.

Transcript line sequence after the Monitor started (times are against the Monitor `tool_result` at 18:31:02.899 = t0):

| t | line | content |
| --- | --- | --- |
| +0.000 s | `user` (tool_result) | `Monitor started (task b1o8fmpyd, expires in 1m unless the source ends first; ...)`, `toolUseResult {"taskId":"b1o8fmpyd","timeoutMs":60000,"persistent":false}` |
| +0.203 s | `queue-operation enqueue` | event notice, `<event>hi</event>` |
| +2.068 s | `queue-operation dequeue`, then `user` (+2.070 s) | same notice, `origin {kind:"task-notification", producer:"session-task"}` |
| +60.004 s | `queue-operation enqueue` | **expiry notice** (text below) |
| +60.020 s | `queue-operation dequeue` | - |
| +60.029 s | `user` | same text as the enqueue, `origin {kind:"task-notification", producer:"session-task"}`, `turnOrigin:"task_notification"` |
| +61.6 s | `assistant` | the model's reply to the notice, then `cost-state` |

Expiry notice, verbatim:

```
<task-notification>
<task-id>b1o8fmpyd</task-id>
<summary>Monitor event: "probe09a"</summary>
<event>[Monitor expired after 1m with 1 event delivered. Re-arm it if you still need the watch.]</event>
</task-notification>
```

- Delay from `timeout_ms` to the enqueue line: 4 ms past 60.000 s. The `1m` in the text is the timeout, not a measured time.
- The `queue-operation` here is `dequeue` (the session was idle), not `remove`. No `attachment` `queued_command` line appeared for it.
- Is it an event or a final notice: its wrapper is the event shape (`<event>` present, no `<status>`). It is the last notice for the task.
  Nothing else was written for task `b1o8fmpyd` afterwards. "1 event delivered" counts the `hi` line, so the count is the number of
  `<event>` notices before it, not including the expiry notice itself.
- Output file `tasks/b1o8fmpyd.output`, full bytes: `hi\n\n[killed]\n`. The `[killed]` trailer is written, with a blank line before it.
- Stream-json only (not in the on-disk transcript), at the expiry moment:
  - `system/task_updated` `{"task_id":"b1o8fmpyd","patch":{"status":"killed","end_time":1794853922908}}` (18:32:02.908, 5 ms after the enqueue)
  - `system/task_notification` `{"task_id":"b1o8fmpyd","status":"stopped","output_file":".../tasks/b1o8fmpyd.output","summary":"probe09a"}`
  - `system/background_tasks_changed` `{"tasks":[]}`
  So the SDK stream reports `stopped` with the bare description as summary, while the transcript carries the `Monitor event:` text.
- The sleep process was gone after the session ended (`pgrep` empty).

## 2. TaskStop for comparison (probe B, same Claude Code)

`Monitor {description:"probe09b", command:"echo hi; sleep 600", timeout_ms:3600000}`, then TaskStop on the task id (session `f7b48c02-...`, task `bj1qygjfx`).

- TaskStop `toolUseResult`: `{"message":"Successfully stopped task: bj1qygjfx (echo hi; sleep 600)","task_id":"bj1qygjfx","task_type":"local_bash","command":"echo hi; sleep 600"}`.
- The only `task-notification` text in the transcript is the `hi` event (enqueue, `attachment queued_command`, `remove`). No `<status>` and no
  `Monitor expired` / `stopped` line after the stop. Same as research 08.
- Output file `tasks/bj1qygjfx.output`, full bytes: `hi\n\n[killed]\n`. Identical tail to expiry.

| Ending | Notice in transcript | Output file tail |
| --- | --- | --- |
| `timeout_ms` expiry | one `Monitor event:` notice, `<event>[Monitor expired after Nm with K event(s) delivered. Re-arm it if you still need the watch.]</event>`, no `<status>` | `\n\n[killed]\n` |
| TaskStop | none | `\n\n[killed]\n` |
| Command exits (research 01) | `<status>completed</status>` notice | `[exited with code N]` |
| SIGTERM of `claude -p` (research 08) | `<status>killed</status>` notice | `\n\n[killed]\n` |
| SIGKILL (research 08) | none | no trailer |

## 3. The `timeout_ms` cap (probes B, C, D)

Each probe requested a Monitor with `command` `echo hi; sleep 600` or `sleep 5000`, then TaskStop. The model's `tool_use` input kept the request; the
result held the cap:

| `input.timeout_ms` | `toolUseResult.timeoutMs` | start text |
| --- | --- | --- |
| 3600000 (probe B) | 600000 | `expires in 10m` |
| 3600000 (probe C) | 600000 | `expires in 10m` |
| 1800000 (probe D) | 600000 | - |
| 900000 (probe D) | 600000 | - |
| 60000 (probe A) | 60000 | `expires in 1m` |

- Tool description in the 2.1.325 ToolSearch result: "Every monitor expires after `timeout_ms` (default 5 minutes, at most 10 minutes): it is killed and
  you get one notice with the event count. Re-arm it if you still need the watch; ..." and "capped to 600000ms". The JSON schema still lists
  `"maximum":3600000` and `"default":300000`, so the schema maximum (3600000) and the real cap (600000) differ.
- Values between 60000 and 900000 other than the default were not probed. The cap point lies somewhere in that range (only 600000 itself, from the text, is known).

## 4. Fixtures (redacted: uuids and session ids replaced with `<uuid>`, cwd shortened, assistant lines dropped)

Probe A, expiry case, one JSON object per line:

```jsonl
{"parentUuid":"<uuid>","isSidechain":false,"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_01M7VF5Rnc9zy8bif4nBdPi8","type":"tool_result","content":"Monitor started (task b1o8fmpyd, expires in 1m unless the source ends first; you get one notice at expiry — re-arm if you still need the watch). You will be notified on each event. Keep working — do not poll or sleep. Events may arrive while you are waiting for the user — an event is not their reply."}]},"uuid":"<uuid>","timestamp":"2026-11-16T18:31:02.899Z","toolUseResult":{"taskId":"b1o8fmpyd","timeoutMs":60000,"persistent":false},"userType":"external","entrypoint":"sdk-cli","cwd":"/tmp/p09/a","sessionId":"<uuid>","version":"2.1.325"}
{"type":"queue-operation","operation":"enqueue","timestamp":"2026-11-16T18:31:03.102Z","sessionId":"<uuid>","content":"<task-notification>\n<task-id>b1o8fmpyd</task-id>\n<summary>Monitor event: \"probe09a\"</summary>\n<event>hi</event>\n</task-notification>"}
{"type":"queue-operation","operation":"dequeue","timestamp":"2026-11-16T18:31:04.967Z","sessionId":"<uuid>"}
{"parentUuid":"<uuid>","isSidechain":false,"type":"user","message":{"role":"user","content":"<task-notification>\n<task-id>b1o8fmpyd</task-id>\n<summary>Monitor event: \"probe09a\"</summary>\n<event>hi</event>\n</task-notification>"},"uuid":"<uuid>","timestamp":"2026-11-16T18:31:04.969Z","origin":{"kind":"task-notification","producer":"session-task"},"promptSource":"system","turnOrigin":"task_notification","userType":"external","entrypoint":"sdk-cli","cwd":"/tmp/p09/a","sessionId":"<uuid>","version":"2.1.325"}
{"type":"queue-operation","operation":"enqueue","timestamp":"2026-11-16T18:32:02.903Z","sessionId":"<uuid>","content":"<task-notification>\n<task-id>b1o8fmpyd</task-id>\n<summary>Monitor event: \"probe09a\"</summary>\n<event>[Monitor expired after 1m with 1 event delivered. Re-arm it if you still need the watch.]</event>\n</task-notification>"}
{"type":"queue-operation","operation":"dequeue","timestamp":"2026-11-16T18:32:02.919Z","sessionId":"<uuid>"}
{"parentUuid":"<uuid>","isSidechain":false,"type":"user","message":{"role":"user","content":"<task-notification>\n<task-id>b1o8fmpyd</task-id>\n<summary>Monitor event: \"probe09a\"</summary>\n<event>[Monitor expired after 1m with 1 event delivered. Re-arm it if you still need the watch.]</event>\n</task-notification>"},"uuid":"<uuid>","timestamp":"2026-11-16T18:32:02.928Z","origin":{"kind":"task-notification","producer":"session-task"},"promptSource":"system","turnOrigin":"task_notification","userType":"external","entrypoint":"sdk-cli","cwd":"/tmp/p09/a","sessionId":"<uuid>","version":"2.1.325"}
```

Output file (`tasks/b1o8fmpyd.output`): `hi\n\n[killed]\n`.

Probe B, capped request and TaskStop case (the lines that matter, assistant and attachment lines dropped):

```jsonl
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01YMCEvXiu7pqrDXvEFTwut6","name":"Monitor","input":{"description":"probe09b","command":"echo hi; sleep 600","timeout_ms":3600000}}]},"timestamp":"2026-11-16T18:31:03.556Z","cwd":"/tmp/p09/b","sessionId":"<uuid>","version":"2.1.325"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_01YMCEvXiu7pqrDXvEFTwut6","type":"tool_result","content":"Monitor started (task bj1qygjfx, expires in 10m unless the source ends first; you get one notice at expiry — re-arm if you still need the watch). You will be notified on each event. Keep working — do not poll or sleep. Events may arrive while you are waiting for the user — an event is not their reply."}]},"timestamp":"2026-11-16T18:31:03.574Z","toolUseResult":{"taskId":"bj1qygjfx","timeoutMs":600000,"persistent":false},"cwd":"/tmp/p09/b","sessionId":"<uuid>","version":"2.1.325"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_013nWLQT1TaSwZK97fMDkRN4","type":"tool_result","content":"{\"message\":\"Successfully stopped task: bj1qygjfx (echo hi; sleep 600)\",\"task_id\":\"bj1qygjfx\",\"task_type\":\"local_bash\",\"command\":\"echo hi; sleep 600\"}"}]},"timestamp":"2026-11-16T18:31:05.223Z","toolUseResult":{"message":"Successfully stopped task: bj1qygjfx (echo hi; sleep 600)","task_id":"bj1qygjfx","task_type":"local_bash","command":"echo hi; sleep 600"},"cwd":"/tmp/p09/b","sessionId":"<uuid>","version":"2.1.325"}
```

Output file (`tasks/bj1qygjfx.output`): `hi\n\n[killed]\n`.

Stream-json lines at expiry (probe A, not in the on-disk transcript):

```jsonl
{"type":"system","subtype":"task_updated","task_id":"b1o8fmpyd","patch":{"status":"killed","end_time":1794853922908}}
{"type":"system","subtype":"task_notification","task_id":"b1o8fmpyd","tool_use_id":"toolu_01M7VF5Rnc9zy8bif4nBdPi8","status":"stopped","summary":"probe09a"}
{"type":"system","subtype":"background_tasks_changed","tasks":[]}
```

## 5. Not probed

- Expiry with `timeout_ms` over 60000 and under 600000, or a Monitor that gets several events before expiry (the "N events delivered" plural form: text seen only as `1 event delivered`).
- Expiry with zero events (the wording for 0 events).
- Expiry in an interactive TUI session, and an expiry that lands while a turn is running (`remove` / `absorbed_mid_turn`, as in research 08 for events).
- A `ws` source Monitor.
