# Agent runner: a herdr-free way to run ralph-loop agents

Source: wayfinder map `agent-runner` (tickets 01-09). Decision tags like `[T05]` point to the map
ticket that settled the point. ADR: 0031 (two native agent runners).

Phase 2 refinement source: wayfinder map `agent-runner-phase2` (tickets 01-07). Its tags are written
`[P2-T0N]` so they do not clash with the first map's `[T0N]`. Stories 103 and up come from it;
older stories it changed say so in their tag. `[P2-review]` marks points settled while grilling
the map's design-review findings.

## Problem Statement

ralph-loop and the server call herdr directly to host each agent session. Some organizations block
herdr. In those places a person cannot run ralph-loop at all. The herdr calls are also spread over
13 herdr-shaped `Deps` fields, and the server's `launch` path bypasses `Deps` completely.

A person also has no simple way to see what a running agent is doing, or to prompt, interrupt or
answer it, without herdr.

## Solution

Add an **agent runner** interface. ralph-loop and the server drive it instead of herdr. An agent
runner is the thing that hosts one agent session for one iteration (not an epic run).

- **herdr** stays as one runner, behaviour unchanged, behind an adapter.
- Two **native runners**, claude only, hosted by `gx server`, no new external tool:
  - **Headless runner** (`claude -p` stream-json). The default.
  - **PTY runner** (interactive claude in a gx-owned PTY). A backup, set by hand.
- Both native runners survive a `gx server` restart and are reattached.
- A per-machine setting picks the runner for new launches: `agent-runner: auto | herdr | headless |
  pty`. A run stays on the runner that started it, even after the setting changes.
- A **watch CLI** shows a running agent: follow mode for a person, dump mode for `gx-investigate`.
- `gx claude doctor` checks the claude features gx relies on.

Delivery: one spec, two phases.

- **Phase 1:** interface, herdr adapter, headless runner, watch CLI, doctor for headless.
- **Phase 2:** PTY runner and its doctor checks. Phase 2 tickets are blocked by phase 1.

Dependencies:

- Implementation is blocked by `orchestrator-daemon-3-cutover` (only `gx server` hosts native agents).
- Investigate tickets depend on `orchestrator-daemon` S6 (auto-investigate).

## User Stories

### Runner interface and selection

1. As a gx maintainer, I want ralph-loop to drive one `Runner` interface, so that herdr and native
   runners are interchangeable. [T05]
2. As a gx maintainer, I want the server's `launch` and `finishRun` to use the same interface, so
   that no code path calls herdr directly. [T05]
3. As a gx maintainer, I want herdr workspaces, tabs, nudges, dialog rules and pane scraping hidden
   inside the herdr adapter, so that ralph-loop sees no herdr concepts. [T05]
4. As a gx maintainer, I want ralph-loop tests to use a small in-memory `Runner` fake instead of
   about 13 stubs, so that tests are short. [T05]
5. As a user in an org that blocks herdr, I want ralph-loop to work without herdr, so that I can
   run epics at all. [T04]
6. As a user, I want `agent-runner: auto | herdr | headless | pty` in my per-machine config, so
   that I choose how new agents run. [map: Selection, P2-T03, P2-T04]
7. As a user with herdr installed and reachable, I want `auto` to pick herdr, so that nothing
   changes for me. [T04]
8. As a user without herdr, I want `auto` to pick headless, so that it works out of the box. [T04]
9. As a user, I want `auto` to never pick pty, so that a hard-to-detect "`-p` is blocked" case is
   always my own explicit choice. [T04]
10. As a user, I want no per-epic override of the runner, so that the setting stays simple. [map]
11. As a user, I want a bad `agent-runner` value to stop the start with a clear error, so that I
    fix it before work begins. [T09]
12. As a user, I want a missing `claude` on PATH to stop the start with a clear error and leave
    the ticket queued, so that nothing is parked wrongly. [T09]

### Operations and status

13. As ralph-loop, I want `Start` (options: label, epic, cwd, kind, args, env) to return a
    `Session` once the agent is idle, so that I can send the first prompt safely. There is no
    `interactive` option. [T05, P2-T03]
14. As ralph-loop, I want `Prompt(session, text)` to return once a new turn started, or
    `ErrNotDelivered`, so that I know the prompt was taken. [T05]
15. As ralph-loop, I want to start a fresh session when `ErrNotDelivered` happens, so that one
    simple rule covers a stalled prompt. [T05]
16. As ralph-loop, I want `Status(session)` to give state, a `Turn` counter, the session id and a
    blocked reason, so that I can decide the next step without scraping. [T05]
17. As ralph-loop, I want the states `working`, `idle`, `done` and `blocked`, with `idle` and
    `done` treated the same, so that the model stays small. [T05]
18. As ralph-loop, I want `blocked` to mean "waiting on a prompt gx did not send", with a free-text
    reason, so that I can park with a useful message. [T05]
19. As ralph-loop, I want `Wait(session, states, timeout)` with a distinct timeout error, so that
    `finishRun` no longer waits forever. [T05]
20. As ralph-loop, I want `Interrupt(session)` to keep the session alive, so that smart-zone can
    send `/compact` after it. [T05]
21. As ralph-loop, I want `Stop(session)` to be idempotent, so that cleanup can run twice safely.
    [T05]
22. As ralph-loop, I want `Find(label)` and `List(epic)`, so that I can detect a duplicate agent
    and reattach. [T05]
23. As ralph-loop, I want `RateLimit(session)` to return a reset time or nothing, so that the rate
    limit pause does not depend on screen text. [T05]
24. As ralph-loop, I want `Answer(session, input)`, so that a person can resolve a blocked prompt.
    [T05]
25. As a maintainer, I want `Session` to be an opaque `{Label, ID, SessionID}`, so that no code
    depends on tab or pane ids. [T05]
26. As a user upgrading gx, I want old `runs.json` files with `pane`/`tab` fields to load into a
    herdr `Session`, so that running epics are not lost. [T05]
27. As a maintainer, I want the label-and-cwd match to be the ownership check, so that gx never
    touches an agent it did not start. [T05]
28. As a maintainer, I want `ErrLabelTaken` to park as `agent_name_taken` on herdr and
    `duplicate-live` on native, `ErrNotDelivered` as `agent_prompt_stalled`, and `ErrNotReady` as
    `blocked-pane`, so that existing park kinds keep their names. [T05]
29. As a maintainer, I want herdr-only health (`CauseHerdr`, `Ping`, `herdr-unavailable`) behind
    an optional `Healthy() error`, with native runners always healthy, so that herdr outages stay
    herdr-only. [T05]
30. As a maintainer, I want the nudge, retype and `errStuckSubmission` logic and the
    `state_change_seq` counter moved into the herdr and pty adapters (the counter becomes
    `Status.Turn`), so that ralph-loop has one delivery rule. [T05]
31. As a maintainer, I want the Claude and Codex rate-limit and context scrapes moved into the
    herdr adapter, so that ralph-loop sees only structured `RateLimit`. [T05]
32. As a maintainer, I want adapters that host a TUI (herdr, pty) to answer `trust_directory`
    themselves, so that ralph-loop only sees `blocked` plus a reason. [T05, T08]

### Native runners: launch

33. As a user, I want the headless runner to launch `claude -p` with stream-json in and out, so
    that state comes from structured events. [T04, T02]
34. As a user, I want both native runners to launch claude with `--permission-mode auto`, so that
    they match what ralph-loop does through herdr today. [T04]
35. As a user, I want agents launched with `DISABLE_AUTOUPDATER=1`, so that claude does not update
    in the middle of a run. [T04]
36. As a user, I want the inherited `CLAUDECODE` and `CLAUDE_CODE_ENTRYPOINT` env vars stripped at
    launch, so that the child session still saves its transcript. [T06, P2-T03]
37. As a user, I want the headless runner to use `--permission-prompt-tool stdio`, so that a
    permission prompt becomes a real `blocked` state. [T04]
38. As a maintainer, I want a fall back to `--permission-prompts none` documented for the day
    claude drops the `stdio` tool, so that the risk is known (denials only). [T04]
39. As a maintainer, I want the native runners to support claude only and the interface to stay
    agent-neutral, so that Codex can be added later. [map]

### Lifetime and reattach

40. As a user, I want a native agent to keep running when `gx server` stops or restarts, so that
    a server restart does not lose work. [T06]
41. As a maintainer, I want the headless agent to run under `setsid` with no holder process, so
    that the design stays small. [T06]
42. As a maintainer, I want each PTY agent owned by a `setsid` holder process that serves a unix
    socket, so that the PTY survives the server. [T06, T03]
43. As a maintainer, I want every native agent process in its own session, so that launchd stop
    and `kickstart -k` do not kill it. [T06]
44. As a maintainer, I want a per-iteration agent directory under the gx data dir, grouped by
    project and named `<epic>-<id>`, so that identity and I/O live in one place. Files per runner:
    headless has `meta.json`, the `stdin` FIFO, `out.jsonl` and `stderr.log`; PTY has `meta.json`,
    `holder.sock`, `raw.log` and `screen.txt` (no `out.jsonl`; its event history is the claude
    transcript). [T06, P2-T03]
45. As a maintainer, I want `meta.json` to hold runner, pid, pid start time, session id and cwd,
    so that reattach can verify identity. Headless adds the read offset (where the session starts
    in `out.jsonl`). PTY stores both the holder pid and the claude pid, each with its start time.
    There is no branch field. [T06, P2-T03]
46. As a maintainer, I want an agent to count as live only if the pid exists, the start time
    matches and the cmdline holds the session id, so that a reused pid is never adopted. The check
    is per runner: on PTY the holder pid and the claude pid must both pass and `holder.sock` must
    answer. [T06, P2-T03]
47. As a maintainer, I want the reattach scan to cover claimed and `needs-repair` tickets, so
    that parked agents are found too. [T06]
48. As a user, I want a headless agent that died while the server was down to count as a normal
    finish when `out.jsonl` ends with a `result` event, so that finished work is landed.
    (Headless only; see story 113 for PTY.) [T06, P2-T03]
49. As a user, I want any other dead agent to park `needs-repair` with
    `agent-died-while-server-down` and keep its directory, so that I can look at it. [T06]
50. As a user, I want no automatic `--resume` of a dead agent, so that gx never repeats work by
    surprise. [T06]
51. As a maintainer, I want the server to reopen a headless agent's `out.jsonl` at the saved
    offset, replay it to rebuild state, save the offset only at event boundaries, and treat events
    as idempotent by `uuid`, so that reattach is exact. (Headless only; see story 114 for PTY.)
    [T06, P2-T03]
52. As a maintainer, I want each runner to remove its own transient files on land or cancel
    (headless: the `stdin` FIFO; PTY: `holder.sock`), so that no stale files stay. [T06, P2-T03]
53. As a user, I want agent logs and `meta.json` kept for the `server.log-retention` duration
    (default 30 days) and pruned on server start and daily, so that I can inspect recent runs.
    Prune uses the file list of the runner named in `meta.json`. [T06, T07, P2-T03]
54. As a user, I want logs of parked tickets never pruned, so that I can still investigate them.
    [T06, T07]

### Parity

55. As a user, I want smart-zone compaction (interrupt, then `/compact` in the same session) on
    both native runners, so that long iterations behave as on herdr. [T08]
56. As a user, I want the budget stop (interrupt, grace period, stop) on both native runners. [T08]
57. As a user, I want the background-task gate, the unexecuted-tool-call nudge and the debounce to
    work on native runners, so that they behave as on herdr (they read the transcript, not the
    runner). [T08]
58. As a user, I want the rate-limit pause on native runners, from `rate_limit_event` on headless
    and from the PTY's own screen buffer on pty (matched as in story 107). Phase 2 ships PTY
    `RateLimit` as always nil: the banner was never captured, so there is no anchor. Until a
    follow-up captures it, a rate-limited PTY iteration parks `zero-commit` instead of pausing.
    [T08, P2-T02, P2-review]
59. As a user, I want prompt delivery on headless to be a stdin write, and the pty runner to do
    its own nudge and retype (via a runner-neutral nudge helper, story 130). [T08, P2-T03]
60. As a user, I want the server's own run path to go through `Runner` now, while its known gaps
    (no smart-zone, blocked or rate-limit handling) stay as they are, so that this spec stays
    bounded. [T05]
61. As a user, I understand the three native gaps: no `trust_directory` on headless (nothing to
    answer), no live terminal attach to a stuck TUI (pty shows a screen snapshot instead, stories
    78 and 126), and pty needs the undocumented `~/.claude/sessions/<pid>.json` status file.
    [T08, P2-T03, P2-T06]

### Blocked prompts and acting on a live agent

62. As a user, I want a permission request on headless to park the ticket `needs-answer` and keep
    the request open, with the tool name and input in the park reason, so that I can decide. [T08]
63. As a user, I want `gx server agents answer <addr> allow|deny [reason]` to resolve a blocked
    prompt on every runner, so that I can unblock an agent in one command. No free text and no menu
    numbers (that would mean scraping each dialog's options). PTY key mapping: stories 109-110.
    [T08, T05, P2-T05]
64. As a user, I want gx to never auto-allow a permission prompt on any runner, so that agents stay
    inside my permission rules. [T08]
65. As a user, I want only `trust_directory` auto-answered, and only on herdr and pty. On pty this
    is the one dialog found by a screen rule (story 105). [T08, P2-T01]
66. As a user, I want a pending headless request to stay answerable after a server restart when its
    `request_id` has no response in `out.jsonl`, so that a restart does not lose it. [T08]
67. As a user, I want a failed FIFO write on `answer` to park `needs-repair`, as for a dead agent
    without a `result`. On pty, a failed `holder.sock` write does the same (story 112). [T08]
68. As a user, I want `gx server agents prompt <addr>` and `interrupt <addr>` as one-shot verbs, so
    that I, investigate or a future watchdog can steer a live agent. [T05]
69. As a user, I want these verbs to take a ticket address (`<epic>/<id>`) that resolves to the
    live iteration, so that I never type a label. [T08]
70. As a user, I want no live terminal attach to a loop session, so that the design stays within what native
    runners can promise. Interactive sessions are the exception (ADR 0037). [map]

### Watch CLI

71. As a user, I want `gx server agents watch <epic>/<id>` to show what an agent is doing, so that
    I do not need herdr to look. [T07]
72. As a user, I want `--follow`/`-f` to print the log so far and then stream new lines until the
    agent ends or I press Ctrl-C. [T07]
73. As a user, I want no flag to dump the whole log, and `--tail N` to dump the last N lines. [T07]
74. As the `gx-investigate` skill, I want `--json` to print raw events, one JSON object per line,
    so that I can parse them. [T07]
75. As a user, I want rendered text lines like `[tool] Bash: go test`, `[text] ...`,
    `[result] ...` by default. [T07]
76. As a maintainer, I want one shared line renderer used by follow, dump and the TUI "Watch"
    item on headless, so that these views look the same. The TUI "Transcript" item renders
    markdown instead (story 125). [T07, T05, P2-T06]
77. As a maintainer, I want no second log: headless reads `out.jsonl`, pty and herdr claude read
    the transcript. [T07]
78. As a user of the PTY runner, I want `--screen` to print the current screen ("what is it stuck
    on") and `--raw` to replay the ANSI log, so that I can see a stuck TUI without attaching.
    Both read the agent directory: `--screen` asks the holder while it is live, else prints the
    saved `screen.txt`; `--raw` replays `raw.log` and needs a terminal as wide as the PTY (~200
    columns). [T07, P2-T02, P2-T03]
79. As a user of herdr claude agents, I want watch to read the transcript (found with
    `gx claude session-path`), so that I have one way to look at any claude agent. [T07]
80. As a user of herdr Codex agents, I want a clear "not supported" message and no pane scraping.
    [T07]
81. As a user, I want a finished agent within retention to dump from the saved log and `--follow`
    to print it and exit. [T07]
82. As a user, I want a pruned log to give "log pruned" and the transcript path. [T07]
83. As a TUI user, I want "Answer in pane" removed and replaced by two `m` menu items,
    "Transcript" and "Watch" (stories 124-126), so that I can look at any agent the same way on
    every runner. Neither is a `Runner` method. [T05, P2-T06]
84. As a TUI user on herdr, I want jump-to-pane (now under "Watch") and the other herdr terminal
    features (`ui/terminalrun`, worktree terminal menu) unchanged, and hidden when herdr is
    missing. [map, P2-T06]

### Investigate

85. As a user on a native runner, I want an investigate ticket to run as an ordinary iteration, so
    that no special launch code exists. [T08]
86. As a user, I want the manual `m -> Investigate` item on a native runner to enqueue an
    investigate ticket on the parked ticket (same as the automatic path), started by the server.
    [T08]
87. As a user on herdr, I want `m -> Investigate` to keep opening an attended, focused tab, so
    that nothing changes. [T08, T05]
88. Withdrawn. `Start` has no `interactive` flag (removed in phase 1, stays removed). Native runs
    are always non-interactive and watched. [T05, P2-T03]

### Claude doctor and preflight

89. As a user, I want `gx claude doctor` (beside `statusline|history|session-path`) to run a short
    canned session in a temp dir and report each assumption gx relies on, so that I see
    drift before it breaks a run. [T04, T09]
90. As a user, I want output as `PASS|FAIL|SKIP <name> - <detail>` lines with a header showing the
    installed claude version and `MinClaudeVersion`, and exit 1 on any FAIL. [T09]
91. As a user, I want a check that depends on a failed check to show `SKIP`. [T09]
92. As a user, I want `--runner headless|pty` to check one runner, defaulting to the runner
    `agent-runner` selects (`auto` means headless; `herdr` also means headless, since the doctor
    never checks herdr). The default reads config only, with no herdr probe. [T09, P2-T07]
93. As a script author, I want `--json` to print `[{runner,name,status,detail}]`. [T09]
94. As a maintainer, I want `--record <dir>` to save the raw events of each canned session, so
    that I can refresh test fixtures. [T09]
95. As a user, I want the doctor to never check herdr and `gx doctor` to stay repo-only. [T04, T09]
96. As a maintainer, I want the doctor to run only when asked, never as preflight. [T04]
97. As a user, I want launch-time preflight to be cheap and make no model call: `claude` on PATH,
    valid `agent-runner`, and the launch-time capability. [T09]
98. As a user, I want a missing capability (`system/init.capabilities` on headless, the pid status
    file on pty) to park the ticket `needs-repair` with the reason and a hint to run
    `gx claude doctor`. The hint is given whether the capability fails in `Start` (where pty sees
    the status file) or in `Prompt`. [T04, T09, P2-T03]
99. As a maintainer, I want preflight to never compare claude versions, only capabilities. [T09]
100. As a maintainer, I want CI to run the same check table against recorded fixtures (CI has no
     claude), so that the doctor and the tests cannot drift apart. [T09]
101. As a maintainer, I want fixtures to carry a header line with the producing claude version, and
     the doctor to warn when installed claude is newer than the fixtures. CI does not fail on old
     fixtures. [T09]
102. As a maintainer, I want the same fixtures to feed the native-runner test fakes. [T09]

### Phase 2: PTY runner launch, status and screen

103. As a user, I want the PTY runner to launch interactive claude in a ~200-column PTY owned by a
     `setsid` holder, with `--session-id <uuid>`, `--permission-mode auto`,
     `--settings '{"tui":"default"}'` (main screen, so scrollback exists) and
     `--disallowedTools AskUserQuestion`, plus the launch env of stories 35-36, so that it runs
     like the headless runner but as a TUI. The holder also takes the session id as an argument,
     so both cmdlines pass the liveness check of story 46. [P2-T02, P2-T05, P2-review]
104. As ralph-loop, I want PTY `Status` to come from `~/.claude/sessions/<pid>.json`: no `status`
     yet means starting, `busy` means `working`, `idle` means `idle`, and `waiting` means `blocked`
     with the `waitingFor` value as the reason, so that no screen scraping drives the state.
     Every in-session dialog sets `waiting` plus `waitingFor` (`permission prompt`,
     `input needed`, `dialog open`, and the rarer `goal proposal`, `sandbox request`,
     `worker request`). [P2-T01]
104a. As ralph-loop, I want PTY `Turn` to be a counter the holder keeps: it goes up on every
     `status` change in the pid file and is served over `holder.sock`, so that it survives a
     server restart. `Prompt` types the text plus Enter and waits a bounded time for `Turn` to
     move; if it does not, it nudges and retypes once (story 59); if it still does not, it returns
     `ErrNotDelivered`. [P2-review]
105. As a user, I want the trust dialog answered by a screen rule (anchor "Yes, I trust this
     folder"), because it shows before the pid file exists. It is the only dialog that needs a
     screen rule. [P2-T01]
106. As a user, I want a PTY start where no pid file appears within a set time to fail with the
     current screen in the reason and park `needs-repair` with the doctor hint (story 98), so that
     an unknown pre-session screen (login, onboarding) is visible. [P2-T01, P2-T03]
107. As a maintainer, I want every PTY screen rule (trust, rate limit, nudge checks) to match
     against the emulator's scrollback tail plus screen, with rows joined by a space, whitespace
     collapsed and gutter glyphs (`│`, `⏺`) stripped, against short normalized anchors, so that
     claude's own hard wrapping does not break a match. The raw log is never matched. [P2-T02]
108. As a maintainer, I want the terminal emulator to be `charmbracelet/x/vt`, pinned by
     pseudo-version (no vendoring) and wrapped behind a small gx interface, so that a fork can be
     swapped in with `replace` if needed. [P2-T02]
109. As a user, I want `answer` on a pty `permission prompt` (tool permission and plan approval) to
     map to keys: `allow` = Enter on the first option ("Yes"), `deny` = Esc, `deny <reason>` = Esc
     and then the reason sent as a normal prompt once the agent is idle. The holder runs Esc, the
     wait for idle and the reason as one step, and keeps reporting `blocked` until the reason is
     typed, so ralph-loop's `Wait` never sees the idle gap and closes the iteration early. If idle
     never comes within a short timeout, the step fails and story 112 parks `needs-repair`.
     [P2-T05, P2-review]
110. As a user, I want any other pty `waitingFor` value to park `needs-answer` with that value in
     the park reason, and to accept only `deny` (Esc). `allow` is refused with `invalid-decision`
     ("not a permission prompt; deny cancels it"). [P2-T05]
111. As a user, I want pty agents to ask their own questions through the ticket's
     `## Needs Answer` section (`AskUserQuestion` is disallowed), so that `input needed` is left
     only for MCP elicitation. [P2-T05]

### Phase 2: holder and reattach

112. As a user, I want a holder crash, or a failed write to `holder.sock`, to park the ticket
     `needs-repair`, so that a broken PTY never hangs a run. When claude exits while the server
     is up, the holder saves `screen.txt` and claude's exit code to the agent directory and exits
     too; the park reason then names the code ("claude exited (code N)") instead of a holder
     crash. [map, P2-T05, P2-review]
113. As a user, I want a PTY agent found dead after a server restart to always park `needs-repair`
     with `agent-died-while-server-down`, because interactive claude stays alive after its turn
     and so never dies as a normal finish. [P2-T03]
114. As a maintainer, I want PTY reattach to reconnect to `holder.sock` and read the pid status file
     again, with no log replay, because the holder keeps the emulator and serves the current screen
     and scrollback. [P2-T03, P2-T06]

### Phase 2: per-run runner routing

115. As a user, I want an iteration to stay on the runner that started it, across server
     restarts, so that I can change `agent-runner` while iterations are live. `agent-runner` only
     picks the runner for new launches. [P2-T04]
116. As a maintainer, I want the server to route every call for a run by the run's runner (reclaim,
     wait, finish, the agent verbs, budget kill, log prune), so that no call reaches a runner that
     did not start the session. [P2-T04]
117. As a user, I want a run whose runner is unavailable at reclaim (for example a herdr run while
     herdr is not running) to park `needs-repair` with a reason naming that runner ("started by
     herdr; herdr unavailable"). [P2-T04]
118. As a user upgrading gx, I want a run with no saved runner to count as herdr, as today. [P2-T04]
119. As a user, I want the server to log one line at start per non-configured runner that still has
     live runs (for example "2 runs still on headless"). No TUI or doctor change. [P2-T04]
120. As a user, I want a herdr run reclaimed by a server configured for headless to be waited on
     and finished through herdr, so that it is no longer left running unwatched. [P2-T04]
121. As a maintainer, I want every reader of an agent directory (watch, prune, log lookup, evolve
     digests) to dispatch on the runner in `meta.json`, so that headless and pty dirs sit side by
     side. [P2-T03, P2-T04]
122. As a maintainer, I want the host health check (herdr-only) to follow the configured runner
     only, so that old runs drain without it. [P2-T04]
123. As a user, I want `agent-runner: pty` accepted again (phase 1 rejected it), while `auto` still
     never picks pty. [P2-T03]

### Phase 2: TUI menu

124. As a TUI user, I want the `m` menu (suggested actions) to open on every ticket row, in any
     status (even closed), so that I can always reach an agent's history. The row badge stays tied
     to the unhealthy-status list, and the "no suggested actions" toast goes away for ticket rows.
     [P2-T06]
125. As a TUI user, I want a "Transcript" item, always listed on every runner, that renders the
     ticket's session as markdown (glamour), scrolled to the bottom. It shows the latest session in
     the ticket's `session_ids`; `[` and `]` step to older ones and the title shows "session N/M".
     With no `session_ids` (never ran, or reset) it shows a "no sessions" toast. [P2-T06]
126. As a TUI user, I want a "Watch" item, listed only while the agent's process or pane is live:
     herdr jumps to the pane; pty shows a screen snapshot from the holder; headless shows the
     rendered event-log snapshot. PTY and headless snapshots refresh with `r`; there is no live
     tail (`gx server agents watch -f` covers live). [P2-T06]

### Phase 2: doctor and shared helpers

127. As a user, when `gx claude doctor` picks the runner by default, I want the header to name it
     and say why, for example `runner: headless (agent-runner herdr; doctor never checks herdr)`.
     Not shown for an explicit `--runner`; `--json` is unchanged. [P2-T07]
128. As a user, I want PTY doctor rows: pid status file with `status` and `waitingFor`, the trust
     screen rule, the rate-limit screen rule (SKIP, "banner never captured", until story 58's
     follow-up lands), and the holder surviving `launchctl kickstart -k` with `holder.sock` still
     answering. The holder check gates the end of phase 2. These rows run live on pty. The
     headless checks phase 1 left as SKIP stay out of scope (a follow-up). [T09, map, P2-T01,
     P2-review]
129. As a maintainer, I want doctor check input to carry screen frames and status-file snapshots,
     not only events, and `--record` to save them, so that PTY checks run in CI on fixtures.
     [P2-T03]
130. As a maintainer, I want the rate-limit text helpers and the prompt nudge moved out of the herdr
     adapter into a runner-neutral package, so that pty reuses them without importing herdr.
     [P2-T03]

## Implementation Decisions

**Interface**

- A new package holds `Runner`. `ralphloop.Deps` drops its 13 herdr-shaped fields and gets one
  `Runner` field. `server.launch` and `finishRun` use it too. [T05]
- Operations: `Start`, `Prompt`, `Status`, `Wait`, `Interrupt`, `Stop`, `Find`, `List`,
  `RateLimit`, `Answer`. Signatures and semantics as in the user stories above. [T05]
- `Status` has `state`, `Turn`, `SessionID`, blocked reason. States: `working`, `idle`, `done`,
  `blocked`. [T05]
- Typed errors: `ErrLabelTaken`, `ErrNotDelivered`, `ErrNotReady`, plus a distinct wait-timeout
  error. They map to the existing park kinds (story 28). `agent_pane_busy` is herdr-internal. [T05]
- `Healthy() error` is optional and herdr-only. [T05]
- `server.Run` swaps `Pane`/`Tab` for `Session`. The `runs.json` loader maps old fields into the
  herdr session. [T05]
- `Start` takes an options value (label, epic, cwd, kind, args, env). No `interactive` field.
  [P2-T03]
- The PTY runner is a third `Runner` implementation and also implements the optional cleaner hook
  (it removes `holder.sock`). `ErrMissingCapability` covers "no pid status file"; `ErrNotReady`
  covers a dialog gx did not raise. [P2-T03]

**Herdr adapter**

- Wraps today's herdr behaviour: workspaces and tabs as a naming layer, nudges, dialog rules, pane
  scraping for rate limits and Codex context exhaustion, `trust_directory`. No behaviour change.
  [T05, T01]
- `TabFocus`, `terminalrun` and the worktree terminal menu stay in the herdr package and are hidden
  on native runners. [T05, map]

**Selection and preflight**

- `agent-runner` is a per-machine setting. `auto` = herdr if installed and reachable, else
  headless. `auto` never picks pty, but `pty` is a valid explicit choice (phase 1's rejection of
  it is undone, and the server's runner factory gains a pty branch). No per-epic override.
  [map, T04, P2-T03]
- Preflight (no model call): no `claude` on PATH or invalid `agent-runner` refuses to start (ticket
  stays queued, error shown). A missing launch-time capability parks `needs-repair` with the doctor
  hint, from `Start` as well as `Prompt`. Preflight probes only the configured runner. [T09,
  P2-T03, P2-T04]

**Per-run routing** (shared plumbing, all runners)

- The runner is chosen per run. `Run.Runner` is the source of truth; empty means herdr. Every
  server call site that acts on a run (reclaim, wait, finish, agent verbs, budget kill, log prune)
  asks for "the runner for this run", not "the runner for this project". [P2-T04]
- Runners are built lazily, one per (runner, project), like today's per-project headless map. PTY
  gets per-project agent roots. Runners other than the configured one are checked lazily per run.
  [P2-T04]
- A run's runner that is unavailable at reclaim parks `needs-repair` naming it. The health check
  follows the configured runner only. The server logs one line per non-configured runner with live
  runs. [P2-T04]
- This fixes the misroute where a herdr run reclaimed under a headless server was finished
  through headless. [P2-T04]
- Agent-directory readers (watch, prune, `lastTouched`, log lookup, evolve digests) dispatch on
  `Meta.Runner` instead of assuming headless. [P2-T03]

**Headless runner (phase 1)**

- `claude -p` with stream-json, `--permission-mode auto`, `--permission-prompt-tool stdio`,
  `DISABLE_AUTOUPDATER=1`, inherited `CLAUDECODE` and `CLAUDE_CODE_ENTRYPOINT` stripped, under
  `setsid`. Stdin is a FIFO opened
  read-write. Stdout is appended to `out.jsonl`. [T02, T04, T06]
- State comes from structured events: session state, command lifecycle, `rate_limit_event`,
  `compact_boundary`. `blocked` comes from a `stdio` permission request. [T02, T08]
- Prompt = stdin write. `Interrupt` and `/compact` act in the same session. [T08]
- Known risks: claude version drift, `--bare` becoming the `-p` default, and the undocumented
  `--permission-prompt-tool stdio`. [T02, T04]

**PTY runner (phase 2)**

- A per-agent `setsid` holder owns the PTY and serves `holder.sock`. Status comes from
  `~/.claude/sessions/<pid>.json`. gx owns the claude UI rules (trust dialog, rate-limit screen,
  nudge and retype) that herdr owns today. Raw PTY log, `screen.txt` and the transcript back the
  watch modes. About 1.7-2.5k lines of Go were estimated. [T03, T04, T06, T07]
- Launch: interactive claude, PTY ~200 columns wide, `--session-id <uuid>` (also an argument of
  the holder, for the liveness check), `--permission-mode auto`,
  `--settings '{"tui":"default"}'`, `--disallowedTools AskUserQuestion`, same env rules as
  headless. Reuses the native launch helpers (env, setsid spawn, uuid, meta write). [P2-T02,
  P2-T05, P2-T03]
- Status mapping from the pid file: missing `status` = starting; `busy` = `working`; `idle` =
  `idle`; `waiting` = `blocked` with `waitingFor` as the reason. `waitingFor` is absent (not null)
  when not waiting. Any unlisted dialog kind reports `permission prompt` (plan approval does).
  The file appears ~0.5 s after the trust dialog is accepted and is deleted on clean exit.
  [P2-T01]
- `Turn`: the holder counts `status` changes in the pid file and serves the count over
  `holder.sock`. `Prompt` = text + Enter, bounded wait for `Turn` to move, one nudge and retype,
  then `ErrNotDelivered`. [P2-review]
- Claude exit with the server up: the holder writes `screen.txt` and claude's exit code to the
  agent directory, then exits; the server parks `needs-repair` naming the code. [P2-review]
- Rate limit: PTY `RateLimit` returns nil in phase 2 (banner never captured); its doctor row
  SKIPs. A follow-up captures the banner and adds the rule. [P2-review]
- Holder crash or claude exit: `Status` and `Wait` return a new `agentrunner.ErrSessionDied`
  carrying the reason; the loop parks `needs-repair` with the park kind headless uses for a dead
  agent. [P2-review]
- `allow` on a non-permission dialog returns a new `agentrunner.ErrInvalidDecision`; the server
  maps it to the `invalid-decision` refusal. [P2-review]
- Holder binary and upgrades: the holder is a hidden `gx agent-holder` subcommand. `holder.sock`
  has a frozen core of ops that never change across gx versions: ping, status and `Turn`, screen
  and scrollback, send keys, stop. Ping returns the protocol version and the optional ops the
  holder supports. New features (the deny-with-reason step first) are optional ops: the server
  checks the list, falls back to the core when it can (Esc, then the reason as a normal prompt)
  or refuses only that action with a clear reason. A gx upgrade never parks a live PTY run; it
  finishes on the holder that started it. Handing the PTY to a new holder (fd passing) was
  rejected as too complex. [P2-review]
- PTY `Interrupt` sends Esc. The runner, not the holder, answers the trust dialog during `Start`;
  the holder knows only the pid file and the deny-with-reason step. [P2-review]
- The transcript background-task probe moves to the runner-neutral package with the rate-limit
  helpers (story 130). Prune handles every agent directory by its own `Meta.Runner`, whatever
  runner is configured. [P2-review]
- Screen rules: trust is required (anchors "Quick safety check: Is this a project you created or
  one you trust?" / "Yes, I trust this folder"). A startup timeout with no pid file fails `Start`
  with the screen in the reason. Optional rules may sharpen a blocked reason (plan vs permission,
  `AskUserQuestion` vs elicitation) but never drive state. [P2-T01]
- Screen text: emulator scrollback tail plus screen, rows joined with `" "`, `\s+` collapsed,
  gutter glyphs stripped, matched against short normalized anchors. `raw.log` is for `--raw` and
  forensics only. The window title is read by a raw-bytes OSC 0 scan (x/vt mangles it). [P2-T02]
- Emulator: `charmbracelet/x/vt`, pinned by pseudo-version, behind a small gx interface, patchable
  via `replace`. A fixture test that replays recorded claude raw output and checks the anchors
  gates every pin bump. [P2-T02]
- The holder keeps the emulator and serves the screen and scrollback over `holder.sock`, and saves
  the last screen to `screen.txt`. The server keeps no screen state, so reattach needs no replay.
  [P2-T06]
- Answer keys: `permission prompt` → `allow` = Enter (first option, "Yes"), `deny` = Esc,
  `deny <reason>` = Esc then the reason as a prompt once idle, run by the holder as one step that
  keeps reporting `blocked` until the reason is typed (timeout → `needs-repair`). Any other `waitingFor` parks
  `needs-answer` with the value in the reason; only `deny` (Esc) is accepted, `allow` returns
  `invalid-decision`. The verb contract stays `allow|deny [reason]`. [P2-T05]
- Liveness: holder pid and claude pid, each with start time, plus `holder.sock` answers. A holder
  crash or a failed socket write parks `needs-repair`. A dead PTY agent after a restart always
  parks `needs-repair` (`agent-died-while-server-down`). [map, P2-T03]
- `DetectRateLimit`, `SecondsUntilReset` and the prompt nudge move from the herdr adapter to a
  runner-neutral package; herdr and pty both use it. [P2-T03]

**Lifetime and reattach** (both runners)

- Agent directory, `meta.json` fields, liveness test, dead-agent rule, cleanup and replay rules as
  in stories 40-54, per runner (stories 113-114 for PTY). Retention is `server.log-retention`,
  default 30 days, pruned on server start and daily. [T06, T07, P2-T03]

**Server verbs and watch**

- New verbs under `gx server agents`: `watch`, `prompt`, `interrupt`, `answer`. All take a ticket
  address. `prompt`, `interrupt` and `answer` talk to the server; `watch` reads the agent directory
  itself. [T05, T07, T08, P2-T03]
- `watch` flags: `--follow/-f`, `--tail N`, `--json`; pty adds `--screen` and `--raw`, which read
  the agent directory (holder while live, else `screen.txt`; `raw.log`). The shared line renderer
  also feeds the TUI "Watch" item on headless. [T07, P2-T03, P2-T06]

**TUI menu** (all runners)

- The suggested-actions `m` menu opens on every ticket row, any status. It is the base for a later
  "context menu on every focused item" effort, so no new keybinding. [P2-T06]
- "Transcript": always listed. Renders the session through the existing claude and codex markdown
  exporters and the ticket markdown renderer. The session lookup used by evolve digests moves to a
  shared package so evolve and the TUI resolve sessions the same way. [P2-T06]
- "Watch": listed only while the agent is live. herdr = jump to pane ("Answer in pane" is
  removed); pty and headless = a live modal following the rendered conversation, ticking only
  while open (see queue-tab-actions.md). [P2-T06]

**Doctor**

- One table of named checks drives `gx claude doctor` (real claude) and CI (fixtures). [T09]
- Headless checks: `system/init` capabilities; session id honoured; transcript written;
  `rate_limit_event`, `compact_boundary` and command-lifecycle events; `stdio` permission request
  raised and `answer` via FIFO works; interrupt; `/compact` in the same session; survives launcher
  exit. [T09]
- PTY checks: the same where they apply, plus the pid status file with `status` and `waitingFor`,
  screen rules (trust dialog, rate limit), and holder survives `kickstart -k` and `holder.sock`
  answers. The survival check needs a PTY live driver, not only `claude -p`. [T09, P2-T01, P2-T03]
- Check input carries events, screen frames and status-file snapshots; `--record` saves all three.
  [P2-T03]
- No `--runner`: use the configured `agent-runner` (config only): `pty` → pty, anything else →
  headless. The header names the runner and why when defaulted. [P2-T07]
- `MinClaudeVersion` is one Go constant in the package that owns the native runners. [T09]
- Fixtures are copied by hand into `testdata/` from `--record`. [T09]

**Investigate**

- No special code. S6 creates an investigate ticket (commitless child of the failed ticket) and it
  runs as an ordinary iteration on every runner. Manual `m -> Investigate` behaves per story 86-87.
  [T08]

## Testing Decisions

- **What is a good test:** it checks external behaviour only (what `Runner` returns, what files
  appear, what CLI prints), not internals.
- **Highest seam: the `Runner` contract.** One shared contract test suite runs against the
  in-memory fake, the herdr adapter (on `testutil/herdrfake`) and each native runner (on a
  fixture-driven fake of claude, `testutil/agentfake`). It covers start, prompt, `ErrNotDelivered`,
  status and `Turn`, wait timeout, interrupt then prompt, idempotent stop, find/list, rate limit,
  and blocked then answer.
- **ralphloop tests** use the small in-memory `Runner` fake and cover park-kind mapping, the
  fresh-session rule, smart-zone and budget stop. This replaces the ~13 per-field stubs.
- **Reattach tests** work on a temp agent dir: live, dead with `result`, dead without `result`, pid
  reuse (start time mismatch), replay idempotency by `uuid`, pending `request_id` after restart,
  and pruning (30 days, parked never pruned).
- **Doctor table tests:** the check table runs in CI against recorded fixtures, with a version
  header and a "newer claude than fixtures" warning test. Output format, `SKIP` chaining, exit code
  and `--json` are tested on the table.
- **Watch CLI tests:** golden tests on the shared renderer for headless `out.jsonl` and the
  transcript, plus `--tail`, `--json`, follow-then-exit on a finished agent, and the pruned-log
  message. PTY adds `--screen` and `--raw`.
- **Real-claude runs** are manual (`gx claude doctor`). CI never needs claude.
- **Process-survival** (`setsid`, FIFO, holder) is checked by the doctor on real claude and by
  small integration tests with a fake long-running child.
- **Prior art:** `testutil/herdrfake` and its use in the ralphloop and server tests; existing
  `gx claude` subcommand tests; golden tests elsewhere in the repo.

Phase 2 additions (seams: the existing `Runner` contract suite stays the highest seam; no new seam
is added beyond the PTY fake's side channel):

- **PTY conformance:** the shared contract suite runs against the PTY runner on a new TUI fake of
  claude. The fake draws a screen, writes `~/.claude/sessions/<pid>.json` (under a temp home) and
  takes test controls on a side channel. The existing title-OSC TUI fake in `testutil/agentfake`
  is its base; the headless conformance harness is the model to copy. [P2-T03]
- **PTY answer keys:** contract cases for `permission prompt` allow (Enter) and deny (Esc, and Esc
  then prompt with a reason), and for a non-permission `waitingFor` (deny only, allow refused with
  `invalid-decision`). A check that the first option is "Yes" on permission and plan approval runs
  against recorded screens. [P2-T05]
- **Screen rules:** table tests on recorded claude screens (trust dialog, startup with no pid
  file) through the join-and-normalize matcher. A replay test over recorded raw output gates x/vt
  pin bumps. [P2-T01, P2-T02]
- **Reattach and liveness (PTY):** temp agent dir with a fake holder: live (both pids and the
  socket), holder gone, socket silent, pid reuse, and "dead after restart parks needs-repair".
  [P2-T03]
- **Per-run routing:** server tests with two fake runners: a run reclaimed, waited on, finished,
  answered, budget-killed and pruned through the runner that started it while the configured
  runner differs; empty runner = herdr; unavailable runner parks with its name; the herdr-under-
  headless misroute as a regression test. [P2-T04]
- **Agent-dir dispatch:** watch, prune, log lookup, evolve digests and `lastTouched` tests on a
  mixed dir (headless + pty). [P2-T03, P2-review]
- **PTY turn and delivery:** contract cases for `Turn` moving on each status change, nudge and
  retype once, then `ErrNotDelivered`; `deny <reason>` never shows `idle` before the reason is
  typed, and its idle timeout parks `needs-repair`. [P2-review]
- **PTY lifecycle:** startup with no pid file within the timeout parks `needs-repair` with the
  screen and doctor hint (story 106); claude exiting with the holder alive parks naming the exit
  code (story 112); launch args hold `--session-id` and `--disallowedTools AskUserQuestion`
  (stories 103, 111). [P2-review]
- **Start log line:** the server logs one line per non-configured runner with live runs
  (story 119). [P2-review]
- **TUI menu:** tests on the suggested-actions model: menu opens on every status; Transcript
  always listed (session stepping, "no sessions" toast); Watch only while live, per runner.
  [P2-T06]
- **Doctor:** PTY check rows on recorded frames and status snapshots; default-runner mapping and
  header text for each `agent-runner` value. [P2-T03, P2-T07]

## Out of Scope

- Herdr-free TUI terminal features (`ui/terminalrun`, the worktree terminal menu). [map]
- A tmux runner: another external tool, not needed while a native runner works. [T04]
- `claude --bg` / `claude agents` as a runner: no clean way to send a prompt or `/compact`, no
  interrupt without a gx PTY, its own org kill switch, research preview. Later idea outside this
  spec: hand a parked ticket to a person with `claude --resume <id> --bg` and `claude attach`.
  [T04]
- Codex on the native runners. [map]
- The Claude Agent SDK (adds a Node or Python runtime dependency). [map]
- Claude Code hooks as a status channel (blocked in some orgs). [map]
- Live terminal attach to a native agent. [map, T05]
- Auto-allow of permission prompts. A config allow-list is a later idea. [T08]
- Automatic `--resume` of a dead agent. [T06]
- Fixing the server path's known gaps (smart-zone, blocked, rate-limit handling). [T05]
- Pane scraping for herdr Codex in the watch CLI. [T07]
- Auto-launch of investigate tickets: owned by `orchestrator-daemon` S6. [T08]
- Any change to headless runner behaviour in phase 2. Shared plumbing that headless also uses is
  allowed: stories 53, 98, 116-121, 124-127 and 129. [P2 map, P2-review]
- Running the headless doctor checks that phase 1 left as SKIP: a follow-up. [P2-review]
- The PTY rate-limit screen rule: a follow-up captures the banner first. [P2-review]
- Free-text answers or menu-number answers to a dialog. [P2-T05]
- A live tail in the TUI "Watch" item, and a screen toggle in "Transcript". [P2-T06]
- The herdr bug where `answer deny "reason"` types the reason into the dialog instead of pressing
  Esc: filed as a follow-up in `gx:follow-ups`. [P2-T05]
- A sticky runner (keep the old runner until its runs drain): rejected for per-run routing.
  [P2-T04]

## Further Notes

- Terms (in `CONTEXT.md`): agent runner, native agent runner, headless runner, PTY runner.
- Findings behind the decisions are in map tickets 01 (herdr touchpoints: 13 commands, 11 MUST
  capabilities), 02 (`claude -p` meets all 11) and 03 (PTY: 7 of 11 direct, 4 need workarounds).
- launchd stop and `kickstart -k` kill a plain child but leave a `setsid` child alive. This is why
  every native agent or holder starts in its own session. [T06]
- Dependencies: `orchestrator-daemon-3-cutover` blocks implementation; `orchestrator-daemon` S6
  provides investigate tickets.
- Phase 2 builds on phase 1 (`agent-runner-phase1-impl`): the `Runner` interface, the watch
  renderer, the doctor framework, the lifetime and reattach layout and the conformance suite.
  Phase 2 tickets go to `agent-runner-phase2-impl`. [P2 map]
- Facts behind the PTY decisions were measured on claude 2.1.296; evidence sits beside the
  `agent-runner-phase2` map tickets. [P2-T01, P2-T02]
- Open PTY risks: the rate-limit banner was never seen, so its wrapping is unknown; the
  join-and-normalize matcher is untested on bordered dialogs (permission prompt); one odd
  scrollback row is unexplained. Launching in a dir claude already trusts avoids the trust dialog
  (trust is inherited from a trusted parent). Test runs leave temp-dir trust entries in
  `~/.claude.json`. [P2-T01, P2-T02]
- Screens before the main session (trust; probably login, onboarding, bypass-permissions warning)
  never write the pid file. [P2-T01]
- `setsid` children survive `launchctl kickstart -k` (tested 2026-10-08); the holder-specific
  doctor check (story 128) still gates the end of phase 2. [P2 map]
- As-built gaps phase 1 left: the doctor runs only the "turn" check live (others SKIP) and does
  not pin a model; `--runner pty` printed only a header. [P2-T03]
