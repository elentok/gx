# Vi keys in the prompt box and a big prompt box

Builds on `docs/specs/webui-vi-mode.md`. Every rule there stays unless this spec says otherwise.

## Problem Statement

With vi mode on, vi keys work only in the large editor. The prompt box itself, where people type most of their text, is a plain textarea. A person who thinks in vi has to open the large editor for every message.

The prompt box is also small. There is no quick way to give it more room while writing a long message, and no way to make that last.

## Solution

With vi mode on and a main pointer that is not coarse (not touch), the prompt box itself becomes a vi editor (CodeMirror 6 with the vim keymap, the same engine as the large editor). It has INSERT, NORMAL and VISUAL modes, and a mode pill at the bottom left of the box's bottom line shows the current mode. With vi mode off, or on a touch device, the prompt box stays a plain textarea. Touch devices still get vi through the large editor.

A new chord, the Leader key then `b`, switches a "big prompt box": every prompt box in the app gets twice the rows. The state is kept in this browser and survives a reload. The chord is the only way to switch it.

The stored mock is `$(gx tickets root)/prompt-box-vi-mode/mocks/02-inline-vi-mode-text-look.html`. It is not pixel-exact. Only the pill's place (bottom left of the box's bottom line) and its text are binding. Every other measure comes from the real prompt box.

## User Stories

1. As a vi user on a desktop, I want vi keys in the chat composer, so that I can write messages without opening the large editor.
2. As a vi user, I want the same vi keys in every prompt box (chat composer, design composer and note, question answer boxes, deny note, start-over reason, schedule prompt), so that the app feels the same everywhere.
3. As a person with vi mode off, I want the plain textarea as today, so that nothing changes for me.
4. As a person on a phone or tablet with touch as the main pointer, I want the plain textarea inline, so that my on-screen keyboard works and vi stays in the large editor.
5. As a person on a narrow desktop window with a mouse, I want inline vi keys, so that window width does not take them away.
6. As a person who plugs in or removes a mouse, I want the box to follow the pointer media query live, so that I need no reload.
7. As a vi user, I want a mode pill reading INSERT, NORMAL or VISUAL at the bottom left of the box's bottom line, so that I always know my mode.
8. As a vi user, I want the pill colored like the large editor's mode text (INSERT green, NORMAL blue, VISUAL amber), so that both places match.
9. As a chat composer user, I want the footer to read `MODE · model … context badge`, so that the pill fits the line I already know.
10. As a user of a box with no footer (the small question answer box), I want the pill alone in the bottom line, so that the mode still shows.
11. As a user, I want the box's note line (a reason, a red refusal, the Codex `$` hint) to keep showing, so that no message is lost.
12. As a vi user, I want the caret to follow vim (bar in INSERT, block in NORMAL), so that it matches the large editor.
13. As a vi user, I want the box to start in INSERT, so that typing works at once.
14. As a vi user, I want the box to keep its mode when focus leaves and comes back, so that I can click away and return.
15. As a vi user, I want the box to go back to INSERT after a submit empties it, so that the next message starts in INSERT.
16. As a vi user, I want my own `dd` that empties the box to leave me in NORMAL, so that only an outside clear resets the mode.
17. As a vi user, I want `ZZ` to do nothing and `:q` and `:wq` to show "Not an editor command" in the inline box, so that I never lose the box by habit.
18. As a vi user, I want `/`, `?` and `:` to work inline, so that I can search and use ex commands.
19. As a vi user, I want the command panel to open inside the box, between the text and the pill line, and to make the box one line taller only while it is open, so that the layout stays stable.
20. As a vi user, I want Cmd-G (Ctrl-G on a non-Mac) in the inline box to open the large editor in INSERT, with my caret and selection carried over, so that I can go big at once.
21. As a vi user, I want closing the large editor to return focus, caret and selection to the inline box, so that I continue where I was.
22. As a chat composer user, I want Cmd/Ctrl-Enter to send from the inline box in every mode, so that sending is the same as with the textarea.
23. As a question answerer, I want Cmd/Ctrl-Enter in an answer box (which has no submit of its own) to still send the answers, so that the shortcut keeps working.
24. As a chat composer user, I want the `/` command menu to work inline in INSERT only, so that vim keys in NORMAL and VISUAL never fight the menu.
25. As a chat composer user, I want Esc to close an open command menu before it leaves INSERT, so that Esc does the nearest thing first.
26. As a vi user, I want Esc in INSERT and VISUAL to belong to vim alone in every host, so that no dialog, view or Interrupt reacts to it.
27. As a user of a Working chat or a Working design session, I want Esc in NORMAL in the chat composer or the design composer to open the Interrupt confirm, so that Esc still stops the agent.
28. As a question answerer, I want Esc in NORMAL to leave the answer box without closing the Questions view, landing on the answer button or, for a text question, on the question's own row, so that it behaves like the textarea and focus never gets lost.
29. As a user of a dialog (start-over reason), I want Esc in NORMAL to close the dialog as today and Esc in INSERT to leave INSERT only, so that typed text survives.
30. As a vi user, I want an inline box inside a dialog or sheet never to open Interrupt, so that a layer on top takes the Esc.
31. As a vi user, I want `n`, `p` and other sheet keys typed in an answer box to reach the editor, so that I do not jump between questions by accident.
32. As a vi user, I want the vi escape sequence setting (such as `jk`) to work inline, so that my setting applies everywhere.
33. As a user, I want the textarea to show until the editor is ready and then swap with the same text, caret and focus, so that I lose nothing while it loads.
34. As a vi user, I want the page to preload the editor code when vi mode is on and the pointer is not coarse, so that the swap is quick.
35. As a user, I want turning vi mode on or off while a box is on screen to swap the box the same way, so that I need no reload.
36. As a user with a disabled box, I want it read-only and dimmed, with the pill dimmed, so that it looks and acts disabled like the textarea.
37. As a user, I want Tab and Shift-Tab to keep moving focus out of the inline box, so that I never get trapped.
38. As a user, I want the expand button to stay at the top right and the text to stay clear of it, so that nothing hides under it.
39. As a user, I want the Leader key then `b` to switch the big prompt box, so that I can make every box taller from the keyboard, even from inside a vi box.
40. As a user, I want the Chord box to list "Big prompt box" when it is off and "Normal prompt box" when it is on, so that the row says what it will do.
41. As a user, I want the chord offered on every page, so that I never wonder where it works.
42. As a user, I want the chord to do nothing while any layer (including the large editor) is open, so that it follows the Leader key's rule.
43. As a user, I want big mode to double each box's rows (2 to 4, 3 to 6, 5 to 10, 8 to 16, and a box with no rows from 2 to 4), so that every box grows in proportion.
44. As a user, I want big mode to apply to the textarea and to the inline vi box alike, so that it does not depend on vi mode or the pointer.
45. As a user, I want the inline box to keep its text, caret and mode when big mode switches, so that I can switch mid-sentence.
46. As a user, I want big mode kept in this browser and to survive a reload, so that I set it once.
47. As a user with two tabs open, I want them to follow the same big mode live, so that the tabs agree.
48. As a phone user, I want no button for big mode, so that a phone never gets a huge box by accident.
49. As a user, I want the large editor to ignore big mode, so that it stays full size.
50. As a user whose storage is blocked, I want the box to work with big mode lasting only until reload, so that nothing breaks.
51. As a screen reader user, I want the inline box to keep the textarea's accessible name and the pill to be a polite status, so that nothing is lost.
52. As a vi user, I want the inline box to keep the vi mode it had before Cmd-G after I close the large editor (VISUAL comes back as NORMAL with the selection as the caret), even if the large editor emptied the text, so that a round trip never resets my mode.
53. As a sighted user, I want the same focus ring on the inline editor as on the textarea, shown only while it has focus, so that I see which box I am in.

## Implementation Decisions

- **Which editor.** A prompt box shows the CodeMirror + vim editor when vi mode is on and the main pointer is not coarse. It uses the same pointer rule and test override as the large editor's touch bar, so one rule serves both. The media query is read live. Otherwise it shows the textarea. The prompt box owns this choice, so every call site (the chat composer, the design composer and note, both question answer boxes, the deny note, the start-over reason, the schedule prompt) gets it with no change.
- **One engine.** The inline editor and the large editor use the same CodeMirror module, the same vim mappings (screen-line `j`/`k`, `gs`, `ge`) and the same vi escape sequence setting. Only the pieces that differ are separate: the inline editor sizes itself by rows, not to fill a layer, and it registers no closer, so `ZZ` finds nothing to run and does nothing. `ZZ` is mapped once, globally, to an action that looks up the closer of the editor it fires in; the inline editor must not unmap or change that mapping, or the large editor's `ZZ` would break. `:q` and `:wq` are not vim ex commands in this engine: the panel shows "Not an editor command" and the box does nothing else.
- **Height.** The inline box has a fixed height of rows times line height and scrolls, like the textarea. A box with no `rows` counts as 2. Its horizontal padding matches the textarea's, with the right side clear of the expand button.
- **Swap.** While the editor chunk loads, the textarea shows. The swap, and the swap after a live vi or pointer change, carries text, caret, selection and focus. The page preloads the chunk when vi is on and the pointer is not coarse.
- **Mode pill.** The prompt box owns the pill, because it owns the bottom line. It reuses the large editor's mode text look and its polite status role. In the bottom line the pill comes first, then the footer (a separator between them when there is a footer). The footer content (model and context badge) is unchanged. The line shows as soon as inline vi applies and reads INSERT while the textarea stands in, so the box does not change height at the swap. A disabled box dims its pill. With vi off, or with touch, there is no pill and no extra line. No Send tip and no tab-complete note goes in this line.
- **Mode rules.** The editor starts in INSERT. It keeps its mode across blur and focus (the engine does not reset on blur). When the parent replaces a non-empty text with an empty text from outside (a submit), the editor goes back to INSERT. A user edit that empties the box does not. A Cmd-G round trip never resets the mode, even if the large editor emptied the text (this carves out the outside-clear reset).
- **Ex line.** `/`, `?` and `:` work. The panel opens inside the box between the text and the pill line and adds one line of height only while open. `:q` and `:wq` show "Not an editor command" in the panel and change nothing, and `ZZ` does nothing.
- **Disabled.** A disabled inline editor is read-only, not focusable by tab, and dimmed like the disabled textarea. Its mode and text stay.
- **Keys.** Mod-Enter runs the box's submit when it has one, in every mode. With no submit it is left unhandled, so the Questions view's send-answers key still works. Mod-G opens the large editor. Tab and Shift-Tab keep moving focus. The inline editor keeps the textarea's accessible name.
- **Cmd-G handoff.** The large editor opens in INSERT with the inline editor's caret and selection. On close, focus, caret and selection return to the inline editor, and it keeps the vi mode it had before Cmd-G (NORMAL stays NORMAL; VISUAL comes back as NORMAL, and the selection carries over as the caret). The large editor's mode does not carry back, and the round trip never resets the mode, even if the large editor emptied the text.
- **Completion menu.** The existing completion menu and key hook work over the inline editor. The menu is shut unless the mode is INSERT, the same rule the large editor already uses. Esc closes an open menu first.
- **Esc rule, all hosts.** The editor publishes its mode on its host element, as the large editor does. INSERT and VISUAL Esc belong to vim alone; NORMAL Esc goes on to the host exactly as it does for a textarea today. Two kinds of listener judge the Esc, and the rule reaches each differently:
  - Window capture-phase listeners (Interrupt in `InterruptControl`, the Questions view's Esc handler in `ChatLog`) run before the editor sees the key, so the editor cannot stop them. Each must judge for itself: Interrupt through the editor's published mode (`escOpensInterrupt` reads `data-vi-mode`), and the Questions view through the shared inline-box marker (it leaves an Esc in an answer box alone, as it does for a textarea, whatever the mode).
  - Window bubble-phase listeners (the `Modal` Esc handler) run after the editor. In INSERT and VISUAL the editor stops the event, so they never see it. In NORMAL it passes.

  The outcomes in NORMAL:
  - Chat composer and design composer (Working session): open the Interrupt confirm.
  - Questions-view answer box: the sheet's own key handler leaves the box and the view stays open. It focuses the question's answer button; a text question has no `button[data-answer]` (and `focusAnswer` does nothing there today), so it blurs the box and focuses the question's own row instead.
  - Modal: closes it.
  - The Interrupt judge treats an inline box inside any dialog or sheet as having that dialog on top, so it never opens Interrupt there. Only the large editor's own dialog counts as "own".
- **Finding a prompt box.** Code that finds a prompt box with a `textarea` selector must also find the inline editor. These sites are known: the first-answer focus and the `n`/`p` guard in `QuestionSheet` (the guard that lets text fields keep their keys), its note focus for the `N` key, the Esc handler in `ChatLog`, and the focus helper in `agents/promptFocus`. A shared marker on the prompt box replaces those selectors; the implementer picks the marker. The question note box keeps `name=question-note` next to the marker, so `QuestionSheet` can tell the note box from an answer box. Global shortcuts already stand down over CodeMirror.
- **Focus ring.** Today the box draws its ring with `&:has(> textarea:focus-visible)`, which matches only a direct child. The inline editor opts out of the global `:focus-visible` ring (`data-focus-ring="none"`, as the textarea does) and the box draws the same ring (same color and inset) through a `:has(.cm-editor.cm-focused)` rule, shown only while the editor has focus. Vitest asserts the rule and the attribute, not a computed `box-shadow` (jsdom cannot compute it). The ring's look is checked in Playwright, never against the mock; the mock check stays the pill's place and text.
- **Preload seam.** The large editor's chunk is a bare `lazy()` import today. The implementer adds a named chunk-preload function, called when vi is on and the pointer is not coarse, as the test seam for story 34.
- **Big prompt box store.** One per-browser store holds the flag, read from local storage at start under a key named like the other `gx.<area>.<name>` keys (the implementer picks the name). Reads and writes are guarded, so blocked storage leaves big mode for the page only. Every prompt box reads it live. The `storage` event keeps other tabs in step. Switching it does not remount the inline editor.
- **Rows.** The effective rows are the box's rows (default 2) times 2 in big mode, for the textarea and the inline editor. Gallery states that use `rows` follow the same rule. The large editor ignores the flag.
- **Chord.** The chord table gets one entry with key `b`, available on every page. Its label reads the store at render time: "Big prompt box" when off, "Normal prompt box" when on. Running it flips the store. The Leader key is already ignored while any layer is open, so the chord does nothing then, and it works from inside the inline editor, since the Leader key listener is on the window in the capture phase. There is no Gear menu or other button.
- **Docs.** The Chords section and the vi mode entries of the flow catalog and glossary (Leader key, Chord box, prompt box, large editor) say the new things in the repo's words. No ADR applies.

## Testing Decisions

- A good test checks what a person sees: the text in the field, the mode text, the rows or height, which layer is open, where focus is, the Chord box label, local storage, and the Esc outcome. It does not check engine internals.
- **Seam 1, vitest (the main seam, in jsdom; CodeMirror runs there).** Prior art: `PromptBox.test.tsx`, `CodeEditor.vim.test.tsx`, `LargeEditor.test.tsx`, `chords.test.ts`, `ChordHost` tests, the `escOpensInterrupt` tests, `QuestionSheet.test.tsx`, `Modal.test.tsx`. It covers:
  - Editor choice: vi off gives a textarea; vi on with a pointer that is not coarse gives the editor; a forced coarse pointer gives a textarea; a live pointer or vi change swaps and keeps text, caret and focus.
  - The pill: text, tone and place per mode; present with a footer and alone; INSERT while the textarea stands in; dimmed when disabled; absent with vi off or touch.
  - Mode rules: INSERT at start; kept across blur and focus; back to INSERT after an outside clear; kept after a user `dd`.
  - `ZZ` does nothing; `:q` and `:wq` show "Not an editor command" and change nothing; `/` and `:` open a panel without changing the rows.
  - Disabled is read-only.
  - Cmd-G opens the large editor in INSERT with the caret and selection, and close restores them; the inline mode before Cmd-G comes back (VISUAL as NORMAL, selection as caret), also when the large editor emptied the text.
  - Mod-Enter submits when a submit exists and passes through when not.
  - The completion menu is open only in INSERT; Esc closes it first.
  - The Esc rule: INSERT and VISUAL stop the event; NORMAL passes. `escOpensInterrupt` returns false for an inline box in a dialog. The Questions view leaves the box on NORMAL Esc and stays open. A Modal closes on NORMAL Esc and not on INSERT Esc. `n` and `p` typed in an answer box do not navigate.
  - The big store: default off; reads local storage; toggles; survives a fresh read; survives blocked storage; the `storage` event updates it.
  - Rows doubling for textarea and editor (2 to 4, 3 to 6, 5 to 10, 8 to 16, none to 4); no remount when the flag flips.
  - The chord entry: label per state, offered on every page, runs the toggle.
  - The large editor ignores big mode.
- **Seam 2, Playwright (only what vitest cannot see: a real keyboard, a real media query, layout, a real server).** Prior art: `vi-editor.e2e.ts`, `chords.e2e.ts`. Per the e2e rules it adds steps to `vi-editor.e2e.ts` flows and adds at most two new flows there (A and B below), and it updates any existing flow that the new editor changes (the flows that fill the Message box through its textarea keep working because the accessible name is the same). It names the vitest tests considered and what they cannot see: real key events through the capture-phase Leader listener, the real `(pointer: coarse)` query, real layout height, and a second page in the same browser context.
- No Go test: no server or schema change.
- Playwright cannot show a real on-screen keyboard; touch is shown by emulation only, and touch inline vi is out of scope.

## Verification

Run against a temporary gx server after the epic is built. Vi mode is turned on with `fixture.cli("vimode", "on")`. Scenarios: `workspaces` (the Default workspace's chat, as `chords.e2e.ts` uses it) for the chat composer, `design-text` (scripts question Q13 "Name the migration" through `DesignTurnText`) for the text answer box, and `design` (the note box of question Q8b, like existing flow 126) for the Esc and `n`/`p` steps. The new flows go in `vi-editor.e2e.ts`.

- Flow A "Inline vi box and big prompt box", scenario `workspaces`, 1280 px, mouse:
  - Vi on: the Message box is the editor with the accessible name "Message"; the pill reads INSERT.
  - Esc gives NORMAL, `v` gives VISUAL, Esc then `i` gives INSERT; typed text appears; `dd` empties it and the pill stays NORMAL.
  - Esc, then `/` opens the panel inside the box; `:q` shows "Not an editor command" and nothing else changes.
  - With the chat Working, Esc in INSERT opens no confirm; the next Esc in NORMAL opens the Interrupt confirm.
  - Leader then `b` in INSERT doubles the box height; the Chord box reads "Normal prompt box" the next time; the text, caret and mode are unchanged.
  - A second page in the same context opens with the box already big, no reload.
  - Leader then `b` while the large editor is open changes nothing.
  - Cmd-G opens the large editor in INSERT; Cmd-G closes it with caret and selection back in the box. With the box in NORMAL before Cmd-G, the pill reads NORMAL after; with a VISUAL selection before, it reads NORMAL after and the selection is the caret.
  - The focus ring shows while the editor has focus and is gone after blur (a real browser check, not against the mock).
  - Sending (Cmd-Enter) empties the box and the pill returns to INSERT.
  - A touch context at the same size shows the textarea, no pill.
  - A mouse at 390 px still shows the editor.
  - Vi off through the CLI turns the box back into a textarea with the same text.
- Flow B "Inline vi box in the Questions view", 1280 px, in two parts:
  - Scenario `design`, in the question's note box (Q8b, like flow 126): `n` and `p` typed in INSERT stay in the text; Esc in INSERT keeps the view and the box; Esc in NORMAL leaves the box and keeps the view open.
  - Scenario `design-text`, the text question Q13 "Name the migration" (its only question): the answer box shows the pill; Esc in NORMAL blurs the box and focuses the question's own row, and the view stays open; Cmd-Enter sends the answers.

**Mock states**

Every state uses the mock `$(gx tickets root)/prompt-box-vi-mode/mocks/02-inline-vi-mode-text-look.html`, is checked for the pill's place and text only, and is named by its first field.

- chat-insert, width 480, scenario `workspaces` (the Default workspace's chat), parts: prompt-box, text, footer, mode-pill (INSERT), model, context, expand, send.
- chat-normal, width 480, scenario `workspaces`, parts: prompt-box, text, footer, mode-pill (NORMAL), model, context, expand, send.
- chat-visual, width 480, scenario `workspaces`, parts: prompt-box, text, footer, mode-pill (VISUAL), model, context, expand, send.
- chat-normal-big, width 480, scenario `workspaces` with big mode on, parts: prompt-box (twice the rows), text, footer, mode-pill (NORMAL), model, context, expand, send.
- answer-normal, width 480, scenario `design-text` (the Questions view's text answer box for Q13, no footer), parts: prompt-box, text, footer (pill alone), mode-pill (NORMAL), expand.
- answer-insert-blurred, width 480, scenario `design-text`, parts: prompt-box (no focus ring), text, footer (pill alone), mode-pill (INSERT), expand.

Editor choice and swap (stories 1 to 1, 3 to 3, 5 to 6, 33 to 35)
- flow A (editor with vi on, textarea after vi off, 390 px mouse, touch context) and vitest seam 1 for the live pointer change. Story 34: a vitest step that the named chunk-preload function (the implementer adds it as the seam) is called when vi is on and the pointer is not coarse, and not called with vi off or a coarse pointer.

Every call site (stories 2 to 2)
- vitest seam 1, one editor per call site; flow A and B cover the chat composer and answer boxes.

Touch (stories 4 to 4)
- flow A touch step; vitest with the coarse-pointer override.

Mode pill (stories 7 to 11)
- story 11 has vitest steps where the note, the red refusal note, the Codex `$` hint and the footer each render together with the pill. Mock states chat-insert, chat-normal, chat-visual, answer-normal and answer-insert-blurred. The verifier checks the pill's place (bottom left of the box's bottom line) and text only, not pixels. Vitest checks the tone.

Mode rules and keys (stories 12 to 17)
- flow A (pill after each key, `dd` step, send step, `:q`); vitest seam 1 for blur and focus and outside clear.

Ex line (stories 18 to 19)
- flow A (`/` and `:q` steps); vitest for the one-line growth.

Cmd-G handoff (stories 20 to 21, 52 to 52)
- flow A (Cmd-G steps, including: Esc to NORMAL, Cmd-G, close, the pill still reads NORMAL); vitest for caret, selection and mode.

Submit (stories 22 to 23)
- flow A (Cmd-Enter send); flow B (answers sent); vitest for the pass-through.

Completion menu (stories 24 to 25)
- vitest seam 1; the existing `agents-commands.e2e.ts` flows still pass with vi off.

Esc rule (stories 26 to 31)
- flow A (INSERT then NORMAL Esc on a Working chat); flow B (Esc in INSERT and NORMAL, `n` and `p`); vitest for the Modal and the dialog case, and for the design composer: Esc in NORMAL opens Interrupt on a Working design session and Esc in INSERT does not.

Small rules (stories 32 to 32, 36 to 38, 51 to 51, 53 to 53)
- vitest seam 1 (escape sequence, read-only and dimmed, Tab and Shift-Tab leave the box, expand button place, accessible name and status role, the focus-ring rule and `data-focus-ring="none"` present, not a computed `box-shadow`). The ring's look (shown on focus, gone on blur) is checked in flow A, never against the mock.

Chord (stories 39 to 42)
- flow A (Leader then `b`, label flip, layer step); vitest for the chord table and label. Existing `chords.e2e.ts` still passes (the Chord box now has one more row on every page; update any step that counts rows).

Big mode rows (stories 43 to 45)
- flow A height step (about double, within the usual 1 px tolerance of the row maths); vitest for the rows table and no remount. Mock state chat-normal-big shows 6 rows for a 3-row box.

Big mode store (stories 46 to 47, 50 to 50)
- flow A (second page in the same context, no reload); vitest for the store, blocked storage and the `storage` event.

No button, large editor (stories 48 to 49)
- vitest (no Gear toggle; the large editor ignores big mode); `united.e2e.ts` flow 28 still passes with no new menu item.

New flows or scenarios to write (these become implementation tickets): flow A and flow B above, in `vi-editor.e2e.ts`, plus the vitest tests of seam 1. Flow B uses two existing scenarios: `design-text` (`catalog/scenarios.go`; it scripts Q13 "Name the migration" through the `DesignTurnText` turn in `testutil/agentfake/design.go`) and `design` (the note box of Q8b). No new fixture scenario is needed.

## Out of Scope

- Inline vi keys on touch devices. Touch keeps the textarea; vi stays in the large editor there.
- A Gear menu (or any button) toggle for big mode. The chord is the only way.
- Big mode inside the large editor, which is already full size.
- Vi keys in single-line inputs or any field that is not the prompt box.
- Syncing big mode across devices or browsers.
- Android on-screen keyboard behavior, which `docs/specs/webui-vi-mode.md` already lists as a known limit.

## Self-decided design questions

- **Tab and Shift-Tab in the inline box.** Question: should Tab insert a tab or indent, or move focus? Choice: they leave the inline box, like the textarea (no indent key, no tab character). Reason: parity with the textarea it replaces. The large editor traps Tab inside its own layer because it is a modal; an inline box sits in the page and must not trap focus.

- **Esc in NORMAL in a text question's answer box.** Question: where does focus go when the question has no answer button? Choice: blur the box and focus the question's own row; the view stays open. Reason: `focusAnswer` finds no button and does nothing, so focus would stay in the box or be lost; the row is the nearest stable target.
- **`:q` and `:wq`.** Question: should they close or clear the box? Choice: no. They are not vim ex commands in this engine, show "Not an editor command", and do nothing else. Reason: parity with `ZZ` doing nothing inline, and no new ex commands are added.
- **VISUAL after Cmd-G.** Question: what mode and selection come back? Choice: NORMAL, with the selection as the caret. Reason: the large editor reopens in INSERT and carries no vim state back; NORMAL is the nearest mode that has no live selection.

## Further Notes

- The earlier spec's rules stay: the vi mode setting is global on the server, vi off leaves Cmd-G as it is today, and `Escape` never closes the large editor.
- The vim registry is global in the page, so registers and the last search are shared between boxes, as in vim.
- The mock is a guide for the pill's place and text, not for pixels.

