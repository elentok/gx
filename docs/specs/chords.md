## Problem Statement

In the web UI, creating a chat, creating a design and removing the open chat all need the mouse or
the command palette. The TUI has a Leader key and a Chord box for this, and the web UI has none.
Each web shortcut today (⌘K, ⌘P, ⌘B, Alt+1..9) is its own window listener, so there is no one place
to add a keyboard action, and no way to see which keys exist.

## Solution

The web UI gets a Leader key. Pressing it opens the Chord box at the bottom right, which lists the
keys that may follow, each with what it does. The first chords are `c` (new chat), `d` (new design)
and `x` (remove the open chat, after the existing "Stop and remove?" confirm). The Leader key is set
on the web settings page, in a new Keybindings section that also holds the vi escape sequence.
The Leader key works while the person types in a text field on a normal page. It does nothing while
a modal is open, and every code editor sits in a bottom sheet.

## User Stories

1. As a person using the web UI, I want to press a Leader key and see a Chord box at the bottom
   right, so that I can find the next key without leaving the keyboard.
2. As a person on a Mac, I want the Leader key to default to ⌘E, so that it works with no setup.
3. As a person on Linux or Windows, I want the Leader key to default to Ctrl+E, so that it works with
   no setup.
4. As a person typing in the Message box, I want the Leader key to open the Chord box anyway, so that
   I do not click out of the field first.
5. As a person editing in a code editor (CodeMirror, plain or vim), which always sits in a bottom
   sheet, I want the Leader key to do nothing and the editor's own binding to run as before, so that a
   chord never starts a new form on top of text I am writing.
6. As a person who pressed the Leader key, I want `c` to start a new chat, so that I open the new
   chat form with two keys.
7. As a person who pressed the Leader key, I want `d` to start a new design, so that I open the new
   design form with two keys.
8. As a person on an agent page that has Remove (the Remove button's own test: any session that is
   not a Queue agent, so a chat, a Fix, a Design or a Triage agent), I want `x` to open the same confirm as Remove ("Stop and remove?" while the agent is live,
   "Remove?" once it has exited) with Keep focused, so that an accidental Enter cannot remove it.
   After the removal I land on the agents list, as with Remove in the sidebar row.
9. As a person on such an agent page, I want the Chord box to show an `x` row marked in red ("Remove
   chat" on a plain chat, "Remove" on the other agents), so that I see the destructive chord before I
   press it.
10. As a person on a design, the Queue, or a Queue agent, I want the Chord box to show no `x` row and
    `x` to do nothing, so that I cannot remove something that has no Remove today. A design here is
    the design page (the spec view), not a Design agent.
11. As a person with the Chord box open, I want to press Esc to close it, so that I can back out
    without side effects.
12. As a person with the Chord box open, I want any key it does not list to close it, so that a
    mistyped key never lingers.
13. As a person with the Chord box open, I want it to stay open with no timeout, so that I can think
    before the next key.
14. As a mouse user, I want to click a row in the Chord box to run it, so that the box works without
    the keyboard too.
15. As a person with the Chord box open in a text field, I want the next key to go to the chord and
    not into the field, so that pressing `c` does not type a letter.
16. As a person with a modal or a palette open, I want the Leader key to do nothing, so that it does
    not stack on top of the dialog I am using.
17. As a person with the command palette, I want the chords to run the same new-chat and new-design
    actions as the palette entries, so that both ways behave the same.
18. As a person who opens the settings page, I want a Keybindings section that holds the two Leader
    key settings and the vi escape sequence, so that I set all my keys in one place.
19. As a person on a Mac, I want one setting for the Mac Leader key (default `cmd+e`) and one for
    other platforms (default `ctrl+e`), so that one config file works on every machine I use.
20. As a person setting a Leader key, I want the settings page to refuse a key with no Cmd, Ctrl or
    Alt, so that I cannot make a plain letter steal my typing.
21. As a person who does not want chords, I want an empty Leader key to turn them off on that
    platform, so that nothing steals my keys.
22. As a person who changes the Leader key, I want the change to work at once with no page reload,
    so that I can try it right away.
23. As a person who opens the Chord box, I want its header to show the Leader key as I set it (for
    example ⌘E or Ctrl+E), so that I see which key I pressed.
24. As a person on the Keybindings section, I want the Leader key's help text to say "Press it to see
    the chords", so that I know where the chord letters are.
25. As a person who already set vi escape, I want its config key `server.web.vi-escape` to stay the
    same, so that my config file keeps working.
26. As a developer, I want the chords to live in one small chord table, so that I can add a chord
    later without a new window listener.

## Implementation Decisions

- **Chord box.** A new web UI component at the bottom right, in the approved Strip look (mock state
  `chat-page` / `other-page`): a blue leader chip on top, one row per chord (bold condensed key, then
  the action), the `x` row in red, a right-aligned "esc closes" hint, no footer rule. Dark theme
  only, Saira and Saira Condensed, web theme tokens. It lists only the chords that work on the open
  page. A click on a row runs it.
- **Chord table.** A small table of chords: key letter, label, the action it calls, a danger flag,
  and an "available" test for the open page. It is the one place that lists chords; the Chord box and
  the key handling both read it. More chords can be added later by adding a row.
- **First chords.**
  - `c` calls the new-chat action the command palette uses (`openNewAgent`).
  - `d` calls the new-design action the command palette uses (`openNewDesign`).
  - `x` runs the existing Remove flow, with Keep focused. It is available exactly when the Remove
    button shows today: `canManage(session)`, which is true for every session that is not a Queue
    agent (a chat, a Fix, a Design or a Triage agent). On a design page, the Queue, or a Queue agent
    it is not listed and does nothing. The row reads "Remove chat" when the session has no `ticket`,
    `fix_of` or `flow`, and "Remove" otherwise. The confirm text follows the agent: "Stop and
    remove?" while live, "Remove?" once exited. After the removal the page goes to the agents list,
    as Remove in the sidebar row does.
  - The remove confirm state is lifted out of the row menu in `useSessionManage` into a shared opener
    and host, like `newItemOpen.ts`, so the row menu and `x` open the same dialog. The dialog is not
    copied. The opener takes the session and a callback on each open, as each caller does today. The
    sidebar row and `x` send the page to the agents list only when the removed chat is the open page.
  - The letters differ from the TUI on purpose (`d` is new design here, remove in the TUI).
- **Key handling.** One capture-phase keydown listener on the window. It matches the Leader key,
  calls `preventDefault()`, and opens the Chord box, in text fields and textareas on normal pages.
  Chromium and WebKit runs showed the capture listener sees Cmd+E and Ctrl+E first in all of them,
  and `preventDefault()` stops the caret jump and CodeMirror's own binding (CodeMirror skips its
  keymap on a default-prevented event, which matters only if the modal guard is ever relaxed).
  Key matching uses `event.code` and the exact set of modifiers, so Alt+E works on a Mac, where
  `event.key` is "Dead". The setting syntax is fixed: modifiers (`cmd`, `ctrl`, `alt`, `shift`)
  joined with `+`, then one letter or digit, in lower case (`cmd+e`, `alt+j`). While the Chord box is open the same listener takes every
  keydown: a listed chord key runs and closes the box, Esc or any other key closes it. Each of those
  keys is prevented so it never reaches a field. Pressing the Leader key again also closes the box,
  as an unlisted key. The listener calls `stopImmediatePropagation()` on every key it takes while the
  box is open, and the box joins the layer stack and has `role="menu"`, so the page checks in `ChatLog` and the
  interrupt logic (`escOpensInterrupt`) skip its Esc. Order of window listeners is not relied on. No other window listener acts on that key: the
  interrupt confirm, the question sheet's Esc, ⌘K, ⌘P, ⌘B and Alt+1..9 do not fire under an open
  box. A click outside the box, a page change and the window losing focus also close it, with no
  action.
- **No timeout.** The Chord box stays until a chord runs, a key closes it, a row is clicked, or one
  of the closes above happens. No timer closes it.
- **Modal guard.** The Leader key is ignored while `hasLayer()` is true (a modal, the jump palette or
  the command palette is open), as ⌘B is. A chord that opens a modal (the `x` confirm) closes the box
  first.
- **Settings.** A new Keybindings section on the settings page, listed with the other sections. It
  holds three settings: the Mac Leader key (default `cmd+e`), the Leader key for other platforms
  (default `ctrl+e`), and vi escape. vi escape moves into this section only; its config key
  `server.web.vi-escape` and its behavior do not change. The two Leader key settings are strings with the config keys
  `server.web.leader-key-mac` and `server.web.leader-key`; the implementer picks their labels,
  following the registry's naming. Each must include
  Cmd, Ctrl or Alt: the registry's validation refuses a value without one, with a message the page
  shows like any other refused save. An empty value is valid and turns chords off on that platform.
  The page shows the Leader key's help text "Press it to see the chords" and does not list the
  chords.
- **Which setting applies.** The page uses the Mac setting on a Mac and the other-platforms setting
  elsewhere. The platform is detected with
  `navigator.userAgentData?.platform ?? navigator.platform` matching /Mac/, so a Playwright context
  can drive it.
- **Live apply.** The two settings reach the page without a reload, the same way vi escape reaches it
  today: they are part of the live settings the server sends the page, and the listener reads the
  current value.
- **Leader display.** The Chord box header shows the Leader key as set, with Mac symbols on a Mac
  (⌘E) and words elsewhere (Ctrl+E). The settings field shows the stored string (`cmd+e`).
- **Browser keys.** The defaults stand on every platform. The spec does not ask the implementer to
  check Firefox or Ctrl+E on Linux and Windows.
- **No change to** the TUI, its `agents.leader-key`, the command palette, ⌘K, ⌘P, ⌘B or Alt+1..9.
  The chord handling is its own listener and does not refactor those.
- **Terms.** Leader key and Chord box are in `GLOSSARY.md`; no new term is needed.

## Testing Decisions

- A good test drives the web UI the way a person does: press keys, click rows, read what shows. It
  does not call the chord table or the listener directly.
- Highest seam: a Playwright e2e step against the fixture server, plus unit tests of the pure parts
  (matching a pressed key to a configured Leader key, and which chords are available for a page).
  The Chord box and the listener take the chord table as an input, with the real table as the
  default, so a unit test can pass its own table.
- Per the flow budget, do not add a flow. Add steps to existing flows on the same scenarios: chord
  steps to the `workspaces` flow (chat, design and epic pages, jump and command palettes) and
  Keybindings steps to the `settings` scenario's global journey flow.
- Registry tests cover the two new entries: defaults, the modifier rule, empty value allowed, and
  that `server.web.vi-escape` keeps its key.
- Prior art: `webui/e2e/jump.e2e.ts` and the Keyboard steps of the workspaces flow for key-driven web
  e2e, `settings-page.e2e.ts` for settings, and `config/registry/entries_core_test.go` and
  `live_keys_test.go` for registry and live keys.

## Verification

Entries are by user story number. "Chord flow" is the new chord steps in the `workspaces` flow
(`webui/e2e/workspaces.e2e.ts`, scenario `workspaces`, which has chats, designs, epics and a Queue
agent). "Settings flow" is the `settings` scenario's **200. Settings page, global journey**.

1. Chord flow: press the Leader key; the Chord box shows at the bottom right. Look check: Mock states
   `chat-page`.
2. Chord flow on a Mac context (Playwright `userAgent`/platform for Mac): the chip reads ⌘E and Meta+E
   opens the box. New step to write.
3. Chord flow on a non-Mac context: the chip reads Ctrl+E and Ctrl+E opens the box. New step to write.
4. Chord flow: focus the Message box, type text, press the Leader key; the box opens and the text
   stays as typed. New step.
5. Chord flow: open the large editor sheet (the `vi-editor` flow already drives a `.cm-editor`),
   press the Leader key; no box opens and the editor text is unchanged. New step.
6. Chord flow: Leader then `c`; the new chat form opens. Same result as the palette entry.
7. Chord flow: Leader then `d`; the new design form opens.
8. Chord flow: on a live chat, Leader then `x`; "Stop and remove?" shows with Keep focused; Enter keeps
   the chat; a second Leader then `x` then Remove removes the chat (`gx agents list` no longer lists
   it) and the page shows the agents list. On a Fix agent page (`paused-epic/01`), `x` opens the same
   confirm. Unit tests: the exited-chat confirm reads "Remove?", and the label rule. New steps.
9. Look check: Mock states `chat-page` (the red `x` row, "Remove chat"). On the Fix agent page the
   row reads "Remove" (the Chord flow reads the text; no new mock state).
10. Chord flow: on a design, the Queue and a Queue agent, the box has `c` and `d` only and `x` does
    nothing. Look check: Mock states `other-page`.
11. Chord flow: Leader then Esc; the box closes and the page is unchanged.
12. Chord flow: Leader then an unlisted key (for example `z`); the box closes and nothing runs.
13. Unit test with fake timers: the box never closes on a timer. Chord flow: open the box, do a
    harmless event (a mouse move); it is still open.
14. Chord flow: open the box, click the `c` row; the new chat form opens.
15. Chord flow: in the Message box, Leader then `c`; no `c` is typed in the field.
16. Chord flow: with the command palette open, and again with a modal open, the Leader key opens no
    box. Existing flows already cover ⌘P and ⌘K opening. With the box open on a Working chat, Esc
    closes the box and opens no interrupt confirm; in the Questions view, Esc closes only the box;
    ⌘K and ⌘P open nothing. Unit test: a click outside,
    a page change and the window losing focus each close the box.
17. Chord flow: Leader then `c` and the command palette's new-chat entry open the same form, started
    on the same project. Existing workspaces steps cover the form's project rule.
18. Settings flow: the Keybindings section lists the two Leader key settings and vi escape; vi escape
    is not in its old section. New step.
19. Settings flow: both Leader settings show their defaults (`cmd+e`, `ctrl+e`); the Mac one is
    used on the Mac context, the other on the non-Mac context. CLI check: `gx server settings` lists both keys with these defaults. New step.
20. Settings flow: save `e` as a Leader key; the page shows "Not saved:" and the draft stays. Save
    `alt+e`; it saves. New step.
21. Settings flow: save an empty Leader key; the Leader key opens no Chord box on that platform. New
    step.
22. Settings flow: open a second page on the same server, change the Leader key to `alt+j` on the
    settings page; on the second page, with no reload (the suite already fails on a reload), `alt+j`
    opens the box and the old key does not.
23. Settings flow: after step 22, the second page's box header reads the new key. Look check: Mock
    states `chat-page` (leader-chip).
24. Settings flow: the Leader key's help text reads "Press it to see the chords" and the section
    lists no chord letters. New step.
25. Registry unit test and CLI check: `server.web.vi-escape` still reads and writes through
    `gx server settings` and `gx config get server.web.vi-escape` with its old key; a value set before this change still
    applies.
26. No flow. Unit test: the Chord box and listener get a test table as an input; its row shows in the
    box and runs. The real table is not changed.

**Mock states**:

- `$(gx tickets root)/chords/mocks/02-mock-chord-box.html`, `chat-page`, 1040px, scenario
  `workspaces` (existing; open a removable chat in Default, on a Mac context), parts: chord-box,
  leader-chip (⌘E), row-new-chat, row-new-design, row-remove-chat (red), esc-hint.
- `$(gx tickets root)/chords/mocks/02-mock-chord-box.html`, `other-page`, 1040px, scenario
  `workspaces` (existing; open the Epics page or a design, on a Mac context), parts: chord-box,
  leader-chip (⌘E), row-new-chat, row-new-design, esc-hint. No `row-remove-chat`.

## Out of Scope

- Chord letters as settings. Only the Leader key is a setting.
- Matching the TUI's letters, and any TUI change. The TUI `agents.leader-key` stays TUI-only.
- More chords (design actions, Queue actions, navigation).
- Touch and phone screens, which have no keyboard.
- Listing the chords on the settings page.
- A fallback Leader key for a browser that will not let the page stop its key. Firefox and Ctrl+E on
  Linux and Windows are untested; the defaults stand and a person who reports a clash can change the
  setting.
- Moving ⌘K, ⌘P, ⌘B or Alt+1..9 onto the chord table or a shared shortcut registry.

## Further Notes

- Research: `docs/research/chords-browser-leader-probe.md` (branch
  `research/chords-browser-leader-probe`) holds the Chromium and WebKit probe. Safari and Chrome on
  Mac were checked by hand in the mock and let the page stop ⌘E.
- The TUI box (`RenderChordOverlay`) is the model for placement and for having no timeout.
