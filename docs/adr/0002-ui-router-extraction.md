# ADR 0002 — Extract nav state machine as `ui/navstate`

## Status
Completed

## Context

Routing logic (tab switching, history push/pop, page activation) lived inside `app.Model` and
`app.routerState`. Two parallel systems tracked tab context:

- `routerState.tabs` — stored `routerTabState` (context fields only: `TabID`, `WorktreeRoot`,
  `Ref`, `InitialPath`). Equivalent in shape to `ViewContext`.
- `model.lastViewStateByTab` — stored full `ViewState` per tab (context + options).

A bridging function `tabViewStateForViewContext` reconciled the two systems. It was temporary
scaffolding, not a permanent abstraction.

## Decision

Extract the nav state machine into `ui/navstate` as a standalone package owning the full navigation
lifecycle: history stack and tab memory (full `ViewState` per tab). `app.Model` becomes a thin
coordinator that forwards tea messages to the active page and renders its output.

The module was named `navstate` rather than `router` to reflect what it actually is — a state
machine for navigation state — without pre-committing to a broader "router" abstraction.

## Outcome

- `routerState.tabs` and `model.lastViewStateByTab` merged into a single source of truth in
  `navstate.State.lastViewStateByTab`.
- `routerTabState` replaced by `nav.ViewContext` (same fields, canonical name).
- `ui/nav` holds the shared message types and constructors; it is imported by all pages.
- `ui/navstate` holds the state machine; it is imported only by `app`.
- Navigation bugs (history corruption, wrong view activated) have one home in `navstate`.
- `navstate` is testable without rendering anything.
