## Problem Statement

A person who wants to rename a chat has to open it and type `/rename <name>` in the Message box. There is no menu item, no chord and no palette action. The person must also know that `/rename` exists, and the Message box refuses text while the chat is not Idle. A chat keeps the name of its terminal title until someone renames it, so a long-running chat often carries a name the person did not choose.

## Solution

A chat can be renamed from the web UI in three ways:

- the Leader chord `{leader} r`,
- the "Rename chat" Command palette action,
- a Rename item in the ⋯ menu of the chat's sidebar row and of the chat page.

Each opens a small popup that shows the current name, selected. Enter renames the chat in gx and, when the chat's agent can take it, sends the agent's own `/rename <name>` to Claude or Codex, so the name matches in the agent's own session list and survives resume. Esc cancels. An empty name or a name with a line break disables Enter.

What happens to the agent depends on the Session status:

- Working or Idle: gx renames and sends `/rename <name>` at once. Both agents run it at once, mid-turn too (research 01).
- Blocked (a permission or approval popup is open): gx renames now and holds the agent command. It sends the command when the popup closes.
- Starting, Rate-limited or Exited: gx renames only. No agent command is sent.

The new name wins over the terminal title from then on, as a typed `/rename` does today.

## User Stories

1. As a person with a chat open, I want to press `{leader} r`, so that I can rename it without leaving the keyboard.
2. As a person with a chat open, I want a "Rename chat" action in the Command palette, so that I can find rename by name when I forget the chord.
3. As a person looking at the sidebar, I want a Rename item in a chat row's ⋯ menu, so that I can rename a chat without opening it first.
4. As a person on a chat page, I want a Rename item in the page's ⋯ menu, so that I can rename the chat I am reading.
5. As a person, I want the popup to open with the current name already selected, so that I can type a new name at once or edit a few letters of the old one.
6. As a person, I want Enter to rename the chat, so that renaming is one keystroke after typing.
7. As a person, I want Esc to close the popup and change nothing, so that I can back out safely.
8. As a person, I want Enter disabled for an empty name, so that I cannot wipe a chat's name by mistake.
9. As a person, I want Enter disabled for a name with a line break, so that a pasted multi-line text cannot break the agent's command.
10. As a person, I want Enter on an unchanged name that I already pinned to just close the popup, so that no useless `/rename` reaches the agent.
11. As a person, I want the new name to show in the sidebar row, the chat page header and the Agents tab at once, so that I see the rename took effect.
12. As a person, I want the new name to win over the terminal title, so that the agent's own title updates never overwrite my name.
13. As a person renaming a Claude chat, I want gx to send Claude's `/rename`, so that `claude --resume <name>` finds the chat.
14. As a person renaming a Codex chat, I want gx to send Codex's `/rename`, so that `codex resume <name>` finds the chat.
15. As a person renaming a chat whose agent is Working, I want the rename to go through at once, so that I do not wait for the turn to end.
16. As a person renaming an Idle chat, I want the rename to go through at once, so that the agent's session list matches.
17. As a person renaming a chat while a permission or approval popup is open, I want the gx name to change now, so that the UI shows my name straight away.
18. As a person in that case, I want the agent command held and sent after the popup closes, so that my typed text never lands in the popup.
19. As a person who renames twice while a popup is open, I want the latest name to replace the held command, so that the agent ends up with the name I chose last.
20. As a person, I want a held command dropped when the chat exits, restarts or goes Rate-limited, so that a stale rename never reaches a different run of the agent.
21. As a person, I want a held command dropped when the popup closes into Starting or Rate-limited, so that gx does not type into an agent that cannot take it.
22. As a person, I want a held command to send at once when the popup closes into Working, even mid-turn, so that the hold ends as soon as the agent can take it.
23. As a person, I want no extra indicator for a held command, so that the UI stays quiet for a rare case.
24. As a person renaming a chat that is Starting or Rate-limited, I want gx to rename it and send nothing, so that nothing is typed into an agent that cannot take it.
25. As a person renaming an Exited chat, I want only the gx name to change, so that the rename works without a live agent.
26. As a person, I want the gx name to keep after a server restart, so that a restart never loses a rename.
27. As a person, I want a failed agent-side rename to show the existing "Send failed" card, so that I learn the agent did not rename, as with any failed send.
28. As a person on a Codex chat, I want an "Unrecognized command" reply to raise that card too, so that a Codex that does not know `/rename` is not silent.
29. As a person, I want Rename only on chats, so that a Design session keeps the name of its Design and a Queue agent keeps the name its loop gave it.
30. As a person on a Design page, a Queue agent page or a Fix agent page, I want `{leader} r` and the palette action hidden, so that I do not see an action that does nothing.
31. As a person on any page that is not a chat, I want `{leader} r` absent from the Chord box, so that the box lists only keys that work.
32. As a person, I want the chord and palette action to rename the chat open on the current page, so that I never rename a chat I cannot see.
33. As a person typing in the Message box, I want `{leader} r` to work as the other chords do, so that I do not have to click out first.
34. As a phone user, I want the Rename item and popup to work at 390 px, so that I can rename from my phone.
35. As a person, I want the popup to keep working with the keyboard only (focus in the field on open, Tab reaches Rename and Cancel), so that mouse-free use stays possible.
36. As a person scripting gx, I want a `gx agents rename` verb that renames a chat, so that the CLI mirrors the web UI as the server API rule asks.
37. As a person who still types `/rename <name>` in the Message box, I want that to keep working as today, so that the new feature breaks no habit.
38. As a person, I want a rename the server refuses (unknown chat, a Design session, a Queue agent, a Fix agent, a bad name) to show its reason in the popup and keep my text, so that I can fix and retry.
39. As a person whose chat still carries an automatic name from the terminal title, I want the popup to say so, so that I know the name was not chosen by me.
40. As a person who confirms that automatic name with Enter, I want it pinned as my name and sent to the agent, so that the next terminal-title change does not overwrite it.
41. As a person who typed a draft in the agent's own terminal, I want Rename to behave like the Message box does (the draft is cleared before the command is typed), so that the rename goes through unless the draft will not clear, which shows the Send failed card.
42. As a person, I want a rename never to mix with any other text gx types into the chat (a composer prompt, a queued action step, an auto-advance Continue, the first prompt or another rename), so that the agent's input is never corrupted.

## Implementation Decisions

- **One server rename verb.** A new session rename verb takes a chat id and a name. It does both the gx rename and the agent send, so the page makes one call and the rule sits in one place. It follows the server API rule: one `POST /v1/...` verb route, one `gx agents rename` verb with `--json`, refusals as results with a stable reason. The existing route-table-versus-CLI-tree test covers the pairing.
- **Why not the prompt route.** The prompt route refuses any session that is not Promptable (Idle). Rename must also work while Working and while not live. The rename verb therefore does not go through that gate. The page's stale-conversation check (`LastMessage`) does not apply either: a rename does not depend on what the conversation shows.
- **Eligibility.** The verb accepts a chat: a plain session with no Session ticket, not a Design session, not a Queue agent, not a Fix agent (the same shape the web uses for "Remove chat"). It refuses anything else with the existing reasons (`unknown-session`, `queue-session`, `not-plain-session` for a Design session and a Fix agent, `text-too-long`) and one new reason, `bad-name`, for an empty name or one with a line break. Reason strings are a public contract (ADR 0028). The route is `POST /v1/sessions/rename` and the verb is `gx agents rename`, matching the neighbouring session verbs (`gx agents` is the CLI group of the session verbs).
- **Name rules.** The name is trimmed. An empty name or one holding a line break is refused, on the server too, with the same check the typed-command path uses (`renameCommandArg` rules). The name length follows the prompt text limit the server already applies. Duplicate names are allowed, as with a typed `/rename` today. A name equal to the current one changes nothing and sends nothing, but only when the name is already pinned (the auto-name flag is clear). When the current name is still an automatic name from the terminal title, the same name is a real rename: it pins the name and sends the command.
- **gx name.** The verb sets the chat's name and clears the auto-name flag, exactly as the typed `/rename` path does, and publishes the session-changed event. That setting logic is shared with the typed path, not copied: the rename moves into one function that both call. The gx name is written for every status, including Exited, before any agent send is considered.
- **Agent command by status.** The server builds the command (`/rename <name>`, the same form for Claude and Codex), so the prefix rule stays closed to the page. Working or Idle (including Working with a background task): send now. Starting, Rate-limited, Exited: send nothing. Blocked: hold.
- **Sending.** The send reuses the one-Enter command path (the one `typeCommandOnce` is) and its failure handling: a failed send raises the "Send failed" gx Actionable, and a Codex "Unrecognized command" raises it too. A successful send clears an earlier "Send failed" card, as it does for the composer. The send runs in the background so it outlives the request.
  - The send re-reads the Session status just before it types, with the same table as the request: Working or Idle types, Blocked holds, anything else (Exited, Starting, Rate-limited, restarted) drops the command.
  - Rename is typed like the Message box types: the agent's input is cleared first (Ctrl+U), so a draft typed in the agent's terminal is lost. This is accepted, because the same happens to any composer prompt.
  - **One typing turn per chat.** The prompt package gets one per-chat lock, held for one clear, type and Enter, never for the agent's whole turn. Every typer takes it: the composer send and its nudges, an action step, auto-advance Continue, the first prompt, the hold release and the rename. A typer that finds it taken waits. This fixes the cause (no typer shares the input today) instead of covering rename against rename only.
  - **One pending-rename slot per chat**, shared by the send and the hold. Only a newer request may replace what the slot holds, so an older queued rename that re-reads Blocked late can never replace a newer held name.
  - The Message box still refuses text while the chat is not Idle. No queue of prompts is added.
- **The hold lives in the prompt package**, as a small per-session holder of a pending rename, beside the code that already sends and watches status. It is in memory only. It is not written to the session's meta and not persisted. Reasons: a restart must lose it (decided), and the session meta is shared state the pages read.
  - A new rename for the same chat replaces the held one.
  - A watcher on the chat's Session status releases the hold. A status leaving Blocked into Working or Idle sends the held command at once, even mid-turn. A status leaving Blocked into Starting or Rate-limited, or the chat going Exited or restarting, drops the held command. A Blocked chat that goes to Rate-limited drops it too.
  - The watcher follows the status stream the server already keeps, not a poll of its own, and it ends when the hold is released or dropped.
  - The existing `SendAction` flow is not reused for the hold: it stops when the session goes Blocked and sends only from Idle, which is the opposite of what the hold needs. The hold is a new mechanism, not a flag on `SendAction`.
  - The hold is not shown in the UI and not in the snapshot.
- **Web UI: one popup.** A single rename popup component takes the chat and is opened from three entry points. It starts with the current name selected, focus in the field, Enter submits, Esc cancels, and Enter is disabled for an empty name or a name with a line break. A refusal from the verb shows in the popup and keeps the typed text. When the chat's current name is an automatic name (auto-name flag set), the popup shows a muted line saying so, such as "Automatic name from the terminal title". The flag is `auto_name`. It is added to the session snapshot type and the generated web types, and both a terminal-title rename and a pin publish it through the session-changed event. The component follows the existing popup and dialog patterns of the web UI (focus handling, the phone sheet).
- **Entry points.**
  - The ⋯ menu: a Rename item in the chat's session menu (the one the sidebar row and the chat page share), shown for a chat only. A Design session, a Queue agent and a Fix agent do not get it.
  - The Leader chord: `r`, labelled "Rename chat", available only on the page of an open chat. It is absent from the Chord box elsewhere.
  - The Command palette: a "Rename chat" action with the same availability as the chord.
  - All three open the same popup for the chat in question: the menu for its own chat, the chord and palette for the chat open on the page.
- **Generated types and the settings guard.** The new request and result types go through the existing generated-types flow. No config key is added, so the settings guard and registry are untouched.
- **Docs.** GLOSSARY.md gets no new term: the feature uses Chat session, Command palette, Leader key, Chord box and Session status. `webui/e2e/CATALOG.md` is updated for the new flow steps.
- **ADRs.** No ADR changes. ADR 0042 (pauses) is not touched: a held rename is not a pause, it shows nothing and asks nothing of the person.

## Testing Decisions

- A good test checks behaviour a person or a script sees: the name in the snapshot, what the agent's input receives, the card that shows, the popup states. It does not check the holder's fields or the watcher's wiring.
- **Server seam: the prompt package's existing test seam** (`sessionprompt_test.go`, `send_internal_test.go`, `action_test.go`, with the fake holder the neighbouring tests use). Cases: Working, Idle, Starting, Rate-limited and Exited each; Blocked then closing into Working, into Idle, into Starting, into Rate-limited and into Exited; two renames while Blocked (latest wins); a restart of the chat while held; a Codex "Unrecognized command" raising "Send failed"; refusals for a Design session, a Queue agent, a Fix agent, an empty name and a line break; an unchanged pinned name; an unchanged automatic name (pinned and sent); a chat that turns Blocked between the request and the send; a rename against a composer send; two sends on one chat in a row.
- The hold is in memory only, so no restart test is needed for it.
- **Route and CLI seam: the `cmd/serverapi` verb tests and the route-table-versus-CLI diff test** for the new verb and its `--json` output.
- **Web unit seam: vitest for the popup** (selected current name, Enter, Esc, disabled states, refusal shown with text kept) and for the chord and menu rules (availability per page kind, label), next to the existing chord, `SessionActions` and Command palette tests.
- **Playwright: extend, do not add.** Per the repo rule, no new flow unless a server round trip or layout needs it. The round trip does need it, so the Chords flow (scenario `workspaces`) and the sidebar-row and ⋯ menu coverage gain steps. The implementer fits them in the existing flows within the `playwrightguard` flow budget (`gx-playwright-e2es` rules apply: wait before state reads, no reload).
- Prior art: `renameCommandArg` tests for the name rule, the Stop and Remove menu tests for menu items, the Chords flow's `x` step for a chord that acts on the open chat.

## Verification

Run these against a temporary gx server after the epic is built. The `workspaces` scenario holds a chat, a Design, a Queue agent and Fix agents, so it covers eligibility. `gx-ui-check` drives the flows in a real browser.

1. Story 1: flow Chords (`webui/e2e/chords.e2e.ts`, scenario `workspaces`), new step: on the chat's page press the Leader key then `r`; the popup opens. Playwright flow to extend.
2. Story 2: flow Chords or the palette step: open the Command palette on a chat page, type "Rename", the "Rename chat" action opens the popup. Extend the existing flow.
3. Story 3: flow Sidebar rows (`sidebar-rows.e2e.ts`) or Workspaces: open ⋯ on a chat's sidebar row, choose Rename, the popup opens. Extend.
4. Story 4: flow Workspaces or `agents-session.e2e.ts`: the chat page's ⋯ menu has Rename and it opens the popup. Extend.
5. Story 5: vitest for the popup (selection) and the flow in story 1 asserts the field holds the current name selected.
6. Story 6: flow in story 1: type a new name, Enter; the sidebar row, the page header and `gx agents list` show it.
7. Story 7: popup vitest plus a flow step: Esc closes it, the name is unchanged in `gx agents list`.
8. Story 8: popup vitest: Enter is disabled for an empty name. Flow step: the Rename button is disabled.
9. Story 9: popup vitest: a pasted line break disables Enter. Server Go test: the verb refuses a name with a line break.
10. Story 10: Go test in the prompt package: an unchanged pinned name writes nothing and sends nothing. CLI check: `gx agents rename <chat> <same name> --json` returns success and the agent input log shows no `/rename`.
11. Story 11: flow in story 6: the new name shows in the sidebar row, the page header and the Agents tab without a reload.
12. Story 12: Go test: after a rename, a terminal-title change does not change the chat's name.
13. Story 13: Go test (prompt package, Claude fake): the agent input receives `/rename <name>` and one Enter. Flow with a Claude chat in `workspaces`: `fixture.input(conv)` shows it.
14. Story 14: same as 13 with the Codex fake (the `codex` scenario or a Codex chat in the new step).
15. Story 15: Go test with the session Working: the command is sent at once. Also a step in an existing flow with the Working fake agent: it receives `/rename` while its turn runs.
16. Story 16: Go test and flow step with an Idle chat.
17. Story 17: Go test with the session Blocked: the gx name changes, the agent input receives nothing yet. Flow: extend `agents-permission.e2e.ts` (its Claude chats reach Blocked with `BASH_DIALOG`): rename from the open page; the sidebar shows the new name while the card is open.
18. Story 18: same flow, after the permission is answered `fixture.input` receives `/rename <name>`; typed text never reaches the card.
19. Story 19: Go test: two renames while Blocked; after the close, the agent receives only the latest.
20. Story 20: Go tests: chat exits, restarts and goes Rate-limited while held; nothing is sent, the gx name stays.
21. Story 21: Go tests: Blocked closes into Starting and into Rate-limited; nothing is sent.
22. Story 22: Go test: Blocked closes into Working; the command is sent at once, mid-turn.
23. Story 23: manual look with `gx-ui-check` during the story 17 flow: no indicator shows for the hold.
24. Story 24: Go tests for Starting and Rate-limited: gx name changes, no agent input. CLI check: `gx agents list` shows the new name.
25. Story 25: Go test and flow with an Exited chat (the `workspaces` scenario has one after Stop): the name changes, the verb succeeds, nothing is sent. CLI: `gx agents list` shows the new name.
26. Story 26: Go test: the name is written to the session meta. CLI check: the restart helper in `restart-helpers.ts`, then `gx agents list` shows the name.
27. Story 27: Go test with a failing send raises "Send failed" with the command in its action. Go only.
28. Story 28: Go test with a Codex pane that draws "Unrecognized command": the "Send failed" card is raised (the existing `checkCodexKnownCommand` test seam).
29. Story 29: Go tests: the verb refuses a Design session and a Queue agent. Flow: the Design page's and the Queue agent's ⋯ menu have no Rename.
30. Story 30: flow Chords: on a Design page, a Queue agent page and a Fix agent page the Chord box shows no `r` row, pressing `r` does nothing, and the palette lacks "Rename chat". Extend the existing "c and d only" step.
31. Story 31: same flow: the epics page and the agents list show no `r` row.
32. Story 32: flow: with two chats in the sidebar, open one and rename by chord and by palette; the open one is renamed, the other is not.
33. Story 33: flow: type a draft in the Message box, press the Leader key then `r`; the popup opens and the draft stays.
34. Story 34: flow `workspaces-touch.e2e.ts` or `phone.e2e.ts` at 390 px: Rename from the ⋯ menu, the popup fits the viewport with no sideways scroll, rename works.
35. Story 35: flow step: popup field has focus on open; Tab reaches Rename and Cancel; Enter and Esc work from the keyboard.
36. Story 36: CLI check: `gx agents rename <chat> <name> --json` prints the result and exits 0; with a Design id it prints the refusal reason and exits non-zero as the other refused verbs do. The route-table-versus-CLI-tree test passes.
37. Story 37: existing flow `agents-commands-send.e2e.ts` and Go `renameCommandArg` tests still pass: typing `/rename <name>` in the Message box renames the chat as before.
38. Story 38: popup vitest and a flow step: a refusal reason shows in the popup and the text stays.
39. Story 39: popup vitest: the muted automatic-name line shows when the auto-name flag is set and is absent when it is clear. Go check: the snapshot carries `auto_name`, and the session-changed event fires on a pin and on a title rename.
40. Story 40: Go test: Enter on an unchanged automatic name clears the flag and sends `/rename`; a later terminal-title change does not change the name.
41. Story 41: Go test: the send clears the input before it types, as the Message box does. Go test with a draft that will not clear: the Send failed card is raised.
42. Story 42: Go tests: a rename while a composer send is typing waits for it and the two never mix; two renames in a row type one after the other; rename A queued, the chat turns Blocked, rename B arrives, and after the popup closes the agent gets B; a rename whose chat turned Exited while it waited is dropped.

New flows or scenarios to write (these become implementation tickets):

- Chords flow steps for `r` (chat opens the popup; Design, Queue agent, Fix agent and non-agent pages hide it), plus the palette and the two ⋯ menus.
- Steps in `agents-permission.e2e.ts` for the hold-then-release flow, using `fixture.input`.

No stored mock exists for this map, so there is no Mock states list.

## Out of Scope

- Renaming Design sessions, Queue agents and Fix agents. A Design session's name comes from its Design, and a Fix agent's name says what it fixes. This is the same shape as "Remove chat".
- Renaming in the TUI.
- Sending `/rename` to an Exited chat after Restore. An Exited chat is renamed in gx only.
- A UI indicator for a held command.
- Keeping a held command across a server restart.
- Bare `/rename` (letting the agent pick a name) from the popup. The popup always sends a name.
- Making names unique across chats.
- A queue of prompts held while the agent works. The Message box keeps refusing text when the chat is not Idle.

## Self-decided design questions

- Fix agents excluded from Rename. Choice: excluded (confirmed in the review grilling). Reason: their name says which ticket they fix, and "Remove chat" excludes them the same way.
- Name trimmed. Choice: trim leading and trailing whitespace. Reason: the typed `/rename` path does the same.
- Duplicate names. Choice: allowed. Reason: the typed `/rename` allows them today.
- One verb does the gx rename and the agent send. Choice: one verb. Reason: the page makes one call and the rule sits in one place.
- Draft in the agent's terminal. Choice: cleared, as the Message box does (decided in the review grilling).

## Further Notes

- Research 01 (branch `research/rename-chat-rename-behavior`): Claude Code 2.1.319 and Codex 0.169.0 run `/rename <name>` inline and at once mid-turn, keep the name across resume and update the terminal title. The Blocked case (typed text going to a permission popup) was not tested, which is why the hold exists.
- Claude adds a "The user named this session ..." reminder to its next model turn. This is accepted.
- `harness.DisplayName` parsing of the glyph prefix was not tested in the research. It does not matter here: the gx name wins over the terminal title once set.
