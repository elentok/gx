# Subagents: a read-only chat page for each subagent

## Problem Statement

When an agent runs subagents, the web UI shows one line per subagent. The task row (`TaskList`) shows the label, the time and, when opened, the model, the token total, the tool-call count and the current call. The parent chat log shows the call as one line in a folded "tools" entry. The person cannot read the prompt the subagent got, what it did, which tools it called or what it handed back. Once the task group ends, the row disappears and nothing is left to click. The Claude chat reader also drops every sidechain line, so a Claude subagent file renders as an empty chat today.

## Solution

Each subagent, Claude or Codex, gets a read-only chat page. The page shows a header, the prompt, the messages, the tool calls and the final result, and it updates live while the subagent runs.

- The page is reached from two places. Each subagent task row gets an "Open ↗" link (a click on the row still expands its inline details). Each subagent call in a parent chat log is a link ("Agent · <type> · <description> ↗"). The call link stays after the task group ends, because the call stays in the chat.
- The links show on the Agents page and on the step pages (Design, Initiative, Triage), because both render the same chat components.
- The URL is `/agents/<session>/subagents/<call id>`. The call id is the Claude `toolUseId` or the Codex `call_id`. Both exist in the parent from the first moment, so the link works while the subagent runs, after it ends and after the group closes.
- The header has the title, the type chip, the status chip, the model, the time, the tokens, the tool-call count, a "Back to parent chat" link and a breadcrumb. A stat or chip with no value is left out. A subagent with no known duration (it ended by an exited session, a new run or the age-out) has no time.
- Below the header: a boxed "Prompt from the parent agent", then the subagent's own messages and tool calls, then a green "Result handed back to the parent" card. While it runs there is a "Working" row with the current call.
- A Claude subagent's own subagent calls are links of the same shape, at any depth. A back link goes up one level. A Codex child's own subagent calls (Codex grandchildren) show no link, see Out of Scope.
- The TUI does not change. Sending a message to a subagent is not possible.

Look: the stored mock (see Mock states). The mock is Claude-only. Its a-parent task list and tools entry were compared with the real ones in the `bg-progress` fixture (results in the Resolution of map ticket `gx:subagents/02`) and the mock was corrected where it differed. "Open ↗", the call links and the header chips do not exist in the app yet, so they have no pair. This spec replaces the mock's original "file gone" wording with the text below.

## User Stories

1. As a person watching an agent, I want to open a subagent from its task row, so that I can read what it was asked and what it does.
2. As a person, I want the task row's "Open ↗" link to leave the row click alone, so that expanding the row's inline details still works.
3. As a person reading the parent chat log, I want each subagent call to be a link, so that I can open the subagent without finding its task row.
4. As a person, I want the call link to keep working after the task group has ended, so that I can read a finished subagent later.
5. As a person on the Agents page, I want the links on the task row and on the call, so that I can reach the subagent from where I read the chat.
6. As a person on a Design, Initiative or Triage step page, I want the same links, so that I do not have to switch pages to read a subagent.
7. As a person, I want the subagent page at a stable URL, so that I can reload it, bookmark it and use the browser Back button.
8. As a person, I want the browser Back button to return me to the step page I came from, so that opening a subagent does not lose my place.
9. As a person, I want the page header to show the title, type, status, model, time, tokens and tool-call count, so that I see what this subagent is and how it is doing. This holds for a foreground Claude subagent while it runs.
10. As a person, I want the title to be the same label the task row shows, so that I recognise the subagent in both places.
11. As a person, I want a header chip or stat with no value to be left out, so that a Codex child with no role or no tool-call count shows no empty chip.
12. As a person, I want the prompt in a boxed block labelled "Prompt from the parent agent", so that I can tell what the subagent was asked.
13. As a person reading a Codex child, I want "Prompt not readable" in the prompt block, so that I know the prompt is encrypted and not missing by mistake.
14. As a person, I want the subagent's messages and tool calls in the same layout as a normal chat, so that I read them the way I already do.
15. As a person watching a running subagent, I want the page to update by itself, so that I see new messages and calls without reloading.
16. As a person, I want a "Working" row with the current call while the subagent runs, so that I see what it does right now.
17. As a person, I want the final result in one green card at the end, so that I find the answer at once, for Claude and Codex alike.
18. As a person, I want the Claude handback call not listed among the tool calls, so that the result is not shown twice.
19. As a person, I want a Codex child's last assistant message shown as the result, so that Codex children read the same way.
20. As a person, I want a status chip with Running, Done, Stopped or Not available, so that I read the outcome without reading the chat.
21. As a person, I want a subagent that ended with no handback to show a muted "Stopped" and no green result card, so that gx does not claim a result that does not exist.
22. As a person, I want a background subagent to stay Running after its call returns, until it really ends, so that the status is true.
23. As a person, I want the status to be the same whether or not the task group is open, so that closing the group does not change a subagent's status.
24. As a person, I want a subagent of an exited session to show Stopped, so that nothing shows Running for a dead session.
25. As a person, I want "Starting…" while the subagent's file is not found and the call still runs, so that I know to wait.
26. As a person, I want "Transcript no longer available" with status "Not available" when the file is gone and the call is not running, so that I know why the page is empty.
27. As a person, I want the header to keep working when the file is missing, from the parent's call, so that I still see the title, type and status. For a Claude subagent I also see the prompt from the parent's call.
28. As a person opening a Codex child that exists only as a compressed `.jsonl.zst`, I want the page to say the file is compressed and that reading compressed children is not supported yet, so that I know what is missing and what to build.
29. As a person, I want a Codex child to show only its own lines and not the copy of the parent's history at the start of its file, so that the page is not full of the parent's conversation.
30. As a person, I want a nested subagent call on a subagent page to be a link of the same shape, so that I can follow a chain of subagents.
31. As a person, I want "Back to parent chat" to go up one level for a nested subagent, so that I walk the chain back in order.
32. As a person, I want the top-level back link and the breadcrumb to go to `/agents/<session>` and name the session, so that the way back is the same from every page.
33. As a person on a phone, I want the header to wrap and the page to have no sideways scroll, so that I can read a subagent on a 390px screen.
34. As a person, I want the page read-only, so that I cannot send a message to a subagent by accident.
35. As a gx operator, I want no new subagent file reads outside a `subagent=` read and the task rows' existing detail reads, so that a subagent nobody opens costs nothing new.
36. As a gx operator, I want each subagent file parsed again only when its size or change time moves, so that watching one costs a stat, not a parse.
37. As a script author, I want `gx agents chat <session> --subagent <call-id>` to print the same content as the page, so that the CLI and the server stay in parity (ADR 0028).
38. As a person using a TUI, I want nothing to change, so that nothing I rely on there moves.
39. As a person, I want an unknown call id to give a clear "no such subagent" result, so that a stale link does not show a broken page.
40. As a person, I want a Claude subagent that has a finished marker but no handback to show Stopped with its last assistant message as a muted result, so that I still find what it said without the page claiming it handed back a result.
41. As a person, I want a Codex child to end by the same rules as a task row (the "completed" event, the `wait` completion, an exited session, a new run, the age-out), so that a Codex child whose spawn call returned at once is not Stopped by mistake.
42. As a person, I want the page of a Starting subagent (Claude or Codex) to fill in by itself when its file appears, even though the parent writes nothing while a foreground call runs.
43. As a person opening an old link whose parent transcript is gone or compressed, I want "Transcript no longer available" and not "no such subagent", so that I know the link was once good. For a compressed (archived) parent I want to be told the parent can be decoded from the chat page.
44. As a person, when the subagent's file is missing but the parent is readable, I want the Starting and Not available pages of a Claude subagent to show the prompt from the parent's call, so that I still see what it was asked. A Codex child shows "Prompt not readable". When the parent is gone or compressed, the page shows no prompt and no header values, because the parent's call cannot be read.
45. As a person following a nested Claude subagent from its call id alone, I want the breadcrumb to name every level above it, so that I see the whole chain.
46. As a person, I want a task row with no call id to have no "Open ↗" link, so that no link points nowhere.

## Implementation Decisions

**Route and CLI (decided in map ticket 05)**
- No new route. `GET /v1/sessions/chat` takes an optional `subagent=<call id>`, with the root session in `session=`. `gx agents chat` gets `--subagent <call-id>`. The CLI is a thin client of the route (ADR 0028). The TUI is unchanged.
- An unknown call id (no such call in the session) is answered like an unknown session on `GET /v1/sessions/chat`: HTTP 404. ADR 0028 refusals are for writes, and `SessionChatResponse` has no `refused` field. The CLI exits non-zero and prints the message. The 404 body carries the message: "no such subagent" for an unknown call id, or the existing unknown-session message for an unknown session. The page and the CLI show the message from the body.
- A known session whose parent transcript cannot be read (deleted after `cleanupPeriodDays`, or a Codex parent that exists only as `.jsonl.zst`) is not an unknown call. The read answers with status "Not available" and the reason `gone` (deleted) or `compressed` (archived). The page shows "Transcript no longer available" and the back link. It shows no header values and no prompt, because the parent's call cannot be read. For `compressed` it adds a line saying the parent session is archived and can be decoded from the chat page (`/agents/<session>`, which has the Decode route `/v1/sessions/chat/archive`). It does not say "not supported", which is the text of a compressed child.
- The web route `/agents/*` already takes a splat. The Agents page route parses `<session>/subagents/<call id>` out of it and renders the subagent page. The step pages link to the same URL. They do not render the subagent page themselves.

**Reading a subagent**
- The server maps the call id to the file on each read, from the parent's records. Claude background call: the launch result's agent id and output file (already on the task record). Claude foreground call, and any Claude call found by its id alone: the agent id shows only in the final `tool_result`, so the server finds the file through the flat `subagents/*.meta.json` files and their `toolUseId` (the file exists from spawn, without `finished`, per ticket 03). This lookup lives in one place. Its result (path, agent id, `parentAgentId`, type, finished marker) feeds the page, the breadcrumb and `withSubagentDetails`, so a running foreground subagent has the same model, tokens and tool-call count in the task row and on the page. Codex: the parent's `SubAgentActivity` "started" event, whose `id` is the call's `call_id`, names the child thread; the child rollout is found by thread id.
- Nesting for Claude: the URL holds only the leaf call id. The `toolUseId` lookup finds a call at any depth, and the breadcrumb chain is built by following `parentAgentId` up to the root session. Each level's title comes from the call as recorded in the file that holds it. Codex grandchildren are out of scope: the server does not look inside a Codex child for further calls, and a nested Codex call has no link.
- The Claude subagent type and the finished marker come from `meta.json`. The Codex role comes from the child's `session_meta` (`agent_role`, null when the call passed no type).
- One "subagent file" reader option, not a global change, keeps sidechain lines. It applies to all three readers that drop them today: the chat reader, `ReadTaskRecords` and the in-flight reader (`transcript/inflight.go`). Without it, a nested Agent call in a subagent file gives no record (no nested link, label or status) and the Working row has no current call. The normal reads of a parent transcript keep dropping sidechain lines. In the chat reader the option also keeps the final `SubagentHandback` call out of the tool list and returns its message as the result.
- A second reader option cuts a Codex child at `subagent_history_start_ordinal`: the lines before it are a copy of the parent's history and are not shown. The child's prompt is an encrypted `agent_message`, so the page shows "Prompt not readable" and never the `NEW_TASK` header. The result of a Codex child is its last assistant message. The same rule gives the muted result of a Claude subagent that has a finished marker and no handback (status Stopped).
- Title: the task row's existing label, for both runners. After the group has closed there is no task row, so the label comes from the same record the row is built from (the parent's call).
- Header values: model, tokens and tool-call count come from the same readers the task row uses today (`withSubagentDetails`, `withCodexChildren`), fed with the file path from the single lookup above, so the row and the page never disagree. Time is the call's start to its end (or to now while running). A subagent with no duration (the `NoDuration` case: exited, earlier run, aged out) has no end time and no Time stat, as in the task row.
- A header chip or stat with no value is omitted. A null Codex role gives no type chip. A Codex child with no tool-call count shows no "Tool calls". The prompt of a Claude call is plain text in `input.prompt` of the parent's call, so the Starting and Not available pages show it. A Codex child shows "Prompt not readable" there.

**Status**
- Done: a handback (Claude), or the parent's "completed" event or a `wait_agent`/`list_agents` output that names the child completed (Codex). Both Codex signals set the same end in the rollout scan.
- Stopped: no handback (Claude) or no completion (Codex), and the subagent ended. A Claude subagent with a finished marker and no handback is Stopped too: its result is its last assistant message, shown as a muted result and not in the green card. A foreground call ends when the parent's call returns. A background Claude subagent ends by the task row's end rules in `resolveTask` (a matched notification or end, an exited parent session, a new run, the age-out), so it does not end when its call returns. A Codex child ends by the task row's rules (the "completed" event, the `wait` agents-status completion, an exited session, a new run, the age-out), not when the `spawn_agent` call returns, because that call returns at once. A "completed" event or a completed `wait` status gives Done. The other ends give Stopped. An exited parent session ends any subagent. Stopped shows a muted chip and no green result card.
- Running: everything else.
- The status never depends on whether the task group is open. The server derives it on every read from the parent's records and the session state. It reuses the `resolveTask` rules and does not keep a second copy of them. For a nested subagent the same rules apply to the call as recorded in the file that holds it, with the root session's exited state.
- Not available: the file is not found and the subagent does not count as running (for a background Claude subagent or a Codex child, "does not count as running" is the `resolveTask` result). A Codex child whose rollout is not found yet, or not linked yet by the "started" event, is not Not available while `resolveTask` counts it as running, because the `spawn_agent` call returns at once.

**Missing file and compressed child (decided in map ticket 06)**
- File not found while the call runs (a background Claude subagent or a Codex child: while `resolveTask` counts it as running, until the task row's end rules say it ended): the page shows "Starting…" (reason `starting`). Otherwise it shows "Transcript no longer available" with status "Not available" (reason `gone`). The header still comes from the parent's call (title, type, status), and for Claude the prompt block shows the prompt from the parent's call. A Codex child with no rollout yet is Starting by the same rule. While a page is open on a Starting subagent, the watch also looks for the file (the `meta.json` lookup or the child rollout lookup) and sends the subagent event when it appears. The page does not depend on the parent's `session-chat-changed` for this, because the parent writes nothing while a foreground call runs. This replaces the mock's "no longer on disk" wording. The mock is Claude-only, so the text names no runner.
- A Codex child that exists only as `.jsonl.zst` is treated as not available (reason `compressed`). The page shows "Transcript no longer available" and a line saying the file is compressed and that reading compressed children is not supported yet. There is no Decode button. A later change can add decoding. Today's parent-chat archive decode does not apply to children.

**Wire**
- A subagent call inside a "tools" chat entry must be marked so the web can render a link instead of plain text. The wire shape of the mark (for example a per-call object next to the existing `calls` strings, or a parallel list) is the implementer's choice. It carries the call id, the type and the description. It is set only for calls the server can resolve: Claude Agent calls at any depth, and Codex spawn calls in the root session. A Codex call inside a Codex child has no mark. It must not change what clients that read only `calls` see.
- When `subagent=` is set, the chat response carries a subagent header: title, type, status, model, time (start and end), tokens, tool-call count, the parent call id and enough to build the breadcrumb (the root session, and the chain of subagent call ids and titles above this one). The exact field names and the shape are the implementer's choice. The messages, tool calls, "Working" row data and result use the existing chat response shapes where they fit. Optional header fields are absent when they have no value. A "not available" reason (`starting`, `gone`, `compressed`) distinguishes the page texts.
- `TaskRecord` gets a call id field for Claude and Codex, and `wire.SessionTask` carries it so that the row can link. A background launch result with no tool_use in the transcript makes a record with no call id. That row shows no "Open ↗" link. Whether the wire also carries the agent id or file path is the implementer's choice; the browser needs only the call id.

**Live updates and cost (decided in map ticket 04)**
- gx watches a subagent file only while a browser has its page open. The watch starts and ends through a web-only browser write listed in `WebOnlyRoutes` (ADR 0028, like the push routes), keyed by the page's event-stream client id (the `?client=` id the focus report uses). It has no CLI verb. The write refuses with named reasons, which are a public contract like the push routes' reasons: `subagent-watch-unknown-client` (the client id has no open event stream) and `unknown-subagent` (the call id is not in the session). The watch ends when that client's event stream disconnects or the page reports it closed. The SSE event name and the route path are the implementer's choice. A subagent in Done, Stopped or Not available state is no longer watched, except that a Starting page keeps the file lookup going until the file appears.
- The page refetches on the subagent event and also on the parent's `session-chat-changed`, which is how it learns that a status changed. `chat.Poll` keeps watching only the parent transcript for all other sessions.
- No new subagent file reads happen outside a chat read with `subagent=`. The existing reads stay: `withSubagentDetails` and `withCodexChildren` read subagent files on a parent chat read with an open task group. The reads go through the existing size-and-mtime file cache. A finished subagent's file is not parsed again.
- How the call-id-to-file lookup is cached is the implementer's choice. It must not rescan the `subagents/` directory on every read of an unchanged file, and it must notice a file that appears later.

**Web**
- The task row (`TaskList`) gets an "Open ↗" link for subagent rows that have a call id. The row click still toggles the inline details.
- The chat log renders a marked subagent call as "Agent · <type> · <description> ↗", linking to the page. The type part is omitted when empty.
- The subagent page reuses the chat components for messages, tool calls and the "Working" row. It adds the header, the prompt block, the result card, the not-available note and the breadcrumb. It shows the message from the 404 body ("no such subagent" or the unknown-session message). For a known session with an unreadable parent it shows the not-available text from the reason.
- The top-level back link always goes to `/agents/<session>`, also from a step page. The browser Back button returns to the step page. A nested subagent's back link goes to the parent subagent's page first.
- Phone width follows the same responsive rules as the chat views. The header wraps and the page has no sideways scroll at 390px. There is no 390px mock state.

**Quick-fix choices recorded**
- None. The compressed child is not decoded because decoding is a feature of its own and is listed under Out of Scope. The page says so in its text.

**CLI output**
- `gx agents chat <session> --subagent <call-id>` prints plain text in this order: a header line (title, then each present value of type, status, model, time, tokens and tool calls, separated by ` · `), the prompt (or "Prompt not readable", or the parent call's prompt for a Starting or Not available Claude subagent whose parent is readable), one line per message as `gx agents chat` prints them, then the result, or the not-available text ("Starting…", "Transcript no longer available", the compressed-file line, or for an archived parent the line that it can be decoded from the chat page). With an unreadable parent there is no header values line and no prompt. `--json` prints the chat response. An unknown call id exits non-zero and prints the message from the 404 body.

**Implementer's choices (summary)**
- The wire shape marking a subagent call in a "tools" entry.
- The shape of the subagent header on the chat response, including the parent call id and the breadcrumb chain.
- The SSE event name and the path of the watch write.
- How the call-id-to-file lookup is cached.
- The names of the reader options that keep sidechain lines and cut a Codex child at `subagent_history_start_ordinal`.

## Testing Decisions

- A good test checks external behavior: what the chat read returns and what the page shows. It does not check internals, timing or the shape of a cache. No timing assertions.
- Highest seam, server: the chat read with `subagent=<call id>` for a fixture session, driven by fixture transcripts, `meta.json` files and rollouts. It is where status, header, prompt, messages, result and the missing-file texts are asserted for Claude and Codex. Prior art: `server/sessions/chat` internal tests (`subagentdetails`, `codexchild`, `taskcost`, `read`).
- Cost seam: the existing file cache with an injected counting compute function. Assertions: two reads of an unchanged subagent file parse it once; after it grows it parses again; a finished subagent's file is not parsed again; with no `subagent=` read and no open task group, no subagent file is opened; a `subagent=` read adds no reads beyond its own. Prior art: the cache tests in `taskcost_internal_test.go`.
- Reader fixtures for each awkward case: a foreground Claude subagent mid-run (`meta.json` with `toolUseId`, no `finished`), a background one whose parent call returned long ago, a handback, a finished marker with no handback (Stopped, muted result), no finished marker and no handback, a nested Claude subagent (grandchild file flat in the same directory, with `parentAgentId`, found by its leaf call id), a nested Agent call in a subagent file seen by `ReadTaskRecords` and the in-flight reader, a Claude parent transcript that is gone, a Codex parent that exists only as `.jsonl.zst`, a task record with no call id, a Codex child ended by each task row end rule (including the `wait` completion), a Codex child with no rollout yet (Starting), a Codex child with a history copy before `subagent_history_start_ordinal` and an encrypted prompt, a Codex child with null role, a compressed-only Codex child, and a missing file. The foreground reader fixture also checks that the task row and the page show the same model, tokens and tool-call count while it runs.
- Web seam: the subagent page component with fixture responses, for each state in Mock states, the omitted chips, the call link and the row link, and the nested back link. Prior art: `TaskList.test.tsx`, `ChatLog.test.tsx`. It includes a row with no call id (no "Open ↗") and a 404 (the body's message, for an unknown subagent and for an unknown session), and a known session with an unreadable parent (gone, and compressed with the decode line, with no header values and no prompt).
- Watch seam, server (Go): with a Starting page watch open, writing the `meta.json` and the subagent file (or the Codex rollout) sends the subagent event with no write to the parent transcript. Also the two refusal reasons of the watch write. The web seam refetches on that event.
- CLI parity: the existing `cmd/server_parity_test.go` diff of the route table against the command tree must stay green with the `--subagent` flag.
- E2E: one new flow file on the `subagents` scenario, at 1280px and 390px, holds every step listed under Verification as new. Flow 208 "Task list" (`task-list.e2e.ts`, `bg-progress`) is updated for the "Open ↗" row link. The flow budget and e2e skill rules apply. Run Go tests with `scripts/proc-cap`.

## Verification

Entries refer to Mock states by state name and do not repeat the look check. "Chat read" is `GET /v1/sessions/chat?session=<id>&subagent=<call id>` on the temporary server, called with the ids of the `subagents` fixture. The CLI check is the same read through `gx agents chat <session> --subagent <call-id> --json`, hand-run with `go run . agents chat ...` because the installed `gx` is older than the worktree. Each CLI check expects exit code 0 and the same content as the chat read, except the unknown-id check, which expects a non-zero exit.

Task row and call links (stories 1 to 6, 30 to 30, 46 to 46)
- Mock states "a-parent". Flow 208 "Task list" (`task-list.e2e.ts`, `bg-progress`, updated) on the Design and Agents pages: the subagent task row has "Open ↗" and clicking the row still expands the details.
- New flow on `subagents` (the one new flow file): the call in the folded tools entry is a link on the Agents page, the Design step page and one other step page (Initiative or Triage). After the task group has ended, the call link is still there and opens the page.
- Same flow: on a subagent page, a nested Claude subagent call is a link of the same shape. A nested call inside a Codex child has no link.

URL and navigation (stories 7 to 8, 31 to 32, 39 to 39, 43 to 43, 45 to 45)
- Same flow: open a subagent from a step page; the URL is `/agents/<session>/subagents/<call id>`; "Back to parent chat" goes to `/agents/<session>`; the browser Back button returns to the step page. Reloading the subagent page shows the same page (declare the reload).
- Same flow: a nested page's back link goes to the parent subagent, then to the session. The breadcrumb names the session and every level above the page, built from the leaf call id alone.
- Chat read with an unknown call id: HTTP 404 with the body "no such subagent", and the page shows it. With an unknown session the 404 body is the unknown-session message and the page shows that, not "no such subagent". CLI: `--subagent nosuch` exits non-zero and prints "no such subagent".
- Chat read of a known session whose parent transcript is gone: status "Not available", reason `gone`, no "no such subagent". For a Codex parent that exists only as `.jsonl.zst`: reason `compressed`. Page check for both: "Transcript no longer available", the back link, no header values and no prompt; for `compressed` also the line that the parent is archived and can be decoded from the chat page, and not the "not supported yet" text.

Header and content (stories 9 to 14, 16 to 19, 29 to 29, 40 to 40, 44 to 44)
- Mock states "a-subagent-running" and "a-subagent-finished". Chat read: the header has title, type, status, model, tokens and tool-call count equal to the task row's fields; the result is the handback message (Claude) or the last assistant message (Codex); the handback call is not in the tool list.
- Chat read of a foreground Claude subagent that is still running (no agent id yet): the header has model, tokens and tool-call count, equal to the task row's. A subagent with no duration has no Time.
- Chat read of a Claude subagent with a finished marker and no handback: status Stopped, its last assistant message is the muted result, and there is no green result card.
- Chat read on the Codex child shape: no message from before `subagent_history_start_ordinal`, the prompt block reads "Prompt not readable", the type chip is absent for a null role and "Tool calls" is absent when there is no count.

Status (stories 20 to 24, 41 to 41)
- New fixture shapes and chat reads: foreground call returned with a handback is Done; returned with no handback is Stopped and has no green result card; a background subagent whose call returned stays Running until its end notification, then Done or Stopped; a subagent of an exited session is Stopped; a Codex child ends by each task row rule (a "completed" event or completed `wait` status is Done, an exited session, a new run or the age-out is Stopped) and is not Stopped just because its `spawn_agent` call returned. Each status is the same with the task group open and closed.
- Mock states "a-subagent-running" (Running chip, working row) and "a-subagent-finished" (Done chip, result).

Missing file and compressed child (stories 25 to 28, 42 to 44)
- Mock state "a-subagent-missing" with the wording of this spec. Chat read: file not found while the call runs gives "Starting…"; file not found otherwise gives "Transcript no longer available" and status "Not available". The header still has title, type and status, and a Claude subagent shows the prompt from the parent's call. A Codex child shows "Prompt not readable". A Codex child with no rollout yet gives "Starting…" while `resolveTask` counts it as running, and "Transcript no longer available" once its end rules say it ended.
- New fixture: a Codex child with only a `.jsonl.zst`. Chat read: not available with the reason "compressed". The page text says the file is compressed and that reading compressed children is not supported yet, and has no Decode button.
- Go server test of the watch: a foreground Claude "Starting…" subagent (and a Codex child with no rollout) whose file then appears while the parent writes nothing; the subagent event is sent with no parent write. The web seam test shows that the page refetches on that event and shows the messages.

Live updates and cost (stories 15 to 15, 35 to 36, 42 to 42)
- Same flow: a running fixture subagent whose file grows; the open page shows the new message with no reload.
- Server tests (Go, run with `scripts/proc-cap`): with no `subagent=` read and no open task group, no subagent file is opened; two reads of an unchanged file parse it once; after growth it parses again; a finished subagent's file is not parsed again; the watch ends when the page closes or its event stream disconnects. The watch write is in `WebOnlyRoutes`, the web-only route test passes, and the write refuses an unknown client id (`subagent-watch-unknown-client`) and an unknown call id (`unknown-subagent`).

CLI and TUI (stories 37 to 38)
- CLI: `go run . agents chat <session> --subagent <call-id>` prints the plain text defined under CLI output; `--json` prints the chat read. `cmd/server_parity_test.go` passes.
- TUI: the Agents tab shows the same rows and no subagent link. Existing TUI checks pass.

Read-only and phone (stories 33 to 34)
- Mock states "a-subagent-running": the page has no composer and no input.
- Same flow at 390px: the header wraps and the page has no sideways scroll, on a running subagent page and a not-available page. The existing phone flow's no-sideways-scroll check style applies.

**Mock states**

- `$(gx tickets root)/gx/subagents/mocks/02-subagent-page.html`, a-parent, 1100px, fixture scenario `subagents` (Claude shape, one running and one finished subagent); parts: parent-head, parent-call-log, task-list.
- `$(gx tickets root)/gx/subagents/mocks/02-subagent-page.html`, a-subagent-running, 1100px, fixture scenario `subagents` (Claude shape, foreground, running); parts: subagent-header, subagent-prompt, working-row.
- `$(gx tickets root)/gx/subagents/mocks/02-subagent-page.html`, a-subagent-finished, 1100px, fixture scenario `subagents` (Claude shape, finished with a handback); parts: subagent-header, subagent-prompt, subagent-result.
- `$(gx tickets root)/gx/subagents/mocks/02-subagent-page.html`, a-subagent-missing, 1100px, fixture scenario `subagents` (Claude shape, file gone and call not running); parts: subagent-header, subagent-prompt, missing-note. The mock shows the prompt block and the note "Transcript no longer available". All part names are tagged in the stored mock and `go run ./testutil/mocklint/cmd` is clean.

New scenarios to write: `subagents` with the shapes named above (Claude foreground running, Claude finished with handback, Claude stopped with no handback, Claude background with a returned call, Claude nested, Claude file gone, Claude starting, Claude finished marker with no handback (Stopped), Codex child with no rollout yet, Claude parent transcript gone, Codex child, Codex child with null role, Codex compressed-only child, Codex nested call with no link, an exited session). These become implementation tickets, together with the new flows listed above.

## Out of Scope

- The TUI: it shows no subagents today and stays that way.
- A detail page for shell and Monitor tasks: they have output, not a chat.
- A list of all subagents of a session. The links on each call are enough for now; the list can come as a follow-up.
- Sending messages to a subagent.
- Decoding a compressed Codex child (`.jsonl.zst`). The page says it is not supported yet.
- Showing a Codex child's prompt. It is encrypted and has no plaintext copy.
- A 390px mock state.
- Codex grandchildren (subagents inside a Codex child): no link. A nested Codex call shows no link and the server does not look for it. This narrows the nesting decision of map ticket `gx:subagents/04`, which applied to both runners, to Claude. Claude nesting is fully supported.

## Further Notes

- Facts behind this spec: map tickets `gx:subagents/01` to `06` and the research file `docs/research/subagent-files.md` on the local branch `research/subagent-files`. A Claude subagent file is all sidechain lines, starts with the prompt and ends with a `SubagentHandback` call. A Codex child file starts with a copy of the parent's history.
- Claude deletes subagent files with the parent after `cleanupPeriodDays` (30 by default). Codex archives a child with its parent, so an archived child can be compressed.
- The page text for a compressed child exists so that a person who meets it, or a developer who reads this spec, knows what to build next.
- Glossary terms used: **Task group** and **Background task**. A subagent's own background tasks stay out of the task list, as the Background progress spec says.
