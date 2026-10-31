## Problem Statement

To restart the gx server, a person must open a terminal and run `gx server restart`. That is a problem when they work from the web UI, often from a phone. They also cannot see which version they run now and which version will start. They cannot tell whether the restart will fail until it does. A failed `--migrate` leaves the server stopped, and then the web UI is gone too.

## Solution

The top-right menu gets a "Restart server" item, after AFK and above the version text. It opens a confirmation dialog. When the dialog opens, the new binary checks its config and does a full migration dry run. The dialog shows the running version, the version that will start, and how many agents are running. It says when the two versions match.

If the new binary cannot run or its config check fails, Restart is disabled and the dialog shows the error. If the dry run fails, the button reads "Restart anyway".

The restart runs the ticket store migration between stop and start. A failed migration does not stop the restart. The server starts, and a strip on every web page names each failed project and its error. Each failed project has a "Fix agent" button.

The tab that clicked Restart reloads by itself when the server returns. Other tabs show a "New version, reload" strip.

## User Stories

1. As a person using the web UI, I want a "Restart server" item in the top-right menu, so that I can restart the server without a terminal.
2. As a person, I want the item after AFK and above the version text, so that I find it in a stable place.
3. As a person, I want to reach the item with the keyboard like the other menu items, so that the menu stays consistent.
4. As a person, I want a confirmation dialog before anything restarts, so that I never restart by accident.
5. As a person, I want the dialog to show "Checking..." with Restart disabled while the checks run, so that I do not restart before I know the result.
6. As a person, I want the dialog to show the running version, so that I know what I run now.
7. As a person, I want the dialog to show the version that will start, read from the binary the restart will actually start, so that I know what I will get.
8. As a person, I want the dialog to say when both versions match, so that I know a restart changes nothing but still reloads config.
9. As a person, I want Restart to stay enabled when the versions match, so that I can apply startup-only config changes.
10. As a person who installed gx to a different path, I want the dialog to show no change, so that it does not claim a version the server will not start.
11. As a person, I want Restart disabled and the error shown when the binary is missing or fails to run, so that the running server is not stopped for nothing.
12. As a person, I want the current server to keep running when the new version cannot be read, so that a bad install does not take gx down.
13. As a person, I want the new binary to check config before the old server stops, so that a bad config cannot leave me without a server.
14. As a person, I want Restart disabled when that config check fails, with the error shown, so that I fix the config first.
15. As a person who downgraded the binary, I want Restart disabled when the binary is too old to run the checks, with the dialog naming `gx server restart` for terminal use, so that I know the way out.
16. As a person, I want the dialog to show how many agents run and say they keep running, so that I am not afraid to restart.
17. As a person, I want to restart while agents work, so that I never wait for a quiet moment.
18. As a person, I want a warning that a migration runs while agents are active, so that I understand the risk.
19. As a person, I want a migration dry run when the dialog opens, so that I learn about migration problems before the restart.
20. As a person, I want the dry run to run every migration step in every project, chained in memory as the real run does, so that the check shows what the migration would do at that moment. A later failure is still possible, and the strip shows it.
21. As a person, I want a failed dry run to show its error and turn the button into "Restart anyway", so that I can still restart on purpose.
22. As a person, I want the migration to take each ticket's lock and, under the lock, re-run that ticket's migration on the fresh contents and validate it before it writes, so that no agent write is lost.
23. As a person, I want a failed migration in one project not to stop the restart, so that the server always comes back.
24. As a person, I want the other projects to still be migrated when one fails, so that one bad project does not block the rest.
25. As a person, I want a strip on every web page that names each failed project and its error, with the command to retry, so that I see the failure on any page.
26. As a person, I want the strip to stay until a later migration of that project passes, so that I do not lose the failure when I close the tab.
27. As a person, I want a "Fix agent" button on each failed project, so that I can have an agent repair the migration from the web.
28. As a person, I want that button to open the existing Fix agent dialog with Copy and Start, so that it works like every other Fix agent.
29. As a person, I want the Fix agent to start in the project's trunk checkout on the strongest tier, with the project name and migration error in its prompt, so that it can fix the cause.
30. As a person, I want the Fix agent to appear in the Agents tab, so that I can follow it.
31. As a person, I want the dialog to stay open as "Restarting..." after I click, so that I see progress.
32. As a person, I want the tab that clicked Restart to reload by itself when the server returns, so that I do not reload by hand.
33. As a person with other tabs open, I want a "New version, reload" strip with a button, so that I do not lose unsent text.
34. As a person, I want the dialog to say after 60 seconds that the server did not return and may still be migrating, naming the restart log (which shows migration progress) and `gx server start`, so that I know how to recover without mistaking a slow migration for a failure.
35. As a person, I want the tab to keep checking after that message and reload if the server returns, so that a slow start still works.
36. As a person, I want agents that were running to be taken back after the restart, so that no work is lost.
37. As a person, I want the dialog to re-check when I click and to refuse with new values if the new version, the config pass or fail, the dry run pass or fail or the set of failing projects changed while the dialog was open, so that I restart into what I confirmed. Changes to the list of files the migration would touch do not count, because agents keep writing.
38. As a person, I want any logged-in web session to be able to restart, under the existing login and request checks, so that no new permission model appears.
39. As a terminal user, I want `gx server restart` and `gx server restart --migrate` to keep their stop-on-failure behaviour, exit codes and output, so that my scripts keep working. The locked write and the clearing of saved failures apply to every migration on purpose.
40. As a person who installed the launchd agent, I want the dialog to read the version from the binary the agent will start, so that the version I confirm is the version that runs.
41. As a person with two tabs open, I want a second restart request to be refused while one runs, with the second tab also showing "Restarting...", so that two restarts never overlap.
42. As a person, I want a passing migration of a project from any entry point (web restart, `gx tickets migrate`, `gx server restart --migrate`) to clear that project's saved failure, and the running server to update the strip live, so that the Fix agent's repair ends the failure without a restart.

## Implementation Decisions

- The server gets a restart request and a restart check request. Both sit behind the existing web login and request checks. Both follow the rule that the server API mirrors the CLI.
- The check runs the binary that the restart will start. That is the binary in the launchd agent when the agent is installed, else the binary at the server's own path. It reports the new version, the config check result and the full dry run result. The implementer picks the request names, the response shape and the way the binary reports its results.
- The new version comes from running that binary's own version command. A missing binary or a binary that fails to run is an unreadable version.
- A binary too old to run the checks is detected from how the check fails. The dialog treats it as a separate case with its own text. The implementer picks the detection method.
- The config check loads and validates the config exactly as `gx server` startup does, and exits non-zero with the startup error text.
- The dry run is extended to run every migration step in every project, chained in memory because each step reads the previous step's output. Today it skips the full ticket rewrite and its validation. The extended dry run must not write anything. It cannot promise the real migration passes, because agents write after the check and later steps read git history and session files. It still works one project at a time on the command line, and the check runs it for all projects.
- Every migration step that writes a ticket takes the ticket's lock, re-runs that ticket's migration on the fresh contents under the lock, and validates the result again before it writes. This is a fix to the migration write itself, so it also protects the CLI migration.
- The restart request starts a detached `gx server restart` from the binary it checked, in its own session, so it survives the server's exit. It uses a new mode that runs the migration, continues after a failed migration, and refreshes the skill bundle like the CLI does. The implementer picks the name of the mode.
- The existing `gx server restart` and its `--migrate` flag stay as they are. A failed migration there still leaves the server stopped.
- Before it stops the old server, the restart re-checks the binary. If the new version, the config pass or fail, the dry run pass or fail or the set of failing projects changed since the dialog showed them, the restart request is refused and the response carries the new values. The dialog then shows them and waits for a new click. Neither the list of files the migration would change nor a changed error text alone causes a refusal, because agent writes change both.
- The server refuses a restart request while one is in progress. The tab whose request was refused shows "Restarting..." and reloads like the tab that clicked. Other tabs follow story 33. Once the old server is down, a second click gets a network error. The dialog treats a network error after the click as "Restarting...", with the same instance-id reload and 60 second timer.
- The detached restart writes a log to a fixed file in the state directory, next to the server's stderr file. The check and restart responses carry its path, so the tab has it before the server goes down.
- Steps that read several tickets take the locks in address order. A ticket-creation step takes the new ticket's lock. A lock wait uses the existing lock timeout, and a timeout fails that project, which the strip reports.
- A passing dry run never clears a saved failure. The server drops saved failures for projects that are no longer configured. The dialog then shows them and waits for a new click.
- A failed migration is not a pause kind and not a new way into an existing pause. ADR 0042 does not apply to it. It is a reported failure per project. The "Fix agent" button on the strip ends it, because the agent repairs the project and runs the migration again. The strip clears when a later migration of that project passes, whichever way it ran. Every passing migration of a project, from any entry point, clears that project's saved failure. A failed CLI migration leaves the saved record unchanged. The running server notices a change to the record and pushes it through the live state, so no restart is needed.
- The migration result must survive the server restart, because the old server exits before the migration finishes. The new server reads the saved failures and serves them to the web UI. The implementer picks where and how they are saved.
- A server that does not come back is also not a pause. gx cannot recover there, because gx itself is down. The web UI cannot end it, so the dialog names `gx server start` and the log path. This is the one case where a terminal is unavoidable. There is no automatic rollback.
- The Fix agent for a failed project runs the same `/gx-investigate` skill as the existing Fix agent. The retry command is `gx tickets migrate`, run in the project's trunk checkout. The strip shows that same command and the prompt holds it. The refusal for a second Fix agent on the same address has the reason `name-taken`, and its message becomes "an agent is already fixing <addr or project>" (today it has a word missing).
- The Fix agent for a failed project reuses the Fix agent route and dialog. The glossary term already covers a project with a failed migration. The route accepts a project as well as a ticket. The session starts in the project's trunk checkout on the strongest tier. Its prompt holds the project name and the migration error. There is one Fix agent per project.
- The strip for failed projects is shown on every web page. The implementer picks how it reaches each page, for example through the live state the pages already read.
- The "New version, reload" strip appears in tabs that did not click Restart. The server reports a per-start instance id, such as its start time. The tab that clicked Restart reloads when the id differs from the one it saw at the click, so a restart to the same version works and a poll that hits the old server does not reload early. Other tabs show the strip only when the build differs. The implementer picks the id's form.
- The tab that clicked Restart reloads on its own. The server is down during the migration, so the dialog cannot show a migrating phase. The 60 second message is one timer from the click, and it says the server may still be migrating. After it, the tab keeps checking the server and reloads if the server returns.
- The dialog opens the checks when it mounts. The dialog states are Checking, Ready, Restart anyway, Disabled with an error, Restarting, and Did not return. The wording follows the map decisions. The implementer picks any wording that the map does not give.
- The menu item sits after AFK and above the version text. It follows the menu's keyboard behavior and works at phone width.
- The agent count in the dialog is the number of running agents at the time of the check. Restart is allowed at any count, including zero.
- Interactive sessions and Queue agents keep the behavior they already have across a server restart. Reclaim takes back running iterations. Restore of interactive sessions is unchanged.

## Testing Decisions

- A good test drives the behavior a person or script can see. It checks the dialog, the strip, the HTTP responses, the exit codes and the state of the real server after a restart. It does not check which internal function ran.
- The highest seam is the web e2e suite on the gxfixture server. It reuses the existing scenarios, the login cookie and the flow catalog. The new flows need a fixture whose server path points at a controllable fake `gx` binary, so that tests can set the version, the check result, the dry run result and the migration result. This is one new scenario family, not a new seam. The restart flow also reloads the tab, and the suite fails on any page reload today. The flow needs an explicit allowance for that one reload. The implementer picks how to express it.
- The second seam is the Go server test harness. It tests the restart check and restart requests with the same web client and fakes that the other server tests use. It covers the refusal when the binary changed, the refusal of an overlapping restart, the saved failure report, the one-Fix-agent-per-project rule and the check results.
- The third seam is the Go lifecycle e2e test that drives the real binary. It already covers stop, start and reclaim. A new case there covers a web restart end to end, including a failed migration that still starts the server and the survival of the detached restart.
- Migration and dry run are tested at the command level, next to the existing migrate tests. The cases are the dry run that runs every step and writes nothing, and the lock, re-run and validation before a write, with a concurrent writer.
- The `gx server restart` command keeps its existing tests. One case confirms that `--migrate` still leaves the server stopped on failure.
- Prior art includes the Fix agent flows in the catalog, the menu flows, the disk banner flows with their held fakes, the server lifecycle e2e test and the restart migrate tests.

## Verification

Flow numbers refer to the web e2e flow catalog. "New flow" items become implementation tickets. All new flows run on a new scenario whose server path points at a fake `gx` binary, as described under Testing Decisions. The implementer picks the scenario names.

1. Run flow 28 and flow 31 for the menu. New flow "Restart server in the menu" checks that the item reads "Restart server" and sits after AFK and above the build text.
2. Flow 31 covers the keyboard for AFK. New flow "Restart server keyboard" opens the item with ArrowDown and Enter and closes the dialog with Esc.
3. Same new flow as story 2.
4. New flow "Restart dialog confirms" checks that the dialog opens first and nothing restarts until Restart is clicked. After Esc, `gx server status` shows the same start time.
5. New flow "Restart dialog checks" with a slow fake check. The dialog reads "Checking..." with Restart disabled, then enables it.
6. Same new flow as story 5, and it checks the running version text against the build text in the menu.
7. Same new flow as story 5, and it checks the new version text from the fake binary.
8. New flow "Restart dialog, same version". The dialog says the versions match and Restart is enabled.
9. Same new flow as story 8, and the restart completes.
10. New flow "Restart dialog, other install path". The fake binary at a different path leaves the dialog showing no change.
11. New flow "Restart dialog, unreadable version" with a missing binary and one that fails to run. Restart is disabled, the error shows, and `gx server status` reports the same server still running.
12. Same new flow as story 11.
13. New flow "Restart dialog, bad config". The fake check fails the config. Restart is disabled and the old server still runs.
14. Same new flow as story 13.
15. New flow "Restart dialog, old binary". The fake binary has no checks. Restart is disabled and the dialog names `gx server restart`.
16. New flow "Restart dialog, running agents". Run flow 14's scenario setup or the `agents` scenario so that agents run. The dialog shows the count and says agents keep running.
17. Same new flow as story 16, and the restart is clicked with agents running.
18. Same new flow as story 16, and the dialog shows the migration warning.
19. New flow "Restart dialog, dry run". The fake dry run fails. The dialog shows the error.
20. Server test and CLI check. `gx tickets migrate --dry-run` in a project whose full rewrite would fail exits non-zero and writes nothing. Compare the ticket files before and after. A second case has step N fail only on step N-1's in-memory output, with ticket files unchanged.
21. Same new flow as story 19, and the button reads "Restart anyway". Clicking it restarts.
22. CLI and server check. A migration test holds a ticket lock and writes from a second writer. The write is kept after the migration. A second case has fresh contents that fail validation: the project fails and the file stays unchanged.
23. New flow "Restart with a failed migration". One project fails to migrate. The server starts and `gx server status` reports it running.
24. Same new flow as story 23, with a second project that migrates. Its tickets show the migrated shape.
25. Same new flow as story 23. On two different pages, the strip names the failed project, its error and the retry command.
26. Same new flow as story 23, then fix the project and run `gx tickets migrate` in it. The strip is gone. A second case restarts again with a passing migration and the strip is gone.
27. Run flow 55 for the existing Fix agent. New flow "Fix agent for a failed migration" clicks the button on the strip.
28. Same new flow as story 27. The dialog shows the prompt and Copy, and Start works.
29. Same new flow as story 27. `gx agents list` shows a session with dir the project's trunk checkout, the strongest tier, and a prompt that holds the project name and the error.
30. Same new flow as story 27. The Agents tab lists the session. A second start is refused with reason `name-taken` and shows the "already fixing" toast.
31. New flow "Restart server, tab reloads". After the click the dialog reads "Restarting...".
32. Same new flow as story 31. The tab reloads by itself and shows the new build text. This flow is allowed one reload.
33. New flow "Restart server, other tab". A second page on the same server shows the "New version, reload" strip with a button. Its unsent composer text is still there before the button is clicked.
34. New flow "Restart server, server does not return". The fake start never brings the server up. After 60 seconds (the flow uses a shortened wait if the implementer adds one) the dialog says the server did not return and may still be migrating, and names the restart log and `gx server start`. The shown log path equals the path the responses carry.
35. Same new flow as story 34, then start the server by hand. The tab reloads by itself.
36. Go lifecycle e2e case. Start a ticket with a running agent, restart through the web request, and check that the iteration is reclaimed and lands.
37. New flow "Restart server, binary changed". Change the fake binary while the dialog is open and click Restart. The dialog shows the new values and the server did not restart.
38. Server test. A request without a login is refused. A logged-in request passes. Run flow 102 for the login itself.
39. CLI checks. `gx server restart` and `gx server restart --migrate` print the same output as before. With a failing migration, `--migrate` exits non-zero and the server stays stopped.
40. Server test with a fake launchd agent whose binary differs from the server's own path. The check reports the agent binary's version. New flow "Restart dialog, launchd binary" shows that version in the dialog. A Go lifecycle e2e case restarts through the launchd agent with a slow fake migration. It checks that the detached restart survives the stop, that no second server starts before the migration ends, and that the server comes back.
41. Server test. A second restart request during a restart is refused. New flow "Restart server, two tabs" shows "Restarting..." in the refused tab, and a second click after the old server is down also shows "Restarting...".
42. Same new flow as story 26, plus a server test that a passing `gx tickets migrate` clears the saved failure and the live state drops it without a restart, and that a failed CLI migration leaves the record unchanged. A dry run does not clear it, and a failure for a project removed from config is dropped.

## Out of Scope

- Downloading or installing a new binary from the web UI. The restart starts the binary already at the server's path.
- Any change to the TUI.
- Any change to the behaviour, exit codes or output of `gx server restart` on the command line, including `--migrate`. A failed migration there still leaves the server stopped. The locked write (story 22) and the clearing of saved failures (story 42) apply to every migration on purpose. The web restart uses a separate mode.
- Automatic rollback when the new server does not start.

## Further Notes

- When both versions match, Restart stays enabled on purpose. Some config changes apply only at startup.
- The dry run only warns. It never blocks the restart, because a person may know better. The config check and an unreadable version do block it, because the restart would leave no server.
- The launchd agent has a fixed binary path and a clean exit stays stopped. When the agent is installed, the detached restart starts the server through it, which starts the agent's binary. The check reads the version from that same binary.
