## Problem Statement

On 2026-11-04 the laptop panicked three times with gx running. Two separate things made it worse, and a third made it hard to stop:

- A test binary re-ran itself without end. About 4,700 processes piled up nested under one test command and macOS stopped answering its own watchdog. gx saw nothing: its CPU watchdog only samples gx's own CPU, so a storm of many small processes does not trip it.
- After a reboot, launchd started the server, and the server resumed every saved session at once: 43 `claude` agents with 43 holders, about 14 GB, load average 509. Most of those sessions were old ones nobody was looking at.
- The only way to stop the server from coming back after each reboot was `launchctl bootout` and `launchctl disable` by hand. gx has `gx server install` but nothing that undoes it.

A person cannot trust gx to be running while they work, or to survive a reboot, because neither a reboot nor a runaway test is safe.

## Solution

Three fixes, so that neither a reboot nor a runaway test can take the laptop down through gx:

1. Server start launches no agent for a dead session. A session whose holder died ends Exited (crashed) and the person presses the existing Restore button when they want it back. Claimed ticket runs keep coming back through the run reclaim step as today. The old resume-all behaviour stays as an opt-in config key, off by default.
2. A process-count watchdog in the server watches the process trees gx launched. A tree with more than 200 descendants is frozen and killed, and the ticket that caused it is paused as `process-storm` with a notification. A storm in a tree gx did not launch gets a notification only. As a backstop, gx launches agents, test commands and herdr panes under a per-uid process cap, so a storm the poll is too slow for fails to fork instead of spreading. A lint test keeps test packages from re-running themselves.
3. `gx server uninstall` stops the server from starting at login: it boots out the launchd job, removes the plist and says what it did.

## User Stories

1. As a person who reboots their laptop, I want the server to start without launching an agent for each old session, so that a reboot does not use 14 GB and pin the CPU.
2. As a person with 40 or more dead sessions, I want none of them restarted when the server starts, so that only work I asked for runs.
3. As a person looking at the Agents tab after a restart, I want a session whose holder died to show as Exited (crashed), so that I can see it was the restart that ended it.
4. As a person who wants an old session back, I want the existing Restore button to relaunch it on its latest conversation, so that I choose which sessions cost memory.
5. As a person using `gx agents`, Telegram or a notification click, I want a dead session to show as Exited with Restore, like in the web UI, so that every surface behaves the same.
6. As a person whose session had a pending question or Unread, I want it to come back Exited with the Unread I had before the stop unchanged, so that I do not lose what the agent asked.
7. As a person with a design session that auto-advance would continue, I want it to follow the same rule as any session, so that a design session is not a way around the restart rule; a design session auto-advance starts later is a new launch.
8. As a person with a claimed ticket run whose agent is still alive when the server restarts, I want the run reattached exactly as it is today, so that ralph-loop work is not interrupted.
8a. As a person whose machine rebooted with a claimed ticket run in flight, I want gx to do what it does today for a run whose agent died: reopen the ticket when the run left nothing behind, otherwise pause it as `iteration-error`, and start no new `claude` except through a fresh claim, so that a reboot never resumes a dead agent behind my back.
9. As a person whose session holder is still alive when the server starts, I want the server to pick it up again and raise any Unread it missed, so that a server restart alone changes nothing for live sessions.
10. As a person who liked the old behaviour, I want `agents.restore-sessions-on-start` to stay as an opt-in that resumes every dead session once, so that I can get it back with one config line.
11. As a person upgrading gx, I want the changelog to tell me the default of `agents.restore-sessions-on-start` is now off, so that I am not surprised that sessions stay Exited after a restart.
12. As a person who set `agents.restore-sessions-on-start` (or its old alias `resume-on-start`) to true explicitly, I want that choice kept, so that an upgrade does not change my setup.
13. As a person whose test command forks without end, I want gx to kill that tree before the laptop slows down, so that a runaway test cannot panic the machine.
14. As a person whose ralph-loop iteration caused a storm, I want its ticket paused as `process-storm` with a reason that says what happened, so that gx does not retry into the same storm.
15. As a person looking at a `process-storm` pause in the web UI, I want a card that says gx cannot recover alone because a retry would storm again, and Retry and Start over buttons, so that I can run the work again or throw away the commits that caused the storm without a CLI command (ADR 0042).
16. As a person away from the screen, I want one notification per kill that names the ticket and the process count, so that I know what was killed and why.
17. As a person running an interactive agent session that forks out of control, I want the same tree kill, with a notification and no ticket pause (a session has no ticket run), so that it cannot take the laptop down either.
18. As a person who runs a legitimate build with many processes outside gx, I want gx to kill nothing there and only tell me, so that gx never kills my own work.
19. As a person who tunes the watchdog, I want `watchdog.max-tree-processes`, `watchdog.interval` and `watchdog.ulimit-headroom` in the config, so that I can change the 200-process limit, the 1-second poll and the 1,000-process cap headroom.
20. As a person whose storm grows faster than the poll, I want agents, test commands and herdr panes launched under `ulimit -u` of the uid's process count plus 1,000, so that the fork fails with "Resource temporarily unavailable" instead of spreading.
21. As a person with a long-lived agent, I want to know that its cap is fixed when it launches, so that if legitimate forks start failing I know a restart of the agent fixes it.
22. As a gx developer, I want a test to fail when a package that imports `servertest` has no `TestMain` calling `servertest.RunSessionProcesses()` and `herdrfake.RunHelperProcess()`, so that a test binary that re-runs itself cannot be added again.
23. As a person who wants gx's server off at login, I want `gx server uninstall` to stop it, so that I need no `launchctl` commands.
24. As a person running `gx server uninstall` while the server runs, I want the server stopped and the output to say so, so that I know it is no longer running.
25. As a person running `gx server uninstall`, I want it to print the plist path it removed and whether it stopped a server, so that I can see what changed.
26. As a person running `gx server uninstall`, I want the output to say that agent holders keep running, so that I do not think my agents were stopped.
27. As a person who never installed autostart, I want `gx server uninstall` to print "not installed" and exit 0, so that I can run it safely in a script.
28. As a person on Linux, I want `gx server uninstall` to refuse with the same launchd-only message as `gx server install`, so that I am not told it worked when it did nothing.
29. As a person who uninstalled and changed my mind, I want `gx server install` to bring autostart back with no extra step, so that undoing it is one command.

## Implementation Decisions

### Server start (fix 1)

- The module is the session resumer's server-start step. It handles the interactive sessions in the sessions directory only (plain, design, triage and fix sessions). Claimed ticket runs are not sessions of that directory: they are handled by the separate run reclaim step, which reattaches live agents and never resumes a dead one into a new `claude`. The map described the exception as "sessions whose meta has a Run"; that field counts a session's resumes and does not mark a ticket run, so no exception is needed in the session step. The reclaim step is unchanged.
- With `agents.restore-sessions-on-start` off, the start step does, in order for each session: skip an Exited one; re-watch a live holder (as today, including the editor-override note and the Unread diff); record the exit of a holder that left an exit code or of a stopped session (as today); end any other dead session Exited (crashed) through the existing failed-start path. It launches no holder and no `claude`. The 100 ms spacing and the resume pass do nothing in this mode.
- With the key on, the old behaviour is unchanged: each dead session is resumed once, spaced out, with its existing refusals (dir gone, unknown harness, no resume, missing binary) ending it Exited. No cap is added.
- A session with a pending question or Unread ends Exited like any other; its saved Unread is not changed by the start. No Dormant state, no lazy resume on open, no keep-alive mark.
- Design sessions follow the same rule as every session.
- Config: the default of `agents.restore-sessions-on-start` changes from true to false in the config defaults, the settings reload watcher keeps working, and the key description in the config schema is rewritten ("Resume every interactive session that was running when the server last stopped. Default: false."). The hidden `resume-on-start` alias keeps working with the same default. The repo has no mechanism that migrates a changed config default (config migrations only move files), and the key is read only when present, so a person who set it explicitly keeps their value. The change is a changelog note, not a migration step.
- GLOSSARY.md: the Restore entry no longer says gx restores sessions on server start by default. It says a person Restores by hand, and that `agents.restore-sessions-on-start` (off by default) makes gx restore the sessions that were live once on server start.
- Code comments and docs that say the start step "auto-resumes" dead sessions are updated to match.

### Watchdog (fix 2)

- A new server watchdog package, shaped like the CPU watchdog (a poller started with the server, config read live from settings, reporting into the server). Its poll interval is `watchdog.interval` (default 1 second).
- Which trees it covers: the process tree of every gx-launched agent: a ralph-loop iteration agent and the test commands it runs, and an interactive session's agent. A process belongs to a tree when its ancestry reaches the tree's root process (the agent's or the iteration's). The implementer picks the lookup method and how the root is found for each runner (PTY, headless, herdr pane).
- Threshold: a gx-launched tree with more than `watchdog.max-tree-processes` descendants (default 200) is a storm.
- Kill: freeze the whole tree with SIGSTOP so nothing can fork, then SIGKILL every process in the snapshot, repeating until the tree is empty.
- After a kill the watchdog tells the iteration's owner that the tree was killed for a storm, and the ticket is paused. A session without a ticket run is killed and notified only.
- Pause: the ticket is paused with a new `pause_reason` `process-storm`, class repair, with a plain-words label in the label table and a row in the pause-to-verb table whose verb is Retry and Start over, the same row as `budget-killed` (the existing web buttons). gx cannot recover alone because an automatic retry would run the same code and storm again; the person chooses Retry, or Start over to throw away the agent's own commits when they caused the storm, and either ends the pause (ADR 0042). The watchdog marks the run as storm-killed before it kills the tree, so the runner writes `process-storm` and not the agent-died or `iteration-error` pause it would otherwise write for the killed agent. The pause detail names the process count and the threshold. Automatic recovery does not start for a `process-storm` pause: it is left out of the recovery trigger in recovery/trigger.go the same way `budget-killed` is, so only the person's Retry or Start over ends it. The pause-to-verb test that fails when a kind has no web verb must pass.
- Schema: the new enum value needs a new named step in the migrations list and the schema snapshot updated, as the repo rules require; a released step name is never reused.
- Notification: one notification per kill, naming the ticket (or session) and the process count.
- A tree gx did not launch: any other process tree of the uid with more than the same threshold of descendants gets one notification per storm and nothing is killed. The notification says gx did not start it and did not touch it. The root of such a tree is a process of the uid whose parent is not owned by the uid (for example launchd). Processes inside gx-launched trees are left out of its count, so a terminal or tmux server that holds a gx tree does not trigger a second notice. One notification is sent per root while it stays over the threshold. The same root counts as a new storm only after it drops back under the threshold.
- Backstop: agents, ralph-loop test commands and herdr pane commands launch under `ulimit -u` of the uid's process count at launch plus `watchdog.ulimit-headroom` (default 1,000). gx's own runners (PTY, headless) set it at launch; for herdr panes gx puts it in the pane command. The cap is fixed at launch, so a long-lived agent keeps its launch-time baseline; the docs say a restart of the agent fixes legitimate forks that start failing. The cap bounds a storm the poll is too slow for; the watchdog stays the main mechanism.
- Config: a `watchdog` block with `max-tree-processes` (200), `interval` (1s) and `ulimit-headroom` (1000), added to the config loader, defaults and config schema. The existing `cpu-watchdog` block is unchanged.
- Lint test: the existing repo-wide guard that every package whose tests import `servertest` has a `TestMain` with both hooks stays and is kept in the test run (it is its own ticket and covers the repo; the runtime guards in the harness already exist). If a package is found without hooks, adding the hooks is part of the ticket.

### `gx server uninstall` (fix 3)

- A new `uninstall` verb under `gx server`, next to `install`. No enable/disable-autostart verbs.
- It runs `launchctl bootout` (which stops the running server), then removes the plist, then prints the plist path removed and whether a server was stopped. It also prints that agent holders keep running.
- It does not run `launchctl disable`: with the plist gone nothing starts at login, and a stale disabled flag would make a later `install` fail.
- A missing plist, or a job that is not loaded, is not an error: it prints "not installed" and exits 0. A job that is loaded but whose plist is already gone still gets booted out.
- On Linux it refuses with the same launchd-only message as `install`.
- `install` is unchanged. The service-manager seam gains an uninstall operation that the launchd agent implements.

## Testing Decisions

- A good test drives external behavior: the session states a restart leaves, what the Agents tab shows, which processes exist, which pause a ticket is in, and the verb's output and exit code. It does not assert call order inside the resumer or the watchdog's internals.
- Seam 1, server start: the server-level session tests that stop a server, kill holders and start a new one. Check a dead session ends Exited (crashed) and no holder or `claude` starts with the key off; the opt-in resumes once with it on; a live holder is re-watched with its missed Unread raised; a pending-question session keeps its saved Unread; a claimed ticket run with a live agent is still reclaimed, and one whose agent died is reopened or paused `iteration-error` with no new `claude`. Prior art: the existing session restore and auto-resume tests and the reclaim tests.
- Seam 2, watchdog: run the watchdog against a fake agent that spawns more descendants than a low test threshold. Check the whole tree is killed, the ticket is paused `process-storm` with a notification, a tree not launched by gx gets a notification only, and a tree under the threshold is left alone. Real child processes are used so the freeze-then-kill loop runs for real. Prior art: the CPU watchdog tests and the pause tests. The `ulimit` backstop is checked by asserting the launch command or environment of each runner carries the cap, plus one real child that hits "Resource temporarily unavailable".
- Seam 3, `gx server uninstall`: the launchd agent tests with the fake `launchctl` runner. Check the commands run, the plist removed, the output, "not installed" with exit 0, and the Linux refusal. Prior art: the existing `install` tests.
- Seam 4, lint: the existing guard test over the repo's tracked test files, with its table test.
- Schema and config: the schema snapshot guard test and the config schema tests cover the new enum value and keys.

## Verification

Run against a temporary gx server.

- Story 1, 2, 3: start a server on a fixture with several saved sessions whose holders are dead; check with `gx agents` that none has a holder or `claude` process, and all show Exited (crashed). Flows: the agents-sidebar and agents-session flows from the catalog. New scenario to write: a fixture with 40 or more dead sessions and a count of `claude` processes after start (an implementation ticket).
- Story 4: press Restore on one of the Exited sessions in the web UI; the session runs on its latest conversation. Flow: the existing restore flow in the agents flows.
- Story 5: `gx agents` shows the dead session as Exited; the other surfaces use the same session state.
- Story 6: a fixture session with a pending question and Unread, dead at stop: after start it is Exited and its Unread is unchanged. New scenario to write.
- Story 7: a dead design session after start is Exited; opening the design page shows Restore.
- Story 8: a claimed ticket run with a live agent before a restart is reattached after it (the restart-run flow).
- Story 8a: kill a claimed run's agent and the server, start the server, and check the ticket is reopened when the run left nothing behind and paused `iteration-error` otherwise, and that no `claude` starts outside a fresh claim. New scenario to write in the fixture: a claimed run whose agent died with the server.
- Story 9: restart the server with a live holder: the session keeps running and stays attached (the restart flow).
- Story 10, 12: set `agents.restore-sessions-on-start` true (and, separately, the `resume-on-start` alias) and restart: dead sessions are resumed once.
- Story 11: the changelog entry for the default change exists.
- Story 13, 14, 16: a fixture iteration whose fake test command forks past a low `watchdog.max-tree-processes`; the tree is killed within a few polls, the ticket is paused with reason `process-storm`, one notification is sent, and the process count returns to baseline. New scenario to write: a fork-storm fake agent in the fixture.
- Story 14, 15: after a `process-storm` pause, no recovery run and no investigate run starts for the ticket.
- Story 15: the pause flow shows the `process-storm` card with the reason text and Retry and Start over buttons; pressing either ends the pause (the pause flow).
- Story 17: a fork-storm interactive session is killed and notified, with no ticket pause. New scenario to write.
- Story 18: a process tree started outside gx with more descendants than the threshold is left alive and notified once.
- Story 19: set the three `watchdog` keys in the config; a reload changes the behaviour (the settings reload flow); the keys appear in `docs/config-schema.json`.
- Story 20, 21: check the launch command of an agent, a test command and a herdr pane carries the cap; a fork past the cap fails with "Resource temporarily unavailable".
- Story 22: `go test ./testmainguard` passes, and fails when a servertest importer lacks the hooks (the table test).
- Story 23, 24, 25, 26: `gx server install`, then `gx server uninstall`: exit 0, output names the plist path, says a server was stopped and that agent holders keep running; `launchctl print gui/<uid>/com.elentok.gx.server` no longer finds the job; the plist is gone.
- Story 27: `gx server uninstall` twice: the second prints "not installed" and exits 0.
- Story 28: on Linux, `gx server uninstall` exits non-zero with the launchd-only message of `install`.
- Story 29: `gx server install` after an uninstall succeeds and starts the job.

## Out of Scope

- Linux systemd install and uninstall. `gx server install` supports only launchd today, and `uninstall` mirrors it. systemd support, for both verbs, gets its own epic later.
- Any Dormant state, lazy resume when a session is opened, resume cap or keep-alive mark for sessions.
- `gx server enable-autostart` and `disable-autostart`, and `launchctl disable`.
- Killing process trees that gx did not launch.
- Changing how claimed ticket runs are reclaimed after a restart.
- Changing `gx server install`.

## Further Notes

- Evidence for the storm: the opendirectoryd stackshot of 2026-11-04 showed 4,658 `settings.test` processes nested parent to child about 2 minutes before the panic; a `ulimit -u` cap at the current count plus 200 stopped a test fork loop with "fork: Resource temporarily unavailable". `scripts/proc-cap` is the manual form of the backstop for test runs.
- The cap counts every process of the uid, so it is the count at launch plus headroom, not a fixed number.
- Source tickets: gx:follow-ups/295, 296 and 297; see the map's tickets 01, 02 and 03 for the full Q&A.
