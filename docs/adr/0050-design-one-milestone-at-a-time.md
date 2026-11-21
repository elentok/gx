# Design one Milestone at a time

Only the next Milestone of an Initiative gets a full Design. Later Milestones stay a name, a
one-line goal and the assumptions they test. Items a Design defers live on the Initiative, not in
`follow-ups`, and so do the assumptions. A Design never edits them beyond adding deferred items.
Only the Gate, run by the person after the impl epic merges, changes assumption states, places
deferred items, reshapes the Milestone list or stops the Initiative. Reason: the first Milestone
gives hard evidence on whether the assumptions hold, so plans written up front for later Milestones
go stale, and big designs produce huge PRs that need many fixes.

## Considered options

- One Design for all Milestones. Rejected: it recreates the long Design and the huge PR.
- Deferred items in `follow-ups`. Rejected: `follow-ups` holds bugs and ideas found after something
  ships. A deferred item is planned scope with a destination Milestone.
