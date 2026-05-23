# Tab/Route Refactor Plan

## Goal

Simplify app navigation by removing overlapping concepts (`tab`, `RouteKind`, per-tab history, global history)
and replacing them with one coherent model that matches user expectations.

## Agreed Outcomes

1. `commit` is a first-class top-level tab.
2. One global back stack for deep navigation.
3. Tab switches do not add back depth.
4. Each tab remembers its latest route and restores it when revisiting.
5. Route identity is updated live from child models.
6. Terminology is unified around `TabID` and `Route.Tab`.

## Canonical Terminology

- `TabID`: top-level destination identity (`worktrees`, `log`, `commit`, `status`).
- `Route`: navigation identity (`TabID` + route context), not UI state.
- `lastRouteByTab`: remembered latest route per tab.
- `selectedWorktree`: focus identity in worktrees list, distinct from `worktreeRoot`.

## Navigation Contract

- `Open(route)`: deep navigation, pushes onto stack.
- `Switch(route)`: tab/session switch target, replaces visible route without push.
- `Back()`: pops one deep level; if at root, quits.
- `RouteChanged(route)`: emitted by active child model whenever restorable route state changes.

## Target Behavior (Critical Scenarios)

### A. Commit context survives tab switch

1. Open `log` tab
2. Enter commit (`Open(commit)`)
3. Switch to `status` tab (`Switch(status)`)
4. Switch to `commit` tab (`Switch(commit)`)
5. App restores same commit route as before

### B. Commit navigation updates log back target

1. Open `log` tab with focused commit X
2. Enter commit X
3. Navigate to commit Y via `,` / `.`
4. Press `q` / `esc` (`Back()`)
5. App returns to `log` with commit Y focused

### C. Worktrees focus restoration on back

1. Select worktree W in `worktrees`
2. Open `log` from W
3. `Back()` to worktrees
4. Worktrees restores focus on W

## Data Model

App shell owns:

- `stack []Route` (single global deep stack)
- `lastRouteByTab map[TabID]Route`
- `activeTab TabID`

Rules:

1. On `Open(r)`:
- push `r` onto `stack`
- set `activeTab = r.Tab`
- set `lastRouteByTab[r.Tab] = r`

2. On `Switch(targetTab)`:
- resolve `r = lastRouteByTab[targetTab]` if present, else default tab route
- replace visible route with `r` (no stack push)
- set `activeTab = targetTab`

3. On `RouteChanged(r)`:
- if top of stack is same tab, replace top route with `r`
- update `lastRouteByTab[r.Tab] = r`

4. On `Back()`:
- if stack non-empty: pop one, reveal previous route
- if no route remains beyond root: quit
- ensure `lastRouteByTab` reflects newly visible route

## Route Payloads by Tab

- `worktrees`: `{tab: worktrees, selectedWorktree}`
- `log`: `{tab: log, worktreeRoot, ref, focusSubject?}`
- `commit`: `{tab: commit, worktreeRoot, ref}`
- `status`: `{tab: status, worktreeRoot, selectedPath}`

Note: no modal state, scroll offsets, or other UI-only state in route payloads.

## Keybindings

- `gw`, `gl`, `gs`, `gc` switch to remembered tab route
- `1`, `2`, `3`, `4` map to `worktrees`, `log`, `status`, `commit`
- `q` / `esc` in commit dispatches `Back()` only

## Refactor Scope (One-Shot)

1. Rename navigation protocol:
- `RouteKind` -> `TabID`
- `Route.Kind` -> `Route.Tab`
- `Push/Replace` -> `Open/Switch`
- add `RouteChanged`

2. Rewrite app shell routing model:
- remove per-tab deep histories
- remove commit->log tab special-case
- adopt `stack + lastRouteByTab`

3. Child emitters for live route updates:
- `worktrees`: emit `RouteChanged` on selected row change
- `log`: emit `RouteChanged` on cursor move/focus change
- `commit`: emit `RouteChanged` on commit ref change (` , . `)
- `status`: emit `RouteChanged` for `selectedPath`

4. Update footer tabs and global tab ordering to 4 tabs.

5. Replace/add tests for scenarios A/B/C and root-back quit behavior.

## Testing Strategy

- Test-first for app routing transitions and restoration behavior.
- Keep behavior-preserving intent except for explicit tab-model changes above.
- Minimum verification:
- `go test ./ui/app`
- targeted tests for `ui/log`, `ui/commit`, `ui/worktrees`, `ui/status`, `ui/nav`
- full suite if feasible before merge.

## Risks

- Large symbol rename (`Kind` -> `Tab`) can collide with unrelated `Kind` fields; must scope replacements carefully.
- One-shot refactor may break many tests simultaneously; prioritize compile restoration first, then behavior tests.
- Route payload growth should be controlled to avoid coupling route state with UI internals.

## Non-Goals

- Persisting route/session state across app restarts.
- Encoding full viewport positions in routes.
- Redesigning page internals unrelated to navigation identity.
