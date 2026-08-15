# The scratch watch is an optimization; the scratch poll is the guarantee

`.scratch` is written by processes gx cannot observe through the repo epoch: another gx, a
ralph-loop running in a sibling worktree, or a hand edit. The Tickets and Queue tabs therefore
reloaded on a blind 2-second timer. That timer is a real cost — it never backs off, and it fires
whether or not anything changed.

We now watch `.scratch` for filesystem events and reload on them, **and** keep a slow (30 s)
periodic reload. The watch is not a replacement for the poll. On macOS the watch is kqueue-based:
it watches directories that were open when the watch was registered, it is not recursive, and it
can silently miss events (new epic directories, fd exhaustion, unusual filesystems). A watcher that
fails to start at all is a supported state — gx logs once and runs on the poll alone.

So: **the watch decides how fast we notice a change; the poll decides whether we notice it at
all.** The poll is what makes "the Queue tab eventually shows the truth" a property rather than a
hope, which is why it stays even though it looks redundant next to a working watcher. Deleting it
because "we have events now" would turn a guaranteed convergence into an unbounded staleness bug
that only reproduces on someone else's filesystem.

## Consequences

- The 30 s interval is a correctness backstop, not a latency knob. Tuning it up is safe; deleting it
  is not.
- Watch events are debounced (300 ms trailing) and rate-limited to one reload per second, because a
  live ralph-loop run writes to `.scratch` continuously — without that ceiling, event-driven
  reloading is *more* expensive than the timer it replaced.
- Correctness tests must pass with the watcher disabled. If a test only passes when events fire, it
  is testing the optimization rather than the guarantee.
