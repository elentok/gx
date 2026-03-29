# BubbleTea v2 Migration Plan

## Current Task: Yank Dialog Space Toggle Regression

- [x] Reproduce the yank dialog checkbox toggle path and identify why pressing space does not uncheck the current item.
- [x] Fix the checklist key handling with the smallest compatible change.
- [x] Add regression coverage for the space-toggle behavior.
- [x] Run targeted tests and record the result.

## Review

- Root cause: `ui/components/checklist.go` only toggled on the literal `" "` key string, while Bubble Tea v2 key handling for the space bar in this path now arrives as `"space"`.
- Fix: accept both `" "` and `"space"` in `Checklist.Update(...)` so the yank dialog can toggle items off again without risking compatibility with any older callers.
- Regression coverage: added `TestChecklistUpdate_TogglesCurrentItemOnSpace` in `ui/components/checklist_test.go`.
- Verification:
  - `go test ./ui/components -count=1`
  - `go test ./ui/worktrees -run 'TestYankModalAppearsAndCancels|TestYankAndPaste' -count=1`

## Goal

Upgrade the repo from Bubble Tea v1 to the Bubble Tea v2 stack described in the upstream guide, while keeping the existing CLI and TUI behavior intact.

## Constraints and assumptions

- This repo currently imports Bubble Tea v1, Lip Gloss v1, and Bubbles v1 APIs.
- Bubble Tea v2 changes are not isolated to import paths; they affect `View()` signatures, key event handling, program setup, and tests.
- We should treat the Bubble Tea, Lip Gloss, and Bubbles upgrades as one coordinated dependency change and verify component compatibility before code edits.

## Migration phases

- [x] Phase 1: Dependency and compatibility prep
  - Confirm the target module paths and latest stable v2 versions for Bubble Tea, Lip Gloss, and Bubbles. Do not use prereleases unless explicitly requested.
  - Update `go.mod` / `go.sum` to the coordinated v2 stack.
  - Replace direct imports:
    - `github.com/charmbracelet/bubbletea` -> `charm.land/bubbletea/v2`
    - `github.com/charmbracelet/lipgloss` -> `charm.land/lipgloss/v2`
    - Audit `bubbles/*` imports and move them to `charm.land/bubbles/v2/*`.
  - Run a compile pass to expose the exact API breakage set after dependency changes.

- [x] Phase 2: Convert all models to the v2 `tea.View` API
  - Update `View()` methods in:
    - `cmd/spinner.go`
    - `cmd/bump.go`
    - `ui/confirm/confirm.go`
    - `ui/worktrees/model_view.go`
  - Replace string returns with `tea.NewView(...)` or explicit `tea.View` construction.
  - Move program-level terminal state into declarative view fields where applicable.
  - For the worktrees UI, set `AltScreen` from `View()` instead of `tea.WithAltScreen()` in program construction.
  - Recheck modal and status-bar rendering to ensure the composed content still displays correctly under `tea.View`.

- [x] Phase 3: Migrate key handling to v2 message types
  - Replace runtime `case tea.KeyMsg:` handlers with `case tea.KeyPressMsg:` unless release events are intentionally needed.
  - Update helper signatures that currently accept `tea.KeyMsg`:
    - `ui/worktrees/model_confirm_modal.go`
    - `ui/worktrees/rename.go`
    - `ui/worktrees/clone.go`
    - `ui/worktrees/new.go`
    - `ui/worktrees/yank.go`
    - `ui/worktrees/paste.go`
    - `ui/worktrees/search.go`
    - `ui/worktrees/model_error_modal.go`
    - `ui/worktrees/model_logs_modal.go`
  - Replace field checks that depend on v1 key structs:
    - `msg.Type == tea.KeyCtrlC`
    - `msg.Type == tea.KeyRunes`
    - direct `tea.KeyEsc`, `tea.KeyEnter`, `tea.KeyCtrlU`, `tea.KeyCtrlN`, `tea.KeyCtrlP` message construction in tests
  - Prefer `msg.String()` / `key.Matches(...)` where possible, and only drop to `Code` / `Mod` checks when needed.
  - Audit for `" "` string matching; replace with `"space"` if present.

- [x] Phase 4: Update program construction and removed imperative APIs
  - Replace removed or relocated program options and methods with declarative `tea.View` fields.
  - Audit `tea.NewProgram(...)` call sites:
    - `cmd/cmd.go`
    - `cmd/spinner.go`
    - `cmd/bump.go`
    - `ui/confirm/confirm.go`
  - Keep `WithInput` / `WithOutput` where still supported, but remove v1-only options.
  - Search for removed program methods and imperative terminal commands even if the initial scan did not find them.
  - Check whether any `WindowSize` requests or `Sequentially` usage need renaming during the compile-fix pass.

- [x] Phase 5: Reconcile component integrations
  - Validate the Bubbles component APIs used here under the selected v2 release:
    - `spinner`
    - `table`
    - `textinput`
    - `viewport`
    - `help`
    - `key`
  - Fix any signature or message-type changes in:
    - `ui/worktrees/model_update.go`
    - `ui/worktrees/model_layout.go`
    - `ui/worktrees/table.go`
    - `ui/worktrees/sidebar.go`
    - `cmd/spinner.go`
  - Pay particular attention to spinner tick wiring and sub-model `Update(...)` calls, since those often surface type changes after the key-message migration.

- [x] Phase 6: Rewrite tests for v2 message construction
  - Update direct message construction in:
    - `cmd/bump_test.go`
    - `ui/worktrees/worktrees_test.go`
  - Replace v1 synthetic key messages with v2 equivalents that model key presses.
  - Re-run test failures after each batch and normalize helpers if repeated message-construction boilerplate emerges.

- [x] Phase 7: Verification
  - Run targeted package tests first for the touched areas:
    - `go test ./cmd ./ui/worktrees ./ui/confirm -count=1`
  - Run the full suite:
    - `go test ./... -count=1`
  - Manually smoke-test the interactive flows most exposed to migration risk:
    - launching `gx`
    - quitting from the main worktrees UI
    - navigation and help toggle
    - rename / clone / new dialogs
    - confirm modal flows
    - spinner-driven pull / push / remote update flows
    - bump picker
  - Capture any behavioral regressions and fix them before calling the migration complete.

## File inventory from the scan

- Primary model `View()` changes
  - `cmd/spinner.go`
  - `cmd/bump.go`
  - `ui/confirm/confirm.go`
  - `ui/worktrees/model_view.go`

- Primary key-event changes
  - `cmd/bump.go`
  - `ui/confirm/confirm.go`
  - `ui/worktrees/model_update.go`
  - `ui/worktrees/model_confirm_modal.go`
  - `ui/worktrees/model_error_modal.go`
  - `ui/worktrees/model_logs_modal.go`
  - `ui/worktrees/rename.go`
  - `ui/worktrees/clone.go`
  - `ui/worktrees/new.go`
  - `ui/worktrees/yank.go`
  - `ui/worktrees/paste.go`
  - `ui/worktrees/search.go`

- Primary program setup changes
  - `cmd/cmd.go`
  - `cmd/spinner.go`
  - `cmd/bump.go`
  - `ui/confirm/confirm.go`

- Primary test changes
  - `cmd/bump_test.go`
  - `ui/worktrees/worktrees_test.go`

## Risks to watch

- Bubbles v2 compatibility may require more than import rewrites; sub-model `Update` and emitted message types may shift together with Bubble Tea v2.
- The worktrees UI currently relies on `tea.WithAltScreen()` at program construction time; that behavior needs to move into the `View()` layer without altering screen lifecycle.
- Tests currently synthesize many v1 key messages directly, so the migration may look larger in tests than in runtime code.
- ANSI-aware table rendering is custom; verify that the switch to Lip Gloss v2 does not change width or truncation behavior in ways that break alignment.

## Review

- Phase 1 completed on 2026-03-18.
- Resolved stable v2 module line:
  - `charm.land/bubbletea/v2 v2.0.2`
  - `charm.land/bubbles/v2 v2.0.0`
  - `charm.land/lipgloss/v2 v2.0.2`
- Rewrote app imports from the v1 GitHub paths to the v2 `charm.land` module paths.
- Ran `go mod tidy` after the import rewrite.
- Forced compile result:
  - Primary blocker is the Bubble Tea v2 `View() tea.View` requirement.
  - Secondary blocker already visible: `viewport.New(...)` no longer accepts width/height positional ints.
- Residual v1 indirect deps remain because `github.com/charmbracelet/x/exp/teatest` still depends on Bubble Tea v1 / Lip Gloss v1 for test helpers. That will need a Phase 6 decision or workaround.
- Phase 2 completed on 2026-03-18.
- Converted all app models with `View()` methods to return `tea.View`.
- Moved alt-screen declaration from `tea.WithAltScreen()` into the worktrees model view.
- Updated viewport construction to `viewport.New(viewport.WithWidth(...), viewport.WithHeight(...))`.
- Updated view-adjacent width/height setters:
  - `viewport.SetWidth` / `viewport.SetHeight`
  - `help.SetWidth`
- Post-Phase-2 compile result:
  - Remaining build failures are key-message API changes in the worktrees UI (`tea.KeyMsg` field access and v1 key constants), which is the expected Phase 3 boundary.
- Phase 3 runtime migration completed on 2026-03-18.
- Switched runtime key handling to `tea.KeyPressMsg` in:
  - `cmd/bump.go`
  - `ui/confirm/confirm.go`
  - `ui/worktrees/model_update.go`
  - `ui/worktrees/model_confirm_modal.go`
  - `ui/worktrees/model_error_modal.go`
  - `ui/worktrees/model_logs_modal.go`
  - `ui/worktrees/rename.go`
  - `ui/worktrees/clone.go`
  - `ui/worktrees/new.go`
  - `ui/worktrees/yank.go`
  - `ui/worktrees/paste.go`
  - `ui/worktrees/search.go`
- Replaced v1 key field checks with `msg.String()` / `key.Matches(...)`.
- Verification:
  - `go build ./...` passes.
  - `go test ./cmd` passes.
  - `go test ./cmd ./ui/confirm ./ui/worktrees -run '^$'` now fails only because `ui/worktrees/worktrees_test.go` uses `github.com/charmbracelet/x/exp/teatest`, which still depends on Bubble Tea v1 and constructs v1 key messages.
- Phase 4 completed on 2026-03-18.
- Audited remaining `tea.NewProgram(...)` call sites:
  - `cmd/cmd.go`
  - `cmd/spinner.go`
  - `cmd/bump.go`
  - `ui/confirm/confirm.go`
- Result:
  - `cmd/cmd.go` already uses plain `tea.NewProgram(m)` with alt-screen declared in the view.
  - `cmd/spinner.go`, `cmd/bump.go`, and `ui/confirm/confirm.go` still use `WithInput` / `WithOutput`, which remain valid in v2.
  - No remaining runtime uses of removed imperative APIs such as `WithAltScreen`, `EnterAltScreen`, `ExitAltScreen`, mouse enable/disable commands, or other deprecated program-level screen toggles were found in app code.
- Phase 5 completed on 2026-03-18.
- Runtime component integrations reconciled during the compile-fix passes in earlier phases:
  - `viewport.New(...)` updated to option-based construction
  - viewport width/height setters updated
  - help width setter updated
  - spinner, textinput, table, key, and viewport integrations compile and pass tests under the selected v2 stack
- Verification:
  - `go build ./...` passes.
  - Representative component-backed integration tests pass in `ui/worktrees`.
- Phase 6 completed on 2026-03-18.
- Replaced the Bubble Tea v1-only external `teatest` dependency with a repo-local v2-compatible harness:
  - `testutil/teatestv2/teatest.go`
- Updated `ui/worktrees/worktrees_test.go` to import the local harness and synthesize v2 `tea.KeyPressMsg` values.
- Updated `cmd/bump_test.go` to use v2 `tea.KeyPressMsg`.
- Improved the local harness to capture full `View().Content` snapshots instead of raw terminal diff output so text assertions remain stable under Bubble Tea v2 rendering.
- Removed the obsolete external dependency from the module graph with `go mod tidy`.
- Verification:
  - `go test ./ui/worktrees -count=1` passes.
  - `go test ./cmd -count=1` passes.
  - `go test ./... -count=1` passes.
- Phase 7 completed on 2026-03-18.
- Representative smoke coverage run:
  - `go test ./cmd -run 'TestPickBump_EnterSelectsPatch|TestRunBump_PatchExplicit' -count=1`
  - `go test ./ui/worktrees -run 'TestDeleteWorktree|TestCloneWorktree|TestNewWorktree|TestYankAndPaste|TestPushWorktree|TestPullMainRefreshesBaseStatus|TestRenameWorktree|TestSearchCyclesMatches' -count=1`
- Final verification:
  - `go test ./... -count=1` passes.
- Note:
  - This closeout used automated smoke coverage through representative integration tests. I did not run a manual live terminal session of `gx` in this turn.
