# Writes split by kind: content goes direct to the file, orchestration goes through the server

Every write to a ticket is one of two kinds, and the kind decides the path.

**Direct write.** Content writes (`gx tickets show|add|section|set`) edit the markdown file under a
per-ticket lock and then ping the server so it re-reads. They work with the server down. The JSON
result carries `"via": "direct"`.

**Server write.** Orchestration writes (enqueue, dequeue, pause, drain, unpark, budget override)
change scheduling, so only the server performs them. The server writes the file first, then updates
its own state, so markdown stays the truth (ADR 0026). The JSON result carries `"via": "server"` and
an `actor`. With the server down the CLI refuses with `server-not-running` (ADR 0028's refusal
contract); it never falls back to a direct write.

**The CLI namespace shows the path.** `gx tickets …` is direct content; `gx server …` is the
server. A caller can tell from the command which path a write takes.

**Repair verbs are the exception.** `gx tickets verify|land|reset|unpark` share one package and run
direct under the land lock when the server is down, so recovery works when it is needed most.

## Why

- **Content must not depend on a process.** Agents edit tickets mid-iteration; a down server must
  not block them.
- **Scheduling needs one writer.** Two writers to queue order or claim state would race. Routing
  those through the server keeps a single decider.
- **Markdown first** keeps the server an index that a restart can rebuild.
- **Rejected:** all writes through the server (agents stall when it is down) and all writes direct
  (the scheduler's decisions get overwritten).

## Consequences

- A new write verb must declare its kind; the route/CLI parity test (ADR 0028) covers the server
  half.
- A direct write that changes scheduling-relevant fields is picked up on the ping, not applied
  instantly; the server re-reads and decides.
- `via` and `actor` are a public contract in the JSON output.
