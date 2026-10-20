# AI Rules

## Agent skills

### Issue tracker

Issues live as local markdown files under `.scratch/`. See
`~/.dotfiles/core/ai/skills/setup-elentok-skills/issue-tracker-local.md`.

### Domain docs

Single-context: `GLOSSARY.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.

## Testing

- Test the packages you touched plus their direct importers (find importers with the gopls-mcp
  references/dependency tools). Don't run `go test ./...` for a local change.
- ralph-loop's land step runs no tests: it only cherry-picks commits onto the feature branch. The
  full suite runs in CI (`.github/workflows/ci.yml`, on pushes and PRs to `main`): `go test ./...
  -timeout=10m`, `make test-race`, and `go test ./e2e/...`.
- Once a targeted run passes, stop. Don't rerun the full suite to double-check.
- While iterating, always pass `-timeout 60s` and `-run <pattern>`. A hung fake otherwise waits for
  the 10-minute default. Run the whole package once at the end. Also pass `-timeout` with `-count=N`
  or `-race`. `server/` is exempt from the 60s limit.
- Run packages one at a time. A multi-package run that moves to the background hides which package
  hangs.
- `server/` is the slowest package by far (minutes, close to the default `go test` timeout under
  load). Run it only when you touched `server/` or `server/servertest/`, and scope it with `-run`.
- Tests must never read the developer's real Claude config (their `editorMode: vim` changes
  outcomes). Call `claudeenv.Isolate()` (`testutil/claudeenv`) from every package's `TestMain`. It
  points `HOME` and a clean `CLAUDE_CONFIG_DIR` at a temp dir, unsets `XDG_*`, and leaves a vim-mode
  `~/.claude.json` as a canary: code that reads `HOME` instead of `CLAUDE_CONFIG_DIR` fails loudly.
- Run web tests only through `make web-test`. A fresh worktree has no `node_modules`, so a bare
  `npx vitest` fails with `Cannot find package '@vitejs/plugin-react'`. `make web-test` installs
  dependencies first, at the cost of a slower run.
- If a test run moves to the background, wait for its completion notice. Don't poll its output
  file, and don't write `sleep` or `until ...; do sleep` loops.

## Package map

There is no `internal/` directory. Top-level packages live at the repo root:

- `cmd/` — CLI commands (`gx tickets`, `gx agent`, ...)
- `server/` — the gx server; `server/servertest/` holds its test helpers
- `ralphloop/` — ralph-loop scheduler, landing and verification
- `tickets/`, `tickets/schema/` — local markdown ticket tracker
- `agentrunner/` — the agent-runner interface; `nativerunner/` — herdr-free runners
- `harness/` — what runs inside an interactive agent session
- `herdr/` — herdr CLI/socket client
- `runner/` — runs a child command with inherited stdio
- `git/` — git operations
- `ui/...` — TUI (`ui/app` is the root model; one package per tab or component)
- `e2e/` — end-to-end tests (slow, CI only)
- `testutil/` — fakes and helpers: `testutil/agentfake`, `testutil/herdrfake`,
  `testutil/runnerfake`, `testutil/herdrctl`, `testutil/teatestv2`, `testutil/claudeenv`

Run `rtk go list ./...` for the full list.

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
