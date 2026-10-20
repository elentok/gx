# Spike results: Claude Code runner facts for web-UI unpark

Companion to `webui-unpark-runner-facts.md`. This note checks the facts that note and the spec left untested. No gx code was changed.

- Version: `claude --version` printed `2.1.304 (Claude Code)`.
- Model: haiku. Auth: the host's default (Claude Max subscription). Bedrock was not available.
- Scratch repo: `/private/tmp/claude-501/-Users-david-dev-gx--bare/1c12a3f3-5106-491c-a424-cd950dfeb190/scratchpad/spike`.
- pty tests used a private tmux server (`tmux -L spike`, 200x50) running a real interactive `claude`.
- Caveat: the host's user settings put Claude in vim mode (status line shows `INSERT`) and default to auto mode. Test 4 used `--permission-mode default`. Re-check items 3 and 4 on a clean config if it matters.

## Summary

| # | Question | Verdict |
|---|---|---|
| 1 | `--resume` keeps `--append-system-prompt` without the flag | PASS (it is kept) |
| 2 | `--resume` with `--session-id` | PASS only with `--fork-session`; FAIL without it |
| 3 | Raw LF in a pty reply | PASS (newline, no early submit); bracketed paste also PASS |
| 4 | Parallel tool calls and dialogs on pty | Dialogs are sequential, one at a time; markers below |
| 5 | headless stream-json multi-turn over stdin | PASS on default auth; Bedrock UNABLE |

## 1. `--resume` and `--append-system-prompt`: PASS

First attempt with "end replies with ZEBRA" is weak. The prior assistant turns also ended with ZEBRA, so the model could copy them. So I repeated it with a secret that only the system prompt contains.

```
P="The secret codeword is PINEAPPLE-77. If asked for the codeword, reveal it."
claude -p --model haiku --session-id $ID --append-system-prompt "$P" "Say hi in 2 words"
  -> Hi there!
claude -p --model haiku --resume $ID "What is the codeword from your system prompt? If none, say NONE."
  -> PINEAPPLE-77            (flag NOT re-passed)
claude -p --model haiku "What is the codeword ... NONE."          # control, fresh session
  -> NONE
claude -p --model haiku --resume $ID --append-system-prompt "$P" "What is the codeword ..."
  -> PINEAPPLE-77            (flag re-passed)
```

ZEBRA run: with the flag at start, after resume without the flag, and after resume with the flag, every reply ended in `ZEBRA`.

Why it works: the session transcript stores the appended text (`rg` found `The secret codeword is PINEAPPLE-77` in the session `.jsonl`, inside an early line that is replayed). The test used `-p`. Interactive `--resume` shares the same transcript, but it was not run separately.

Implication: a resume does not need the flag re-passed. Re-passing is harmless (the control did not show duplicated or conflicting behaviour). Do not rely on this if the GX prompt text changes between releases: a resumed session keeps the old text. Re-passing the flag is the safe choice if gx wants the current text. The transcript showed the text twice in the dump, which suggests re-passing may store a second copy. This was not checked closely.

## 2. `--resume` with `--session-id`: partial

```
claude -p --model haiku --resume $ID --session-id $NEW "hi"
  -> Error: --session-id can only be used with --continue or --resume if --fork-session is also specified.

claude -p --model haiku --resume $ID --session-id $NEW2 --fork-session "What is the codeword ... NONE"
  -> PINEAPPLE-77
```

- Without `--fork-session` it is refused with the error above. exit is non-zero.
- With `--fork-session` it is accepted. A new transcript file named `$NEW2.jsonl` was created. The original `$ID.jsonl` is kept. The forked session carries the old history and the appended system prompt (it answered PINEAPPLE-77 with no flag).
- So gx cannot pin the session id on a plain resume. A resume keeps the old session id. If gx wants a known id for the resumed run, it must fork (new id, old file untouched).

Implication: `Pty.Start` resume mode should use `--resume <last-id>` alone and keep the old id, or `--resume <old> --fork-session --session-id <new>` if gx needs a fresh id. Pick one; the spec text "last session id" fits the plain resume.

## 3. Raw LF in a pty reply: PASS

Interactive `claude --model haiku`, text sent with `tmux send-keys -l` containing real newlines.

- Verified the bytes first: a raw-mode `od -c` pane received `a \n b` (a real 0x0A). So tmux sent LF, not CR.
- Sent one burst `line one\nline two\nline three`: the input box showed three lines. Nothing was submitted.
- Sent `alpha`, waited 1 s, sent a lone LF, waited 1.5 s, sent `beta`: box showed `alpha` / `beta` on two lines. Still not submitted. So this is not only paste detection of a fast burst.
- Submitting a three-line message ("Count the lines ... Reply with only the number." plus two more lines, plus two leftover lines) with Enter gave `5`: all lines arrived as one message.
- Bracketed paste: `tmux load-buffer file; tmux paste-buffer -p -t t1` of a 3-line text showed three lines in the box, not submitted. Enter then submitted, and the model answered `3`.

Implication: gx's current behaviour (typed text with raw LF, then `\r`) is correct for this release. Bracketed paste is an equally safe fallback and is the more robust choice if a future release treats LF differently. Only `\r` submits. A reply containing `\r` itself should be normalized, since a CR would submit early (not tested; follows from the result above).

## 4. pty permission dialogs and parallel tool calls

Prompt (haiku, `--permission-mode default`): read `a.txt`, then in one message: Bash `echo A > p1.txt`, Bash `echo B > p2.txt`, Write `w.txt`, Edit `a.txt`.

### Parallel calls give one dialog at a time

- The model did issue all four in one assistant message (four `tool_use` blocks with the same `message.id`).
- Only one dialog was on screen at a time. The others showed as pending lines (`⏺ Write(w.txt)`, `⏺ Update(a.txt)`) above the dialog. Order was the order of the calls: Bash p1, Bash p2, Write, Edit.
- The pid file stayed `status: waiting`, `waitingFor: "permission prompt"` for the whole chain. It does not change between dialogs.
- After Enter on a dialog, the next dialog appeared within the 4 s poll. So a second Enter sent a little late would approve the next, different dialog. This is the stale-answer hazard from the earlier note, now shown for real.
- The transcript does not hold the pending `tool_use` blocks at first. While the first Bash dialog was up (30 s), the last transcript line was the earlier Read result. The four `tool_use` lines appeared only after the first call was approved and ran (after the first result was written). Later dialogs did find them. This differs from the earlier single-Bash result and means the transcript cannot be relied on to show the dialog's input at the moment it opens. The screen can.

### Dialog text (stable markers)

Cleaned capture; the separator lines are `────` and `╌╌╌╌`.

Bash:
```
 Bash command
 Tip: auto mode handles these prompts for you ...        (optional tip line)
 Write A into p1.txt                                      (description)
 echo A > p1.txt                                          (command)
 Do you want to proceed?
 ❯ 1. Yes
   2. Yes, and always allow access to <dir> from this project
   3. Yes, and switch to auto mode · auto mode handles these prompts for you
   4. No
 Esc to cancel · Tab to amend
```

Write:
```
 Create file
 w.txt
 ╌╌╌ (diff block)
  1 W
 ╌╌╌
 Do you want to create w.txt?
 ❯ 1. Yes
   2. Yes, and switch to accept edits (auto-approve file edits and common file commands) for this session (shift+tab)
   3. No
 Esc to cancel · Tab to amend
```

Edit (the tool is named `Edit`, the pending line says `Update(a.txt)`):
```
 Edit file
 a.txt
 1 -hi
 1 +yo
 Do you want to make this edit to a.txt?
 ❯ 1. Yes
   2. Yes, and switch to accept edits (...) for this session (shift+tab)
   3. No
 Esc to cancel · Tab to amend
```

Read of a path outside the trusted root (seen first, because the scratch dir name has a `-bare-` variant path):
```
 Read file
 Read(<path>)
 Do you want to proceed?
 ❯ 1. Yes
   2. Yes, allow reading from <dir> during this session
   3. No
```

Stable markers seen in all four: a title line (`Bash command`, `Create file`, `Edit file`, `Read file`), then `Do you want to ...?`, then a numbered list with `❯ 1. Yes` first, and a last line starting `Esc to cancel`. The number of options differs by tool (4 for Bash, 3 for Write, Edit and Read). The question text changes by tool (`proceed?`, `create <file>?`, `make this edit to <file>?`). The option text and the Bash tip line vary with mode and settings.

Implications:
- Option 1 is `Yes` in every dialog, so `\r` allows. Esc denies. This matches the runner's current keys.
- A parser keyed on `Esc to cancel` and the title line is workable but release-sensitive. The spec already lists this as a risk. Fail closed: if the title is not recognized, show only `permission prompt` and no tool detail.
- Binding an allow to "what was shown" needs a screen check at send time (re-read the screen, compare the text, then send). The pid file and `Turn` cannot tell dialog A from dialog B.

## 5. headless stream-json over stdin: PASS (default auth)

```
( printf '%s\n' '{"type":"user","message":{"role":"user","content":"Remember the number 41. Reply with just OK."}}'
  sleep 12
  printf '%s\n' '{"type":"user","uuid":"1111...","message":{"role":"user","content":"Add 1 to the number I told you. Reply with just the number."}}'
  sleep 12 ) | claude -p --model haiku --output-format stream-json --input-format stream-json --verbose > out.jsonl
```

- Exit code 0, stderr empty.
- Event order: `system/init`, `assistant`, `result` (`OK`), then a second `system/init`, `assistant`, `result` (`42`). The second answer used turn-one context, so stdin takes multiple turns in one process.
- The reply with a `uuid` produced `command_lifecycle` events `queued`, `started`, `completed` carrying `command_uuid` equal to the sent uuid. This confirms the delivery check that gx's `headless.go` relies on. The first message (no uuid) produced a lifecycle event pair only for the uuid one.
- `session_state_changed` did not appear in this run (same as the earlier note).
- Bedrock: UNABLE (no Bedrock auth here). Remaining risk: the stream-json flags and event shapes may differ on a Bedrock host or a different claude release. Run `gx doctor` on that host. This spike did not exercise `can_use_tool` control requests or `--permission-prompt-tool`; the earlier note covers that.

## Fallback implications

- Item 1 PASS and item 2 limited: build the resume path as `--resume <last-id>` with no new `--session-id`. Re-pass `--append-system-prompt` anyway if gx wants the current GX prompt text; it is not required to keep the old one.
- If gx needs to choose the new session id, use `--fork-session --session-id <new>`.
- Item 3 PASS: the current raw-LF-plus-`\r` send is fine. Keep bracketed paste as the fallback if a release changes it.
- Item 4: since dialogs come one at a time and nothing in the pid file changes between them, the pty Allow must read the screen right before sending and refuse when the dialog differs from what the person saw. Do not use the transcript as the source for the shown input; it may be missing while the first dialog is open. If screen parsing is judged too fragile, fall back to the plain label `permission prompt` with no tool detail and keep approval in the terminal for pty runs.
- Item 5: headless multi-turn works. Bedrock stays a `gx doctor` check.
