# Split-view tab C2 refactor

## Problem Statement

The two split-view tabs — **log** and **stash** — are built on the same `ui/splitview` container but
are structured in opposite, inconsistent ways:

- **stash**'s entry point is `Tab` (constructed via `NewTab`), with a separate inner `Model` for the
  list panel. Every *other* tab's entry point is `Model` (via `NewModel`), so stash is the lone
  outlier at the package boundary.
- **log**'s page `Model` *is* the list — it owns the rows and renders them inline, and feeds
  splitview a stub `logListAdapter` (four no-op methods). stash, by contrast, factors its list out
  into a real `splitview.ListPanel`.

A developer reading the code sees "stash factored, log flat, stash named differently" and cannot
tell whether the divergence is intentional. The inconsistent entry-point name and the split/flat
structural mismatch make the split-view tabs harder to learn, navigate, and extend.

## Solution

Normalize both split-view tabs onto a single shape (ADR 0009, "C2"):

```
Model { listPanel, detail commit.Model, split splitview.Model }
```

Each split-view tab exposes `Model` as its app entry point (constructed via `NewModel`) and
orchestrates two sub-models: an unexported `listPanel` (the list panel, composing `ui/list.Model`)
and the shared `commit.Model` detail panel, joined by `ui/splitview`.

- **stash** is brought to this shape by renaming: `Tab` → `Model`, inner `Model` → `listPanel`,
  `NewTab` → `NewModel`, plus a file reshuffle to the `model_*` convention.
- **log** is *raised up* to this shape (not stash flattened down) by extracting its list into a real
  `listPanel`. The page keeps all cross-cutting concerns and feeds the panel filtered rows plus
  render hints; the `logListAdapter` stub is deleted.

From a user's perspective the application behaves identically — this is an internal structural
normalization. The "user" here is the developer working in this codebase.

## User Stories

1. As a developer, I want every tab's app entry point to be `Model` constructed via `NewModel`, so
   that I can find and construct any tab without remembering per-tab naming exceptions.
2. As a developer, I want the stash tab's top-level type renamed from `Tab` to `Model`, so that it
   matches log, status, and worktrees.
3. As a developer, I want the stash list panel renamed from `Model` to an unexported `listPanel`, so
   that the two stash types are no longer confusingly both called `Model`.
4. As a developer, I want stash's files reshuffled to the `model_*` convention (`model.go`,
   `model_keys.go`, `model_ops.go`, `list_panel.go`), so that file layout matches the other tabs.
5. As a developer, I want the app shell's stash construction call site updated to `NewModel`, so that
   the app compiles against the renamed entry point.
6. As a developer, I want the app shell's stash type assertion in tests updated to `Model`, so that
   existing app tests keep passing.
7. As a developer, I want log's list extracted into an unexported `listPanel` sub-model, so that
   log's list is an independently testable component like stash's.
8. As a developer, I want log's `listPanel` to own `rows`, the `list.Model`, list navigation, and row
   rendering, so that all list concerns live in one cohesive module.
9. As a developer, I want log's page `Model` to retain search, filter, the flash lifecycle, the
   pseudo-log-line source data, ref decorations, and all commit operations, so that cross-cutting
   page concerns stay at the page level.
10. As a developer, I want log's page to feed the `listPanel` filtered/visible rows plus render hints
    (search matches, flash subject/until, pseudo-log-line, decorations), so that the panel renders
    correctly without reaching into page state.
11. As a developer, I want the `logListAdapter` stub deleted once the real `listPanel` exists, so
    that there is no dead indirection left behind.
12. As a developer, I want both `listPanel` types to satisfy `splitview.ListPanel` (real
    `SelectedRef`, `View`, sizing), so that splitview is fed real panels in both tabs.
13. As a developer, I want `listPanel` to remain unexported in both packages, so that the panel is an
    internal implementation detail accessed only through the splitview interface.
14. As a developer, I want the stash list panel's existing behavior tests retargeted to the renamed
    type, so that coverage is preserved through the rename.
15. As a developer, I want the stash page `Model`'s key-manager, help, and split-routing behavior
    covered by tests, so that the orchestration is guarded.
16. As a developer, I want log's `listPanel` covered by isolated tests for navigation, `SelectedRef`,
    and rendering given injected hints, so that the new deep module is verified independently.
17. As a developer, I want log's page `Model` covered by behavior tests proving search, filter,
    flash-on-jump, the pseudo-log-line, and decorations still work after extraction, so that Phase 2
    has a regression guard.
18. As a developer, I want the build and full test suite green after each phase, so that every phase
    is independently shippable.
19. As a developer using the log tab, I want search-match highlighting, the filter, the
    flash-on-jump animation, the pseudo-log-line, and ref decorations to render exactly as before, so
    that the extraction is behavior-preserving.
20. As a developer using the stash tab, I want navigation, apply/pop/drop, stash create, the key
    manager, help, and split focus routing to behave exactly as before, so that the rename is
    behavior-preserving.
21. As a future developer adding a split-view tab, I want a documented canonical shape (ADR 0009) to
    follow, so that I do not reinvent or re-diverge.
22. As a future developer, I want the worktrees exclusion recorded with its rationale, so that I do
    not waste time trying to force worktrees into the split-view mold.

## Implementation Decisions

- **Target architecture (ADR 0009, C2):** split-view tabs are `Model { listPanel, detail
  commit.Model, split }`. `Model` is the only exported entry point per tab, constructed via
  `NewModel`. `listPanel` is unexported and satisfies `splitview.ListPanel`.
- **Direction:** raise log up to the factored shape; do **not** flatten stash down to log's stub
  pattern. Rationale: the factored shape is the better design (independently testable panels) and
  matches the codebase's composition grain (ADR 0001: compose `ui/list.Model`; commit composes
  `filetree.Model[T]` / `diffview.Model`).
- **No shared generic list widget (C1 rejected):** each tab owns its own `listPanel` of identical
  *shape*. The lists render different item types and log carries cross-cutting features a shared
  generic would have to absorb — premature generalization across two tabs. The shared piece remains
  `ui/list.Model` (navigation state).
- **Detail panel is unchanged:** `commit.Model` is already the shared detail panel embedded by both
  tabs. Nothing to do on the detail side.
- **Render-hints contract (log):** the page owns search, filter, flash lifecycle, pseudo-log-line
  source data, and decorations. It computes filtered/visible rows and passes them, plus render hints,
  into the `listPanel` at render/update time. The panel does not read page state directly. Hints
  identified from the current log code: search-match set/highlight, flash subject + expiry, the
  pseudo-log-line row, and ref decorations/badges.
- **Stash file reshuffle:** page `Model` lives in `model.go` (+ `model_keys.go`, `model_ops.go`); the
  panel lives in `list_panel.go` with its `View()`. (Convention-following; exact filenames may adjust
  during implementation.)
- **Module list:**
  - stash `listPanel` — rename of existing inner `Model`.
  - stash page `Model` — rename of `Tab`.
  - log `listPanel` — new extraction (the deep module).
  - log page `Model` — modified orchestrator; `logListAdapter` removed.
- **External call sites for the stash rename (entire blast radius):** the app shell's stash
  construction and the app shell's stash type assertion in tests. All other stash references are
  internal to the package.

## Testing Decisions

- **What makes a good test here:** assert external behavior, not implementation details. For a
  `listPanel`: given entries/rows and a sequence of navigation inputs, assert the resulting selection
  and `SelectedRef`; given rows + render hints, assert the rendered output reflects them
  (e.g. highlight present, flashed row marked, pseudo-line present). For a page `Model`: drive key
  presses / messages and assert observable outcomes (focus moves, commands emitted, search/filter
  results), not private field shapes.
- **Modules to test (all four, per developer):**
  - **log `listPanel`** — isolated tests for navigation, `SelectedRef`, and rendering given injected
    hints (search match, flash, pseudo-line, decorations). Highest-value new coverage.
  - **stash `listPanel`** — existing list-panel behavior tests retargeted to the renamed type; fill
    nav/`SelectedRef` gaps if any.
  - **log page `Model`** — behavior tests that search, filter, flash-on-jump, the pseudo-log-line,
    and decorations still work after extraction (Phase 2 regression guard).
  - **stash page `Model`** — existing tab tests retargeted to `Model`; key-manager, help, and
    split-routing behavior.
- **Prior art:** `ui/stashlist/*_test.go` (existing list-panel and tab behavior tests),
  `ui/log/*_test.go` (log behavior tests), `ui/worktrees/model_help_test.go` (help-overlay
  open/close pattern), `ui/app/model_test.go` (tab construction + type assertion).

## Out of Scope

- **Worktrees normalization** — different archetype (list + passive *sidebar*, no splitview). See
  ADR 0009 and the Detail-panel-vs-Sidebar distinction in CONTEXT.md.
- **A shared generic list widget** across tabs (C1).
- **Worktrees' `table.Model` → `list.Model`** — a separate, content-justified divergence.
- **Any user-visible behavior change** — this is an internal structural refactor; both tabs must
  behave identically before and after.
- **The keymanager + help work already landed on stash** — already committed; not re-done here.

## Further Notes

- Phase 1 (stash rename) is low risk; Phase 2 (log extraction) is the genuinely risky phase. The log
  acceptance gate is manual/behavioral verification of: search highlight, filter, flash-on-jump,
  pseudo-log-line, and decorations — these are easy to silently break during extraction.
- Sequencing is vertical and independently shippable: stash conforms after Phase 1; log converges in
  Phase 2; Phase 3 verifies both match the C2 shape with no stub adapters remaining.
- References: ADR 0009 (this refactor), ADR 0001 (shared `ui/list.Model`), ADR 0006 (commit tab
  removed), CONTEXT.md (Detail panel, Sidebar, Split view, List panel).
