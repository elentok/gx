## Problem Statement

After a breaking change to the ticket schema, the person has to run `gx server restart --migrate` by hand. If they forget, or the server restarts by itself (login, crash restart, `gx server start`), the new binary runs on old tickets. The migration chain (`migrateTracker`, 13 steps) has no version and no record of past runs, so gx cannot tell which steps a store still needs. A running number or a single "last migrated" marker would also miss a step when two epics that change the schema land in either order.

## Solution

Every start of the gx server process finds the migration steps a project still needs and applies them before it schedules any work. There is no flag and no manual step.

- Each migration step has a fixed, unique name. Each project records the names it has applied. The server runs every step in the binary that is not on the project's list.
- The server listens first, then migrates all projects in the background. The handshake and the web snapshot report "migrating" until all migrations end. No work is scheduled until then.
- If a project's migration fails, the failure is saved and the server starts anyway. The project gets a **Migration block**: the server claims none of its tickets until the failure record clears, while other projects keep running. The web strip shows the failure with two buttons: "Retry migration" and "Fix agent".
- `gx server restart --migrate` is gone. `gx tickets migrate <path>` stays for retries and for rerunning steps by hand.
- A ticket write waits while its project migrates, so nobody writes into half-migrated tickets.
- The restart dialog dry-runs only the steps that are pending for the new binary.
- The decision is recorded in [ADR 0045](../adr/0045-server-runs-ticket-migrations.md).

## User Stories

1. As a gx user, I want the server to apply pending ticket-store migrations when it starts, so that I never run `gx server restart --migrate` by hand.
2. As a gx user, I want the same behaviour for every way the server starts (login, crash restart, `gx server start`, `gx server restart`, web restart), so that no start path leaves the store behind the binary.
3. As a gx user, I want no new ticket work to be scheduled until migration ends (queue tickets, recovery tickets and merges started by the scheduler), so that no new agent starts against tickets in the old shape. Agents that survive a restart keep working, and their writes wait (story 35).
4. As a gx user, I want each migration step to have a unique name that the store remembers, so that two epics that change the schema can land in either order and both steps still run.
5. As a gx user, I want the applied list to be per project, so that one project's state never decides what another project runs.
6. As a gx user, I want the applied list to be a plain text file with one step name per line, kept in the project directory of the git-backed store, so that I can read it and git history keeps it.
7. As a gx user, I want only migration code and project creation to write the list, and migration code only after a step succeeds for that project, so that a failed step is never marked as applied.
8. As a gx user with an existing store and no lists yet, I want the first start to run every step once and then record them, so that the store is brought current without any manual step.
9. As a gx user creating a new project, I want it to start with every current step name already on its list, so that a fresh project is not migrated for nothing.
10. As a gx user, I want backfill steps to run once like every other step, so that a start does not redo slow history scans.
11. As a gx user, I want a step to count as applied when it finishes without error, even if it skipped unreadable transcripts, so that the 487 harmless skipped-transcript notes on the gx project do not make the step run on every start.
12. As a gx user, I want skip notes to go to the log only and not to the web strip, so that the strip shows only failures that need me.
13. As a gx user, I want `gx tickets migrate <path>` to still rerun the steps by hand, so that I can retry or repeat a backfill.
14. As a gx user, I want `gx server start` to return as soon as the server listens, so that a long migration never hits the 45 second handshake limit and files a false launchd failure.
15. As a gx user, I want the handshake to report "migrating" while migration runs, so that `gx server start`, `gx server status` and other clients can tell a busy server from a dead one.
16. As a web user, I want the web snapshot to report "migrating", and the page to say so, so that I know why nothing is scheduled yet.
17. As a gx user, I want a failed migration to be saved and the server to start anyway, so that one broken project never takes the whole server down.
18. As a gx user, I want a project with a failed migration to get a Migration block (none of its tickets claimed) while other projects keep scheduling, so that a failure stays contained.
19. As a web user, I want a strip that shows the failed project and its error, with a "Retry migration" button and a "Fix agent" button, so that I can recover from the browser (ADR 0042).
20. As a web user, I want "Retry migration" to run the project's pending steps again, so that a transient failure ends without a restart.
21. As a web user, I want "Fix agent" to keep working as it does today, so that I can hand a real defect to an agent.
22. As a gx user, I want the Migration block to end, and the project's tickets to be scheduled, as soon as a migration of that project passes, by either button or by `gx tickets migrate <path>`, so that I never have to restart the server to release a project.
23. As a gx user, I want the failure strip to go when the Migration block ends, and the server to clear a project's saved failure when its own start-up run passes, so that the page matches the server.
24. As a gx user running an older binary after a newer one, I want a step name in the list that my binary does not know to be ignored with one logged warning, so that a downgrade or a branch switch does not break the start.
25. As a gx user, I want the list to be append-only, so that a name stays on the list for a newer binary to find.
26. As a gx user, I want the restart dialog to dry-run only the steps that are pending for the new binary, per project, so that the check is fast and tells me what the restart would do.
27. As a web user, I want a failed dry run to keep the warning and the "Restart anyway" button, and to say that the project will be in Migration block after the restart and can be retried from the strip, so that I know the real consequence.
28. As a web user, I want the line "A migration runs while the agents work." to appear only when some project has pending steps, so that the dialog does not claim a migration that will not happen.
29. As a developer, I want one ordered registry of named steps, used by both the CLI and the server, so that they can never drift apart. A code review checks that no second step list exists.
30. As a developer, I want a test that rejects duplicate or empty step names and a registration that panics on a duplicate, so that a mistake fails fast.
31. As a developer, I want a guard test that fails when the ticket schema changes without a new step, with a message that says to add a new named step and then update the snapshot, so that a schema change cannot ship without a migration.
32. As a developer reviewing a design, I want the gx-design review rubric to flag a schema change with no new step, so that the gap is caught before implementation.
33. As a gx user, I want `gx server restart` to no longer take `--migrate`, so that there is one way to migrate and no stale flag.
34. As a gx user, I want `gx tickets migrate --to-store` to stay as it is and to write no applied list, so that moving an old tracker into the store is unchanged and the first server start runs every step on it once.
35. As a gx user, I want every ticket-store write (a ticket file update, an epic setting, a ref, a move, a ticket-log append, a new ticket or a new epic in the project; for example answer, edit, land, agent output) to wait while its project migrates and then continue on the new shape, so that a write is never lost or applied to half-migrated data, and readers see old or new data, never half.
36. As a gx user, I want a project in Migration block to show "Migration blocked" on the web strip, so that I can tell it from Paused and Missing trunk.
37. As a gx user, I want the server to start on a store that still has old-shape tickets, and to run no merge recovery, run reclaim, parent-defect scan or first ticket rescan until all migrations end, so that no start-up code works on half-migrated tickets.

## Implementation Decisions

**Step registry (decision 08)**

- One ordered registry lives in a package that both the `cmd` package and the `server` package import. It replaces the hard-coded chain in `migrateTracker`.
- Each entry has a fixed unique name and a function. The current steps become registry entries in their current order, except that the pause migration, `Migrate` and the question-section step (`MigrateQuestionSections`) form one entry, a single ordered unit. Its order is: `PlanPauseMigration`, `Migrate`, `MigrateQuestionSections`, `ApplyPauseMigration`. The plan must run first, because `Migrate` and the question-section step lose a repair ticket's pause reason (see `tickets/migrate_pause.go`). The apply must run after the question-section step, because `ApplyPauseMigration` renames "## Needs Answer" to "## Paused" and `MigrateQuestionSections` still looks for "## Needs Answer"; this keeps today's order of those steps. A retry reruns the whole unit, and the plan finds the reason again in the event log. The backfill steps that ran between `Migrate` and the apply today move after the unit. The implementer picks the names. The order is the run order and is kept.
- Registration panics on a duplicate name. A unit test rejects duplicate and empty names.
- A released name is never reused, and never renamed. Rule for contributors: to change a step's behaviour, add a new step.
- `gx tickets migrate` and the server run the same registry and differ only in which names they skip. The server skips every name on the project's list. `gx tickets migrate <path>` skips none, so it reruns all steps by hand as it does today (the steps are already safe to repeat).
- Both record each step after it succeeds for that project. A name is appended only when it is not on the list, so the list never holds a duplicate line.
- A dry run uses the registry and the skip rule of the command that owns it. `gx restart-check` skips names on the project's list, like the server. `gx tickets migrate --dry-run <path>` skips none, like the manual run. A dry run writes nothing outside its scratch copy.

**Applied list (decisions 01, 02, 03, 06)**

- Per project. A plain text file, one step name per line, append-only, in the project directory of the git-backed store (not the state dir). The implementer picks the file name and any hidden marker rule (comments, blank lines).
- Only migration code and project creation write it. Migration code writes only after a step succeeds for that project. The write goes into the store's git history like other store files; whether it is its own commit follows how the store already commits tickets.
- A project with no list runs every step in the binary once, then records them. A new project created by the server (project add, outbox, scratch) is created with the list already holding every current step name. A project created by `gx tickets migrate --to-store` gets no list: `MigrateIntoStore` applies only the legacy frontmatter fixes, so the first server start runs every step on it.
- A step is applied when it finishes without error. Skipped unreadable transcripts do not count as errors. Skip notes go to the log only and never to the web strip or the saved failure record.
- A name in the list that the binary does not know is ignored. Known steps still run. The server logs one warning per start for the set of unknown names (not one per name per project).
- A failed step is not recorded, and the steps after it do not run. A later retry runs the failed step and the rest.

**Where it runs (decisions: location, 04)**

- Migration runs inside the `gx server` process, started at launch whatever started it (launchd or `gx server start`). launchd never runs `start` or `restart` command code, so no command code may be where the migration lives.
- Order at startup: the server takes its lock, listens, and answers the handshake first. Then it migrates all projects in the background. The projects are migrated one after another; the implementer may choose to parallelise if it does not break the rule below.
- No work is scheduled until all projects' migrations have ended, pass or fail. Scheduling covers every source that claims tickets or starts agents on its own (the queue, recovery tickets, merges triggered by the scheduler). Manual actions by the person are not blocked, but their writes to a migrating project wait (next bullets).
- Start-up code that reads or changes tickets waits for all migrations to end: `recoverMerges`, `reclaimRuns`, `scanParentDefects` and the first ticket rescan run after the last migration, pass or fail. `server.New` does not fail when the store has old-shape tickets: its index scan is tolerant (a ticket or epic it cannot read is skipped and shows after the rescan) or deferred until the migrations end. The implementer picks which. A project with a saved failure is rescanned and recovered like the others once all migrations have ended.
- The handshake and the web snapshot carry a "migrating" state, true from the moment the server listens until all migrations end. `gx server status` shows it. The web shows one plain "Migrating tickets" line while it is true (see "Failure and Migration block" below).
- `gx server start` returns when the handshake answers, so the 45 second `stopTimeout` limit no longer depends on migration time. A dry run on the gx project takes 23 s and a real run is untimed, so this is a real risk without the change.
- A project that is created while the server runs (new project in the store) is created with the full list, so it needs no migration.
- A per-project migration lock keeps the server and `gx tickets migrate <path>` from migrating the same project at once. It is a shared/exclusive lock: a migration takes it exclusive, and ticket writes take it shared, so writes do not queue behind each other when no migration runs. The implementer picks the mechanism (a lock file, for example). A manual run while the server migrates that project waits for it or refuses with a clear message; the implementer picks which.
- Every write path into a project of the ticket store is covered, not only the three named here: ticket file updates (`schema.UpdateTicket*`, used by the server's `ask.go`, `recovery.go` and `followups.go` and by `cmd/tickets_section.go`), ticket-log appends, new-ticket creation, and the other writers (loader writes, epic settings and creation, refs, move, start-over). The implementer lists them with gopls (`go_symbol_references`), not from this list. Each takes the lock shared. A write to a project (a person's answer or edit, a landing, an agent's ticket output) waits for a running migration of that project, then proceeds on the new shape. Readers take no lock and see old or new data, never half, so migration steps must replace files atomically. Agents that survive a restart keep working. Only their writes wait. Retry migration takes the lock too, so a person's edit to a project in Migration block waits for it as well.
- Migration steps write through an inner write path that does not take the lock, so a step never waits for the lock its own migration holds (`ApplyPauseMigration` and `MigrateQuestionSections` call `schema.UpdateTicketWithBody` and the log append today). Lock order is the migration lock first, then `LockTicket`, everywhere.
- A waiting write has no time limit, because a migration always ends, pass or fail. After 1 s of waiting the CLI prints "waiting for the migration of <project>", so an agent's `gx` call shows why it is slow.

**Failure and Migration block (decision 05, failure)**

- A failed project: the failure is saved with the existing migration failure record (`migratefail`), the server starts anyway, and the project gets a **Migration block**.
- A Migration block is its own gate, not Paused and not Missing trunk. The server claims none of that project's tickets until the failure record clears, then carries on by itself. In code it is a gate function beside `trunkProblem` in `server/missingtrunk.go`, not a reuse of it, so the two states keep separate reasons and labels. Other projects keep scheduling once all migrations have ended.
- No new pause kind. Why gx cannot recover alone: a migration step failed on this project's own tickets (for example an invalid ticket the schema now rejects). Retrying the same step on the same data gives the same error, and a repair may need a person's judgement about their content. So gx saves the failure and blocks the project. The block ends from the web: "Retry migration" (for a transient or an already-fixed cause) and "Fix agent" (for a real defect). Either path ends the block once a migration passes, as ADR 0042 requires. `gx tickets migrate <path>` also ends it but is not the only way.
- The block ends when the project's saved failure record clears. The server's existing watcher of that record sees it, so Retry migration, a manual `gx tickets migrate <path>` and a Fix agent repair all end the block without a server restart. A Migration block stops only the scheduler: a person can still answer, edit and land tickets in that project (their writes wait only while a migration of that project runs).
- The server clears a project's saved failure when its own start-up run for that project passes, and when Retry migration passes, as `gx tickets migrate` does. A stale record from an earlier start or a pre-epic web restart therefore does not block a project after a passing start.
- The failure record gets a file lock around its read-change-write. The server records failures from its background run and from Retry, while `gx tickets migrate` clears them from another process, and without the lock a clear of project B could undo a record of project A.
- While migrating, the web shows one plain "Migrating tickets" status line and blocks nothing in the page.
- "Retry migration" is a new server verb the web serves. It runs the project's pending steps in the server, like the start-up run. It is refused while a migration of that project is already running.
- "Fix agent" is the existing button and dialog, unchanged. The agent runs `gx tickets migrate <path>` after its fix.
- The web strip keeps what it shows today (project, error, retry command) and gains the "Retry migration" button next to "Fix agent". The strip reads "Migration blocked" for the project. The strip row goes when the block ends.
- A migration failure on one project has no effect on the "migrating" state ending for the others.

**Commands (decision: commands)**

- `gx server restart` loses `--migrate`. The CLI restart and the web restart both stop the server and start it again; the new process does the migration. Where `restartSteps` migrated between stop and start, it now just stops and starts.
- The web restart keeps logging to the restart log so the dialog's timeout text still has a log to name. The hidden `--continue` flag stays accepted and only turns on restart-log output. The running (old) server runs `<new bin> server restart --continue` (`server/restart.go:109`), so a new binary that rejects the flag breaks the first web restart into it. The implementer may not drop or rename it.
- `gx tickets migrate <path>` stays. `gx tickets migrate --to-store` is unchanged, and writes no applied list.
- The dialog timeout text "The server did not return. It may still be migrating." is now rare (the server answers while migrating); it keeps its meaning for a server that did not start at all, and the implementer rewords it if the old words are wrong.

**Restart dialog and `restart-check` (decision 07)**

- The restart check dry-runs only the steps that are pending for the new binary, per project: the new binary's registry minus the project's applied list. A project with no pending steps reports ok with no file changes, and costs no scratch copy.
- A project with no list has every step pending, so its dry run still runs the whole chain.
- `gx restart-check` skips names on the project's list, like the server (not like `gx tickets migrate`, which skips none). A failed dry run keeps the warning and the "Restart anyway" button. The text changes from "The failed migration is saved, and the server starts anyway." to say that the project will be in Migration block after the restart and can be retried from the strip.
- The line "A migration runs while the agents work." shows only when the agents-running line shows and at least one project has pending steps. The restart-check report therefore tells the web which projects have pending steps; the implementer picks the field (a per-project count or flag).

**Schema guard (decision 08)**

- A guard test stores a snapshot of the ticket schema: fields, enums and section names. It fails when the schema changes without the snapshot changing. Its failure message says to add a new named step to the registry and then update the snapshot.
- The snapshot file format and location are the implementer's choice, but it must be readable in review.
- The gx-design review rubric (the repo copy under `skills/gx-design/SKILL.md`, "Design-review rubric (step 5)") gets one line: a schema change with no new migration step is a finding. This spec does not rely on `.ai/index.md`; the installed binary does not ship it.

**Domain terms and ADRs**

- Use the GLOSSARY terms: project, ticket store, Fix agent, Paused, Missing trunk, **Migration block**, **Migration step**. "Hold" is not a glossary term, so the spec says "in Migration block" for a project, never "blocked" (the strip text "Migration blocked" is the one exception).
- GLOSSARY.md gets entries for **Migration block** and **Migration step**, and the **Fix agent** entry says "or at server start" in place of "in a web restart". The **Missing trunk** _Avoid_ line changes to "held or paused project (a project is never Paused; a ticket is Blocked by its `blocked_by` edges; a project with a failed migration is in **Migration block**)".
- ADR 0042 is respected: no new pause kind, and the web buttons that end the block are named above.
- [ADR 0045](../adr/0045-server-runs-ticket-migrations.md) records that the server runs migrations itself from a per-project applied list in the store.

## Testing Decisions

A good test checks behaviour from outside: what a started server does with a store, what the handshake and snapshot say, what the CLI prints. It does not check which function calls which.

Seams (existing seams preferred, highest first):

1. **Server over its API** (`servertest.Harness` and the real process fixtures in `testutil/gxfixture`): start a server on a store with pending steps, read the handshake and snapshot while it migrates, assert no ticket is claimed until migration ends, assert the list file and the failure record after. This is the one main seam. Prior art: `server/restartcheck_test.go`, `server/restart_test.go`, and the `restart-projects` fixture scenario.
2. **Registry and applied list** (unit tests in the registry package): duplicate and empty names, the panic on duplicate, skip-by-name, record-after-success, unknown name ignored with one warning, a step that returns an error is not recorded, first run on a store with no list, a new project's list.
3. **Schema guard test**: the snapshot comparison itself, and a check that the message names the next action.
4. **CLI**: `gx tickets migrate <path>` reruns every step and records; `gx server restart` rejects `--migrate`. Prior art: `cmd/tickets_migrate` tests.
5. **Test hook for the fixture server.** `testutil/gxfixture` and `servertest.Harness` get a test-only hook that holds a migration open (until the test releases it) and one that adds a failing step or a step with skip notes to the registry. The implementer picks the mechanism (extra test-only registry entries, an env var, or an injected registry). It is compiled out of, or inert in, the release binary.
6. **Restart dialog** (vitest on `RestartDialog` and the strip, using the injected `check` and `restart` props) and **web e2e flows** for the user-visible parts. Prior art: `RestartDialog.test.tsx`, `FailedMigrationStrip.test.tsx`, flows 133 to 146.

Behaviours that need tests at seam 1: the server answers the handshake while migrating; a ticket write waits for a running migration of its project; the server clears a stale failure on a passing start; `gx server start` returns without waiting for migration; scheduling waits for all projects; a failed project is in Migration block while another is scheduled; the Migration block ends after a passing retry by the button, by the CLI, and by the Fix agent route; an unknown name logs once; a restart with nothing pending runs nothing.

## Verification

Every entry below runs against a temporary gx server. Where a flow needs a fixture scenario that does not exist, it appears under "New flows and scenarios". The catalog is `webui/e2e/CATALOG.md`: each new flow gets the next free number, a scenario, steps and a pass line, and the code goes in the e2e file the catalog's header lists for its area (restart flows live with flows 133 to 146). Two test hooks (see Testing Decisions, seam 5) are used below: **hold-open** (a migration stays running until the test releases it) and **inject-step** (the test adds a failing step or a step with skip notes).

1. Story 1: run a server on a store with one pending step (new flow A); no flag was given and the step ran. CLI: `gx server start` on a fixture store with a `broken-shape` ticket in the old shape, then `gx tickets show` prints the new shape.
2. Story 2: run flow A from each start path: `gx server start`, `gx server restart`, web restart (flows 141 and A). Crash restart and login start get a real check (new flow H): start the `gx server` process directly, as launchd does (no `start` or `restart` command code), on a store with a pending step; assert it migrated. Kill the process, add another pending step through inject-step, start it again the same way, and assert the new step ran and the earlier ones did not. Each start ends migrated.
3. Story 3: new flow B (no new work during migration): hold-open the migration; assert a ready queue ticket is not claimed, a pending recovery ticket does not start, and a merge the scheduler would start does not start; all start after the migration ends. An agent that was already running is not killed.
4. Story 4: unit test on the registry (two steps landing in either order). CLI check: build two stores, one with step X on its list and one with step Y; start both; assert each store ran the step it lacked, and both lists now hold both names.
5. Story 5: new flow C: two projects, only one with a list; only the other runs its steps (assert the lists).
6. Story 6: CLI check: `cat` the list file in the project directory; it is plain text, one name per line.
7. Story 7: new flow D (failing step not recorded): inject-step a failing step; its name is not in the file; steps before it are. CLI check: only migration code and project creation write the file (no other command changes it).
8. Story 8: flow A with no list file; afterwards the file holds every name in the binary.
9. Story 9: CLI check: create a project through the server and `cat` its list; it holds every current name; the server's next start runs nothing.
10. Story 10: flow A, then a second start; the log shows no backfill step ran.
11. Story 11: CLI check on a fixture store (a temporary project with one unreadable transcript, never the real gx store): the skipping step is on the list afterwards; the log (not the strip) has the skip note; the next start does not rerun it.
12. Story 12: flow D variant: inject-step a step with skip notes; the notes appear in the server log and the strip shows nothing.
13. Story 13: CLI check: `gx tickets migrate <path>` after a full start reruns all steps and exits 0; the file has no duplicate names, and a rerun appended only missing names.
14. Story 14: CLI check: hold-open a migration for 60 s; `gx server start` exits 0 in well under 45 s and prints no launchd failure report.
15. Story 15: CLI check: during the held migration, `gx server status` says migrating; the handshake JSON has the migrating state; after, it does not.
16. Story 16: new flow B: the page shows a plain "Migrating tickets" line while held, and the line goes when migration ends. Web snapshot check through the e2e.
17. Story 17: new flow E (failed start): see below. CLI: after a start with one broken project, `gx server status` is running and the failure record has the project.
18. Story 18: new flow E: `broken` is in Migration block (its ready ticket is not claimed), `demo` schedules.
19. Story 19: flow 139 rewritten (see below) and new flow E: the strip reads "Migration blocked" for `broken`, the error, the command, and both buttons.
20. Story 20: new flow F (Retry migration): fix the ticket on disk, click "Retry migration"; the strip goes and `broken` schedules. Also a click while the retry runs is refused.
21. Story 21: flow 140 unchanged except it starts from a failed server start, not a failed restart.
22. Story 22: flow F with three variants: Retry migration, `gx tickets migrate '<dir>'` by hand, and the Fix agent route (the session runs the command); each ends the Migration block with no restart (the pid is unchanged).
23. Story 23: flow F: the strip is gone after the block ends, and reappears on the next failing start. Stale record: save a failure for `broken`, fix its ticket, restart; the server's start-up run passes and clears the record, and the strip goes (flow 139 step 4). Lock: two processes record and clear failures for different projects in parallel; the final file holds exactly the expected records (unit test on `migratefail`).
24. Story 24: CLI check: add an unknown name to a project's list, start; the log has exactly one warning; known steps ran; the file still holds the unknown name.
25. Story 25: CLI check: after any start the unknown name from story 24 is still in the file and the other lines were only added to.
26. Story 26: flow 138 rewritten and new flow G (pending steps only): the dialog's check for a project with nothing pending runs no dry run (fast, ok); a project with a pending step reports its files. CLI check against the real check: run the real `gx restart-check` on a fixture store with one project fully listed and one with a pending step; the first reports ok with no scratch copy, the second reports its files. Also a project with a list holding a name the dry run would otherwise rerun is skipped (server rule, not manual rule).
27. Story 27: flow 138 rewritten: the alert reads "broken: fixture migration failed", says the project will be in Migration block after the restart and can be retried from the strip, the button reads "Restart anyway", and the click restarts.
28. Story 28: flow 137 rewritten and new flow G: the line "A migration runs while the agents work." shows when some project has pending steps and is absent when none has.
29. Story 29: code-structure review check (not a test; Testing Decisions rule out import tests). The review confirms the CLI and the server read steps from the one registry and that no second list of steps exists. `rg` for the old chain in `cmd/tickets_migrate.go` finds none.
30. Story 30: unit tests for duplicate and empty names, and for the registration panic.
31. Story 31: unit test: change the ticket schema in a scratch copy of the guard and see the failure message ("add a new named step, then update the snapshot").
32. Story 32: a text check: `rg` finds the new line in `skills/gx-design/SKILL.md`.
33. Story 33: CLI check: `gx server restart --migrate` exits non-zero with "unknown flag"; `gx server restart --help` has no `--migrate`; `gx server restart --continue` is still accepted.
34. Story 34: CLI check: `gx tickets migrate --to-store --dry-run <old-root>` prints what it did before this epic. After a real `--to-store`, the new project has no list file, and the first server start runs every step on it. A pause-migration case: a needs-repair ticket imported by `--to-store` keeps its pause reason after that start. Pause step order: run a partial migration that stops after the pause unit's `Migrate`, then retry; the whole unit reruns and the reason from the event log is kept. A needs-answer ticket with a "## Needs Answer" section gets its question in the ticket log, and the section becomes "## Paused" only after that.
35. Story 35: new flow I (writes wait): hold-open the migration of `demo`; send an answer to a ticket of `demo` and an edit to a ticket of another project; the first does not return until the migration ends and then lands on the new shape, the second returns at once. A read during the migration returns a whole old or whole new ticket. Log-append case: a ticket-log append to `demo` during the hold waits the same way and lands after it. The same for a write to a project in Migration block during Retry migration. The same for the other write paths: an epic setting, a move and an epic creation in `demo` wait and land after the hold. After 1 s of waiting the CLI printed "waiting for the migration of demo", and the write has no time limit. Deadlock case: a migration with a pausing step (the pause unit, which writes tickets itself) completes under hold-open release while a write waits; it does not block on its own lock.
36. Story 36: flow E: the strip text is "Migration blocked", not "Paused" or "Missing trunk"; the project is not listed as paused.
37. Story 37: flow B (hold-open) on a store whose `demo` ticket has an old shape the strict index scan rejects: the server starts and answers the handshake; while held, the log shows no merge recovery, run reclaim, parent-defect scan or ticket rescan; after the release each runs once and the ticket shows in the snapshot in its new shape. A project with a saved failure (`migrate-broken`) is rescanned after the end too. Unit test on the index scan with an old-shape ticket: `server.New` does not fail.

Flows that exist and change: 137 (migration line is conditional), 138 (text changes; no longer saves a failure), 139 (the failure is produced by a start, not by `restart --continue`; add Retry migration and the Migration block; the `provisional_destination` to `destination_unconfirmed` rewrite now happens at start), 140 (starts from a failed server start), 141 to 146 (rerun; they must still pass).

New flows and scenarios to write (these become implementation tickets):

- **Flow A, server migrates on start.** Scenario `migrate-pending`: the real server runs against a store with a project that has no list and a ticket in the old shape. Pass: no flag, the ticket is migrated, the list holds every name, the second start runs nothing.
- **Flow B, migrating state and no scheduling.** Scenario `migrate-held`: the migration is held open by the fixture. Pass: the handshake and snapshot say migrating, the page shows the line, a ready ticket is not claimed, `gx server start` returns fast, and when the migration ends the ticket is claimed.
- **Flow C, per-project list.** Two projects, one with a list.
- **Flow D, failed step not recorded, skip notes in the log only.**
- **Flow E, failed start and Migration block.** Scenario `migrate-broken`: project `broken` fails, `demo` is fine. Pass as stories 17, 18 and 19.
- **Flow F, ends the Migration block three ways.** Reuses `migrate-broken`.
- **Flow H, crash restart and login start.** Scenario `migrate-pending` started as a bare `gx server` process, killed and started again.
- **Flow I, writes wait for migration.** Reuses `migrate-held`.
- **Flow G, restart dialog with and without pending steps.** The `restart-projects` scenario needs a per-project pending count in its fake `check.json`.
- Unit and package tests for the registry, list, guard snapshot and unknown names (no flow).

There are no mocks for this epic, so there is no Mock states list.

## Out of Scope

- Config and state-file migration (`config.WarnOnMigrateFailure`). It already runs by itself at CLI start and has no manual step.
- Steps that undo a migration, and downgrades of the store. The store is a git repo, so git history is the way back.
- Several machines sharing one store. The store pushes to its remote and never pulls, so it is not a concern now.
- A new pause kind for a failed migration.
- Making the slow steps faster (the 23 s dry run on gx is mostly copying files).
- Changing what any of the 13 existing steps do.

## Further Notes

- Facts on 2026-11-01: `migrateTracker` has 13 steps and no version. A dry run on the gx project takes 23 s and reports 490 changed files (487 skipped transcripts, 3 grilling counts). The store pushes to its remote and never pulls.
- Today's chain never comes back clean on the gx project (the skipped transcripts). That is why "applied" means "finished without error", not "changed nothing".
- The clean fix for the existing restart path is to remove the migration from `restartSteps`, not to keep a second migration runner there; the spec removes it.
