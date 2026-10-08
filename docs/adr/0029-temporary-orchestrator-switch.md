# A temporary global switch picks the in-process loop or the server, and the other refuses to claim

> Superseded: removed at the orchestrator-daemon cutover. The server is the only scheduler; the
> `orchestrator` config key and the TUI's in-process mode are gone. Kept as history.

While the server is being built, gx keeps both schedulers. A global `config.json` key,
`orchestrator: in-process | server`, selects one. The non-selected scheduler refuses to claim any
ticket.

**One selects, one refuses.** The in-process ralph-loop and the server never both claim. Each checks
the switch before a claim and stands down if it is not the selected one. The key is global only,
never a per-project key.

**Temporary by design.** The switch exists so gx, which builds itself daily, is never broken while
the server stages land. A single cutover ticket, written alongside the server work, deletes the
switch, the in-process loop and the per-repo Queue/Attach vocabulary. After that there is one
scheduler.

## Why

- **No dual claims.** Two schedulers reading the same tickets would double-launch iterations. A
  refusal at claim time is simpler and safer than coordinating them.
- **Rollback is one line.** Flipping the key back to `in-process` restores the old path while the
  server is still being proven. Cutover waits for a week without a rollback.
- **Rejected:** letting both run with a lock between them (a permanent coordination cost for a
  temporary state), and building the server behind a branch (gx could not use it to build itself).

## Consequences

- Every claim path checks the switch. A new claim path without the check is a bug.
- Live state does not migrate between modes: after a flip, re-enqueue by hand.
- The switch, its config key and this ADR's "refuses to claim" rule are removed at cutover; the ADR
  stays as history.
