# Ghostty split launches on macOS

Epic: `gx:ghostty-splits`. Handed off as `ghostty-splits-impl`. Background facts: the research note
`docs/research/ghostty-applescript-api.md` (branch `research/ghostty-applescript-api`). Respects
ADR 0004 (`gx run` for error-resilient splits), ADR 0005 (directional flags) and ADR 0035
(OS-specific code without build tags).

## Problem Statement

A gx user who works in Ghostty (without tmux, kitty or herdr) gets no **split launch**. Every
command gx would open beside the TUI — `git commit`, `git rebase -i`, `$EDITOR` from the `ee`/`es`/
`ev`/`et` keys, the comment and answer editors, `gx term --right lazygit` from neovim — runs as a
**takeover launch** instead, or shows "split not supported for this terminal". The worktree
terminal key does not open its menu and says "use tmux or kitty for more options". Ghostty 1.3.0 added an AppleScript
API that can open splits and tabs, but gx does not use it.

## Solution

gx detects Ghostty on macOS from `TERM_PROGRAM=ghostty` and treats it as a fourth split backend
next to tmux, kitty (with remote control) and herdr. A split launch on Ghostty runs `osascript`
with a small fixed AppleScript that opens a stacked split (`--below`), a side-by-side split
(`--right`) or a new tab (`--tab`) in the user's Ghostty window. The new split starts the user's
normal shell in the right directory, and gx types the command into it (the same way the herdr
backend does), followed by `exit`, so the split closes when the command ends. A failing command
still shows the `gx run` "press Enter to close" footer (ADR 0004).

When the AppleScript call cannot work — Ghostty older than 1.3.0, AppleScript turned off in the
Ghostty config, or macOS Automation permission denied — gx prints a short hint that names the fix
and runs the command in place, the same way it treats kitty without remote control. `gx doctor`
shows what gx detected, and `gx doctor --check-ghostty` runs a live, read-only AppleScript probe.

There is no config key. On Linux, Ghostty has no AppleScript, so gx treats it as a plain terminal.

## User Stories

1. As a Ghostty user on macOS, I want gx to detect Ghostty automatically, so that I do not need to
   set any config to get splits.
2. As a Ghostty user, I want `git commit` from the status tab to open in a split below the TUI, so
   that I can see my diff while I write the message.
3. As a Ghostty user, I want `git rebase -i` from the log tab to open in a split, so that the TUI
   stays visible during the rebase.
4. As a Ghostty user, I want `es` to open `$EDITOR` in a stacked split, so that it behaves the same
   as on tmux, kitty and herdr.
5. As a Ghostty user, I want `ev` to open `$EDITOR` side by side, so that I can edit next to the
   TUI.
6. As a Ghostty user, I want `et` to open `$EDITOR` in a new Ghostty tab, so that I get a full-size
   editor without leaving gx.
7. As a Ghostty user, I want `ee` to keep its current behaviour (default split), so that my muscle
   memory still works.
8. As a Ghostty user, I want the comment editor and the ticket answer editor to open in a split,
   so that they act like every other split launch.
9. As a Ghostty user, I want resuming an agent conversation from the history view to open in a
   split, so that it matches the other backends.
10. As a neovim user in Ghostty, I want `gx term --below lazygit` to open lazygit in a split below
    neovim, so that my existing mappings work in Ghostty.
11. As a neovim user in Ghostty, I want `gx term --right <cmd>` to open side by side, so that the
    layout matches tmux and kitty (ADR 0005).
12. As a Ghostty user, I want `gx term --tab <cmd>` to open a new Ghostty tab, so that long-running
    commands get their own tab.
13. As a Ghostty user, I want `gx term` with no command to open my shell in a split below, so that
    it acts like on the other backends.
14. As a Ghostty user, I want `gx term --here <cmd>` to still run in place, so that I can opt out of
    a split.
15. As a Ghostty user, I want the worktree terminal menu to offer hsplit, vsplit and tab, so that I
    can open a shell in any worktree next to gx.
16. As a Ghostty user, I want the worktree terminal menu to hide the "session" option, so that I
    am not offered something Ghostty cannot do.
17. As a Ghostty user, I want a shell opened from the worktree terminal menu to start in that
    worktree's directory, so that I can work there right away.
18. As a Ghostty user, I want the split to close by itself when the command succeeds, so that I do
    not have to close panes by hand.
19. As a Ghostty user, I want a failing command to keep its split open with the `gx run` footer,
    so that I can read the error (ADR 0004).
20. As a Ghostty user, I want only one "press Enter" prompt on failure, so that closing a failed
    split is one key, not two.
21. As a Ghostty user, I want the launched command to see my normal shell setup (PATH, aliases,
    rc files), so that `$EDITOR`, `lazygit` and `git` are found.
22. As a Ghostty user, I want focus to go back to the gx pane after the split closes, so that I can
    keep using the TUI without clicking.
23. As a Ghostty user, I want the "opened ghostty split" toast after a split launch, so that I know
    where the program went (GLOSSARY "Split launch").
24. As a Ghostty user with AppleScript turned off in my config, I want a hint that says
    `set macos-applescript = true`, so that I know the exact fix.
25. As a Ghostty user who denied the macOS Automation prompt, I want a hint that points to System
    Settings › Privacy & Security › Automation, so that I can allow it.
26. As a Ghostty user on a version older than 1.3.0, I want gx to run commands in place without
    calling osascript, so that I get a working command and no AppleScript compile error.
27. As a Ghostty user, I want any other osascript failure shown as an error with its output, and
    my command not run in place, so that a split that may already have opened never runs `git
    commit` or `git rebase` twice.
28. As a Ghostty user, I want a split launch that failed before any split opened (version gate,
    Automation or `macos-applescript` denied, AppleScript compile error, no Ghostty window) to
    still run my command in place, so that the action I asked for happens.
29. As a Ghostty user on Linux, I want gx to treat Ghostty as a plain terminal, so that nothing
    tries to call osascript.
30. As a user running tmux inside Ghostty, I want tmux to win, so that splits open as tmux panes
    like today.
31. As a user running herdr or kitty, I want detection to stay as it is, so that nothing changes
    for me.
32. As a Ghostty user, I want `gx doctor` to show `TERM_PROGRAM`, `TERM_PROGRAM_VERSION` and
    whether my version passes the gate, so that I can see why splits do or do not work.
33. As a Ghostty user, I want `gx doctor --check-ghostty` to tell me whether gx can reach Ghostty
    by AppleScript, so that I can debug before I try a split.
34. As a Ghostty user, I want `--check-ghostty` to be opt-in, so that plain `gx doctor` stays fast
    and never shows a macOS prompt.
35. As a user outside Ghostty, I want `gx doctor --check-ghostty` to say SKIP and exit 0, so that
    it does not fail scripts on other terminals.
36. As a Ghostty user, I want `--check-ghostty` to exit non-zero on FAIL, so that I can use it in a
    script.
37. As a reader of `gx term --help`, the README and GLOSSARY, I want all four backends (tmux, kitty,
    herdr, ghostty) listed, so that I know Ghostty is supported.
38. As a Ghostty user, I want paths and arguments with spaces or quotes to reach the command
    unchanged, so that odd file names do not break a split launch.
39. As a gx maintainer, I want the Ghostty backend tested on every OS through the existing
    `runCommand` seam, so that a Linux CI run catches a broken macOS backend (ADR 0035).

## Implementation Decisions

### Detection (`ui` terminal module)

- Add a `TerminalGhostty` value. Its `String()` label is `"ghostty"` and `CanSplit()` is true.
- Detection order: herdr → kitty (remote, then plain kitty) → tmux → ghostty → plain. Ghostty is
  detected when `TERM_PROGRAM` is `ghostty` **and** the OS is macOS. On any other OS it falls
  through to plain. The version gate is **not** part of detection (see below), so an old Ghostty is
  still `TerminalGhostty`.
- Per ADR 0035 the OS check uses an injected `goos` value, not build tags. The public
  `DetectTerminalFrom(getenv)` signature stays the same (it has many callers) and passes the real
  OS to a new exported `DetectTerminalFor(getenv, goos)`. Tests call `DetectTerminalFor`.
- `cmd` gets a `goos string` field on its `deps` struct (default `runtime.GOOS`). `gx term` and
  `gx doctor` detect with `ui.DetectTerminalFor(d.getenv, d.goos)`, so their ghostty test rows set
  `goos = "darwin"` and pass on Linux CI (US 39).
- No config key.

### Launch (`ui/terminalrun` module)

- `launchSplit` gets a `TerminalGhostty` case. It returns the app label `"ghostty"`.
- New helper `runOsaScript(script string, argv ...string)` that calls the existing `runCommand`
  seam as `runCommand("osascript", "-e", script, argv...)`. No new seam.
- One Go constant AppleScript per split type: `HSplit` → `split … direction down`, `VSplit` →
  `split … direction right`, `Tab` → `new tab in front window`. No `left`/`up` (ADR 0005). Each
  script reads its values from `on run argv`, so gx never escapes text into AppleScript source.
  The argv is always exactly two items: the working directory and the initial input (empty for a
  shell launch). Shape of the HSplit script (the contract, not final text):

  ```applescript
  on run argv
    set cwd to item 1 of argv
    set typed to item 2 of argv
    tell application "Ghostty"
      set cfg to new surface configuration
      set initial working directory of cfg to cwd
      if typed is not "" then set initial input of cfg to typed
      split (focused terminal of selected tab of front window) direction down with configuration cfg
    end tell
  end run
  ```

- **Typed input, not `command`.** Setting `command` makes Ghostty force wait-after-command (an extra
  "press any key" on every exit, on top of the `gx run` footer), and it runs outside the user's
  shell rc (no user PATH). So the surface starts the user's own shell, and the command goes in
  `initial input` as `'<gx>' 'run' '<prog>' '<arg>'…; exit` plus a newline. Every token is quoted
  with the existing `shellQuote` helper, as in the herdr backend. The gx path is absolute
  (`gxPath`, ADR 0004).
- **Shell launches** (the worktree terminal menu and `gx term` with no command) set no initial
  input: the new split already runs the user's shell. The caller says it is a shell launch
  explicitly; `terminalrun` never compares `program` to `$SHELL`. New entry points:
  `CommandWithSplitShell(worktreeRoot, terminal, splitType, shell, done)` for the TUI (the worktree
  menu calls it), and a shell option on `Launch` for `gx term` (which already knows `isShell`). On
  tmux, kitty and herdr these pass `shell` as the program, exactly as today; on Ghostty they send
  empty input. Bare non-shell launches (the history "resume conversation" action) still go through
  `CommandWithSplitBare` and get typed input, just without the `gx run` prefix.
- `initial working directory` is the worktree root / `--cwd` value, already absolute. Ghostty
  silently ignores a bad path, so no extra check.
- No `environment variables`. Known gap: variables that exist only in gx's process (for example
  from direnv or a nix shell) do not reach the split. The user's shell rc runs, so PATH is fine.
- **Targeting:** always `focused terminal of selected tab of front window`. No fallback. If the user
  switches Ghostty windows between the key press and the osascript call, the split lands in the
  other window; this race is accepted. (Ghostty 1.3.1 has no surface id env var; `tty` targeting
  arrives only after 1.3.1.)
- **Focus:** the new split takes focus (Ghostty does this). gx adds no refocus command (unlike
  herdr) and relies on Ghostty refocusing the neighbour when the split closes. The manual check
  verifies this. Fallback if it fails: the script returns the source terminal's `id` before the
  split, and the typed input ends with an osascript call that focuses `terminal id <id>`.
- **Version gate:** before any osascript call, read `TERM_PROGRAM_VERSION`. If it is below 1.3.0
  or cannot be parsed, do not call osascript; return a fallback error with the hint
  `Ghostty <version> has no AppleScript splits (needs 1.3.0 or later)`. No upper bound (1.4 may
  break the API; the failure path below covers that). Parse the leading `MAJOR.MINOR.PATCH` and
  ignore a `-…` or `+…` suffix.
- **Failure hints.** On a non-zero osascript exit, classify its output:
  - contains `-1743` and `macos-applescript` → `Ghostty: set macos-applescript = true in your Ghostty config`
  - contains `-1743` (any other text) → `allow gx/Ghostty under System Settings › Privacy & Security › Automation`
  - contains `-2740`, `-2741` (compile error), `-1708` (doesn't understand) or `-1728` (can't get
    front window) → `Ghostty split failed: <osascript output>`
  - anything else → `Ghostty split failed: <osascript output>`
- **Fallback contract.** Only failures where no split can have opened fall back: the version gate,
  `-1743` (both hints), and `-2740`/`-2741`/`-1708`/`-1728`. They return one typed fallback error
  that carries the hint. Every caller that gets it runs the same program in place:
  - `gx term`: print `gx: <hint> — running in place.` to stderr, then exec-replace (same path as
    kitty without remote control).
  - TUI (`Command`, `CommandWithSplit`, `CommandWithSplitBare`, `CommandWithSplitShell`): show the
    hint as a warning toast and run the program as a takeover launch (`tea.ExecProcess`, still
    wrapped in `gx run` when the split would have been). The `done` callback gets
    `splitApp == ""`, as for any in-place run. Mechanism: the async `tea.Cmd` closure that ran
    osascript returns `tea.Batch(notify.Warning(hint), tea.ExecProcess(c, cb))()`; bubbletea v2
    handles the returned batch message, so no synchronous pre-check is needed.
  The fallback lives in `terminalrun`, so callers outside it only need to handle the error once.
- **Any other osascript failure** (a split may already have opened) is a plain error, never a
  fallback, so a command is never run twice. `gx term` prints it to stderr and exits non-zero; the
  TUI passes it to `done` as a launch error, as for a failed tmux or kitty split.
- `Command` (the default-split path used by commit, rebase, `ee`, the comment and answer editors)
  gets a Ghostty branch that goes through `launchSplit` with `HSplit`, like its herdr branch.

### Worktree terminal menu (`ui/worktrees`)

- On Ghostty the menu shows hsplit, vsplit and tab only; "session" is hidden. Menu items become a
  function of the terminal.
- hsplit/vsplit/tab call `CommandWithSplitShell` with the user's shell, on every backend.
- Both info strings become "use tmux, kitty, herdr or ghostty for more options": the plain-terminal
  text on the open-terminal key (`model_keys.go`, today "use tmux or kitty", missing herdr too) and
  the unreachable default in `executeTerminalAction`. Ghostty users on Linux or on Ghostty < 1.3.0
  still see it, because detection or the gate leaves them on plain / in-place launches.

### `gx doctor` (`cmd`)

- The Runtime block adds `TERM_PROGRAM=…` and `TERM_PROGRAM_VERSION=…` lines. When the detected
  terminal is ghostty it also prints a gate line: `ghostty version: 1.3.1 ok` or
  `ghostty version: <v> < 1.3.0, splits run in place`. No osascript call in plain `gx doctor`.
- New opt-in flag `--check-ghostty`, shaped like `--check-blocked-form` (it runs instead of the
  repo checks):
  - Not ghostty (including Linux, where detection gives plain) → `SKIP: terminal is <label>, not ghostty`, exit 0.
  - Version gate fails → `FAIL: <version hint>`, non-zero exit, no probe.
  - Otherwise run the read-only probe `tell application "Ghostty" to count windows` through
    `runOsaScript`. Success → `PASS: Ghostty AppleScript reachable`. Failure → `FAIL: <hint>`
    (same three hints as launches), non-zero exit.
  - It may trigger the macOS Automation prompt; this is intended. Its help text says "may show a
    macOS Automation prompt".
- `terminalrun` exports the probe (one function returning the fallback error or nil) and the gate
  check, so doctor and launches share the hints. `cmd` reaches the probe through a new field on its
  existing `deps` struct (same pattern as `explainAgent`), so doctor tests do not run osascript.

### Docs

- `gx term` short and long help, the README `gx term` section, and GLOSSARY "Split launch" list all
  four backends: tmux, kitty (with remote control), herdr, ghostty (macOS, 1.3.0+). README notes the
  Automation prompt and the `macos-applescript` setting.

## Testing Decisions

- Good tests check external behaviour: which terminal is detected, which exact command gx would run,
  what the user sees (hint, toast, doctor line), and which fallback happens. They do not check
  internal helpers or call order beyond that.
- **Detection** (`ui` terminal tests, table style as in the existing detection tests): ghostty on
  darwin → ghostty; ghostty on linux → plain; ghostty plus `TMUX` → tmux; ghostty plus kitty vars →
  kitty; ghostty plus `HERDR_ENV` → herdr. `String()` and `CanSplit()` rows for ghostty.
- **Launch** (`ui/terminalrun` tests, swapping `runCommand` as the existing tmux/kitty/herdr tests
  do): for each split type assert the call is `osascript`, `-e`, that type's script constant, then
  argv `[cwd, input]`. Assert the exact input string, including an arg with a space and an arg with
  a single quote. A shell launch (`CommandWithSplitShell`, `Launch` with the shell option) sends an
  empty input; on tmux/kitty/herdr it still passes the shell as the program. A bare non-shell
  launch has no `gx run` prefix. `Command` on ghostty uses the `down` script.
- **Version gate:** versions `1.2.9`, empty and `garbage` → no `runCommand` call and a fallback
  error with the version hint; `1.3.0`, `1.3.1`, `1.4.0` and a suffixed version → osascript is
  called.
- **Failure hints:** fake osascript output for the macos-applescript, Automation, each pre-launch
  code and an unknown error → the matching hint text, and a fallback error only for the pre-launch
  cases.
- **TUI fallback:** the `CommandWithSplit` table gains ghostty rows; a fallback error makes the
  closure return a batch message holding a warning toast and an exec (in-place run). An unknown
  osascript error reaches `done` as an error with no exec.
- **`gx term`** (`cmd` term tests with the existing `launchInSplit`/`execReplace` seams, `goos =
  "darwin"`): on ghostty a fallback error prints the hint to stderr and exec-replaces; a non-fallback
  error prints it and exits non-zero without exec-replace; a successful launch does neither. Help
  text names all four backends.
- **`gx doctor`** (`cmd` doctor tests, as for `--check-blocked-form`): Runtime block shows the new
  lines and the gate line; `--check-ghostty` PASS, each FAIL, version-gate FAIL without a probe,
  and SKIP with exit 0.
- **Worktree menu:** on ghostty the menu has no "session" item, and the other three items reach
  `CommandWithSplitShell` with ghostty. On plain, the open-terminal key shows the new info string.
- **Manual check on a real Ghostty (macOS, 1.3.x).** Run once before closing the ticket:
  1. `gx doctor --check-ghostty` from a shell inside Ghostty. Record whether macOS shows an
     Automation prompt for a process inside Ghostty (open fact from the research).
  2. `gx term --below`, `--right`, `--tab` with a command, and with no command.
  3. From the TUI: commit, `es`/`ev`/`et`, rebase, worktree menu hsplit/vsplit/tab.
  4. A failing command shows the `gx run` footer once, and Enter closes the split.
  5. After the split closes, focus returns to the gx pane. If not, apply the focus fallback above.
  6. With `macos-applescript = false`, a split prints the hint and runs in place.
  7. Shell setup (US 21): in a `gx term --below` split, `command -v lazygit "$EDITOR"` succeeds and
     a PATH entry added only in the rc file is present; `es` from the TUI finds `$EDITOR`.
  8. Typed input survives shell startup: repeat `gx term --below <cmd>` with zsh (with a real rc,
     e.g. powerlevel10k instant prompt), fish and bash as Ghostty's shell. If one drops the typed
     command, file a follow-up (see Further Notes).

## Out of Scope

- A Ghostty agent runner: agents keep running in herdr, headless or PTY runners.
- Ghostty splits on Linux (no AppleScript there).
- `--left` / `--above` directions (ADR 0005).
- A Ghostty "session" (for example a new window) in the worktree terminal menu. Possible follow-up.
- Passing gx's environment to the split (`environment variables`).
- `tty`-based targeting of gx's own terminal (needs a Ghostty release after 1.3.1).

## Further Notes

Assumptions made while writing this spec (the map did not decide them):

- The hint text for the version gate (`Ghostty <version> has no AppleScript splits (needs 1.3.0 or
  later)`) and the rule that a `-…`/`+…` suffix on the version is ignored, so tip builds pass.
- The fallback is one typed error handled inside `terminalrun`, and in the TUI it shows as a
  warning toast followed by a takeover launch.
- A Ghostty shell launch starts Ghostty's configured shell, which is usually but not always
  `$SHELL`.
- Known risk, accepted: `initial input` is typed before the shell's prompt is ready (herdr needed
  `waitForPaneShell` for this). A shell setup that flushes typeahead could drop the command.
  Ghostty has no readiness API; the manual check tests zsh, fish and bash, and a failure becomes a
  follow-up.
- `new tab` targets `front window` (same rule as splits) rather than Ghostty's preferred parent
  window.
- `cmd` gets two new `deps` fields, the probe and `goos`, so term and doctor tests stay hermetic
  on every OS; the map's "no new seam" was read as "no new seam in `terminalrun`".
- The doctor gate line is shown only when the detected terminal is ghostty.

Open questions (answer during the manual check or implementation):

- Does a process inside Ghostty get the Automation prompt when it targets Ghostty itself? The
  manual check answers this.
- Does Ghostty reliably refocus the source pane when a split closes, with more than two splits in
  the tab? If not, the focus fallback above is needed.
- If the user sets `wait-after-command = true` in their Ghostty config, the split stays open after
  `exit`. Accepted for now; mention in the README if it shows up.
- Ghostty's AppleScript API is a preview and may break in 1.4. No upper version bound is set; the
  "Ghostty split failed" hint plus in-place run is the safety net.
