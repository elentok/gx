# AI Rules

## Agent skills

### Issue tracker

Issues live as local markdown files under `.scratch/`. See
`~/.dotfiles/core/ai/skills/setup-elentok-skills/issue-tracker-local.md`.

### Domain docs

Single-context: `GLOSSARY.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.

## Core principle: gx must be visually beautiful

Beauty is a requirement, not polish. A feature that works but looks cluttered is not done.

- Before adding UI, check how it sits next to what is already on screen (headers, footer, panels).
- Prefer fewer visual elements: color and weight over boxes, pills and extra glyphs.
- Show only what the person needs right now. Move details (pids, versions, debug info) to the help
  dialog or a modal.
- When unsure, render the screen (e.g. in tmux) and look at it before calling the work done.

## UI

- See ./design-system.md for UI work
- Don't show any keymaps on the statusbar, only "? help"
- Whenever possible prefer using the LSP (gopls-mcp) over grepping
- Always prefix Go/shell commands with `rtk` (e.g. `rtk go test ./...`, `rtk go build ./...`,
  `rtk go vet ./...`) — raw `go test ./...` alone costs ~200K tokens in this repo and has caused
  multiple context blowups
- Transient feedback uses `ui/notify` (not `m.statusMsg`): emit `notify.Info/Success/Warning/Error/Progress()` as `tea.Cmd`
- Avoid creating `New()` functions and prefer more descriptive functions, e.g.
  `log.NewModel()`, `keys.NewManager()`, ...
