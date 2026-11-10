# chat-interrupt/03: interrupting Codex without a quit risk

Ticket: `gx:chat-interrupt/03`. Branch `chat-interrupt-design`. Nothing was changed in product code.
Codex source: shallow clone of `openai/codex` at commit `a92eae0891724eb960ccfc82904551d152786cc3`
(2026-11-10, main). Installed CLI here: `codex-cli 0.167.0`; the clone may be newer, so line numbers
can drift. Paths below are relative to `codex-rs/` in that clone.

## Answer

Send **Esc** to Codex instead of Ctrl+C. While a turn runs, one Esc interrupts it. While idle, one Esc
only arms a harmless "edit previous message" hint. Keep Ctrl+C for Claude. See "Recommendation".

## 1. Codex: Esc vs Ctrl+C

Ctrl+C (`tui/src/chatwidget/interaction.rs:652-712`, `on_ctrl_c`):
- Double-press quit is off: `DOUBLE_PRESS_QUIT_SHORTCUT_ENABLED = false` (`tui/src/bottom_pane/mod.rs:255`).
- Path with it off (`interaction.rs:690-701`): task running -> `submit_op(AppCommand::interrupt())`;
  otherwise -> `request_quit_without_confirmation()`. So an idle Ctrl+C quits Codex at once.
- Before that, `bottom_pane.on_ctrl_c()` may handle it (a popup or modal closes), so Ctrl+C is not a
  quit then. An empty idle composer is the quit case.

Esc, turn running:
- Default binding `chat.interrupt_turn = plain(Esc)` (`tui/src/keymap.rs:1866`; remappable, may be unbound).
- `BottomPane::should_interrupt_running_task` (`bottom_pane/mod.rs:1849-1864`) is true when the key is the
  interrupt key, `is_task_running`, no modal or popup is active, the shortcut overlay is not showing, it
  is not `/agents`+Esc, and vim-insert Esc does not own the key. It does not check that the composer is
  empty, so Esc interrupts a running turn even with draft text in the input.
- It then calls `status.interrupt()` or `app_event_tx.interrupt()` (`bottom_pane/mod.rs:1001-1008`),
  which sends `AppCommand::interrupt()` (`tui/src/app_event_sender.rs:73-75`): the same op Ctrl+C sends.
  The turn ends, Codex keeps running.
- A popup (skill/slash/file) or a request-user-input overlay takes Esc first (popup: dismiss; overlay:
  its own `interrupt_turn_keys` handling, `bottom_pane/request_user_input/mod.rs:1200,1220`).
  Test name: `esc_with_skill_popup_does_not_interrupt_task` (`bottom_pane/mod.rs:3888`).

Esc, idle (`tui/src/app/input.rs:702-732`, `tui/src/app_backtrack.rs:144-155`):
- `should_handle_backtrack_esc` (`app/input.rs:854-862`) needs composer empty and "normal backtrack
  mode" (`!is_task_running`, `bottom_pane/mod.rs:1931`).
- Empty composer, 1st Esc: `prime_backtrack()` sets `primed` and shows the footer hint "Esc again to edit
  previous" (`app_backtrack.rs:270-277`; footer `EscHint`, `bottom_pane/footer.rs:218-223`). No quit, no
  interrupt, no text change.
- 2nd Esc while primed: `open_backtrack_preview` opens the transcript overlay on the last user message
  (`app_backtrack.rs:152-153,280-295`). That is a modal view the person must leave (Esc cancels,
  `app_backtrack/legacy_input.rs:211-213`). Enter would start a revert of the session to that prompt.
  gx sends one Esc per click, so this needs two clicks to reach.
- Any non-Esc key press cancels the primed state (`app/input.rs:741-753`). So the person typing, or gx
  typing a prompt, clears it.
- Text in the composer, idle: `should_handle_backtrack_esc` is false, so Esc goes to the composer
  (`app/input.rs:717-723`). It clears a popup or dismissed-token state, or does nothing; it does not
  quit or clear the text. (Not traced through every composer sub-state.)
- Idle with a primed state left over: the next prompt gx or the person types resets it. A later turn
  start does not matter because `is_normal_backtrack_mode` goes false and Esc then interrupts.

Net: Esc has no quit path and no data loss when idle. Worst case is a footer hint, or with two clicks
the history overlay.

## 2. Other interrupt paths in Codex

- App-server protocol: `turn/interrupt` with `{threadId, turnId}`
  (`app-server-protocol/src/protocol/common.rs:1125`, `protocol/v2/turn.rs:361-364`). It is race-safe: with
  no active turn, or a stale `turnId`, it returns an error ("no active turn to interrupt", "expected active
  turn id ...", `app-server/src/request_processors/turn_processor.rs:1465-1492`).
  Not usable by gx today: gx launches the interactive `codex` TUI in a PTY (`harness/codex.go:121-140`,
  no `--remote`/listen flags), and the TUI talks to an in-process app-server. A control socket exists in
  `~/.codex/app-server-control/` here, but gx does not use it and has no turn id. Adopting it is a large
  change for this one fix.
- Other keys: `chat.interrupt_turn` is the only interrupt binding found. Users can rebind or unbind it
  (`tui/src/keymap.rs:4281-4298`), so Esc is a default, not a guarantee. gx launches Codex with its own
  `-c` flags (`harness/codex.go:97-110`) and could pin `tui.keymap.chat.interrupt_turn` the same way
  (unverified; the key path is `tui.keymap.chat.interrupt_turn`, `keymap.rs:955`).
- No Ctrl+C variant is safe: `on_ctrl_c` quits when no task runs.

## 3. Claude Code: Ctrl+C and Esc when idle

From https://code.claude.com/docs/en/interactive-mode (General controls table):
- Ctrl+C: "Interrupts a running operation. If nothing is running, the first press clears the prompt
  input and a second press exits Claude Code." One idle Ctrl+C clears a draft, never quits. So the race
  costs the person's typed draft at worst (they can bring it back, per the same page).
- Esc: "Stop the current response or tool call mid-turn ... Claude keeps the work done so far." A single
  idle Esc has no documented effect (only closes dialogs).
- Esc+Esc: with text, clears the draft; with empty input, opens the rewind menu.
- Vim mode: Esc enters NORMAL mode (same page, vim table). gx already special-cases this
  (`harness/vim.go:21-42` sends `i` before typing). An idle Esc in a vim-mode Claude session would drop
  it into NORMAL mode, and gx's next typed text would land in NORMAL mode. So Esc is not safe for Claude
  in vim mode; Ctrl+C is the better key for Claude.

## 4. How gx's Session status reaches Working and leaves it

- Codex reports state by OSC 7501 (`state=working|idle|blocked|error:app=codex`), emitted by the Codex TUI
  (`tui/src/terminal_program_status.rs`, snapshot `..iterm_session_status_fields.snap`). gx's
  `codexWatcher` (`harness/codexwatch.go:21-90`) keeps the last report; before the first report it falls
  back to the terminal title spinner. Status is derived on demand.
- The holder's `signalHub` is fed every PTY write (`nativerunner/holder/holdersock.go:455-465`). The
  `watch` stream recomputes `h.watcher.Status()` on each change and pushes a `StreamStatus` only when it
  differs (`holdersock.go:380-428`). The server loop (`server/sessions/sessionwatch.go:~150-215`) maps it
  with `SessionStatusOf` (`sessions.go:51-66`) and stores it in `meta.Status` with `StatusChangedAt`.
  There is no polling for the state: it is push on PTY output.
- Lag: Codex's own idle report -> PTY -> holder parse -> unix socket -> server meta: milliseconds. Web
  clients then get it through the server's event stream (not traced here), so the page can show Working
  for a bit longer. The state token (`SessionStateToken`, `sessionstate.go:36-38`) guards stop/restore
  against stale pages, but `InterruptSession` does not use it: it only checks `meta.Info().Status.State ==
  Working` at server time (`sessionstate.go:22`). It is not wired through `CheckSteppedWrite` (that is for
  web stepped writes; not confirmed for interrupt in `routes.go:32`).
- The race has three parts: (a) turn ends before the web click (page stale: server check at line 22 sees
  Idle and refuses - fine); (b) turn ends between that check and the key write (server-side window, ms);
  (c) turn ends while the key is in the PTY, before Codex reads it (unavoidable by any check).
- Holder-side check: the holder can read live state right before writing. `opStatus` returns
  `h.watcher.Status()` directly (`holdersock.go:244-250`) and `opScreen` returns the live rows
  (`holdersock.go:268`); `opKeys` just writes to the PTY (`holdersock.go:269-277`). There is no "send keys
  only if working" op, and the holder protocol has no interrupt verb (`sessionstate.go:13-14`). A new op
  could check `watcher.Status().State == Working` and write in one step. This shrinks (b) to
  microseconds but cannot touch (c), and it reads the same OSC-derived status the server already has, so
  it adds nothing against (c).

## Recommendation

Send a different key to Codex: **Esc** (`\x1b`), and keep Ctrl+C (`\x03`) for Claude.
- Codex Esc removes the only harmful outcome (quit) in all three race parts; no check can do that.
- Do not use Esc for Claude: idle Esc enters NORMAL mode in vim-mode setups, and Claude's idle Ctrl+C
  only clears a draft (the first press never exits).
- Key choice per harness belongs in the `harness` layer next to `Launch`/`Resume`, not in
  `InterruptSession`, which today hardcodes `"\x03"`.
- Optional hardening, cheap: pin `tui.keymap.chat.interrupt_turn=["esc"]` in the Codex launch `-c` args
  so a user rebind cannot break it (verify the TOML shape first). Send a lone `\x1b` in its own write so
  the terminal parser does not read it as an escape-sequence prefix.
- Residual risks to accept: Esc while a popup, request-user-input overlay or `/agents` command is open
  closes that instead of interrupting (the person sees the turn still running and can click again);
  an idle Esc leaves a footer hint until the next key; two idle Esc presses open the history overlay.
- A holder-side state check is not worth a new protocol op for this bug. Optionally a test seam:
  verify with a fake or real Codex that Esc interrupts a running turn and leaves an idle one alone
  (`tui-testing-with-herdr` or the `agentfake` Codex program).

## Unverified

- Behavior of Esc in the composer with draft text across every sub-state (paste burst, bash mode, history
  search); read only the main path.
- That `-c tui.keymap.chat.interrupt_turn` works as a launch override.
- Real-Codex 0.167.0 behavior: sources are `main` at the commit above, not the installed binary's tag.
