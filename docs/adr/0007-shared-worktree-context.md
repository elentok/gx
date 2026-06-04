# ADR-0007: Log and status tabs share worktree context

**Status**: Accepted

## Context

Log and status tabs each maintained independent `WorktreeRoot` values in `lastViewStateByTab`.
Switching from log (worktree Y) to status via `g+3` restored status's own last-remembered
worktree, which was often the default worktree — not worktree Y. This caused silent context
mismatches with no indication to the user.

The root was in `TabViewStateForViewContext`: when switching without an explicit `WorktreeRoot`,
it fell through to per-tab memory, which was independently seeded.

## Decision

Tab switching carries the active `WorktreeRoot` to the target tab. When `Switch(ViewState{Tab:
TabStatus})` is called with no explicit `WorktreeRoot`, the navstate injects the current active
tab's `WorktreeRoot` instead of restoring the target tab's independent memory.

## Consequences

- `g+2`/`g+3` (and `g+l`/`g+s`) now always keep log and status pointed at the same worktree.
- Deliberately running log on worktree Y and status on worktree X simultaneously is no longer
  possible via tab switching. The worktrees tab (Enter on a worktree row) remains the explicit
  mechanism to set a tab's worktree context.
- The pseudo-log-line's Enter action (`nav.Switch(Tab: TabStatus, WorktreeRoot: m.worktreeRoot)`)
  is now redundant for context-carrying but remains useful as a navigation shortcut.
