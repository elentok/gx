## Problem Statement

When gx runs as an installed app on a Mac, the window has no back and forward buttons. A person who
wants to return to the last page has no control for it on screen.

A person at the keyboard also has no gx key for back and forward, and no way to open the next or the
previous sidebar item without moving focus to the sidebar or knowing the item's Alt+number.

## Solution

The sidebar header gets a Back button and a Forward button, on every screen size, in browser tabs and
in installed apps. A button is greyed out when there is nowhere to go. The version text leaves that
header and moves to the end of the footer ⋮ menu.

The Leader key gets four new chords. `,` goes back and `.` goes forward. `j` opens the next sidebar
item and `k` opens the previous one. `j` and `k` leave focus where it was. Back and forward treat focus
as the browser's own back and forward do in gx today.

Back and Forward need the browser's Navigation API. A browser without it shows no Back or Forward
button and no Back or Forward chord. `j` and `k` work in every browser.

## User Stories

Back and forward buttons

1. As a person using the installed Mac app, I want a Back button in the sidebar header, so that I can
   return to the page I was on.
2. As a person who went back, I want a Forward button, so that I can return to the page I left.
3. As a person using gx in a browser tab, I want the same two buttons, so that gx looks and works the
   same in every window.
4. As a person on a desktop screen, I want Back and Forward to be the two rightmost header buttons,
   after Hide sidebar, so that Search and Hide sidebar keep their order.
5. As a person on a phone or a tablet, I want the same header with the same four buttons, so that I
   can go back without a browser toolbar.
6. As a person on the first page of a visit, I want Back greyed out, so that I know there is nowhere
   to go.
7. As a person on the newest page, I want Forward greyed out, so that I know there is nowhere to go.
8. As a person who moves between pages, I want the greyed-out state to follow every move, however I
   made it, so that the buttons are never wrong.
9. As a person who reloads the page, I want Back and Forward to keep working, so that a reload does
   not lose my way back.
10. As a person who opens gx at its root address or at an unknown address, I want to land on the Epics
    page with Back greyed out, so that Back never leads to a page that only sends me forward again.
11. As a person who has just finished the first-run setup, I want Back greyed out on the first page,
    so that Back does not lead to the finished setup form.
12. As a person who hid the sidebar, I want no extra back or forward buttons on the page, so that the
    page header stays as it is. The chords still work.
13. As a person on a phone or a tablet with the sidebar open, I want it to stay open when I tap Back
    or Forward, so that I can tap several times in a row.
14. As a person using a screen reader, I want the buttons to be named "Back" and "Forward", so that I
    know what they do.
15. As a person using a browser without the Navigation API, I want no Back or Forward button, so that
    I never see a control with a wrong state.
16. As a person who uses the browser's own back and forward keys or gestures, I want gx to leave them
    alone, so that they behave as before.

Tooltips

17. As a mouse user, I want the Back and Forward tooltips to show my own Leader key and the chord, so
    that I can learn the keyboard way.
18. As a person with no usable Leader key, I want the tooltips to read only "Back" and "Forward", so
    that gx does not show a chord I cannot type.
19. As a mouse user, I want all four header buttons to use the same kind of tooltip, so that the
    header behaves in one way.
20. As a mouse user on a greyed-out button, I want its tooltip to show, so that I can tell what the
    button is.
21. As a touch user, I want no tooltip when I tap a header button, so that nothing covers the page.

Version

22. As a person who needs the server version, I want it as the last line of the footer ⋮ menu on every
    screen size, so that I can still find it.
23. As a person writing a bug report, I want a click on that line to copy only the version, so that I
    can paste it as it is.
24. As a person who copied the version, I want the menu to close and a toast to say "Version copied"
    or "Could not copy", so that I know what happened.
25. As a person whose page does not know the server version yet, I want no version line and no extra
    separator, so that the menu has no empty row.

Back and forward chords

26. As a keyboard user, I want Leader then `,` to go back, so that my hands stay on the keyboard.
27. As a keyboard user, I want Leader then `.` to go forward, so that I can undo a step back.
28. As a person who types on a Hebrew or another layout, I want the two chords to follow the physical
    keys, so that they work without switching layout.
29. As a person who pressed Shift, Cmd, Ctrl or Alt with `,` or `.`, I want the Chord box to close and
    nothing else to happen, so that these chords follow the same rule as the others.
30. As a person who opened the Chord box, I want "Back" and "Forward" rows after the other app chords,
    so that I can find them and click them.
31. As a person with nowhere to go in one direction, I want that row hidden and its key to do nothing,
    so that the box lists only chords that work.
32. As a person with the sidebar hidden, I want the two chords to work, so that I do not need the
    buttons.

Next and previous sidebar item

33. As a keyboard user, I want Leader then `j` to open the next sidebar item, so that I can walk down
    my work.
34. As a keyboard user, I want Leader then `k` to open the previous sidebar item, so that I can walk
    back up.
35. As a keyboard user, I want the order to be the one the sidebar shows, so that the move matches what
    I see in every sort.
36. As a keyboard user, I want epic, Design, chat, draft and nested agent rows visited, and section
    headers and footer links skipped, so that every step opens an item.
37. As a keyboard user, I want rows inside a collapsed row or a closed section skipped, and nothing
    expanded, so that the sidebar keeps its shape.
38. As a keyboard user on the first or the last row, I want no wrap, so that I do not jump to the other
    end by mistake.
39. As a keyboard user, I want one step per Leader press, so that each press has one effect.
40. As a keyboard user, I want focus to stay where it was, and a chat page opened this way not to take
    focus into its prompt, so that I can keep stepping.
41. As a keyboard user, I want the sidebar list to scroll to the new row, so that I can see where I am.
42. As a keyboard user, I want each step to be a history entry, so that Back returns to the last item.
43. As a keyboard user with the sidebar hidden, I want `j` and `k` to work and the sidebar to stay
    hidden, so that I can use the full width.
44. As a person with a sliding sidebar, I want `j`, `k`, `,` and `.` to leave it open or closed as it
    was, so that a chord only changes the page.
45. As a person in select mode, I want `j` and `k` to work as usual, so that the keys do not change
    meaning.
46. As a person on a ticket, answer, Design, epic or Diff page of an item, I want that item's row to
    count as the current one, so that `j` opens the row after it and `k` the row before it.
47. As a person on a chat's Diff page, I want the chat's row to count as the current one, so that the
    next step starts from the chat.
48. As a person whose current row is hidden inside a collapsed row or a closed section, I want the step
    to start from the place of that row, so that I move to its neighbours.
49. As a person on a page with no row, such as Settings, Stats, an archive page or a file page, I want
    `j` and `k` to continue from the last row I was on, so that a side visit does not lose my place.
50. As a person who reloads a page with no row, I want gx to still know my last row in this tab, so
    that a reload does not lose my place.
51. As a person with no earlier row, or whose last row has left the sidebar, I want `j` to open the
    first visible row and `k` the last one, so that the keys always have a start.
52. As a person who opened the Chord box, I want "Next item" and "Previous item" rows after "Back" and
    "Forward", each hidden when it has no target, so that the box lists only chords that work.
53. As a person using a browser without the Navigation API, I want `j` and `k` to work and the Chord
    box to show no "Back" or "Forward" row, so that I lose only what the browser cannot do.

## Implementation Decisions

### History state

- One hook owns the history state. It answers whether the Navigation API is present, whether Back is
  possible and whether Forward is possible. It reads `canGoBack` and `canGoForward` and updates on the
  `currententrychange` event. The buttons, the Chord box and the chords all read this one hook.
- gx treats the API as missing when `window.navigation` has no value.
- There is no fallback. gx does not read the router's `history.state.idx` and stores no history
  position of its own. Plain `#/...` links create history entries with no router state, so such a
  count is wrong.
- Going back or forward moves through the browser's session history. The call that does it is the
  implementer's choice.
- gx does no installed-app detection for this feature.
- gx binds no Cmd+[, Cmd+], Cmd or Ctrl with an arrow, or swipe gesture.

### Entries that lead nowhere

- The catch-all route, which sends the root address and unknown addresses to the Epics page, replaces
  the current history entry instead of adding one. Today a fresh open leaves two entries and Back is
  enabled on the first page.
- Finishing the first-run setup replaces the setup page's history entry instead of adding one. The app
  then opens on the Epics page with Back greyed out.

### Sidebar header

- The header holds, in order, the gx logo, a spacer, Search, Hide sidebar, Back and Forward.
- All four buttons are 34px icon buttons with a radius of 8px, dim text colour and the raised surface
  on hover. This is the style the Search and Hide sidebar buttons declare today.
- None of the four buttons shrinks (`flex: none`). This is new: today's buttons do not declare it.
- Back and Forward use `LuChevronLeft` and `LuChevronRight` from the app's icon library, at 20px. The
  stroke width makes the lines as thick as the Hide sidebar icon's, 1.6px on screen, which is 1.92 in
  the icon's 24-unit box.
- A greyed-out button has `aria-disabled="true"`, not the `disabled` attribute, so that it still gets
  pointer and focus events for its tooltip. It ignores clicks, has opacity 0.5 and shows no hover
  surface.
- The buttons exist only in the sidebar header. A hidden sidebar shows none.
- On the sliding sidebar, a tap on Back or Forward leaves the sidebar open.
- The accessible names are "Back" and "Forward".

### Tooltips

- All four header buttons use the Chakra tooltip. None has a native `title`.
- Search keeps its text, "Search (⌘K)" on a Mac and "Search (Ctrl+K)" elsewhere. Hide sidebar reads
  "Hide sidebar".
- Back reads "Back (⌘E ,)" and Forward "Forward (⌘E .)", where the first part is the person's Leader
  key for this platform, shown as the Chord box shows it. With an empty or unreadable Leader key they
  read "Back" and "Forward".
- A greyed-out button still shows its tooltip. A tap on a touch screen shows no tooltip.

### Version line

- The sidebar header no longer shows the version.
- The footer ⋮ menu ends with a separator and the line "gx <version>", on one line, in faint 12px
  text, on every screen size.
- A click copies the version without the "gx " prefix, closes the menu and shows the toast "Version
  copied", or "Could not copy" when the copy fails. It uses the app's existing clipboard helper and
  toaster.
- With no known server version, the menu has neither the line nor its separator.
- The browser flows that read the running version from the header read it from this menu line.

### Chords

- The four chords are app chords. They match the physical key: `Comma`, `Period`, `KeyJ` and `KeyK`.
  The chord matching, which accepts only letters today, accepts these two punctuation keys.
- Shift, Cmd, Ctrl or Alt with any of the four closes the box and does nothing.
- The app chords end with, in this order, `,` "Back", `.` "Forward", `j` "Next item" and `k` "Previous
  item". A row is hidden, and its key does nothing, when it has no history entry or no target. The
  rows follow changes while the box is open.
- Without the Navigation API, the `,` and `.` chords do not exist.
- The existing rules for the Leader key stay: it opens nothing over a modal, and one press allows one
  chord.
- The four chords never open, close or expand anything in the sidebar. `j` and `k` do not move
  focus.
- `,` and `.` do not change the prompt focus rule. A chat page that was opened with a prompt focus
  request, for example by Enter on its sidebar row, focuses the prompt again when a step back or
  forward lands on it. The person chose to keep this behaviour.

### Next and previous item

- The target is decided by one pure module from these inputs: the open page's address, the sidebar
  model, the rows the sidebar renders and the last row of this tab.
- The order is the order of the rendered rows, also while the sidebar is hidden. The place of a row
  that is not rendered comes from the sidebar model.
- Every row that opens a page is visited. These are epic, Design, chat, chat draft and Design draft
  rows, nested agent rows, and the agent rows under Other, Triage included. Section headers, group
  headers and footer links are not visited.
- A row owns these pages.
  - Every row owns the page its link opens.
  - An epic or Design row also owns the epic, Design, ticket, answer and Diff pages of all its epic
    addresses, including the `-impl` epic of a joined pair. A Diff page belongs to it when the page
    shows a range of one of those epics, or the worktree of one of those Designs or of a ticket in
    them.
  - A chat row or an agent row also owns the Diff page of that session's own worktree.
  - File viewer pages, archive pages and every other page belong to no row.
- The current row is found like this.
  - If the open page belongs to rows, the current row is the first of them from the top. gx remembers
    it as the last row.
  - Otherwise the current row is the last row, if the sidebar still holds it, visible or not.
  - Otherwise there is no current row. `j` targets the first visible row and `k` the last.
- With a current row, `j` targets the first visible row after its place and `k` the first visible row
  before it. For a hidden nested agent, the row before is its parent. There is no wrap.
- gx remembers the last row per browser tab, and keeps it across a reload. The storage key is the
  implementer's choice. Blocked storage must not break the page.
- Opening the target adds a history entry, does not ask the chat page to focus its prompt, and scrolls
  the sidebar list so the new row is in view.
- The highlight rule of the sidebar does not change.
- Select mode and the sliding sidebar behave the same as the docked sidebar.

## Testing Decisions

- A good test drives what a person does, a key press or a click, and checks what the person sees: the
  address, the button state, the Chord box rows, the menu and the toast. It does not check which
  history call ran or how the module is built.
- The browser flow against a fixture server covers only what needs a real browser: history moves,
  reload, the three layouts, real key presses, focus, hover and sidebar scroll. The `workspaces`
  scenario already has epic, Design and chat rows, nested agents, held epics and an Other section. The
  `web-setup` scenario covers the setup entry.
- The nav steps are a new flow, although "Workspaces" and "Chords" start the same scenario. Both run
  with a 120 s time limit and were already split to stay inside it, and this journey does not fit in
  either. The ticket that adds the flow raises the `playwrightguard` flow budget if needed and says why
  in its title.
- What a component renders for its props is tested in component tests, not in a flow.
- The one new unit seam is the pure next and previous module. Its table tests cover page ownership,
  hidden rows, nested agents, the last row and the ends of the list. Prior art is the sidebar model's
  tests.
- Component tests of the app shell, the Chord host, the Chord box and the gear menu cover the buttons,
  tooltip text, the empty Leader key, the missing `title` attribute, the header and Chord box rows
  without the Navigation API, and the version menu line.
  The test environment has no Navigation API, so a test installs a small fake `window.navigation`, and
  leaves it out for the unsupported-browser cases. Prior art is the Chord host, Chord box, app shell
  and gear menu tests.
- The hover on a greyed-out button also runs in WebKit, the engine of the Safari Dock app. The flow
  starts a WebKit browser for that step, and the CI workflow and the Makefile e2e target install WebKit
  beside Chromium.

## Verification

Buttons and history state (stories 1 to 5, 7 to 9, 12 to 14, 16)
- Flows: a new nav flow on scenario `workspaces`. The implementer picks the file and the flow's name.
  At 1280px it checks the header order, opens two rows by click, and checks Back and Forward after
  each of a row click, a link inside a page, a Back click, the browser's own back, and a reload. With
  the sidebar hidden, no Back or Forward button is on the page. At 744px and 390px the open sidebar
  shows the four buttons and stays open after a tap on Back and on Forward. Each button is found by
  the accessible name "Back" or "Forward". Look check: the Mock states `b2-shown-both`,
  `b2-shown-back-off` and `b2-hidden`.
- Checks: `gx brief -C webui playwright <file> <flow numbers>` exits 0. A key listener added by the
  flow sees Meta+[ and Meta+] arrive with `defaultPrevented` false.
- New flows to write: the nav flow above. It becomes an implementation ticket.

First page (stories 6, 10, 11)
- Flows: a step in the new nav flow opens the root address in a new page, and then an unknown
  address in another new page. Each lands on `#/epics` with Back greyed out. The flows
  "99. First-run setup", "100. Setup from the QR link" and "101. Setup and login with TOTP off" in
  `setup.e2e.ts` end with Back greyed out.
- Checks: on each of those pages, `navigation.canGoBack` is `false`.
- New flows to write: the steps above.

Unsupported browser (stories 15, 53)
- Checks: component tests with no `window.navigation`. The header has Search and Hide sidebar only. The
  Chord box has "Next item" and no "Back" or "Forward" row. Leader then `,` closes the box and leaves
  the address. Leader then `j` opens a row.

Tooltips (stories 17 to 21)
- Flows: a step in the new nav flow hovers a greyed-out Back in Chromium and in WebKit and sees its
  tooltip. At 390px with touch, a tap on Forward shows no tooltip.
- Checks: component tests read each header button's tooltip with a Mac and a Linux platform and with an
  empty Leader key. They find no `title` attribute on any header button, and a click on a greyed-out
  button moves nowhere.
- New flows to write: the step above.

Version line (stories 22 to 25)
- Flows: the existing restart flows that compare versions read the version from the footer menu line
  and still pass. Look check: the Mock state `b2-shown-menu`.
- Checks: component tests of the gear menu. The header shows no version. The last menu item reads
  "gx <version>". A click copies the `build` value without "gx ", closes the menu and shows "Version
  copied". With a failing clipboard write the toast reads "Could not copy". With no `build` there is
  neither the item nor its separator.
- New flows to write: none. The restart helpers change.

Back and forward chords (stories 26 to 32)
- Flows: the "Chords" flow in `chords.e2e.ts` and the new nav flow. Leader then `,` and `.` move
  through history, also with the sidebar hidden. Leader then Shift+`,` closes the box and leaves the
  address. A step back to a chat opened with Enter focuses its prompt, as the browser's own back does.
- Checks: component tests of the Chord box and the Chord host. The box lists "Back" and "Forward" after
  "History" or the last other app row, hides the one with no entry, and a click on a row runs it.
- Checks: a component test sends `code: "Comma"` with a Hebrew `key` value and gets Back.
- New flows to write: the steps above.

Next and previous order (stories 33 to 45, 52)
- Flows: the new nav flow on `workspaces`. From the first row, repeated Leader then `j` visits the
  visible rows in the sidebar's order and stops at the last, where "Next item" is hidden. `k` walks
  back. Collapsed rows and closed sections are skipped and stay closed. Focus stays in the Message box
  it was in, and a newly opened chat does not focus its prompt. Back returns to the previous item. With
  the sidebar hidden, at 390px open and closed, and in select mode, the keys work and the sidebar
  keeps its state. After a step to a row below the fold, that row is in view.
- Checks: `gx brief -C webui playwright <file> <flow numbers>` exits 0.
- New flows to write: the steps above.

Current row (stories 46 to 51)
- Flows: the new nav flow on `workspaces`. On a ticket page of an epic and on a chat's Diff page, `j`
  and `k` open the neighbours of that item's row. With a nested agent's page open and its row collapsed
  again, `k` opens the parent and `j` the row after the hidden place. From the Settings page, also
  after a reload, `j` continues from the last row. In a new tab opened on Settings, `j` opens the first
  visible row and `k` the last. After a workspace switch that removes the last row, the same holds.
- Checks: the unit table of the next and previous module covers the answer page, the Design page, the
  `-impl` address of a joined pair, each Diff page kind, an archive page and a file viewer page.
- New flows to write: the steps above.

**Mock states**
- `$(gx tickets root)/back-fwd-nav/mocks/03-button-placement.html`, `b2-shown-both`, 1280 px, scenario `ready`, parts: nav-buttons.
- `$(gx tickets root)/back-fwd-nav/mocks/03-button-placement.html`, `b2-shown-back-off`, 1280 px, scenario `ready`, parts: nav-buttons.
- `$(gx tickets root)/back-fwd-nav/mocks/03-button-placement.html`, `b2-shown-menu`, 1280 px, scenario `ready`, parts: nav-buttons, gear-menu, version-item.
- `$(gx tickets root)/back-fwd-nav/mocks/03-button-placement.html`, `b2-hidden`, 1280 px, scenario `ready`, parts: none (the sidebar is hidden and no button shows).

## Out of Scope

- The TUI.
- A fallback for browsers without the Navigation API.
- Installed-app detection.
- Back and forward buttons outside the sidebar header, for example in the page header.
- Binding the browser's own back and forward keys or gestures.
- Wrapping at the ends of the sidebar, key repeat after one Leader press, and expanding rows or
  sections.
- Changing which row the sidebar highlights.
- Changing when a chat page focuses its prompt after a step back or forward, or after a reload.

## Further Notes

- The stored mock shows the header buttons, the greyed-out state, the menu line and the hidden
  sidebar. It was approved before the tooltips, the phone and tablet header and the click on the
  version line were decided. This spec adds those. Phone and tablet need no mock of their own. The
  review later switched the chevrons to the icon library and the greyed-out state to `aria-disabled`,
  and the stored mock shows both.
- Chromium 157 and WebKit 27.2 keep `canGoBack` and `canGoForward` across a reload. A real Safari Dock
  app and a real Chrome app window were not tested. They use the same engines.
- The glossary entry **Chord box** covers the new punctuation chords.
