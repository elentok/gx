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
  full suite runs in CI (`.github/workflows/ci.yml`, on pushes and PRs to `main`): `make test`
  (`scripts/test.sh`: race, other Go, server and web unit tests side by side), `make web-e2e`, and
  `go test ./e2e/...`. `make test` shares one CPU budget between its steps, half the cores by
  default; set `GX_TEST_CPUS=<n>` to change it. Every `gx brief go-test` run caps itself the same
  way (GOMAXPROCS at two fifths of that budget) unless the environment already sets GOMAXPROCS.
- Once a targeted run passes, stop. Don't rerun the full suite to double-check.
- Verify with `gx check` in the foreground, unpiped. Iterate with `gx brief`. Never pipe, `tail` or
  background either.
- `server/` is the slowest package. Run it only when you touched `server/` or
  `server/servertest/`, and scope it with `-run` while iterating. A plain `go test ./server` runs
  the tests in parallel and takes about 2 minutes.
- Tests must never read the developer's real Claude config (their `editorMode: vim` changes
  outcomes). Pass `HOME`, `CLAUDE_CONFIG_DIR` and the rest through config (`claudeenv.In(home)`
  returns the env entries for `server.Config.Env` or `exec.Cmd.Env`, with a vim-mode `~/.claude.json`
  as a canary: code that reads `HOME` instead of `CLAUDE_CONFIG_DIR` fails loudly). Never use
  `t.Setenv` or `os.Setenv` for these. `envguard/envguard_test.go` bans new process-env
  reads of `HOME`, `PATH`, `XDG_*`, `CLAUDE_CONFIG_DIR` and `CODEX_HOME`, `os.UserHomeDir()` and
  `exec.Command` without a `.Env =`; its allowlist lists what is left.
- Run web tests through `make web-test`. gx installs dependencies when it creates an iteration
  worktree, so web tests work in a fresh worktree. `make web-test` still installs first, at the cost
  of a slower run.
- `gx brief` tools: `go-test`, `go-build`, `go-vet`, `vitest`, `tsc`, `playwright`, `conflicts`.
  Any other name exits 2 with `unknown tool`. Use `gx brief go-build ./...` and
  `gx brief go-vet ./cmd`, never `go build ./... | tail`.
- Quiet web runs, from anywhere in the repo (always pass `-C webui`; the shell cwd persists between
  calls, so never `cd webui`):
  - `gx brief -C webui vitest src/epic`
  - `gx brief -C webui tsc`
  - e2e flows: `gx brief -C webui playwright disk.e2e.ts 76 77`. Bare flow numbers become one `-g`
    pattern. Never write `-g "^76\."`: Playwright matches `-g` against `[project] › file › title`,
    so a `^` anchor finds no tests. It adds the config and the line reporter, and runs
    `make web-build` only when `webui/dist/index.html` is missing.
- Unix socket paths are capped near 100 bytes, and `t.TempDir()` can exceed that. Holder-socket
  tests (anything that binds `holder.sock`) need a short `TMPDIR` or a short state root; use
  `shortTempDir` in `nativerunner` tests. The failure is `bind: invalid argument`.
- Tests never read live machine state, such as sibling worktrees, real branches or the live
  server's state. Use `t.TempDir()` and `testutil.TempRepo`.

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

## Pauses

Pause only when gx cannot recover on its own. Every pause needs a web button that ends it, never
a gx CLI step. A new pause kind needs a row in the pause-to-verb table. See
[ADR 0042](../docs/adr/0042-few-pauses-all-recoverable-from-the-web.md).

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
  socket, state dir, hold file and `log_file` (the server's stdout+stderr, which holds the cleanup
  command's output tail); SIGTERM stops it. Per-ticket worker behaviour
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

Testing a click on a Chakra menu item needs `waitFor`. See "Testing a Chakra menu" in
`webui/README.md`.

Web checks:

- Type-check with `gx brief tsc`, not a hand-built `pnpm exec tsc ... | tail`.
- The web UI does not use prettier. Never run it: it reformats unrelated lines.
- Regenerate `webui/src/generated/server.ts` with `make web-types`. tygo cannot resolve a constant
  or alias from another package (it emits `any`), so give wire types in `server/` literal values
  and map cross-package types in `tygo.yaml` `type_mappings`. Check `git diff` for `any`.
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
