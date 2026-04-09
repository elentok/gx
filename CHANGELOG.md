# Changelog

## v0.11.4

- `gx status` and `gx wt` now keep SSH/HTTPS credential prompts inside the TUI by detecting Git/SSH input requests and opening an in-app input modal instead of suspending to the terminal
- User-initiated TUI network actions now run through a PTY-backed Git runner so passphrases, usernames, and passwords can be submitted directly from the app
- Fixed the PTY credential flow so handled passphrase prompts are not rediscovered and resubmitted incorrectly, which could cause repeated SSH key prompts and failed authentication

## v0.11.3

- User-initiated Git network actions now run interactively so SSH key passphrase prompts can be answered in `gx push`, `gx status`, and `gx wt`
- Background Git commands still fail fast on credential prompts to avoid hanging the UI
- Added `o` to view the latest command output in `gx status` and `gx wt`; composite actions now include labeled output for every step, such as stash, pull/rebase/push, and stash pop
- Changed lazygit shortcuts to `g` in `gx status` and `gx wt`

## v0.11.2

- Added session-scoped diff context controls in `gx status` with `[` / `]`, clamped to a minimum of `-U1`
- Show the current diff context in the status footer and added help/docs for the new context controls

## v0.11.1

- `gx push` now asks for confirmation before checking remote divergence
- Push actions in `gx status` and `gx wt` now consistently confirm first, then run divergence checks
- Fixed side-by-side `delta` rendering so line-number colors match the configured theme instead of falling back to delta's default side-by-side colors
- Added `make test-docker-ubuntu` plus a helper script to run the test suite in a CI-like Ubuntu container with `git-delta` installed

## v0.11.0

- Added side-by-side diff render mode in `gx status` (`s`) with full interactive staging support across hunk, line, and visual selection flows
- Added side-by-side hunk gutter indicators and improved side-by-side rendering fidelity (adaptive width, fullscreen width recalculation, dimmed section separators)
- On very wide screens (`>140` cols), status pane now uses 17% width to prioritize diff space
- Hardened status E2E reliability on CI by disabling repo auto-gc in remote/clone test setups
- Side-by-side mode now explicitly requires `delta`; CI installs `delta` so side-by-side coverage runs there too
- Made `delta` rendering more consistent across environments by generating a temp config with the expected side-by-side hunk-header settings
- Reuse the generated temp `delta` config for the process lifetime instead of recreating it on every render

## v0.10.3

- Updated `gx status` docs and UX highlights, including clearer yank shortcuts (`yy` / `yl` / `ya` / `yf`)
- Added branch sync summary to the status pane header (synced/ahead/behind/diverged)
- Added mouse-wheel scrolling in status diff panes (unstaged/staged and fullscreen)
- Updated `e` in diff view to open `$EDITOR` at the selected hunk/line when supported by the editor

## v0.10.2

- Fixed intermittent CI/status lock contention by running read-only status probes with `git --no-optional-locks`
- Applied the lock-avoidance path to stage file listing and uncommitted-change collection

## v0.10.1

- Updated status yank mappings to a clearer set: `yy` (content), `yl` (location), `ya` (all context), and `yf` (filename)
- In `gx status` diff view, yank actions now respect focus granularity (hunk, line, or visual selection)

## v0.10.0

- Renamed the `gx stage` command to `gx status`
- Updated command routing, usage/help text, and tests to use `gx status`
- Updated docs and prompts to reflect the new command name

## v0.9.1

- Fixed diverged push force-push target in `gx stage`: force push now correctly uses the remote name (`origin`) instead of the upstream ref (`origin/<branch>`)

## v0.9.0

- Divergence detection: before pushing gx will detect if he branch has diverged and will offer the user to rebase, force push or abort
  (across `gx push`, `gx wt`, and `gx stage`)
- `gx stage` UX updates:
  - `.` / `,` jump to next/previous file from diff view
  - fullscreen diff now hides the status pane
  - `ol` opens `lazygit log`
  - `e` opens the currently selected file in `$EDITOR` from both status and diff views
- Improved stage patch robustness by falling back to line-range patch application when hunk apply reports a corrupt patch

## v0.8.0

- `gx stage`:
  - Added visual line-range mode (`v`) so you can select multi-line blocks and stage/unstage them with `space`
  - Added discard flows via `d` with mandatory confirmation prompts (status-file discard semantics, unstaged line/hunk/range discard, staged `d` as unstage)
  - Added stage yank mappings: `yc` for AI-friendly diff context and `yf` for filename-only yank
  - Improved test coverage with additional unit and E2E tests for visual mode, discard, and yank flows
  - Refactored internals by splitting the large monolithic model/view files into focused modules for update, key handling, navigation, runtime state, and rendering

## v0.7.2

- Expanded `gx stage` with action keys: pull (`p`), push (`P`), rebase (`b`), and amend (`A`) with confirmations
- Push in stage now matches worktrees behavior: detects GitHub PR URLs and asks whether to open them
- Added live, cancellable action output overlays in stage (`ctrl+c` cancels running git command)
- Improved stage navigation and UX: debounced status diff loading while scrolling, parent-folder focus on `h`, and additional regression/E2E coverage for push/pull/rebase flows
- Refactored shared UI/runtime pieces used by both stage and worktrees (URL opener, confirm/output modal primitives, cancellable command runner)

## v0.7.0

- Added a dedicated `gx stage` TUI for file, hunk, and line staging/unstaging with split unstaged/staged diff panes

## v0.6.1

- Fixed the yank files dialog so pressing `space` can toggle selected files off again
- Added regression coverage for the checklist space-toggle behavior

## v0.6.0

- Migrated the TUI stack to Bubble Tea v2, Bubbles v2, and Lip Gloss v2
- Replaced the old Bubble Tea v1 `teatest` dependency with a small repo-local v2 test harness

## v0.5.5

- Worktree Base column now refreshes after pulling the main branch via stash-pull

## v0.5.4

- Dirty column now uses colored styles: yellow for modified, cyan for untracked, magenta for both
- In portrait (stacked) layout, the table now sizes to fit its content rather than taking a fixed percentage of the screen height

## v0.5.3

- Confirm/error/logs/yank modals are now rendered as overlays, keeping the worktrees table and sidebar visible in the background
- Removed the Branch column from the worktrees table; when a worktree's branch name differs from its directory name, the branch is shown inline in the Worktree column as `(branch-name)`
- Fixed confirm dialog title being hardcoded as "gx push"

## v0.5.2

- Added `gx bump` command: creates an annotated version tag and optionally pushes; accepts `major`, `minor`, or `patch` as an optional argument, or shows an interactive picker with the resulting version for each option

## v0.5.1

- Main branch worktree always appears first in the list
- Main branch name and branch are rendered in orange to distinguish them at a glance
- With nerd font icons, the main worktree uses a home icon (`󰋜`) instead of the folder icon

## v0.5.0

- Added `gx stashify <cmd...>`: stashes uncommitted changes, runs the command, auto-pops on success, prompts to pop on failure
- Added `b` keybinding to rebase the selected worktree on main; confirms before rebasing; if dirty, offers to stash first
- Pull (`p`) on a dirty worktree now asks to stash first; cancelling shows "Pull aborted (dirty worktree)"
- Pulling the main branch now refreshes the Base column for all worktrees
- Sidebar now shows the latest commit (hash, subject, date, and relative date)

## v0.4.3

- Added `N` keybinding: create a new worktree and open a new tmux session (same name, cwd set to the worktree path), switching to it immediately
- Added `T` keybinding: create a new worktree and open a new tmux window
- Push (`P`) now shows a confirmation modal before executing
- Added `o` keybinding to view the output log of the last pull/push job
- Fixed `gx wt clone` to run `git fetch origin` and set up local branch upstreams after cloning

## v0.4.2

- Added `Base` column to the worktree table: `✓` if the branch is rebased on main, `✗` if it needs a rebase
- Added "Base" section to the sidebar showing the same rebase status for the selected worktree
- Fixed table scroll window rendering more rows than the table height, which could push the status bar off-screen

## v0.4.1

- Added vim-like search: press `/` to enter search mode, type to filter and highlight matching worktree names and branches, `ctrl+n` / `ctrl+p` to jump between matches, `enter` or `esc` to exit
- The Worktree column now takes the remaining space; Status column is fixed at ~20% width
- Fixed ANSI-styled cell content corrupting column alignment in the table — replaced `bubbles/table`'s internal renderer (which used `runewidth.Truncate`, not ANSI-aware) with a custom one using `charmbracelet/x/ansi.Truncate`

## v0.4.0

- Added `l` keybinding to open the selected worktree in lazygit (suspends the UI, restores it when lazygit exits)
- Consolidated worktree-related CLI commands under `gx wt`:
  - `gx wt list` — list worktree names
  - `gx wt abs-path <name>` — print absolute path of a worktree
  - `gx wt clone <url> [dir]` — clone using the `.bare` trick

## v0.3.2

- Added `gx list-worktrees` command that prints all worktree names, one per line
- Added `gx worktree-abs-path <name>` command that prints the absolute path of the named worktree
- When pushing a branch for the first time, the GitHub PR creation URL is detected and a modal asks whether to open it in the browser (defaults to Yes)
- Fixed `run` to capture stderr even on success (needed for parsing remote push output)

## v0.3.1

- Rebinded pull to `p` and push to `P`, freeing up the old `l` / `s` keys
- After yanking files (pressing `y` and confirming), the app enters a dedicated paste mode where only navigation (`j`/`k`) and `p` to paste (or `esc` to cancel) are active — this is what freed `p` for pull in normal mode
- Refreshes the worktree list after a paste completes

## v0.3.0

- `gx clone-wt` now uses the `.bare` directory trick: clones into `my-repo/.bare/` and writes a `my-repo/.git` file pointing to it, so worktrees live cleanly alongside `.bare/` rather than inside it
- Delete worktree now shows a spinner while the deletion runs and a "Worktree {name} deleted successfully" toast on completion
- Added `gx doctor` command to check a repo for common configuration issues:
  - Verifies the origin fetch refspec is set correctly
  - For `.bare`-style repos: verifies the outer `.git` file points to `.bare`
  - For `.bare`-style repos: verifies each worktree's `.git` file points to the correct location
- Added `gx doctor --fix` to interactively apply fixes with confirmation prompts

## v0.2.1

- Added `U` keybinding to run `git remote update` and refresh all worktree statuses

## v0.2.0

- Added `gx version` command (also `--version`, `-v`) to print the current binary version
- Added `scripts/bump.sh` for bumping the version, creating an annotated git tag

## v0.1.5

- `gx clone-wt` now immediately fixes the fetch refspec after cloning, so remote tracking refs populate correctly on the first fetch
- On startup, the worktrees view checks whether the fetch refspec is misconfigured or remote tracking refs are missing, and offers to fix it automatically
- Delete and track confirmations are now shown as a centred modal with Yes/No buttons instead of a status-bar prompt
- Pull and push now also refresh the sidebar after completing
- Fixed a bug where the `origin/<branch>` fallback could match a bad local branch instead of the remote tracking ref

## v0.1.4

- Added `R` keybinding to refresh the worktree list and all statuses

## v0.1.3

- Added `t` keybinding to set a remote tracking branch for the selected worktree

## v0.1.2

- The sidebar now shows a "no remote tracking branch" note with a hint to press `t` when no upstream is configured

## v0.1.1

- Status column now shows ahead/behind relative to the remote tracking branch instead of the main branch
- Sidebar ahead/behind commit lists now compare against the remote tracking branch instead of main
- Sidebar section headings updated to "Commits ahead of remote" and "Commits behind remote"
