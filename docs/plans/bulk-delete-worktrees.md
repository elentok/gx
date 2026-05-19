# Bulk Delete Worktrees

## Design decisions

- Parallel deletes, capped at 3 concurrent
- When any worktrees are selected, `d` acts on the selected set; otherwise acts on cursor item
- Icon: reuse `Icons.Check` (no new `IconSet` role needed)
- UI term: "selected"; code field: `selectedWorktrees map[string]bool`
- Confirm modal: extend `RenderConfirmModal` with optional items list (additive, backward-compatible)
- Summary: reuse `modeLogs` (populate `lastJobLog` / `lastJobLabel`)
- Single vs bulk decided at completion by `len(deleteQueue) == 1`
- `deleteResultMsg` always carries `name` (fix existing gap)
- Selected set always cleared on post-deletion refresh

## Tasks

- [x] Rename `selectedWorktree()` → `cursorWorktree()` throughout `ui/worktrees/`
- [x] Add `selectedWorktrees map[string]bool` to `Model` (keyed by worktree name)
- [x] Add `space` key binding to toggle selection on cursor row; clear all selections with `esc` when in normal mode
- [x] Render `Icons.Check` in a new leading column on selected rows in `buildRows()`
- [x] Extend `components.RenderConfirmModal` (and `UpdateConfirm` signature if needed) to accept an optional `[]string` items list rendered as a bulleted list below the prompt
- [x] Fix `cmdDelete`: always populate `name: wt.Name` in every `deleteResultMsg` return path
- [x] Add `modeDeleteProgress` to the `mode` enum
- [x] Add delete-progress state to `Model`:
  - `deleteQueue []git.Worktree` — pending worktrees
  - `deleteInFlight int` — currently running deletions (cap 3)
  - `deleteSteps []components.Step` — one per worktree in the batch
  - `deleteResults []deleteResultMsg` — accumulated results
- [x] Implement `enterDeleteProgress(worktrees []git.Worktree) (Model, tea.Cmd)`:
  - sets `modeDeleteProgress`, initialises steps and queue
  - dispatches up to 3 `cmdDelete` calls via `tea.Batch`
- [x] Update `enterDeleteConfirm`: use `selectedWorktrees` set when non-empty, otherwise cursor item; call the extended confirm modal with the worktree name list
- [x] Handle `deleteResultMsg` in `modeDeleteProgress`:
  - mark the matching step done or failed
  - decrement `deleteInFlight`, dispatch next queued delete if any
  - when all done, call completion handler
- [x] Implement completion handler:
  - always refresh worktree list and clear `selectedWorktrees`
  - if `len(batch) == 1`: success → `notify.Info("Deleted <name>")`, failure → `modeLogs` with error output
  - if `len(batch) > 1`: populate `lastJobLog` with success/failure summary, switch to `modeLogs`
- [x] Render `modeDeleteProgress` modal: `RenderSteps` inside `ui.RenderModalFrame`, overlaid with `ui.OverlayCenter`
- [x] Update key hints / help to show `space` = select, and `d` hint to reflect bulk capability
