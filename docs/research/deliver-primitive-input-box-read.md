# Can gx read "our text in the input box" without the dialog echo fooling it?

Ticket: `gx:deliver-primitive/08`. Follows `gx:deliver-primitive/02` (`docs/research/deliver-primitive-local-slash-commands.md`).

## Answer

- No existing reader is right on every screen. Not one of `harness.Typing.InputIsEmpty`, `holdsText`, `draftIsOurs` or `screenShowsPrompt` alone, and not `InputIsEmpty` gated by `claudeui.InputBox`.
- `harness.Typing.InputIsEmpty` has a bug that ticket 02 did not see: it misreads a slash command that Claude highlights. A typed `/help` or `/model` that sits unsent in the box reads `InputEmpty`. That includes the exact swallowed-Enter screen. The gated read would never press Enter there.
- A combination that is right on all 14 screens: `claudeui.InputBox(plainRows)` as the gate, then the plain text of that box row starts with our text (first 20 chars) or holds `[Pasted text`. I call it `boxRowHolds` below. It uses no styling.
- The alternative: fix `applySGR` (skip the `38;...` / `48;...` colour arguments), then `InputBox && InputIsEmpty==NotEmpty`. Also right on all 14 screens. It needs a separate "is it ours" check, because NotEmpty does not say whose text it is.
- The `tuiclaude` fake models none of what these screens need (see the last section).

## Method

- Probe: `claude --model haiku` (Claude Code 2.1.330) in a fresh scratch dir, through a Python `pty` I control, 120x40. Env `TERM=xterm-256color`, `COLORTERM=truecolor`, the same pair the holder forces (`nativerunner/holder/holder.go:316-323`). Raw PTY bytes saved with a byte offset per step. Not a gx or existing Claude session.
- Ticket 02's captures were not saved to disk, so I captured again.
- Replay: a throwaway Go module (`replace github.com/elentok/gx => /Users/david/dev/gx/deliver-primitive-design`) feeds the bytes up to each offset into `agentrunner/screen.NewScreen(120, 40)`. Rows for the readers come from `Screen.Rows()` (plain) and `Screen.StyledRows()` (styled), the same calls gx's holder reads use. So the styled rows are what production sees, not a tmux `-e` re-encoding.
- Real, imported: `harness.ClaudeTyping().InputIsEmpty` (`harness/typing.go:117`), `claudeui.InputBox` (`agentrunner/claudeui/screenmatch.go:32`).
- Copied verbatim, because they are unexported: `holdsText` and `draftIsOurs` (`agentrunner/claudeui/nudge.go:96-109`; they live in `claudeui`, not in the prompt package) and `screenShowsPrompt` with `prefixRunes = 40` (`server/sessions/prompt/firstprompt.go:693-705`). `holdsText` and `draftIsOurs` get `Screen()` as the whole visible screen, joined with newlines, as `p.Screen()` returns at the call site (`agentrunner/claudeui/nudge.go:265`).
- "Fixed SGR" and `boxRowHolds` are my own throwaway code in the temp module, not repo code.
- The temp scripts live in `/private/tmp/claude-501/i08-rgha/` (`drive.py`, `mod/main.go`, `screens/*.txt|*.styled`). They may be cleaned up; the table and excerpts below are the record.
- No repo file was changed.

## The bug in `InputIsEmpty`

- `applySGR` (`harness/typing.go:257-277`) splits an SGR on `;` and switches on each number. It does not skip the arguments of `38` / `48`. In `38;2;R;G;B` the `2` sets `dim`; a component equal to `0`, `1`, `7`, `22` or `27` resets, bolds, reverses or clears something; `38;5;N` has the same hazard.
- Claude draws a valid slash command in the box as `\x1b[38;2;177;185;249m/help\x1b[m` (probe, `swallowed_after_enter` styled row 14). `hasDraftText` (`harness/typing.go:210`) sees `dim` and calls the row empty. Source of the bytes: my capture; source of the parse: `harness/typing.go:257-277`.
- Under the holder env Claude paints the dialog echo row as `\x1b[38;2;153;153;153;48;2;55;55;55m❯\x1b[39m \x1b[38;2;255;255;255m/help\x1b[39m` (probe, `help_open` row 9). The same bug reads that as dim too. So `InputIsEmpty` says `InputEmpty` on an open dialog, which is right only by accident.
- Ticket 02 reported the dialog as `NotEmpty`. That came from frames rebuilt from tmux `-e` output, which uses `38;5;231` (256-colour) and has no `2` argument. The holder never runs in that mode, so on the real screens the dialog is `Empty` and the typed-command case is the wrong one. Correction to ticket 02's table: dialog `NotEmpty` was an artifact of the capture format.
- Users hit by the bug: `PressEnterIfStillTyped` (`server/sessions/prompt/typing.go:105-120`), `nudgeEnter` (`agentrunner/claudeui/nudge.go:255-275`), `nativerunner/pty_personinput.go:224`. All three treat a coloured typed command as an empty box. I did not run those functions; the claim follows from the readers' results below.

## Screens

Every screen is one frame from the same session, replayed at the step's byte offset. Truth = is our text sitting in the input box right now.

| Screen | What it shows | Truth |
|---|---|---|
| `idle_empty` | box with `❯ `, no text | no |
| `typed_co_menu` | `/co` typed, suggestion menu below (menu row `  ❯ /copy ...`) | yes |
| `typed_model_menu` | `/model` typed, menu open | yes |
| `model_open` | echo `❯ /model` at col 0, one rule, `Select model` dialog, no input row | no |
| `model_landed_empty` | after Esc: `⎿  Kept model as Haiku 5.5`, echo in scrollback, empty box | no |
| `help_open` | echo `❯ /help`, `Help` dialog, no input row | no |
| `help_landed_empty` | after Esc: `⎿  Help dialog dismissed`, empty box | no |
| `after_chat` | a plain chat turn done, empty box | no |
| `compact_running` | echo `❯ /compact`, `✢ Compacting conversation… (1s ...)`, empty box, `esc to interrupt` | no |
| `compact_done` | `✻ Conversation compacted`, echo, `⎿  Compacted`, empty box | no |
| `pasted_unsent` | `/he<U+200B>lp` bracketed-pasted, not sent (the emulator draws no glyph for U+200B; row reads `❯ /help`) | yes |
| `swallowed_after_enter` | one Enter after that paste: text kept, now coloured `/help`, suggestion menu open | yes |
| `after_esc_1` | Esc closed the menu: `/help` still in the box, notice `Removed 1 invisible character · review and press Enter to send` | yes |
| `final_empty` | Ctrl+U: empty box | no |

The text passed to the text readers is the stripped command (`/model`, `/help`, `/compact`, `/co`), as `Deliver` would hold it.

## Table: screen x reader (WRONG = differs from truth)

Columns: A = `InputIsEmpty == NotEmpty`. B = `InputBox(rows)` (a gate, not a verdict; true means a box is on screen). C = `holdsText`. D = `draftIsOurs`. E = `screenShowsPrompt`. F = `InputBox && A`. G = `InputBox && fixed-SGR NotEmpty`. H = `boxRowHolds` (InputBox gate + box-row plain text starts with our text or `[Pasted text`).

| Screen | Truth | A | B | C | D | E | F | G | H |
|---|---|---|---|---|---|---|---|---|---|
| idle_empty | no | no | yes | no | no | no | no | no | no |
| typed_co_menu | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| typed_model_menu | yes | **no WRONG** | yes | yes | yes | yes | **no WRONG** | yes | yes |
| model_open | no | no | **no** | **yes WRONG** | **yes WRONG** | **yes WRONG** | no | no | no |
| model_landed_empty | no | no | yes | **yes WRONG** | **yes WRONG** | **yes WRONG** | no | no | no |
| help_open | no | no | **no** | **yes WRONG** | **yes WRONG** | **yes WRONG** | no | no | no |
| help_landed_empty | no | no | yes | **yes WRONG** | **yes WRONG** | **yes WRONG** | no | no | no |
| after_chat | no | no | yes | no | no | no | no | no | no |
| compact_running | no | no | yes | **yes WRONG** | **yes WRONG** | **yes WRONG** | no | no | no |
| compact_done | no | no | yes | **yes WRONG** | **yes WRONG** | **yes WRONG** | no | no | no |
| pasted_unsent | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| swallowed_after_enter | yes | **no WRONG** | yes | yes | yes | yes | **no WRONG** | yes | yes |
| after_esc_1 | yes | **no WRONG** | yes | yes | yes | yes | **no WRONG** | yes | yes |
| final_empty | no | no | yes | no | no | no | no | no | no |

Columns A (`InputIsEmpty`) and F are wrong exactly on the three screens where our text is in the box as a recognised slash command. Columns C, D and E are right on the three typed screens only by luck: they are wrong on all six landed or dialog screens, because the echo stays on the visible screen. Column G (after fixing `applySGR`) and column H are right on all 14. (Reproduce: the raw output of my run printed these columns; "fixed SGR" skips `38`/`48` arguments and is otherwise `inputState` for the first input row only, without the multi-line continuation rows.)

Notes on the table:

- `model_open` and `help_open`: `InputBox` is false because there is one rule above the dialog, not a `❯` row between two. This matches the doc comment at `agentrunner/claudeui/screenmatch.go:29-31`. It is the gate that keeps Enter out of dialogs.
- `compact_running`: `InputBox` is true and the box is empty, so no Enter. The echo `❯ /compact` sits in scrollback above the spinner and has no rule under it.
- The suggestion menu row (`  ❯ /copy ...`, `  ❯ /model ...`) does not fool `InputBox`: it is below the bottom rule, so no `❯` row sits between two rules except the input row. It does not fool `InputIsEmpty` either: its first cell is a space, not `❯`.

## What each combination gets right on the swallowed case

- The swallowed-Enter screen (`swallowed_after_enter`) is the one `Deliver` must act on. Only G and H read "yes". F (`InputBox && InputIsEmpty`) reads "no" and would stay silent; the text then sits there until the end of the window.
- After the swallow Claude has stripped the invisible character, so the text is now a valid command and Claude highlights it. The raw paste, with the invisible character still in it, was uncoloured (`pasted_unsent`: `❯\xa0/help` with no SGR), so a test that only checks the first read before the Enter will not see the bug.

## What this means for the slash policy

- Gate every "press Enter again" with `claudeui.InputBox(Screen rows)`. It holds on dialogs (no Enter) and on a running `/compact` (empty box, no Enter).
- Decide "ours" from the box row only, never from the whole screen: `strings.HasPrefix(normalizedBoxRowText, first 20 chars of text)` or `strings.Contains(boxRowText, "[Pasted text")`. A draft that is the person's does not match and gets no Enter, the same intent as `draftIsOurs`.
- Or fix `applySGR` and keep `InputIsEmpty`, plus a box-row text match for "ours". Both are product changes; the fix also repairs `PressEnterIfStillTyped`, `nudgeEnter` and `pty_personinput.go`, which share the bug. Recommend filing it as a follow-up either way.
- Not tested: a placeholder in the box (the probe showed none), a text over 512 bytes that Claude shows as `[Pasted text #N +M lines]`, a multi-row wrapped draft, a person's draft in the box, a narrow terminal, a torn frame during resize, a tool-permission dialog (`❯ 1. Yes` inside rules). `boxRowHolds` was only exercised on the 14 screens above.
- For a multi-row draft, `boxRowHolds` as written reads only the first row, which is where our first 20 characters sit.

## What the `tuiclaude` fake must model

Today (`testutil/agentfake/tuiclaude/tui_state.go:330-368`, `tui_prompt.go:389-418`):

- Typed text echoes as `> ` + text on a plain screen. Only an empty idle box has `❯ ` between two 20-dash rules (`tui_state.go:361-363`).
- Busy draws `> prompt` and `✻ Working…` with no box (`tui_state.go:353`).
- A slash command goes through `slashCommand`: `dialogNext` switches to the Waiting status and draws the `Do you want to proceed?` frame; no `/model` or `/help` dialog frame; no suggestion menu; no highlight colour; no swallow (the `ActionSwallowEnter` model is in `agentfake.InputBox`, `testutil/agentfake/programinput.go:211-250`, which `tuiclaude` does not use).
- `colored()` only wraps the welcome line in red.

To make the table test cover these screens the fake needs:

1. A real input box on every frame where Claude has one: rule, `❯\xa0` + draft, rule, each at the full width. Typed text goes in that row, not in a `> ` echo. Rules are at least 3 `─` (`isRule` needs `───`).
2. A highlighted draft: when the draft is a known slash command, draw it as `\x1b[38;2;177;185;249m<cmd>\x1b[m` (this colour is what the probe showed). A plain draft stays uncoloured. This is what makes the `applySGR` bug visible; without it the table test passes on a broken reader.
3. A suggestion menu under the bottom rule while a `/...` draft is typed: rows `  ❯ /name   description`, two leading spaces.
4. The swallowed Enter for text that starts with `/` or `!` and holds an invisible character: first Enter strips the character, keeps the (now highlighted) text in the box, shows the notice `Removed 1 invisible character · review and press Enter to send`, and does not submit. The second Enter submits. (Claude's strip set is wider than gx's; see `docs/research/deliver-primitive-invisible-character.md`.)
5. Dialog frames for `/model` and `/help` after Enter: the styled echo row at column 0 (`\x1b[38;2;153;153;153;48;2;55;55;55m❯\x1b[39m \x1b[38;2;255;255;255m/help\x1b[39m`), one rule, the dialog body, and no input row. Esc closes it: `⎿  Kept model as ...` or `⎿  Help dialog dismissed` under the echo, and the empty box returns. Enter inside the `/model` dialog must save and close it, so a stray Enter shows up as a state change the test can assert.
6. `/compact` running: echo row, a spinner row, then the empty box and `esc to interrupt` in the footer. It ends after a delay with `✻ Conversation compacted` and `⎿  Compacted`. Enter on the empty box does nothing, and a counter of how many compactions ran lets the test assert "ran once".
7. A landed local command in scrollback: the echo `❯ /model` followed by `⎿  ...`, with an empty box below. This is the screen that fools the whole-screen readers.
8. The truecolor env: the screens above are the `COLORTERM=truecolor` ones. The fake already branches on `TERM`/`COLORTERM` (`tui_state.go:319-327`); the new SGR must follow the same switch, and the table test should run with truecolor set, because that is the holder env.

Without 1, 2 and 5 the table test would pass with `holdsText` or `InputIsEmpty` in place, which are the readers that fail on real screens.
