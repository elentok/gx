# Cleanup Roadmap: status / commit / explorer / diff

## PR1: Extract shared explorer diff pane renderer (lowest risk, biggest dedupe)

- Add `ui/explorer/render_diffpane.go` with a host-configurable renderer:
  - Input: `SectionData`, `viewport.Model`, nav mode, focus flags, marker/search callbacks, color/style config, width/height
  - Output: rendered `[]string` (panel body rows), plus optional right-title metadata (e.g. scroll %)
- Migrate:
  - `ui/status/explorer_view.go` to call shared renderer for row loop
  - `ui/commit/view.go` diff pane row loop to same renderer
- Keep host-specific concerns outside renderer (panel frame title, binary-summary wording, staged/unstaged accent selection)
- Tests:
  - Add table tests in `ui/explorer` for marker precedence and search highlighting
  - Keep existing status/commit view tests passing unchanged

## PR2: Introduce reusable search controller + align behavior

- Add `ui/explorer/search_controller.go`:
  - Manages query, mode, cursor, next/prev
  - Exposes hooks for `list scope` and `diff scope`
  - Uses existing `ComputeDiffSearchMatches` / `ApplyDiffSearchMatch`
- Migrate:
  - `ui/status/explorer_search.go` to thin adapter layer
  - `ui/commit/model_search.go` to same controller
- Decide one shared behavior for `enter/esc` in search (currently inconsistent), document it, and apply in both
- Tests:
  - Shared controller tests for lifecycle and cursor semantics
  - Keep host tests focused on scope mapping only

## PR3: Extract yank action helpers

- Add `ui/explorer/yank_action.go` (or similar):
  - Build location/content/all-context payloads from `SectionData`, nav mode, selected file path, and focus state
  - Return typed result (`text`, `status message`, error code) to avoid duplicated branching
- Migrate:
  - `ui/status/explorer_yank.go`
  - `ui/commit/model_yank.go`
- Leave clipboard write in host layer for testability and side-effect ownership
- Tests:
  - Shared unit tests for payload creation across hunk/line/visual selections
  - Keep one lightweight host test per screen for wiring

## PR4: Split large update loops into nested reducers

- Status:
  - Extract `updateOverlays`, `updateSearch`, `updateDiffExplorer`, `updateStatusPage`, `updateActionsRunner`
- Commit:
  - Extract `updateHelp`, `updateSearch`, `updateHeader`, `updateSidebar`, `updateDiff`
- Top-level `Update` becomes router + ordered reducer chain
- Preserve message ordering semantics exactly (especially modal precedence)
- Tests:
  - Add reducer-order tests for “modal open blocks base key handling”
  - Keep e2e tests as regression net

## PR5: Refine package boundaries in `ui/diff`

- Create subpackages:
  - `ui/diff/core`: parse AST + patch builders + raw mappings
  - `ui/diff/render`: row kinds, wrap/padding, symlink labels/summary formatting
- Move functions without behavior changes; keep compatibility shims in `ui/diff` for one release if needed
- Tests:
  - Move existing tests with minimal rewrite
  - Add a package-boundary lint/check (optional) to prevent re-mixing

## Execution order

1. PR1
2. PR2
3. PR3
4. PR4
5. PR5

## Why this order

- PR1–PR3 remove duplication with minimal behavioral risk and immediately reduce maintenance cost
- PR4 then simplifies model architecture once shared primitives exist
- PR5 is mostly packaging/ownership cleanup after call sites are stabilized

## Success criteria

- No behavior regressions in existing `status`/`commit` interaction tests
- Reduced duplicated LOC in search/yank/diff rendering paths
- New features in explorer diff behavior can be added in one place and observed in both screens
