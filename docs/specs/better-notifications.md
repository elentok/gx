## Problem Statement

A person reads gx's Telegram and Slack messages mostly as phone toasts, and a toast shows only the start of a message. Today that start is useless. Every message opens with the project name in bold (which reads "gx" because the project is called gx), then a blank line, then a vague headline such as "needs answer" or "needs repair". The ticket's identity, the reason and the title come later, if at all. The person already has a gx-specific Telegram bot and Slack app, so the name "gx" tells them nothing.

The detail is also messy. A pause shows `<kind>: <raw error>`, for example a stall nudge, a dead-session socket path, herdr CLI output or pasted git output. A recovery escalation shows a rule id and a file path. Pause, escalation and conflict messages carry no ticket title. A one-off result shows a full address with no title. The messages end with a `[gx] project/epic/NN` identity line that repeats what the sender name already says.

Two more things are wrong:

- There are too many ticket messages. The ticket-finished message fires for every ticket and drowns out the pauses and errors that matter.
- A Design session that waits on a question round, and a Design that reaches Branch, are silent in chat unless AFK mode happens to send a generic session alert.

## Solution

Every Chat message gets the same new layout. The first line is the **Headline**: a status icon, a **Status word**, the ticket's `epic/NN` and the ticket's title. A message with no ticket puts the epic's name (or the session's name) in the ticket's place. The headline is plain text with no markup, in Telegram and Slack alike, so a toast shows it unchanged.

After the headline come plain-word lines saying what happened. The last line is the project name, alone. The bold project-name header on top and the `[gx] epic/NN` identity line are gone. Raw pause reasons become one plain sentence for each known family, and an unknown reason falls back to its first line cut at 100 characters. The counts line leaves the pause, conflict and escalation messages and stays on the progress update and on epic merged or complete. `gx notify "..."` text goes out as written.

The 6-second batch stays, one send per tick. A tick that holds several messages starts with a summary headline whose icon is the most urgent message's, so a toast still shows what matters most. A tick with one message sends it as is.

Three things change in what reaches Chat:

- The per-ticket finished message goes away. The epic merged or complete message stays.
- A per-epic **Progress** message goes out every 15 minutes while a ticket runs in the epic. It tells the person the server is alive and how far the epic is.
- A Design that waits on a question round sends a **Question** message (once per round, unless the person has the session open on screen). A Design that reaches Branch sends a **Ready** message naming its `<epic>-impl` epic and ticket count (once per Design).

The approved layout of every message is the stored mock, listed under Verification as Mock states.

## User Stories

1. As a person reading a phone toast, I want the first line of every Chat message to be its Headline, so that I see what happened without opening the chat.
2. As a person with a gx-specific bot and app, I want no "gx" header on top of messages, so that the toast does not waste its first line on a name I already know.
3. As a person reading a toast, I want the Headline to be plain text with no markup in Telegram and Slack, so that no markers show in the preview.
4. As a person, I want the Headline to read icon, Status word, `epic/NN`, title, so that every ticket message tells me what, which ticket and which work.
5. As a person, I want a message about an epic to name the epic where a ticket would go, so that the layout is the same everywhere.
6. As a person, I want a message about a session to name the session where a ticket would go, so that I know which session needs me.
7. As a person, I want the project name as the last line of a message, so that I still know which project it came from, without it taking the toast. Server notices (the long-ticket notice included) and the herdr digest have no project line.
8. As a person, I want the `[gx] epic/NN` identity line gone, so that nothing repeats what the Headline already says.
9. As a person, I want a pause on a self-reported question to show the agent's one-line question summary, so that I can answer from the chat.
10. As a person, I want an answer pause with no open question to say so in a plain sentence, so that I know to open the ticket.
11. As a person, I want a zero-commit pause to say the agent finished with no commits and gave no reason, so that I know what to check.
12. As a person, I want a stall-nudge pause to say the agent did not accept its prompt, so that I do not read an internal error string.
13. As a person, I want a dead-session pause to say the agent session ended unexpectedly, without a socket path, so that the line is short.
14. As a person, I want a herdr-lost-the-agent pause to say so in one sentence, without CLI output or JSON.
15. As a person, I want a worktree-already-exists pause to say the ticket's worktree folder exists.
16. As a person, I want a blocked-prompt pause to say the agent waits on a permission prompt and where to approve it.
17. As a person, I want an unknown pause reason to fall back to its first line cut at 100 characters, so that a new reason is never hidden and never floods the chat.
18. As a person, I want pause wording to depend on the pause kind and a stable marker, so that rewording an error message never changes the chat text silently.
19. As a person, I want ticket messages (pause, conflict, escalation, result) to carry the ticket's title, so that I can tell which work they concern.
20. As a person, I want a conflict message to say an agent is resolving the merge conflict, so that I know I need do nothing.
21. As a person, I want a recovery escalation for a ticket to say automatic recovery gave up and why, without a rule id or a report path, so that it reads as a sentence.
22. As a person, I want an epic-level escalation to name the epic and say all tickets are paused, so that I see the scale.
23. As a person, I want a one-off result to show Done, the one-off's address and title, and its result (or the line "No result recorded." when it is empty), so that I know what finished.
24. As a person, I want a one-off's result text kept as today (cut at 1500 characters), so that the result still reads in full where it fits.
25. As a person, I want the counts line off pause, conflict and escalation messages, so that the detail is not buried.
26. As a person, I want the counts line kept on the progress update and on epic merged or complete, so that I see the epic's state.
27. As a person, I want the epic merged message to stay, naming the number of tickets merged, elapsed time, cost and tickets recovered automatically, so that I know the work landed.
28. As a person, I want an epic that is complete but not merged to say "Not merged", so that I know a step remains.
29. As a person, I want no chat message for each ticket that finishes, so that the chat is not flooded.
30. As a person, I want the finished event still recorded for the TUI, the Queue tab and the run log, so that nothing but the chat message is lost.
31. As a person, I want one progress message per epic every 15 minutes while a ticket runs, the timer starting when the epic's first ticket starts running and stopping when none run, so that I know the server is still working.
32. As a person, I want the progress headline to read `Progress <epic> · N of M done`, so that the toast shows how far along the epic is.
33. As a person, I want the progress lines to list running tickets with elapsed time, tickets finished since the last update (count, elapsed, cost) and tickets left, so that I see pace and spend.
34. As a person, I want no progress message while no ticket runs in the epic, so that a quiet epic stays quiet.
35. As a person, I want progress updates to go through the existing gate (source `epic:<project>/<epic>`) like every other message, so that the global mute stops them and the gate's send-rate limit counts them: each message is recorded in the gate's event series and each tick counts as one send.
36. As a person, I want one progress timer per epic however its tickets run (any runner or harness), so that I get one message, not several.
37. As a person, I want the progress timer to restart after a server restart, with the first update a full interval later, so that I get no burst on boot.
38. As a person, I want a Design waiting on a question round to send a Question message once per round, so that I know it needs me.
39. As a person, I want the Question message to name the Design, the step, the round number, the round's first question and the Design page link, so that I can act from the phone.
40. As a person, I want no Question message when the Design session is open on screen, using the same rule as the OS banner, so that I am not pinged while looking at it.
41. As a person, I want that decision made once, when the round starts waiting, so that closing the session later does not send a late message.
42. As a person away from the machine (AFK mode), I want the Question message to replace the generic session alert for that wait, so that I get one message, not two.
43. As a person in AFK mode, I want other session waits to keep the session alert, so that blocked and exited sessions still reach me.
44. As a person, I want a restart never to resend a Question for a round already sent, so that the chat stays clean.
45. As a person, I want a Ready message when a Design reaches Branch, naming the `<epic>-impl` epic, its ticket count and a link to it, so that I know it is ready to queue.
46. As a person, I want the Ready message sent once per Design, even across restarts, so that I am not pinged twice.
47. As a person, I want Question and Ready to go through the existing gate (source `epic:<project>/<epic>`; for Ready, the Design's own epic, the same source as its Question), counted by the gate's send-rate limit and stopped by the global mute, so that they follow the same rules as every other message: each message is recorded in the gate's event series and each tick counts as one send.
48. As a person, I want session alerts (AFK) to read `🔔 <Status word> <session name>` with the reason below, so that the toast shows what the session needs.
49. As a person, I want server notices (herdr, disk, budget, trunk, project, started, daily summary, extra usage, long ticket) to keep today's phrase, capitalized, behind an icon, so that they stay readable. The long-ticket notice keeps the mock's look, names its ticket in its phrase and has no project line.
50. As a person, I want server notices to have no project line, so that no project is invented for a machine-level event.
51. As a person, I want the herdr digest to list each paused ticket on a line and carry no project line, so that the digest stays one message.
52. As a person, I want mute messages to follow the Headline shape (`🔇 Muted <epic>/<NN> <title>`, `🚫 Muted all notifications`), so that they are as clear as any other.
53. As a person, I want the test message to follow the Headline shape, so that I can check the layout from `gx config test-notifications`.
54. As a person, I want `gx notify "..."` text delivered exactly as written, so that my own scripts control their message.
55. As a person, I want several messages in one 6-second tick to start with a summary headline `<icon> N updates: <words>`, where N counts messages after dedup and the words are unique, in urgency order, with a count when repeated, so that the toast shows how much happened.
56. As a person, I want the summary icon to be the most urgent message's (by message kind: repair, then answer or question, then escalated, then conflict, then results, then progress and information), so that the worst news leads.
57. As a person, I want each message under the summary to keep its own Headline and, where it has one, its project line, so that the batch reads as separate messages.
58. As a person, I want a tick with one message to send it with no summary, so that a single message is not wrapped.
59. As a person, I want identical messages in one tick still collapsed with a count (` ×N` at the end of the Headline), so that a burst of the same pause stays short.
60. As a maintainer, I want one layout function for every chat message with no identity-line parameter, so that the layout cannot drift between message types.
61. As a maintainer, I want the dead run-loop chat renderers deleted, so that no unused layout code remains.
62. As a maintainer, I want the Design question and ready marks persisted in the server's notification state, so that they survive a restart without a schema change to epics or tickets.

## Implementation Decisions

**Layout (all Chat messages, Telegram and Slack)**
- One layout function renders every message: Headline, then detail lines, then the project name alone as the last line. It takes no identity-line parameter. The Headline is plain text, with no bold or other markup, on both transports. Telegram body text is still escaped as the transport needs; the implementer picks how.
- The bold project group header the server adds on top is removed, and so is the `[gx] epic/NN` identity line. The test message and the "globally muted" notice lose their `[gx]` prefix too.
- Verified against the code: the identity line and the run-loop renderers have no non-chat caller. The only construction of the run-loop chat sink is the server's chat wrapper, and the `gx notify` CLI path sends text on its own. So the identity line is deleted outright, with no kept variant. The dead run-loop renderers (epic started, iteration started/paused/resumed, epic paused, epic failed, and the run-loop epic complete) are deleted with their sink methods and tests. If the implementer finds a non-chat caller after all, that caller keeps the line.
- The Headline shape is `<icon> <Status word> <epic>/<NN> <title>`. A message about an epic uses `<icon> <Status word> <epic>`. A session message uses `<icon> <Status word> <session name>`. The icons and Status words are in the Mock states list and the approved mock; the implementer picks the internal table.
- Status words and icons: Answer 🙋, Repair 🛑, Conflict 🔀, Escalated ⚠️, Done ✅ (a one-off result), Merged 🎉, Complete ✅, Progress ⏳, Question ❓, Ready 📦, session alerts 🔔 with Done, Blocked, Rate-limited or Exited (today's status words), Muted 🔇 and 🚫, Test 🔔.
- Mute messages: a muted epic's message is `🔇 Muted <epic>/<NN> <title>` with the mock's detail line and a project line; the global one is `🚫 Muted all notifications`.
- Server notices keep today's phrase, capitalized, behind the icon, with no separate Status word (for example `🔌 Herdr unavailable`). They have no project, so no last line. The herdr digest has no last line either.
- Ticket titles reach every ticket message. The call sites that hold only an identifier or an address today (pause, child pause, escalation, conflict, result, long ticket, and the gate's mute notice, whose source is the ticket path) read the title from the ticket they already locate; the implementer picks how, keeping one lookup shared by the callers.
- The counts line is kept only on the progress update and on epic merged or complete. It is dropped from pause, conflict and escalation messages.
- A recovery escalation drops the rule id and the report path from chat. Its detail is one plain sentence naming that automatic recovery gave up and the failure in the pause wording below.
- A one-off result message: Headline `✅ Done <address> <title>`, then the Result section as today (cut at 1500 characters), then the project line.  When the Result is empty the message is the Headline, the line "No result recorded.", then the project line.
- `gx notify "..."` text goes out unchanged, with no Headline and no project line.

**Pause wording**
- `PauseChatDetail` keeps its place and maps a pause to one plain sentence. It matches on the pause kind plus a stable marker, never on error text. Where a source does not yet attach a stable marker (stall nudge, dead session, herdr-lost agent, existing worktree, blocked prompt), the source adds one. The implementer picks marker names and form.
- Families to map: self-reported with an open question (the agent's one-line summary, as today), answer pause with no question, zero commits, stall nudge, session died, herdr lost the agent, worktree exists, blocked prompt. Wording per family is in the approved mock.
- An unmatched reason uses its first line, cut at 100 characters with an ellipsis.
- No pause kind is added and no pause is removed, so ADR 0042 is untouched (every pause stays recoverable from the web).

**Batching**
- The 6-second tick and one send per tick stay. A tick with two or more messages starts with `<icon> N updates: <status words>`, then each message with its own Headline and project line. A tick with one message sends it as is. Dedup with a count stays.
- Urgency is ranked by message kind, never by icon. The summary icon is the most urgent message's. The order, most urgent first:
  1. Repair, and work-holding server notices (hard budget limit, low disk, herdr down, trunk missing).
  2. Answer and Question, and session Blocked, Rate-limited and Exited.
  3. Escalated.
  4. Conflict.
  5. Results: Done, Merged, Complete, Ready.
  6. Information: Progress, other server notices (including the long-ticket notice), session Done, mute and test messages.
- Summary words: N counts messages after dedup, and the words are counted after dedup too. The words are the unique Status words in urgency order, with ` ×N` after a word that repeats (`Repair ×2, Answer`). `Repair ×2` means two distinct Repair messages; a deduped burst shows one `Repair` word and a `×N` on its own Headline. A server notice adds the word `Notice`. After three words the list stops and ends with `…`.
- Dedup puts ` ×N` at the end of the Headline. The project line stays bare.
- The gate's send-rate limit and the mute gate still count one send per tick as they do today. Batching never splits a message across sends.

**Progress update**
- A new notify kind `progress-update` (the implementer confirms the exact string; the map fixes this name) goes through the existing gate with source `epic:<project>/<epic>`, so the gate's send-rate limit counts it (each message is recorded in the gate's event series and each tick counts as one send) and the global mute stops it. Its source has no ticket, so the per-source storm mute never applies; only the global mute stops it. It is a notify kind only, not a pause or event kind: it does not appear in `gx server events kinds`, which lists pause kinds only (verified).
- One timer per epic across all runners and harnesses, 15 minutes, held in server memory only. The timer starts when the epic's first ticket starts running and stops when none run. The first update counts the tickets finished since the timer started. After a restart the timer starts again and the first update is a full interval later.
- A tick of the timer sends a message only when a ticket runs in the epic. Otherwise it sends nothing and the timer keeps running.
- Headline: `⏳ Progress <epic> · N of M done`. Lines: Running (tickets with elapsed time), Finished since the last update (count, elapsed, cost), Left (count). The project line closes it.
- The interval must be reachable from tests and the fixture through a config or option, so verification does not wait 15 minutes. The implementer picks the knob.

**Removed message**
- The ticket-finished chat message (`iteration-finished`) is removed from Chat. The event is still emitted to the TUI, the Queue tab and the run log. The epic merged and epic complete messages stay, in the new layout.

**Design question message**
- Sent when a Design session starts waiting on a question round (a Grilling round the person has to answer). Headline `❓ Question <epic>`, then "Design waits at the <step> step. Round N.", the round's first question, the Design page link, then the project line. When the web listener is off the link is left out, as the OS banner's link is.
- Once per round: the mark is keyed on the Design session and the round number and persisted in the server's notification state file next to the gate's series, so a restart never resends.
- Suppression is decided once, when the round starts waiting, with the one "open on screen" check the OS banner uses. The shared rule is: no message while the session is open in the web UI, or open in a TUI while AFK mode is off. In AFK mode a TUI-open session does not suppress chat, exactly as the banner code already behaves (verified in the session banner firing path: the web check always suppresses, the TUI check suppresses only when AFK is off). The rule becomes one shared function used by the banner and by the Question message, not a copy. If the person closes the session afterwards, nothing is sent later.
- In AFK mode the Question message replaces the generic `🔔` session alert for that wait only. Every other wait (blocked, exited, rate-limited, done) keeps the alert. The OS banner and sound are unchanged.
- The message goes to the project's destination (its override or the global one). Kind: `design-question` (implementer picks the final string), goes through the existing gate with source `epic:<project>/<epic>`, counted by the gate's send-rate limit (recorded in the gate's event series, one send per tick) and stopped by the global mute. Its source has no ticket, so the per-source storm mute never applies; only the global mute stops it.

**Design ready message**
- Sent once when a Design reaches Branch. Headline `📦 Ready <epic>-impl`, then "The design is finished. N tickets are ready to queue.", a link to the new epic, then the project line. N is the ticket count of the `<epic>-impl` epic. Link omitted when the web listener is off.
- The mark is keyed on the `<epic>-impl` epic and stored in the server's notification state file, so a restart or a repeated Branch never resends.
- Kind: `design-ready` (implementer picks the final string), goes through the existing gate with the Design's own epic as source (`epic:<project>/<epic>`, the same source as its Question), counted by the gate's send-rate limit (recorded in the gate's event series, one send per tick) and stopped by the global mute. Its source has no ticket, so the per-source storm mute never applies; only the global mute stops it.

**Persistence**
- No schema change and no migration. Both sent-marks (Question keyed on session plus round, Ready keyed on the `<epic>-impl` epic) live in the server's notification state file next to the gate's series, so they persist across restarts.

**Out-of-scope surfaces**
- OS banners, TUI toasts and the Queue tab are unchanged. Banners keep their own title field.
- The set of events that reach Chat is unchanged apart from the three new messages and the removed ticket-finished message.

**Implementer's choices (the person delegates these)**
- Names of the `design-question` and `design-ready` notify kinds, and whether `progress-update` keeps exactly that string.
- Names and form of the stable pause markers added at pause sources.
- The internal icon and Status word table, and how the layout function is shaped.
- The knob that shortens the 15-minute progress interval and the 6-second tick in tests and the fixture.
- How a ticket title is looked up at call sites that hold only an identifier.
- The shared "open on screen" function's name and where it lives.
- Escaping details per transport and elapsed/cost formatting (matching today's formats).

## Testing Decisions

- A good test sends a real event through the server (a pause, a merge, a Design wait, a timer tick) and asserts on the text a Chat recorder receives and on how many sends happen. It does not assert on which helper rendered the text.
- Seam 1, the only seam for behavior: the server-level test rig with a chat recorder (a fake Slack webhook and a fake Telegram Bot API, with the short flush interval). It covers every message in the Mock states list, batching and summary headlines, the progress timer, the Question and Ready messages with restarts, muting, the budget gate and the removal of the finished message. Prior art: the existing server chat tests (`server_notify_test`, `configreload_chat_test`) and the chat recorder and Slack hook rigs under the server test rigs.
- Seam 2, pure unit: `PauseChatDetail` mapping (each family, the 100-character fallback, kind plus marker matching, no match on error text), and the layout function (Headline shape, plain headline, project last line, no identity line). Prior art: the existing notification text and MarkdownV2 validity tests, which stay green for the new text.
- Seam 3, the shared "open on screen" check: a test table over web open, TUI open, AFK on and off, asserting the banner and the Question message agree. Prior art: the existing session banner and session focus tests.
- Seam 4, CLI: `gx notify "..."` delivers the text as written; `gx config test-notifications` sends the new test message. Prior art: the existing notify CLI tests.
- The Mock states are checked in a verify run on a temporary server (see Verification), not in unit tests. A rendered-text snapshot for each mock state in Seam 1 keeps the text honest.
- The existing tests that assert the old layout (bold project header, identity line, `needs answer` headline, finished message) are updated or deleted with the code they cover.
- Run Go tests per package with `scripts/proc-cap`, never the whole repo at once.

## Verification

A verify ticket runs these against a temporary gx server after the epic is built. The flow catalog has no flow for Chat messages today (Chat is not a web surface), so every story below needs the new scenarios and flows listed at the end of this section. Each step reads the captured messages that the new chat-capture scenario records. Where a story entry names a Mock states line, that line carries the look check; the entry does not repeat it.

1. Headline first. Run the `chat-tickets` scenario; read the first line of every captured message. Pass: it is the Headline, with no bold project line and no `[gx]` text in any message. Mock states: tickets.
2. No "gx" header. Same capture. Pass: no message starts with the project name alone, and no message starts with a bold line. Mock states: tickets.
3. Plain headline. Same capture, both Telegram and Slack. Pass: the Headline carries no markup characters in either transport. Mock states: tickets.
4. Headline shape. Run `chat-tickets`. Pass: ticket messages read icon, Status word, `epic/NN`, title. Mock states: tickets.
5. Epic name in place. Run `chat-epics`. Pass: epic messages read icon, Status word, epic name. Mock states: epics.
6. Session name in place. Run `chat-sessions` with AFK on. Pass: session alerts read icon, Status word, session name. Mock states: sessions.
7. Project last line. Run every chat scenario (`chat-tickets`, `chat-epics`, `chat-sessions`, `chat-server`, `design-question-wait`, `design-branch`) and check the project line across all captures. Pass: ticket, epic, Design and session messages end with the project name alone; server notices (long-ticket included) and the digest do not. Mock states: tickets, epics, design, sessions, server.
8. No identity line. Same capture. Pass: no message has a line starting `[gx]`. Mock states: tickets.
9. Self-reported question. `chat-tickets` has a ticket paused with an open question. Pass: the detail is the one-line summary. Mock states: tickets (answer-question).
10. Answer without question. Pass: the detail is the "left no question" sentence. Mock states: tickets (answer-noquestion).
11. Zero commit. Pass: the detail is the "no commits, no reason" sentence. Mock states: tickets (answer-zerocommit).
12. Stall nudge. Pass: the "did not accept its prompt" sentence. Mock states: tickets (repair-stall).
13. Dead session. Pass: the "ended unexpectedly" sentence, no socket path. Mock states: tickets (repair-died).
14. Herdr lost the agent. Pass: the one-sentence herdr text, no JSON. Mock states: tickets (repair-herdr).
15. Worktree exists. Pass: the worktree-folder sentence. Mock states: tickets (repair-worktree).
16. Blocked prompt. Pass: the permission-prompt sentence. Mock states: tickets (repair-blocked).
17. Unknown reason. Pass: first line, at most 100 characters, with an ellipsis. Mock states: tickets (repair-fallback). Also run the unit mapping test: `go test` of the pause-chat package under `scripts/proc-cap`.
18. Marker matching. Unit seam only: the mapping test changes the error text and still gets the same sentence. No flow.
19. Titles. In `chat-tickets`, every pause, conflict, escalation and result headline carries the ticket title. Mock states: tickets.
20. Conflict. Trigger a conflict-resolution start. Pass: the "agent is resolving" sentence. Mock states: tickets (conflict).
21. Ticket escalation. Pass: the sentence has no rule id and no report path. Mock states: tickets (escalated-ticket).
22. Epic escalation. Pass: the epic-level sentence. Mock states: epics (escalated-epic).
23. One-off result. Run a one-off with notify and a Result. Pass: `Done` headline with address and title, the result below. Repeat with an empty Result: Headline, "No result recorded.", project line. Mock states: tickets (done-result).
24. Long result. Run a one-off with notify and a Result over 1500 characters. Pass: the result is cut at 1500 characters with an ellipsis.
25. Counts line. Pass: pause, conflict and escalation messages have no counts line. Mock states: tickets, epics.
26. Counts line kept. Pass: progress and epic merged or complete have it. Mock states: epics (progress, merged, complete).
27. Epic merged. Run `chat-epics` and merge an epic. Pass: tickets merged, elapsed, cost, recovered count. Mock states: epics (merged).
28. Complete, not merged. Pass: the "Not merged." line. Mock states: epics (complete).
29. No finished message. Finish a ticket in a running epic. Pass: no message captured for it, and `gx tickets log` still shows the iteration-finished event. Also covers story 30.
30. Finished event kept. As in 29; also the Queue tab still shows the ticket done (run the existing Queue flow for done tickets).
31. Progress cadence. Run the `running-ticket-tick` scenario with the short interval. Pass: one message per interval per epic while a ticket runs, the timer starting when the epic's first ticket starts running. Mock states: epics (progress).
32. Progress headline. Same scenario. Pass: `Progress <epic> · N of M done`. Mock states: epics (progress).
33. Progress lines. Same scenario with a ticket finished between ticks. Pass: Running, Finished since the last update, Left lines; the first update counts tickets finished since the timer started. Mock states: epics (progress).
34. Skip when idle. Same scenario after the ticket stops. Pass: no further progress message.
35. Global mute. Trip the global mute. Pass: progress stops. Re-enable: progress resumes. Each progress message is recorded in the gate's event series and each tick counts as one send against the gate's send-rate limit.
36. One timer per epic. Run an epic with two runners or harnesses. Pass: one message per tick, not two.
37. Restart timer. Restart the server mid-run. Pass: no message in the first interval after restart, one a full interval later.
38. Question once per round. Run the `design-question-wait` scenario. Pass: one Question message when the round starts waiting, none more while it waits. Mock states: design (design-question).
39. Question content. Same. Pass: epic, step, round number, first question, Design page link. Mock states: design (design-question).
40. Open on screen. In `design-question-wait`, open the session in the web UI before the round starts waiting. Pass: no message. Repeat with the session in a TUI and AFK off (none) and AFK on (sent). Use the Agents tab flows (the Design session open).
41. Decided once. Same scenario: session open when the round starts, then closed. Pass: no message afterwards.
42. AFK replaces alert. `design-question-wait` with AFK on. Pass: exactly one message for the wait, the Question, not the Question plus a session alert. Mock states: design (design-question), sessions.
43. Other waits keep alert. With AFK on, make a session blocked. Pass: the `🔔 Blocked` alert arrives. Mock states: sessions.
44. Restart no resend. Restart the server during a waiting round. Pass: no second Question for that round; a new round sends again.
45. Ready. Run the `design-branch` scenario and take the Design to Branch. Pass: one Ready message with the `<epic>-impl` name, the ticket count and the link. Mock states: design (design-ready).
46. Ready once. Restart and repeat the Branch step. Pass: no second Ready.
47. Global mute. Trip the global mute. Pass: Question and Ready stop. Re-enable: they resume. Each message is recorded in the gate's event series and each tick counts as one send against the gate's send-rate limit.
48. Session alerts. Run `chat-sessions` with AFK on. Pass: Done, Blocked, Rate-limited and Exited alerts. Mock states: sessions (session-done, session-blocked, session-ratelimited, session-exited).
49. Server notices. Run `chat-server`. Pass: each notice keeps today's phrase, capitalized, behind its icon. Mock states: server.
50. No project on notices. Same. Pass: no notice ends with a project line. Mock states: server.
51. Herdr digest. Pause three tickets while herdr is down, then restore it. Pass: one digest message with one line per ticket. Mock states: server (srv-digest).
52. Mute messages. Trip a mute and the global mute. Pass: `🔇 Muted <epic>/<NN> <title>` with its detail and project lines, and the muted-all headline. Mock states: meta (muted, muted-all).
53. Test message. `gx config test-notifications` against the capture endpoint. Pass: the test headline. Mock states: meta (test).
54. Free text. `gx notify "Deploy finished on staging"`. Pass: exactly that text, nothing added; exit code 0. Mock states: meta (cli).
55. Summary headline. Run `chat-batch` and queue three messages inside one tick. Pass: one send, with a summary headline. Mock states: batching (batch-three).
56. Urgency order. Run `chat-batch` with each icon mix. Pass: the summary icon follows the order. Include one mixed batch (for example a server notice with two distinct Repair messages, an Answer and a session Done) and check the icon, N after dedup, the words in order with counts (words counted after dedup, so `Repair ×2` means two distinct Repair messages), the `Notice` word and the three-word `…` cut. A deduped burst of one Repair shows one `Repair` word and a `×N` on its own Headline. Mock states: batching (batch-three).
57. Each keeps headline. Same. Pass: each message below the summary has its own Headline and, where it has one, its project line. Mock states: batching (batch-three).
58. Single message. One message in a tick. Pass: no summary. Mock states: batching (batch-single).
59. Dedup. Send the same pause message twice in one tick. Pass: one message with ` ×N` at the end of its Headline; the project line stays bare.
60. One layout function. Unit/code check by the code-review ticket: no second layout path and no identity-line parameter remain. No flow.
61. Dead renderers. Run `go build ./...` and the package vet through `gx check`. Pass: clean, with the renderers gone. No flow.
62. Persisted marks. Covered by 44 and 46. Also, `gx tickets validate` on the tickets of the Design and its `-impl` epic. Pass: valid, no schema change.

**Scenarios and flows to write** (these become implementation tickets; the catalog covers none of them):
- A `chat-capture` base in the fixture: the temporary server's Telegram base URL and Slack webhook point at a capture endpoint, the flush interval and the progress interval are short, and a verb or file lets the verify step read the captured messages. Prior art: the server test rigs with a chat recorder.
- `chat-tickets`: tickets in the paused, conflict, escalated and one-off states, one per mock part in the tickets state. Prior art: the `paused` and `mixed` scenarios.
- `chat-batch`: no destination override; can queue a pause, an answer, a session Done and a server notice in one tick. Needed for batching (stories 55 to 59).
- `chat-epics`: an epic that merges, an epic that completes unmerged, an epic escalation. Prior art: the `united` and `dirty-trunk` scenarios.
- `chat-sessions`: sessions that go idle, blocked, rate-limited and exited with AFK on. Prior art: the `rate-limited` and `blocked-menu` scenarios.
- `chat-server`: server notices (herdr down and up, disk, budget, trunk, project, started, long ticket) and the herdr digest. Prior art: the `disk-low` and `health` scenarios.
- `design-question-wait`: a Design session that goes idle after a Grilling round, with controls to open the session on screen and to turn AFK on. Needed for the Design question wait (stories 38 to 44).
- `design-branch`: a Design at the Branch step with a `<epic>-impl` epic of known ticket count. Needed for the Design reaching Branch (stories 45 to 47).
- `running-ticket-tick`: an epic with a running ticket, held long enough for several short progress ticks, with a second ticket that finishes between ticks, and a second runner of the same epic (for the one-timer-per-epic check). Needed for the running ticket with the 15-minute tick (stories 31 to 37).
- A flow for each of the above that reads the capture and asserts the pass line of its stories.
- Scenario and flow names above are working names; the implementer picks the final ones.

**Mock states** (the stored mock is `$(gx tickets root)/better-notifications/mocks/04-new-layout-mock.html`; the root is `/Users/david/.local/share/gx/tickets/gx`, so the full path is `/Users/david/.local/share/gx/tickets/gx/better-notifications/mocks/04-new-layout-mock.html`). Each part has a Telegram, a Slack and a toast cell. Every state is 1040 px wide. No state exists as a web page, so the fixture scenario produces the message text and the check compares it with the part's text.

- `/Users/david/.local/share/gx/tickets/gx/better-notifications/mocks/04-new-layout-mock.html`, state `tickets`, width 1040, fixture scenario `chat-tickets` (new), parts: answer-question, answer-noquestion, answer-zerocommit, repair-stall, repair-died, repair-herdr, repair-worktree, repair-blocked, repair-fallback, conflict, escalated-ticket, done-result.
- `/Users/david/.local/share/gx/tickets/gx/better-notifications/mocks/04-new-layout-mock.html`, state `epics`, width 1040, fixture scenario `chat-epics` (new) and `running-ticket-tick` (new) for the progress part, parts: merged, complete, escalated-epic, progress.
- `/Users/david/.local/share/gx/tickets/gx/better-notifications/mocks/04-new-layout-mock.html`, state `design`, width 1040, fixture scenarios `design-question-wait` (new) and `design-branch` (new), parts: design-question, design-ready.
- `/Users/david/.local/share/gx/tickets/gx/better-notifications/mocks/04-new-layout-mock.html`, state `sessions`, width 1040, fixture scenario `chat-sessions` (new), parts: session-done, session-blocked, session-ratelimited, session-exited.
- `/Users/david/.local/share/gx/tickets/gx/better-notifications/mocks/04-new-layout-mock.html`, state `server`, width 1040, fixture scenario `chat-server` (new), parts: srv-started, srv-longticket, srv-extra, srv-daily, srv-soft, srv-hard, srv-disklow, srv-diskresume, srv-diskclean, srv-herdr-down, srv-herdr-up, srv-project, srv-trunk-missing, srv-trunk-found, srv-digest.
- `/Users/david/.local/share/gx/tickets/gx/better-notifications/mocks/04-new-layout-mock.html`, state `meta`, width 1040, fixture scenario `chat-capture` (new) with a tripped mute, `gx config test-notifications` and `gx notify`, parts: muted, muted-all, test, cli. The mock's `muted` part changes to the headline `🔇 Muted <epic>/<NN> <title>`, keeping its detail line and project line.
- `/Users/david/.local/share/gx/tickets/gx/better-notifications/mocks/04-new-layout-mock.html`, state `batching`, width 1040, fixture scenario `chat-batch` (new) with several messages in one tick, parts: batch-three, batch-single.

## Out of Scope

- OS banners, TUI toasts, the Queue tab and the web UI: unchanged.
- Which events reach Chat, apart from the three new messages and the removed finished message.
- The gate's send-rate limit and mute: unchanged. The new messages go through them as they are.
- A cap on sends per tick (one send per tick needs none), and per-message sending.
- Device testing of how each phone client truncates a toast. The vendors publish no line or character counts; the layout puts the Headline first and keeps it short, and no more is promised.
- New pause kinds or a change to ADR 0042's guarantee.
- A schema change to epics or tickets.
- Persisting the progress timer across a restart.
- A one-off result being re-sent repeatedly for one ticket (filed as a follow-up, see gx:follow-ups).

## Further Notes

- Research on today's messages is `docs/research/current-chat-messages.md` on the branch `research/current-chat-messages`. Research on toasts says no vendor publishes preview limits; Telegram shows the bot name then the body, and Slack mobile uses the top-level text only.
- Claims checked against the code while writing this spec: the banner firing path suppresses on web-open, and on TUI-open unless AFK is on (the shared rule above copies this); `gx server events kinds` lists pause kinds only, so notify kinds are not registered there; the run-loop chat renderers and the identity line have no caller outside the server's chat wrapper.
- Glossary terms used: Chat, Headline, Status word, Counts line, Destination, AFK mode, Banner, Design session, Design branch, Grilling round, Progress. If the implementation settles a name for the "open on screen" rule or the Question and Ready messages, add it to GLOSSARY.md.
