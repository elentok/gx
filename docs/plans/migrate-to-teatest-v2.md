# Migrate To `teatest/v2`

## Why this exists

During the Bubble Tea v2 migration, the runtime code was moved to:

- `charm.land/bubbletea/v2`
- `charm.land/bubbles/v2`
- `charm.land/lipgloss/v2`

The original worktrees test suite depended on
`github.com/charmbracelet/x/exp/teatest`, but the installed package in this
repo still targeted Bubble Tea v1. That made it incompatible with the migrated
app and tests.

We chose to keep a small repo-local harness for now and defer switching to the
upstream `teatest/v2` package until it is stable and practical to adopt.

## What I learned

- The old `github.com/charmbracelet/x/exp/teatest` package imports Bubble Tea v1 directly.
- That breaks in two ways after the app migrates to Bubble Tea v2:
  - `NewTestModel` expects a v1 `tea.Model`
  - test input helpers emit v1 `tea.KeyMsg` values
- The worktrees tests only used a very small subset of the old `teatest` surface:
  - `NewTestModel`
  - `WithInitialTermSize`
  - `WaitFor`
  - `WithDuration`
  - `WithFinalTimeout`
  - `Output`
  - `Send`
  - `Type`
  - `WaitFinished`
- The raw terminal output under Bubble Tea v2 was not a great assertion target for this suite because the renderer emits terminal diff sequences. Capturing full view snapshots from `View().Content` is much more stable for text-based assertions.

## What I did

- Added a small local Bubble Tea v2 test harness at:
  - [testutil/teatestv2/teatest.go](/Users/david/dev/gx/main/testutil/teatestv2/teatest.go)
- Updated:
  - [ui/worktrees/worktrees_test.go](/Users/david/dev/gx/main/ui/worktrees/worktrees_test.go)
  - [cmd/bump_test.go](/Users/david/dev/gx/main/cmd/bump_test.go)
- Removed the old external `teatest` dependency from [go.mod](/Users/david/dev/gx/main/go.mod).

## What the local harness does

The local harness is intentionally small. It provides:

- `NewTestModel`
- `WithInitialTermSize`
- `WaitFor`
- `WithDuration`
- `WithFinalTimeout`
- `Output`
- `Send`
- `Type`
- `WaitFinished`

Implementation details:

- Runs a Bubble Tea v2 program with in-memory input and output.
- Uses `tea.WithWindowSize(...)` to seed the initial terminal size.
- Uses `tea.WithoutSignals()` for tests.
- Captures stable snapshots by wrapping the model and writing each `View().Content` to the output buffer.
- Sends v2 `tea.KeyPressMsg` values for typing and synthetic key events.

## Why we did not switch to upstream `teatest/v2` yet

- The current local harness is tiny and already verified by the full test
  suite.
- The upstream `x` repo is explicitly experimental.
- Switching immediately would add churn after the migration was already
  stabilized.
- The local harness captures exactly the behavior this repo needs today.

## What is needed when `teatest/v2` is ready

1. Confirm the upstream package is published and usable from this repo.
2. Compare its API against the local harness usage in:
   - [ui/worktrees/worktrees_test.go](/Users/david/dev/gx/main/ui/worktrees/worktrees_test.go)
3. Check whether it supports one of these output models:
   - stable full-frame snapshots
   - a way to observe `View().Content` reliably
   - an equivalent output stream that keeps the current assertions stable
4. Replace the local import:
   - `gx/testutil/teatestv2`
     with the upstream `teatest/v2` package.
5. Update helper calls if the option or method names differ.
6. Run:
   - `go test ./ui/worktrees -count=1`
   - `go test ./... -count=1`
7. Remove:
   - [testutil/teatestv2/teatest.go](/Users/david/dev/gx/main/testutil/teatestv2/teatest.go)

## How to check if `teatest/v2` is ready

Check these in order:

1. Package availability
   - Look for a real importable package path for the v2 package in the `charmbracelet/x` repo.
   - Confirm it has package docs or a published module path you can `go get`.

2. Bubble Tea v2 alignment
   - Verify it imports `charm.land/bubbletea/v2`, not `github.com/charmbracelet/bubbletea`.

3. API fit
   - Confirm it provides equivalents for the small surface this repo uses:
     - `NewTestModel`
     - initial terminal sizing
     - wait helpers with timeout
     - `Send`
     - `Type`
     - `Output`
     - `WaitFinished`

4. Output behavior
   - Verify the output it exposes works with text assertions in this repo.
   - If it only exposes renderer diff output, expect additional test rewrites.

5. Stability signal
   - Check the upstream package docs, README, and recent commit activity.
   - Since `x` is experimental, prefer adopting it only once the package looks intentionally maintained and not in obvious flux.

## Suggested quick readiness commands

Run some combination of these when revisiting:

```bash
go list -m -versions github.com/charmbracelet/x/exp/teatest/v2
go doc github.com/charmbracelet/x/exp/teatest/v2
rg -n "package teatest" "$GOMODCACHE"/github.com/charmbracelet/x*
rg -n "charm.land/bubbletea/v2|github.com/charmbracelet/bubbletea" "$GOMODCACHE"/github.com/charmbracelet/x*
```

If `go list` or `go doc` cannot resolve a real package, it is not ready for adoption in this repo yet.

## Current status

- Local v2 harness is in place and working.
- `go test ./... -count=1` passes with the local harness.
- Revisit this task only if upstream `teatest/v2` becomes clearly usable and offers a maintenance benefit over the local helper.
