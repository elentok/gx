# Mouse Scroll Plan

## Goal

Mouse wheel scrolls whatever panel it hovers over (list or diff) in commit, status, and log views.
Scroll offset is independent of selection/active item; when scrolling moves the viewport past the
selection, selection snaps to the nearest visible edge.

## Snap rule (both list and diff)

- Scroll **down** past selection → new selection = **first visible item** (lowest index still in
  viewport)
- Scroll **up** past selection → new selection = **last visible item** (highest index still in
  viewport)

For diffs in NavModeHunk: "first visible hunk" = first hunk whose `displayStart >= newViewportTop`.
For diffs in NavModeLine: same logic using `ChangedDisplay[line]`.

---

## Phase 1 — `ui/list.Model`

Create `ui/list/list.go`:

```
type Model struct {
    selected     int
    scrollOffset int
}

func (m *Model) Selected() int
func (m *Model) SetSelected(i, total int)           // clamps to [0, total)
func (m *Model) Offset() int
func (m *Model) Navigate(delta, total, visibleH int) // move selection + EnsureVisible
func (m *Model) ScrollViewport(delta, total, visibleH int) // scroll + snap
func (m *Model) EnsureSelectionVisible(total, visibleH int) // adjust offset to show selection
func (m *Model) VisibleRange(total, visibleH int) (start, end int)
```

`ScrollViewport`: clamps offset to `[0, max(0, total-visibleH)]`, then if `selected < offset`
sets `selected = offset`; if `selected >= offset+visibleH` sets `selected = offset+visibleH-1`.

`Navigate`: moves `selected` by delta (clamped), then calls `EnsureSelectionVisible` (minimal
viewport shift to keep selection on screen — no centering).

`VisibleRange`: returns `(offset, min(offset+visibleH, total))`.

Tests live in `ui/list/list_test.go`.

- [x] Create `ui/list/list.go` with `Model` and all methods
- [x] Write tests for scroll+snap, navigate+ensureVisible, edge cases (empty, delta larger than total)

---

## Phase 2 — Adopt `ui/list.Model` in the three panels

### 2a — `filetree.Model[T]`

- Replace `selected int` with `list list.Model`
- Update `SelectedIndex()`, `SetSelectedIndex(i)` to delegate to `list`
- Add `ScrollViewport(delta, visibleH int)` method
- Update `Navigate` (j/k) to call `list.Navigate` instead of raw increment

- [x] Update `ui/filetree/model.go`
- [x] Update callers in `ui/commit/`

### 2b — `statusData` in `ui/status`

- Add `listState list.Model` to `statusData`
- Update `statusData.selected` reads/writes to use `listState.Selected()`
- Add `ScrollViewport` call path from mouse handler

- [x] Update `ui/status/model_state.go` and related files

### 2c — `log.Model`

- Replace `cursor int` with `list list.Model`
- Update `cursor` reads/writes to use `list.Selected()`
- Replace inline centering in `visibleLines()` with `list.VisibleRange()`

- [x] Update `ui/log/model.go`, `ui/log/view.go`, `ui/log/model_keys.go`

---

## Phase 3 — Update `sidebar.BuildVisibleRenderableRows`

Change signature to accept explicit `offset` instead of computing from selection:

```go
func BuildVisibleRenderableRows[T any](
    entries []T, selected, offset, innerH int,
    build func(int, T) RenderableRow,
) []RenderableRow
```

Remove `visibleRowsForSelection`. Pass `list.VisibleRange()` output as `offset`.

- [x] Update `ui/sidebar/sidebar.go`
- [x] Update callers: `ui/commit/view.go`, `ui/status/view_panes.go`

---

## Phase 4 — Diff viewport snap

Add to `diffview.Model`:

```go
func (m *Model) ScrollViewport(delta int)
```

Logic:
1. Call `m.viewport.ScrollDown(delta)` or `ScrollUp(-delta)`
2. Get new `yOffset = m.viewport.YOffset()`, `visibleH = m.viewport.VisibleLineCount()`
3. In NavModeHunk: check if `HunkDisplayRange[ActiveHunk]` overlaps `[yOffset, yOffset+visibleH)`.
   If not: find first (down) or last (up) hunk whose range intersects the viewport.
4. In NavModeLine: same using `ChangedDisplay[ActiveLine]`.
5. Update `ActiveHunk` or `ActiveLine`.

Replace direct `viewport.ScrollDown/Up` calls in `model_keys.go` (J/K bindings) with
`ScrollViewport` so keyboard scroll also triggers snap.

- [x] Add `ScrollViewport` to `ui/diffview/model.go`
- [x] Update `ui/commit/model_keys.go` (J/K, ctrl+d/u bindings)
- [x] Update `ui/status/diff_keys.go` (same bindings)
- [x] Tests in `ui/diffview/model_test.go`

---

## Phase 5 — Mouse handlers

### All views: enable mouse reporting

Each view's `View()` must set `v.MouseMode = tea.MouseModeCellMotion`.

- [ ] `ui/commit/view.go` — already added, verify
- [ ] `ui/status/view_main.go` — already present
- [ ] `ui/log/view.go` — add

### commit `model_mouse.go` (rewrite)

```
mouse over diff pane  → diffModel.ScrollViewport(dir * 3)
mouse over filetree   → fileTreeModel.ScrollViewport(dir, visibleH)
```

`mouseOverDiff(x, y)` stays for position detection. Add `mouseOverFiletree(x, y)` as complement.

- [ ] Rewrite `ui/commit/model_mouse.go`
- [ ] Tests

### status `model_mouse.go` (extend)

Currently only scrolls diff. Add filetree scroll:

```
mouse over diff pane  → existing ScrollDown/Up, replace with diffModel.ScrollViewport
mouse over filetree   → statusData.listState.ScrollViewport(dir, visibleH)
```

- [ ] Update `ui/status/model_mouse.go`
- [ ] Tests

### log `model_mouse.go` (new)

Log has only a list panel. Any wheel event scrolls the list:

```
any mouse wheel → list.ScrollViewport(dir * 3, total, visibleH)
```

- [ ] Create `ui/log/model_mouse.go`
- [ ] Add `case tea.MouseWheelMsg` to `ui/log/model_update.go`
- [ ] Tests

---

## Phase 6 — Clean up current broken commit mouse code

Remove the interim `model_mouse.go` written before this plan and replace it with the Phase 5
version. Remove the `focusDiff` bypass added during debugging.

- [ ] Remove/replace `ui/commit/model_mouse.go`
- [ ] Remove temporary `focusDiff` bypass

---

## Completion checklist

- [ ] All phases pass `go test ./...`
- [ ] Manual smoke: mouse scroll over diff in commit view scrolls diff, selection snaps
- [ ] Manual smoke: mouse scroll over filetree in commit view scrolls files, diff updates
- [ ] Manual smoke: mouse scroll in log scrolls commit list
- [ ] Manual smoke: mouse scroll over status filetree scrolls file list
