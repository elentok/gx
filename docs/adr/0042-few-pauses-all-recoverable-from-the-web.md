# Few pauses, and every pause is recoverable from the web UI

A pause stops work until a person acts, so gx pauses only when it cannot recover on its own. Every
pause, on a ticket or on an epic merge, offers at least one web button that ends it. When the cause
lies outside gx (a hand-made commit, a deleted trunk branch, a dead herdr), the card names the fix,
and a button finishes the job after it. No pause needs a gx CLI command.

"Every pause" means ticket pauses plus the `merge-error`, `dirty-trunk`, `trunk-missing` and
`needs-rebase` merge blocks. `blocked-cycle` and `parent-defect` are graph events, not pauses.

## Consequences

- Each pause kind has a row in the pause-to-verb table with at least one verb the web serves. A
  test fails when one is missing.
- A spec that adds a pause kind, or a new way into an existing one, says why gx cannot recover on
  its own and which web button ends the pause. The gx-design review raises a finding when it
  doesn't.
