# Notifications Plan

## Decisions

- **State lives in `app.Model`** — sub-models emit `tea.Cmd` returning a `NotifyMsg`; `app.View()` renders the overlay (same pattern as chord hints today).
- **Stack** — all active notifications visible simultaneously, capped at 4.
- **Dismissal** — info/success/warning/error auto-dismiss after 5s. In-progress has no TTL; dismissed by replacing it with the same ID.
- **IDs** — stable string IDs chosen by the caller (e.g. `"push"`, `"pull"`). Only in-progress notifications need an ID.
- **Spinner** — single `bubbles/spinner` instance in `app.Model`; runs only when at least one in-progress notification is active.
- **Width** — content-driven, capped at 40 columns.
- **Warning icon** — `⚠` (unicode) / `` nerd font.

## Types

```go
type NotifyKind int
const (
    NotifyInfo NotifyKind = iota
    NotifySuccess   // green, ✔ / 
    NotifyWarning   // orange, ⚠ / 
    NotifyError     // red, ✘ / 󰅙
    NotifyProgress  // cyan, spinner
)
```

## API (new package `ui/notify`)

```go
// NotifyMsg is emitted by sub-models as a tea.Cmd result.
type NotifyMsg struct {
    ID      string     // optional; required for Progress, used for replace-by-ID
    Kind    NotifyKind
    Message string
}

func Info(msg string) tea.Cmd
func Success(msg string) tea.Cmd
func Warning(msg string) tea.Cmd
func Error(msg string) tea.Cmd
func Progress(id, msg string) tea.Cmd
func Close(id string) tea.Cmd  // explicitly close a progress notification
```

## `app.Model` changes

- Add `notifications []notification` field (each has ID, kind, message, expiresAt).
- Add `spinner spinner.Model` field.
- In `Update()`: handle `NotifyMsg` — push/replace by ID, set TTL for non-progress kinds, start spinner tick if needed.
- In `Update()`: handle `spinner.TickMsg` — advance frame, re-emit tick if any progress notification is still active.
- In `Update()`: handle `notifyTickMsg` — prune expired notifications.
- In `View()`: render notification stack via `OverlayTopRight` (adjusted to 2-column/2-row margin).

## Migration

- Remove `statusMsg string` and `statusUntil time.Time` fields from `ui/status`, `ui/worktrees`, `ui/commit`.
- Replace every `m.statusMsg = ...` with `return m, notify.Success(...)` / `notify.Error(...)` / etc.
- Remove `statusMessageTTL` constant and `statusTickCmd` in `ui/status`.
- Update footer rendering in each view to drop the status text section.
- Update tests: replace `m.statusMsg` assertions with inspecting the returned `tea.Cmd` or the rendered overlay.

## Tasks

- [ ] Create `ui/notify` package with `NotifyMsg`, kind constants, and helper constructors
- [ ] Add `IconSet` entries for warning (`⚠` / ``) and spinner placeholder
- [ ] Add `notifications` + `spinner` fields to `app.Model`; handle `NotifyMsg` in `app.Update()`
- [ ] Render notification stack overlay in `app.View()` (top-right, 2-col/2-row margin, max 40 cols, cap 4)
- [ ] Wire spinner tick into `app.Model`
- [ ] Migrate `ui/status` — remove `statusMsg`/`statusUntil`, emit `NotifyMsg` commands
- [ ] Migrate `ui/worktrees` — remove `statusMsg`, emit `NotifyMsg` commands
- [ ] Migrate `ui/commit` — remove `statusMsg`, emit `NotifyMsg` commands
- [ ] Update all affected tests
- [ ] Verify no remaining `m.statusMsg =` assignments (DoD)
