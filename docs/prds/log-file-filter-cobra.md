# PRD — `gx log -f/--file` via cobra CLI migration

## Problem Statement

As a `gx` user I can already open the log pre-filtered to a file from inside the TUI — selecting a
file in the status view and pressing `gh` opens the log scoped to that file (and a line range when
focused on the diff). But there's no way to do this from the shell. If I'm already on the command
line looking at a file, I have to launch `gx`, navigate to status, find the file in the filetree,
and press `gh` — instead of just asking for it directly.

Separately, the CLI is a hand-rolled `switch` over `args[0]` with bespoke positional-argument
checks. It has no flag parsing, no per-command help, and no shell completion, so adding any flag
(like `--file`) means hand-writing an argument parser.

## Solution

Add a `-f`/`--file` argument to `gx log` so it opens pre-filtered to a file, equivalent to the
status `gh` mapping:

```
gx log -f ui/log/model.go
gx log --file=ui/log/model.go abc123
```

The path is typed cwd-relative (so shell tab-completion works from any subdirectory) and resolved
to the repo-relative form the log filter expects. Renamed files show their full history across the
rename.

To get there cleanly, the CLI is migrated to `github.com/spf13/cobra` (see
[ADR 0003](../adr/0003-cobra-cli.md)). This is the bulk of the work; the flag rides on top. The
migration also yields per-command `--help` and `gx completion <shell>` for free, including
file-path completion on `-f`.

## User Stories

1. As a shell user, I want `gx log -f <path>`, so that I can open the file-filtered log without
   navigating the TUI.
2. As a shell user, I want both `-f` and `--file`, so that I can use whichever form I prefer.
3. As a shell user, I want `--file=<path>` to work, so that the long flag behaves like every other
   GNU-style flag.
4. As a shell user, I want to combine the file filter with a ref (`gx log -f <path> <ref>` or
   `gx log <ref> -f <path>`), so that I can scope a file's history starting at a given commit.
5. As a shell user standing in a subdirectory, I want to pass a path relative to my current
   directory, so that tab-completion gives me a working argument.
6. As a shell user, I want a clear error if the path resolves outside the repository, so that I'm
   not silently given an empty or wrong result.
7. As a shell user, I want a non-existent or history-less path to simply show an empty log (like
   `git log -- <path>`), so that the behavior is predictable and matches git.
8. As a shell user, I want `gx log -f <path>` to behave the same as the status `gh` mapping, so
   that I get one consistent file-filtered-log experience regardless of entry point.
9. As a shell user filtering a renamed file, I want to see history from before the rename, so that
   I get the file's full story.
10. As a TUI user pressing `gh`, I want rename history too, so that the in-app file log is as
    complete as the CLI one.
11. As a shell user, I want `gx log --help`, so that I can discover the `-f` flag and its meaning.
12. As a shell user, I want `gx <command> --help` for every command, so that I can discover options
    without reading source.
13. As a shell user, I want `gx completion fish|bash|zsh`, so that I can enable shell completion.
14. As a shell user, I want `gx log -f <TAB>` to complete filesystem paths, so that I can pick a
    file quickly and correctly.
15. As a `gx stashify <cmd>` user, I want the wrapped command's exit code to propagate silently,
    so that scripts depending on it keep working after the migration.
16. As a shell user, I want error messages to print once (not duplicated, not followed by a usage
    dump on runtime failures), so that output stays clean.
17. As a maintainer, I want every existing command (`status`, `log`, `show`, `worktrees`/`wt` and
    its subcommands, `push`, `init`, `edit-config`, `bump`, `stashify`, `doctor`, `version`) to
    behave exactly as before after the migration, so that the change is safe.
18. As a maintainer, I want the `deps` injection seam preserved, so that CLI tests stay fast and
    don't shell out.
19. As a maintainer, I want the path-resolution logic extracted as a pure function, so that its
    edge cases are unit-testable in isolation.

## Implementation Decisions

**CLI framework — cobra, full migration.** Replace the `switch` in the `cmd` package with a
`newRootCmd(deps) *cobra.Command` factory that builds the entire command tree, each `RunE` closing
over the injected `deps`. `Execute()` constructs the root with `defaultDeps()` and runs it. Cobra
chosen over urfave/cli for ecosystem and completion parity. Migration is full, not hybrid — no two
parsing styles coexisting. (ADR 0003.)

**Testability seam preserved.** The `deps` struct of injected function pointers stays. Tests build
the root via the factory with fake deps and drive it with cobra's `SetArgs` + `SetOut`/`SetErr`.

**Error / output contract preserved.** Root command sets `SilenceErrors = true` and
`SilenceUsage = true`; errors are returned unchanged to `main`. `main` remains the single printer
and continues to unwrap `*ExitError` for child-process exit-code pass-through (used by `stashify`).
Cobra knows nothing about `ExitError`, so `main` must inspect the error regardless; letting cobra
print would re-emit `exit status N` for a flow meant to be silent. `SilenceUsage` also prevents a
full usage dump after runtime (non-parse) failures.

**Help text.** Switch from the hand-written usage printer to cobra-generated help: populate
`Use`/`Short` per command and a root `Long`, carrying over the existing descriptions. Delete the
hand-written usage function. Nothing tests the exact text.

**`log` command flag.** Register `-f`/`--file` (string). Within `RunE`: resolve the raw path
against the current directory and the repository root, then pass a `LogOptions{Ref, File}` value to
the log launcher.

**`runLog` signature.** Change the `deps.runLog` field from `func(string) error` to
`func(LogOptions) error`, where `LogOptions{Ref, File string}`. The launcher sets `FilterPath` on
the `InitialRoute` ViewState (`Tab: TabLog`). The options struct leaves room for future
`StartLine`/`EndLine` without churning the signature again.

**Path resolver — extracted deep module.** Add a pure function in the `git` package, alongside
`WorktreeRoot`, that takes the current directory, the raw user-supplied path, and the repository
root, and returns a repo-relative path or an error if it escapes the root. No I/O beyond `filepath`
operations. This is the one piece with real edge cases (subdir resolution, `..` traversal,
absolute input, repo-root escape), so it is isolated for direct testing.

**Follow-renames — shared default.** In `LogEntriesFiltered`, append `--follow` whenever a
file-only filter is active (path set, line-range mode off). This is intentionally shared: both
`gx log -f` and the status `gh` mapping go through this path, so both gain rename-following. This
slightly changes existing `gh` behavior (renamed files now show pre-rename history) — a deliberate,
strict improvement. `--follow` is incompatible with `-L`, so the line-range branch is untouched.

**Completions.** Keep cobra's auto-added `completion` command and register file-path completion on
the `-f` flag (cwd-relative, matching the resolver's input contract).

## Testing Decisions

Good tests here assert **external behavior** — the resolved path, the parsed flag values reaching
the launcher, the git output for a renamed file, the ViewState the launcher produces — not internal
wiring. Cobra's own parsing is not re-tested; only that our commands route arguments correctly.

- **Path resolver** (unit, pure): resolution from a subdirectory; an already-repo-relative path;
  absolute input under the root; `..` traversal that stays inside; and rejection of a path that
  escapes the repo root. Fast, no git. This is the primary new test target.
- **Cobra dispatch/parsing**: port `cmd/cmd_test.go` to the factory. Keep the existing dispatch
  pattern (inject a fake handler, assert it received the parsed values) and integration pattern
  (real repo, assert on captured stdout). Add cases for `-f`, `--file`, `--file=`, and ref+flag in
  both orders. Prior art: the existing `Test Execute_*` tests in `cmd/cmd_test.go`.
- **Follow-renames** (git integration): in a temp repo, rename a file with history and assert the
  filtered log includes commits from before the rename. Prior art: `git/log_test.go`,
  `git/log_entries_test.go`.
- **`runLog` plumbing**: assert the resolved file lands as `FilterPath` on the `InitialRoute`
  ViewState. Prior art: `ui/status/actions_test.go` (`filterLogViewState` assertions).

## Out of Scope

- **Line-range filtering from the CLI** (e.g. `gx log -f path:10,20`). The `gh` mapping supports it
  via diff focus, but it is incompatible with the `--follow` default and not part of the stated
  need. `LogOptions` leaves room to add it later.
- **No validation beyond the repo-root escape check.** Non-existent or history-less paths yield an
  empty log by design (matches `git log`).
- **No behavioral changes to any command other than `log`** — the migration is behavior-preserving
  for the rest.

## Further Notes

The original request was "add a `-f` flag." Grilling reframed it: the flag is small, but doing it
through a real CLI library means a full cobra migration, which is the bulk of the work and is
captured in ADR 0003. The single most important correctness finding is that `gx log -f` and the
`gh` mapping would otherwise **diverge** on rename-following; the shared `--follow` change keeps
them consistent at the cost of a small (positive) change to existing `gh` behavior.
