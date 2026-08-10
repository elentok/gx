# lifecycle-contract

The contract half of the `lifecycle-refactor` expand–contract sequence. The full design, user
stories, and rationale live in that epic's spec — read `lifecycle-refactor.md` first; this
document only records why these two tickets are a separate epic.

`lifecycle-refactor` is deliberately backward-compatible throughout: it adds `draft`, migrates the
tracker data, swaps the resolver, and changes run-lifecycle behavior, but never rejects a ticket
that used to be accepted. That is what lets it run to completion against the `gx` binary that was
installed when the run started.

This epic removes the old shape: the `children` frontmatter field, the `needs-triage`,
`ready-for-agent`, and `ready-for-human` statuses, and the loader's acceptance of anything but the
post-migration shape.

**Run this epic only after `lifecycle-refactor` has landed on main and `gx` has been rebuilt and
reinstalled.** Ticket 01's whole purpose is that the loader now rejects what it used to accept, and
that cannot be meaningfully verified against a binary that doesn't yet reject anything. The
dependency on `lifecycle-refactor` is therefore run order, not a `blocked_by` edge — blocking edges
only resolve within an epic.

Out of scope here, as in the parent epic: the `agent_name_taken` write race, `gx tickets next`, a
headless loop CLI, and ticket renumbering.
