## Problem Statement

Navigation terminology in the app is hard to reason about because one concept currently carries two different concerns:

1. durable tab context that determines whether a tab page instance should be reused or rebuilt, and
2. transient navigation options that tune behavior inside a currently active view.

Using a single overloaded term makes code review and maintenance harder. Ambiguous conversion/equality helper names blur behavior boundaries between tab identity and view-level focus/filter behavior.

## Solution

Adopt a clearer domain model centered on `ViewState` with two explicit parts:

- `ViewContext`: durable app-navigation context used for tab page reuse/reset decisions.
- `ViewOptions`: transient behavior options that affect how a view opens or focuses content but do not define tab instance identity.

Unify navigation APIs and model contracts around this vocabulary so contributors can quickly understand which comparisons are full payload comparisons versus tab-context comparisons.

## User Stories

1. As a maintainer, I want navigation names to describe behavior directly, so that I can understand tab-switch logic quickly.
2. As a maintainer, I want durable tab identity separated from transient view options, so that equality checks are obvious and safe.
3. As a maintainer, I want app-shell tab reuse/reset decisions to depend only on `ViewContext`, so that transient focus/filter updates do not rebuild pages.
4. As a contributor, I want the top-level navigation payload to be called `ViewState`, so that the term matches how the app behaves.
5. As a contributor, I want child models to expose `CurrentViewState`, so that app-shell orchestration is consistent across routed views.
6. As a contributor, I want event names to align with the domain (`ViewStateChanged`), so that message intent is clear.
7. As a contributor, I want app navigation contracts to avoid overloaded words like route identity/state, so that naming does not require tribal knowledge.
8. As a reviewer, I want tests to express the durable/transient split explicitly, so that regressions are caught where behavior boundaries matter.
9. As a user, I want tab switching to preserve context predictably, so that returning to a tab restores the expected location.
10. As a user, I want transient focus behavior (for example commit subject focus) to work without forcing page reconstruction, so that navigation feels smooth.
11. As a maintainer, I want glossary language in `CONTEXT.md` to match production terminology, so that docs and code reinforce each other.
12. As a maintainer, I want backwards-compatible behavior in navigation flows while renaming, so that the refactor does not change user-visible behavior.
13. As a maintainer, I want message parsing and emitting paths renamed consistently, so that no mixed old/new terminology remains.
14. As a maintainer, I want tab history logic to keep semantics unchanged during the rename, so that deep navigation behavior remains stable.
15. As a maintainer, I want transitive references in status/log/commit/worktrees to compile under one vocabulary, so that onboarding is easier.
16. As a maintainer, I want the app-shell to remain the source of truth for navigation state transitions, so that responsibilities stay clear.
17. As a contributor, I want APIs and interfaces to be coherent enough that adding future view options is straightforward, so that extension work is lower risk.
18. As a contributor, I want durable context conversion helpers to be explicitly named for intent, so that helper purpose is obvious at call sites.
19. As a maintainer, I want equality semantics represented by type boundaries rather than ad-hoc helper names, so that behavior is self-documenting.
20. As a maintainer, I want refactor scope and non-goals documented, so that follow-up work can be planned without ambiguity.

## Implementation Decisions

- Canonical terminology:
  - `ViewState` is the top-level navigation payload.
  - `ViewContext` is the durable subset used for tab instance reuse/reset logic.
  - `ViewOptions` is the transient subset used for intra-view behavior adjustments.
- `ViewContext` fields are the stable app-navigation context used for tab instance identity:
  - `Tab`, `WorktreeRoot`, `Ref`, `InitialPath`.
- `ViewOptions` fields are transient, behavior-tuning options:
  - `FocusSubject`, `FilterPath`, `FilterStartLine`, `FilterEndLine`.
- App-shell tab memory stores full `ViewState`, but tab reuse/reset comparison uses only `ViewContext`.
- Navigation contracts are renamed consistently:
  - provider contract becomes `ViewStateProvider` with `CurrentViewState()`.
  - change event family becomes `ViewStateChanged` with corresponding emit/parse helpers.
- Equality semantics are explicit:
  - full `ViewState` equality is distinct from `ViewContext` equality.
  - tab reuse/reset is keyed to `ViewContext` equality only.
- Refactor is behavior-preserving:
  - no user-visible navigation semantics change.
  - existing flows for open/switch/back and app-shell ownership remain intact.
- Deep module opportunity:
  - extract a small navigation-state adapter that performs `ViewState` ↔ `ViewContext` operations and comparison rules behind a stable interface.
  - this module should be testable in isolation with table-driven cases.

## Testing Decisions

- Good tests assert observable behavior and contracts, not private implementation details.
- Core tests focus on behavior boundaries:
  - tab page reuse/reset triggers on `ViewContext` changes.
  - transient `ViewOptions` changes do not trigger tab rebuild.
  - app-shell memory and restore-by-tab remain correct.
  - event emission/parsing reflects renamed contracts and unchanged behavior.
- Modules to test:
  - app-shell navigation orchestration.
  - nav event helpers and provider contract integration.
  - routed child models that expose current view state and emit change events.
  - isolated deep module (if extracted) for context/options split and comparison semantics.
- Prior art:
  - existing app model navigation tests.
  - existing nav helper tests.
  - routed model tests in log/status/commit/worktrees for navigation updates.

## Out of Scope

- Redesigning navigation UX or introducing new user-facing navigation features.
- Changing semantics of deep history behavior beyond terminology and type-boundary clarity.
- Reworking unrelated panel, selection, or diff navigation concerns.
- Introducing persistence/schema changes outside current in-memory navigation model.

## Further Notes

- This PRD intentionally chooses clarity over minimal rename scope, because mixed terminology would preserve existing ambiguity.
- The main risk is partial rename drift; implementation should be done atomically and validated with full test coverage.
- Follow-up improvements can introduce stricter type encapsulation once naming and contracts are stabilized.
