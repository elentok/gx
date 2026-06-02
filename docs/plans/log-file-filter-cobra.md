# Plan — `gx log -f/--file` via cobra CLI migration

Add a `-f`/`--file` argument to `gx log` that opens the log pre-filtered to a file (equivalent to
the status view `gh` mapping). The work is split: first migrate the CLI to cobra (behavior-
preserving), then add the flag. See [ADR 0003](../adr/0003-cobra-cli.md).

## Decisions (from grilling)

- **Path semantics**: `-f` takes a **cwd-relative** path, resolved to **repo-relative** before it
  populates `FilterPath` (keeps the existing repo-relative contract; tab-completion works). Reject
  paths that resolve outside the repo root.
- **Validation**: none beyond the repo-root escape check. An empty filtered log is fine (matches
  `git log -- nonexistent`).
- **Library**: cobra, full migration of all commands; `-f` added last.
- **Testability seam**: `newRootCmd(d deps) *cobra.Command` factory closing over `deps`; tests use
  `SetArgs`/`SetOut`/`SetErr`. `deps` struct kept, except `runLog`.
- **`runLog` signature**: `func(LogOptions) error`, `LogOptions{Ref, File string}`. Path resolved
  in the cobra `RunE` handler; `runLog` stays a thin launcher.
- **Follow-renames**: shared default — append `--follow` in `LogEntriesFiltered` whenever a
  file-only filter is active (`Path != "" && !useLineRange`). The `gh` mapping inherits this too.
- **Errors**: `SilenceErrors = true` + `SilenceUsage = true`; `main.go` unchanged.
- **Help**: cobra-generated; delete `printUsage`.
- **Completions**: include the `completion` command + file-path completion on `-f`.
- **Line range**: out of scope (can't combine with `--follow`; `LogOptions` leaves room for later).

## Phase 1 — cobra migration (behavior-preserving)

- [x] Add `github.com/spf13/cobra` to `go.mod`.
- [x] Add `newRootCmd(d deps) *cobra.Command` building the full command tree: `status`/`s`,
      `log`, `show`, `worktrees`/`wt` (+ `list`, `abs-path`, `clone`), `push`/`ps`, `init`,
      `edit-config`, `bump`, `stashify`, `doctor`, `version`.
- [x] Each `RunE` closes over `d` and calls the existing handler; preserve arg/positional
      validation (e.g. `status [path]`, `show [ref]`).
- [x] Set `SilenceErrors`/`SilenceUsage = true`; route cobra out/err through `deps` writers.
- [x] Populate `Use`/`Short` (+ root `Long`) from the old `printUsage` text; delete `printUsage`.
- [x] `Execute()` → reuses `execute()` which builds `newRootCmd(d)` and runs it.
- [x] Port `cmd/cmd_test.go`: kept the `execute(args, d)` seam (now builds the root via the factory);
      `TestExecute_UnknownCommand` now asserts the returned error (usage is silenced, `main` prints once).
- [x] `go build` + `go test ./cmd/...` green; manual smoke of each command.

> Note: `stashify` and `doctor` use `DisableFlagParsing: true` so their own arg/flag handling is
> preserved untouched (flags pass through to the wrapped command / `runDoctor`'s manual parser).
> `--version`/`-v` come from cobra's `root.Version`; a `version` subcommand is also registered.

## Phase 2 — `-f`/`--file` flag

- [x] Add `git.RepoRelativePath(cwd, rawPath, repoRoot)` pure resolver in `git/stage.go`
      (cwd-relative/absolute → clean repo-relative; rejects paths escaping the root) + unit
      tests in `git/repo_relative_path_test.go`. (task main-n7v.2)
- [x] Change `deps.runLog` to `func(LogOptions) error`; add `LogOptions{Ref, File string}`. (main-n7v.4)
- [x] On the `log` command: `flags.StringVarP(&file, "file", "f", "", "...")`.
- [x] In `RunE`: resolve `file` cwd-relative → repo-relative against the repo root (via
      `resolveLogFile` → `git.RepoRelativePath`, canonicalizing cwd to match git's root); error if it
      escapes the root; pass through `LogOptions{Ref, File: resolved}`.
- [x] `runLog` sets `FilterPath` on the `InitialRoute` ViewState (`Tab: TabLog, …`).
- [x] Register file-path completion for `-f` (`MarkFlagFilename`).
- [x] Tests: `-f`/`--file`/`--file=`, with and without a ref (both orders); path resolution from a
      subdir; repo-root escape rejected.

## Phase 3 — follow-renames

- [x] In `git/log.go` `LogEntriesFiltered`, append `--follow` when `Path != "" && !useLineRange`.
- [x] Test that a renamed file's log shows pre-rename history (covers both `-f` and `gh`).
      (`TestLogEntriesFiltered_FollowsRenames` in `git/log_entries_test.go`; task main-n7v.3)

## Phase 4 — docs

- [x] Update README CLI/usage section for `gx log -f` and `gx completion`.
- [x] Update CHANGELOG via the changelog skill (Unreleased section).
- [x] Flip ADR 0003 status to Completed.
