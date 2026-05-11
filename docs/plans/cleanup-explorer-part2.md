# Cleanup Explorer Part 2: Status-First Nested-Model Stabilization

## Summary
Stabilize `ui/status` as the reference architecture before migrating `ui/commit`.
Use existing `search.Model` (no new search engine), but move ownership/wiring into status-owned child boundaries.
End-state target: remove `ui/explorer` entirely if possible; if any shared helpers remain, rename/split into explicit pure packages (no ambiguous `explorer` namespace).

## Implementation Changes

### Wave 1: Clarify ownership and remove ambiguous naming in `ui/status`
- [x] Rename `ui/status/explorer_*` files to domain-specific names (`diff_*`, `section_*`, `yank_*`, etc.).
- [x] Remove alias indirection (`explorer_aliases.go`), define status-local enums/types for focus/section/nav/render.
- [x] Preserve runtime behavior; this wave is structural clarity + dependency visibility only.
- [x] Keep temporary calls into current `ui/explorer` helpers where needed.

### Wave 2: Extract status-owned nested child boundaries (without changing UX)
- [x] Replace temporary wrapper boundaries with a real status-owned `diffArea` state container.
- [x] Keep page focus on the parent status model; `diffArea` owns active section, staged/unstaged diff panes, fullscreen state, flash state, and shared diff controls.
- [x] Move shared diff controls into `ui/diffview.Model`: `SetRenderMode(RenderModeUnified|RenderModeSideBySide)`, `SetNavMode(NavModeHunk|NavModeLine)`, and `EnableWrap(bool)`.
- [x] Keep current `search.Model`, but make diff-search wiring status-owned (query/match/cursor/nav integration owned by status diff code, not a shared explorer namespace).
- [x] Keep parent `Model` as orchestration shell (modals, actions, page routing, chord handling, top-level key dispatch).
- [x] Shared code usage must be pure/helper-only; no shared mutable state structs across pages.

### Wave 3: Remove or rename `ui/explorer` with explicit package semantics
- [x] Attempt full removal by inlining feature-specific logic into `ui/status` and `ui/commit`.
- [x] If shared helpers are still valuable, split/rename into explicit packages by concern (examples: diff navigation helpers, sidebar row render helpers, yank formatting helpers).
- [x] Ban reintroduction of a catch-all `explorer` package name.

### Wave 3a: Detach `ui/diffview` from `ui/explorer`
- [x] Move `explorer.SectionData` and its builders/reflow helpers into `ui/diffview`; rename it to `DiffBuffer`.
- [x] Treat `DiffBuffer` as one pane's parsed/rendered diff buffer plus cursor/visual-selection state, not generic data.
- [x] Move diff navigation/search/yank helpers that operate on the buffer into `ui/diffview` or narrow subpackages under it.
- [x] Remove `diffview.Model.ExplorerNavMode`; `ui/diffview` must not expose conversion helpers back to `ui/explorer`.
- [x] Update `ui/status` and `ui/commit` to use `diffview.DiffBuffer`, `diffview.NavMode`, and `diffview.RenderMode` directly.
- [x] Remove the temporary aliases in `ui/status/diff_types.go` (`navMode`, `diffRenderMode`, `navHunk`, `navLine`, `renderUnified`, `renderSideBySide`) once status call sites use `diffview` names directly.
- [x] Leave sidebar/filetree helpers out of `ui/diffview`; split them separately only if they remain shared.

### Wave 4: Apply stabilized pattern to `ui/commit`
- [x] Mirror status architecture in commit:
  - [x] commit-owned diff interaction boundary,
  - [x] commit-owned diff-search wiring boundary (still using existing `search.Model`),
  - [x] commit-owned filetree/sidebar boundary.
- [x] Reuse only explicit pure shared packages chosen in Wave 3.
- [x] No alias shims or ambiguous cross-feature ownership.

## Public/Internal Interface Changes
- Internal refactor only; no intentional keymap/UX changes during Wave 1/2.
- Package-level moves/renames expected in Wave 3; keep temporary compatibility wrappers only if needed to land incrementally.
- [x] `ui/explorer` is removed and replaced by explicitly named status/commit-local code plus pure shared packages.

## Test Plan
- [x] Wave 1: full `ui/status` unit + e2e parity run.
- [x] Wave 1: compile checks for removed alias paths/types.
- [x] Wave 2: focused tests for diff/search ownership boundaries and parent orchestration routing.
- [x] Wave 2: preserve existing regressions: section stability, staging/unstaging behavior, flash behavior, footer behavior, mouse wheel routing.
- [x] Wave 3: run `ui/status` + `ui/commit` + shared helper package tests to confirm no hidden coupling.
- [x] Wave 4: full `ui/commit` and `ui/status` suites to validate architecture parity and no behavior regressions.

## Assumptions
- Existing user-facing behavior remains stable during status stabilization unless fixing known bugs.
- Existing `search.Model` remains the only search engine/model; we are only changing ownership/wiring boundaries.
- Preferred end state is no `ui/explorer`; fallback is explicit, narrow, pure shared packages with clear names.
