# Plan: Amend Specific Commit from Log / Commit View

## Summary

Add an `A` keybinding to the log view and commit view that lets the user amend a
specific commit with the currently staged changes.

## Design Decisions

### Git mechanism
- If target commit **is HEAD**: `git commit --amend --no-edit`
- If target commit **is not HEAD**:
  1. Auto-stash any unstaged worktree changes (if present)
  2. `git commit --fixup=<hash>`
  3. `GIT_SEQUENCE_EDITOR=true git rebase -i --autosquash <hash>^`
  4. Pop stash (if stashed)

### Preconditions
- Nothing staged → show error status message, abort (no modal)

### `ui/amend` package — full orchestrator

`amend.Model` is a domain-aware UI component that owns the entire amend lifecycle:

1. **Confirm phase**: shows commit info + staged files + pushed warning; user picks yes/no
2. **Running phase**: modal stays open; buttons hidden; Steps component shows live progress

`amend.Model` is responsible for:
- Calling git at `Open()` time to check HEAD status and uncommitted changes, then building `[]Step`
- Emitting the first step `tea.Cmd` when the user accepts
- Handling step result messages internally, advancing the step state and emitting the next cmd
- Managing the spinner tick

`amend.Model.Update` signature matches `search.Model`:
```go
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Result)
```

`IsOpen` stays `true` for the full lifecycle (confirm + running). The parent overlays
`m.amendConfirm.View(width)` whenever `m.amendConfirm.IsOpen`.

```go
type Result struct {
    Decided  bool  // user made a yes/no choice (confirm phase)
    Accepted bool  // user chose yes
    Done     bool  // running phase finished (success or failure)
    Err      error // non-nil if a step failed
}
```

### Steps component — `ui/amend/steps.go`

```go
type Step struct {
    TitleBefore  string // shown while pending   e.g. "rebase"
    RunningTitle string // shown while running   e.g. "rebasing..."
    TitleAfter   string // shown when done       e.g. "rebased"
    TitleFailed  string // shown when failed     e.g. "rebase failed"
    IsRunning    bool
    IsDone       bool
    HasFailed    bool
}
```

Rendering:
```
{spinner} creating fixup commit...    ← IsRunning
{empty checkbox} rebase               ← pending

{checked checkbox} created fixup commit  ← IsDone
{spinner} rebasing...                    ← IsRunning

{checked checkbox} created fixup commit  ← IsDone
{✗ checkbox} rebase failed               ← HasFailed (modal stays open, user dismisses)
```

Spinner: reuse `charm.land/bubbles/v2/spinner` (already used in worktrees).

### Failure handling
Surface the failed step inline in the Steps view (mark step `HasFailed`). Modal stays
open; user presses esc/enter to dismiss.

### Pushed-commit detection
`git branch -r --contains <hash>` — if any remote ref contains the hash, show the warning.
`git.IsCommitPushed(root, hash string) (bool, error)` — already implemented.

### Conflict handling
Surface stderr in the failed step; leave the repo in the conflicted state. No auto-abort.

### After success
- Log view: reload + try to focus the amended commit by subject match; fall back to top
- Commit view: navigate to log (`nav.Replace(nav.RouteLog)`)

### Parent routing

Both log and commit parent models route all messages through `amend.Model.Update` while
`m.amendConfirm.IsOpen`, then act on `result.Done`:

```go
// in Update():
if m.amendConfirm.IsOpen {
    next, cmd, result := m.amendConfirm.Update(msg)
    m.amendConfirm = next
    if result.Done {
        return m.handleAmendDone(result.Err)
    }
    return m, cmd
}
```

## Implementation Tasks

### Already done
- [x] `git.IsCommitPushed(root, hash string) (bool, error)`
- [x] `git.StagedFiles(root string) ([]string, error)`
- [x] `ui.StyleWarning` in styles.go
- [x] `ui/amend` package skeleton (`amend.Model`, confirm view, basic `Open`/`Update`/`View`)
- [x] Wire `A` binding in commit view
- [x] Wire `A` binding in log view
- [x] After success: navigate/reload + focus by subject

### Remaining
- [ ] `ui/amend/steps.go` — `Step` type + `Steps` render function
- [ ] `amend.Model`: store `worktreeRoot`, embed spinner
- [ ] `amend.Model.Open()`: check HEAD + uncommitted changes, build `[]Step`
- [ ] `amend.Model.Update()`: new signature `(tea.Msg) (Model, tea.Cmd, Result)`
  - confirm phase key handling → on accept, emit step 1 cmd
  - step result message handling → advance steps, emit next cmd
  - spinner tick handling
- [ ] `amend.Model.View()`: confirm view (confirm phase) vs steps view (running phase)
- [ ] `git.AmendSpecific` refactor: split into per-step functions used by amend.Model
- [ ] `amend.Result` struct with `{Decided, Accepted, Done, Err}`
- [ ] Update parent routing in log and commit models to new `amend.Model.Update` signature
- [ ] Remove `amendRunning` from parent models (now owned by `amend.Model`)
- [ ] Update CHANGELOG
