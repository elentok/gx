# gx server: multi-project orchestration, one-offs, and auto-investigate

Handoff spec from the Orchestrator daemon wayfinder map (`.scratch/orchestrator-daemon/map.md` in
the local ticket tracker — decisions `01`–`17` in that epic's `issues/` directory). The epic slug
`orchestrator-daemon` is kept; everything else uses ticket 11's vocabulary. The long-lived process
is the **server** (never "daemon" or "orchestrator" in user-facing text), the unit of work is a
**ticket**, an agent is **launched** (never "spawned"), and the per-ticket record is the **event
log** (never "run log"). Where the map or a ticket uses an older word, this spec translates it.

## Problem Statement

gx runs ralph-loop epics from inside the TUI process. That one fact causes most of the problems below.

- **Quitting the TUI stops the work.** The loop registry, scheduler, cost poller and chat sinks all
  live in the TUI. A quit guard (`CanQuit`) exists only to stop the operator from killing their own
  runs. Nothing can schedule work while no TUI is open.
- **One repo at a time.** Tickets live in each repo's `.bare/.scratch`. The Queue is per repo, the
  attach lock is per repo, and the run budget counts "since Attach". There is no single place that
  sees every project's work, so there is no way to share agent slots or a daily budget across them.
- **Nothing outside gx can submit work.** A cron entry, an Alfred shortcut, a phone or a keybinding
  cannot say "run this prompt and tell me the answer". Every agent run starts from a hand-authored
  epic in a repo and a person pressing keys in the TUI.
- **Recovery is manual and blind.** When a ticket fails unattended, a person must notice, open
  `gx-investigate`, and reconstruct what happened by scraping log files and state files. Half of the
  known failure modes cannot even be detected from the logs: launch-phase failures park the ticket
  `needs-repair` and write no event at all (ticket 09 found zero log events for six documented
  incidents). The largest single fault class — 25 tickets across 10 epics that lost their commits —
  left nothing to recover from.
- **The ticket history is unprotected.** 72 epics of tickets and logs live in an untracked
  `.scratch` directory per repo. Agents read and write ticket files by path, which forces
  `foldscratch` hacks and gives agents filesystem reach outside their worktree.
- **The vocabulary has drifted.** "task", "spawn", "attach", "epic run", "daemon", "orchestrator"
  and "job" overlap or collide, and renaming them piecemeal would mean renaming twice.

gx is also used every day to build gx. Whatever replaces this cannot leave gx broken for two weeks.

## Solution

A long-lived, terminal-detached **gx server** becomes the sole scheduler for every registered
project on the machine. It exposes one HTTP/JSON API (unix socket, opt-in loopback TCP) whose every
route has a matching `--json` CLI verb, plus an event stream. The TUI becomes one client of that
server: it renders a view model built from a snapshot plus streamed events, and its keys call the
same verbs a script would.

- **One model of work.** Everything is a **ticket**. An **epic** is a top-level ticket with
  children; a **one-off** is a top-level ticket without them; an **investigate ticket** is a
  `type: investigate` child of a failed ticket, and does not count as a child for that test.
  Projects are containers, not tickets. `parent` means containment; `blocked_by` works on any
  ticket, across epics in one project, and implies the base branch for commitful work.
- **One global ticket store.** Every project's tickets and event logs move into one git-backed
  directory that only the server commits. Markdown stays the truth; the server is an index over it
  plus its own state (queue, live iteration handles, locks) in a state directory. Agents
  touch tickets only through `gx tickets` verbs by address (`project:epic/06`).
- **Multi-project.** Projects register explicitly (`project.json` in the ticket store). One
  server-wide queue, three slot caps (global, per epic, optional per project), one daily budget, and
  notifications routed per destination.
- **External triggers.** `gx server one-off "<prompt>"` (or `--file`) creates and enqueues a
  one-off, defaulting to a commitless `type: prompt` ticket that writes its answer into a
  `## Result` section. Cron, launchers and a phone all call this one verb; the server has no
  scheduler of its own.
- **Auto-investigate.** When a ticket fails, server-side **recovery rules** apply known mechanical
  remedies from a catalog, and an **investigate ticket** (an agent) runs only when judgment is
  needed or nothing matched. Recovery repairs the orchestration, never the work: it may nudge a pane
  or call the server's own write verbs, never write code. High-authority remedies are proposed and
  wait for one-key human approval. Unmatched failures produce a typed report and a deduped follow-up
  draft, so the catalog grows from real failures.
- **Incremental migration.** Seven stages (S0–S6) in five standalone implementation epics, launched
  by hand in order with the by-hand steps between them, a temporary
  `orchestrator: in-process | server` switch, and a cutover after S3 has run for one week without
  rollback.

## User Stories

### Observability (S0)

1. As an operator, I want every park and terminal outcome (`needs-answer`, `needs-repair`, `done`,
   `cancelled`) to write the ticket, append exactly one event, and send its notification through one
   code path, so that no failure can park a ticket silently. (13)
2. As an operator, I want every failure event to carry a `kind` from a closed enum, so that tools
   match failures on `(type, kind)` instead of on free-text `reason`. (13)
3. As an operator, I want each failed launch attempt recorded as a `launch-failed` event with the
   herdr error code, the attempt number and the iteration label, so that launch-phase failures stop
   being invisible. (13, 09)
4. As an operator, I want a ticket whose launches fail N times in a row parked `needs-repair` with
   kind `retry-exhausted`, so that a broken launch cannot burn agent launches forever. (13, 09)
5. As an operator, I want a ticket that parks and is re-claimed M times inside a window parked
   `needs-repair` with kind `spinning`, so that a park/re-claim spin like `conflict-lifecycle/04`'s
   1040-event loop is quarantined in minutes. (13, 09)
6. As an operator, I want the retry-storm and spin thresholds to be config keys with defaults, so
   that I can tune them without a rebuild. (13)
7. As a recovery tool, I want `park_kind` written both to the ticket's frontmatter (current state,
   cleared on resume) and as `kind` on the event (history, never cleared), so that I can read the
   cause of a park after a human has already resumed it. (13)
8. As a developer, I want a test that fails if any ticket status write bypasses the park path, so
   that a future catch-all cannot skip logging again. (13)
9. As a developer, I want every event to stay under 4096 bytes, and every failure event to carry a
   fixed set of fields (`type`, `kind`, `address`, truncated `reason`, plus `iteration` and
   `attempt` when applicable) with unknown fields omitted, so that appends stay atomic and consumers
   never parse placeholders. `kind` is required on failure and recovery events only. (13)
10. As a developer, I want the `kind` enum published by a CLI verb, so that the recovery catalog
    references machine names, not prose. (13)

### Ticket model and ticket store (S1)

11. As an operator, I want all my tickets and event logs in one git-backed ticket store, so that
    years of epic history are versioned and cannot be lost with a worktree. (03)
12. As an operator, I want `gx tickets migrate <old-.scratch>` to copy a repo's `.scratch` tree
    (including `.archive` and logs) into the ticket store, fix legacy frontmatter, rewrite
    `type: task` to `type: implement`, create `project.json` if missing, and validate every migrated
    ticket, so that one command moves a repo. (11, 12)
13. As an operator on a second machine, I want `gx tickets migrate --dry-run` to convert and validate
    in a temp dir without touching the ticket store, so that I can check a migration before running
    it. (11, 12)
14. As an operator, I want migrate to be idempotent and refuse per existing address, so that a
    re-run never overwrites tickets already in the store. (11)
15. As an operator, I want migrate to refuse while any ticket is `claimed`, so that I never move a
    ticket out from under a live agent. (12)
16. As an operator, I want migrate to leave the old `.scratch` tree in place as a read-only backup
    until cutover, so that I can inspect the old state if something looks wrong. (12)
17. As an operator, I want every top-level ticket (epic or one-off) to be a directory with a
    `ticket.md` entry file, and everything below it to be a flat file in `issues/` linked by
    `parent`, so that the tree has one shape. (11, 03)
18. As an operator, I want an epic's `ticket.md` to replace `map.md` and `epic.yaml`, with
    `status`, `blocked_by`, `base:` and timing fields in its frontmatter, so that an epic is just a
    ticket with children. (11, 01)
19. As an operator, I want every ticket to have one canonical address `project:epic/06`, with `06`
    and `epic/06` accepted only as input, so that logs, trailers and the API name a ticket the same
    way. (03, 11)
20. As an agent, I want `gx tickets show <addr>`, `add <parent-addr> --slug … --body -`,
    `section <addr> "<Heading>" -`, and `set <addr>` to work by address, so that I never need a
    file path or filesystem access outside my worktree. (03)
21. As an agent, I want my iteration prompt to name a ticket address, not a path, so that the
    implement skill works the same for every project. (03, 12)
22. As an operator, I want hand-editing ticket files to keep working, with the file's state
    overriding the server index on every rescan, so that I can still fix a ticket in my editor.
    (03)
23. As an operator, I want `type: task` renamed to `type: implement`, with the loader accepting
    `task` as an alias until cutover, so that old files keep loading during migration. (11)
24. As an operator, I want a terminal `cancelled` status distinct from `done`, so that a withdrawn
    ticket stops counting as outstanding without claiming the work was performed. Cancel (from S3)
    refuses `ticket-live` on a live pane unless `--stop`, and cancelling an epic cancels its
    non-terminal descendants. (01)
25. As an operator, I want a dangling `blocked_by` reference to render the ticket `Error`, not
    silently blocked, so that a ticket whose blocker vanished never runs and never hides. (01, 16)
26. As an operator, I want `gx tickets validate` to detect `blocked_by` cycles, including a ticket
    blocked by its own ancestor or descendant, and report every edge in the cycle, so that a
    deadlock is caught before a run. (16)
27. As an operator, I want `gx tickets validate` to load the ticket's whole project, and
    `--all` to walk the whole ticket store, so that cross-ticket errors are found. (16)
28. As an operator, I want `done` and `cancelled` tickets to skip graph checks, so that archived
    history with deleted blockers does not fail migrate's validation. (16)
29. As an operator, I want `gx merge`'s JSON field `base` renamed to `target`, so that the server's
    land step does not feed a target into a field called base. (11)
30. As an operator, I want the in-process loop to claim, run and land tickets read from the ticket
    store after S1 (checked by the server epic's first tickets, which double as the S1 smoke run),
    so that I can use gx every day while the server is built. (12)

### Server process and lifecycle (S2)

31. As an operator, I want `gx server install` to register a launchd agent that starts at login and
    restarts only after a crash, so that the server is always there without me starting it. (06)
32. As an operator without launchd installed, I want `gx server start` to self-detach from the
    terminal, so that closing the terminal does not stop the server. (06)
33. As a script, I want `gx server start` on a running server to print `already running (pid N)` and
    exit 0, so that it is safe to call repeatedly. (06)
34. As an operator, I want a second server to fail with `already running (pid N)` because of one
    server lock in the state dir, so that two schedulers can never run on one machine. (04, 11)
35. As an operator, I want `gx server stop` to stop claiming, let an in-flight land finish, flush the
    ticket store commit (from S3; the S2 read-only server never commits), and exit 0 while live
    agents keep running in their panes, so that stopping the server never kills work. (06, 03)
36. As an operator, I want upgrading to be `gx server restart`, and `gx server status` to warn when
    the binary on disk differs from the running build, so that I know when a restart is needed.
    (06)
37. As an operator, I want JSON `server.log` in the state dir with size rotation and
    `gx server logs [-f] [--level] [--json]`, so that I can read server diagnostics separately from
    ticket event logs. (06, 11)
38. As an operator, I want the server to start even when herdr is down, report `herdr unavailable`
    in status and the stream, and retry herdr every 30 s, so that reads and the API keep working.
    (06)
39. As a client, I want `GET` snapshot then subscribe to the event stream from the snapshot's
    sequence number, and to re-snapshot on any sequence gap or reconnect (the server drops a
    subscriber that falls behind its buffer), so that I get consistent live state with no gaps.
    (05)
40. As a client, I want an integer API version in the handshake, and to go read-only on a version
    mismatch, so that an old TUI never sends writes a new server misreads. (05)
41. As a phone or future web UI, I want an opt-in loopback TCP listener serving the same routes as
    the unix socket, so that I can reach the server over a VPN tunnel. (05)
42. As an operator, I want `gx server tickets explain <addr>` to show the scheduler's own verdict for
    one ticket (eligible / blocked by X / slot cap / project unavailable / parked + reason / claim
    re-read mismatch), so that "why isn't this running" has one true answer. In S2 it gives only
    the verdicts the store can show (blocked by X, `Error`, parked + reason, draft) and
    `unknown: in-process scheduler` for the rest; full verdicts from S3. (05)
43. As an investigator, I want read verbs for tickets, queue, live iterations (pane, worktree,
    branch, resolved base), projects with state, locks with holders, and event history by address,
    so that I never scrape state files again. (05)
44. As a developer, I want a CI test that diffs the API route table against the CLI command tree,
    so that an endpoint without a `--json` verb fails the build. (05)
45. As a developer, I want an in-process server test harness on a temp state dir and socket, with
    the fake herdr, driven only through the API client, so that server behavior is testable from S2
    on. (12)

### Server orchestrates (S3)

46. As an operator, I want the server to be the scheduler when the switch says `server`, with the
    TUI's in-process loop, loop registry and quit guard kept only behind `orchestrator: in-process`
    for rollback until cutover, so that quitting the TUI never affects a server-run ticket. (07, 12)
47. As an operator, I want one server-wide queue of addresses, persisted in the state dir, so that
    queue order survives a restart and is not mixed with ticket content. (03, 04)
48. As an operator, I want `gx server tickets land | reset | unpark | verify | park | cancel |
    relaunch` and the `gx server queue` verbs to be server writes, each with a `--json` result or a
    `{"refused":true,"reason":…}` refusal, so that every orchestration change has guards and one
    contract. (05, 11)
49. As an operator, I want content edits (`gx tickets show | add | section | set`) to be direct
    writes under a per-ticket lock that then ping the server, so that content editing works whether
    or not the server is up, and the stream updates at once. (05, 11)
50. As an operator, I want `gx tickets set --status` to lose the orchestration statuses, so that no
    single verb mixes a direct write with a server write. (05)
51. As an operator, I want every `--json` result to carry `via: direct | server` and
    `actor: human | agent | recovery`, so that I can see which path made a change and who asked.
    (11)
52. As an operator with the server down, I want the four repair verbs (`land`, `reset`, `unpark`,
    `verify`) to run direct under the land lock and print `server not running — ran directly under
    land lock`, so that I can still repair a ticket. (05, 11)
53. As a script, I want every other server verb to refuse `server-not-running` with a
    `gx server start` hint and never auto-start the server, so that cron calls never launch
    unrequested servers. (06)
54. As an operator, I want the server to reclaim still-live iterations on start (pane exists, agent
    name matches, worktree exists) without asking, so that a restart is invisible to running work.
    (06, 11)
55. As an operator, I want a reclaim mismatch to park the ticket `needs-repair` kind
    `handle-mismatch` with a `## Needs Repair` section, so that a broken handle is visible and
    recoverable. (06, 13)
56. As an operator, I want the server, on start, to treat a land lock held by a dead owner as a land
    that was in flight — clear the orphan lock and run the `verify` logic for that epic: finish,
    roll back, or park `ambiguous-land` — so that a crash never half-lands a ticket. (06, 13)
57. As an operator, I want one committer of the ticket store at a time — the scheduler the switch
    selects — committing 60 s after the last change and at once on `done`, `cancelled` or any park,
    with an optional best-effort push remote, so that there are no commit races and history is
    backed up. (03)
58. As an operator, I want the server to re-read a ticket file at claim and refuse if it changed, so
    that a missed file-watch event can only delay a schedule, never cause a wrong one. (03)
59. As an operator, I want `blocked_by` to accept `epic/06` across epics in one project, with
    ancestry computed (a ticket runs only if no ancestor is blocked), so that one epic can wait for
    another. (01, 16)
60. As an operator, I want a cross-project `blocked_by` reference to be a validation error
    ("cross-project blocking not supported"), with the grammar reserved, so that adding it later is
    not a format break. (01, 16)
61. As an operator, I want a commitful ticket blocked by exactly one unlanded same-project
    commitful ticket to base on that blocker's branch, and on its default base (leaf → parent's
    branch, root → trunk) once the blocker landed or when it has no blockers, derived at claim time,
    so that dependent work starts from the code it depends on. (01, 02)
62. As an operator, I want a ticket with two or more unlanded commitful blockers and no explicit
    `base:` to park `needs-answer` naming the blockers, and `validate` to warn earlier, so that gx
    never invents a merge base. (02, 16)
63. As an operator, I want an optional `base:` override (branch name or ticket reference) and a
    derived, never-stored target, so that PR chains retarget for free when a blocker lands. (02)
64. As an operator, I want `base:` on a commitless ticket or in a `vcs: none` project to be a
    validation error, so that a meaningless field is caught. (02, 16)
65. As an investigator, I want the resolved base (ref + SHA) stamped into `resolved_base:`
    frontmatter at every claim, and `validate` to warn when it differs from the last `claimed`
    event, so that I can see what a ticket started from without reading git. (02, 03)
66. As an operator, I want the server to create an epic's feature branch from its resolved base
    when its first child is claimed, adopting a same-named hand-made branch, so that branch setup is
    automatic. (02)
67. As an operator, I want the server to land a root ticket by calling the deterministic `gx merge`
    core and park it on `needs_rebase`, so that landing never rebases on its own. (02)
68. As an operator of an ff-only project, I want blocking to release `on-landed` and an opt-in
    automatic ff-only merge once the epic's code-review ticket is done, refusing rather than
    rebasing, so that "done but unlanded" stops existing there. Ships in S4. (01)
69. As an operator of a PR project, I want blocking to release `on-done` with the dependent's PR
    targeting the blocker's PR, so that a review wait does not stall the queue. Ships in S4. (01)
70. As an operator, I want the `Ralph-Loop-Ticket` trailer to stamp the ticket's address from S1,
    with the parser still accepting the old `<featureBranch>/<id>` form, so that identity survives
    a retarget and old history still reads. (02, 03)
71. As an operator, I want the TUI to show a server indicator (`server ● pid N` / `○ down` /
    `◐ read-only — restart` / `⚠ herdr unavailable`) on every tab, so that I always know if
    orchestration is available. (07, 11)
72. As an operator with the server down, I want the Queue tab cleared with a "server down — `s` to
    start" banner, the Tickets tab still reading markdown with live columns blank, and orchestration
    keys greyed with the reason, so that reads degrade and nothing pretends to work. (07)
73. As an operator, I want a connected TUI to run no fsnotify watch and no poll timer, refreshing
    only from stream events, and to fall back to the watch + 30s poll only when the server is down,
    so that an idle TUI does no work of its own. (07)
74. As an operator, I want `a` to enqueue checked tickets (live or not) and `r` to atomically replace
    one project's pending queue entries, refusing `ticket-live` for a checked ticket with a live
    iteration in its subtree, so that queueing is one verb each. (07)
75. As an operator, I want to pick the agent at enqueue time in the `a`/`r` confirm, so that the
    run-start modal is no longer needed. (07)
76. As an operator, I want `x` on the Queue tab to delete with a live-iteration refusal, so that it
    can no longer orphan a pane. (07)
77. As an operator, I want each pending Queue row to show its `explain` verdict as subtext, delivered
    with the snapshot and events, so that I see why it waits. (07)
78. As an operator, I want toasts only from live stream events, never replayed after reconnect, and
    chat notifications sent once by the server, so that two open TUIs do not double-notify my phone.
    (07)
79. As an operator, I want the server to own the chat sink, route server-level events (crash,
    restart, herdr down/back, budget latch, daily summary) to the global destination, and hold
    parks whose `kind` carries `cause: herdr` during a herdr outage behind one digest, so that an
    outage is one message, not twenty. (17, 13)
80. As an operator, I want the in-process loop to refuse to claim once the switch says `server`, and
    the server to refuse claim-affecting writes with `scheduler-not-selected` while it says
    `in-process`, so that one machine never has two schedulers. (12)
81. As an operator, I want to roll back from S3 by flipping `orchestrator` back to `in-process`, with
    the TUI back in its in-process mode (server indicator hidden) and server reads still available,
    so that a broken server cannot stop me from using gx on itself. (12)

### Multi-project (S4)

82. As an operator, I want `gx project add [path] [--name] [--vcs none]` to register a project
    explicitly, with a one-line hint when I run gx in an unregistered repo, so that a typo'd cwd never
    becomes a project. (04)
83. As an operator, I want the project name (a unique slug, default the repo's dir basename, the dir
    holding `.bare` in a bare layout) to be the project's identity and its path a mutable field, so
    that moving a repo does not break addresses. (04)
84. As an operator, I want a project whose path is missing to show `unavailable`, not run its
    tickets, and notify once, with `gx project set-path` as the fix, so that a moved repo fails
    visibly. (04)
85. As an operator, I want `gx project remove` to refuse while any of its tickets are queued or
    running and to keep its tickets in the store, so that removal never loses history. (04)
86. As an operator, I want `project.json` to override only a whitelist of `config.json` keys (vcs,
    trunk, landing policy, auto-ff-merge, agent/model, skills, notification destination,
    `execution-queue.max-agents-per-epic`, project `max-agents`) and reject others, so that a typo
    is an error, not a silent no-op. (04, 14)
87. As an operator, I want a ticket to start only when the global `execution-queue.max-agents`
    (default 4), `execution-queue.max-agents-per-epic` (default 2) and the optional project
    `max-agents` all have room, so that no project can starve the machine. (14)
88. As an operator, I want a slot to mean one live agent, with parked, waiting and landing tickets
    holding none, so that a stuck ticket never blocks the pool. (14)
89. As an operator, I want lowering a cap to stop new starts only, applied at the next scheduling
    decision without a restart, so that changing a cap never kills work. (14)
90. As an operator, I want `explain` to name every full cap with its count (`waiting: global cap
    4/4`), so that I know which cap to raise. (14)
91. As an operator, I want plain FIFO across projects with caps as the only protection, so that the
    order I see is the order things run. (14)
92. As an operator, I want one budget per budget day (local midnight), counting every ticket kind
    including one-offs and investigate tickets, kept in a persisted budget ledger of live cost
    deltas from S3 on (no interim since-server-start baseline), so that a restart cannot reset
    spend. (15)
93. As an operator, I want the soft limit to pause new starts across all projects and the hard limit
    to stop every live pane and park them `needs-repair` kind `budget-killed`, so that the budget
    holds machine-wide. (15)
94. As an operator, I want `budget-killed` tickets never auto-investigated, so that recovery does not
    spend money while over budget. (15, 10)
95. As an operator, I want `p` to keep its three states (budget-latched → `gx budget override`,
    else pause/resume) with user pause and budget pause independent and persisted until midnight, so
    that a restart never bypasses the budget. (15)
96. As an operator, I want `gx budget status --json` (today's total, limits, latch/override state,
    per-project breakdown) included in the snapshot, and the Queue tab header to show `today $X of
    $Y`, so that I see spend from anywhere. (15)
97. As an operator, I want the extra-usage subscription check run at server start and on every
    enqueue, with its banner on the enqueue modal, so that I am warned before paid usage starts.
    (15)
98. As an operator, I want the Queue tab to be global with a project prefix and filter, and the
    Tickets tab to default to the cwd project with an all-projects toggle, so that I can see
    everything or focus on one repo. (07, 12)
99. As an operator, I want a notification destination (transport + target) to be the unit of
    batching, gating and muting, with a `project.json` override replacing the global block, so
    that each project can talk to its own chat. (17)
100. As an operator, I want a batch that spans projects grouped under one header per project, so
     that a mixed batch stays readable. (17)
101. As an operator, I want existing transport-keyed mutes mapped to the global destination on load,
     and `gx notify --enable/--disable <transport> [--project <name>]` and per-destination
     `--status`, so that mute control keeps working. (17)

### One-offs (S5)

102. As a cron entry, I want `gx server one-off "<prompt>"` with only the prompt required (project
     from `--project`, else the cwd's project, else `scratch`), so that one line in a crontab is
     enough. (08, 11)
103. As an operator, I want repo-less one-offs to land in a built-in `scratch` project (`vcs: none`)
     that cannot be removed, so that "search the web and tell me" has somewhere to run. (01, 04)
104. As an operator, I want one-offs to default to commitless `type: prompt`, with `--commits`
     creating `type: implement`, so that a cron prompt never commits by accident. (08, 11)
105. As an operator, I want `--file <path>` to use a markdown file (frontmatter + body, flags
     override) as the payload, so that I keep reusable prompts in my dotfiles with no template noun.
     (08)
106. As a script, I want the default to print the address and exit 0, and `--wait` to block on the
     stream (surviving a server restart) and exit with a fixed code per outcome (see the exit-code
     table in S5) — from S6 waiting through a pending recovery and exiting 9 on escalation — so
     that a script can act on the result. (08, 10)
107. As a script, I want `--timeout <dur>` to exit with its own code while the one-off keeps running,
     so that giving up waiting never cancels work. (08)
108. As an operator, I want the agent to write its answer into a `## Result` section that `--wait`
     prints, so that a repo-less one-off is useful. (08, 11)
109. As an operator, I want parks always sent to chat and success sent only with `--notify` /
     `notify: true` (the truncated `## Result` plus the address), so that routine cron one-offs are
     silent. (08)
110. As a cron entry, I want a dedupe key (`--unique <key>`, default the file's absolute,
     symlink-resolved path, `--no-unique` to opt out) that refuses `duplicate-live` with the
     existing address while a copy is not `done` or `cancelled`, so that a broken one-off does not
     pile up. (08, 11)
111. As an operator, I want commitful one-offs to run in a worktree and land like an epic, commitless
     git one-offs to run in a detached worktree at the resolved base that is removed afterwards,
     and `scratch` one-offs to run in a kept per-ticket subdir deleted on archive, so that a one-off
     never touches my live checkout. (02, 08)
112. As a cron entry, I want a one-off submit to refuse `server-not-running` with no spool, so that a
     one-off never fires hours late. (08)
113. As an operator, I want one-offs at the queue tail by default and `--front` at the head, with
     `--front` never exceeding a cap, so that urgent work goes next without breaking limits. (08,
     14)
114. As an operator, I want a thin `/gx-one-off <addr>` skill (read body, do the work, write
     `## Result`, report iteration status) chosen by ticket type, so that one-offs do not carry the
     implement skill's TDD rules. (08, 11)
115. As an operator, I want `submitted` and `submit-refused` (kind `duplicate-live`) events, and
     result notifications logged as `notification-sent` with `notify_kind=result`, so that one-offs
     are observable like any ticket. (13)

### Auto-investigate (S6)

116. As an operator, I want a recovery catalog published by `gx recovery catalog --json`, each entry
     with an id, a `(type, kind)` signature plus predicates, `executor: rule | agent`,
     `authority: low | medium | high`, allowed remedy verbs, and `enabled`, so that recovery
     behavior is data I can read and switch off. (10)
117. As an operator, I want a global `recovery.enabled` kill switch and per-entry disable, so that I
     can stop all automatic recovery at once. (10)
118. As an operator, I want recovery rules to apply low/medium-authority remedies in Go with no pane
     and no tokens, so that common failures heal for free. (10)
119. As an operator, I want high-authority remedies never auto-applied but proposed with the exact
     verb (a `recovery-proposed` event and a `## Proposed Remedy` section), and approved with `A`
     on the Queue tab (`gx server tickets approve <addr>`, which runs the latest proposal and
     refuses `proposal-stale` if the ticket changed since), so that work-destroying actions always
     have a human beat. (10, 11)
120. As an operator, I want recovery to trigger on every `needs-repair` kind, on `needs-answer` kind
     `zero-commit`, on `blocked-pane` only for an allow-listed dialog, and to diagnose only on a
     `deadlocked` event (emitted by the scheduler when an epic enters deadlock, kind `all-parked`
     or `blocked-cycle`) — and never on `self-reported` or commitless finishes, so that recovery
     acts only where it can help. (10)
121. As an operator, I want recovery rules to call the same server verbs as everyone else
     (`via: server`, `actor: recovery`), so that recovery has no private write path. (10, 11)
122. As an operator, I want an investigate ticket to be a commitless, fork-lettered child of the
     failed ticket, enqueued `--front`, running in a detached worktree at the feature-branch tip
     (else the failed ticket's resolved base; in a `vcs: none` project, `scratch-workspace/<slug>/`),
     with the parent shown waiting-for-children, so that recovery can never land code. (10, 14)
123. As an operator, I want `type: investigate` tickets to be non-investigatable, and any ticket to
     opt out with `recover: false`, so that recovery never recurses. (10, map)
124. As an operator, I want at most one automatic recovery per `(ticket, kind)` and three per ticket,
     counted from the event log, and a re-fail in the next iteration treated as a failed recovery
     and escalated with both failures, so that recovery cannot loop. (10)
125. As an operator, I want `recovery-matched`, `recovery-applied` and `recovery-escalated` events in
     the event log of the affected ticket's top-level ticket (epic or one-off), so that every
     recovery is auditable. (10, 11)
126. As an operator, I want an unmatched or judgment failure to produce a `## Result` report with
     Observed / Evidence / Diagnosis / Proposal (`catalog-entry`, `orchestrator-fix`, or
     `human-only`), so that I get a diagnosis, not a guess. (10)
127. As a gx developer, I want `catalog-entry` and `orchestrator-fix` proposals auto-filed as
     `draft` research tickets in a configured follow-ups epic (default `gx:follow-ups`, created as
     `draft` if missing), deduped by `(type, kind)` and proposal class into a `## Comments`
     occurrence line, and attached only to the escalation message when that project does not exist,
     so that the catalog grows from real failures. (10)
128. As an operator, I want the original park notification held while recovery runs (sent anyway
     after 10 min, a config key), silence on success, and one message on escalation, so that a
     healed failure never pages me. (10)
129. As an operator, I want lost-commit tickets with no iteration branch to escalate only, so that
     recovery never invents a commit range. (10, map)
130. As an operator, I want `gx server tickets nudge <addr>` to be the only way to nudge a pane, so
     that every nudge is logged, streamed and rate-limitable. (05, 10)
131. As an operator, I want `gx-investigate` to gain an unattended mode limited to the matched
     entry's remedy verbs, while `m → Investigate` keeps opening an attended tab, so that the same
     skill serves both. (10)
132. As an operator, I want the interim pane-nudge grant removed from `gx-investigate` once the nudge
     verb and rules R2/R5 ship, so that no agent calls herdr directly. (12, 10)

### Migration and cutover

133. As an operator, I want the stages split into five standalone, releasable epics that I launch
     by hand in order, so that the loop cannot run past a by-hand step. Done when
     `gx tickets validate` passes on every ticket of the five epics. (12, amended)
134. As an operator, I want the store move to happen by hand between the foundation and server
     epics, so that nothing is `claimed` while tickets move. Done when `gx tickets root` points into
     the store, `gx tickets validate --all` passes, and the server epic's first tickets have run
     end to end from the store. (12, amended)
135. As an operator, I want the scheduler change to happen by hand between the server and cutover
     epics. Done when `gx server status` reports running, `orchestrator` is `server`, and the next
     epic is in the server's queue. (12, amended)
136. As an operator, I want the cutover epic published with the others, so that the dual path
     cannot be forgotten. Launching it after one week with no rollback is a staging rule, not
     product behavior. (12, amended)
137. As an operator, I want cutover to remove the in-process loop, loop registry and quit guard from
     the TUI, and delete the run-start modal, drain-and-replace, attach lock, reattach flows,
     QueueStore, `foldscratch`, cost baselines, the aliases and the switch, so that no second
     orchestration path survives. (12)
138. As an operator, I want the old `gx tickets land/reset/unpark/verify` to stay as hidden aliases
     with a warning until cutover, so that my muscle memory and skills keep working while I
     migrate. (11)
139. As a reviewer, I want each stage's ticket group to include a ticket that applies that stage's
     slice of the CONTEXT.md patch, so that the glossary always describes the code that exists.
     Done when the slice's entries are in CONTEXT.md once the stage's tickets are done. (11)

### Added in review (S2–S4)

Numbered after 139 so that earlier story numbers stay stable.

140. As an operator, I want `gx server status` to warn when no ticket-store push remote is
     configured, so that I know my ticket history has no backup. (03)
141. As an operator, I want the S2 read-only server to run beside the in-process loop without
     claiming or writing, so that I can try the server before it schedules anything. (12)
142. As an operator, I want each root row in the Queue tab to show that root's cost for today, so
     that I see which epic or one-off is spending. (15)
143. As an operator, I want `gx budget override` to be a server write with a `--json` result or a
     refusal, so that lifting a budget latch goes through the same guards and events as every other
     orchestration change. (15, 05)

## Implementation Decisions

Each decision names the stage that owns it. Where the map fixes the stage, the bullet cites the
ticket. Where no ticket fixed it, the user confirmed the stage in the design review and the bullet
says **(stage confirmed in review)**; a stage still open says **(stage inferred)** and is listed in
Further Notes. Code names that the map calls "node" stay `node` only as Go identifiers.

### Staging rules (12)

- **Five standalone implementation epics** (amends ticket 12's "one implementation epic" with draft
  gate tickets). Each epic is releasable on its own and launched by hand, in this order:
  1. `orchestrator-daemon-1-foundation` — S0 + S1.
  2. `orchestrator-daemon-2-server` — S2 + S3.
  3. `orchestrator-daemon-3-cutover` — Cutover.
  4. `orchestrator-daemon-4-projects-one-offs` — S4 + S5, plus dropping the old cap-key aliases
     (it needs S4's cap rename, so it cannot ship with the cutover epic).
  5. `orchestrator-daemon-5-recovery` — S6.

  There is no cross-epic `blocked_by`: launch order covers it, and a ticket that builds on an
  earlier epic's work says so in prose. Every ticket is published `open`; there are no draft gate
  tickets. Each epic ends with one trailing `type: code-review` ticket. Ralph-loop builds all code,
  including the cutover deletions.
- **Stage spine**: S0 observability → S1 ticket store + terminology → S2 read-only server + harness
  → S3 server orchestrates migrated projects → Cutover → S4 multi-project → S5 one-offs → S6
  auto-investigate. Cutover follows S3 and now precedes S4.
- **By-hand steps between epics**:
  - **After foundation**: install the binary → make sure no ticket is `claimed` →
    `gx tickets migrate --dry-run` → `gx tickets migrate` (this carries the already-published later
    epics into the global store) → check `gx tickets root` and the epics. The server epic's first
    tickets double as the S1 smoke run; fix forward.
  - **After server**: install the binary → `gx server install` → start the server → set
    `orchestrator: server` → enqueue the next epic on the server. If the server is broken the run
    stalls; flip back and the old loop continues.
  - **Cutover epic**: launched after the server has run one week with no rollback.
  - The projects/one-offs and recovery epics need no by-hand step beyond launching them.
- **Temporary dual path**: global `config.json` key `orchestrator: in-process | server` — not on
  the `project.json` whitelist, so a machine has exactly one scheduler. The S2 read-only server may
  run beside the in-process loop. From S3, the scheduler that is not selected refuses to claim.
  While the switch says `in-process`, a running server (launchd may keep it alive after a rollback)
  refuses every claim-affecting write (enqueue, cancel, one-off, queue changes) with
  `scheduler-not-selected`; its reads stay available. The TUI then runs in its old in-process mode
  and hides the server indicator.
- **Usable** at every stage means: a ralph-loop epic on gx itself runs end to end. Rollback before
  the S1 smoke run (the server epic's first tickets) = reinstall the previous tagged binary; after
  it, fix forward (no reverse migrate). From S3, rollback = flip the switch.
- **Live state**: no migration code. `queue-state.json` is not migrated (re-enqueue by hand after the
  server epic); the attach lock stays as the exclusion guard until cutover deletes it.
- **Other machines** run `migrate --dry-run` then `migrate` after their own foundation install. A ticket-store
  push remote is a one-way backup, never shared between machines.
- **CONTEXT.md** is amended per stage, from ticket 11's prepared patch, when each concept ships.
  ADRs are written at the same time. Slice mapping **(stage confirmed in review)**: S1 — Ticket, Epic,
  Ticket store, Address, Project / project name / project file, Event log, Children un-retired,
  `type: implement`; S2 — Server, Server lock, Snapshot, Explain; S3 — Direct write / Server write,
  View model, Server indicator, Launch, Reclaim, server-wide Queue, Parked/Deadlocked as epic
  states, Base / Target, Fix ticket, Budget day / budget ledger; S4 — Slot / Cap, destination; S5 —
  One-off, Dedupe key, Result; S6 — Recovery rule, Investigate ticket; Cutover — the removals
  (Attach family, Epic run, Reattach, Live, drain-and-replace, run-start modal), each leaving an
  `_Avoid_` pointer.

### Config keys (04, 13, 14, 15)

One table of the keys this spec adds or renames. Keys a ticket did not name are this spec's choice.
`project.json` uses the same key paths as `config.json`, except the project cap.

| Key | File | Default | Stage |
| --- | --- | --- | --- |
| `orchestrator` (`in-process` \| `server`) | `config.json` only | `in-process` | S3 |
| `ticket-store.path` | `config.json` only | `<data dir>/tickets` | S1 |
| `ticket-store.commit-debounce` | `config.json` only | 60 s | S1 |
| `ticket-store.push-remote` | `config.json` only | unset | S3 |
| `server.tcp-listen` | `config.json` only | off | S2 |
| `server.auto-merge-epic` | `config.json` only | off | S4 |
| `execution-queue.retry-storm-launches` (N) | `config.json` only | 3 | S0 |
| `execution-queue.spin-cycles` (M) | `config.json` only | 3 | S0 |
| `execution-queue.spin-window` | `config.json` only | 5 min | S0 |
| `execution-queue.max-agents` | `config.json` only | 4 | S4 |
| `execution-queue.max-agents-per-epic` | both | 2 | S4 |
| `max-agents` (project cap) | `project.json` only | unset (no cap) | S4 |
| `budget.*` (existing) | `config.json` only | — | S3 (ledger) |
| `vcs`, `trunk`, `landing.policy`, `landing.auto-ff-merge` | both | `git`, repo default, none, off | S4 |
| `agents.*`, `skills.*`, `notifications.*` (existing) | both | — | S4 |
| `recovery.enabled` | `config.json` only | on | S6 |
| `recovery.notify-hold` | `config.json` only | 10 min | S6 |
| `recovery.follow-ups` | `config.json` only | `gx:follow-ups` | S6 |

### S0 — Observability on today's ralphloop (13, 09)

- **One park path.** A single `park()` function handles every park and terminal outcome
  (`needs-answer`, `needs-repair`, `done`, `cancelled`): it writes the ticket, appends exactly one
  event, and fires the notification. The loop's catch-all that today parks `needs-repair` without
  logging goes through it.
- **Event shape**: one event type per outcome (today's types plus `launch-failed`) and a required
  `kind` from a closed enum (`agent_name_taken`, `zero-commit`, `handle-mismatch`, …). Signatures
  match `(type, kind)`, never `reason`.
- **Required fields** on every failure event — a fixed set: `type`, `kind`, `address` (ticket id
  until S1, the canonical address after), `iteration` when known, `attempt` when applicable,
  truncated `reason`. Unknown fields are omitted. `kind` is required on failure and recovery events
  only; other events (`submitted`, `iteration-started`, …) omit it. Every event stays under
  `PIPE_BUF` (4096 B).
- **`cause: herdr`** is an attribute of a `kind` in the enum, not an event field: it marks the
  kinds a herdr outage can cause (the `launch-failed` herdr error codes, `handle-mismatch` raised
  during an outage). S3's outage fold holds parks on that attribute.
- **`park_kind`** is written twice: frontmatter (current state, cleared on resume) and the event's
  `kind` (history). Recovery reads the log.
- **`launch-failed`**: one per failed attempt, `kind` = herdr error code, plus `attempt` and the
  iteration label. Closes the pre-`iteration-started` blind spot.
- **Retry-storm cap and spinner quarantine**, sequenced after `park()` and `launch-failed`: N
  consecutive `launch-failed` → park `needs-repair` kind `retry-exhausted`; M park/re-claim cycles in
  a window → park kind `spinning`. N, M and the window are config keys with defaults (see Config
  keys). Ticket 09's R1 signature (≥3 cycles in 5 minutes) is the data for the spin default. N
  defaults to 3 (one more than today's in-iteration `maxLaunchAttempts` of 2, a different counter);
  no data backs it yet, so revisit it from S0's event-log data before S6.
- **One Go package owns the event contract**: event types and `kind` enums. The file log and (from
  S2) the event stream serialize the same types; the durable event log is a subset of the stream.
  The `kind` enum is published by a CLI verb, like `gx tickets schema`.
- **Content-aware transcript reader** follows S0 and is not a prerequisite for it. It extends the
  transcript module (one parser), is opt-in, and keeps only the last assistant text, truncated. It
  is an explicit `blocked_by` of S6's R2 catalog ticket (10 handoff) **(stage confirmed in
  review)**.
- Later stages add their own event types to the same package: `reclaimed` (13 §9's `readopted`,
  renamed under 11's vocabulary), `herdr-unavailable` / `herdr-available` on transition only,
  `submitted`, `submit-refused`, `deadlocked`, `recovery-*` (incl. `recovery-proposed`), budget
  threshold/soft/hard events, and the `ticket-changed` and explain-verdict-change stream events
  (06, 07, 08, 10, 13, 15). These are wire codes, fixed before S0 ships.

### S1 — Global ticket store and terminology (03, 11, 12, 16)

- **Data dir and state dir** (03 §11): XDG-style paths on both Linux and macOS, resolved once by
  the `config` package, which honors `XDG_DATA_HOME` / `XDG_STATE_HOME`. The **data dir** is
  `~/.local/share/gx/`; it holds the ticket store (`tickets/`) and, beside it and never inside the
  store's git repo, `scratch-workspace/` (S5). The **state dir** is `~/.local/state/gx/` (see S2).
- **Ticket store**: one global git repo, default `<data dir>/tickets/<project>/...`, a
  config key resolved by the `config` package. `git.Repo.ScratchRoot()` is the seam; the known
  bypasses (`defaultScratchDir` in ralphloop, the Tickets UI model, the logger's `gx.log`) move with
  it. `gx tickets root` points into the store. The in-process loop keeps running, now against the
  store.
- **Markdown is truth, the server is an index** (03 §2). Everything a ticket file can say, the file
  is authoritative for. Hand editing keeps working: on every rescan, the file's state overrides the
  server index.
- **On-disk shape** (11 §6): only top-level tickets are directories, each with `ticket.md`. An epic's
  `ticket.md` replaces `map.md` + `epic.yaml` (frontmatter: `status`, `blocked_by`, `base:`, timing
  fields; body optional — a plan or spec link). Everything below a top-level ticket is a flat file
  in its `issues/`, linked by `parent:`, fork-lettered; a forked parent never becomes a directory.
- **Model** (01): one ticket type for every unit of work; project is a container, never a ticket,
  and never null; `parent` is the one containment edge (epic membership becomes a real edge, not
  directory position); no `forked_from` edge. If provenance is ever needed it is a scalar field
  (`origin: decompose | fork | recovery | review-fix`), never an edge. Liveness stays a separate
  axis from `status` (`iteration_status`, reclaim) and is never folded into the enum.
- **`cancelled`**: a new terminal status distinct from `done` (01 §4). S1 owns the schema value,
  since 16 §9 already treats it as terminal at S1 **(stage confirmed in review)**; the cancel verb
  is a server write and lands in S3.
- **Event log**: one event log per top-level ticket (epic or one-off; today `run-log.jsonl`) in that
  ticket's directory in the store. Events of every ticket below it go there. Events with no
  address (`herdr-unavailable`, budget latches, server crash/restart) go to one **server event
  log** in the store, outside any project. Both are committed; a top-level ticket's log is
  compressed when it is archived (03 §3). Distinct from `server.log`. Ticket 11's glossary line
  ("the typed per-ticket event record") changes to match.
- **Address**: canonical `project:epic/06`; short forms `06` / `epic/06` are input only and are
  resolved before being stored (03 §5).
- **Agents use verbs, by address** (03 §4): `gx tickets show <addr>`, `gx tickets add <parent-addr>
  --slug … --body -` (create with body in one call, no draft→open two-step for agents),
  `gx tickets section <addr> "<Heading>" -`, `gx tickets set <addr>` (address or path). Iteration
  prompts send `/{skill} <addr>`. The skill/prompt path audit is S1 work (12). `root` / `epics` stay
  for humans and scripts. Scratch folding becomes deletable (deleted at cutover).
- **Per-ticket file lock** (05 §3): every writer of a ticket file takes it from S1 — the in-process
  loop, the CLI direct verbs, and later the server. It also closes the fork-clobber race (09 R11)
  **(stage confirmed in review: S1; the server ping is S3)**.
- **Trailer** (02 §10, 03 §5) **(stage confirmed in review: S1)**: `Ralph-Loop-Ticket` stays; its
  value becomes the ticket's canonical address from S1, when addresses exist (the S1 smoke run
  exercises it). The parser keeps accepting `<featureBranch>/<id>` for history.
- **Store commits during S1–S2** (03 §6): one shared commit-loop package — debounced and immediate
  commits as in S3's Store commits — run by the in-process loop in S1 and moved into the server in
  S3. There are never two committers: before S3 only the in-process loop commits, and from S3 the
  `orchestrator` switch picks one. The S2 read-only server does not commit.
- **Widened `gx tickets migrate <old-.scratch>`** (11 §4, 12): legacy frontmatter fixes,
  `task → implement`, copy into the store (including `.archive` and event logs), `epic.yaml` /
  `map.md` → `ticket.md`, create `project.json` if missing (name from the repo or `--project`),
  validate every migrated ticket (failures reported by address, non-zero exit). Idempotent: refuses
  per existing address. Refuses only while a ticket is `claimed`. `--dry-run` converts and validates
  in a temp dir. Migrate **copies**; the old `.scratch` stays as a read-only backup until cutover.
- **`type: task` → `type: implement`** (11 §5): 728 files rewritten by migrate; the loader accepts
  `task` as an alias until cutover and rejects it after. Only the stub default in `gx tickets add`
  switches on the string. Update `gx-to-tickets`, `gx-local-tracker.md` (also add the missing
  `conflict-resolution` to its enum), and the skills bundle test fixture.
- **`gx merge` JSON `base` → `target`**, plus the `gx-merge` / `gx-cleanup` skills (11 §7)
  **(stage confirmed in review: S1 "terminology")**.
- **`blocked_by` resolver** (16, S1 part): one shared resolver feeds both `validate` (error,
  non-zero exit) and the loader/scheduler (ticket renders `Error`, never runnable), with the same
  verdict and message. Dangling bare ref → `Error` (replacing today's silent `blocked`). Cycle
  detection over `blocked_by` + ancestry (A↔B at any depth, blocked by own ancestor, blocked by own
  descendant), reporting every edge, like the parent-graph cycle check. `validate <addr|path>`
  loads the whole project; `--all` walks the store; migrate's validate step reuses it. `done` / `cancelled` tickets
  skip graph checks (per-file checks still run). Before S3, qualified refs (`epic/06`) are rejected
  as malformed.
- **`project.json`** (04 §1, §10): a project **is** `<store>/<project>/project.json`, committed with
  its tickets; JSON, using the `config` package's pointer-field partial struct. S1 creates it via
  migrate. Until S4, migrate is the only way to register a project: S3 orchestrates every project
  that has a `project.json`, and gx run in a cwd with no project refuses with a "run
  `gx tickets migrate`" hint. `gx project add`, the other project verbs and whitelist enforcement
  land in S4 **(stage confirmed in review)**.

### S2 — Read-only server, lifecycle, and test harness (05, 06, 12)

- **Process and supervision** (06 §1): `gx server install` writes a launchd agent with `RunAtLoad`
  and `KeepAlive = {SuccessfulExit: false}`. With the plist installed, `start` / `stop` / `restart`
  go through `launchctl` (`kickstart` / SIGTERM), the only start path. Without it, `start`
  self-detaches (setsid, closed inherited fds, own log, no controlling terminal). macOS/launchd only
  for `install`, shaped so a systemd user unit can be added later.
- **State dir** (03 §11): `~/.local/state/gx/` on both Linux and macOS (`XDG_STATE_HOME` honored),
  resolved via `config`, created mode `0700`. Holds the socket, the server lock, `server.log`,
  `server.stderr`, and from S3 the queue, iteration handles, land locks, the budget ledger, and
  `gx.log`. Never inside the ticket store or the data dir.
- **Server lock** (04 §6): flock on a lock file in the state dir holding the pid. A second server
  fails `already running (pid N)`. `gx server start` on a running server prints that and exits 0.
- **Stop** (06 §2): SIGTERM → stop claiming → let an in-flight land finish (short, bounded) → flush
  the store commit loop (from S3; the S2 read-only server never commits) → release the lock → exit
  0. Live iterations keep running in herdr. No `--drain`.
- **Upgrade = restart** (06 §5): no hot-swap or self-restart. `gx server status` warns when the
  on-disk binary differs from the running build, and when no store push remote is configured (03
  §7).
- **Logs** (06 §6): `slog` JSON at `server.log`, `info` default, size-rotated (~10 MB × 3); launchd
  stdout/stderr → `server.stderr`. `gx server logs [-f] [--level] [--json]` pretty-prints by default.
- **herdr down at start** (06 §8): start anyway; report `herdr unavailable` in status and the
  stream; retry herdr every 30 s.
- **Transport** (05 §1): one HTTP/JSON API; unix socket always on, mode `0600` inside the `0700`
  state dir; loopback TCP opt-in via config (off by default), same handlers and routes, and
  `gx server status` warns while it is on. No auth (out of scope); TCP binds loopback only.
- **Route shape** (05 §2): verb routes mirroring the CLI 1:1, versioned under `/v1/`, keyed by
  address. No generic PATCH. Every response is a JSON result or `{"refused":true,"reason":<stable
  code>,"message":…}` (the shipped tier-2 contract). Ticket 05's `nodes` resource and `/v1/jobs`
  submit route (words ticket 11 retires) become: `/v1/tickets/{addr}/<verb>`, `/v1/one-offs`,
  `/v1/projects/…`, `/v1/budget/…`, `/v1/queue/<verb>`, and `/v1/events/…`. The verb segment is
  the CLI verb from the naming table in S3 (e.g. `/v1/queue/move`, `/v1/tickets/{addr}/relaunch`).
- **Versioning** (05): the handshake returns an integer API version and a build id. Same version,
  different build → works, with an "older build" hint. Different version → client goes read-only
  and says `gx server restart`.
- **Event stream** (05 §8): SSE on both listeners, a server-wide sequence number on every event.
  Clients `GET` the snapshot (state + seq), then subscribe from that seq. No replay across a server
  restart; clients re-snapshot. Ephemeral events (pane activity, index refresh) are stream-only.
  A subscriber that falls behind its buffer is dropped by the server. A client re-snapshots on any
  reconnect and on any sequence gap (seq not contiguous).
- **Read surface** (05 §9): tickets (tree, by address), queue, live iterations (pane, worktree,
  branch, resolved base), projects with state, locks with holders, event history filtered by
  address, agent transcript paths (never parsed by the API — the transcript module owns parsing).
- **Explain** (05 §9): `GET …/explain` for one ticket, computed by **the same function the
  scheduler runs**, never a parallel copy. In S2 that function runs against the store while the
  in-process loop still schedules, so it returns only store-derivable verdicts (blocked by X,
  `Error`, parked + reason, draft) and `unknown: in-process scheduler` for queue and slot verdicts.
  Full verdicts from S3.
- **CLI parity** (05 §10): every endpoint has a CLI verb with `--json`, reads included
  (`gx server status`, `gx server tickets explain | history | follow <addr>`). Agents
  and cron use the CLI, never raw HTTP; skills stay CLI-only. A CI test diffs the route table
  against the command tree.
- **Restart rebuild** (03 §13): recompute the index by rescanning the store; reload queue, order
  and iteration handles from the state dir (from S3); do not replay the in-memory stream. One
  recursive watch on the store plus a guaranteed poll (ADR 0025's pair) replaces N per-repo
  watches.
- **Test harness** (12 §6), shipped in S2 as a prerequisite: an in-process server on a temp state
  dir and temp socket, the `run_realgit` fake herdr, driven only through the API client. See
  Testing Decisions.

### S3 — Server orchestrates migrated projects; TUI becomes a client (05, 06, 07, 02, 01, 16, 17, 15)

- **Ralphloop is copied into the server** (07 §1, 12 §2): `ralphloop`, the loop registry,
  scheduling/backfill, cost aggregator, budget checks and hard-limit kill, chat sinks, attach-lock
  use, queue writes, and herdr liveness probes run in the server when the switch says `server`.
  The TUI's in-process path stays, behind `orchestrator: in-process`, only so that rollback is a
  switch flip; cutover deletes it (12 §8). Ralphloop unit tests move with the package; the
  `run_realgit_*` scenarios are ported onto the S2 harness.
- **Projects** (04): the server orchestrates every project that has a `project.json` (created by
  migrate until S4's `gx project add`).
- **Budget ledger** (15) **(pulled into S3 in review)**: the per-day budget ledger described in S4's
  Budget bullet ships with S3, so the server never counts "since Attach" and has no interim
  since-server-start baseline. S4 adds the multi-project parts (per-project breakdown, global soft
  limit across projects).
- **Server-wide queue** (03 §10, 04 §8): one ordered queue of addresses in the state dir. Queue
  intent is never a markdown field. TUI checkbox selection stays TUI-local.
- **Write split** (05 §3, 11 §3):
  - **Direct writes** — body, sections, `draft ↔ open`, agent `--iteration-status` — go to the file
    under the per-ticket lock, then ping the server ("address changed"). Same path whether the
    server is up or down.
  - **Server writes** — claim, done, park, cancel, queue changes — go through the server only. The
    server writes the file before it acks, so markdown stays truth.
  - Every `--json` result carries `via: direct | server` and `actor: human | agent | recovery`.
- **Cancel vs delete**: cancel changes status; delete (`x`, `gx server queue remove`) changes only
  the queue. Cancelling a `claimed` ticket with a live pane refuses `ticket-live` unless `--stop`,
  which stops the pane the way the budget hard kill does, then cancels. Cancelling an epic cancels
  all of its non-terminal descendants.
- **CLI namespaces** (11 §4, 05 §12): `gx tickets show | add | section | set` are direct writes;
  `gx tickets set --status` loses the orchestration statuses. `gx server tickets` and
  `gx server queue` verbs (naming table below) are server writes and reads (nudge and approve ship
  in S6). Help groups commands as "Ticket content (direct)" and "Server". The
  shipped `gx tickets land/reset/unpark/verify` become hidden aliases with a warning until cutover.
- **Naming table** for every server verb; each route mirrors it (`/v1/queue/move`,
  `/v1/tickets/{addr}/relaunch`, …):

  | Namespace | Verbs | Covers |
  | --- | --- | --- |
  | `gx server queue` | `add \| remove \| move \| replace \| drain \| pause \| resume` | enqueue, dequeue / delete with cascade, reorder, per-project replace, drain, server-wide pause |
  | `gx server tickets` | `land \| reset \| unpark \| verify \| park \| cancel \| nudge \| relaunch \| approve \| explain \| history \| follow` | repair, status, iteration relaunch, approval, reads, event-follow |
  | `gx server one-off` | — | submit (S5) |
  | `gx server events` | `kinds` | publish the `kind` enum |
  | `gx project` | `add \| remove \| set-path \| list` | registration (S4) |
  | `gx budget` | `status \| override` | budget (S3, with the ledger) |

  `gx server tickets enqueue` (11 §4) is `gx server queue add`.
- **Repair verbs** (05 §4–§5): `verify | land | reset | unpark` stay one shared package that the CLI
  and the server both call. The package owns guards, reason codes, and the land lock (state dir,
  keyed by the epic's root address, since it serializes landings onto one feature branch; it
  records its owner), so a CLI land and a server land cannot race. Server down → only these four run
  direct under the land lock, printing `server not running — ran directly under land lock` (and
  `via: direct`); every other server verb refuses `server-not-running`.
- **Atomic park `needs-repair`** with a required one-line reason is an API verb (05 §6), closing the
  gap that the CLI cannot write it.
- **Caller guards run in the server** (05 §11): the client sends caller context (cwd, branch); the
  server evaluates guards (e.g. the `ralph-loop/*` refusal) from the shared package.
- **Not running → refuse** (06 §7): CLI verbs refuse `server-not-running` with a `gx server start`
  hint and never auto-start. The TUI confirms "server is down — start it now?".
- **Reclaim** (06 §3): for each reloaded handle — pane exists, agent name matches, worktree exists →
  reclaim, no confirm. Any mismatch → park `needs-repair` kind `handle-mismatch` with a
  `## Needs Repair` section. The Queue-tab reattach confirm is retired.
- **Crash mid-land** (06 §4): the in-flight signal is a land lock held by a dead owner, read from
  the shipped land-lock owner record and its liveness check. The shipped land marker is not that
  signal: it records a conflict a land left behind, not a land in progress. On start, before
  scheduling, for each such lock: clear the orphan lock, then run the `verify` logic for that
  epic — clean landed / clean not-landed → finish or roll back; ambiguous → park `needs-repair`
  kind `ambiguous-land`.
- **Store commits** (03 §6–§7): from S3 the selected scheduler — the server when the switch says
  `server` — is the only committer of the ticket store (see S1 for the shared commit loop).
  Debounced (60 s after the last change, a config key) plus an immediate commit on `done`,
  `cancelled` or any park.
  Edits made while the server was down are committed on the next start. Message `<addr>: <status
  change>` or `sync: N files`. Optional push remote, best-effort, after each commit; a failed push
  is logged and never blocks.
- **Claim re-read** (03 §8): claim re-reads the ticket file and refuses if it changed.
- **`blocked_by`, S3 part** (16 §8, 01 §5–§6): qualified grammar `epic/06` (and the reserved
  `project:epic/06`, which is an error: "cross-project blocking not supported"; the ticket's own
  project prefix is accepted), cross-epic cycles, `base:` and its checks, the `gx-local-tracker.md`
  line (block on another epic with `blocked_by: [epic/06]`, never park as a stand-in — no migrate
  code for the `needs-attention` workaround), and `gx tickets schema` text. A ticket is runnable
  only if no ancestor is blocked and none of its own blockers is blocking; children never copy an
  ancestor's `blocked_by`. After S3, a rollback to the in-process loop treats qualified refs as
  `Error` (fail closed).
- **Base and target** (02, 01 §7): every commitful ticket has a base and a target, default equal
  (leaf → parent's branch; root → trunk; PR chain → the blocker's feature branch). Only `base:` is
  declarable (branch name or ticket reference); target is derived on every read, never stored.
  Derivation keys on `IsCommitless()`, not on type, and runs at claim time (02 §1–§2):
  - exactly one unlanded same-project commitful blocker → base = that blocker's branch;
  - blocker landed, or no commitful blockers → the default base (leaf → parent's branch, root →
    trunk);
  - `base: <ticket ref>` → the referenced ticket's landed base if it landed, else its branch;
  - two or more unlanded commitful blockers without `base:` → park `needs-answer` naming the
    blockers; `validate` warns.

  Automatic derivation applies only to roots (epic-level `blocked_by`) and to leaves blocked by a
  leaf in the same epic. A leaf with a cross-epic commitful blocker gets no derived base: it waits
  until that blocker lands, then uses its default base; `validate` warns. Commitless tickets have
  target none and read at the parent's branch tip; `base:` on them, or in a `vcs: none` project, is
  a validation error. Cross-project blockers never derive a base.
- **`resolved_base:`** (02 §3, 03 §12): frontmatter (ref + SHA) that the server re-stamps at every
  claim, like `actual_context_window`. Humans may edit the file, so `validate` warns when it
  differs from the last `claimed` event.
- **Feature branch creation** moves to the server (02 §7): created from the root's resolved base when
  its first child is claimed, named after the ticket's slug; a same-named existing branch is adopted
  and its merge-base with the base recorded as the resolved base.
- **Landing** (02 §8): no merge ticket type; the server calls the deterministic `gx merge` core
  (ff-only, never rebases, ADR 0015) as a root's final step. `needs_rebase` → park the root; a human
  runs `gx-merge`.
- Multi-epic chaining is not built separately — it falls out of `blocked_by` + base derivation
  (02 §9). The trailer change ships in S1; landing policy in S4.
- **TUI as a client** (07):
  - The TUI keeps a **view model** only: snapshot + stream events reduced into render state;
    `reduceLiveEvent` survives as a pure reducer over server events. In server mode `CanQuit` does
    not guard quit; the quit guard itself is deleted at cutover.
  - **Server indicator** on every tab: `server ● pid N` / `○ down` / `◐ read-only — restart` /
    `⚠ herdr unavailable`. "(attached)" is deleted.
  - **Server down**: Queue tab clears with a "server down — `s` to start" banner, background
    reconnect, re-snapshot on reconnect. Tickets tab reads markdown; live columns (iteration, pane,
    cost) are blank, not stale. Server keys and menu entries are greyed with the reason when down or
    read-only.
  - **Refresh**: connected → stream events only; the TUI runs no fsnotify watch and no poll timer
    (no 2 s/30 s poll, no 300 ms registry poll). Down → the Tickets tab falls back to its own
    watch + 30 s poll against the store (read-only degrade, not an orchestration path).
  - **Keys** (07 §6): `a` enqueue checked tickets (always allowed); `r` atomic per-project replace of
    pending entries, refused `ticket-live` (07's `node-live`, renamed under 11) for a checked
    ticket with a live iteration in its subtree; `D` drain; `s` one menu routed by write kind; `p`
    server-wide pause/resume; `c`/`C` dequeue; `x` delete with cascade and the live refusal
    (queue-only — cancel is a status change); `enter` on a parked row → unpark; `A` on a row with a
    pending recovery proposal → approve (S6). Agent chosen at enqueue (`--agent`, default from
    project config), picker in the `a`/`r` confirm. `m` Answer… is a direct write; Answer in pane reads the
    pane id from the live-iterations read; Resume → unpark; Investigate / Unmute & Reopen kept.
    Drain-and-replace and the run-start modal are retired.
  - **Toasts** from stream events only, never from a snapshot, no replay after reconnect. Chat is
    sent once by the server.
  - **Pending rows** show the `explain` verdict delivered with snapshot/events, never polled per row.
- **Notifications in the server** (17 §8, S3 part): the chat sink moves into the server unchanged
  (one destination ⇒ one batcher, one gate). Server-level events (crash/restart, herdr
  unavailable/back, budget latch, daily summary) go to the global destination only. **Outage fold**:
  while "herdr unavailable" is active, parks whose `kind` carries `cause: herdr` (S0) are held; one
  server message when the outage starts, one digest when herdr is back or after a 10-minute cap,
  each held park delivered to its own project's destination. Every notification is prefixed with
  the project name (04 §12).
- **Switch**: from S3 the non-selected scheduler refuses to claim (12 §2); a non-selected server
  refuses claim-affecting writes with `scheduler-not-selected` (see Staging rules).
- **Cutover epic** is published with the others, `open`, and launched by hand (see Cutover below).

### Cutover (12 §8)

Its own epic, launched by the human after S3 has run one week with no rollback; it ships before S4.
Deletes:

- The TUI's in-process loop, loop registry, `CanQuit` / quit guard, run-start modal,
  drain-and-replace, TUI cost aggregator and event sinks.
- The attach lock, the Queue-tab reattach flow, the reattach scan, the reattach confirm flow.
- `queue-state.json` / QueueStore, scratch folding.
- Cost `baselines` map, attach-count poller hooks, the `.session/run-log.jsonl` budget trail.
- Aliases: `type: task` (rejected after), hidden `gx tickets land/reset/unpark/verify`,
  `max-concurrent-tickets-per-epic`, the `max-concurrent-epics` warning. The two cap-key aliases
  are dropped in the projects/one-offs epic instead, since S4's cap rename creates them.
- The old epic shape (`epic.yaml`, no `ticket.md`): test fixtures migrate to `ticket.md`, then the
  loader's old-shape reader is deleted (`gx tickets migrate` keeps converting old trees).
- The `orchestrator` switch and the in-process orchestration path.
- Old per-repo `.scratch` trees — a by-hand step after the dogfood week.
- **Kept**: the idle-cost watch + poll code, as the TUI's server-down fallback (07).

### S4 — Multi-project (04, 14, 15, 17, 07)

- **Registration** (04 §2–§5, §13): `gx project add [path] [--name] [--vcs none]`, path defaults to
  cwd; path must be a git repo unless `--vcs none`; one project per path (re-adding names the
  existing project); names unique, `scratch` reserved; nested repos are separate paths. Running gx
  in an unregistered repo fails with a hint (`gx project add .`); the TUI may offer it as a confirm,
  never auto-registers. `gx project remove <name>` refuses while any of its tickets are queued or
  running and keeps its tickets. `gx project set-path`. Rename is not in v1. Project add / remove /
  set-path / list are server writes and reads (05 §6) **(stage confirmed in review: S4)**. Before
  S4, migrate is the only registration path (see S1 `project.json`).
- **Identity** (04 §3): project name = first address segment; default the repo dir basename (the
  dir holding `.bare` in a bare layout, never a worktree). Path is mutable.
- **`unavailable`** (04 §5): checked at start and on poll. Its tickets do not run and show the reason;
  one notification. No auto-discovery.
- **Config layering** (04 §9, 14): `project.json` may override only a whitelist — `vcs`, trunk
  branch, landing policy, auto-ff-merge, agent/model, skills, notification destination,
  `execution-queue.max-agents-per-epic`, and the optional project `max-agents`. UI settings, global
  `execution-queue.max-agents`, `budget` and `orchestrator` are `config.json` only.
  Non-whitelisted keys are rejected. Full key paths are in Config keys.
- **Landing policy per project** (01 §8) **(stage confirmed in review: S4)**: `on-landed` for
  ff-only projects (derived base always trunk), `on-done` + a real PR chain for PR projects. Opt-in
  automatic ff-only merge gated on (a) fast-forward possible without rebase and (b) the epic's
  code-review ticket done; otherwise park and leave it to `gx-merge`. Never a global default.
- **Slot caps** (14, names per 11): global `execution-queue.max-agents` (default 4),
  `execution-queue.max-agents-per-epic` (default 2, counted per root — epic or one-off — replaces
  `max-concurrent-tickets-per-epic`; one-offs never hit it but count toward the others), optional
  project `max-agents`. A ticket starts only when every
  applicable cap has room. `max-concurrent-epics` is dropped (ignored with a warning until cutover).
  A slot is one live agent (claimed + running pane), plain count; parked, waiting,
  waiting-for-children and landing hold nothing. No bypass: one-offs, `scratch` tickets and
  investigate tickets share the pool; `--front` is queue position only. No per-ticket parallelism
  override. Lowering a cap stops new starts only, applied at the next scheduling decision.
  Plain FIFO across projects, no fair-share. `explain` names every full cap with its count.
- **Budget** (15): the ledger, latches, override and `gx budget` verbs ship in S3 (see S3 Budget
  ledger); S4 makes them span projects. One global budget per budget day (local midnight),
  `config.json` only. Soft limit
  pauses new starts on every project; hard limit stops every live pane (ctrl+c, 15 s grace,
  `TabClose` panes whose cost still rises) and pauses new starts. A persisted **budget ledger** in
  the state dir adds each live iteration's cost delta per 30 s poll to today's bucket (an iteration
  crossing midnight splits correctly); the per-epic `baselines` map dies at cutover. All ticket kinds
  count. A hard kill parks `needs-repair` kind `budget-killed`, never matched by recovery; the way
  back is the override plus unpark. `gx budget override` (server write); `gx budget status --json`
  (today's total, limits, latch/override, per-project breakdown), included in the snapshot. `p`
  keeps three states; user pause and budget pause are independent flags. Latches, the sticky
  override point and the notification high-water mark persist with the ledger and reset at midnight.
  The extra-usage check runs at server start and on every enqueue; its banner moves to the enqueue
  modal; if on, one chat notification per day. Queue header `today $X of $Y` with the existing
  80 %/100 % colours; per-root cost on each root row.
- **TUI scope** (07 §2, 12): Queue tab global with a project prefix and a filter defaulting to all;
  Tickets tab defaults to the cwd project with an all-projects toggle (outside a registered project:
  all + hint). Git tabs stay cwd-repo-scoped.
- **Notifications, S4 part** (17): a **destination** (transport + target) is the unit of batching,
  gating and muting; two projects with the same target share one. A `project.json` override replaces
  the whole global block (no per-transport merge); `scratch` uses global. One batcher per destination
  (6 s window); a multi-project batch renders grouped under one header per project. The gate is keyed
  by destination with unchanged thresholds; on load, old transport-keyed entries map to the global
  destination's key (no migrate command). `gx notify --enable/--disable <transport>` acts on every
  destination of that transport; `--project <name>` narrows it; `--status` lists each destination
  and its projects.

### S5 — One-offs (08, 11, 04)

- **Submit** (08 §1, 11 §4): `gx server one-off "<prompt>" | --file <path>` = create + enqueue in
  one server call. Required: the prompt (ticket body). Project: `--project`, else the registered
  project owning cwd, else `scratch`. Name: `--name`, else the prompt's first words plus a short
  unique suffix. Agent: `--agent`, else the project default. Optional `--base`, `--blocked-by
  <addr>…` (S3 rules), `expected_context_window` (no default).
- **`scratch` project** (04 §11, 01 §2) **(stage confirmed in review: S5)**: auto-created at
  server start as `<store>/scratch/project.json` with `vcs: none`, cwd a server-owned
  `scratch-workspace/` in the data dir, beside the store and outside its git repo (see S1). Cannot
  be removed. Tickets in it are commitless by policy.
- **Types** (08 §2, 11 §5): one-offs default to `type: prompt` (new, always commitless).
  `--commits` creates `type: implement`. `vcs: none` is always commitless.
- **File payload** (08 §3): `--file` reads frontmatter + body; flags override frontmatter. No
  template noun, no store directory. Over HTTP the file is a server-side path (localhost only).
- **Waiting** (08 §4): default prints the address (`--json` → `{address, status}`), exit 0.
  `--wait` blocks on the stream (reconnecting across a server restart) until terminal or parked,
  prints the `## Result`, and exits with a code from the table below. `--timeout <dur>` (no
  default) exits with its own code; the server never cancels because a waiter left. From S6, a park
  with recovery pending does not end the wait: `--wait` keeps waiting until recovery succeeds (then
  the ticket's real outcome), escalates (exit 9), or the recovery notify hold cap expires (the
  park's own code).
- **Exit codes** (fixed; scripts depend on them; tested through seam A). 1 and 2 stay generic
  errors and usage; 126+ are left to the shell.

  | Code | Outcome |
  | --- | --- |
  | 0 | `done` (or submitted, without `--wait`) |
  | 3 | `needs-answer` |
  | 4 | `needs-repair` |
  | 5 | `cancelled` |
  | 6 | `--timeout` reached (the one-off keeps running) |
  | 7 | `duplicate-live` refusal |
  | 8 | `server-not-running` refusal |
  | 9 | recovery escalated (S6) |
- **Result** (08 §5): the agent writes `## Result` via `gx tickets section <addr>`.
- **Notification** (08 §6): parks always reach chat, routed by project, no opt-out. Success is
  silent unless `--notify` / `notify: true`, which sends the truncated `## Result` with the address.
- **Dedupe key** (08 §7, 11 §7): `--unique <key>` / `unique:`; with `--file` the default is the
  file's absolute, symlink-resolved path; `--no-unique` opts out; plain prompts have none. A ticket
  in the same project with the same key that is not `done` / `cancelled` (parked copies included) →
  refuse `duplicate-live`, return its address, exit 7. With `--wait`, print the notice to stderr
  and wait on the existing ticket.
- **Where it runs** (08 §8, 02 §6): git commitful → worktree + iteration branch, lands through the
  project's landing policy like an epic, never cherry-picked straight onto trunk; git commitless →
  detached worktree at the resolved base, removed when the ticket ends, never the live checkout;
  `scratch` → per-ticket `scratch-workspace/<slug>/`, kept after the ticket ends, deleted on archive.
- **Server down** (08 §9): refuse `server-not-running`, exit 8, no spool.
- **Queue position** (08 §10): tail by default; `--front` at the head; caps still apply (14).
- **Skill** (08 §11, 11): `/gx-one-off <addr>` — read the body, do the work, write `## Result`,
  report iteration status. Chosen by ticket type. Commitful one-offs still pass the normal landing
  gate.
- **Events** (13 §9): `submitted`; `submit-refused` kind `duplicate-live`; result notify reuses
  `notification-sent` with `notify_kind=result`.

### S6 — Auto-investigate (10, 09, 13)

- **Two layers** (10 §1): server **recovery rules** (Go) match `(type, kind)` and apply mechanical
  remedies — no pane, no tokens. An **investigate ticket** (agent) starts only when the matched entry
  needs judgment (R2 transcript reading, R3 completeness proof, R4 dialog reading) or nothing
  matched.
- **Catalog is Go data** (10 §2) in one `recovery` package: entry = `id`, signature (`(type, kind)` +
  predicates), `executor: rule | agent`, `authority: low | medium | high`, remedy (allowed verbs),
  `enabled`. Published by `gx recovery catalog --json`; an investigate ticket's prompt gets the
  matched entry from it. Global `recovery.enabled` kill switch plus per-entry disable. Entries are
  sized from ticket 09's 14 failure modes (R1–R14) and its event-log data, not from `gotchas.md`.
  The ticket breakdown has one ticket per R-entry, each deciding that entry's executor, authority
  and enabled-at-launch. An entry with no S0 event data behind it defaults to `enabled: false`.
- **Authority** (10 §3): low/medium auto-apply; high never does — recovery diagnoses, writes the
  exact proposed verb, escalates, and a human approves with one TUI key that runs
  `gx server tickets approve`. R3 commitless-done and R6 clear-to-`open` are high; R3 `land` of a
  `recoverable` branch after the completeness check is medium. R13 vanishes under the server's
  durable queue.
- **Proposals and approve** (10 §3, 11): a proposed high-authority remedy is stored twice — a
  `recovery-proposed` event and a `## Proposed Remedy` section in the ticket. `gx server tickets
  approve <addr>` runs the latest proposal and refuses `proposal-stale` if the ticket changed since
  it was proposed. The TUI key is `A` on the Queue tab.
- **Triggers** (10 §4): every `needs-repair` kind (including `retry-exhausted`, `spinning`,
  `handle-mismatch`, `ambiguous-land`) except `budget-killed` (15); `needs-answer` / `zero-commit`;
  `needs-answer` / `blocked-pane` only when a rule matches an allow-listed dialog; a `deadlocked`
  event → diagnosis only. Not `self-reported`, not commitless finish.
- **`deadlocked` event**: deadlock (nothing runnable, nothing live, children not all done) is a
  derived state, so the scheduler emits a `deadlocked` event when an epic enters it, with kind
  `all-parked` or `blocked-cycle`. Emitted on transition only; tested through seam A.
- **Same verbs** (10 §5, 11 §3): rules call the API's own verbs with `via: server, actor: recovery`:
  same refusal codes, land lock, events. No private write path.
- **Investigate ticket** (10 §6, 03 §9, 14 §3): `type: investigate`, a fork-lettered child of the
  failed ticket in its `issues/` (`06a`; no new suffix). The parent stays parked and renders
  waiting-for-children. Enqueued `--front`, counted against caps (it takes the slot the parked
  parent freed). Commitless by type, target none — so "no tier 3" is structural. Runs in a detached
  worktree at the first of: the feature-branch tip; else the failed ticket's resolved base (a
  commitless one-off in a git project, or a branch that is gone); else, in a `vcs: none` project,
  `scratch-workspace/<slug>/`. Non-investigatable by type; its own failure parks `needs-repair` and
  notifies a human. Other tickets opt out with `recover: false`. One-offs are recoverable by
  default.
- **Epic vs one-off with an investigate child**: epic vs one-off is decided by non-investigate
  children only. An investigate child does not turn a one-off into an epic (kind, caps and
  rendering unchanged); the TUI renders it nested under the one-off, as under an epic.
- **Skill** (10 §7, §14): `gx-investigate` gains an **unattended** mode — shared background and
  inventory; writes limited to the matched entry's remedy verbs; no match → report + park only.
  **Attended** mode keeps the human go-ahead. `m → Investigate` still opens an attended interactive
  tab, not a ticket.
- **Guard rails** (10 §8): at most 1 automatic recovery per `(ticket, kind)`, 3 per ticket. A
  recovered ticket that fails again in its next iteration is a **failed recovery** → escalate at once
  with both failures. Counters derive from the event log (count `recovery-applied` per address), so
  a resume cannot clear them.
- **Recording** (10 §9): `recovery-matched` (entry id, signature), `recovery-applied` (remedy,
  outcome), `recovery-proposed` (verb), `recovery-escalated` in the event log of the affected
  ticket's top-level ticket (S1), using S0's schema; each carries the `kind` of the failure it
  answers. The long write-up lives in the investigate ticket's own file. No separate server-level
  recovery log.
- **Report** (10 §10): the investigate ticket's `## Result` holds **Observed** (`(type, kind)`
  events, seq/timestamps), **Evidence**, **Diagnosis**, and **Proposal** — exactly one of
  `catalog-entry` (draft signature + remedy + authority), `orchestrator-fix` (file pointers), or
  `human-only` (09's residue class). For the first two, auto-file a `draft` research ticket in the
  follow-ups epic named by `recovery.follow-ups` (default `gx:follow-ups`), deduped: an open draft
  with the same `(type, kind)` + proposal class gets a `## Comments` occurrence line instead. A
  missing epic is created as `draft`. A missing project is logged, and the proposal is attached to
  the escalation message only.
- **Notification** (10 §11): the original park notification is held while recovery runs (cap
  `recovery.notify-hold`, default 10 min, then sent). Successful recovery → no chat, counted in
  the epic-complete summary. Escalation → one message naming the matched entry or "no match", with a link to the report.
- **Lost commits with no iteration branch** (10 §12): escalate-only.
- **Nudge** (05 §7, 10 §13, 12 §7): `gx server tickets nudge <addr>` (the iteration nudge route) is
  the only tier-1 path; every nudge is logged, streamed and rate-limitable. When it and rules R2/R5
  ship, the interim pane-nudge grant is deleted from `gx-investigate` — in S6, not at cutover.

## Testing Decisions

### What makes a good test here

- Test external behavior only: what a client sees through the API or CLI (results, refusal codes,
  `via` / `actor`), what lands in ticket files and event logs, and what reaches chat. Never assert
  on scheduler internals, goroutines, or timer mechanics.
- Drive the server the way a client does. A test that reaches into server internals to set up or
  check state is testing the wrong seam.
- The event log is the contract recovery reads, so event assertions are by `(type, kind)` and
  required fields, never by `reason` text.
- Every refusal is a stable `reason` code; tests assert the code, not the message.
- Agent behavior is not asserted in Go. For the report (story 126), the test feeds a fixture
  `## Result` with a Proposal and asserts that the server files or dedupes the follow-up. For the
  skill modes (story 131), the unattended mode's allowed-verbs list is checked by the existing
  skills bundle test.

### Seams (approved)

The user approved seams A–G in the design review. The aim is the fewest, highest seams. Seams A,
E and F follow directly from tickets 12 §6, 05 §10 and 13 §2.

- **A. Server harness through the API client** (S2 onward; the primary seam). An in-process server
  on a temp state dir, temp socket and temp ticket store, with the `run_realgit` fake herdr (a fake
  executable, `writeFakeExecutable`), driven only through the same API client the CLI uses.
  Covers nearly everything from S2 to S6: snapshot + stream sequencing, explain verdicts,
  server writes and refusals, write split and pings, reclaim and `handle-mismatch`, crash-mid-land
  recovery, base derivation and `needs-answer` on ambiguous base, slot caps, budget latches and
  ledger, notification routing and outage fold (via a fake chat transport), one-off submit /
  `--wait` / dedupe and the exit-code table, the `deadlocked` event, and recovery guard rails,
  approve and escalation end to end. The S3 port of the `run_realgit_*` scenarios lands here.
- **B. Ralphloop `run_realgit` harness** (S0 and S1 only; existing). S0's `park()`, `launch-failed`,
  retry-exhausted and spinning parks are asserted as event-log lines + ticket frontmatter through
  the existing in-process loop; S1 re-runs it with the loop reading the ticket store. Retired for
  new tests once seam A exists.
- **C. CLI command level against a temp ticket store** (S1, plus the direct-write verbs after).
  `gx tickets migrate` (incl. `--dry-run`, idempotency, `claimed` refusal), `gx tickets validate`
  (resolver, dangling, cycles, `--all`, terminal skip, S3 grammar and `base:` checks), and the
  address-based `show / add / section / set` verbs, called as functions against temp dirs like
  today's `cmd/tickets_*_test.go`.
- **D. TUI view-model reducer** (S3). Snapshot + a sequence of server events in, render state out:
  server indicator states, Queue clear on disconnect, explain subtext, toasts only from live events.
  A pure reducer, like today's `reduceLiveEvent` tests. No server needed.
- **E. One real-binary e2e under herdr** (decided, 12 §6): start → enqueue → land → stop → restart
  reclaims, in `e2e/`.
- **F. Structural CI tests** (decided): the route table ↔ command tree diff (05 §10), and "no status
  write bypasses `park()`" (13 §2).
- **G. Pure recovery-catalog matcher** (S6). Event sequence in, matched catalog entry (or no
  match) out. Table-driven over the R-entries, one row per signature and predicate case, with no
  server. Seam A only checks that a match leads to the right `recovery-*` events.

### Modules under test

- The event contract package (S0): schema, `kind` enum, size limit, required fields.
- The ticket store, loader and shared `blocked_by` resolver (S1, S3).
- The server: API handlers, scheduler, write split, lifecycle, reclaim (S2, S3).
- The repair-verb package (shared by CLI and server).
- Slot caps, budget ledger, notification routing (S4).
- One-off submit and wait (S5).
- The `recovery` package: its pure catalog matcher (seam G) and its rules (S6).
- The TUI view model (S3).

### Prior art

- `ralphloop/run_realgit_*_test.go` and `run_realgit_helpers_test.go` — real git, fake herdr
  executable; the base for seams A and B.
- `cmd/tickets_land_test.go`, `tickets_reset_test.go`, `tickets_verify_test.go`,
  `tickets_validate_test.go` — the shipped JSON / exit-code contract for repair verbs and validate.
- `ralphloop/eventlog_test.go`, `ralphloop/eventsink_contract_test.go` — event log format and sink
  contract.
- `ralphloop/notification_gate_test.go`, `chat_eventsink_test.go` (`fakeChatTransport`) — gate and
  batcher behavior, reused per destination.
- `ralphloop/permit_test.go` — slot permit logic that caps extend.
- `ui/tickets/model_live_test.go`, `ui/tickets/queue_test.go` — live-event reduction in the TUI.
- `tickets/parentgraph.go` tests — cycle reporting style for `blocked_by` cycles.
- `e2e/` — existing herdr-driven real-binary tests.

## Out of Scope

- **Multi-machine / remote server** — localhost only.
- **Authentication and network exposure** — bind loopback; use a tunnel or VPN if a phone needs to
  reach it.
- **Building a web UI** — not built here, but the API stays transport-agnostic enough not to
  preclude one (snapshot, stream, budget status are already in the API).
- **Replacing herdr** as the agent executor.
- **Replacing markdown** as the on-disk ticket format. Its location moves to the global ticket store,
  and the server keeps an index plus its own state beside it (03).
- **Cron-like recurring scheduling inside the server** — external cron calls `gx server one-off`.
- **Lost-commit discovery** — finding the commit range for a done ticket whose iteration branch is
  gone (R3's 25-ticket class) and proving it complete. A later effort; escalate-only here (10 §12,
  12).
- Also ruled out by individual decisions: cross-project `blocked_by` machinery (grammar reserved
  only, 01 §6); a spool for submits while the server is down (08 §9); per-project or rolling budgets
  (15 §1); fair-share scheduling and cap bypass (14); `gx project mv` rename (04 §3); a systemd unit
  (06 §9); a separate merge ticket type (02 §8); live-state migration (12 §5).
- **Per-project pause** — not in v1. Only server-wide pause (`p`, `gx server queue pause`) exists;
  caps and `gx project remove` / `unavailable` cover the need (07 §6 handed it to 14, which did not
  take it).

## Further Notes

### Source

- Decision trail: `.scratch/orchestrator-daemon/map.md` and closed tickets `01`–`17` in its
  `issues/`. Each bullet above cites its ticket; the `## Resolution` of that ticket holds the detail.
- Shipped primitive this builds on: `ticket-land-recovery-impl` (`gx tickets verify | land | reset |
  unpark`, one JSON / exit-code contract, per-epic land lock).
- Ticket 11 carries the full CONTEXT.md patch (new, changed and removed entries). It is not
  repeated here; each stage applies its slice.
- Side bug found in 03, not in this scope: the real `~/.config/gx/queue-state.json` contains a test
  temp path — a test writes to the user's real config.

### Amended standing decisions

Where a later ticket or the design review changed a map standing decision, this spec follows the
change. Listed so that reviewers do not re-flag them:

- **"No dual path"** (map) → a temporary `orchestrator: in-process | server` switch from S3 until
  cutover, so rollback is a switch flip (12 §2).
- **"Interim nudge ends at cutover"** (map's line for 10) → the interim pane-nudge grant ends in S6,
  when the nudge verb and rules R2/R5 ship (12 §7). The map's 10 line is not yet updated.
- **`via: recovery`** (10) → `via: server` plus `actor: recovery` (11 §3).
- **Event log "per ticket"** (11's glossary) → one event log per top-level ticket plus one server
  event log (S1, Event log). 11's glossary line changes to match.

### Open questions

None. The design review settled every open question, including the CONTEXT.md slice mapping.

### Next step

Done: `gx-to-tickets` published the five epics listed in Staging rules, including the cutover epic
and one ticket per recovery-catalog R-entry.
