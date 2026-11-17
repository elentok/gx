# TUI behaviour that only the herdr e2e suite covers

Ticket: `gx:remove-herdr-runner/03`. Question: for each test in the four TUI e2e files, which behaviour does it check, does another test already cover it, and which herdr-free rig could host a port of what is left?

## Short answer

- Only the herdr runner and `herdrctl` are herdr. The TUI tests in these files do not need herdr at all. `agents_terminal_test.go` and `context_menu_test.go` drive the real `gx` binary through their own `pty` plus `charmbracelet/x/vt` emulator (`vtTerm`, `e2e/agents_terminal_test.go:204` `startVTTerm`). Their only herdr ties are `herdrctl.RequireHerdr` (`agents_terminal_test.go:75`), `herdrSocketEnv()` (`:87`) and `writeGxConfig`, which pins `"agent-runner": "herdr"` (`e2e/server_lifecycle_test.go:202`). `gx_worktrees_herdr_test.go` and `server_tui_test.go` use `herdrctl.NewWorkspace` as the terminal.
- No other test in the repo runs the real `gx` TUI binary in a terminal emulator. The `x/vt` import, `creack/pty` TUI driving and `startTUI` exist only in `e2e/` (checked with `rg 'x/vt"'` over `*.go`). In-process coverage is `testutil/teatestv2` and direct `Model.Update`/`View` tests.
- Six behaviours have no test at the layer where the e2e catches them (listed at the end). Four of them need the real binary, a real server and a real terminal, so the only rig that fits is the `vtTerm` helper moved out of `e2e/` into a surviving package. That is a design question for the map (see the end).

## Test by test

### `e2e/agents_terminal_test.go`

All six tests start a real server (`newAgentsEnv`, `:73`) whose `claude` is `testutil/agentfake`'s TUI fake, create one session, and run `gx agents` or `gx tickets` in a 140x40 `vtTerm` (`startTUI`, `:337`).

**`TestAgentsTerminal_ShowsTheAgentsColors` (`:348`)**
- Checks: the agent's red welcome line shows red in the outer terminal, although the server started with no `TERM`/`COLORTERM` (`serverEnv` strips both, `:89-93`). The fake colors its output only when it sees `TERM` or `COLORTERM` (`testutil/agentfake/tuiclaude/tui_state.go:298-306`). The holder is what sets them (`nativerunner/holder/holder.go:302-309`, used at `:179`).
- Chain: holder child env -> holder screen -> attach frame ANSI -> TUI panel render -> terminal.
- Other coverage:
  - Frame rows carry ANSI over the wire: `nativerunner/holder/holderattach_test.go:72` (red row asserted at `:98`).
  - Live panel render keeps colors: none. `ui/agents/attach_test.go:194` asserts on `plainView`, which strips ANSI (`attach_test.go` `plainView` helper). `ui/agents/terminal_test.go:61` colors only the Exited last screen, and `ui/agents/terminal/terminal_view.go:165` strips it with `ansi.Strip`.
  - Holder sets `TERM`/`COLORTERM` for the child: none. `terminalEnv` has no test (`rg terminalEnv nativerunner` finds only `holder.go`).
- Gap: yes, two parts (child env; live render keeps SGR).
- Ports:
  - Child env: extend `nativerunner/holder/holderattach_test.go` using `scriptHolder` (`:113`, runs `RunTestHolder` -> `runHolder`, which calls `terminalEnv`). Unset `TERM` with `t.Setenv`, run `printf "$TERM $COLORTERM"`, assert the screen. This tests the behaviour directly, with no server.
  - Live render keeps colors: extend `ui/agents/attach_test.go` with `fakeAttach.frame` (`:115`) carrying `"\x1b[31m..."` and assert on the raw `m.View().Content`, not `plainView`.

**`TestAgentsTerminal_TypesShiftedLetters` (`:361`)**
- Checks: typing `Hi There!` in the focused terminal reaches the agent as `Hi There!` (the capital H, the `!`).
- Chain: outer terminal bytes -> bubbletea key decode -> `screen.Key{Code, Text, Mod}` (`ui/agents/terminal/attach.go:243-247`) -> attach wire -> holder -> `screen.SendKey` -> pty.
- Other coverage:
  - Encoding of shifted text: `agentrunner/screen/screen_test.go:402` (shift letter, caps lock, alt+shift).
  - The UI passes Code/Text/Mod through unchanged: `ui/agents/attach_test.go:233`, but its presses have no Shift and no uppercase letter.
  - Holder `SendKey` reaches the child: `nativerunner/holder/holderattach_test.go:166` and `testutil/agentfake/attachinput_test.go:40`, lowercase only.
  - Raw bytes `H` through bubbletea's decoder into a `KeyPressMsg`: none.
- Gap: partial. Each link is tested, but not an uppercase letter end to end, and not the decoder step.
- Port: add a `{Code: 'h', Text: "H", Mod: tea.ModShift}` press to `ui/agents/attach_test.go:233` (asserts the UI forwards Text/Mod), plus an `H` case next to `TestHolderAttach_KeyEchoSkipsThePacing` (`holderattach_test.go:166`) that echoes through a real holder. The bubbletea decode step is only reachable from a real terminal byte stream (see the `vtTerm` rig below); if a port is wanted, one `vtTerm` test covers it with the next two behaviours.

**`TestAgentsTerminal_ShowsTheAgentsCursor` (`:374`)**
- Checks: the focused terminal shows the real cursor right after the agent's text (`x == px+len("> abc")`, same row).
- Chain: holder frame cursor -> `View.TerminalCursor` (`ui/agents/terminal/terminal_view.go:171`) -> `Model.View().Cursor` (`ui/agents/model.go:454`) -> app shell shifts it below chrome -> terminal.
- Other coverage:
  - Holder frame carries the cursor: `holderattach_test.go:87`.
  - App shell shifts a page cursor below the chrome: `ui/app/model_mouse_test.go:245` (inside `TestPageCursorMovesBelowChrome`, with a stub page).
  - `TerminalCursor` itself (focus, live, offset, copy mode, `Covered`, `ScreenSize` clamp, `ScreenOrigin` offset): none. `rg TerminalCursor` finds no test.
- Gap: yes, the Agents page's own cursor logic.
- Port: `ui/agents/attach_test.go`: send a frame with `Cursor{X, Y, Visible}` (extend `fakeAttach.frame` at `:115`), focus the terminal, assert `m.View().Cursor` position equals `ScreenOrigin()` plus the frame cursor; also assert nil when unfocused, scrolled (`offset > 0`) and covered.

**`TestAgentsTerminal_WheelNeverReachesTheAgentAsArrows` (`:392`)**
- Checks: the wheel over a focused terminal does not become arrow keys for the agent (claude warns about that). The `vtTerm` mimics alternate-scroll: if the TUI did not turn mouse reporting on, the wheel would arrive as three `\x1b[A` (`wheelUp`, `:253`).
- Other coverage:
  - Wheel over the panel becomes a holder scroll, not keys, in either focus: `ui/agents/scroll_test.go:39` (log is only `scroll:N`).
  - Mouse reporting is on for the Agents tab: `ui/app/hover_filter_test.go:36-55` (`MouseModeAllMotion` for `TabAgents`) and `ui/agents/hover_test.go:111`.
  - A wheel in copy mode: `ui/agents/copymode_test.go:158`.
- Gap: none. The e2e outcome follows from "mouse reporting stays on" (tested) plus "wheel is handled as a scroll, never as a key" (tested).
- No port needed.

**`TestAgentsTerminal_WheelReachesAnAgentThatReportsTheMouse` (`:411`)**
- Checks: an agent in the alternate screen with mouse reporting on gets the wheel as a mouse event (`wheel up 1 down 0`).
- Other coverage:
  - UI sends the wheel to a mouse-reporting child: `ui/agents/scroll_test.go:56`.
  - Holder encodes it for the child's modes: `nativerunner/holder/holderattach_test.go:264` (real child reads `\033[<64;3;1M`).
  - Screen-level mode handling: `agentrunner/screen/screen_test.go:466`.
- Gap: none.
- No port needed.

**`TestAgentsTerminal_AttachesWhenSwitchedToAfterTheStateLoaded` (`:424`)**
- Checks: the app opens on Tickets, the server state loads, then key `8` builds the Agents page with state already present, and the page must dial the selected session. The regression (commit `635aa18b2`) was in `ui/app` not returning the page's dial command: `newHistoryEntry` (`ui/app/model.go:260`) hands the command to `applySwitch` (`ui/app/model_tabs.go:77-84`) and `handleOpen` (`ui/app/model_update.go:193-195`).
- Other coverage:
  - The Agents page dials on its first `WithServerState`: `ui/agents/attach_test.go:152` (`attachModel`, via `withStatusOfA` at `:162`) with `dialed` (`:172`), and `:585` (`TestAttach_QueueAgentDialsItsHolderDir`).
  - Leaving and returning redials: `ui/agents/attach_test.go:435`.
  - Switching to the Agents tab in the shell: `ui/app/model_tabswitch_test.go:237` (no sessions, checks only `ActiveTab`).
  - A shell test that switches to Agents with sessions loaded and expects the dial: none.
- Gap: yes, the shell wiring only.
- Port: `ui/app/server_stream_test.go`. `newStreamHarness` (`:203`), `streamFake.sessions`, `probeUp` (`:186`) and `switchTo` (`:191`) already do the work, and `TestServerStream_SessionFocusShowsSessionInThisTUIOnly` (`:288`) sets sessions the same way. Load a session, `probeUp`, `switchTo(nav.TabAgents)`, `pump`, and assert a `terminal.AttachedMsg` for that session came back through `h.msgs`. `agentsui.NewModel` hard-wires `terminal.DialHolderAttach` (`ui/agents/model.go:107`), so the dial fails against no holder; the `AttachedMsg` carrying the session ID is still the proof. A `WithDialer` option would let the test use a fake holder (small seam, see the design questions).

### `e2e/server_tui_test.go`

**`TestServerTUI_ShowsServerModeAndDropsOnStop` (`:18`)**
- Checks: the real TUI process connects to a real, running server (tickets come from the server: the epic name shows) and shows no `server down`; after `gx server stop` the indicator reads `server down`.
- Other coverage:
  - Indicator down/up/banner from a fake client: `ui/app/server_conn_test.go:112` (`TestServerMode_ConnectionDropAndReconnect`), `ui/app/title_row_test.go:76`.
  - Tickets tab renders server state: `ui/tickets/server_mode_test.go:85-250`.
  - Client/server wire: `apiclient/client_test.go` and the `cmd/*` tests that use `servertest`.
  - A real `gx` TUI process wired to a real server socket (`cmd/tui/ui.go:328` `serverDeps`, `cmdbase.ServerClient`): none. `cmd/tui/ui_test.go` only tests `tabTarget`.
- Gap: yes, the process-level wiring (real socket, real probe against a stopped server). Each part has a unit test.
- Port: the same real-binary `vtTerm` rig as the Agents tests, started against `gx server start` with the PTY runner (no herdr). Today's test also pins herdr (`writeGxConfig` at `server_lifecycle_test.go:200`); the port drops that.

### `e2e/gx_worktrees_herdr_test.go`

**`TestWorktreesTab_LaunchesAndListsWorktree` (`:54`)**
- Checks: `gx worktrees` starts as a binary, renders a first frame and lists the seeded linked worktree `feature-x`. The comment calls it "a small smoke scenario proving the herdr-backed harness works" (`gx_worktrees_herdr_test.go:48-53`; header at `:1-7`).
- Other coverage:
  - The Worktrees page lists worktrees in-process: `ui/worktrees/worktrees_test.go:81` (`TestDeleteConfirmationAppearsAndCancels` waits for `feature-a` at `:84`), `:98`, `:141`, all via `teatestv2` (`startTUI`, `:27`).
  - Command wiring without a terminal: `cmd/tui/cmd_worktree_test.go:22`.
  - Binary launch smoke in a terminal: none.
- Gap: the page behaviour is covered. Only "the binary starts and draws" is not, and it is the same smoke the other `vtTerm` tests already imply (they start the same binary).
- Port: none needed. If a launch smoke is wanted, a single `gx worktrees` case in the `vtTerm` rig costs nothing once the rig exists.

### `e2e/context_menu_test.go`

**`TestContextMenu_OpensClosesAndRuns` (`:62`)**, two subtests, in the real Tickets tab (server running):
- `keyboard` (`:72`): `m` opens the menu right under the selected row (`waitMenuBelow`, `:40`: title at x=3, y=rowY+1); `esc` closes it; reopen, type `draft`, `enter` closes it and writes `status: draft` to the ticket file.
- `mouse` (`:87`): right-click on the row opens it under the row; a left click over the preview (outside the menu) closes it; right-click again and left-click `Set open` runs it and writes `status: open`.
- Other coverage:
  - Esc closes without running: `ui/contextmenu/contextmenu_test.go:91`. Enter runs an item and closes: `:83`. Filtering by typing: `:98`, `:116`.
  - Left click runs an enabled item: `:213`; anchored hit-testing: `:266`; click outside (above, left, right, right button) closes: `:281`.
  - Placement below the anchor, flipping above, clamping: `:240` (`Origin`, `ui/contextmenu/contextmenu.go:161`).
  - Setting status writes the file: `ui/tickets/ticket_menu_test.go:168` (`Set open`, `Set done`; via `chooseTicketMenuItem`, `:41`).
  - Right-click on a ticket row selects it and starts the menu; on preview, blank space or a header it does nothing: `ui/tickets/model_nav_test.go:135-170`. The Queue tab has the same for its own rows: `ui/tickets/queue/scroll_mouse_test.go:331-351`.
  - A click while the menu is open goes to `Menu.UpdateMouse` with `menuAnchor()`: `ui/tickets/model.go:226-230`. A test where the click lands on the preview and still closes the menu through the page: none (the contextmenu test covers the menu alone, with its own anchor).
  - `menuAnchor()` (`ui/tickets/row_menus_host.go:73`, `X: 1, Y: 1+line`) and the menu's final screen position `(3, rowY+1)` after the app chrome offset: none. `rg menuAnchor` in tests finds nothing.
- Gap: partial. Everything except "the menu really draws at (3, rowY+1) on the screen, including the app chrome, and a click at absolute screen coordinates over the preview or an item hits what is drawn".
- Port: extend `ui/tickets/model_nav_test.go` (it already finds a ticket's sidebar line with `selectedSidebarLine`): press `m`, render `m.View()`, and assert the menu title row/column equals `menuAnchor` placed by `Origin`; then send `MouseClickMsg` at the drawn item's cell and over the preview. The chrome offset is `ui/app/model_mouse_test.go:242` (`TestPageCursorMovesBelowChrome`) style; add one app-level case there with the Tickets page if the offset is wanted. In-process is enough: the mismatch the e2e guards against is a coordinate mismatch between draw and hit test, and both are pure functions of the model.

## Uncovered behaviours and the rig for each

1. **Holder sets `TERM`/`COLORTERM` on the child when the server has none.** Rig: `nativerunner/holder/holderattach_test.go` (`scriptHolder`, `t.Setenv("TERM", "")`).
2. **Live attach render keeps the child's colors.** Rig: `ui/agents/attach_test.go` (`fakeAttach.frame` with SGR, assert raw `View().Content`).
3. **Shifted/uppercase letter reaches the child.** Rig: `ui/agents/attach_test.go:233` (Shift press) plus `holderattach_test.go:166` (`H` echo). The bubbletea byte decode step only fits the `vtTerm` rig.
4. **`TerminalCursor` placement and gating.** Rig: `ui/agents/attach_test.go` (`View().Cursor`).
5. **Shell dials the Agents session when switching to Agents with state already loaded.** Rig: `ui/app/server_stream_test.go` harness (`switchTo(nav.TabAgents)`).
6. **Context menu draws at the anchor and clicks hit what is drawn.** Rig: `ui/tickets/model_nav_test.go` (+ one `ui/app` case for chrome).
7. **Real `gx` TUI process against a real server (server mode shown, drops on stop).** Rig: the `vtTerm` pty+emulator helper, moved out of `e2e/` into a surviving package, with the server on the PTY runner. There is no other rig that runs the real binary in a terminal.

Not a gap: wheel never as arrows, wheel to a mouse-reporting agent, worktrees list (covered above).

## Design questions the map has not decided

- Where does the `vtTerm` rig live once `e2e/` is deleted? It (`startVTTerm`, `waitText`, `fg`, `cursor`, `click`, `wheelUp`, `trackMouseModes`; `e2e/agents_terminal_test.go:204-326`, `context_menu_test.go:17-49`) is herdr-free and is the only real-binary terminal rig. Suggestion: extract it to a package (for example `testutil/vtterm`) and keep one small suite for "real binary, real server" tests (items 3's decode step and 7, optionally a `gx worktrees` launch smoke), run on the PTY runner. If the map would rather keep the TUI tests in-process only, items 3 (decode step) and 7 stay uncovered.
- Should `agentsui.NewModel` get a dialer option (for item 5)? Without it, the shell test can only check that an `AttachedMsg` for the session comes back, not that a fake holder was attached.
