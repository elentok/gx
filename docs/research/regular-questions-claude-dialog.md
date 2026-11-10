# Reading and answering Claude's question dialog (AskUserQuestion) from outside

Ticket: `gx:regular-questions/01`. Tags: **[run]** verified by a real run (Claude Code 2.1.319, tmux, haiku, scratch dirs only), **[docs]** from Claude Code's official docs (code.claude.com hooks + tools-reference), **[source]** from the gx source in this worktree.

## Answer in short

- Read path: a **PreToolUse hook on `AskUserQuestion`**. It receives the full question set and the `tool_use_id` on stdin the moment the dialog would open.
- Answer path: the same hook **blocks until the web UI answers**, then returns `permissionDecision: "allow"` plus `updatedInput` carrying `answers`. Claude gets it as a normal answered tool call. No keystrokes.
- The transcript JSONL and the pid status file cannot carry the content. The terminal screen can, but only fragilely.

## 1. Where the dialog content can be read while it is open

| Source | Verdict |
|---|---|
| PreToolUse hook | **Reliable.** [run] stdin carried `hook_event_name`, `tool_name: "AskUserQuestion"`, `tool_use_id`, `session_id`, `transcript_path`, `cwd`, and `tool_input.questions[]` with `question`, `header`, `multiSelect`, `options[].label/description`. [docs] hooks reference, "AskUserQuestion" input table (1 to 4 questions, optional `multiSelect`). The "Other"/"Type something" row is not in `options`; it is always offered by the dialog. |
| Transcript JSONL | **Not usable while open.** [run] With the dialog open for about 12 s, the transcript held the user prompt but **no assistant `tool_use` line**. The `tool_use` (stamped 09:11:03, the creation time) and the `tool_result` (09:11:46) both appeared only after the answer. Same shape seen in an older real transcript (tool_use stamped 09:43:56, result 09:46:24). So the transcript is a post-hoc record, not a live read. A second Esc run showed the same. |
| Pid status file (`~/.claude/sessions/<pid>.json`) | **Signal only.** [run] with the dialog open: `status: "waiting"`, `waitingFor: "input needed"` (a permission prompt reads `"permission prompt"`, matching `harness/claudepid.go` `claudePermissionPrompt`). No question content. gx already maps this to kind `question` [source]. |
| Terminal screen (vt) | **Possible, fragile.** [run] The dialog renders tabs (`☐ Color  ☐ Toppings  ✔ Submit`), the question, numbered options with indented descriptions, `Type something.`, `Chat about this`. `agentrunner/claudeui/testdata/dialog-askuserquestion.screen.txt` already has a single-question capture [source]. Multi-select shows `[ ]`/`[✔]` rows; the layout differs by mode, and `claudeui.ParseDialog` deliberately returns not-approvable for it [source: `dialog.go` comment "layout is not a stable contract"]. Long options wrap and scroll. Do not build the primary path on this. |
| Notification hook | Not enough. [docs] `permission_prompt` fires only after about 6 s of idle and carries no questions. |

## 2. Sending the answer back

### Recommended: blocking PreToolUse hook (reliable)

- [docs] "`AskUserQuestion` and `ExitPlanMode` need `updatedInput` paired with `allow`"; echo the original `questions` and add `answers`, a map of question text to chosen label; "Multi-select answers join labels with commas". "Claude doesn't set this field; supply it via `updatedInput` to answer programmatically."
- [run] In an **interactive** TUI session (not `-p`), a hook that slept 10 s, then returned `allow` + `updatedInput.answers`, produced `User answered Claude's questions: ... → HOOK-ANSWER`. Claude read it as the user's answer. A **free-text answer that matches no option label worked** (the "Other" case), since `answers` is a plain string.
- [run] While the hook runs, **the terminal dialog is not shown**; the TUI shows `running PreToolUse hook` and the pid status is `busy`, not `waiting`. So gx must learn about the question from the hook itself (the hook calls the gx server), not from the pid file.
- Several questions: one hook call carries all; `answers` has one entry per question text. Multi-select: labels joined with `", "` (the dialog's own transcript result for Cheese + Egg was `"Cheese, Egg"` [run]). Keep the exact question strings as keys.
- Cancel from the web: return `permissionDecision: "deny"` with a reason [docs]. Claude sees a rejected tool use.
- gx already installs hooks into Claude launches by merging `--settings` JSON (`harness/claudesettings.go` `MergeClaudeSettings`, tested with a `PreToolUse` merge) and has a `gx agent guard-*` hook command pattern (`cmd/agent/agent_guard.go`) [source]. A `gx agent ask-hook` command is the same shape: read stdin, long-poll the server for the answer, print the JSON.
- Timeout: [docs] command hooks default to **600 s** (`timeout` field in seconds); on timeout "the call continues through the normal permission flow", which for `AskUserQuestion` shows the terminal dialog. Set `timeout` high, or let a timeout fall back to the dialog and the Screen button, which is a good failure mode.

### Fallback: keystrokes into the dialog (works, fragile)

Observed key map [run], Claude Code 2.1.319, tmux `send-keys`:

- Single-select: the cursor starts on option 1. `Down` moves, `Enter` picks and **advances to the next question tab**.
- Multi-select: `Enter` **toggles** the row under the cursor (it does not advance). Move with `Down` to the `Submit` row below `Type something`, then `Enter` to advance.
- After the last question, a **review screen** (`Review your answers`, `1. Submit answers`, `2. Cancel`) appears when there are two or more questions; `Enter` submits. With one question, a single `Enter` on the answer submitted straight away.
- "Other": `Down` to the `Type something.` row, then **type the text directly** (the row becomes an inline input), `Enter`. Worked for a single question. Multi-question Other was not tried.
- Esc cancels: the transcript gets an `is_error` tool_result "The user doesn't want to proceed with this tool use" plus `[Request interrupted by user for tool use]` [run].
- Fragility: tab count, review-step presence, `Chat about this` row, hint footer text, and the 60 s `askUserQuestionTimeout` auto-submit [docs: tools-reference "Question auto-continue timeout"] all vary. There is no per-keystroke feedback, so a stale screen sends wrong picks. Use only if a hook cannot be installed (for example a Claude launched by a user outside gx).

Also not available: an `-p` `defer` flow [docs] ("honored only in non-interactive mode"); gx's plain chats are interactive.

## 3. Telling that the dialog closed

- Hook path [run]: pressing Esc while the hook was blocked **sent SIGTERM to the hook process** (a trap file proved it) and the turn ended. The hook can catch SIGTERM, tell the server the question is gone, and clear the cards. If the answer arrives first, the hook exits normally. So the hook's own process lifetime is the open/closed signal, and it needs no polling. Answering "in the terminal" is not possible while the hook holds (the dialog isn't drawn), which removes the draft-versus-terminal race listed under "Not yet specified" for this path; Screen shows the "running PreToolUse hook" line instead.
- Keystroke or screen path: the pid status leaves `waiting` (observed `idle` after the answer and after Esc) [run]; the transcript gains the `tool_result` (answer: `Your questions have been answered: ...`; cancel: `is_error`) [run]. Use pid status for fast clearing, the transcript to learn which way it closed.

## 4. Version dependence

- Tested on **Claude Code 2.1.319** only. The hook contract for `AskUserQuestion` (`updatedInput` with `answers`) is in the current docs [docs]; the docs name no introduction version for it, so the earliest supporting release is an **open fact**.
- Related notes in the docs: `permission_prompt` for a sandboxed network request needs v2.1.246+; `defer` plan-mode resume needs v2.1.246+; neither affects this design.
- The dialog layout is explicitly not a contract (`claudeui/dialog.go` comment), another reason to prefer the hook.
- `answers` shape for multi-select is a comma-joined string; a label containing a comma is ambiguous for Claude's reading. Unverified edge.

## Recommended read path and answer path

1. At Claude launch (plain chats only; Queue agents and Design/Triage keep `--disallowedTools AskUserQuestion`), merge a PreToolUse hook with `matcher: "AskUserQuestion"` into `--settings`.
2. The hook (`gx agent ...`) posts `{session, tool_use_id, questions}` to the server and long-polls. The server publishes it into the chat so cards appear (a "dialog question" source distinct from printed ❓ rounds).
3. The person answers on the web; the server resolves the poll; the hook prints `allow` + `updatedInput{questions, answers}`. Cancel prints `deny`.
4. Hook exit or SIGTERM (Esc, session death) clears the cards. A `tool_use_id` mismatch on submit refuses a stale answer.
5. Keep the keystroke path out of the first cut. If ever needed, drive it from a fresh vt read and verify the screen before each key.

Trade-off: the hook hides the terminal dialog while it waits, so the terminal cannot also answer. That is the cleaner contract (one place answers) but means "Screen" shows no dialog for these questions. The hook timeout is the escape hatch.

## Open facts

- Earliest Claude Code version honoring `updatedInput.answers` for AskUserQuestion in interactive sessions; behavior on older gx-supported versions.
- Whether hook `answers` with an `annotations`/notes field is supported (the dialog result had `annotations: {}`).
- Behavior with two or more questions through the hook (only one question tested through the hook; the shape follows the docs and the dialog's own result).
- Multi-question "Other" via keystrokes; wrap and scroll behavior with long option lists.
- Hook behavior when the gx server is down (hook should fail open: print nothing, exit 0, so the normal dialog shows; confirm exit-0-with-no-output shows the dialog).
- Interaction with `askUserQuestionTimeout` and Remote Control; not tested.
