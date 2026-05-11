# Normalize Chord Keys & Keymap Help Across Views

## Goal

Consistent chord and help UX across all views (worktrees, status, log, commit):

1. Statusbars show only `? help` — no inline keymaps
2. `?` opens a keymaps overlay in every view
3. Starting a chord shows a top-right corner overlay instead of inline statusbar text
4. `esc` aborts any in-progress chord

## Commit View: One Help Overlay with Sections

**Recommendation: one overlay, three labeled sections** (Header, Files, Diff).  
Simpler: single `?` handler, single modal, no logic to track which pane is focused. User sees the full picture.

---

## Current State

| View      | Statusbar             | `?` help | Chord hint         |
|-----------|-----------------------|----------|--------------------|
| Worktrees | `? help` only ✓       | ✓        | inline statusMsg   |
| Status    | context + `? help` ✓  | ✓        | inline statusMsg   |
| Log       | `/ search · q back · L lazygit log` ✗ | ✗ | inline statusMsg |
| Commit    | `j/k move...` (left) + `gw worktrees...` (right) ✗ | ✗ | inline statusMsg |

App-level `g` chord (tab switching): app model intercepts `g` first — children never see it in tabs mode, no hint shown currently.

---

## New Shared Components

### `ui/chord_overlay.go` (new file)

```go
func RenderChordOverlay(prefix string, bindings []key.Binding, useNerdFont bool) string
```

Renders the styled top-right box:

```
╭ g ─────────────────────────────╮
│ g ➜ top                        │
│ w ➜ worktrees                  │
│ l ➜ log                        │
│ s ➜ status                     │
│               esc  close       │
╰────────────────────────────────╯
```

Uses lipgloss rounded border and existing color palette. Width auto-sized to content.

Also defines the shared interface:

```go
// ChordHinter is implemented by models that expose chord completion hints.
type ChordHinter interface {
    ChordHints(prefix string) []key.Binding
}
```

The app model queries the active child via this interface to build the combined overlay.

### `ui/overlay.go` — add `OverlayTopRight`

```go
func OverlayTopRight(bg, fg string, screenW int) string
// Places fg flush to top-right corner: x = screenW - fgW, y = 0
```

---

## File-by-File Changes

### `ui/log/view.go`
- `footerView()`: replace `"/ search · q back · L lazygit log"` right hint with `ui.StyleHint.Render("? help")`
- `View()`: add chord overlay when `m.keyPrefix != ""`; add help modal overlay when `m.helpOpen`

### `ui/log/model_update.go`
- Remove `m.statusMsg = ui.RenderInlineBindings(...)` for `"g"` chord start
- Add `case "?"`: call `m.enterHelpMode()`
- Add `"esc"` handling in `handleChordKey` for `[`/`]` prefixes (currently only `g` is handled)

### `ui/log/model.go`
- Add `helpOpen bool` and `helpViewport viewport.Model` fields

### `ui/log/model_help.go` (new file)
- `enterHelpMode()`, `handleHelpKey()`, `helpFullView()`, `helpModalView()` — same pattern as `ui/worktrees/model_help.go`
- `ChordHints(prefix string) []key.Binding` for `"g"`, `"["`, `"]"` prefixes

### `ui/commit/view.go`
- `footerView()`: replace both left (`"j/k move..."`) and right (`"gw worktrees..."`) with just `ui.StyleHint.Render("? help")` right-aligned
- `View()`: add chord overlay when `m.keyPrefix != ""`; add help modal overlay when `m.helpOpen`

### `ui/commit/model_update.go`
- Remove `m.setStatus("yy content · ...")` for `"y"` chord start
- Add `"esc"` in the `"g"` branch of `handleChordKey` (only `"y"` branch handles esc today)
- Add `case "?"`: call `m.enterHelpMode()`

### `ui/commit/model.go`
- Add `helpOpen bool` and `helpViewport viewport.Model` fields

### `ui/commit/model_help.go` (new file)
- Same pattern as worktrees help; `helpFullView()` renders three labeled sections: **Header**, **Files**, **Diff**
- `ChordHints(prefix string) []key.Binding` for `"g"` and `"y"` prefixes

### `ui/worktrees/model_chord_key.go`
- Remove `m.statusMsg = ui.RenderInlineBindings(...)` for `"g"` chord start (line 67)

### `ui/worktrees/model_view.go`
- `View()`: add chord overlay when `m.keyPrefix != ""`

### `ui/worktrees/keys.go`
- Add `ChordHints(prefix string) []key.Binding` method

### `ui/status/model_keys.go`
- Remove the three `m.setStatus(m.inlineHints(...))` calls for `"g"`, `"c"`, `"y"` chord starts

### `ui/status/view_main.go`
- Add chord overlay when `m.keyPrefix != ""` (before help/modal overlays, only when no modal is open)

### `ui/status/keys.go`
- Add `ChordHints(prefix string) []key.Binding` method

### `ui/app/model.go`
- `View()`: when `m.keyPrefix != ""`, build combined bindings (app-level via `m.appChordHints(prefix)` + child-level via `ChordHinter` interface), render and apply `ui.OverlayTopRight`

### `ui/app/model_tabs.go`
- Add `appChordHints(prefix string) []key.Binding` returning `g,`/`g.`/`gw`/`gl`/`gs` bindings

---

## Implementation Order

1. Add `OverlayTopRight` to `ui/overlay.go`
2. Create `ui/chord_overlay.go` (shared component + `ChordHinter` interface)
3. Fix log view: footer + help modal + chord overlay + `?` key
4. Fix commit view: footer + help modal + chord overlay + `?` key
5. Remove inline chord hints from worktrees, status
6. Add chord overlay rendering to worktrees, status `View()` methods
7. Add chord overlay to app `View()` (combined app + child hints)
8. Implement `ChordHints()` on all four child models

---

## Verification

```bash
go build ./...
go test ./ui/...
```

Manual checks:
- Each view: `?` → overlay appears; `esc`/`q` dismisses it
- Commit: overlay has three sections (Header, Files, Diff)
- Each view: `g` → top-right chord overlay; `esc` → dismissed with no action
- Status: `c` then `esc` → aborted; `y` then `esc` → aborted
- Log/commit: statusbar shows only `? help` right-aligned
- App (tabs) mode: `g` → overlay shows combined app + child hints
