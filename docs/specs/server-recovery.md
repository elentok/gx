# Server recovery: stop parking and stranding tickets, and recover the ones that still stick

## Problem Statement

`gx server` runs tickets unattended. Too often a ticket stops moving for a reason that has nothing to
do with its work, and a person has to step in: find the herdr pane, close it, run a manual `reset`,
re-land by hand, or relaunch and hope.

The causes are known. Each one was traced in this effort's map:

- **A server stop or crash strands in-flight tickets.** The claim is written before launch, but the
  runs.json entry only after a successful launch. A stop in between leaves a `claimed` ticket that
  reclaim never sees. Reclaim itself never checks the branch: any agent lookup failure parks
  `handle-mismatch`, even when the agent simply died before doing anything. Some reclaim errors are
  only logged, and the ticket sits `claimed` with no sign.
- **A leftover agent blocks or hijacks the relaunch.** Panes survive parks, resets and server
  kills. The next launch gets `agent_name_taken` and parks `iteration-error`, or, worse, adopts the
  old idle agent because its cwd matches. A merge ticket always matches, so a relaunch can attach to
  the old idle merge agent, read its idle pane as a finish, and park again with an empty
  `iteration_status`.
- **A late report is thrown away.** An agent that goes quiet early gets parked `zero-commit`. When it
  then reports `finished` (with commits, or as commitless), nothing adopts that report. `unpark`
  writes only `status: open`, and the next claim clears `iteration_status`, so the finish is lost and
  the ticket relaunches from scratch. Merge tickets have the same hole under `iteration-error`.
- **The background-task gate is fooled one way and stuck the other.** An `api-request-blob` echo of
  the agent's own `cat .../tasks/<id>.output` resolves an outstanding task, so the finish wait lets
  go too early and the ticket parks `zero-commit`. The other way round, a dead background shell holds
  the gate for up to two hours, and R10 (`release-gate`) ships disabled and refuses exactly the
  dirty or zero-commit case it would meet.
- **The launch prompt sometimes arrives broken.** The nudge retypes the full prompt without clearing
  the input, so a cold Claude can get a leading newline or a doubled slash command, and the
  iteration never starts its skill. Nudge decisions are not logged, so this was invisible.
- **Headless runs are half visible.** `gx server iterations`, the live check, `close-pane`, `wait`
  and nudge only see herdr agents. A headless ticket that needs recovery cannot get it.
- **Stopping a native agent is unsafe.** `Pty.Cleanup` removes the holder socket without checking
  the holder is dead. Group signals go to pids that are never re-checked, and the wait for exit has
  no cap.
- **The land path is wrong in small, costly ways.** The finish path writes into a no-op event sink,
  so conflict-started and child-needs-repair never reach chat. A trailer-only match counts as
  landed. A conflict agent ignores the parent's model and effort. Parking a `done` ticket is not
  refused. Codex's `--add-dir` points at the store root, not the project. A park with no run-log
  reason produces a blank `Kind`.
- **The land lock is keyed and held badly.** The lock key and land target come from three different
  "root" tickets (file order, the ticket that just finished, the lowest-numbered one), so two
  callers can lock different keys for the same epic, and a `rootRefs` error can lock on `""`. The
  lock is held across store writes, the retro submit and notice sends, which blocks claiming in
  other epics; and the retro and some notices are only once-only because of the lock, while others
  are sent twice.

Each fix is small. Together they decide whether an unattended server needs a person every night.

## Solution

Fix every known cause at its root, and add the recovery that covers what still slips through, on
both herdr and headless runners.

- **Reclaim decides from evidence.** A claim whose agent is gone, whose iteration branch has no
  commits past base and whose worktree is clean is reopened on its own. A runs.json entry is written
  before launch, marked `launching`, so a claim cut off mid-launch is reclaimed too. Everything else
  parks visibly, never only in a log.
- **Server launches never adopt.** A leftover agent holding the ticket's label is closed and the
  launch retried once when it is untracked and idle or done; a working one parks `needs-repair`
  naming its pane. `relaunch` refuses `ticket-live` on a working leftover and offers `--stop`, like
  `cancel`.
- **A late finish is adopted.** When a parked ticket's agent reports `finished` after the park, the
  server adopts it like an on-time finish: it lands (commits), closes `done` (commitless), or closes
  `done` (a merge ticket whose merge checks pass). `unpark` and `relaunch` run the same check first.
- **The gate is honest.** Only an allowlist of transcript lines resolves a background task. R10 is
  turned on, checks idle through the run's runner, releases where the loop's own parks can take
  over, and parks the one case where landing would lose edits.
- **The prompt is retyped clean.** The input is cleared before every retype, and every nudge
  decision is logged.
- **Recovery goes through the run's runner.** Live check, `gx server iterations`, `close-pane`,
  `wait` and nudge use the runner recorded in runs.json, so headless tickets are recoverable.
- **Native stops are safe.** Every signal re-checks the pid's identity first; every wait is capped.
- **The land path tells the truth.** Real event sink, no trailer-only proof, inherited model and
  effort, a done guard on every park, honest fallback reasons, the right `--add-dir`.
- **One land target, one lock key, short holds.** The **Root ticket** decides an epic's base and
  target for every caller. `landEpic` decides under the lock and its caller acts after unlock.

## User Stories

### Server stop and crash

1. As a developer who stopped the server mid-launch, I want the ticket whose agent never started to
   go back to `open` by itself, so that I do not find it in `needs-repair` the next morning.
2. As a developer, I want reclaim to reopen a claim only when the agent is gone, the iteration
   branch has no commits past its base and the worktree is clean, so that no work is ever discarded
   by an automatic reopen.
3. As a developer, I want "clean" to mean `git status --porcelain` is empty (ignored files do not
   count), so that the check is plain git. gx writes no files of its own into a ticket worktree.
4. As a developer, I want a reclaim that finds commits or uncommitted edits to keep parking
   `needs-repair`, so that the investigate flow looks at work that might be worth keeping.
5. As a developer, I want the reopen to clean the worktree and branch through the same path as
   `repair.Reset`, so that a reopened ticket starts from the same state as a reset one.
6. As a developer reading the run log, I want a `claim-reopened` event for every automatic reopen,
   so that I can see why a ticket went back to `open` without a person.
7. As a developer on the headless runner, I want the same reclaim rule when a headless agent died, so
   that one rule covers both runners instead of today's `iteration-error`.
8. As a developer whose server crashed between the claim and the launch, I want a `launching`
   runs.json entry written before the launch, so that reclaim sees the claim at all.
8a. As a developer, I want reclaim to restore a `launching` entry whose agent is working as a live
   run, so that an agent the server itself started keeps going.
8b. As a developer, I want reclaim to close the agent of a `launching` entry when it is idle or done,
   then apply story 2's rule, so that an agent that may never have been prompted does not sit idle.
9. As a developer, I want a reclaim error that is not a handle mismatch to park the ticket
   `needs-repair` with its reason, so that a ticket never sits `claimed` with only a log line to
   explain it.

### Leftover agents and relaunch

10. As a developer, I want a server launch to never adopt an agent that already holds the ticket's
    label, so that a relaunch always gets a fresh agent with a fresh prompt.
11. As a developer, I want an untracked leftover agent that is idle or done to be closed and the
    launch retried once, so that a stale pane does not park the relaunch.
12. As a developer, I want a second `agent_name_taken` after that retry to park `needs-repair` /
    `agent_name_taken` after a `launch-failed` event, so that gx never loops on closing panes and R6
    can route it to the investigate agent.
13. As a developer, I want a leftover agent that is still working to park the ticket `needs-repair`
    / `agent_name_taken` with the pane id, cwd and status in the reason, so that the investigate agent can decide and use
    `close-pane`.
14. As a developer, I want gx to never close a holder that a live runs.json run tracks, so that a
    working iteration is never killed by another launch.
15. As a developer reading the run log, I want an event each time gx closes a stale holder, naming
    the pane, cwd and status, so that a closed pane is never a mystery.
16. As a developer, I want `repair.Reset` to close the ticket's pane when it is idle or done, so that
    a reset followed by a relaunch does not trip on its own leftover.
17. As a developer inspecting a park, I want parks to keep leaving the pane open, so that I can read
    what the agent was doing.
18. As a developer, I want `relaunch` on a ticket with a working leftover agent to refuse
    `ticket-live` and offer `--stop`, so that I choose whether to kill a working agent.
19. As a developer, I want `cancel` to see leftover panes that are not in the registry too, so that
    `cancel` and `relaunch` agree on what "live" means.
20. As a maintainer, I want one shared "stop the live session" helper behind both verbs, so that the
    two cannot drift.
21. As a developer, I want a human `relaunch` to reset the recovery budget, so that recovery that
    gave up before can try again after I stepped in.
22. As a developer, I want a recovery-driven relaunch to keep counting against the budget, so that
    recovery cannot reset its own caps.

### Late finish

23. As a developer, I want a `zero-commit` park whose agent reports `finished` afterwards, with
    commits on its branch, to land by itself, so that a false park costs nothing.
24. As a developer, I want a `zero-commit` park whose agent reports `finished` and `commitless`
    afterwards, with no commits on its branch, to close `done` without a land, so that a commitless
    fork parent unblocks its children.
25. As a developer, I want a merge ticket parked `merge-unreported` whose agent reports `finished`
    afterwards to close `done` once the merge checks pass again, so that a merge that did happen is
    not re-run.
26. As a developer, I want adoption only when the ticket's status and park kind are unchanged since
    the park, so that a ticket I unparked, reset or re-parked is never overridden.
27. As a developer, I want adoption to require that `finished` was set after the park, so that an
    old report left on file never closes a ticket.
28. As a developer, I want a late commit with no `finished` to stay parked, so that half-done work is
    never landed on a guess.
29. As a developer, I want `self-reported` parks never adopted, so that a question for me stays a
    question.
30. As a developer, I want an adopted late finish to go through the normal finish wait (idle debounce
    and background-task gate) through the run's runner, so that gx never closes a pane mid-turn.
31. As a developer, I want the pane closed after adoption, so that the adopted ticket leaves no
    leftover for the next launch.
31a. As a developer, I want adoption to find the runner, label and session from the park event, and
    to skip the wait and close when the agent is already gone, so that adoption works after the
    park removed the runs.json entry.
32. As a developer, I want adoption to cancel an investigate ticket that has not started yet, with a
    "late finish adopted" resolution, so that no agent is spent on a solved problem.
33. As a developer, I want a running investigate ticket left alone, so that its own `verify` sees the
    landing and it closes itself commitless.
34. As a developer reading history, I want a `late-finish-adopted` run-log event with the park time,
    finish time and landed SHA (empty when nothing landed), shown in `gx server tickets history`, so
    that I can see what gx adopted.
35. As a developer, I want `unpark` to run the same check first and report "adopted" when it matches,
    so that resuming a ticket never wipes a finish already on file.
36. As a developer, I want `relaunch` to run the same check first and land or close with no launch,
    so that a relaunch can never wipe a finish reported after the park.
37. As a developer, I want no adoption at claim time, so that a `reset` or a deliberate reopen never
    closes a ticket on an old report.
38. As a developer recovering by hand, I want `gx tickets land` to accept a `zero-commit`
    `needs-answer` ticket whose branch has commits, so that I can land a late commit that came with
    no `finished`.
39. As a developer, I want `land` to find the repo from the ticket's project, and to refuse with
    "wrong repo, run from <path>" when it cannot, so that running it from the wrong directory is not
    reported as a missing branch.
40. As a developer, I want `land` to keep refusing `live_agent_on_tab`, so that I run `close-pane`
    before landing work an agent may still be writing.
41. As a developer looking at a blocked fork child, I want `explain` to name the undone parent as the
    blocker, so that I know which ticket to look at.
42. As a developer, I want `commitless-done` kept as the manual fallback, so that a case adoption
    misses still has a verb.

### The background-task gate

43. As a developer, I want a background task resolved only by a task notification, a `TaskOutput` or
    `TaskStop` result, a `queue-operation` line or an `attachment` line, so that an echo of the
    agent's own request never lets the finish gate go early.
44. As a developer, I want every other transcript line type, including ones Claude adds later, to
    never resolve a task, so that a new line type fails safe.
45. As a developer with a dead background shell, I want R10 turned on, so that a held gate is
    released without waiting two hours.
46. As a developer on the headless runner, I want R10 and the gate watchdog to check idle through the
    run's runner, so that the release works for headless runs too.
47. As a developer, I want R10 to release a gate held 15 minutes on an idle agent with zero commits,
    dirty or clean, so that the loop's own `zero-commit` park takes over through its one park path.
48. As a developer, I want R10 to release on a clean worktree with commits ahead, so that finished
    work lands.
49. As a developer, I want R10 to park `needs-answer` / `background-task-gate` naming the dirty files when there are commits
    ahead and uncommitted edits, leaving worktree and pane in place, so that landing never loses the
    edits at cleanup.
50. As a developer, I want the `zero-commit` park reason to say when the worktree has uncommitted
    changes, with a file count, so that I can tell "did nothing" from "did not commit".
51. As a developer, I want the two-hour cap kept as a backstop, so that a gate can never hold
    forever.

### The launch prompt

52. As a developer on herdr or the PTY runner, I want the whole input cleared (repeated Ctrl+U) before
    every full retype, so that the slash command is never prefixed by a newline or doubled.
53. As a developer, I want every nudge decision (Enter sent, retype, stuck, timeout) in the run log,
    so that a bad launch is visible after the fact.

### Headless parity for recovery

54. As a developer, I want `gx server iterations` to list headless runs with session, worktree,
    branch, base and transcript, and empty pane and tab, so that I see every live iteration.
55. As a developer, I want the live check to use the run's runner (`Find` succeeds and status is not
    `done`, `blocked` counts as live), so that herdr and headless runs are judged the same way.
56. As the investigate agent, I want `close-pane` to stop a headless run through its runner, so that
    the verb works on every ticket.
57. As the investigate agent, I want `wait` to use the runner's own wait, so that R7 works on
    headless runs.
58. As the investigate agent, I want nudge to send through the run's runner prompt instead of
    refusing, so that R7 (wait + nudge) can recover a headless ticket.
59. As a developer, I want the nudge watch to record the real runner kind, so that its events are not
    labelled herdr for a headless run.

### Safe native stops

60. As a developer, I want `Pty.Cleanup` to refuse and keep the socket while the holder is live, so
    that a dead claude with a live holder never orphans the holder.
61. As a developer, I want every group signal to re-check pid, start time and session id first, for
    launched and reattached sessions alike, so that gx never kills an unrelated process that reused
    the pid.
62. As a developer, I want a pid that cannot be confirmed never signalled, so that a failed lookup is
    never a reason to kill.
63. As a developer, I want the wait for exit capped at one more grace period, after which Stop
    returns an error naming the pid, so that a stop can never hang the server.
64. As a developer on the headless runner, I want `Headless.Stop` to follow the same rule, so that
    `close-pane` is safe on both native runners.

### The land path

65. As a developer, I want conflict-started and child-needs-repair events from the finish path to
    reach chat, so that I hear about them.
66. As a developer, I want a trailer match with neither the recorded SHA nor a patch-id match to park
    `ambiguous-land` naming the SHAs, so that a trailer alone never counts as landed.
67. As a developer, I want the conflict agent to use the parent ticket's model and effort, else the
    project default, so that a conflict is resolved by the agent I chose.
68. As a developer, I want every park write to refuse a `done` ticket and log the refusal, so that a
    landed ticket is never re-parked by a late budget kill or reclaim.
69. As a developer, I want a park with no run-log reason to say so and point at the run log, with a
    non-blank kind, so that a fallback is honest.
70. As a developer on Codex, I want `--add-dir` to get the project dir, so that Codex can read the
    project's tickets.
71. As a developer, I want each finish to keep landing itself, serialised by the land lock, with the
    defer cap kept at 70 minutes, so that the land path does not grow a second queue.

### The land lock and the root ticket

72. As a developer, I want an epic's base and land target taken from its **Root ticket** (the
    lowest-numbered ticket, whatever its status), so that every caller locks the same key.
73. As a developer, I want a non-root ticket's `base:` to move only where that ticket starts, so that
    it never moves where the epic lands.
74a. As a developer with an epic already in flight, I want its existing feature branch to keep its
    base when the root-ticket rule ships, so that no branch under work moves.
74. As a maintainer, I want one helper returning the root ticket and the epic's refs, used by every
    caller including claim-time `resolveBase`, so that where an epic starts and where it lands always
    agree.
75. As a developer, I want a `rootRefs` error in `completeEpic` to park the root visibly
    (trunk-missing, or `land-error` with the `gx server epics merge` hint), so that an all-done epic
    never strands silently in the queue and gx never locks on an empty target.
76. As a developer, I want `landEpic` to decide under the lock and its caller to send notices and the
    retro after unlock, so that claiming in other epics is never blocked by a land's side effects.
77. As a developer, I want the retro submitted only by the caller that made the stamp transition, so
    that it stays once-only without the lock.
78. As a developer, I want the default-path notice gated the same way, so that two `completeEpic`
    calls never send it twice.
79. As a developer, I want a needs-rebase park to send its notice and retro only when the park write
    changed state, so that a repeat park is quiet.
80. As a developer, I want `stampIfLanded` to skip its tick when the lock is held, so that a poll
    never queues behind a land.
81. As a developer, I want `epicMerge` to follow the same decide-then-act rule, so that the manual
    merge verb holds the lock no longer than a land.

## Implementation Decisions

### Reclaim (server stop and crash)

- `Server.reclaim` / `reclaimHeadless` apply one rule for both runners. When the agent is gone:
  - iteration branch has no commits past its base **and** the worktree is clean (`git status
    --porcelain` empty; gx writes nothing of its own into a ticket worktree) → clean worktree and branch through `repair.Reset`'s logic, reopen to
    `open`, append a `claim-reopened` run-log event (beside `stale-pane-closed` and
    `late-finish-adopted`);
  - otherwise → today's `needs-repair` park, which feeds the investigate flow.
- The launch and reclaim naming already agree (`ralphloop.IterationIdentity`). No change.
- `claimAndLaunch` writes the runs.json entry **before** launch with a `launching` state, and flips
  it after a successful start. Reclaim treats a `launching` entry with no agent by the rule above. A `launching` entry whose
  agent is working is restored as a live run (it is the server's own agent, so this is not
  adoption). One whose agent is idle or done is closed through the run's runner, then the rule
  above applies. No
  startup orphan sweep and no separate graceful-stop path.
- Reclaim errors other than a handle mismatch (ticket gone, parse errors) park `needs-repair` with
  their reason instead of only logging.
- The status-write-then-event-append order in the park path stays (not reproducible; not changed).

### Launches never adopt

- Every server launch path (human relaunch, recovery relaunch, reclaim relaunch) refuses to adopt a
  holder of the ticket's label. herdr's `RecoverStart` no longer returns `StartAdopt` on a cwd match
  for server launches.
- On `agent_name_taken` (`herdr.AgentNameTakenError`): a holder that no live runs.json run tracks
  and whose status is `idle` or `done` is a stale holder. gx closes its tab, appends a
  `stale-pane-closed` run-log event (pane id, cwd, status) and retries the start once. A second
  failure parks. A working holder parks `needs-repair` with pane id, cwd and status in the reason.
  Both parks use kind `agent_name_taken` (not `iteration-error`), and the server path first appends
  a `launch-failed` / `agent_name_taken` run-log event, so R6's name-taken entry matches and the
  investigate agent decides on `close-pane` + `relaunch`. R11 stays disabled and unchanged.
  The earlier "cwd no longer exists" condition is dropped.
- Server restarts keep restoring live runs from runs.json; that is not a launch. The in-process
  loop's `adoptedLaunch` is unchanged. Headless and PTY already refuse with `ErrLabelTaken`.
- `repair.Reset` closes the ticket's pane when it is `idle`/`done`; a working pane keeps today's
  warning. Parks keep panes.
- `ticketRelaunch` and `ticketCancel` both detect a live agent through `Server.liveSession`, which
  sees leftover panes outside the registry. A working leftover refuses `ticket-live`; `--stop` runs
  one shared helper (`liveSession` + the run's runner `Stop`).
- Recovery budget (`guardRailStop`): only `recovery-applied` events after the last human relaunch
  count toward the per-kind and per-ticket caps, and the same cut applies to the `ticket-reset` and
  `manual-land` counts. `ticketRelaunch` appends a `human-relaunch` run-log event when the actor is
  not recovery; that event is the cut. A recovery-driven relaunch writes no such event and counts
  as today.

### Late-finish adoption

One rule, three outcomes. The server's existing store rescan (fsnotify + poll) detects it; no new
plumbing.

- **Conditions** (all): the ticket is untouched since the park, which means exactly: its status
  and park kind are the ones the park wrote (no other field is compared); `iteration_status: finished` was
  set after the park; and the park is one of:
  - `needs-answer` / `zero-commit`, branch has commits past the feature branch → **land**;
  - `needs-answer` / `zero-commit`, `commitless: true`, no commits on the branch → **close `done`**,
    no land (same write as `FinishCommitless`); fork children unblock on their own;
  - `needs-repair` / `merge-unreported`, and `mergeFinishFault` passes again (no rebase in
    progress, target is an ancestor of the epic branch) → **close `done`**, no land.
- Every park write records the run's runner kind, label and session id on the park event, because
  the park deletes the runs.json entry. Adoption takes the runner from there.
- Before acting: the normal finish wait (idle debounce + background-task gate) through that runner;
  then close the pane; then land or close. If the runner no longer finds the agent, adoption skips
  the wait and the close and goes straight to land or close.
- New park kind **`merge-unreported`**, written by `mergeFinishFault`'s "stopped without reporting
  finished or needs-answer" branch (today `iteration-error`). Other merge faults keep
  `iteration-error` and are never adopted.
- `self-reported` parks and late commits without `finished` are never adopted.
- An open investigate ticket for the adopted ticket is cancelled with Resolution "late finish
  adopted" if it has not started; a running one is left alone.
- Run-log event `late-finish-adopted`: ticket, park time, finish time, landed SHA (empty for the two
  close outcomes). Shown in `gx server tickets history`. A `deadlocked` epic clears through the
  normal rescan.
- `unpark` (`repair.Unpark` / `ralphloop.UnparkTicket`) and `ticketRelaunch` run the same check
  first. On a match, `unpark` returns "adopted" and the ticket ends `done` (or lands); `relaunch`
  lands or closes with no launch. No adoption at claim time.
- No recovery-catalog entry; R3 is unchanged. Adoption is a finish, not a remedy (see Further
  Notes).
- `gx tickets land` accepts `needs-answer` when the park kind is `zero-commit` and verify reports the
  branch recoverable; other `needs-answer` kinds stay refused. `land` resolves the repo from the
  ticket's project and refuses "wrong repo, run from <path>" when it cannot.
  `live_agent_on_tab` stays.
- `explain` (`Server.explainIn`) names an undone parent when the parent-not-done edge is what holds
  a fork child, beside `UnresolvedBlockers`.
- `commitless-done` is unchanged.

### Background-task gate and R10

- `transcript` background-task resolution becomes an allowlist; `reportsOnTasks` has no default
  `true`. A marker resolves only on a non-sidechain line that is: a `user` string containing
  `<task-notification>` (the interim-note exclusion stays); a `user` `tool_result` from `TaskOutput`
  or `TaskStop`; a `queue-operation`; or an `attachment`. No empty-timestamp rule.
- R10 launches enabled. R10 and `gatewatch` check idle through the run's runner, not
  `herdr.AgentGet`.
- With the agent idle and the gate held 15 minutes, R10:
  - releases on zero commits (dirty or clean): the loop's existing `zero-commit` park then runs;
  - releases on a clean worktree with commits ahead: the finish lands;
  - on a dirty worktree with commits ahead, parks `needs-answer` / `background-task-gate` with the
    dirty files in the reason and leaves worktree and pane. R10 matches only
    `background-task-gate-held` events, so its own park never re-matches it.
  The `release-gate` refusals that blocked these cases go.
- The normal finish path gains no dirty check.
- `BackgroundTaskAgedOutCap` stays at 2 hours.
- The `zero-commit` park reason names a dirty worktree with a file count ("no commits landed;
  worktree has uncommitted changes (3 files)"); a clean worktree keeps "no commits landed".
- Rejected: output-file mtime as liveness; a lower cap for Bash shells; a post-send
  `<command-message>` check.

### Launch prompt

- `claudeui.PromptWithNudge` clears the whole input (repeated Ctrl+U) before every full retype. Applies to herdr and PTY; headless sends stream-json and is unaffected.
- Each nudge decision (Enter sent, retype, stuck, timeout) appends a run-log event.

### Recovery through the run's runner

- `Server.liveIteration` uses the runner named by the run's runs.json `runner`: `Find(label)`
  succeeds; `blocked` is live. A herdr pane in `done` also counts as live (the agent finished its turn
  but the pane is still there), as gx:server-recovery-impl/09a decided.
- `IterationInfo` for headless fills `Session`, `Worktree`, `Branch`, `Base`, `Transcript`; `Pane`
  and `Tab` stay empty, no placeholders. The TUI already gates focus on `Tab != ""`; the CLI prints
  an empty pane cell.
- `close-pane` calls the run's runner `Stop` (herdr keeps `TabClose`). Headless Stop: interrupt (10s
  grace) → SIGTERM group (5s) → SIGKILL → Cleanup. A dirty worktree left behind falls to the reclaim
  rule.
- `wait` (R7) uses `agentrunner.Runner.Wait(idle|done, 15m)`. No new runner API.
- Nudge (`ticketNudge`) sends through the run's runner `Prompt`, keeps the budget check and the
  nudged event, and `watchNudgedPane` records the real runner kind.

### Native runner stops

- `Pty.Cleanup` returns an error and keeps `holder.sock` while the holder is live. Live = the
  holder's pid + start time + session id (`pidAlive`), not `ptyAlive`. Cleanup never kills.
- Right before every `kill(-pid, …)` (including `killSessions`), re-check that pid with `pidAlive`,
  for launched and reattached sessions. The only exception is the failed-launch path, which has no
  meta yet.
- A pid that fails the re-check, or whose lookup errors, is not signalled. The exit wait after that
  is capped at one more grace period; on timeout Stop returns an error naming the pid. No uncapped
  wait on `exited`.
- `Headless.Stop` re-checks before SIGTERM and SIGKILL and uses the same capped wait.

### Land path

- `ralphloop.FinishOutcome` is patched in place, no new outcome type: `Kind` is never blank, and the
  fallback reason says the run log had none and points at it.
- `FinishIteration` and `WaitIterationFinished` take the server's real event sink instead of
  `noopEventSink`.
- `alreadyApplied` drops trailer-only proof. Trailer present but neither the recorded SHA nor a
  patch-id matches → park `needs-repair` / `ambiguous-land` with a reason naming the SHAs.
- Every server launch takes the project's `agents.*` model and effort (hot-reloaded). A conflict agent
  uses the parent ticket's run model and effort instead, else that default. The park event records the
  run's model and effort, so an adopted late finish lands under the launch model.
- The shared park write refuses a `done` ticket and logs the refusal. This covers
  `markNeedsRepairWithReason`, the ralphloop park and `Server.parkTicket`.
- Codex `--add-dir` in the server launch gets the project's ticket scratch dir joined with the epic name, the same as the in-process loop.
- No per-epic land queue. Each finish lands itself under the land lock. `landDeferCap` stays at 70
  minutes. The server keeps landing each ticket from its own finish (it never had the in-process
  loop's lowest-first queue); the in-process loop is unchanged.

### Root ticket, land target and lock

- **Root ticket** = the epic's lowest-numbered ticket (number, then identifier), any status,
  cancelled included — the `lowestTicket` rule. It replaces `e.Tickets[0]` and "the ticket that just
  finished".
- One helper returns the root ticket and the epic's refs (base, target) from the epic's own
  `base:`/`blocked_by` and the root ticket's `base:`/`blocked_by`. Every caller that means "the
  epic's refs" uses it: `completeEpic`, `landRoot`, `nothingToLand`, `stampIfLanded`, `epicMerge`,
  `epicMergeTarget`, `landTarget` and claim-time `resolveBase`. Accepted change: an epic whose
  first-claimed ticket is not the lowest and has its own `base:` now starts from the root ticket's
  refs. The new refs apply only when the feature branch is created. An existing feature branch is
  adopted as today (stamp from `merge-base(feature, ref)`) and keeps its base.
- `completeEpic` never locks on an empty target. A `rootRefs` error parks the root like `landRoot`
  does: trunk missing → trunk-missing state; else a `land-error` park with the
  `gx server epics merge` hint.
- `landEpic` returns a decision (merged, already, removed, park kind, sendNotice, submitRetro).
  Stamps, `removeRoot` and git stay under `trunkLocks`; the caller sends notices and runs
  `submitRetro` after unlock.
- `submitRetro` and the default-path notice are set only for the caller that made the stamp
  transition (first to stamp `completed_at` or `merged_at`). No mutex around the one-off dedupe.
- A needs-rebase park sends its notice and retro only when the park write changed state.
- `stampIfLanded` uses `TryLock` and skips the tick when the lock is held; its notice goes after
  unlock, gated on `removed`.
- `epicMerge`: `ensureFeatureWorktree`, `ensureRootQueued` and git stay under the lock; notices and
  retro go after unlock from the decision.

### Manual step

- Delete the phantom dirs by hand, once, after checking no live epic has these names:
  `~/.local/share/gx/tickets/{orchestrator-daemon-4-projects-one-offs,orchestrator-daemon-4b-projects-one-offs,orchestrator-daemon-5-recovery}`
  and `~/.local/state/gx/land-locks/tickets/`. No migration code.

## Testing Decisions

A good test drives the server or the runner from outside and asserts what a person would see: the
ticket file's status, park kind and reason; the run-log events appended; the commits on the feature
branch; whether a pane or process was closed or signalled; the verb's result and refusal reason. It
does not assert which helper ran or in what order, except where the order is the behaviour (the
input clear before the retype; the re-check before the signal).

### Seams

- **The server, through `server/export_test.go` hooks and `testutil/runnerfake`** is the main seam.
  Reclaim, launches-never-adopt, late-finish adoption, `relaunch`/`cancel`/`unpark`, recovery verbs,
  R10, the land decision and the lock all go through it. Running the same cases against a herdr-shaped
  and a headless-shaped fake runner is what proves parity.
- **`ralphloop`'s land and finish seams** (`LandTicket`, `FinishIteration`, the park write) for
  `FinishOutcome`, `alreadyApplied`, the done guard and the event sink.
- **`nativerunner`** (`Pty`, `Headless`) for Cleanup refusal, the pid re-check and the capped wait.
  These need real processes, not the fake runner.
- **`transcript`** for the allowlist.
- **`claudeui.PromptWithNudge`** for the clear-before-retype order.

New: one `export_test.go` hook that holds the trunk lock, for the lock test below. No other new seam.

### What gets tested

- **Reclaim**: dead agent + empty branch + clean worktree reopens and logs, for both runner shapes;
  commits or edits park `needs-repair`; a `launching` entry with no agent reopens; a `launching` entry with a
  working agent is restored live; one with an idle or done agent is closed, then reopens or parks; a non-mismatch
  error parks with its reason.
- **Launches**: stale untracked idle/done holder is closed and the launch retried once, with a
  `stale-pane-closed` event; a tracked or working holder parks `agent_name_taken` naming the pane,
  after a `launch-failed` event, and R6 matches it; a second failure parks the same way; no server launch adopts.
- **`relaunch` / `cancel`**: working leftover refuses `ticket-live`; `--stop` stops it through the
  run's runner; a human relaunch resets the budget count, a recovery relaunch does not.
- **Late finish**: each of the three outcomes, on both runner shapes, with the runner read from the
  park event; an agent already gone skips the wait and close; each failed condition (touched since park, `finished`
  before the park, no `finished`, `self-reported`, other merge faults) stays parked; unstarted
  investigate ticket cancelled, running one left; `unpark` and `relaunch` adopt first; no adoption at
  claim.
- **Gate**: the two regression transcripts from 12 (blob echoes only → `OutstandingFresh`; plus
  `queue-operation` and `<task-notification>` → `Resolved`, `AwaitingTurnEnd`); R10's three
  outcomes; R10 does not fire on its own `background-task-gate` park; R10 idle through a headless-shaped runner; the dirty zero-commit park reason.
- **Prompt**: the clear is sent before the retyped text; each nudge decision appends an event.
- **Headless parity**: a headless run appears in `/v1/iterations` with empty pane/tab; `close-pane`
  stops it; R7 wait + nudge work on it.
- **Native stops**: Cleanup refuses with a live holder; a pid whose start time no longer matches is
  not signalled; Stop errors after the capped wait.
- **Land path**: trailer-only match parks `ambiguous-land`; parking a `done` ticket is refused and
  logged; a conflict-started event reaches the sink; fallback reason and non-blank kind; conflict
  agent inherits model and effort.
- **Lock and root**: an epic whose feature branch already exists keeps its base under the new rule;
  every epic-refs caller gets the same target for an epic whose file order and
  finish order differ from the lowest number; `rootRefs` error parks visibly; with the trunk lock
  held, `keepClaiming` still claims in another epic and `stampIfLanded` stamps on a later tick after
  release; the retro and default notice are sent once across two `completeEpic` calls.

### Prior art

- `server/reclaim_route_test.go`, `server/runs_load_test.go` for reclaim.
- `server/ticketwrites_test.go` for `relaunch` / `cancel` refusals.
- `server/recovery_test.go`, `server/recovery_plan_test.go`, `server/nudge_test.go` for verbs,
  budget and nudge.
- `server/handlanded_test.go`, `server/epicmerge_test.go`, `server/landrecover_test.go` for the land
  lock and root paths.
- `ralphloop/serverland_land_test.go`, `ralphloop/serverland_wait_test.go`, `ralphloop/land_test.go`,
  `ralphloop/park_guard_test.go`, `ralphloop/unpark_test.go` for land, finish, park and unpark.
- `nativerunner/pty_test.go`, `nativerunner/headless_test.go` for real-process stop tests.
- `transcript/background_task_test.go`, `agentrunner/claudeui/nudge_test.go`.
- `testutil/runnerfake` (with its conformance test) and `testutil/herdrfake` for runner shapes.

## Out of Scope

- **The watchdog agent** that hunts for stuck tickets (former ticket 02).
- **Headless or native-runner features** beyond what recovery and `gx server iterations` need.
- **False-park causes no ticket in this map names.** No open-ended hunt.
- **Tickets left in their own maps**: agent-runner-hardening/02, agent-sessions/01 and 03,
  fork-handling/02, agent-skills/04.
- **A per-epic land queue** and any change to `landDeferCap`.
- **A non-root ticket's cross-epic blocker on an unlanded epic** not reaching its branch.
- **A post-send `<command-message>` check**, output-file mtime liveness, and a lower Bash-shell cap.
- **Startup orphan sweeps** and migration code for the phantom dirs.
- **The in-process loop's `adoptedLaunch` guard**, which stays as it is.

## Further Notes

### Why late-finish adoption is not a recovery remedy

Recorded in ADR 0038.

ADR 0034 makes marking a ticket commitless-done a `high`-authority remedy that never auto-applies.
Late-finish adoption closes commitless tickets unattended anyway. This is consistent: the server is
not deciding the ticket is done, the agent reported it, exactly as an on-time commitless finish does
(`adoptCommitlessFinish`). A normal finish lands with no acceptance check, so adopting a late one is
the same trust. The conditions (untouched since park, `finished` after park, `zero-commit` or
`merge-unreported` only) keep it from acting on anything a person or an old report put there.

### Why launches stop adopting

Recorded in ADR 0039.

A merge ticket's cwd is always the feature worktree, so the cwd match in `RecoverStart` always hit
and a relaunch attached to the old idle agent. The server never prompted it, read the idle pane as a
finish after the debounce, and parked again. Implement tickets also have a fixed cwd, so they are
exposed the same way. Closing a stale holder and launching fresh is cheaper than any test of whether
an old agent is safe to reuse.

### Order of risk

The lock rework (decide under the lock, act after) and the Root-ticket helper touch every land. The
"accepted change" in start refs applies only to feature branches created after it ships. Land these with their tests first, before the recovery work that
leans on them.

