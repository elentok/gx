# What Codex records live about running commands and subagents

Ticket: `gx:bg-progress/02`. Installed Codex: `codex-cli 0.169.0`. Source read at `openai/codex` tag `rust-v0.169.0`.

Claim tags: **[run]** measured on real rollouts under `~/.codex/sessions` or by running gx code on them, **[source]** read in the Codex or gx source, **[open]** not confirmed. Facts only, no design choices.

## Summary

- Codex runs a long command in one of two ways. Both start from a tool call, and neither writes a "started" line to the rollout. Before 0.144.1 the calls are `exec_command` and `write_stdin`. From 0.144.1 the model calls a JavaScript `exec` tool whose script calls `tools.exec_command(...)` and `tools.write_stdin(...)`, and polls a yielded script with `wait`.
- A running command leaves only its opening tool call in the rollout. Its end is a separate line: `event_msg/exec_command_end` up to 0.128, nothing from 0.129 to 0.146, and `event_msg/item_completed` with `item.type = CommandExecution` from 0.147. That line holds the full command, cwd, exit code, duration and output, written when the process ends, not when the agent polls.
- Output of a running command is never in the rollout. Codex marks Exec begin, output deltas and item-started events as non-durable. The final output is in the rollout, capped at 64 KiB.
- Several commands run at once (up to 64 unified-exec processes). Matching start to end works by `process_id` (it equals the `session_id` the agent saw) only when the command outlived its first yield. Otherwise the match is by command text and time.
- Subagents each have their own rollout file, linked to the parent by `session_meta`. The parent rollout holds `SubAgentActivity` events (model, effort, path, thread id) and `list_agents` / `wait_agent` outputs. Tokens and the current step are only in the child's rollout.
- Rollouts are append-only, one JSON object per line, with an `ordinal` equal to the line index (from 0.142.3 in every file checked). Local plain rollouts: median 0.3 MB, p90 3.0 MB, max 99 MB. gx re-reads the whole file whenever its size or mtime changes.
- gx today reads none of this for Codex beyond open calls (`codexsession/inflight.go`) and the `Ready` window title. On real rollouts, gx's in-flight reader shows nothing while a `wait`-polled script runs, and shows the label `wait` or `exec` or `spawn_agent` at other moments.

## 1. Corpus and versions

- 108 plain `*.jsonl` rollouts under `~/.codex/sessions/2026/10/29` to `2026/11/15`, and about 305 `*.jsonl.zst` rollouts from `2026/03` to `2026/10`. Three more `.zst` files sit in `~/.codex/archived_sessions`. [run: `fd -e jsonl -e zst`]
- `cli_version` in line 1 (`session_meta`): plain files 0.147.0 (1), 0.166.0 (52), 0.166.0-alpha.5.1 (7), 0.167.0 (9), 0.169.0 (39). Compressed files cover 0.111.0, 0.115.0 to 0.118.0, 0.120.0 to 0.122.0, 0.125.0, 0.128.0 to 0.131.0, 0.133.0, 0.135.0, 0.137.0, 0.142.3 to 0.147.0. There is no file between 0.147.0 and 0.166.0. [run: `zstd -dc <file> | head -1`]
- Codex compresses old rollouts to `.jsonl.zst` and decompresses one again before appending to it. [source: `codex-rs/rollout/src/recorder.rs`, `compression::materialize_rollout_for_append`; gx: `codexsession/chat.go` `ArchivedRolloutIn`]
- gx's reader keys on `session_meta` / `event_msg` / `response_item` lines (`codexsession/chat.go`, `type chatRolloutLine`) and warns when a rollout has turns but no readable message (`Chat.Unreadable`).

### Tool shapes by version

Counts are calls summed over the files of each version. [run: `jq` over every rollout, per `cli_version`]

| Versions | Shell tools | Wait tools | Subagent tools |
| --- | --- | --- | --- |
| 0.111.0 to 0.143.0 | `function_call` `exec_command`, `write_stdin` | none | v1: `spawn_agent`, `wait_agent`, `close_agent` (0.115.0 on) |
| 0.144.1 to 0.169.0 | `custom_tool_call` `exec` (JavaScript) | `function_call` `wait` (by `cell_id`) | v2: `spawn_agent`, `followup_task`, `send_message`, `interrupt_agent`, `list_agents`, `wait_agent`, `close_agent` |
| 0.166.0 and later | as above | `function_call` `sleep` (`{"duration_ms":20000}`, 0.166.0: 31 calls, 0.169.0: 9) | as above |

0.143.0 is the last version here with the old shape (`exec_command` 166 calls, `write_stdin` 21); 0.144.1 is the first with `exec` (44 calls). [run]

### Which line records a command's end, by version

| Versions | Line that carries the command's result |
| --- | --- |
| 0.111.0 to 0.116.0 | only the tool's own output line (`function_call_output`). No `exec_command_end` in any file of these versions. |
| 0.118.0 to 0.128.0 | `event_msg` with `payload.type = exec_command_end` (7,503 lines in all), plus the tool's own output line |
| 0.129.0 to 0.146.1 | only the tool's own output line (`function_call_output` or `custom_tool_call_output`). No `exec_command_end`, no `CommandExecution` item in any file of these versions. |
| 0.147.0 to 0.169.0 | `event_msg` with `payload.type = item_completed` and `payload.item.type = CommandExecution`, plus the tool's own output line |

[run: counts of `[type, payload.type, payload.item.type]` per version over all files, script `events.sh`] Seven stray `CommandExecution` items sit in 0.118.0 files. [run]

Why 0.129.0 to 0.146.1 has no command event: the persistence policy keeps only `ItemCompleted` items that have no raw response item or legacy equivalent in `Legacy` history mode, and keeps every `ItemCompleted` in `Paginated` mode. `ExecCommandEnd` is listed as transient and is never written. [source: `codex-rs/rollout/src/policy.rs` at `rust-v0.169.0`] I did not read the policy of the versions in between. [open]

## 2. How a long command runs

### 2.1 Current shape (0.144.1 and later): `exec` cells

One `custom_tool_call` named `exec`. Its `input` is JavaScript source. Example, `rollout-2026-11-13T18-35-47-01a1d5c3-e338-7d81-95d5-1c0f8193e2ba.jsonl` (0.169.0), ordinal 523:

```
{"type":"custom_tool_call","name":"exec","call_id":"call_0f...","input":"const r = await tools.exec_command({\"cmd\":\"gx check\",\"workdir\":\"/Users/david/dev/gx/main\",\"yield_time_ms\":30000,\"max_output_tokens\":30000,\"sandbox_permissions\":\"require_escalated\",...}); text(r.output)\n"}
```

Nested tools seen in `input` over the plain files of `2026/10` and `2026/11`: `tools.exec_command` 2,235, `tools.write_stdin` 336, `tools.apply_patch` 185, `tools.web__run` 129, `tools.view_image` 98, plus MCP tools. [run]

What the agent gets back depends on how long the script runs. The tool's output line (`custom_tool_call_output`, same `call_id`) has one of three heads. [source: `codex-rs/core/src/tools/code_mode/mod.rs`, `format_script_status`]

- `Script completed` (or `Script failed`, `Script terminated`): the script ended before its yield time. Real line: `"[{\"type\":\"input_text\",\"text\":\"Script completed\\nWall time 0.1 seconds\\nOutput:\\n\"},{\"type\":\"input_text\",\"text\":\"{\\\"chunk_id\\\":\\\"50b826\\\",\\\"wall_time_seconds\\\":0,\\\"exit_code\\\":2,\\\"original_token_count\\\":25,\\\"output\\\":\\\"check: mkdir...` [run: same file, ordinal 519]. The inner JSON is whatever the script printed. With `text(JSON.stringify(r))` it holds `chunk_id`, `wall_time_seconds`, `exit_code` or `session_id`, `original_token_count`, `output`. With `text(r.output)` it holds only the output text, so 0.144.1 to 0.146.1 rollouts mostly show no exit code. [run: 0.146.0 file `rollout-2026-08-03T23-44-14-019fc95e-b34a-7342-834c-c638dd5c4d3c.jsonl.zst`]
- `Script running with cell ID 65\nWall time 31.0 seconds\nOutput:\n`: the script is still running (a yield). The output after `Output:` was empty in every sample (all `Script running` lines in `~/.codex/sessions/2026/10` and `2026/11`; none longer than 70 characters). Line, ordinal 525 of the file above: the exec's own `call_id` is on this line, and the cell id appears here for the first time. [run]
- An inner JSON with `"session_id":15918` and empty output: the script ended at its yield time but the shell command is still alive (a unified-exec session). Example, `rollout-2026-11-13T22-00-52-01a1d67f-92f3-7512-a4ca-d0e1dcd46473.jsonl` ordinal 102: `{"chunk_id":"6a8cef","wall_time_seconds":1.002809167,"session_id":15918,"original_token_count":0,"output":""}`. [run]

A yielded cell is polled with a top-level `function_call` `wait`, arguments `{"cell_id":"65","yield_time_ms":30000,"max_tokens":30000}`, and a `function_call_output` with a different `call_id`. The output repeats `Script running with cell ID 65 ... Output:\n` until the script ends, then reads `Script completed\nWall time 18.2 seconds\nOutput:\n` followed by the script's output. The final line does not repeat the cell id, so a reader links it through the `wait` call's `call_id` and `cell_id` argument. [run: same file, ordinals 531 to 636: 20 polls for cell 65, ending at ordinal 636] `wait` also takes `terminate` (stops the cell). [source: `codex-rs/core/src/tools/code_mode/wait_spec.rs`]

The session-id style is polled by a second `exec` cell calling `tools.write_stdin({session_id:65578,chars:"",yield_time_ms:30000,...})`, or, in the pre-0.144 shape, by a top-level `write_stdin` call. A `\u0003` in `chars` sends Ctrl-C. [run: ordinals 209 and 304 of the 22-00-52 file]

Limits. `yield_time_ms` is clamped to 250 to 30,000 ms (an empty poll waits at least 5,000 ms), the unified-exec output buffer is 1 MiB, there is a cap of 64 processes, and the default `max_output_tokens` is 10,000. [source: `codex-rs/core/src/unified_exec/mod.rs`: `MIN_YIELD_TIME_MS`, `MAX_YIELD_TIME_MS`, `MIN_EMPTY_YIELD_TIME_MS`, `UNIFIED_EXEC_OUTPUT_MAX_BYTES`, `MAX_UNIFIED_EXEC_PROCESSES`, `DEFAULT_MAX_OUTPUT_TOKENS`]

### 2.2 Old shape (0.111.0 to 0.143.0): `exec_command` and `write_stdin`

- `function_call` `exec_command`, arguments `{"cmd":...,"workdir":...,"yield_time_ms":1000,"max_output_tokens":12000}`.
- Its `function_call_output` head: `Chunk ID: 24979e\nWall time: 1.0021 seconds\nProcess running with session ID 11710\nOriginal token count: 78\nOutput:\n<output so far>` while alive, or `...Process exited with code 0\nOriginal token count: 0\nOutput:\n` when done. The running line carries the output produced so far. [run: `rollout-2026-05-04T09-28-19-019df1ac-608c-7003-a482-7fb90dc64b8d.jsonl.zst`, 0.128.0, 2026-05-04T06:38:22]
- Polls: `function_call` `write_stdin` with `{"session_id":79477,"chars":"","yield_time_ms":1000,...}`.
- To 0.128.0 the end is also an `event_msg` `exec_command_end` with `call_id` equal to the original `exec_command` call, `process_id` equal to the session id, `command`, `cwd`, `exit_code`, `duration`, `aggregated_output`, `stdout`, `stderr`, `status`, `source: "unified_exec_startup"`. It is written when the process ends, before the agent polls: end at `06:38:23.036Z`, next poll at `06:38:25.471Z`. [run: same file, 310 such lines]

### 2.3 Is there a "start" line or a live output line?

No. In 0.169.0 the persistence policy lists `ExecCommandBegin`, `ExecCommandOutputDelta`, `TerminalInteraction`, `ItemStarted`, `ExecApprovalRequest` and the `Collab*Begin` events as `None` (not written). [source: `codex-rs/rollout/src/policy.rs`, `persisted_event_msg`] Only the model-facing tool call and its output lines, and the `ItemCompleted` item, reach the file. [run: no line in the 0.166.0 to 0.169.0 files names a started command]

What marks the start of a command:

| Moment | Line | Time source |
| --- | --- | --- |
| Agent asks | `custom_tool_call` `exec` (or `function_call` `exec_command`) | line `timestamp` |
| Process starts | not written. The `CommandExecution` item records `started_at_ms` when it is written at the end. | item field |
| Approval pending | not written. The `gx check` call at 17:15:21 (`require_escalated`) returned `Script running with cell ID 65`; its `CommandExecution` has `started_at_ms` = 17:25:21.2, `completed_at_ms` = 17:25:54.5 and `duration` 28.6 s (so the item's span is 33.3 s, 4.7 s longer than `duration`). Between 17:15:21 and 17:25:54 the rollout holds `wait` polls, agent messages and token counts, and no line about an approval. | [run: ordinals 523, 525 to 636, 635] |

So the call time and the item's `started_at_ms` can be ten minutes apart. [run]

## 3. The `CommandExecution` item (0.147.0 and later)

One example, `rollout-2026-11-13T18-35-47-01a1d5c3-e338-7d81-95d5-1c0f8193e2ba.jsonl`, ordinal 51 (found with `jq 'select(.payload.item.process_id=="22711")'`), trimmed:

```
{"type":"event_msg","payload":{"type":"item_completed","thread_id":"01a1d5c3-...","turn_id":"01a1d5c4-...",
 "item":{"type":"CommandExecution","id":"exec-38176c9e-...","process_id":"22711",
   "command":["/bin/zsh","-lc","pnpm -C webui build -- --sourcemap"],"cwd":"file:///Users/david/dev/gx/main",
   "parsed_cmd":[{"type":"unknown","cmd":"pnpm -C webui build -- --sourcemap"}],"source":"unified_exec_startup",
   "status":"completed","aggregated_output":"$ tsc --noEmit && vite build ...","exit_code":0,
   "duration":{"secs":2,"nanos":902406334}},
 "started_at_ms":1794587821967,"completed_at_ms":1794587824870}}
```

[run] Fields: `id` (`exec-<uuid>`, not the model's `call_id`), `process_id`, `command` (argv, last element is the command text), `cwd` (a `file://` URI), `parsed_cmd`, `source`, `status` (`completed` 98, `failed` 9 in that file), `aggregated_output`, `exit_code`, `duration`, and `started_at_ms`/`completed_at_ms` on the payload. Over the 0.166.0 to 0.169.0 plain files: 2,035 items; `aggregated_output` length p50 1,264, p90 18,054, max 65,536 characters. [run]

- Output size: the policy cuts `aggregated_output` to 64 KiB with `\n... command output truncated for persistence ...\n` in the middle. The marker is in 3 or more local rollouts. [source: `rollout/src/policy.rs`, `PERSISTED_COMMAND_OUTPUT_MAX_BYTES`; run]
- The item stands for a command, not a cell: a script that calls `exec_command` three times writes three items; a script with `Promise.all` of two `exec_command` calls writes two items that overlap in `started_at_ms`/`completed_at_ms`. [run: `rollout-2026-11-13T22-00-52-01a1d67f-92f3-7512-a4ca-d0e1dcd46473.jsonl`, the cell at ordinal 304 and the item at ordinal 306]
- A command can end with status `failed` and `exit_code` 143 without finishing on its own. In `rollout-2026-11-10T21-55-06-01a1c707-4cd6-7943-b0eb-6e3272d26d88.jsonl` three sleeps (`sleep 90` completed, exit 0, 90.0 s; two `sleep 60`, failed, exit 143, after 59.7 s and 42.9 s) were all written within 1.8 s of each other at 19:57:36 to 19:57:37, ordinals 59 to 61, which are the last lines of the file. The cause of the 143 is not in the rollout. [run; open]
- A process the agent walked away from (aborted poll, `aborted by user after 7.4s` on the `exec` output) kept running until the session ended. [run: same file, ordinals 19 to 21 and 59]

### 3.1 Matching a start to its end

- Old shape to 0.128.0: `exec_command_end.call_id` equals the `exec_command` call's `call_id`. Exact. [run]
- Current shape: the `CommandExecution` item has no `call_id`. Links that exist:
  - `process_id` in the item equals `session_id` in the inner JSON of a cell that returned while the command lived (`15918`, `65578`, `8367` in the 22-00-52 file, all matched). [run]
  - For a command that ended inside its first yield, the inner JSON has no `session_id`; only text and time match the exec call to the item. Over 2,035 items in the 0.166.0 to 0.169.0 plain files, a test of "the command text occurs in an `exec` input that started at most 120 s before `started_at_ms`" matched exactly one call for 1,852, more than one for 88, none for 95. [run: `match.py`; the none cases include scripts that build the command text in JavaScript]
  - A yielded cell's `wait` outputs link to the cell id; the cell's final `Script completed` line does not carry the cell id. [run]

## 4. Several commands at once

- Parallel inside one cell: `Promise.all([tools.exec_command(...), tools.write_stdin(...)])`. [run: ordinal 304 of the 22-00-52 file]
- Parallel across cells: three `sleep` sessions (`56016`, `45234`, `35355`) were alive at the same time, each started in its own cell, each returned `session_id` after a 1 s yield. [run: `rollout-2026-11-10T21-55-06-...`]
- Two cells running at once (both yielded, `wait`-polled): maximum seen is 2, in `rollout-2026-11-14T11-13-09-01a1d955-038c-70b3-8c4d-115421b55ea0.jsonl`. Of the plain files of `2026/10` and `2026/11`, 9 had a yielded cell, 29 yielded cells in all, none unfinished at the end of its file. [run: `cells.py`]
- Command overlap by `started_at_ms`/`completed_at_ms` (point-in-time count at each start) in the 108 plain files: 63 files with 0 or 1 command item (maximum 0), 22 with maximum 1, 11 with maximum 2, 12 with maximum 3. [run]
- The TUI's own counter reads "N background terminal(s) running · /ps to view · /stop to close". [source: `codex-rs/tui/src/bottom_pane/unified_exec_footer.rs`]

## 5. Per-command data

| Datum | Where | Versions |
| --- | --- | --- |
| Start time | `timestamp` of the `exec` / `exec_command` call line (agent asked); `started_at_ms` on the `CommandExecution` item (process started, written at the end) | call line: all. item: 0.147.0+ |
| End time | `timestamp` / `completed_at_ms` of the `exec_command_end` or `CommandExecution` line; otherwise the `timestamp` of the output line that reports it, which waits for the agent's next poll | `exec_command_end` to 0.128.0; item 0.147.0+ |
| Exit code | `exec_command_end.exit_code`; `CommandExecution.exit_code`; `Process exited with code N` in old output text; inner JSON `exit_code` only if the script prints it | see versions |
| Command text | `exec_command_end.command`; `CommandExecution.command` (argv) and `parsed_cmd`; the JavaScript `input` or the `cmd` argument of the call (the whole script, not only the command) | all |
| Full output | `aggregated_output` (64 KiB cap) in the end line; the tool's output text, cut by `max_output_tokens` | end line: see versions |
| Output size | `original_token_count` in the tool's output (old text head `Original token count: 78`, inner JSON `original_token_count`) | all with tool output |
| Last lines of output while running | not in the rollout. Old shape: the output text of a poll while running holds the output so far. Current shape: every `Script running` line seen was empty. | see §2 |
| Working directory | `workdir` argument; `cwd` in the end line | all |
| Approval pending | not written | all |

## 6. Subagents

Codex has subagents. Two generations exist in the local rollouts. [run]

### 6.1 v1 (0.115.0 to 0.142.5)

- Parent: `function_call` `spawn_agent`, arguments `{"agent_type":"default","model":"gpt-5.4-mini","reasoning_effort":"low","fork_context":true,"message":"..."}`. Output `{"agent_id":"019e12bb-7911-7b52-83fd-f43d1c57c2de","nickname":"Anscombe"}`. [run: `rollout-2026-05-10T16-45-16-019e1222-93cb-7be1-b506-1c5639917140.jsonl.zst`, 0.130.0]
- `wait_agent` arguments `{"targets":["<agent_id>"],"timeout_ms":30000}`; output `{"status":{"<agent_id>":{"completed":"<final message>"}}}`. `close_agent` exists from 0.133.0. [run]
- The child has its own rollout. Its `session_meta` carries `source.subagent.thread_spawn.{parent_thread_id,depth,agent_path,agent_nickname,agent_role}` and `forked_from_id`. [run: `rollout-2026-05-10T19-32-16-019e12bb-7911-...jsonl.zst`]

### 6.2 v2 (0.144.1 and later)

- Tools: `spawn_agent {task_name, fork_turns, message}` (0.147.0 samples add `model`, `reasoning_effort`), `followup_task`, `send_message {target, message}`, `interrupt_agent`, `list_agents {}`, `wait_agent {timeout_ms}`, `close_agent`. The `message` text is encrypted (`gAAAAA...`) in every sample from 0.144.1 on. [run]
- `session_meta.multi_agent_version` is `"v2"` in some sessions, `"disabled"` in others (plain files, 108 in all: 10 `v2`, 26 `disabled`, the rest unset). [run]
- `spawn_agent` output: `{"task_name":"/root/web_perf_research"}`. `list_agents` output: `{"agents":[{"agent_name":"/root","agent_status":"running"},{"agent_name":"/root/web_perf_research","agent_status":"running"}]}`. A finished agent shows `"agent_status":{"completed":"<final message>"}`. `wait_agent` output: `{"message":"Wait completed.","timed_out":false}` or `{"message":"Wait timed out.","timed_out":true}`. `wait_agent` timeouts are clamped to a 10,000 ms minimum (`"Requested timeout of 1000ms was clamped to the minimum of 10000ms."`). [run: `rollout-2026-11-13T18-35-47-01a1d5c3-...jsonl`, ordinals 1109 to 1333]
- Events in the parent rollout (`event_msg` / `item_completed`):
  - `SubAgentActivity`, `kind` one of `started`, `interacted`, `completed`, with `agent_thread_id` (the child's session id), `agent_path` and, on `started` only, `model` and `reasoning_effort`. Example: `{"type":"SubAgentActivity","model":"gpt-5.6-sol","reasoning_effort":"medium","id":"call_8454...","kind":"started","agent_thread_id":"01a1d640-0c00-7d60-a436-e1a2974961c8","agent_path":"/root/web_perf_research"}`; the `completed` event id is `subagent-completed-<child turn id>`. [run: ordinals 1111, 1253, 1322, 1328]
  - `CollabAgentToolCall` (`tool: "wait"`, `status`, `sender_thread_id`, `receiver_thread_ids`, `agents_states`) with `started_at_ms` and `completed_at_ms` giving the wait's span. In the samples `receiver_thread_ids` and `agents_states` were empty. 0.147.0 and later only. [run: ordinals 1203 to 1283]
  - `response_item` `agent_message` with `author`, `recipient` and a body whose first text part reads `Message Type: MESSAGE` or `FINAL_ANSWER`, `Task name: /root`, `Sender: /root/web_perf_research`; the payload is encrypted except for `FINAL_ANSWER`. It is preceded by an `inter_agent_communication_metadata` line `{"trigger_turn":false}`. [run: ordinals 1286 to 1333]
- Counts of `SubAgentActivity` items: 0.144.1: 7, 0.145.0: 3, 0.146.0: 113, 0.147.0: 5, 0.166.0: 28, 0.167.0: 10, 0.169.0: 6. [run]
- Child rollout, example `rollout-2026-11-13T20-51-24-01a1d640-0c00-7d60-a436-e1a2974961c8.jsonl` (241 lines, 983 KB):
  - `session_meta` (line 1): `forked_from_id`, `parent_thread_id`, `source.subagent.thread_spawn {parent_thread_id, depth:1, agent_path:"/root/web_perf_research", agent_nickname:"Carver", agent_role:null}`, `thread_source:"subagent"`, `subagent_history_start_ordinal:21` (the first 21 lines are copied parent history, including a second `session_meta` with the parent's id).
  - Model and effort: `turn_context` `model:"gpt-5.6-sol"`, `effort:"medium"`.
  - Tokens: `event_msg` `token_count` with `info.total_token_usage` (`input_tokens`, `cached_input_tokens`, `output_tokens`, `reasoning_output_tokens`, `total_tokens`) and `last_token_usage`. The totals start at the child's own first call (39,286), not at the parent's. Last line: `total_tokens` 3,174,164 (3,088,256 cached input), at 19:03:13.
  - Current step: the child's own `response_item` lines (`function_call`, `custom_tool_call`, `reasoning`, `message`) and `event_msg` `item_completed` items.
  - Status: `task_started`, `task_complete` (with `last_agent_message`, `duration_ms`) and `turn_aborted` events; a finished child ends in `task_complete`.
  - The file name holds the child's session id, so the path is found by that id under `~/.codex/sessions/<yyyy>/<mm>/<dd>/` (same lookup as `codexsession.RolloutPathIn`). The parent line that gives the id is `SubAgentActivity.agent_thread_id`, or `agent_id` in v1 `spawn_agent` output. [run]
- Other subagent-source rollouts exist that the agent did not spawn: `source: {"subagent":{"other":"guardian"}}`, `thread_source: "guardian_review"`, 0.169.0 files such as `01a1d6af...`, `01a1d955...`, `01a1d67f...`, `01a1d6c6...`. [run: header scan of `2026/11`]

## 7. Cost of reading

### 7.1 Sizes

- 108 plain rollouts, 272 MB in all: min 47 KB, p25 157 KB, p50 306 KB, p75 1.03 MB, p90 3.0 MB, p95 4.0 MB, max 99 MB. [run: `stat`]
- The largest, `rollout-2026-10-30T10-29-54-01a18bee-0479-7a30-8723-051454935700.jsonl` (0.166.0), is 99 MB over 1,189 lines. Its longest line is 6.0 MB; `rollout-2026-11-05T10-50-36-01a1aae7-1f00-7641-9b4e-e05afd0f2e3e.jsonl` has 33 lines and a 3.0 MB line. A busy 0.169.0 session, `rollout-2026-11-13T18-35-47-...jsonl`: 4.0 MB over 1,526 lines, p50 line 873 bytes, p99 32 KB, max 126 KB. [run]
- Bytes in that 4.0 MB file by line type: `item_completed` 1.12 MB (393 lines), `custom_tool_call_output` 0.98 MB, `reasoning` 0.68 MB, `message` 0.18 MB, `token_usage_record` 0.18 MB, `token_count` 0.17 MB, `compacted` 0.13 MB, `turn_context` 0.12 MB. [run]
- Each command's output is written twice in 0.147.0 and later: once in the tool output line and once in `aggregated_output`. [run: both present in the `gx check` example]

### 7.2 Append-only and incremental reads

- The recorder opens the file with append, writes each line plus `\n` and flushes, and writes a leading `\n` first if the last byte of a resumed file is not one. [source: `codex-rs/rollout/src/recorder.rs`, `JsonlWriter::write_line`, `.append(true)`]
- A thread revert writes a new rollout file named `rollout-<timestamp>-<thread id>_<rollout id>.jsonl` rather than rewriting. [source: `recorder.rs`, `rollout_id_override`]
- `ordinal` is the line index (0-based) in every sampled file of 0.142.3 and later (all of 34 files of 0.146.0, 35 of 0.147.0, 52 of 0.166.0, 9 of 0.167.0, 39 of 0.169.0), and in 3 of 8 sampled 0.130.0 files; older files have no `ordinal` field. [run: `ordinal.py` over a one-in-three sample plus every `2026/10` and `2026/11` file]
- gx today: `server/sessions/chat/cache.go` `Cache.Read` returns a cached snapshot while the file's size and mtime are unchanged, and otherwise runs `chatreader.ReadFile` plus `codexsession.ReadInFlightTools`, each of which reads the whole file with `transcript.ForEachLine` (`transcript/chat.go`, 64 KiB buffered reader, whole-line `json.Unmarshal`). There is no offset-based read. [source: gx]
- A run of gx's own `ReadInFlightTools` over the 99 MB file took 0.34 s wall, 0.10 s user, 33 MB peak RSS (warm page cache, one run). Over the 4.0 MB and 7.6 MB files it took 0.00 to 0.01 s. [run: a scratch `main` importing `codexsession`, `/usr/bin/time -l`]

### 7.3 Opt-in Codex trace

Codex can write a separate trace bundle (`manifest.json`, `trace.jsonl`, `payloads/*.json`) when `CODEX_ROLLOUT_TRACE_ROOT` is set. It records terminal operations, code cells and multi-agent edges live, and `codex debug trace-reduce <bundle>` reduces it. Off by default; no bundle exists locally. [source: `codex-rs/rollout-trace/README.md`; run: no such directory found under `~/.codex`]

### 7.4 Live app-server calls (not used by gx)

The experimental app-server v2 protocol has `thread/backgroundTerminals/list` (returns `item_id`, `process_id`, `command`, `cwd`, `os_pid`, `cpu_percent`, `rss_kb` per terminal), `thread/backgroundTerminals/terminate`, `thread/backgroundTerminals/clean`, and the notifications `item/started`, `item/completed`, `item/commandExecution/outputDelta`. [source: `codex-rs/app-server-protocol/src/protocol/common.rs` and `codex-rs/app-server-protocol/src/protocol/v2/thread.rs` at `rust-v0.169.0`] `~/.codex/app-server-control/app-server-control.sock` and `~/.codex/ipc/ipc.sock` exist on this machine. gx's tree has no reference to `backgroundTerminals`; gx runs the Codex TUI in a PTY. [run: `rg` over the design worktree]

## 8. What gx reads today

| Reader | What it reads | Notes |
| --- | --- | --- |
| `codexsession/inflight.go` `ReadInFlightTools` | `function_call` and `custom_tool_call` lines of the current turn (after the last `task_started`) with no `function_call_output` / `custom_tool_call_output` of the same `call_id`, newest first. Label from the call's `arguments` JSON, else the first line of `input`. | Called by `server/sessions/chat/cache.go:92`. Tested with the pre-0.144 `exec_command` shape (`codexsession/inflight_test.go`). |
| `codexsession/chat.go` `ReadChat` | message, tool-run, `turn_aborted`, `task_complete`, `compacted` lines | Tool runs fold into `ToolCount`, `LastTool`, `Calls`. No `item_completed`. |
| `harness/codexwatch.go` | OSC 7501 state and the window title. `Background = (state working && title == "Ready")`. | `Status().Background`. |
| `ralphloop/supervise/backgroundhold.go` `BackgroundTaskLive` | Codex: `Runner.Status().Background`. | `WaitForBackgroundTasks` (`waitforfinish.go:658`) returns at once for any agent but Claude. |

Behavior of `ReadInFlightTools` on real 0.169.0 rollout prefixes (`rollout-2026-11-13T18-35-47-...jsonl`, cut after N lines, run through a scratch program that imports the package): [run]

| Cut at | Moment | Result |
| --- | --- | --- |
| line 524 | `exec` call just written (`gx check` cell) | one call, `{Kind: other, Label: exec}` |
| line 526 | cell yielded: `Script running with cell ID 65` written | no calls |
| line 532 | a `wait {cell_id:"65"}` poll open | one call, `{Kind: other, Label: wait}` |
| line 1112 | `spawn_agent` output written | one call, `{Kind: other, Label: spawn_agent}` |

While a `wait` poll is closed and no new poll has started, nothing is in flight. [run]

## 9. Stability across versions

- Line envelope `{"timestamp", ("ordinal"), "type", "payload"}` is the same in every file. `ordinal` and a `metadata` object (`history_sequence`, `sampled_through_sequence`, ...) appear on later versions. [run]
- Tool shape changed once, at 0.144.1 (`exec_command` + `write_stdin` to JavaScript `exec` + `wait`), and the 0.166.0 to 0.169.0 files use the same `exec` shape. [run]
- The command-result line changed three times: `exec_command_end` (0.118.0 to 0.128.0, none before), none (0.129.0 to 0.146.1), `CommandExecution` item (0.147.0 on). [run]
- Subagent tools changed at 0.144.1 (v1 id-based to v2 path-based, encrypted message). `SubAgentActivity` first appears in 0.144.1 files, `CollabAgentToolCall` in 0.147.0. [run]
- Text heads of tool outputs differ by shape: `Chunk ID: ...\nWall time: 1.0021 seconds\nProcess running with session ID N\n...` (old, with a colon) and `Script running with cell ID N\nWall time 31.0 seconds\nOutput:\n` (current, no colon). [run]
- The version-bound `CommandExecution` and `SubAgentActivity` types are `TurnItem` variants of Codex's `ItemCompleted` event. The paginated-history persistence policy that writes them is Codex code, and `rollout/src/policy.rs` shows no stability promise. [source]
- No Codex build between 0.147.0 and 0.166.0 has a rollout here. [run]

## 10. Not confirmed

- Whether approval waits ever leave a line in the rollout (none was seen in the one 10-minute wait and in the gx check examples). [open]
- The persistence policy of 0.129.0 to 0.146.1; the table in §1 is the observed result, the cause is read from 0.169.0. [open]
- `agents_states` and `receiver_thread_ids` of `CollabAgentToolCall` hold values for `wait_agent` calls with targets; the samples are all empty. [open]
- v1 `wait_agent` status for a still-running child (only `completed` seen). [open]
- Cold-cache read times. [open]
