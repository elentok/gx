# Claude AskUserQuestion dialog facts, confirmed by real runs

Ticket `gx:regular-questions/05`. Extends `regular-questions-claude-dialog.md` (01) and `regular-questions-dialogs.md` (03).

Method: **Claude Code 2.1.319**, Haiku 5.5, interactive TUI in tmux (`send-keys`), scratch cwd only, `--settings <file>` per run, hook scripts logging stdin. Pid status read from `~/.claude/sessions/<pid>.json` (polled every 50 ms with the screen). Tags: **[run]** observed, **[docs]** code.claude.com tools-reference, **[changelog]** `anthropics/claude-code` CHANGELOG.md, **[unverified]**. All runs used the same 3-question set (Color single, Toppings multi, Name single) unless stated.

## Answer in short

- A PreToolUse hook answers 2+ questions, multi-select, "Other" free text and `annotations`. A hook that prints nothing and exits 0, or exits 1 (or any non-2 non-zero, e.g. a crash), shows the normal dialog. Exit 2 blocks the tool call and Claude sees the stderr.
- With no hook (disabled or absent) the dialog shows and the pid file reads `waiting` / `input needed` at the same poll tick the dialog is drawn, with no gap with no status.
- The fallback can tell AskUserQuestion apart from slash-command pickers by `waitingFor`: pickers report **`"dialog open"`**, the question dialog reports **`"input needed"`**, tool and plan permission report `"permission prompt"`.
- Digit keys work in this dialog but behave differently per screen (table below). Review and single-select digits act at once with no Enter.

## 1. Hook with several questions [run]

Stdin: `tool_name: "AskUserQuestion"`, `tool_use_id`, `session_id`, `transcript_path`, `cwd`, `permission_mode`, `tool_input.questions[]` (`question`, `header`, `multiSelect`, `options[]{label,description}`), plus `prompt_id`, `effort`. No "Other" option in `options`.

Output used: `hookSpecificOutput{hookEventName:"PreToolUse", permissionDecision:"allow", updatedInput:{questions:<original>, answers:{...}}}`.

- `answers` is a flat map keyed by the **exact question text**; every value a string.
- Multi-select: labels joined `", "` (`"Cheese, Egg"`). Accepted and shown.
- "Other": any string that matches no label (`"my free text answer"`) is accepted as is.
- `annotations`: accepted. `annotations: {"Which color?": {"notes": "hello note"}}` is kept in `toolUseResult.annotations` and Claude's tool result reads `"Which color?"="Blue" notes: hello note`. Keyed by question text, `notes` field. (`preview` not tried.)
- Tool result text to Claude via hook: `The user answered: "Q"="A", ... Read the answers carefully ...`. The dialog itself produces `Your questions have been answered: ...` when every answer is a picked option, and the same `The user answered:` wording as the hook when any answer was typed free text [run; docs: "neutral wording"].
- Hooked turn shows no dialog (03/01 finding, unchanged).

## 2. Hook failure [run]

Simulated "gx server down" with hook variants (a real dead port gives the same exit paths):

| Hook | Result |
|---|---|
| prints nothing, exit 0 | normal dialog shown, pid `waiting`/`input needed` |
| exit 1 with stderr | normal dialog shown (non-blocking error) |
| crash with exit 5 (a jq error) | normal dialog shown |
| exit 2 with stderr | tool call blocked, no dialog; pid goes idle; Claude is told `PreToolUse:AskUserQuestion hook error: ... <stderr>` and reports it |

So the fail-open rule for the gx hook: on any server error print nothing and exit 0 (or exit 1). Never exit 2.

## 3. Hooks disabled [run]

`disableAllHooks: true` with the hook still in the settings: the hook never runs (no stdin log), the dialog shows. A launch with no hook entry is the same screen. `allowManagedHooksOnly` needs a managed-settings file under `/Library`; **not run** (would need root). The changelog treats it with `disableAllHooks` as the "hooks off" pair [changelog 2.1.x `/goal` fix], so expect the same.

Pid timeline (50 ms poll, `idle` before the prompt): idle 0 s, `busy` at 1.8-2.0 s (prompt submitted), `waiting`/`input needed` at 4.3-4.7 s, and the screen showed the dialog in the same poll tick. The file never vanished or lacked a status. A second repeat gave 4.26 s for both. So "waiting" is a reliable trigger and the screen is ready when it flips. After the answer or Esc: `busy` then `idle`.

## 4. Which screens report `waiting` [run]

| Screen | `status` / `waitingFor` |
|---|---|
| AskUserQuestion dialog | `waiting` / `input needed` |
| Tool permission prompt (Bash, forced by an `ask` rule) | `waiting` / `permission prompt` |
| ExitPlanMode plan approval | `waiting` / `permission prompt` |
| Slash pickers and panels: `/model`, `/config`, `/resume`, `/theme`, `/permissions`, `/mcp`, `/status`, `/help`, `/rewind`, `/hooks`, `/plugin`, `/memory`, `/login`, `/effort`, `/skills`, `/usage` | `waiting` / **`dialog open`** |
| `/agents`, `/output-style`, `/terminal-setup`, `/context` | `idle` (inline, no dialog state) |
| `/doctor`, `/export`, `/feedback`, `/btw` | `busy` |
| Startup trust-this-folder | shown before any session reached `idle`; cursor starts on **`No, exit`** (not on Yes) [run]. Not pid-classified. |

03 assumed any `waiting` other than `permission prompt` is `question`; that is wrong for `dialog open`. Today's `claudepid.go` maps it to `question`, so every picker would look like a question card. The fallback must require `waitingFor == "input needed"` and then verify the screen. MCP elicitation and server-approval dialogs were **not** run [unverified].

## 5. Keystroke path, no hook [run]

Screens (120 cols, 3 questions). Tab bar `←  ☐ Color  ☐ Toppings  ☐ Name  ✔ Submit  →` (`☒` once answered, `☐` unanswered), then the question, rows, a rule, `N. Chat about this`, footer `Enter to select · Tab/Arrow keys to navigate · Esc to cancel`.

Single-select (`❯` on the current row, each option followed by an indented description row):
```
Which color?
❯ 1. Red
     Red
  2. Blue
  ...
  4. Type something.
────
  5. Chat about this
```
Multi-select (a checkbox on each row, `Type something` has a checkbox, a row below it is the advance row):
```
Which toppings?
❯ 1. [ ] Cheese
         Cheese
  ...
  4. [ ] Type something
     Next
────
  5. Chat about this
```
The advance row is labelled **`Next`** on a non-last question and **`Submit`** on the last question (2-question run). `Next`/`Submit` has no number and sits between row 4 and the rule.

Review screen (2+ questions; an answered single shows `→ Beta`; multi shows `→ Egg, Sausage`):
```
Review your answers
 ● Which color?
   → Purple
 ...
Ready to submit your answers?
❯ 1. Submit answers
  2. Cancel
```

Key effects:

| Key | Effect |
|---|---|
| Single-select `Enter` | picks the row under `❯`, tab becomes `☒`, advances to next question (first row `❯`) |
| Single-select `Down` x N then `Enter` | works; `❯` starts on row 1 on every new question |
| Multi-select `Enter` on an option | toggles `[✔]`, stays on the row |
| Multi-select `Down` to the `Next`/`Submit` row, `Enter` | advances (with nothing checked it still advances) |
| `Down` past the last row | goes to `N. Chat about this`; `Up` from row 1 of a 4-row multi-select wrapped to row 4 (`Type something`) |
| "Other", single-select: `Down` to `Type something.` and type | row becomes the typed text (`❯ 4. Purple`); `Enter` submits it and advances. Footer gains `ctrl+g to edit in Vim` |
| "Other", multi-select: `Down` to row 4 and type | typing **auto-checks** the row (`[✔] Sausage`). **`Enter` then unchecks it** (trap); a second `Enter` rechecks. Move `Down` to `Next` and `Enter` without pressing Enter on the row |
| Typing while the cursor is not on the Other row | ignored |
| Review `Enter` / `1` | submit; `Up` wraps to `2. Cancel`; `Enter` there cancels (tool_result `is_error`, "The user doesn't want to proceed", then idle) |
| `Esc` | cancels like Cancel |

The footer is **not** a cursor signal: `ctrl+g to edit in Vim` stays once the input row was visited, including on `Next` and `Chat about this`. Verify the cursor from the `❯` row.

Final answers (Other typed in Q1 and Q3, `Sausage` in Q2): `answers` = `{"Which color?":"Purple","Which toppings?":"Ham, Sausage","Which name?":"Gamma"}`, `annotations {}`. A typed Other in a multi-question dialog works, in single and multi-select.

Single question: one `Enter` on the answer submits with no review screen (01); with 2 questions the review screen exists.

### Long, wrapped, narrow [run, 80 cols, long labels/descriptions]

- Question text wraps with a `│ ` gutter on every row (`│ Which deployment strategy ...`). Review shows the same gutter on the question (` │ ● Which ...`, ` │   the constraints`).
- Option labels wrap with the continuation indented under the label text (5 spaces single-select, 9 spaces multi-select), no number on continuation rows. Descriptions follow, wrapped and indented the same. A parser that stops at a non-`N.` row loses options (same failure as 03's wrapped permission rows).
- Review answers wrap: `   → Enable verbose ... window` then `   and keep it, Freeze unrelated deploys ...`; multi values run together across wrap.
- At 80x50 the whole 2-question dialog fit (about 27 rows). At 80x16 the list scrolls: the first/last hidden option rows are replaced by markers, `↑ 2. Short` and `↓ 4. Canary` (the `❯`/space column holds `↑` or `↓`), the question, tab bar and the row with `❯` can be scrolled off screen, and the footer wraps (`...Esc to` / `cancel`). The cursor row is not always visible in the captured screen, so a per-step verify cannot rely on `❯` plus the question being in view; it must treat "no `❯` row visible" and `↑`/`↓` markers as a mismatch (or enlarge the PTY first).
- 120 cols: no wrapping with short options; footer on one line.

### 60 s `askUserQuestionTimeout` [run + docs]

Set via `--settings` (`"askUserQuestionTimeout":"60s"`; also user settings or `/config` [docs]). In tmux with no focus reporting it counted down. About 42 s after the last key a right-aligned `auto-continue in 17s · any key ...` appeared on the line below the footer, counting 17, 11, 5; at about 60 s the dialog closed. Pid `waiting` -> `busy` (59.6 s) -> `idle` (62.6 s). Selected rows (Red, Egg) were submitted, unanswered Name was not. Claude got `No response after 60s — the user may be away from keyboard ... Before going idle the user had selected: "Which color?"="Red", "Which toppings?"="Egg".` `answers` held only the two. Any key restarts the timer [docs]. Not started in background sessions, with Remote Control, or in screen reader mode [docs]; default is no timeout [changelog 2.1.200]. So a stuck read can race a closing dialog: re-verify before every key.

## 6. Digit keys [run]

| Screen | Digit effect |
|---|---|
| Single-select, option row `N` | selects N and advances at once (no Enter) |
| Single-select, the `Type something.` row number (4) | moves `❯` to the input row; does not advance; type, then `Enter` |
| Multi-select, option row `N` | toggles N, cursor does not move |
| Multi-select, `Type something` number | checks the empty Other row (`[✔] Type something`) |
| Multi-select, a digit while on `Next` row | ignored |
| `N. Chat about this` number (5) | ends the dialog at once; the turn goes `busy` with Claude chatting |
| Review `1` / `2` | submit / cancel at once |
| Permission prompt (03) | selects directly with no Enter |

Same family as Permission dialogs (digit acts immediately), but here the effect depends on single versus multi and the Other row: do not treat a digit as "pick option N" without knowing the screen kind.

## 7. Version [changelog]

- Hooks `updatedInput` with `permissionDecision: "allow"` satisfying `AskUserQuestion`: added in **2.1.85**, worded "enabling headless integrations". It does not say interactive. Only 2.1.319 was run, so **interactive support on 2.1.85 is unverified**; the 01 interactive result is on 2.1.319.
- Also: multi-select answers as an array were discarded until **2.1.136** (send strings); typed Other dropped in multi-select until **2.1.181**; free-text answers get neutral wording since **2.1.216**; no auto-continue by default since **2.1.200**; single-select last question auto-submit (no review) since **2.0.55**; `waitingFor` appears in `claude agents --json` since **2.1.162**.
- 2.1.321 changelog: `claude -p`/SDK now denies a host answer that sets the undocumented `afkTimeoutMs`; Cowork cloud sessions ignore PostToolUse answer replacement. Not relevant to PreToolUse in gx.

## Implications for the design

1. Fallback trigger: `waiting` and `waitingFor == "input needed"`, then require on screen: tab bar row, `Chat about this`, footer. `"dialog open"` is a picker, never a question.
2. Per-step verify: re-read the screen, find the `❯` row and its number, the tab bar state (`☒`/`☐`), and the footer; refuse on any `↑`/`↓` scroll marker, missing `❯`, or unknown layout. Prefer a tall PTY.
3. Sequence for a multi-select Other: type text with the cursor on the Other row; do not `Enter` it; `Down` to `Next`/`Submit`.
4. Close and race: the 60 s timer, Esc in the terminal, and the hook's SIGTERM can all end the dialog; watch the pid status leave `waiting`.
5. Hook: fail open (print nothing, exit 0).

## Not run

- `allowManagedHooksOnly` (needs managed settings), MCP elicitation and server-approval dialogs, a settings merge that drops the hook (equivalent to no hook), Claude older than 2.1.319, the hook's `preview` annotation field, a real dead-port server (exit paths simulated).
