# herdr is a multiplexer, not an agent runner

> Supersedes ADRs [0031](0031-two-native-agent-runners.md), [0043](0043-auto-picks-the-pty-runner.md)
> and [0044](0044-codex-runs-on-the-pty-runner.md): the herdr runner is removed. The PTY runner as
> the default stands.

The herdr runner is gone. herdr stays only as a terminal multiplexer: Split launch, the worktrees
tab "open herdr workspace", and the Ping gate before the TUI multiplexer menu. The `herdr/` package
keeps `CLI`, `FindOrCreateWorkspace` (and `FindWorkspace`) and `Ping`.

Why: nobody runs agents on herdr. Its code paths, pause kinds, banners and docs made investigations
harder.

- `agent-runner: herdr` in a config reads as `auto` with a one-line warning, at start and at each
  reload that reads it. The settings API stores a `herdr` write as `auto`. The settings page shows
  `auto` and the same warning. One normaliser in `agentrunner` serves startup, the settings API and
  the settings reload.
- A run whose recorded runner cannot be built is a gone agent, not an error. An empty claim reopens
  (`claim-reopened`). A claim that holds work pauses `handle-mismatch` with a reason that names the
  removed runner, and Retry or Start over ends it. There is no auto-restart, and no code discards a
  worktree that holds work. An empty runner name in an old record reads as gone, not as herdr.
- `agent_pane_busy` leaves the pause kinds and the ticket pause reasons. A named migration step
  rewrites it to `iteration-error`, and the legacy read shim maps it for old run logs. The event
  types `herdr-unavailable` and `herdr-available`, the `herdr-agent-lost` marker and the herdr
  outage fold are deleted. Old log lines still load because readers do not validate kinds.
  `agent_name_taken`, `agent_prompt_stalled`, `handle-mismatch` and `blocked-pane` stay, because the
  PTY and headless runners also raise them.
- The herdr e2e suite, the `herdr-e2e` CI job and `make e2e` are gone. The TUI behaviour only they
  covered is tested in-process and by a small real-binary suite on the PTY runner in `make test`.
  `testutil/herdrfake` is gone, and the `herdr/` tests use a shell-script stub.

What each superseded ADR loses:

- 0031: the "Some organizations block herdr" framing and headless-by-default are already history.
- 0043: `auto` picks the PTY runner, and its herdr-by-hand options go.
- 0044: Codex runs on the PTY runner, and its clause that herdr hosts Codex goes.
