# chat-interrupt/04: Esc on a live Codex TUI

Ticket `gx:chat-interrupt/04`. Codex CLI 0.167.0, run in a throwaway tmux pane (110x35) in
`/private/tmp/claude-501/codex-probe`, launched as `codex -s read-only -a never`. Real Codex config untouched.
Turns were held open with the prompt "Run the shell command: sleep 90. Then reply done." Screens below are
real `tmux capture-pane -p` output (blank lines and the banner trimmed). No product code changed.

## Write path

The holder sends Esc as one write of the single byte `\x1b` (`nativerunner/holder/holdersock.go:333`,
`h.ptmx.Write([]byte("\x1b"))`; also `holderanswer.go:139,185,239`). `tmux send-keys Escape` also writes the
lone `\x1b` byte to the pane PTY, so the probe matches the holder. A lone ESC was read as Esc, not as an Alt
prefix: in every case below it acted within the first poll (<=250 ms poll interval on the interrupt case;
no visible delay). Caveat: tmux's own `escape-time` only affects input from a real terminal, not
`send-keys`, so this checks Codex's reading of a lone ESC, not tmux's.

## Results

| # | Case | Result |
|---|------|--------|
| 1 | Esc while turn runs, empty input | Turn ends, Codex alive. |
| 2 | Esc while turn runs, draft `my draft` | Turn ends, draft kept, Codex alive. |
| 3 | Esc while turn runs, `/` popup open | 1st Esc closes only the popup (turn keeps running, `/` text stays); 2nd Esc interrupts. |
| 4 | Idle, empty, no history, Esc x1 and x2 | Footer hint absent after x1; x2 shows `No previous message to edit.` Nothing else. |
| 5 | Idle, empty, history exists, Esc x1 | Footer `esc again to edit previous message`. |
| 6 | Same, Esc x2 | Opens the transcript overlay: `Browsing transcript ... esc back`. Third Esc closes it. |
| 7 | Idle, draft `hello draft`, Esc x1 and x2 | Draft untouched, no overlay, no hint change. |
| 8 | Idle, `/` popup, Esc x1 | Popup closes; `/` stays in the input. |
| 9 | Idle, `/mo` popup text, Esc x1 | Popup closes; text stays. |
| 10 | `-c 'tui.keymap.chat.interrupt_turn=["esc"]'` | Codex starts normally. Esc is already the default (the status line says `esc to interrupt` with no flag), so the flag changes nothing observable. |

### 1/2. Esc while running, with draft (case 2)

Before (draft typed mid-turn; footer offers queueing):
```
› Run the shell command: sleep 90. Then reply done.
• I'll run sleep 90, then reply when it finishes.
• Working (14s • esc to interrupt) · 1 background terminal running · /ps to view · /stop to close
› my draft
  GPT-6.1-Sol low · 9.59K used · Context 2% used · /private/tmp/claude-501/codex-probe · weekly 95% left
  tab to queue message
```
After one Esc (3 s later):
```
• I'll run sleep 90, then reply when it finishes.
■ Conversation interrupted - use /feedback if something went wrong
  1 background terminal running · /ps to view · /stop to close
› my draft
  GPT-6.1-Sol low · 9.81K used · Context 2% used · ...
```
Case 1 (no draft) after one Esc:
```
■ Conversation interrupted - use /feedback if something went wrong
  2 background terminals running · /ps to view · /stop to close
› Ask Codex to do anything
```
Side effect: the shell tool's `sleep` was NOT killed. The status line keeps `N background terminal(s)
running` after the interrupt (they pile up: 1, 2, 3 across turns). Esc ends the turn, not the command.

### 3. Esc while running, popup open

```
• Working (12s • esc to interrupt) · 3 background terminals running ...
› /
```
That is the screen after the 1st Esc: popup gone, turn still `Working`. After the 2nd Esc:
```
■ Conversation interrupted - use /feedback if something went wrong
› /
```

### 4-7. Idle

Empty composer, no history, Esc x2:
```
• No previous message to edit.
› Ask Codex to do anything
```
Empty composer, with history, Esc x1 (footer line):
```
  esc again to edit previous message
```
Esc x2 (footer line; the body shows the transcript):
```
  Browsing transcript · ↑↓/jk scroll · ←→/hl prompts · ⌃t details · ↵ rewind · esc back
```
One more Esc returns to the normal screen. Enter in that overlay would rewind (not sent).

Draft `hello draft`, Esc x2: screen unchanged, `› hello draft` still in the input.

### 8-9. Idle popup

`/` shows the command list (`/model`, `/fast`, `/presets`, ...). One Esc: list gone, `› /` stays. Same for `/mo`.

### Not run

- `/agents` view and a history overlay opened while a turn runs: not probed (cost). Source says the
  `/agents`+Esc case does not interrupt.
- A mid-turn overlay (transcript) was not opened.
- `interrupt_turn` unbound or remapped: not probed. Pinning with `-c` is accepted but unproven to override
  a user keymap file; the default alone gave `esc to interrupt`.
- Ctrl+C idle for contrast: not run (source says it quits).

## Verdict

Esc is safe and effective as gx's Codex interrupt, with caveats:
- Effective: one lone `\x1b` interrupts a running turn with or without a draft; Codex stays up; draft kept.
- Safe idle: one Esc is a no-op or a footer hint; draft never lost; no quit. Only a second Esc in a row
  opens the transcript overlay (needs a follow-up Esc to leave). gx should send one Esc per click.
- A late Esc landing on an open popup closes the popup instead of interrupting (needs a second Esc).
- Caveat: Esc does not stop the running shell command; it lingers as a background terminal.
