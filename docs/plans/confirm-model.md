# Plan: Embeddable Confirm Modal (`ui/confirm`)

## Background

The worktrees model manages its own inline confirm modal via six fields in `Model`
(`confirmPrompt`, `confirmItems`, `confirmYes`, `confirmCmd`, `confirmSpinnerLabel`,
`confirmCancelMsg`) and a `modeConfirm` mode. This pattern is not reusable and
scatters confirm-related state across the parent model.

The existing `ui/confirm` package is a standalone CLI tool (`Run()` / `RunWithNerd()`)
that spawns its own `tea.NewProgram`. It is not embeddable.

The `ui/status` model has a similar inline confirm pattern (11 `confirm*` fields), and
`ui/log` has a typed `rebaseConfirmState` struct. These are intentionally left out of
this migration: the status fields carry action-specific payload (paths, patch, remote,
branch) rather than a generic closure, making the refactor a larger and separate
concern; the log struct is already well-encapsulated.

## Decisions

### Scope: worktrees only

Only `ui/worktrees` is migrated in this plan. Status and log are out of scope because
their confirm patterns are fundamentally different (status stores action payload rather
than a closure; log already has a typed struct). Either can be revisited independently.

### Move `ui/confirm` → `cli/confirm`

The existing `ui/confirm` package belongs at the CLI layer, not the UI layer — it runs
its own `tea.NewProgram` and is only called from `cmd/cmd.go`. Moving it to
`cli/confirm` makes that boundary explicit and frees `ui/confirm` for the new
embeddable model. Only two import sites in `cmd/cmd.go` need updating.

### New `ui/confirm.Model` — embeddable sub-model

Follows the `pull.Model` pattern: `IsOpen bool`, `Open(opts)`, `Update(msg)`, `View(width)`.
The parent routes key events to the model when `IsOpen` and acts on the returned `Result`.

### Options struct (not option functions)

The variadic-option-function pattern is best suited for public APIs with a large or
open-ended option set. This is an internal package with exactly three optional fields.
A plain struct is simpler, more readable, and zero values are meaningful.

```go
type Options struct {
    Prompt       string
    Items        []string // optional bullet list rendered below the prompt
    AcceptCmd    tea.Cmd  // executed when the user confirms
    SpinnerLabel string   // returned in Result so the parent can start its own spinner
    CancelMsg    string   // emitted as notify.Info when the user cancels
}
```

`AcceptCmd` (not `Cmd`) makes the field's trigger condition explicit at the call site.

### Spinner stays in the parent

The worktrees spinner is a shared resource used by fetch, delete, push, and other
operations — not exclusively by the confirm flow. `confirm.Model` does not own a
spinner. Instead, `SpinnerLabel` is stored in `Options` and echoed back in `Result`
so the parent can start its own spinner after the user confirms.

### CancelMsg and SpinnerLabel flow through Options → Result / Cmd

`confirm.Model` stores both internally from `Options`. On accept it returns the
`AcceptCmd` as `tea.Cmd` and `Result{Accepted: true, SpinnerLabel: ...}`. On cancel it
returns `notify.Info(CancelMsg)` as `tea.Cmd` (empty string = nil cmd). This makes the
parent's response to any outcome uniform:

```go
next, cmd, result := m.confirm.Update(msg)
m.confirm = next
if result.Done {
    m.mode = modeNormal
    if result.Accepted && result.SpinnerLabel != "" {
        m.spinnerActive = true
        m.spinnerLabel = result.SpinnerLabel
        return m, tea.Batch(cmd, m.spinner.Tick)
    }
    return m, cmd
}
return m, cmd
```

### `modeConfirm` replaced by `m.confirm.IsOpen`

Removes one entry from the mode enum. The parent checks `m.confirm.IsOpen` to decide
whether to route events to the confirm model, consistent with how `m.pull.IsOpen` is
used today.

## API

```go
package confirm

type Options struct {
    Prompt       string
    Items        []string
    AcceptCmd    tea.Cmd
    SpinnerLabel string
    CancelMsg    string
}

type Result struct {
    Done         bool
    Accepted     bool
    SpinnerLabel string
}

type Model struct {
    IsOpen bool
    // unexported fields
}

func New() Model
func (m Model) Open(opts Options) Model
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Result)
func (m Model) View(width int) string
```

## Tasks

- [x] Move `ui/confirm` → `cli/confirm`; update imports in `cmd/cmd.go`
- [x] Create `ui/confirm` package with `Model`, `Options`, `Result`
- [x] Replace the 6 `confirm*` fields in `ui/worktrees/model_state.go` with `confirm confirm.Model`
- [x] Remove `modeConfirm` from the mode enum; replace checks with `m.confirm.IsOpen`
- [x] Replace `model_confirm_modal.go` with delegation to `m.confirm.Open` / `m.confirm.Update` / `m.confirm.View`
- [x] Delete `ui/worktrees/model_confirm_modal.go`
- [x] Update all `enterConfirm` / `enterConfirmWithCancel` call sites in worktrees
- [x] Verify tests pass
