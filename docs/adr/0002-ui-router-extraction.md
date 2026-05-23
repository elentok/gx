# ADR 0002 — Extract `ui/router` as a standalone routing module

## Status
In progress

## Context

Routing logic (tab switching, history push/pop, page activation) lives inside `app.Model` and
`app.routerState`. Two parallel systems currently track tab context:

- `routerState.tabs` — stores `routerTabState` (context fields only: `TabID`, `WorktreeRoot`,
  `Ref`, `InitialPath`) per tab. Equivalent in shape to `ViewContext`.
- `model.lastViewStateByTab` — stores full `ViewState` per tab (context + options). The canonical
  tab memory per the ViewState PRD.

The bridging function `tabViewStateForViewContext` exists solely to reconcile these two systems: it
reads from `routerState.tabs` to fill in `model.lastViewStateByTab` when switching tabs with no
explicit context. It is temporary scaffolding, not a permanent abstraction.

`routerState` was extracted from `app.Model` in commit `097ab79` as a first step, with the intent
of eventually promoting it to a full `ui/router` module.

## Decision

Extract `ui/router` as a standalone module owning the full routing lifecycle: history stack, tab
memory (full `ViewState` per tab), and view cache. `app.Model` becomes a thin coordinator that
forwards tea messages to the active view and renders its output.

When this extraction is complete:
- `routerState.tabs` and `model.lastViewStateByTab` merge into a single source of truth inside
  `ui/router`.
- `routerTabState` is replaced by `ViewContext` (same fields, canonical name).
- `tabViewStateForViewContext` disappears — the router answers "what context is this tab at?"
  directly.

## Consequences

- Routing bugs (history corruption, wrong view activated) will have one home.
- Views stop needing to know they are cached — they implement `Activate()`/`Deactivate()`.
- `ui/router` is testable without rendering anything.
- Until the extraction is complete, the two parallel tracking systems and the bridging function
  are expected and should not be refactored in isolation.
