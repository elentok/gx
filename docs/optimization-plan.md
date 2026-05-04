# Status Startup Optimization Plan

## Goals

- Reduce the time it takes for `gx status` to reach first paint
- Stop loading data for inactive tabs during app startup
- Remove the status-page commit-history frame and move commit-history color
  context to the dedicated commits/log surface
- Keep caching as an optional later step, not part of the first pass

## Confirmed Direction

1. Apply the existing `shared` / `local-only` / `remote-only` colors in the
   dedicated commits/log surface
2. Build tab data lazily
   On startup, only load data relevant to the active tab
3. Keep caching as an optional later step

## Likely Startup Cost Today

The current `gx status` startup path does all of the following synchronously:

- branch state
  - `CurrentBranch`
  - `UpstreamBranch`
  - `BranchSyncStatusAgainstRef`
- branch history for the status-side `Commits` frame
  - `BranchHistorySinceMain`
- status scan
  - `ListStageFiles`
- initial diff load for the selected file
  - raw unstaged diff
  - colorized unstaged diff
  - raw staged diff
  - colorized staged diff

The biggest cost is likely the initial diff and delta work, with the branch
history frame as the next obvious synchronous cost to remove.

## Implementation Order

1. Add timing instrumentation around the main status startup calls
2. Remove the status-page `Commits` frame and its startup branch-history load
3. Move commit-history color context to the dedicated commits/log surface, built
   lazily there
4. Change app startup so only the active tab is initialized and loaded
5. Defer initial status diff loading until after first paint
6. Split raw diff loading from async delta colorization
7. Defer status branch sync loading until after first paint
8. Consider caching only after the above are complete

## Expected Wins

- Removing the status `Commits` frame should cut one expensive startup query
- Lazy tab loading should prevent unrelated worktree/log/status models from
  loading on startup
- Deferring diff work should produce the largest first-paint improvement
- Async delta colorization should preserve responsiveness while keeping the rich
  diff view

## Optional Later Step

- Cache branch sync, branch history, and diff results keyed by branch tip, file,
  render mode, and diff context
