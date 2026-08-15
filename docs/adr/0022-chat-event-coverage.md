# Chat carries the run's headline state, and its counts are epic-truth

Which of `EventSink`'s events reach Slack/Telegram was never decided — a method reached chat if and
only if `slackEventSink`/`telegramEventSink` happened to override it, which is why a ticket starting
work sent nothing at all. The rule is now stated: **chat carries changes to the run's headline state
— what gx is running, what it's blocked on, and when it ends — at ticket granularity or coarser.**
Anything happening *inside* one iteration (transcript lines, context occupancy, cherry-picking,
smart-zone compaction) and anything that is startup reconciliation housekeeping stays TUI-only.

The obvious competing rule — "chat is for facts that change what an away-from-keyboard person would
*do*" — was rejected because it excludes both `IterationStarted` and `IterationFinished`. It would
have ruled out the very gap that opened this decision while evicting a message gx already ships. The
headline rule keeps the boundary at *granularity* rather than at *actionability*, which is a
property of the event rather than a guess about the reader's intent.

## Consequences

- **Membership.** Chat: `EpicStarted`, `IterationStarted`, `IterationPaused`, `IterationResumed`,
  `IterationFinished`, `TicketNeedsHuman` (ticket 05's single parking event), `EpicParked`,
  `EpicComplete`, `EpicFailed`, `DrainComplete`. Everything else on `EventSink` is TUI-only.
  `TicketUnrecoverable` reaches chat *through* `TicketNeedsHuman` — it is a parking write like any
  other — rather than as a member of its own, which keeps "every park is one event from one site"
  literally true.
- **`NoTicketsFound` and `AlreadyComplete` are folded into `EpicStarted`** rather than chatting
  separately, buying the invariant **every epic that leaves the queue emits exactly one start
  message**. That invariant is what makes a *missing* start message meaningful, and it matters
  because epics are promoted off a queue — nobody is at the terminal when the second epic starts.
- **Counts are epic-truth, always.** The old `Completed` was a per-`Run`-call counter starting at
  zero (`loop.go:405`), so a resumed epic reported `1/10 done` when six were done; the old `Total`
  was scope-limited. Both are replaced by `epic.DoneCount()`/`epic.TotalCount()`, recomputed from
  disk. The messages read as statements about the epic, so they must be true about the epic.
- **Fractions are replaced by a counts line** — a tally of states rather than `8/10`. Fixed order
  `done · in progress · parked · blocked · ready · total`; zero clauses are suppressed except `done`
  and `total`, which always render. A parallel **queue counts line**
  (`done · in progress · parked · failed · total`) rides on the two epic-level messages only. The
  tally shape was chosen over a fraction because the queue's denominator moves — an epic can be
  added mid-run — and a fraction would read as a progress bar that goes backwards.
- **A counts line appears only where the counts materially moved**: epic started, ticket landed,
  ticket parked, epic complete, drain complete. Not on ticket started, paused, or resumed, where it
  would repeat the previous message with one ticket shifted.
- **One `chatEventSink`** parameterized by an `mrkdwnStyle` and a transport replaces the two
  near-identical decorators, so an event can no longer land in Slack and not Telegram. A
  reflection-driven contract test enumerates every `EventSink` method against an explicit chat
  yes/no map and fails when a method is added without a verdict — the rule is pinned by a test, not
  by prose.
- **`EpicFailed` fires from the registry, not from `ralphloop`.** `RunStateFailed` is set at
  `ui/tickets/loop_registry.go:612`, after `ralphloop.Run` has already returned an error, and the
  registry also fails runs for reasons `Run` never sees (attach conflicts). Firing it from inside
  `Run` would produce a chat surface with holes in exactly the failure modes most worth pushing to a
  phone. This is the one documented exception to chat living in `ralphloop`, and it is called out in
  the contract test.
- **Chat coverage stays a fixed set, not config** (ticket 05's ruling, re-confirmed against the new
  volume of roughly 6–12 messages per hour on a busy two-parallel run).
- **Ticket 05's separate coalesced parked-summary message at run start is dropped** — `EpicStarted`'s
  parked clause carries the same news in the same instant, and two messages one second apart train
  the reader to ignore both.
