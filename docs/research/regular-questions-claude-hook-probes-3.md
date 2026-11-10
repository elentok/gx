# Claude AskUserQuestion hook probes, round 3 (auto mode, deny)

Ticket `gx:regular-questions/10`. Follow-up to `regular-questions-codex-claude-probes-2.md` (09), whose fake hook and local server it reuses.

Version: **Claude Code 2.1.319**, Haiku 5.5, tmux 120x40, macOS, scratch cwd under the session scratchpad (never the repo). Every run was `claude --permission-mode auto --model haiku --settings <hook file>`, the mode gx adds by default. The hook's own stdin confirmed `permission_mode: "auto"`. Tag **[run]** = seen in a real run. Captures: `/private/tmp/claude-501/-Users-david-dev-gx-regular-questions-design/b9e43643-b042-456b-923b-599cd7db1cf5/scratchpad/cap10_*.txt` and the session transcripts under `~/.claude/projects/*b9e43643*work-cl/`. The fake hooks (`allow` = ticket 09's `hook.py` posting to a local server, `deny`, `exit2`, `silent`) are in `scratchpad/fake/`.

## Answer in short

- **Allow + `updatedInput.answers` works unchanged in auto mode [run].** Three questions (a multi-select, "Other" on a single-select, "Other" appended to the multi-select), plus `annotations` on all three. The dialog never showed. The TUI printed `User answered Claude's questions:` with the three answers (no notes). Claude received one `tool_result` (not an error), and read the notes back. The pid file read `idle` afterward.
- **A hook that prints nothing still shows the dialog in auto mode [run].** Same dialog as without auto mode (`1. Red`, `2. Blue`, `3. Type something.`, `4. Chat about this`). The pid file read **`waiting`** while it was open.
- **`permissionDecision: "deny"` blocks the call, shows no dialog, and Claude continues the turn with text, not a re-ask [run, 2 of 2].** Claude sees an error `tool_result` whose text is `PreToolUse:AskUserQuestion hook error: <permissionDecisionReason>`. The turn ends with a plain-text reply in both runs, the pid file reads `idle`, and the TUI shows `⎿  Error: PreToolUse:AskUserQuestion hook error: <reason>`.
- The model's reply follows the reason text. With "The person dismissed this question from the web UI.", one run said it would not retry because the dismissal was deliberate; the other asked the question again in plain text (`Which would you like, Rex or Max?`). Neither called `AskUserQuestion` a second time.

## 1. Allow with 3 questions, "Other" and `annotations` [run]

Evidence: `cap10_allow.txt`, transcript `d081cac3-…jsonl`.

The server returned `answers` `{"Which color?":"teal-ish custom","Which toppings?":"Cheese, Egg, pineapple (other)","What name?":"Bob"}` (an "Other" on a single-select, and one string of two labels plus free text on the multi-select) and `annotations` `{"<q>":{"notes":...}}` on every question (`preview` on the toppings one), plus the original `questions`.

- Dialog hidden: yes. The prompt went straight to `User answered Claude's questions:` and the model's next text.
- Tool result Claude saw (`is_error` absent, `toolUseResult` carries `questions`, `answers` and `annotations` verbatim):
  `The user answered: "Which color?"="teal-ish custom" notes: NOTE-ONE-xyzzy, "Which toppings?"="Cheese, Egg, pineapple (other)" selected preview:\nPREVIEW-TWO notes: NOTE-TWO-plugh, "What name?"="Bob" notes: NOTE-THREE-quux. Read the answers carefully — they may request clarification, changes, or that you not proceed — and follow what they actually say.`
- The model quoted all three notes and the preview, and noted that "teal-ish" and "pineapple (other)" were not among its options. It took no action on the notes.
- Pid file: `status: idle` after the turn. It never showed `waiting` during the hook (the run was not sampled mid-hook; the hook answered at once).
- Same shape as ticket 09's probes: auto mode changed nothing.

## 2. Silent hook (fail-open) in auto mode [run]

Evidence: `cap10_silent.txt`, `cap10_silent_esc.txt`.

The hook read stdin and printed nothing, exit 0. The three-question dialog opened as it does without a hook. The pid file read `status: waiting`. Esc closed it: TUI `User declined to answer questions` with the three question headers, `Crunched for 2s · done`, pid `idle`.

## 3. Deny, exit 2 and Esc compared [run]

Evidence: `cap10_deny.txt`, `cap10_deny2.txt`, `cap10_exit2.txt`, `cap10_silent_esc.txt`; transcripts `af3cc97f-…`, `ac25c155-…`, `b1974c29-…`.

Hook output for deny: `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"The person dismissed this question from the web UI."}}`, exit 0.

| Path | Dialog | `tool_result` text Claude sees | `is_error` | Claude's next step | Pid after |
|---|---|---|---|---|---|
| Hook `deny` + reason | none | `PreToolUse:AskUserQuestion hook error: The person dismissed this question from the web UI.` | true | text reply, turn ends (2 of 2 runs, no second tool call) | `idle` |
| Hook exit 2, stderr text | none | `PreToolUse:AskUserQuestion hook error: [<hook path>]: blocked by exit 2` (the stderr text; the hook's path is prefixed) | true | text reply that blames the hook, turn ends | `idle` |
| Esc on the dialog (no hook decision) | shown, then closed | `The user doesn't want to proceed with this tool use. The tool use was rejected (eg. if it was a file edit, the new_string was NOT written to the file). STOP what you are doing and wait for the user to tell you how to proceed.` (`toolUseResult: "User rejected tool use"`); the TUI line is `User declined to answer questions` | true | no further model turn | `idle` |

- `deny` and exit 2 look alike to Claude: an error naming a hook. The model reads the error text, so the **reason string steers what it says**. Esc is different: its text tells the model to STOP and wait, and the model says nothing more.
- A `deny` is therefore not the same signal as Esc. A dismiss that should read as "the person declined, wait for them" could use a reason phrased the same way (`The user doesn't want to proceed ... wait for the user to tell you how to proceed.`). That wording was not run.
- All three leave the session `idle`, so gx's own idle detection ends the "asked" state.

## Could not determine

- What the pid file shows while the allow hook is mid-flight (every allow ran too fast to sample).
- Whether a different reason string makes Claude re-call the tool (two reasons were not compared; only the one string was run, twice, on Haiku).
- Behavior on other models, or on versions other than 2.1.319.
- gx's real hook and server were not run; the fake follows the documented contract.
