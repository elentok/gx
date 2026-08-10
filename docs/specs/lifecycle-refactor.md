# lifecycle-refactor

## Problem Statement

Running an epic through ralph-loop is unreliable in ways that keep recurring, and every fix opens a
new edge case. Concretely, as an operator:

- Tickets start before their dependencies have finished. A ticket forked mid-flight can begin work
  while the ticket it was supposed to wait for hasn't done anything yet.
- Tickets that should start never do — a fork chain deadlocks against itself and the run reports
  "no unblocked tickets left but isn't all done".
- A `blocked_by` on a specific ticket is sometimes silently ignored, so unrelated work interleaves.
- An epic reports complete while forked work inside it is unfinished.
- When an agent asks a question in its pane, the run is already over by the time you get there —
  answering it does nothing, and if nothing else was in flight the run exited with an error.
- A ticket handed back to a human is immediately re-claimed by the orchestrator.
- A freshly allocated ticket stub, with no description written into it yet, is schedulable.

Eight distinct bugs of this shape are recorded in `gx-investigate`'s gotchas list. They are not
independent: all of them are mis-tunings of one over-complicated mechanism, and fixing them
one at a time has been producing new ones.

The mechanism is blocker resolution. A mid-flight fork currently tells two lies. It marks the
original ticket `done` when its work is not finished, so nothing downstream can trust a ticket's
`done`; and it gives each fork child `blocked_by: <original>`, so resolving the original walks into
tickets that are themselves waiting on the original. Undoing those two lies requires knowing, at
every step of the walk, which tickets to carve out as "family" — self, descendants, fork siblings,
and only for inherited tokens but not direct ones. That carve-out logic is the entire bug surface.

## Solution

Stop telling the lies, and the carve-out logic becomes unnecessary rather than merely correct.

A fork's only structural edge is `parent`. A fork child's `blocked_by` stays empty — it inherits its
parent's position in the dependency graph by being its child, not by copying its parent's tokens.
A blocker is then answered by one question, *is it still blocking?*, where a ticket is blocking until
its own work is `done` and every ticket in its fork subtree is likewise no longer blocking. That is a
plain post-order walk over `parent` reverse-edges with no exclusions, no cycle guard, and no
special cases. Five of the eight recorded bugs become unrepresentable.

Alongside it, two smaller corrections that the same investigation surfaced:

A ticket's status vocabulary collapses to six values, each mapping to exactly one scheduler
behavior. A newly allocated stub is born `draft` and is not schedulable until someone has actually
written it.

And a run that has nothing left to do but is waiting on a human **parks** instead of exiting. You
answer the agent in its pane, the ticket's status clears, and the run carries on. A parked epic
gives up its slot in the epic-level concurrency cap, so it doesn't starve queued epics while it
waits for you.

## User Stories

### Dependency resolution

1. As an operator, I want a ticket that depends on a forked ticket to wait for the fork's entire
   subtree, so that downstream work never starts against half-finished plumbing.
2. As an operator, I want a forked ticket to start as soon as its parent hands off, so that a fork
   never deadlocks against its own children.
3. As an operator, I want `blocked_by` naming a specific fork sibling to be genuinely enforced, so
   that declaring an ordering between two siblings actually produces that ordering.
4. As an operator, I want a fork to be able to produce several children that run in parallel, so
   that an agent discovering two independent pieces of work isn't forced to serialize them.
5. As an operator, I want a ticket whose `blocked_by` names an id that doesn't exist to stay
   blocked, so that a typo fails loudly instead of silently unblocking work.
6. As an operator, I want blocker resolution to be unaffected by how deep a fork chain goes, so that
   a ticket forked from a forked ticket behaves like any other.
7. As an agent forking a ticket, I want to write only the child's `parent` field, so that I can't
   corrupt scheduling by forgetting a second bookkeeping write.
8. As an agent forking a ticket, I want to never touch another ticket's file after handoff, so that
   I can't clobber a status the scheduler just set.
9. As a maintainer, I want a malformed `parent` edge rejected before it reaches the resolver, so
   that the resolver can be a plain recursion with no defensive guards.
10. As a maintainer, I want two agents re-parenting tickets at the same time to be unable to
    jointly create a cycle, so that the acyclicity invariant holds under concurrency and not just
    against one writer at a time.

### Ticket states

11. As an operator, I want a ticket that has been allocated but not yet written to be unschedulable,
    so that no agent is ever handed an empty ticket.
12. As an authoring agent, I want to allocate a ticket id before I've written the body, so that I
    can compose the ticket iteratively without racing the scheduler.
13. As an authoring agent, I want the transition out of `draft` to be refused while the body is
    empty, so that the "nothing schedulable is empty" rule is enforced where it can be checked.
14. As an operator, I want a ticket that omits `status:` entirely to be unable to sneak past the
    empty-ticket rule, so that hand-authoring a file isn't a way around `draft`.
15. As an operator, I want a forked ticket whose subtree is unfinished to be displayed as waiting
    for its children rather than as done, so that the Tickets tab doesn't tell me work is complete
    when it isn't.
16. As an operator, I want an epic to count as complete only when its fork subtrees are complete,
    so that a completion notification means what it says.
17. As an operator, I want exactly one status meaning "ready to be picked up", so that I don't have
    to remember which of several spellings the scheduler honors.
18. As an operator, I want every status to map to exactly one scheduler behavior, so that reading a
    ticket's status tells me whether it will be picked up.
19. As an operator, I want a ticket that needs a human to never be claimed by the orchestrator, so
    that handing work back to myself actually removes it from the queue.
20. As an operator, I want `needs-info` and `needs-attention` to stay distinct, so that I can tell
    whether the ticket is asking me a question about the work or reporting that the machinery
    broke.
21. As a maintainer, I want no ticket to be created already `done`, so that no ticket can reach a
    terminal state without its own dependencies ever having been checked.

### Run lifecycle

22. As an operator, I want a run that hits a human-blocking ticket to keep working on everything
    else, so that one stalled ticket doesn't idle the whole epic.
23. As an operator, I want a run with nothing left to do but a human-clearable ticket to park rather
    than exit, so that I can answer the agent and have the run continue.
24. As an operator, I want an epic whose only remaining tickets are drafts to park rather than
    error, so that an epic being authored isn't reported as broken.
25. As an operator, I want to be notified when a run parks, so that an unattended run waiting on me
    isn't silently invisible.
26. As an operator, I want a parked run to resume by itself once I've cleared the ticket in its
    pane, so that I don't have to switch to the Queue tab to continue.
27. As an operator, I want to resume a parked epic manually from the Queue tab, so that I can
    recover a stall that doesn't clear itself.
28. As an operator, I want the Queue tab to show which epic is parked and why, so that I know where
    my attention is needed.
29. As an operator, I want a parked epic to release its concurrency slot, so that queued epics start
    instead of waiting behind work that's waiting on me.
30. As an operator, I want a resumed epic to re-queue normally if the cap is full, so that resuming
    doesn't exceed the concurrency I configured.
31. As an operator, I want two epics resuming at the same moment to be unable to jointly exceed the
    cap, so that the concurrency limit holds under a simultaneous resume as well as a staggered one.
32. As an operator, I want a cleared ticket whose iteration is still live to resume into that same
    iteration, so that a second agent isn't launched against a pane I'm already talking to.
33. As an operator, I want a cleared ticket whose iteration has ended to resume by launching a fresh
    one, so that a stall whose agent is gone still makes progress.
34. As an operator, I want a stalled ticket left behind by a crashed or unrecoverable iteration to
    be recoverable, so that clearing it doesn't produce a ticket no agent will ever pick up.
35. As an operator, I want a run with no runnable work and nothing a human could clear to be
    reported as a failure, so that a genuine dependency error is distinguishable from waiting on me.
36. As an operator, I want a parked run to keep waiting indefinitely, so that coming back hours
    later finds the run and its panes recoverable.

### Code review

37. As an operator, I want a code-review ticket to wait for every other ticket in its epic, so that
    it reviews finished work.
38. As an operator, I want a code-review ticket to wait for the whole of a forked ticket's subtree,
    so that a ticket that split before the review started is reviewed in full.
39. As an operator, I want a ticket added or forked after the review was claimed to not re-block it,
    so that late scope doesn't stall a review that's already running.
40. As an operator, I want a code-review ticket's displayed frontmatter to show what's in the file,
    so that the preview panel isn't a wall of synthesized ids.

### Migration

41. As an operator, I want my existing epics migrated in one pass, so that in-flight work keeps
    scheduling correctly after the upgrade.
42. As an operator, I want migration to fix the known-malformed fork chains in my tracker, so that I
    don't have to hand-repair them.
43. As an operator, I want migration to refuse to write a tracker it would leave invalid, so that a
    half-applied migration can't be what teaches me the loader got stricter.
44. As a maintainer, I want the loader to accept only the new shape, so that the resolver complexity
    being deleted here can't come back through a compatibility path.
45. As an operator, I want `gx tickets validate` to reject the old shape, so that a stale skill
    writing old-style tickets fails loudly.

## Implementation Decisions

Recorded as ADR-0016 (`fork-subtree-blocker-resolution`) and ADR-0017 (`stalled-epics-park-instead-
of-exiting`). Vocabulary is in `CONTEXT.md` under Ticket States, Ticket Forking, and Ticket
Dependencies.

### Resolution model

- The `tickets` package exposes one predicate in place of the current
  `UnresolvedBlockers`/`FullyDone` pair and their helpers: a ticket is **blocking** while its own
  status is not `done`, or while any ticket in its fork subtree is blocking. Fork subtree membership
  is derived from `parent` reverse-edges only.
- `isSelfOrDescendant`, `isForkSibling`, `isDescendantOf`, the `exclude` hook, the `visiting` cycle
  guard, and the `children`-based descendant walk are all removed.
- A `blocked_by` token naming no ticket in the epic keeps counting as blocking.
- A ticket pathologically naming itself in `blocked_by` is rejected at write time rather than
  special-cased at read time.
- Acyclicity of `parent` is an invariant of a loaded `Epic`, not of a single ticket. Per-ticket
  schema validation has no epic context and therefore cannot check it; the check is that `parent`
  names an existing ticket in the epic that is not in the writing ticket's own fork subtree, which
  requires the whole graph. It is enforced in two places:
  - **At load.** `Epic` construction validates the complete parent graph and never exposes a cyclic
    or dangling one. This is what lets the resolver be unguarded recursion: a hand-edited or
    externally written file can't reach it. *(As shipped: the loader quarantines each invalid edge —
    drops that ticket's `parent`, records the reason, renders it as an error — rather than failing
    the whole load, so one bad edge can't make the rest of the tracker unreadable. The invariant the
    resolver depends on is identical either way.)*
  - **At write, under the existing lock.** `gx tickets set --parent` acquires the epic's allocation
    lock (the same one `add` already uses to allocate ids), re-loads the epic inside it, validates
    the resulting graph, and writes — all under the lock. Without this, two concurrent re-parents
    can each validate against a snapshot and jointly produce a cycle.
- The resolver carries no cycle defense. The `visiting` guard is deleted, not relocated.
- `RenderedStatus` loses its `type: code-review` branch. A code-review ticket's `blocked_by` is
  expanded at load time to every other non-code-review ticket in the epic; the expansion is a
  resolver-side view and is never written to the file, so the preview panel keeps rendering literal
  frontmatter.
- The expansion is recomputed on every load and is **not** frozen at claim time. Freezing was
  considered and rejected: `blocked_by` only gates entry to the frontier, and nothing re-checks
  blockers on a ticket that is already claimed, so a frozen snapshot changes no observable behavior
  except on re-claim after a stall — where recomputing is the more correct answer anyway. A ticket
  added or forked after the review was claimed is consequently *not* covered by that review. That
  is accepted rather than fixed: covering it would require invalidating and restarting a running
  review, and by construction every other ticket was already `done` when the review claimed.
- `RenderedStatus` gains a computed **waiting-for-children** state for a ticket whose own status is
  `done` but whose fork subtree is still blocking. It is never a value written to disk. It is not a
  settled state, so an epic containing one is not complete.

### Frontmatter schema

- `children` is removed from the ticket schema, from `gx tickets set`, and from the preview panel's
  frontmatter rendering. Children are derived from `parent` wherever they're needed.
- `parent` remains, and becomes the sole structural fork edge.
- The status enum becomes `draft`, `open`, `claimed`, `needs-info`, `needs-attention`, `done`.
  `needs-triage`, `ready-for-agent`, and `ready-for-human` are removed.
- A missing `status:` continues to read as `open` **during this epic only**, so nothing in the
  tracker stops loading mid-refactor. Migration stamps an explicit status on every ticket, and
  `lifecycle-contract` then makes `status` required. This is what closes the empty-ticket hole
  completely: guarding only the `draft` → `open` transition would leave a hand-authored file with
  no `status:` and no body schedulable, and the fix belongs in the schema rather than as a
  body-emptiness check in frontier construction.

### Scheduler behavior per status

| Status | Scheduler |
|---|---|
| `draft` | never claim; run parks |
| `open` | claim when no blocker is blocking |
| `claimed` | already running |
| `needs-info` | never claim; run parks |
| `needs-attention` | never claim; run parks |
| `done` | finished (subject to fork subtree) |

### Write-time rules

- `gx tickets add` writes `draft`, never `open`, and never `done`.
- `gx tickets set --status open` is refused when the ticket body is empty. `gx tickets validate`
  accepts a body-less `draft` — the invariant is guarded at the transition, not at rest.
- No status-transition pairs are refused. An earlier draft of this spec had `set` refuse
  `needs-attention` → `open` and `needs-info` → `claimed`, on the theory that the stalled status
  determines how the ticket resumes. It doesn't — see Run lifecycle below; whether a live iteration
  survives is a property of the run, not of the status, so the schema cannot know which transition
  is correct.
- The existing refusal of `--status done` with a still-blocking `blocked_by` (unless `--force`) is
  retained and re-expressed against the new predicate.

### Run lifecycle

- The `settled` concept is removed. A run exits when every ticket is `done` (with fork subtrees
  complete), **parks** when it has no runnable work and at least one ticket a human could clear,
  and **errors** when it has no runnable work and nothing a human could clear.
- "A human could clear it" covers `needs-info`, `needs-attention`, and `draft`. Including `draft`
  is what keeps an epic still being authored from being reported as broken, and it makes the error
  case a genuine corruption signal — a dangling `blocked_by`, a cycle the loader somehow admitted —
  rather than a routine outcome.
- Parking keeps scheduling any other runnable ticket first; it only parks once nothing is runnable.
- Parking fires a notification through the existing event sinks.
- A parked run polls the epic on its existing cadence and resumes automatically when the stalled
  ticket's status clears.
- **Resume is driven by iteration ownership, not by status.** On resume the run attempts to reattach
  the ticket's iteration using the same reattach path it already uses at startup for tickets found
  `claimed`. Reattach succeeds → the ticket goes to `claimed` and that iteration continues in place.
  Reattach fails → the ticket goes to `open` and a fresh iteration is launched.
- A matching pane is *not* the predicate. `needs-attention` has three producers and only one of them
  leaves an owned iteration: the operator-attention gate does, but a generic iteration error is
  written after the goroutine has already exited, and reconciliation writes it for a ticket with no
  iteration at all. A pane can outlive its goroutine, and writing `claimed` restores neither the
  run's active count nor any attachment — so a pane-liveness test would strand exactly the tickets
  it was meant to rescue. The predicate is "this run owns a live iteration for this ticket, or
  reattaching to one succeeds".
- The two statuses stay distinct for what they tell the operator — `needs-info` is the agent asking
  a question about the work, `needs-attention` is the machinery reporting it broke — not for how
  they resume.
- The Queue tab renders a parked epic with its stall reason, and the pane to jump to when the run
  still owns a live iteration for the stalled ticket. Enter on that row is the manual resume,
  reusing the key's existing "start this epic" meaning.
- The per-run launched-ticket registry is cleared for a ticket whose iteration ends in a stalled
  status, so a ticket set back to `open` is claimable again. The registry's meaning becomes
  "currently launched", not "ever launched".
- `MaxConcurrentEpics` is redefined to count epics with at least one running iteration. A parked
  epic holds no slot; on resume it re-enters the queue and waits for a slot if the cap is full.
- Enforcing that requires an admission seam the registry doesn't currently have. Today it admits
  once, when a run is first started, and each live `Run` then polls and claims independently — so
  two parked runs resuming at the same moment would both claim before the registry observed either.
  The registry gains an acquire/release permit that `Run` holds around each zero-active → first-claim
  transition: acquired synchronously before the first claim, released on park. The cap is then
  enforced at every admission rather than only at the first.
- The `resume-signal` file plumbing is deleted: the signal writer, the path helper, the
  `ResumeSignaled` dependency, and its three poll sites. Resume is in-process. A headless loop is
  explicitly not a goal.

### Migration

- A one-shot `gx tickets migrate` rewrites every ticket in the tracker root: drops `children`, drops
  `blocked_by` entries that name the ticket's own `parent`, stamps an explicit `status` on any
  ticket missing one, maps the three removed statuses, and repairs the known-malformed
  `children`/`parent` chains by trusting `parent` and discarding `children`.
- The status mapping is `ready-for-agent` → `open`, `needs-triage` → `draft`, and `ready-for-human`
  → `needs-info`. All three are in the currently accepted enum, so leaving any of them unmapped
  would make those tickets unloadable after the contraction. `ready-for-human` maps to `needs-info`
  rather than `open` because its meaning — handed back to a human — is precisely what `needs-info`
  now covers; mapping it to `open` would re-create the bug where the orchestrator re-claims a ticket
  that was handed back.
- Migration is transactional per tracker root: it validates the complete result, including every
  epic's parent graph, before writing anything, and fails without writing if the result would be
  invalid. A half-applied migration must not be how the operator discovers the loader got stricter.
- The loader accepts only the post-migration shape. No read-side compatibility.
- Migration is idempotent and reports what it changed.
- The skills that write tickets (`gx-local-tracker`, `gx-to-tickets`, `gx-code-review`) are updated
  in the same epic: the fork protocol loses its "set `blocked_by` to the original" and "set
  `children`" steps, and gains the `draft` → `open` handoff.

## Testing Decisions

A good test here asserts externally observable behavior — which tickets a loaded epic offers as its
frontier, what a CLI invocation writes to disk and prints, which tickets a run claims and in what
order, and what an epic's rendered rows say. It does not assert which helper computed the answer.
The current test suite is already shaped this way and the new tests continue it.

No new seams. Three existing ones carry almost everything:

- **`tickets.Epic` as an in-memory value** — `tickets/status_test.go`. The whole resolution model,
  fork subtree membership, waiting-for-children, code-review expansion, the frontier, and the
  parent-graph validation that `Epic` construction now performs (a cyclic or dangling `parent`
  never reaches the resolver). This is
  the highest seam available for the core change: pure, no filesystem, and it's where all eight
  recorded bugs already have regression tests. Every one of those tests is rewritten against the new
  predicate, and the ones covering carve-out behavior that no longer exists are deleted with a note
  in the gotchas list pointing at this epic.
- **`cmd.Execute(args, io)` against a temp tracker root** — `cmd/tickets_set_test.go`,
  `cmd/tickets_add_test.go`. The `draft` default, the empty-body gate, `parent` acyclicity rejection
  on `set --parent`, the removal of `--children`, and `gx tickets migrate` (given a fixture tree in
  the old shape, including the known-malformed chains, a `ready-for-human` ticket, a ticket with no
  `status:`, and a tree whose result would be invalid — which must leave every file untouched).
- **`ralphloop.Run(opts, fakeDeps(), sink)`** — `ralphloop/loop_test.go`, whose `fakeDeps()` already
  runs a whole epic with no herdr, git, or transcript. Parking instead of exiting, continuing to
  schedule other work before parking, parking on a draft-only epic, auto-resume on status clear,
  both resume outcomes (reattach succeeds → `claimed` and no second iteration; reattach fails →
  `open` and a fresh iteration), resume of a stall whose iteration never existed, the
  launched-registry clearing, deadlock-vs-stalled classification, and the notification on park.
  `run_realgit_subtickets_test.go` covers the fork paths against real git and is updated alongside.

The `Epic` seam and the `Run` seam must meet: alongside the resolution tests above, a small set of
`ralphloop.Run` tests assert the *scheduling consequences* of the new model end-to-end, so the two
layers are provably consistent rather than each correct in isolation. Specifically — a fork chain
claims in dependency order; a fork's parallel children are both claimed once their parent hands off;
a ticket blocked on a forked ticket is not claimed until the whole fork subtree is done; a ticket
declaring `blocked_by` on a specific fork sibling waits for exactly that sibling; and an epic
containing a waiting-for-children ticket does not report complete. These are behavioral assertions
over claim order and completion, not re-tests of the predicate.

The fourth seam is needed for one thing only: the Queue tab's parked row and its Enter override,
tested through the existing `ui/tickets` model tests with `testutil/teatestv2`, following
`queue_rows_test.go`.

Concurrency-slot release needs both levels. A registry-level test following `loop_registry_test.go`
covers the permit itself, but it is not sufficient: the cap is only actually enforced if `Run`
acquires before claiming, so the seams must meet here too. A `Run`-plus-registry integration test
drives two runs that park and resume simultaneously against a cap of one, and asserts that no more
than one holds a permit at any point.

## Out of Scope

- **The `agent_name_taken` write race** — two unsynchronized writers sharing one unlocked ticket
  file. It is orthogonal (it would be a bug under either resolution model) and needs its own
  file-locking or compare-and-swap design. It gets cheaper to fix after this epic, because the fork
  protocol no longer requires a parent's agent to write to a child's file at all. Separate epic.
  This is distinct from, and wider than, the lock this epic puts around `set --parent`: that one
  guards a single graph edge using a lock that already exists, whereas `agent_name_taken` is about
  two writers sharing an arbitrary ticket file with no coordination at all.
- **`gx tickets next <epic>`** — the "print the next eligible ticket's path" command that
  `/gx-grill`, `/gx-tdd`, and the code-review skill would drive off. Downstream of this epic: it
  should be built against the new frontier semantics, not the old. Separate epic.
- **A headless `gx ralph-loop <epic>` CLI.** Explicitly rejected; the loop runs in the TUI process.
- **Ticket renumbering.** Flattening fork ids to a single letter level was considered and dropped:
  the Queue tab's durable state is keyed by file path, so renaming orphans checked/queued state for
  in-flight epics.
- **Lowering `MaxConcurrentTicketsPerEpic`/`MaxConcurrentEpics`** as a mitigation. Rejected — it
  hides the ordering bugs rather than fixing them.
- **The `superseded` status** found in five archived tickets. Archive-only; not in the enum, not
  worth migrating.

## Further Notes

The prior design exploration in `docs/schedulers-research.md` reached a different conclusion — a
strictly linear fork chain with at most one successor, plus a rewrite of downstream `blocked_by`
entries at fork time. That is superseded. The linearity constraint was load-bearing only under the
old model, where several siblings meant several mutually blocking inherited tokens; once children
carry no inherited tokens, a fan-out costs exactly what a chain costs. That document's survey of
how Temporal, Celery, BullMQ, Airflow, and Kubernetes handle live task graphs remains the useful
part, and its central finding holds here: every mature scheduler keeps "this is the same unit of
work, continued" as a separate primitive from fan-out/join, which is what `parent` becomes.

Two claims made during design and corrected on inspection, recorded so they aren't re-derived:
there is no `gx ralph-loop resume` command, and there is no CLI that runs the loop at all —
`ralphloop.Run` is called from exactly one place, the TUI's loop registry.

A design review after the first draft changed seven decisions, all recorded above in place. The
three that reversed a stated position, kept here so the earlier reasoning isn't re-adopted:

- **Code-review eligibility no longer freezes at claim.** The freeze was meant to keep a
  post-claim fork covered by a running review. It can't: `blocked_by` gates only entry to the
  frontier, and a claimed review is never re-checked against it. The freeze was very nearly a
  no-op, and the user story it existed for was the thing that was wrong.
- **Resume is not chosen by status.** `needs-attention` → same pane and `needs-info` → fresh
  iteration reads well but is false for two of `needs-attention`'s three producers, both of which
  leave no owned iteration. Reattach success is the real predicate; a pane-liveness check is not a
  sufficient substitute, since a pane outlives its goroutine.
- **Parent acyclicity is not a per-ticket schema check.** It needs the whole epic, so it moved to
  `Epic` construction, plus the alloc lock on `set --parent` for the concurrent case.
