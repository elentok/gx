# The ticket store: markdown is truth, the server is an index

Tickets used to live in each repo's `<repo>/.bare/.scratch`. They now live in one global,
git-backed **ticket store**, `<store>/<project>/<epic>/…`. The format is still plain markdown with
frontmatter, so hand-editing keeps working.

**Everything a ticket file can say, the file is authoritative for.** Any state the server keeps
(the queue, a scan of the tickets) is an index: it is rebuilt by rescanning, and on a conflict the
markdown wins. The server owns only what markdown never holds — queue order and membership, live
iteration handles, locks.

## Why

- **One namespace.** `project:epic/06` needs a single root; per-repo `.scratch` directories gave
  every project its own.
- **History is protected.** Dozens of epics of tickets and event logs are the most valuable data gx
  has. They no longer sit in a working tree that a `git clean` or a worktree removal can take.
- **No lock-in.** Rejected a database that markdown is projected from: it would end hand-editing and
  make every external edit a sync problem.

## Consequences

- Only gx commits the store. Agents, the CLI and humans never run git in it, so there are no commit
  races; edits made while gx is down are committed on the next start.
- Agents touch tickets through `gx tickets` verbs, by address, never by path.
- Rescans must tolerate a stale index. A watch speeds up noticing external edits; the poll
  guarantees it (ADR 0025). Claiming re-reads the file, so a missed event can delay a schedule but
  never produce a wrong one.
- Queue state, locks and `gx.log` are server-owned and live in the per-OS state dir, never in the
  store.
