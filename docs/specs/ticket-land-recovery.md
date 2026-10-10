# Manual ticket landing for stuck-ticket recovery

## Problem Statement

A ralph-loop iteration can finish its work and still leave the ticket stuck. The agent committed on
its iteration branch, but the commits never reached the epic's feature branch: the run crashed, the
budget killer fired, the pane wedged, or the land queue never got to it. The ticket sits `claimed` or
`needs-repair`, its worktree, branch and tab still on disk.

Today there is no way out of that state that does not cost more than the work it recovers.

- gx already knows how to re-land those commits — `reconcile`'s repair path does exactly this — but
  it only runs at `ralphloop.Run` startup. Recovering one ticket means relaunching the whole loop on
  a dead or parked epic.
- Doing it by hand means cherry-picking the range yourself and then guessing at the
  `Ralph-Loop-Ticket` trailer. That trailer is scoped by **feature branch name**, not epic slug. A
  hand-written value that gets the scoping wrong never counts as landed, so the ticket looks
  recovered and is not. This single detail is why a command beats written instructions.
- A hand landing also skips the frontmatter metrics. `actual_cost == 0` is read elsewhere as "never
  landed", so a concurrent budget kill can re-park a ticket that was just recovered.
- There is no read that answers "did this ticket's work actually land?". The one exported helper is
  trailer-only, so it disagrees with the orchestrator's own three-rung check.
- The other ending — "this work is junk, run the ticket again" — has no command either. Deleting the
  iteration branch by hand also kills its reflog, and a leftover herdr tab makes the "clean re-run"
  silently attach to the dead agent's session instead of starting fresh.
- When a ticket parks on a question, a person has no obvious place to answer it. The resume gesture
  exists in the `m` menu, but nothing points at the `## Needs Answer` section that needs writing
  first. The original failure was discoverability, not a missing mechanism.

The person hitting all of this is usually mid-investigation, with the `gx-investigate` skill open.
That skill is chartered "never edits code", so it can only describe the recovery it just diagnosed
and hand the human a list of manual git commands to mistype.

## Solution

Give recovery its own commands, built on gx's own landing logic, and let the investigation skill
drive them.

Four commands under the existing `tickets` noun, one ticket per invocation:

- **`gx tickets land <epic> <id>`** — re-lands a stuck ticket's commits the way gx would:
  cherry-pick, metrics, trailers, `status: done`, run-log event. Conflict is a reported outcome, not
  a crash.
- **`gx tickets verify <epic> [id]`** — a write-free read that reports, per ticket, whether its work
  landed and what evidence says so. No policy, no guard.
- **`gx tickets reset <epic> <id>`** — the other ending: attic the iteration branch, clear the
  machine-written frontmatter, close a stale tab, and record why.
- **`gx tickets unpark <epic> <id>`** — the existing "Resume (I answered)" write, callable from a
  script or an agent that cannot press `m`.

Underneath, one shared landing seam in `ralphloop` so manual landing and live landing can never
drift apart, and one shared landing-truth gatherer so `verify` and `land` can never disagree.

Around them, two human-facing changes:

- The **`gx-investigate` skill widens**: it may repair ralph-loop state, never product code, and
  every write needs an explicit go-ahead from the person.
- The **`m` menu gains an "Answer…" item** that opens `$EDITOR` at the `## Needs Answer` heading and
  offers to resume the ticket when an answer was actually written.

## User Stories

### Landing a stuck ticket

1. As a developer recovering a dead epic, I want to re-land one stuck ticket's commits with a single
   command, so that I do not have to relaunch the whole ralph-loop run just to trigger its startup
   repair.
2. As a developer, I want the landing to use gx's own cherry-pick-and-stamp path, so that a manually
   landed ticket is byte-for-byte indistinguishable from one the orchestrator landed.
3. As a developer, I want the `Ralph-Loop-Ticket` trailer written for me, so that I cannot
   accidentally scope it by epic slug instead of feature branch and produce a landing that never
   counts as landed.
4. As a developer, I want `land` to derive the commit range from the ticket's deterministic iteration
   branch, so that I never have to work out the merge-base myself.
5. As a developer with an unusual case, I want `--from`/`--to` overrides, so that I can land an
   explicit range when branch discovery does not apply — and so that a future commit-recovery effort
   has the argument it needs without reopening this seam.
6. As a developer, I want landing to be idempotent, so that re-running `land` on an already-landed
   ticket reports "already applied" and exits 0 rather than double-applying or erroring.
7. As a developer, I want `land` to accept `done`, `claimed` and `needs-repair` tickets, so that the
   statuses a stuck ticket actually sits in are all covered.
8. As a developer, I want `land` to refuse `draft` and `open` tickets, so that a command that lands
   commits never runs against a ticket where no iteration ever ran.
9. As a developer, I want `land` to refuse a `needs-answer` ticket, so that a ticket which is merely
   waiting on me — with a designed resume path and an agent that will keep building on the same
   iteration branch — does not get its work split across two separate landings.
10. As a developer, I want `land` to always leave the ticket at `status: done` — including when the
    commits were already landed and only the status is stale (`AlreadyApplied` on a `claimed` ticket
    writes `status: done`) — so that landing is the declaration of doneness and there is no
    half-landed state with commits on the branch but no status.
11. As a developer, I want `land` to refuse a ticket explicitly flagged commitless, so that a ticket
    whose author declared it has no commits is never handed a commit range.
12. As a developer, I want the metric trailers and frontmatter fields stamped on a manual landing,
    so that a later budget kill does not read `actual_cost: 0` as "never landed" and re-park the
    ticket I just recovered.
13. As a developer, I want gx to recover the agent session for those metrics from the run log rather
    than asking me for it, so that the command still takes only an epic and a ticket id.
14. As a developer, I want gx to never fabricate metrics it cannot read, so that a missing session
    shows up as an empty session on the run-log event rather than a plausible-looking wall-clock
    number invented at land time.
15. As a developer reading recovered metrics, I want `gx tickets land --help` and the `## Comments`
    note the skill writes after a landing to say that elapsed time is the least trustworthy of the
    three, so that I do not treat a manually landed ticket's duration as the time it really took.

### When the cherry-pick conflicts

16. As a developer, I want a conflicting cherry-pick to be a reported outcome rather than an error,
    so that a script can branch on it and a person is not shown a stack trace for an ordinary event.
17. As a developer, I want `land` to exit 0 for every outcome including a conflict, and exit non-zero
    only for a genuine failure, so that exit codes stay meaningful in a wrapper script.
18. As a tool author, I want `--json` to emit the landing result verbatim, so that an agent can act
    on structured data instead of parsing prose.
19. As a developer resolving a conflict by hand, I want the sequencer state left intact, so that I
    can fix the conflict in the feature worktree and continue.
20. As a developer, I want `land --continue` to verify that the sequencer is clear and that HEAD
    actually moved before it stamps, so that a "continue" on an abandoned resolution cannot stamp a
    trailer onto work that never landed.
21. As a developer who changed my mind, I want `land --abort` to abort the cherry-pick and clear the
    land marker and lock, and touch nothing else (no status, no ticket file), so that the escape
    hatch has a blast radius I can predict and a leftover marker never blocks a later landing of a
    different ticket.
22. As a developer, I want no status write on a conflict, so that a ticket is never marked done while
    its commits sit unresolved.
23. As a developer returning to a machine, I want a land marker beside the lock, so that gx can tell
    my pending, deliberate resolution apart from stale state left by a crash.
24. As the orchestrator, I want to read that same marker, so that a live run and a human resolution
    do not each assume the other's sequencer state is garbage.

### Not stepping on a live run

25. As a developer, I want the invariant to be "nothing may land a ticket a live iteration currently
    owns", so that the rule is about ownership rather than about trusting one caller more than
    another.
26. As a developer, I want `land` to refuse when a live herdr tab exists for the iteration, so that
    the only liveness signal gx actually has is the one that gates the write.
27. As a developer recovering a crashed run, I want an override for that refusal, so that a tab which
    outlived its agent does not block exactly the case this command exists for.
28. As a developer, I want `land` to refuse to run from a `ralph-loop/*` working directory, so that
    an iteration agent does not land its own ticket by following a stale instruction.
29. As a developer reading `gx tickets land --help`, I want the ralph-loop-directory refusal
    described there as a guard rail rather than a boundary, so that nobody mistakes an evadable `cd`
    for a security control.
30. As a developer, I want `land` to compute every target from the epic and ticket arguments, with
    only the guard reading the working directory, so that where I stand never changes what gets
    landed.
31. As an orchestrator maintainer, I want the land queue to take the same on-disk lock `land` takes,
    so that in-process serialization stops being the only thing keeping two landings apart.

### Verifying

32. As a developer mid-investigation, I want a write-free `verify` read, so that I can ask "did this
    land?" without changing anything.
33. As a developer, I want `verify` to answer for non-`done` tickets too, so that the common stuck
    case — `claimed` or `needs-repair`, because the loop lands before it marks done — is the case it
    handles best.
34. As a developer, I want `verify` built on the same three-rung evidence ladder the orchestrator
    uses, so that `verify` and `land` can never give me different answers about the same ticket.
35. As a developer, I want `verify` to report status, landing, evidence and leftovers as separate
    fields with no verdict folded over them, so that a `claimed` ticket which has leftovers by design
    is not mislabelled as broken.
36. As a developer, I want `verify` to name which rung answered, so that I can judge how much to
    trust it before acting on it.
37. As a developer, I want "unknown" to be a real answer that exits 0, so that a check which could
    not run is never silently reported as "not landed".
38. As a developer, I want `verify` to carry no guard at all, so that reading state during an
    investigation is never blocked by a live tab.
39. As a developer, I want `verify` to take no lock and read the land marker instead, so that a read
    can never block a landing, and a read taken mid-landing labels itself as such.
40. As a developer at a terminal, I want the human table filtered to the interesting rows with a
    required count of what was hidden, so that a long epic stays readable and I still know something
    was left out.
41. As a tool author, I want `--json` to always be unfiltered, so that a machine consumer never has
    to know about the human filter.
42. As a maintainer, I want landing facts gathered in one place and policy applied by its callers, so
    that the next consumer does not need a second truth function.

### Resetting instead of landing

43. As a developer who judged the work incomplete, I want `gx tickets reset <epic> <id>`, so that the
    investigation has a second ending besides landing.
44. As a developer, I want reset to move the iteration branch to an attic ref rather than delete it,
    so that the commits and the reflog survive a decision I might regret.
45. As a developer, I want reset to clear only the machine-written frontmatter and to retire any
    `## Needs Repair` / `## Needs Answer` section into `## Comments` (never delete it), so that
    everything a person or agent wrote on the ticket is preserved, including a parked question.
46. As a developer, I want reset to require a reason and write it into the ticket's comments framed
    as unverified partial work, so that the next agent knows the branch history is not a clean slate.
47. As a developer, I want reset to refuse when an agent is genuinely alive on the tab, so that I
    cannot pull the state out from under a working iteration.
48. As a developer, I want reset to close a stale tab rather than merely refuse under it, so that the
    "clean re-run" does not attach to the dead agent's session because the agent name is still taken.
49. As a developer, I want reset to refuse outright on a ticket that has fork children, with no
    force, so that I do not re-block every child of a parent whose criteria already moved onto them.
50. As a developer, I want to be told to reset the child instead, so that the refusal comes with the
    next action.
51. As a developer, I want reset to accept `claimed`, `needs-repair` and `needs-answer`, and `done`
    only with `--force`, so that its list can differ from `land`'s where discarding a parked ticket
    splits nothing.
52. As a developer, I want `--force` to only silence the already-landed refusal and never revert a
    landing, so that reset never rewrites the shared feature branch.
53. As a developer resetting during a live run, I want a warning that the effect is only half
    complete, so that I am not surprised when downstream tickets unblock but this one is not
    re-queued until the run restarts.
54. As a developer, I want a missing iteration branch to be a normal outcome rather than an error, so
    that the frontmatter half still runs and the result says plainly that there was no branch.
55. As a tool author, I want every refusal from all four commands (`land`, `verify`, `reset`,
    `unpark`) to carry a machine-readable reason code in one shared `--json` envelope, so that the
    skill and any later auto-recovery branch on data rather than on prose.
56. As a developer, I want `gx cleanup scan` to report attic refs and never remove them, so that I
    can see what accumulated without a sweep deciding for me.

### Unparking and answering

57. As an agent launched from the `m` menu, I want `gx tickets unpark <epic> <id>`, so that I can
    perform the resume write even though I cannot press a key in a TUI.
58. As a developer whose ticket parked on a question, I want an "Answer…" item in the `m` menu, so
    that I have an obvious place to write the answer instead of hunting for the file.
59. As a developer, I want "Answer…" to open my editor positioned at the `## Needs Answer` heading,
    so that I land on the question rather than the top of the file.
60. As a developer, I want gx to compare that section before and after my edit, so that it can tell
    whether I actually answered.
61. As a developer who answered, I want a confirm modal offering to resume the ticket, so that
    resuming is one keystroke away from writing the answer.
62. As a developer who did not answer, I want a modal that says so and offers to keep editing, so
    that I do not accidentally resume a ticket whose question is still open.
63. As a developer whose parked agent still has a live pane, I want the menu to offer "Answer in
    pane" instead, so that I answer where the agent is waiting and the existing auto-unpark clears
    the park by itself.
64. As a developer, I want "Resume (I answered)" to stay in the menu, listed second, so that the
    gesture I already know does not disappear.
65a. As a developer reading a parked ticket's rendered `## Needs Answer` section, I want a one-line
    hint beside it pointing at the "Answer…" menu item, so that I can find where to answer.
65b. As a developer whose editor opens in a split or tab, I want "Answer…" to tell me to edit the
    ticket by hand rather than pretend it can detect my answer, so that I am never told "no answer
    found" while the editor is still open.

### The investigation skill

65. As a developer running `gx-investigate`, I want the skill allowed to repair ralph-loop state, so
    that it can finish the recovery it just diagnosed instead of handing me git commands.
66. As a developer, I want its charter to stay "never edits product code", so that widening it does
    not turn a diagnostic skill into an implementer.
67. As a developer, I want every write to require my explicit go-ahead, so that an unattended
    auto-investigate agent can never land or reset on its own judgement.
68. As a developer, I want `verify` treated as always free, so that reading costs me no approval
    prompts.
69. As a developer, I want the skill — not `land` — to judge completeness, so that the judgement
    happens where the acceptance criteria and the diff can both be read.
70. As a developer, I want the skill to check the acceptance criteria against the diff and run the
    repo's checks in the iteration worktree, so that "it landed" is not confused with "it is
    finished".
71. As a developer, I want the transcript treated as advisory only, so that an agent's own account of
    its work never outweighs the diff.
72. As a developer, I want thin evidence to block the skill's autonomy but never my own explicit
    "land it anyway", so that I stay the final authority.
73. As a developer, I want the ending chosen by status first and completeness second, so that a
    ticket is never offered an ending its status does not allow.
74. As a developer, I want a `needs-answer` ticket routed to answer-then-unpark and never to either
    ending, so that the resume path is not bypassed.
75. As a developer, I want every failure branch to stop and report, branching on the JSON reason
    codes and naming my next gesture including the `m` menu path, so that a stopped investigation
    always tells me what to do next.
76. As a developer, I want a live agent on the tab to start an investigation rather than end one, so
    that the skill goes on to read iteration status, run log, transcript and `verify` to explain why
    the ticket is not finishing.
77. As a developer with a wedged pane, I want the skill to be able to send a single bare keypress
    nudge under four required preconditions, so that an agent that wedged after it started working
    can be poked at all — gx itself stops nudging once "working" is observed.
78. As a developer, I want that nudge restricted to bare keypresses and never text, so that the skill
    cannot answer a question on my behalf.
79. As a developer, I want the nudge sent once and followed by a fresh observation, so that it cannot
    turn into a loop.
80. As a maintainer, I want the nudge marked interim with a sunset pointing at the auto-recovery
    effort, so that the daemon adopts this rule instead of inventing a second one.
81. As a developer, I want each recovery reported at the action — a run-log event, plus a comment on
    the ticket for a landing — rather than only in the skill's write-up, so that the record survives
    the investigation session.
82. As a developer, I want a nudge to leave no ticket record, so that a keypress is not dressed up as
    a state change.

### Machine consumers

83. As the future auto-recovery effort, I want both commands fully non-interactive and
    machine-callable, so that a tier-2 remedy can shell out to them rather than reimplementing
    landing.
84. As a tool author, I want the JSON contract and exit codes stable across all four commands, so
    that one parsing strategy works for all of them.

### Additions from design review

85. As a developer, I want reset to remove the iteration worktree as well as attic the branch, so that
    the clean re-run does not fail with `agent_name_taken` on a surviving worktree.
86. As a developer who wants the work gone for good, I want `reset --delete-branch`, so that pruning
    the attic is a deliberate gesture of mine and never a side effect.
87. As a developer, I want `land` on a ticket whose iteration branch is gone to refuse cleanly with
    a reason code and point me at `--from`/`--to`, so that I know the commits are not recoverable by
    discovery but can still be landed by explicit range.
88. As a developer, I want `verify --all` for the full table, so that the filtered default never
    hides rows I want to see.
89. As a developer, I want `land` to fail fast with a `land_locked` reason when another landing or a
    pending `--continue` holds the lock, so that a hand landing never hangs, and I want the live
    land queue to defer that ticket and retry rather than block, so that my unresolved conflict
    never stalls a running epic.
90. As a developer who just recovered a ticket, I want to know that its leftover worktree, branch and
    tab are expected and get swept by a later live run, so that I do not read `verify`'s leftovers
    as a broken recovery.

## Implementation Decisions

### The landing seam

- The cut is made **inside** the existing conflict-resolving cherry-pick, not around it. The conflict
  machinery wraps the happy path rather than following it, so a cut placed outside would have to
  duplicate the happy path.
- Two pieces come out: a **cherry-pick core** that performs the pick and reports whether it
  conflicted, and a **land stamp** that writes metrics, trailers and resolves the landed SHA.
- The exported entry point is `ralphloop.LandTicket`, composed of both. It stays in `ralphloop`; it
  is not promoted to its own package.
- It takes narrowed `LandDeps` and `LandParams` types, not the full orchestrator `Deps`. `LandParams`
  carries an explicit `SourceRange` rather than deriving the range internally, so the commit source
  is an argument.
- It returns a `LandResult` with a `Landed | AlreadyApplied | Conflicted` outcome plus the landed
  SHA, the trailer value and whether metrics were stamped (see "JSON contracts"). **A conflict is a
  result, never an error.**
- Both existing callers reroute through it: the live iteration land path and reconcile's repair path.
  There is no second landing implementation.
- Guardrail is an **on-disk lock shared with the land queue**, not a refusal to run while a loop is
  live. The orchestrator's land queue starts taking that same lock — its serialization is currently
  in-process only.
- **Acquisition policy: fail fast, never block.** `land` that finds the lock held refuses with reason
  `land_locked`. The land queue that finds it held (including by a human's pending `--continue`,
  which can last hours) does not wait: it parks that ticket as "land deferred" and retries on its
  next tick. Neither side can deadlock a live run.
- A conflict leaves sequencer state intact so a `--continue` mode has something to continue.

### `gx tickets land`

- Lives under the `tickets` noun beside `set` and `validate`. Ticket-scoped, one ticket per
  invocation. No epic-wide sweep.
- `--json` emits `LandResult` verbatim. **Exit 0 for every outcome including `Conflicted`; exit 1
  only for a real error.**
- A **land marker** is written beside the lock when `land` leaves a conflict. It records epic,
  ticket id, source range and the pre-pick HEAD SHA. It distinguishes a human's pending resolution
  from stale crash state. Both `land` and the orchestrator read it.
- **Preflight is three-way** on the marker and `CherryPickInProgress`: marker for *this* ticket ⇒
  report "conflict pending" and point at `--continue` / `--abort`; marker for a *different* ticket ⇒
  refuse; cherry-pick in progress with **no** marker ⇒ refuse, never trample an unknown owner.
- On the orchestrator side, `cherryPickWithConflictResolution` checks the same marker and **skips its
  stale-abort** when one exists, so a loop starting mid-resolution cannot destroy a human's work.
- `--abort` aborts the cherry-pick and **clears the marker and lock**, touching nothing else.
  `--continue` verifies the sequencer is clear and that HEAD differs from the marker's pre-pick SHA
  before stamping. No status is written on a conflict.
- **Accepted statuses: `done`, `claimed`, `needs-repair`.** Refused: `draft` and `open` (no iteration
  ran) and `needs-answer` (waiting on a person, with a designed resume path; its commits sit on the
  same deterministic iteration branch the resuming agent keeps building on).
- **Landing is the declaration of doneness**: `land` always leaves the ticket at `status: done`. There
  is no `--done` flag and no status-less mode. Consequently `land` never judges completeness itself —
  that judgement is the skill's, and this is a hard constraint on it.
- Already-landed ⇒ `AlreadyApplied`, exit 0, detected by the **full three-rung ladder** (recorded-SHA
  reachability → patch-equivalence → trailer), never trailer-only. `AlreadyApplied` does no trailer
  re-stamp but **does write `status: done` when the current status is not `done`**. This is the cure
  for the most common stuck case: commits already landed, ticket still `claimed`. Without it nothing
  would repair the status.
- **Missing iteration branch**: when no `--from`/`--to` is given and the iteration branch is gone,
  `land` refuses with reason `iteration_branch_missing` ("no iteration branch; commits not
  recoverable") and tells the caller that `--from`/`--to` can still land an explicit range.
- **Expected leftovers**: `land` cleans nothing. A manually landed ticket therefore classifies as
  `doneStaleCleanup`, not `doneOK`, and a later live run sweeps its worktree, branch and tab. This is
  intended. `verify` reporting leftovers on a just-recovered ticket is normal, not a failure.
- The `--help` text states the guard-rail nature of the working-directory refusal and that recovered
  elapsed time is the least trustworthy of the three metrics.
- Refuses on the explicit commitless flag only, never on a broader type-derived notion of
  commitlessness.
- `SourceRange` is derived from the iteration branch by default, with `--from`/`--to` as a
  no-discovery escape hatch.
- Guard: refuses when run from a `ralph-loop/*` working directory. Deliberately **guard-rail tier** —
  evadable by changing directory, no attestation token — and the help text says so. Every target is
  computed from the epic and ticket arguments; only the guard reads the working directory.
- Liveness is the **herdr tab**, gx's only such signal. `land` refuses when the iteration's tab
  exists, with an override flag, because a tab outlives its agent and would otherwise block the
  recovery case. **No new `landing` status is introduced** — `claimed` plus a finished iteration
  status already is one, and a status cannot answer a liveness question.
- Metrics are recovered, not fabricated. The landing-metrics writer needs only agent, cwd, session id
  and ticket path; the run log plus the last-iteration-session lookup recovers that from the epic and
  ticket id alone — the same pattern the commitless metrics stamp already uses. When no session is
  recoverable, the `manual-land` event's empty agent-session field plus its reason is the legibility
  marker. **No new frontmatter field, and no wall-clock fallback.**
- The deciding reader for metrics is the frontmatter, not the trailers: all three metric trailers are
  write-only today, and a zero `actual_cost` is read elsewhere as "never landed", so omitting metrics
  would let a concurrent budget kill re-park a recovered ticket.

### `gx tickets verify`

- `gx tickets verify <epic> [id]`. Write-free. **No guard at all** — deliberately asymmetric with
  `land`: reading owns nothing, and refusing under a live tab would break the investigation case.
- Built on the same three-rung ladder, so `verify` and `land` can never disagree.
- Covering non-`done` statuses is its **main** case: the loop lands before it marks done, so a stuck
  ticket is almost always `claimed` or `needs-repair`.
- Reports `(status, landing, evidence, leftovers)` as **orthogonal fields with no policy folded in**.
  Naming the rung that answered is what gives the skill grounds for trust. The exact shape is in
  "JSON contracts".
- `unknown` is an answer and exits 0. It is never collapsed into "not landed".
- Herdr is optional. No lock is taken; it reads the land marker so a read taken mid-landing
  self-labels.
- Human output is a filtered table with a **required** hidden-count footer. The filter keeps anything
  whose landing is not `landed`, plus anything `landed` whose status is not `done`. `--all` shows the
  full table. `--json` is always unfiltered, regardless of `--all`.
- **Architecture: facts are shared, policy is not.** `ralphloop.VerifyEpic(VerifyDeps, VerifyParams)`
  becomes the single landing-truth gatherer. The existing done-ticket classifier becomes a fold over
  it. `LandedTickets` un-exports — it is trailer-only, and the caller its doc claims does not exist.
  A write-free `VerifyDeps` is a type-level proof that verify cannot mutate.

### `gx tickets reset`

- The recovery's second ending. One ticket per invocation, matching `land`.
- **Attics** the iteration branch rather than deleting it: the iteration branch name has no attempt
  counter, and deleting it kills the reflog too.
- Clears the machine-only frontmatter through a new `tickets.Reset`: `status` → **`open`**, and
  zeroes `iteration_status`, `park_kind`, `session_ids`, `actual_context_window`, `elapsed_time`,
  `actual_cost`, `compactions`, `commitless`. It also **retires any `## Needs Repair` /
  `## Needs Answer` section into `## Comments`**, so a parked question is preserved. None of these
  fields past `status` are `gx tickets set`-settable, by design.
- **Removes the iteration worktree** and closes the tab (both cheap to recreate). A surviving worktree
  at the deterministic path would make the clean re-run fail with `agent_name_taken`.
- **`--delete-branch`** deletes the branch instead of attic-ing it, for a genuinely-gone reset. It is
  the only deliberate way to prune an attic'd branch; nothing else ever does.
- **Fork children ⇒ refuse outright, with no force.** Re-opening a parent re-blocks every child, and
  the fork protocol already moved the parent's criteria onto them. The refusal names the child as the
  thing to reset instead.
- Resettable statuses: `claimed`, `needs-repair`, `needs-answer`; `done` only with `--force`.
  Deliberately unlike `land`'s list — discarding a parked ticket splits nothing.
- `--force` **only** silences the already-landed refusal. It never reverts a landing; rewriting the
  shared feature branch is outside reset's blast radius.
- Tab handling splits three ways on whether an agent is alive: agent alive ⇒ refuse; stale tab ⇒
  close it, because the launch path silently *attaches* to a stale pane when the agent name is taken,
  so a leftover tab would make a "clean re-run" adopt the dead agent's session; no tab ⇒ nothing.
- A missing iteration branch is **not an error**: the frontmatter half still runs and the result
  records a null attic ref, said out loud.
- Requires `--reason`, written into `## Comments` framed as unverified partial work.
- Every refusal carries a `--json` reason code, in the shared refusal envelope (see "JSON contracts").
- **Regression under a live run is accepted as half-effective**, with a warning rather than a fix:
  the launched set is cleared for parked outcomes but never for a landed ticket, so a forced reset
  un-blocks downstream while never re-queueing the ticket itself until the run restarts.
- **Nothing prunes the attic.** `gx cleanup scan` learns to *report* attic refs and never to remove
  them.

### Run-log events

- Two new event types: `manual-land` and `ticket-reset`.
- `manual-land` carries the recovered agent session when there was one, and an empty session plus a
  reason when there was not — that emptiness is the marker that metrics could not be recovered.
- Writing these from a one-shot CLI needs an exported append entry point on the event log; the
  existing exported "log notifications configured" writer is the precedent.
- **`Event` schema change.** The existing `Event` struct (`ralphloop/eventlog.go`) has `SHA`,
  `Reason` and `AgentSession` but nothing for the new payloads. Add `Outcome` and `TrailerValue`
  (used by `manual-land`) and `AtticRef` (used by `ticket-reset`). All three are `omitempty`.
- **No on-disk locking for the append.** `logEvent` serializes only in-process, so a CLI appending
  while a loop runs relies on `O_APPEND` atomicity: each event is written with a single `write` call
  and stays under `PIPE_BUF` (4096 bytes). Events must keep to that size. The `Reason` text is the
  only unbounded field, so it is truncated to fit.

### JSON contracts

All four commands share one contract.

- **Success**: exit 0, the command's result as JSON on stdout.
- **Refusal or error**: exit 1, one envelope on stdout:
  `{"refused": true, "reason": "<code>", "message": "<human text>"}`. `reason` is a stable
  machine-readable code (for example `land_locked`, `iteration_branch_missing`, `live_agent_on_tab`,
  `fork_children`). The skill branches on `reason`, never on `message`.
- **`land`** emits `LandResult`:
  `{"outcome": "landed|already-applied|conflicted", "sha": string, "trailer_value": string,
  "metrics_stamped": bool}`. `metrics_stamped` is false when no session was recoverable.
- **`verify`** emits a list of `TicketVerification`:
  `{ID, Status, Landing, Evidence, SHA, Leftovers, Unknown}`.
  - `Landing` ∈ `landed | recoverable | unrecoverable | not-expected | unknown`.
  - `Evidence` ∈ `sha | patch-id | trailer | none`.
  - `Leftovers` is `{Tab, Worktree, Branch}`, each a `*bool`. **`null` means herdr did not answer**,
    never `false`.
  - A landing in flight (land marker present) is reported alongside the list.
  - `recoverable` / `unrecoverable` keep the existing done-ticket classifier's vocabulary without the
    `done` prefix. `not-expected` covers commitless and never-iterated tickets.

### `gx tickets unpark`

- A thin command over the already-exported unpark write. It exists because the investigate agent
  launched from the `m` menu cannot press `m`.
- This reframes the epic's original premise: the unpark gesture already exists, and a live loop needs
  no kick because it reloads the epic each iteration. The original failure was **discoverability**.

### The `m` menu "Answer…" item

- Opens `$EDITOR` at the `## Needs Answer` heading, then on exit compares that section's body before
  and after, and offers a standard confirm modal: *"Resume the ticket?"* when answered, *"No answer
  found… Keep editing?"* when not.
- **In-place edits only.** The editor-finished message fires on editor exit only for in-place
  launches. For a split or tab launch it fires when the pane is created, while the editor still runs.
  So the before/after comparison and the modal run only on the in-place path. When the terminal would
  split, "Answer…" opens the editor at the heading but shows a hint to edit the ticket by hand and
  then use "Resume (I answered)". No new wait-for-exit mechanism is built.
- The rendered `## Needs Answer` section also gains a one-line hint pointing at the menu item.
- The menu **branches on park kind** by whether the pane is live. Pane live ⇒ "Answer in pane", which
  only focuses the tab and lets the existing auto-unpark clear the park. Pane gone ⇒ "Answer…".
- This widens the suggested-actions builder's signature on purpose.
- "Resume (I answered)" stays, listed second.
- Existing pieces: the confirm modal component, the in-place editor exit message, and the editor
  launch helper's currently unused line-number argument. The split-mode path has no exit signal, hence
  the in-place restriction above.

### The `gx-investigate` skill

- Charter changes from "never edits code" to **"never edits *product* code — may repair ralph-loop
  state"**.
- **Every write needs an explicit go-ahead.** `verify` is write-free and always free. The unattended
  auto-investigate agent must never land on its own judgement.
- Completeness verification is the skill's job alone. Acceptance-criteria-vs-diff and repo checks in
  the iteration worktree are **required**; the transcript is advisory only. Thin evidence blocks the
  skill's own autonomy, never the human's explicit "land it anyway".
- The ending is forced **by status first** (reset's and land's accepted-status lists differ), then by
  completeness. `needs-answer` is answer-then-unpark and never either ending.
- Every failure branch stops and reports, branching on the JSON reason codes, and always names the
  human's next gesture including the `m` menu path.
- **A live agent on the tab starts an investigation, not a stop.** The land refusal stands, but the
  skill then reads iteration status, the run log, the transcript and `verify` to answer *why it is
  not finishing*.
- **Interim pane nudge, newly in scope.** gx's only `enter` is the launch trust dialog, and its
  prompt nudger stops nudging once "working" is observed — so an agent that wedges after starting
  gets no nudge at all. The skill may send bare keypresses only (never text, so it cannot answer for
  a human), under four required preconditions, once, then re-observe. Marked interim with a sunset
  pointing at the auto-recovery effort's own remedy ticket.
- Recoveries are reported **at the action** — run-log event, plus a `## Comments` note for a
  landing — never through the skill's diagnosis write-up. A nudge records nothing.
- The skill's gotchas file gains one line about the originating bug only.

## Testing Decisions

A good test here asserts external behavior: the outcome value returned, the ticket file's resulting
frontmatter and body, the commits and trailers on the feature branch, the run-log lines appended, and
the exit code and JSON shape of the command. It does not assert which internal helper was called or
in what order.

### Seams (settled while charting — not reopened here)

- **`ralphloop.LandTicket(LandDeps, LandParams) (LandResult, error)`** is the landing seam. Narrowed,
  write-capable deps; explicit `SourceRange`; conflict returned as a `LandResult` outcome rather than
  an error. Both the live iteration path and reconcile's repair path go through it, which is what
  makes one set of landing tests cover all three callers.
- **`ralphloop.VerifyEpic(VerifyDeps, VerifyParams)`** is the landing-truth seam. Write-free deps, so
  a test can prove by type that verify cannot mutate. The done-ticket classifier becomes a fold over
  its output, so the ladder is tested once and both consumers inherit it. `LandedTickets`
  un-exports and is no longer a seam.

These two are the whole approved seam set. Everything else is tested through existing seams.

### Additions this spec makes to the seam set

- **An exported run-log append**, needed because `manual-land` and `ticket-reset` are written from a
  one-shot CLI rather than from inside a run. This is a small extension in the shape of the existing
  exported notifications-configured writer, not a new architectural seam — but the map never named
  it, so it is called out here.
- **`tickets.Reset`**, named by the reset decision, for clearing machine-only frontmatter.

### What gets tested

- **The landing seam** — landed / already-applied / conflicted outcomes; trailer value scoped by
  feature branch; metrics stamped when a session is recoverable and omitted (never fabricated) when
  it is not; idempotence across a second call.
- **The verify gatherer** — each rung answering in isolation, the rung name reported, `unknown` on a
  check that could not run, orthogonal fields for a `claimed` ticket with leftovers by design.
- **The `land` command** — status acceptance and refusal per status, the ralph-loop-branch guard,
  the live-tab refusal and its override, exit codes, `--json` shape, `--abort` and `--continue`.
- **The `reset` command** — attic ref created, frontmatter cleared, comment written, fork-children
  refusal, stale-tab close vs live-agent refusal, missing branch as a normal outcome, reason codes on
  every refusal.
- **The `unpark` command** — same resulting file as the menu action.
- **The menu** — item set per park kind, and the answered/not-answered branch after the editor exits.

### Prior art

- `ralphloop/reconcile_repair_test.go` and `ralphloop/reconcile_classify_test.go` for exercising the
  repair and classification paths against a fake `Deps`.
- `ralphloop/landqueue_test.go` for the land path's outcome assertions.
- `ralphloop/loop_cherrypick_test.go` and `ralphloop/conflict_resolution_production_test.go` for
  conflict behavior.
- `cmd/tickets_set_test.go` for CLI-level tests that call the command's `run*` function directly,
  including the existing ralph-loop-branch status guard — the closest prior art for `land`'s own
  guard.
- `cmd/cleanup_scan_test.go` for a `--json`-vs-human dual-output command.
- `ui/tickets/suggested_actions_test.go` for the menu item set per status.

## Out of Scope

- **Epic-wide headless reconcile.** Ruled out in favour of ticket-scoped commands.
- **Recovering commits whose iteration branch no longer exists.** Confirmed not the cause here.
  Discovery is feasible — dangling commits are subject-tagged with epic and ticket — but time-boxed
  by git's prune expiry, identified by agent-authored prose, and unable to distinguish lost commits
  from a successful landing's rehashed originals without corroboration. The explicit `SourceRange`
  supplies the argument a later effort would need, so this can be built without reopening the seam.
  Standing constraint for that effort: recovered commits must be judged complete against the ticket
  before landing, because an interrupted iteration's partial series matches the subject convention
  just as well as a finished one.
- **Any other change to how the orchestrator lands during a live run.** The one exception is the land
  queue taking the shared on-disk lock, which is in scope.
- **Auto-recovery in the orchestrator-daemon effort.** `land` and `reset` are tier-2 remedies for it,
  making it a second consumer of these commands, but building that loop is out of scope. The standing
  constraint it needs — both commands stay fully non-interactive and machine-callable — is delivered
  by the JSON and exit-code contract above. The scoped exception is the tier-1 pane nudge granted to
  `gx-investigate` as explicitly interim; the rest of tier-1 stays out of scope.
- **The Codex metrics gap.** Codex session stats are built with no cost field while the
  metrics-available flag stays true, so every Codex ticket lands `actual_cost: 0` — which is read
  elsewhere as "never landed". Pre-existing in the live path, not introduced by manual landing.
  Recorded so the next person meets it as a known bug rather than a discovery.
- **Teaching the TUI Tickets tab about landing state.** The weaker trailer-only helper's documented
  UI caller does not exist, so nothing is left dangling when it un-exports. A future TUI wanting this
  would need caching — the verify gatherer is one trailer map plus per-ticket git calls, not
  per-frame safe. That is the TUI's problem, never a reason for a second truth function.
- **Whether announce-and-stop is the right way for an agent to ask a question at all.** gx already
  does both kinds of park: a gate park keeps its pane live and auto-unparks when the pane unblocks;
  an announce-and-stop park deliberately releases everything to free the concurrency permit. The
  complaint lands on the second. Rethinking that tradeoff is a real question about how agents *ask*,
  not about recovering stuck tickets. Its near-term symptom — not knowing where to answer — is
  handled by the "Answer…" menu item without touching the tradeoff.
- **Cleanup of iteration worktrees, branches and tabs by `land`.** `land` lands. Mid-investigation
  that state is evidence. The removal recovery needs lives on `reset`, which removes the worktree and
  closes the tab, and deletes the branch only with `--delete-branch`. A landed ticket's leftovers are
  swept by a later live run.

## Further Notes

### Stated implementation risk

Adding the on-disk land lock to the orchestrator's own land queue may deadlock or regress a live run.
The land queue currently serializes in-process only; giving it a shared cross-process lock is the one
in-scope change to live landing, and it is the change most able to hurt a running epic. This is
recorded as a risk to watch during implementation, **not** an open design question — the decision to
share the lock stands. The mitigation is the fail-fast acquisition policy under `gx tickets land`:
nothing blocks on the lock, and the land queue defers on contention.

The risk is scoped to the land queue and `land` itself. `verify` is excluded: it never takes the
lock, reading the land marker instead.

### Root cause, for the record

Reconcile's repair path already does everything `land` needs to do. It simply only runs at
`ralphloop.Run` startup, so investigating a dead or parked epic cannot trigger it without relaunching
the whole loop. This spec does not add a capability gx lacked; it makes an existing one reachable
one ticket at a time.

### Why a command and not documentation

The `Ralph-Loop-Ticket` trailer is scoped by feature branch name, not epic slug, and the landed-check
cuts that branch prefix and skips anything that does not match. A hand-written trailer with the wrong
scoping produces a ticket that looks landed to a human and is invisible to gx. Prose instructions
cannot make that safe; a command can.

### Trustworthiness of recovered metrics

- **Tokens** — trustworthy. It is a peak, not a sum.
- **Cost** — trustworthy on Claude. Always `$0.00` on Codex, pre-existing and out of scope above.
- **Elapsed** — the misleading one. It measures until the agent last spoke, not until the work
  landed, and covers only the most recent iteration attempt. Stamped anyway, because the alternative
  is a zero that reads as "never landed" — but never invented when no session can be read.

### The premise corrections found while charting

Three beliefs this effort started with turned out to be wrong, and the design above reflects the
corrections rather than the originals:

1. The ralph-loop branch guard was assumed to block gx's own landing status write. It does not — the
   guard lives only in `tickets set`, and gx has always written through `ralphloop`. There was no
   exemption to grant; `land` is a new command inheriting no guard, so it needed one written for it.
2. `gx cleanup` was assumed to already sweep leftover state. It is scan-only, with no removal
   subcommand at all. Nothing sweeps today, which is why removal landed on `reset`.
3. The trailer-only landed helper was assumed to be exported for a UI caller. That caller does not
   exist, and trailer-only would have let `verify` disagree with `land`. It un-exports, and verify is
   built on the full ladder.
