# An Initiative is an epic kind, not a new entity

An Initiative is stored as an epic of kind `initiative` in the normal ticket store. gx never
schedules it and the Epics page skips it. Its Milestones are an ordered list in its body, not
tickets. Each Milestone has one Design (a map epic that names the Initiative and the Milestone in
its frontmatter) and one `-impl` epic. Reason: the store, addresses, CLI and migration gate already
work, so Milestone 1 fits in one impl epic. The web and CLI read Initiative-shaped data only, so a
separate entity stays possible later.

## Considered options

- A separate entity with its own store and addresses. Rejected for now: it doubles the plumbing
  before one Milestone has shipped.
- Milestones as child tickets. Rejected: a Milestone has no status of its own. Its state is derived
  from its Design and its impl epic.
