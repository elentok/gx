# One-offs inbox

## Problem Statement

Every `gx server one-off` call creates its own top-level epic holding one ticket. A busy project
piles up dozens of these epics. They crowd the epic list and the Queue tab, and each one is its own
queue root with its own per-root cap, so the number of one-off agents is hard to bound. Finished
one-offs are invisible in Recently finished, which lists only loop epics. The `followup-inline-editor`
epic would add many more one-off tasks and is waiting on this work.

## Solution

Each project gets one permanent `one-offs` epic. Every one-off is a ticket inside it, addressed as
`project:one-offs/NN`. The epic stays `draft`, so it never takes part in loop-epic landing. When a
one-off ends `done` or `cancelled`, the server moves it to `one-offs-archived`, the same way
`follow-ups` and `follow-ups-archived` work. A one-off that failed, is parked, needs an answer or
needs repair stays in `one-offs` until a person closes it with `gx server tickets cancel`.

One-offs never commit. They share their own cap, `execution-queue.max-one-off-agents`. They appear
in Recently finished (webui and TUI), mixed with epics by time and marked as one-offs.

## User Stories

1. As a gx user, I want all my one-offs in one `one-offs` epic per project, so that the epic list
   and Queue tab stay readable.
2. As a gx user, I want a one-off submitted with `gx server one-off` to land as
   `project:one-offs/NN`, so that I can address it like any other ticket.
3. As a gx user, I want `--name` to become the ticket's slug, so that I still get a readable
   address.
4. As a gx user, I want the `one-offs` epic created on the first submit of a project, so that I
   never set it up by hand.
5. As a gx user, I want the `one-offs` epic to stay `draft`, so that loop-epic landing never
   considers it. Check: with all one-offs done, the epic never reaches AllDone, landing, merge or
   retro.
6. As a gx user, I want a `done` one-off to move to `one-offs-archived`, so that `one-offs` shows
   only live and broken work.
7. As a gx user, I want a `cancelled` one-off to move to `one-offs-archived` too, so that closing
   a stuck one-off cleans up the same way.
8. As a gx user, I want failed, parked, needs-answer and needs-repair one-offs to stay in
   `one-offs`, so that nothing broken disappears before I look at it.
9. As a gx user, I want to close a stuck one-off with `gx server tickets cancel`, so that I need
   no new verb.
10. As a gx user, I want the archive move to happen whichever path ended the ticket (agent finish,
    late finish, repair, hand-landed, or a manual status change), so that I never see a done
    ticket left behind.
11. As a gx user, I want `gx tickets close --sweep` to keep touching only `follow-ups`, so that
    its meaning does not change.
12. As a script author, I want `gx tickets show` on the old address `project:one-offs/NN` to find
    the archived ticket, so that stored addresses keep working.
13. As a script author, I want `gx server one-off --wait` to keep waiting and print the Result
    after the ticket has moved to the archive, so that the move is invisible to me.
14. As a script author, I want `--wait` to survive a server restart across the archive move, so
    that a restart never loses the result.
15. As a reader of the archive, I want the archived ticket to carry `- original: <old address>`
    in its Resolution, so that I can trace it back.
16. As a debugger, I want event logs to keep the old address, so that history reads as it was
    written.
17. As a gx user, I want other tickets that were blocked by a moved one-off to keep working, so
    that the move never breaks a `blocked_by` edge. The existing rule for a `cancelled` blocker is
    unchanged; the move adds no new one.
18. As an operator, I want a `max-one-off-agents` setting (default 3), so that one-offs cannot
    flood the machine.
19. As an operator, I want to override `max-one-off-agents` per project, so that a heavy project
    can get more or fewer slots.
20. As an operator, I want the global and project caps to still apply to one-offs (ADR 0032), so
    that one-offs never bypass them.
21. As an operator, I want the per-root cap not to apply to the `one-offs` root, so that the
    one-off cap alone governs it. Check: with a per-root cap of 1 and `max-one-off-agents` of 3,
    three one-offs run at once.
22. As a gx user, I want `--agent` to choose the agent per one-off, so that I can run codex and
    claude one-offs side by side.
23. As a gx user, I want the chosen agent stored on the ticket, so that the claim launches the
    right one even though all one-offs share a queue item.
24. As a gx user, I want a one-off without `--agent` to use the project default agent, as today.
25. As a gx user, I want one-offs with different agents to share one cap, so that the cap counts
    agents, not agent kinds.
26. As a gx user, I want `--front` to move the whole shared `one-offs` queue item to the head, so
    that an urgent one-off starts first.
27. As the server, I want recovery and CPU-spike one-offs to keep using `--front`, so that their
    behavior does not change. Check: a recovery one-off submitted behind other roots lands at the
    queue head.
28. As a gx user, I want `--commits` and `--base` gone from `gx server one-off`, so that the
    contract is simple: one-offs never commit.
29. As a gx user, I want a `--file` payload with a `commits:` or `base:` key refused with reason
    `option-removed` and nothing written, so that old payloads fail loudly.
30. As a gx user, I want the refusal message to name the key and say one-offs never commit, so
    that I know what to change.
31. As a gx user, I want the no-VCS check gone, so that one-offs run in any project. Check: a
    one-off submitted in a project without a VCS is accepted.
32. As an agent author, I want the `gx-one-off` skill to say one-offs never commit and to drop the
    `--commits` and `--base` docs, so that agents do not use removed flags.
33. As a gx user, I want `--unique` dedupe to count only live one-offs (`done` and `cancelled`
    never count) and to skip `one-offs-archived`, so that duplicates are still refused and a
    finished one-off never blocks a new one.
34. As a gx user, I want retro and evolve tickets submitted through the same path, so that they
    land in `one-offs` and are archived like any other one-off.
35. As a gx user, I want a retro'd epic's `evolve_ticket` to keep working after the retro ticket
    is archived, so that `gx evolve` still finds it.
36. As a gx user, I want a `completed_at` stamped on every one-off that ends `done`, so that
    finished one-offs have a time. Loop tickets are out of scope here. Check: a one-off that ends
    `done` has `completed_at`; one that ends `cancelled` has none.
37. As a webui user, I want finished one-offs in Recently finished, so that I see what my one-offs
    did.
38. As a webui user, I want one-offs mixed with epics by completion time, so that the list reads
    as one timeline.
39. As a webui user, I want one-offs to share the single `recently-finished` count, so that there
    is one setting for the list length.
40. As a webui user, I want each one-off entry marked as a one-off and showing no done/total, so
    that I do not mistake it for an epic.
41. As a TUI user, I want the Queue tab to show one-offs in the same Recently finished list with a
    `one-off` marker and no done/total, so that both clients agree.
42. As a client author, I want a single `recently_finished` list in the snapshot, with no second
    field, so that I read one source.
43. As a gx user, I want the `one-offs` root dequeued when it has no live tickets and re-queued on
    the next submit, so that an idle project shows no permanent entry. Check: after the last
    one-off archives, the Queue tab shows no `one-offs` entry; the next submit shows it again.
44. As a gx user, I want a failed, parked, needs-answer or needs-repair one-off to keep the root
    queued, so that broken work stays visible. Check: a failed one-off keeps the entry.
45. As a gx user, I want the `one-offs` card to list every ticket still in `one-offs` (running,
    parked, failed, needs-answer, needs-repair), with no done/total, so that nothing broken is
    hidden and it never looks like a half-finished epic.
46. As a gx user, I want the `one-offs` card never to show `awaiting-merge`, so that it does not
    suggest a merge that cannot happen. Check: a card whose tickets are all `done` and about to
    move never reads `awaiting-merge`.
47. As a gx user, I want existing one-off epics left alone, so that in-flight work is not
    disturbed by an upgrade. Check: after an upgrade, an old single-ticket one-off epic still
    runs, and nothing is moved or renamed.
48. As a maintainer, I want the Glossary "One-off" entry rewritten, so that the vocabulary matches
    the behavior.

## Implementation Decisions

**Placement and identity**
- Each project has one `one-offs` epic, status `draft`, created on first submit. A one-off is a
  ticket in it, `project:one-offs/NN`. `--name` is the ticket slug (the default name from the
  prompt's first words stays).
- `rootOf` is unchanged, so all one-offs of a project share one root, `project:one-offs`.
  `completeRootIfDone` skips that epic, so it never reaches AllDone, landing, merge or retro.
- The one-off ticket shape no longer relies on "an epic with one `one-off` ticket file". Detection
  of a one-off ticket (silent chat notice on done, commitless launch, Result handling, evolve
  skipping one-off epics, budget grouping) keys off the `one-offs` / `one-offs-archived` epic
  names. The implementer picks the helper names. Old single-ticket one-off epics keep being
  detected and run as they do today; they are not migrated. They count toward the global and
  project caps, but not toward `max-one-off-agents`. During an upgrade, old and new one-offs run
  side by side, and the Queue tab shows both kinds.
- A ticket created by hand in `one-offs` runs as a one-off, commitless, under the same cap. A
  ticket with a missing or unknown agent field uses the project default agent.
- Ticket numbers in `one-offs` are allocated above the highest number in `one-offs` and
  `one-offs-archived` together, so a number is never reused and an `original:` address never
  collides.
- Every one-off is commitless: it needs no worktree, branch, base or landing step. The type is
  `prompt` unless `--type` gives `evolve` or `diagnose`.

**Request contract**
- `--commits` and `--base` are removed from the CLI, from the request type and from the payload
  frontmatter understood by `--file`.
- A payload with a `commits:` or `base:` key is refused before anything is written, with the new
  reason `option-removed`. The message names the key and says one-offs never commit. The reasons
  `commits-unsupported` and `invalid-base` are dropped, with their use of the no-VCS check.
- `--front` stays. It moves the shared `one-offs` queue item to the queue head and leaves it there
  until the root is dequeued, which resets its position. Repeated front submits can keep other
  roots waiting; that is accepted, as `--front` works today.
- `--agent` stays. The agent is stored on the ticket as a new frontmatter field (the implementer
  picks its name). The claim reads it for the ticket it launches. Without `--agent`, the project's
  default agent applies, as today.
- Dedupe (`--unique`, `--no-unique`, the `--file` default key, `duplicate-live`) keeps its flags and
  scans the whole project. Only live one-offs count: `done` and `cancelled` never do, and
  `one-offs-archived` is not scanned.
- Retro and evolve submissions (`RetroSubmit`, recovery, CPU-spike) go through the same submit path.
  A retro's `evolve_ticket` stamp keeps the address it was written with; after the ticket moves,
  every reader resolves it through the shared `original:` lookup described below (the writer in
  `server/retrostamp.go`, the readers in `cmd/evolve.go` and the server).

**Cap**
- New config key `execution-queue.max-one-off-agents`, default 3, overridable per project in
  project config, set the same way as the other `execution-queue` keys. It caps live one-off agents
  of a project (tickets that are `claimed` or running) in place of the per-root cap for the
  `one-offs` root. The global and project caps still apply (ADR 0032). All agents count the same,
  whatever the agent kind. The config schema, config reload and defaults follow the existing
  execution-queue keys.

**Queue and display**
- The `one-offs` root is one queue item. It is queued on submit when absent, and dequeued when the
  project has no live one-off tickets. A failed, parked, needs-answer or needs-repair ticket is
  live and keeps it queued. Order inside the root follows ticket order and `blocked_by`.
- The archive check, the dequeue and a submit's re-queue run under one lock. A submit that arrives
  during the check wins and keeps the root queued.
- Its epic card lists every ticket still in `one-offs` (running, parked, failed, needs-answer,
  needs-repair), shows no done/total, and its state is never `awaiting-merge`.

**Archive**
- The server moves every `done` or `cancelled` ticket in `one-offs` to `one-offs-archived`,
  whatever wrote the status. It acts on what its store index sees, so no per-writer hook exists.
  `gx tickets move` refuses tickets with run history, so the server performs the move through its
  own store path. `one-offs-archived` is created on first move.
- The move rewrites inbound `blocked_by` references, as `planMove` does today.
- The move order is: write the ticket into `one-offs-archived`, then remove it from `one-offs`. On
  startup the server removes any ticket that exists in both places.
- Cancelling a one-off with a live run stops the run first, then moves the ticket.
- The archived ticket's Resolution keeps `- original: <old address>`. One shared helper does the
  fallback: when an old address is not found, it looks up `original:` in `one-offs-archived`. Every
  reader of a ticket address uses it: `gx tickets show`, `--wait`, `gx server tickets cancel`,
  `blocked_by`, `evolve_ticket`, and chat/notify and webui links. There is no alias table. Event
  logs keep the old address.
- A person closes a stuck one-off with `gx server tickets cancel`. `gx tickets close --sweep`
  stays follow-ups only.
- Only `done` and `cancelled` move; no other status triggers a move.

**completed_at and Recently finished**
- Every one-off gets `completed_at` stamped when it ends `done` (epics keep their own stamp; loop
  tickets are unchanged). A `cancelled` one-off, including a cancelled retro or evolve one-off,
  gets none, so it never appears in Recently finished.
- The snapshot keeps one `recently_finished` list. Its entries gain a boolean `one_off` field that
  the clients read to tell a one-off from an epic. A one-off entry carries its ticket address in
  the existing finished-entry address field. One-off entries carry no done/total.
- The list is built from the index: epics with `completed_at`, plus tickets in `one-offs-archived`
  with `completed_at` (and any done ticket still in `one-offs` before its move). It is sorted by
  completion time, newest first, cut to the existing `server.recently-finished` count.
- The webui and the TUI Queue tab render one-off entries with a `one-off` marker and no done/total.

**Docs and skills**
- GLOSSARY.md "One-off" entry is rewritten to match (the uncommitted edit already covers most of
  it; align the rest). The `gx-one-off` skill drops `--commits` and `--base` and says one-offs
  never commit. ADR 0040 (shared root and cap) is already written.

## Testing Decisions

- A good test drives external behavior: a submit, a status change, a snapshot read, a CLI call. It
  does not assert internal helper names or file layout beyond the documented ticket addresses.
- Highest seam: the server test harness (`server/servertest`) with a real store and a fake agent,
  exercised through the one-off submit API and the snapshot. It covers placement, the archive move
  after each ending path, queue dequeue and re-queue, the cap and `--front`.
- Second seam: the `gx server one-off` command and `--wait` (`cmd` tests with a fake server), for
  flag removal, `option-removed` refusals and the `original:` follow after a move.
- Third seam: the `original:` fallback helper, with one test per reader (`gx tickets show`,
  `--wait`, `gx server tickets cancel`, `blocked_by`, `evolve_ticket`).
- Also covered: a mixed-agent run under one cap (stories 22 to 25); the `option-removed` message
  naming the key (stories 29 and 30); old epics left untouched and running next to new one-offs
  (story 47); a submit during the archive move (story 43); a crash between the archive write and
  the removal; cancelling a one-off with a live run; number allocation across both epics; a
  hand-made ticket and an unknown agent field.
- Snapshot-level tests for Recently finished (mixed epics and one-offs, shared count, marker, no
  done/total). Webui and TUI rendering get focused render tests for the marker.
- Prior art: `server/oneoff_test.go`, `server/oneoff_commits_test.go` (to be rewritten for
  `option-removed`), `server/recentlyfinished_test.go`, `cmd/server_oneoff_test.go`,
  `cmd/server_oneoff_wait_test.go`, and the follow-ups close tests for the archive pattern.

## Out of Scope

- Anything beyond the Problem Statement and Solution above.
- Migrating existing one-off epics into `one-offs`.
- Pruning or retention of `one-offs-archived`. A follow-up covers it.
- Stamping `completed_at` on loop tickets.

## Further Notes

- Pattern to copy: `follow-ups` and `follow-ups-archived`.
- This epic blocks `followup-inline-editor`.
- Implementer's choices, left open on purpose: the ticket frontmatter field for the agent, helper
  names for detecting one-off tickets, and the exact wording of the `option-removed` message.
