# Codex request_user_input and Claude plan approval, confirmed by real runs

Ticket `gx:regular-questions/07`. Extends `regular-questions-codex-question.md` (02), `regular-questions-claude-dialog-facts.md` (05) and `regular-questions-dialogs.md` (03).

Versions: **codex-cli 0.166.0** (model `gpt-6.1-sol`, effort low), **Claude Code 2.1.319** (Haiku 5.5), tmux 3.8, macOS. Every run used a scratch cwd under the session scratchpad (`.../scratchpad/work/cx`, `work/cx2`, `work/cl`), never the repo. Claim tag **[run]** means seen in a real run; each claim names its evidence. Captures are `cap_*.txt` and rollout lines, kept in the scratchpad (`/private/tmp/claude-501/-Users-david-dev-gx-regular-questions-design/8a90e66e-290f-4167-adc3-3d9f9e90d29b/scratchpad/`).

Codex runs used `CODEX_HOME=<scratchpad>/ch` (a copy of `auth.json`, a minimal `config.toml` that trusts the scratch cwd) so rollouts landed in `ch/sessions/...` and the user's `~/.codex` stayed untouched, plus `-c features.default_mode_request_user_input=true`, 120x40 tmux panes. Claude runs used the real `~/.claude`; accepting the trust dialog for the scratch cwd added one trust entry there.

## Answer in short

- **A launch with the flag opens a blocking dialog that `codexDialogs` does not know.** "Background server has incompatible feature settings" (`1. Run without daemon this time / 2. Restart with these settings / 3. Cancel`, cursor on 3). It appeared on every launch with the flag, in a fresh home and in the real `~/.codex`. Passing **`--no-daemon`** removes it. gx passes no `--no-daemon` today. This is the one thing the spec must change besides the flag.
- The flag's other effect is a non-blocking status-bar warning ("Under-development features enabled"). It does not block detection.
- Digits answer every Codex question: `k` picks option `k` and advances; on the last question it submits. The "None of the above" row is the digit after the last option and submits at once as `"None of the above"` with no note.
- Claude plan dialog: digits **1** and **2** act at once. Digit **3** only moves the cursor; the row becomes a text box, text is typed and `Enter` submits it as feedback. Enter on an empty row 3 rejects the plan with no feedback.
- The plan dialog's last row names `~/.claude/plans/<name>.md` in every run at 80, 120 and 160 columns. At 80 columns it wraps to two lines when the name is long. A long plan is cut off above the rule.

## 1. Codex overlay screens and digit keys [run]

Evidence: `cap_c1_overlay.txt`, `cap_c3_q1.txt`, `cap_c3_q2.txt`, `cap_c3_q3.txt`; rollout `function_call_output` lines.

One question (`pick`, Red/Blue). The model adds the description text; the options order is the call's order:

```
  Question 1/1 (1 unanswered)
  Which color?
  › 1. Red                Choose red.
    2. Blue               Choose blue.
    3. None of the above  Optionally, add details in notes (tab)
  tab to add notes | enter to submit answer | esc to interrupt
```

Key `1` submits. Rollout: `{"answers":{"pick":{"answers":["Red"]}}}`.

Three questions (`a` Size 2 options, `b` Shape 3 options, `c` Mode 2 options). Each screen has the same shape; the counter and footer change:

```
  Question 1/3 (3 unanswered)          footer: ... | ←/→ to navigate questions | esc to interrupt
  Question 2/3 (2 unanswered)          (3 options + "4. None of the above")
  Question 3/3 (1 unanswered)          footer: tab to add notes | enter to submit all | ...
```

Keys `2`, `3`, `1` (one key per question, no Enter) produced Q1 Large, Q2 Oval, Q3 Fast and closed the overlay after the third key. Rollout:

```
{"answers":{"c":{"answers":["Fast"]},"b":{"answers":["Oval"]},"a":{"answers":["Large"]}}}
```

- The `answers` object key order is **not** the question order (here `c, b, a`; another run `b, c, a`). Match by id.
- The number of the "None of the above" row is one more than the option count (3 for two options, 4 for three).
- The last question's footer says `enter to submit all`; the others say `enter to submit answer`.
- The model, not the overlay, writes the option descriptions in these probes (the prompt gave none); the overlay prints whatever the call holds.

## 2. "None of the above" row [run]

Evidence: `cap_c1_none_row.txt`, `cap_c1_none_enter.txt`, `cap_c1_none_typed.txt`, `cap_c1_digit3.txt`.

- `Down Down` puts the cursor on `3. None of the above`. `Enter` does **not** submit; it opens a notes line under the list (`› Add notes`, footer `tab or esc to clear notes | enter to submit answer`). Typing fills it; `Enter` submits.
- Keys `Down Down Enter "my own color" Enter` produced `{"answers":{"pick":{"answers":["None of the above","user_note: my own color"]}}}`.
- Digit `3` on a two-option question submits at once with no notes line: `{"answers":{"pick":{"answers":["None of the above"]}}}`.

## 3. Notes on an option [run]

Evidence: `cap_c1_tab_on_first.txt`, `cap_c1_note_then_digit.txt`, `cap_c1_note_tab_again.txt`, `cap_c3_note_enter_advance.txt`.

- `Tab` on the highlighted option opens the notes line (`› Add notes`); typing then `Enter` submits that highlighted option with the note. In the 3-question run `Down Tab "big one" Enter` answered Q1 and advanced to Q2; the final output held `"a":{"answers":["Large","user_note: big one"]}`.
- The note format is a second `answers` entry `"user_note: <text>"`.
- **A note together with a digit cannot be sent in one question.** While the notes line is focused a digit is typed into it (`because` then `2` showed `› because2`). A second `Tab` returns to the list and clears the note (`cap_c1_note_tab_again.txt`); `2` then submitted `["Blue"]` with no note. So a note is always sent as: move to the option with `Down`, `Tab`, text, `Enter`. Digits are for answers with no note.
- A note on one question and digits on the others in a 3-question call work in sequence: `Down Tab "big one" Enter`, `3`, `2` gave `a` Large + note, `b` Oval, `c` Slow.

## 4. "Submit with unanswered?" confirm [run]

Evidence: `cap_c3_skip_to_q3.txt`, `cap_c3_confirm.txt`, `cap_c3_confirm_esc.txt`.

- It opens when the **last-question submit** is pressed while some other question is unanswered. Reached with `2` (Q1), `Right` (skips Q2 to Q3, header reads `Question 3/3 (2 unanswered)`), `1` (Q3). Screen:

```
  Submit with unanswered questions?
  1 unanswered question
  › 1. Proceed  Submit with 1 unanswered question
    2. Go back  Return to the first unanswered question
  Press enter to confirm or esc to go back
```

- `Enter` (on `1. Proceed`) submits. The output has an empty list for the skipped id: `{"answers":{"c":{"answers":["Fast"]},"b":{"answers":[]},"a":{"answers":["Large"]}}}`. Codex's turn then showed `Shape: No answer provided`.
- `Esc` goes back to the first unanswered question (`Question 2/3`), with the earlier answers kept. A digit there answered Q2 and moved to Q3.
- Answering every question in order never opens it.

## 5. Startup warning and dialog with the flag [run]

Evidence: `cap_daemon_dialog_scratchhome.txt`, `cap_daemon_dialog_2nd.txt`, `cap_realhome_noflag.txt`, `cap_realhome_flag.txt`, `cap_realhome_flag_nodaemon.txt`, `cap_scratch_flag_nodaemon.txt`.

- **Blocking dialog.** With the flag and no `--no-daemon`, Codex 0.166.0 shows before anything else:

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

  It showed on the first launch in a fresh home, again on the next launch (a managed `codex app-server ... --managed-daemon` process was left running by Codex itself in that home; `codex app-server daemon stop` stopped it), and in the real `~/.codex` where a user daemon (0.167.0) was already running. Without the flag the real home showed only the update dialog, no daemon dialog.
- **None of `codexDialogs`' strings match it** (`do you trust the contents`, `trust this directory`, `hooks review`, `update available`, `model nudge`), so `StartupScreen` returns no dialog and gx sends nothing. The cursor starts on `3. Cancel`, so a blind Enter would pick Cancel; what Cancel does to the process was not run.
- **`--no-daemon` avoids it.** `codex --no-daemon -c check_for_update_on_startup=false -c features.default_mode_request_user_input=true` in the real home reached the normal Codex screen with no dialog; in the scratch home with the prompt, it reached the overlay directly (`cap_scratch_flag_nodaemon.txt`). `harness/codex.go` has no `daemon` or `--no-daemon` string, so today's launch args would hit the dialog.
- **Non-blocking warning.** The status bar shows `⚠ N warnings · f2 to view`. Under F2 the first warning is `Startup: Under-development features enabled: default_mode_request_user_input. Under-development features are incomplete and may behave unpredictably. To suppress this warning, set suppress_unstable_features_warning = true in ...`. It is a bar item, not a dialog, and it does not cover the screen. The same count also holds MCP start failures from the user's own config (`cua_repl`, `gopls-mcp`, `node_repl`). In a scratch-home run after the dialog was dismissed with `1`, F2 read `Running without the shared background server: This session requires default_mode_request_user_input to be enabled.`
- With `--no-daemon`, no other dialog appeared in either home. The scratch home had its cwd trusted by config, so the trust dialog was not exercised.

Not determined: whether Codex 0.167.0 (the version a user update dialog offers) keeps the dialog, since only 0.166.0 was run.

## 6. Esc in the overlay (interrupt) [run]

Evidence: `cap_c1_esc.txt`; rollout lines 13 to 18 of the Esc run.

- Screen after Esc: `■ Conversation interrupted - use /feedback if something went wrong`, input box back.
- Rollout, in order: `response_item function_call` (`request_user_input`) at 12:30:25.075; `function_call_output` with `call_id` equal to the call and `output = "aborted by user after 0.8s"` at 12:30:25.884; `event_msg turn_aborted` with `reason: "interrupted"` at 12:30:25.887.
- So the call is closed by a `function_call_output` whose output is the text `aborted by user after <n>s`, not a JSON `answers` object. A reader that only watches for `function_call_output` sees the close; one that parses `answers` must handle this plain string.
- Esc while the notes line is focused clears the notes (footer says `tab or esc to clear notes`); the interrupt is Esc on the option list. The 3-option overlay's Esc during the confirm goes back (section 4), it is not an interrupt.

## 7. Is the `function_call` on disk before the overlay is visible [run]

Method: a 50 ms poller started at launch (`timing.py`) recorded when the rollout file first held `"name":"request_user_input"` and when the pane first showed `Question 1/1` with `esc to interrupt`, over 5 runs.

| Run | `function_call` on disk | overlay visible |
|---|---|---|
| t1 | 2.83 s | 5.66 s |
| t2 | 2.64 s | 5.38 s |
| t3 | 4.23 s | 7.96 s |
| t4 | 2.62 s | 5.64 s |
| t5 | 3.22 s | 6.87 s |

The line was on disk first in 5 of 5 runs, 2.7 to 3.7 s earlier. Times include the daemon-dialog dismissal (the poller pressed `1`). Evidence in the rollouts: `function_call` timestamp 12:25:47.07 with the overlay captured later. Five runs on one model; no run showed the reverse order.

## 8. Claude plan approval (ExitPlanMode), 2.1.319 [run]

Dialog (120 cols; plan text above, the plan file name is generated and differs each run):

```
  ────────────────────────────────────────────────────────────────────────────────
   Ready to code?
   Here is Claude's plan:
  ╌╌╌╌╌╌╌╌╌ (plan text) ╌╌╌╌╌╌╌╌╌
  ────────────────────────────────────────────────────────────────────────────────
   Claude has written up a plan and is ready to execute. Would you like to proceed?
   ❯ 1. Yes, and use auto mode
     2. Yes, manually approve edits
     3. Tell Claude what to change
        shift+tab to approve with this feedback
   ctrl+g to edit in Vim · ~/.claude/plans/plan-add-a-function-iterative-steele.md
```

(The first row reads `Yes, and use auto mode` here because the user's settings enable auto mode; the option labels are not fixed across setups and were not varied.)

### Option 3 after a digit [run]

Evidence: `cap_plan_digit3.txt`, `cap_plan_typed.txt`, `cap_plan_after_feedback.txt`, the Claude transcript `ca0d516b-....jsonl`.

- Digit `3` moves the cursor to row 3 and **does not submit**. The row is an inline text box: typing `use a docstring too` replaced the label (`❯ 3. use a docstring too`).
- `Enter` submits it. No separate feedback dialog appears.
- The screen: Claude revised the plan file (a new bullet "Add a docstring ...") and showed the same dialog again with fresh plan text.
- The transcript: the `ExitPlanMode` `tool_result` is `is_error: true`, content `The user doesn't want to proceed with this tool use. The tool use was rejected (...). To tell you how to proceed, the user said:\nuse a docstring too`. The `ExitPlanMode` tool_use is repeated with the new plan.
- `Enter` on row 3 with no text (`3`, `Enter`): the plan is rejected with no feedback. Screen shows `User rejected Claude's plan:` with the plan in a box, the turn ends idle (still plan mode), and the `tool_result` is `is_error: true`, `... STOP what you are doing and wait for the user to tell you how to proceed.`, `toolUseResult: "User rejected tool use"` (`cap_plan_digit3_enter_empty.txt`).

### Last row names the plan file [run]

Evidence: `cap_plan_w120s.txt`, `cap_plan_w160s.txt`, `cap_plan_a80.txt`, `cap_plan_b80.txt`, `cap_plan_a120.txt` and others.

Eleven runs (short and long plans), all showed `ctrl+g to edit in Vim · ~/.claude/plans/<name>.md` as the last non-empty row:

| Width | Runs | Result |
|---|---|---|
| 80 | 3 | one line when the name is short (`plan-add-a-function-jazzy-prism.md`); **two lines** when long: `ctrl+g to edit in Vim ·` then `~/.claude/plans/plan-add-a-function-snappy-bachman.md` (also `plan-a-refactor-of-toasty-ladybug.md`, `...stateless-volcano.md`) |
| 120 | 5 | always one line |
| 160 | 3 | always one line |

- The name is `plan-<first words of the request>-<adjective>-<name>.md`, so it is not predictable. The file is not under the project; the path is shown with `~` for the home directory.
- At 80 columns the question line also wraps (`...Would you like to` / `proceed?`), so a detector must not match the question on one line. `Tell Claude what to change` and the last row were each on single lines in every run.
- Not reproduced: a path longer than the width (no name here was long enough to be cut off), so the wrapping, not truncation, is what was seen.

### Long plan, plan text above the rule [run]

Evidence: `cap_plan_w80l.txt`, `cap_plan_a120.txt`, `cap_plan_a160.txt` (45-step plan, 40-row pane).

- The plan text is on screen above the rule, but **only its first part**: about 14 lines at 80 columns (steps 1 to 5), 17 lines at 120 (steps 1 to 5) and 20 at 160 (steps 1 to 13). A `↓` marker sits at the right edge of the last plan row at 80 and 120 columns when the plan is cut off. The rest is not on screen; the full plan is only in the plan file (and in `tool_use.input.plan` of the second `ExitPlanMode` call, as seen after feedback).
- A short plan (3 bullets) showed in full at all three widths.
- With a taller pane the visible plan region is taller; not measured.

### Digit keys [run]

| Dialog | Key | Effect | Evidence |
|---|---|---|---|
| Plan | `1` | approves at once, status line `auto mode on`, no Enter | `cap_plan_digit1.txt` |
| Plan | `2` | approves at once, status line `manual mode on` | `cap_plan_digit2.txt` |
| Plan | `3` | moves to the feedback text box only | `cap_plan_digit3.txt` |
| Read outside the project (default permission mode) | `2` | selected `Yes, allow reading from <dir> during this session` at once; the read ran and Claude answered `secret line one`; status line `manual mode on` | `cap_read_outside.txt`, `cap_read_outside_digit2.txt` |
| Read outside the project | `3` | `No` at once: `Interrupted · What should Claude do instead?` | `cap_read_outside_digit3.txt` |

Read-outside dialog:

```
 Do you want to proceed?
 ❯ 1. Yes
   2. Yes, allow reading from <dir> during this session
   3. No
 Esc to cancel · Tab to amend
```

The first row (`Read(<path>)` header) wrapped across lines at 120 columns because the path was long; the dialog shape is unchanged. Digit `1` in this dialog was not pressed (1, 2 and 3 are the only rows; 2 and 3 confirmed).

## Could not determine

- Whether `--no-daemon` or the dialog differs on Codex 0.167.0.
- What `3. Cancel` in the daemon dialog does to the process (not pressed).
- A note and a digit in **one** keypress sequence on the same question: not possible (section 3), not a probe failure.
- A plan-file path too long for 80 columns to wrap into a cut-off: none produced.
- Plan-dialog behaviour in a taller terminal than 40 rows, and with plan-mode variants where the first option differs (the user's settings made the first row the auto-mode label).
- Esc and `shift+tab` in the plan dialog were not run.
