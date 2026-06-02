# Plan: `gx run` for error-resilient split/tab commands

See [ADR 0004](../adr/0004-gx-run-resilient-splits.md) for the decision and rationale.

## Goal

When a command launched into a split/tab (or in-place) **fails**, keep the pane open
with the output visible and a `press Enter to close…` prompt, instead of the pane
vanishing. Succeeding commands close immediately as before. Interactive shells are
unaffected.

## Tasks

### 1. The `gx run` subcommand

- [x] Add `newRunCmd(d deps)` in `cmd/` (mirroring `newStashifyCmd`): `Use: "run
      <cmd...>"`, `Hidden: true`, `DisableFlagParsing: true`, `RunE` → `runRun(args, d)`.
- [x] Register it in `newRootCmd`'s `AddCommand(...)`.
- [x] Implement `runRun(args, d)`:
  - [x] Reject empty args with a usage error.
  - [x] `exec.Command(args[0], args[1:]...)` with stdin/stdout/stderr inherited from `d`.
  - [x] On success (nil/zero): return nil.
  - [x] On non-zero: print the failure footer (separator, `gx: command failed (exit
        N)`, `$ <quoted command>`, `press Enter to close…`), block on an Enter read
        from `d.stdin`, then return `&ExitError{Code: N}` so `main.go` forwards the code.
  - [x] Add a display-only command-quoting helper (quote args containing spaces/specials).

### 2. Wire `gx run` into `ui/terminalrun`

- [x] Add gx-path resolution: `os.Executable()` cached once, fallback `"gx"`.
- [x] Introduce a helper that rewrites `(program, args)` → `(gxPath, ["run", program, args…])`.
- [x] Apply it in the split/tab and in-place branches of `Command` / `CommandCustom` /
      `CommandWithSplit` for command launches (commit, rebase, edit).
- [x] Do **not** apply it to the interactive-shell launches in
      `ui/worktrees/terminal_menu.go`.

### 3. Remove the old shell-wrapper

- [x] Delete `splitShellCommand` and `escapeShellArg` from `ui/terminalrun/terminalrun.go`.
- [x] Remove the `keepOpen` parameter from `CommandCustom` (and collapse `Command` if it
      becomes a trivial wrapper).
- [x] Update the `git rebase -i` call site (`ui/log/model_rebase.go`) to the new signature.
- [x] Remove now-dead tests / update `terminalrun_test.go`.

### 4. Tests

- [x] `cmd` test for `gx run`: success returns nil; failure prints footer + returns
      `*ExitError` with the child's code; Enter unblocks (drive via fake stdin).
- [x] `terminalrun` test: command launches are rewritten to `gx run …`; shell launches
      are not; gx-path fallback works.

### 5. Manual verification

- [ ] In tmux: `git commit` with nothing staged → split stays open showing the error +
      prompt; pressing Enter closes it.
- [ ] In tmux: successful `git commit` → split closes immediately.
- [ ] In kitty (remote control): same two cases.
- [ ] Plain terminal: failed `git commit` pauses before the TUI redraws.
- [ ] `git rebase -i` aborted/failed → pane stays open; clean rebase closes.
