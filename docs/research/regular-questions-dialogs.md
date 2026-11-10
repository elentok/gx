# Which permission prompts and dialogs gx can read and answer

Ticket `gx:regular-questions/03`. Claims are marked **[run]** (verified by a real run on 2026-11-10, Claude Code 2.1.319 in tmux, Haiku/Sonnet 5.5), **[docs]** (Claude Code docs), **[source]** (gx source in this worktree, or its recorded real captures in `agentrunner/claudeui/testdata/`), **[unverified]**.

AskUserQuestion and Codex `request_user_input` are covered by sibling tickets 01 and 02. They appear here only where they share the pipe.

## 1. How the banner reads and answers a permission prompt today

**Status.** The harness decides `why`, not the screen.
- Claude: the pid status file says `waiting` with `waitingFor`. `"permission prompt"` means `permission`. Anything else (`"input needed"` in all the tests) means `question`. A startup dialog on screen while Starting is `dialog`. An OSC 7501 `blocked kind=...` report overrides the pid file. [source: `harness/claudepid.go` `pidFileStatus`, `programStatus`, `Status`; `harness/claude.go` `StartupScreen`]
- Codex: only OSC 7501 `blocked` (kind `permission`, else `question`), `error` (turn error, `error`), or a startup dialog on screen (`dialog`). [source: `harness/codexwatch.go`]
- The comment on `holder.PermissionPrompt` says the same `waitingFor` covers both tool permission and plan approval. So plan approval reports `why=permission`. [source: `nativerunner/holder/holderanswer.go:18`]
- `SessionFailure{kind:"blocked", why, label}` is built from status alone, so the text is always "Waiting at a prompt in the terminal". Dialog title and options are never on the failure. [source: `server/sessions/chat/failure.go`]

**Read.** `GET /v1/sessions/permission?session=` (`server/sessions/chat/permission.go`). Queue agents are refused. The session must be live and Blocked. Then:
- Codex with a hook request file (`permission-request.json`, written by `gx agent permission-request`): kind `prompt`, text = the shell command or tool name, `request_id` = a UUID. Exact, no screen. [source]
- Otherwise it reads the screen: Codex `codexui.ParseDialog` looks for a line `Blocked by hook` and takes the paragraph under it. Claude `claudeui.ReadDialog` finds the last `Do you want ...?` or `Claude has written up a plan ... ?` row and the numbered options under it. Response kind is `prompt`, `plan` or `unreadable`; `request_id` = sha256(header + body)[:8]. [source]
- The response carries only `kind`, `text`, `request_id`. The option list that `ReadDialog` parses (`Options`, `Cursor`) is dropped before it reaches the wire. [source: `permissionFromScreen`, `wire.SessionPermissionResponse`]

**Answer.** `POST /v1/sessions/keys {permission: allow|deny, request_id, only_blocked}` -> `answerSessionPermission` -> `holder.AnswerSessionPermission` with `StrictAllow` and `UnreadableDeny`. [source: `server/sessions/prompt/keys.go`, `nativerunner/holder/holderanswer.go`]
- Claude Allow: re-reads the screen, refuses with `request-changed` if `request_id` differs, refuses `not-approvable` unless the cursor is on option 1 and option 1 is exactly `Yes`; then sends Enter. Deny: Esc, whatever the screen shows. Then it waits for the session to leave Blocked (`keys-not-delivered` otherwise).
- Codex hooked: writes `permission-decision.json` (allow or deny plus reason). Unhooked: `y` or Esc after matching the parsed dialog id.
- Web sends no deny reason (the Codex path and the Claude `denyWithReason` path support one; the banner does not use it).

**Options the real prompt has that the banner leaves out.** Real Bash prompt, captured [run] and in `testdata/permission-w160.screen.txt` [source]:

```
Do you want to proceed?
 > 1. Yes
   2. Yes, and always allow access to <dir> from this project
   3. Yes, and switch to auto mode
   4. No
```

- Edit prompt: `2. Yes, and switch to accept edits ... for this session (shift+tab)`, `3. No` [source: `testdata/dialog-edit.screen.txt`].
- Read outside the project: `2. Yes, allow reading from outside/ during this session` [source: `testdata/dialog-read.screen.txt`].
- gx reads these labels (`ReadDialog.Options`) but only exposes Allow once (option 1) and Deny (Esc). "Allow always / for this session / switch to auto mode" are not sent by gx and are deliberately excluded by `StrictAllow` so Enter never picks a standing permission. Tab-to-amend is not exposed.
- **Digit keys select an option directly with no Enter** [run: pressing `4` on the dialog above chose No at once and the turn ended "Interrupted"]. gx's `/v1/sessions/keys` key table has no digits (`sessionKeyBytes`: esc, enter, tab, shift+tab, backspace, arrows, ctrl+c), and `text` is typed as a prompt. So picking option N today needs `down` x (N-1) + `enter`, which depends on the cursor's start row (the ❯ cursor is not always on 1), or a new digit key.
- **Wrapped options are cut short** [run]: at 140 columns option 2's path wrapped onto two rows, so the continuation row failed the `^\d+\.` match and `ReadDialog` stopped. Options 3 and 4 were lost. (`ParseDialog` returns an empty Dialog on the same screen.) A gx PTY is often narrower than the real terminal, so this is a real risk.
- **Plan text is not read** [source, not run]: in the plan screen (`testdata/plan-approval.screen.txt`, same layout seen [run]) the plan body sits above a rule that precedes the question. `headerAndBody` takes only rows between the last rule above the question and the question, so Header and Body are empty and the `request_id` is the hash of an empty string for every plan. The card would show no plan and the id cannot tell two plans apart.
- Claude launches with `--permission-mode auto`, so ordinary tool permission prompts are rare in gx sessions; the classifier decides and prompts only for explicit `ask` rules, AskUserQuestion, MCP `requiresUserInteraction`, critical `rm`, and reads outside the dirs [docs: permission-modes]. Plan mode and plan approval still happen when the person (or a skill) enters plan mode with Shift+Tab. Codex runs `--approve-for-me`, so the reviewer asks only on denial or doubt [source: `docs/specs/codex-support.md:260`].

## 2. Dialog table

"Answer path" names how a web card could pick an option from outside. "Needs" is the gap to close.

| Dialog | `why` kind | Readable by gx | Answer path today | Needs for a card |
|---|---|---|---|---|
| Claude tool permission (Bash, Edit, Create, Read, other tools, MCP tools) | `permission` | Yes. Title, body, question and numbered options from the screen (`ReadDialog`). Only Bash command, Edit file, Create file are "approvable"; Read file and unknown tools get `Allow` refused `not-approvable` [source: `approvableHeaders`, `testdata/dialog-unknown.screen.txt`]. Wrapped options truncated [run] | Allow = Enter on option 1 `Yes`; Deny = Esc, via `permission` field | Return options + cursor in the response; digit key (works [run]); fix wrap parsing |
| Claude plan approval (ExitPlanMode) | `permission` (shares waitingFor) | Options yes (`1. Yes, and use auto mode`, `2. Yes, manually approve edits`, `3. Tell Claude what to change` plus a non-numbered `shift+tab ...` sub-row [run]). Plan text no (see above) | Only Close dialog (Esc) in the banner. Esc closes the dialog and stays in plan mode [run]. Allow is refused: option 1 is not plain `Yes` | Read the plan: from the transcript (the plan file is named on the dialog's last row, `~/.claude/plans/<name>.md` [run]) or from the screen above the rule. Option 3 opens a feedback text box (not probed) |
| Claude AskUserQuestion | `question` (waitingFor `input needed`) | Yes, screen: `☐ header`, question, `1. Red 2. Blue 3. Type something.` [source: `testdata/dialog-askuserquestion.screen.txt`]. `ReadDialog` does not parse it (no `Do you want` row) | Esc only; see ticket 01 | Ticket 01 |
| Claude trust-this-folder (startup) | `dialog` (Starting + screen anchors) | Yes, two anchors | gx answers it itself (`TrustKey`: Down then Enter), no person involved [source: `harness/claude.go`] | None; never a card unless auto-answer fails |
| Claude login, onboarding, theme, update, model prompts, "bypass permissions" confirmation | Not classified. Pre-session unknown screen stays `Starting`, then a startup timeout reports it [source: `screenmatch.go` comment `StartupNoSession`]. Mid-session ones (e.g. a model switch confirmation, MCP server approval, `/resume`, `/model` pickers) would report `question` if Claude writes `waiting` | Unknown. No capture exists | None. Screen | Leave to Screen [unverified: which of these set `waitingFor`] |
| Claude MCP elicitation (a server asks the person) | Likely `question` [docs: Notification type `elicitation_dialog`] | Unknown, no capture [unverified] | None | Screen |
| Claude rate limit / auth / error text | `error` or the rate-limit and auth failures, not dialogs | n/a | Existing banner (Retry etc.) | none |
| Codex approval, hook fired | `permission` (OSC 7501 `blocked kind=permission`) | Yes, exact: tool name, shell command, id from the hook file | Allow/Deny via decision file (+ deny reason) [source] | Only two decisions exist; Codex's real prompt options (approve for session, etc.) are not offered by the hook route |
| Codex approval, hook did not fire | `permission` | Partly: title `Blocked by hook` and message. Text is from spec, never captured from real Codex [source: `codexui/dialog.go` comment] | `y` / Esc after id match; Allow refused if unreadable, Deny allowed | Real capture needed |
| Codex `request_user_input` | `question` | Ticket 02 | Ticket 02 | Ticket 02 |
| Codex startup dialogs (trust, hooks review, update, model nudge) | `dialog` | Yes by substring; only trust wording is from real Codex, others are what the fake draws [source: `harness/codex.go` `codexDialogs`] | gx answers Enter itself | None |
| Codex plan mode, collaboration prompts | not started by gx; the guard cannot detect Plan mode [source: spec line 446] | No | None | Screen |
| Codex turn error | `error` | Message from the rollout | Prompt starts the next turn | existing |

Cards that gx can build today with small server changes: Claude and Codex tool permission (options list), Claude plan approval (after fixing the plan text read), and the hooked Codex approval. Everything else stays on Screen.

## 3. Dialogs gx cannot read reliably (leave to Screen)

- Any `question` status other than AskUserQuestion and `request_user_input`: nothing says which dialog it is. `why=question` only means the pid file said `waiting` for a reason that is not `permission prompt`. [source]
- Pre-session login, onboarding and other startup screens (no anchor), MCP elicitation, mid-session pickers, anything with free text or a form.
- Codex unhooked approval until a real capture exists (the `Blocked by hook` text is unverified against real Codex).
- A permission screen with wrapped option rows: today `ReadDialog` keeps only the options before the wrap [run], and `ParseDialog` returns nothing.
- A Claude dialog taller than the visible rows (a long Bash command or diff). `ReadDialog` finds the question row, but the header and body can be scrolled off; the PTY width and height are gx's, not the person's.

## 4. Telling that a dialog closed or changed

What exists:
- Blocked status is the coarse gate: `only_blocked` refuses with `not-blocked` when the session is not Blocked, and a permission answer requires `why=permission` [source: `keys.go`].
- For permission only, `request_id` (hash of header and body) is compared against a fresh screen read before any key goes out: mismatch gives `request-changed` [source]. Codex hooked requests use a per-request UUID, which cannot repeat. After sending, the answer waits for the session to leave Blocked (`awaitNotBlocked` / `WaitTurn`).
- Close dialog (`only_blocked` key) checks Blocked only. It has no id: a different dialog that opened between render and click would also get its Esc. [source]

Gaps for the sheet:
- Claude permission ids ignore the options and the question line, and the hash is of header+body only (deliberately, to survive tip and cwd changes). Two Bash prompts with the same command (a retry) share an id. Plan dialogs hash to the same constant (see above). A stale Allow on a repeated identical command would be accepted, which is still "the same question" but the person did not see this occurrence.
- Nothing carries a "blocked episode" counter. Claude's pid file has a turn counter (`Turn` in the holder status, used by `WaitTurn`); Codex has OSC 7501 `working` count (`Turns()`). A blocked-since marker (turn + status change time) in the question's identity would make "the dialog closed and a new one opened" detectable for every kind, not only ones whose text differs.
- Cards should clear when status leaves Blocked (the failure clears with its condition [source: `SessionFailure` comment]), so a terminal answer through Screen also removes them.

**Recommendation** (cleaner than leaving it per-kind): add one read endpoint (extend `/v1/sessions/permission` or a sibling) that returns `{kind, title, body, options[{n,label}], cursor, id, episode}` for any dialog gx can parse, where `episode` = turn counter + hash of the full screen block (question row + options). Send an answer as `{choice: n, id}`; the server re-reads, refuses on id mismatch, and sends the digit key (verified to select directly [run]) so cursor position no longer matters. Allow-once/deny stay as shortcuts for options 1 and the last. The trade-off against the existing "Allow once only" safety: exposing "always allow" and "switch to auto mode" makes standing grants one tap from a phone, so show them with their label and require the person to pick them (never default), and keep `StrictAllow` for the Allow shortcut.

## Open facts
- Real Codex 0.166.0 approval dialog text and options (no capture; `codexui` text is from the spec). Not probed here.
- Which Claude mid-session dialogs set pid `waitingFor` to something other than `permission prompt` and so report `question` (MCP approval, model picker, `/` pickers).
- What option 3 of plan approval ("Tell Claude what to change") does after a digit press (feedback text box) and whether gx can then type into it.
- Whether the plan text can be read reliably: untested end to end; derived from `headerAndBody` and the fixture. A quick go test on `testdata/plan-approval.screen.txt` would confirm it.
- PermissionRequest hook details for Claude (fields, whether it fires for ExitPlanMode and AskUserQuestion): the hooks reference page was too long to fetch past its first 100000 characters [docs, partial]. The docs do list Notification types `permission_prompt`, `elicitation_dialog`, `agent_needs_input` and PermissionRequest decision `hookSpecificOutput.decision.behavior`. A hook route would give exact tool input for Claude as Codex has, but gx's Claude sessions use screen reading today.
- Whether digit keys select options in every Claude dialog (verified on the Bash permission dialog only) and in Codex dialogs.
