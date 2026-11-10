## Problem Statement

A person watching a working Chat session in the web UI cannot stop it. The Agents composer swaps Send for an Interrupt button, but the server refuses the click: the web route for interrupt accepts only a Design session, so a Chat session answers "not a design session". A person who sees the agent going the wrong way has to leave the web and press Ctrl+C in a terminal.

Three more things make interrupting poor even where it works:

- There is no key. Stopping a runaway agent should take one key press, and Esc is what both Claude Code and Codex use in their own terminals.
- Ctrl+C is the wrong key for Codex. An idle Codex quits on one Ctrl+C, so an interrupt that lands just after the turn ended kills the session.
- The chat log shows an interrupted turn badly. Claude's "[Request interrupted by user]" line is drawn as a user bubble that looks typed by the person. A Codex interrupt shows nothing: the log just stops.

## Solution

From a session's web page, while its agent is Working, a person can interrupt it with the Interrupt button or with Esc. This holds for Chat sessions and Design sessions, on Claude and on Codex.

- Esc opens the same confirm the button opens, with its Interrupt button focused. Enter confirms. Esc closes it.
- The server accepts the interrupt for any interactive session except a Queue agent, the same rule Stop and Remove already use.
- gx sends each harness its own interrupt key: one Esc to Codex, one Ctrl+C to Claude.
- The chat log shows an interrupted turn as a dim "Turn interrupted" separator, for both harnesses, no matter who interrupted.

## User Stories

1. As a person with a working Chat session open in the web UI, I want the Interrupt button to work, so that I can stop an agent that is going the wrong way without opening a terminal.
2. As a person with a working Design session open, I want Interrupt to keep working as it does today, so that the new check does not take it away.
3. As a person on a session's page while its agent is Working, I want to press Esc to start an interrupt, so that I can stop the agent without reaching for the mouse.
4. As a person typing in the message box while the agent works, I want Esc to start an interrupt from inside the box, so that I do not have to click out of it first.
5. As a person, I want Esc to open the confirm instead of interrupting at once, so that a stray key press cannot end the agent's turn.
6. As a person, I want the confirm opened by Esc to have its Interrupt button focused, so that Esc then Enter interrupts with two key presses.
7. As a person, I want Enter in that confirm to interrupt and Esc to close it without interrupting, so that I can back out with the same key.
8. As a person with a popover, the command palette or the Questions view open, I want Esc to close that first, so that Esc keeps closing what is on top of the page. The expanded editor is the exception: it does not close on Esc today, so see story 10.
9. As a person with a completion menu, a dialog or another control that already handles Esc open, I want it to take the key first, so that Esc never does two things at once.
10. As a Vi-mode user in the expanded editor (Vi exists only there, not in the inline message box), I want an Esc that Vi uses to leave insert or visual mode to do nothing else, and an Esc in NORMAL mode to open the interrupt confirm even over the editor, so that Vi editing still works.
11. As a person with a session that is not Working, I want Esc to keep its current meaning, so that nothing changes when there is nothing to interrupt.
12. As a person on a session that exits Working while the confirm is open, I want the confirm to go away with the button, so that I cannot confirm an interrupt of nothing.
13. As a person on a Design session's page and on its Agents page, I want the same Esc behavior in both, so that I do not have to remember where I am.
14. As a person using a Chat session on Claude, I want an interrupt to end the turn and keep the session, so that I can send a new prompt right after.
15. As a person using a Chat session on Codex, I want an interrupt to end the turn and keep my draft, so that I do not lose what I was typing.
16. As a Codex user, I accept that a late interrupt, one that lands after the turn ended, never quits Codex, because Codex treats an idle Esc as harmless (known Codex behavior, probed in `docs/research/chat-interrupt-codex-esc-probe.md`), so that a click at the wrong moment cannot kill the session.
17. As a Codex user, I accept that a late interrupt that lands on an open popup or overlay only closes it, because Codex gives the Esc to the popup first (same probe notes), so that it does no harm.
18. As a Claude user, I want gx to keep sending Ctrl+C, so that Esc does not push my Vi-mode input into NORMAL mode and break later typing.
19. As a person, I want one key per Interrupt click, so that a click never sends two keys and does something I did not ask for.
20. As a person using the `gx` CLI to interrupt a session, I want the same per-harness key, so that the CLI and the web act the same.
21. As a person, I want the Interrupt button's tooltip to name the Esc key, so that I learn the shortcut where I look for the action.
22. As a person on a phone, I want the Interrupt button without a key hint, so that the UI does not mention a key I do not have.
23. As a person on a phone, I want no Esc handling, so that nothing changes there.
24. As a person, I want no command palette entry and no status bar hint for Interrupt, so that the UI stays as it is apart from the tooltip.
25. As a person reading a Claude chat after an interrupt, I want a dim "Turn interrupted" separator instead of a user bubble, so that I do not think I typed it.
26. As a person reading a Codex chat after an interrupt, I want the same separator, so that the log does not just stop.
27. As a person reading a Design session's log, I want the same separator, so that Chat sessions and Design sessions read alike.
28. As a person, I want the separator to read the same whether the web button, Esc or a terminal interrupted, so that the log does not guess who did it.
29. As a person, I want a Codex turn aborted for any reason other than `interrupted` to stay hidden as it is today, so that only real interrupts show a marker.
30. As a person, I want one interrupt to give one separator, so that a Codex interrupt does not show twice.
31. As a person, I want the raw `<turn_aborted>` text never to show as a prompt bubble, whichever role Codex writes it with, so that a Codex version change does not bring back a fake user bubble.
32. As a person who types the words "[Request interrupted by user]" inside a longer prompt, I want it to stay a normal prompt, so that my own text is not swallowed.
33. As a Codex user, I accept that the shell command that was running stays listed as a background terminal after an Esc interrupt, because Codex keeps it.
34. As a person with a Queue agent open, I want Interrupt to stay unavailable in the web, so that ralph-loop keeps driving it.
35. As a person with a stale page, I want an interrupt on a session whose state moved to be refused as stale, so that I never interrupt a turn I did not see.

## Implementation Decisions

**Server route**

- The web handler of the session interrupt route uses the same check as Stop and Remove: any interactive session except a Queue agent, plus the session state token. A Design session still passes. A Chat session now passes.
- Refusals stay as they are: a Queue agent gets the Queue-session refusal, a moved state gets the stale refusal, a session that is not Working gets the not-working refusal.
- The route's non-web handler (CLI and API) is unchanged apart from the key choice below. Queue agents stay refused on the web only.
- The existing Go test that expects a plain session to be refused with `not-design-session` changes to expect the new rule.

**Interrupt key per harness**

- The harness layer decides the interrupt key. It has two answers: Esc (one `\x1b` byte) for Codex and Ctrl+C (`\x03`) for Claude. A harness with no entry of its own (including the fixture-only `nochat` harness, which runs the fake Claude) gets Ctrl+C. The implementer picks the interface and method name; the existing per-harness typing rules (submit key, clear key) are the nearest example.
- Session interrupt asks the session's harness for its key and sends it as its own holder write: one key per call, nothing before it, nothing after it. Both the web route and the CLI and API route reach this, so one change covers every caller.
- There is no holder-side "send only if Working" check. The holder reads the same status as the server, so the check cannot close the race. The server keeps its own Working check. A late interrupt costs a Claude user at most a draft, and a Codex user nothing, since Esc never quits.
- Out of scope for this change: the interrupt that ralph-loop and the server budget use on Queue agents and loop sessions through the agent runner. It keeps its state-aware behavior (ADR 0031).

**Web keys and UI**

- The Agents composer (Chat sessions and Design sessions) and the Design composer already swap Send or Continue for Interrupt while the session is Working and open `InterruptPopover`. Both keep that. The shared pieces move into one place so the two composers cannot drift: the Esc handling and the confirm state live in one shared hook or component, and both composers use it. The implementer picks the shape.
- Esc is handled on a session's page, in Chat sessions and in Design sessions, only while the session is Working and the Interrupt button is shown and enabled (the same conditions as the button: connected, no interrupt already pending). It applies on a wide screen only.
- It also applies when focus is inside the message box.
- An Esc already handled by something on top is not an interrupt: an open popover (the Interrupt confirm included), the command palette, the jump menu, any modal or dialog, the completion menu, the Questions view, or a field that uses Esc to cancel an edit. The layer stack in the web UI (`useIsTopLayer` and the dismiss layer) is the place that decides who is on top. Only Esc with nothing on top starts an interrupt. The listener runs in the capture phase, so a `defaultPrevented` check would always read false; a field that uses Esc to cancel an edit is covered by the layer and on-top checks, not by `defaultPrevented`.
- Vi mode exists only in the expanded editor. There, an Esc that Vi uses (insert or visual mode) goes no further. An Esc in NORMAL mode starts the interrupt, even though the editor is open, and the confirm must be reachable above it. The editor reports whether it consumed the key. The editor itself still does not close on Esc.
- Esc in the Questions view of the chat log keeps leaving that view. The next Esc starts the interrupt.
- Esc opens the existing confirm with the Interrupt button focused. While the confirm is open, Enter confirms and Esc closes it. Closing returns focus to where the confirm's final-focus rule puts it today (the Interrupt button).
- The confirm opened by clicking the button keeps its current initial focus (Cancel). Only the Esc-opened confirm focuses Interrupt.
- When the session stops being Working, the button and its confirm go with the swap back to Send or Continue.
- Confirming sends the interrupt with the session's state token, as the button does now.
- The Interrupt button's tooltip names the key ("Esc"). On a phone width the tooltip names no key. No status bar hint. No command palette entry. No TUI change.

**Chat log**

- A new chat message kind, `interrupted`, travels from the server's chat reader to the web, next to the existing compaction kind. The generated web types gain it by the project's usual generator.
- The web draws it as a dim, centered separator reading "Turn interrupted", in the same style as the "Context compacted" separator. It shows no source. It is the same in the Agents chat and in the Design chat, since they share the chat renderer.
- Claude: a user line whose whole trimmed text is "[Request interrupted by user]" or "[Request interrupted by user for tool use]" becomes the `interrupted` message. It no longer becomes a person prompt. The same text inside a longer prompt stays a normal prompt.
- Codex: only the `event_msg` `turn_aborted` with reason `interrupted` makes the message. Other reasons stay hidden as today. The `response_item` `<turn_aborted>` fragment never adds a message, so one interrupt gives one separator.
- Codex: a `message` that starts with `<turn_aborted>` is hidden for role `developer` and for role `user`.
- The marker is not a person message. Consumers that count or split on person messages (rounds, grilling counts, the last prompt) stop seeing the Claude marker as one. Like a compaction, it ends the run of trailing agent texts that follows a person message, which is how the Claude marker behaved while it was a person prompt.

## Testing Decisions

A good test checks behavior from outside: the HTTP route's answer, the bytes the session receives, the messages the reader returns, the text and buttons a person sees. It does not check which helper called which.

Seams (the highest existing seam for each rule; no new seam is introduced):

- **Web route and key:** the server session tests that start a real server with a fake agent (`servertest` with `StartPromptSession`, `startWebDesign`). They already assert that a Working session receives the interrupt entry, that a stale token and an idle session are refused, and the Design-session web route. Extend them: a plain Chat session now passes, a Queue agent is refused, the fake Claude receives Ctrl+C, the fake Codex receives one `\x1b` and nothing else. Today the fake Codex logs the same interrupt entry for Esc and Ctrl+C, so the fake changes first: it logs each interrupt key as its own entry (or logs raw input, as the Claude TUI fake does), and the Go test asserts on that log. The harness layer's key choice is covered through this seam, plus one table test next to the existing typing tests (key per harness, default for an unknown harness).
- **Chat reader:** the existing reader tests with transcript fixtures (`chatreader` and its `testdata`, the transcript and Codex session chat tests). Add Claude lines (both marker forms, whole text and inside a longer prompt) and Codex rollout lines (`turn_aborted` with `interrupted`, with another reason, the `<turn_aborted>` fragment as role `developer` and as role `user`). Assert the message sequence, including that one Codex interrupt gives one message and that the person-turn logic ignores the marker.
- **Web components:** vitest with jsdom, on the composers and the chat log (`AgentComposer`, `AgentsPage.composer`, the Design `Composer` and `ChatLog` tests). Cover Esc opening the confirm with the Interrupt button focused, Enter confirming, Esc closing, Esc doing nothing while not Working or on a phone width, an open layer taking Esc first, Vi insert then NORMAL, the tooltip text, and the separator.
- **Playwright:** a flow is allowed here because the rule needs a real keyboard, real focus and a real server round trip with the fixture's fake agents. It extends existing flows on the Agents composer and the Design composer scenarios rather than adding one (see Verification). Everything else stays in vitest and Go tests.

Prior art: the `Interrupt` tests in the session tests; `AgentComposer.test.tsx` and `Composer.test.tsx`; `CodeEditor.vim.test.tsx` for Vi mode; `ChatLog.test.tsx` and the `compacted` cases in the reader tests for the separator.

## Verification

A verify ticket runs these against a temporary gx server. Browser steps use gx-ui-check and the flow catalog (`webui/e2e/CATALOG.md`).

1. Stories 1 and 14 (Chat session Interrupt on Claude):
   - Catalog flows: 75 (Read a session, on `agents-tools`, three Working Claude sessions) for the Working state; 76 (Send and draft) for the composer.
   - New steps, added to flow 75 on `agents-tools`: click Interrupt on a Working Chat session, confirm, and see the fake Claude receive Ctrl+C and the session leave Working.
   - CLI: with the fixture env set, `gx agents interrupt <session>` on a Working Chat session exits 0; on an idle one it prints the not-working refusal.
2. Story 2 (Design session):
   - Catalog flows: the design flow "First step, ids, answer, Continue and a fresh round" and flow 14 (Working row, `design-working`).
   - New steps: on `design-working`, click Interrupt, confirm, the session leaves Working.
3. Stories 3, 5, 6, 7 (Esc, confirm, focus, Enter and Esc in the confirm):
   - New steps in the flow 75 extension: press Esc on the page while Working, the confirm opens with Interrupt focused, Enter confirms; again, Esc opens, Esc closes and the session stays Working.
4. Story 4 (Esc from inside the message box): new step in the same flow: focus the message box, press Esc, the confirm opens.
5. Stories 8, 9 (something on top takes Esc first): new steps: with the command palette open, Esc closes it and no confirm opens; with the confirm open, Esc closes the confirm and nothing is interrupted.
6. Story 10 (Vi mode): vitest covers the hook. The real vim keymap check is the new step in the existing Vi editor flow: in a Working session, Esc in insert mode opens no confirm, a second Esc opens it.
7. Story 11 (not Working): new step: with an idle session, Esc leaves the page as it was (no confirm).
8. Story 12 (confirm goes when Working ends): new step: with the confirm open, the fixture agent finishes its turn, the confirm and the Interrupt button leave.
9. Story 13 (Design page and Agents page): run the Esc steps of 3 on `design-working` on the Design page, then on the same session's Agents page.
10. Stories 15 and 19 (Codex key): Go server tests only (the fake Codex logs exactly one Esc entry). Catalog flow 118 is not involved. No browser step. Stories 16 and 17 are known Codex behavior from the probe notes; gx has no check for them.
11. Story 18 (Claude Ctrl+C): the Go server test of story 1.
12. Story 20 (CLI, same key): the CLI check of story 1 on a Claude session; the Codex byte is the Go server test in entry 10, which goes through the same session interrupt.
13. Stories 21 to 24 (tooltip, phone, no hint, no palette entry):
    - Catalog flows: 75 (the Working row at 390px) and flow 152 (Phone, command menu) for the phone and palette checks.
    - New steps: hover Interrupt at 1280px and read the tooltip naming Esc; at 390px the tooltip names no key and Esc opens nothing; the command palette has no Interrupt entry.
14. Stories 25 to 28 (separator): new scenario or fixture lines: a transcript with a Claude marker line and a Codex `turn_aborted` interrupted rollout, shown in a Chat session and in a Design session. Pass: a dim "Turn interrupted" separator, no user bubble, the same text in all four.
15. Stories 29 to 32 (reader rules): Go reader tests (no browser). Pass: other reasons hide, one separator per Codex interrupt, the fragment hidden for both roles, the marker text inside a longer prompt stays a prompt.
16. Story 33: no check; it states a known Codex side effect.
17. Stories 34 and 35 (Queue agent, stale): Go server tests. Catalog flows 73 and 75 show a Queue agent without an Interrupt button.

New flows or scenarios to write (these become implementation tickets):

- Steps added to flow 75 on `agents-tools` and to a flow on `design-working`, as listed above. No new flow unless the steps cannot extend an existing one on the same scenario.
- A fixture scenario or an extension of an existing one that holds a chat with an interrupt marker for Claude and for Codex (stories 25 to 28).
- A change to the fake Codex so it logs Esc and Ctrl+C as separate entries. This is its own implementation ticket, and the next item depends on it.
- A Go server test for the Codex Esc bytes and the Claude Ctrl+C bytes.
- One step added to the existing Vi editor flow (`webui/e2e/vi-editor.e2e.ts`): in a Working session, Esc in insert mode opens no confirm, and a second Esc opens it.

No stored mock exists, so there is no Mock states list.

## Out of Scope

- A web interrupt for Queue agents. They open read-only and ralph-loop drives them. The route keeps refusing them.
- Any TUI change. The Agents tab's attached terminal already takes Ctrl+C.
- An interrupt key on phones. The button stays.
- A command palette entry for Interrupt, and a status bar hint.
- A holder-side state check before the key, and any new holder op.
- Ralph-loop's and the budget's own interrupt of agents through the agent runner.
- Stopping the background terminal Codex keeps listed after an Esc interrupt.
- A marker that says who interrupted.

## Further Notes

- Evidence for the Codex key: `docs/research/chat-interrupt-codex-interrupt.md` (source reading) and `docs/research/chat-interrupt-codex-esc-probe.md` (a live Codex 0.167.0 in a PTY, written the way the holder writes). A lone Esc byte reads as Esc, not an Alt prefix. Running with a draft: Esc ends the turn and keeps the draft. Running with a popup open: the first Esc closes only the popup. Idle: Esc never quits and never clears text; with empty input it shows an "esc again to edit previous message" hint, and a second Esc opens the transcript overlay (a third closes it). The config key `tui.keymap.chat.interrupt_turn` already defaults to Esc.
- Why Claude keeps Ctrl+C: in Claude's Vi mode, Esc enters NORMAL mode and would break gx's later typing. An idle Ctrl+C clears the draft at worst.
- Upstream Codex declares role `user` for the `<turn_aborted>` fragment, while the installed version writes `developer`. Hiding it for both roles covers a version change.
- Terms: a Chat session is the plain interactive session; a Design session has a Session ticket. This spec never calls a Design session a chat.
