# Codex runs on the PTY runner

> Amends ADR [0043](0043-auto-picks-the-pty-runner.md): its rule that a Codex ticket on a native
> runner runs on herdr.
>
> Superseded by ADR [0048](0048-herdr-is-a-multiplexer-not-an-agent-runner.md): the herdr runner is
> removed, so its clause that herdr hosts Codex goes.

Codex runs on the PTY runner under `agent-runner: auto`, `pty` and `headless`. The headless runner
hosts only Claude, so a Codex launch under `headless` runs on the PTY runner too. herdr hosts Codex
only under `agent-runner: herdr`, with a reduced flow: no live reply, no approval cards and no
resume.

Why: the PTY runner is where gx gets its Codex signals (OSC 7501 from the hooks gx installs), its
input guard and its approval cards. herdr can show a Codex pane, but gx cannot read those signals
through it.

Iterations already running Codex on herdr keep herdr until they end. There is no config migration.
