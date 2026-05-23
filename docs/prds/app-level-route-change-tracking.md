## Problem Statement

As a developer working on app navigation, view-state change tracking is duplicated across routed views (`worktrees`, `log`, `status`, `commit`) via per-model `defer` logic in each `Update` function. This creates maintenance overhead, naming confusion, and increased risk of behavior drift when adding new routed views or modifying navigation semantics.

## Solution

Move view-state comparison and `ViewStateChanged` emission to the app shell around child-model `Update` calls. Child models expose their current navigation view state via a shared interface, and the app model becomes the default owner of view-state emission while preserving compatibility with explicit state-change messages.

From the user perspective, behavior stays the same:
- Tab and history context keeps tracking the latest view state per `TabID`.
- Back/switch/open semantics remain unchanged.
- View-state updates still reflect focus changes such as selected worktree/file/commit subject.

## User Stories

1. As a maintainer, I want view-state change logic centralized in the app shell, so that navigation behavior is consistent across tabs.
2. As a maintainer, I want routed views to stop duplicating defer wrappers, so that `Update` functions remain focused on view behavior.
3. As a maintainer, I want a single comparison/emission rule for view-state changes, so that regressions are less likely.
4. As a maintainer, I want naming that reflects current navigation state, so that interfaces are understandable without internal context.
5. As a user switching tabs, I want the app to remember my last view state per `TabID`, so that I return to my previous context.
6. As a user navigating in status, I want selected-file view context to update automatically, so that shell-level navigation state remains accurate.
7. As a user navigating in log, I want focused commit-subject view options to update automatically, so that returning to log restores expected focus.
8. As a user navigating in worktrees, I want view-state tracking to start once data is loaded, so that empty/preload states don’t break later navigation.
9. As a maintainer, I want async child updates (reloads, focus events, modal completions) to participate in view-state tracking, so that state is never stale.
10. As a maintainer, I want explicit `ViewStateChanged` messages to remain supported, so that specialized flows can still emit manual state-change events.
11. As a test author, I want app-level tests that verify view-state updates after child state changes, so that the refactor is protected by behavioral tests.
12. As a future contributor, I want adding a new routed model to be interface-based, so that integration requires minimal boilerplate.
13. As a maintainer, I want view-state tracking to ignore non-routed models by default, so that unrelated models cannot accidentally affect navigation history.
14. As a maintainer, I want unchanged external behavior for `open/switch/back`, so that this refactor remains low-risk and reversible.
15. As a reviewer, I want clear boundaries between view-state construction and emission, so that code ownership is obvious.

## Implementation Decisions

- Introduce a shared app-navigation interface in the nav domain:
  - `ViewStateProvider` with `CurrentViewState() (ViewState, bool)`.
- Use `CurrentViewState` consistently as the per-view method to expose a uniform contract.
- Keep view-state construction local to each routed model (`worktrees`, `log`, `status`, `commit`) because each model owns its view context/options.
- Move pre/post state capture and `AppendViewStateChanged` invocation into app model child-update flow, wrapping every child `Update(msg)` call.
- Preserve comparator/emitter semantics by continuing to use `AppendViewStateChanged` as the canonical rule.
- Keep app handling of incoming `ViewStateChanged` messages for backward compatibility and specialized explicit emission cases.
- Preserve `EnableNavigation` gating semantics exactly; no behavior changes to command-only mode.
- Keep `ViewState` as navigation identity (not generic UI state), split into durable `ViewContext` and transient `ViewOptions`.

### Candidate Deep Modules

- `ViewState Tracking Adapter` (new app-shell helper):
  - Responsibility: Given previous/current child model + message, derive state-change cmd with shared rules.
  - Value: Encapsulates pre/post extraction and gating into a narrow, stable behavior surface.
- `Routed View Contract` (nav interface):
  - Responsibility: Stable contract between app shell and routed child models.
  - Value: Decouples app shell from concrete model packages and makes onboarding new routed tabs predictable.

## Testing Decisions

- Good tests assert externally observable behavior:
  - whether app updates last-view-state memory correctly,
  - whether view-state-changed commands are emitted only when state actually changes,
  - whether non-context changes do not trigger context-reset behavior.
- Avoid testing implementation details:
  - do not assert defer usage,
  - do not assert internal helper call counts,
  - do not couple tests to method-private control flow.
- Modules to test:
  - app shell view-state tracking behavior around child updates,
  - state emission comparator behavior (existing append semantics),
  - routed model conformance to `ViewStateProvider` through behavior-driven integration tests.
- Prior art in codebase:
  - app router/tab/history tests,
  - state-change and navigation behavior in routed model tests,
  - existing model update tests that already verify focus/filter transitions.

## Out of Scope

- Changing view-state payload behavior.
- Altering back-stack semantics.
- Changing `open/switch/back` message contracts.
- Refactoring unrelated keybinding or modal behavior.
- Reworking notification behavior.

## Further Notes

- This is a structural refactor with expected behavior parity.
- If a future routed model needs custom state emission timing, explicit `ViewStateChanged` messages remain available.
- ADR is not required at this stage because the decision is low-surprise and easily reversible.
