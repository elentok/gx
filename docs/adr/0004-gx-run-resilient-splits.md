# ADR 0004 — `gx run` for error-resilient split/tab commands

## Status
Accepted

## Context

`gx` launches some commands into a new tmux/kitty split or tab (e.g. `git commit`,
`git rebase -i`, opening `$EDITOR` on a file). When such a command **fails**, the
split/tab closes immediately and the output — including the error that explains
*why* — is gone before the user can read it.

A partial mechanism already exists in `ui/terminalrun`: `splitShellCommand(command,
keepOpen)` wraps the command in a fish-or-sh snippet that prints a "press Enter to
close" line and `read`s a key. It has two problems:

- It keeps the pane open on **both** success and failure (annoying on success).
- It is wired into `git rebase -i` only, and it depends on `$SHELL` detection
  (currently only fish vs. sh) plus per-argument shell escaping into a single
  string — fragile with complex arguments and non-fish/sh shells.

For tmux/kitty the launch call (`exec.Command("tmux"…).Run()`) returns as soon as
the split is *spawned*; the `done(err,…)` callback's `err` reflects only whether the
split launched, never the inner command's exit code. So the parent gx never learns
whether the inner command succeeded.

## Decision

Add an internal `gx run <program> [args…]` subcommand and have `ui/terminalrun`
prepend it to the command it launches into a split/tab (and to the in-place
`tea.ExecProcess` path). The split therefore runs `gx run git commit` rather than
`git commit` directly.

`gx run`:

- Uses `DisableFlagParsing: true`; everything after `run` is the child program
  (`args[0]`) and its arguments (`args[1:]`) verbatim — no flag interpretation.
  (Same mold as the existing `gx stashify`.)
- **Inherits** stdio so interactive children (the commit `$EDITOR`, the rebase todo
  editor) work and their output lands directly in the pane.
- On **zero** exit: returns immediately; the pane closes as before.
- On **non-zero** exit: prints a footer beneath the already-visible output and blocks
  until the user presses **Enter**, then exits with the child's code:

  ```
  ─────────────────────────────────
  gx: command failed (exit 1)
  $ git commit -m "fix thing"
  press Enter to close…
  ```

  The `$ …` line is the full command (shell-style quoting on args with spaces, for
  display only) so the user can see exactly what ran.
- Propagates the child's exit code via the existing `*cmd.ExitError` →
  `main.go` pass-through (the same path `gx stashify` uses), so the pane's own
  context (`$?`, tmux `remain-on-exit`) sees the real result.

`ui/terminalrun` resolves gx's own absolute path via `os.Executable()` (cached,
falling back to `"gx"`) because the split runs in a fresh shell where `gx` may not be
on `PATH`.

Scope: wrap the **command** launches (commit, rebase, edit). **Interactive shell**
launches (the worktrees terminal menu) stay bare — a shell's exit code is whatever
its last command was, and "press Enter to close" is meaningless there. The parent-side
behavior is unchanged: gx still refreshes on the existing `*FinishedMsg`; all failure
visibility lives inside the split.

The old `splitShellCommand`, `escapeShellArg`, and the `keepOpen` parameter on
`CommandCustom` are deleted.

## Considered Options

- **Extend the inline shell-wrapper** (make `keepOpen` keep-open-only-on-error and
  wire it everywhere). Rejected: keeps the `$SHELL` detection and per-arg escaping
  fragility, and can't cleanly capture/propagate the exit code.
- **Per-multiplexer "remain on exit"** (tmux `remain-on-exit on`, kitty
  `--hold`). Rejected: terminal-specific, keeps the pane open on success too, no
  uniform prompt, and does nothing for the in-place/plain-terminal path.

## Consequences

- The split now runs gx recursively (`gx run …`). A future reader seeing `gx run git
  commit` in `ui/terminalrun` needs this ADR to understand why.
- Plain-terminal users benefit too: the in-place `tea.ExecProcess` path is wrapped,
  so a failed `git commit` pauses before the TUI redraws over its output.
- `gx run` is registered but hidden from `--help`; it can be promoted to a documented
  command later without churn.
