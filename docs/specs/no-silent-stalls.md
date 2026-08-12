# No silent stalls

> Follow-on epic to `lifecycle-refactor`. Written from the wayfinder map at
> `.scratch/no-silent-stalls-map/map.md`, whose 15 decision tickets are all closed. Almost every
> decision below is recorded in one of those tickets or in ADRs 0018–0023; this spec states the
> shape, not the reasoning, and cites the ticket that holds the reasoning.
>
> **Two spec reviews (Codex, Claude) amended it after the map closed.** Section D's park now **ends
> its iteration** rather than keeping a goroutine alive to poll, which reverses ticket 06's shape and
> deletes its permit work; section A gains the positive validated-commitless rule and adoption's
> precedence over landing; section F gains `EpicFailed`'s reporter lifetime. These four are not
> traceable to a map ticket — they are corrections to it, and each states its own reasoning inline
> because no closed ticket holds it.
>
> **Named for its outcome.** Per ADR 0020 a **stall** is the invisible-progress failure this epic
> eliminates — it is never made visible, it is converted into a **park**, a deliberate on-disk state
> a person can see and act on. The map that produced this spec carries the same name with a `-map`
> suffix, leaving `no-silent-stalls` free for the implementation epic `/gx-to-tickets` publishes.

## Problem Statement

A gx-driven agent can stop making progress without gx being able to see it. The origin incident:
ticket `11a` of `lifecycle-refactor` sat `claimed` with no fault status and no message anywhere,
while its live Claude agent waited on an `AskUserQuestion` prompt in a herdr pane. Claude Code's
interactive tools block the pane but write nothing to disk, and on-disk ticket status is gx's only
observation channel. Nobody found out until a person happened to look at the pane.

That single incident sits on top of four independent defects, each of which produces the same
experience — *the run looks fine and isn't*:

1. **A blocked pane is invisible and unbounded.** herdr publishes a per-pane `blocked` state, but
   `blocked` is absent from Claude's wait list, so the iteration re-loops forever and never releases
   its `MaxConcurrentEpics` permit. Above the smart zone gx sends `ctrl+c` and `/compact` at the
   pane, which gives the stall an exit and — on an **unverified but plausible** reading of the
   incident — destroys the pending question in the process. Section D no longer rests on that
   reading: under the shape decided there, no gx keystroke can reach a parked pane by construction.
2. **`done` does not mean landed.** The agent is instructed to write `status: done` from inside its
   iteration worktree while its commits are unlanded; `claimNext` re-reads the epic on every scan at
   `MaxParallel=2`, and a fresh iteration bases off the feature tip. A downstream ticket can start
   on a tree that is missing the work it depends on. Fork children have the same defect by a
   different route.
3. **The two human-waiting statuses are vague and half-plumbed.** `needs-info` and `needs-attention`
   name no axis a person can act on; one of them had no reason field at all; and the events that
   announce them are ad-hoc.
4. **Chat coverage is accidental.** An event reaches Slack/Telegram iff a wrapper sink happens to
   override it, which is why "tell me when a ticket starts work" doesn't work. The progress numbers
   those messages carry are run-local, so a resumed epic reports `1/10 done` when six are done.

The user-visible cost: an epic that has silently stopped, a permit leak that starves other epics, a
question nobody can see, and — when a person does look — a Queue tab that cannot say what anyone is
waiting for.

## Solution

A ticket that is waiting on a person becomes **parked**: a deliberate, on-disk, visible state with a
reason a person can read, reachable on every surface. **Stall** is reserved for the failure this epic
eliminates (ADR 0020).

From a user's perspective:

- **Agents never block a pane on a question they could write down.** An agent that needs something
  only a person can supply commits what is green, writes the question into its ticket, reports
  `iteration-status needs-answer`, and **exits** — releasing its worktree, tab, and permit. The
  ticket resumes later on its own branch (ticket 04).
- **When a pane blocks anyway** — an involuntary permission prompt — gx notices within ~15s, parks
  the ticket as `needs-answer` with a body stub pointing at the pane, **ends the iteration** (which
  releases the permit through the path that already exists), and never types into a pane holding an
  unanswered question. The pane and its worktree stay alive. When the person answers in the pane, a
  scheduler scan notices the pane left `blocked`, unparks the ticket, and reattaches to the pane it
  was already using (ticket 06).
- **Two park statuses that name what is being asked of you**: `needs-answer` (a person must supply
  an answer, nothing is broken) and `needs-repair` (gx hit a fault it can't resolve). Both carry a
  required reason and a body section (ADR 0018, tickets 02/13).
- **`status` is what gx knows; `iteration_status` is what the agent last said.** gx writes
  `status: done` only after the cherry-pick lands — or, for a ticket whose work is legitimately
  commitless, after gx has validated that — so `done` ⇒ **landed or validated commitless**, by
  construction (ADR 0019, tickets 03/09).
- **Every park reaches the person on all three surfaces**: the Queue row shows the park reason as
  subtext and the preview auto-scrolls to the section, the per-epic header counts parked tickets in
  orange/red, and one chat message goes out per parking write (tickets 05/12).
- **Chat carries the run's headline state on a decided, contract-tested set of events**, with
  epic-truth counts (tickets 10, ADR 0022).

## User Stories

1. As a developer running an epic overnight, I want an agent that hits a question to write it into
   the ticket and exit, so that the run keeps going on other tickets instead of stopping silently.
2. As a developer, I want the exited agent to commit whatever is already green, so that its work
   isn't discarded while the question waits.
3. As a developer, I want a zero-commit stop-to-ask **not** to set `commitless`, so that the flag
   keeps meaning "this ticket's work is legitimately codeless" for the UI and
   `scripts/ralph-stats.mjs` rather than also covering tickets that simply haven't committed yet.
4. As a developer, I want the agent's `## Needs Answer` section to state the question, the options it
   weighed, and what it would do with each answer, so that I can answer in one pass without reading
   the diff.
5. As a developer, I want a `## Handoff` section listing what's done, what's left, files touched, and
   which skills the next agent should invoke, so that the discarded context is recoverable.
6. As a developer, I want to answer by appending under `## Needs Answer` and setting `status: open`,
   so that unparking is one edit with no special command.
7. As a developer, I want the next agent to read that section first, retire it into `## Comments`,
   and not re-ask the same question, so that answering once is enough.
8. As a developer, I want the resumed iteration to reuse its own branch and land every commit across
   the answer boundary in one pick, so that stopping to ask costs no work.
9. As a developer, I want a pane that blocks on an involuntary prompt to park its ticket within
   seconds, so that a permission dialog can't consume a whole night.
10. As a developer, I want the parked ticket's body to tell me the question is *in the pane* and name
    the herdr agent to attach to, so that I don't hunt for a question that was never written to disk.
11. As a developer, I want the pane identified by its iteration label rather than a raw pane id, so
    that the instruction still works after a restart or reattach.
12. As a developer, I want gx to unpark the ticket by itself once the pane starts working again, so
    that answering in the pane is the entire gesture.
13. As a developer, I want a parked iteration to release its `MaxConcurrentEpics` permit, so that one
    blocked pane can't starve every other epic.
14. As a developer, I want the unparked ticket to reattach to its still-live pane rather than
    launching a fresh agent, so that the answer I typed into that pane isn't thrown away.
15. As a developer, I want gx to never send `ctrl+c` or `/compact` at a pane displaying an unanswered
    question, so that a smart-zone breach can't destroy the question I was about to answer.
16. As a developer, I want no timeout on a park, so that a question waiting for me over a weekend
    doesn't escalate into a fault.
17. As a developer, I want `needs-answer` and `needs-repair` to be distinguishable at a glance, so
    that I can tell "you're being asked something" from "something is broken".
18. As a developer, I want every park to carry a required one-line reason, so that the Queue tells me
    *why* without opening the ticket.
19. As a developer, I want the fault side's `## Needs Repair` section to carry a summary line,
    optional detail, and a best-effort state block (iteration label, branch, worktree), so that I can
    find the wreckage.
20. As a developer, I want gx to demote a live park section into `## Comments` at claim rather than at
    reattach, so that a ticket parked, unparked, and re-parked doesn't accumulate stacked sections.
21. As a developer, I want a ticket unparked to `open` to keep its section visible until an agent
    actually claims it, so that I can still see what I was told after I unpark it.
22. As a developer, I want `status: done` to mean the work is on the feature branch, so that I can
    trust the Queue without checking git.
23. As a developer, I want a fork child blocked until its parent's own work has landed, so that no
    agent starts on a tree missing the work it forked from.
24. As a developer, I want a parent that ends in a fault state to hold its child, so that a broken
    parent can't silently seed a child iteration.
25. As a developer, I want fork blocking to be non-recursive on the parent's own status, so that
    forking never deadlocks.
26. As a developer, I want the agent to report `--iteration-status` and never write `--status done`,
    so that the landing decision stays with gx.
27. As a developer, I want an agent's report to be able to *start* a landing but never *conclude*
    one, so that the commit count and cherry-pick remain the deciding facts.
28. As a developer working a wayfinder map by hand, I want `gx tickets set --status` to keep working
    exactly as it does today, so that hand-driven epics are not collateral damage of the guard.
29. As a developer, I want the guard to bind only iteration agents, recognised by their
    `ralph-loop/*` branch, so that no flag I could alias away is what protects the invariant.
30. As a developer, I want an agent on an iteration branch to still be able to promote a `draft`
    ticket to `open`, so that mid-flight forking keeps working.
31. As a developer, I want `needs-answer` and `needs-repair` rejected from the CLI for every caller,
    so that the park states can only be reached by the machine that observed the park.
32. As a developer, I want a Tickets-tab keymap to change a ticket's status from a menu, so that
    unparking is a normal gesture rather than a text edit.
33. As a developer, I want that menu to offer different values depending on whether a loop is running
    the epic right now, so that it doesn't invite me to assert something only gx can know.
34. As a developer looking at the Queue tab, I want a parked row to show its park reason as ellipsised
    subtext, so that I can triage without opening anything.
35. As a developer, I want that subtext read from the ticket file on disk, so that the row is
    identical before and after a gx restart.
36. As a developer, I want selecting a parked row to auto-scroll the preview to its park section, so
    that the reason I opened the ticket is what I see first.
37. As a developer, I want that section highlighted in foreground orange (`needs-answer`) or red
    (`needs-repair`), so that severity is legible without reading.
38. As a developer, I want a parked count on the per-epic header, so that "is anything waiting on me
    in this epic?" is answerable at a glance.
39. As a developer away from my desk, I want one chat message per parking write, so that a stopped
    ticket reaches my phone.
40. As a developer, I want a chat message when an epic starts, so that a *missing* start message is
    meaningful.
41. As a developer, I want a chat message when a ticket starts work, so that I can follow a run I'm
    not watching.
42. As a developer, I want a chat message when an epic fails, including failures that happen after
    the run returns, so that the failure modes most worth pushing to a phone aren't the ones with
    holes.
43. As a developer, I want everything happening *inside* one iteration to stay out of chat, so that
    the channel remains readable at ~6–12 messages an hour.
44. As a developer, I want chat coverage pinned by a contract test that fails when a new event has no
    chat verdict, so that the next event can't silently fall through the way `IterationStarted` did.
45. As a developer, I want counts to describe the **epic**, not the current run, so that a resumed
    epic doesn't report `1/10 done` when six are done.
46. As a developer, I want a counts line (`8 done · 1 parked: 07 · 1 ready · 10 total`) instead of a
    fraction, so that a moving denominator doesn't read as a progress bar going backwards.
47. As a developer, I want the parked identifiers listed inline in that counts line, so that I know
    which ticket to open.
48. As a developer, I want a queue counts line on epic-level messages only, so that I get a
    run-wide picture without repeating it on every ticket message.
49. As a developer, I want every chat message to end with a `[gx] epic/identifier` identity line, so
    that messages from parallel epics are attributable.
50. As a developer, I want the retired status names rejected loudly rather than silently accepted, so
    that a stale ticket file or skill doc fails fast instead of scheduling wrong.
51. As a developer, I want the skill bundle to ban the retired vocabulary in prose, so that an agent
    is never handed an instruction that produces a ticket gx rejects.
52. As a developer, I want that ban narrowed so it can't fire on herdr's own `agent_status` field, so
    that the docs whose job is to describe herdr can keep describing it.
53. As a developer, I want `CONTEXT.md` to carry `iteration_status` beside `agent_status`, so that the
    near-collision that drove the naming is recorded rather than rediscovered.
54. As a developer, I want the migration to be one phase with an explicit gate ticket, so that no
    downstream ticket starts against a vocabulary that doesn't exist yet.
55. As a developer, I want the phase order expressed only as `blocked_by` edges, so that the
    scheduler and the documentation can't disagree.
56. As a developer, I want `gx doctor` to include an interactive check that herdr still reports a
    blocked Claude form correctly, so that a Claude Code upgrade can't silently break the gate.
57. As a developer, I want a runbook describing that regression procedure, so that the check is
    actionable when it fails.
58. As a developer, I want dead code and prose claiming a `gx ralph-loop` CLI removed, so that no
    agent reasons about an entry point that doesn't exist.
59. As a developer, I want a legitimately commitless ticket to reach `done` without a cherry-pick, so
    that research, grilling, and code-review tickets still close once the agent can no longer write
    `done` itself.
60. As a developer, I want a commitless `done` to satisfy the fork edge, so that a commitless parent
    doesn't block its child forever.
61. As a developer, I want a `needs-answer` report to be honoured *before* gx counts commits or
    lands them, so that stopping to ask can't land a half-finished ticket as `done`.
62. As a developer, I want that same precedence applied when gx reconciles an orphaned branch at
    startup, so that a restart can't land what a live run would have parked.
63. As a developer, I want an epic-level failure to reach chat even though it is recorded after the
    run's sink has closed, so that the failure mode most worth pushing to a phone isn't the one
    that silently drops.

## Implementation Decisions

### A. The field model (ADR 0019, tickets 03 / 09 / 04)

- Two fields. **`status`** is gx-owned and remains the **sole** scheduling authority over its six
  values (`draft`, `open`, `claimed`, `needs-answer`, `needs-repair`, `done`), so `Frontier`,
  `RenderedStatus`, `Blocking`, and `isParked` all keep their single-enum branch.
  **`iteration_status`** is agent-owned, values `working` / `needs-answer` / `finished` / absent,
  cleared on every claim and reattach.
- The field is named `iteration_status`, never `agent_status` — herdr already publishes
  `agent_status` for pane state in the very files this work touches.
- gx **adopts** an `iteration_status` report and writes `status` itself. The verb is **adopt**, never
  "promote" (which already means `draft`→`open` and epic auto-promotion).
- Invariant: **an agent's report can start a landing, never conclude one.** `finished` is a second
  wake-up into the landing path; the commit count and the cherry-pick still decide.
- `commitless` stays a separate bool. A `finished` report with neither commits nor `commitless` is
  rejected. A zero-commit `needs-answer` stop is **not** `commitless`.
- **The positive commitless rule**, which the field split makes load-bearing: `iteration_status:
  finished` **+** `commitless: true` **+** zero commits ahead ⇒ **gx writes `status: done` with no
  cherry-pick.** Today that close is gated on the *agent* having moved the ticket off `claimed`
  itself (`finishIteration`'s `current.IsCommitless() && current.Status != schema.StatusClaimed`),
  which section B's guard removes; without this rule every commitless ticket falls through to the
  zero-commit fault path and parks. The `iteration_status` report replaces the non-`claimed` status
  as the "this was deliberate" signal, which is what the field was introduced for. The rule applies
  identically on the reattached close, so the backfill-metadata path carries it too.
- Consequently the invariant is **`done` ⇒ landed or validated commitless**, and a commitless `done`
  satisfies section B's fork edge like any other `done` — the alternative deadlocks every commitless
  parent's child.
- **The invariant is scoped to gx's own writes on a loop-driven epic**, which is the only scope it
  can have: `done` stays in the CLI's settable set for non-agent callers (section B), so a person
  working a hand-driven epic can still write `done` on a ticket with nothing landed, exactly as
  today. That is not a hole — a hand-driven epic has no cherry-pick to land and no scheduler making
  decisions off the guarantee. What the invariant buys is that **no loop-driven `done` was written by
  anything but gx after it landed the work**, which is what the fork edge and `classifyDoneTicket`
  rely on. Stated because "done ⇒ landed by construction" reads as universal and isn't.
- **Adoption precedes landing.** A valid `needs-answer` report is honoured *before* gx counts
  commits, lands them, cleans up, or emits `IterationFinished`; it yields a parked outcome and
  nothing else. Without the ordering, an agent that commits what is green and then stops to ask has
  its partial work cherry-picked and marked `done` while the question is still unanswered. The same
  precedence binds startup reconciliation, which today lands orphaned `claimed` branches that carry
  commits without consulting any report.
- Migration writes `iteration_status` into zero existing files.

### B. gx as sole `done` writer (ticket 03, ticket 11 §9A)

- `status: done` is written only after `landCherryPick` succeeds — or, on the validated-commitless
  route of section A, in its place — making `done` ⇒ landed-or-validated-commitless by construction
  and collapsing `classifyDoneTicket`'s ambiguity to one direction.
- **Fork children gain an implicit blocking edge on their parent's own `status: done`,
  non-recursive.** The existing recursive rule for `blocked_by` tokens is unchanged; the two
  predicates answer different questions, and recursion here would deadlock every fork. `parent` edges
  are already guaranteed acyclic.
- A parent that ends in a park state holds its child until a person unparks it.
- **CLI guard**: `gx tickets set` infers caller identity from the working directory's git branch. A
  `ralph-loop/*` prefix means an iteration agent, which is refused `--status` for every value
  **except** promoting `draft` → `open`. Branch beat the alternatives on *accident* resistance: a
  flag is the thing an agent reaches for by habit, a cwd path moves with worktrees, and an env var
  isn't available (`herdr agent start` takes none). It is **a guard-rail, not a boundary** — an
  agent that `cd`s out of its worktree evades it as easily as it could alias a flag away, and the
  failure mode this defends against is an agent following stale instructions, not one trying to get
  out. An unrecognised branch means no guard, never a false refusal.
- `needs-answer` / `needs-repair` are rejected from the CLI for **every** caller in both modes; they
  are machine-written. The CLI's settable set is `draft`, `open`, `claimed`, `done`. No override
  flag; `--force` keeps its single existing meaning.
- Nothing on disk marks a **hand-driven epic** versus a **loop-driven epic** — the guard keys off the
  caller, the TUI menu off live `loop_registry` state, and validation off the write.

### C. Announce-and-stop (ticket 04)

- An agent under ralph-loop **never calls an interactive prompt**. When it needs a person it: commits
  what is green; writes `## Needs Answer` (question, options weighed, what it would do with each),
  opening with a one-line summary; writes `## Handoff` (done / left / files / suggested skills,
  referencing by path); sets `--iteration-status needs-answer`; and exits.
- Not a fork — no child ticket, the same ticket resumes. The resuming agent retires both sections
  into `## Comments` and must not re-ask without new information.
- **Branch preserved, worktree dropped.** The iteration branch survives the wait; the worktree, tab
  and permit are released. Resume attaches a worktree to the *existing* branch — `AddWorktree` is
  unconditionally `-b` today, so this is real work — and computes its landing base as
  `MergeBase(featureBranch, iterBranch)`. `CherryPickRange` already lands multiple commits across the
  answer boundary in order.
- The adoption path **skips zero-commit fault detection**; that check belongs to the landing path.
- The normative rule lands in `gx-local-tracker.md` (three other bundle skills also run unattended);
  the commit-first procedure lands in `gx-implement/SKILL.md`.
- Scope: **voluntary** asks only. Involuntary permission prompts are section D.

### D. The orchestrator gate (ticket 06, ADR 0021)

- `blocked` joins Claude's `until` list, so the wait returns on herdr's first 300ms tick. A single
  re-check after `blockedDwellMs = 15_000` (unexported constant, like every other timing in that
  file) precedes the park. The dwell is a **fixed window, not a settle timer**: a pane that leaves
  and re-enters `blocked` inside it does not restart it — the re-check reads the pane's state at the
  end of the window and nothing else.
- gx writes `status: needs-answer` with a reason **and** a stub `## Needs Answer` section. The stub
  is what distinguishes this **pane-answered** park from C's **ticket-answered** one: the question
  exists only in the pane, so a person answers in herdr rather than in the file. It also gives the
  TUI auto-scroll a target.
- The pane is named by its **iteration label** in both the reason and the stub, never a raw pane id.

**The park ends the iteration** — the decision this section turns on. The parking write is followed
by the iteration goroutine *returning*, exactly as C's announce-and-stop does. The difference from C
is only what survives: C drops the worktree and keeps the branch, whereas here the agent is still
live in its pane, so the **pane, tab, and worktree all stay** and only gx's watcher goes away.

Everything else in this section is a consequence of that, and each consequence is a deletion:

- **The permit needs no new mechanism.** `active` decrements through the existing results channel and
  the existing `active == 0` release fires on its own. No `Permit.TryAcquire()`, no over-subscription
  path, no release from the iteration goroutine. The alternative — keeping the iteration alive to
  poll — splits `active` into "goroutines to wait for" and "work holding a slot", which forces a
  second counter, a try-first acquire, and cross-goroutine mutation of `permitHeld` (a plain bool
  owned by `Run`'s scheduler goroutine, so also a data race). ADR 0021's reasoning stands; this
  section simply no longer needs to invoke it.
- **The scheduler is not paused.** `waitForAttentionRecovery`'s `Gate.pause` — which today stops the
  whole epic from claiming — is **not** carried over. Pausing would contradict the promise that the
  run keeps going on other tickets, and a released permit buys nothing if the epic can't claim.
- **Smart-zone breach recovery cannot reach a parked pane**, because no iteration is running to
  perform it. This is a construction, not a suppression rule. The **inverse** guard is the one that
  needs stating: gx must **not park while it is itself mid-smart-zone-recovery**, since its own
  `ctrl+c`/`/compact` sequence can drive a pane into a state herdr reports as `blocked` (the same
  state Codex's compact confirmation produces). The gate checks that before parking.
- **No timeout**: a parked iteration burns no tokens and holds no slot.
- **The one place this is weaker than the mechanism it replaces**, stated rather than hidden: ADR
  0021 objected to a *blocking* `Acquire` on resume, because it makes gx wait for a slot in order to
  describe an agent that is already running. Park-and-exit re-acquires through the ordinary
  `active == 0 → acquirePermit()` path, which blocks. In the common case this costs nothing — if the
  epic has other tickets running the permit is already held and no acquire happens. The exposed case
  is a park that is the epic's only remaining work: gx may then wait on a slot while a live agent
  works in the pane. Accepted, because the wait ends as soon as any epic finishes, and the
  alternative is to reintroduce `TryAcquire` for one narrow case. If it ever bites in practice,
  try-first acquire is the fix and ADR 0021 already contains the reasoning.

**Unparking is a scheduler scan, not a recovery loop.** A pre-pass in the scan loop, on the scheduler
goroutine where the epic is already loaded off disk, walks the epic's `needs-answer` tickets; for each
one whose pane is still live and has left `blocked`, it **retires the stub** into `## Comments` and
sets `status: open`. The ticket then travels the reattach path that already exists —
`claimNext` sees `everLaunched[id]`, `resumeReattachable` finds the pane **by iteration label**, and
`launch(ticket, reattach: true)` picks the agent back up. A person who instead hand-edits the ticket
to `open` hits the identical path, so the two gestures cannot race each other and no ownership or
generation check is needed.

- **The live-pane predicate is what separates the two parks**, and it is the only thing that needs
  to. Both write `status: needs-answer`, but a C park has already released its pane, tab, and
  worktree, so `resumeReattachable` reports false and the pre-pass skips it — leaving it to be
  answered in the file and unparked by a person, which is C's whole design. A D park reports true and
  is auto-unparked. Nothing needs to record *which kind* a park was; the question "is there still an
  agent in there?" answers it, and answers it correctly even if a person kills the pane of a D park
  (it degrades into a C park and waits for a file answer, then relaunches fresh).
- **The scan needs one wake source it doesn't have.** With nothing else runnable the loop already
  re-reads the epic every `parkPollInterval` (30s), so unparking is free there. With siblings
  running it blocks indefinitely on `<-results`, which must become a `select` over `<-results` and a
  poll tick — reusing the existing `d.ParkTimer` seam. That is the section's only scheduler change.
- **`alreadyFinished` keeps excluding `blocked`** (`launch.go`), and the exclusion is now more
  clearly right, not less: a pane found already blocked at reattach must fall into `waitForFinish`
  rather than be treated as finished, where it takes the dwell and re-parks. Reattach → blocked →
  15s → park is the correct cycle, so no ticket should "simplify" that exclusion away.
- Both agents' blocked panes take this path: `waitForAttentionRecovery` collapses into a
  park-and-exit helper `parkOnBlocked(status, reason, pauseKind)` with Codex's quota check upstream
  of it, and its recovery half becomes the scan pre-pass above. This moves the Codex call site from
  the fault side to `needs-answer` **in this ticket**, not in the migration — which is also what
  makes the pre-pass sufficient for Codex, whose interventions resolve without anyone editing a
  ticket.
- Pre-flight: a `docs/` runbook referenced from a comment at the gate, plus an interactive
  `gx doctor` check that asserts `matched_rule.id == "live_blocked_form"` against a pane the operator
  has driven into the form. Interactive by necessity — nothing can drive Claude into that form
  headlessly.

### E. The park surface (tickets 05 / 12 / 13, ADRs 0018 / 0020)

- `TicketNeedsInfo`, the fault side's `IterationPaused`, and the dead-on-arrival
  `TicketStillNeedsAttention` collapse into **one `TicketNeedsHuman(identifier, epic, status,
  reason)`**, fired at gx's parking write. Ask-vs-fault becomes a rendering choice, not a plumbing
  fork.
- **TUI**: a reducer case records that the ticket parked and triggers a **re-read from disk**; the row
  subtext is the **park reason** — the first non-empty line after `## Needs Answer` /
  `## Needs Repair` — with markers stripped and ellipsised. Rendering is gated on **parked status,
  not on the section's presence**, so the two sides' different retirement mechanisms can't produce a
  stale row.
- Selecting a parked row auto-scrolls the preview to that section, highlighted **foreground**
  orange (`needs-answer`) / red (`needs-repair`).
- The per-epic header's existing `parked — …` slot carries the count, recoloring
  `epicStatusProblemStyle` rather than adding a third style. No tab-level total.
- **`## Needs Repair` shape**: summary line (guaranteed by the helper splitting the reason at its
  first newline — call sites pass `err.Error()` verbatim), optional detail, then a best-effort state
  block (`iteration`, `branch`, `worktree`; fields omitted rather than `unknown`). No fault-side
  `## Handoff`: no fault writer has an agent alive to author one.
- **Retirement principle**: *a section the next agent must consume is retired by that agent; a
  section only a person reads is retired by gx.* So the resuming agent retires C's `## Needs Answer`
  / `## Handoff`; gx demotes `## Needs Repair` into `## Comments` as a dated sub-entry **at claim,
  not at reattach** — which also fixes the live stacking bug where `*body +=` appends
  unconditionally. D's stub is the third case and follows the same principle to a different moment:
  no agent ever reads it (the question was answered in the pane, by the agent that is still running),
  so gx retires it **at unpark**, in the scan pre-pass that clears the status.
- **`draft` is an author park, and notifies nothing.** ADR 0020 counts `draft` as parked, which is
  right for scheduling — it is not runnable and only a person clears it — but it has no machine
  writer, so it carries no reason, no park section, no chat message, and no row subtext. Everything
  in this section is scoped to the two *machine* parks; `draft` keeps exactly its current surface.
- Validation is **write-conditional**, on the write that sets the status — never at rest, which would
  reject hand-authored tickets and make the loader fail on files it merely observes.
- `EpicParked` is unchanged: "nothing left to run" stays distinct from "one parked ticket in a busy
  epic".

### F. Chat coverage and counts (ticket 10, ADR 0022)

- **Rule**: chat carries changes to *what gx is running, what it's blocked on, and when it ends*, at
  ticket granularity or coarser. Anything inside one iteration, and all startup reconciliation
  housekeeping, is TUI-only.
- **Members**: `EpicStarted` (new), `IterationStarted`, `IterationPaused`/`Resumed`,
  `IterationFinished`, `TicketNeedsHuman`, `EpicParked`, `EpicComplete`, `EpicFailed` (new).
  `TicketUnrecoverable` reaches chat *via* `TicketNeedsHuman` (it is a parking write);
  `NoTicketsFound`/`AlreadyComplete` fold into `EpicStarted`, buying the invariant **every epic that
  leaves the queue emits exactly one start message**.
- **Structure**: the two decorator sinks collapse into **one `chatEventSink`** parameterized by an
  mrkdwn style and a transport, pinned by a **reflection contract test** that fails when an
  `EventSink` method is added without a chat verdict. `EpicStarted` fires **after `Acquire()`**, not
  at `Run` entry.
- **`EpicFailed` needs a reporter that outlives the sink.** It fires from the registry — the one
  documented exception — because `Run` has already returned by then; but `loopRegistry.finish`
  calls `sink.Close()` and waits on `drainDone` *before* it records the error, so as stated the
  message would be emitted into a closed sink and dropped. The registry gets a failure reporter that
  stays live across the drain and emits after it, and the contract test's documented exception
  covers the reporter, not just the call site.
- **One chat message per outcome.** A park currently emits both `IterationPaused` and (after the
  collapse) `TicketNeedsHuman`, and both are chat members. `TicketNeedsHuman` is the chat-visible one
  for every park from either source; `IterationPaused`/`Resumed` stay chat members only for pause
  kinds that are *not* parks. The ticket enumerates every outcome — both park sources, unpark,
  landing, validated-commitless close, rejected report, registry failure — against exactly one chat
  event and one owner each.
- **Counts become epic-truth** (`DoneCount`/`TotalCount`, recomputed from disk), identical whether the
  run is scoped, resumed, or fresh. This reaches the TUI too, since both read the same
  `IterationStats`/`LiveEvent` payloads. Counts are taken from the scan loop's **existing**
  `loadNamedEpic` result rather than a fresh walk — 592 ticket files re-parsed per chat event is a
  cost the scheduler has already paid on that pass.
- A `done` parent still `WaitingForChildren` keeps its existing counts treatment — **not done**, per
  `OpenCount`'s documented rule that an outstanding fork subtree is outstanding work — and lands in
  the counts line's **blocked** bucket, which is what it is. This is called out because the counts
  line makes the buckets visible for the first time and the overlay is the one status whose bucket
  isn't self-evident; nothing about `DoneCount` changes.
- **Counts line** replaces fractions: fixed order `done · in progress · parked · blocked · ready ·
  total`, zeros suppressed except `done` and `total`, parked identifiers listed inline capped at 5
  then `+N more`. A parallel **queue counts line** rides on the two epic-level messages only. A counts
  line appears only where counts materially moved — epic started, ticket landed, ticket parked, epic
  complete — never on ticket started/paused/resumed.
- **Message contract**: `{emoji} *{headline}*` / blank / optional counts line / optional detail line /
  `[gx] {epic}` or `[gx] {epic}/{identifier}` last — built by one shared constructor. The iteration
  label may appear as detail content, never as the identity line.
- Signature changes this forces: `IterationStarted` takes the full ticket (threaded into
  `agentRunParams`); `IterationPaused`/`Resumed` gain the identifier (every call site already has
  it).
- **No config**: a fixed set, no per-event on/off map, no verbosity level, at ~6–12 messages/hour.
- **There is no `gx ralph-loop` CLI.** `ralphloop.Run`'s only production caller is the TUI's loop
  registry; scope comes from the TUI's checkbox selection. `textEventSink`, `NewTextEventSink`, and
  `ralphloop.Report` are dead, and ~12 doc comments plus one user-facing string claim an invocation
  that doesn't exist — including `PauseNeedsAttention`'s own doc comment, which still points at
  `gx ralph-loop resume`.

### G. The migration (tickets 02 / 07 / 08 / 15, ADR 0018)

- **Clean cut, no compatibility read path.** The data payload is **one** ticket file of 592 (`.archive`
  has zero), so accepting both spellings would keep the vague names working and defeat the rename.
  Copy `lifecycle-contract`'s shape: build the migration vehicle first, cut the enum clean, keep the
  old names only as migrate-only legacy constants, reject the old shape loudly, pin it with a contract
  test, ban the names from skill prose.
- **Renamed**: `needs-info` → `needs-answer`, `needs-attention` → `needs-repair`, `isHumanClearable` →
  `isParked`, `MarkNeedsAttentionWithReason` → `MarkNeedsRepairWithReason`, `## Needs Attention` →
  `## Needs Repair`. `EventSink` method names, `LiveEvent` kinds, `PauseKind`, and **log values** are
  all in the rename — `run-log.jsonl` is mixed-vocabulary across the cut by design. `scripts/ralph-stats.mjs`'s
  `INTERVENTION_TYPES` set is now tracked and is inside the blast radius.
- `cmd/tickets_set.go`'s status enum and help text drop both park statuses.
- **Ban list** (`retiredTrackerTerms`, `skills/bundle_test.go`): `needs-info`, `needs-attention`,
  `isHumanClearable` as flat bans; **`agent_status:` with a trailing colon** — the narrowed form,
  because the scan is `strings.Contains` over bundle markdown and herdr's field must stay writable in
  prose (`` `agent_status` ``) and in quoted payloads (`"agent_status": "blocked"`). No allowlist
  mechanism. `stalled` and `push` stay **off** the list (common English, review-enforced).
- Historical mentions in `gx-investigate/gotchas.md` are **reworded, not exempted**.
- **House rule for superseded ADRs**: a one-line quoted note at the top naming the superseder, body
  never edited, written by whoever supersedes. ADR 0017 gets two such notes (0018, 0020).
- **Glossary** (`CONTEXT.md`, describing shipped behaviour so it lands with the migration): `adopt`,
  `iteration_status` / `agent_status` as a pair, **park reason**, **counts line**, the TUI/toast/chat
  surface terms, **hand-driven epic** / **loop-driven epic**.

### H. Phase order (ticket 14)

The **blocking graph is the only statement of the order**; this spec states rationale and never
restates the edges. The combined cut is four tickets in one phase:

- **M1** — rename cut: migration vehicle, enum, migrate-only legacy constants, loud rejection, contract
  test.
- **M2** — the `status` / `iteration_status` field split. Blocked by M1. **M2 is the phase gate.**
- **M3** — normative docs and the ban list. Blocked by M2.
- **M4** — ADR, `CONTEXT.md`, and spec prose. Blocked by M2.

Prose splits in two because `lifecycle-contract`'s single prose ticket hit **92K against a 30K
estimate** while its code cut came in at 42K against 50K — the file count is not where the cost is.

**Publish instructions for `/gx-to-tickets`** (the rule, never a list):

1. The four migration tickets come first and are numbered first.
2. Their internal order is M1 → M2 → {M3, M4}.
3. M2 is the phase gate; M3 and M4 are leaves.
4. Every other ticket carries `blocked_by: [<M2>]` **iff** it reads or writes a renamed symbol, a
   renamed status value, a renamed body heading, or the new `iteration_status` field — evaluated per
   ticket as it is drafted. Confirmed pre-cut today: the dead-code/stale-CLI sweep and the
   Tickets-tab keymap. **06's Codex call-site move is not an exception** — it writes `needs-answer`,
   which doesn't exist before the cut.
5. Chain the three `gx-local-tracker.md` writers — M3 → 04's docs ticket → 13's section-contract
   ticket — recording on those two edges that this is **file-contention serialisation, not a semantic
   dependency**. All three are leaves, so the chain costs nothing on the critical path.
6. No phase-boundary marker ticket: the wall of M2-blocked rows *is* the boundary.
7. The trailing `type: code-review` ticket is exempt from the fan-in by design.

### Ticket inventory

Beyond M1–M4: sole-`done`-writer + CLI guard; **validated-commitless close** (including the
reattached path); fork blocking edge; announce-and-stop docs; branch-reuse resume; **adoption
precedence over landing, in the iteration and in reconcile**; the gate (park-and-exit); **the unpark
scan pre-pass + `<-results` poll tick**; runbook + `gx doctor`; `TicketNeedsHuman` collapse; TUI
parked-row subtext + auto-scroll; per-epic header count + recolor; `## Needs Repair` section
contract; retirement at claim; Tickets-tab change-status keymap; `chatEventSink` collapse + contract
test; **`EpicFailed` registry reporter outliving the sink drain**; epic-truth counts; counts-line
renderer + message constructor; `EpicStarted`; iteration-event signature widening; dead-code and
stale-CLI sweep; trailing code review. Roughly 23–26 tickets.

The permit ticket that earlier drafts carried (`Permit.TryAcquire` + over-subscription) is **gone**,
not deferred: section D's park-and-exit shape removes the need for it.

## Testing Decisions

A good test here observes **external behaviour at the highest available seam** — what gx writes to a
ticket file, what it sends to a sink, what the reducer renders — never a private helper's shape. The
seams below already exist and carry fakes; prefer them to new ones.

- **`Deps.AgentWait` fake (`ralphloop`)** — the existing seam for the gate. Covers: blocked → dwell →
  park write → iteration returns; and "no `AgentPrompt` call is made while the pane is blocked",
  which is the destructive-interrupt regression in one line. Prior art: `waitforfinish` tests as
  rewritten by `compact-issue`.
- **`Deps.ParkTimer` fake plus the scan loop** — the seam for unparking: a parked ticket whose pane
  has left `blocked` is set `open`, its stub retired, and reattached rather than relaunched. The case
  worth its own test is **a park while a sibling is still running**, since that is the one the
  existing `<-results` block would otherwise starve. Prior art: `loop_park_test.go`.
- **Fake `Permit` (`ralphloop/permit_test.go`)** — "a park releases the epic's slot", asserted
  through the *existing* release path. There is deliberately no try-first or over-subscription case
  to write; that the fake needs no new methods is the check that section D's simplification held.
- **Ticket-file round trip through the `tickets` package** — the seam for the field split, the CLI
  guard, the park write contract, the section contract, and retirement at claim. Assertions are on
  the file as loaded, not on frontmatter strings. Prior art: `lifecycle-contract`'s enum contract
  test and `cmd/tickets_set_test.go`.
- **Deadlock regression for the fork edge** — `02` done + `02b` open ⇒ `02b` schedulable; `02`
  claimed + `02b` open ⇒ `02b` blocked. Required seam, stated by ticket 03.
- **`EventSink` reflection contract test** — enumerates every method against an explicit chat yes/no
  map and fails when a method is added without a verdict, with `EpicFailed`'s registry-reporter
  origin as the one documented exception. This is the enforcement mechanism, not merely a test — so
  the map must be written by enumerating **all ~25 methods on the interface**, spelling out the ~16
  explicit *no* verdicts, not by transcribing section F's members list. A map that only lists the
  members is not a safety net; it is the members list with extra steps.
- **Counts-line renderer as a pure function** — table test over the five documented shapes (fresh,
  resumed, already done, empty, mid-run park), including zero suppression and the 5-identifier cap.
- **`reduceLiveEvent`** — the existing reducer seam for parked-row subtext and header counts; assert
  on rendered row state given an event plus an on-disk ticket, since the subtext is read from disk.
- **`skills/bundle_test.go`** — the seam for the ban list. A fixture with `` `agent_status` `` in
  prose passes; one with `agent_status:` in a frontmatter block fails.
- **Real-git tests (`run_realgit_*`)** — the seam for branch-reuse resume: stop with commits, resume,
  land both commits in one pick.
- **`gx doctor`'s blocked-form check is deliberately not automated** — nothing can drive Claude Code
  into that form headlessly and no pane is blocked at startup, which rules out both a test and a
  startup assertion. The runbook is the durable artifact.

## Out of Scope

- **A generic idle-agent watchdog.** Consultant finding 5, reaffirmed twice: the gate covers the
  failure mode actually observed, and a general watchdog is a much larger design with no confirmed
  second use case.
- **Resuming a parked epic whose `Run()` goroutine is gone** — the pre-existing `queue-parked-reattach`
  epic. It inherits a rewritten premise and must be **re-scoped after this epic lands**: *both* parks
  now end their iteration, so no park leaves a dead goroutine behind, and in-process resume is the
  scan pre-pass. What survives for that epic is the narrow case its body already points at — the gx
  process died or restarted while an epic was parked — plus the case where a pane-park was the epic's
  last runnable ticket, so `Run` returned via `EpicParked` while the pane stayed alive. That second
  case is today's behaviour unchanged, not a regression this epic introduces.
- **`compact-issue`'s deferred write-clobber race** between the scheduler's ticket writes and an
  agent's own writes. That is an ownership/locking bug; this epic's race is semantics and survives
  perfect locking.
- **Enriching the fault-side state block** by threading worktree/branch back through the iteration
  results channel. Best-effort emission is day one; this earns its own ticket if it ever earns one.
- **Unifying `## Handoff` with the mid-flight fork path's design notes.** An impl-level cleanup,
  deliberately not decided.
- **Per-event chat configuration** and any verbosity level.
- **Reopening the `iteration_status` name** or the six-value `status` enum.

## Further Notes

- **ADRs already written** by the map: 0018 (`needs-answer` vs `needs-repair`), 0019 (`status` and
  `iteration_status`, amended in place by ticket 04's `adopt` rename), 0020 (parked, not stalled),
  0021 (permit bounds agents gx starts), 0022 (chat event coverage), 0023 (status ownership by
  writer). 0017 is superseded in part by 0018 and 0020, and **0019's Consequences section is
  superseded in part by 0023** — 0019 still says `--status` accepts `needs-answer`/`needs-repair` and
  that `claimed`/`done` are reachable with `--force`, both of which 0023 reverses. Per section G's
  house rule the superseder writes the note, so this one belongs to 0023 and lands in M4.
- **ADR 0021 needs a note of its own, written by this spec's section D.** Its headline claim — the
  permit bounds the agents gx *starts*, so a park may release a slot while a live agent sits in a
  pane — is what still licenses releasing the permit at a park, and stands. Its **Consequences are
  superseded**: there is no `Permit.TryAcquire()`, no try-first re-acquire, and a run's peak agent
  count no longer exceeds `MaxConcurrentEpics` in the way it describes, because the parked iteration
  has exited. ADR 0017's older "released on park" model turns out to be the accurate one. This note
  lands in M4 alongside the 0023 one.
- No new ADR is expected from implementation.
- **`lifecycle-refactor` is fully merged into `main`**, as are `compact-issue` and
  `lifecycle-contract`. `main` is the read target; citations against the old branch are stale.
- **Run-order dependency**: the `lifecycle-contract` epic must land strictly after
  `lifecycle-refactor` merges and gx is rebuilt. Not a `blocked_by` edge.
- The origin diagnosis lives at
  `.scratch/lifecycle-refactor/issues/11a1-interactive-askuserquestion-invisible-stall-research.md`;
  don't reopen it.
