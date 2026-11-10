## Problem Statement

When a web chat session (an Agents-page chat, or a Design or Triage chat) waits on the person, the web usually cannot show what it waits on. Claude's question dialog (AskUserQuestion), a permission prompt, a plan approval and Codex's question tool all block in the terminal, and the chat shows a banner with Screen and, at best, Allow once and Deny. A Queue agent's open `gx tickets ask` question shows only on the ticket page. The Agents-page chat never reads the printed ❓ **Grilling round** at all, so only the Design and Triage chats can answer one in the questions sheet. The person has to open the terminal screen to answer most of what a chat asks.

## Solution

Every web chat shows what its session waits on as cards in the questions sheet, and the person answers from the web. The server builds one list of question cards per chat, from five sources: Claude's question dialog, Codex's question tool, permission and plan prompts gx can read, printed ❓ rounds, and a Queue agent's open `gx tickets ask` question. The web renders the list the same way on the Agents page and on the Design and Triage pages, and every answer goes through one new endpoint that re-reads the source, refuses a stale card and routes the answer to the right channel. The Screen button stays as the fallback for anything gx cannot read.

## User Stories

1. As a person chatting with Claude on the Agents page, I want Claude's question dialog to appear as cards in the questions sheet, so that I can answer without opening the terminal screen.
2. As a person, I want single-select, multi-select and several-question dialogs shown with their headers, option labels and option descriptions, so that I see everything the terminal dialog shows.
3. As a person, I want a free-text "Other" answer on a Claude dialog question, so that I can answer something the options do not cover.
4. As a person, I want an optional note on a Claude dialog question answered through the hook, so that I can add context that the agent reads.
5. As a person, I want Submit disabled until every question on a Claude or Codex question card is answered, so that I do not send half an answer.
6. As a person, I want a dismiss button on a Claude dialog card, so that I can decline the question from the web the way Esc does in the terminal.
7. As a person, I want a dismiss to leave Claude idle after one short text reply instead of asking again, so that dismissing works like Esc.
8. As a person, I want an "Answer in the terminal" action on a hook-path dialog card, so that I can release the hook and use the real dialog when I prefer it, with no second card for that same dialog.
9. As a person, I want the card to clear when I answer in the terminal, with Screen, or when the dialog is cancelled, so that the sheet never shows a question that is gone.
10. As a person, I want a hook-held question to survive a down gx server, a hook timeout or hooks that do not run, so that I can always still answer in the terminal; after the 600 s hook timeout the keystroke fallback card shows so that I can still answer from the web.
11. As a person whose Claude dialog was not caught by the hook (hooks disabled, a Claude gx did not launch), I want gx to read the dialog from the screen and answer it by keys, so that I still get a card.
12. As a person, I want the keystroke fallback to stop and show an error on the card when the screen does not match what gx expects, so that a wrong key never lands in the dialog.
13. As a person, I want the keystroke-fallback card to have no note field, so that the card promises only what reaches the agent.
14. As a person chatting with Codex on the Agents page, I want Codex's question tool (`request_user_input`) to work in a plain chat and show as cards, so that Codex can ask me questions.
15. As a person, I want Codex question options shown with their descriptions and a free-text row, so that I can pick or type an answer.
16. As a person, I want an optional note on any Codex option, so that I can explain my pick.
17. As a person, I want a Codex card with several questions answered in one submit, so that the overlay finishes with each answer matched to its question.
18. As a person, I want a plain Codex chat to start without stalling on the "Background server has incompatible feature settings" dialog, so that enabling the question tool never blocks startup.
19. As a person, I want `gx codex doctor` to report a clear failure when Codex stops honoring the question flag, so that I learn the tool no longer works.
20. As a person, I want Queue agents, Design, Triage, conflict, doctor and Fix sessions to keep blocking Codex's question tool, so that unattended work keeps asking through `gx tickets ask`.
21. As a person, I want a plain Codex chat to get no GX system prompt, so that Codex is not told to use `gx tickets ask` or to avoid printed questions.
22. As a person, I want a Claude permission prompt shown as a card with every option and its exact label, so that I can pick any option, including a standing grant.
23. As a person, I want no option preselected and no recommended badge on a standing grant, so that I do not grant more than I meant.
24. As a person, I want a Claude plan approval shown as a card with the full plan text, so that I can read the plan before I approve it.
25. As a person, I want the plan card's row labelled "Tell Claude what to change" to be a text row that must be filled before Submit enables, and to send the digit the dialog gives that row, so that I can send feedback and the plan revises.
26. As a person, I want a dismiss button on a plan or permission card that does what Deny or Esc does today, so that I can reject without feedback.
27. As a person, I want any Claude permission or plan dialog that parses into numbered options to become a card, and one that does not parse to get only the banner and Screen, so that a card never shows a guess; a plan dialog without the "Tell Claude what to change" label gets plain digit options.
28. As a person, I want a Codex permission prompt that gx hooks shown as a card with Allow and Deny, so that I can approve it from the web.
29. As a person, I want an unhooked Codex approval and any dialog gx cannot read (login, onboarding, MCP elicitation, pickers) to show no card, so that the sheet only offers what gx can send safely.
30. As a person, I want a Queue agent's chat to show its open `gx tickets ask` question as a card, so that I can answer it without going to the ticket page.
31. As a person, I want a Queue agent's chat to show no permission or dialog cards, so that unattended agents stay on the ticket-ask path.
32. As a person, I want printed ❓ rounds shown as cards on the Agents page too, so that a plain chat's questions work like a Design's.
33. As a person, I want the same answer path for printed rounds on every page, so that the round prompt format is the same everywhere.
34. As a person, I want Huh? on printed round cards only, so that I never get an unclear explanation on a dialog, permission or plan card.
35. As a person, I want to mark Huh?, add notes and attach images on a printed round card, so that nothing the Design page lets me send today is lost.
36. As a person, I want the sheet to show only the live blocker when several sources are pending, so that I answer one thing at a time.
37. As a person, I want the other pending cards to return after the live blocker clears, if still current, so that nothing is lost.
38. As a person, I want the live blocker first, then a ticket ask, then a printed round, so that the most urgent card comes first.
39. As a person, I want a stale answer refused with a clear message when the question has changed or closed, so that my answer never goes to a different question.
40. As a person, I want my draft kept while the card stays the same and dropped when the card changes, so that I lose no typing and never send a draft to a new question.
41. As a person, I want a push notification for a dialog, Codex or permission card in a plain chat, throttled like the Design Question notification, so that I know a chat waits on me when I am away.
42. As a person, I want the banner and Screen button to stay on every blocked chat, so that I can always fall back to the terminal.
43. As a person, I want the Design page to send its answers through the same endpoint, so that the Design, Triage and Agents pages behave the same.
44. As a person on a phone, I want the sheet on the Agents page to work at phone width like the Design page's, so that I can answer from my phone.
45. As a developer, I want the decision "the hook is the primary path, keys are the fallback" recorded in an ADR, so that later work does not reverse it unknowingly.
46. As a person, I want a chat whose hook question is registered to count as Blocked with kind `question` until the hook exits, so that status, the banner, Actionable, live-blocker ordering and the push notification all show it waits on me.
47. As a person, I want conflict, doctor and Fix sessions on the Agents page to show permission, plan and printed-round cards (and the ticket-ask card where the session belongs to a ticket), with no dialog cards and no plain-chat push, so that I can answer them from the web without unattended sessions gaining the question tool.
48. As a person, I want a Codex card that appears before the overlay exists to refuse my answer with a "not ready, retry" message instead of sending keys, so that no key lands before the overlay can take it.
49. As a person, I want a Codex answer that stops with an error on the card when the overlay's header, question or cursor row does not match what gx expects, so that a stray terminal keystroke never mixes into my answers.

## Implementation Decisions

**Card model (server builds, web renders).**
- `GET /v1/sessions/chat` returns one `questions` list per chat. Each item has a source (Claude dialog, Codex question, permission, printed round, ticket ask), an id, a type (yes-no, options, multi-select, text) and the question data. The web only renders it.
- Multi-select is a new question type in the shared card. Its answers go to Claude as labels joined with `, `.
- Card ids per source: a hook card's id is the `tool_use_id`; a Codex card's id is the `call_id`; a keystroke-fallback card and a permission card use the screen dialog id plus a turn counter plus a hash of the full block; a plan card uses the plan file name plus the hash of the whole dialog block, so it differs per plan; a printed round uses the round id the server already parses; a ticket ask uses the ticket address plus the ask's id or hash. Stale refusal and draft keeping use these ids.
- Several sources pending: the sheet shows only the live blocker (permission, plan, Claude or Codex dialog, hook question). Order when all are pending: live blocker, then ticket ask, then printed round. The others return after it clears if still current.
- A card clears when its source says it closed: the hook exits, the rollout gains the matching `function_call_output`, the blocked state ends. A Screen or terminal answer therefore clears it too.
- A draft is kept while the card id stays the same and dropped when it changes.
- The Agents page chat reads the same list and shows the same sheet pattern the Design and Triage chat uses today, on the shared card.

**One answer endpoint.**
- New `POST /v1/sessions/answer` takes the card's source, id and structured answers (per-question answer, Huh? marks, notes, images). The server re-reads the source, refuses a stale id, and routes to the hook, the keys, the round prompt or the ticket answer code.
- Printed rounds use it too. The server composes the round prompt and sends it, so the round format lives in Go. The Design page's client-side round composer logic moves to the server, and the Design page calls the endpoint.
- Existing endpoints (`POST /v1/tickets/answer`, the permission read and Allow and Deny keys) stay for their current callers: the ticket page form and the banner.
- `POST /v1/sessions/keys` gains digit keys.

**Claude question dialog (plain Claude chats only; Design and Triage chats get no dialog cards).**
- A PreToolUse hook on `AskUserQuestion`, merged into the plain Claude launch's settings, is the primary read and answer path. It blocks until the web answers, then returns `allow` with `updatedInput` carrying `questions` and `answers` (keyed by the exact question text; multi-select labels joined with `, `; "Other" free text works on every question). Per-question notes go in `annotations` keyed by question text as `{notes, preview}`; the model reads them, the terminal summary does not show them.
- While a hook question is registered with the server, the session counts as Blocked with kind `question`, until the hook exits. That state drives the session status, the banner, Actionable, live-blocker ordering and the push notification.
- The hook never exits 2. When the server is down, or on any error, it prints nothing and exits 0, so the terminal dialog shows. At the 600 s hook timeout the terminal dialog takes over, the hook card clears and the keystroke fallback card shows for that dialog (ticket 08).
- Dismiss sends `deny` with the reason "The user doesn't want to proceed with this question. Do not ask again. STOP and wait for the user to tell you how to proceed." gx expects one short text turn from Claude and the pid going idle; the reply is not silent like Esc, because no `deny` reason reproduces that.
- "Answer in the terminal" releases the hook (prints nothing, exits 0), so the real dialog appears. gx remembers the released `tool_use_id` and shows no keystroke fallback card for that dialog. While the hook holds, the terminal shows "running PreToolUse hook" and the sheet is the only place to answer.
- Hook-path and Codex question cards show the note field. Keystroke-fallback, permission and plan cards do not.
- Fallback when the hook does not run: the pid reports `waiting` with `waitingFor` "input needed" and no hook-registered question after a short wait (the implementer picks the delay). gx then reads the dialog from the screen and answers by keys, supporting single-select, multi-select, several questions with the review step and "Other" text (in multi-select, move to Next after typing, since Enter unchecks Other). It re-reads the screen before each key and stops on any mismatch, shows an error on the card and leaves the dialog open for Screen. The fallback requires `waitingFor` "input needed": slash pickers report "dialog open" and permission and plan prompts report "permission prompt", so they are never mistaken for a question dialog. A layout gx cannot parse (wrapped numberless options at narrow widths, a scrolled list hiding the question, tab bar or cursor row) counts as a mismatch and gives no card. Pane width is not a spec decision: the server reads the screen at the session's real width. Dismiss on a fallback card sends Esc.
- Tested against Claude Code 2.1.319, under `--permission-mode auto`, which gx adds to every Claude launch. The hook's `updatedInput` answers are documented from 2.1.85 for headless use; other versions that ignore the hook land on the fallback.

**Codex question tool (plain Codex chats only).**
- Plain chats are launched with `-c features.default_mode_request_user_input=true`, drop the `request_user_input` guard hook group and skip the whole GX system prompt. A plain chat is a launch with no flow, ticket or fix-of. Queue, conflict, doctor, Design, Triage and Fix keep the block. Resume of a plain chat reuses the launch arguments plus the subcommand (`codex --no-daemon -c <flag> resume <id>`), opens straight to the chat with no daemon dialog, and keeps `request_user_input` (probe: `docs/research/regular-questions-codex-probes-3.md`).
- Plain Codex chats with the flag also get `--no-daemon`, and startup-dialog detection learns to answer the "Background server has incompatible feature settings" dialog with digit 1 ("Run without daemon this time"), so a stray daemon cannot stall startup. Other Codex launches are unchanged.
- Read: the open `request_user_input` `function_call` in the rollout (id, header, question, 2 to 3 options with label and description, no multi-select; the client adds a free-text row). The call is on disk 2.7 to 3.7 s before the overlay shows. Card closes when the rollout gains a `function_call_output` with the same `call_id`. A submit re-reads the in-flight `call_id` and refuses a stale one.
- A Codex card is listed from the rollout as soon as the call is on disk, but the answer endpoint sends keys only once the session is blocked with kind `question` (OSC 7501) and the screen shows the overlay. Before that it returns a "not ready, retry" error, which the card shows.
- Answer by keys: a digit picks that option and advances, and the last digit submits with no Enter. The cursor resets to the top row on every question, so moves are position-independent: option k is digit k, and "None of the above" (the free-text row) is digit (option count + 1). A note on an option is Down x (k-1), Tab, text, Enter; a note and a digit cannot combine, so the server sends the digit when there is no note and the note sequence when there is one. A free-text answer on "None of the above" is Down to the last row, Enter, text, Enter. Before each question's keys the server re-reads the screen and checks the header `Question N/M (U unanswered)`, the question text (wrapped lines joined and whitespace normalised), the row count and each row label against the card, and the `›` cursor row. After a digit it waits a bounded time for the next expected header before counting a mismatch (the implementer picks the delay). A layout it cannot parse counts as a mismatch. On a mismatch it sends no further keys and shows an error on the card, leaving the overlay open for Screen. If it stops before any key was sent, the card keeps its retry. If it stops after some keys were sent, the overlay already holds earlier answers, so the card disables Submit for that `call_id` and points to Screen. Output is matched to questions by id, not by object key order. Every question must be answered before Submit.
- Codex question cards have no dismiss; the person uses Screen to interrupt (Esc writes an abort string and `turn_aborted`).
- Codex permission cards: only the hooked case, from the Codex hook file, with Allow and Deny. Unhooked approvals show no card.
- `gx codex doctor` runs `codex features list` with the launch's `-c` flag string and fails, naming the flag, unless the `default_mode_request_user_input` row exists, its stage is not `removed` and its state is `true` (an unknown `features.*` key passed with `-c` is only warned about and ignored, so exec exit codes cannot detect it). There is no automatic fallback to printed rounds.
- Tested against codex-cli 0.166.0 and 0.167.0. The cursor reset, the `features list` columns and resume were probed on 0.167.0 only.

**Permission prompts and plan approval (Claude).**
- Any Claude permission or plan dialog whose screen parses into numbered options becomes a card, answered by digit key. It shows every option with its exact label. Nothing is preselected, and a standing grant (for example "always allow") gets no recommended badge. A dialog that does not parse gets no card, only the banner and Screen.
- gx fixes the wrapped-option-row parse so wrapped rows do not truncate the option list.
- The plan card reads the full plan from the plan file the dialog's last row names (read at 80, 120 and 160 columns). With the file unreadable, the card shows the options only. The parse marks the text row by its label "Tell Claude what to change" and sends that row's parsed digit (not a fixed 3); Submit stays disabled while that row's text is empty, and the text is typed into the inline feedback box and submitted with Enter. A plan dialog without that label gets plain digit options. Dismiss sends Esc, which rejects the plan and leaves the session idle in plan mode. On a permission card, dismiss does what the banner's Deny does today.
- The Allow once shortcut on the banner stays strict. Standing-grant options are labelled and never the default.
- Design and Triage chats get permission and plan cards, printed-round cards and the ticket-ask card, but not dialog cards.
- Conflict, doctor and Fix sessions (Claude or Codex) on the Agents page get permission, plan and printed-round cards, plus the ticket-ask card where the session belongs to a ticket. They get no dialog cards (Claude AskUserQuestion is banned there, and Codex keeps its question-tool block) and no plain-chat push.

**Queue agents.** Their chat shows only the ticket-ask card, answered through the new endpoint, which calls the existing ticket answer code (prompting the live agent). No permission or dialog cards; banner and Screen stay.

**Huh?** On printed ❓ round cards only.

**Notifications.** A push for a dialog (including a registered hook question), Codex or permission card in a plain chat, throttled like the Design Question notification and decided by the open-on-screen rule.

**Shared decisions.** No ticket or epic schema change, no new pause kind and no new config key. The names of the hook marker, launch field and internal types are the implementer's choice.

**Glossary and ADR.** Add three terms to `GLOSSARY.md`: **Question sheet** (the panel in a web chat that shows the chat's question cards, which the Design and Triage chats call the Questions view today), **Question card** (one item in the sheet, built by the server from one source, with a source, id and type), and **Live blocker** (the permission, plan or dialog prompt that currently blocks a session; only its card shows while it is open). Add an ADR recording that the hook is the primary path for Claude's question dialog and keys are the fallback.

## Testing Decisions

- A good test drives external behavior only: what the chat response lists, what the endpoint accepts or refuses, what reaches the fake agent, what the page shows and sends. It does not assert internal markers or parsing helpers.
- Highest seam: Go server tests through `servertest`, with `agentfake` extended to open a Claude dialog, a Codex `request_user_input` call, a permission prompt, a plan approval and a ticket ask, and to record the keys, hook answers and prompts it receives. Cover: card building, the answer endpoint per source, stale refusal, live-blocker ordering, card clearing, the keystroke fallback stopping on a mismatch, and the launch differences between plain and non-plain Codex sessions.
- vitest for the shared card: the multi-select type, the required-answer rule for Submit, the required text row, and draft keeping and dropping on id change.
- Existing e2e flows host the web checks: `agents-permission.e2e.ts` and `codex.e2e.ts` get Agents-page sheet steps, and the Design flows cover the moved round composer. A new flow is added only if no existing scenario can host a step.
- Prior art: `agents-permission.e2e.ts` (Allow once and Deny, Codex hook and screen), `chat-images.e2e.ts` (Design composer round), the design answer flows in `design.e2e.ts`, and `cmd/codex/codex_doctor_test.go` for the doctor probe.

## Verification

A verify ticket runs these against a temporary gx server. The real Claude and Codex binaries are not in the fixture, so the dialog, question-tool and plan checks run against the extended `agentfake` fixture sessions. Steps named "new" are written as implementation tickets, because the catalog does not cover them.

1. Claude dialog card on the Agents page. Flows: extend 157 and 158 in `agents-permission.e2e.ts` with a Claude-dialog step. Checks: `GET /v1/sessions/chat` for the fixture chat lists a `questions` item with source Claude dialog; `gx agents chat` for the session shows the answer afterwards. New: an `agents` scenario step that opens a fake Claude dialog.
2. Dialog content. Check: the chat response carries each question's header, options, descriptions and multi-select flag. New: unit-level `servertest` case with a three-question dialog (one multi-select).
3. "Other" answer. Check: `POST /v1/sessions/answer` with free text on a question returns exit 200 and the fake agent receives that text as the answer. New flow step in the story 1 flow.
4. Note on hook card. Check: the fake agent's hook result carries `annotations` keyed by question text. New `servertest` case.
5. Submit gating. Flow: story 1 flow asserts Submit disabled until every question is answered. Vitest for the card.
6. Dialog dismiss. Check: dismiss returns `deny` with the Esc-style reason in the fake hook result and the card clears. New flow step.
7. Dismiss behavior. Check: the same dismiss result carries the exact reason text from Implementation Decisions. The model's reply is recorded in `docs/research/regular-questions-claude-hook-probes-4.md` and is not re-run by this epic; verify the reason string only.
8. Answer in the terminal. Check: the action makes the fake hook print nothing and exit 0 and the card clears; while that dialog stays open the chat response lists no keystroke fallback card for the released `tool_use_id`. New flow step and `servertest` case.
9. Card clearing. Check: answering through the fake agent's terminal side (or Screen keys) clears the card without a reload. Flow: story 1 flow, second step.
10. Hook fail-open. Check: with the server stopped the fake hook exits 0 with no output (`servertest` hook binary test, exit code 0, empty stdout); at the 600 s timeout the hook card clears and the chat response then lists the keystroke fallback card for that dialog (test with a shortened timeout; ticket 08).
11. Keystroke fallback card. Check: a fake Claude with hooks disabled reaches `waiting` / "input needed" and `GET /v1/sessions/chat` lists a keystroke card; answering sends the expected keys. New `agentfake` scenario.
12. Mismatch stop. Check: a screen that changes between keys makes the answer endpoint return an error, sends no further keys, and the card shows the error. New `servertest` case.
13. No note on fallback. Flow: story 11 flow asserts the note field is absent. Vitest for the card.
14. Codex question card. Flow: extend `codex.e2e.ts` with an Agents-page sheet step. Check: the chat response lists a Codex question card from the rollout's open `request_user_input` call. New: a fixture rollout with that call.
15. Codex options and free-text row. Check: card options carry labels and descriptions and a free-text row. Covered by the story 14 flow.
16. Codex note. Check: answering with a note makes the fake overlay receive Down, Tab, text, Enter, and without a note receive the digit alone. New `servertest` case.
17. Codex several questions. Check: a three-question call is answered by digit keys, the last digit submits, and the rollout output is matched by id. New `servertest` case.
18. Codex startup dialog. Check: a fake Codex that shows the "incompatible feature settings" dialog gets digit 1 and the session reaches idle; the plain chat's launch arguments, and its `Resume` arguments, include `--no-daemon` and the flag. New `servertest` case on launch arguments and startup detection.
19. Doctor. Check: `gx codex doctor` exits non-zero and names the flag when a fake `codex features list` lacks the row, shows it `removed` or shows it `false`, and exits 0 when the row is present and `true`. Extend `cmd/codex/codex_doctor_test.go`.
20. Non-plain Codex keeps the block. Check: launch arguments for Queue, Design, Triage, conflict, doctor and Fix sessions still install the guard hook and omit the flag and `--no-daemon`, for both `Launch` and `Resume`. `servertest` launch-argument cases.
21. No GX system prompt on a plain Codex chat. Check: the plain chat launch carries no GX system prompt; the Queue launch still does. Same `servertest` cases.
22. Permission card options. Flow: extend 158 with the Agents-page sheet step. Check: the card lists every option with its exact label, including "always allow"; answering sends the digit. New: wrapped-row fixture screen.
23. No preselection. Flow: story 22 flow asserts no option is selected and no recommended badge. Vitest for the card.
24. Plan card. Check: the card shows the text of the plan file and its id changes with a different plan. New: plan scenario and plan file in the fixture. Flow: new step in the story 22 flow.
25. Plan option 3. Flow: story 24 flow asserts Submit disabled while the text row is empty, then the fake receives that row's parsed digit, the text and Enter. Check: a plan fixture whose text row carries the label at a digit other than 3 receives that digit, and a plan fixture without the label lists plain digit options with no text row. New flow step and `servertest` case.
26. Plan and permission dismiss. Check: the fake receives Esc. Flow: story 22 and 24 flows.
27. Unparsable dialog. Check: a fixture screen that does not parse yields no card and the banner and Screen stay. New `servertest` case; flow: story 22 flow.
28. Codex hooked permission. Flow: 158 (Codex hook step) gets the Agents-page card step. Check: Allow and Deny are the only options.
29. Unreadable dialogs. Check: unhooked Codex approval, login, MCP elicitation and picker screens give no card, only the banner. New `servertest` cases.
30. Ticket ask card on a Queue agent. Check: `gx tickets ask` from a fixture worker shows a card in the agent's chat, `POST /v1/sessions/answer` answers it and the ticket resumes, same effect as `POST /v1/tickets/answer`. New flow step in a Queue-agent scenario.
31. No permission or dialog cards on a Queue agent. Check: the chat response for a Queue agent in a permission prompt lists no permission card; the banner stays. New `servertest` case.
32. Printed round on the Agents page. Check: a fixture chat that prints a ❓ round lists printed-round cards on the Agents page. New flow step in `agents-session.e2e.ts` or the `agents` scenario.
33. Same answer path. Flow: design answer flows in `design.e2e.ts` ("First step, ids, answer, Continue and a fresh round", "An all-defaulted round") and `chat-images.e2e.ts` 189 (design composer round) keep passing through the endpoint. Check: the prompt the agent receives is identical whether sent from the Design or the Agents page.
34. Huh? only on printed rounds. Flow: story 32 flow asserts Huh? shows on printed cards and on no dialog, Codex, permission or plan card.
35. Huh?, notes and images on a printed round. Flow: 189 design composer round; Check: images and marks arrive in the round prompt.
36. One live blocker. Check: with a permission prompt and a printed round pending, the chat response lists only the permission card. New `servertest` case.
37. Cards return. Check: after the live blocker clears the printed round is listed again. Same case.
38. Order. Check: ticket ask is listed before a printed round when no live blocker exists. New `servertest` case.
39. Stale answer. Check: answering with an old card id returns an error and sends nothing to the agent. New `servertest` case per source, each using that source's id (hook `tool_use_id`, Codex `call_id`, dialog id + turn counter + block hash, plan file name + block hash, round id, ticket address + ask id or hash).
40. Drafts. Vitest: a draft is kept on same id and dropped on a new id, for each source's id form. Flow: story 1 flow retypes after a new dialog.
41. Notifications. Check: a plain-chat dialog, Codex or permission card produces one push, a second within the throttle produces none. New `servertest` case beside the Design Question notification tests.
42. Banner and Screen stay. Flow: 157 (Close dialog, Agents page and design page) keeps passing with cards visible.
43. Design page through the endpoint. Flow: `design.e2e.ts` answer flows and `triage.e2e.ts` 199 (the questions link and "Send back to the agent" step) keep passing.
44. Phone width. Flow: `phone.e2e.ts` 24 (Phone, Design page) plus a new step on the Agents page at 390 px: the sheet shows, no sideways scroll.
45. ADR. Check: the ADR file exists and states the hook as primary and keys as fallback; `GLOSSARY.md` has the three new terms (`grep` for Question sheet, Question card, Live blocker).
46. Hook question counts as Blocked. Check: while the fake hook holds a registered question, `GET /v1/sessions/chat` and `gx agents list` show the session as waiting (Blocked, kind `question`), Actionable includes it and the banner shows; after the hook exits the state clears. New flow step in the story 1 flow and `servertest` case.
47. Conflict, doctor and Fix sessions. Check: for a fixture conflict, doctor and Fix session (Claude and Codex), the chat response lists permission, plan and printed-round cards, plus the ticket-ask card for a session that belongs to a ticket, lists no dialog card even with a fake Claude AskUserQuestion or Codex question call open, and produces no plain-chat push. New `servertest` case.
48. Codex answer before the overlay. Check: with the rollout call on disk but the session not yet Blocked with kind `question` (no OSC 7501) or the screen not showing the overlay, the card is listed, the answer endpoint returns a "not ready, retry" error, sends no keys and the card shows the error; once both hold, the same answer sends the keys. New `servertest` case.
49. Codex overlay mismatch. Check: with a fake Codex overlay whose header, question text or `›` cursor row differs from the card's expectation before a question's keys (including a second or later question), the answer endpoint sends no further keys and the card shows the error; with a matching three-question overlay whose cursor reset to row 1 on each question, option k is sent as digit k and "None of the above" as digit (option count + 1). Also: a header that redraws late within the wait is not a mismatch; a changed row count or row label is; a wrapped question compares equal after joining; a stop before any key keeps retry; a stop after keys disables Submit for that `call_id` and shows the Screen pointer. New `servertest` case.

## Out of Scope

- The TUI: this effort covers the web only.
- The ticket page's own single-card answer form stays as it is.
- An automatic printed-round fallback when Codex's question flag stops working; `gx codex doctor` reports the failure only.
- Cards for unhooked Codex approvals, MCP elicitation, login, onboarding, pickers and any dialog gx cannot parse; the banner and Screen cover them.
- Permission or dialog cards for Queue agents, and dialog cards on Design and Triage chats.
- A dismiss on Codex question cards.
- Claude Code versions other than the tested 2.1.319 and codex-cli versions other than 0.166.0 and 0.167.0 (other versions land on the fallbacks above).
- Codex daemon option 2 ("Restart with these settings").
- Codex's in-chat `/fork` (it writes a new rollout file and was never probed).

## Further Notes

- Research behind this spec is on this branch under `docs/research/regular-questions-*.md`.
- The `gx tickets ask` card is answered through the same ticket answer code as the ticket page, so a ticket answered on the web in either place clears the other.
