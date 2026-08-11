# The ticket's status splits into a gx-owned `status` and an agent-owned `iteration_status`

ADR [0018](0018-needs-answer-vs-needs-repair.md) re-cut the two human-clearable statuses; this one
decides which field holds what, and who may write it.

Ticket 03 of the `stall-visibility` map established *that* the single `status` field splits in two:
`status` becomes gx-owned and is written only after the cherry-pick lands, so `done` ⇒ landed by
construction, closing the done-before-landed race; the second field gives gx an on-disk "the agent is
finished / stuck" signal instead of inferring one from the herdr pane. It deliberately did not decide
what the two fields contain.

## Decision

**`status` is gx-owned and remains the sole scheduling authority.** Its value set is unchanged from
the six today, with 0018's rename applied: `draft`, `open`, `claimed`, `needs-answer`, `needs-repair`,
`done`. Every scheduling predicate — `ralphloop.Frontier`, `Epic.RenderedStatus`, `Epic.Blocking`,
`isHumanClearable` — keeps its single-enum branch and reads `status` alone. `Blocking` in particular
stays exactly as ticket 03 froze it, including the implicit non-recursive `parent` edge on the
parent's own `status: done`.

**`iteration_status` is written by the agent and reports on itself**: `working`, `needs-answer`,
`finished`, or **absent**. Absent is a meaningful value — no agent has spoken for this ticket in this
claim — and gx clears the field on every claim and every reattach, so a report is never readable
outside the claim that produced it.

**gx adopts the report, rather than reading two fields.** When gx sees
`iteration_status: needs-answer`, gx writes `status: needs-answer`. When it sees `finished`, it
enters its landing path and — only if that path succeeds — writes `status: done`. Nothing in the
scheduler ever branches on both fields at once.

The verb is **adopt**, not "promote". Ticket 04 of the `stall-visibility` map found "promote"
already carrying two unrelated meanings in this repo — `draft` → `open` (`CONTEXT.md`, and the
tracker contract's "Promoting it is a deliberate, separate step") and a queued epic entering
`MaxConcurrentEpics`'s auto-promotion queue — so this ADR's sense was renamed rather than becoming a
third. "Adopt" also carries the right connotation that gx *chooses* whether to: the `finished` case
can decline.

The field was very nearly named `agent_status`, which is how ticket 03 refers to it. That name is
already taken: herdr's pane payload serializes a field as `agent_status` (`herdr/agent.go`) carrying
`idle`/`done`/`blocked`, read throughout `ralphloop/waitforfinish.go` — the very pane signal this work
exists to stop trusting. `iteration_status` names what the field describes (the state of the current
iteration) rather than who wrote it, and cannot be confused with herdr's field in the one file that
holds both meanings.

### The invariant on `finished`

**An agent's report can start a landing, never conclude one.** `iteration_status: finished` is a
second way into `finishIteration`, alongside the pane going idle — that is the part that closes the
observability gap. Everything after the wake-up is unchanged: the commits-ahead count decides
landed-vs-nothing, `landCherryPick` must succeed, and gx writes `status: done` itself, afterwards. An
agent that reports `finished` with broken, unpushed, or nonexistent commits takes the ordinary
zero-commit fault path.

### Who writes the human-clearable states

Ticket 09 asked whether 0018's ask/fault split maps onto agent-owned vs gx-owned. **It does not, and
does not need to.** `needs-answer` has two producers of different kinds: an agent announcing a
question, and gx's own interactive-prompt gate observing a blocked pane. Both land on
`status: needs-answer`, by different routes — the agent's route is a report gx adopts; the gate is
gx-side code writing `status` directly, with no round trip through an agent-owned field. The reasons
they record differ accordingly: the agent's own words, versus "gx concluded a human is needed".

## Considered options

- **A dual-field scheduling predicate**, with the human-clearable states living on `iteration_status`
  and `claimNext`/`Frontier`/`isHumanClearable` reading both. Rejected: the adoption lag it guards
  against does not exist — an agent only runs on a `claimed` ticket, and `claimed` is already
  unschedulable, so no window makes a stalled ticket claimable. It would teach a second field to every
  one of the ~78 sites the migration touches, for a race that cannot occur.
- **`iteration_status: finished` as authoritative for `done`.** Rejected: it is another agent
  self-report, and `compact-issue` landed the opposite lesson — a Claude pane's self-report of idle was
  believed and was wrong. The commit count is ground truth git already holds.
- **Folding `commitless` into an `iteration_status: finished-no-commits` value.** Rejected:
  `IsCommitless()` is type-derived for `research`/`grilling`/`code-review` tickets that never write any
  `iteration_status`, so folding would leave two disagreeing sources for one fact — and the field is
  read by the UI and `scripts/ralph-stats.mjs`. It stays a separate bool, with gx rejecting a
  `finished` report that has zero commits and no `commitless` rather than silently reclassifying it.
- **Keeping the name `agent_status`** with a glossary note disambiguating it from herdr's field.
  Rejected: a glossary note is the weakest available fix placed exactly where the confusion is most
  expensive.

## Consequences

- **The CLI gains `--iteration-status`** (`working`/`needs-answer`/`finished`), which is an agent's
  only status verb. `--status` accepts `draft`, `open`, `needs-answer`, `needs-repair` — a human
  parking a ticket they know is broken is a real workflow and breaks no invariant — while `claimed`
  and `done` become loop-internal, reachable only with `--force`. This widens `--force`'s meaning: it
  currently only bypasses the unresolved-`blocked_by` check on `done` (`cmd/tickets_set.go`), and
  becomes "write a loop-internal value" as well. Retired spellings reject loudly, per ticket 07's
  clean-cut precedent.
- **0018's required reason becomes enforceable rather than advisory.** `gx tickets set
  --iteration-status needs-answer` fails unless the body has a `## Needs Answer` section, mirroring
  `checkBodyBeforeOpen`; and the adoption path refuses to adopt a `needs-answer` report with no such
  section, writing `status: needs-repair` instead — a question nobody can read is not answerable, which
  makes it a fault. Ticket 04 adds the converse guard: the `needs-answer` adoption path must **not**
  run zero-commit fault detection, which belongs to the landing path alone — a ticket that stops to
  ask a question fully expects to commit after it resumes, so zero commits there is legal and is not
  `commitless`.
- **Clearing is one write.** A person clears a `needs-answer`/`needs-repair` ticket by writing
  `status: open`; the stale report is removed by clear-on-claim, not by hand. No dedicated `clear`
  verb.
- **The Queue tab renders `iteration_status` as subtext on `claimed` rows only** — "implementing…",
  "landing…", and, for a claim with no report, "starting…". The icon and color stay a pure function of
  `RenderedStatus`, so no existing switch grows a case. A `claimed` row sitting at "no report yet" is
  the invisible stall made visible.
- **The migration writes nothing into existing ticket files.** `iteration_status` is add-only and
  absent everywhere until the first agent reports under the new model; the rename half still touches
  the single ticket file ticket 07 found.
- **0018 is untouched.** It decides the names and axis of the two human-clearable statuses; this ADR
  decides which field holds what and who may write it.
