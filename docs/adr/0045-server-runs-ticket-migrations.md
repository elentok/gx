# The server runs ticket migrations itself, from a per-project applied list

The `gx server` process applies pending ticket-store migrations at every start, before it schedules
work. Each migration step has a unique name, and each project keeps a plain-text list of the names it
has applied in the git-backed store. The server runs every step in the binary that is not on the
list.

Why: `launchd` starts the server without running any `start` or `restart` command code, so only the
server process sees every start path (login, crash restart, CLI and web restart). A global version
number or a single "last migrated" marker misses a step when two epics that change the schema land
in either order. A name list per project has no such gap and shows in git history.

A failed step gives that project a Migration block, not a pause (ADR
[0042](0042-few-pauses-all-recoverable-from-the-web.md)). gx cannot recover alone: the same step on
the same data fails the same way, and a repair may need a person's judgement. The web buttons
"Retry migration" and "Fix agent" end the block.

## Consequences

- Adding a step means adding a new named entry to the one registry. A released name is never
  renamed or reused.
- The pause migration, `Migrate` and the question-section step are one registry entry, run in that
  order (plan pause, `Migrate`, question sections, apply pause). A retry reruns the whole unit.
- The project's migration lock is shared/exclusive. Ticket file updates, ticket-log appends and
  new-ticket creation take it shared and wait while that project migrates. A migration takes it
  exclusive and writes through an inner path that does not take it. Lock order: migration lock,
  then `LockTicket`.
- A waiting write has no time limit. After 1 s the CLI prints "waiting for the migration of
  <project>".
- The Migration block gate sits beside `trunkProblem` in `server/missingtrunk.go`.
