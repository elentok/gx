# The server API has one verb route per CLI verb, refusals are results, and state is snapshot + stream

The server's HTTP API (`/v1/...`) is shaped like the CLI: every route has a `gx server …` verb with
`--json`, and the verb is a thin client of the route.

**Verb routes.** One route per verb (`GET /v1/queue`, `POST /v1/queue/add`, `GET /v1/tickets/explain`),
not a generic resource or RPC endpoint. The route table in `server/server.go` is the single list;
a test in `cmd` diffs it against the CLI command tree, so a route without a verb (or the reverse)
fails the build.

**Refusals are results.** A write the server declines because of state or input (unknown ticket,
already queued, bad position) answers `200` with `refused: true`, a stable `reason` and a
`message`. Clients switch on `reason`. HTTP errors are for transport and bugs only: bad JSON is
`400`, a failed persist is `500`.

**Snapshot, then stream.** A client reads `GET /v1/snapshot` (full state plus `seq`), then follows
`GET /v1/events?since=<seq>` (SSE). The server keeps a bounded in-memory window of recent events;
a `since` older than the window answers `410`, which means "take a new snapshot".

## Why

- **One vocabulary.** Agents, scripts and the TUI use the same verbs the human types, so the CLI
  is documentation for the API and the API is testable through the CLI.
- **Parity is checkable.** A fixed route table can be diffed mechanically; a generic endpoint
  cannot.
- **Refusals are expected outcomes.** Mapping them to HTTP errors makes callers tell "the server
  said no" from "the server broke". A result with a reason keeps the two apart.
- **Snapshot + stream** lets the TUI render without polling and recover from any gap with one
  rule.
- Rejected: pushing the full state on every event (large, wasteful) and a persisted event window
  (the ticket markdown is the truth, ADR 0026, so a restart just forces a re-snapshot).

## Consequences

- Adding a route means adding a CLI verb in the same change.
- Refusal `reason` strings are a public contract. Renaming one breaks clients.
- The TCP listener has no auth, so it binds to loopback addresses only.
- Clients must handle `410` by re-snapshotting, never by retrying the same `since`.
