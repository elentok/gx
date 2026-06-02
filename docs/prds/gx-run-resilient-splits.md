# PRD: `gx run` — error-resilient split/tab commands

See [ADR 0004](../adr/0004-gx-run-resilient-splits.md) for the decision record.

## Problem Statement

When I launch a command into a new split or tab from gx — `git commit`, `git rebase
-i`, or opening `$EDITOR` on a file — and that command **fails**, the split/tab closes
instantly. The output that would tell me *why* it failed (the commit hook message, the
"nothing staged" error, a merge conflict) is gone before I can read it. I'm left
staring at the gx UI with no idea what went wrong.

I don't want the pane to *always* stay open — on success it should just close. I only
want it to stick around when there was an error, so I can read it and dismiss it.

## Solution

gx commands that run in a split/tab are launched through a new internal `gx run`
wrapper. When the wrapped command **succeeds**, the pane closes immediately, exactly
as today. When it **fails**, the pane stays open showing the command's output plus a
footer:

```
─────────────────────────────────
gx: command failed (exit 1)
$ git commit -m "fix thing"
press Enter to close…
```

The footer shows the exit code and the exact command that ran, so I can see what
failed at a glance. Pressing Enter closes the pane. This also helps on plain terminals
(no tmux/kitty), where the command runs in-place and the TUI would otherwise redraw
over the error.

## User Stories

1. As a gx user, I want a failed `git commit` in a split to keep the split open, so that I can read why the commit failed.
2. As a gx user, I want a successful `git commit` split to close immediately, so that my workflow isn't interrupted by an extra keypress.
3. As a gx user, I want the failed command's full output to remain visible, so that I can diagnose the failure without re-running anything.
4. As a gx user, I want the failure footer to show the exact command that ran (with its arguments), so that I can see precisely what was executed.
5. As a gx user, I want the failure footer to show the exit code, so that I can distinguish kinds of failures.
6. As a gx user, I want to press Enter to dismiss the failed pane, so that closing it is simple and predictable.
7. As a gx user, I want a failed `git rebase -i` (e.g. conflict or abort) to keep its pane open, so that I can see the rebase output.
8. As a gx user, I want a clean `git rebase -i` to close its pane immediately, so that the common case stays frictionless.
9. As a gx user editing a file in a split, I want a non-zero editor exit to keep the pane open, so that I notice when the editor reported a problem.
10. As a gx user, I want the interactive `$EDITOR` inside a wrapped split to behave normally (full TTY), so that wrapping doesn't break editing.
11. As a gx user on a plain terminal (no multiplexer), I want a failed in-place `git commit` to pause before the TUI redraws, so that I can still read the error.
12. As a gx user, I want opening an interactive shell from the worktrees terminal menu to be unaffected, so that the shell's own exit doesn't trigger a meaningless "press Enter" prompt.
13. As a gx user, I want the wrapper to find the gx binary even when `gx` isn't on the split's `PATH`, so that wrapping works regardless of shell setup.
14. As a gx user, I want the wrapped command's real exit code to propagate to the pane's shell, so that `$?` and tmux `remain-on-exit` reflect the true result.
15. As a gx maintainer, I want `gx run` registered but hidden from `--help`, so that it doesn't clutter the user-facing command list while remaining usable.
16. As a gx maintainer, I want `gx run` to pass all arguments through verbatim (no flag parsing), so that `git commit -m "x"` and similar reach the child untouched.
17. As a gx maintainer, I want the fragile fish/sh shell-wrapper and per-arg escaping removed, so that there's a single, robust mechanism to maintain.
18. As a gx maintainer, I want the run/footer/wait logic in a deep, isolated module, so that it can be unit-tested without spawning real splits.
19. As a gx user, I want parent gx behavior (status refresh after commit) unchanged, so that the only difference is failure visibility inside the split.

## Implementation Decisions

**Modules**

- **`runner` (new deep module).** Encapsulates the run-child-and-keep-open-on-failure
  behavior behind a small interface: run a program with given args and inherited
  stdio (in/out/err), return the child's exit code. On non-zero exit it writes the
  failure footer to the output stream and blocks reading from the input stream until
  it sees an Enter. It knows nothing about cobra or terminal detection. Includes a
  display-only command-quoting helper (quotes args containing spaces/specials). This
  is the unit-testable core.
- **`gx run` cobra command (thin, in `cmd/`).** Mirrors the existing `gx stashify`:
  `Hidden: true`, `DisableFlagParsing: true`. Treats `args[0]` as the child program
  and `args[1:]` as its arguments verbatim; rejects empty args with a usage error.
  Delegates to the `runner` module wired to `d.stdin/d.stdout/d.stderr`. Maps a
  non-zero result to `*ExitError{Code}` so `main.go` forwards the exit code (the same
  pass-through `gx stashify` uses).
- **`terminalrun` (modify).** Adds gx-path resolution via `os.Executable()` (resolved
  once, cached; fallback to `"gx"`). Adds a rewrite step that turns a launch's
  `(program, args)` into `(gxPath, ["run", program, args…])`, applied to the
  split/tab branches and the in-place `tea.ExecProcess` branch for command launches
  (commit, rebase, edit). Deletes `splitShellCommand`, `escapeShellArg`, and the
  `keepOpen` parameter on `CommandCustom`; updates the `git rebase -i` call site
  accordingly.

**Behavior**

- Stdio is inherited by the child so interactive children (commit/rebase editors)
  keep a real TTY; the failure footer is appended beneath whatever output already
  printed.
- Zero exit → return immediately, no footer, pane closes. Non-zero exit → footer +
  block on Enter → exit with the child's code.
- Footer format: a separator rule, `gx: command failed (exit N)`, `$ <quoted
  command>`, then `press Enter to close…`.

**Scope of wrapping**

- Wrapped: command launches for `git commit`, `git rebase -i`, and file edits, in
  both the split/tab and in-place paths.
- Not wrapped: interactive shell launches from the worktrees terminal menu.
- Parent-side gx behavior is unchanged — it still refreshes on the existing
  `*FinishedMsg`; for tmux/kitty the launch call still returns when the split spawns
  and does not learn the inner exit code. All failure visibility lives in the split.

## Testing Decisions

Good tests here assert **external behavior** through a module's public seam, not
internals: given fake stdin/stdout and a program that exits 0 vs. non-zero, assert the
returned code, whether a footer was written, and that an Enter on stdin unblocks. No
real splits/multiplexers are spawned.

- **`runner` module** — unit tests: success returns 0 and writes no footer; failure
  writes the footer (containing the exit code and the quoted command) and returns the
  child's code; a blocking read unblocks when Enter arrives on the fake input. Test
  the command-quoting helper directly (plain args vs. args with spaces/quotes).
- **`gx run` cobra command** — drive through the existing `execute(args, deps)` seam
  with fake deps (as in `cmd/cmd_test.go`): non-zero child maps to `*ExitError` with
  the right code; empty args produce a usage error; args after `run` reach the child
  verbatim (including flag-like tokens such as `-m`).
- **`terminalrun` wiring** — test that command launches are rewritten to invoke
  `gx run …` while interactive-shell launches are not, and that gx-path resolution
  falls back to `"gx"` when `os.Executable()` is unavailable. Follow the existing
  `terminalrun_test.go` style.

Prior art: `cmd/cmd_test.go` (cobra seam with fake deps, `*ExitError` assertions for
`gx stashify`), `cmd/stashify_test.go`, and `ui/terminalrun/terminalrun_test.go`.

## Out of Scope

- "Press **any** key" dismissal (raw-mode input). First cut is Enter only; an any-key
  upgrade can follow.
- Capturing and replaying child output (we inherit stdio instead).
- Wrapping interactive shell launches, or giving shells a keep-open prompt.
- New parent-side plumbing to learn the inner exit code for tmux/kitty splits.
- A configuration toggle for always/never keep-open.
- Promoting `gx run` to a documented, user-facing command (it stays hidden for now).

## Further Notes

- `gx run` deliberately reuses the `gx stashify` pattern (`DisableFlagParsing` +
  `<cmd...>` + `*ExitError` pass-through) so it slots into the existing CLI seam with
  no new error-handling machinery.
- The split running gx recursively (`gx run git commit`) is the surprising bit a
  future reader will question — ADR 0004 exists to explain it.
