# Ghostty AppleScript API: facts for split launches

Research for ticket `gx:ghostty-splits/01` (2026-10-18). Answers come from Ghostty's own
sources. Each fact is marked **confirmed** (with its source) or **unknown** / **derived**.

Sources used:

- [AppleScript docs]: https://ghostty.org/docs/features/applescript
- [1.3.0 notes]: https://ghostty.org/docs/install/release-notes/1-3-0
- [config ref]: https://ghostty.org/docs/config/reference
- [sdef v1.3.1]: https://github.com/ghostty-org/ghostty/blob/v1.3.1/macos/Ghostty.sdef
  (same file as `~/Applications/Ghostty.app/Contents/Resources/Ghostty.sdef` on this Mac)
- [ScriptTerminal]: https://github.com/ghostty-org/ghostty/blob/v1.3.1/macos/Sources/Features/AppleScript/ScriptTerminal.swift
- [AppDelegate+AppleScript]: https://github.com/ghostty-org/ghostty/blob/v1.3.1/macos/Sources/Features/AppleScript/AppDelegate+AppleScript.swift
- [ScriptSurfaceConfiguration]: https://github.com/ghostty-org/ghostty/blob/v1.3.1/macos/Sources/Features/AppleScript/ScriptSurfaceConfiguration.swift
- [embedded.zig]: https://github.com/ghostty-org/ghostty/blob/v1.3.1/src/apprt/embedded.zig (surface options, ~L488-575)
- [Exec.zig]: https://github.com/ghostty-org/ghostty/blob/v1.3.1/src/termio/Exec.zig (env ~L620-860, macOS login wrapping ~L1398-1530)
- [BaseTerminalController]: https://github.com/ghostty-org/ghostty/blob/v1.3.1/macos/Sources/Features/Terminal/BaseTerminalController.swift (`newSplit`, ~L236)
- [pid/tty commit]: https://github.com/ghostty-org/ghostty/commit/9a9002202b (2026-04-20, after v1.3.1)

Local install: `~/Applications/Ghostty.app`, version **1.3.1** (stable), bundle id
`com.mitchellh.ghostty`. Note: it is in `~/Applications`, not `/Applications`.

## Version and stability

- **Confirmed**: AppleScript was added in **Ghostty 1.3.0** (released 2026-03-09). [AppleScript docs], [1.3.0 notes]
- **Confirmed**: it is a **preview feature**. The 1.3.0 notes say they expect "breaking API
  changes and add significant new features in 1.4". [1.3.0 notes]
- **Confirmed**: on by default on macOS. `macos-applescript = false` turns it off. [AppleScript docs]
- **Confirmed**: latest tags are v1.3.0 and v1.3.1. `main` has changes not yet released (see "Targeting"). Checked via `gh api repos/ghostty-org/ghostty/tags`.
- **Confirmed**: every child process gets `TERM_PROGRAM=ghostty` and
  `TERM_PROGRAM_VERSION=<version>`. gx can check `>= 1.3.0` without calling osascript. [Exec.zig] L742-743

## Can split / new tab take command, cwd and env?

**Yes, all three, through a `surface configuration` record.** [sdef v1.3.1]

```applescript
tell application "Ghostty"
  set cfg to new surface configuration
  set initial working directory of cfg to "/abs/path"
  set command of cfg to "/abs/path/to/gx run -- lazygit"
  set environment variables of cfg to {"PATH=/opt/homebrew/bin:/usr/bin:/bin", "FOO=bar"}
  set t to split (focused terminal of selected tab of front window) direction right with configuration cfg
  -- or: set newTab to new tab in front window with configuration cfg
end tell
```

Record fields (all **confirmed**, [sdef v1.3.1], [ScriptSurfaceConfiguration]):

| Field | Type | Notes |
| --- | --- | --- |
| `initial working directory` | text | If it is not an absolute, existing directory, Ghostty logs a warning and **ignores it silently** (no script error). [embedded.zig] L488-525 |
| `command` | text | Runs instead of the shell. Empty string = not set. |
| `environment variables` | list of `"KEY=VALUE"` | An item without `=` is an error (`errAECoercionFail`, -1700). Added **on top of** Ghostty's own env (the `env` config map). |
| `initial input` | text | Sent to the terminal after launch. |
| `wait after command` | boolean | See "When the command exits". |
| `font size` | real | |

- `split` directions: `right`, `left`, `down`, `up`. [sdef v1.3.1]
- `new tab` takes optional `in <window>`. Without it, Ghostty uses its "preferred parent" window. [AppDelegate+AppleScript] L252-264
- `new window` also takes `with configuration`.

### How `command` is run (important)

- **Confirmed**: `command` from AppleScript is set as a `.shell` command. [embedded.zig] L529-534
- **Confirmed**: on macOS a `.shell` command runs as
  `/usr/bin/login -flp <user> /bin/bash --noprofile --norc -c "exec -l <command>"`. [Exec.zig] L1484-1530
- **Derived** from the above:
  - The user's shell rc files are **not** loaded. PATH is whatever Ghostty's own process env has
    (from launchd when started from the Dock), plus login(1) defaults. It is **not** gx's env.
    gx should pass an absolute binary path and put PATH (and anything else it needs) in
    `environment variables`. There is no "copy env" switch like kitty's `--copy-env`.
  - The string goes after `exec -l`, so `a && b` would `exec a` and never run `b`. Pass a single
    command, or wrap a compound one: `/bin/sh -c '<...>'`.
  - The string is parsed by bash, so shell quoting rules apply to the args.

### Is there a "send text" / "send keys" verb?

- **Confirmed**: yes. `input text "..." to <terminal>` pastes text (as a paste, so a newline or a
  following `send key "enter" to <terminal>` is needed to run it). `send key "<name>"
  [modifiers "control,shift"] to <terminal>` sends a key event. [sdef v1.3.1], [ScriptInputTextCommand](https://github.com/ghostty-org/ghostty/blob/v1.3.1/macos/Sources/Features/AppleScript/ScriptInputTextCommand.swift), [AppleScript docs]
- Also `initial input` in the configuration, and `perform action "<ghostty action>" on <terminal>`.
- So gx does **not** need herdr-style "type into the new shell". `command` + env + cwd is enough.

## What `split` / `new tab` return

- **Confirmed**: `split` returns the new `terminal`. `new tab` returns the new `tab`; its terminal is `focused terminal of <tab>` or `terminal 1 of <tab>`. `new window` returns a `window`. [sdef v1.3.1]
- **Confirmed**: a returned terminal has an object specifier `terminal id "<UUID>"` that stays valid for the surface's life, so it can be used later. [ScriptTerminal] L160-177, [AppDelegate+AppleScript] L104-110
- **Confirmed** verbs on a terminal: `focus`, `close` (no confirm dialog), `split`, `input text`, `send key`, `perform action`. Readable properties: `id`, `name` (title), `working directory`. Tab: `select tab`, `close tab`. Window: `activate window`, `close window`. [sdef v1.3.1]
- **Confirmed**: the new split takes focus (`replaceSurfaceTree(... moveFocusTo: newView ...)`). [BaseTerminalController] `newSplit`
- On a dead terminal, commands fail with "Terminal surface is no longer available." (`errAEEventFailed`, -10000). [ScriptTerminal]

## Targeting gx's own terminal

- **Confirmed**: in **1.3.1** there is **no** env var with the surface id. The only Ghostty vars
  given to children are `GHOSTTY_RESOURCES_DIR`, `GHOSTTY_BIN_DIR`, `GHOSTTY_SHELL_FEATURES`,
  `TERM`, `COLORTERM`, `TERMINFO`, `TERM_PROGRAM`, `TERM_PROGRAM_VERSION`. Same on `main`. [Exec.zig]
- **Confirmed**: in 1.3.1 the terminal class has only `id`, `name`, `working directory`. No `tty` or `pid`. [sdef v1.3.1]
- **Confirmed**: `main` (unreleased, after 1.3.1) adds `pid` (foreground process) and `tty` (e.g.
  `/dev/ttys016`) to `terminal`. [pid/tty commit]. With that, gx could use
  `first terminal whose tty is "<gx's tty>"`. Expected in 1.4 (**unknown**: no release date).
- Options on 1.3.1 (**derived, not tested**):
  1. `focused terminal of selected tab of front window`. Correct when the user just pressed a
     key in gx. `front window` is Ghostty's frontmost window even if Ghostty is not the active app.
  2. Set a unique title from gx (OSC 2), then `first terminal whose name is "<marker>"`, then
     restore the title. Fragile: shells/programs may rewrite the title.
  3. `working directory` matching is ambiguous (many terminals share a cwd).

## When the command exits

- **Confirmed (important)**: if `command` is set, Ghostty **forces `wait-after-command = true`**.
  The `wait after command` field can only turn it on; `false` does not turn it off. [embedded.zig] L529-534, L572-574
- **Confirmed**: with wait-after-command the surface "will stay open until any keypress is
  received". [config ref] `wait-after-command`
- **Derived**: so a split launched with `command` never closes by itself. If gx's `gx run`
  wrapper also asks "press Enter to close", the user gets two prompts. Options (**untested**):
  accept Ghostty's own wait prompt and drop gx's; or leave `command` unset (the user's shell
  starts) and type the command with `initial input` / `input text` (herdr-style, the split then
  stays as a shell).

## Permissions and errors

- **Confirmed**: access is "secured by macOS Automation permissions (TCC)": macOS prompts before
  another app can control Ghostty. [AppleScript docs], [1.3.0 notes]
- **Confirmed**: Ghostty's Info.plist has `NSAppleEventsUsageDescription = "A program running
  within Ghostty would like to use AppleScript."` (shown when a program *inside* Ghostty sends
  Apple events). Local `Info.plist` of 1.3.1.
- **Unknown**: whether a prompt appears when osascript runs **inside** Ghostty and targets Ghostty
  itself (TCC treats Ghostty as the responsible process, so sender and target are the same app).
  Needs the manual check on a real Ghostty.
- **Confirmed**: when `macos-applescript = false`, every command fails with
  `errAEEventNotPermitted` (-1743) and the message "AppleScript is disabled by the
  macos-applescript configuration." Properties return empty values. [AppDelegate+AppleScript] L300-316
- **Derived** (standard macOS behaviour, not checked here):
  - TCC denial: osascript exits 1 with `Not authorized to send Apple events to Ghostty. (-1743)`.
    Same code as "disabled by config"; tell them apart by the message.
  - Ghostty < 1.3 (no dictionary): the script does not compile; osascript exits 1 with a syntax
    error (-2741, e.g. "Expected end of line but found identifier"). Better: check
    `TERM_PROGRAM_VERSION` first.
  - Ghostty not running: `tell application "Ghostty"` starts it. Not a real case for gx (gx runs inside Ghostty).
- **Confirmed** other errors: bad direction -> -1715 "Missing or unknown split direction."; failed
  split -> -10000 "Failed to create split."; "Terminal is not in a splittable window." [ScriptTerminal]

## Limits, latency, quoting

- **Unknown**: no documented rate limit or latency. Handlers run on the main thread (`@MainActor`). [ScriptTerminal]
- **Derived** quoting advice:
  - AppleScript strings need `\` and `"` escaped. To avoid that, pass values as arguments:
    `osascript - <cwd> <cmd> <env...>` with the script on stdin and `on run argv`.
  - The `command` string is then parsed by bash (see "How `command` is run"), so args need shell quoting.

## Impact on the map's charting decisions

- No decision is contradicted. The user has 1.3.1, which has split/new tab with command, cwd, env.
- Caveats for the spec:
  - The API is a **preview**; 1.4 may break it.
  - No reliable way to target gx's own terminal on 1.3.1 (`tty` only on unreleased `main`).
  - Ghostty forces "wait after command" when `command` is set, which affects the `gx run`
    "press Enter to close" behaviour (open question in the map).
  - The command does not inherit gx's env or the user's shell PATH.
