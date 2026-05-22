## Problem Statement

The app navigation model is hard to understand and hard to evolve. Users and maintainers currently deal with overlapping concepts: tab identity, route kind, commit-as-log special cases, per-tab deep histories, and global history behavior. This creates ambiguity in expected behavior (especially around switching tabs, going back, and restoring focus), and increases the risk of regressions when adding or changing navigation flows.

## Solution

Adopt a single, explicit navigation model centered on `TabID` and `Route.Tab`, with one global deep-navigation stack and per-tab route memory.

From the user perspective:
- `worktrees`, `log`, `commit`, and `status` are first-class tabs.
- Opening deeper screens uses `Open` semantics and is reversible with `Back`.
- Switching tabs does not pollute back depth and restores the last route seen in that tab.
- Returning from deep routes restores the expected focused context (worktree, commit, status path) using live route updates.

## User Stories

1. As a user, I want `commit` to be a first-class tab, so that the footer and tab highlight always match the page I am viewing.
2. As a user, I want one consistent back behavior, so that I can predict what `q`/`esc` will do.
3. As a user, I want deep navigation to use explicit open/back semantics, so that drill-down workflows feel coherent.
4. As a user, I want tab switching to preserve my place, so that moving between tabs does not reset context.
5. As a user, I want switching tabs to avoid adding history depth, so that back navigation reflects drill-down steps rather than tab hops.
6. As a user, I want `worktrees -> log -> back` to restore the previously focused worktree, so that I can continue from where I left off.
7. As a user, I want `log -> commit -> status -> commit` to restore the same commit route, so that cross-tab pivots do not lose deep context.
8. As a user, I want commit `,`/`.` navigation to update restore state live, so that back returns me to log focused on the commit I last reached.
9. As a user, I want back from commit to return to the right underlying route, so that commit does not need special-case routing rules.
10. As a user, I want `gw/gl/gs/gc` and `1/2/3/4` to behave symmetrically, so that tab navigation is easy to memorize.
11. As a maintainer, I want one canonical term for tab identity (`TabID`), so that code and docs share the same mental model.
12. As a maintainer, I want route identity to exclude UI-only details, so that route payloads remain stable and testable.
13. As a maintainer, I want app-shell to own navigation state transitions, so that child pages communicate intent rather than mutating routing directly.
14. As a maintainer, I want child pages to emit live route updates, so that app-shell can restore focus correctly without deep coupling.
15. As a maintainer, I want to remove commit-to-log tab mapping special cases, so that route-to-tab behavior is explicit and uniform.
16. As a maintainer, I want the navigation message vocabulary to match product semantics (`Open`, `Switch`, `Back`, `RouteChanged`), so that intent is clear in code reviews.
17. As a maintainer, I want tests to assert external navigation behavior, so that refactors remain safe without coupling tests to internals.
18. As a maintainer, I want scenario-driven regression tests for commit/tab/back flows, so that user-critical workflows remain protected.
19. As a contributor, I want a simple deep module for route state transitions, so that future navigation changes are localized.
20. As a contributor, I want routing terms aligned with the glossary, so that onboarding cost is lower.
21. As a user, I want root-level back to quit as before, so that existing quit muscle memory is preserved.
22. As a user, I want status tab restoration to bring me back to the previous selected path, so that interrupted file-level work resumes quickly.
23. As a maintainer, I want this done as a one-shot refactor with a strict behavior bar, so that we avoid long-lived dual models.
24. As a maintainer, I want clear non-goals (no persisted session state, no viewport payloads), so that scope stays controlled.

## Implementation Decisions

- Introduce a canonical tab identity type `TabID` and migrate route shape to `Route.Tab`.
- Replace browser-like message names with navigation-intent names:
  - `Open(route)` for deep navigation
  - `Switch(route)` for tab/session switching
  - `Back()` for reverse deep navigation
  - `RouteChanged(route)` for live route identity updates from active pages
- Build/modify a deep router-state module that encapsulates transition rules behind a small interface:
  - input events: `Open`, `Switch`, `Back`, `RouteChanged`
  - owned state: global `stack []Route`, `lastRouteByTab map[TabID]Route`, `activeTab`
  - deterministic transition outputs used by app-shell to coordinate page model lifecycle
- Remove per-tab deep histories as a first-class mechanism; retain per-tab memory only as `lastRouteByTab`.
- Promote `commit` to first-class tab identity in footer and keybindings.
- Make tab switching resolve to remembered route for target tab, defaulting to tab-root route when none exists.
- Define route payload contracts by tab:
  - `worktrees`: tab + selected worktree focus identity
  - `log`: tab + worktree root + ref (+ optional focus subject)
  - `commit`: tab + worktree root + ref
  - `status`: tab + worktree root + selected path
- Keep route payloads as navigation identity only (no modal/viewport/transient UI internals).
- App-shell remains source of truth for navigation state and lifecycle orchestration; child models emit intent messages only.
- Child models emit `RouteChanged` live:
  - `worktrees` on selection move
  - `log` on cursor/focus changes
  - `commit` on current ref changes (` , . `)
  - `status` on selected path changes
- Preserve contextual quit behavior: `Back` on root route quits.
- Align terminology in domain glossary with refactor (`TabID`, route identity, selected worktree) to reduce future ambiguity.

## Testing Decisions

- Good tests assert observable navigation behavior and restore outcomes, not internal storage layout.
- Test the deep router-state module directly for transition correctness and edge cases.
- Keep app-shell integration tests focused on user-visible routing behavior:
  - 4-tab switching symmetry
  - `Open`/`Back` semantics
  - remembered route restoration per tab
  - root back quits
- Add explicit scenario tests for:
  - `log -> commit -> status -> commit` restoration
  - commit navigation to Y then back to log focusing Y
  - `worktrees -> log -> back` worktree focus restoration
  - status selected-path restoration after tab pivots
- Update prior tests that currently encode `Push/Replace` or commit-to-log tab coupling.
- Maintain prior art style from existing `ui/app`, `ui/nav`, and page-level message tests.

## Out of Scope

- Persisting tab/session route memory across process restarts.
- Encoding viewport offsets, scroll positions, modal visibility, or other UI internals in routes.
- Redesigning non-navigation page internals beyond what is needed to emit route identity updates.
- Introducing new visual themes or layout redesigns unrelated to tab/navigation semantics.

## Further Notes

- This change is intentionally one-shot to avoid prolonged dual routing models.
- The expected behavioral changes are intentional and user-driven: commit as explicit tab and tab restoration semantics.
- The solution follows existing glossary language and preserves ADR direction toward shared, testable deep modules with simple interfaces.
- Success criteria: routing behavior becomes predictable under one mental model, and navigation bugs become localizable to router-state and app-shell tests.
