# ADR 0003 — Migrate the CLI to cobra

## Status
Proposed

## Context

The CLI was a hand-rolled `switch` in `cmd/cmd.go` over `args[0]`, with a `deps` struct of
injected function pointers (`runLog func(string) error`, …) as the testability seam — all of
`cmd/cmd_test.go` drives the CLI through `execute(args, deps)`. Adding a `-f`/`--file` flag to
`gx log` (so it opens pre-filtered to a file, like the status `gh` mapping) was the trigger: rather
than hand-parse flags, we wanted a real CLI framework, partly to get shell completion for
`gx log -f <TAB>`.

## Decision

Adopt `github.com/spf13/cobra` for the entire CLI (all commands, not a hybrid). Cobra was chosen
over urfave/cli for its larger ecosystem and completion parity (it also powers `gh`).

The `deps` seam is preserved via a command factory: `newRootCmd(d deps) *cobra.Command` builds the
whole command tree with each `RunE` closing over `d`. `Execute()` becomes
`newRootCmd(defaultDeps()).Execute()`. Tests build the root with fake deps and drive it with
`SetArgs` + `SetOut`/`SetErr`.

Error handling: `SilenceErrors = true` and `SilenceUsage = true` on the root, returning errors
unchanged to `main.go`. This preserves the existing contract where `main.go` is the single printer
and unwraps `*cmd.ExitError` for child-process exit-code pass-through (used by `gx stashify`) —
cobra knows nothing about `ExitError`, so `main.go` must inspect the error regardless, and letting
cobra print would re-emit `Error: exit status N` for what is meant to be a silent pass-through.

## Considered Options

- **Hand-parse `-f` for the `log` command only** — smallest diff, no dependency. Rejected: user
  wanted a proper CLI library and the completion payoff.
- **Incremental (cobra root, migrate `log` only)** — leaves two parsing styles and a half-rewired
  `deps` seam. Rejected as the worst of both worlds.

## Consequences

- Help/usage text switches from the hand-written `printUsage` to cobra-generated output
  (`Use`/`Short`/`Long` per command). Nothing tested the exact text.
- `gx completion <shell>` is available; `-f` is registered with file-path completion.
- `deps.runLog` changes from `func(string) error` to `func(LogOptions) error` (`LogOptions{Ref,
  File string}`) to carry the file filter and leave room for future options without further churn.
