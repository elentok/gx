# ADR 0001 — Shared `ui/list.Model` for list panel state

## Status
Accepted

## Context

Three views (commit, status, log) each have a navigable list panel (filetree, file list, commit
list). All three used the same scroll pattern: center the viewport on the selection. There was no
independent scroll offset.

When adding mouse-wheel scroll support, two designs were considered:

**Option A — pure functions in `ui/sidebar`**  
Add `ScrollList(total, selected, offset, delta, visibleH int) (newOffset, newSelected int)` and
related helpers. Each model stores its own `scrollOffset int`. `sidebar.BuildVisibleRenderableRows`
gains an `offset` parameter.

**Option B — shared `ui/list.Model` struct**  
A small struct owning `selected` and `scrollOffset`, with `Navigate`, `ScrollViewport`, and
`EnsureSelectionVisible` methods. Each panel embeds it.

## Decision

Option B. The deciding factor was planned multi-select: tracking a selected-item set alongside the
scroll offset and cursor is a single addition to one struct rather than three parallel fields spread
across three models. Pure functions would require every caller to thread an extra argument for each
piece of multi-select state.

## Consequences

- `ui/list.Model` becomes the canonical home for list navigation state (selection, scroll offset,
  and future multi-select).
- `filetree.Model[T]`, `statusData`, and `log.Model` each embed or hold a `list.Model`.
- `sidebar.BuildVisibleRenderableRows` is updated to accept an explicit offset instead of computing
  it from selection.
- The log view's inline `visibleLines` centering logic is replaced by `list.Model.VisibleRange`.
