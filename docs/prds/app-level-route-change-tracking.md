## Problem Statement

As a developer working on app navigation, route-change tracking is duplicated across routed views (`worktrees`, `log`, `status`, `commit`) via per-model `defer` logic in each `Update` function. This creates maintenance overhead, naming confusion (`currentRouteIdentity`), and increased risk of behavior drift when adding new routed views or modifying route semantics.

## Solution

Move route-change comparison and `RouteChanged` emission to the app shell around child-model `Update` calls. Child models expose their current navigation route via a shared interface, and the app model becomes the default owner of route-change emission while preserving compatibility with explicit route messages.

From the user perspective, behavior stays the same:
- Tab and history context keeps tracking the latest route per `TabID`.
- Back/switch/open semantics remain unchanged.
- Route updates still reflect focus changes such as selected worktree/file/commit subject.

## User Stories

1. As a maintainer, I want route-change logic centralized in the app shell, so that navigation behavior is consistent across tabs.
2. As a maintainer, I want routed views to stop duplicating defer wrappers, so that `Update` functions remain focused on view behavior.
3. As a maintainer, I want a single comparison/emission rule for route changes, so that regressions are less likely.
4. As a maintainer, I want naming that reflects current navigation state, so that interfaces are understandable without internal context.
5. As a user switching tabs, I want the app to remember my last route per `TabID`, so that I return to my previous context.
6. As a user navigating in status, I want selected-file route context to update automatically, so that shell-level navigation state remains accurate.
7. As a user navigating in log, I want focused commit-subject route context to update automatically, so that returning to log restores expected focus.
8. As a user navigating in worktrees, I want route tracking to start once data is loaded, so that empty/preload states don’t break later navigation.
9. As a maintainer, I want async child updates (reloads, focus events, modal completions) to participate in route tracking, so that route state is never stale.
10. As a maintainer, I want explicit `RouteChanged` messages to remain supported, so that specialized flows can still emit manual route events.
11. As a test author, I want app-level tests that verify route updates after child state changes, so that the refactor is protected by behavioral tests.
12. As a future contributor, I want adding a new routed model to be interface-based, so that integration requires minimal boilerplate.
13. As a maintainer, I want route tracking to ignore non-routed models by default, so that unrelated models cannot accidentally affect navigation history.
14. As a maintainer, I want unchanged external behavior for `open/switch/back`, so that this refactor remains low-risk and reversible.
15. As a reviewer, I want clear boundaries between route construction and route emission, so that code ownership is obvious.

## Implementation Decisions

- Introduce a shared app-navigation interface in the nav domain:
  - `RouteProvider` with `CurrentRoute() (Route, bool)`.
- Rename per-view route method from `currentRouteIdentity` to `CurrentRoute` to remove ambiguous terminology and expose a uniform contract.
- Keep route construction local to each routed model (`worktrees`, `log`, `status`, `commit`) because each model owns its route context.
- Move pre/post route capture and `AppendRouteChanged` invocation into app model child-update flow, wrapping every child `Update(msg)` call.
- Preserve comparator/emitter semantics by continuing to use `AppendRouteChanged` as the canonical rule.
- Keep app handling of incoming `RouteChanged` messages for backward compatibility and specialized explicit emission cases.
- Preserve `EnableNavigation` gating semantics exactly; no behavior changes to command-only mode.
- Keep `Route` as navigation identity (not UI state), aligned with glossary language in `CONTEXT.md`.

### Candidate Deep Modules

- `Route Tracking Adapter` (new app-shell helper):
  - Responsibility: Given previous/current child model + message, derive route change cmd with shared rules.
  - Value: Encapsulates pre/post extraction and gating into a narrow, stable behavior surface.
- `Routed View Contract` (nav interface):
  - Responsibility: Stable contract between app shell and routed child models.
  - Value: Decouples app shell from concrete model packages and makes onboarding new routed tabs predictable.

## Testing Decisions

- Good tests assert externally observable behavior:
  - whether app updates last-route memory correctly,
  - whether route-changed commands are emitted only when route actually changes,
  - whether non-route changes do not emit route events.
- Avoid testing implementation details:
  - do not assert defer usage,
  - do not assert internal helper call counts,
  - do not couple tests to method-private control flow.
- Modules to test:
  - app shell route tracking behavior around child updates,
  - route emission comparator behavior (existing `AppendRouteChanged` semantics),
  - routed model conformance to `RouteProvider` through behavior-driven integration tests.
- Prior art in codebase:
  - app router/tab/history tests,
  - route-changed and navigation behavior in routed model tests,
  - existing model update tests that already verify route-relevant focus transitions.

## Out of Scope

- Changing route schema fields.
- Altering back-stack semantics.
- Changing `open/switch/back` message contracts.
- Refactoring unrelated keybinding or modal behavior.
- Reworking notification behavior.

## Further Notes

- This is a structural refactor with expected behavior parity.
- If a future routed model needs custom route emission timing, explicit `RouteChanged` messages remain available.
- ADR is not required at this stage because the decision is low-surprise and easily reversible.
