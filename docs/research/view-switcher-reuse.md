# View switcher: what to reuse (gx:title-bar/01)

What existing gx code the fuzzy view switcher (`gx › <view> ▾`, opened by `ctrl+o` or a click on
the title) should build on. All sources are local files at `main` (134074bf).

## Candidates

| Code | What it has | What it lacks for the switcher |
|---|---|---|
| `ui/components/modal_menu.go` `MenuState`, `UpdateMenu`, `RenderMenuList` | Shared, used by ~10 callers (`ui/bump`, `ui/push`, `ui/worktrees`, `ui/tickets`, `ui/agents/actions.go`, CLI `ui/menu`). Keys: `ctrl+n`/`down`, `ctrl+p`/`up`, `enter`, `esc`, `backspace`, printable = filter (`modal_menu.go:59-94`) | Flat list, no groups. Filter is plain `ui.FuzzyMatch` subsequence (`ui/fuzzy.go:8`), no ranking, no match highlight. `MenuItem` has only `Label/Value/Detail`: no badge, no current marker. Renders `▸` cursor, not the shared row highlight. No mouse. |
| `ui/agents/create.go` Where picker (617fa326, 64e46003) | Grouped list with group headers (`pickerRow`, `rows()` `create.go:306-320`), ranked filter: groups sorted by best score, items by score (`filter()` `create.go:214-247`), match highlight (`highlightRunes`), header-aware scrolling (`scroll()` `create.go:324-338`), `ui.RenderRowHighlight` cursor row, per-row trailing muted detail (session count, `create.go:574-578`) | Private to `ui/agents`, fused into the create form (`createForm` fields, tab order, `formEntry` with project/dir). No mouse. |
| `ui/agents/fuzzy.go` `fuzzyScore`, `highlightRunes` | Scored multi-term fuzzy match (word-boundary and consecutive bonuses, gap penalty) returning matched rune positions; renderer using `ui.StyleFilterMatch` | Private to `ui/agents`. |
| `ui/agents/jump.go` jump popup | Same key contract as the menu, plus `tab` to toggle order; scrolling (`jumpOffset`); `RenderModalFrame` with `TitleInBorder` and a `RightTitle` count (`jump.go:130-140`) | Domain-specific rows (sessions); flat; uses unranked `ui.FuzzyMatch`. |
| `ui/notifyhistory` | The only **app-level** modal: `app.Model` owns it, intercepts keys and swallows mouse while open, lets every other msg fall through (`ui/app/model_update.go:24-36`), draws it last in `View()` (`ui/app/model.go:155-157`) | Not a picker. It is the wiring precedent. |
| `ui/confirm` `UpdateMouse` | Click hit-testing of a centered overlay: translate screen coords by `ui.OverlayCenterOrigin`, then hit-test the local frame (`ui/confirm/confirm.go:131-141`) | Precedent for mouse only. |
| `ui/filter`, `ui/search` | Text-input filter/search with `textinput` | Page-level narrowing (ADR 0011); heavier than needed. |

## Recommendation

Build on the **Where picker's model + `ui/agents/fuzzy.go`**, extracted into a new shared component.
Do not extend `MenuState`: its `Visible()` order and plain filter are relied on by ~10 callers, and
adding ranking or groups there changes all of them.

### What moves to `ui/` and `ui/components/`

Per `.ai/design-system.md` ("Add it there first", "Do not start by implementing it locally"):

1. `ui/agents/fuzzy.go` → `ui/fuzzy.go`, exported next to `FuzzyMatch`:
   `ui.FuzzyScore(query, text) (score int, pos []int, ok bool)` and
   `ui.HighlightMatches(text string, pos []int) string`. Agents calls switch to them; tests
   (`ui/agents/fuzzy_test.go`) move with them.
2. New `ui/components/picker.go` (name open, e.g. `PickerState`), the generic part of the Where
   picker:
   - `PickerItem{Label, Value, Group string; Badge string; Current bool}`.
   - Ranked grouped filter (port of `createForm.filter`): with a query, items ranked by score inside
     a group and groups by their best score; with an empty query, the given order (git, then agents).
   - `Rows()` with group headers; cursor indexes matches, not rows (as `createForm.sel`).
   - Header-aware `scroll(n)` (port of `createForm.scroll`).
   - `UpdatePicker(msg, state)` with the exact `UpdateMenu` contract and return tuple
     `(state, decided, accepted, handled)`, so the two components feel the same.
   - `RenderPickerList(state, width)` using `ui.RenderRowHighlight` for the cursor row,
     `ui.HighlightMatches` for labels, a muted group header style, the badge right-aligned,
     and a current-view marker from a new `ui.Icons(...)` role (not a raw glyph).
   - `HitRow(localY) (item int, ok bool)` for mouse, mapping a body line to a match.
   - A hint from `ui.RenderInlineBindings` reusing the menu's bindings.
   The Where picker can migrate onto it later (follow-up, not required for this epic).
3. The switcher itself stays in `ui/app` (it only knows views and the Actionable count):
   `ui/app/switcher.go` builds items from `tabSpecs()` (the single source of tab order/labels,
   `ui/app/model_tabs.go:322-336`), sets `Current` from `navState.ActiveTab()`, and sets the agents
   `Badge` from `m.stream.vm.ActionableCount()` (`model_tabs.go:341`, `viewmodel/viewmodel.go:67`).
   Frame: `ui.RenderModalFrame` with `TitleInBorder: true`, like `jump.go`.

## Key handling as an app-level overlay

Follow the `notifyHistory` pattern in `app.Model.Update`:

- **Open.** In the key branch of `Update`, before `handleShellChordKey`, catch `ctrl+o` when the
  active page is not passing raw keys to a terminal. Today `InputFocused()` is the only signal and
  the shell-chord path skips entirely when it is true (`model_update.go:113-123`). `ctrl+o` is free
  in text inputs, so gate on a narrower signal: a new optional page interface (e.g.
  `TerminalFocused() bool`) that `ui/agents` implements from `m.terminalFocused`. Apply the same
  `ModalOpen()` guard as `switchTab` / `openNotifyHistory` ("close the modal first").
- **Open from the agents terminal (`ctrl+a ctrl+o`).** Add a `"ctrl+o"` case to
  `runLeaderAction` (`ui/agents/leader.go:102`) that returns a new nav message (e.g.
  `nav.OpenSwitcher()` next to `nav.Switch` in `ui/nav/nav.go`); `app.Model.Update` handles it like
  `nav.IsSwitch`. Leader `1-8` already does the same with `nav.Switch` (`leader.go:142-144`). Add
  the binding to `leaderBindings` so help and the which-key box show it.
- **While open.** First thing after the notify bookkeeping (next to the `notifyHistory` block):
  key presses go to `components.UpdatePicker`; `ctrl+n`/`down`, `ctrl+p`/`up` move, typing filters,
  `backspace` edits, `esc` closes, `enter` closes and calls `m.switchTab(nav.ViewState{Tab: id})`
  (the same path as the number keys, so live-page reuse, reload gate and history clearing stay
  identical). `ctrl+o` again could close it (toggle). Non-key, non-mouse msgs fall through so page
  polling keeps running. Digits filter instead of switching tabs while open.
- **Chord overlay.** `m.keyPrefix` handling stays as is; opening the switcher should clear a
  pending `g` prefix.

## Mouse handling

- Mouse reaches `app.Model` only when the page's view enables it: `View()` copies
  `child.MouseMode` (`ui/app/model.go:165`). Every top-level page uses `ui.NewMainView`, which sets
  `MouseModeCellMotion` (`ui/view.go:8-13`), so this holds today.
- **Title click.** Replace `tabHitAt` (bottom row, `model_tabs.go:352-370`) with a `titleHitAt`
  for `y == 0` over the `gx › <view> ▾` span, computed from the same render helper so they cannot
  drift (the `tabSpecs` comment states that rule). Hit → open the switcher.
- **Inside the switcher.** Swallow every `tea.MouseMsg` while open (as `notifyHistory` does) so the
  page under it never acts. On a left click: translate to the overlay's local frame (like
  `confirm.UpdateMouse` with `ui.OverlayCenterOrigin`, or the top-left origin if it drops down under
  the title), subtract the frame's border/query lines, and call `HitRow`; a hit on an item switches,
  a click outside the frame closes. Wheel can move the cursor, but with 8 items it is optional.

## Risks

- **`ctrl+o` stolen from the agent.** If the app catches `ctrl+o` before routing, the agents
  terminal loses it (Claude Code uses it). Needs the terminal-focus signal above; `InputFocused()`
  is too broad (it is also true in the create form, jump popup, copy mode).
- **Agents tab has no `ModalOpen()`.** Its popups (jump, create, confirm, menu) are only guarded via
  `InputFocused()`. If `ctrl+o` is caught outside the `InputFocused` gate, it would open the
  switcher over an agents popup. Either have agents implement `ModalOpen()` or treat any agents
  popup as blocking.
- **Ordering in `Update`.** The switcher block must sit before the server/store/stream handlers'
  early returns only for keys and mouse; putting it after them is fine for keys, but it must come
  before `tabHitAt`/title click and before `handleShellChordKey`.
- **Two filters in the codebase.** `ui.FuzzyMatch` (unranked) and `ui.FuzzyScore` (ranked) will
  coexist; menus keep the old one until a deliberate migration.
- **Notifications overlap.** Notifications draw at the top right with a 1-row margin
  (`model.go:158-160`); the map already decides to move them down one row. A drop-down under the
  title must also be drawn below the notify stack or the stack must hide while it is open.
- **Narrow terminals.** Frame width and the badge column need a minimum width; still open in the
  map ("Not yet specified").
