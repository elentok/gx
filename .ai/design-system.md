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
- Do not hand-roll borders, padding, or hint rows in screen packages unless there is a strong screen-specific reason

3. Reuse shared components for common interaction patterns

- confirm: `ui/components/modal_confirm.go`
- menu: `ui/components/modal_menu.go`
- input: `ui/components/modal_input.go`
- output/log modal: `ui/components/modal_output.go`
- checklist: `ui/components/checklist.go`

4. Use semantic icons, not raw glyphs

- Start from `ui.Icons(useNerdFont)`
- Add new icon roles there if needed
- Do not scatter raw nerd-font/private-use glyphs across screens unless there is no shared role yet

5. Keep CLI and TUI conceptually aligned

- If CLI and TUI both expose a menu, confirm, output view, or status message, they should share wording and interaction semantics where practical
- They do not need identical shells, but they should feel like the same product

6. Keep screen-local code for domain rendering only

Acceptable screen-local logic:

- status rows and diff rendering
- worktree tables and sidebar content
- domain-specific titles and state decisions

Not acceptable by default:

- custom menu rendering
- custom modal hint formats
- one-off success/error wording for common actions
- duplicate keybinding copy

## Preferred Patterns

### Hints

- Good: derive from `key.Binding` and `ui.RenderInlineBindings(...)`
- Bad: hard-coded strings like `"j/k navigate · enter select · esc cancel"`

### Status messages

- Good: `ui.StatusWithHints(ui.MessageComplete("push"), keys.Logs)`
- Bad: ad hoc concatenation like `"Pushed" + "  ·  o  view output"`

### Modals

- Good: call a shared component or `ui.RenderModalFrame(...)`
- Bad: assembling bordered modal text inline in a screen package

### Icons

- Good: `ui.Icons(useNerd).Search`
- Bad: embedding `"󰍉"` directly in screen code

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
