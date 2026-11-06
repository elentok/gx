# What each chat message says today

Research for ticket gx:better-notifications/02. Read from the code on branch
`better-notifications-design` (commit 54e3ac5f0) and from real data on 2026-11-06.

## Summary

- Every message goes through one layout, `chatmarkup.Style.Message(emoji, headline, counts, detail,
  identity)`. It prints `emoji *headline*`, a blank line, an optional counts line, an optional detail
  block, then the identity line. The identity line is `[gx] <epic>` or `[gx] <epic>/<NN>`.
- On the server, `ServerChat.sendIn` files each message under a bold project name (a "group"). The
  6-second batch (`renderBatch`) joins messages of one project under one bold project line, and
  joins projects with `---`. So the first line a phone shows is the project name, not the headline.
- Only `ServerChat` is wired in production. `newChatEventSink` is called in one place
  (`server_chat.go:152`) with a no-op inner sink. So the run-loop methods of `chatEventSink`
  (`EpicStarted`, `IterationStarted`, `IterationPaused`, `IterationResumed`, `IterationFinished`,
  `TicketNeedsHuman`, `EpicPaused`, `EpicComplete`) are never called in production. Their renderers
  (`epicStartedText`, `iterationStartedText`, `iterationPausedText`, `iterationResumedText`,
  `epicPausedText`, `epicFailedText`, `epicCompleteText`) are dead for chat. Treat them as legacy
  when mocking. `epicFailedText` has no caller at all.
- A ticket title is available at only one live call site: `TicketDone`. Every pause, recovery
  escalation, conflict and result message has the ticket ID (or only an address) and no title,
  though most of them hold the ticket file path and could read the title from it.
- Pause detail is `<kind>: <reason>` with the raw reason string, except a `self-reported` pause that
  has an open question, which uses the question's one-line summary. Recovery escalation detail is
  a four-line machine block. A one-off result is the ticket's `## Result` section, cut at 1500
  characters.
- The run log only records `notification-sent` events for `result` messages (121 events, all
  Telegram). All other kinds are sent through the server sink, which has no run log, so their
  rendered text is not stored anywhere. The pause and escalation examples below are rendered by the
  code's own format from real `paused` and `recovery-escalated` events in the run logs. They are
  marked "rendered". The result examples are the stored text.

## Sources

| What | Where |
|---|---|
| Layout | `chatmarkup/chatmarkup.go` `Style.Message` |
| Templates | `ralphloop/notify/notification_text.go` |
| Server call sites, `Notice`, `SessionAlert` | `ralphloop/notify/server_chat.go` |
| Batch, group header, mute notices | `ralphloop/notify/chat_eventsink.go` (`renderBatch`, `sendIn`) |
| Kind constants | `ralphloop/notify/eventlog_notify.go` (`notifyKind*`), `server_chat.go` (`notifyKindResult`, `notifyKindSessionAlert`) |
| Pause detail | `server/ticketwrite/pause_chat.go` `PauseChatDetail` |
| Escalation detail | `server/recovery/recovery.go` `escalate` |
| AFK session alerts | `server/sessions/sessionbanner.go` `fireBanner` |
| Real data | `~/.local/share/gx/tickets/*/run-log.jsonl`, `.../issues/*.jsonl` (ticket logs), `~/.local/share/gx/sessions/*/meta.json`, `~/.local/state/gx/server.log` |

## The layout

For a message with a project group, Telegram receives this shape (MarkdownV2 escaping omitted):

```
*gx*                              <- group header: bold project name, added by renderBatch

🆘 *needs answer*                 <- emoji + bold headline
                                  <- blank line
<detail line(s)>                  <- optional counts line, then optional detail
[gx] gx/better-epic-header-impl/04  <- identity line
```

`Style.Message` always puts a blank line after the headline. `identityLine` returns
`[gx] <epic>` or `[gx] <epic>/<ticket>`. On the server the "epic" is `project/epic`, so a pause on
ticket 04 of epic `foo` in project `gx` ends with `[gx] gx/foo/04`.

Identical texts in one batch window collapse into one with ` ×N` appended to the last line.

## Every message kind

Kinds marked "legacy" are rendered by code that production never calls (see Summary).

| # | Notify kind | Renderer | Triggered by | Headline | Detail lines | Identity line | Title available at call site? |
|---|---|---|---|---|---|---|---|
| 1 | `ticket-needs-human` (answer class) | `ticketNeedsHumanText`, via `ServerChat.Pause` | A ticket pause with an answer-class kind: `ticketwrite.Writer.sendPause` (`pause_ticket.go:78`), `Merger.pauseRoot` (`epicmerge_root.go:296`) | `🆘 needs answer` | `<kind>: <reason>` (or the open question's summary for `self-reported`), then the epic counts line | `[gx] project/epic/NN` | No. Gets `addr.ID` and the ticket path only. The `kind` argument is ignored for the headline. |
| 2 | `ticket-needs-human` (repair class) | `ticketNeedsHumanText`, via `ServerChat.Pause` | Same call; the pause status is repair class | `🛑 needs repair` | Same as row 1 | Same | No |
| 3 | `ticket-needs-human` (child pause) | `ticketNeedsHumanText`, via `finishSink.TicketNeedsHuman` | A conflict-resolution child ticket pauses during the server finish path (`server_chat.go:211`). The ticket's own pause is dropped | `🆘 needs answer` or `🛑 needs repair` | Same as row 1 | `[gx] project/epic/<child NN>` | No. Identifier only. Mute source is the epic, not the ticket. |
| 4 | `ticket-needs-human` (recovery) | inline `Message("⚠️", "recovery escalated", ...)` in `ServerChat.Escalated` | `Recovery.escalate` (`recovery.go:214`): a remedy failed, or left the ticket paused, or no rule matched | `⚠️ recovery escalated` | Four lines: `<failure kind>: <failure reason>`, then why (`<reason>[: <outcome>]`), `entry: <rule id or "no match">`, `report: <ticket file path>` | `[gx] project/epic/NN`, or `[gx] project/epic` for an epic-level escalation | No. `ref.Ticket` is loaded by the caller, but only its ID and path reach `Escalated`. |
| 5 | `conflict-resolution-started` | `conflictResolutionStartedText` | `finishSink.ConflictResolutionStarted` | `🔀 resolving merge conflict` | `An agent is resolving the cherry-pick conflict` | `[gx] project/epic/NN` | No. Identifier only. |
| 6 | `iteration-finished` | `iterationFinishedText`, via `ServerChat.TicketDone` | `sched/run.go:339`: a ticket merged | `✅ <ticket title>` (the title is the headline, no status word) | `<elapsed> · <tokens> · <cost> · <N done · N in progress · N total>` (the counts go in the "counts" slot, so it is the first line after the blank) | `[gx] project/epic/NN` | Yes. Gets the full `tickets.Ticket`. |
| 7 | `epic-complete` (merged) | `epicOutcomeText`, glyph `🎉` | `Merger.notifyEpicOutcome` after the epic branch went onto its target | `🎉 epic merged` | Counts line (`N done · ... · N total`), then `N ticket(s) merged in <elapsed> · <cost>` and `· N recovered automatically` when above zero | `[gx] project/epic` | No ticket. Epic only. |
| 8 | `epic-complete` (not merged) | `epicOutcomeText`, glyph `✅` | Same, for an epic that completed without merging (also after a `needs-rebase` root pause) | `✅ epic complete (not merged)` | Same as row 7 | `[gx] project/epic` | No ticket |
| 9 | `result` | inline `Message("✅", address+" finished", ...)` in `ServerChat.Result` | A one-off submitted with notify finishes (`oneoff.go:373` `NotifyResult`), or an investigate ticket whose proposal has no follow-ups project (`followups.go:78`) | `✅ project:epic/NN finished` (a full address, no title) | The ticket's `## Result` section, cut at 1500 characters with `…`. Empty when the section is empty. | `[gx] server` (literal, not the project) | No. Address only, though `NotifyResult` parses the ticket file just before. |
| 10 | `ticket-needs-human` (herdr digest) | inline in `ServerChat.PauseDigest` | `Writer.holdPause` collects pauses while herdr is down; `FlushPauseFold` sends one message when herdr is back or the cap timer fires | `🅿️ N paused while herdr was down` | One line per pause: `[project] epic/NN — status (kind): reason` | `[gx] server` | No. Lines carry IDs only. |
| 11 | `server-started` | `Notice` | `server.go:507` | `🚀 server started` | `build <version>` | `[gx] server` | n/a |
| 12 | `long-ticket` | `Notice` | `budget/longticket.go:63` | `⏳ long ticket` | `<project:epic/NN> has run <N> assistant turns` | `[gx] server` | No. Address only. |
| 13 | `extra-usage` | `Notice` | `budget/extrausage.go:62` | `⚠️ extra usage is on` | `Your Claude account will auto-purchase extra usage once included usage runs out.` | `[gx] server` | n/a |
| 14 | `daily-summary` | `Notice` | `budget/notify.go:83` | `📊 daily summary` | `<date>: $X spent` | `[gx] server` | n/a |
| 15 | `budget-soft-limit` | `Notice` | `budget/notify.go:87` | `💸 budget soft limit reached` | `$X of $Y today` | `[gx] server` | n/a |
| 16 | `budget-hard-limit` | `Notice` | `budget/notify.go:91` | `🛑 budget hard limit reached` | `$X of $Y today` | `[gx] server` | n/a |
| 17 | `disk-warn` | `Notice` | `disk/diskwatch.go:271` | `💾 low disk space on <disk>` | `<free> free, under the warn threshold of <warn>` | `[gx] server` | n/a |
| 18 | `disk-resume` | `Notice` | `disk/diskwatch.go:241` | `💾 dispatch resumed on <disk>` | `<free> free; queued tickets start again` | `[gx] server` | n/a |
| 19 | `disk-clean` | `Notice` | `disk/diskclean.go:177` | `🧹 disk clean finished on <disk>` or `disk clean failed on <disk>` | `<outcome>; <free> free now; dispatch is held` (or `is not held`) | `[gx] server` | n/a |
| 20 | `herdr-unavailable` | `Notice` | `sched/herdrwatch.go:164` | `🔌 herdr unavailable` | `<herdr is down \| herdr was not found on PATH>; waiting: <addresses>` | `[gx] server` | No. Addresses only. |
| 21 | `herdr-available` | `Notice` | `sched/herdrwatch.go:169` | `✅ herdr back` | `herdr answers again` | `[gx] server` | n/a |
| 22 | `project-unavailable` | `Notice` | `projects/unavailable.go:80` | `📁 project <name> unavailable` | `<path> does not exist; its tickets will not run. Fix it with gx project set-path <name> <path>` | `[gx] server` | n/a |
| 23 | `trunk-missing` | `Notice` | `merge/missingtrunk.go:103` | `🪵 project <name>: trunk missing` | `<reason>; its tickets will not be claimed until it is fixed` | `[gx] server` | n/a |
| 24 | `trunk-found` | `Notice` | `merge/missingtrunk.go:87` | `🌳 project <name>: trunk found` | `trunk found, claiming resumes` | `[gx] server` | n/a |
| 25 | `session-alert` | inline in `ServerChat.SessionAlert` | `sessionbanner.go` `fireBanner`, only when AFK is on | `🔔 <project> · <session name>` | Banner body (a reason, or `State: kind — message`), plus the Design page link for a Design session | `[gx] project` | A session name only. See the AFK section. |
| 26 | `muted` | `mutedText` | The gate's per-source mute trips (edge-triggered) | `🔇 muting this` | none | `[gx] ` with empty epic and ticket on the server (the server sink has no epic name) | No |
| 27 | `globally-muted` | `globallyMutedText` | The gate's global mute trips | `🚫 globally muted` | ``re-enable with `gx notify --enable <transport>` `` | `[gx]` | n/a |
| 28 | `test` | `testMessageText` | `gx config test-notifications` | `🔔 test notification` | `If you can see this, notifications are working.` | `[gx]` | n/a |
| 29 | `cli-notify` | `notify.go` | `gx notify "..."` | The text as written | n/a | n/a | n/a |
| L1 | `epic-started` (legacy) | `epicStartedText` | Run-loop `EpicStarted`; never called on the server | `🚀 epic started` | Counts line | `[gx] epic` | No |
| L2 | `iteration-started` (legacy) | `iterationStartedText` | Run-loop `IterationStarted` | `▶️ <ticket title>` | none | `[gx] epic/NN` | Yes |
| L3 | `iteration-paused` (legacy) | `iterationPausedText` | Run-loop `IterationPaused`, non-repair only (rate limit) | `⏸ paused` | `<label>: <reason>` | `[gx] epic/NN` | No |
| L4 | `iteration-resumed` (legacy) | `iterationResumedText` | Run-loop `IterationResumed` | `▶️ resumed` | `<label>` | `[gx] epic/NN` | No |
| L5 | `epic-paused` (legacy) | `epicPausedText` | Run-loop `EpicPaused` | `🅿️ epic paused` | `Nothing runnable left; waiting on <ids>` | `[gx] epic` | No |
| L6 | `epic-complete` (legacy) | `epicCompleteText` | Run-loop `EpicComplete` | `🎉 epic complete` | Counts line, then `N ticket(s) merged in <elapsed> · <cost>` | `[gx] epic` | No |
| L7 | `epic-failed` (legacy, no caller) | `epicFailedText` | none | `🔥 epic failed` | Counts line, then the raw Go error | `[gx] epic` | No |

Notes on the table:

- Rows 11 to 24 are all `ServerChat.Notice`. They are not filed under a project group, so there is no
  bold project line on them. They go to the global destination only, never to a project override.
- Rows 1 to 9 are filed under the bold project name. Rows 10 to 28 use plain `send`, with no group.
- Row 10 and rows 11 to 24 go to the global destinations only. Rows 1 to 9 and 25 go to the project's
  destinations (its override, or the global ones).
- Code comments and the real headline text disagree. `ticketNeedsHumanText`'s doc comment says
  "paused for an answer" and "paused for repair"; the code prints `needs answer` and `needs repair`.
- `ticketNeedsHumanText` takes `kind events.Kind` and never uses it. Nothing in the headline says
  why the ticket paused; that lives only in the detail line.

## Where the detail text comes from

| Message | Detail source | Shape |
|---|---|---|
| Pause (rows 1 to 3) | `PauseChatDetail(ticketPath, kind, reason)` | A raw error string with the kind in front: `iteration-error: <Go error chain>`. For `self-reported` with an open question: the question's one-line `summary`, written by the agent (a ticket section in effect). |
| Recovery escalation (row 4) | `recovery.go` `fmt.Sprintf("%s: %s\n%s\nentry: %s\nreport: %s", ...)` | Raw failure kind and reason, a rule ID, and an absolute file path. |
| Result (row 9) | The ticket's `## Result` section | Free Markdown written by an agent, escaped but not rendered, cut at 1500 runes. |
| Conflict (row 5) | Fixed sentence | `An agent is resolving the cherry-pick conflict` |
| Notices (rows 11 to 24) | Fixed sentence with values filled in | Plain words. These are already readable. |
| Herdr digest (row 10) | `fmt.Sprintf("[%s] %s/%s — %s (%s): %s", ...)` | Raw reason per line, with an em dash and a bracketed project. |
| AFK alert (row 25) | Banner reason or `sessionStatusText` | Fixed words plus the harness's own status message. |

### Pause reasons by kind in the run logs

189 `paused` events across all run logs: `self-reported` 100, `iteration-error` 79, `blocked-pane` 6,
`zero-commit` 3, `agent_name_taken` 1. The answer-class kinds are `self-reported` and `zero-commit`;
the rest are repair class (from the `needs-answer` and `needs-repair` log events).

The iteration-error text comes straight from Go errors (`agentrunner: ...`, `launch failed: ...`,
`stall nudge for ...`). Most of the 79 are the same "stall nudge" string.

## Real examples

### Pause messages (rendered from real `paused` events)

Each block shows the message body as Telegram would show it, before MarkdownV2 escaping. The bold
project line above it (`*gx*`) is left out. The counts line is made up here (`4 done · 1 paused: NN ·
12 total`) because the log does not store it; the real one is computed at send time.

**Messy 1. Stall nudge, 79 of 189 pauses have this family of text.** Source:
`gx/agent-lifecycle-impl` run log, 2026-11-01 23:59, ticket 08a (about 25 more identical, differing
only in the ticket ID).

```
🛑 needs repair

iteration-error: stall nudge for agent-lifecycle-impl-iter-08a: agentrunner: prompt not delivered: submission never reached the pane: input not empty after clearing
4 done · 1 paused: 08a · 12 total
[gx] gx/agent-lifecycle-impl/08a
```

**Messy 2. Dead session with a socket path.** Source: `gx/webui-paste-image-impl` run log,
2026-11-06 00:57, ticket 04.

```
🛑 needs repair

iteration-error: agentrunner: session died: holder failed: dial unix /Users/david/.local/state/gx/agents/gx/webui-paste-image-impl-iter-04/holder.sock: i/o timeout
...
[gx] gx/webui-paste-image-impl/04
```

**Messy 3. A herdr CLI command and JSON, on several lines.** Source: `gx/cursor-support` run log,
2026-10-30 18:34, ticket 02.

```
🛑 needs repair

iteration-error: agentrunner: session not found: $ herdr agent wait w13F:pA --until idle --until done --timeout 1999

exit status 1

{"error":{"code":"agent_not_found","message":"agent target w13F:pA not found"},"id":"cli:agent:wait"}
...
[gx] gx/cursor-support/02
```

**Messy 4. A git error pasted whole.** Source: `gx/follow-ups-2026-11-01` run log, 2026-11-01
21:45, ticket 05a.

```
🛑 needs repair

iteration-error: launch failed: creating detached worktree at refs/heads/follow-ups-2026-11-01: git worktree add --detach /Users/david/dev/gx/follow-ups-2026-11-01-item-05a refs/heads/follow-ups-2026-11-01 failed (exit 128):

Preparing worktree (detached HEAD 543d5276)
fatal: '/Users/david/dev/gx/follow-ups-2026-11-01-item-05a' already exists
...
[gx] gx/follow-ups-2026-11-01/05a
```

**Messy 5. A pnpm log in the reason.** Source: `gx/one-offs` run log, 2026-10-29 08:24, ticket 53. The
reason starts `launch failed: installing dependencies in /Users/david/dev/gx/one-offs-item-53/web:
pnpm install --frozen-lockfile: ✓ Lockfile passes supply-chain policies (verified 4d ago)` and
continues with the whole install output (the stored text is cut at 400 characters here; the log keeps
more).

**Messy 6. A long instruction.** Source: `gx/better-epic-header-impl` run log, 2026-11-06 00:19,
ticket 06, kind `blocked-pane`.

```
🛑 needs repair

blocked-pane: better-epic-header-impl-iter-06 is blocked on a prompt gx did not send (permission prompt); press Approve or Deny on the card, press Retry when it has no card, answer with `gx server agents answer`, or type in the Agents tab live panel
...
[gx] gx/better-epic-header-impl/06
```

**Messy 7. Internal jargon.** Source: `gx/codex-support-impl` run log, 2026-10-31 16:36, ticket 44.

```
🛑 needs repair

iteration-error: agent reported finished at 2026-10-31T14:31:29Z but the ticket was never processed
```

**Short and clear, for contrast.** `zero-commit: no commits merged` (`gx/cursor-support/01`) and
`zero-commit: no result reported` (`gx/one-offs/52`). These read fine apart from the kind prefix.

**Self-reported, with a question summary (good).** With an open question the kind prefix is dropped
and the detail is only the agent's one-line summary. Source: ticket logs under
`gx/*/issues/*.jsonl`; 109 questions carry a `summary`. Example:

```
🆘 needs answer

Finish gate rejects one follow-up for 25 failures; lift the gate, or split 08a47 into 25 tickets?
...
[gx] gx/agent-lifecycle-impl/08a15
```

Source: `gx/agent-lifecycle-impl/issues/08a15-testautoadvance-endingsthatneveradvance-done.jsonl`.
Other real summaries: `Mock check blocked: browser rejects the fixture's self-signed https cert. How
to proceed?` (`gx/webui-security-impl/32`), `The agent is blocked on a prompt gx did not send.`
(`gx/webui-agents-skills-impl/37`).

**Self-reported, no question (messy).** When no question is open the text falls back to the pause
reason. The run log shows `agent asked via iteration_status` for the first four self-reported
pauses in `gx/agent-lifecycle-impl`, so the chat line would be:

```
🆘 needs answer

self-reported: agent asked via iteration_status
```

### Recovery escalation (rendered)

Real `recovery-escalated` events are short. Source: `gx/split-server-pkg-impl` run log, 2026-11-04
18:07, ticket 33: kind `iteration-error`, reason `recovery investigate-retry for iteration-error
failed: ticket failed again`. The chat body is built from the failure kind and reason, the why, the
entry and the report path:

```
⚠️ recovery escalated

iteration-error: <failure reason as paused>
recovery investigate-retry for iteration-error failed: ticket failed again
entry: investigate-retry
report: /Users/david/.local/share/gx/tickets/gx/split-server-pkg-impl/issues/33-....md
[gx] gx/split-server-pkg-impl/33
```

The failure reason line is the same raw pause reason as in the pause examples above. The entry ID is
filled in by hand here; the log event does not store it. 45 `recovery-escalated` events exist, and
most are epic-level (`already recovered all-parked 1 time(s)`, `all-paused`) with no ticket, so their
identity line is `[gx] project/epic`.

### One-off results (stored text, Telegram-escaped)

121 `notification-sent` events, all `result`, all in `gx/one-offs`. 67 of them are the empty form,
66 of those for ticket 53 alone (the same ticket re-sent). Stored body, 2026-10-21 02:09:

```
✅ *gx:one\-offs/01 finished*

\[gx\] server
```

Stored body, 2026-11-04 09:56 (one of the 66 empty ones for 53):

```
✅ *gx:one\-offs/53 finished*

\[gx\] server
```

Stored body, 2026-10-21 03:02 (head and tail; the whole body is 1,673 characters):

```
✅ *gx:one\-offs/02 finished*

4 new · 9 seen again · 10 not filed · 20 flagged unread

\*\*New follow\-ups\*\*

\- \`gx:follow\-ups/22\` validate should flag a ticket number in the body that is missing from \`blocked\_by\`
\- \`gx:follow\-ups/23\` agents still run the whole \`\./server\` test package repeatedly, and \`verify\` runs it too \(regression\)
...
\- \`gx:agent\-skills/01\` fork trigger skipped, context estimates 2\-3x low
...
\[gx\] server
```

What this shows:

- The Markdown in the result is escaped, not rendered. Bold shows as literal `**New follow-ups**`
  and code spans as literal backticks.
- The headline carries `gx:one-offs/02` and no ticket title.
- The identity line says `server`, not the project or the epic.
- A result with an empty `## Result` section still sends a message (67 of 121).
- The result is cut at 1500 characters; the text before the cut is not trimmed to a line break.

### Ticket merged (row 6)

`TicketDone` is the only ticket-level message with a title. No stored example exists. The code
renders:

```
✅ <ticket title>

5m32s · 39k tok · $1.23 · 7 done · 1 in progress · 12 total
[gx] project/epic/NN
```

The title is the headline, so a long title fills the first line, and the status word is only the
icon. Cost and tokens come from the merged ticket's metrics.

## AFK session alerts

Code: `server/sessions/sessionbanner.go` `fireBanner` calls `mg.alerts.SessionAlert(info.Project,
override, title, chatBody)` when AFK is on. AFK skips the focus check; the OS banner and the sound
keep it.

- Title: `bannerTitle(info)` is `<project> · <session name>` (`gx · fixed-context-monitor`).
- Body: the trigger's reason when there is one, else `sessionStatusText(info)`.
  - `sessionStatusText` builds `<Label>[: kind][ — message]` with labels `Done` (idle), `Blocked`,
    `Rate-limited`, `Exited`. For example `Blocked: permission — run ls?`.
  - Reasons passed to `TriggerBanner` include: the actionable reason (`a.Reason`),
    `Auto-advance stopped after N runs in a row: <reason>`, and failure labels (`Agent crashed`,
    `Session ended`, `Rate limited`, `Claude login needed`, `No progress for 15 min`, `Agent hit an
    error`, `Waiting at a prompt in the terminal`). Auth and stalled pass their label as the reason;
    other failure kinds use the status text.
- Chat body adds a new line with the Design page link (`<web url>/#/d/<root>`) when the session is a
  Design and the web listener is on.
- Chat layout: `Message("🔔", title, "", body, identityLine(project, ""))`, so the bell and the
  `project · name` are the bold headline, and the last line is `[gx] project`. It is sent with
  `s.send` (ungrouped), not under the bold project line. Gate source is `server`, kind
  `session-alert`.
- The name has no ticket title or ID. For an implement-ticket or free-form session it is the name the
  person gave or the harness reported, such as `gx:better-epic-header/06 ralph-loop bug`,
  `Queued epics not starting gx:fast-queue-startup`, `remote-control1`, `design-create-tickets-bug`.

What a Design session's alert looks like today. A Design session is named after its epic
(session `9f66f8`: `name` `fast-queue-startup`, `ticket` `gx:fast-queue-startup`, `design: true`).
When it goes idle, the status text is `Done`:

```
🔔 *gx · fast\-queue\-startup*

Done
https://<web host>/#/d/gx:fast-queue-startup
\[gx\] gx
```

(Rendered by the code from the session data above. The link host and the exact root form come from
`designLink` and `queue.RootOf`; the placeholder host is not in the log. The server log records only
OS-banner failures for these sessions, not the chat text.) When blocked on a permission it reads
`Blocked: permission — <message>`. There is no question-round alert and no "ready to queue"
alert today; those are the two new messages the destination asks for. A Done Design exiting cleanly is
excluded from the failure alert by `designDone`.

## Findings for the mocks

1. Today's first line on a phone is the bold project name, then the emoji headline. The headline for
   pauses is a two-word status (`needs answer`, `needs repair`) that carries no ticket or title.
2. The ticket ID is in the last line only, in a three-part form (`project/epic/NN`).
3. Pause and escalation details are raw strings with machine prefixes. The mock needs a
   plain-word description step for the `iteration-error` family (stall nudge, session died, herdr
   wait failed, worktree exists, launch failed) and for `blocked-pane`.
4. Titles: only `TicketDone` has one. For every other ticket-level message the title is reachable from
   the ticket path the call site already holds (`Pause`, `Escalated`, `FinishSink`) or from the address
   (`Result`, `long-ticket`, herdr digest and notice). Epic-level and Notice messages have no ticket.
5. Dead code for chat: the seven legacy renderers. The new layout does not need to restyle them.
6. The `result` and `session-alert` messages use `server` or the project as identity instead of the
   epic. The `muted` notice has an empty identity.
7. The Notice family (rows 11 to 24) is already plain words; only the headline order and the identity
   line change.
