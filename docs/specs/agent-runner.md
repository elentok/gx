# Agent runner: a herdr-free way to run ralph-loop agents

Source: wayfinder map `agent-runner` (tickets 01-09). Decision tags like `[T05]` point to the map
ticket that settled the point. ADR: 0031 (two native agent runners).

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
- A per-machine setting picks the runner: `agent_runner: auto | herdr | headless | pty`.
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
6. As a user, I want `agent_runner: auto | herdr | headless | pty` in my per-machine config, so
   that I choose how agents run. [map: Selection]
7. As a user with herdr installed and reachable, I want `auto` to pick herdr, so that nothing
   changes for me. [T04]
8. As a user without herdr, I want `auto` to pick headless, so that it works out of the box. [T04]
9. As a user, I want `auto` to never pick pty, so that a hard-to-detect "`-p` is blocked" case is
   always my own explicit choice. [T04]
10. As a user, I want no per-epic override of the runner, so that the setting stays simple. [map]
11. As a user, I want a bad `agent_runner` value to stop the start with a clear error, so that I
    fix it before work begins. [T09]
12. As a user, I want a missing `claude` on PATH to stop the start with a clear error and leave
    the ticket queued, so that nothing is parked wrongly. [T09]

### Operations and status

13. As ralph-loop, I want `Start(label, cwd, kind, args, env, interactive)` to return a `Session`
    once the agent is idle, so that I can send the first prompt safely. [T05]
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
36. As a user, I want inherited `CLAUDE*` env vars stripped at launch, so that the child session
    still saves its transcript. [T06]
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
    project and named `<epic>-<id>`, holding `meta.json`, the `stdin` FIFO, `out.jsonl` and (PTY)
    `holder.sock`, so that identity and I/O live in one place. [T06]
45. As a maintainer, I want `meta.json` to hold runner, pid, pid start time, session id, cwd,
    branch and saved read offset, so that reattach can verify identity. [T06]
46. As a maintainer, I want an agent to count as live only if the pid exists, the start time
    matches and the cmdline holds the session id (PTY: and `holder.sock` answers), so that a
    reused pid is never adopted. [T06]
47. As a maintainer, I want the reattach scan to cover claimed and `needs-repair` tickets, so
    that parked agents are found too. [T06]
48. As a user, I want an agent that died while the server was down to count as a normal finish
    when `out.jsonl` ends with a `result` event, so that finished work is landed. [T06]
49. As a user, I want any other dead agent to park `needs-repair` with
    `agent-died-while-server-down` and keep its directory, so that I can look at it. [T06]
50. As a user, I want no automatic `--resume` of a dead agent, so that gx never repeats work by
    surprise. [T06]
51. As a maintainer, I want the server to reopen `out.jsonl` at the saved offset, replay it to
    rebuild state, save the offset only at event boundaries, and treat events as idempotent by
    `uuid`, so that reattach is exact. [T06]
52. As a maintainer, I want the FIFO, socket and pid files removed on land or cancel, so that no
    stale files stay. [T06]
53. As a user, I want `out.jsonl` and `meta.json` kept for N days (config, default 30) and pruned
    on server start, so that I can inspect recent runs. [T06, T07]
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
    and from the PTY's own screen buffer on pty. [T08]
59. As a user, I want prompt delivery on headless to be a stdin write, and the pty runner to do
    its own nudge and retype. [T08]
60. As a user, I want the server's own run path to go through `Runner` now, while its known gaps
    (no smart-zone, blocked or rate-limit handling) stay as they are, so that this spec stays
    bounded. [T05]
61. As a user, I understand the three native gaps: no `trust_directory` on headless (nothing to
    answer), no live view of a stuck TUI (the log is shown instead), and pty needs the undocumented
    `~/.claude/sessions/<pid>.json` status file. [T08]

### Blocked prompts and acting on a live agent

62. As a user, I want a permission request on headless to park the ticket `needs-answer` and keep
    the request open, with the tool name and input in the park reason, so that I can decide. [T08]
63. As a user, I want `gx server agents answer <addr> allow|deny` to resolve a blocked prompt, so
    that I can unblock an agent in one command. [T08, T05]
64. As a user, I want gx to never auto-allow a permission prompt on any runner, so that agents stay
    inside my permission rules. [T08]
65. As a user, I want only `trust_directory` auto-answered, and only on herdr and pty. [T08]
66. As a user, I want a pending headless request to stay answerable after a server restart when its
    `request_id` has no response in `out.jsonl`, so that a restart does not lose it. [T08]
67. As a user, I want a failed FIFO write on `answer` to park `needs-repair`, as for a dead agent
    without a `result`. [T08]
68. As a user, I want `gx server agents prompt <addr>` and `interrupt <addr>` as one-shot verbs, so
    that I, investigate or a future watchdog can steer a live agent. [T05]
69. As a user, I want these verbs to take a ticket address (`<epic>/<id>`) that resolves to the
    live iteration, so that I never type a label. [T08]
70. As a user, I want no live terminal attach, so that the design stays within what native
    runners can promise. [map]

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
76. As a maintainer, I want one shared renderer used by follow, dump and the TUI "Watch agent"
    modal, so that all views look the same. [T07, T05]
77. As a maintainer, I want no second log: headless reads `out.jsonl`, pty and herdr claude read
    the transcript. [T07]
78. As a user of the PTY runner, I want `--screen` to print the current screen ("what is it stuck
    on") and `--raw` to replay the ANSI log. [T07]
79. As a user of herdr claude agents, I want watch to read the transcript (found with
    `gx claude session-path`), so that I have one way to look at any claude agent. [T07]
80. As a user of herdr Codex agents, I want a clear "not supported" message and no pane scraping.
    [T07]
81. As a user, I want a finished agent within retention to dump from the saved log and `--follow`
    to print it and exit. [T07]
82. As a user, I want a pruned log to give "log pruned" and the transcript path. [T07]
83. As a TUI user on a native runner, I want "Answer in pane" replaced by "Watch agent", a modal
    that tails the iteration log live. It is not a `Runner` method. [T05]
84. As a TUI user on herdr, I want jump-to-pane and the other herdr terminal features
    (`ui/terminalrun`, worktree terminal menu) unchanged, and hidden when herdr is missing. [map]

### Investigate

85. As a user on a native runner, I want an investigate ticket to run as an ordinary iteration, so
    that no special launch code exists. [T08]
86. As a user, I want the manual `m -> Investigate` item on a native runner to enqueue an
    investigate ticket on the parked ticket (same as the automatic path), started by the server.
    [T08]
87. As a user on herdr, I want `m -> Investigate` to keep opening an attended, focused tab, so
    that nothing changes. [T08, T05]
88. As a maintainer, I want the `interactive` flag on `Start` to be honoured on herdr only, so that
    native runs are always non-interactive and watched. [T05]

### Claude doctor and preflight

89. As a user, I want `gx claude doctor` (beside `statusline|history|session-path`) to run a short
    canned haiku session in a temp dir and report each assumption gx relies on, so that I see
    drift before it breaks a run. [T04, T09]
90. As a user, I want output as `PASS|FAIL|SKIP <name> - <detail>` lines with a header showing the
    installed claude version and `MinClaudeVersion`, and exit 1 on any FAIL. [T09]
91. As a user, I want a check that depends on a failed check to show `SKIP`. [T09]
92. As a user, I want `--runner headless|pty` to check one runner, defaulting to the runner
    `agent_runner` selects (`auto` means headless). [T09]
93. As a script author, I want `--json` to print `[{runner,name,status,detail}]`. [T09]
94. As a maintainer, I want `--record <dir>` to save the raw events of each canned session, so
    that I can refresh test fixtures. [T09]
95. As a user, I want the doctor to never check herdr and `gx doctor` to stay repo-only. [T04, T09]
96. As a maintainer, I want the doctor to run only when asked, never as preflight. [T04]
97. As a user, I want launch-time preflight to be cheap and make no model call: `claude` on PATH,
    valid `agent_runner`, and the launch-time capability. [T09]
98. As a user, I want a missing capability (`system/init.capabilities` on headless, the pid status
    file on pty) to park the ticket `needs-repair` with the reason and a hint to run
    `gx claude doctor`. [T04, T09]
99. As a maintainer, I want preflight to never compare claude versions, only capabilities. [T09]
100. As a maintainer, I want CI to run the same check table against recorded fixtures (CI has no
     claude), so that the doctor and the tests cannot drift apart. [T09]
101. As a maintainer, I want fixtures to carry a header line with the producing claude version, and
     the doctor to warn when installed claude is newer than the fixtures. CI does not fail on old
     fixtures. [T09]
102. As a maintainer, I want the same fixtures to feed the native-runner test fakes. [T09]

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

**Herdr adapter**

- Wraps today's herdr behaviour: workspaces and tabs as a naming layer, nudges, dialog rules, pane
  scraping for rate limits and Codex context exhaustion, `trust_directory`. No behaviour change.
  [T05, T01]
- `TabFocus`, `terminalrun` and the worktree terminal menu stay in the herdr package and are hidden
  on native runners. [T05, map]

**Selection and preflight**

- `agent_runner` is a per-machine setting. `auto` = herdr if installed and reachable, else
  headless. `auto` never picks pty. No per-epic override. [map, T04]
- Preflight (no model call): no `claude` on PATH or invalid `agent_runner` refuses to start (ticket
  stays queued, error shown). A missing launch-time capability parks `needs-repair`. [T09]

**Headless runner (phase 1)**

- `claude -p` with stream-json, `--permission-mode auto`, `--permission-prompt-tool stdio`,
  `DISABLE_AUTOUPDATER=1`, inherited `CLAUDE*` env stripped, under `setsid`. Stdin is a FIFO opened
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

**Lifetime and reattach** (both runners)

- Agent directory, `meta.json` fields, liveness test, dead-agent rule, cleanup and replay rules as
  in stories 40-54. Retention is a config value, default 30 days, pruned on server start. [T06, T07]

**Server verbs and watch**

- New verbs under `gx server agents`: `watch`, `prompt`, `interrupt`, `answer`. All take a ticket
  address and talk to the server. [T05, T07, T08]
- `watch` flags: `--follow/-f`, `--tail N`, `--json`; pty adds `--screen` and `--raw`. One shared
  renderer also feeds the TUI "Watch agent" modal. [T07]

**Doctor**

- One table of named checks drives `gx claude doctor` (real claude) and CI (fixtures). [T09]
- Headless checks: `system/init` capabilities; session id honoured; transcript written;
  `rate_limit_event`, `compact_boundary` and command-lifecycle events; `stdio` permission request
  raised and `answer` via FIFO works; interrupt; `/compact` in the same session; survives launcher
  exit. [T09]
- PTY checks: the same where they apply, plus the pid status file with `status`, screen rules (trust
  dialog, rate limit), and holder survives and `holder.sock` answers. [T09]
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

## Further Notes

- Terms (in `CONTEXT.md`): agent runner, native agent runner, headless runner, PTY runner.
- Findings behind the decisions are in map tickets 01 (herdr touchpoints: 13 commands, 11 MUST
  capabilities), 02 (`claude -p` meets all 11) and 03 (PTY: 7 of 11 direct, 4 need workarounds).
- launchd stop and `kickstart -k` kill a plain child but leave a `setsid` child alive. This is why
  every native agent or holder starts in its own session. [T06]
- Dependencies: `orchestrator-daemon-3-cutover` blocks implementation; `orchestrator-daemon` S6
  provides investigate tickets.
