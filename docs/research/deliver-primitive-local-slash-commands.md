# What local slash commands do on screen and in the transcript

Ticket: `gx:deliver-primitive/02`. Question: what do `/model`, `/help` and `/compact` do on the screen and in the transcript JSONL, so `Deliver`'s slash policy can decide "landed" without a second Enter running the command twice.

## Method

- Probe: `claude --model haiku` (Claude Code v2.1.330, Haiku 5.5, auto mode) in a fresh scratch dir, inside a detached `tmux` (120x40). Keys sent with `tmux send-keys`, frames read with `capture-pane -p` (plain) and `-e` (styled). Transcript read from `~/.claude/projects/<slug>/<session>.jsonl` with `jq`. Not a gx or existing Claude session.
- Code read: `agentrunner/claudeui`, `harness/typing.go`, `server/sessions/prompt/{action,typing,firstprompt}.go`, `testutil/agentfake/tuiclaude`.
- "Probe" cites below are my own captures from that run (not saved as files). The scratch dir and tmux server are removed.
- One extra check: I ran the real `harness.ClaudeTyping().InputIsEmpty` from a throwaway Go module outside the repo (`replace` to this worktree) on frames built from the captured styled rows. No repo file was changed.

## Answers

### 1. Does Claude write a transcript record for a local command?

Yes, but late. It is not a usable early "landed" signal.

- `/model` then Esc wrote two `system` / `local_command` records with the same timestamp: `<command-name>/model</command-name>...` and `<local-command-stdout>Kept model as ...`. (Probe: records at 13:55:38.261Z, the moment of Esc.)
- `/help` then Esc: same shape, `<command-name>/help</command-name>` and `<local-command-stdout>Help dialog dismissed</local-command-stdout>` (13:55:51.532Z).
- While the `/model` or `/help` dialog is open the file does not change. I counted JSONL lines with the `/help` dialog open: 7 before, 7 after 1.5 s open; 9 only after Esc. So no record exists while the command is "running".
- `/compact`: three `user` records: a `<local-command-caveat>`, `<command-name>/compact</command-name>...`, and `<local-command-stdout>Compacted ...`, plus a `system` / `compact_boundary` (`compactMetadata.trigger = "manual"`) and a `isCompactSummary` user record. The command record's own timestamp is the Enter time (13:57:24.253Z), but the line is not on disk until the compaction ends. Poll test: count of user records starting with `<command-name>/compact` stayed 3 for 7 polls (about 5 s of compaction) and became 4 at the same time as `compact_boundary` (13:57:58.413Z). Compaction took 5 s here (`durationMs` 5233 in the first run).
- Consequence: the transcript confirms a local command only after it ends (dialog closed, compaction done). `LaunchedBy` (`agentrunner/claudeui/launchproof.go:159`) already matches the `<command-name>` tag, so it works for these after the fact. It cannot tell a swallowed Enter from a slow compaction in the first seconds.
- Dialog commands (`/model`, `/help`) never get a record until the person (or gx) dismisses the dialog. A policy that waits for the record before it stops would wait forever.

### 2. Does the input box hide or redraw while the command runs?

It depends on the command. Three behaviours seen:

- Dialog commands (`/model`, `/help`): after Enter the input box is gone. The screen shows the echoed command at column 0 (`❯ /model`), one rule, and the dialog (`Select model ...`, `Help ...`). There is no input row between two rules. (Probe screens "Enter+0.3s" and "help+0.4".) After Esc, an `⎿  Kept model as Haiku 5.5` or `⎿  Help dialog dismissed` line appears under the echo and a fresh empty `❯ ` input box returns.
- `/compact`: the input box stays, empty, the whole time. The screen shows the echo (`❯ /compact`), a spinner row (`✶ Compacting conversation… (3s · ↓ 289 tokens)`), a tip row, then the rule / empty `❯ ` / rule box. Footer adds `esc to interrupt`. (Probe frames every 0.4 s for 3.2 s.) When done: `✻ Conversation compacted`, `❯ /compact`, `⎿  Compacted (ctrl+o to see full summary)`, empty box.
- Typed but not sent: input row `❯ /compact`, rules above and below, and a suggestion menu under it.

So an undecided read can mean "ran" (dialog: no input row) or "still typed" (menu open). The map's rule "our text gone and the screen changed means landed" holds for all three: after Enter the typed text is no longer in the input row.

### 3. Can gx read the input line on its own?

Yes, with `harness`, not with the screen-text helpers.

- `harness.Typing.InputIsEmpty(styledRows)` (`harness/typing.go:29-32`, `inputState` at `:166`) finds the last row whose first cell is `❯` (`lastInputRow`, `:153`) and judges draft text by style: a non-space rune that is neither dim nor reverse video is a draft (`hasDraftText`, `:210`). It needs styled rows (`StyledScreenRead`), else `InputUndecided`. Users: `server/sessions/prompt/typing.go:90-106` (`InputState`, `inputRead`) and `nativerunner/pty_personinput.go:213`.
- Suggestion menu rows are not mistaken for the input: the menu row is `  ❯ /compact ...` with two leading spaces (probe styled capture), so the first cell is a space, not `❯`.
- I ran the real function on frames rebuilt from the probe's styled rows:

  | Frame | Verdict |
  |---|---|
  | `/compact` typed, menu open | NotEmpty (correct) |
  | `/compact` running, empty box below echo | Empty (correct) |
  | slash text only in scrollback echo, empty box | Empty (correct) |
  | dialog dismissed, empty box | Empty (correct) |
  | `/help` dialog open (echo at col 0, no input row) | **NotEmpty (wrong)** |

- The wrong row is a real hazard. While a dialog is open the only column-0 `❯` row is the echo of the command, drawn styled `\x1b[38;5;246m\x1b[48;5;237m❯\x1b[39m \x1b[38;5;231m/help` (text colour 231, not dim, not reverse). `hasDraftText` reads it as a draft, so `InputIsEmpty` says `InputNotEmpty` on a dialog screen. `PressEnterIfStillTyped` (`server/sessions/prompt/typing.go:121`) and `nudgeEnter` (`agentrunner/claudeui/nudge.go:261`) both press Enter on `NotEmpty`. In the `/model` dialog Enter saves the model; in other dialogs it picks the highlighted option. So the existing reader needs a "this is an input box" gate before it can be the slash policy's "our text still in the input" check.
- A usable gate exists: `claudeui.InputBox(rows)` (`agentrunner/claudeui/screenmatch.go:32`) requires a `❯` row between two rules. By reading the code (not run), it takes plain rows and says no for the dialog frame (one rule only) and yes for typed, running-`/compact` and idle frames. The echo row has no rule under it, so it is not taken for the box. Combining it with `InputIsEmpty` (same frame, same read) would separate "input box present, text in it" from "dialog".
- Text-contains checks are weaker. `holdsText` (`agentrunner/claudeui/nudge.go:102`) and `draftIsOurs` (`:96`) search the whole `Screen(0)` for the first 20 characters of the text. After a successful `/compact` or `/help` the echo `❯ /compact` is on the visible screen, so these say "text still there". Only the styled input-row check, gated by `InputBox`, avoids that. `screenShowsPrompt` in `server/sessions/prompt/firstprompt.go:699` has the same weakness (whole-screen substring).
- `agentrunner/screen` offers no input-row reader. It gives rows, styled rows, scrollback and cursor (`agentrunner/screen/screen.go:241-264, 311`). The cursor position could pin the input row, but nothing uses it for that today.
- The fake claude (`testutil/agentfake/tuiclaude/tui_state.go:330-368`) does not model any of this. It echoes slash text as `> ` (not `❯`), draws no dialog frame without an input row for `/model`, shows no suggestion menu, and `/compact` goes Busy with no input box (`tui_prompt.go:265`, `:395-404`). A table test with only the fake will not reproduce the dialog false positive or the "box stays during compaction" case. Fixtures from this probe would.

### 4. How does `/compact` act when its first Enter is swallowed, and when gx presses Enter again?

- Swallowed first Enter, reproduced: I pasted `/compact` plus U+2028 as a bracketed paste. The input showed `❯ /compact` (U+2028 still in the styled row), no menu. The first Enter did nothing visible: after 0.6 s the row was `❯ /compact`, the U+2028 was gone from the row, footer showed `ctrl+g to edit in Vim`, and nothing ran (no spinner). The second Enter started compaction at once (`Compacting conversation… (0s ...)`). One `compact_boundary` followed. So the swallow is real for a slash command, and a second Enter on "our text, input not empty" is correct. (The "Removed 1 invisible character" notice was not on screen at my 0.6 s read; the stripped text and the swallowed Enter are what I observed.)
- Two Enters sent together after typing `/compact` (`send-keys Enter Enter`): one compaction, one `compact_boundary`. The second Enter arrived on the empty box and did nothing.
- A late Enter (about 1.4 s into compaction, while the spinner ran): still one compaction, nothing queued. The input box was empty, so Enter had no effect. A third Enter pressed about 1 s into the pasted case behaved the same.
- So a repeated Enter on `/compact` is safe only because the box is empty after it lands. The risk is a wrong "still typed" read. During `/compact` the read is `Empty` (see table), so a gated `InputIsEmpty` does not press again. The danger case is the dialog commands (item 3), where a repeat Enter acts on the dialog.
- I did not test `/compact` with its Enter swallowed twice in a row, or with the text over 512 bytes, or typed keys (not a paste) with U+2028.

## Not found / not tested

- Which character triggered the real "Removed 1 invisible character" notice in the failing chat send: not tested (needs that prompt).
- Whether Claude warns for typed keys vs. pastes: not tested; the swallow here was a paste.
- `/clear`, `/rename`, `/exit`: not probed.
- The resize settle time: not part of this ticket (see `docs/research/deliver-primitive-resize-settle.md` if present).

## What this means for the slash policy

- Transcript is a confirmation after the fact (command ends), never the first signal. Do not wait on it for dialog commands.
- "Our text still in the input" must be decided from the styled input row, gated by `claudeui.InputBox`, never from a whole-screen substring and never from a `❯`-at-column-0 row alone.
- During a dialog the input row does not exist: read as "undecided or landed", press nothing. This matches the map's plain-screen rule.
- During `/compact` the box stays and reads Empty. Landed.
