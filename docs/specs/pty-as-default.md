# PTY as the default agent runner

## Problem Statement

gx hosts its Queue agents on herdr by default. `agent-runner: auto` picks herdr when herdr answers,
and falls back to the headless runner otherwise. A person who wants to depend less on herdr can set
`agent-runner: pty` by hand, but the PTY runner still has gaps that herdr does not have:

- A person cannot open a PTY Queue agent and type into it. "Go to pane" works only on herdr.
- The TUI's Investigate item always opens a herdr tab, whatever the runner.
- After a server restart, PTY `Find` and `List` only see agents that a tracked run re-adopted. A
  paused ticket's agent can outlive its run, so gx forgets it, and a resume starts a second claude
  under the same label. PTY `Start` deletes a live holder's socket and does exactly that.
- Repair merge's live-agent guard and `verify`'s leftover check always ask herdr, whatever runner
  hosts the ticket.
- `Pty.RateLimit` never reports a limit.
- The server wait loop never acts on a rate limit, on any runner. A rate-limited agent looks idle,
  gets the stall nudge, and ends in a finish or a zero-commit pause. The code that handled rate
  limits sits in the old in-process loop, which production no longer runs.
- The server wait loop pauses a blocked agent only on the headless runner. A PTY or herdr agent that
  blocks on a prompt gx did not send waits forever, and nothing tells a person.
- The herdr watch follows only the configured runner. After a switch to PTY it would go quiet, yet
  Codex tickets and running herdr iterations still need herdr.
- A Codex ticket on PTY or headless launches claude, because native runners ignore the agent kind.

## Solution

The PTY runner closes these gaps, and `agent-runner: auto` picks PTY whenever claude is on PATH.
herdr and headless run only when a person sets them by hand. Codex never runs on PTY in this effort:
on a native runner, a Codex ticket runs on herdr.

From the person's side:

- After the next server restart, a person on `auto` runs every Claude Queue agent on PTY. The
  changelog says so and names `agent-runner: herdr` as the way back. Running herdr iterations keep
  herdr until they end.
- "Go to pane" on a PTY Queue agent opens it in the TUI Agents tab's live terminal panel. The panel
  opens read-only, and a confirm unlocks typing.
- The TUI Agents tab and the web Agents page list PTY Queue agents and Design agents in their own
  groups. Both groups are hidden by default and open read-only. Their rows show only a status mark
  and never count as "needs you".
- Investigate in the TUI starts a Fix agent through the same route as the web "Fix agent" button,
  and lands on it in the Agents tab, ready for typing.
- A blocked agent on any runner pauses the ticket `needs-answer`, with a reason that says where to
  answer.
- A rate-limited agent on any runner holds in place. The ticket shows "rate-limited, resets <time>".
  gx sends "continue" at the reset and starts no new ticket for that harness until then.
- A herdr outage holds only the tickets that need herdr. The "herdr down" banner shows only while
  one of those is queued or running.

## User Stories

### Switching the default

1. As a person on `agent-runner: auto` with claude on PATH, I want gx to run my Claude Queue agents
   on the PTY runner after a server restart, so that my queue no longer depends on herdr.
2. As a person on `auto` without claude on PATH, I want `gx server` to refuse to start with an error
   that names claude and suggests `agent-runner: herdr`, so that I know what is missing and how to
   keep running on herdr (for example with only Codex installed).
3. As a person who wants herdr back, I want `agent-runner: herdr` to keep working exactly as before,
   so that herdr stays a backup.
4. As a person who wants the headless runner, I want `agent-runner: headless` to keep working, so
   that I can still pick it by hand.
5. As a person with herdr iterations running when the server restarts on PTY, I want those
   iterations to finish on herdr, so that nothing I started is lost or moved mid-run.
6. As a person reading the changelog, I want an entry that says `auto` now picks PTY after a server
   restart, that no config migration is needed, that `auto` without claude on PATH now refuses to
   start, and that `agent-runner: herdr` brings herdr back, so that the switch does not surprise me.
7. As a contributor, I want an ADR that supersedes ADR 0031's "PTY is never picked automatically",
   and a glossary that no longer calls PTY "the backup" or headless "the default native choice", so
   that the docs match the code.
8. As a person who uses split launch (tmux, kitty, herdr, Ghostty), I want it to behave as before,
   so that the runner switch does not change how my terminal opens programs.

### Codex routing

9. As a person on PTY or headless with a Codex ticket in the queue, I want the server to start that
   ticket on herdr by itself, so that Codex keeps working while PTY cannot host it.
10. As a person whose Codex ticket is queued while herdr is down or not on PATH, I want the ticket to
    wait with a clear reason, so that I know it waits on herdr and not on a bug.
11. As a person whose Codex ticket runs on herdr under a native configured runner, I want its launch
    to use the `$gx-…` skill prefix as on herdr today, so that the Codex agent runs the right skill.

### The herdr watch

12. As a person on PTY with herdr on PATH, I want the server to keep probing herdr, so that it knows
    whether Codex tickets and leftover herdr iterations can run.
13. As a person on PTY while herdr is down, I want my Claude tickets to keep starting, so that a herdr
    outage does not stop work that does not need herdr.
14. As a person on PTY while herdr is down, I want only the tickets that run on herdr to wait, so that
    a claim never starts without an agent behind it.
15. As a person on PTY, I want the "herdr down" banner to show only while a queued or running ticket
    needs herdr, so that I am not warned about a tool I am not using. herdr missing from PATH counts
    as down, and the banner then says herdr was not found.
16. As a person on PTY, I want no separate PTY health banner, so that the screen shows only what needs
    me. PTY launch failures already pause with a reason.

### Live view for PTY Queue agents

17. As a person, I want "Go to pane" on a PTY Queue agent to open the TUI Agents tab's live terminal
    panel on that agent, so that I can watch it as it works.
18. As a person, I want "Go to pane" on PTY to be dimmed only when the agent's holder is gone, not
    when herdr is down, so that the reason matches the runner.
19. As a person, I want "Go to pane" on a herdr Queue agent to focus its herdr tab as before, so that
    herdr users lose nothing.
20. As a person, I want a Queue agent or Design agent to open read-only in the Agents tab and on the
    web Agents page, so that I do not type over gx's driving by accident.
21. As a person, I want read-only to still allow scrolling, the mouse wheel, copy, and the ticket
    menu's prompt, interrupt and answer verbs, so that I can read and act without unlocking.
22. As a person, I want to unlock typing through a confirm dialog that names the kind of agent, with
    Cancel focused by default, so that unlocking is a deliberate choice.
23. As a person, I want each new open of a Queue agent or Design agent to start read-only again, so
    that an unlock does not carry over by surprise.
24. As a person on the web Design page, I want its composer to stay writable, so that the Design page
    keeps working as before.
25. As a person typing into an unlocked Queue agent, I want gx's next prompt to wait while my draft is
    in the input, so that gx never types over me.
26. As a person typing into a Queue agent, I want my key or paste to stop gx's pending nudges for the
    current prompt, so that gx does not fight my input.
27. As a person typing into a Queue agent, I want each key to restart the idle and stall timers, so
    that gx does not nudge or stall-pause an agent I am working with.
28. As a person only watching a Queue agent, I want watching to change nothing in the loop, so that
    looking is always safe.
29. As a person viewing at a width other than 200 columns, I want gx's screen rules to keep working on
    a PTY Queue agent, so that the live view does not break status, nudges or answers.

### Queue agents and Design agents in the Agents list

30. As a person, I want the TUI Agents tab and the web Agents page to list Queue agents hosted on the
    PTY runner in a "Queue agents" group, so that I can find them next to my sessions.
31. As a person, I want herdr and headless Queue agents to stay off the Agents list, so that the list
    shows only agents it can open.
32. As a person, I want the web "Design sessions" switch renamed "Design agents", with a new "Queue
    agents" switch above it, so that both groups have the same control.
33. As a person in the TUI, I want `t q` and `t d` to show or hide Queue agents and Design agents, and
    I want both keys in the Agents tab's `?` help, so that the TUI matches the web.
34. As a person, I want both groups hidden by default and my switch state remembered, so that the list
    stays short unless I ask.
35. As a person, I want a hidden group to show how many agents it holds, so that I know they exist.
36. As a person, I want an agent I open on purpose ("Go to pane", the jump list, a Design run from the
    Queue tab, the selected session) to show even when its group is hidden, so that I always see what
    I asked for.
37. As a person, I want Queue agent and Design agent rows to show only a status mark (Working, Idle,
    Blocked, Rate-limited, Exited), with no Unread or Actionable mark, so that the ticket's own pause
    stays the one place that asks me to act.
38. As a person, I want "Agents · N needs you" in the TUI and the web to count only my own interactive
    sessions, not Queue agents or Design agents, so that the count is not doubled by a pause I
    already see.

### Investigate as a Fix agent

39. As a person, I want the TUI ticket menu's Investigate to start a Fix agent through the fix-agent
    route on every runner, so that Investigate no longer needs herdr.
40. As a person, I want Investigate offered on a ticket paused for an answer, paused for repair, or in
    error, and I want the agent's prompt to name the real state, so that the agent knows what it is
    looking at.
41. As a person, I want the web "Fix agent" button to stay repair-only, so that the web does not
    change.
42. As a person, I want the TUI to switch to the Agents tab with the new Fix agent selected and
    writable, so that I can talk to it at once.
43. As a person, I want a second Investigate on the same ticket to open the existing Fix agent, so
    that I never get two.
44. As a person, I want Investigate on a ticket with an open investigate ticket to show the route's
    "already being investigated in `<addr>`" refusal, so that the TUI matches the web.
45. As a person, I want the Fix agent to end like any interactive session (I close it, or it exits),
    so that gx does not kill it when the ticket leaves its paused state.

### Finding PTY agents after a restart

46. As a person whose paused ticket's PTY agent outlived its run, I want gx to find that agent after a
    server restart, so that a resume or relaunch never starts a second claude under the same label.
47. As a person, I want `List` for an epic to include every live PTY agent of that epic, tracked or
    not, so that leftover checks see them.
48. As a person, I want a PTY launch with a label held by a live holder in the same directory to adopt
    that holder, unless the launch asks for no adoption, so that PTY acts as herdr does.
49. As a person, I want a PTY launch with a label held by a live holder in another directory, or with
    no adoption asked for, to fail with "label taken" and the holder's directory, without touching
    the holder's files, so that the server's stale-holder path works on PTY unchanged.
50. As a person, I want a holder whose process lives but whose socket stays silent to count as taken,
    so that gx never starts a second claude next to a hung one, and `Stop` can still kill it.
51. As a person running repair merge or `verify`, I want the live-agent guard and the leftover check
    to ask the runner that hosts the ticket, so that they see PTY agents.
52. As a person reading `--ignore-live-tab` help, I want it to say "agent session" instead of "herdr
    tab", so that the help fits every runner.

### Blocked agents

53. As a person, I want a Queue agent that blocks on a prompt gx did not send to pause its ticket
    `needs-answer` on every runner that reports blocked (PTY, herdr, Codex on herdr, headless), so
    that a blocked agent never waits unseen.
54. As a person, I want PTY and herdr to wait 15 seconds and check again before pausing, so that a
    prompt claude closes by itself does not pause the ticket.
55. As a person, I want the headless runner to keep pausing at once, so that its behaviour does not
    change.
56. As a person, I want the PTY pause reason to name the web answer button, `gx server agents answer`,
    and typing in the Agents tab live panel, so that I know every way to answer.
57. As a person, I want a web button to end every blocked pause, so that no pause needs a CLI step
    (ADR 0042). Approve/Deny ends a pause on an approvable tool card. On any other dialog (a question
    menu), Retry on PTY or herdr interrupts the agent and then sends the retry prompt.
58. As a person who answers in the live panel instead, I want the ticket to lift its pause once the
    agent leaves the blocked state, so that I do not have to answer twice.

### Rate limits

59. As a person, I want a rate-limited PTY agent to be noticed from its transcript, with the reset time
    read from claude's message (or from the statusline usage record when the message has none), so
    that PTY reports limits as herdr does.
60. As a person, I want the server wait loop to handle a rate limit on every runner that reports one
    (PTY, herdr, headless, Codex quota on herdr), and to check it before the blocked pause, so that a
    quota banner never parks a ticket `needs-answer`.
61. As a person, I want a rate-limited run to hold its slot rather than pause, so that gx recovers on
    its own (ADR 0042).
62. As a person, I want the ticket to show "rate-limited, resets <time>" while it holds, so that I know
    why it is quiet.
63. As a person, I want no stall nudge during the hold, so that gx does not prompt an agent that cannot
    answer.
64. As a person, I want gx to send "continue" at the reset time, so that the agent picks up where it
    stopped.
65. As a person, I want gx to send "continue" every 5 minutes when it has no reset time (no usage data
    and none in the message), and to stay on hold only if that turn hits the limit again, so that the
    hold still ends on PTY, whose transcript keeps the old error while the agent is idle.
66. As a person, I want the server to claim no new tickets for that harness until the reset, and the
    waiting tickets to say why, so that gx does not start agents that would hit the same limit.
67. As a person, I want a hold to come back after a server restart, so that a restart does not lose
    it.
68. As a person, I want interactive sessions to keep showing the raw rate-limit message without
    pausing, so that they do not change.
69. As a person running `gx claude doctor`, I want the PTY `rate-limit-screen` check to become a live,
    transcript-based check, so that the check no longer reads "skipped".

### Doctor and leftovers

70. As a person, I want `gx doctor --check-blocked-form` to say that it checks herdr only, and to point
    PTY users to `gx claude doctor`, so that I run the right check.
71. As a contributor, I want the blocked-form runbook and the code comment to describe the server wait
    loop's blocked pause, so that the docs match the running code.
72. As a contributor, I want no server code path to build a herdr runner by default when it needs the
    runner that hosts a ticket, so that PTY tickets are never checked against herdr.

### Usage gate

73. As a person on a claude.ai Pro/Max account, I want gx to stop claiming new Claude tickets once the
    5-hour or weekly usage reaches 98%, so that new agents do not start only to hit the limit.
    Running agents keep working.
74. As a person, I want the waiting tickets to say "usage at N% of the 5-hour (or weekly) limit,
    resets <time>", and the gate to lift at that reset time, so that I know why nothing starts and
    when it will.
75. As a person who wants queued tickets to start anyway, I want a web button and a CLI verb that
    lift the usage gate until that window resets, so that a weekly gate never leaves me waiting for
    days with no way past it.

## Implementation Decisions

### Runner selection

- `agentrunner.Select`: `auto` (and empty) picks PTY when claude is on PATH. Otherwise it returns an
  error that names claude and suggests `agent-runner: herdr`. It no longer pings herdr. Explicit `herdr`, `headless` and `pty` behave as
  today. The `ChoicePTY` comment ("auto never picks it") goes away.
- The server preflight log records the chosen runner as today.
- `Config.Runner` is required. `server.New` returns an error when it is nil and never builds a
  default runner.
- A new ADR supersedes ADR 0031's rule that `auto` never picks PTY. It records that `auto` picks PTY
  when claude is on PATH, that headless and herdr are picked only by hand, and that a Codex ticket on
  a native runner runs on herdr. The implementer picks the ADR's number and file name.
- Glossary: the PTY runner and headless runner entries change to match. `docs/specs/agent-runner.md`
  and `docs/specs/agent-runner-hardening.md` are not rewritten. The new ADR is the record.
- Changelog: one entry with the switch, the restart, no migration, that `auto` without claude on PATH
  now refuses to start, and `agent-runner: herdr` as the way back.

### Codex routing

- The runner for a new launch comes from the ticket's harness, not only from the configured runner.
  A Codex ticket gets herdr whenever the configured runner is PTY or headless. A Claude ticket gets
  the configured runner. One server function makes this choice, and every launch path uses it
  (claim, retry, relaunch, start over).
- The tracked run records herdr as its runner, so the existing "an iteration keeps its runner" rule
  covers wait, finish, stop and reattach.
- Codex on PTY is the next epic (`gx:multi-harness-runners`). This spec adds no Codex support to
  native runners.

### The herdr watch and the claim gate

- The server probes herdr whenever herdr is on PATH, whatever the configured runner. The probe and
  its retry interval stay as today.
- The global gate in `claimNext` ("do nothing while herdr is down") becomes a per-ticket check: a
  ticket whose launch runner is herdr is skipped while herdr is down or missing, and shows a wait
  reason. Other tickets claim as usual. The implementer names the check.
- herdr missing from PATH counts as down, for both the per-ticket claim check and the banner.
- The "herdr down" banner and its chat notice show only while a queued or running ticket needs herdr
  and herdr is down. Its text says which tickets wait, and says herdr was not found when it is not on
  PATH.
- No PTY health signal is added.

### Blocked pause on the server wait loop

- The server wait loop (`WaitIterationFinished`) waits for blocked on every runner, not only
  headless. On blocked it first checks the rate limit (see below). If the agent is not limited:
  - Headless (Claude) pauses at once, as today.
  - PTY and herdr (Claude or Codex) wait the 15-second dwell, re-read the status once, and pause
    `needs-answer` (kind `blocked-pane`) only if the agent is still blocked.
- The old in-process path's dwell logic moves into the shared loop. No runner-specific special case
  is added.
- The PTY reason names the web answer button, `gx server agents answer`, and typing in the Agents tab
  live panel. herdr's reason names the answer verb and the pane.
- `pausesOnBlock` and the waiting-run code (keep the run, mark it waiting, watch for unblock, and the
  answer verb's unblock path) widen from Claude on headless to every runner that reports blocked. It
  is one shared rule with no headless special case. Today `leaveBlocked`, `waitsOnHeadlessBlock` and
  the answer verb all check `pausesOnBlock`, which is true only for Claude on headless
  (`server/runner.go`, `server/headless_block.go`).
- The unblock watch then ends the pause when the agent leaves blocked, on every runner. This is how an
  answer typed in the live panel lifts the pause.
- The web ends a blocked pause with Approve/Deny when the block is an approvable tool card. On any
  other dialog (a question menu), Retry on a PTY or herdr run does this:
  1. It ends the pause through the same `unblockMu` path as the answer verb, so the unblock watch and
     Retry never both act ("the first one in does the work", `server/headless_block.go`).
  2. It calls the hosting runner's `Interrupt`, which presses Esc, waits for idle and clears the input
     line. A raw Esc is not used: it skips the wait, and claude may start a turn that the busy guard
     then refuses the prompt against.
  3. It sends the retry prompt through the normal prompt path.
- gx answers the trust dialog at startup (`claudeui.TrustKey`), so it never becomes a blocked pause.
- This is a new way into the existing `needs-answer` pause. gx cannot answer a prompt it did not send,
  and the web answer button ends the pause (ADR 0042).

### Rate-limit hold on the server wait loop

- `Pty.RateLimit` reads the holder's transcript rate-limit status (`error: rate_limit` or status 429)
  to learn that a limit was hit. It does not scan the screen. The reset time comes, in this order,
  from:
  1. The message, parsed with the existing `claudeui` parsers. It names the limit that fired.
  2. The agent's usage record (see "Usage record from the statusline"): the latest `resets_at` among
     the windows at 98% or more.
  3. Neither: limited with a zero reset time.
  A reset time in the past means the server sends "continue" at once.
- The server wait loop asks the hosting runner's `RateLimit` when the agent goes idle or blocked,
  before the blocked pause and before the finish check. For Codex it reads the quota as the old loop
  did.
- When the agent is limited, the run enters a hold:
  - The run keeps its slot and its tracked state, and gains a "rate-limited until <time>" mark. The
    implementer picks where the run state keeps it.
  - The ticket shows "rate-limited, resets <time>" wherever a running ticket's state shows: the TUI
    Queue tab, the web epic row and the ticket page. With no reset time it shows "rate-limited".
  - The stall nudge and the stall and idle timers do not run during the hold.
  - At the reset time plus the existing buffer, the server prompts "continue" through the normal
    prompt path (busy guard, `/` rule), then the wait goes on. If claude's own auto-continue already
    resumed the agent, the busy guard sends nothing.
  - With a zero reset time, the server sends "continue" every 5 minutes. It stays on hold only if
    that new turn hits the limit again. It does not re-check the transcript alone, because on PTY an
    idle agent's transcript keeps the old error as its newest entry.
  - The `paused-rate-limit` and `resumed` events are logged as in the old loop, for friction counts
    and chat.
- While any run of a harness holds, `claimNext` claims no new ticket of that harness. Those tickets
  show a wait reason. The implementer picks its exact text, and it uses the same mechanism as the
  herdr-down wait.
- After a server restart, reattach runs the rate-limit check before the scheduler's first claim
  pass, and puts the run back on hold. So a restart never claims a new ticket of a limited harness.
- This is not a pause and adds no pause kind.
- Interactive sessions do not change.
- herdr and headless keep their current rate-limit detection. Only PTY gets the usage record.
- The old in-process loop's rate-limit and blocked-pane code is deleted once the shared loop covers
  it, if production still has no caller.

### PTY `Find`, `List` and `Start`

- `Find(label)`: on a miss in memory, read `<root>/<label>/meta.json` and check that the holder pid
  (with its start time) is alive and its socket answers. Adopt the holder into memory with the same
  code as `Reattach`.
- `List(epic)`: scan every agent dir under the root, keep the ones whose `meta.epic` matches, and
  adopt the live ones.
- `Start`:
  - A live holder under the label in the same cwd is adopted, unless `NoAdopt` is set.
  - Otherwise `Start` returns `LabelTakenError` with the holder's cwd and never touches its files.
  - A dead holder's dir is cleaned as today.
  - Server launches keep passing `NoAdopt` (ADR 0039).
- A holder whose pid lives but whose socket stays silent counts as taken:
  - `Start` refuses.
  - `Find` returns it marked dead.
  - Calls on it fail with `ErrSessionDied`.
  - `Stop` kills it by pid.

### Calls that hard-wire herdr

- Repair merge's live-agent guard and `verify`'s leftovers use the runner that hosts the ticket: the
  tracked run's runner, else the launch runner from the routing rule. Every server call that builds
  `ralphloop.DefaultDeps()` and reads its runner gets the same fix. `DefaultDeps()` stays only for
  code that runs no agent.
- `liveIteration` keeps its herdr-only tab lookup behind the runner name, as today.
- `--ignore-live-tab` help says "agent session".

### Live view for PTY Queue agents (TUI)

- "Go to pane" on a PTY Queue agent switches to the Agents tab with that agent selected and its live
  terminal panel open, through the holder's `attach` op (ADR 0037). On herdr it focuses the herdr tab
  as today. On PTY it is dimmed only when the holder is gone.
- Read-only mode on the panel drops keys and paste, and keeps scrolling, the wheel, copy and the
  ticket menu verbs. Unlocking takes a confirm dialog that names the kind of agent ("gx is driving
  this Queue agent…" / "This Design agent is waiting on gx-design…"), with Cancel focused. Each new
  open starts read-only.
- A key or paste on a Queue agent (`StreamInput`) stops gx's pending nudges for the current prompt and
  restarts the idle and stall timers. gx's own prompts wait while the input holds a draft, as for
  interactive sessions.
- Screen rules must work at any width: the PTY follows the size of the viewer that last focused or
  typed.

### Agents list groups (TUI and web)

- The server's sessions listing adds PTY-hosted Queue agents as a group with their status. It does not
  list herdr or headless Queue agents. Design agents keep their current source.
- A Queue agent row shows only its status mark (Working, Idle, Blocked, Rate-limited, Exited). The
  sessions listing reads Rate-limited from the run's hold mark, since the hold lives in the run state,
  not in the session status. It has
  no Unread or Actionable mark and does not count in "N needs you". Design agent rows get the same
  treatment. "N needs you" (the TUI title row and tab switcher badge, the web Agents tab and
  switcher) counts only a person's own interactive sessions, Fix agents included.
- The Design page keeps its own design state (questions, Auto-advance, Continue). This change touches
  only the Agents list rows and counts.
- Web: a "Queue agents" switch above the "Design sessions" switch, which is renamed "Design agents".
  Both start off and are remembered in browser storage.
- TUI: `t q` and `t d` toggle the groups. Both start hidden and appear in the `?` help. The implementer
  picks where the TUI stores their state.
- A hidden group shows its count. An agent opened on purpose (Go to pane, the jump list, a Design run
  from the Queue tab, the selected session) shows even when its group is hidden.
- The web Agents page opens Queue agents and Design agents with the composer off and a line saying
  why. A confirm turns it on. The web Design page composer is never read-only.
- The implementer picks how a Queue agent row is named (iteration label or ticket address) and whether
  its header links to the ticket page.

### Investigate as a Fix agent

- The TUI ticket menu's Investigate calls `POST /v1/tickets/fix-agent` on every runner. The herdr tab
  code (`cmdLaunchInvestigate`) and the "herdr is down" reason on Investigate go away.
- The route accepts three states: paused for an answer, paused for repair, and error. The prompt names
  the real state. The web "Fix agent" button stays repair-only.
- The session is the server's `fix-<addr>` interactive session in the trunk checkout. The TUI's
  `investigate-<id>` name goes away.
- After the start, the TUI switches to the Agents tab with the Fix agent selected and writable. A
  second Investigate opens the existing session. An open investigate ticket for the same ticket gets
  the route's "already being investigated" refusal.

### Doctor

- `claudedoctor`'s PTY `rate-limit-screen` check becomes a live check that reads the transcript rate
  limit, using the existing fixtures for the transcript API error and the claude pid watcher. The
  implementer may rename it to match what it checks.
- `gx doctor --check-blocked-form` stays herdr-only. Its help and the runbook say so and point PTY
  users to `gx claude doctor`. The runbook and the code comment on the blocked pause describe the
  server wait loop.

## Testing Decisions

- A good test drives the highest seam and checks behaviour a person or caller sees: a ticket's status
  and reason, a run's runner, a session list, a pause. It never checks private fields or call order.
- Server behaviour is tested through `servertest` with `runnerfake`:
  - Codex routing: PTY or headless configured, a Codex ticket starts on the herdr fake via
    `servertest.WithRunnerFor`. The tests assert the `$gx-…` skill prefix and the herdr-down wait
    reason.
  - The per-ticket herdr gate and the banner rule.
  - The blocked pause per runner: dwell, recheck, reason text, and unblock lifting the pause.
  - The rate-limit hold: slot kept, no nudge, "continue" at the reset, the 5-minute "continue" with
    no reset time, no claims for the harness, return after a restart.
  - The usage gate: a sample at 98% blocks Claude claims with the wait reason, Codex still claims,
    and the gate lifts at `resets_at` on a fake clock.
  - The hosting-runner fix for repair merge and `verify`.
  - Prior art: the Find-by-label nudge, budget and recovery tests moved by `gx:server-tests-pty-impl`,
    `claim_resume_test.go`, `ask_waiting_test.go`.
  - New idle-path behaviour needs an off switch that is off by default in `servertest`, so fake
    agents that never report do not hang.
- The fake `herdr` CLI is used only where herdr's CLI is the subject (the `gx:server-tests-pty` rule).
- PTY `Find`, `List`, `Start` adoption and `RateLimit` are tested in `nativerunner` against the
  fakeclaude holder. Holder-socket tests use `shortTempDir`. Where a behaviour is shared across
  runners, it goes into the `agentrunner/runnertest` conformance suite.
- `agentrunner.Select` gets table tests over the probe.
- `claudedoctor`'s check uses the existing transcript fixtures. `cmd` doctor tests cover the help
  text.
- TUI: `ui/agents` model tests with the fake client cover groups, `t q`/`t d`, read-only, the confirm,
  the help and the badge count. `ui/tickets` tests cover "Go to pane" on PTY and Investigate calling
  the route. Prior art: `attach_test.go`, `sidebar_test.go`, `ui/app/switcher_test.go`.
- Web: vitest for the switches, read-only composer and counts (prior art: `AgentsPage.sidebar.test.tsx`,
  `AgentsPage.switcher.test.tsx`). Playwright flows run on `gxfixture`, which already uses the PTY
  runner with fakeclaude.
- Every package's `TestMain` keeps `claudeenv.Isolate()`.

## Verification

A verify ticket runs these against a temporary gx server (`gxfixture`, PTY runner, fakeclaude).

New scenarios and flows to write. Each becomes an implementation ticket. The implementer picks flow
numbers after the highest in `webui/e2e/CATALOG.md`, and file names.

- Scenario `queue-agents`: an epic with one Claude ticket whose fakeclaude worker idles in a long
  turn, plus a design session. The Agents page then lists a PTY Queue agent and a Design agent.
- Scenario `blocked-pty`: a Claude ticket whose worker blocks on a permission prompt and stays
  blocked. `paused`'s `approve-epic` may serve if it stays blocked past the dwell.
- Scenario `rate-limited`: a Claude ticket whose worker writes a rate-limit API error to its
  transcript with a reset a minute ahead, then a second queued ticket. This needs a fakeclaude
  control that appends the transcript error (`error: "rate_limit"`, a "resets …" message), if the
  `agents-failures` work has not added one.
- Flow "Queue agents switch": `queue-agents`. The switches read "Queue agents" and "Design agents",
  both off with counts. Checking "Queue agents" shows the row with only a status mark. The fresh page
  keeps it. "Agents · N needs you" ignores both groups.
- Flow "Read-only Queue agent": `queue-agents`. Opening the Queue agent shows the composer off with
  its line. The confirm turns it on, and Cancel keeps it off. Reopening is read-only again.
- Flow "Blocked pause on PTY": `blocked-pty`. After the dwell, the ticket card reads "needs answer"
  and the reason names the answer button. The answer button clears it without a reload.
- Flow "Rate-limit hold": `rate-limited`. The ticket shows "rate-limited, resets <time>" and is not
  paused. The second ticket waits with its reason. After the reset, the first ticket goes on and the
  second starts.

Per story:

1. `cmd` preflight test with `agent-runner: auto`, claude on PATH and herdr up: the "agent-runner
   selected" log line reads `runner=pty`. (The fixture forces `agent-runner: pty`, so it cannot test
   `auto`.)
2. Start `gx server` with `auto` and a PATH without claude; it exits non-zero with an error naming
   claude and `agent-runner: herdr`.
3. Config `agent-runner: herdr` with herdr running; the log reads `runner=herdr` (manual, herdr
   needed).
4. Config `agent-runner: headless`; the log reads `runner=headless`.
5. Server test: a tracked herdr run reattaches on herdr after a restart with PTY configured.
6. Read the CHANGELOG entry.
7. Read the new ADR and the glossary entries for PTY runner and headless runner.
8. Flow catalog: none; existing split-launch tests pass.
9. Server test (Codex routing, PTY and headless configured): the run's runner is herdr.
10. Server test: with herdr down, the Codex ticket stays queued with the herdr wait reason. The same
    reason shows in `gx server tickets show <addr>`.
11. Server test: the Codex launch prompt starts with `$gx-`.
12. Server test: herdr probe runs with PTY configured and herdr on PATH.
13. Server test: herdr down, a Claude ticket claims and launches.
14. Server test: herdr down, a Codex ticket does not claim.
15. Server test and web: no banner with herdr down and no herdr ticket; the banner shows once a Codex
    ticket is queued. Server test: with herdr not on PATH, the claim check holds the Codex ticket and
    the banner says herdr was not found.
16. Web: no PTY health banner on the fixture.
17. TUI: "Go to pane" on a PTY Queue agent lands on the Agents tab with the live panel (`ui/tickets`
    and `ui/agents` tests; manual in tmux).
18. TUI test: dimmed with the holder gone, not dimmed with herdr down.
19. TUI test: herdr Queue agent focuses its tab.
20. Flow "Read-only Queue agent"; TUI test for dropped keys.
21. TUI test: scroll, copy and menu verbs work while read-only.
22. Flow "Read-only Queue agent" (dialog text, Cancel focus); TUI test.
23. Flow "Read-only Queue agent" (reopen).
24. Existing Design page composer flows (flows 47, 49) still pass.
25. Server test: a gx prompt waits while the PTY input holds a draft.
26. Server test: `StreamInput` on a Queue agent cancels pending nudges.
27. Server test: a key restarts the idle and stall timers.
28. Server test: an attach with no keys changes no timer.
29. `nativerunner` test: status, nudge and answer rules at width 120.
30. Flow "Queue agents switch"; TUI sidebar test.
31. Server test: a herdr or headless tracked run is absent from `GET /v1/sessions`.
32. Flow "Queue agents switch"; flow 74 "Design sessions toggle" updated to "Design agents".
33. TUI test: `t q`, `t d` and the `?` help.
34. Flow "Queue agents switch" (fresh page); TUI test for remembered state.
35. Flow "Queue agents switch" (counts).
36. TUI test: "Go to pane" shows a hidden-group agent; flow 74's selected-session rule.
37. Flow "Queue agents switch" (status mark only); vitest. On the `rate-limited` scenario, a Queue
    agent row shows Rate-limited.
38. Flow 72 "Agents tab" with a Design agent that is Actionable: the count excludes it; TUI switcher
    badge test.
39. TUI test: Investigate posts to the fix-agent route. `gx agents list` shows `fix-<addr>`.
40. Server test: the route accepts answer, repair and error states, and the prompt names each.
41. Flow 55 "Fix it with an agent" still passes.
42. TUI test: lands on the Agents tab, Fix agent selected and writable.
43. TUI test: a second Investigate selects the existing session; `gx agents list` count unchanged.
44. TUI test: the "already being investigated" refusal shows.
45. Server test: the Fix agent stays after the ticket resumes.
46. `nativerunner` test: `Find` adopts a live untracked holder. Server test: resume after restart
    starts no second claude.
47. `nativerunner` test: `List` returns untracked live agents of the epic.
48. `nativerunner` test: `Start` adopts a same-cwd holder without `NoAdopt`.
49. `nativerunner` test: `LabelTakenError` with the cwd; holder files untouched. Server test: the
    stale-holder path on PTY.
50. `nativerunner` test: silent socket counts as taken; `Stop` kills by pid.
51. Server test: repair merge refuses with a live PTY agent; `verify` reports a PTY leftover.
52. The help of the `gx server` repair-merge command that owns `--ignore-live-tab` shows "agent
    session".
53. Flow "Blocked pause on PTY"; server tests for herdr and Codex-on-herdr fakes.
54. Server test: a block that clears inside 15 seconds does not pause.
55. Existing headless block tests pass.
56. Flow "Blocked pause on PTY" (reason text).
57. Flow "Blocked pause on PTY" (answer button); flow 3's Approve row still passes. A new fixture
    scenario with a non-approvable block (a question menu): Retry interrupts, sends the prompt and
    lifts the pause. Server test with the unblock watch running: Retry ends the pause once and sends
    one prompt.
58. Server test per runner (PTY, herdr, headless): the agent leaving blocked lifts the pause.
59. `nativerunner` test: `RateLimit` from a transcript error: the message's reset time wins over a
    usage record; a record alone gives the latest `resets_at` of the windows at 98% or more; neither
    gives a zero reset time; a past reset time continues at once. `cmd` test: the statusline mode writes the `rate_limits`
    record and still renders the line.
60. Server test: a Codex quota banner while blocked holds, never pauses `needs-answer`.
61. Flow "Rate-limit hold" (not paused); server test for the kept slot.
62. Flow "Rate-limit hold" (text); `gx server tickets show <addr>` shows the hold.
63. Server test: no stall nudge during the hold.
64. Server test: "continue" prompt at the reset.
65. Server test on a fake clock: with zero reset time, "continue" goes out every 5 minutes; the hold
    ends after a turn that is not limited and stays after a turn that is.
66. Flow "Rate-limit hold" (second ticket waits); server test per harness.
67. Server test: restart during the hold, then hold again, with no new claim of that harness.
68. Flow 80 "Failure notices" (rate-limit row) still passes.
69. `gx claude doctor` lists the PTY rate-limit check as run, not skipped.
70. `gx doctor --help` text names herdr and `gx claude doctor`.
71. Read the runbook.
72. `rg DefaultDeps` in `server/`: only calls that run no agent remain; server tests from story 51.
73. Server test: a usage sample at 98% of the 5-hour window, and one of the weekly window, each block
    a Claude claim; a Codex ticket still claims; a sample at 97% does not block.
74. Server test: the wait reason names the window, the percentage and the reset time; the gate lifts
    at `resets_at` on a fake clock with no new sample; the gate comes back after a restart. Flow
    "Rate-limit hold" or a new fixture scenario shows the reason on a queued ticket. Server tests:
    a person's relaunch, a handoff restart and a Fix agent start while the gate holds; with both the
    hold and the gate, the hold's reason shows.
75. Server test: the override lets a Claude ticket claim at 99%, and a new sample at 99% does not bring
    the gate back before `resets_at`; the override survives a restart. Flow: the web override button
    on the usage-gate wait reason; `cmd` test for the CLI verb.

### Usage record from the statusline

- Claude Code passes the statusline command a JSON object. For claude.ai Pro/Max accounts it holds
  `rate_limits.five_hour` and `rate_limits.seven_day`, each with `used_percentage` and `resets_at`
  (Unix seconds). It refreshes after each API response, is missing before the first reply and for
  API-key accounts, and drops a window once its `resets_at` passes
  (https://code.claude.com/docs/en/statusline).
- PTY agents get a `statusLine` entry in the `--settings` JSON gx already passes. `ptyLaunchArgs`
  builds that JSON per agent instead of the fixed `{"tui":"default"}`. The command is the absolute gx
  path (`os.Executable`) running `gx claude statusline` in a mode that takes the agent's dir as a
  flag. The mode renders the line as today and also writes the `rate_limits` object, with the time it
  was seen, into that dir. A failed write never changes the rendered line or the exit code. The
  implementer names the mode, the flag and the file. Interactive sessions keep the person's own
  statusline.
- gx does not call the undocumented `api.anthropic.com/api/oauth/usage` endpoint. It needs the OAuth
  token from the keychain and can change in any release.
- Claude Code's own auto-continue at a usage limit (since v2.1.234) stays on.

### Usage gate on claims

- The server keeps the newest usage sample seen across PTY agents' records. The implementer picks
  where it lives, and it survives a server restart.
- While the newest sample shows the 5-hour or the weekly window at 98% or more, and that window's
  `resets_at` has not passed, `claimNext` claims no new Claude ticket. The 98% threshold is fixed,
  with no config key. Codex tickets are not gated.
- The gate lifts at `resets_at`, even if no new sample arrives (the data is stale while agents are
  idle).
- Waiting Claude tickets show "usage at N% of the 5-hour limit, resets <time>" (or "weekly"). It uses
  the same wait mechanism as the herdr-down wait and the rate-limit hold.
- Running agents are not stopped or held by the gate. The rate-limit hold still handles a real limit
  hit.
- With no usage data (API-key accounts, or no reply yet), there is no gate.
- The gate stays on when extra usage is on (`server/extra_usage.go`), even though the account keeps
  working past 100%. The override below is the way past it.
- The gate covers queue claims and automatic relaunches (recovery). It does not gate a person's own
  relaunch or Retry, handoff restarts of a running ticket, or Design, Fix and interactive sessions.
- When the rate-limit hold and the gate both apply to a ticket, the hold's reason shows.
- Override: a web button and a CLI verb, like the budget override (`server/budget.go`
  `budgetOverride`). It lifts the gate for new claims until that window's `resets_at`. The window
  stays at 98% or more until then, so lifting it only until the next sample would not work. Running
  tickets are never gated, so the override is only about starting queued ones. The implementer names
  the verb and the route, and where the override is stored so it survives a restart.
- This is not a pause and adds no pause kind.

## Out of Scope

- Codex on the PTY runner. It is the next epic (`gx:multi-harness-runners`). Until then, Codex runs
  on herdr.
- Migrating running herdr iterations to PTY.
- herdr e2e tests (`make e2e`, `e2e/`) beyond what the switch needs.
- A live terminal on the web. The web Agents page keeps its chat view and composer.
- Listing herdr or headless Queue agents in the Agents list.
- A PTY health signal or banner.
- `gx:agent-sessions/01` (pid files without `startedAt`) and `gx:agents-tab-v2/01` (loop sessions
  adopt the harness seam). Neither blocks the switch.
- Split launch (tmux, kitty, herdr, Ghostty). It depends on the terminal, not the runner.
- Moving the `UntilPtyAsDefault` server tests. `gx:server-tests-pty-impl` already did that.

## Self-decided design questions

- Can an answer typed in the live panel lift a `needs-answer` blocked pause? Yes. The unblock watch
  widens to every runner (see "Blocked pause on the server wait loop"), so the pause lifts once the
  agent leaves blocked. Ticket 02 makes the
  live panel a way to answer, and a pause that outlived its cause would need a second click.
- Do the idle and stall timers run during a rate-limit hold? No. A hold is not a stall, and a stall
  pause during a hold would turn a hold into a pause, which ADR 0042 rules out.
- Where does the hold's reason show? Wherever a running ticket's state shows (TUI Queue tab, web epic
  row, ticket page), plus a wait reason on queued tickets of the same harness. One state with one
  reason is simpler than a separate banner. The herdr-down wait already works this way.
- Does a hold survive a server restart? Yes. Reattach runs the rate-limit check before the first
  claim pass and finds the limit again. No new stored state is needed.
- What does a Codex ticket do when herdr is not on PATH at all? It waits with a reason, the same as
  herdr down. Without herdr, gx cannot recover, but a queued ticket that has not claimed needs no
  pause. It starts once herdr appears.
- With `auto` and claude missing, does gx fall back to herdr? No (confirmed in the review). It fails
  at start with an error naming claude and suggesting `agent-runner: herdr`. The Destination says herdr
  runs only when set by hand. A person with herdr and only Codex sets `agent-runner: herdr`, and the
  changelog says so.
- Do Fix agents count in "N needs you"? Yes. A Fix agent is a person's own interactive session, not a
  Queue agent or a Design agent.
- Does the Design page lose its Actionable and Unread state with the Agents list change? No. Round 2
  changed only the Agents list rows and the "needs you" count. Auto-advance and the Design page
  read their own state.
- Is the old in-process loop's rate-limit and blocked code kept? No. It moves into the shared server
  wait loop, and the old copy is deleted if nothing in production calls it. Keeping two copies would
  let them drift.
- How does a Queue agent row leave the list? With its PTY session: once gx stops the session at
  finish, or a person stops it. A ticket paused with its agent still alive (blocked or asking) keeps
  its row. The list shows live hosting, not ticket status.

## Further Notes

- Map decisions: `gx:pty-as-default` tickets 01 to 08. Ticket 06 took over `gx:pause-and-recovery/01`
  and the herdr watch. Ticket 05 took over `gx:claude-doctor-live/01`.
- ADRs that apply: 0031 (superseded in part by the new ADR), 0037 (live attach), 0039 (server
  launches never adopt), 0042 (pauses recoverable from the web).
