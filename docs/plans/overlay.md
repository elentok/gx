# Overlay Modal Plan

## Goal

Render confirm/error/logs/yank modals as overlays on top of the worktrees view, so the table and sidebar remain visible in the background.

## How it works today

`View()` has early-returns for `modeConfirm`, `modeYank`, `modeError`, and `modeLogs`. Each modal's `*ModalView()` method calls `lipgloss.Place(m.width, m.height, ...)` to center the box on a **blank canvas** — the background is never rendered.

## Approach

### 1. New file: `ui/worktrees/overlay.go`

A single exported function:

```go
func placeOverlay(bg, fg string, x, y int) string
```

- Splits both strings into lines on `"\n"`
- For each fg line at row `i`, targets bg row `y+i`
- Computes the fg line's visual width with `ansi.StringWidth`
- Builds the composited line:
  - **left**: `ansi.Truncate(bgLine, x, "")`, space-padded to exactly `x` columns
  - **middle**: the fg line verbatim
  - **right**: `ansi.TruncateLeft(bgLine, x+fgLineWidth, "")` (bg content after the modal's right edge)
- Both `ansi.Truncate` and `ansi.TruncateLeft` are ANSI-aware, so border colors and styles on the background are fully preserved

### 2. `model_view.go`

- Extract `normalView()` from the existing `View()` body (table + sidebar + status bar)
- Remove the four early-returns for modal modes
- When in a modal mode, call `normalView()` for the background, then overlay the modal:

```go
func (m Model) View() string {
    bg := m.normalView()
    switch m.mode {
    case modeConfirm:
        return placeOverlay(bg, m.confirmModalContent(), ...)
    case modeError:
        return placeOverlay(bg, m.errorModalContent(), ...)
    case modeLogs:
        return placeOverlay(bg, m.logsModalContent(), ...)
    case modeYank:
        return placeOverlay(bg, m.yankModalContent(), ...)
    }
    return bg
}
```

The `(m.width-modalW)/2`, `(m.height-modalH)/2` position is computed using `lipgloss.Width`/`lipgloss.Height` on the rendered modal string.

### 3. Modal view methods

Each `*ModalView()` currently calls `lipgloss.Place(m.width, m.height, ...)` which positions the box on a blank canvas. Split into:

- `*ModalContent() string` — returns just the styled box (no placement)
- `*ModalView() string` — calls `placeOverlay(m.normalView(), m.modalContent(), x, y)` (or just inline in `View()`)

Since `View()` will handle placement, the individual methods only need to return the box.

## Files changed

| File | Change |
|------|--------|
| `ui/worktrees/overlay.go` | **New** — `placeOverlay(bg, fg string, x, y int) string` |
| `ui/worktrees/model_view.go` | Extract `normalView()`, route modal modes through `placeOverlay` |
| `ui/worktrees/model_confirm_modal.go` | `confirmModalView()` returns just the box |
| `ui/worktrees/model_error_modal.go` | `errorModalView()` returns just the box |
| `ui/worktrees/model_logs_modal.go` | `logsModalView()` returns just the box |
| `ui/worktrees/yank.go` | `yankModalView()` returns just the box |

## Out of scope

- `modeNew`, `modeRename`, `modeClone`, `modeSearch` — these render in the status bar, not as full-screen overlays, and don't need to change.
- `ui/confirm/confirm.go` — used only for the CLI commands (`gx push`, `gx bump`), not the TUI.
