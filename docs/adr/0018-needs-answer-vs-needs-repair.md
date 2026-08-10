# The two human-clearable stall statuses split on asked-vs-told, as `needs-answer` and `needs-repair`

`needs-info` and `needs-attention` kept getting confused, because three different sources claimed
three different axes and none of them was the one the code implemented. The tracker contract said
information vs judgment. `CONTEXT.md` said the *work* is stalled and an agent wrote it vs the
*machinery* is stalled and the orchestrator wrote it. The code said neither: `needs-info` had exactly
one automatic writer, the zero-commit finish in `finishIteration`, which means "the agent landed
nothing" and not "a human has a fact I need" — while `markDoneTicketUnrecoverable` wrote
`needs-attention` for a ticket with no live iteration at all, so "machinery stalled mid-flight" didn't
hold either.

We decided the split is real but the labels were wrong, and re-cut it on a new axis: **is a person
being asked for something, or told about something?** `needs-info` becomes **`needs-answer`** — the
ticket cannot proceed until a person supplies an answer or a decision, and nothing is broken.
`needs-attention` becomes **`needs-repair`** — gx hit a fault it cannot resolve on its own and a
person must investigate. Both were renamed rather than just the vague one, because leaving
"attention" in place would have preserved the confusion at half the migration cost. `repair` was
picked because it already carries this meaning here: `repairRecoverableTicket`,
`doneRecoverable`/`doneUnrecoverable`, and `markDoneTicketUnrecoverable` is literally "auto-repair
can't, a human must".

The split survived because it was never decorative: the fault side recorded a reason in the ticket
body, was re-announced at restart, was checked for reattach eligibility, and could self-heal in-run
via `waitForAttentionRecovery`; the ask side did none of these. What was identical was queue
treatment — both are `isHumanClearable`, neither ever enters the frontier, both park the epic — and
that is deliberately unchanged.

## Considered options

- **Collapse to one status carrying a reason.** Rejected: one status forces a single notification
  policy on both cases, so either every quiet ask joins the fault side's alarm behaviour or real
  faults lose their alarm.
- **Three statuses**, separating "the agent produced nothing" from "the agent is asking". Rejected as
  re-multiplying exactly the pick-rule overlap this decision exists to remove — the no-result case is
  a fault, not a third kind of stall.
- **Rename `needs-info` only**, keeping `needs-attention`. Genuinely cheaper: the fault side has four
  automatic writers to the ask side's one. Rejected on the grounds above.

## Consequences

- **The zero-commit finish moves to `needs-repair`, and gains a reason.** Under the new axis nobody
  was asked anything — an iteration that lands no commits is an unexplained no-result. This also fixes
  a real asymmetry: `MarkNeedsInfo` took no reason while `MarkNeedsAttentionWithReason` did, so the
  status that fired when an agent gave up was the one that could not say why.
- **`needs-answer` requires a reason**, appended to the ticket body under a `## Needs Answer` heading,
  mirroring the fault side's `## Needs Attention`. The orchestrator gate for interactive-prompt stalls
  writes this status with no agent prose behind it, so the reason has to land in the file. A status
  that cannot be written without saying why is hard to misuse.
- **`needs-repair` is never agent-authored.** It means gx's own machinery failed, so an agent
  announcing that it is about to block on a question writes `needs-answer` — which is what the
  `gx-implement` pre-question rule will say.
- **The pick rule is one sentence with no overlap**: ask ⇒ `needs-answer`, fault ⇒ `needs-repair`.
  Every existing writer lands unambiguously on one side, including `markDoneTicketUnrecoverable` (a
  fault, no iteration) and the interactive-prompt gate (an ask, live pane), with no special cases.
- **ADR 0017's last consequence bullet is superseded.** It recorded that the two statuses "stay
  distinct" on the work-vs-machinery axis, with `needs-info` as "the agent asking a question about the
  work". That axis is what this ADR replaces; 0017's substance about parking, reattach-decides-resume,
  and the concurrency permit is untouched.
- **Migration is a rename across frontmatter, code, and docs.** `lifecycle-contract` retired three
  statuses shortly before this, so there is a worked example to copy rather than a shape to invent.
  `CONTEXT.md`'s glossary entries for both statuses are wrong until it lands.
