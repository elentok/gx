# ADR 0009 — Split-view tabs orchestrate a list panel + a detail panel

## Status
Accepted

## Context

Two tabs are split views (per CONTEXT.md): **log** and **stash**. Both pair an interactive list
panel with an interactive commit **detail panel** (`commit.Model`) inside `ui/splitview`. Despite
sharing the splitview interface, they were structured oppositely:

- **log** — the page `Model` *is* the list. It owns the rows and renders them inline in its own
  `view.go`, and feeds splitview a stub `logListAdapter` (four no-op methods carrying only
  `SelectedRef`).
- **stash** — the page type was `Tab`, which delegated to a separate inner `Model` that is a real
  `splitview.ListPanel` (owns entries, `list.Model`, navigation, rendering, loading).

This produced two inconsistencies:

1. **Entry-point name.** Every other registered tab exposes `Model` (`statusui.NewModel`,
   `logui.NewModel`, `worktrees.NewWithSettings`). Stash alone exposed `Tab` via
   `stashlistui.NewTab`.
2. **Internal shape.** log flattens its list into the page; stash factors it out. The divergence was
   complexity-driven — log's list rendering is entangled with page-level features (search-match
   highlighting, filter, the flash animation, the pseudo-log-line, ref decorations), so it was never
   cheaply separable; stash's list is trivial and factored out for free.

The codebase's documented grain (ADR 0001) is that page `Model`s **compose reusable panel widgets**:
`ui/list.Model` is the canonical list-navigation widget; `filetree.Model[T]` and `diffview.Model`
are shared across commit and status. `commit.Model` is already the shared, reusable detail panel
embedded by both log and stash.

Three options were considered:

- **A — rename only.** `Tab` → `Model`; leave log flat. Fixes the entry-point name, leaves the
  internal-shape divergence.
- **B — flatten stash to match log.** Fold stash's panel into the page + a stub adapter. Achieves
  uniformity by degrading stash to log's legacy (weaker) shape.
- **C — factored uniformity.** Both split-view tabs become `Model { listPanel, detail commit.Model,
  split }`, each owning its own list-panel sub-model (composing `list.Model`). stash already matches
  this after the rename; log is raised up to it by extracting its list into a sub-model. A shared
  generic list widget across the two tabs (C1) was rejected as premature generalization — the lists
  render different item types and log carries cross-cutting features the generic would have to
  absorb; per-tab list sub-models of identical *shape* (C2) give the uniformity without the
  over-reach.

## Decision

**Option C2.** Split-view tabs are structured as a page `Model` orchestrating two sub-models: a
list-panel sub-model (unexported `listPanel`, composing `ui/list.Model`) and the shared
`commit.Model` detail panel, joined by `ui/splitview`.

- stash: rename `Tab` → `Model` and inner `Model` → `listPanel`. (After this, stash conforms.)
- log: extract its list into a `listPanel` sub-model. The page retains cross-cutting concerns
  (search, filter, flash, pseudo-log-line, decorations, commit operations) and feeds the panel
  filtered rows + render hints, the same way `commit.Model` feeds `filetree.Model`.

We raise log up to the factored shape rather than flattening stash down, because the factored shape
is the better design (independently testable panels, a clean boundary) and matches the documented
composition grain.

### Scope: split-view tabs only

This applies to **log and stash only**. **Worktrees is deliberately excluded.** Worktrees is a
different archetype — a list (`table.Model`) paired with a **sidebar**, not a detail panel. Per
CONTEXT.md, a *detail panel* is interactive and focusable (the user navigates into it and back); a
*sidebar* is a passive, read-only projection of the selection (rendered by a pure function,
`renderSidebarContent`). Worktrees does not use splitview and has no focus-into-detail, orientation,
or fullscreen semantics. Forcing it into this orchestration would impose split-view machinery it has
no use for. Structure follows essence.

(Worktrees' list uses `table.Model` rather than `list.Model`; that is a separate, content-justified
divergence — the list has real columns — and is not changed here.)

## Consequences

- Both split-view tabs expose `Model` as the app entry point, constructed via `NewModel`.
- Each split-view tab owns an unexported `listPanel` sub-model that satisfies `splitview.ListPanel`
  and is independently testable.
- `logListAdapter` (the stub) is removed; log's list rendering and navigation move into its
  `listPanel`.
- The detail side is unchanged — `commit.Model` remains the shared detail panel for both tabs.
- Future split-view tabs follow this shape. Non-split list+sidebar tabs (worktrees) do not.
