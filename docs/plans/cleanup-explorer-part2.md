# Cleanup Explorer Part 2: Status-First Nested-Model Stabilization

## Summary
Stabilize `ui/status` as the reference architecture before migrating `ui/commit`.
Use existing `search.Model` (no new search engine), but move ownership/wiring into status-owned child boundaries.
End-state target: remove `ui/explorer` entirely if possible; if any shared helpers remain, rename/split into explicit pure packages (no ambiguous `explorer` namespace).

## Implementation Changes

### Wave 1: Clarify ownership and remove ambiguous naming in `ui/status`
- Rename `ui/status/explorer_*` files to domain-specific names (`diff_*`, `section_*`, `yank_*`, etc.).
- Remove alias indirection (`explorer_aliases.go`), define status-local enums/types for focus/section/nav/render.
- Preserve runtime behavior; this wave is structural clarity + dependency visibility only.
- Keep temporary calls into current `ui/explorer` helpers where needed.

### Wave 2: Extract status-owned nested child boundaries (without changing UX)
- Keep current `search.Model`, but make diff-search wiring status-owned (query/match/cursor/nav integration owned by status child boundary, not page root).
- Move diff interaction state/routing (section, nav mode, render mode, flash, viewport sync, active visibility) behind a status child boundary.
- Keep parent `Model` as orchestration shell (modals, actions, page routing, chord handling, top-level key dispatch).
- Shared code usage must be pure/helper-only; no shared mutable state structs across pages.

### Wave 3: Remove or rename `ui/explorer` with explicit package semantics
- Attempt full removal by inlining feature-specific logic into `ui/status` and `ui/commit`.
- If shared helpers are still valuable, split/rename into explicit packages by concern (examples: diff navigation helpers, sidebar row render helpers, yank formatting helpers).
- Ban reintroduction of a catch-all `explorer` package name.

### Wave 4: Apply stabilized pattern to `ui/commit`
- Mirror status architecture in commit:
  - commit-owned diff interaction boundary,
  - commit-owned diff-search wiring boundary (still using existing `search.Model`),
  - commit-owned filetree/sidebar boundary.
- Reuse only explicit pure shared packages chosen in Wave 3.
- No alias shims or ambiguous cross-feature ownership.

## Public/Internal Interface Changes
- Internal refactor only; no intentional keymap/UX changes during Wave 1/2.
- Package-level moves/renames expected in Wave 3; keep temporary compatibility wrappers only if needed to land incrementally.
- `ui/explorer` is deprecated during migration and must be fully removed or replaced by explicitly named pure packages at completion.

## Test Plan
- Wave 1:
  - full `ui/status` unit + e2e parity run.
  - compile checks for removed alias paths/types.
- Wave 2:
  - focused tests for diff/search ownership boundaries and parent orchestration routing.
  - preserve existing regressions: section stability, staging/unstaging behavior, flash behavior, footer behavior, mouse wheel routing.
- Wave 3:
  - run `ui/status` + `ui/commit` + shared helper package tests to confirm no hidden coupling.
- Wave 4:
  - full `ui/commit` and `ui/status` suites to validate architecture parity and no behavior regressions.

## Assumptions
- Existing user-facing behavior remains stable during status stabilization unless fixing known bugs.
- Existing `search.Model` remains the only search engine/model; we are only changing ownership/wiring boundaries.
- Preferred end state is no `ui/explorer`; fallback is explicit, narrow, pure shared packages with clear names.
