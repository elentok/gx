# Filetree Keybindings Unification Plan

## Summary
Unify filetree behavior across `ui/status` and `ui/commit` by moving filetree semantics into `ui/filetree` and consuming structured update results instead of synthetic key events or message round-trips.

Goals:
- Keep `h/left`, `l/right`, `enter`, `j/k` behavior consistent across pages.
- Reduce duplicated key-routing logic in parent pages.
- Make child effects explicit via `Result` (similar to `search.Result`).

## Scope
- In scope:
  - `ui/filetree`
  - `ui/status` filetree integration
  - `ui/commit` filetree integration
  - regression tests for collapse/expand parity
- Out of scope:
  - unrelated diff interactions
  - app-level routing/chords unrelated to filetree semantics

## Design
### 1) Replace filetree event messages with a result model
- Add `filetree.Result` with fields such as:
  - `Handled`
  - `SelectionChanged`
  - `RebuildRequested`
  - `OpenSelected`
- Keep `tea.Cmd` return for search input updates only.
- Parents handle filetree effects synchronously from `Result`.

### 2) Make filetree operations explicit APIs
- Add explicit methods on `filetree.Model` for directory/selection operations:
  - `ToggleSelectedDir`
  - `CollapseSelectedDir`
  - `ExpandSelectedDir`
  - `FocusParent`
- Avoid synthetic key events in parent pages.

### 3) Parent page responsibilities
- Parent owns:
  - pane focus transitions
  - page-level actions (`reload`, routing, modals, diff enter)
- Filetree child owns:
  - tree navigation/toggle/expand/collapse/open semantics

## Work Plan
### Wave 1: `ui/filetree` result contract
- [x] Introduce `filetree.Result`.
- [x] Update `filetree.Model.Update` to return `Result` instead of child-specific messages.
- [x] Update `ui/filetree` tests for the new contract.

### Wave 2: `ui/status` migration
- [x] Consume `filetree.Result` in `status` focused-child flow.
- [x] Remove `filetree.RebuildRequestedMsg` / `filetree.OpenSelectedMsg` handling from status runtime.
- [x] Add regression tests for:
  - [x] `h` collapses selected directory.
  - [x] `enter` collapses selected directory.
  - [x] `l/right` expands selected directory.
- [x] Fix status filetree routing so dir rows delegate to filetree behavior.

### Wave 3: `ui/commit` cleanup and parity
- [x] Remove synthetic key-event calls into filetree (`Update(KeyPressMsg{...})`) for tree operations.
- [x] Use explicit filetree methods (`ToggleSelectedDir`, `CollapseSelectedDir`, `ExpandSelectedDir`, `FocusParent`).
- [x] Keep commit regression tests for `h` and `enter` directory collapse.
- [x] Add commit regression test for `l/right` expanding collapsed directory.

### Wave 4: follow-up hardening
- [x] Add a tiny shared parity test helper (optional) for status/commit directory toggle expectations.
- [x] Re-audit parent key-routing to ensure file rows still open diff while dir rows expand/collapse.

## Validation
- Required:
  - `go test ./ui/filetree/...`
  - `go test ./ui/status/...`
  - `go test ./ui/commit/...`
- Extended:
  - `go test ./ui/log/... ./ui/worktrees/...` to catch unrelated regressions from key-routing edits.

