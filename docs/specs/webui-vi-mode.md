# Web UI large editor with optional vi mode

## Problem Statement

Writing long text in the web UI is hard. Answers, notes, deny reasons and agent messages all go into small text boxes. Only the note box has a way to open a bigger editor, and that editor is a plain text area.

Some people think in vi. They want to move and edit with vi keys in these fields, on a desktop and on a phone or tablet. Today they cannot.

## Solution

Every multi-line text field gets an expand button that opens one large editor. The editor uses variable-width text, because the fields hold prose.

A global vi mode switch sits next to AFK in the top-right menu. The server stores it, so all browsers share it. It is off by default.

- Vi mode off: the large editor takes plain typing. Cmd-G and Ctrl-G stay with the browser.
- Vi mode on: Cmd-G (Ctrl-G on a non-Mac) opens the editor from a focused, enabled multi-line field. The editor has normal, insert and visual modes and shows the current mode as text.
- On a touch device the editor has a bottom bar. With vi mode on it holds the mode text, Esc and Done. With vi mode off it holds Done only.

Text saves as you type. There is no cancel. Escape never closes the editor.

The editor engine is CodeMirror 6 with `@replit/codemirror-vim`. It loads on demand, so the app does not pay for it until the editor opens.

Known limits:

- Android on-screen keyboards are untested. The engine has an open report of letters inserted in normal mode and of a stuck insert mode. Autocorrect, dictation and IME text cannot be cancelled by any engine. This spec does not promise full vi on Android.
- On Safari on iPad, normal mode worked in a prototype with the on-screen keyboard. Cmd-G on iPadOS Safari with a hardware keyboard is unverified. The key test covered desktop Chrome, Safari and Firefox on a plain page, not the editor.
- Vi mode on gets no promise of full screen reader support for vi commands.

## User Stories

1. As a person who writes long answers, I want an expand button on every multi-line text field, so that I can write in a large editor.
2. As a phone user, I want the expand button on every field at every screen size, so that I can use the large editor on a phone.
3. As a person who types short text in the Jump menu or the Epics page, I want those single-line inputs to stay as they are, so that a modal editor does not get in my way.
4. As a person who does not use vi, I want vi mode off by default, so that nothing changes for me.
5. As a vi user, I want a vi mode switch next to AFK in the top-right menu, so that I can turn it on in one click.
6. As a person with several browsers, I want the vi mode setting stored on the server, so that every browser uses the same setting.
7. As a person who uses the CLI, I want a command to read and set vi mode like `gx afk`, so that I can script it and check it.
8. As a vi user, I want Cmd-G (Ctrl-G on a non-Mac) to open the editor from the field I am typing in, so that I do not reach for the mouse.
9. As a person who uses the browser's find next, I want Cmd-G to keep working outside a multi-line field, so that vi mode does not break search.
10. As a person with vi mode off, I want Cmd-G and Ctrl-G to stay with the browser everywhere, so that find next always works.
11. As a person looking at a disabled field, I want its expand button disabled and Cmd-G to do nothing, so that I cannot edit what is locked.
12. As a vi user, I want the editor to open in insert mode with the cursor where it was in the field, so that I can keep typing at once.
13. As a vi user, I want the current mode shown as text (INSERT, NORMAL, VISUAL), so that I always know which mode I am in.
14. As a vi user, I want normal, insert and visual modes, so that I can move and edit as in vi.
15. As a vi user, I want the movement keys `h j k l w e 0 f gg G`, so that I can move fast.
16. As a vi user, I want `j` and `k` to move by screen line, so that they do not jump a whole wrapped paragraph.
17. As a vi user, I want `j` and `k` to forget the old column after I click or tap, so that the next move starts from the new spot.
18. As a vi user, I want the edit keys `x d c y p u` and Ctrl-R, so that I can change and undo text.
19. As a vi user, I want `i a o` and `.`, so that I can insert and repeat edits.
20. As a phone vi user, I want `gs` and `ge` to go to the line start and end, so that I do not need `0` or `$` on a phone keyboard.
21. As a vi user, I want `ZZ` to close the editor, so that I can leave with the keyboard.
22. As a vi user, I want Cmd-G (Ctrl-G) in the editor to close it, so that the same key opens and closes it.
23. As a person on a wide screen, I want a press on the backdrop to close the editor, so that I can leave with the mouse.
24. As a person in the editor, I want a Done button that is always there, so that I can always close it.
25. As a person in the editor, I want Escape to never close it, so that a vi Escape does not throw away my place.
26. As a person who opens the editor over a sheet or modal, I want Escape to affect only the top layer, so that nothing underneath closes by accident.
27. As a person who opens the editor over the question sheet or the start-over reason modal, I want a backdrop press to close only the editor, so that the layer underneath stays open.
28. As a vi user, I want the Jump menu's Cmd-K to do nothing while the editor is open, so that a menu does not open over my text.
29. As a person who closes the editor, I want focus and my cursor or selection to return to the field, so that I can go on typing.
30. As a phone user, I want the refocus on close to keep the on-screen keyboard open, so that it does not flicker.
31. As a person who types, I want my text to save as I type, with no save step, so that I never lose it.
32. As a person with a field that has a submit action, I want Cmd-Enter or Ctrl-Enter in the editor to close it and submit, so that I can send from the editor.
33. As a person with a field that has only a button, I want Cmd-Enter in the editor to do nothing, so that I do not submit by mistake.
34. As a phone user with vi mode on, I want a bottom bar with the mode text, Esc and Done, so that I can leave insert mode and close without a hardware key.
35. As a phone user, I want the bar and its keys to be 44 px high, so that I can tap them.
36. As a phone user, I want the bar to sit just above the on-screen keyboard, so that I can reach it with the keyboard open.
37. As a phone user, I want the on-screen keyboard to stay open in normal mode, so that I can type commands.
38. As a touch user with vi mode off, I want the bar to show only Done, so that I can close the editor without vi helpers.
39. As a tablet user with a hardware keyboard, I want the bar to stay because my pointer is touch, so that the layout does not change when I attach a keyboard.
40. As a desktop user with a mouse, I want no bottom bar, so that the editor does not carry touch helpers.
41. As a person on a wide screen with a mouse, I want the editor in an 860 by 680 modal with a title header and a footer with Done, so that it looks like the note editor I know.
42. As a wide-screen vi user with a mouse, I want the mode text at the left of the footer, so that I can see the mode.
43. As a tablet user (wide screen, touch), I want the modal with its title header and the bottom bar owning Done and the mode text, with no footer, so that I see one Done and one mode text.
44. As a person with a narrow desktop window (narrow screen, mouse), I want the bottom sheet with its title header and the footer with Done and the mode text, with no bar and no x button, so that I get the desktop controls in a small window.
45. As a person who turns vi mode on or off in another browser while an editor is open, I want the editor to switch live and keep my text and cursor, so that I lose nothing.
46. As a keyboard user, I want Tab and Shift-Tab to move within the editor (editor, Esc on touch, Done), so that focus does not leave the layer.
47. As a person who types prose, I want no tab character inserted by Tab, so that focus moves are never blocked.
48. As a screen reader user, I want the mode text announced politely once per change, so that I hear mode changes without noise on every key.
49. As a screen reader user, I want the editor to be a labelled dialog and the editable area to have a name, so that I know where I am.
50. As a screen reader user with vi mode off, I want the engine's normal screen reader support, so that I can write plain text.
51. As a person who uses the shared modal, I want the shared modal to trap Tab, so that focus stays in the top layer.
52. As a person who starts a ticket over, I want the start-over dialog to close on Escape and to trap Tab like the shared modal, so that it behaves the same.
53. As an Android user, I want the spec to be clear that full vi is not promised on my keyboard, so that I know what to expect.
54. As a developer, I want one shared expandable text field in all 8 multi-line fields, so that behaviour is the same everywhere.
55. As a developer, I want the parent to own the text, so that there is no copy of it and no save step.
56. As a person who does not use the editor, I want the editor code to load only when I open it, so that the app does not get slower.
57. As a person with a field that needs its own size and name, I want each field to set its own rows, accessible name and editor title, so that labels stay right.
58. As a developer, I want the shared layer (modal and bottom sheet) to have options to turn off Escape close and narrow-screen backdrop close, so that the editor can use it without hacks.

## Implementation Decisions

Engine and loading

- The large editor uses CodeMirror 6 with `@replit/codemirror-vim`. A dynamic import loads it when the editor first opens. The research is on the branch `research/webui-vi-editor-engine`. The branch is never merged.
- One editor serves both modes. A CodeMirror `Compartment` switches vi mode on and off, so the switch applies live.
- The editor uses variable-width text and overrides the engine's default monospace font. The vi block cursor takes the glyph width.
- `ZZ` is host code on the engine's documented action API, because the default keymap has no `ZZ`.
- `j` and `k` map to `gj` and `gk`. The stored column (`vim.lastMotion`) is reset after a click or tap.
- `gs` and `ge` map to the line start and end, in normal and visual mode. `ge` replaces vim's "end of previous word".
- Commands in scope: modes normal, insert and visual; `h j k l w e 0 f gg G`; `gs ge`; `x d c y p u`; Ctrl-R; `i a o`; `.`; `ZZ`.

The setting

- A global vi mode setting is stored like AFK. AFK lives in a small JSON file in the server state dir, written through a rename. A write publishes a change event, and the snapshot carries the value. Vi mode copies this: its own file in the state dir, its own change event, and its own snapshot field. The implementer picks the file name, the event name and the field name.
- The write route takes an optional `on`. A missing `on` toggles. It returns the new value. The default is off. A missing snapshot field counts as off.
- The route needs a CLI verb, or the route parity test fails. A new verb mirrors `gx afk [on|off]`: no argument toggles, and `--json` prints the structured result. The implementer picks the verb name and the route path. The parity test table gets its entry.
- The web client writes through the same helper pattern as `afk.ts`. A failed write shows an error toast and leaves the switch as it was. The snapshot refetch after the change event repaints the switch.
- The top-right menu gets a checkbox menu item with a switch, next to AFK, on the same condition as AFK (it shows when the page can switch the setting). It is not on the Settings page.
- The TUI does not change.

The shared field

- One shared expandable text field replaces the textarea and expand button in `NoteBox`. All 8 multi-line fields use it: the Composer message (a NoteBox), the QuestionSheet note (a NoteBox), the QuestionSheet answer, the AnswerForm answer, the AnswerForm note, the PermissionCard deny note, the start-over reason, and the AgentComposer message.
- Props: the value and change callback (the parent owns the text), rows, accessible label, a required editor title, disabled, an optional submit action, and the existing autofocus. The editor reads the same value and calls the same callback.
- The expand button sits top right inside the field, as in `NoteBox`. It is disabled when the field is disabled.
- The field's selection (start and end) goes to the editor on open and returns to the field on close.
- Fields keep their own rows. The question sheet keeps its Note toggle. No field gains a new Cmd-Enter submit. Only the fields that have a submit action today (the Composer and the AgentComposer) submit from the editor. Both set the action on wide screens only, because a phone has no key send. That rule stays: a narrow screen has no submit action on them, so Cmd-Enter does nothing there, touch or not.
- The 2 single-line inputs (Epics page, Jump menu) are unchanged.

Opening and closing

- With vi mode off, Cmd-G and Ctrl-G are not handled, so the browser keeps them. With vi mode on, Cmd-G (Ctrl-G on a non-Mac) opens the editor from a focused, enabled multi-line field and cancels the browser's find next. Outside such a field the key is not handled. A disabled field does not open the editor.
- Chrome, Safari and Firefox all let the page cancel these keys, so there is no fallback shortcut.
- The editor opens in insert mode, with the cursor where it was in the field.
- Ways to close: the Done button, Cmd-G or Ctrl-G again (vi mode on), `ZZ` (vi mode on), and a press on the backdrop on a wide screen (touch or mouse). A narrow mouse window does not close it. Escape never closes the editor, in any mode.
- Cmd-Enter or Ctrl-Enter, on a field with a submit action, closes the editor and submits. On other fields it does nothing. The editor handles it itself; the form underneath does not fire.
- Focus returns to the field on every close except after a submit, where the field may be gone or disabled. On a phone the refocus keeps the keyboard open.

Layers and Escape

- The editor is a layer with `role="dialog"`, built on the shared layer (`Modal` and `BottomSheet`). One small shared helper tells a layer whether it is the topmost. `Modal` and the Jump menu call it and react to Escape only when they are topmost. The shared layer gets two explicit options. One turns off Escape-to-close. The other turns off the backdrop press on narrow screens. The editor sets both. The implementer picks the option names. In vi normal mode Escape bubbles out of the engine, and no layer underneath reacts. `ChatLog` already checks for an open dialog and needs no change. The implementer picks the helper's name.
- The Jump menu's Cmd-K and Ctrl-K do nothing while the editor is open, by the same rule.
- The editor renders in a portal above the sheet. A press on its backdrop closes only the editor. The sheet's own backdrop test ignores clicks that bubble through the React tree.
- `StartOverDialog` in the recovery actions is a hand-rolled overlay today, with its own backdrop click and no Escape handler. It moves onto the shared `Modal`. It gains Escape close and the Tab trap, and it follows the topmost-layer rule. Escape in the small reason field closes the dialog and drops the typed reason, like Cancel. The editor opens over it as over any other layer.

Layout

- Layout is chosen by width. The bar is chosen by pointer. The two are independent, so there are four cases:
  - Wide screen, mouse: a centred 860 by 680 modal with a title header, the editor, and a footer. The footer has the mode text at the left (vi mode on only), "Saved as you type", and Done at the right. There is no x button. A backdrop press closes it.
  - Wide screen, touch (a tablet): the same modal with its title header, but no footer. The bottom bar owns Done and the mode text, so there is one Done and one mode text. A backdrop press closes it. The modal follows the visual viewport, like the sheet's fill-viewport mode, with its height capped at 680 px, so the on-screen keyboard never covers its lower half and the bar sits just above the keyboard.
  - Narrow screen, mouse (a small desktop window): the bottom sheet with its title header, and the same footer as the wide modal (Done, mode text with vi mode on, "Saved as you type"). No bar and no x button. A backdrop press does not close it.
  - Narrow screen, touch (a phone): the bottom sheet, with the bar and no header and no footer. A backdrop press does not close it.
- The bar is shown when the primary pointer is touch (`pointer: coarse`), at any width. One row, left to right. With vi mode on: mode text pill (INSERT, NORMAL or VISUAL), Esc, then Done at the right. With vi mode off: Done only. The pill, Esc and Done are 44 px high. The bar's content is the same in every mode.
- Only one `role="status"` mode region exists at a time: in the footer when there is no bar, in the bar when there is one.
- The bar is pinned to the visual viewport (height and offset), so it sits just above the on-screen keyboard.
- The helper bar does not depend on keyboard detection. A touch device with a hardware keyboard keeps the bar, and the keys work too. Hardware Esc and the bar's Esc both act as vi Esc.
- Cmd-G and `ZZ` work the same on a touch device with a keyboard. Cmd-Enter submits only where the field has a submit action (see the shared field).
- The mode text is a polite live region (`role="status"`). Each change is announced once.
- No animation is added, so no reduced-motion work.

Accessibility

- The editor layer uses `role="dialog"`, `aria-modal` and `aria-label` set to the field's title, through the existing `Modal`.
- The shared `Modal` gets a focus trap. Tab and Shift-Tab cycle within the layer (editor, Esc on touch, Done). The editor never inserts a tab character.
- The editable area has the accessible name "<title> text", is multiline, and keeps the placeholder.
- Done and Esc have text names and are reachable with Tab. The expand button keeps its name.
- Vi mode off: the engine's default screen reader support. Vi mode on: no vi announcements and no promise of full support for vi commands.

Test seams

- Seam 1, server HTTP: the write route and the snapshot, through the existing server test harness.
- Seam 2, CLI: the new verb against a fake or fixture server.
- Seam 3, Vitest components in jsdom: the shared field, the layer helper, the top bar menu item, and the shared Modal focus trap.
- Seam 4, browser e2e (Playwright with the fixture server): the editor with real keys, in new flows in `vi-editor.e2e.ts`. Checks that need the bar use touch emulation: a shared Playwright project or helper that sets `hasTouch: true` and `isMobile: true`. No e2e test sets these today. The implementer picks the project or helper.

## Testing Decisions

- A good test checks what a person or a script sees: the text in the field, which layer is open, where focus is, the snapshot value, the CLI output and exit code. It does not check engine internals.
- Go tests (seam 1): the route sets, clears and toggles; the value survives a server restart; the snapshot shows it; a write publishes one event; a missing state file means off. Prior art: `server/queue/web_afk_test.go` and `server/queue/afk_test.go`.
- CLI tests (seam 2): the verb with `on`, `off` and no argument; `--json`; the server-not-running refusal. Prior art: `cmd/afk_test.go`. The route parity test passes.
- Vitest (seam 3): the shared field passes value, change, disabled, rows and label; the shared layer's Escape option and narrow-screen backdrop option each work on and off; the start-over dialog closes on Escape; the expand button opens the editor; the helper makes only the topmost layer react to Escape; `Modal` traps Tab; the menu item switches and calls the write. Prior art: `NoteBox.test.tsx`, `Modal.test.tsx`, `BottomSheet.test.tsx`, `TopBar.test.tsx`, `afk.test.ts`.
- E2e (seam 4): real key presses for the command list, Cmd-G, `ZZ`, Cmd-Enter, Escape over a sheet, and touch emulation at 390 px for the bar. The existing flows that click "Open the note in a large editor" (`design-page.e2e.ts` flows 71 to 75, and the composer note steps in the CATALOG) keep passing, with the changes in the editor's footer and close controls updated. Prior art: flow 28 for the AFK menu item and `design-page.e2e.ts` for the note editor.
- Playwright cannot show a real on-screen keyboard. The phone and Android checks are manual, in the verify ticket.

## Verification

Run against a temporary gx server after the epic is built. Vi mode is turned on in any scenario with `fixture.cli(...)` and the new verb. Scenarios: `design` for checks 26 and 33, the pause scenario of `pause.e2e.ts` flow 3 (`paused`) for checks 27 and 52, and `design-working` (the message box with no round) for the rest.

Touch emulation means a Playwright project or helper with `hasTouch: true` and `isMobile: true`. Every check that needs the bar uses it. "390 px" alone means width only.

The new flows live in `vi-editor.e2e.ts` and are numbered from 117:

- Flow 117, "Vi editor: open and close": checks 8 to 12, 21 to 25, 29, 31 to 33.
- Flow 118, "Vi editor: commands": checks 13 to 20.
- Flow 119, "Vi editor: layers and focus": checks 26 to 28, 46, 47, 49, 51, 52.
- Flow 120, "Vi editor: layouts and touch bar": checks 24, 34, 35, 38 to 44.
- Flow 121, "Vi editor: setting": checks 5, 6 and 45.

Existing flows are named by file and name or number.

1. `design-page.e2e.ts` flows 71 to 75 and the composer note steps: the expand button is on the note and the message box. Vitest (seam 3): each of the 8 fields renders the expand button.
2. Flow 117 at 390 px: the expand button shows on the message box. Vitest (seam 3).
3. Vitest (seam 3): the Jump menu input and the Epics page input have no expand button. `jump.e2e.ts` flows 20 to 22 still pass.
4. Go test (seam 1): a fresh server's snapshot shows vi mode off. CLI check: the new verb with `--json` prints off on a fresh fixture.
5. `united.e2e.ts` flow 28 extended, and flow 121: the menu has a "Vi mode" checkbox item next to AFK; click it and `aria-checked` turns true.
6. Go test (seam 1): after a write and a server restart the value is kept. Flow 121: a second browser page shows the switch on after the first turns it on.
7. CLI check: the verb with no argument toggles and prints the new state; `on` and `off` set it; `--json` prints the structured result; with no server it refuses and exits non-zero. `go test ./cmd` passes, including the route parity test.
8. Flow 117: vi mode on, focus the message box, press Cmd-G (Ctrl-G on a non-Mac); the editor opens.
9. Flow 117: vi mode on, press Cmd-G with focus on the page body; no editor opens.
10. Flow 117: vi mode off, press Cmd-G in the field; no editor opens and the event is not cancelled. Manual: in Chrome, Safari and Firefox with vi mode off, find next works.
11. Vitest (seam 3): a disabled field has a disabled expand button and Cmd-G opens nothing.
12. Flow 117: the editor opens in INSERT with the caret where the field's caret was; typed text appears at that spot.
13. Flow 118 and Mock states insert, normal and visual: the mode text shows INSERT on open, and each mock state shows its mode text.
14. Flow 118: read the mode text. Escape shows NORMAL, `v` shows VISUAL, Escape then `i` shows INSERT.
15. Flow 118: with vi mode on, `h j k l w e 0 f{char} gg G` move the caret as vim does.
16. Flow 118: in a text of one long wrapped paragraph, `j` moves one screen line, not to the paragraph end.
17. Flow 118: move with `j`, click elsewhere, press `j`; the caret moves from the clicked column.
18. Flow 118: press Escape, then `x`, `dw`, `yy`, `p`, `u`, Ctrl-R, `iXY` Escape, `.`, and `v` with `d`; read the field text back after each step.
19. Flow 118: `i`, `a`, `o` and `.` change the text and mode as in vim.
20. Flow 118 with touch emulation at 390 px: `gs` and `ge` in normal and visual mode move to the line start and end.
21. Flow 117: press `ZZ` in normal mode; the editor closes, the text is kept in the field.
22. Flow 117: press Cmd-G (Ctrl-G) inside the editor; the editor closes.
23. Flow 117 at desktop width: press the backdrop; the editor closes. At 390 px (narrow) a backdrop press does not close it. Vitest (seam 3): the shared layer's narrow-screen backdrop option, on and off.
24. Flow 117 at desktop width, and flow 120 with touch emulation at 390 px, with vi mode on and off: Done is visible and closes the editor.
25. Flow 117: press Escape in insert and normal mode; the editor stays open. Vitest (seam 3): the shared layer's Escape option, on and off.
26. Flow 119 on `design`: open the question sheet, open the editor, press Escape; the sheet and the editor stay open. Vitest (seam 3): Modal and Jump menu react only when topmost.
27. Flow 119 on `paused`: open the start-over reason dialog, open the editor, press the editor's backdrop at desktop width; only the editor closes.
28. Flow 119: open the editor and press Cmd-K; the Jump menu does not open. Close the editor; Cmd-K opens it.
29. Flow 117: close the editor by each way (Done, Cmd-G, `ZZ`, backdrop); focus is on the field and the field's caret is the editor's caret.
30. Manual on a real phone: close the editor; the on-screen keyboard stays open.
31. Flow 117: type in the editor; the field's value updates as you type and the parent's request body has the text.
32. `agents-composer.e2e.ts` "79. Keyboard send" plus flow 117, at desktop width: type in the agent composer editor, press Cmd-Enter; the editor closes and the message sends.
33. Flow 117 on `design`: in the question answer editor, press Cmd-Enter; nothing happens and the editor stays open. At 390 px the composer has no submit action, so Cmd-Enter in its editor does nothing either.
34. Flow 120 with touch emulation at 390 px and vi mode on, and Mock states insert, normal and visual: the bar has the mode text, Esc and Done; Esc leaves insert mode; Done closes.
35. Flow 120 with touch emulation at 390 px: the pill, Esc and Done are 44 px high (bounding box check).
36. Manual on Safari on iPad, in portrait and in landscape: with the keyboard open, the bar sits just above it, and in landscape the modal's lower half is not under the keyboard. Playwright cannot show this.
37. Manual on Safari on iPad: in normal mode the on-screen keyboard sends commands and inserts no letters, and the mode does not get stuck.
38. Flow 120 with touch emulation at 390 px and vi mode off: the bar shows Done only, with no pill and no Esc.
39. Flow 120 with touch emulation at desktop width: the bar shows, and the modal is at most 680 px high.
40. Flow 120 with a mouse at desktop width: no bar.
41. Flow 120 with a mouse at desktop width: the modal is 860 by 680 with a title header, footer, Done and "Saved as you type", and no x button.
42. Flow 120 with a mouse at desktop width and vi mode on: the mode text is at the left of the footer and changes with the mode. With vi mode off, no mode text.
43. Flow 120 with touch emulation at desktop width: the modal has the title header and the bar, no footer, exactly one Done and exactly one `role="status"` mode text (vi mode on).
44. Flow 120 with a mouse at 390 px: the sheet has the title header and the footer with Done and the mode text (vi mode on), no bar and no x button.
45. Flow 121: with the editor open and vi mode on, flip the setting through the CLI verb; the mode text goes away and the vi keys stop working. Flip it back; the vi keys work. The text and caret are the same throughout.
46. Flow 119: press Tab and Shift-Tab; focus cycles through the editor and Done (and Esc with touch emulation) and never leaves the layer.
47. Flow 119: press Tab in the editor; the text has no tab character.
48. Vitest (seam 3): the mode text sits in a `role="status"` region and changes once per mode change. Manual with VoiceOver on Safari: the mode change is spoken.
49. Flow 119: `getByRole("dialog", { name: <title> })` exists; the editable area has the name "<title> text".
50. Manual with VoiceOver on Safari, vi mode off: plain typing and reading work.
51. Vitest (seam 3): the shared `Modal` traps Tab. Flow 119: in a plain modal, Tab never leaves it.
52. Flow 119 on `paused`: open the start-over dialog, press Escape; it closes. Tab never leaves it. With the editor open over it, Escape closes neither. Vitest (seam 3): `StartOverDialog` renders through the shared `Modal`.
53. Manual on a real Android phone with vi mode on: record what works and what does not in the verify ticket's Resolution. This is a check, not a pass or fail for vi; the spec makes no promise.
54. Vitest (seam 3): every one of the 8 fields renders through the shared field. Shell check: `rg -n '<Textarea' webui/src -g '!*.test.*'` shows exactly one hit, in the shared field's file.
55. Vitest (seam 3): the editor reads the parent's value and calls the parent's change callback; the field's state is not copied.
56. Playwright network check: the first page load does not request the editor chunk; opening the editor does.
57. Vitest (seam 3): each field's accessible label and editor title are the ones it passes in.
58. Vitest (seam 3): the shared layer's Escape option and narrow-screen backdrop option each work on and off (same tests as checks 23 and 25).

**Mock states**

- `$(gx tickets root)/webui-vi-mode/mocks/02-mobile-editor.html` · insert · 390 px with touch emulation · `design-working`, vi mode on, editor open on the message box in insert mode · editor, bar, mode-pill, esc-key, done-button, keyboard
- `$(gx tickets root)/webui-vi-mode/mocks/02-mobile-editor.html` · normal · 390 px with touch emulation · `design-working`, vi mode on, editor open, after Escape · editor, bar, mode-pill, esc-key, done-button, keyboard
- `$(gx tickets root)/webui-vi-mode/mocks/02-mobile-editor.html` · visual · 390 px with touch emulation · `design-working`, vi mode on, editor open, after `v` · editor, bar, mode-pill, esc-key, done-button, keyboard

The mock has no `keyboard` part in a real browser; it is a stand-in for the on-screen keyboard, so check the other five parts and the bar's place above the keyboard by hand on a phone.

New flows and scenarios to write (these become implementation tickets):

- Flows 117 to 121 in the new `vi-editor.e2e.ts`, with their catalog entries.
- The touch-emulation Playwright project or helper (`hasTouch: true`, `isMobile: true`).
- A manual verification list for iPad Safari, Android and VoiceOver, in the verify ticket.
- The edit to `united.e2e.ts` flow 28 for the new menu item.

One new fixture scenario, `design-text`, plays a text-answer question, because check 33 (the question answer editor) has no such editor in `design`. Every other check uses an existing scenario.

## Out of Scope

- Moving `FixAgentDialog` onto the shared `Modal`. It is a second hand-rolled overlay with no text field, so it is outside the Destination.
- Vi mode in the TUI.
- Single-line inputs: the Epics page and the Jump menu.
- Ex commands such as `:s`, and macros.
- A cancel button. Text saves as you type.
- A monospace font option.
- A fallback shortcut for Cmd-G, and any keyboard detection.
- A promise of full vi on Android on-screen keyboards, or of full screen reader support for vi commands.
- Reduced-motion work, because no animation is added.

## Further Notes

- The editor adds about 135 KB gzipped in its own chunk. The app is about 335 KB gzipped today.
- The engine is MIT licensed. Releases are rare (16 months between 6.3.0 and 6.4.0).
- Cmd-G cancel was tested on a plain page, not in the editor. Flows 117 to 121 and the manual browser check cover the editor itself.
- Upstream issue replit/codemirror-vim#200 describes the Android problem.
- The mock came from the prototype on the branch `prototype/webui-vi-mobile`. Layout "all-in-bar" was tested on Safari on an iPad Mini. Inside the claude.ai iframe the keyboard was invisible to the page, so the bar's place above the keyboard was proven at the top level of Safari only.
- `StartOverDialog` moves onto the shared `Modal` as part of this epic, because the focus trap and the topmost-layer rule do not reach a hand-rolled overlay.
- Terms used: AFK mode, snapshot, layer (the topmost layer owns Escape).
