# Design System Guide

Use this file when making UI changes in `gx`.

## Goal

Keep new UI work inside the shared design system instead of adding screen-local styling, hints, icons, or interaction patterns.

Read this alongside:

- `docs/design-system/spec.md`
- `docs/design-system/research.md`

This file is the short operational version for AI agents.

## Current Shape

The design system is centered in `ui/`:

- theme/style foundation: `ui/styles.go`
- semantic icons: `ui/icons.go`
- feedback and hint helpers: `ui/feedback.go`, `ui/keyhints.go`
- shared frames and overlays: `ui/frame.go`, `ui/overlay.go`
- shared interaction components: `ui/components/`
- **notifications**: `ui/notify/notify.go` — emit `notify.Info/Success/Warning/Error/Progress()` as
  tea.Cmd; handled and rendered by `app.Model` as a top-right overlay

Screen packages should compose these:

- `ui/status`
- `ui/worktrees`
- CLI flows in `cmd/`

## Rules

1. Reuse semantic helpers before adding strings

- Prefer `ui.RenderInlineBindings(...)` for key hints
- Prefer `ui.JoinStatus(...)` and `ui.StatusWithHints(...)` for status/footer messages
- Prefer `ui.MessageOpening(...)`, `ui.MessageClosed(...)`, `ui.MessageComplete(...)`, `ui.MessageAborted(...)`, `ui.MessageNoOutput()`

2. Reuse shared modal/panel surfaces

- Use `ui.RenderModalFrame(...)` and `ui.RenderPanelFrame(...)`
- Use `ui.OverlayCenter(...)` for centered modal overlays
- Use `ui.OverlayBottomCenter(bg, fg, screenW, y)` + `ui.RenderModalFrame(TitleInBorder: true)` for
  text-input overlays (horizontally centered, y from `settings.InputModalBottom.ResolveY`)
- Do not hand-roll borders, padding, or hint rows in screen packages unless there is a strong
  screen-specific reason

3. Reuse shared components for common interaction patterns

- confirm: `ui/components/modal_confirm.go`
- menu: `ui/components/modal_menu.go`
- input: `ui/components/modal_input.go` — for prompt-style inputs; for single-line text entry use
  `RenderModalFrame` with `TitleInBorder: true` and `OverlayBottomCenter`
- output/log modal: `ui/components/modal_output.go`
- checklist: `ui/components/checklist.go`

4. Use semantic icons, not raw glyphs

- Start from `ui.Icons(useNerdFont)`
- Add new icon roles there if needed
- Do not scatter raw nerd-font/private-use glyphs across screens unless there is no shared role yet
- **Icon editing**: Never use Write/Edit tools to rewrite `ui/icons.go` entirely — nerd font PUA characters (U+E000–U+F8FF) will be silently dropped. Use a Python script to insert them by Unicode codepoint: `python3 -c "icon = chr(0xF071); ..."`

5. Use the notification system for transient feedback

- Emit `notify.Info(msg)`, `notify.Success(msg)`, `notify.Warning(msg)`, `notify.Error(msg)`, `notify.Progress(label)` as `tea.Cmd` returns — never set `m.statusMsg` directly
- `notify.NotifyMsg` is only handled by `app.Model`; screen packages that instantiate themselves (e.g., in tests via `status.New()`) will not see notifications in their rendered output
- For E2E tests that bypass `app.Model`, verify side-effects via git state or model stability rather than notification text

6. Keep CLI and TUI conceptually aligned

- If CLI and TUI both expose a menu, confirm, output view, or status message, they should share wording and interaction semantics where practical
- They do not need identical shells, but they should feel like the same product

7. Keep screen-local code for domain rendering only

Acceptable screen-local logic:

- status rows and diff rendering
- worktree tables and sidebar content
- domain-specific titles and state decisions

Not acceptable by default:

- custom menu rendering
- custom modal hint formats
- one-off success/error wording for common actions
- duplicate keybinding copy

8. Extract repeated Lip Gloss styles into named variables

- If a style is reused or makes a render helper harder to read, extract it to a package-level variable with a semantic name
- Prefer `activeTabStyle`, `inactiveTabStyle`, `errorTitleStyle`, etc. over rebuilding the same `lipgloss.NewStyle()` chain inline
- Keep render helpers focused on structure and state decisions, not long styling chains

## Preferred Patterns

### Hints

- Good: derive from `key.Binding` and `ui.RenderInlineBindings(...)`
- Bad: hard-coded strings like `"j/k navigate · enter select · esc cancel"`

### Status messages (transient notifications)

- Good: `return m, notify.Info("pushed")` or `return m, notify.Success(ui.MessageComplete("push"))`
- Bad: `m.statusMsg = "pushed"` — the `statusMsg` field no longer exists on screen models
- Note: `notify.*()` commands are only rendered when routed through `app.Model`. Screen-level E2E tests (`status.New()`, `worktrees.New()`) will not display them.

### Modals

- Good: call a shared component or `ui.RenderModalFrame(...)`
- Bad: assembling bordered modal text inline in a screen package

### Text-input overlays

- Good: `ui.OverlayBottomCenter(bg, ui.RenderModalFrame(..., TitleInBorder: true), w, settings.InputModalBottom.ResolveY(h, fgH))`
- Bad: rendering the text input in the status bar or hand-rolling a bordered input box

### Icons

- Good: `ui.Icons(useNerd).Search`
- Bad: embedding `"󰍉"` directly in screen code

### Lip Gloss styles

- Good: define a named style variable once, then call `style.Render(...)`
- Bad: embedding long `lipgloss.NewStyle().Foreground(...).Background(...).Padding(...)` chains inline inside small render helpers

## If You Need Something New

When adding a new reusable UI idea:

1. Decide whether it belongs in:

- `ui/styles.go` / `ui/icons.go`
- `ui/feedback.go` / `ui/keyhints.go`
- `ui/frame.go` / `ui/overlay.go`
- `ui/components/`

2. Add it there first

3. Then update the screen package to consume it

Do not start by implementing it locally and “maybe extracting later”.

## Review Checklist

Before finishing UI work, check:

- Did I add any hard-coded hint strings that should use `RenderInlineBindings`?
- Did I add any raw glyphs that should come from `ui.Icons(...)`?
- Did I add any ad hoc status/copy that should use `ui/feedback.go`?
- Did I bypass `ui/components` for a common interaction pattern?
- Would the same change need to be made twice in `status` and `worktrees`?

If yes, the change probably belongs in the shared layer first.
