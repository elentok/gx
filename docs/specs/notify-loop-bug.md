# Fix the false-finish / infinite-reclaim notification loop

Produced by [01 — waitForFinish's idle debounce is too short for a real background job](issues/01-waitforfinish-idle-debounce-too-short-for-background-job.md),
which diagnosed this live on `test-suite-perf/04b1` and `test-suite-perf/06`, and refined through a
second-opinion consultation covering 11 findings (see resolution ledger in conversation history).

## Problem Statement

When an agent legitimately backgrounds a long shell command (`go test`, `go test -race`) and goes
quiet waiting for the automatic completion notification — the documented, correct way to wait —
`waitForFinish` can't tell that quiet apart from a genuine end of turn. Its idle-detection debounce
(3s + 2s) is sized for a brief mid-turn output gap, not for a background job that can run minutes
to hours. Once the debounce elapses, `finishIteration`'s zero-commit path marks the ticket
needs-answer and leaves the worktree/tab/pane alive for inspection — but `unparkAnswered` then
can't distinguish that from a person having genuinely answered a parked ticket, since both look
like a live, non-blocked pane. It reopens the ticket, the scheduler reattaches to the same
still-busy pane, and — because reattach has its own separate short-circuit that skips
`waitForFinish` entirely when the pane already looks idle — immediately re-concludes "finished"
again with no debounce at all. Each cycle fires a Telegram notification, producing an unbounded
burst (12+ minutes / dozens of messages in the incidents observed) for something that was never
actually broken.

Separately, two different situations write the same "needs-answer, pane left alive" shape today —
a pane genuinely blocked on an interactive prompt, and gx's own uncertain zero-commit guess — and
nothing on the ticket records which one happened. (A third situation, the agent self-reporting a
real question, writes the same needs-answer status but releases its pane/worktree/tab via
`finishCleanup` before parking, so it's not actually part of the "pane left alive" ambiguity —
worth tracking as its own `park_kind` for triage even though it isn't part of this bug's
mechanism.) That ambiguity between the two live-pane cases is what lets `unparkAnswered` misfire on
the zero-commit case.

## Solution

**Fix 1 — give `waitForFinish` (and the reattach path) a way to know a background task is still
outstanding, and stop guessing.** `transcript` (the package that already reads Claude Code's session
`.jsonl` for context occupancy and compaction boundaries) gains a reader that pairs a backgrounded
shell command's start marker (`backgroundTaskId`) against its resolution (a `task-notification`
matching the same task id, matched primarily on task id with `tool-use-id` as corroboration when
present). The reader reports one of five states per relevant marker: `resolved` (a matching
notification exists — its status text, success/failure/killed, doesn't matter, only that a
notification exists), `outstanding-fresh` (no matching notification yet, marker age under the cap),
`outstanding-aged-out` (no matching notification, marker older than the cap — neutral: neither
gates nor counts as evidence), `unreadable` (transcript exists but failed to parse), or
`unsupported` (no transcript at all, e.g. Codex). Sidechain (subagent-originated) markers are
excluded entirely — a subagent's background task belongs to the subagent's lifetime and must never
gate the parent iteration. An `unreadable`/`unsupported` read is never evidence for anything: the
gate fails open (default to pre-fix behavior, don't hold) and Fix 2's disproof (below) fails closed
(don't reopen) on it.

While a task is `outstanding-fresh`, the pane's idle/done status is provisional, not authoritative —
mirroring how a compaction-boundary count already overrides a premature idle report during
smart-zone recovery. This gate is consulted at **both** of the two places an iteration can conclude
"finished": `confirmFinished` (the normal path) and `reattachIteration`'s `alreadyFinished`
short-circuit (which today skips `waitForFinish`'s debounce entirely when a reattached pane already
looks idle — the exact mechanism both real incidents exercised, since the false-finish → reopen →
reattach cycle rehits this short-circuit on every pass). While the gate holds, the check sleeps
`smartZonePollMs` between reads rather than spinning; the outer wait's own elapsed-time accounting
accumulates (does not reset) while the gate holds, so a caller with an existing `FinishTimeoutMs`
(conflict-resolution agents, 30 min) still eventually times out rather than becoming immortal. Each
hold/release/expiry is logged to `run-log.jsonl` once per task id, not per tick, naming the task id
so a future investigation doesn't require re-reading the raw session transcript by hand. A marker
that stays `outstanding-fresh` for roughly 2 hours ages out (its own distinct expiry event logged)
and falls through to today's zero-commit park behavior — a deliberately generous cap sized for a
legitimately long test suite, not the 3-10 minute jobs seen so far. Claude-only for now — Codex has
no equivalent transcript signal, reads as `unsupported`, and fails open exactly as it does for the
existing compaction-boundary gate, so this is a pure addition with no behavior change for Codex
iterations.

`waitForBackgroundTasks` (the gate this fix adds) is a work-preservation guard against a premature
park/teardown mid-background-job — it stops `confirmFinished`/`reattachIteration` from concluding
"finished" while a backgrounded command is still outstanding. It is not, by itself, the fix for the
notification-storm/reopen-loop bug described above: that storm is independently prevented by Fix 2's
`park_kind`/`clearableParkedTicket` mechanism (a `zero-commit` park only auto-reopens once new
commits appear, never on the background-task signal resolving by itself). Without Fix 1, both real
incidents would still eventually self-heal once the agent's commit lands — Fix 1 just removes the
stall window in between, by never mis-parking the ticket as needs-answer while the job is still
running.

**Fix 2 — record which kind of park happened, and stop inferring it from pane liveness.**
Needs-answer parks record which of three things happened, in a new `park_kind` frontmatter field:
`blocked-pane` (a genuinely blocked interactive prompt), `self-reported` (the agent deliberately
asked a real question — pane already released), or `zero-commit` (gx's own uncertain guess — no
commits, no self-report, pane left alive). A ticket parked before this field existed (no `park_kind`
at all) is treated as `zero-commit` — the conservative default — but **only when the ticket's
rendered status is `needs-answer`**; `needs-repair` and `draft` parks never carry a `park_kind` and
keep today's liveness-based reattachability unchanged.

`blocked-pane` and `self-reported` auto-reopen once the pane is live and unblocked, same as today.
`zero-commit` auto-reopens only when the pane is live **and commits have appeared** on the ticket's
branch since it was parked — never on the background-task signal resolving by itself (a resolved
signal is a one-way/latching fact, so treating it as a reopen trigger on its own would let the same
park be disproved and re-parked forever; commits are the only trigger that is inherently
self-terminating, since a successful reopen either lands work and finishes or genuinely re-parks).
Clearing a `zero-commit` park with no new commits requires a person to look at the ticket and flip
its status by hand.

This "is this park clearable" question is asked from three places today (`unparkAnswered`, the
park-timer's `hasLiveParkedTicket` liveness check, and `EpicParked`'s `Reattachable` flag surfaced in
the Telegram notification itself) and is extracted into one shared predicate so the three can't drift
out of sync — in particular so the Telegram message never claims a `zero-commit` park is reattachable
when gx will never auto-reattach it. The predicate takes the epic and branches on
`epic.RenderedStatus(ticket)` (not raw `Status` — the three existing call sites reach status two
different ways today, and a raw-`Status` predicate would route a ticket down the wrong branch
whenever they diverge). The commits check (`CommitsAhead`) only runs when it's actually needed
(`park_kind == zero-commit && live`) so the common case pays nothing extra, and a git failure
degrades to "not clearable" rather than propagating as an error that would kill the whole run.

`park_kind` is cleared in `Claim` — the single choke point every reopen path funnels through
(auto-unpark, a hand-edited status flip back to open, the queue tab's suggested-actions menu) —
rather than only in the auto-unpark path, so a `zero-commit` park that a person clears by hand
doesn't keep a stale, misleading label after it's resolved normally.

## User Stories

1. As someone running ralph-loop, I want an iteration that's genuinely waiting on a backgrounded
   `go test` run to stay claimed until that job resolves, so that I don't get flooded with false
   needs-answer notifications while the agent is still working.
2. As someone running ralph-loop, I want a ticket that's falsely flagged needs-answer while its
   agent is still busy to never get silently reopened and reclaimed by the scheduler, so that one
   false alarm can't turn into an unbounded reopen/reclaim loop — including the reopen → reattach →
   immediate-re-finish cycle both real incidents actually hit.
3. As someone running ralph-loop, I want a ticket that's blocked on an interactive prompt gx didn't
   send to keep auto-reopening once I clear that prompt, exactly as it does today, so that this fix
   doesn't touch the one park kind that already works correctly.
4. As someone investigating a "why is this ticket stuck claimed" report, I want `run-log.jsonl` to
   show that a background task was outstanding and gated a false-finish, so that I don't have to
   re-read the raw session transcript by hand to reconstruct what happened (as this bug's own
   diagnosis required, twice).
5. As someone investigating a needs-answer ticket, I want to see at a glance whether it was parked
   because a pane was genuinely blocked, because the agent asked a real question, or because gx
   guessed from zero commits, so that I know whether it's safe to just answer in the pane or whether
   it needs a closer look.
6. As someone running ralph-loop against Codex tickets, I want Codex iterations to behave exactly as
   they do today, so that a Claude-specific fix doesn't introduce a new gap or regression for the
   other agent kind.
7. As someone who already has tickets parked needs-answer when this fix ships, I want those old
   parks to default to the conservative `zero-commit` behavior, so that an in-flight false park
   doesn't get auto-reopened into the same loop this fix is meant to close.
8. As someone reviewing this fix later, I want the background-task gate to ride on a generous,
   explicitly-logged cap (not the tiny debounce it replaces) rather than holding forever, so that a
   legitimately long-running background job (an hour-long test suite, say) is never mistaken for a
   stuck iteration, while a genuinely abandoned one doesn't wedge the scheduler indefinitely either.
9. As someone running a conflict-resolution agent with its own overall time limit, I want time spent
   waiting on a held background-task gate to still count against that limit, so that a stuck
   background job can't make the agent immortal.
10. As someone watching the parked-epic Telegram notification, I want its "reattachable" claim to
    agree with what the scheduler will actually do, so that I'm not told a `zero-commit` park is
    reattachable when gx will never auto-reattach it.
11. As someone whose ticket has a `needs-repair` or `draft` park (unrelated to this bug), I want its
    reattach/liveness behavior to be completely unaffected by this fix, so that a change scoped to
    the notification-loop bug doesn't regress a different, unrelated park kind.
12. As someone whose ticket reattaches to an already-idle pane after a genuine pause between commits
    (no background task involved), I want gx to still apply a brief debounce before concluding the
    iteration is finished, so that a partial commit isn't landed and the worktree torn down out from
    under an agent that's still mid-turn.

## Implementation Decisions

- **New reader in the `transcript` package** (alongside `ReadOccupancy`/`ReadCompactions`): given a
  session's `.jsonl` path, scans for non-sidechain `tool_result` entries carrying a
  `backgroundTaskId`, then scans forward for a later entry with `origin.kind == "task-notification"`
  whose `<task-id>` matches (accepting `<tool-use-id>` as corroboration when present, not required).
  Reports, per marker, one of five states: `resolved`, `outstanding-fresh`, `outstanding-aged-out`
  (against the ~2h cap), `unreadable`, `unsupported`. Include enough (task id) for the run-log event
  to name it concretely.
- **Wired into `Deps`** the same way `ReadOccupancy`/`ReadCompactions` are: a function field keyed by
  `cwd, sessionID`, called only for `p.Agent == AgentClaude` (Codex reads as `unsupported` — fails
  open, no behavior change).
- **Gates two call sites in `waitforfinish.go`/`iteration.go`**: `confirmFinished`'s conclusion (once
  the existing debounce confirms the pane looks idle, an `outstanding-fresh` task additionally holds
  that conclusion open) and `reattachIteration`'s `alreadyFinished` short-circuit (which today skips
  `waitForFinish` entirely) — the latter needs a session id to read from, recovered the same way
  Finding 3 already established: `agent.AgentSession` when non-empty, else the most recent id in the
  ticket's `SessionIDs`, else `lastIterationSession` over the run log. Both gates mirror how
  `compactBoundarySnapshot`/`stickyBaseline` already gate a premature idle report during smart-zone
  recovery — additive checks, not replacements for the existing debounce.
- **Pacing while held**: sleep `smartZonePollMs` between re-reads instead of a tight loop (mirrors
  `waitForCompactionSignal`'s existing pacing). Elapsed-time accounting accumulates across a held
  gate rather than resetting, so an existing `p.FinishTimeoutMs` (set only by conflict-resolution
  callers today) still bounds total wait time; whichever bound is shorter — the ~2h per-marker cap or
  a caller's `FinishTimeoutMs` — wins with no extra logic.
- **Cap**: ~2 hours of accumulated `outstanding-fresh` time per marker before it ages out to
  `outstanding-aged-out` and falls through to today's zero-commit park path. A distinct run-log event
  marks expiry, separate from the hold/release events.
- **New `run-log.jsonl` events**: gate-held (once per task id, naming it), gate-released, and
  gate-expired — not one event per poll tick.
- **New ticket frontmatter field `park_kind`** (`omitempty`), three values:
  - `blocked-pane` — written where `parkOnBlockedPane` parks today (genuinely blocked interactive
    prompt).
  - `self-reported` — written where `adoptNeedsAnswerReport` parks today (the agent's own
    `iteration_status: needs-answer`). Pane is already released via `finishCleanup` by this point.
  - `zero-commit` — written where `finishIteration`'s zero-commit fallback parks today (no commits,
    no self-report, pane left alive).

  These three call sites currently share two functions (`MarkNeedsAnswer`,
  `MarkNeedsAnswerWithReasonAndStub`); each needs to stamp the right `park_kind` for its site.
- **New shared predicate `clearableParkedTicket`** in `unpark.go`, taking the epic and a ticket,
  branching on `epic.RenderedStatus(ticket)`:
  - `needs-answer`: read `park_kind` (missing ⇒ `zero-commit`); `blocked-pane`/`self-reported` ⇒
    clearable once live and unblocked, same as today; `zero-commit` ⇒ clearable only when live AND
    `CommitsAhead` shows new commits since park (git check gated behind this branch so it's skipped
    entirely for the common cases; a git failure degrades to "not clearable", never propagates as an
    error).
  - `needs-repair`/`draft`: unchanged liveness-only rule, exactly as today — `park_kind` is never
    consulted for these.
  Used by `unparkAnswered`, `hasLiveParkedTicket`, and `EpicParked`'s `Reattachable` flag (`loop.go`)
  — all three call the same function instead of three separate approximations of the same question.
- **`park_kind` lifecycle**: cleared in `Claim` (`loop.go`), the single choke point every reopen path
  (auto-unpark, hand-edited status, suggested-actions menu) funnels through — not in `UnparkTicket`
  alone, which would miss the hand-edited path.
- **Reattach-debounce gap (bonus, in-scope since the same code is already being edited)**: apply the
  same `confirmFinished`-style debounce check inside `reattachIteration`'s `alreadyFinished` branch
  even when no background-task marker is involved, so a genuine pause between commits (no background
  task at all) can't be misread as finished with zero debounce.
- **CONTEXT.md glossary addition**: "Background task" — a shell command a Claude agent moved off its
  own foreground turn, resolved later by a `task-notification` matched on task id; "outstanding"
  while unresolved, "resolved" once a matching notification is seen (regardless of the reported
  status), "aged out" once it exceeds the cap without resolving. Distinct from the pane-level
  Reattach/Live terms already documented — this is turn-level, inside one still-live iteration. Also
  update the existing Parked/Unpark glossary section: name `park_kind`'s three values, correct the
  "announce-and-stop park releases its pane" claim to apply only to `self-reported` (not
  `zero-commit`), and note that "gate park"/"announce-and-stop park" are retired in favor of
  `blocked-pane`/`zero-commit`/`self-reported` to avoid colliding with the already-established `Gate`
  type and `.gates()`.
- **Fix 1 → Fix 2 sequencing**: soft/prudential ordering, not a hard data dependency — Fix 2's
  disproof predicate no longer reads Fix 1's signal at all. Ship Fix 1 first anyway: without it, Fix
  2 alone would still self-heal both observed incidents once the agent goes on to commit, but there'd
  be a stall window between the false park and that commit landing. `gx-to-tickets` should record
  this as an ordering preference, not a blocking edge — don't over-serialize the two tickets.

## Testing Decisions

- **The background-task reader** (`transcript` package): test against fixture `.jsonl` files (same
  style as `transcript`'s existing occupancy/compaction tests) covering: no background task at all;
  one resolved (including a non-`completed` terminal status, e.g. failed/killed — must still read as
  `resolved`); one outstanding-fresh; one outstanding-aged-out; a sidechain-scoped marker (must be
  excluded, never produced as a gating marker); a `task-notification` for an id that never had a
  matching start marker (malformed/unrelated — must not be treated as outstanding); an unreadable
  transcript file; a missing transcript file (`unsupported`).
- **The `confirmFinished` gate and the `reattachIteration` short-circuit gate**: `ralphloop` tests
  using the existing `herdrfake` pane-status fixtures, holding a pane idle (no output) longer than
  the existing debounce while the new signal reports `outstanding-fresh`, asserting neither path
  concludes finished until the signal clears or ages out — prior art: the existing smart-zone
  recovery tests that assert on `compactBoundarySnapshot`/`stickyBaseline` gating behavior.
- **Pacing/accumulation**: a test asserting elapsed time accumulates (does not reset) across a held
  gate, and that a caller-set `FinishTimeoutMs` still fires while the gate is held.
- **The new run-log events**: assert gate-held (once, not per tick), gate-released, and gate-expired
  are each written with the right ticket id / task id, using the existing `run-log.jsonl` event-log
  test helpers.
- **`park_kind` stamping**: one test per producer (`parkOnBlockedPane`, `adoptNeedsAnswerReport`,
  `finishIteration`'s zero-commit fallback) asserting the ticket lands with the right `park_kind`
  value — prior art: existing tests around `MarkNeedsAnswer`/`MarkNeedsAnswerWithReasonAndStub` in
  `claim_test.go`.
- **`clearableParkedTicket`**: table-driven over `park_kind` values (including missing) crossed with
  rendered status (`needs-answer`, `needs-repair`, `draft`), pane liveness, and commit presence,
  asserting: `blocked-pane`/`self-reported` clearable once live+unblocked; `zero-commit`/missing (on
  `needs-answer` only) clearable only when live AND commits appeared, and specifically **not**
  clearable on live-with-no-commits even when the background-task signal reads resolved;
  `self-reported` + no live pane ⇒ not clearable (the pane was already released, so "live" never
  applies here); `needs-repair`/`draft` behavior unaffected by `park_kind` entirely. A git-failure
  case asserting "not clearable", not an error. Prior art: `unpark.go`'s existing tests already
  exercise the live/blocked pane matrix; this extends it by `park_kind` and rendered status.
- **`Claim` clearing `park_kind`**: a test asserting a ticket claimed via any route (auto-unpark or a
  hand-edited status flip) loses its `park_kind` field entirely (`omitempty` — absent from
  frontmatter, not an empty string).
- **`EpicParked`'s `Reattachable` flag**: a test asserting it agrees with `clearableParkedTicket`
  rather than raw liveness, specifically for a `zero-commit` park (must report not reattachable) vs a
  `blocked-pane` park (must report reattachable once live).
- Only external behavior is asserted throughout (ticket status/frontmatter after the call, run-log
  contents, whether `waitForFinish`/`reattachIteration` conclude finished) — never internal call
  counts or private state.

## Out of Scope

- Codex support for background-task detection. Codex has no equivalent transcript signal today;
  this fix does not add one, and Codex iterations behave exactly as they do before this fix
  (reader reports `unsupported`, gate fails open).
- Any change to how `parkOnBlockedPane`'s blocked-pane detection itself works — only how its park is
  labeled.
- Retroactively re-labeling tickets already parked before this ships. They're covered by the
  missing-field default (treated as `zero-commit` when rendered status is `needs-answer`), not by a
  migration that rewrites their frontmatter.
- Renaming `Gate`/`.gates()` or any other existing use of "gate" already established in the
  codebase — only the informal, previously-undocumented "gate park"/"announce-and-stop park" names
  are retired.
- Any change to `needs-repair`/`draft` park handling. They deliberately keep today's liveness-only
  rule; `park_kind` is never read or written for them.

## Further Notes

This bug was diagnosed twice against the same shape (`test-suite-perf/04b1`, then
`test-suite-perf/06`), both times an agent correctly backgrounding a multi-minute `go test`/`go test
-race` run and going quiet exactly as instructed — meaning this is not a rare edge case but a
routine one: any ticket whose implementation includes a test run long enough to background is a
candidate for this loop today. See ticket `01` for the full transcript/run-log evidence from both
incidents.

This spec went through an 11-finding second-opinion consultation after first being drafted. Two of
those findings changed the design materially after the first pass looked complete: dropping the
background-signal clause from Fix 2's disproof predicate (an early draft would have let a resolved
signal re-disprove the same park forever), and gating `reattachIteration`'s short-circuit in
addition to `confirmFinished` (an early draft only fixed one of the two places an iteration can
falsely conclude "finished" — the reattach path is the one both real incidents actually looped
through).
