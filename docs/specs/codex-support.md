# Codex support

## Problem Statement

gx runs Claude. Codex runs only for Queue tickets, and only on herdr. The Agents tab, Design runs and
the Fix agent cannot start Codex at all, and the native runners ignore the agent kind and start
`claude`. A person who wants to work with Codex today cannot just switch. Several things are also
missing or only half there for Codex: guard-bash, the system prompt, replies from the web, approval
cards, resume, the web chat and composer, failure detection, the charting handoff, grilling counts,
recovery, background tasks, the compaction gate, and a doctor. Skills type `/` where Codex needs
`$`, and they install where Codex does not look. There is no way to say which harness new agents
should use.

## Solution

Codex runs on the PTY runner with every feature Claude has on PTY. A **Default harness** setting
decides what every new agent runs. A gap is allowed only where Codex cannot do the thing, and this
spec lists each gap with what the person sees instead.

From the person's side:

- A Settings "Agents" page has a Claude/Codex control for the global default, and a row per project
  with an override. The TUI has a "Default harness..." command.
- New agents everywhere follow the default: Agents tab agents, Design runs, the Fix agent, TUI
  Investigate, root auto-queue tickets, and code-review, verify and evolve tickets.
- A Queue ticket with no explicit `--agent` reads the default when it is claimed. A switch applies to
  waiting tickets and to the next design step. Running agents keep their harness.
- Agent rows, Queue rows and the design page show `harness · tier · model effort`, for Claude too.
- Codex agents get live replies, approval cards, resume, the web chat and composer, the usage gate,
  the rate-limit hold, failure pauses, smart-zone compaction and background-task marks.
- `gx codex doctor` checks the Codex install against what gx needs. `gx codex setup` installs the
  managed approval rules.
- herdr stays a hand-picked backup that hosts Codex with a reduced flow.
- Switching harness when a limit is reached is not part of this effort.

## User Stories

Default harness setting

1. As a person who uses gx, I want one Default harness setting, so that I can switch from Claude to Codex without changing how I start work.
2. As a person, I want a Settings "Agents" page with a Claude/Codex control for the global default, so that I can change it in the web.
3. As a person, I want one row per project on that page, with an Override button when the project inherits and a harness chip with Remove override when it overrides, so that one project can differ from the rest.
4. As a person, I want the Settings home to show a row "Agents" with `Default: <harness>`, so that I can see the current default without opening the page.
5. As a TUI user, I want a "Default harness..." command that opens a small picker for the global value and, inside a project, that project (Follow global, Claude, Codex), so that I need no web page.
6. As a person who keeps config in dotfiles, I want the control to write through a symlink and keep my other keys, so that my dotfiles link survives.
7. As a person, I want a notice after a switch that says how many waiting tickets and the next design step will use the new default and how many running agents keep theirs, so that I know what the switch touched.
8. As a person, I want the server to refuse to start when the default harness is not on PATH, with an error that names the program, so that I find out at once.
9. As a person, I want the control and the TUI picker to refuse a harness whose program is not on PATH, with the same message, so that a running server never holds a default it cannot launch.
10. As a person, I want a one-line note on the Agents page when `agent-runner` is `headless` or `herdr`, so that I know where Codex runs.

Who follows the default

11. As a person, I want a new agent from the Agents tab to start on the default harness, so that I need no picker in the Create form.
12. As a person, I want every fresh design session to read the current default, so that a design can move between harnesses between steps, and a chain inside one session to stay on its harness.
13. As a person, I want the Fix agent and TUI Investigate to follow the default, so that every new agent behaves the same.
14. As a person, I want root auto-queue tickets and code-review, verify and evolve tickets to follow the default, so that nothing stays on Claude by accident.
15. As a person, I want a waiting Queue ticket with no `--agent` to read the default when gx claims it, so that a switch applies to tickets that already wait.
16. As a person, I want a retry of a ticket that already ran to keep its harness, so that a retry continues the same conversation kind.
17. As a person, I want Queue items stored before this change to stay on Claude, so that nothing I queued earlier changes under me.
18. As a person, I want `gx server queue --agent` and `gx agents new --harness` to stay as an override for one launch, so that I can still pick by hand.
19. As a person, I want a running session to keep the harness it started with, so that a switch never changes work in progress.
20. As a person, I want a ticket pinned to a harness that is not on PATH to wait in the queue with a reason that names the program and the `--agent` override, so that I can fix it.

Rows

21. As a person, I want Agent rows in the web to show `harness · tier · model effort`, for Claude too, so that I see what runs.
22. As a person, I want Queue rows to show the same three parts, "follows default" for a waiting ticket with no pin, and the harness plus "pinned" for a pinned one, so that I know what a ticket will use.
23. As a person, I want the design page header to show the same three parts, so that I know which harness runs the design.
24. As a TUI user, I want TUI Agent rows, Queue rows and the design view to show the same three parts, so that the TUI and the web agree.
25. As a person, I want a session with no reply yet to show "<Tier> · CLI default", so that the row is honest before the model is known.
26. As a person, I want the model to come from the newest reply, so that a model switched by hand shows after the next reply, with a note on the row.

Hosting and launch

27. As a person on `auto` or `pty`, I want Codex agents on the PTY runner, so that Codex has the PTY features.
28. As a person on `headless`, I want a Codex launch to run on the PTY runner while Claude stays on headless, so that Codex works without changing my runner.
29. As a person on `agent-runner: herdr`, I want Codex to run on herdr with a reduced flow, so that I still have a backup.
30. As a person, I want the "herdr down" banner and the per-ticket herdr claim check to apply only while `agent-runner` is `herdr`, so that PTY users never see herdr noise.
31. As a person on `auto` or `pty`, I want gx to need the default harness on PATH and not `claude`, so that a Codex-only machine works.
32. As a person, I want a Claude launch with no `claude` on PATH to wait with a reason, so that it behaves like a pinned missing harness.
33. As a person, I want every Codex agent to start with one launch shape (workspace-write sandbox, `on-request` approvals, the automatic reviewer, network access on), so that behaviour does not depend on the agent kind.
34. As a person, I want gx to pass its hooks inline and set `check_for_update_on_startup=false`, so that no hook trust dialog or update dialog takes a typed key.
35. As a person, I want `git commit` to work in a linked worktree, and design step 8 to add a worktree, so that gx adds the worktree gitdir, the bare repo common dir, the ticket-store directory and the scratch dir as writable roots.
36. As a person, I want `gx codex setup` to install the managed approval rules and the gopls approvals after showing what it writes and asking, so that the reviewer asks less often.
37. As a person, I want the GX system prompt on every fresh Codex launch, so that Codex follows the same rules as Claude.

Signals and control

38. As a person, I want gx to read a Codex agent's state (working, idle, blocked, done, error) from OSC 7501 with the title as a fallback, so that status is right within the same instant as the turn end.
39. As a person, I want a web reply to reach a live Codex agent only when it is idle, so that a prompt never lands mid-turn.
40. As a person, I want the composer to queue text while Codex works, to send multi-line text verbatim, and to hint that Codex skills start with `$` when I type `/gx-...`, so that the composer behaves like Claude's.
41. As a person, I want an approval card with Approve and Deny (with my reason) for a Codex approval, so that I can answer from the web.
42. As a person, I want gx to parse the approval dialog from the screen when the hook did not fire, so that a card still appears.
43. As a person, I want a Codex agent that stays blocked past the 15-second dwell to pause the ticket `needs-answer`, and the web to end it with Approve or Deny on a card and Retry otherwise, so that I can always recover.
44. As a person, I want gx to answer or disable every startup dialog (trust, hooks review, update, model nudge), so that none becomes a pause.
45. As a person, I want a claim to run `codex resume` when gx has the session id and the rollout exists, and otherwise to start from the Handoff, so that work continues.
46. As a person, I want the Codex process to stay alive at the idle composer after `gx tickets ask`, keeping its slot and worktree, so that my answer reaches the same session.
47. As a person, I want an answer to a ticket whose agent is gone to reopen the ticket so that the next claim starts from the Handoff, so that no answer is lost.
48. As a person, I want Interrupt on a Codex agent to leave a still-running tool as a background terminal with its marker kept, so that I decide whether to stop it with `/stop`.
49. As a person, I want Restore of an Exited interactive Codex session to relaunch it with `codex resume` on its latest conversation when the rollout exists, so that Restore works like Claude's.

Chat and the Design flow

50. As a person, I want the web chat to read Claude and Codex through one reader that returns one message list, so that the chat looks the same.
51. As a person, I want the chat to show agent text, my prompts, folded tool runs and a compacted entry, and to hide reasoning, developer lines and injected context, and to show a `$gx-` prompt as a command, so that the chat stays readable.
52. As a person in a Design on Codex, I want grilling rounds, the run-end line, the charting handoff and grilling counts to work as on Claude, so that Design has full parity.
53. As a person, I want the run-end line to name the session's own harness command (`/new`, `$gx-design` on Codex), so that I can copy it.
54. As a person, I want a notice "Codex archived this conversation" with a button that decodes the archived rollout on demand, so that old chats open without slowing a page load.
55. As a person, I want a clear notice when gx cannot read a Codex conversation, naming the Codex version and pointing to `gx codex doctor`, and saying the terminal and agent are unaffected, so that I know it is a reader problem.

Limits, failures and compaction

56. As a person, I want the usage gate to hold new Codex claims from the `rate_limits` data in Codex's rollout, with its own gate per harness, so that Codex limits work like Claude's.
57. As a person, I want the wait reason to name the harness ("Codex usage: N% of the 5h limit, resets ..."), and the override to lift only that harness's gate, so that a Claude gate never blocks Codex.
58. As a person, I want a Codex agent that hits a usage limit to hold in place with "rate-limited: resets <time>", so that nothing is lost.
59. As a person, I want a Codex turn that ends in an error (OSC 7501 `error`) to pause the ticket `needs-answer` after the dwell, with Codex's own error text and a Retry button, so that I can see what went wrong and retry.
60. As a person, I want a silent Codex agent to get the normal stall nudge, so that a network stall does not wait forever.
61. As a person, I want recovery and session lookup to read the Codex rollout, and a Codex agent that died before its first prompt to count as having done nothing, so that the retry starts from the Handoff with no error.
62. As a person, I want the smart zone to read Codex context through `gx agent context-window`, with the same single ceiling and growth table, so that long Codex runs stay inside it.
63. As a person, I want the compaction finish gate to count `compacted` rollout items for Codex like Claude, and to wait to the 5-minute cap when the rollout is unreadable, so that gx never trusts the pane.
64. As a person, I want a Codex agent with a live background terminal to be promptable and marked on its row, and not to count as idle, so that I can still reach it.

Other screens and cost

65. As a TUI user, I want Watch on a Codex agent to behave as for Claude, so that I see the holder's screen.
66. As a person, I want Codex rows to show tokens and a dash in the dollar columns, so that I do not see made-up dollars.
67. As a person, I want dollar totals and budget limits to count Claude only, so that Codex tokens do not distort them.
68. As a person, I want the extra-usage check to run only for Claude items, so that Codex enqueues show no wrong warning.

Skills

69. As a person, I want gx skills installed where Codex looks (`~/.agents/skills`), the old `~/.codex/prompts` copies removed, and a warning about duplicates in `$CODEX_HOME/skills`, so that Codex finds each skill once.
70. As a person, I want every launch to type the skill prefix through one function (`/` for Claude, `$` for Codex), so that no site can type the wrong one.
71. As a person, I want one skill bundle with neutral wording and the Claude tool name in parentheses, so that skills work on both harnesses.
72. As a person, I want `gx agent model <tier>` to print `model [effort]`, and skills to pass both to subagents, so that tiers hold on Codex.
73. As a person, I want the 11 manual skills hidden from Codex's implicit use, so that Codex does not start them on its own.
74. As a person, I want `gx agent context-window --fits` to work in a symlinked working directory, so that macOS temp paths do not break it.

Doctor and tests

75. As a person, I want `gx codex doctor` to check the version (0.166.0 or newer), the flags and `-c` keys gx passes, OSC 7501 and the title session id, hooks (including a denying `PermissionRequest`), a probe commit in a throwaway linked worktree, and the rollout reader on a real rollout, so that I can re-check gx's assumptions after a Codex upgrade.
76. As a person, I want `gx codex doctor` to warn when Codex hides the spawn model fields and when the managed rules are missing, so that I know what the doctor sees.
77. As a person, I want `gx codex doctor` to run the same way as `gx claude doctor`, including `--record` to capture real streams and rollouts as test data, so that the two doctors feel the same.
78. As an agent that investigates a stuck run, I want to be told to run the rollout-format check when I suspect a Codex format mismatch, so that I find format changes fast.
79. As a gx developer, I want a `fakecodex` binary on the shared agentfake core, so that CI tests Codex flows without a real Codex or a login.
80. As a gx developer, I want a hand-run smoke checklist tagged "real Codex only", run after each Codex upgrade, so that the cases a fake cannot cover still get checked.
81. As a gx developer, I want a test that fails on a hardcoded `"/gx-` literal in non-test Go code, so that the prefix rule holds.

## Implementation Decisions

### The Default harness

- One harness value, read from the global config and, when present, from the project's own config.
  The project value wins. Values are `claude` and `codex`. The implementer picks the key name and the
  API shape.
- The web and TUI control writes the global value to `~/.config/gx/config.json` and a project value
  to the project's `project.json`. The write follows a symlink so a dotfiles link survives, and it
  keeps other keys. Remove override deletes the key from `project.json`.
- The control uses the same PATH check as server start. If the chosen harness is not on PATH, the
  write is refused and the message names the program. One check serves both places. The rule is
  that a running server never holds a default it cannot launch.
- Where it is read: the Queue claim (a ticket with no `--agent`), a new design session, an Agents tab
  Create, the Fix agent and TUI Investigate, and root auto-queue tickets. The harness read at
  launch is recorded on the run. A running session never changes harness.
- A retry or relaunch of a ticket that already ran uses the harness recorded for it. Queue items
  stored before this change carry an explicit `claude`. No migration is written.
- `gx server queue --agent` and `gx agents new --harness` override the default for one launch.
- Server start: the default harness (global value) must be on PATH, and a missing program ends start
  with an error that names it. This is separate from the Claude check below, so a Codex-only machine
  starts.
- A ticket pinned to a harness that is not on PATH stays queued with a wait reason. The reason names
  the program and the `--agent` override. It shows in `gx server tickets explain`.
- The Settings page note: while `agent-runner` is `headless` it reads "Codex runs on PTY". While it is
  `herdr` it reads "Codex runs on herdr with a reduced flow".
- The switch notice reads the count of waiting tickets and running agents when the write succeeds.

### Rows and labels

- Agent rows, Queue rows and the design page header show `harness · tier · model effort`, in web and
  TUI, for Claude too. Today Claude hides the chip, and Queue rows and the design page show nothing.
- Tier is what gx launched with and never changes. The labels Light, Worker and Strongest stay. The
  model after the tier comes from config and from the newest reply (Claude) or the newest Codex
  rollout turn. A session with no reply yet reads "<Tier> · CLI default".
- A row can carry a "switched by hand" note when the shown model differs from the model the launch
  tier chose. The model names in the mock are placeholders.
- A waiting Queue ticket with no pin reads "follows default". A pinned one shows its harness and
  "pinned".

### Runner selection and hosting

- This epic amends the Codex routing that `pty-as-default` adds and removes the Codex-to-herdr route.
  The one per-launch runner function now returns PTY for a Codex launch under `headless`, `auto` and
  `pty`. It returns herdr for Codex only under `agent-runner: herdr`. The run records its runner, so
  "an iteration keeps its runner" covers wait, finish, stop and reattach. Claude launches stay on
  the configured runner.
- `auto` and `pty` need the default harness on PATH, not `claude`. A Claude launch with no `claude`
  on PATH waits in the queue with a reason, like a pinned missing harness. `headless` still needs
  `claude` on PATH, because it hosts only Claude.
- The herdr watch, the "herdr down" banner and the per-ticket herdr claim check apply only while
  `agent-runner` is `herdr`.
- A new ADR records this: Codex runs on PTY under `auto`, `pty` and `headless`; herdr hosts Codex only
  by hand with a reduced flow. It amends the Codex rule that the `pty-as-default` ADR records. The
  implementer picks its number and file name. The glossary entries for Harness, Default harness, PTY
  runner and headless runner change to match. One changelog entry names the new setting, the
  `gx codex` commands and the 0.166.0 minimum.

### Launch shape and permissions

- One shape for every Codex agent (design, Agents tab, Fix, Investigate, queue, code-review, verify,
  evolve): `--sandbox workspace-write --ask-for-approval on-request --approve-for-me`, plus
  `-c sandbox_workspace_write.network_access=true`. No `danger-full-access`, no per-kind sandbox.
- `--approve-for-me` sends approvals to the automatic reviewer. Obvious actions ask nothing. Only a
  reviewer denial or doubt reaches the person as an approval card.
- `network_access=true` is needed because the sandbox otherwise blocks the gx unix socket. It opens
  the internet to the agent too, because Codex has no per-socket switch. This is a listed risk.
- Writable roots use the `sandbox_workspace_write.writable_roots` config key (not `--add-dir`): the
  worktree's own gitdir, the bare repo's common git dir, the project's ticket-store directory and the
  scratch dir. A plain checkout also needs its `.git`.
- Hooks are passed inline with `-c hooks.<Event>=...` and `--dangerously-bypass-hook-trust`. The flag
  also runs any `hooks.json` that a repo holds without review. This is a listed risk. gx also passes
  `check_for_update_on_startup=false`.
- Hook set: `SessionStart` (session id and rollout path), `PreToolUse` on `Bash` running
  `gx agent guard-bash` (same checks as Claude, reads the Codex payload, exits 2 to block; `apply_patch`
  stays open because it is Codex's edit tool), `PreToolUse` on `request_user_input` (blocks it with
  "run `gx tickets ask`"; a hook cannot detect Plan mode), `PermissionRequest` (holds the dialog for
  the approval card), and `Stop` where it helps. gx installs no `PreCompact` or `PostCompact` hook.
- System prompt: `-c developer_instructions=<GX system prompt>` on a fresh launch only, never on
  `codex resume`. The tool name in the text is `request_user_input` for Codex. The guard-bash hint
  names the harness's edit tool.
- `--ask-for-approval` accepts only `on-request` and `never`. gx never uses `untrusted`.
- `gx codex setup` replaces the old configure-codex approvals script, which is deleted. It installs
  the managed rules file (git add, commit, diff, status, log, cherry-pick) and the gopls MCP
  approvals. It is idempotent, shows what it will write, asks first, and never touches a rules file
  it does not manage. With the reviewer on, the rules are optional: they save reviewer round trips.
  Discovery: `gx codex doctor` warns "run `gx codex setup`" on missing rules, the Agents page shows
  the same hint when Codex is chosen, and the README's Codex section names it. The startup refusal
  never mentions it.

### Signals, input and approvals

- The PTY stream is the way in: OSC 7501 (`idle`, `working`, `blocked kind=permission|question`,
  `done`, `error`) with the OSC 0 title as a fallback, fed through the harness `Program` and `Ready`
  seams. Hooks give structured text. The rollout gives tokens, rate limits and errors. There is no
  app-server. Codex 0.166.0 is the minimum, and gx builds no fallback for older versions.
- `disable_paste_burst=true` is set so Enter after typed text is not a newline.
- A live reply is typed as a normal prompt only when OSC 7501 says `idle` (the busy guard). From the
  composer, gx sends with Tab so text queues until the turn ends, as for Claude. Text goes verbatim,
  multi-line text in one paste. A `/gx-...` command in a Codex composer is refused by Codex, so the
  composer shows a hint that Codex skills start with `$`.
- Approvals: a `PermissionRequest` hook holds the dialog and gives the card its text. Approve sends
  allow. Deny sends the person's reason, which the model sees. When the hook does not fire, gx
  parses the screen dialog (the 0.166.0 text `Blocked by hook` plus the message) and answers with
  keys.
- Startup dialogs (trust, hooks review, update, model nudge) are answered or disabled by gx. They
  look "ready" until a session id shows in the title, so gx handles them before it sends a prompt.
- The Background flag (OSC 7501 `working` with title `Ready`) feeds the shared `Background` status
  field. A Codex agent with a live background terminal is promptable, shows a marker on its row, and
  is not idle. The Claude-only transcript probe and the Claude finish gate stay as they are.
- Interrupt does not send `/stop`. An interrupted tool stays as a background terminal and the marker
  stays.

### Blocked, error and recovery pauses

- No new pause kind. Two new ways into the existing `needs-answer` pause (kind `blocked-pane`), as
  ADR 0042 requires:
  - OSC 7501 `blocked` still blocked after the 15-second dwell. gx cannot answer a prompt it did
    not send, and the model's choice (approve or deny) belongs to the person. The web ends the pause
    with Approve or Deny on an approvable card. Otherwise Retry (Interrupt, then the retry prompt)
    ends it, or typing in the live panel when the agent leaves blocked. `request_user_input` is not
    a way in: it appears only in Plan mode, which gx never starts.
  - OSC 7501 `error` counts as a blocked pane. gx cannot recover on its own because it cannot know
    if the cause (a login problem, a failed request, a quota) clears, and it does not auto-retry. The
    pause shows the error text from the rollout (`task_complete.error`) and the web ends it with
    Retry (the retry prompt). Usage-limit errors do not pause: they go to the rate-limit hold.
- No screen matcher for a network stall. The idle-agent stall nudge covers a silent agent.
- Recovery and session lookup read the rollout through the shared message reader. When the agent
  died before its first prompt there is no rollout, so gx treats the agent as having done nothing and
  the retry starts from the Handoff, with no error.
- gx shows Codex's own error text and adds no "log in" hint.

### Resume, slot and replies

- A claim runs `codex resume <id> --cd <dir>` when gx has the session id (from the `SessionStart`
  hook) and the rollout file exists. Otherwise it starts from the Handoff. No setting. Restore of an
  interactive Codex session uses the same rule.
- The Codex process stays alive at the idle composer after `gx tickets ask` and keeps its slot and
  worktree, as Claude does. If the agent is gone, the answer reopens the ticket.

### Chat and the shared message reader

- One reader returns the shared message list for every harness. Claude's reader moves behind it
  unchanged. The Codex reader reads `response_item` lines (`message`, `function_call`,
  `custom_tool_call`) from the rollout at the `SessionStart` hook's `transcript_path`, cached by size
  and mtime. It does not use app-server or `event_msg` lines.
- The chat shows agent text, the person's prompts, folded tool runs (count and last tool name) and a
  `compacted` entry. Reasoning, developer lines and the injected `AGENTS.md` and
  `<environment_context>` lines stay hidden. A prompt starting with `$gx-` shows as a command.
- Rounds, the run-end line, the charting handoff and grilling counts run on the shared list.
- Archived rollouts (`.jsonl.zst`, after about 7 days) show a notice with a button that decodes the
  file on demand. Decoding never runs on a plain page load. gx has no zstd library today, so the
  implementer picks one.
- Unknown rollout lines are skipped. When the rollout has turns (`task_complete` lines) but gx read
  zero messages, the chat shows "gx cannot read this Codex conversation (Codex <version>). The
  terminal and the agent are unaffected." and points to `gx codex doctor`.
- The `codexsession` symlinked-cwd compare is fixed (the rollout stores the real path). `--fits` also
  reads "no context usage yet" before the first model call, and the stray `CLAUDE_CODE_SESSION_ID`
  case is handled; `gx agent link` and `step` use only `GX_AGENT_SESSION`.

### Limits, compaction and cost

- The Codex usage gate follows the rule of the Claude gate: the newest sample of the 5-hour or the
  weekly window at 98% or more, with `resets_at` not passed, claims no new Codex ticket. The sample
  comes from `token_count.rate_limits` in Codex rollouts. The threshold is fixed. The gate lifts at
  `resets_at` without a new sample. Each harness has its own gate and its own override: the override
  verb takes a harness argument and defaults to the default harness. The implementer picks the verb
  and the storage. Running agents are not held. No pause kind is added.
- A held ticket shows the wait reason with the harness name. The web override button lifts only that
  ticket's harness gate.
- The rate-limit hold on the server wait loop covers Codex: the reset time comes from the rollout
  error and `rate_limits`. The hold keeps the slot, sends "continue" at the reset, and stops claims
  for that harness until then.
- Compaction: the finish gate counts `compacted` rollout items, like Claude. The Codex special case in
  `readCompactBoundaries` goes away. An unreadable rollout means "unavailable": gx waits until the
  5-minute cap (`smartZoneCompactTimeoutMs`), then gives up with the existing smart-zone pause
  reason, never trusting the pane. A compaction gx did not start (Codex's own) is ignored. The
  smart zone reads the rollout through `gx agent context-window`, with the single `SmartZoneCeiling`
  and growth table.
- Cost: Codex rows show tokens and a dash in the dollar columns. Dollar totals and budget limits
  count Claude only. The extra-usage check runs only when the item's harness is `claude`.
- TUI Watch is the same item as for Claude. On PTY it shows the holder's screen, refreshed with `r`.
  Herdr Codex agents jump to the pane.

### Skills

- One bundle serves both harnesses. Skill text uses neutral words with the Claude tool name in
  parentheses ("file edit (Edit)", "shell call (Bash)", "start a subagent (Agent tool)"). One shared
  tool-name table in the shared tracker note maps each phrase to the Claude and Codex tool. No
  generated copy per harness. The implementer picks the table's shape.
- One function owns the launch prefix (`/` for Claude, `$` for Codex). Every call site that types a
  skill passes the target session's harness: the design launcher, the actionable and handoff
  starters, the session starter, the Fix agent and the suggested actions. A test fails on a hardcoded
  `"/gx-` literal in non-test Go code. The implementer names the test.
- `gx agent model <tier>` prints `model [effort]`. Claude prints the model only. Skills pass the
  first word as `model` and the second, if present, as `reasoning_effort`. If Codex hides the spawn
  fields, the subagent inherits the session model and effort and the step goes on. A test checks every
  skill that reads `gx agent model` against the new output.
- Packaging: the Codex skills root becomes `~/.agents/skills`. Install removes the managed
  `~/.codex/prompts` copies and warns about duplicates in `$CODEX_HOME/skills`. `--dev` links whole
  skill directories for Codex, because a file symlink is not found. The 11 skills with
  `disable-model-invocation: true` get an `agents/openai.yaml` with `allow_implicit_invocation:
  false`, and a bundle test checks both directions. The wrong "Invocation policy" paragraph in the
  skills guide is fixed. The implementer picks the yaml content.
- Skill text: `Read`, `Edit`, `Write`, `TaskStop`, `ToolSearch`, `run_in_background` and Bash
  `timeout` wording gives way to the table. Subagent skills say "start every subagent in one turn,
  then wait", use `fork_turns: none` on a tier override, state the parallel cap, and drop
  `isolation: "worktree"`. Skills read a callee's `SKILL.md` by path in place of the Skill tool. The
  run-end example names `$gx-design` for Codex. `gx-investigate` gets a harness-neutral session
  lookup and the Codex rollout path.

### Doctor

- `gx codex doctor` matches `gx claude doctor` in shape: fixture mode, live mode and `--record`.
  Live checks run in a throwaway linked worktree:
  - the version is at least 0.166.0 (the minimum gx supports);
  - `--approve-for-me`, `--dangerously-bypass-hook-trust` and every `-c` key gx passes are accepted;
  - OSC 7501 reaches the PTY and the title carries a session id;
  - hooks fire and `PermissionRequest` can deny;
  - one probe commit works with the gitdir as a writable root;
  - the rollout reader parses a real rollout.
- The rollout-format check parses the newest real rollout under `~/.codex/sessions` and a fixture,
  and verifies each field the readers use: `session_meta.id`, the `response_item` message,
  function_call and custom_tool_call shapes, `task_complete` with `error`, `token_count.rate_limits`
  and `compacted`. It names the missing field.
- Warnings: Codex hides the spawn model fields (the message names the key, and says tier choice is
  ignored for subagents), and managed rules are missing.
- `--record` captures real OSC streams, title frames and rollouts from 0.166.0 into test data.

### What Codex cannot do, and what the person sees instead

| Gap | What the person sees |
| --- | --- |
| No Codex history list | The history browser and `claude --resume` stay Claude-only. A person resumes a Codex session by hand with `codex resume`. |
| No dollar cost | Codex rows show tokens and a dash in dollar columns. Dollar totals count Claude only. |
| No budget limits | Budget soft and hard limits skip Codex. The usage gate and the rate-limit hold are the Codex limits. |
| No extra-usage warning | Codex enqueues show no warning. |
| No background-task hold | A Codex turn that ends with a terminal still running finishes at turn end, and the ticket finishes while a terminal ran. Closing the session kills the terminals. The implementation first verifies if a Codex terminal wakes the agent on exit, and if it does, the gate holds like Claude's with the same cap and recovery. |
| No `/stop` on interrupt | An interrupted tool stays as a background terminal and the row keeps the marker. The person types `/stop`. |
| No background-task signal or `run_in_background` for skills | A step that asks for it waits or runs inline. |
| No auto-retry on error | The ticket pauses `needs-answer` with Codex's error text and a Retry button. |
| No screen-only stall detection | The row shows the normal working state until the idle stall nudge fires. |
| No rollout before the first prompt | A Codex agent that died before its first prompt restarts from the Handoff, with no chat to read. |
| No per-tool disallow flag | A `PreToolUse` hook blocks the call and the agent asks through `gx tickets ask`. |
| No Plan mode switch, no `request_user_input` card | The hook blocks the call. gx never starts Plan mode, so no card is needed. |
| No Claude `auto` classifier | The automatic reviewer replaces it, with different behaviour. An obvious action asks nothing. A reviewer denial or doubt shows as an approval card. How often the reviewer asks, and what the model sees on a denial, are untested. |
| Network access cannot be limited to the gx socket | `network_access=true` also opens the internet to a Codex agent. |
| `--dangerously-bypass-hook-trust` runs any `hooks.json` a repo holds | A repo hook runs without review. |
| No `PreCompact` hook use, no "still compacting" signal | The same fixed 5-minute cap as Claude. gx ignores a compaction it did not start. |
| Subagent caps of 3 (v2) or 6 (v1) | A larger fan-out runs in waves. |
| Tier model and effort may be ignored when the config hides the spawn fields | `gx codex doctor` warns. The subagent inherits the session model. |
| Low-effort Codex follows skills less reliably | Visible only in the run log. |
| Archived rollouts need a decode | A notice with a button, not an automatic load. |
| Unknown rollout format | A notice in the chat, and `gx codex doctor` names the missing field. |
| Codex versions below 0.166.0 | Unsupported. `gx codex doctor` fails. gx builds no fallback. |
| Model shown only after the next reply | A hand switch of model shows after the next reply. |
| herdr hosting: no live reply, no approval cards, no resume | Codex on herdr gets a reduced flow. The person answers in the pane. |
| Switching harness at a limit | Not built. A limited ticket holds until the reset. |

### Testing seams

- The highest seam is `servertest` with `runnerfake` for server behaviour, and `gxfixture` with
  `fakecodex` for web flows. The Settings control is tested through the web, and the config write
  through the server API.
- `fakecodex` is a new binary on the shared `agentfake` core. `gxfixture` gets a `--fakecodex` flag
  that installs it as `codex` on PATH. It speaks OSC 7501 and the title, writes a rollout under
  `CODEX_HOME` (session meta, `token_count` with `rate_limits`, `compacted`, `task_complete` with
  `error`), and runs the hooks gx installs (`SessionStart`, `PreToolUse`, `PermissionRequest`,
  `Stop`) with Codex's JSON payloads on stdin, so the real `gx agent guard-bash` and the approval
  hook run in tests. The production `codexsession` reader reads the fake's rollouts in the same
  tests, so the two cannot drift. The fake reports 0.166.0 and a test fixes that version.

## Testing Decisions

- A good test drives the highest seam and checks what a person or caller sees: a ticket's status and
  reason, a run's runner and harness, a row's text, a pause, a config file. It never checks private
  fields or call order.
- Server behaviour is tested through `servertest` with `runnerfake`, the real `agentfake`-based
  `fakecodex` for flows, and Playwright on `gxfixture` for web flows. Server and vitest tests cover
  the launch shape, guard-bash, resume, recovery, the missing-binary startup refusal, the harness
  choice per entry point, and the runner function.
- CI runs only fixture-mode doctor checks, the fake-based server tests and the Playwright flows. No
  job needs a real Codex or a login.
- Recorded 0.166.0 fixtures (`gx codex doctor --record`) are committed as test data, like the
  `claudedoctor` testdata. A Codex upgrade means re-recording, and a failing fixture check shows the
  format change.
- `gx codex setup` tests are unit tests on a temporary `CODEX_HOME` through the env helper, never the
  real `~/.codex`. They check idempotence, the preview of what it writes, that an unmanaged rules
  file is untouched, and that other keys survive.
- Every package's `TestMain` keeps `claudeenv.Isolate()`, and the `envguard` ban on process-env reads
  (including `CODEX_HOME`) holds.
- Prior art: `servertest` Codex routing tests from `pty-as-default`, `claudedoctor` fixtures, the
  config-write tests for `project.json`, the Settings page tests in `webui`, and the flows 113 to
  116 and 103 to 104 (Queue agents switch, blocked pause, rate-limit hold, usage gate).

## Verification

A verify ticket runs these against a temporary gx server (`gxfixture`, PTY runner, `--fakecodex` and
fakeclaude). The implementer picks flow numbers after the highest in `webui/e2e/CATALOG.md`, and
file names.

New scenarios and flows to write. Each becomes an implementation ticket.

- Scenario `default-harness`: a project list with one project that overrides and two that inherit,
  one waiting ticket and at least one running Claude agent (so the switch notice has counts), an
  Agents tab with one Claude and one Codex agent, and one Claude agent whose newest reply names a
  model other than its tier's.
- Scenario `codex-queue`: one running Codex ticket, one running Claude ticket, a waiting ticket with
  no pin and a waiting ticket pinned to Codex.
- Scenario `codex-blocked`: a Codex ticket whose worker raises a `PermissionRequest` hook and stays
  blocked.
- Scenario `codex-rate-limited`: a Codex ticket whose rollout writes `rate_limits` at the limit and a
  queued Codex ticket, with a Claude ticket running.
- Scenario `codex-error`: a Codex ticket whose worker sends OSC 7501 `error` and writes a
  `task_complete.error`.
- Scenario `codex-design`: a Design run on Codex through a grilling round.
- Flow "Default harness setting": `default-harness`. Open Settings, open Agents, choose Codex, add
  and remove a project override. The config file holds the value and keeps other keys. A new Agents
  tab agent starts as Codex.
- Flow "Codex queue rows": `codex-queue`. Queue rows show the parts. Switching the default applies to
  the waiting unpinned ticket and not to the pinned one or the running ones. Run once with
  `--fakecodex` off to read the missing-harness reason.
- Flow "Codex approval card": `codex-blocked`. Approve and Deny reach the fake.
- Flow "Codex rate-limit wait": `codex-rate-limited`. The per-harness wait reason shows on the queued
  ticket while the Claude ticket still runs.
- Flow "Codex error pause": `codex-error`. The ticket pauses `needs-answer` with the rollout error
  text. Retry lifts the pause.
- Flow "Codex design run": `codex-design`. A grilling round, the run-end line and the Continue button.

Per story:

1. Flows "Default harness setting" and "Codex queue rows". `cmd` and server tests: with the default
   `codex`, an Agents tab agent, a design session, a Fix agent and a queue claim each start `codex`.
2. Flow "Default harness setting". Mock states `settings-agents`.
3. Flow "Default harness setting". Mock states `settings-agents` (override and inherit rows).
4. Flow "Default harness setting". Mock states `settings-home`.
5. TUI model test: the command opens the picker for global and, in a project, for the project. It
   writes the same keys as the web.
6. Server test on a temp config dir: the target is a symlink, the write keeps the link and other
   keys. Project write keeps other `project.json` keys. Remove override deletes the key.
7. Flow "Default harness setting". Mock states `settings-agents-switched`.
8. `cmd` test: a PATH without `codex` and default `codex`: non-zero exit, the message names the
   program. Mock states `start-error`.
9. Server test: the web write and the TUI picker refuse a harness not on PATH, the message names the
   program, and the file is unchanged.
10. Flow "Default harness setting" with `agent-runner` set to `headless` then `herdr`: the note reads
    as in the decisions. Vitest for the note text.
11. Flow "Default harness setting" (new agent starts as Codex); `gx agents list` shows the harness.
12. Flow "Codex design run". Server test: a design session started after a switch uses the new
    default, a chain inside one session keeps its harness.
13. Server tests: the Fix agent route and TUI Investigate start the default harness (`ui/tickets`
    test calls the route).
14. Server tests: a root auto-queue ticket and code-review, verify and evolve tickets claim with the
    default harness.
15. Flow "Codex queue rows" (switch applies to the waiting unpinned ticket). Server test: claim reads
    the default at claim time.
16. Server test: retry of a ticket that ran on Claude after a switch to Codex runs Claude.
17. Server test: a stored item with explicit `claude` stays `claude` after a switch.
18. `cmd` tests: `gx server queue --agent codex` and `gx agents new --harness claude` override for one
    launch.
19. Flow "Codex queue rows": the running tickets keep their rows after the switch. Mock states
    `settings-agents-switched`.
20. Flow "Codex queue rows" with `--fakecodex` off. Mock states `queue-rows` (missing-harness-reason).
    `gx server tickets explain <addr>` shows the same reason.
21. Flow "Default harness setting" (Agents tab). Mock states `agents-rows`.
22. Flow "Codex queue rows". Mock states `queue-rows`.
23. Flow "Codex design run". Mock states `design-header`.
24. TUI model tests for Agent rows, Queue rows and the design view.
25. Flow "Default harness setting": a fresh session shows "<Tier> · CLI default". Mock states
    `agents-rows`.
26. Flow "Default harness setting": the model changes after the next reply. Mock states `agents-rows`
    ("switched by hand").
27. Server test: the run's runner is PTY for Codex under `auto` and `pty`.
28. Server test: under `headless`, a Codex ticket records PTY and a Claude ticket records headless.
29. Server test with the herdr fake: under `agent-runner: herdr`, Codex runs on herdr.
30. Server test: no herdr probe banner under PTY, the banner shows only under `herdr`.
31. `cmd` test: `auto` with only `codex` on PATH and default `codex` starts. Default `claude` with no
    `claude` on PATH refuses.
32. Server test: a Claude launch with no `claude` on PATH waits with the reason.
33. Server test: the argument list of a Codex launch for each agent kind holds the shape (a table
    test).
34. Server test: the launch passes the inline hooks, the bypass flag and the update setting.
35. `gx codex doctor` probe commit check in a throwaway linked worktree. Server test: writable roots in
    the launch list.
36. Unit tests of `gx codex setup` (see Testing Decisions).
37. Server test: the system prompt is on a fresh launch and not on `codex resume`.
38. Flow "Codex approval card" and fakecodex tests: each OSC 7501 state maps to the row state.
39. Server test: a reply while working is refused by the busy guard and sent at `idle`.
40. Vitest on the composer: Tab send, verbatim multi-line text, the `$` hint on `/gx-`.
41. Flow "Codex approval card".
42. Server test: the hook does not fire and the screen parser (0.166.0 text) raises the card.
43. Flow "Codex approval card"; server test: blocked past 15 seconds pauses, a block under 15 seconds
    does not, Retry on a non-approvable block interrupts and sends the retry prompt.
44. Server tests with fakecodex startup dialogs: none becomes a pause.
45. Server tests: resume with the id and the rollout, start from the Handoff without.
46. Server test: `gx tickets ask` keeps the process, slot and worktree.
47. Server test: an answer with no agent reopens the ticket and the next claim uses the Handoff.
48. Server test: Interrupt sends no `/stop` and the row keeps the marker.
49. Server test: Restore of an exited Codex interactive session runs `codex resume`.
50. Vitest and server tests: the chat reads both harnesses through the one reader.
51. Vitest on the Codex chat view (shown and hidden items, `$gx-` as command).
52. Flow "Codex design run"; server tests for grilling counts on the shared list.
53. Server test: the run-end line for a Codex session names `$gx-design`.
54. Server test with a `.jsonl.zst` fixture: no decode on page load, the button decodes.
55. Vitest and server test: turns present and zero messages gives the notice.
56. Flow "Codex rate-limit wait" (a sample at 98% blocks Codex claims). Server test on a fake clock.
57. Flow "Codex rate-limit wait" (reason names Codex); server test: the Codex override lifts only
    Codex. `cmd` test for the verb with a harness argument.
58. Server test: the Codex hold keeps the slot, sends "continue" at the reset and stops claims for the
    harness.
59. Flow "Codex error pause".
60. Server test: a silent Codex agent gets the idle stall nudge.
61. Server test: recovery reads the rollout, and with no rollout the retry starts from the Handoff
    without an error.
62. Server test: `gx agent context-window` reads the fake's rollout, the smart-zone ceiling applies.
63. Server tests: the finish gate counts `compacted` items, an unreadable rollout waits to the cap.
64. Server test: the Background flag makes the agent promptable and marked, and the finish gate does
    not hold.
65. TUI model test: Watch on PTY shows the screen, and on herdr jumps to the pane.
66. Vitest and TUI test: tokens and a dash in the dollar columns for a Codex row.
67. Server test: dollar totals and budget limits skip Codex.
68. Server test: the extra-usage check runs for `claude` items only.
69. Skills install tests on a temp home: new root, old copies removed, duplicate warning.
70. The `"/gx-` literal test; server tests for the prefix per harness.
71. Bundle test: no `Read`/`Edit`/`Write` wording outside the table.
72. A test of every skill that reads `gx agent model` against the new output.
73. Bundle test for `agents/openai.yaml` in both directions.
74. Test: `--fits` in a symlinked working directory.
75. `gx codex doctor` fixture mode in CI. Live mode by hand.
76. Doctor test with a config that hides the spawn fields, and with no managed rules.
77. `gx codex doctor --help` and `--record` tests match the Claude doctor's.
78. Read the `gx-investigate` skill text.
79. `fakecodex` tests, including the version fix.
80. Hand-run smoke checklist (tagged "real Codex only", run after each Codex upgrade, never in CI):
    a usage-limit error and an auth failure (text and the `needs-answer` pause); an auto-compaction
    at the context limit; the Background flag and wake behaviour; `codex resume` after a restart; a
    reviewer denial under `--approve-for-me`.
81. The hardcoded-literal test.

Mock states (the stored mock `$(gx tickets root)/codex-support/mocks/07-default-harness-control.html`):

- `$(gx tickets root)/codex-support/mocks/07-default-harness-control.html`, state `settings-home`,
  width 480, scenario `default-harness`, parts `settings-list`, `agents-link`.
- `$(gx tickets root)/codex-support/mocks/07-default-harness-control.html`, state `settings-agents`,
  width 640, scenario `default-harness`, parts `default-harness`, `project-list`.
- `$(gx tickets root)/codex-support/mocks/07-default-harness-control.html`, state
  `settings-agents-switched`, width 640, scenario `default-harness`, parts `default-harness`,
  `switch-notice`.
- `$(gx tickets root)/codex-support/mocks/07-default-harness-control.html`, state `agents-rows`,
  width 640, scenario `default-harness`, part `agent-rows`.
- `$(gx tickets root)/codex-support/mocks/07-default-harness-control.html`, state `queue-rows`,
  width 640, scenario `codex-queue` (the part `missing-harness-reason` runs with `--fakecodex` off),
  parts `queue-rows`, `missing-harness-reason`.
- `$(gx tickets root)/codex-support/mocks/07-default-harness-control.html`, state `design-header`,
  width 640, scenario `codex-design`, part `design-harness`.
- `$(gx tickets root)/codex-support/mocks/07-default-harness-control.html`, state `start-error`,
  width 640, no fixture scenario (a console and log message, checked by the `cmd` test in story 8),
  part `missing-default-error`.

## Out of Scope

- Switching harness when a limit is reached.
- opencode. This effort covers Claude and Codex only.
- A harness picker in the Agents tab Create form. The Default harness control replaces it.
- Live replies, approval cards and resume on herdr. herdr stays a backup with its reduced flow.
- Resume on the headless runner. It is Claude-only.
- A Codex history list and `codex resume` from the history browser.
- Codex versions below 0.166.0, and any fallback for them.
- `codex app-server` as a way in.
- Migrating queue items stored before this change.

## Further Notes

- Map decisions: `gx:codex-support` tickets 01 to 14. This map takes over `gx:multi-harness-runners`,
  whose research stays readable there (mainly ticket 04: Codex rollout format and session mapping).
  Research notes live on the branches `research/codex-signals-on-pty`, `research/gx-skills-on-codex`
  and `research/codex-retest-0-166`.
- This spec reads `pty-as-default` as already built. It removes the Codex-to-herdr route that spec
  adds.
- ADRs that apply: 0031 (superseded in part by the `pty-as-default` ADR, which this effort amends),
  0042 (pauses recoverable from the web), 0039 (server launches never adopt), 0037 (live attach).
- Untested at design time, to be checked in implementation: how often the automatic reviewer
  escalates and what the model sees on a denial, whether a duplicate `developer_instructions` reaches
  the model on resume, whether a Codex background terminal wakes the agent when it exits, and
  `--add-dir` itself (the `writable_roots` key was the tested path).
