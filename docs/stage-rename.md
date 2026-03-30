# Stage Rename Support Plan

## Goal

Add first-class rename/move support to `gx stage` so moved files are easy to spot and understand in both the status tree and diff panes.

## Status/Icon Mapping

### Sidebar entry mapping

| Semantic status | Git signal (porcelain) | Nerd icon | Fallback | Suggested color |
| --- | --- | --- | --- | --- |
| New file | `??`, `A` | `` | `N` | green |
| Modified file | `M` (and default changed) | `` | `M` | amber/orange |
| Deleted file | `D` | `` | `D` | dim gray (faint) |
| Renamed/moved file | `R` | `󰁔` | `R` | cyan/blue |
| Copied file (optional) | `C` | `` | `C` | cyan |
| Conflict/unmerged (future) | `U*`, `AA`, `DD`, etc. | `` | `!` | red |

### Directory icon mapping (unchanged)

| Entry | Nerd icon | Fallback |
| --- | --- | --- |
| Collapsed dir | `` | `▸` |
| Expanded dir | `` | `▾` |

### Staged/unstaged meta markers (unchanged)

| Meta state | Nerd | Fallback |
| --- | --- | --- |
| Staged only | `` | `✓` |
| Staged + unstaged | `` | `+` |

## Sidebar Label Format

For renamed/moved files, render display text as:

`old/path.ext -> new/path.ext`

Rules:

- Keep node placement in tree by **new path** (destination).
- Keep full source and destination in the label when width allows.
- For narrow widths, truncate both sides intelligently (preserve filename tails).

## Diff View Marking

Add a rename context header above hunks:

- `renamed: old/path.ext -> new/path.ext`
- optional: `similarity: <n>%` if available from git output.

If a rename has no content change, render a small rename-only diff body instead of blank content.

## Data Model / Parsing Changes

1. Extend status parsing to capture rename source path from porcelain v1 `-z` records.
2. Extend stage status entry model to store rename metadata:
   - `RenameFrom string`
   - `RenameTo string` (or reuse `Path` as destination)
3. Keep current path-based tree logic centered on destination path.
4. Update helper functions (`statusEntryColor`, icon chooser, display formatter) to detect and render rename state.

## UX Behavior

- Selecting a renamed file should open diff for the destination path (existing behavior path-based).
- Stage/unstage file-level toggle still applies to that renamed entry.
- Hunk/line stage behavior remains unchanged unless git diff shape for rename-only entries requires special handling.

## Rollout Steps

1. Add rename metadata in git status layer.
2. Add rename icon/color/label rendering in status tree.
3. Add rename context line in diff pane.
4. Handle rename-only diffs gracefully.
5. Add tests (unit + E2E).

## Test Plan

### Unit tests

- Status parsing captures `R` entries with source+dest paths.
- Sidebar rendering uses rename icon and `old -> new` label.
- Color selection for moved files is distinct from modified/deleted.

### E2E tests (`ui/stage/e2e_test.go`)

- Rename tracked file and verify sidebar shows moved marker + source/dest label.
- Verify diff pane includes rename context line.
- Stage/unstage renamed file from status pane and assert git state updates correctly.

## Notes

- Start with `R` (rename) support; `C` (copy) can be folded in later with the same pattern.
- Conflict styling is listed for future consistency and does not block rename support.
