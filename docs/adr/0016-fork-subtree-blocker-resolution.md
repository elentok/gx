# Fork children carry no `blocked_by`; blockers resolve over the fork subtree

A ticket forked mid-flight used to be modeled by marking the original `done` (with `commitless:
true`) and giving each fork child `blocked_by: [<original>]`. Both halves are lies the resolver then
had to unwind: the original's `done` doesn't mean its work is finished, so `blocked_by: 04` couldn't
trust `04`'s status and had to walk `04`'s whole subtree — but that walk then reached the very
children that declared `blocked_by: 04`, so each child deadlocked against itself. `tickets/status.go`
grew `isSelfOrDescendant`, `isForkSibling`, `isDescendantOf`, an `exclude` hook and a `visiting`
cycle guard purely to carve those self-references back out, and every one of the eight bugs in
`skills/gx-investigate/gotchas.md` is a mis-tuning of that carve-out — including one where the
sibling exclusion silently swallowed a *legitimate* `02c blocked_by: 02b` dependency.

We decided a fork's only structural edge is `parent`. A fork child's `blocked_by` stays empty; it
inherits its parent's position in the dependency graph by being its child, not by copying its
parent's tokens. `blocked_by: X` then resolves to a single question — *is X still blocking?* — where
X is blocking until its own work is `done` and every ticket in its fork subtree is likewise no
longer blocking. That is a plain post-order walk over `parent` reverse-edges with no exclusions and
no cycle guard, because `parent` edges are acyclic by construction — a fork child's ID is always
allocated after its parent's — and that construction is enforced rather than assumed. Five of the
eight documented bugs become unrepresentable rather than fixed.

Acyclicity is an invariant of an `Epic`, not of a ticket: the check is "`parent` names an existing
ticket in this epic that is not in my own fork subtree", which no single-ticket validation can see.
It is enforced at `Epic` construction, which fails rather than exposing a cyclic or dangling graph
to the unguarded recursion, and again on `gx tickets set --parent`, which re-loads and validates
under the epic's existing allocation lock so that two concurrent re-parents cannot each validate
against a stale snapshot and jointly close a cycle.

## Consequences

- The `children` frontmatter field is removed. It was authoritative for nothing and its
  best-effort backfill was the direct cause of gotcha #1; children are now derived from `parent`.
- A fork parent whose subtree is unfinished renders as **waiting for children** — a computed status,
  never written to disk. Previously it rendered `Done` and counted as settled, so an epic could
  report complete with unfinished fork work in it.
- Forks may fan out to several children in parallel. The earlier "strictly linear forks" proposal
  was load-bearing only under the old model, where multiple siblings meant multiple mutually
  blocking inherited tokens.
- `type: code-review` loses its bespoke eligibility rule (`hasOtherOpenTicket`): its `blocked_by` is
  expanded at load to every other non-code-review ticket, recomputed on every load. A ticket that
  had already forked when the review became eligible is covered in full, because each expanded token
  resolves over that ticket's fork subtree. A ticket added or forked *after* the review was claimed
  is not covered: `blocked_by` gates only entry to the frontier, and a claimed ticket is never
  re-checked against it. Freezing the expansion at claim was considered and rejected as a near-no-op
  that implied a guarantee the mechanism cannot make.
- Existing epics are migrated once by `gx tickets migrate`. The loader does not keep understanding
  the old shape — read-side compatibility would mean retaining exactly the resolver complexity this
  decision removes. Migration validates the whole result, parent graphs included, before writing
  anything, so it cannot leave a tracker the stricter loader will reject.
