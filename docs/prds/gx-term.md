# PRD — `gx term` CLI terminal launcher

## Problem Statement

`gx` already knows how to launch a command into a tmux/kitty split or tab (it does
this internally for `git commit`, `git rebase -i`, opening `$EDITOR`, and the
worktrees terminal menu). That capability lives entirely inside the TUI's
`ui/terminalrun` package and is only reachable from within the running app. As a
user, I can't reach it from the outside — e.g. from a neovim mapping I'd like to
open `lazygit`, a test run, or a shell into a kitty/tmux split next to my editor,
and have it fall back to running in place when I'm on a plain terminal. Today I'd
have to hand-write per-terminal `tmux split-window` / `kitty @ launch` invocations
and reimplement the fallback myself.

## Solution

Add a user-facing `gx term` command that launches a command (or `$SHELL`) into a
split, tab, or in place:

```
gx term [direction] [--cwd <dir>] [command [args...]]

gx term                  # shell, split below (default)
gx term --below nvim     # nvim in a split below, in the current dir
gx term --right lazygit  # lazygit in a side-by-side split
gx term --tab npm test   # npm test in a new tab
gx term --here ls        # run in the current terminal (exec-replace)
```

It honors the direction on tmux and kitty-with-remote-control, and falls back to
running in place anywhere else — so the same neovim mapping works on every
terminal. Failed commands keep their pane open (via `gx run`) so the error stays
readable; an interactive shell opens bare.

## User Stories

1. As a neovim user, I want to open `lazygit` in a split below my editor, so that I
   can review changes without leaving my session.
2. As a neovim user, I want to open a command in a side-by-side split, so that I
   can watch its output next to my code.
3. As a neovim user, I want to open a command in a new tab, so that it doesn't
   shrink my current pane.
4. As a neovim user, I want a single mapping to work whether I'm on tmux or kitty,
   so that I don't maintain per-terminal config.
5. As a user on a plain terminal (no multiplexer), I want `gx term` to run the
   command in place, so that the same command/mapping still works.
6. As a kitty user without remote control enabled, I want a clear hint that I need
   to enable it, so that I know why my split request ran in place instead.
7. As a user, I want `gx term` with no command to drop me into a shell in the
   chosen split/tab, so that I can quickly open a terminal in my current directory.
8. As a user, I want the split's working directory to default to where I invoked
   `gx term`, so that the new pane is in the right place.
9. As a tool/script author, I want a `--cwd` flag, so that I can set the launched
   command's directory explicitly.
10. As a user, I want `gx term` to work outside a git repository, so that I can use
    it as a general-purpose launcher.
11. As a user running a command that fails in a split, I want the pane to stay open
    showing the error, so that I can read why it failed before it disappears.
12. As a user opening a shell (no command), I want the pane to NOT pause on exit,
    so that closing the shell closes the pane as expected.
13. As a user, I want `--right` and `--below` to produce the same visual layout on
    tmux and kitty, so that the flag names are a reliable promise.
14. As a user, I want to pass a command with its own flags
    (`gx term --below nvim -u NONE file`), so that I don't have to escape or quote
    them.
15. As a user, I want `--here` to forward the command's exit code, so that a caller
    can tell whether it succeeded.
16. As a user, I want an error if I combine conflicting directions
    (`gx term --below --tab`), so that the behavior is never ambiguous.
17. As a user, I want `gx term` to appear in `gx --help`, so that it's
    discoverable.
18. As a maintainer, I want the CLI and the TUI to launch through the same code, so
    that per-terminal behavior can't drift between them.

## Implementation Decisions

See [ADR 0005](../adr/0005-gx-term-directional-flags.md) (directional flag naming)
and [ADR 0004](../adr/0004-gx-run-resilient-splits.md) (the `gx run` wrapper).

- **Command name**: `gx term`. The package keeps its `terminalrun` name.
- **Directions** (mutually exclusive flags): `--right`, `--below`, `--tab`,
  `--here`. Default (no flag) = `--below`. Named by visual outcome, not by the
  tmux/kitty `hsplit`/`vsplit` keyword (which means opposite things across the two
  terminals). No `--left`/`--above` — kitty cannot select a side. (ADR 0005)
- **Direction → `SplitType`**: `--right → HSplit`, `--below → VSplit`,
  `--tab → Tab`, `--here → InPlace`. The internal enum keeps its `HSplit`/`VSplit`
  names; the flag→enum mapping lives in the CLI layer.
- **Synchronous launch core**: extract an unexported `launchSplit(worktreeRoot,
  terminal, splitType, program, args) (splitApp string, err error)` in
  `ui/terminalrun`, holding the tmux/kitty arg-building + exec + kitty error
  formatting currently inlined in `commandWithSplit`. The TUI calls it inside its
  existing `tea.Cmd` closure; the CLI calls it (or a thin exported wrapper)
  directly. `InPlace` in the TUI still uses `tea.ExecProcess`.
- **Wrapping**: an explicit command is wrapped in `gx run` (pane stays open on
  failure — ADR 0004); `$SHELL` (no command) is launched **bare** (unwrapped),
  because a shell's exit code is meaningless — same rationale as the existing
  `*Bare` variants.
- **In-place mechanics**: `--here` and the non-splittable fallback
  `syscall.Exec`-replace the gx process (after chdir to the resolved cwd). Exit
  code is naturally the command's, signals pass straight through, no lingering gx
  parent. Exec-replace cannot wrap, which makes "in-place is never wrapped"
  automatic.
- **Arg parsing**: cobra with `SetInterspersed(false)` — gx's own flags must
  precede the command; everything from the command name onward passes through
  verbatim. `--` still works as an explicit terminator for a program name starting
  with `-`.
- **cwd**: inherit the current working directory by default; `--cwd <dir>`
  overrides. No git-repo requirement (unlike `status`/`log`/`show`).
- **Terminal detection**: reuse `ui.DetectTerminalFrom(getenv)` and
  `Terminal.CanSplit()`. Split/tab requested but `!CanSplit()` → in-place fallback.
- **Non-splittable fallback messaging**: plain terminal → silent in-place; kitty
  with `KITTY_WINDOW_ID` but no `KITTY_LISTEN_ON` → one-line stderr hint pointing
  at remote control, printed before the exec, then in-place.
- **Flag conflicts**: more than one direction flag → usage error.
- **Visibility**: registered visibly in `--help` (unlike the hidden `gx run`).
- **Modules**:
  1. `terminalrun.launchSplit` — deep module; synchronous per-terminal launch.
  2. CLI direction-resolution helper — pure flag→`SplitType` + conflict/default.
  3. `runTerm` — CLI orchestration glue (detection, cwd, wrap decision, split vs
     exec-replace, stderr hint), wired through the existing `deps` seam.

## Testing Decisions

Good tests here assert **external behavior** — the launch arguments produced, the
resolved `SplitType`, the chosen branch (split vs in-place), the exit/error
surfaced — not internal call sequencing. Three modules get tests:

- **Direction resolution** (pure helper): table test — each flag → expected
  `SplitType`; no flag → `VSplit`; multiple flags → error.
- **`runTerm` orchestration**: use the existing cobra `deps`/`SetArgs`/`SetOut`/
  `SetErr` seam (prior art: `cmd/cmd_test.go`, `cmd/run_test.go`). Cover arg
  pass-through with `SetInterspersed(false)` (e.g. `term --below nvim -u NONE`),
  `--cwd`, no-command→`$SHELL`, terminal-branch selection via an injected `getenv`
  (tmux/kitty-remote → split path; plain/kitty-no-remote → in-place), and the
  kitty-no-remote stderr hint. The actual `syscall.Exec` is the boundary — assert
  up to the decision/args, not the replacement itself.
- **`launchSplit` core**: table test over `SplitType` asserting the exact
  tmux/kitty argument vectors and kitty error formatting. Introduce an **exec
  seam** (a package var holding the command runner, like the existing
  `osExecutable` seam in `ui/terminalrun/terminalrun.go`) so the test verifies args
  without spawning real `tmux`/`kitty`. Prior art: `ui/terminalrun/terminalrun_test.go`
  and `ui/terminal_test.go` (injected `getenv`).

## Out of Scope

- `--left` / `--above` (kitty cannot select a side — ADR 0005).
- A per-invocation wrap override (`--keep-open`/`--no-keep-open`); the
  explicit-command-vs-shell rule is fixed.
- tmux session / kitty named-session behaviors from the worktrees terminal menu
  (those stay TUI-only).
- Changing the internal `SplitType` enum names or the TUI's existing default-split
  behavior.
- Multiple commands / shell pipelines as a single arg string — the command is a
  program + argv, not a shell line (consistent with `gx run`).

## Further Notes

- The package keeps its `terminalrun` name; only a new user-facing `gx term`
  command and the extracted `launchSplit` core are added.
- A neovim mapping example belongs in the README so the headline use case is
  discoverable.
- ADR 0005 should be flipped to Completed once shipped.
