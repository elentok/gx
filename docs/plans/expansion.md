# Expansion Implementation Plan

## Goals

1. Introduce a real command tree for `gx` (subcommands + aliases).
2. Move the existing TUI entrypoint to `gx worktrees` with alias `gx wt`.
3. Add `gx clone-wt`:
   - clone as bare repo
   - create an initial linked worktree on the primary branch (`main`/`master`)
4. Add `gx push`:
   - push current branch
   - on non-fast-forward/rejected push, prompt for confirmation and retry with force push

## Research Decision: CLI Command Framework

For a multi-command Go CLI with aliases, help, and shell completion, the best fit is `cobra`:

- `cobra` is purpose-built for nested subcommands, aliases, command help, and completion.
- It is the de-facto standard in major Go CLIs.
- It keeps command parsing/dispatch separated from business logic.

Primary references:

- https://pkg.go.dev/github.com/spf13/cobra
- https://github.com/spf13/cobra
- https://cobra.dev/docs/how-to-guides/shell-completion/

## Proposed CLI UX

- `gx worktrees` (alias: `gx wt`) launches the current Bubble Tea worktree manager.
- `gx clone-wt <repo-url> [directory]` clones a bare repo, then creates an initial worktree for the default branch.
- `gx push` pushes from the current worktree directory.
- `gx` with no args should default to `gx worktrees` for backward compatibility.

## Implementation Plan

## 1) Restructure Entrypoint Around Commands

### Changes

- Add a new command package (for example `cmd/`) with:
  - root command (`gx`)
  - `worktrees` command (`wt` alias)
  - `clone-wt` command
  - `push` command
- Refactor `main.go` to call `cmd.Execute()`.
- Move current TUI launch flow into a reusable function used by `worktrees`.

### Notes

- Keep all git logic in `git/` package.
- Keep TUI logic in `ui/worktrees`.
- Command layer should only parse args/flags and orchestrate calls.

## 2) Move Existing TUI to `gx worktrees` / `gx wt`

### Changes

- Extract current `main.go` behavior (repo detection + active worktree detection + Bubble Tea program run) into command action.
- Keep exactly the same runtime behavior and errors.
- Ensure `gx` (no args) still opens TUI by setting root `RunE` to same action as `worktrees`.

### Tests

- Add command tests to verify:
  - `gx` invokes worktrees flow
  - `gx worktrees` works
  - `gx wt` works

## 3) Add `gx clone-wt`

### Behavior

1. Run bare clone (`git clone --bare <url> <dir>`).
2. Detect primary branch from remote HEAD.
3. Add initial linked worktree under `<dir>/<branch>` and check out that branch.

### Proposed git-layer additions

- `git.CloneBare(url, dir string) error`
- `git.RemoteDefaultBranch(repoRoot string) (string, error)` (resolve from `origin/HEAD`, fallback `main`, then `master`)
- `git.AddWorktreeFromRemote(repo Repo, worktreePath, branch, remoteBranch string) error`

### Edge cases

- Destination exists -> clear actionable error.
- Remote has neither `main` nor `master` and `origin/HEAD` missing -> fail with explicit message requiring branch flag (optional future enhancement).
- Clone succeeds but worktree creation fails -> return error with next-step hint.

### Tests

- Integration test with temp upstream repo:
  - verifies clone is bare
  - verifies initial worktree exists
  - verifies branch is checked out and matches remote default branch

## 4) Add `gx push` With Force Confirmation

### Behavior

1. Must run inside a worktree (not bare root).
2. Resolve current branch; reject detached HEAD.
3. Resolve remote (`branch.<name>.remote`, fallback `origin`).
4. Attempt normal push.
5. If push fails due to rejected/non-fast-forward:
   - show a styled confirmation UI (Bubble Tea + lipgloss/bubbles), e.g.:
     `Push rejected for <remote>/<branch>. Force push with lease?`
   - on yes, run force push (`--force-with-lease` preferred; fallback `--force` only if explicitly requested later)
   - on no, exit cleanly with non-zero status and explanatory message
6. For other push failures, return error without force prompt.

### Proposed git-layer additions

- `git.CurrentBranch(dir string) (string, error)`
- `git.PushBranch(worktreePath, remote, branch string) error` (can wrap existing `Push`)
- `git.PushBranchForceWithLease(worktreePath, remote, branch string) error`
- `git.IsNonFastForwardPushError(err error) bool` (inspect `RunError` stderr/stdout patterns)
- `ui/confirm` (new): minimal reusable Bubble Tea confirmation model with lipgloss styling and `yes/no` result.

### Tests

- Unit tests for non-fast-forward detection with representative git stderr.
- Integration tests:
  - fast-forward push succeeds
  - rejected push prompts and declines
  - rejected push prompts and force-with-lease succeeds
  - detached HEAD returns explicit error

## 5) Keep TUI Push/Pull Behavior Consistent

### Changes

- Update TUI push path (`ui/worktrees/pullpush.go`) to use shared push helper (`PushBranch`), so CLI and TUI do not diverge.
- Decide whether TUI should also use force-confirm flow:
  - Phase 1: keep existing behavior (no prompt in TUI) to minimize risk.
  - Phase 2 (optional): add TUI modal confirmation and force-with-lease support.

## 6) Documentation and Migration

### Changes

- Update `README.md` command usage:
  - `gx`, `gx worktrees`, `gx wt`
  - `gx clone-wt`
  - `gx push`
- Add examples and expected prompts.
- Mention that `gx` defaults to worktrees for compatibility.

## Milestones

1. Command framework + `worktrees`/`wt` wired and tested.
2. `clone-wt` command implemented and tested.
3. `push` command with confirmation flow implemented and tested.
4. README and final polish.

## Risks and Mitigations

- Git stderr text can vary by version:
  - mitigate by matching multiple known non-fast-forward patterns and covering in tests.
- Clone bootstrap branch detection may be ambiguous:
  - prefer remote HEAD first, then deterministic fallback order.
- Command refactor could break existing startup behavior:
  - preserve existing TUI launch path in a single shared function and test `gx` no-arg behavior explicitly.
