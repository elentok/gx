# A ticket waiting on a person is *parked*, not *stalled*, and a person *unparks* it

`stalled` was doing two opposite jobs. The failure mode this whole line of work exists to eliminate
is the **invisible stall**: an agent that stopped making progress without gx being able to see it —
the origin incident, where ticket `11a` sat `claimed` while its live agent waited on an
`AskUserQuestion` prompt that never touched disk. But `stalledTickets`, `StalledTicket`, and
`LiveEvent.Stalled` meant nearly the reverse: a ticket that stopped **visibly and deliberately**, on
disk, in a status a person can act on. One word for the thing we are building and the thing we are
destroying.

We reserved **stall** for the failure mode and named the deliberate state **parked**. The word was
already half-adopted and pointing the right way: the tracker contract describes `draft` as "work its
author parked deliberately", and `draft` is one of the three statuses the predicate matches. It also
composes upward without a second concept — an epic parks exactly when every in-scope ticket left is
parked or blocked, which is what `EpicParked` already meant.

`isHumanClearable` is renamed **`isParked`** rather than kept alongside. Two reasons. It is
redundant: its body is membership in `{needs-answer, needs-repair, draft}`, which is the definition
of parked, so it states no test beyond the state itself. And **`clear` is already taken by a
destructive action** — the Queue tab's `c`/`C` keymaps clear complete and clear checked, which
*delete tickets from the queue* (`ui/tickets/queue_clear.go`). In the vocabulary a user actually
sees, "human-clearable" reads as "a human may delete this".

The verb for ending a park is **unpark** — the write is `status: open`, returning the ticket to the
frontier. `clear` was rejected for the collision above; `resume` was rejected because the pause gate
already owns it (`gate.ForceResume`, `IterationResumed`).

The whole vocabulary is now one word: *a ticket parks, is parked, gets unparked; an epic parks when
everything left is parked or blocked; a stall is the failure where none of that was visible.*

## Considered options

- **Keep `stalled` for tickets and rely on context.** Rejected: the two meanings collide inside a
  single document — an epic spec about stall visibility cannot use `stalledTickets` to mean the
  successful case without a reader mis-parsing it.
- **Rename the nouns but keep `isHumanClearable` as the predicate**, on a test-vs-state distinction.
  Rejected once the body was read: there is no test there, only set membership, and the name imports
  the `clear` collision into the scheduler.
- **Rename `clear` in the Queue tab instead**, freeing the word for this. Rejected as the more
  expensive fix to the less important name: `clear` is correct for deleting rows, and the keymaps are
  muscle memory.

## Consequences

- The rename covers Go identifiers and prose only — **no on-disk data spells "stalled"**, unlike
  ADR 0018's status rename. It rides the same combined migration cut as the `needs-answer`/
  `needs-repair` rename and the `status`/`iteration_status` split, so the tree is walked once.
- In scope: `StalledTicket`→`ParkedTicket`, `stalledTickets()`→`parkedTickets()`,
  `isHumanClearable()`→`isParked()`, `LiveEvent.Stalled`→`LiveEvent.Parked`, and the surrounding
  comments in `ralphloop/loop.go`, `ralphloop/eventsink.go`, `ui/tickets/loop_registry.go`,
  `ui/tickets/queue_header.go`.
- Prose supersedes: ADR 0017's title and body ("stalled epics park"), ADR 0018's "human-clearable
  stall statuses", ADR 0019's "clearing is one write" (now "unparking is one write"), and
  `docs/specs/lifecycle-refactor.md`. Those documents are otherwise unchanged — this ADR replaces
  their vocabulary, not their decisions.
- `EpicParked` keeps its name and its trigger. It was already correct.
