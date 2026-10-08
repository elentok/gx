# Recovery repairs the orchestration, never the work, and high authority never auto-applies

When a ticket parks, the server's recovery tries to get it moving again without a person. Recovery
may act in two tiers: nudge a live pane, and call the server's own verbs (park, relaunch,
commitless-done, release-gate, set-parent, …). It may never write code or commit. Those changes
would land with no review, cherry-pick check or code-review gate around them. An investigate ticket
is commitless by type with no target, so this holds by construction, not by prompt.

Each catalog entry carries an authority level, and the level decides who acts:

| Authority | Who acts |
| --------- | -------- |
| `low`, `medium` | applied unattended, by a recovery rule or an investigate ticket |
| `high` | nobody unattended: recovery writes a `## Proposed Remedy` and escalates; a person runs it with `gx server tickets approve` (key `A`) |

An entry with `executor: person` only escalates, whatever its authority. An entry with `executor:
recognize` is only recorded as matched: the park and its message stand, and it never counts as
recovered. A rule remedy that ends ok but leaves the ticket parked releases the park message too.
`high` is for remedies that
can lose work or hide a real failure, such as marking a ticket commitless-done or clearing it back
to `open`.

## Considered Options

- **One authority switch for the whole catalog** — either too timid for the safe remedies (relaunch,
  close a pane) or too bold for the ones that can lose work. A per-entry level lets the safe ones
  run unattended from day one.
- **Let recovery commit fixes** — the agent that diagnoses a failure would also write the fix, and
  that fix would land unreviewed. A wrong but confident story nobody reads is the failure mode this
  avoids. Code fixes go out as draft follow-up tickets instead.
- **A private write path for recovery** — rules call the same verbs as a person, with the same
  refusals, land lock and events, stamped `actor: recovery`. A second path would drift from the
  first.
- **Auto-apply `high` after a delay** — a silent default still acts with no one looking. One key
  press to approve is cheap.

## Consequences

- A `high` proposal waits until a person approves it. Approval refuses `proposal-stale` when the
  ticket changed after the proposal was written.
- Recovery cannot fix a bug in the work itself. Such a failure escalates with a report, and the
  report becomes a catalog entry or an orchestrator fix.
- Raising an entry's authority is a code change to the catalog, reviewed like any other.
