# Web UI agent views: tool in flight, settled Screen modal, vim-mode launch check

Source: map `gx:webui-agent-views` (tickets 01 to 04). Ticket numbers appear as `[T01]` to `[T04]`.
Research behind the third change: `docs/research/webui-agent-views-03-slash-prompt-hang.md`.

## Problem Statement

Three things in the web UI and the agent launch path let down the person who watches an agent work.

1. During a long turn the Working row (design chat and Agents tab chat) reads only "Working". It
   cannot say whether the agent runs a shell command, waits on a sub-agent or searches. One turn ran
   11 minutes with the row unchanged.
2. In the Screen modal, a press of a key button (Enter, arrows, Esc, Tab) refetches the screen right
   away, before the agent has redrawn. The person sees the old rows, cannot tell whether the key did
   anything and presses Refresh by hand.
3. A fresh Claude agent sometimes stops taking keys after gx types its first prompt (one that starts
   with `/`). The prompt sits unsubmitted, the footer reads `-- INSERT --`, and even Ctrl+U changes
   nothing. About 12 sibling tickets launched on 2026-11-02 each needed a relaunch.

## Solution

1. The Working row names the tool call that is running now, for example "running shell: make test
   (+2 more)". The count opens a popover listing the other calls in flight.
2. After a key press or Type text, the Screen modal shows the screen once the agent has redrawn it.
   It marks the rows that changed, and says "Screen unchanged" when nothing changed.
3. gx checks, on every Claude launch (Queue agents on the native PTY launcher or herdr, and web
   sessions), that Claude is not in vim mode before it types the first prompt. If a vim footer shows,
   gx logs an event and relaunches once. The hung-pane recovery that `gx:agent-launch-and-recovery-impl`
   03 to 05 ships (already on trunk) stays as the fallback.

Honest status of change 3: the mechanism inside Claude is **unproven and was not reproduced**. What
is verified is a correlation (see Further Notes). The spec makes the likely cure a checked invariant
instead of a silent setting; it does not claim to have found the cause.

## User Stories

### Working row

1. As a person watching a design chat, I want the Working row to name the tool call running now, so
   that a long turn no longer reads only "Working".
2. As a person watching an Agents tab chat, I want the same tool-in-flight text in its Working row,
   so that both chats behave alike.
3. As a person, I want a shell call shown as "running shell: <command>", so that I see which
   command holds the turn.
4. As a person, I want a sub-agent call shown as "waiting for sub-agent: <label>", so that I know
   the turn waits on someone else.
5. As a person, I want a search call shown as "searching: <label>", so that I can tell a search from
   a stall.
6. As a person, I want any other tool shown with its tool name as the label, so that no call is
   hidden.
7. As a person, I want long labels cut to 60 characters with an ellipsis, taken from the first line
   of a command, so that the row stays one line.
8. As a person, I want the newest unmatched call shown as the current tool, so that the row follows
   the work.
9. As a person, I want a count such as "(+2 more)" when several calls run at once, so that I know
   there are more.
10. As a person, I want the count to be a button that opens a popover with each other call as kind
    and label, so that I can see all of them.
11. As a phone user, I want the popover to open on tap, so that I can read it without hover.
12. As a keyboard user, I want the count to open its popover on focus, so that I do not need a
    mouse.
13. As a person, I want the row ordered step, title, current tool, then "waiting on" background
    tasks, so that the row reads the same every time.
14. As a person, I want a backgrounded shell never to show twice, so that the row is not noisy.
15. As a person, I want the row to read "Working" as today when nothing is in flight, so that idle
    gaps between tools look unchanged.
16. As a Codex user, I want the same behavior for Codex sessions, so that the row is not Claude-only.

### Screen modal

17. As a person looking at a dialog in the Screen modal, I want the screen to show the agent's
    redraw after I press a key button, so that I do not press Refresh by hand.
18. As a person, I want the same settled screen after Type text, so that I see what my text did.
19. As a person, I want the rows that changed since the last read marked, so that a small dialog
    change is easy to see.
20. As a person, I want the marks to fade after a few seconds, so that old changes do not linger.
21. As a person, I want "Screen unchanged" when the settled screen equals the old one, so that I
    know the key did nothing.
22. As a person, I want Refresh to stay and do one plain read, so that I can still look again.
23. As a person, I want the modal to still show the last rows read when the settle fails or hits its
    cap, so that a slow agent never leaves me with an error and no screen.
24. As a chat user, I want quick keys in chat and the CLI to keep today's fast reply, so that they do
    not wait on a settle.
25. As a person with a holder-backed session (every session the keys route accepts), I want the
    settle to work the same for Claude and Codex, so that the modal does not depend on the agent.

### Vim-mode launch check

26. As a person launching a Claude agent, I want gx to check Claude is not in vim mode before the
    first prompt is typed, so that the prompt cannot land in a wedged editor.
27. As a person, I want the check on all three Claude launch paths (the native PTY launcher, the
    herdr runner, and the web session first prompt in `firstPrompt.deliver`), so that no launch skips
    it. The headless runner has no editor and is skipped.
28. As a person, I want either vim footer (`-- INSERT --` or `-- NORMAL --`) to trip the check, so
    that a vim editor in any mode is caught.
29. As a person, I want gx to log an event and relaunch Claude once when a footer shows, so that I
    can see it happened and the agent starts clean.
30. As a person, I want that relaunch to happen after Ready and before the prompt is typed, so that
    no prompt is lost in the first Claude.
31. As a person, I want the vim check to relaunch at most once per launch, so that a stuck setting
    cannot loop. The existing hung-pane recovery keeps its own budget and outcome.
32. As a person, I want gx, when a footer persists after the relaunch, to log again, type the prompt
    anyway and rely on the hung-pane recovery, so that the launch still moves on.
33. As a person, I want the vim check to add no pause kind and no new way into an existing one, so
    that the queue and the Agents tab show nothing new to learn. A persistent footer on a pane that
    also hangs may end in the existing `agent_prompt_stalled` launch failure, which the hung-pane
    recovery already produces today and which the Retry button on the parked ticket ends.
34. As a Codex user, I want Codex launches left alone, so that they pay no cost for a Claude setting.
35. As a person, I want a healthy Claude launch (no footer) to pay no extra relaunch and no visible
    delay beyond one settled screen read, so that the common path stays fast.

## Implementation Decisions

### Working row [T01]

- The server builds the chat response's in-flight tools from tool calls that have no result yet.
  Nothing is stored. Only calls in the current turn count, with the turn found the way
  `transcript/unexecuted_tool_call.go` finds it, so a call left open by an interrupted earlier turn
  never shows as running. Sidechain (`isSidechain`) lines belong to a sub-agent and are skipped.
  Backgrounded tasks (shell, Monitor, sub-agent) stay with the existing `background` list.
- Kind and label per tool (unknown tools are `other` with the tool name as label):

  | Kind | Claude tools (label source) | Codex tools |
  | --- | --- | --- |
  | `shell` | Bash (`command`) | `exec_command` (`cmd`) |
  | `agent` | Agent, Task (`description`, else `subagent_type`) | none today |
  | `search` | Grep, Glob (`pattern`), WebSearch (`query`), WebFetch (`url`) | `web_search` (`query`) |

  The implementer checks the real tool names against the transcript readers and fills any gap with
  `other`.
- Each transcript reader fills the field: the Claude reader from `tool_use` entries without a
  `tool_result`; the Codex reader from `function_call` and `custom_tool_call` entries without an
  output. The field shape is shared.
- The chat response gains `current` (the newest unmatched call: `kind`, `label`) and `others` (older
  unmatched calls, same shape, newest first). Both are empty when nothing is in flight. API types
  are regenerated with tygo; type names are the implementer's choice.
- Kinds: `shell`, `agent`, `search`, `other`. `other` carries the tool name as its label.
- The server cuts the label to 60 characters with an ellipsis, from the first line of a command. The
  popover shows the same capped labels.
- The Working row component renders the current tool and, when `others` is not empty, a count button
  that opens a popover on hover, focus or tap. The popover must work by keyboard and touch. Popover
  markup is the implementer's choice.
- A backgrounded shell already has a result, so it is not in flight and never shows twice.
- Row order: step, title, current tool, then "waiting on" background tasks.

### Screen modal [T02]

- The keys endpoint (`POST /v1/sessions/keys`) gains an opt-in request flag. When set, after sending
  the key or text the server reads the screen every 100 ms until two consecutive reads match, up to
  about 1.5 s, and returns the settled rows in the response. The browser does no timing.
- The settle covers every session the keys route accepts. That route dials only a holder
  (`holder.DialHolder`) and refuses Queue agents, so these are holder-backed Claude and Codex
  sessions. The settle reads the holder screen through the same `liveRows` helper the screen route
  uses (`server/sessions/chat/screen.go`); the keys route reads no screen today.
- The settle runs after `Watches.MetaMu` is released, so it never blocks status updates or other
  session routes. It is ignored for `only_blocked` and `permission` requests, which keep today's
  behavior. The settled rows go in a new optional `SessionResult.rows` field; types are regenerated
  with tygo.
- The modal's key buttons and Type text set the flag. Chat quick keys and the CLI do not, and keep
  the fast reply.
- If a settle read fails or hits the cap, the response still succeeds and carries the last rows
  read. The modal shows them. Manual Refresh stays and does one plain read.
- The modal compares the returned rows with the previous read and marks changed rows. The marks fade
  after a few seconds (duration is the implementer's choice). When the settled rows equal the old
  ones the modal says "Screen unchanged".

### Vim-mode launch check [T03, T04]

- One shared vim-mode check is called by all three Claude launch paths: the native PTY launcher
  (`nativerunner`), the herdr runner, and the web session first prompt (`firstPrompt.deliver` in
  `server/sessions/prompt/firstprompt.go`). It runs after Ready and before the prompt is typed. Codex
  and the headless runner skip it (Codex is unaffected by the hang; the headless runner has no editor).
- The check builds on the existing `harness` footer helpers (`harness/vimprobe.go`: `footerHasInsert`,
  `settledRows`) and runs only when the launch applied the `editorMode: normal` override
  (`Meta.EditorModeNormal`). `ReadyToType` keeps its mid-session `/config` toggle behavior; today's
  probe treats `-- INSERT --` as good news there, and this launch check is separate from it.
- The check reads the screen with a settled read (two equal reads, as `settledRows` does) so a
  half-drawn frame is not misread. It trips on either vim footer (`-- INSERT --` or `-- NORMAL --`).
  If the screen never settles the check counts as passed and logs a line.
- On a trip: log an event, relaunch Claude once, then continue the launch as usual. Who relaunches:
  the check returns a typed error from the prompt step, and the caller that already owns stop and
  relaunch does the single relaunch. That is the supervisor launch loop
  (`ralphloop/supervise/startandprompt.go`) for Queue agents on either runner, and the session manager's
  existing restart path for web sessions. A relaunch is a new session id, Meta and transcript.
- If a footer is still shown after the relaunch: log the event again, type the prompt anyway and rely
  on the hung-pane recovery. The vim check relaunches at most once per launch. It adds no pause kind
  and no new way into an existing one: a pane that also hangs already ends in the existing
  `agent_prompt_stalled` launch failure today.
- The existing launch with `editorMode: normal` stays; the check makes it a verified invariant
  rather than an assumed one. A unit test also asserts the final claude argv carries exactly one
  `--settings` with `editorMode: normal`.
- Event name and fields are the implementer's choice.
- No bracketed-paste or typing change: gx keeps typing the prompt as one plain write and Enter as a
  separate write, and keeps proving delivery from the transcript. Reconsider only if a vim-mode hang
  is ever reproduced.
- The hung-pane recovery (`gx:agent-launch-and-recovery-impl` 03 to 05, `HungPaneError` in
  `agentrunner/claudeui/nudge.go`) is already on trunk. This epic does not change it.

## Testing Decisions

A good test checks external behavior: what the response carries, what the modal shows, which
launches happen. It does not check internal calls or timing constants beyond the contract.

- **Working row, Go.** Transcript fixtures for Claude and Codex with unmatched calls, matched calls
  and a backgrounded shell. Assert `current`, `others` order, kinds, the 60-character cap and the
  empty case. Prior art: the existing chat snapshot tests.
- **Working row, vitest.** The row renders step, title, current tool and background tasks in order;
  the count button opens its popover on click and focus; empty state reads "Working".
- **Working row, in-flight scope.** Go fixtures: an unmatched call from an earlier turn is not
  shown, a sidechain call is not shown, a tool-name table case per kind.
- **Keys settle, Go.** A fake holder whose screen changes on the second read: the opt-in request
  returns the new rows in `SessionResult.rows`; a request without the flag returns fast; a read error
  or a never-settling screen still succeeds with the last rows; `only_blocked` and `permission`
  requests ignore the flag; the settle runs with `MetaMu` released.
- **Screen modal, vitest.** A fake poster returns new rows: changed rows are marked, marks fade,
  equal rows show "Screen unchanged", Refresh does one plain read.
- **Vim-mode check, Go.** New fake-Claude knobs "vim wedge" (draws a vim footer and stops taking
  keys) and "ignore the editor-mode override", added as `GX_AGENTFAKE_TUI_*` env knobs beside
  `TUIClearIgnoredEnv` and `TUIClearLeavesIEnv` in `testutil/agentfake/tui_control.go`. Cases, each on
  all three launch paths: no footer means one launch and the prompt typed; a footer once means one
  relaunch then the prompt typed; a footer after the relaunch means two logged events, the prompt
  typed anyway and no further vim relaunch. Codex launch shows no check. The argv unit test above.
- **Fallback test.** A launch where the footer persists and the pane hangs ends in the existing
  hung-pane recovery (the `hung-pane-*` fixture scenarios in `testutil/gxfixture/scenarios.go` are
  prior art).
- **E2E.** Existing catalog flows cover the Screen modal key bar, commands and permission dialogs
  (see Verification). The Working row has no catalog flow; one is written or an existing chat flow
  is extended. The `gx-playwright-e2es` flow budget applies.

## Verification

Run against a temporary gx server after the epic is built. Catalog flows are in the flow catalog
(`webui/e2e/CATALOG.md`).

1. Design chat names the tool. New or extended flow (see "New flows" below): a design chat fixture
   with a running tool call; the Working row reads "running shell: ...".
2. Agents tab chat names the tool. Same new or extended flow, on the Agents tab chat.
3. Shell wording. Same flow, shell fixture: "running shell: <command>".
4. Sub-agent wording. Same flow, sub-agent fixture: "waiting for sub-agent: <label>". Go test for
   kind mapping.
5. Search wording. Same flow, search fixture: "searching: <label>". Go test for kind mapping.
6. Other tool. Go test: `other` carries the tool name as label.
7. Label cap. Go test: a command over 60 characters on several lines is cut to the first line, 60
   characters, with an ellipsis.
8. Newest is current. Go test: three unmatched calls, newest is `current`, `others` newest first.
9. Count. New or extended flow: two or more calls in flight show "(+N more)".
10. Popover. Same flow: the count button opens a popover listing the other calls as kind and label.
11. Phone tap. Same flow at 390px: tap opens the popover.
12. Keyboard. Same flow: Tab to the count button opens the popover.
13. Row order. vitest on the row; the new flow asserts the visible order with a background task.
14. No double show. Go test: a backgrounded shell with a result is not in `current` or `others`.
15. Empty state. New or extended flow: between tools the row reads "Working".
16. Codex. Go test with a Codex transcript fixture; the new flow's Codex session variant if the
    scenario holds one.
17. Key button settle. Catalog flow `agents-keys` 156 ("Key bar and Type field in the screen view")
    and 160 (phone), extended to assert the screen shows the redrawn rows with no Refresh click.
    Also `agents-permission` 157 and 158, whose dialogs change on a key.
18. Type text settle. `agents-keys` 156 and `agents-commands-send` 153 and 154, which use Type;
    assert the settled screen appears without Refresh.
19. Changed rows marked. Extend `agents-keys` 156: after a key that changes the screen, changed rows
    carry the mark. vitest for the comparison.
20. Marks fade. vitest with fake timers; a browser check is not needed.
21. "Screen unchanged". Extend `agents-keys` 156: a key that does not change the fixture screen shows
    "Screen unchanged".
22. Refresh. `agents-keys` 156: Refresh still does one plain read.
23. Failure and cap. Go test with a failing and a never-settling fake holder; response still
    succeeds with last rows.
24. Fast replies. CLI check: `gx` keys send without the flag returns without waiting; Go test
    asserts the response has no settled rows. Chat quick keys in `agents-commands-send` 153 still
    pass.
25. Holder-backed sessions. Go test with a fake holder for a Claude and a Codex session through the
    keys route; the settle reads the holder screen through `liveRows`.
26. Check before the prompt. Go test with the fake-Claude "vim wedge" knob: the prompt is not typed
    into the wedged first Claude.
27. All three launch paths. The same Go test table runs on the native PTY launcher, the herdr runner
    and the web session first prompt (`firstPrompt.deliver`).
28. Both footers. Go test cases for `-- INSERT --` and `-- NORMAL --`.
29. Log and relaunch once. Go test: one event, two Claude launches, then the prompt is typed. Check
    the event in the server log of a temporary server started with the "ignore the editor-mode
    override" knob.
30. After Ready, before typing. Go test asserts order: Ready, check, relaunch, prompt.
31. At most one vim relaunch. Go test: footer on every launch yields exactly one vim relaunch (two
    launches) from the vim check.
32. Persistent footer. Go test: two events logged, prompt typed anyway. The follow-on recovery is the
    fallback test, using the `hung-pane-*` fixture scenarios.
33. No new pause. Go test: the persistent-footer case adds no pause kind. With a hanging pane too, it
    ends in the existing `agent_prompt_stalled` launch failure, ended by the Retry button on the parked
    ticket.
34. Codex untouched. Go test: a Codex launch triggers no check and no relaunch.
35. Healthy path. Go test: no footer yields one launch and no event, with one settled read.

New flows or scenarios to write (become implementation tickets):

- A Working row flow (new, or an extension of an existing chat flow) with a scenario holding a
  session with one running shell call, and one with several calls in flight including a sub-agent
  and a search, at desktop and 390px.
- Extensions to `agents-keys` 156 for settled rows, changed-row marks and "Screen unchanged"; the
  fixture holder must change its screen a moment after a key.
- Fake-Claude knobs "vim wedge" and "ignore the editor-mode override" for the Go launch tests.

## Out of Scope

- Reproducing the Claude hang or naming the exact trigger inside Claude.
- Any change to the hung-pane recovery itself (`gx:agent-launch-and-recovery-impl` 03 to 05, on trunk).
- A new pause kind, or any queue or Agents tab status for the vim-mode check.
- Bracketed paste or other changes to how the launch prompt is typed.
- A vim-mode check for Codex.
- Browser-side polling or timing for the Screen modal.
- Settling for chat quick keys or the CLI.
- Making the keys route and the Screen modal work for herdr (Queue agent) panes. The settle covers
  holder-backed sessions only.

## Further Notes

- Evidence for change 3 [T03]: all 17 hung panes out of 1,294 `raw.log` files showed the `-- INSERT
  --` footer. That is 17 of about 193 vim-mode sessions and 0 of about 1,100 without it, including
  219 launches on the same Claude build (v2.1.316). The slash menu was not drawn in any hung screen,
  and the launches were 40 to 90 seconds apart, so it was not a simultaneous burst.
- `editorMode: normal` shipped earlier (`agent-lifecycle-impl/13`). No hang has appeared since it
  took effect on 2026-11-02. Time is a confound: this is strong correlation, not proof.
- Reproduction failed: 24 scripted vim-mode launches on Claude 2.1.319 (sequential and 3 at a time)
  all worked. The hung runs were on 2.1.316, which was not tested.
- Because the cause is unproven, the check may never trip in practice. It is cheap, it makes the
  invariant visible in the log, and the fallback stays.
- Implementer's choices: event names and fields, tygo type names, popover markup, mark fade
  duration.
