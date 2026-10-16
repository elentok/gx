# Title bar: replace the bottom tab bar with a top title row and a view switcher

## Problem Statement

gx draws a row of eight tab pills at the bottom of every screen, plus a server pill
(`server ● pid 58011`) and, on some pages, a footer with hints and state (`filetree · tmux · ? help`,
`diff: mode:hunk · render:unified · wrap:off`, a `VISUAL` prefix, the worktrees busy spinner and
paste-mode hints).

From the person's side:

- Nine boxes of equal weight compete with the panel headers. The bar sits on the raw terminal
  background while the panels are tinted, so it looks pasted on. The server pill is a different
  shape from the tabs. gx's core principle is that it must be visually beautiful, and this bar is
  the loudest, least useful thing on screen.
- Most of what the bar shows is noise most of the time: seven inactive tab names, a healthy server's
  pid, the split app's name, the filetree/diff focus label. The few things that need attention (an
  agent waiting on the person, a server that is down) look the same as everything else.
- The footer is the last row. Navigation and state live far from where the eye is (the panel
  headers at the top).
- The only way to reach a view is to know its number or `g` chord, or to click its pill. There is no
  "find a view by typing its name".
- On the agents tab the tab bar is drawn over the last row of the sidebar and of the terminal, so
  that row (and sometimes the cursor) is hidden. On the stash tab the page's own footer is cut off.

## Solution

A quiet title row at the top replaces the tab bar and every page footer:

```
 gx › status ▾                                              agents 2   ●  ? help
 (blank row)
 ⎇ Changes in  main ↑2                     Unstaged (empty)                    ±3
 ...
```

- **Left:** `gx ›` (dim), the current view's name (bold), and `▾` (dim), which says "this opens a
  menu". Only the current view is shown, never the full list of tabs.
- **Right:** only things that need attention: the agents Actionable count (`agents 2`, the number in
  red, shown only when it is above zero), the server state as one colored icon (words only when the
  server is unhealthy), and `? help`.
- A blank row separates the title row from the panel headers. No page keeps a footer.
- **View switcher:** `ctrl+o`, or a click on the title, opens a fuzzy list of all views, grouped
  (git: worktrees, log, status, stash, prs; agents: tickets, queue, agents). Type to filter, `enter`
  to go, `esc` to close. The current view is marked and agents shows its Actionable count. In the
  agents terminal `ctrl+o` goes to the agent (Claude Code uses it); there the switcher opens with the
  leader: `ctrl+a ctrl+o`.
- **Focus the agents sidebar:** `ctrl+q` in the agents terminal moves focus to the sidebar in one
  key, without the leader. It is configurable, like the leader key. `ctrl+a tab` keeps working.
- Number keys `1`-`8` and the `g` chords keep working exactly as today.
- Details that used to sit in the bar move to the help dialog: server pid and state detail, the gx
  version (already there), and the split app (herdr / tmux / kitty).
- Page state that lived in footers moves into panel headers: status diff options into the focused
  diff panel header; the worktrees busy spinner and paste mode into the `Worktrees` panel header.
- Notifications move down one row so they never cover the title row.
- On narrow terminals the title row shortens in a fixed order and never needs a "terminal too small"
  screen.

Every page gets one fewer content row than today (two chrome rows at the top instead of one at the
bottom). The agents tab gets back the row that the tab bar used to hide.

## User Stories

Title row

1. As a gx user, I want to see the name of the view I'm in at the top left, so that I always know
   where I am without scanning a row of tabs.
2. As a gx user, I want `gx ›` to be dim and the view name bold, so that the eye lands on the view
   name and the brand stays quiet.
3. As a gx user, I want a `▾` after the view name, so that I can tell it opens a menu.
4. As a gx user, I want a blank row between the title row and the panel headers, so that the title
   row does not read as one more panel header.
5. As a gx user, I want the title row to look the same on every page, so that gx feels like one app.
6. As a gx user on a drill-down page (e.g. a filtered log opened from status), I want the title to
   name the view I'm looking at, so that the title is never stale.
7. As a gx user, I want no bottom tab bar and no page footer, so that the screen has less chrome and
   one loud thing at most.

Attention items

8. As a gx user, I want to see `agents N` with `N` in red when sessions wait on me, so that I notice
   from any view that an agent needs me.
9. As a gx user, I want the agents count to disappear when nothing is Actionable, so that the title
   row is silent when nothing needs me.
10. As a gx user on the agents tab, I want the Actionable count to still show in the title row, so
    that it behaves the same on every view.
11. As a gx user, I want the server state as a single colored icon when the server is healthy, so
    that it reads as "server OK" without words.
12. As a gx user, I want words next to the server icon when the server is down, read-only or has
    herdr unavailable, so that I can't miss an unhealthy server and know what is wrong.
13. As a gx user, I want no server icon before the first probe has answered, so that gx never shows
    a guess.
14. As a gx user without nerd fonts, I want plain-glyph fallbacks for the server icon, so that the
    state still reads correctly.
15. As a gx user, I want `? help` at the far right, so that I always know how to discover keys.

Narrow terminals

16. As a gx user on a narrow terminal, I want the title row to shorten in a fixed order (`? help` to
    `?`, shorter server words, then the server words drop, then `gx ›`, then `▾`, then the view name
    is cut with `…`), so that the most important items stay visible.
17. As a gx user on a narrow terminal, I want the agents count and the server icon to never drop, so
    that I never miss something that needs me.
18. As a gx user, I want the title row to follow only the terminal width, not the page layout, so
    that it behaves the same on stacked and side-by-side layouts.
19. As a gx user on a 40-column terminal, I want the whole healthy title row to fit, so that gx stays
    usable in a small split.
20. As a gx user on a very narrow terminal, I want the row to be cut at the edge rather than show an
    error screen, so that gx never refuses to draw.

View switcher

21. As a gx user, I want to press `ctrl+o` to open a list of all views, so that I can move between
    views without remembering numbers or chords.
22. As a gx user, I want to click the title to open the same list, so that the mouse works too.
23. As a gx user on a narrow terminal, I want a click anywhere on the left group as drawn (even a cut
    view name) to open the switcher, so that the click target never vanishes.
24. As a gx user, I want to type to filter the list with fuzzy matching, so that `tk` finds tickets.
25. As a gx user, I want matches ranked and the matched letters highlighted, so that I see why an item
    matched and the best match is first.
26. As a gx user, I want views grouped under `git` and `agents` headers, so that the list is easy to
    scan.
27. As a gx user, I want the current view marked in the list, so that I know where I am.
28. As a gx user, I want the agents item to show its Actionable count in red, so that the switcher
    carries the same attention signal as the title row.
29. As a gx user, I want `ctrl+n`/`down` and `ctrl+p`/`up` to move, `enter` to go and `esc` to close,
    so that the switcher works like every other gx menu.
30. As a gx user, I want `enter` in the switcher to behave exactly like pressing the view's number
    key, so that the page keeps its state and reloads the same way.
31. As a gx user, I want to click an item in the switcher to go there, and to click outside it to
    close it, so that the mouse works inside it too.
32. As a gx user, I want digits to filter (not switch tabs) while the switcher is open, so that typing
    never jumps away by surprise.
33. As a gx user, I want the page behind the switcher to ignore my keys and clicks while it is open,
    so that nothing happens behind it.
34. As a gx user, I want background updates (agents, server, polling) to keep running while the
    switcher is open, so that nothing is stale when I close it.
35. As a gx user with a page modal open, I want `ctrl+o` or a title click to tell me to close the
    modal first, so that it behaves like the number keys.
36. As a gx user typing in a filter or search input, I want `ctrl+o` to open the switcher, so that it
    works from anywhere a terminal is not focused.
37. As a gx user in the agents terminal, I want `ctrl+o` to go to the agent, so that Claude Code's own
    `ctrl+o` keeps working.
38. As a gx user in the agents terminal, I want `ctrl+a ctrl+o` to open the switcher, so that I can
    still reach it without leaving the terminal.
39. As a gx user, I want `ctrl+a tab` and `ctrl+a 1`-`8` to keep working in the agents terminal, so
    that my existing ways out are unchanged.
39a. As a gx user in the agents terminal, I want `ctrl+q` to focus the sidebar in one key, so that I
    can leave the terminal without a leader chord.
39b. As a gx user with the sidebar hidden, I want `ctrl+q` to show it and focus it, so that the key
    always lands me in the sidebar.
39c. As a gx user, I want to rebind the focus-sidebar key in config, so that I can free `ctrl+q` if an
    agent needs it.
40. As a gx user, I want number keys `1`-`8` and `g` chords (`gw`, `gl`, `gs`, `g,`, `g.` …) to keep
    working, so that my muscle memory still works.
41. As a gx user, I want `ctrl+o` listed in the help dialog, `ctrl+a ctrl+o` in the agents leader
    which-key box, and `ctrl+q` in the agents help, so that I can discover them.

Help dialog

42. As a gx user, I want the help dialog to show the server state, pid and detail, so that the
    details I lost from the bar are one key away.
43. As a gx user, I want the help dialog to show which split app gx uses (herdr / tmux / kitty) on
    every page, so that I can check it without a footer.
44. As a gx user, I want clicking `? help` in the title row to open help, even while a text input or
    the agents terminal has focus, so that the visible hint is always a button.

Status page

45. As a gx user in the status diff view, I want to see `wrap` with its icon in the panel accent only
    when wrap is on, so that I see non-default settings and nothing else.
46. As a gx user, I want no render-mode label (unified / side-by-side), so that the header stays quiet
    when the diff itself already shows the mode.
47. As a gx user, I want `VISUAL` as a filled accent chip in the focused diff panel header, so that
    visual mode is obvious.
48. As a gx user, I want the diff context shown as `±3` instead of `Context: 3`, so that the header is
    shorter; the help dialog explains it.
49. As a gx user, I want the diff nav mode (`hunk` / `line`) to stay dim in the focused diff panel
    header, so that I still know how `j`/`k` moves.
50. As a gx user, I want only the focused diff panel to show these options, so that two panels don't
    repeat the same thing.
51. As a gx user, I want the focused panel's accent to show focus, so that I don't need the old
    `filetree` / `diff:…` footer label.

Worktrees page

52. As a gx user, I want a dim spinner and label on the right of the `Worktrees` header while a job
    runs, so that I see it is busy without a footer.
53. As a gx user in paste mode, I want a filled `PASTE` chip and a dim `3 files from feat-x` on the
    right of the `Worktrees` header, so that I know I'm pasting and what.
54. As a gx user entering paste mode, I want one notification `p paste · esc cancel`, so that I learn
    the keys once without permanent hints on the page.
55. As a gx user, I want a paste-mode section in the help dialog, so that I can look the keys up later.
56. As a gx user on a narrow terminal, I want the right-side text to shorten with `…` first, then the
    `Worktrees` title, and the spinner and `PASTE` chip to never drop, so that the state stays visible.
56a. As a gx user pasting while a job runs, I want paste mode to win the header, as today, so that the
     header shows one state at a time.

Layout

57. As a gx user on the agents tab, I want the sidebar's last row and the terminal's last row to be
    visible, so that nothing is hidden under chrome any more.
58. As a gx user on the agents tab, I want the terminal cursor to sit where the agent draws it, so
    that typing looks right after the content moved down two rows.
59. As a gx user, I want every click, wheel and hover in a page to hit what I point at, so that moving
    the page down two rows breaks nothing.
60. As a gx user viewing an image diff (kitty), I want the images placed in the panel, not two rows
    too high, so that image diffs still line up.
61. As a gx user, I want notifications to start below the blank row, so that they never cover the
    title row.
62. As a gx user, I want the switcher, notifications and chord hints to never overlap in a way that
    hides the switcher, so that the menu I opened is always readable.

## Implementation Decisions

### App shell (`ui/app`)

- **Height contract.** The app sends every page `max(1, H-2)` rows (title row + blank row). `View()`
  normalizes the page to `W × (H-2)` and prepends the title row and the blank row. The tab-injection
  path (`injectTabsIntoFooter`, `tabsView`) is removed.
- **Title-row renderer.** One function in the app builds the title row for a given width from: the
  active view's label, the Actionable count (`ActionableCount()` on the view model), the server
  connection and the nerd-font setting. It returns the rendered line and the column span of the left
  group (and of `? help`). The click hit-test uses this same function, so drawing and clicking can't
  drift (the rule the `tabSpecs` comment states today). `tabHitAt` is replaced by a title hit-test on
  `y == 0`.
- **Single source of view order and labels.** `tabSpecs()` stays the one list of views, labels and
  order. The title, the switcher and `g ,` / `g .` all read it. The agents label no longer carries the
  count; the count is a separate attention item.
- **Current view name.** The title shows the label of the active page's tab (the top of the history
  stack when a drill-down page is open, else the live tab).
- **Styles.** Both chrome rows sit on the deep background tint (`ColorDeepBg`, the mockup's crust)
  across the full width. `gx` and `▾` use `ColorOverlay`, `›` uses `ColorSurface1`, the view name is
  `ColorText` bold. In `agents N`, `agents` is `ColorOverlay` and `N` is `ColorRed` bold. In `? help`,
  `?` is `ColorSubtle` bold and `help` is `ColorOverlay`. Styles are named package-level variables.
- **Drop order (width only).** From widest to narrowest, each step applies only when the row doesn't
  fit:
  1. `? help` → `?`
  2. Unhealthy server words shorten: `down`, `read-only`, `no herdr`.
  3. Server words drop; the colored icon stays.
  4. `gx ›` drops.
  5. `▾` drops.
  6. The view name is cut with `…`.
  7. Below that the row is cut at the right edge.

  The agents count and the server icon never drop. There is no minimum width and no "too small"
  screen. At least one space separates left and right groups.
- **Server indicator.** `ServerConn.indicator()` is replaced by a renderer that returns an icon, a
  color and two word forms (full and short) per state:

  | State | Icon color | Full words | Short words |
  |---|---|---|---|
  | Unknown (not probed) | nothing drawn | | |
  | Up | green | none | none |
  | Down | red | `server down` | `down` |
  | Read-only (API mismatch) | yellow | `read-only — restart` | `read-only` |
  | Herdr unavailable | yellow | `herdr unavailable` | `no herdr` |

  The icons are new semantic roles in `ui.Icons(...)` (one per state, a server glyph with nerd fonts;
  `●`, `○`, `◐`, `⚠` without). No raw glyphs in `ui/app`. Icon codepoints are added with a script, per
  the design-system rule for PUA glyphs.
- **Mouse translation.** Every `tea.MouseMsg` (click, release, motion, wheel) with `Y >= 2` reaches
  the page with `Y -= 2`. Events with `Y < 2` stay with the app: a left click on the left group opens
  the switcher; a left click on `? help` opens help (see below); anything else on those rows does
  nothing. No page hit-test code changes.
- **Cursor.** `View()` adds 2 to `child.Cursor.Y` (only agents sets a cursor today).
- **Notifications.** The notify stack is drawn with a top margin of 2, below the blank row.
- **Overlay order in `View()`.** Page, then chord hints, then notify history, then notifications,
  then the switcher last, so the switcher is never covered.
- **`? help` click.** The app sends a new navigation message, `nav.OpenHelp()` (next to
  `nav.Switch`), to the active page. Every page handles it by opening its help, the same way its `?`
  handler does, whatever has focus: a focused text input or the agents terminal does not swallow it,
  and nothing is typed. A synthetic `?` key is not used, since it would type `?` into an input or
  send it to the agent. If the page already shows a modal, the click does nothing.

### View switcher

- **Shared fuzzy match.** The agents scorer and highlighter move from `ui/agents` to `ui` as
  `ui.FuzzyScore(query, text) (score, positions, ok)` and `ui.HighlightMatches(text, positions)`.
  Agents calls switch to them; their tests move with them. `ui.FuzzyMatch` (unranked) stays for
  existing menus.
- **New shared picker in `ui/components`** (generic part of the agents Where picker; not an extension
  of `MenuState`, whose flat unranked order ~10 callers rely on):
  - Item: label, value, group, badge, current flag.
  - Ranked grouped filter: with a query, items ranked by score within a group and groups ordered by
    their best score; with an empty query, the given order.
  - Rows with group headers; the cursor indexes matches, not rows; header-aware scrolling.
  - `UpdatePicker(msg, state)` with the same return contract as `UpdateMenu`
    (`state, decided, accepted, handled`) and the same keys: `ctrl+n`/`down`, `ctrl+p`/`up`, `enter`,
    `esc`, `backspace`, printable characters filter.
  - Render: cursor row via `ui.RenderRowHighlight`, labels via `ui.HighlightMatches`, muted group
    headers, badge in red after the label, the current item marked with a dim `current` word.
  - A row hit-test that maps a body line to a match, for the mouse.
  - Hint via `ui.RenderInlineBindings`, reusing the menu's bindings.
  - The agents Where picker may migrate onto it later; not required here.
- **The switcher itself lives in `ui/app`.** It builds items from `tabSpecs()`: group `git`
  (worktrees, log, status, stash, prs) then group `agents` (tickets, queue, agents). It sets `current`
  from the active tab and the agents badge from the Actionable count. Frame: `ui.RenderModalFrame` with
  `TitleInBorder: true` and the title `go to`; the first body line is the query (`›` prompt).
- **Placement.** A drop-down anchored under the title: column 2, row 2 (the first panel-header row).
  Width 34 columns, or the terminal width minus 4 if smaller. Height is the full list, or the rows
  available above the bottom edge (it scrolls).
- **App-level overlay, on the notify-history pattern.** While open: key presses go to `UpdatePicker`;
  every mouse message is swallowed by the app (left click inside a row switches, click outside the
  frame closes); all other messages fall through to the normal path, so polling and streams keep
  running. `enter` closes and calls the same `switchTab` the number keys use. `esc` closes.
  `ctrl+o` while open closes it. Digits filter. An empty result shows a dim `no matches` row and
  `enter` does nothing.
- **Opening with `ctrl+o`.** Caught in the app's key branch before `handleShellChordKey`, when:
  - the active page does not report a focused terminal: a new optional page interface
    `TerminalFocused() bool`, implemented by the agents page as terminal focused **or** leader
    pending, so the `ctrl+o` in `ctrl+a ctrl+o` reaches the leader. Copy mode does not count (it is
    gx's own mode, with no `ctrl+o` binding), so `ctrl+o` opens the switcher from copy mode.
    (`InputFocused()` is too broad: it is also true in text inputs, where `ctrl+o` is free);
  - no page chord is pending;
  - the same `ModalOpen()` guard as `switchTab` passes; otherwise it shows `close the modal first`.
  Opening clears a pending `g` prefix.
- **Agents tab modal guard.** The agents page implements `ModalOpen()`, true while its jump, create,
  confirm, menu or help popup is open, so `ctrl+o` and a title click can't open the switcher over it.
- **Opening from the agents terminal.** A new leader action `ctrl+o` emits a new navigation message
  (`nav.OpenSwitcher()`, next to `nav.Switch`). The app handles it like `ctrl+o` (same modal guard).
  The binding is added to the agents leader bindings, so the which-key box and help show it.
- **Focus-sidebar key.** A new agents config field, `focus-sidebar-key`, read like `leader-key` (one
  key or a list), default `ctrl+q`. While the terminal is focused, the key is matched before it is
  sent to the session, right after the leader check, so it never reaches the agent. It sets focus to
  the sidebar and un-hides the sidebar if it was hidden. In the sidebar it does nothing. Pressed right
  after the leader, it is sent to the session, like a doubled leader, so an agent can still get
  `ctrl+q`. It is listed in the agents help and in the terminal's own hint text where `ctrl+a tab`
  appears today.
- **Key binding.** A new app binding `ctrl+o` "switch view" in the `App` category, so it shows in
  every page's help next to the number keys and `g` chords.

### Help dialog

- The help dialog shows an "About" section at the top on every page: gx version (the bottom-right
  border label stays), server state in words, server pid, and the split app (herdr / tmux / kitty).
- The app is the only owner of server state, so the help package gets this section from a provider
  the app registers once at startup (a function returning the current lines). Pages keep building
  their help as today and need no new wiring.
- The status help gets a line explaining `±N` (diff context lines). The worktrees help gets a
  paste-mode section (`p` paste, `esc` cancel).

### Pages

- **All pages:** drop their own `-1` for the footer and their `? help` footer row
  (worktrees, log list panel, commit detail, status, stash, prs, tickets, queue).
- **Status:** the footer (`helpLine` and helpers) is removed. The six copies of `m.height - 1` fold
  into `mainContentHeight()`, which becomes `m.height`. The focused diff panel header shows, on its
  right side, in this order: the `VISUAL` chip (filled, panel accent background) when visual mode is
  on; `wrap` with the wrap icon in the panel accent (orange for Unstaged, green for Staged) when wrap
  is on; then the existing dim nav mode (`hunk` / `line`), `±N` context and scroll %. No render-mode
  indicator. No focus label. Without nerd fonts the wrap icon drops and only the word shows.
- **Worktrees:** `helpLineCount` becomes 0 and `statusBarView` goes away. The `Worktrees` panel header
  shows on its right side, dim: the spinner and label while busy. In paste mode it shows the filled
  accent `PASTE` chip (same style as `VISUAL`) and a dim `N files from <worktree>`. Paste mode wins, as
  today: while pasting, a running job's spinner and label are hidden. Entering paste mode
  emits one `notify.Info("p paste · esc cancel")` built from the bindings. On narrow widths the
  right-side text (busy label, or `N files from <worktree>`) truncates with `…` first, then the
  `Worktrees` title; the spinner and the chip never drop. Job results are reported as
  today.
- **Agents:** PTY rows go from `m.height-2` to `m.height-1` (terminal and new-session paths); the
  sidebar body stays `m.height-1`. PTY resize follows through the existing attach-size sync.
- **Kitty image diffs:** image placements are absolute screen cells. A single exported constant in
  `ui` for the app chrome height (2) is added to the screen origin by every image-diff host (status
  panel geometry, and the log/stash commit-detail origins), so the offset lives in one place.
- **Other overlays:** page modals are page-relative and need no change. Chord hints (bottom-right) and
  notify history (centered) keep using the full screen.

## Testing Decisions

- **Good tests here test external behavior only:** what the person sees in the rendered frame and
  which view is active after a key or click. Never assert on private render helpers, intermediate
  segments or internal span structs.
- **Seam 1, primary: `app.Model` `Update`/`View`.** Most behavior is tested here, by sending
  `tea.WindowSizeMsg`, key and mouse messages, server-conn and stream messages, then asserting on
  `View()` text (ANSI-stripped) and on the active tab. Prior art: `ui/app/model_mouse_test.go`
  (`newAppForMouseTest`, `mouseRecorderStub`), `ui/app/model_notifyhistory_test.go` (an app-level
  overlay that swallows keys/mouse and lets ticks through), `ui/app/server_conn_test.go` and
  `ui/app/server_stream_test.go` (server state and Actionable count). Cases:
  - Title row text at several widths for each server state, including the full drop order and the
    "agents count and server icon never drop" rule; no server icon before the first probe.
  - Click on the left group (also when the name is cut) opens the switcher; click on `? help` sends
    `nav.OpenHelp()` to the page, also when the stub reports a focused input; a click on row 2+ reaches a stub page with `Y` reduced by 2; wheel and motion too.
  - The page receives a `WindowSizeMsg` with `H-2`; the frame is exactly `H` lines.
  - `ctrl+o` opens; typing filters; `enter` switches; `esc` closes; digits filter while open; keys and
    clicks don't reach the page while open; a tick still does.
  - `ctrl+o` with a page reporting `TerminalFocused()` reaches the page, not the switcher; with
    `ModalOpen()` it shows `close the modal first`; `nav.OpenSwitcher()` opens it.
  - `1`-`8` and `g` chords still switch.
  - Notifications start at row 2.
  - The cursor from a stub page comes out at `Y+2`.
  Existing tests that call `tabsView`, `tabHitAt` or `injectTabsIntoFooter` are rewritten against
  `View()` and clicks.
- **Golden files for the look.** Title row at 120, 60, 40 and 30 columns (healthy, down, read-only,
  with and without Actionable, nerd and plain icons) and the switcher open (empty query, a query with
  highlights, no matches). Prior art: `ui/agents/sidebar_test.go` `assertGolden` with the `-update`
  flag and `ui/agents/testdata/*.golden`. A person reviews these goldens visually; the core principle
  is that gx must be beautiful.
- **Seam 2, existing page seams, only for page-local moves.** Each page keeps being tested through
  its own model's `Update`/`View`:
  - Status: diff panel header shows `VISUAL` chip, `wrap` only when on, `±N`, nav mode, no render
    mode, no footer; image-diff geometry includes the chrome offset. Prior art: `ui/status/e2e_test.go`
    (teatest via `testutil/teatestv2`) and existing status view tests.
  - Worktrees: header shows spinner while busy and `PASTE` + `N files from …` in paste mode; paste
    mode hides the spinner of a running job; at narrow widths the right-side text truncates before the
    title; entering paste mode emits the notification; no status bar. Prior art: `ui/worktrees/worktrees_test.go`.
  - Agents: `ctrl+q` from the terminal focuses the sidebar (and un-hides it), is not sent to the
    session, does nothing in the sidebar, and leader then `ctrl+q` sends it to the session; a
    configured `focus-sidebar-key` replaces it. Config parsing of `focus-sidebar-key` is tested next
    to `leader-key` in `config/agents_test.go`.
  - Agents: `TerminalFocused()` is true with the terminal focused or the leader pending, false in
    copy mode; `nav.OpenHelp()` opens the agents help while the terminal is focused, and nothing is
    sent to the session.
  - Every page: `nav.OpenHelp()` opens its help, also with a text input focused, and types nothing.
  - Agents: PTY gets `height-1` rows; leader `ctrl+o` returns the open-switcher message; `ModalOpen()`
    is true for each popup; goldens updated for the visible last row. Prior art:
    `ui/agents/model_test.go`, `ui/agents/terminal_test.go` goldens.
  - Help: the About section renders the provider's lines. Prior art: `ui/help/help_test.go`.
  - Pages that dropped `-1`: existing geometry tests are updated, not new ones added.
- **Seam 3, end-to-end: the real gx app under teatest.** Build the real app with `app.New` on a
  temp repo (`testutil.TempRepo`), run it with `teatest.NewTestModel` (`testutil/teatestv2`) at a
  fixed size, and wait on the rendered output. Highest-level prior art: `ui/app/queue_e2e_test.go`
  (real `app.New` + teatest + `waitForAppText`). Page-level examples: `ui/status/e2e_test.go`
  (`waitForStageText`, `waitForCurrentFrameCondition`) and `ui/log/e2e_test.go`. There is no
  cmd-level e2e harness. One main-flow test:
  - Start on the worktrees view at 120×40. Line 0 shows `gx › worktrees ▾` and `? help`, line 1 is
    blank, and the last line is page content, not a tab bar.
  - Send `ctrl+o`. The `go to` switcher appears with the `git` and `agents` groups.
  - Type `stat` (only `status` matches; `sts` would rank `stash` first). `status` is the cursor row. Send `enter`. The switcher closes and line 0
    reads `gx › status ▾`.
  - Click the title (row 0, inside the left group). The switcher opens again; `esc` closes it.
  - A page gets `H-2` rows: the status panel headers start on line 2 and the frame is exactly 40
    lines, checked on the current frame (as `waitForCurrentFrameCondition` does).
- **Fuzzy scorer:** the existing agents fuzzy tests move with `ui.FuzzyScore`/`ui.HighlightMatches`
  unchanged. The new picker component is covered through the switcher at Seam 1; it gets its own unit
  tests only if a second caller adopts it.

## Out of Scope

- Merging or removing tabs (e.g. folding stash into status, queue into tickets).
- The broader visual redesign tracked in `gx:tui-visual-redesign`.
- Migrating the agents Where picker or any `MenuState` caller onto the new picker.
- `ui/history` (a separate program with its own screen).
- Other new mappings to leave the agents terminal beyond `ctrl+q`; `ctrl+a tab` and `ctrl+a 1`-`8`
  already cover the rest.
- Changing what the server reports or how it is probed.

## Further Notes

- Produced from the `gx:title-bar` map: option T from the mockups
  (https://claude.ai/artifact/JybsizB72gEDbTuzZpkQT7), the diff-options prototype
  (https://claude.ai/artifact/AQGAJBJGABRQTq6Lw7sVAZ), two grillings (narrow terminals; worktrees
  busy and paste-mode status) and two research notes: `docs/research/view-switcher-reuse.md` and
  `docs/research/layout-height-without-footer.md`.
- Side effects that are wanted: the agents sidebar's last row and terminal's last row become visible;
  the stash footer stops being cut off.
- Widths behind the drop order: `gx › worktrees ▾` is 16 columns (widest left group); the healthy
  right side is about 15-19; the read-only label is the longest at about 21. Everything fits at 40
  columns healthy, about 50 read-only.
- The design-system spec lives at `docs/design/design-system-spec.md`, and `.ai/design-system.md`
  is the operational guide. `GLOSSARY.md`'s **Server indicator** entry ("the Queue tab's label") is stale and should be updated
  to describe the title-row icon when this lands.

### Assumptions not decided on the map

- `ctrl+q` details: it un-hides a hidden sidebar, does nothing when the sidebar already has focus,
  and leader then `ctrl+q` sends it to the session. The config field is named `focus-sidebar-key`.
- The agents attention item reads `agents N` (word dim, number red), as in the option T mockup, and is
  hidden when `N` is 0.
- Both chrome rows use the deep background tint (`ColorDeepBg`) across the full width, as in the
  mockup; the exact title-row colors are taken from the mockup.
- Server word forms: full `server down` / `read-only — restart` / `herdr unavailable`; short `down` /
  `read-only` / `no herdr`. Colors: up green, down red, read-only and herdr-unavailable yellow. The
  nerd-font glyphs are left to the implementer (one server glyph family, color carries the state);
  plain fallbacks reuse today's `●` `○` `◐` `⚠`.
- Switcher look follows the T2 mockup: frame title `go to`, `›` query prompt, groups `git` and
  `agents`, current item marked with a dim `current` word (not a new icon role, as the research
  suggested), red count after `agents`.
- Switcher placement: a drop-down at column 2, row 2, 34 columns wide (or width-4), drawn on top of
  notifications. Not centered.
- `ctrl+o` while the switcher is open closes it. Empty result shows `no matches`; `enter` does
  nothing then.
- `ctrl+o` works while a text input (filter/search) is focused, but not while a page chord is pending
  or a page modal is open (`close the modal first`).
- The agents page implements `ModalOpen()` for its popups (jump, create, confirm, menu, help).
- Clicking `? help` sends `nav.OpenHelp()` to the active page (decided in review). Clicking the server icon or the
  agents count does nothing.
- On a drill-down page the title shows the label of the tab that page belongs to.
- The help dialog's About section is fed by a provider the app registers once (package-level hook in
  `ui/help`), not by threading server state into every page. It lists version, server state, pid and
  split app; it does not add a server build version unless the handshake already reports one.
- The kitty image offset is one exported chrome-height constant in `ui`, read by each image-diff host.
- The worktrees paste-mode notification is `notify.Info`, built from the key bindings.
