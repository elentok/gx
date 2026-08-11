# `MaxConcurrentEpics` bounds the agents gx *starts*, not the agents that are running

When an iteration parks because its agent is blocked on an interactive prompt in its pane, gx
**releases the epic's concurrency permit**, and on recovery re-acquires it **try-first: if no slot is
free, it logs the fact and proceeds over-subscribed by one** rather than blocking. That reframes what
the limit means. It is not an invariant over how many agents are alive — it is a gate on how many
epics gx will *launch work into*. A parked iteration has an agent sitting idle at a question; it
costs a slot and consumes nothing.

The alternative was the behaviour that already existed for Codex, where `waitForAttentionRecovery`
pauses the epic's scheduler gate but never touches `opts.Permit`. Ticket 01 confirmed what that costs
in the Claude case: a blocked pane never satisfies the `until` list, `FinishTimeoutMs` is zero on the
normal iteration path, so the loop re-polls forever and the `MaxConcurrentEpics` permit is held with
**no upper bound** — one unanswered question starves every other epic for as long as the human is
away. A gate that only made that starvation *visible* would be a cosmetic fix to the problem this
whole effort exists to solve.

Blocking on re-acquisition was rejected for a sharper reason than latency: **it would be a lie**. The
pane stays live across the park, so the instant the human answers, the Claude agent resumes working
whether or not gx holds a permit. A blocking `Acquire` would leave gx waiting for a slot to describe
an agent that is already back at work, and refusing the slot would not reduce the load by one byte —
it would only make gx's accounting disagree with the machine. Over-subscribing by one and saying so
in the log keeps the accounting honest.

## Consequences

- The `Permit` interface (`ralphloop/loop.go`) gains a non-blocking **`TryAcquire() bool`** alongside
  `Acquire`/`Release`. Recovery is the only caller; every existing acquisition path still blocks.
- The over-subscription is bounded in practice by the number of *simultaneously parked, then
  simultaneously answered* iterations — a human answering one pane at a time can exceed the limit by
  one at a time, and each excess resolves as the recovered iterations finish.
- A run's peak agent count can now exceed `MaxConcurrentEpics`. Anything reading the limit as a hard
  ceiling on live panes (docs, dashboards, future scheduling work) is wrong and should be corrected
  to the "agents gx starts" reading.
- This is the ask side only. It says nothing about the fault path, where an iteration that ends in
  `needs-repair` has no live agent to account for.
