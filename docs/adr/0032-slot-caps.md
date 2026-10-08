# Slot caps: one live agent per slot, no bypass, FIFO

The daemon runs one queue for every project, so it caps live agents. A **slot** is one live agent
(claimed, with a running pane), counted plainly. A node starts only when the global cap, its root's
cap and its project's cap (if set) all have room. Every kind of job shares these caps: one-offs,
`scratch` jobs and `type: investigate` nodes get no reserved slot and `--front` never overflows a
cap. Queue order is plain FIFO, with no fair-share between projects.

## Considered Options

- **Weighted slots** (by model or `expected_context_window`) — more precise, but cost control is
  the budget's job, and a plain count is easy to explain.
- **A reserved one-off slot, or `--front` exceeding the global cap by one** — protects one-offs from
  a long epic, but breaks "N means N". Starvation is already bounded by the per-root cap and by
  iteration turnover: a waiting `--front` node gets the next free slot. Revisit only if real waits
  hurt.
- **Fair-share between projects** — the per-root and optional project caps already protect a
  project from its neighbours.
- **A per-node parallelism override** — ordering is expressed with `blocked_by` instead.

## Consequences

- Lowering a cap never kills a running agent; it only stops new starts, at the next scheduling
  decision, without a restart.
- An `investigate` node takes the slot its parked parent freed.
- `max-concurrent-epics` is gone: a one-off is also a root, so counting epics measures nothing.
