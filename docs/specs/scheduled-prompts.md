## Problem Statement

A person wants gx to run a prompt at set times without anyone watching, for example "Check the weather
and notify me" every day at 07:00 and 21:00. Today gx has no way to do this. The server has no
time-of-day timer, and the only outside trigger is `gx server one-off`, which a person must call from
cron or a script on each machine. There is no web page to set the prompt and its times, and nothing
tells the person what ran, what was skipped, or why.

## Solution

A new web page, "Scheduled prompts", lists **Schedules**. A person adds a Schedule with a name, a
prompt, the days of the week, one or more times of day, a project, a harness and a Late limit. At
each set time the gx server starts the prompt as a One-off in the Schedule's project (a **Scheduled
run**). The run's agent may tell the person something by running `gx notify "<message>"`, which sends
to Telegram or Slack. Nothing is sent unless the agent runs it.

Each Schedule has a **Schedule switch** per machine, so two machines that share a ticket store do not
both run it unless the person wants that. The Schedule page shows the last 10 runs, each linked to its
One-off ticket and Result, and shows skipped runs with the reason. A person can also run a Schedule
now. `gx schedules` CLI verbs mirror every action on the page.

## User Stories

1. As a person, I want to open "Scheduled prompts" from the Epics menu in the sidebar footer, so that
   I can find my Schedules without a new top-level entry.
2. As a person, I want a list of my Schedules showing each name, its days and times, and its next run
   time, so that I can see at a glance what will run next.
3. As a person, I want a Schedule that is off on this machine to say "Off here" instead of a next run
   time, so that I do not expect a run that will not happen.
4. As a person, I want a banner when every Schedule is off on this machine, so that I notice that
   nothing will run here.
5. As a person, I want an empty state with an example when I have no Schedules, so that I know what
   the page is for.
6. As a person, I want a "New" button in the page header, so that I can add a Schedule.
7. As a person, I want to turn a Schedule on or off for this machine from its list row, so that I can
   control it without opening it.
8. As a person, I want each list row to open the Schedule page, so that I can read and edit the
   Schedule.
9. As a person, I want to set a Schedule's name, prompt, days, times, project, harness and Late limit
   on one page, so that I can define it in one place.
10. As a person, I want day pills plus "Weekdays", "Every day" and "Weekends" shortcuts, so that I can
    pick days fast.
11. As a person, I want to add and remove several times of day, so that one Schedule can run twice a
    day.
12. As a person, I want the page to say times are local time on this machine, so that I do not
    misread them.
13. As a person, I want the project to default to `scratch`, so that a Schedule needs no project
    choice to work.
14. As a person, I want the harness to default to "Default (<harness name>)" and to pin claude or
    codex if I pick one, so that a Schedule follows my default harness unless I choose otherwise.
15. As a person, I want a Late limit in hours that defaults to 2, so that a run missed while the
    machine slept or the server was down still starts if it is not too late.
16. As a person, I want the prompt written in the shared prompt box, so that I edit it the way I edit
    other prompts.
17. As a person, I want Save and Discard to appear only when I changed a field, and "Create" on a new
    Schedule, so that nothing saves by accident.
18. As a person, I want inline errors when a required field is missing or wrong, so that I can fix it
    before saving.
19. As a person, I want the server to refuse an invalid Schedule with a reason, so that the CLI and
    the page agree on what is valid.
20. As a person, I want a Schedule to start its One-off at each set time within about a minute while
    the server is up, so that "07:00" means 07:00.
21. As a person, I want a Scheduled run to be a One-off in the Schedule's project, so that it uses the
    one-off queue, the caps and the usage gate like any other One-off.
22. As a person, I want gx to add a short note to the prompt of each Scheduled run that explains
    `gx notify`, so that the agent can notify me without me writing the command into the prompt.
23. As a person, I want `gx notify` to behave exactly as it does today in a Scheduled run, so that
    one notification path serves every session.
24. As a person, I want a run missed while the server was down or the machine slept to start when
    the server is back, if it is late by less than the Late limit, so that a short outage does not
    lose a run.
25. As a person, I want only the latest missed time to run, and only once, so that a long outage does
    not start a burst of runs.
26. As a person, I want time that passed while the switch was off never to run, so that turning a
    Schedule on does not start old runs.
27. As a person, I want a run past the Late limit to be skipped and logged with the reason, so that I
    can see why nothing ran.
28. As a person, I want the Late limit to count from the set time to the moment the run is queued, so
    that a long queue wait does not turn a run into a skip.
29. As a person, I want a new run to be skipped when the previous run of the same Schedule has
    started, is paused or has failed, so that two runs of one Schedule never work at once and a
    broken run is not buried under later runs. A paused or failed run's skip reason says "previous
    run needs attention" and links its ticket. A run that is only running gives "previous run
    started".
30. As a person, I want a new run to replace the previous run when that run is still queued and never
    started, so that the queue holds one run per Schedule.
31. As a person, I want "Run now" on a Schedule page, so that I can test a prompt without waiting.
32. As a person, I want "Run now" to follow the same replace and refuse rules as a timed run, and to
    show a visible reason when it is refused, so that I know why nothing started.
33. As a person, I want "Run now" to work while the switch is off on this machine, and the page to
    say so, so that I can test a Schedule I have not switched on.
34. As a person, I want "Run now" to queue the run even when the one-off cap is full, so that the run
    starts when a slot frees.
35. As a person, I want the Schedule page to list the last 10 runs, each with its time, a link to its
    One-off ticket, a one-line Result and a status (Finished, Ran late, Skipped, Needs answer,
    Running, Queued, Cancelled), so that I can see what happened. A Run now row shows "Run now" in
    the time column.
36. As a person, I want skipped runs to show without a ticket link and with their reason, so that I
    can tell a skip from a run.
37. As a person, I want runs started by any machine that shares the ticket store to show in Last
    runs, so that I see the full history in one place.
38. As a person, I want a Schedule to run on every machine where its switch is on, each with its own
    copy of the run, so that two machines do not block each other.
39. As a person, I want a new Schedule to be on only on the machine that created it, so that adding
    it on one machine does not start runs on my other machines.
40. As a person, I want a missing project to skip the run, show "project not found" on the list row
    and the Schedule page, and send me one Telegram or Slack error message per skipped run time
    through the usual `gx notify` gate, so that a broken Schedule does not fail silently.
41. As a person, I want the same skipped run time not to send its error message again after a server
    restart, so that I am not spammed.
42. As a person, I want to rename a Schedule without changing which Schedule it is, so that its
    history stays attached.
43. As a person, I want gx to refuse a name whose slug exists in the same project, so that two
    Schedules never collide.
44. As a person, I want the project to be read-only on an existing Schedule, so that its history and
    runs stay with one project.
45. As a person, I want an edit to apply when I press Save, and times before the save never to count
    as missed, so that an edit does not start a run I did not ask for.
46. As a person, I want to delete a Schedule with an inline confirm that says what happens to its
    runs, so that I do not delete by accident.
47. As a person, I want delete to cancel queued runs that never started and let live and paused runs
    continue, so that delete stops future work without killing work in progress.
48. As a person, I want a Scheduled run that pauses or needs an answer to show as "Needs answer" and
    be recoverable, or cancellable, from the One-off ticket page, so that a Schedule adds no new pause kind.
49. As a person, I want a time that does not exist because of a daylight saving change to run at the
    first valid moment after the gap, so that the run is not lost.
50. As a person, I want a time that happens twice on a daylight saving change to run once, so that I
    do not get a duplicate run.
51. As a script or agent, I want `gx schedules` verbs that list, show, create, edit, switch, run now
    and delete Schedules with `--json`, so that I can do everything the page does from the CLI.
52. As a person, I want a Scheduled run to wait on the usage gate like any One-off, so that Schedules
    do not burn my limit past the gate.

## Implementation Decisions

- **A Schedule is a stored prompt plus run times.** It has a name, a slug, a prompt, a set of days of
  the week, a set of times of day (HH:MM, local time of the machine), a project, an optional harness
  and a Late limit in hours. The Late limit defaults to 2 and may be decimal. There is no cron
  expression now. The stored form leaves room to add one later without breaking stored Schedules.
- **Storage.** One markdown file per Schedule, under its project in the ticket store, so every machine
  that shares the store sees it. The implementer picks the file layout and the slug function. The
  store's existing scans (ticket listing, epic listing, migrations, archive moves) must ignore the new
  files. The Schedule file is a new kind of file, not a ticket or epic schema change, so it needs no
  Migration step of its own.
- **Slug and project are fixed at creation.** Rename changes only the display name. A name whose slug
  already exists in that project is refused. The project is read-only on an existing Schedule.
- **Schedule switch.** The switch is per machine and lives in that machine's state directory, not in
  the Schedule file. A new Schedule is on only on the machine that created it. Another machine shows
  it as "Off here" until its person turns it on.
- **A Scheduled run is a One-off.** The server submits it through the existing One-off path, in the
  Schedule's project, with the Schedule's harness. It is queued, counts against the one-off cap
  (ADR 0040) and the global and project caps (ADR 0032), waits on the usage gate, and ends by itself.
  A Scheduled run is not a new Chat session. No new pause kind is added.
- **Intended time on the One-off.** The One-off ticket gets one optional field, `scheduled_for`: the
  set time the run was meant for. It is empty for Run now. Only the scheduler sets it, through the
  in-process submit path. `wire.OneOffRequest` and `gx server one-off` do not change, so an outside
  caller cannot fake a Scheduled time. Last runs reads it for the time column, for "Ran late" and for
  Run now rows, across machines. The Dedupe key cannot hold it, because the key is the same for every
  run of a Schedule. The implementer picks the exact name only if `scheduled_for` clashes with an
  existing field.
- **Migration.** The new field is optional and absent means "not a Scheduled time". Tickets written
  before it need no rewrite, so the step rewrites nothing. A no-op named Migration step is still
  registered for `scheduled_for`, because `TestSchemaSnapshot` fails for any ticket schema field change
  until a named step exists and the schema snapshot is updated. The change is a schema and validation
  update only, and `gx tickets validate` accepts tickets with and without the field.
- **Harness.** The first option is "Default (<harness name>)", stored as unset and resolved on every
  run. Picking claude or codex pins it.
- **Prompt.** The server sends the person's prompt followed by a note. The note says the run was
  started by the Schedule "<name>" with no one watching, that `gx notify "<message>"` sends to
  Telegram and Slack only, that nothing is sent unless the agent runs it, that the message should
  start with "<name>:" and be short, and that the agent should not ask questions with `gx tickets ask`
  unless it cannot continue. The implementer may polish the wording.
- **Notify.** A Scheduled run uses `gx notify "<message>"` as it works today. There is no new verb and
  no server route for notify. It reaches Telegram and Slack only and shows no banner. The existing
  gate is unchanged, so the 5-per-minute mute, manual mutes and muted transports apply. gx adds no
  check for whether a session is a Scheduled run. The message is what the agent writes, with no added
  headline or link.
- **Timer.** The server checks once a minute and starts a run within about a minute of its set time
  while it is up. The same check does the catch-up after a sleep or restart. The implementer picks
  the ticker interval and how the scheduler gets its clock.
- **Catch-up baseline.** Each machine stores a last-handled time per Schedule in its state directory.
  It starts at creation or at switch-on. After a restart or wake, the latest set time after the
  baseline runs if it is late by less than the Late limit, once. Older missed times are neither run
  nor logged. Only the latest missed time runs or is logged as a skip. Time that passed while the
  switch was off never runs. An edit applies at Save, and times before the save never count as
  missed. The Schedule file stores an `updated_at` time, and each machine moves its baseline forward
  to `updated_at` when that is later, so a machine that was asleep during the edit also skips the
  earlier times.
- **Late limit.** Lateness counts from the set time to the moment the run is queued. Queue wait does
  not count. A run past the limit is skipped and logged. A run is "late" when its One-off was created
  more than one check interval (one minute) after `scheduled_for`. This comes from stored data, so
  every machine that shares the store agrees. The regular once-a-minute check always queues within one
  interval, so it never makes a run late. Only the catch-up path after a sleep or restart does.
- **Dedupe.** Each run carries the Dedupe key `schedule:<project>/<slug>@<machine>`. The machine part
  is a stable id that differs between machines that share a ticket store. The implementer picks how
  it is made. If the previous run of this Schedule on this machine is still queued and never started,
  the new run replaces it. If it has started, the new run is skipped. A paused run counts as started.
  A failed run also counts as started, because a failed One-off stays live under its Dedupe key. For a
  paused or failed run the skip reason is "previous run needs attention" and links the ticket. For a
  run that is only running it is "previous run started". Later runs skip until a person closes the
  paused or failed ticket.
- **Skip log.** Each machine keeps a skip log in its state directory with the newest 10 entries per
  Schedule, each with a time and a reason (past the Late limit, project not found, previous run
  started, previous run needs attention). The page merges it with the One-offs found by the key
  prefix `schedule:<project>/<slug>@`, so runs from every machine show. Skip rows show only on the
  machine that logged them. The log also stops the "project not found" error for a run time from
  repeating after a restart. The implementer cleans up state-dir entries of deleted Schedules.
- **Missing project.** The run is skipped, the list row and the Schedule page read "project not
  found" in place of the next run, and the server sends one error message per skipped run time
  through the existing `gx notify` gate (source `cli`, with its mutes), to Telegram or Slack. The
  skip log stops a repeat of the same run time after a restart. Project not found takes priority
  over the Late limit.
- **Two machines with the switch on.** Each runs its own copy. The "previous run live" check and the
  replace rule are per machine.
- **Run now.** It follows the timed rule. It replaces a queued never-started run. If the previous run
  started, is paused or has failed, Run now is refused with a visible reason on the page and in the
  CLI (the same reasons as the skip log, with the ticket link for "needs attention"), and no skip is
  logged. If the Schedule's project is not found, Run now is refused with the existing One-off reason
  `unknown-project` and no skip is logged. "project not found" stays as display text only. A full one-off cap only queues the run. It works while
  the switch is off on this machine, and the page says so.
- **Delete.** Delete removes the file. Each machine's scheduler cancels only its own queued
  never-started runs, when it sees the Schedule file is gone, and removes its state-dir entries. Live
  and paused runs continue. The delete confirm says so. Turning the switch off leaves a queued run
  alone and only stops new runs.
- **Daylight saving.** A time that does not exist on a spring-forward day runs at the first valid
  moment after the gap. A time that happens twice on a fall-back day runs once, at the first
  occurrence.
- **Validation.** Name, prompt, at least one day and at least one time are required. Duplicate times
  collapse into one. The Late limit must be a positive number of hours. The page shows inline errors.
  The server refuses with a `reason` per failure and a `refused` result, as ADR 0028 says.
- **Server API and CLI (ADR 0028).** One server route per `gx schedules` verb, each verb with
  `--json`, and the route table test in `cmd` must pass. The verbs cover list, show, create, edit,
  switch on and off, run now and delete. The implementer picks the verb names. Refusals are results,
  not HTTP errors.
- **Web.** The sidebar footer's Epics menu gets a separator after Archive and then "Scheduled
  prompts". The route is `/schedules`, with a Schedule page and a new-Schedule page under it.
  - List page: plain rows, no cards, no grouping. Each row has the name, the days and times, the next
    run (or "Off here") and the switch. The header has "New". There is an empty state and an "all off
    on this machine" banner.
  - Schedule page: always editable, in the Saira font. Rows with the label on the left: Days (pills
    and the three shortcuts), Times (chips, local time), Project, Harness, Late limit in hours, Prompt
    and Last runs. The name is an inline field in the header, with Run now and the switch beside it. A
    quiet "Delete this schedule" link sits at the bottom with an inline confirm. A Save and Discard
    bar shows only when a field changed. A new Schedule shows "Create". There is no autosave.
  - The Prompt field is the shared `PromptBox`. No new prompt component.
  - Last runs shows 10 rows, each with the time, a link to the One-off ticket, a one-line Result and
    a status. Skipped rows have no ticket link. The time column shows `scheduled_for`, or "Run now"
    for a run with no `scheduled_for`. The status follows this table:

    | One-off state | Status |
    | --- | --- |
    | Queued, never started | Queued |
    | Started and running | Running |
    | Finished, created more than one minute after `scheduled_for` | Ran late |
    | Finished, any other | Finished |
    | Paused, or failed | Needs answer |
    | Cancelled for any reason (replace rule, delete or a person) | Cancelled |
    | No One-off (skip log entry) | Skipped |

    A late run that is still running shows Running. "Ran late" appears only once it finished.
- **Pauses (ADR 0042).** A Scheduled run that pauses is an ordinary One-off pause. gx can already
  recover it from the web with the One-off's own buttons, and a Schedule adds no pause kind and no new
  way into one. The One-off ticket page has no web way to close a One-off today (the cancel route is
  socket-only), so this epic adds a web Cancel button for One-off tickets and the route behind it. A paused or failed run does stop later runs of its Schedule, but that is a skip, not a
  pause. gx cannot judge a failed run, and an automatic retry would bury it. The Cancel button on the One-off ticket
  page (added by this epic) ends the block, and the next set time then runs.
- **Glossary.** The terms Schedule, Scheduled run, Late limit and Schedule switch are already in
  GLOSSARY.md under "Schedules". No new term is added.

## Testing Decisions

- A good test checks external behavior: what a person sees on the page, what a `gx schedules` verb or
  server route returns, and which One-offs exist after a time passes. It does not check the scheduler's
  private state or the file layout.
- **Highest seam.** The server's `gx schedules` routes, with the real scheduler, the real One-off
  submit path and a controllable clock, tested in the server package like the existing One-off route
  tests. The timed rules (catch-up, Late limit, replace and skip, DST, two machines sharing a store,
  delete, missing project) are tested here, because a browser flow cannot move time. This is the one new
  seam. The implementer picks how the clock is injected.
- **Web.** Render and form logic (validation messages, the Save bar, day shortcuts, row states) is
  covered by vitest in `webui/src`. One Playwright flow on a new `schedules` fixture scenario covers
  the journey across pages with a real server (menu entry, list, open, edit, save, create, switch, Run
  now, delete). It adds one flow, so `maxFlows` in `playwrightguard` and `webui/e2e/CATALOG.md` go up
  by one and the ticket says why. It extends that one flow with steps rather than adding more.
- **Prior art.** Server One-off route tests and Dedupe tests, the route-table-to-CLI parity test in
  `cmd`, the `triage` and `limits` fixture scenarios with shapes, and `epics.e2e.ts` for a multi-page
  journey.

## Verification

A verify ticket runs these against a temporary gx server. Timed checks use real time: create a
Schedule whose time is two minutes ahead and wait for it, because the timer fires within about a
minute. The verb names are the implementer's, so entries name verbs by role (list, show, create, edit,
switch, run-now, delete).

1. Story 1. Flow `schedules` (new): open the Epics menu in the sidebar footer, see the separator after
   Archive and "Scheduled prompts", click it, land on `/schedules`.
2. Story 2. Flow `schedules` (new), mock state `list`. CLI: the list verb with `--json` prints each
   name, days, times and next run, exit 0.
3. Story 3. Flow `schedules` (new): the row of a Schedule turned off here reads "Off here". Mock
   state `list`.
4. Story 4. Flow `schedules` (new), mock state `list-switch-off`: turn off the last switch and see the
   banner.
5. Story 5. Flow `schedules` (new) on shape `empty`, mock state `list-empty`.
6. Story 6. Flow `schedules` (new): "New" opens the new-Schedule page, mock state
   `schedule-page-new`.
7. Story 7. Flow `schedules` (new): flip a row's switch and see "Off here" and the next run swap. CLI:
   the switch verb turns it on and off, and show reports the switch for this machine.
8. Story 8. Flow `schedules` (new): a row opens the Schedule page, mock state `schedule-page`.
9. Story 9. Flow `schedules` (new): edit every field, Save, reload the list, see the values. CLI: show
   with `--json` returns the same fields.
10. Story 10. New vitest in `webui/src`: the three shortcuts set the day pills.
11. Story 11. Flow `schedules` (new): add a second time, remove a time. Mock state `schedule-page`.
12. Story 12. Mock state `schedule-page` shows the local-time hint.
13. Story 13. Flow `schedules` (new): a new Schedule has project `scratch`. CLI: create with no project
    prints project `scratch`.
14. Story 14. CLI: create with no harness, then show prints the harness as unset and the default
    harness name. Create with `codex`, run now, and `gx tickets show` of the One-off shows codex. New
    vitest: the first harness option reads "Default (<name>)".
15. Story 15. CLI: create with no Late limit, show prints 2. New vitest: Late limit accepts a decimal
    and refuses 0 and a negative number.
16. Story 16. Mock states `schedule-page` and `schedule-page-new` show the shared prompt box.
17. Story 17. Flow `schedules` (new), mock states `schedule-page-unsaved` and `schedule-page-new`: the
    Save and Discard bar appears after an edit, Discard restores the fields, a new Schedule shows
    "Create".
18. Story 18. New vitest: each missing field shows an inline error and blocks Save.
19. Story 19. CLI: create with an empty name, empty prompt, no day, no time, and Late limit 0 each exit
    with a refusal whose `reason` names the failure and create nothing. Duplicate times collapse into
    one.
20. Story 20. CLI and server: create a Schedule whose time is two minutes ahead, switch on, wait. A
    One-off with key `schedule:<project>/<slug>@<machine>` exists within about a minute of the time.
    New Go server test with a controllable clock covers the same rule.
21. Story 21. CLI: `gx tickets show` of that One-off shows it in the `one-offs` epic of the Schedule's
    project. New Go server test: it counts against the one-off cap and not the per-root cap.
22. Story 22. CLI: `gx tickets show` of the One-off prints the person's prompt followed by the
    notify note naming the Schedule.
23. Story 23. CLI: `gx notify --status` and `gx notify "<msg>"` behave as before. New Go test: no
    server route or verb for notify was added (the route-table parity test passes).
24. Story 24. New Go server test: server stopped across a set time, restarted within the Late limit,
    one run starts. Real run: stop the temp server across a set time and restart it, one One-off
    appears.
25. Story 25. New Go server test: three set times missed, one run starts, for the latest time.
26. Story 26. New Go server test: time passes with the switch off, switch on, no run starts.
27. Story 27. New Go server test: restart past the Late limit, no run, one skip-log entry with the
    reason. Flow `schedules` (new) on shape `page`: the "Past the 2 h Late limit" skipped row in mock
    state `schedule-page`.
28. Story 28. New Go server test: a run queued on time and held by a full cap for longer than the
    Late limit is not skipped.
29. Story 29. New Go server test: a started (running) previous run makes the next timed run skip with
    "previous run started", a skip-log entry and no new One-off. A paused or failed previous run skips
    with "previous run needs attention" and the ticket link, and keeps skipping until its ticket is
    closed. Cancelling the ticket with the new Cancel button on its One-off ticket page (a web route, added
    by this epic) lets the next set time queue a run. New Go web test: the web cancel route cancels a
    queued One-off with no state token and refuses a ticket outside the `one-offs` epic.
30. Story 30. New Go server test: a queued never-started previous run is cancelled and the new run is
    queued.
31. Story 31. Flow `schedules` (new): click Run now, a Last runs row with a ticket link appears. CLI:
    the run-now verb prints the new One-off address, exit 0.
32. Story 32. CLI: run-now twice while the first run is started exits non-zero with a refusal
    `reason`, creates nothing and logs no skip. Run-now with a paused or failed previous run is refused
    with "needs attention" and the ticket link. Run-now on a Schedule whose project is gone is refused
    with `unknown-project` and logs no skip. Flow `schedules` (new): the page shows the refusal
    reason.
33. Story 33. Flow `schedules` (new): with the switch off, Run now works and the page says it does.
34. Story 34. New Go server test: with the one-off cap full, run-now queues a One-off and returns
    success.
35. Story 35. Flow `schedules` (new) on shape `page`, mock state `schedule-page`: four rows, the three
    statuses Finished, Ran late and Skipped, and links to the One-off tickets. New vitest: Needs
    answer, Running, Queued, Cancelled and the "Run now" time label render. New Go server test: each
    row of the status table maps from its One-off state (a One-off cancelled by a person shows
    Cancelled, and one created more than a minute after `scheduled_for` shows Ran late once finished),
    and only the last 10 rows come back. A
    One-off created with Run now has no `scheduled_for`, a timed one has it, and `gx tickets
    validate` accepts a One-off ticket without the field.
36. Story 36. Flow `schedules` (new) on shape `page`: the Skipped row has no link and shows its
    reason. Mock state `schedule-page`.
37. Story 37. New Go server test: two machine ids sharing one ticket store, One-offs of both appear in
    Last runs, skip rows only for this machine.
38. Story 38. New Go server test: two scheduler instances with different machine ids, both on, each
    submits its own run and neither is refused as a duplicate of the other.
39. Story 39. CLI: create a Schedule, then show it with a second state directory on the same ticket
    store. The switch reads off there.
40. Story 40. New Go server test: a Schedule whose project is gone skips each run time with "project
    not found" and sends one error per skipped run time through the `gx notify` gate. Flow
    `schedules` (new) on shape `missing-project`: the list row shows "project not found" in place of
    the next run, and the Schedule page shows it in the same place. The mock has no state for it, so
    the row follows the `list` layout and the page follows `schedule-page`.
41. Story 41. New Go server test: restart the scheduler with the skip log in place, no second error
    message for the same run time.
42. Story 42. CLI: edit the name, show prints the new name and the same slug, the old runs still
    list.
43. Story 43. CLI: create a second Schedule whose name has the same slug in the same project exits
    with a refusal and creates nothing.
44. Story 44. CLI: edit the project of an existing Schedule is refused. Flow `schedules` (new): the
    project field is read-only on an existing Schedule.
45. Story 45. New Go server test: edit the times after a time has passed, no run starts for the
    earlier time. Two scheduler instances on one store: the one that did not save, and was asleep,
    also starts no run for the earlier time, because its baseline moves to `updated_at`.
46. Story 46. Flow `schedules` (new): "Delete this schedule" shows an inline confirm that names the
    effect on queued and live runs, then removes the row.
47. Story 47. New Go server test: delete cancels a queued never-started run and leaves a started and
    a paused run alone. With two machine ids, each cancels only its own queued run. Turning the
    switch off leaves a queued run alone.
48. Story 48. Flow `schedules` (new) on shape `needs-answer`, page follows mock state `schedule-page`:
    a Needs answer row links to the One-off ticket page, which offers its existing recovery buttons and the new Cancel button. New Go server test: a Scheduled run adds no pause kind
    (the pause-to-verb table test passes).
49. Story 49. New Go server test: a time inside a spring-forward gap runs at the first valid moment
    after it.
50. Story 50. New Go server test: a time inside a fall-back repeat runs once.
51. Story 51. CLI: list, show, create, edit, switch, run-now and delete each work with `--json` and
    exit 0. The route-table-to-CLI parity test passes.
52. Story 52. New Go server test: with the usage gate closed, a Scheduled run stays queued like any
    One-off and starts when the gate opens.

New flows or scenarios to write (each becomes an implementation ticket):

- Fixture scenario `schedules` in `testutil/gxfixture`, with shapes `list` (three Schedules, two on
  here and one off here), `all-off` (all three off here), `empty` (none), `page` (the "Weather
  check" Schedule with four Last runs: Finished, Finished, Ran late and Skipped), `needs-answer` (the
  same Schedule with one paused run, shown as Needs answer) and `missing-project` (one Schedule whose
  project is gone). Run times in the seed are relative to now.
- Flow `schedules` in `webui/e2e/schedules.e2e.ts`, one journey across the shapes above. Raise
  `maxFlows` by one and add its entry to `CATALOG.md`.
- New Go server tests with a controllable clock for stories 20, 24 to 30, 32, 34, 35, 37 to 41, 45,
  47 to 50 and 52, and the web cancel route test (story 29).
- New vitest files in `webui/src` for stories 10, 14, 15, 18 and 35.
- Schema change: the optional `scheduled_for` field on the One-off ticket only (not the request), with
  the Migration decision above (a no-op named Migration step and the updated schema snapshot) and a
  validation test.
- Web Cancel for One-off tickets: make the cancel route a web write and add a Cancel button to the
  One-off ticket page (stories 29 and 48), with a vitest and a web route test.

**Mock states**

- The fixture shapes `needs-answer` and `missing-project` have no mock state of their own. They reuse
  `list` and `schedule-page`.

- `/Users/david/.local/share/gx/tickets/gx/scheduled-prompts/mocks/03-schedules-page-prototype.html`, `list`, 900 px, scenario `schedules` shape `list`, parts: page-title, new-button,
  schedule-rows, schedule-name, schedule-when, next-run, switch.
- `/Users/david/.local/share/gx/tickets/gx/scheduled-prompts/mocks/03-schedules-page-prototype.html`, `list-switch-off`, 900 px, scenario `schedules` shape `all-off`, parts: page-title,
  new-button, off-banner, schedule-rows, schedule-name, schedule-when, next-run, switch.
- `/Users/david/.local/share/gx/tickets/gx/scheduled-prompts/mocks/03-schedules-page-prototype.html`, `list-empty`, 900 px, scenario `schedules` shape `empty`, parts: page-title, new-button,
  empty-text.
- `/Users/david/.local/share/gx/tickets/gx/scheduled-prompts/mocks/03-schedules-page-prototype.html`, `schedule-page`, 900 px, scenario `schedules` shape `page`, parts: back-link, name, run-now,
  switch, row-days, days, row-times, times, row-project, row-harness, row-late-limit, row-prompt,
  prompt-box, row-last-runs, last-runs, delete-link. Known differences from the mock, where the spec
  wins: the Harness select starts with "Default (<name>)", and the Project field is read-only.
- `/Users/david/.local/share/gx/tickets/gx/scheduled-prompts/mocks/03-schedules-page-prototype.html`, `schedule-page-unsaved`, 900 px, scenario `schedules` shape `page` after one field is edited,
  parts: the parts of `schedule-page` plus save-bar. Same known differences as `schedule-page`.
- `/Users/david/.local/share/gx/tickets/gx/scheduled-prompts/mocks/03-schedules-page-prototype.html`, `schedule-page-new`, 900 px, scenario `schedules` shape `empty` at the new-Schedule route,
  parts: back-link, name, row-days, days, row-times, times, row-project, row-harness, row-late-limit,
  row-prompt, prompt-box, save-bar. Known difference from the mock, where the spec wins: the Harness
  select starts with "Default (<name>)". The Project select is editable here, as in the mock, since
  the Schedule is new.

## Out of Scope

- Cron expressions and any run-time syntax beyond days of the week and times of day.
- A new notify verb, a server notify route, banners, Web Push, or any notify path other than the
  existing `gx notify`.
- Running a Schedule on a machine the person did not switch on.
- Backfilling every missed run, or running time that passed while the switch was off.
- Moving a Schedule to another project, or changing its slug.
- A per-machine view of which machine ran a run.
- A history file for runs. The One-offs and the skip log are the history.
- A phone layout check for the page beyond the existing web shell behavior.
- New config keys.

## Further Notes

- The ticket store can be shared by two machines. Only the Schedule file is shared. The switch, the
  last-handled time and the skip log are per machine.
- A Scheduled run needs the server up. gx does not queue runs for a down server, matching ADR 0033.
  Catch-up on restart covers short outages within the Late limit.
- The notify note in the prompt is a convention for the agent. gx does not check that the agent runs
  `gx notify`, and a person who has no Telegram or Slack set up gets no message.
