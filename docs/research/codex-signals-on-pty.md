# Codex signals on a PTY (gx:codex-support/01)

What gx can read from, and send to, a Codex session that a PTY holder hosts. Tested against real Codex. Three ways in were compared, plus two the experiments turned up. Each claim is marked.

- **[O147]** observed on codex-cli 0.147.0.
- **[O166]** observed on codex-cli 0.166.0.
- **[S]** read from the openai/codex source at tag `rust-v0.147.0` or `rust-v0.166.0` (`codex-rs/...`), not run.

## Scope and method

- **The version moved during the run.** The `codex` on PATH was 0.147.0 until about 10:29 local time on 2026-10-30. Something outside the run then upgraded it to 0.166.0 (the user's `~/.codex/packages/app-server-daemon` already ran 0.166.0). I installed 0.147.0 with `npm i @openai/codex@0.147.0` into a scratch directory and re-ran the version-sensitive checks on both. Checks marked only [O166] were not re-run on 0.147.0.
- **Rig.** A private `gx agent-holder` per run (the real holder: same PTY, same `charmbracelet/x/vt` screen, same `holder.sock` ops, `raw.log` for the raw byte stream), driven by a Python client of `holder.sock`. No herdr, no tmux, no existing session touched.
- **Isolation.** `CODEX_HOME` pointed at a temp directory holding a copy of `auth.json`, a minimal `config.toml` and a test `hooks.json`. The real `~/.codex` was never written. `auth.json` was not printed. The auth copy was deleted at the end. `last_refresh` in both copies matched afterwards, so no token rotated.
- **Repo.** A throwaway git repo under `$TMPDIR`. Model `gpt-5.6-luna`, `model_reasoning_effort="low"`, one-line prompts. All Codex processes I started were stopped.
- **Observers.** Every run recorded the screen, the OSC stream in `raw.log`, a hook logger (all 11 events, microsecond timestamps), and rollout file growth. Latencies below come from these timelines and include about 100 ms of poll delay.
- **Not used.** `--dangerously-bypass-hook-trust` ran in two early runs. The sandbox then refused it as a safety bypass, so every later run trusted the hooks through Codex's own review dialog. Its behaviour for inline hooks is [S] only.
- **Live failures happened on their own.** Around 10:28 the account returned `429 Too Many Requests` and `Selected model is at capacity` for `gpt-5.6-luna` for a few minutes. These are real failures, so they are used below.

## Answers in short

| Question | Answer |
| :- | :- |
| Turn state | OSC 0 title and OSC 7501 in the raw PTY stream. Title works on 0.147.0 and 0.166.0. OSC 7501 is new (absent in 0.147.0, present in 0.166.0). Both flip at the turn end, in the same millisecond as the rollout's `task_complete`. The screen's `esc to interrupt` hint does not: it disappears when the answer starts streaming, up to seconds before the turn ends. |
| Blocked | Title `[ ! ] Action Required`, plus OSC 7501 `blocked` with `kind=permission` or `kind=question` on 0.166.0. Hooks give the structured text (`PermissionRequest`, `PreToolUse` for `request_user_input`) 50 to 150 ms before the dialog draws. Four different startup dialogs also exist and none of them moves the title or OSC 7501 off "ready". |
| Input | Set `disable_paste_burst=true`, or an Enter within a few ms of typed text becomes a newline. Enter while busy steers the running turn. Tab queues. `/` and `$` open popups. Ctrl+C clears the whole composer, Ctrl+U clears one line, and Ctrl+C on an empty idle composer quits Codex. |
| Session id | The title shows the id (first 29 characters) from launch, before any prompt. The `SessionStart` hook gives the full id and the rollout path, but only at the first prompt. |
| Resume | `codex resume <id> --cd <dir>` works from a recreated worktree and skips the working-directory dialog. The session keeps its instructions without the flag. |
| Answering | Yes, three ways. A `PermissionRequest` hook can hold the dialog and return allow or deny with a message the model sees. Keys (`y`, `Esc`, digits) work. An app-server second client can answer both approvals and `request_user_input`. |
| Tool control | `PreToolUse` blocks `Bash`, `apply_patch`, MCP and other tools with a reason. `developer_instructions` adds a system prompt. |
| Compaction | `PreCompact` and `PostCompact` hooks with `trigger` of `manual` or `auto`, a `compacted` rollout item, and title/OSC 7501 `working` while it runs. |
| Rate limits and failures | Rollout `token_count.rate_limits` and `task_complete.error`. A failed turn fires no `Stop` hook. OSC 7501 says `error`. A network stall shows only on screen. |

## 1. Turn state

### Signals seen during one turn

Times are seconds from the Enter key. "Hook" rows are when the hook script ran.

| Event | 0.147.0 run | 0.166.0 run | Source of the signal |
| :- | :- | :- | :- |
| Enter pressed | 0.00 | 0.00 | gx |
| Title `Working`, OSC 7501 `working`, rollout `task_started` | +0.13 to +0.4 | +0.15 to +0.4 | PTY stream, rollout |
| `SessionStart` (first prompt only) and `UserPromptSubmit` hooks | +0.15 to +0.3 after Working | same | hook |
| `esc to interrupt` leaves the screen | answer starts streaming | same | screen |
| `Stop` hook | 60 to 70 ms before Ready | 140 ms before Ready | hook |
| Title `Ready`, OSC 7501 `done`, rollout `task_complete` | same instant | same instant | PTY stream, rollout |

- [O147] A 25-line story: the screen hint vanished at +2.4 s, the title stayed `Working`, and the title went `Ready` at +4.7 s. The hint is not a turn-over signal. Matching `esc to interrupt` would report idle 2.3 s early, and for long answers (the earlier herdr note saw 5 to 18 s) much more.
- [O166] OSC 7501 follows the same turn: `idle` at launch, `working`, `done` at the end. `done` is followed by `idle` before the next `working` (`acknowledge_program_status`, [S] `chatwidget/status_surfaces.rs`). An interrupt gives `idle`, not `done`. A failed turn gives `error` ([O166] with a bad API key).
- [O166] OSC 7501 stays `working` while a background terminal runs. After Esc during `sleep 25`, the title said `Ready` 0.15 s later, and OSC 7501 stayed `working` until the sleep ended (27.5 s), then went `idle`. So "title Ready and OSC 7501 working" is the Codex form of gx's `Background` flag. The screen shows `1 background terminal running · /ps to view · /stop to close`. [S] `desired_program_status` counts `unified_exec_processes`.
- [O147] Startup: the title is `Starting` while MCP servers boot, then `Ready`. [O166] OSC 7501 reports `working` during `Starting`, then `idle`.

### The title

The title is the TUI's own, set with `tui.terminal_title` (in the published `config.schema.json` of both versions). It is the only signal in both versions that needs no file and no screen. With no setting it shows the project name and a spinner glyph while working ([O147]: `repo` idle, `⠸ repo` working). gx should set the items so the text is stable:

```
-c 'tui.terminal_title=["spinner","status","session-id","context-used","five-hour-limit","weekly-limit"]'
```

[O147, O166] That produced, for example:

- `Ready | 01a18bf0-2b28-73c1-922c-519d4... | Context 1% used | weekly 99% left`
- `⠙ Working | 01a18bf0-...`, `⠸ Thinking | ...`, `⠋ Starting | ...`
- `[ ! ] Action Required | ...`, alternating with `[ . ] Action Required` about every second.

The `status` text is `Starting`, `Ready`, `Working`, `Thinking` or `Waiting` ([S] `run_state_status_text`). With `spinner` in the list, a blocked dialog replaces the spinner and status with the `Action Required` prefix, and `status` is dropped.

### Hooks as a turn signal

`UserPromptSubmit` and `Stop` carry `turn_id` and `last_assistant_message`. Limits:

- [O147, O166] `Stop` does not fire when the turn is interrupted (Esc or Ctrl+C) or fails (429, overloaded, 401). A waiter that needs `Stop` would hang.
- [O166] Enter while busy fires `UserPromptSubmit` again, inside the same turn, so it is not a turn counter.
- [O147] `SessionStart` fires at the first prompt, not at launch (see section 4).

### Rollout file

[O147, O166] `event_msg` `task_started`, `task_complete` (carries `error`), `turn_aborted`, `token_count`, and a `compacted` item. They arrive within the same 100 ms as the title change. The path comes from the `SessionStart` hook (`transcript_path`). gx already reads this format (`codexsession`), and the format is internal (see section 11).

## 2. Blocked

### Dialogs seen

| Dialog | Text to match | Seen on | How gx can tell it from the rest | Answer |
| :- | :- | :- | :- | :- |
| Command approval | `Would you like to run the following command?`, `Reason: ...`, `$ <command>`, options `1. Yes, proceed (y)`, `2. Yes, and don't ask again for commands that start with ... (p)`, `3. No, and tell Codex what to do differently (esc)`, `Press enter to confirm or esc to cancel` | 147, 166 | Title `Action Required`. 0.166.0: OSC 7501 `blocked` `kind=permission`. `PermissionRequest` hook with `tool_name` and `tool_input.command`. | `y`, `Esc`. Avoid `p` (writes an exec-policy rule). |
| `request_user_input` | `Question 1/1 (1 unanswered)`, the question, numbered options plus `None of the above`, `tab to add notes \| enter to submit answer \| esc to interrupt` | 147, 166 | Title `Action Required`. 0.166.0: OSC 7501 `blocked` `kind=question`. `PreToolUse` hook with `tool_name` `request_user_input` and the full `questions` array. | A digit picks and submits. Details in section 6. |
| Trust directory | `Do you trust the contents of this directory?` ... `1. Yes, continue` | 147 | Title has no session id yet (see below). | `Enter` |
| Hooks need review | `Hooks need review`, `1. Review hooks`, `2. Trust all and continue`, `3. Continue without trusting` | 147, 166 | Same. Appears when a hook is new or changed. | `2` then `Enter` writes `trusted_hash` entries to the user `config.toml`. |
| New-model nudge | `Meet GPT-6 Luna`, `1. Try new model`, `2. Use existing model` | 166 | Same. Shown a limited number of times per model; `2` wrote `[notice.model_migrations]` to the config. | `2` then `Enter` |
| Resume directory | `Working directory · resume`, `1. Use session directory`, `2. Use current directory`, ... | 166 | Same. Only when the cwd differs. | Avoid with `--cd` (section 5). |

- [O147] `request_user_input` is only available in Plan mode. In Default mode it needs `--enable default_mode_request_user_input`, an under-development feature that prints a warning banner. [S] `tools/src/tool_config.rs`. I did not test the Default-mode call without the flag.
- Not seen: the approval dialog for `apply_patch` and the MCP elicitation dialog. [S] they exist (`approval_overlay.rs`, `mcp_server_elicitation.rs`) and map to `kind=permission` and `kind=question`.
- Not run: the update-available dialog (I set `check_for_update_on_startup=false`) and the retired-model migration prompt (`model_migration.rs`).

### Startup dialogs hide behind "ready"

[O166] (one run) While a pre-session dialog is up, the title reads `Ready | Context 0% used` and OSC 7501 says `idle`. Both look ready for a prompt. They differ from the real ready state in one way: the title has no session id until the session starts. After the dialogs, the title becomes `Ready | 01a18bfe-2262-... ` (id present), then `Starting`, then `Ready` again with OSC 7501 `idle`. So gx's ready signal for Codex is "title carries a session id, status `Ready`". If the id never appears, gx classifies the screen (as `StartupScreen` does for Claude) to answer the dialog.

[O147] Typing a prompt while the Hooks-review dialog was up made Enter choose "Review hooks". A prompt sent too early is lost or misrouted, as with the trust dialog.

## 3. Input

- **Typing and Enter.** [O166] With default settings, typing `Reply with just: g0` and Enter with no gap did not submit. Enter became a newline, and the text was submitted later joined to the next paste. A 50 ms gap submitted. This is Codex's paste-burst detection. [O166] With `-c disable_paste_burst=true`, a 0 ms gap submitted every time. The key is in `config.schema.json` of 0.147.0 and 0.166.0 (`tui.disable_paste_burst`, with the top-level form as the older spelling). gx's `TypeText` followed by `\r` adds no deliberate gap below 512 characters, so this flag is needed.
- **Long text.** [O166] A 2185-character paste (the holder's bracketed paste) showed as `[Pasted Content 2185 chars]` in the composer and reached the `UserPromptSubmit` hook byte for byte. A composer-empty check that reads the dim placeholder will see that text as a draft.
- **Busy composer.** [O166] Enter while a turn runs steers the running turn: the text joined the same turn, `UserPromptSubmit` fired again, the answer covered both, and `Stop` fired once. Tab queues it (`tab to queue message` under the composer, then `Queued follow-up inputs`). gx's rule of waiting for not-busy before typing (`awaitNotBusy`) still fits. Typing a prompt into a busy Codex must be avoided, not queued.
- **`/` prefix.** [O166] Typing `/com` opens a command popup (`/compact  summarize conversation to prevent hitting the context limit`). `/compact`, `/status`, `/new` and `/model` run as in the earlier herdr note. `/status` prints the full session id, context window use and the weekly limit. [O166] A prompt that starts with `/` but is a path (`/tmp/foo is a path. Reply with just: path`) was a normal prompt.
- **`$` prefix.** [O166] Typing `$gxprobe` opens a skill popup (`gxprobe [Skill] ...  enter insert · esc close`). The first Enter inserts the skill and the composer keeps `$gxprobe`. A second Enter submitted it (the hook saw the prompt `$gxprobe`). I did not test `$name` followed by more text.
- **Interrupt.**
  - [O147, O166] Esc during a tool call or reasoning: title `Ready` after 0.09 to 0.15 s, rollout `turn_aborted`, screen `■ Conversation interrupted`. 0.166.0 also reports OSC 7501 `idle`. The cancelled prompt is not restored to the composer.
  - [O166] One Esc with the hint absent from the screen (answer streaming) interrupted too. The earlier herdr note ([O147], 5 of 5) saw Esc ignored while the answer streams. I did not repeat that on 0.147.0, so gx should keep Ctrl+C as the busy key and decide by title or OSC 7501, never by the screen hint.
  - [O166] An interrupted tool keeps running as a background terminal. `/stop` closes it ([S] screen hint only; not run).
- **Clearing the composer.** [O166] Ctrl+C with text clears the whole composer, multi-line too, and Codex stays up. Ctrl+U removes only the current line (3 pasted lines became 2). [O166] One Ctrl+C on an empty idle composer quit Codex at once: the process and the holder were gone within 1.5 s. gx must never send Ctrl+C or a second Ctrl+C unless the state says busy.

## 4. Session id

Codex cannot take an id at launch.

| Source | When it has the id | What it gives | Notes |
| :- | :- | :- | :- |
| Title `session-id` item | At launch, before any prompt [O147, O166] | First 29 characters (the item is cut to 32 with `...`) | A UUIDv7 prefix of 29 characters is enough to match a rollout file. |
| `SessionStart` hook | First prompt, not launch [O147]. Also `source` `resume` [O147, O166] and `compact` [O166]. | Full `session_id`, `transcript_path`, `model`, `source` | Rollout path is in the payload. Hook `env` is Codex's env, so `GX_AGENT_SESSION` reaches the hook [O147]. |
| Rollout file and `threads` table in `state_5.sqlite` | First prompt only. The rollout file and the row did not exist after launch [O166]. | Full id, path, cwd | Internal formats. |
| `/status` | On demand | Full id (`Session: 01a18bf7-...`) | Types into the composer; costs a screen read. |
| Environment of a tool call | Never | `CODEX_HOME`, `CODEX_VERSION`, `CODEX_CI` are set; no `CODEX_THREAD_ID` [O166] | Not usable. |

- A Codex that exits before its first prompt leaves no rollout, so there is nothing to resume ([S]: app-server `thread/resume` also answered `no rollout found for thread id` before the first prompt [O166]).
- The mapping gx needs is: title prefix at launch, then the full id and rollout path when the hook arrives. The hook has no gx session id, so gx matches by `GX_AGENT_SESSION` in its env, which it already sets on the holder child.

## 5. Resume

- [O166] The holder rig started `codex resume <id>` in a copy of the repo at a different path. With the default settings, a blocking `Working directory · resume` dialog appeared and swallowed the typed prompt.
- [O147, O166] `codex resume <id> --cd <new dir>` skipped the dialog, kept the same session id and appended to the same rollout file. [S] `effective_resume_cwd_mode`: `--cd` forces the "current" mode. `tui.resume_cwd="current"` does the same on 0.166.0 and is in the 0.147.0 schema, but I only ran `--cd` on both versions.
- [O166] The resumed session kept its instructions. The first run used `-c developer_instructions="The secret word is PINEAPPLE..."`. The resume had no such flag and still answered `PINEAPPLE`. I did not test passing the flag again, so I cannot say whether it would duplicate the instruction.
- [O147, O166] On resume, `SessionStart` fires with `source` `resume` at the first prompt, and the title shows the id at launch.
- "Does a reply reach a live session?" After resume, a prompt typed into the PTY reached the model and was answered in both versions. I did not test two Codex processes on one session.

## 6. Answering from outside the TUI

### Approvals

| Way | Observed | What the model sees | Cost |
| :- | :- | :- | :- |
| Keys `y` | [O147, O166] Approves at once, no Enter. Title `Working` 0.1 s later. | Command output | Reads the dialog from the screen to know what is approved. |
| Key `Esc` | [O147, O166] Denies and ends the turn (`turn_aborted`). | `You canceled the request`, turn over | A reason needs a second prompt after the turn ends. |
| `PermissionRequest` hook, allow | [O147] `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}` ran the command with no dialog. | Command output | Needs the hook trusted. |
| `PermissionRequest` hook, deny with message | [O147] The command did not run, the turn continued, the TUI showed `PermissionRequest hook (blocked) feedback: <message>`. | The tool error `Rejected("<message>")` | Same. |
| `PermissionRequest` hook, hold | [O147] A hook that waited 5 s on a control file kept the TUI on `Working` with no dialog. Releasing it with allow ran the command. | n/a | The terminal user sees no dialog while it is held. |
| App-server second client | [O166] Reply `{"decision":"accept"}` to `item/commandExecution/requestApproval`: resolved in 0.26 s, the TUI dialog closed, the command ran. | Command output | Needs `--remote` and an app-server (section 10). The response has `accept`, `acceptForSession`, `decline` (agent continues), `cancel` (turn interrupted). No free-text reason field. |

- [S] `permission_request.rs`: any deny wins over an allow. A hook that exits 0 with no output, exits non-zero other than 2, times out, or prints invalid JSON gives no verdict and the normal dialog appears. [O147] An empty-output hook was followed by the dialog. A hook that exits 2 with text on stderr is a deny.
- The deny-with-message path fixes a weakness of the Claude flow. Esc ends the turn and the reason needs a second prompt. The hook denies, keeps the turn alive and hands the reason straight to the model.

### `request_user_input`

- [O147, O166] The dialog takes keys. `1` or `2` picks that option and submits at once. `3` (the last option, "None of the above") submits at once with no note. To add text, move to the option, press `Tab`, type, press `Enter`: `Down Down`, `Tab`, `Because it is nice`, `Enter` gave `{"answers":{"preferred_color":{"answers":["None of the above","user_note: Because it is nice"]}}}`.
- [O147] The `PostToolUse` hook carried the answer in `tool_response`. `PreToolUse` carried the questions (`id`, `header`, `question`, `options[{label,description}]`) 150 ms before the dialog. That hook cannot answer. Only a `deny` with a reason would reach the model, and it would count as a blocked tool. I did not use it.
- [O166] A second app-server client received the server request `item/tool/requestUserInput` with the same questions, replied `{"answers":{"<id>":{"answers":["Blue"]}}}`, and the dialog closed in 0.33 s.
- Not tested: a request with more than one question (the dialog says `Question 1/N`).

## 7. Tool control and system prompt

- **Hook events.** The names are `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PreCompact`, `PostCompact`, `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `SubagentStart`, `SubagentStop`, `Stop` ([S] `hooks/src/lib.rs`, JSON schemas in `hooks/schema/generated`). `PreToolUse` and `PermissionRequest` can block. The others observe, or add context.
- **Guard role.** [O166] A `PreToolUse` hook returning `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"GX-GUARD: ..."}}` blocked both `apply_patch` and `Bash`. The TUI printed `Blocked by hook` with the reason, and the model quoted it. `tool_input.command` has the command text, so gx can match `git push` and the other guard-bash patterns. [S] Exit code 2 with stderr also blocks. [S] Matchers are tool names; `apply_patch` also matches `Write` and `Edit`, `spawn_agent` matches `Agent`, and MCP tools are `mcp__<server>__<tool>`.
- **Coverage gap.** [S] `write_stdin` (input to a running command) has no `PreToolUse` payload. [O147] `request_user_input` does.
- **System prompt.** `-c developer_instructions="..."` adds a developer-role message ([S] `config_toml.rs`, "inserted as a `developer` role message"). [O166] It worked and survived resume. `model_instructions_file` replaces the built-in instructions, and Codex's own comment discourages it. `AGENTS.md` in the repo or `CODEX_HOME` is read as project docs (not tested here). `instructions` is the older system field (not tested).
- **Disabling a tool.** [S] `tools.experimental_request_user_input.enabled=false` is the switch for that tool. The wider disallowed-tools mapping belongs to `gx:codex-support/05`.
- **Hook trust.** Hooks come from `hooks.json` next to `config.toml`, from `[hooks]` in `config.toml`, or inline with `-c` ([O166] `-c 'hooks.Stop=[{hooks=[{type="command",command="...",timeout=10}]}]'` ran). Unless the hook is trusted, Codex shows `Hooks need review` at start.
  - Trust is a `trusted_hash` per hook, stored in the user `config.toml` under `[hooks.state."<file>:<event>:<group>:<index>"]`. Inline hooks use the key `/<session-flags>/config.toml:stop:0:0`. The hash covers the command string and timeout, so any change re-asks.
  - [S] `--dangerously-bypass-hook-trust` runs enabled hooks without stored trust for one launch (`hooks/src/engine/discovery.rs`). Its help text says "Intended only for automation that already vets hook sources", which is gx's case. It prints `⚠ --dangerously-bypass-hook-trust is enabled` in the transcript [O147, two early runs].
  - Hooks run in Codex's process environment, so `GX_AGENT_SESSION` and `CODEX_HOME` are inherited [O147].

## 8. Compaction

- [O166] `/compact`: `PreCompact` (trigger `manual`) at +0.5 s, title `Working`, OSC 7501 `working`, rollout `task_started`; `PostCompact` +2 s later; rollout `compacted`; then title `Ready`, OSC 7501 `done`, `task_complete`. The screen shows `• Context compacted · 2s`.
- [O166] Auto compaction (`-c model_auto_compact_token_limit=4000`) ran before the next prompt: `PreCompact` and `PostCompact` with `trigger` `auto`, then `SessionStart` with `source` `compact`, then `UserPromptSubmit`. Title and OSC 7501 showed `working` for the 2 s of compaction. The prompt then ran normally and the turn ended with `Stop`.
- The title's `context-used` item gives `Context N% used` of the model window (258400 tokens for `gpt-5.6-luna`, [O166] in `task_started.model_context_window`), refreshed per turn. That is the input gx's smart zone needs. [S] `status_line_value_for_item(ContextUsed)`.
- No signal for "context is nearly full" exists except that percentage. The out-of-room error (`ContextWindowExceeded`) is a failed turn (section 9).

## 9. Rate limits and failures

| Case | Seen | Where it appears |
| :- | :- | :- |
| Usage data | [O147, O166] | Rollout `event_msg` `token_count` with `rate_limits{limit_id, primary{used_percent, window_minutes, resets_at}, secondary, credits, plan_type, rate_limit_reached_type}`. The title items `five-hour-limit` and `weekly-limit` show `weekly 99% left`. `/status` prints `Weekly limit: [...] 99% left (resets 10:50 on 5 Nov)`. This account's `primary` was the weekly window (`window_minutes` 10080) and `secondary` was null, so gx must key windows by `window_minutes`. App-server sends `account/rateLimits/updated` ([S]). |
| 429 | [O147] real | Screen `■ exceeded retry limit, last status: 429 Too Many Requests`. Rollout `task_complete.error` = `{message, codex_error_info:{response_too_many_failed_attempts:{http_status_code:429}}}`, duration 564 ms. No `Stop` hook. |
| Model at capacity | [O147] real | `task_complete.error.codex_error_info` = `"server_overloaded"`, message `Selected model is at capacity. Please try a different model.` No `Stop` hook. |
| Auth (bad API key) | [O166] | After about 16 s of retries: screen `■ unexpected status 401 Unauthorized: Incorrect API key provided: ...`, title `Ready`, OSC 7501 `error`, rollout `task_complete.error` with `codex_error_info:{http_connection_failed:{http_status_code:401}}`. No `Stop` hook. I tested an API-key `auth.json`, not an expired ChatGPT login. |
| Usage limit | not reached | [S] `CodexErrorInfo::UsageLimitExceeded`, message starts `You’ve hit your usage limit` (`protocol/src/error.rs`). It should take the same failed-turn path. |
| Network stall | [O166] with an unreachable provider | After 73 s the screen showed `• Reconnecting... waiting for network (1m 13s • esc to interrupt)` and `Connection failed: error sending request`. The title stayed `⠦ Thinking` and OSC 7501 stayed `working`. No rollout error yet. Only the screen or a time limit shows it. |

- The failed-turn signature is: title `Ready`, OSC 7501 `error` (0.166.0), rollout `task_complete` with an `error`, and no `Stop`. gx can tell a failed turn from a finished one by the `error` field. The error enum is `CodexErrorInfo` ([S] `protocol/src/protocol.rs`): `ContextWindowExceeded`, `UsageLimitExceeded`, `RateLimitExceeded`, `ServerOverloaded`, `Unauthorized`, `HttpConnectionFailed`, `ResponseTooManyFailedAttempts` and others.
- An expired ChatGPT login: not tested. The error would come as `Unauthorized` ([S]) or the app-server `Blocked(Auth)` path (`user_verification.rs`), which maps to `kind=auth` in OSC 7501.

## 10. App-server with the TUI on `--remote`

Setup used: `codex app-server --listen unix://<path>` with the same `CODEX_HOME`, the TUI as `codex --remote unix://<path>` inside the holder, and a hand-written WebSocket client on the Unix socket. [O166]

- The TUI works unchanged under `--remote`. The title, OSC 7501 and dialogs were identical, including `blocked` then `working` when the second client approved.
- A second client connects with `initialize`, `thread/loaded/list`, then `thread/resume {threadId}` to subscribe. It received `thread/status/changed` with `activeFlags` `waitingOnApproval` and `waitingOnUserInput`, `turn/started`, `turn/completed`, `hook/started` and `hook/completed`, `item/started` and `item/completed`, the server requests above, and `serverRequest/resolved` when anyone answers.
- `thread/resume` failed with `no rollout found` before the first prompt, so the second client can attach only after turn 1.
- The second client started a turn with `turn/start`, and it works while the TUI is idle. `turn/interrupt` needs a `turnId` (my first call failed on that; not retried).
- Protocol surface: [S] 95 client requests, 70 server notifications and 10 server requests in the generated JSON schema (`app-server-protocol/schema/json`). `codex app-server --help` labels it `[experimental]`.
- [S] Hooks run inside the app-server process, so env and config come from the server, not the TUI.

## 11. Cost of each way in, and how it breaks on an upgrade

| Way in | Needs | Stable? | What an upgrade breaks (seen or likely) |
| :- | :- | :- | :- |
| Screen | The holder's emulator | No. Wording and layout. | Seen between 0.147.0 and 0.166.0: header text (`>_ OpenAI Codex (v0.147.0)` box vs plain), a new footer, `Starting` banners. New dialogs appear (resume directory, hooks review, new-model nudge). The `esc to interrupt` hint is not a turn signal at all. |
| OSC 0 title (`tui.terminal_title`) | A config key in the published schema | Config key: yes. Text: items and wording are chosen by gx. | `status` words and `Action Required` text could change. The session id is cut to 32 characters. |
| OSC 7501 | Terminal-side protocol (the holder already parses it) | New. Absent in 0.147.0, present in 0.166.0. [S] added 2026-10-09 (`#52725`), `done` and `blocked` kinds 2026-10-14, a guard for old VTE terminals 2026-10-28. | Missing on old Codex: gx must fall back to the title. The first release with it is not known; the first commit is an ancestor of `rust-v0.163.0` ([S] GitHub compare). |
| Hooks | `hooks` feature (stable in `codex features list`), trust handling | Documented JSON schemas, generated files in the repo. | Trust model is new and strict: command text changes re-ask. Events can be added; unknown events are ignored. No hook fires for failed turns. |
| Rollout JSONL | File reads | Internal, two history modes (earlier note). | Event names changed before (`event_msg` `error` versus `task_complete.error`). Files are compressed after about 7 days (earlier note). |
| App-server | An extra process, `--remote`, a WebSocket client | `[experimental]`, but has a generated JSON schema | Protocol and `--remote` can change. If the server dies, the TUI dies. A new process per agent adds a failure mode to `Reattach`. |

- Two upgrade facts that break plans: `codex` can change under a running gx (it happened here, once, in the middle of the run), and the first 0.166.0 launch in a fresh `CODEX_HOME` shows more startup dialogs than 0.147.0 did.

## 12. Recommendation

Use the PTY stream and hooks as the way in. Keep the screen as a last resort. Do not make app-server the base yet.

Why not the others first:
- The screen has no signal for turn end, and the dialogs differ every release.
- App-server has the best typed contract, and a second client can answer approvals and questions. But it is experimental, it moves the TUI behind another process, and `Reattach` and the holder's "one PTY, one child" model would change. Everything the PTY stream and hooks give is already enough.
- The rollout is internal and carries no live signal that arrives earlier than the title.

| Signal | Way in | Fallback |
| :- | :- | :- |
| Working, idle, done, error | OSC 7501 on 0.166.0 and later, fed to the harness `Watcher.Program`. | OSC 0 title with `tui.terminal_title`, parsed from the holder's raw tap. |
| Ready for a prompt | Title has a session id and status `Ready`, OSC 7501 `idle`. | Screen composer marker `›`. |
| Background task | OSC 7501 `working` while the title says `Ready`. | None on 0.147.0 (the screen text `background terminal running`). |
| Blocked, kind | OSC 7501 `blocked` `kind`. | Title `Action Required`. |
| Blocked, text | `PermissionRequest` and `PreToolUse` hook payloads (structured). | Screen dialog parser. |
| Session id | Title prefix at launch, full id from `SessionStart` (`transcript_path`). | Rollout file name. |
| Approve or deny, with reason | `PermissionRequest` hook held by gx (allow, or deny with a message). Falls through to the dialog on any hook failure. | Keys `y` and `Esc`. |
| Answer `request_user_input` | Keys, digit or `Tab` flow, checked against OSC 7501 `kind=question`. | None. App-server later. |
| Guard-bash | `PreToolUse` hook (exit 2 or `permissionDecision` deny). | None. |
| System prompt | `-c developer_instructions=...` | `AGENTS.md`. |
| Resume | `codex resume <id> --cd <dir>`; no instruction flag needed. | None. |
| Compaction | `PreCompact` and `PostCompact` hooks; title `context-used` for the smart zone. | Screen `Context compacted`. |
| Rate limits, failures | Rollout `token_count.rate_limits` and `task_complete.error` read from `transcript_path`. | Title `weekly N% left`; OSC 7501 `error`. |
| Stall | Time limit on `working` plus the screen text `Reconnecting`. | None. |

Launch flags this implies: `-c tui.terminal_title=[...]`, `-c disable_paste_burst=true`, `-c developer_instructions=...`, `-c hooks.<Event>=[...]` for the six events gx uses, `--dangerously-bypass-hook-trust` (or a one-time answer to the review dialog), and `--sandbox workspace-write --ask-for-approval on-request` as today. The hook command should be one fixed `gx` subcommand and pass per-session data through env (`GX_AGENT_SESSION`), so its trust hash never changes.

Design consequences the spec should state:
- Ready, blocked and idle have a PTY-stream form, so Codex needs no pid-status file. The harness interface already has `Program`, `Ready` and `ReadyOSC7501`; Codex fills them.
- The harness must handle the dialogs above (hooks review, new-model nudge, resume directory, trust) the way `StartupScreen` handles Claude's trust dialog, and must not send keys while the title lacks a session id.
- The idle Ctrl+C rule is stricter for Codex than Claude: Ctrl+C on an idle empty composer quits it.
- Codex 0.147.0 has no OSC 7501. Either set a minimum version in `gx codex doctor` or implement the title fallback. The title path covers every signal except the background-task flag and the blocked kind.
- A failed turn ends with no hook, so waiters must watch the title or OSC 7501 and then read the rollout error.

## 13. Not tested

- Two Codex processes on one session; resume while the first is alive.
- `$name` followed by text; `/model` and other pickers on 0.166.0; the retired-model migration prompt; the update dialog.
- `request_user_input` in Default mode without the feature flag; more than one question; the Plan-mode path.
- An expired ChatGPT login, a real usage-limit error, and the `kind=auth` OSC 7501.
- Esc during answer streaming on 0.147.0 (the earlier herdr note says it is ignored; one run on 0.166.0 interrupted).
- `--dangerously-bypass-hook-trust` with inline hooks, and a hook `trusted_hash` passed on the command line.
- Hook failure modes other than empty output: a timeout, exit 1, invalid JSON ([S] only).
- Passing `developer_instructions` again on resume.
- App-server `turn/steer`, `turn/interrupt` with a `turnId`, a shared server for several agents, and server restart with the TUI attached.
- Which release first emitted OSC 7501.
- `AGENTS.md` handling, `model_instructions_file`, and execpolicy `.rules` as a second guard mechanism.
- Apply-patch approval and MCP elicitation dialogs.
