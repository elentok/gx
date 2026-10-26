# AI Rules

## Reading files

The harness elides the middle of a long `Read` ("... (N lines omitted)"), and code you did not see
gets edited blind. Read a whole file only when it is under 150 lines. Otherwise run `rg -n` first,
then `Read` with `offset` and `limit` around the match. Use line numbers or symbols the ticket
already names.

## Agent skills

### Issue tracker

Issues live as local markdown files outside the repo. There is no `.scratch/` directory and no
`gx tickets list` command. Use these instead:

- `gx tickets root` prints the store path.
- `gx tickets epics` lists the epics.
- `gx tickets show <addr>` shows a ticket or an epic.

See `~/.dotfiles/core/ai/skills/setup-elentok-skills/issue-tracker-local.md`.

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
  For the whole package, run `make test-server`: it shards the package across 5 processes, about
  2 min.
- Tests must never read the developer's real Claude config (their `editorMode: vim` changes
  outcomes). Call `claudeenv.Isolate()` (`testutil/claudeenv`) from every package's `TestMain`. It
  points `HOME` and a clean `CLAUDE_CONFIG_DIR` at a temp dir, unsets `XDG_*`, and leaves a vim-mode
  `~/.claude.json` as a canary: code that reads `HOME` instead of `CLAUDE_CONFIG_DIR` fails loudly.
- Run web tests through `make web-test`. gx installs dependencies when it creates an iteration
  worktree, so web tests work in a fresh worktree. `make web-test` still installs first, at the cost
  of a slower run.
- Unix socket paths are capped near 100 bytes, and `t.TempDir()` can exceed that. Holder-socket
  tests (anything that binds `holder.sock`) need a short `TMPDIR` or a short state root; use
  `shortTempDir` in `nativerunner` tests. The failure is `bind: invalid argument`.
- Tests never read live machine state, such as sibling worktrees, real branches or the live
  server's state. Use `t.TempDir()` and `testutil.TempRepo`.
- If a test run moves to the background, wait for its completion notice. Don't poll its output
  file, and don't write `sleep` or `until ...; do sleep` loops.

## Adding a server route

A new route needs all of these. A missing one fails a test, not the build:

- `routeTable` in `server/server.go`.
- A CLI verb: `routeVerbs` in `cmd/server_parity_test.go` plus the command itself
  (`TestRouteTableHasCLIVerbs`).
- The web mux in `server/web.go` when the browser calls it (`webWrites` for writes).
- The allowlist in `TestWeb_EverythingElseIs404` (`server/web_test.go`).

Server tests: build fixtures with `servertest.WriteTicketWith` (`server/servertest/servertest.go`)
instead of hand-written helpers. A web test that needs a project repo also needs
`SetProjectRepoNoAutoMerge`; `newFFFixture` does not provide one.

`servertest` fake agents idle without sending a report. New idle-path or finish-path behaviour (a
stall nudge, say) makes every fake agent hang until the test times out. Give it an off switch that
is off by default in `servertest`.

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
- `testutil/gxfixture` — e2e fixture program: `gxfixture <ready|mixed|finishing|edge>` starts an
  isolated real `gx server` (pty runner, `fakeclaude` as `claude`) and prints JSON with `web_url`,
  socket, state dir and hold file; SIGTERM stops it. Per-ticket worker behaviour
  (finish/ask/fail) lives in `.agentfake-worker.json`, see `scenarios.go`.

Run `go list ./...` for the full list.

## File size

Keep a Go source file under about 1000 lines. When one grows past that, move a cohesive group of
declarations into a new file in the same package (for example `server/sessionstate.go` out of
`server/sessions.go`). Moves only: don't change behaviour in the same commit.

## Core principle: gx must be visually beautiful

Beauty is a requirement, not polish. A feature that works but looks cluttered is not done.

- Before adding UI, check how it sits next to what is already on screen (headers, footer, panels).
- Prefer fewer visual elements: color and weight over boxes, pills and extra glyphs.
- Show only what the person needs right now. Move details (pids, versions, debug info) to the help
  dialog or a modal.
- When unsure, render the screen (e.g. in tmux) and look at it before calling the work done.

## Web UI

Before adding or changing a page or component in `webui/`, read `webui/README.md`. Two rules apply
to every page:

- A page that shows server data repaints from a `sessionStorage` copy after a reload
  (`webui/src/pageCache.ts`).
- A page without data yet shows the centered `Spinner`, never skeleton placeholders.

Web checks:

- Type-check with `gx brief tsc`, not a hand-built `pnpm exec tsc ... | tail`.
- The web UI does not use prettier. Never run it: it reformats unrelated lines.
- Run e2e with `make web-e2e`. It rebuilds the web bundle first. `pnpm e2e` does not rebuild.

## UI

- See ./design-system.md for UI work
- Don't show any keymaps on the statusbar, only "? help"
- Whenever possible prefer using the LSP (gopls-mcp) over grepping
- Run Go tests per package (`go test ./<pkg>/...`), never `go test ./...`: the whole repo is slow
  and loud, and has caused context blowups. Never add `-v`.
- Transient feedback uses `ui/notify` (not `m.statusMsg`): emit `notify.Info/Success/Warning/Error/Progress()` as `tea.Cmd`
- Avoid creating `New()` functions and prefer more descriptive functions, e.g.
  `log.NewModel()`, `keys.NewManager()`, ...
