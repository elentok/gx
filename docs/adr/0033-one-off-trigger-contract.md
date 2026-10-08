# One-off trigger contract: one verb, no spool, a fixed exit-code table

External callers (cron, scripts, other agents) start work with one verb: `gx server one-off
"<prompt>"`. It creates a top-level ticket and queues it in one server call. When the server is not
running the call is refused and nothing is written, so a script always knows whether its work was
accepted. `--wait` turns the same call into a blocking one whose exit code says how the ticket ended.

## Exit codes

| Code | Meaning |
| ---- | ------- |
| 0 | submitted (no `--wait`), or done (with `--wait`) |
| 3 | needs-answer |
| 4 | needs-repair |
| 5 | cancelled |
| 6 | `--timeout` expired; the ticket keeps running |
| 7 | duplicate-live (no `--wait`) |
| 8 | server not running |

The numbers are stable: scripts depend on them. 1 and 2 are left to generic failure and usage
errors, and 126+ to the shell. With `--wait`, a duplicate-live refusal waits on the existing ticket
and uses the table above instead of 7.

## Considered Options

- **A spool directory the server drains later** — lets a submit succeed while the server is down,
  but it needs a second source of truth, ordering rules and a way to report errors back to a caller
  that has already exited. A visible refusal is simpler and tells the caller to start the server.
- **A separate verb for waiting** (`submit` plus `wait <addr>`) — two calls leave a gap where a
  duplicate or a restart can lose the ticket. One verb with `--wait` also lets a duplicate-live
  refusal follow the ticket that already exists.
- **Auto-starting the server from the client** — hides a stopped daemon and races the launchd
  start path. A refusal with a `gx server start` hint is enough.

## Consequences

- A script that must not lose work checks for exit 8 and retries after starting the server.
- `--timeout` never cancels: the ticket outlives the waiter, and a later `--wait` can pick it up.
- Exit codes are only added, never renumbered.
