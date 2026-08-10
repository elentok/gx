# Smart-zone compaction: gate the finish-up prompt on a confirmed compaction boundary

Shipped and merged to main. This document was reconciled with the implementation after the fact:
the snapshot-robustness design, the finish-poll gate, and the file-order invariant's one exception
describe what landed, not the pre-implementation plan, which the epic's review tickets `05a` and
`05b` revised for reasons recorded below.

## Problem Statement

When an agent iteration's context grows past the configured smart zone, gx interrupts the agent,
sends `/compact`, and then sends a "finish-up" prompt telling the agent its context was compacted
and asking it to wrap up quickly.

The finish-up prompt is being delivered while the compaction is still running. Claude Code queues
incoming text while it is busy, and delivering that queued text cancels the in-progress compaction.
The context is never reduced. Thirty seconds later the smart-zone poll reads the same over-budget
number and does the whole thing again — a second interrupt, a second `/compact` — on top of whatever
work the agent had resumed.

From the operator's point of view: an iteration visibly starts compacting, the compaction is
cancelled, the agent is interrupted twice in ninety seconds, and roughly a minute and a half of
agent time is destroyed. In the diagnosed incident the whole compact-and-finish-up contract completed
in 4.5 seconds — physically impossible for a real compaction — and the session transcript contained
exactly one compaction boundary, belonging to the *second* `/compact`, not the first.

There is a second, quieter version of the same problem. Even when a compaction genuinely succeeds,
gx's occupancy reading stays stale until the agent produces its next assistant turn, so the very next
poll tick can declare another breach and interrupt the finish-up work gx just correctly delivered.

## Solution

gx already reads the only ground-truth signal that a compaction completed: Claude Code appends a
compaction boundary line to its session transcript when, and only when, a compaction finishes. A
cancelled compaction writes none. gx snapshots the count of those boundaries before submitting
`/compact` and can compare it afterwards.

Today that signal is used as a fallback for a *slow* pane and is discarded the moment the pane
reports itself idle. The fix makes it authoritative: the finish-up prompt is sent only once the
boundary count has actually advanced past the snapshot. A pane claiming to be idle while the count
says otherwise is treated as a pane that is wrong, not as a completed compaction.

Four supporting changes make that gate safe to run unattended:

- The gated wait paces itself in real time and gives up with a distinguishable error, so a genuinely
  stuck compaction fails cleanly instead of spinning or silently letting the finish-up through. The
  error travels out through the recovery routine's existing error return and is absorbed one level
  up rather than ending the iteration on the first occurrence.
- After two *consecutive* gated give-ups on one iteration — the count resets on any successful
  recovery — gx stops retrying and escalates to `needs-attention` rather than looping forever or
  falling back to the behavior that caused the bug.
- The snapshot read is treated as three distinct states, not two. Only an agent with no boundary
  signal at all disables the gate. A snapshot that is merely unreadable right now — an errored read,
  or a transcript that does not exist yet — holds the gate closed instead, and a failed initial read
  is retried once. Collapsing that middle state into the first is what would quietly restore the
  original bug the moment a read blips.
- The breach test treats occupancy as unusable while the newest compaction boundary sits later in
  the transcript than the newest assistant turn carrying usage, so a successful compaction is not
  immediately followed by a spurious re-breach. Recency is decided by position in the append-only
  file rather than by timestamps, which are not reliably distinct. Answering that ordering question
  needs a new reader: the two that exist today each return a bare number and neither can say which
  line came last. The new reader reports the occupancy *and* the staleness rather than withholding
  the number, so the occupancy still reaches the display on every tick and only the breach decision
  is suppressed. Nothing changes for the numbers recorded on landed tickets or shown by `gx agent`.

Agents with no boundary signal at all — Codex — are untouched throughout.

## User Stories

1. As an operator watching an iteration compact, I want the compaction to run to completion, so that
   the context is actually reduced instead of being cancelled seconds later.
2. As an operator, I want the finish-up prompt to arrive only after compaction finishes, so that it
   is read as an instruction rather than swallowed as input that aborts the compaction.
3. As an operator, I want a pane that wrongly reports itself idle to be disbelieved, so that a
   herdr-level observation gap cannot destroy agent work.
4. As an operator, I want a slow compaction on a very large context to be waited out, so that
   "slow" is never mistaken for "finished".
5. As an operator, I want a genuinely stuck compaction to fail after a bounded wait, so that one bad
   iteration cannot hang indefinitely.
6. As an operator, I want a stuck compaction's failure to leave the pane alone rather than firing the
   finish-up prompt anyway, so that the give-up path cannot reintroduce the original bug.
7. As an operator running unattended overnight, I want repeated gated failures on one iteration to
   escalate to `needs-attention`, so that I find a flagged ticket in the morning instead of an agent
   that has been interrupting itself every ten minutes.
8. As an operator, I want the escalation to happen after a small number of attempts, so that one
   transient failure is retried but a real problem surfaces quickly.
9. As an operator reading `run-log.jsonl` during an investigation, I want a distinct event when the
   gate holds a prematurely-idle pane, so that I can see the gate working rather than inferring it.
10. As an operator, I want the existing "wait expired" event to keep meaning only that the pane wait
    genuinely timed out, so that the two different causes stay filterable by event type.
11. As an operator, I want a successful compaction not to be followed by an immediate second breach,
    so that gx does not interrupt the finish-up work it just delivered.
12. As an operator, I want occupancy reported as stale after a compaction, so that no *decision* is
    made on a number gx already knows is wrong while the display still has something to show.
13. As an operator, I want `gx agent` to keep showing occupancy as it does today, so that fixing the
    scheduler's reading does not blank out an unrelated display.
14. As an operator, I want a landed ticket to keep recording its actual context window, so that the
    staleness fix does not silently drop metrics for iterations that compact near the end.
15. As a Codex user, I want smart-zone recovery to behave exactly as it does today, so that a fix
    aimed at Claude's transcript signal does not change an agent that has no such signal.
16. As a developer debugging a future regression, I want the gated wait's give-up to be identifiable
    as its own failure mode, so that it is not confused with a transport error or a prompt failure.
17. As a developer, I want the per-iteration failure counter to count only that give-up, so that
    unrelated failures cannot trigger a spurious escalation.
18. As a developer, I want the counter to live with the iteration rather than inside the recovery
    routine, so that the Codex context-exhaustion caller does not share it.
19. As a developer, I want a snapshot that could not be read to still gate on a real boundary signal,
    so that one unlucky read does not disable the gate for an entire recovery.
20. As a developer, I want that fallback to ask whether a boundary landed *after* recovery began
    rather than to adopt a late count, so that a snapshot taken after the boundary already landed can
    never deadlock the gate against a compaction that really succeeded.
21. As a developer, I want transcript read errors during the wait to hold the gate closed, so that a
    persistent read problem surfaces as an ordinary give-up rather than as a resurrected bug.
22. As a developer, I want the gated wait to sleep in real time rather than spin, so that the
    ten-minute outer bound means ten minutes and not twenty instantaneous iterations.
23. As a developer, I want a test that would fail if that sleep were miswritten, so that a
    stubbed-out sleep in the test harness cannot hide a unit-conversion bug.
24. As a developer, I want the test double to be able to model a pane that lies about being idle, so
    that the production suite can express the failure that actually happened in production.
25. As a developer, I want that mode to be opt-in, so that every existing scenario keeps its current
    behavior.
26. As a developer, I want the test double to stop pairing a low-occupancy assistant line with every
    boundary, so that the fake stops masking the stale-occupancy problem real Claude Code exhibits.
27. As a developer, I want the scenario that depends on that paired line to opt back into it, so that
    changing the default does not break an unrelated scheduling assertion.
28. As a developer, I want a production-level regression test that fails if the gate is reverted, so
    that this bug cannot come back unnoticed.
29. As a developer reading the recovery code, I want the branch that is now Codex-only to say so, so
    that a future reader does not treat unreachable code as a live path.
30. As a developer, I want the one production test whose fake models the buggy shape to be named and
    repaired deliberately, so that nobody weakens the gate to make a red test go green.
31. As a developer, I want the alternative of making that test's transcript unreadable to be
    explicitly forbidden with its reason, so that the cheapest-looking repair does not silently
    delete the scenario.
32. As a reviewer at the end of the epic, I want the cross-ticket invariants written down, so that
    behavior split across five sessions can be checked as one whole.

## Implementation Decisions

**The gate.** The compaction-boundary count becomes an authoritative gate rather than a slow-pane
fallback. Whenever a snapshot is available, a pane-reported completion whose count has not advanced
past the snapshot is treated as another poll tick, not as completion. The pane wait remains the
pacing mechanism and the existing extended timeout remains the give-up bound.

**Three snapshot states, one of which fails open.** The boundary read has three outcomes and they
are kept distinct rather than collapsed into a have-it/don't-have-it boolean. *Unsupported* — the
agent has no boundary signal at all, which today means Codex — is the only state that fails open.
*Confirmed snapshot* engages the gate. *Temporarily unavailable* — a Claude session whose transcript
errored, or does not exist yet — holds the gate closed. The distinction matters because the
underlying dependency reports "unsupported" and "not there yet" the same way, as an unavailable read
with no error, and treating the second as the first silently reverts to the original bug whenever a
read blips.

Four conditions currently collapse into that one unavailable-with-no-error return, and each is
assigned a state explicitly rather than left to inference. A Codex agent and a missing read
dependency are *unsupported* — no waiting produces a signal that the build or the agent does not
have. An unidentified session and a transcript that is not there yet are *unavailable* — transient
startup conditions on an agent that does have the signal. The unidentified session is the one most
likely to be mis-bucketed, and mis-bucketing it fails a Claude agent open.

**Codex is untouched.** The snapshot is unsupported for Codex by construction, so the gate never
engages there and that path behaves exactly as today. A consequence: the recovery routine's
`blocked`-status branch, which exists for Codex's compaction-confirmation state, becomes reachable
only on the unsupported path. It is documented in a comment, not deleted and not guarded — it is
unreachable in practice for Claude, not impossible by construction.

**Pacing.** On the gated path the pane wait returns immediately — that is the premise of the bug —
so it contributes no pacing. Each gated tick sleeps explicitly for one poll interval and advances the
same elapsed counter the timeout path advances. That sleep belongs to the gate-held branch alone: a
pane wait that times out has already spent a real poll interval inside the wait, and sleeping there
as well would double-pace it and turn the ten-minute bound into twenty. Since both branches advance
the counter identically, unifying them into one shared sleep is the natural-looking mistake. The
millisecond conversion on that sleep is
load-bearing: the poll interval is a bare integer-millisecond constant, and omitting the conversion
turns a thirty-second tick into thirty microseconds, collapsing the ten-minute bound into roughly
twenty instant iterations. Test dependencies stub sleeping to a no-op, so this is asserted directly.

**Give-up.** At the extended bound the gate returns a synthesized non-nil error wrapping a new
package-level sentinel, so recovery takes its existing failure branch and the finish-up prompt is
never sent. Returning the pane's own nil error there would let recovery walk straight into the
finish-up prompt and reintroduce the bug ten minutes later. The recovery routine surfaces that error
to its caller through its existing error return rather than collapsing it into its boolean: every
failure branch currently produces the same observable, and the attempt bound must count only this
one. That return is fatal-propagating today, so the poll loop's call site is changed to absorb this
one sentinel and continue polling while every other error propagates as before — without that, the
first give-up would end the iteration and the attempt bound below would be unreachable.

**Attempt bound.** The iteration-level poll loop counts *consecutive* gated give-ups, resetting to
zero on any successful recovery. Without the reset the counter is a lifetime tally and two unrelated
give-ups hours apart in a long iteration, each already recovered from, would escalate a healthy run.
Recovery failures that are not the gated give-up neither increment nor reset it — they say nothing
about whether compaction is progressing. The reset is keyed on the recovery routine's success
boolean, never on the absence of an error: its ordinary failure branch reports through the sink and
the run-log and returns no error at all, so "no error" is true for exactly the failures that must
not reset. The poll loop currently discards that boolean and has to stop doing so. After two consecutive give-ups the loop stops retrying and
returns an error that routes the ticket to `needs-attention`, following the same path the Codex
context-exhaustion failure already uses; the marking itself happens in the run's per-result handling,
not in the poll loop, which is why the persisted status is asserted at a different seam from the
returned error. There is deliberately no fallback to sending the finish-up prompt after the bound — that would reintroduce the cancellation this epic exists to prevent, merely
rate-limited. The counter is per-iteration and lives in the poll loop, not in the recovery routine,
which is also called from the Codex exhaustion path and must not share it. "Stop touching the pane"
means not entering a further breach cycle; it cannot retract the interrupt that already preceded the
second attempt.

**Snapshot robustness.** The snapshot is sticky: taken once at the start of recovery and never
replaced. Retrying the read on a later tick and adopting the result would deadlock rather than help
— the gate's predicate is "count exceeds snapshot", so a snapshot adopted after the boundary already
landed is permanently at or above the count, the gate can never open, a successful compaction reports
as a give-up, and the attempt bound escalates a healthy iteration to `needs-attention`.

An unavailable snapshot therefore does not fall back to a count comparison at all; it **switches
predicate**. Alongside the snapshot, recovery records the instant it began, and the gate asks a
different question for the rest of that recovery: has any compaction boundary landed *since* that
instant? A boundary in the transcript that postdates the moment `/compact` was submitted is proof of
this compaction regardless of what the pre-existing count was, so the deadlock cannot arise. This
needs a second reader — the count-since-an-instant question is not answerable from the scalar count —
and it is the one place in this epic that compares timestamps (see below). Unavailable reads *inside*
the loop, under either predicate, hold the gate closed and count toward the bound rather than being
treated as "no snapshot, trust the pane".

**The finish poll is gated too, not just the compaction wait.** A gated give-up leaves the poll loop
holding the snapshot it gave up on. While that snapshot is unresolved, a pane that subsequently
reports the *iteration* finished is not trusted either: the loop paces one poll interval and
re-checks the boundary before accepting the finish. The premature-idle pane that motivates this epic
lies about compaction being done; there is no reason to believe the same pane about the work being
done moments later, and accepting it would land the same mid-compaction cancellation one step further
along. A second silent re-check counts as another give-up, so a pane stuck in that state escalates
rather than reporting a phantom success.

**Event labeling.** The completion signal becomes tri-state: pane-confirmed, gated-then-confirmed,
and timeout-then-confirmed. The gated route logs a new event; the existing wait-expired event is
reserved for the genuine timeout route, which an existing production test asserts on. `run-log.jsonl`
is the primary diagnostic instrument for this class of bug — it is how this one was found — so a
label describing the opposite of what happened is a real cost.

**Stale occupancy.** The breach test treats occupancy as unusable while the newest compaction
boundary is more recent than the newest assistant turn carrying usage. The breach check already bails
when occupancy is unavailable, so declining to breach is exactly the right answer: no breach fires
until the agent produces a real post-compaction number.

Recency *between two transcript lines* is decided by **file order**, never by timestamps. The
transcript is append-only, so the later line is the later event, while a boundary and the turn beside
it can carry byte-identical timestamps — the compaction fake writes exactly that pair. The rule is:
scanning from the end, if a boundary is met before any assistant-usage line, the reading is stale.

The one deliberate exception is the unavailable-snapshot predicate above, which orders a transcript
line against a **wall-clock instant gx itself recorded**, not against another line. File position
cannot answer that, so the comparison is unavoidable there; it is also safe from the identical-
timestamp hazard, because the two things compared never come from the same writer. It does inherit
two edges the file-order rule does not have: a boundary line whose timestamp is missing or unparsable
never counts, and the comparison is strict, so a boundary stamped in the same instant as the recorded
start is excluded. Both fail closed — the gate holds and the recovery gives up — which is the safe
direction, but they are the reason this exception is confined to that one predicate.

This requires a **new dependency**, not a filter over the existing ones. The two readers available
today are scalars — one returns the last occupancy, the other a count of boundaries — and neither
carries position, so "which of these is newer" cannot be composed from them. A new reader answers
the ordering question directly in a single tail scan.

**It reports staleness alongside the occupancy rather than in place of it**, because the call site
it serves has two consumers, not one. The smart-zone poll loop reads occupancy once per tick and
uses that one value twice: to decide whether a breach fired, and to feed the per-tick emission that
drives `gx agent` and the TUI. A reader that answered a bare "unknown" would take the emission down
with the breach check and stall the display until the agent next spoke — collateral nobody asked
for. Returning both values keeps the two consumers separable at a single read: the emission uses the
occupancy whenever one was found, exactly as today, and only the breach test additionally requires
the not-stale flag.

That reader is consumed **only** by the smart-zone poll loop. It is deliberately not folded into
ralphloop's general occupancy wrapper, whose other call sites do not want staleness at all:
done-metadata stamping would lose a landed ticket's recorded context window whenever an iteration
compacts near the end, and the occupancy emitted at iteration start or reattach would blank
`gx agent`'s display after every compaction. The shared transcript reader keeps its semantics as
well: it is refactored onto the tail-scan helper the new readers share, behavior-preservingly and
under its existing tests, but no caller of it observes a change.

**Test double.** The Claude compaction fake gains two opt-in flags on one options surface: a
premature-idle pane mode, and control over the low-occupancy assistant line it currently pairs with
every boundary. The paired line becomes opt-in with boundary-only as the new default, because real
Claude Code writes no such line and the pairing is precisely why no existing test catches the
stale-occupancy problem. The existing scheduling scenario opts back in so its token assertion is
unaffected. The fake has exactly two construction sites and both change. The unit tests share one
constructor helper, so the flags are threaded through that helper's signature rather than set on an
individual test — setting them inside the helper would opt every unit test back in and undo the
change. The test that asserts the paired line opts in explicitly and gains a sibling asserting the
new boundary-only default. The write order inside paired mode does not change — boundary first,
assistant line second — because that models a compaction that finished and an agent that then spoke,
which is the real post-compaction sequence, and it is what keeps the file-order freshness rule from
suppressing the scheduling scenario's occupancy read. In premature-idle mode the scenario's poll handler advances virtual time one poll
interval per dispatch rather than jumping the clock at submission — the fake writes its boundary
lazily against a virtual deadline and the harness clock never self-advances, so without a per-tick
advance the boundary never lands and the test reaches the give-up path instead of the green path.

**Sequencing.** `01` (gate, pacing, sentinel, event, and the one production-test repair) comes
first and blocks `01a` (snapshot robustness), `01b` (attempt bound), and `02` (fake modes and the
production-level regression test). Those three all edit the same recovery routine, so they are
additionally chained `01a` → `01b` → `02` to keep cherry-picks from colliding. `03` (stale occupancy)
is genuinely independent and blocked by nothing — it touches a different reader and can run in
parallel from the start. `05` (code review) is blocked by every implementation ticket, `03`
included; without those edges the scheduler's frontier would offer it on the first pass and it would
review nothing. The attempt bound sits immediately after the gate rather than at the end because the
gate alone ships an unbounded retry cycle.

## Testing Decisions

A good test here asserts what an operator or the agent would observe — was a finish-up prompt sent,
how many interrupts happened, which event landed in the run-log, did the ticket end in
`needs-attention` — and not how the recovery routine is structured internally. The bug being fixed
was invisible to a green suite precisely because the existing fakes modeled an honest pane; tests
that encode the implementation's assumptions rather than the agent's real behavior are what let it
through.

Five seams. Four are existing; the fifth is a new reader introduced by the staleness rule and tested
at its own level.

**The recovery routine driven through dependency fakes** is the primary seam and carries most of the
work: the gate holding against a prematurely-idle pane, the finish-up prompt appearing only after the
count advances, the sleep-per-tick pacing, the give-up error satisfying an `errors.Is` check against
the sentinel with zero finish-up prompts sent, the no-snapshot path behaving as before, the snapshot
retry firing once and only once, read errors holding the gate closed, and the two events going to the
two right routes. Prior art: the existing `TestRecoverSmartZoneBreach_*` unit tests, which already
establish the fake shape and the transcript-count stub.

**The iteration poll loop through the same fakes** is the same seam one altitude up, used where the
behavior spans multiple recovery attempts: two consecutive gated give-ups returning the escalation
error with no third `/compact`, a give-up followed by a success resetting the counter so a later
give-up does not escalate, non-gated failures neither incrementing nor resetting it, the sentinel
being absorbed rather than propagated on a first give-up, and no second breach on the tick after a
successful compaction. Prior art: the `TestWaitForFinish_*` family.

**The run's per-result handling** is where the ticket is actually written to `needs-attention`, so
that one assertion cannot be made at the poll-loop seam and is made here instead — the poll loop only
returns the error. This split is the reason the attempt bound spans two seams rather than one.

**The herdrfake production scenario** exercises the whole path through real dependency wiring. Its
value is that the fake enforces the protocol itself: it rejects a finish-up prompt submitted before
the boundary exists, and rejects interrupt keys sent during an active compaction. Those guards are
the assertions — a regression trips them without anyone writing a bespoke check. This seam owns the
premature-idle regression test, which must be verified to fail against a locally reverted gate. Prior
art: the two existing production regression tests, one of which is repaired rather than replaced
because it pins an unrelated stray-Enter regression.

**The new smart-zone occupancy reader** is the narrowest seam and is new because the ordering
question cannot be answered by any reader that exists today: a transcript whose newest line is a
boundary reports its occupancy as stale; a boundary older than the last assistant turn does not; and
two lines carrying identical timestamps are ordered by file position. It also carries the negative
assertions that the rule stayed scoped — done-metadata stamping and the iteration-started occupancy
emission still report a value for a transcript whose newest line is a boundary, and the poll loop's
own per-tick emission keeps firing during the window while only the breach decision is suppressed.

## Out of Scope

- Any change to how `/compact` itself is submitted, or to the interrupt that precedes it.
- Codex's compaction and context-exhaustion handling.
- The shared transcript occupancy reader's semantics, and the `gx agent` display and done-metadata
  stamping that depend on them.
- Auto-compaction triggered by Claude Code itself rather than by gx.
- The unlocked-shared-file race between the scheduler's ticket writes and an agent's own writes,
  which is a separate known issue.
- Any TUI change to show compaction state; the run-log event is the operator-visible surface here.

## Further Notes

The diagnosis is recorded in full, with the run-log and transcript evidence, in the
`lifecycle-refactor` epic's research ticket on this incident.

Two premises could not be verified from this repo and are carried as known uncertainty. First,
whether herdr's wait returns immediately when the pane already matches the requested state: its help
text documents only the flags, and its source is not in this repo, so the pacing analysis rests on
the incident timing and on how the fakes behave. Second, whether the repaired production test is
green on the first run — that conclusion is static reasoning about ordering and occupancy, not an
executed suite.

The plan was reviewed by three read-only consultations before publication. The reviews changed it
substantially: the original two-ticket plan would have shipped a gate that spun without pacing and
returned a nil error at its bound, letting the finish-up through anyway — the bug surviving its own
fix. They also surfaced the stale-occupancy half of the incident, which no ticket had covered, and
caught that the fake's paired low-occupancy line was the reason no test could see it.

A fourth review, of this spec, produced six further corrections, all folded in above. Two were
design gaps rather than wording. The staleness rule had been scoped one level too high — to
ralphloop's general occupancy wrapper, which also feeds done-metadata stamping and the
iteration-started emission the spec promised to leave alone — and, more seriously, could not be
implemented at all over the existing scalar readers, which carry no ordering information; hence the
new dependency and the raised estimate on `03`. The snapshot-failure policy was contradictory
between `01` and `01a`, which the three-state model now resolves. The rest were corrections of fact:
the ticket sequence the spec described was not encoded in the blocking edges and left the code
review claimable on the first scheduler pass; "consecutive" give-ups had no reset rule; the fake's
paired-line change could not leave literally every existing test untouched, and `03` had the fake's
two write lines in the wrong order, predicting test fallout that does not exist; and the recovery
routine already returns an error, so the sentinel needs a call-site change rather than a new return
value.

A fifth review confirmed the diagnosis and the shape of the plan against the code, and found six
further gaps, also folded in. The largest was that the breach-check occupancy read has two consumers
sharing one call — the breach decision and the per-tick display feed — so a reader returning a bare
"unknown" would have stalled the display as a side effect; the reader now returns occupancy and
staleness together. The second was that the give-up counter's reset could not key on the absence of
an error, since the ordinary failure branch returns none. The rest: the gated sleep must not also
fire on the pane-timeout branch; the four conditions that collapse into a single unavailable read
each need an explicit state, an unidentified session most of all; both of the fake's construction
sites change rather than one, through a shared test helper; and the staleness rule must be verified
against both production regression tests, whose transcripts also end on a boundary, not only against
the scheduling scenario.
