# Status Side-by-Side Plan

## Goal

Add a toggle in `gx status` diff view between:

- current interactive unified diff mode
- side-by-side render mode (read-only viewport mode)

using delta side-by-side rendering (`--side-by-side`) for display.

## Why This Approach

We currently rely on **raw unified diff** parsing for all interactive behaviors:

- hunk/line cursor tracking
- visual selection bounds
- stage/unstage/discard patch synthesis
- yank line-range context
- search-to-line mapping

Those behaviors depend on stable mappings between parsed raw lines and rendered display rows (`displayToRaw`, `rawToDisplay`).

Delta side-by-side output is not line-aligned with unified raw lines (wrapping, paired columns, additional formatting). If we switch rendering directly to delta side-by-side while keeping interactions active, cursor/selection/patch targeting can become inconsistent.

So we choose a safe rollout:

1. add side-by-side rendering as a read-only mode
2. keep all interactive patch operations in unified mode
3. preserve correctness now, iterate toward full interactive side-by-side later

## Scope (Phase 1)

### In scope

- Diff render mode toggle key in diff focus (proposed: `s`)
- Side-by-side rendering via delta `--side-by-side`
- Read-only side-by-side mode:
  - allow scrolling, section switching, file switching, fullscreen, refresh
  - disable hunk/line editing operations and visual selection
- Clear status/help messaging when an operation is unavailable in side-by-side mode

### Out of scope (Phase 1)

- Interactive hunk/line stage/unstage/discard/yank while in side-by-side mode
- Preserving line-level cursor fidelity in side-by-side mode
- Custom side-by-side renderer implementation

## UX Design

### Mode model

Add `diffRenderMode` to status model:

- `renderUnified` (default, current behavior)
- `renderSideBySide` (new, read-only)

### Toggle behavior

- Key `s` in diff focus toggles render mode.
- Toggling triggers diff reload/re-render for current file/section.
- Persist only for current session (no config persistence in Phase 1).

### Allowed actions in side-by-side mode

- `j/k`, `J/K`, `ctrl+u/d` scrolling
- `tab` section switch
- `,/.` previous/next file
- `f` fullscreen
- `w` soft-wrap toggle (if still meaningful)
- `r` refresh

### Disabled actions in side-by-side mode

When attempted, show status message:
`side-by-side is read-only; press s for interactive mode`

Disabled:

- `space` (stage/unstage)
- `d` (discard/unstage)
- `a` (hunk/line mode toggle)
- `v` (visual mode)
- `yy/yl/ya` line/hunk/selection context operations tied to interactive selection

`yf` (filename yank) can remain enabled since it is selection-independent.

## Delta Integration

Current flow:

- raw unified from `git diff --no-color` (source of truth)
- colorized output from delta (`colorizeWithDelta`)

Phase 1 side-by-side changes:

- extend delta colorization path with a side-by-side flag
- pass `--side-by-side` when `renderSideBySide` is active
- continue obtaining raw unified diff unchanged for model data correctness

Potential implementation shape:

- `DiffPathWithDelta(..., sideBySide bool)`
- `DiffUntrackedPath(..., color bool, sideBySide bool, ...)`
- `colorizeWithDelta(raw, opts)` where opts includes `sideBySide`

Fallback remains unchanged: if delta fails, use git color output (non-side-by-side) and continue functioning.

## Impact on Hunk/Line Handling

### Unified mode (unchanged)

- Full interactive hunk/line/visual behavior remains authoritative.
- Existing parsing and patch synthesis remain unchanged.

### Side-by-side mode (new)

- Parsed hunks/lines still exist internally (from raw unified diff), but UI does not expose editable selection semantics.
- Active selection markers and visual-range indicators are hidden or ignored.
- No stage/unstage/discard operations allowed from this mode.

This avoids mismatches between visible side-by-side rows and actionable parsed line indices.

## Files/Areas Likely Affected

- `ui/status/model_state.go` (render mode enum + field)
- `ui/status/model_keys.go` (toggle key + side-by-side guardrails)
- `ui/status/view_footer_help.go` (help text updates)
- `ui/status/model_diffstate.go` (reload using selected render mode)
- `ui/status/view_panes.go` (side-by-side render path / marker suppression)
- `git/stage.go` (delta option plumbing for side-by-side)

## Test Plan

### Unit tests (`ui/status/model_test.go`)

- Toggle `s` switches render mode in diff focus.
- `space` in side-by-side mode shows read-only status and does not alter git state.
- `d`, `a`, `v`, and selection-dependent yanks are blocked in side-by-side mode.
- Navigation and scrolling still work in side-by-side mode.
- `yf` still works in side-by-side mode.
- Toggle back to unified restores interactive operations.

### Integration/package tests

- `go test ./ui/status -count=1`
- `go test ./git -count=1` (delta argument plumbing)
- `go test ./... -count=1`

## Rollout Steps

1. Add model render-mode state + default.
2. Add key toggle and help/status messaging.
3. Thread side-by-side option through delta rendering calls.
4. Guard interactive actions when in side-by-side mode.
5. Update diff pane rendering behavior for read-only side-by-side.
6. Add tests.
7. Validate full suite.

## Future Phase (Interactive Side-by-Side)

To support interactive stage/unstage in side-by-side mode safely, implement a custom side-by-side renderer from parsed unified diff while preserving syntax coloring (for example by using delta colorized lines as style source). That would restore stable line/hunk mapping and enable full parity with unified mode.
