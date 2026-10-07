# Two native agent runners: headless by default, PTY by hand

Some organizations block herdr, so `gx server` hosts claude agents itself. We build two native agent
runners behind the same agent runner interface: the **headless runner** (`claude -p` stream-json)
and the **PTY runner** (interactive claude on a terminal gx owns). Headless is the default because
it reports status, blocked prompts, rate limits and compaction as structured events, and survives a
server restart with no extra process. The PTY runner exists only as a backup in case an org or a
claude release disables `-p`, so `agent_runner: auto` never picks it — a person sets
`agent_runner: pty` by hand. Both runners aim for the same feature parity; a gap is allowed only
where one runner cannot do it.

## Considered Options

- **Headless only** — less code, but one claude change (e.g. `--bare` becoming the `-p` default,
  or `-p` blocked by policy) would leave orgs without herdr with no runner at all.
- **PTY only** — screen-scraping fails silently when claude's UI changes, gx would take over herdr's
  rule upkeep, and restart survival needs a holder process per agent.
- **tmux** — another external tool, which this effort set out to avoid. Not needed while either
  native runner works.

## Consequences

- `gx claude doctor` runs a short canned session per runner, so a person can re-check gx's
  assumptions about claude after every claude upgrade.
- Agents launch with `DISABLE_AUTOUPDATER=1` and `--permission-mode auto`, and park `needs-repair`
  when a claude feature the runner depends on is missing.
