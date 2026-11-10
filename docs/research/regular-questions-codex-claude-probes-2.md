# Codex startup dialog and Claude hook/plan probes, round 2

Ticket `gx:regular-questions/09`. Follow-up to `regular-questions-codex-claude-probes.md` (07); also uses `regular-questions-claude-dialog.md` (01) and `regular-questions-claude-dialog-facts.md` (05).

Versions: **codex-cli 0.166.0** (installed when the work began) and **codex-cli 0.167.0** (the newest release; see "Side effect" below), model GPT-6.1-Sol. **Claude Code 2.1.319**, Haiku 5.5 for the hook probes (the plan-mode run switched to Sonnet 5.5 in the status line by itself). tmux, 120x40, macOS. Every run used a scratch cwd under the session scratchpad (`work/cx`, `work/cl`, `work/cl3`), never the repo. Codex ran with `CODEX_HOME=<scratchpad>/ch` (copied `auth.json`, a `config.toml` that trusts the scratch cwd). Claim tag **[run]** = seen in a real run, evidence file named. Captures live in `/private/tmp/claude-501/-Users-david-dev-gx-regular-questions-design/0cc954f2-9a5f-4978-8b3c-c90e290e5511/scratchpad/` (`cap9_*.txt`, `transcript_cl1.jsonl`, `transcript_plan.jsonl`, `fake/`).

## Answer in short

- **The Codex dialog still appears on 0.167.0**, same as 0.166.0, but only for a launch whose only `-c` override is the feature flag. Rows: `1. Run without daemon this time`, `2. Restart with these settings (recommended)`, `3. Cancel` (cursor starts on 3). **Digit `1` = run without the daemon this time.**
- **`3. Cancel` ends the process**: it prints `Error: Cannot use the shared background server: This session requires default_mode_request_user_input to be enabled. To work without the background server, rerun the same command with --no-daemon ...` and exits with **code 1**. Enter on the default row, digit `3` and Esc all do the same (0.167.0).
- **`--no-daemon` removes the dialog** (both versions). A plain chat then works unchanged: reply came back, the rollout was written under `$CODEX_HOME/sessions/...`, and the `request_user_input` overlay still appeared and took a digit. The only differences seen: one status-bar warning (`Under-development features enabled`) instead of the "running without the shared server" one, and no daemon process is used.
- **Surprise: any additional `-c` override also removes the dialog.** `-c check_for_update_on_startup=false -c features.default_mode_request_user_input=true` (no `--no-daemon`) went straight to a chat with the warning `Running without the shared background server: command-line configuration overrides (-c, --enable, --disable, or --search) requires embedded mode.` So the dialog shows only when the flag is the sole override. Passing `--no-daemon` is the reliable way and is explicit.
- **Claude hook, 3 questions: works.** One hook call answered all three; "Other" free text on all three questions works (any string matching no label); the answers reach Claude as `"Q"="A", ...`.
- **`annotations` is read by Claude.** Shape `annotations: {"<question text>": {"notes": "...", "preview": "..."}}`. Claude's tool result text renders `"Q"="A" notes: <notes>` and, if `preview` is set, `selected preview:\n<preview> notes: <notes>`. The model quoted them back. The TUI's own answer summary does not show them.
- **Fail-open confirmed**: server down (connection refused), server accepted but never answered (hook's own timeout), and Claude's own hook `timeout` expiring all end with the normal terminal dialog. Never exit 2.
- **Plan dialog Esc**: closes the dialog, turn ends, session goes `idle` in plan mode. Transcript: `ExitPlanMode` `tool_result` `is_error: true`, text `The user doesn't want to proceed with this tool use. ... STOP what you are doing and wait for the user to tell you how to proceed.`, `toolUseResult: "User rejected tool use"`, then a user line `[Request interrupted by user for tool use]`.

## Side effect to know about

While probing 0.166.0 without `check_for_update_on_startup=false`, Codex showed its update dialog (`1. Update now (runs npm install -g @openai/codex)`). My next keypress sequence (`Enter` after typing a prompt) hit it, so **the global Codex was upgraded from 0.166.0 to 0.167.0** (`codex --version` now says 0.167.0). That also means 0.167.0 is the newest and now the installed version. All 0.166.0 results below were captured before the upgrade (header `v0.166.0` in the captures); all 0.167.0 results after. A scratch copy (`scratchpad/cx167`) was also installed with `npm i --prefix`, unused afterwards.

## 1. Codex startup dialog [run]

Evidence: `cap9_166_flagonly.txt`, `cap9_167_dialog.txt`.

Launch `codex -c features.default_mode_request_user_input=true` (CODEX_HOME scratch, tmux). On both 0.166.0 and 0.167.0, before the chat UI:

```
  Background server has incompatible feature settings
  This session requires default_mode_request_user_input to be enabled
  These settings persist and can disable functionality for other clients. Restart may interrupt active or queued work.
  Restart will use these shared feature settings:
    default_mode_request_user_input = true
  1. Run without daemon this time
  2. Restart with these settings (recommended)
› 3. Cancel
```

- Digit `1` (0.166.0, `cap9_166_digit1.txt`): dialog closes, normal chat screen. F2 shows `Running without the shared background server: This session requires default_mode_request_user_input to be enabled.`
- Option 2 (restart the shared daemon) was not pressed: it would change a shared daemon; not needed for the question.

## 2. What `3. Cancel` does [run]

Evidence: `cap9_166_cancel.txt` (0.166.0, digit `3`), `cap9_167_cancel_enter.txt` (Enter on the default row), `cap9_167_cancel_digit3.txt`, `cap9_167_dialog_esc.txt` (Esc).

All four produce the same output, then the shell prompt (the probe's `echo EXIT=$?` printed `EXIT=1`):

```
Error: Cannot use the shared background server: This session requires default_mode_request_user_input to be enabled.
To work without the background server, rerun the same command with --no-daemon (including resume or fork and its arguments).
EXIT=1
```

So Cancel (and Esc) **exits the Codex process with code 1** and an error that names `--no-daemon`. For a gx pane that means the session dies. The cursor starts on Cancel, so a blind Enter kills it.

## 3. `--no-daemon` [run]

Evidence: `cap9_167_nodaemon_start.txt`, `cap9_167_nodaemon_chat.txt`, `cap9_167_nodaemon_f2.txt`, `cap9_167_nodaemon_overlay.txt`; 0.166.0 earlier run in `cap9_166_nodaemon_start.txt` (an update dialog got in the way, see Side effect, but the daemon dialog did not show).

`codex --no-daemon -c features.default_mode_request_user_input=true` on 0.167.0:
- No daemon dialog; straight to `› Ask Codex to do anything`.
- Plain chat: prompt `Reply with exactly: PONG` produced `• PONG`, `Worked for 3s`. Rollout `ch/sessions/2026/11/10/rollout-...jsonl` was written, so gx's rollout reader sees the same file layout as before.
- Status bar: `⚠ 1 warning`; F2 shows only `Under-development features enabled: default_mode_request_user_input ...`. (Without `--no-daemon` but with the dialog answered `1`, there are two warnings.)
- The `request_user_input` overlay still appears (`Question 1/1`, `1. Red`, `2. Blue`, `3. None of the above`); digit `2` answered it and Codex finished its turn.
- Typing a prompt and pressing Enter in the same `send-keys` call left the text in the input box unsent once (the 0.167.0 input treats it as a paste); a separate `Enter` sent it. Send the prompt text, pause, then Enter.
- Other overrides: `-c check_for_update_on_startup=false` plus the flag, no `--no-daemon` (`cap9_167_extra_c.txt`) also avoids the dialog, with the embedded-mode warning quoted in the short answer. A launch with no flag and no `-c` showed no dialog and 1 warning (`cap9_167_noflag.txt`) and started a managed daemon in the scratch home, which I stopped with `codex app-server daemon stop`.
- Not seen: any other change to what a plain chat needs. Resume/fork were not run; the error text says they need `--no-daemon` too.

## 4. Claude AskUserQuestion through a hook [run]

**What I built.** gx has no AskUserQuestion hook yet (only `gx agent guard-bash` and Codex's guard-question exist in `cmd/agent/agent_guard.go`), so I built a fake of the contract from `regular-questions-claude-dialog.md`: `scratchpad/fake/hook.py` (a PreToolUse command hook on matcher `AskUserQuestion`, passed via `--settings`; it logs stdin, POSTs it to a tiny local HTTP server `fake/server.py`, and prints `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","updatedInput":<server body>}}`), with the server returning canned `answers`/`annotations` plus the original `questions`. On any error or timeout the hook prints nothing and exits 0. The real gx server was not touched.

Prompt for every probe: ask `AskUserQuestion` once with three questions (`Which color?` single Red/Blue; `Which toppings?` multi Cheese/Egg/Ham; `What name?` single Bob/Amy), then quote the tool result.

Hook stdin (`fake/hook_stdin.jsonl`): `session_id`, `transcript_path`, `cwd`, `permission_mode`, `prompt_id`, `effort`, `hook_event_name: "PreToolUse"`, `tool_name: "AskUserQuestion"`, `tool_input.questions[]{question,header,options[]{label,description},multiSelect}`, `tool_use_id`. No "Other" row in `options`.

| Probe | `answers` / `annotations` returned | Tool result Claude saw (transcript `tool_result`) |
|---|---|---|
| 3 questions, all picked options | `{"Which color?":"Blue","Which toppings?":"Cheese, Egg","What name?":"Bob"}` | `Your questions have been answered: "Which color?"="Blue", "Which toppings?"="Cheese, Egg", "What name?"="Bob". You can now continue with these answers in mind.` |
| "Other" on all three | `"teal-ish custom"`, `"Cheese, pineapple (other)"` (multi: one real label plus a free text), `"Zed custom"` | `The user answered: "Which color?"="teal-ish custom", "Which toppings?"="Cheese, pineapple (other)", "What name?"="Zed custom". Read the answers carefully — they may request clarification, changes, or that you not proceed — and follow what they actually say.` |
| With `annotations` on all three (`notes`; `preview` on toppings) | `annotations: {"Which color?":{"notes":"NOTE-ONE-xyzzy"},"Which toppings?":{"notes":"NOTE-TWO-plugh","preview":"PREVIEW-TWO"},"What name?":{"notes":"NOTE-THREE-quux"}}` | `The user answered: "Which color?"="Blue" notes: NOTE-ONE-xyzzy, "Which toppings?"="Cheese, Egg" selected preview:\nPREVIEW-TWO notes: NOTE-TWO-plugh, "What name?"="Bob" notes: NOTE-THREE-quux. Read the answers carefully ...` |

Evidence: `cap9_cl_multi.txt`, `cap9_cl_other.txt`, `cap9_cl_ann.txt`, `transcript_cl1.jsonl`.

- The hook returned no dialog; the TUI showed `User answered Claude's questions:` with `· Q → A` lines for all three. The annotation notes are **not** shown in that TUI summary, but Claude read them: asked "did you see any notes", the model answered yes and named all three notes. With no annotations it said it saw none.
- The notes are keyed by the **question text**, as `answers` is. `toolUseResult` in the transcript keeps `annotations` verbatim next to `answers`.
- The model may not act on a note (it said "the result doesn't say what the notes mean"), so notes are context, not instructions.
- The wording flips from `Your questions have been answered:` (all values are option labels) to `The user answered: ... Read the answers carefully ...` when any value is not an option label, or when annotations are present (confirmed in the third row, whose answers were all labels).
- Multi-select "Other": an answer list is just one string; a label list plus free text in one string (`"Cheese, pineapple (other)"`) was accepted.

## 5. Fail-open [run]

Same three-question prompt, hook settings unchanged; only the server changed.

| Situation | Hook behavior | Result | Evidence |
|---|---|---|---|
| Server down (killed, port refused) | `URLError(ConnectionRefusedError(61))` logged; prints nothing, exit 0 | The normal terminal dialog opened (`Which color?` with `1. Red`, `2. Blue`, `3. Type something.`, `4. Chat about this`) | `cap9_cl_down.txt`, `fake/hook_err.log` |
| Server accepts, never answers; hook timeout 5 s | `TimeoutError` after 5 s; prints nothing, exit 0 | Screen showed `running PreToolUse hook · 6s`, then the normal dialog | `cap9_cl_noanswer.txt`, `cap9_cl_noanswer2.txt` |
| Server never answers, hook itself waits 100 s, Claude's own hook `timeout` set to 3 | Killed by Claude at 3 s | Normal dialog | `cap9_cl_hooktimeout.txt` |

So when the hook cannot get an answer by any path (nothing listens, nothing answers, or Claude's timeout fires) the user falls back to the terminal dialog. Pressing Esc on that dialog gives `User declined to answer questions` in the transcript (`cap9_cl_noanswer.txt`). Exit 2 is not part of this table; 05 already showed it blocks the tool.

## 6. Claude plan dialog (ExitPlanMode), Esc [run]

Evidence: `cap9_plan_dialog.txt`, `cap9_plan_esc.txt`, `transcript_plan.jsonl`. Launch `claude --permission-mode plan` in a scratch cwd, no hook.

- Dialog: same as 07 (`1. Yes, and use auto mode`, `2. Yes, manually approve edits`, `3. Tell Claude what to change`, last row `ctrl+g to edit in Vim · ~/.claude/plans/make-a-3-bullet-plan-witty-dragon.md`).
- **Esc** at that dialog: the dialog closed at once. Screen: `⏺ Updated plan`, `⎿ /plan to preview`, `⎿ User rejected Claude's plan:` followed by the plan in a box, `✻ Crunched for 4s · done`, empty prompt, status line still `⏸ plan mode on`. No further model turn.
- The pid file read `status: idle` afterward.
- Transcript, in order: assistant `Write` (plan file), assistant `ExitPlanMode`, user `tool_result` with `is_error: true`, content `The user doesn't want to proceed with this tool use. The tool use was rejected (eg. if it was a file edit, the new_string was NOT written to the file). STOP what you are doing and wait for the user to tell you how to proceed.`, `toolUseResult: "User rejected tool use"`, then a user text line `[Request interrupted by user for tool use]`, then a `turn_duration` system line.
- Compared with 07: Esc looks the same on screen as `3` then `Enter` with no text (rejected, idle, still plan mode), plus the extra `[Request interrupted by user for tool use]` transcript line, which the empty-feedback path in 07 was not recorded with.

## Could not determine

- Whether option 2 (`Restart with these settings`) leaves the shared daemon changed for other clients (not pressed; it would touch a shared daemon).
- Resume/fork behavior under `--no-daemon` (not run).
- The earliest Claude version that accepts `annotations` through a hook (only 2.1.319 run).
- `preview` was set on one (multi-select) question only; a call whose options carry their own `preview` fields was not run.
- gx's real hook and server were not exercised; the fake follows the documented contract, so gx-specific exit paths (gx binary missing, socket permission) are untested.
