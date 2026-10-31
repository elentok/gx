# `agent-runner: auto` picks the PTY runner

> Supersedes in part ADR [0031](0031-two-native-agent-runners.md): its rule that `auto` never picks
> the PTY runner and that headless is the default.
>
> Amended in part by ADR [0044](0044-codex-runs-on-the-pty-runner.md): a Codex ticket now runs on
> the PTY runner, not herdr.

`agent-runner: auto` now resolves to the PTY runner whenever claude is on PATH. It no longer pings
herdr. A person can still ask for the others by hand:

- `agent-runner: herdr` brings herdr back. herdr down is an error, not a fallback.
- `agent-runner: headless` picks the headless runner.
- `auto` with claude missing is an error that names claude and suggests `agent-runner: herdr`, and
  `gx server` refuses to start.
- A Codex ticket on a native runner runs on herdr, because the native runners host only claude.

Why: the PTY runner is the one that lets a person open a running Queue agent, watch it and, after a
confirm, type into it. Headless and herdr stay available for the cases that need them, but neither
is picked for a person who set nothing.

Iterations already running on herdr keep herdr until they end. There is no config migration.
