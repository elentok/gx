# Plan — `gx term` CLI terminal launcher

Expose the TUI-only `ui/terminalrun` launcher as a user-facing CLI command:
open a command into a tmux/kitty split or tab, falling back to running in place
when no multiplexer is available. Headline use case: launching things from
neovim (e.g. `gx term --below lazygit`). See
[ADR 0005](../adr/0005-gx-term-directional-flags.md) for the flag-naming
rationale and [ADR 0004](../adr/0004-gx-run-resilient-splits.md) for the
`gx run` wrapping it reuses.

## Surface

```
gx term [direction] [--cwd <dir>] [command [args...]]
```

- Directions (mutually exclusive): `--right`, `--below`, `--tab`, `--here`.
- Default (no direction) = `--below`.
- No command → launch `$SHELL`.

## Decisions (from grilling)

- **Name**: `gx term`. The package keeps its `terminalrun` name internally.
- **Directions**: `--right`/`--below`/`--tab`/`--here`, default `--below`. Named
  by visual outcome, not tmux/kitty keyword. No `--left`/`--above` (kitty can't
  pick a side). See ADR 0005.
- **Direction → SplitType**: `--right → HSplit`, `--below → VSplit`,
  `--tab → Tab`, `--here → InPlace`. (Internal enum keeps `HSplit`/`VSplit`.)
- **Wrapping**: an explicit command is wrapped in `gx run` (pane stays open on
  failure); `$SHELL` (no command) is **unwrapped** (shell exit code is
  meaningless — same rationale as the existing `*Bare` variants).
- **In-place mechanics**: `--here` and the non-splittable fallback
  `syscall.Exec`-replace the gx process — exit code is naturally the command's,
  signals pass straight through, no lingering gx parent. Exec-replace can't (and
  shouldn't) wrap, which makes "in-place is never wrapped" automatic.
- **Arg parsing**: `SetInterspersed(false)` — gx's own flags must precede the
  command; everything from the command name onward is passed through verbatim.
  `--` still works as an explicit terminator for a program name starting with
  `-`.
- **cwd**: inherit the current working directory by default; `--cwd <dir>`
  overrides. **No git-repo requirement** — `gx term` works anywhere (unlike
  `status`/`log`/`show`).
- **Non-splittable fallback**: plain terminal → silent in-place; kitty without
  remote control (`KITTY_WINDOW_ID` set, no `KITTY_LISTEN_ON`) → one-line stderr
  hint pointing at remote control, then in-place. Printed before the exec.
- **Flag conflicts**: more than one direction → usage error.
- **Visibility**: shown in `--help` (unlike the hidden `gx run`).

## Phase 1 — extract a synchronous launch core in `terminalrun`

- [x] Add an unexported synchronous core, e.g.
      `launchSplit(worktreeRoot string, terminal ui.Terminal, splitType SplitType, program string, args []string) (splitApp string, err error)`,
      holding the tmux/kitty arg-building + `exec.Command(...).Run()` /
      `.CombinedOutput()` + kitty error formatting currently inlined in
      `commandWithSplit`.
- [x] Rewrite the tmux/kitty branches of `commandWithSplit` to call `launchSplit`
      inside their existing `tea.Cmd` closure (behavior-preserving). `InPlace`
      keeps using `tea.ExecProcess`; the `notify.Warning("split not supported")`
      fallback for non-splittable terminals stays for the TUI path.
- [x] `go test ./ui/terminalrun/...` green (existing tests unchanged).

## Phase 2 — `gx term` command

- [x] Add `cmd/term.go`: `newTermCmd(d deps) *cobra.Command` with
      `Use: "term [command...]"`, `Short`, `SetInterspersed(false)`, bool flags
      `--right`/`--below`/`--tab`/`--here` and string flag `--cwd`. Register it
      in `newRootCmd`'s `AddCommand` list.
- [x] `runTerm(args, d)`:
      - Resolve direction → `SplitType` (default `VSplit`/`--below`); error if
        >1 direction flag set.
      - Resolve cwd: `--cwd` if set, else `d.getwd()`.
      - Resolve program/args: if `len(args) == 0`, use `$SHELL` (fallback
        `/bin/sh`) and mark as a bare/shell launch; else `args[0]` + `args[1:]`
        as an explicit (wrapped) command.
      - Detect terminal via `ui.DetectTerminalFrom(d.getenv)`.
- [x] **Split/tab path** (terminal `CanSplit()` and direction ≠ `--here`):
      `wrapRun` the program/args **only for explicit commands** (not the shell),
      then call `terminalrun.launchSplit` (export it or add a thin exported
      `Launch` wrapper for the cmd package). gx returns once the pane is spawned.
- [x] **In-place path** (`--here`, or non-splittable terminal): if a split was
      requested on kitty-without-remote, print the one-line stderr hint. Then
      `syscall.Exec` the program with inherited stdio in the resolved cwd
      (chdir first). Never wrapped.
- [x] Map syscall.Exec failure (e.g. program not found) to a normal CLI error;
      the command's own exit code flows through the exec-replace naturally.

## Phase 3 — tests

- [x] `cmd/term_test.go`: direction → SplitType resolution; default = below;
      conflicting directions error; `--cwd` override; no-command → `$SHELL`;
      arg pass-through with `SetInterspersed(false)` (e.g.
      `term --below nvim -u NONE file`); `--` terminator.
- [x] terminal-detection branches (tmux/kitty-remote split vs plain/kitty-no-
      remote in-place) via injected `getenv`; assert the kitty-no-remote stderr
      hint.
- [x] `ui/terminalrun` core: a `launchSplit` unit test exercising the tmux/kitty
      arg construction (table-driven over `SplitType`), if not already covered.

## Phase 4 — docs

- [x] README: document `gx term` in the CLI/usage section with the neovim
      example.
- [x] CHANGELOG: add to the Unreleased section via the changelog skill.
- [x] Flip ADR 0005 status to Completed once shipped.
