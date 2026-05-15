# Vim-Style ctrl+d/ctrl+u Plan

## Goal

Consistent vim-style ctrl+d/ctrl+u across commit, status, and log views.
Both scroll AND cursor move together by `DefaultScroll` lines, so the cursor
stays at the same screen position. No-op when already at the boundary.

From vim docs: "The cursor is moved the same number of lines down in the file
(if possible)."

## Design decisions

- **Scroll amount**: `const DefaultScroll = 7` in `ui/list/list.go` (single place to change)
- **Diff**: co-scroll — find nearest hunk/line by display position (`activeDisplay + delta`),
  nearest by distance (not ceil), unified mode is scroll-only (no cursor move)
- **Worktrees**: leave alone — uses `charm.land/bubbles/v2/table` which handles it natively
- **Header viewport** (commit): use `DefaultScroll` instead of `visibleH/2`
- **Naming**: `ScrollPage(delta int)` for vim co-scroll everywhere; replaces the old
  snap-based `ScrollPage(direction int)` in diffview

---

## Phase 1 — `ui/list/list.go`

- Add `const DefaultScroll = 7`
- Add `ScrollPage(delta, total, visibleH int)`:
  - No-op if already at boundary in direction of delta
  - Move `selected` by delta (clamped to [0, total-1])
  - Move `scrollOffset` by delta (clamped to [0, max(0, total-visibleH)])
  - Comment: vim-style ctrl+d/u — cursor and viewport move together
- Add tests in `ui/list/list_test.go`

- [x] Add `DefaultScroll` const and `ScrollPage` to `ui/list/list.go`
- [x] Write tests

---

## Phase 2 — `ui/filetree/model.go`

- Add `ScrollPage(delta int)` delegating to `m.list.ScrollPage(delta, len(m.entries), m.visibleH)`

- [x] Add `ScrollPage` to `ui/filetree/model.go`

---

## Phase 3 — `ui/diffview/model.go`

Replace snap-based `ScrollPage(direction int)` with vim co-scroll `ScrollPage(delta int)`:

1. Scroll viewport by `delta` display lines
2. Call `coScrollActive(delta)`:
   - NavModeHunk: find hunk with `HunkDisplayRange[i][0]` nearest to `activeDisplay + delta`
   - NavModeLine: find changed line with `ChangedDisplay[i]` nearest to `activeDisplay + delta`
   - Unified mode: no-op (HunkDisplayRange/ChangedDisplay are nil)
3. Keep `snapActiveToViewport()` unchanged (still used by `ScrollViewport` for mouse)

- [x] Rewrite `ScrollPage` in `ui/diffview/model.go`
- [x] Add `coScrollActive`, `nearestHunk`, `nearestChangedLine` helpers
- [x] Write tests in `ui/diffview/model_test.go`

---

## Phase 4 — Fix `SetVisibleHeight` in resize paths

`m.visibleH` is currently only set by mouse handlers. Keyboard `ScrollPage` and
`Navigate` need it for correct `EnsureSelectionVisible` clamping.

- `ui/commit/model_diff.go` or `syncDiffViewport`: call
  `m.fileTreeModel.SetVisibleHeight(innerFilesH)` after computing layout
- `ui/status/model_update.go` in `handleWindowSize`: call
  `m.fileTreeModel.SetVisibleHeight(innerFiletreeH)` after computing layout

- [x] Fix commit resize path
- [x] Fix status resize path

---

## Phase 5 — Wire ctrl+d/u in all views

### commit

- `model_header.go`: `scrollHeaderPage` uses `list.DefaultScroll` instead of `visibleH/2`
- `model_diff.go`: `scrollDiffPage` passes `direction * list.DefaultScroll` to `diffModel.ScrollPage`
- `sidebar.go`: add `scrollSidebarPage(direction int)` — calls `fileTreeModel.ScrollPage(direction * list.DefaultScroll)`, then `refreshDiff()` if selection changed
- `model_keys.go`: `bindingPageDown/Up` when filetree focused (neither `focusHeader` nor `focusDiff`) → call `scrollSidebarPage(direction)`

- [x] Update `ui/commit/model_header.go`
- [x] Update `ui/commit/model_diff.go`
- [x] Add `scrollSidebarPage` to `ui/commit/sidebar.go`
- [x] Update `ui/commit/model_keys.go`

### status

- `diffarea/model.go`: change `ScrollPage(direction int)` → `ScrollPage(delta int)`
- `diff_keys.go`: pass `list.DefaultScroll` / `-list.DefaultScroll`
- `status_page_nav.go`: replace `scrollFiletreePage(direction)` body with
  `fileTreeModel.ScrollPage(direction * list.DefaultScroll)` + reload diff if changed

- [x] Update `ui/status/diffarea/model.go`
- [x] Update `ui/status/diff_keys.go`
- [x] Update `ui/status/status_page_nav.go`

### log

- Add `bindingPageDown`/`bindingPageUp` bindings in log key manager
- `model_keys.go`: handle them with `m.list.ScrollPage(±list.DefaultScroll, len(m.rows), visibleH)`

- [x] Add bindings to `ui/log/model_keys.go`
- [x] Handle in `dispatchBinding`

---

## Completion checklist

- [x] All phases pass `go test ./...`
- [x] Manual: ctrl+d/u in commit diff — cursor and viewport move together
- [x] Manual: ctrl+d/u in commit filetree — file selection scrolls, diff reloads
- [x] Manual: ctrl+d/u in status diff — cursor and viewport move together
- [x] Manual: ctrl+d/u in status filetree — selection scrolls, diff reloads
- [x] Manual: ctrl+d/u in log — selection scrolls
- [x] Manual: ctrl+d/u in commit header — scrolls by 7 lines
