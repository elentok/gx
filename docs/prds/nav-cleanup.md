# PRD: Navigation Layer Cleanup

## Problem Statement

The `ui/nav`, `ui/navstate`, and `ui/app` packages accumulated naming residue and structural
anti-patterns from successive refactors. Field names reference old terminology (`router`, `stack`,
`route`), the `Transition` type carries information that `app.Model` already knows from the nav
message it received, `handleShellChordKey` mutates the model in place instead of returning a new
model, and file names like `model_route.go` no longer match the concept they implement.

## Solution

A focused cleanup pass that: renames stale identifiers to match current concepts, removes the
`Transition` intermediary so `navstate` methods return only what they uniquely provide, refactors
`handleShellChordKey` to follow the Bubble Tea immutable-update contract, and renames files to
match the vocabulary established by `nav.ViewState`.

## User Stories

1. As a contributor reading `app.Model`, I want field names that match the current vocabulary, so
   that I don't need prior context to understand what `router` or `stack` refers to.
2. As a contributor reading `navstate`, I want method signatures that return the minimum needed,
   so that I can understand what each method uniquely provides without reading the `Transition`
   dispatch table.
3. As a contributor reading `app.Model.Update`, I want each nav message handled directly without a
   dispatch layer, so that the coordination logic is obvious at the call site.
4. As a contributor reading `handleShellChordKey`, I want it to return a new model instead of
   mutating via a pointer receiver, so that it follows the same contract as the rest of the update
   path.
5. As a contributor browsing `ui/log`, `ui/commit`, `ui/status`, `ui/worktrees`, I want files named
   `model_viewstate.go` instead of `model_route.go`, so that the name matches the type
   (`ViewState`) those files implement.
6. As a contributor reading `navstate.go`, I want `initMissingTabs` instead of `ensureTabs`, so
   that the function name communicates what it does (initialize absent map entries) rather than
   leaving "ensure" ambiguous.

## Implementation Decisions

### Renames — identifiers

- `navstate.State.ensureTabs` → `initMissingTabs`
- `app.Model.router` → `navState` (field storing a `navstate.State`)
- `app.Model.stack` → `history` (the deep-navigation history slice of `historyEntry`)
- `navstate.State.stack` → `history` (the parallel `[]nav.ViewState` in the state machine)

### Renames — files

- `ui/log/model_route.go` → `ui/log/model_viewstate.go`
- `ui/commit/model_route.go` → `ui/commit/model_viewstate.go`
- `ui/status/model_route.go` → `ui/status/model_viewstate.go`
- `ui/worktrees/model_route.go` → `ui/worktrees/model_viewstate.go`
- `ui/nav/route_changed.go` → `ui/nav/viewstate_changed.go`

### Remove `Transition` type from `navstate`

The `Transition` struct carries `Kind`, `ActiveTab`, `ViewState`, `PrevViewState`, and
`PoppedEntry`. All of this except `ViewState` (the resolved state after applying tab memory) is
either redundant with the nav message type or available via `navstate.State` accessors after the
call.

New method signatures:

```go
func (s *State) Switch(vs nav.ViewState) nav.ViewState
func (s *State) Open(vs nav.ViewState) nav.ViewState
func (s *State) Back() (active nav.ViewState, quit bool)
func (s *State) ApplyViewStateChanged(vs nav.ViewState) nav.ViewState
```

`app.Model.Update` captures the prev active state before calling `Switch`, and reads
`s.ActiveTab()` / `s.Active()` after any call — no intermediary struct needed.

`applyTransition` is removed. Each nav message branch in `Update` handles coordination directly
(or via small focused helpers per message type, e.g. `applySwitch`, `applyOpen`, `applyBack`).

### `handleShellChordKey` — immutable update contract

Change signature from:
```go
func (m *Model) handleShellChordKey(msg tea.KeyPressMsg) (bool, tea.Cmd)
```
to:
```go
func (m Model) handleShellChordKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool)
```

The caller in `Update` already patterns-matches on the bool to decide whether to return early; the
return order `(Model, tea.Cmd, bool)` puts the model first, consistent with `applySwitch`,
`switchTab`, and the rest of the update path.

## Testing Decisions

Good tests verify observable state after an operation, not implementation details of the transition
mechanism. They should not import or reference `Transition`, `TransitionKind`, or any removed type.

### `ui/navstate` tests

After removing `Transition`, existing tests are rewritten to assert on:
- `s.ActiveTab()` — which tab is active after the operation
- `s.Active()` — the resolved `ViewState` at the top of the history
- The `(nav.ViewState, bool)` return of `Back()` for the quit case

Prior art: `ui/navstate/navstate_test.go` (existing tests, to be updated in place).

### `ui/app` tests

Existing tests in `ui/app/model_test.go` are updated to reference `m.navState` instead of
`m.router`, and `m.history` instead of `m.stack`. No new test scenarios are needed — the
refactor is purely structural.

## Out of Scope

- Merging `ui/nav` and `ui/navstate` into a single package (kept separate by design — `nav` is the
  lightweight message bus imported by all pages; `navstate` is the state machine imported only by
  `app`).
- Renaming the `Tab` field in `nav.ViewState` / `nav.ViewContext` (kept as-is; `vs.Tab` is
  unambiguous in context).
- Any behaviour changes to routing, tab memory, or page lifecycle hooks.
- The eventual `ui/router` promotion described in ADR 0002 (now updated to reflect the actual
  outcome; this PRD closes the remaining naming debt without pursuing that larger extraction).

## Further Notes

ADR 0002 has been updated to reflect that the extraction landed as `ui/navstate` rather than the
originally proposed `ui/router`. This PRD does not reopen that decision.

All changes are mechanical renames or structural refactors with no user-visible behaviour change.
The test suite should pass without modification to test scenarios — only identifier references
change.
