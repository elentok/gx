# UI features: Auto-queue, Cleanup section, Tips page

Source: map `gx:ui-features` (tickets 01, 02 and 03, all resolved). Delivered as one implementation
epic, `ui-features-impl`, with one chain of tickets per feature (Auto-queue, Cleanup, Tips). The
three chains share no code, so they can run side by side. Each chain ends with its own tests; the
epic's verify ticket runs the Verification section below.

## Problem Statement

1. **Queuing a finished design is a manual step.** When a Design session finishes and its
   implementation epic (`<map>-impl`) is ready, the person must go and press Queue by hand. The
   Design page's Auto menu already shows an "Auto-queue" row, but it is disabled ("Coming soon") and
   nothing on the server stores or acts on it.
2. **Cleaning up leftovers needs an agent.** Leftover worktrees, merged branches and done epics are
   removed only by `/gx-cleanup`, which the person must start by hand and which spends an agent and
   a confirm round on cases a button could decide. The scan behind it also misses archived epics,
   misses cherry-picked commits (it checks only ancestry against trunk), skips detached worktrees
   and flags the standing `one-offs` epic as done but unmerged.
3. **gx has nowhere to give machine setup advice.** gx creates and deletes huge numbers of files (a
   worktree per iteration, `node_modules` per worktree, test temp dirs, Go build output) and macOS
   Spotlight indexes all of it, slowing the agents' tests and the server. The fix is a one-time
   setting the person makes by hand, and each person finds it only after the machine slows down.

## Solution

1. **Auto-queue** is a per-session switch on a Design session, off by default. With it on, when the
   Design reaches Branch (the `<map>-impl` epic exists), the Design page shows "Queuing <epic> in Ns"
   with a Cancel. When the countdown ends the server queues the epic, or skips with a banner naming
   why it could not.
2. **A Cleanup page** in the web UI scans a project and lists what can be removed. Buttons act on
   the safe cases (archive an epic, remove a worktree, delete a branch). One agent button hands the
   cases a button cannot decide to a one-off that runs `/gx-cleanup`.
3. **A Tips page** in the web UI lists static machine setup advice. This epic ships one tip:
   exclude the worktree folders from Spotlight, with the System Settings steps and the real folder
   of each registered project.

## User Stories

### Auto-queue

1. As a person running a Design, I want an Auto-queue switch on the Design session, so that the
   implementation epic is queued without me coming back to press Queue.
2. As a person, I want Auto-queue off by default, so that nothing is queued until I ask for it.
3. As a person on the Design page, I want the Auto menu's Auto-queue row to be enabled and show the
   session's real state, so that I can see and flip the switch.
4. As a person on a phone, I want the Auto-queue row in the phone footer's Auto menu (short name
   "Queue"), so that I can flip it away from my desk.
5. As a person, I want a countdown "Queuing <epic> in Ns" with a Cancel when the design reaches
   Branch, so that I can stop the queue if I want to review the epic first.
6. As a person on a phone, I want the countdown on its own strip above the footer, like
   auto-advance's, so that Cancel is a full-size tap target.
7. As a person, I want Cancel to stop only the Auto-queue countdown, so that cancelling one does not
   cancel an Auto-advance countdown running at the same time, and the reverse.
8. As a person, I want the countdown to use the same delay as auto-advance, so that I learn one
   timing and there is no new setting.
9. As a person, I want the epic queued as soon as the countdown ends, so that implementation starts
   without another step.
10. As a person, I want the server to run the same readiness check when the countdown ends that it
    ran when the countdown started (the Design is at Branch and not held, and the epic is open, not a
    follow-ups epic, not queued and never run), so that a draft, a changed or a hand-queued epic is
    never queued behind my back.
11. As a person, I want a banner with the reason when the re-check skips the queue, so that I know
    the epic is waiting for me and why.
12. As a person, I want the switch accepted for any Design session even when its epic would be
    refused later, so that the switch is never refused for a reason that only matters at the end.
13. As a person, I want switching Auto-queue on while the epic is ready (it passes the readiness
    check of story 10) to start the countdown at once, and
    otherwise to start nothing, so that I do not have to wait for another event and never see a
    pointless countdown.
14. As a person, I want switching Auto-queue off to cancel a running countdown and show a banner, so
    that off always means nothing will be queued.
15. As a person, I want the switch to survive a server restart, so that I do not set it again.
16. As a person, I want a running countdown dropped by a restart, with me queuing by hand from the
    Design page, so that a restart never queues something later on its own (same as auto-advance).
17. As a person, I want an epic with `base:` or one blocked by another epic queued as usual, so that
    Auto-queue does not special-case those and the queue's own rules decide when it runs.
18. As a person, I want the switch refused for sessions that are not Design sessions, so that it
    never silently does nothing.
19. As a person using the TUI, I want the Agents tab actions menu to have the same Auto-queue switch
    as it has for auto-advance, so that I can flip it from the terminal UI.
20. As a person using the TUI, I want the actions menu to offer a Cancel for a running Auto-queue
    countdown, so that I can stop it there.
21. As a script or agent, I want a `gx` verb to set and to cancel Auto-queue for a session, so that
    the CLI matches every server route (ADR 0028).
22. As a person who never turns the switch on, I want no behaviour change at all, so that existing
    Designs work exactly as before.

### Cleanup

23. As a person, I want a Cleanup page reachable from the app menu next to Settings, so that I can
    find it without a command.
24. As a person with several projects, I want to pick the project the Cleanup page scans, so that I
    clean one repository at a time.
25. As a person, I want the page to list worktrees, branches and epics that can be removed, grouped
    by what the button will do, so that I see the safe cases first.
26. As a person, I want each worktree to show its size when it is known, so that I can see what the
    cleanup frees.
27. As a person, I want the page usable before the sizes are ready, with sizes filling in when the
    background `du` ends, so that a slow disk does not block the page.
28. As a person, I want a button that archives a done epic that has landed on trunk, so that
    finished epics leave the active list.
29. As a person, I want archive refused for a queued epic, with the reason shown, so that I never
    archive work the queue still holds.
30. As a person, I want a Remove button for a worktree whose branch is merged (compared by patch ids
    against trunk, not only by ancestry; an iteration branch against its epic's branch, then
    trunk), so that leftover worktrees go with one click.
31. As a person, I want Remove on a merged branch's worktree to delete the branch after the
    worktree, so that no branch is left pointing at nothing.
32. As a person, I want a Delete button for a merged branch with no worktree, so that merged
    branches do not pile up.
33. As a person, I want iteration branches that were cherry-picked onto their epic's branch deleted
    with `git branch -D` only after a server-side patch-id check passes for every commit, so that a
    cherry-picked branch is recognised as done. A squash-merged branch, with one commit or several,
    and with or without a worktree, is deleted the same way after a whole-branch patch-id check.
34. As a person, I want a branch with any commit the check cannot match never deleted by a button
    and offered to the agent instead, so that no unmerged work is lost.
35. As a person, I want a Remove button for an unmerged worktree that is clean and idle (no
    uncommitted changes, no live session), so that I can reclaim its `node_modules` and build
    output.
36. As a person, I want that Remove to show a warning that the ignored files (`node_modules`, build
    output) are lost and the branch stays, and to ask for confirmation before removing, so that the
    cost is clear and nothing is removed by accident.
37. As a person, I want a dirty or live-session worktree never offered a Remove button, so that
    in-use work is safe.
38. As a person, I want iteration branches of archived epics recommended too, so that finished and
    archived epics leave no branches behind.
39. As a person, I want detached-HEAD worktrees listed, so that they are not invisible leftovers.
40. As a person, I want the standing `one-offs` epic not flagged done but unmerged, so that the page
    never offers an agent to "clean" it.
41. As a person, I want every write to run without `--force` and under the worktree lock the
    ralph-loop uses, so that a button never races an agent adding or removing a worktree.
42. As a person, I want a row git refuses to change to stay on the page with git's own reason shown,
    so that I know what to fix.
43. As a person, I want one agent button for the cases no button can decide (unresolved branches and
    done-but-unmerged epics), so that I hand over only the hard part.
44. As a person, I want that button to start a one-off that runs `/gx-cleanup` on those branches and
    epics only, so that the agent does not redo the safe cases.
45. As a person, I want the agent button absent when there is nothing for the agent to decide, so
    that I never start an empty run.
46. As a person, I want the list to refresh after a write, so that I see what is left.
47. As a script or agent, I want `gx server` verbs for the scan and each write, so that every route
    has a CLI verb (ADR 0028), and `gx cleanup scan` to keep working for `/gx-cleanup`.
48. As a person, I want Cleanup to act only when I press a button, so that nothing is removed in the
    background.

### Tips

49. As a person, I want a Tips page reachable from the app menu next to Cleanup, so that I find
    machine setup advice in the app.
50. As a person on macOS, I want a tip to exclude my worktree folders from Spotlight, so that
    indexing stops slowing my agents and the server.
51. As a person, I want the tip to give the System Settings steps (Spotlight, Search Privacy, add
    the folder), so that I can do it without searching.
52. As a person, I want the tip to name the real worktree folder of each registered project, so that
    I add the right folders rather than a placeholder.
53. As a person, I want the tip always listed, with no "done" badge or detection, so that the page
    never claims something gx cannot know (the Search Privacy list needs root to read).
54. As a person not on macOS, I want a page saying there are no tips for this machine, so that I am
    not shown advice that does not apply.
55. As a maintainer, I want tips defined in one Go list (id, title, body, platform) served by an API
    call, so that a later tip is one more list entry.
56. As a maintainer, I want only the Spotlight tip shipped now, so that the parallel-test-runs and
    `fseventsd` tips stay out of this epic.
57. As a script, I want a `gx server` verb for the tips call, so that the route has a CLI verb
    (ADR 0028).

## Implementation Decisions

### Auto-queue

- **State.** `auto_queue` is a positive flag saved in the session meta next to `auto_advance`, off
  by default (the opposite of auto-advance's default). It has its own in-memory deadline, exposed as
  `auto_queue_at` in the session info, and its own Cancel route. Auto-advance's countdown, deadline
  and Cancel are never read or changed by Auto-queue. A countdown is in memory only: a restart drops
  it and the switch stays.
- **Routes.** One route to set the switch and one to cancel the countdown, each with a `gx agents`
  verb like auto-advance's (ADR 0028; the parity test covers them). Both are refusal results, not
  HTTP errors. The set route refuses a non-Design session. The implementer picks the route paths,
  verb names and refusal reason strings (reasons are a public contract once released).
- **Readiness check.** One server-side check decides the trigger, the switch-on rule and the end of
  the countdown. The epic is ready when the Design is at Branch (not past it) and not held (the same
  hold that auto-advance's `designHeld` backstop reads), and the `<map>-impl` epic is open, is not a
  follow-ups epic, is not queued and has never run ("never run" is read the way `ralphloop`'s
  `epicHasRun` reads it). Epics with `base:` or blocked by another epic are ready as usual.
- **Trigger.** The countdown starts when the Design reaches Branch, for a Design whose switch is on,
  and only if the readiness check passes. If it fails, nothing starts and no banner shows. The switch that counts is the newest Design session of
  the map's, live or exited; the countdown and its banner show on that session. The countdown starts
  only on a step change to Branch seen inside a running server, never on the first scan after a
  restart (Design Ready's first scan treats every Design as moved, so reusing it as is would
  restart countdowns for every Design already at Branch). The implementer picks where the server
  hooks this. Design Ready's once-per-Design mark is not reused: Auto-queue has its own state.
- **Switch-on rule.** Turning the switch on starts a countdown at once only when the readiness check
  passes. Otherwise turning it on starts nothing and shows no banner.
- **Held Designs.** A held Design fails the readiness check, so it starts no countdown. If the
  Design becomes held during a countdown, the end-of-countdown check skips with a banner.
- **Delay.** The auto-advance delay: 10 seconds, no user config key. The only override is the
  `GX_AUTO_ADVANCE_DELAY` test hook, and a negative value there turns both countdowns off. The
  `servertest` fixture sets a negative delay by default, so Auto-queue server tests set a delay the
  way the `autoadvance` fixture does.
- **End of the countdown.** The server runs the readiness check again, then queues through the same
  operation as the Epic page's Queue button. If the check fails (for example the person queued the
  epic by hand during the countdown), it queues nothing and shows a banner with the failing
  condition. A refusal from the queue itself also shows as a banner.
- **Switch off.** Cancels a running countdown and shows a banner. Cancel shows a banner too.
- **No new pause (ADR 0042).** A skipped Auto-queue is a banner on the Design session, not a pause
  kind: the epic stays an ordinary unqueued epic, and the person's recovery is the Queue button the
  Epic page already has. No pause kind and no new way into an existing one is added.
- **Web.** The Auto menu's Auto-queue row in the Design footer is enabled, reads `session.auto_queue`
  and posts the flip. The "Coming soon" text goes. A countdown "Queuing <epic> in Ns" with Cancel
  shows beside the menu on a wide screen and as its own strip above the footer on a phone, the same
  way auto-advance's does (the stored external mock's variant D strip is the look reference; the
  mock is an artifact link in ticket 01, reference only). Both countdowns can show at once; the
  implementer picks how two strips stack. The wire types for the web are generated; regenerate them.
- **TUI.** The Agents tab actions menu gets an Auto-queue switch item with the same label shape as
  auto-advance's ("Auto-queue: on/off"), and a "Cancel auto-queue" item while a countdown runs.
  Both show for Design sessions only.
- **Schema.** `auto_queue` lives in session meta, not in the ticket or epic schema, so no migration
  step is needed. If the implementer finds the flag must go in a ticket or epic file after all, the
  change needs a named step in the migrations package and a snapshot update (see `.ai/index.md`).

### Cleanup

- **Scan moves out of `cmd`.** The scan (epics, worktrees, housekeeping, attic refs) moves into a
  package the server can import. `gx cleanup scan` stays as a thin client of it with the same
  commands and flags, so `/gx-cleanup` keeps working. The implementer updates the `gx-cleanup`
  skill if its input changes (new fields, archived epics in the list).
- **Scan fixes.** The scan loads archived epics, so iteration branches of archived epics can be
  recommended. The merged check compares patch ids against trunk, not only ancestry. Iteration
  branches are the one exception: they match their epic's branch, and trunk if the epic's branch is
  gone. Detached-HEAD worktrees are listed. The standing `one-offs` epic is not
  flagged done but unmerged. Attic refs stay report-only: nothing in gx prunes them and the page
  does not offer to.
- **Read route.** `GET /v1/cleanup?project=` returns the scan for a project, with each worktree's
  size. Sizes come from a background `du`, cached, and the response carries sizes that are ready;
  the page shows the rest when ready (via the snapshot and stream or a re-read; the implementer
  picks). Cache invalidation after a removal is the implementer's choice.
- **Write routes.** One route per action: archive an epic, remove a worktree, delete a branch (each
  with a `gx server` verb). All run under the ralph-loop worktree lock, never pass `--force`, and
  answer refusal results. Archive refuses a queued epic. A row git refuses stays, with git's reason
  in the result and on the row. The routes are web writes: they take the web login's checks as other
  web writes do, and the matching verbs are separate socket routes (ADR 0028 amendment).
- **Button rules.**
  - Archive: done epic that has landed on trunk (merged).
  - Merged means one of two patch-id checks passes. Per-commit: every commit's patch id matches a
    commit on trunk (cherry-pick). Whole-branch: the patch id of the combined `merge-base..tip` diff
    matches a commit on trunk (squash merge, including a squash of several commits). For an
    iteration branch, the baseline is its epic's branch (or trunk if that branch is gone).
  - Remove a worktree whose branch is merged: removes the worktree, then deletes the branch (a
    branch is never deleted before its worktree is gone).
  - Delete a merged branch with no worktree.
  - One deletion rule for both: `git branch -d` only when ancestry shows the branch merged into
    trunk (`-d` checks against upstream or HEAD, which is trunk in the bare repo); `git branch -D`
    for every other proven merge (cherry-pick, squash, or an iteration branch merged into an epic
    branch that is not on trunk yet), and only after the server re-runs the check that proved it
    when the write arrives. A branch that passes neither patch-id check has no button; it goes to
    the agent group.
  - Remove an unmerged worktree that is clean and idle (no uncommitted changes, no live session): a
    separate Remove button that warns the ignored files are lost and the branch stays, and asks to
    confirm before anything is removed. The branch is not touched.
  - The server re-checks the rule when the write arrives (state may have changed since the scan) and
    refuses with a reason if it no longer holds. A dirty worktree or one with a live session never
    gets a button.
  - No `ralph-loop/attic/` move. Idle for the unmerged Remove button means exactly: no uncommitted
    changes and no live session. A recent commit (the scan's 48-hour "active work" window) does not
    hide that button, because its warning and confirmation cover the risk. The window still
    suppresses the scan's own "delete" recommendation, as before.
- **Agent button.** One button on the Cleanup page starts a one-off through the existing `POST
  /v1/oneoff` (ADR 0033, 0040: one-offs share the one-off cap and root). A one-off is a ticket in the
  project's `one-offs` epic run by a background agent, not an interactive chat. Its prompt runs
  `/gx-cleanup` on the unresolved branches and the done-but-unmerged epics only, naming them. Hidden
  when there are none. The implementer picks the prompt text and the one-off's name.
- **The agent's confirm is an answer pause (ADR 0042).** `/gx-cleanup` stops at one confirm before
  it deletes, and asks about each done-but-unmerged epic. In a one-off the agent asks through `gx
  tickets ask`, so the one-off goes to the existing needs-answer pause. This is expected, not a
  fault. gx cannot confirm deletions for the person, so it cannot recover on its own. The web
  Answer card on the one-off's ticket page ends the pause. This adds no pause kind and no new way
  into one. Answers in plain chat sessions are not handled today and stay out of this epic.
- **Page and navigation.** A Cleanup page and a Tips page, both opened from the app (gear) menu,
  as two adjacent entries next to Settings. Cleanup shows one project at a time with a project
  picker. The implementer picks the route paths, which project is the default and the layout of the
  groups, within the existing web UI patterns and phone width.
- **No new pause (ADR 0042).** A refused row is a result shown on the row, never a pause. Cleanup
  acts only on a button press: no schedule, no automatic run.

### Tips

- **Source.** One Go list of tips, each with an id, a title, a body and a platform, served to the
  web UI by one API call with a `gx server` verb (ADR 0028). Platform filtering happens on the
  server using the server machine's OS. The list is static: no detection, no status.
- **Spotlight tip.** Platform macOS. The body gives the System Settings steps: Spotlight, Search
  Privacy, add the folder. It does not mention `.metadata_never_index`. It names the real worktree
  folder of each registered project, taken from the project registry (the tip's folder entries are
  filled by the call, not typed into the list). The implementer picks how the body and the folder
  list are carried in the response and what "the worktree folder" means for a bare-repo project
  (the folder that holds its worktrees).
- **Other systems.** The page says there are no tips for this machine.
- **Scope.** Only the Spotlight tip ships. Later tips are one more list entry.
- **Page.** Reached from the gear menu next to Cleanup. Web only: no TUI help section and no
  `gx doctor` check.

## Testing Decisions

- A good test checks behaviour a person or caller sees (a countdown, a button, a result, a route's
  answer), not internals such as which function ran.
- **Seams, highest first.**
  1. The server fixture (`servertest` and `testutil/gxfixture`) for server behaviour: Auto-queue
     (flip, countdown, cancel, queue, re-check skip, restart) and the Cleanup and Tips routes and
     their CLI verbs. Cleanup runs against real temporary git repos, as the scan tests do.
  2. Component tests (vitest) for the Design footer, the Auto menu, the countdown strip, the Cleanup
     page (groups, confirmation, refusal text) and the Tips page (macOS list, empty state).
  3. Playwright flows only where a real round trip or layout is needed, and only by adding steps to
     an existing flow on the same scenario (`gx-playwright-e2es`; `playwrightguard` caps flows).
     Auto-queue extends the Design page's auto-advance flow and the phone Design flow. Cleanup needs
     one new scenario and, if no existing flow fits, one flow (the budget and `CATALOG.md` move
     together). Tips need no flow beyond a step on the Cleanup flow, or a component test.
  4. The TUI actions menu, tested as the auto-advance items are.
- **Prior art.** Auto-advance tests in the prompt package and the `autoadvance` fixture in
  `servertest`; `ui/agents` actions tests for the TUI items; the Design page and footer component
  tests; the existing scan tests with real repos; the route-to-verb parity test in `cmd`.
- Patch-id checks need real commits: a branch that is cherry-picked, one with one extra commit, one
  reachable by ancestry, a one-commit squash and a multi-commit squash (each with and without a
  worktree), and an iteration branch merged by ancestry into an epic branch that is not on trunk.
  The `cleanup` scenario holds each of these.
- Auto-queue readiness has a server test per failing condition (see Verification 11).
- Tests run capped (`scripts/proc-cap`); a package using `servertest` needs both TestMain hooks.

## Verification

A verify ticket runs these against a temporary gx server after the epic is built. Flow names come
from `webui/e2e/CATALOG.md`.

1. **Story 1, 9.** Flow: Design page "Auto-advance" flow, extended. Check: with the switch on and a
   Design that reaches Branch, the countdown ends and the impl epic shows as queued
   (`gx server queue` lists it).
2. **Story 2, 22.** Check: a fresh Design session reports `auto_queue` false in
   `gx agents` JSON; with the switch never turned on, reaching Branch queues nothing.
3. **Story 3.** Flow: Design page Auto menu step: the Auto-queue row is enabled, shows off, flips
   to on, and the "Coming soon" text is gone.
4. **Story 4.** Flow: **24. Phone, Design page** extended: at 390px the Auto menu shows "Queue" and
   flips it.
5. **Story 5.** Flow: Design page: "Queuing <epic> in Ns" with Cancel appears at Branch.
6. **Story 6.** Flow: **24. Phone, Design page**: the countdown is its own strip above the footer
   with a Cancel at least 40px tall. New step.
7. **Story 7.** Check: with both countdowns running, Cancel on Auto-queue leaves auto-advance's
   `auto_advance_at` set, and the reverse. Covered by a server test; the flow checks the visible
   strips.
8. **Story 8.** Check: run the fixture with `GX_AUTO_ADVANCE_DELAY` set; Auto-queue's deadline is
   that long after Branch.
9. **Story 1, 9.** Flow: Design page "Auto-advance" flow, extended. Check: with the switch on and a
   Design that reaches Branch, the countdown ends and the impl epic shows as queued
   (`gx server queue` lists it).
10. **Story 10, 11.** Check: set the impl epic to draft before the countdown ends; nothing is
   queued and the session's banner names the reason. Same for a follow-ups epic, for an epic the
   person queued by hand during the countdown, and for a Design held during the countdown.
11. **Story 10, 11.** Check: set the impl epic to draft before the countdown ends; nothing is
   queued and the session's banner names the reason. Same for a follow-ups epic, for an epic the
   person queued by hand during the countdown, and for a Design held during the countdown.
12. **Story 12, 18.** CLI: the verb to set Auto-queue on a Design session exits 0 with `refused:
    false`, and on a non-Design session answers `refused: true` with a reason (`--json`).
13. **Story 13.** Check: epic ready, switch on: the countdown shows at once. One case per failing
    readiness condition, each with the switch turned on and each expecting no countdown and no
    banner: the Design is past Branch, the epic is queued, the epic has run, the epic is not open,
    the Design is held. One more case: the switch is already on, the Design reaches Branch while the
    epic is already queued, and no countdown starts.
14. **Story 14.** Check: switch off during a countdown: the countdown ends and a banner shows.
15. **Story 15, 16.** Check: flip on, restart the fixture server (`restart` flows' helper): the
    switch is still on and no countdown shows. Also restart with a Design at Branch, the switch on
    and the impl epic unqueued: after the restart no countdown starts and nothing is queued.
    With two Design sessions on one map, the newest one's switch decides.
16. **Story 15, 16.** Check: flip on, restart the fixture server (`restart` flows' helper): the
    switch is still on and no countdown shows. Also restart with a Design at Branch, the switch on
    and the impl epic unqueued: after the restart no countdown starts and nothing is queued.
    With two Design sessions on one map, the newest one's switch decides.
17. **Story 17.** Check: an impl epic with `base:` set is queued by the countdown like any other.
18. **Story 12, 18.** CLI: the verb to set Auto-queue on a Design session exits 0 with `refused:
    false`, and on a non-Design session answers `refused: true` with a reason (`--json`).
19. **Story 19, 20.** Check: the `ui/agents` actions test shows "Auto-queue: on/off" and "Cancel
    auto-queue" for a Design session only.
20. **Story 19, 20.** Check: the `ui/agents` actions test shows "Auto-queue: on/off" and "Cancel
    auto-queue" for a Design session only.
21. **Story 21.** CLI: the set and cancel verbs print the session result and exit 0; the route-to-verb
    parity test passes.
22. **Story 2, 22.** Check: a fresh Design session reports `auto_queue` false in
   `gx agents` JSON; with the switch never turned on, reaching Branch queues nothing.
23. **Story 23, 49.** Flow: new Cleanup flow: the gear menu lists Cleanup and Tips next to Settings
    and each opens its page.
24. **Story 24, 25.** Flow: new Cleanup flow on the new `cleanup` scenario: pick a project, see the
    groups.
25. **Story 24, 25.** Flow: new Cleanup flow on the new `cleanup` scenario: pick a project, see the
    groups.
26. **Story 26, 27.** Check: the page renders with sizes empty, then shows them. Component test with
    a delayed read; the flow waits for a size.
27. **Story 26, 27.** Check: the page renders with sizes empty, then shows them. Component test with
    a delayed read; the flow waits for a size.
28. **Story 28, 29.** Check: archive a done merged epic: it leaves the list and `gx tickets epics`
    no longer lists it. A queued epic answers `refused: true` through the verb.
29. **Story 28, 29.** Check: archive a done merged epic: it leaves the list and `gx tickets epics`
    no longer lists it. A queued epic answers `refused: true` through the verb.
30. **Story 30, 31.** Check: Remove on a merged worktree: the folder and the branch are gone
    (`git worktree list`, `git branch`).
31. **Story 30, 31.** Check: Remove on a merged worktree: the folder and the branch are gone
    (`git worktree list`, `git branch`).
32. **Story 32.** Check: Delete on a merged branch with no worktree removes it.
33. **Story 33, 34.** Check: a cherry-picked iteration branch is deleted with `-D`; a branch with one
    extra commit has no button and shows in the agent group. A squash-merged branch, of one commit
    and of several, is deleted with `-D`, with and without a worktree. A branch merged by ancestry
    into trunk is deleted with `-d`. An iteration branch merged by ancestry into an epic branch that
    is not on trunk is deleted with `-D` and git does not refuse it.
34. **Story 33, 34.** Check: a cherry-picked iteration branch is deleted with `-D`; a branch with one
    extra commit has no button and shows in the agent group. A squash-merged branch, of one commit
    and of several, is deleted with `-D`, with and without a worktree. A branch merged by ancestry
    into trunk is deleted with `-d`. An iteration branch merged by ancestry into an epic branch that
    is not on trunk is deleted with `-D` and git does not refuse it.
35. **Story 35, 36.** Flow: Remove on an unmerged clean idle worktree shows the warning and a
    confirm; Cancel removes nothing; Confirm removes the folder and leaves the branch.
36. **Story 35, 36.** Flow: Remove on an unmerged clean idle worktree shows the warning and a
    confirm; Cancel removes nothing; Confirm removes the folder and leaves the branch.
37. **Story 37.** Check: a dirty worktree and one with a live session show no Remove button, and the
    write verb refuses them.
38. **Story 38.** Check: scan JSON includes an iteration branch of an archived epic with a delete
    recommendation.
39. **Story 39, 40.** Check: scan JSON lists a detached worktree and the `one-offs` epic is not done
    but unmerged.
40. **Story 39, 40.** Check: scan JSON lists a detached worktree and the `one-offs` epic is not done
    but unmerged.
41. **Story 41, 42.** Check: hold the worktree lock in a test and the write waits; a row git refuses
    stays with git's reason in the result.
42. **Story 41, 42.** Check: hold the worktree lock in a test and the write waits; a row git refuses
    stays with git's reason in the result.
43. **Story 43, 44, 45.** Flow: the agent button starts a one-off (`gx server queue` or the Agents
    page shows it) whose prompt names only the unresolved branches and done-but-unmerged epics; the
    button is absent when none exist. The one-off reaches needs-answer at the skill's confirm, and
    the web Answer card on its ticket page answers it and ends the pause.
44. **Story 43, 44, 45.** Flow: the agent button starts a one-off (`gx server queue` or the Agents
    page shows it) whose prompt names only the unresolved branches and done-but-unmerged epics; the
    button is absent when none exist. The one-off reaches needs-answer at the skill's confirm, and
    the web Answer card on its ticket page answers it and ends the pause.
45. **Story 43, 44, 45.** Flow: the agent button starts a one-off (`gx server queue` or the Agents
    page shows it) whose prompt names only the unresolved branches and done-but-unmerged epics; the
    button is absent when none exist. The one-off reaches needs-answer at the skill's confirm, and
    the web Answer card on its ticket page answers it and ends the pause.
46. **Story 46.** Flow: after a write the row is gone without a page reload.
47. **Story 47.** CLI: `gx server` scan and write verbs print `--json`; `gx cleanup scan` still
    prints its text and `--json`; the parity test passes.
48. **Story 48.** Check: with the page open and no press, no worktree or branch changes.
49. **Story 23, 49.** Flow: new Cleanup flow: the gear menu lists Cleanup and Tips next to Settings
    and each opens its page.
50. **Story 50, 51, 52, 53.** Flow: Tips page on macOS fixture: the Spotlight tip is listed with the
    System Settings steps, the registered project's real folder, and no "done" badge. A component
    test covers the text; the folder needs the fixture's project registry.
51. **Story 50, 51, 52, 53.** Flow: Tips page on macOS fixture: the Spotlight tip is listed with the
    System Settings steps, the registered project's real folder, and no "done" badge. A component
    test covers the text; the folder needs the fixture's project registry.
52. **Story 50, 51, 52, 53.** Flow: Tips page on macOS fixture: the Spotlight tip is listed with the
    System Settings steps, the registered project's real folder, and no "done" badge. A component
    test covers the text; the folder needs the fixture's project registry.
53. **Story 50, 51, 52, 53.** Flow: Tips page on macOS fixture: the Spotlight tip is listed with the
    System Settings steps, the registered project's real folder, and no "done" badge. A component
    test covers the text; the folder needs the fixture's project registry.
54. **Story 54.** Check: a component test with an empty list shows "no tips for this machine"; a
    server test with a non-macOS platform returns an empty list.
55. **Story 55, 56.** Check: the tips route returns exactly one tip on macOS and the list is the only
    source (a test adds a fake entry and sees it served).
56. **Story 55, 56.** Check: the tips route returns exactly one tip on macOS and the list is the only
    source (a test adds a fake entry and sees it served).
57. **Story 57.** CLI: the tips verb prints `--json` and exits 0; the parity test passes.

New flows or scenarios to write (these become implementation tickets): a `cleanup` fixture scenario
(a project with a merged worktree, a cherry-picked iteration branch, a branch with an unmatched
commit, a clean idle unmerged worktree, a dirty worktree, a done merged epic, a done unmerged epic)
and one Cleanup flow (or steps on an existing flow) that covers stories 23 to 25, 35 to 36, 43 to 45 and 49. Auto-queue
adds steps to existing flows only. No Mock states list: no mock is stored under the map's `mocks/`
directory (the ticket 01 mock is an external artifact link, reference only).

## Out of Scope

- Anything `gx:disk-space` already ships: removing a feature worktree on merge, the disk-space
  thresholds, the `go clean -cache` cleanup command, and the Playwright and `claudeenv` temp-dir
  leak fixes.
- Automatic or scheduled cleanup. Cleanup acts only on a button press.
- Acting on tips. gx never changes system settings (it does not limit parallel test runs,
  `gx:follow-ups/50`, or restart `fseventsd`, `gx:follow-ups/45`).
- Other tips: parallel test runs and `fseventsd` stay out; a later tip is one more list entry.
- A TUI Tips or Cleanup section, and a `gx doctor` check.
- Detecting whether a folder is excluded from Spotlight; the `.metadata_never_index` advice.
- Moving iteration branches to `ralph-loop/attic/`, and pruning attic refs.
- Resuming an Auto-queue countdown after a restart.
- Any new pause kind or merge block (ADR 0042).

## Further Notes

- Glossary: add **Auto-queue** next to **Auto-advance** (the server queues a Design's
  implementation epic by itself after a countdown, once the Design reaches Branch). The
  implementer adds it with the Auto-queue chain. Cleanup and Tips are plain page names and need no
  entry.
- Changelog entries for each feature belong in `CHANGELOG.md` when they ship.
- `gx-cleanup`'s agent flow stays as it is; the page only calls it for the hard cases.
