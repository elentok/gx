# Stage/WT Refactor Plan

## Goal

Unify duplicated modal/action plumbing between `gx stage` and `gx wt` so both screens share:

- confirm modal behavior
- output/error modal behavior
- URL opening behavior
- streaming command execution with live output and cancel support

while keeping existing UX behavior intact (or improving consistency).

## Accepted Direction

1. ErrorModal becomes a thin wrapper over OutputModal.
2. Add lightweight streaming command runner with cancel.
3. Do the second DRY pass and migrate `gx wt` to shared modal primitives.

## Scope

### In Scope

- Shared modal primitives for confirm/output/error.
- Shared URL opener helper (`open`/`xdg-open`).
- Shared lightweight streaming command runner:
  - live stdout/stderr capture
  - polling/consume API for TUI
  - cancellation support (`ctrl+c` in running overlay)
- Stage action flow updates:
  - pre-confirm push/rebase with specific prompts
  - PR URL confirm flow matching `gx wt`
- Worktrees migration to shared modal primitives for DRYness.

### Out of Scope

- Redesigning table/sidebar layout.
- Reworking git domain APIs beyond what is needed for integration.
- Changing keybindings beyond parity requirements.

## Architecture

### 1) Shared UI modal primitives

Create shared components under `ui/components`:

#### ConfirmModal (shared)

- State: prompt/body text + selected yes/no
- Behavior:
  - left/h or right/l toggles selection
  - y/n quick select
  - enter confirms selected choice
  - esc/q cancels
- Rendering: reusable bordered modal with consistent hint/footer

#### OutputModal (shared)

- State: title + content viewport + dismiss hint text
- Behavior:
  - scroll via viewport key updates
  - dismiss on esc/enter/q (configurable)
- Rendering: reusable bordered modal with title + viewport + hint

#### ErrorModal (thin wrapper over OutputModal)

- Uses OutputModal internals with:
  - red accent/border/title
  - optional "o view output" hint/action wiring where needed
- No separate duplicated rendering logic.

### 2) Shared runtime helper: streaming command runner

Add a small shared utility:

- Start command with args + working dir
- Stream stdout/stderr into internal buffer
- Expose:
  - `Consume()` incremental chunk API
  - `Result()` completion API
  - `Cancel()` kill process API
- Thread-safe access to mutable state
- Suitable for Bubble Tea polling loop (`tea.Tick`)

### 3) Shared URL opener helper

Extract platform-specific URL opener from `ui/worktrees/pullpush.go`:

- macOS: `open <url>`
- Linux: `xdg-open <url>`

Both stage and worktrees use this helper.

## Behavior parity requirements

### Stage (`gx stage`)

#### Push (`P`)

- Add pre-confirm: `Push branch <branch> to <remote>?`
- On success:
  - parse PR URL using existing git parsing behavior (`git.ExtractPRURL` / `git.PushBranch` semantics)
  - if URL found, show confirm:
    - `Open pull request page?`
    - `<url>`
    - default selection Yes
  - Yes => open URL via shared URL opener
- On non-fast-forward rejection:
  - show force-push confirm (existing behavior parity)

#### Rebase (`b`)

- Add pre-confirm: `Rebase branch <branch> on origin/master?`
- Preserve stashify behavior and `git fetch origin` first.
- Show live output overlay during run.
- On failure after stash:
  - confirm to pop stash.

#### Running overlay

- Use shared OutputModal + shared runner.
- `ctrl+c` cancels running command.
- Keep scroll + dismiss behavior consistent.

#### Amend (`A`)

- Keep preview/confirm behavior:
  - last commit subject
  - up to 10 changed filenames + `...`

### Worktrees (`gx wt`) DRY pass

- Migrate existing confirm modal implementation to shared ConfirmModal.
- Migrate logs/error modal rendering to shared OutputModal/ErrorModal wrapper.
- Keep user-visible behavior unchanged:
  - prompts
  - keybindings
  - status messages/spinner semantics
  - PR URL confirm/open flow

## Suggested implementation phases

### Phase 1: shared building blocks

- Add shared ConfirmModal, OutputModal, ErrorModal wrapper.
- Add shared URL opener helper.
- Add shared streaming runner.

### Phase 2: stage integration

- Replace stage-specific modal/runner code with shared components.
- Add push/rebase pre-confirms.
- Add PR URL confirm/open parity with worktrees.

### Phase 3: worktrees DRY migration

- Switch worktrees confirm/error/log modal code to shared components.
- Remove duplicated modal rendering/handling code.

### Phase 4: cleanup

- Delete obsolete screen-specific modal helpers.
- Tighten docs/help text where needed.
- Ensure no behavior regressions.

## Testing plan

### Unit tests

- ConfirmModal key handling transitions.
- OutputModal dismiss/scroll behavior.
- Runner:
  - incremental consume
  - completion result
  - cancellation path
- Stage push path:
  - pre-confirm text includes branch+remote
  - PR URL confirm appears when URL exists
- Stage rebase path:
  - pre-confirm text includes branch + `origin/master`

### Integration / E2E

- Stage:
  - `P` -> confirm -> run -> PR URL confirm
  - `b` -> confirm -> run
  - running overlay cancel via `ctrl+c`
- Worktrees:
  - smoke tests for confirm/error/log overlays after migration
  - push non-fast-forward force confirm still works
- Full workflow E2Es for stage push/pull/rebase actions:
  - create dummy repos and remotes
  - perform stage + commit in test setup
  - run pull/push/rebase through `gx stage`
  - fake editor in tests (for example with `GIT_EDITOR` script)
  - disable tmux behavior in tests where needed (for example unsetting `TMUX`)

### Regression suite

- `go test ./ui/stage -count=1`
- `go test ./ui/worktrees -count=1`
- `go test ./... -count=1`

## Risks and mitigations

- Risk: modal behavior drift during extraction.
  - Mitigation: preserve old keymap/hints and add focused tests before migration.
- Risk: runner cancellation edge cases.
  - Mitigation: explicit cancel tests + process cleanup checks.
- Risk: PR URL flow inconsistencies between stage and worktrees.
  - Mitigation: one shared parser/open helper and parity tests.

## Done criteria

- Stage and worktrees use shared modal primitives.
- Stage and worktrees use shared URL opener helper.
- Stage uses shared streaming runner and cancel works via `ctrl+c`.
- Stage push/rebase have specific pre-confirms.
- Stage PR URL confirm/open behavior matches worktrees exactly.
- Full test suite passes.
