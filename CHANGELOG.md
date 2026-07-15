# Changelog

## v0.19.12 - 2026-07-15

- Align log and commit views commit rendering

## v0.19.11 - 2026-07-15

- Log: commit rows now render across two lines — subject (with graph and push/pull state) on top, hash/date/author/decoration badges indented below. Decoration badges are back as condensed boxed pills. Selection and flash highlighting span both lines.

## v0.19.10 - 2026-07-11

- Commit view and log: decoration badges (branch/tag) now render as plain colored text instead of a dark-background pill, joined to the subject by a delicate " · " separator. This avoids the subject shifting column depending on whether a row has decorations.
- Log: the working-tree pseudo-row now always reads "working tree: <detail>" instead of relying on fixed-width padding that only lined up in wide panels.

## v0.19.9 - 2026-07-11

- Commit view: the header now shows branch/tag decorations next to the subject/author line, styled like the log view's badges (dark background, colored text). Decorations wrap onto extra header lines when they don't fit.
- Log: rows in narrow panels now render in a condensed style — relative dates drop the " ago" suffix (e.g. "2h" instead of "2h ago"), the gap before decoration badges narrows to one space, and multiple decorations merge into a single badge group with one shared background and per-decoration text colors.

## v0.19.8 - 2026-07-11

- Status: directories in the file tree can now be discarded directly — discarding a directory undoes tracked changes and deletes untracked files for everything inside it.

## v0.19.7 - 2026-06-29

- Bump: fixed version bump failing when a non-semver tag exists between HEAD and the last version tag.
- AI keys: renamed yank-for-AI binding `ya` → `ay`, and ask-AI binding `cm` → `ai`.
- Takeover apps (editor, comment editor, lazygit log): success no longer shows a stale "opening…/closed" toast — the screen refresh on return is feedback enough. Git commit still reports via a "committed" success notification.

## v0.19.6 - 2026-06-17

- Log and status tabs now show the current worktree name in the top-right of the panel frame (next to the ref), prefixed with a worktree icon when nerd fonts are enabled. The log tab's left title is now simply "Log".

## v0.19.5 - 2027-06-16

- Running `gx` with no args from a bare repo root (the `.bare`-trick layout, before `cd`-ing into a worktree) now opens the worktree UI instead of erroring with "must be run from a regular repo or linked worktree".

## v0.19.4 - 2026-06-10

- Pull/push: don't show notification if aborted
- Push: allow aborting

## v0.19.3 - 2026-06-09

- Help: the help page (`?`) got a substantial overhaul — bindings now render across multiple columns to fit more on screen, with a scrollbar when the content overflows. Added filtering: start typing to filter the visible bindings by key or description. Chord bindings are displayed more cleanly, and duplicate/merged key bindings are consolidated.

## v0.19.2 - 2026-06-08

- Log: fixed amend (`A`) stalling when triggered with the commit detail panel focused — the modal got stuck after creating the fixup commit and never ran the autosquash rebase. The log now keeps forwarding the detail panel's async modal messages, so the amend completes.
- Splits: split terminology now matches vim. "Horizontal split" / `es` opens a **stacked** (top/bottom) pane like vim `:split`; "vertical split" / `ev` opens a **side-by-side** pane like vim `:vsplit`. This swaps the behavior of the `es`/`ev` edit-in-split chords and the worktree terminal menu's `h`/`v` actions; `gx term --right`/`--below` are unchanged (they were already named by visual outcome).

## v0.19.1 - 2026-06-08

- Commit/Log/Stash: inline image diffs now render in the commit detail view too, so image changes are shown directly (via the kitty graphics protocol) when viewing a commit in the log and stash tabs, not just in the status diff panel.

## v0.19.0 - 2026-06-08

- Status: added inline image diffs — image files in the diff panel are now rendered directly via the kitty graphics protocol on supported terminals, instead of a binary-file summary. Toggle with the new `image-diffs` config option (enabled by default).

## v0.18.2 - 2026-06-06

- Prevent tab switching when a modal is open

## v0.18.1 - 2026-06-06

- Internal: normalized log and stash tabs onto a shared internal shape — each tab now has an unexported `listPanel` sub-model (row rendering, list navigation, `splitview.ListPanel`) separate from the page orchestrator. Both tabs expose `NewModel` as the sole public constructor.

## v0.18.0 - 2026-06-04

- Stash: added a **Stash tab** (the fourth tab, reachable with `4`, `g S`, or `,` / `.`) that lists the repo's stashes in a split view — the stash list on one side and the selected stash's diff on the other. Apply (`a`), pop (`p`), drop (`d`), or create a new stash (`s`) directly from the list; `enter` / `l` focuses the diff panel, and `t o` toggles the split orientation (which also auto-stacks on narrow terminals).
- CLI: added `gx stash` to open the stash UI directly.
- Log: the log tab now renders in the same shared split view (commit list + commit detail), with a pseudo worktree-status row pinned at the top so uncommitted changes are visible alongside history.
- UI: switching tabs no longer clears the screen or flickers. Cached tabs reload only when the repo actually changed since they were last loaded (epoch-based invalidation), instead of re-shelling git on every activation; mutating ops (commit, stash, push, …) signal the change so other tabs refresh on their next visit. Manual refresh (`R`) is unchanged.
- Log: fixed commit-detail focus routing for `h` / `l`, split-panel focus handling, and collapsed-commit frame dimming; realigned the pseudo status line and log columns.

## v0.17.7 - 2026-06-03

- CLI: `gx init` and `gx edit-config` are replaced by a `gx config` subcommand group — `gx config edit` opens the config in `$EDITOR` (creating it if missing), `gx config show` prints the effective merged config as JSON, and `gx config defaults` prints the built-in defaults as JSON.
- Config: fixed log config merging — `log.important-refs` and `log.hide-refs` from the user config now merge correctly with defaults instead of replacing the entire log section.
- Fix: relative timestamps now include the year for dates older than one year.

## v0.17.6 - 2026-06-02

- Status: added `Sa` / `Ss` stash shortcuts. `Sa` stashes all tracked changes (staged + unstaged); `Ss` stashes only staged changes, leaving unstaged modifications in place. Both prompt for an optional stash name before running.

## v0.17.5 - 2026-06-02

- CLI: added `gx term` to launch a command (or `$SHELL`) into a tmux/kitty split, tab, or in place. Directions are named by visual outcome (`--right`/`--below` default/`--tab`/`--here`) so the same flag lays out identically on tmux and kitty; `--cwd` sets the working directory. On a plain terminal (or kitty without remote control) it runs in place, so the same invocation works everywhere — handy for opening things from neovim into a split.
- Splits/tabs: fixed the kitty split direction — side-by-side and stacked splits were swapped relative to tmux, so they now produce matching layouts on both terminals (affects the worktree terminal menu and `gx term`).

## v0.17.4 - 2026-06-02

- Log: added `gx log -f/--file <path>` to open the log pre-filtered to a file (path is taken relative to your current directory). Follows renames, so pre-rename history is included.
- Log: file-filtered logs now follow renames everywhere — both `gx log -f` and the status `gh` mapping show a file's history from before it was renamed.
- CLI: shell completion is now available via `gx completion <bash|zsh|fish|powershell>`.
- Splits/tabs: commands run in a split or tab (commit, rebase, editing a file) now keep their pane open when they fail, showing the exit code, the command that ran, and a `press Enter to close…` prompt — so you can read the error instead of the pane vanishing. Successful commands close immediately as before.

## v0.17.3 - 2026-05-31

- Log: improve columns

## v0.17.2 - 2026-05-31

- Worktrees: kitty session names now use full repo and worktree names, with optional exact aliases for long names.

## v0.17.1 - 2026-05-31

- Status: fix indentation issue

## v0.17.0 - 2026-05-30

- Search box now renders inline inside the diff view and filetree (no longer a floating overlay)
- Search: pressing `/` in results mode reopens the search box with the current query pre-filled
- Status: confirming a search syncs the query and highlights to the inactive pane, so both staged and unstaged sections show counters after confirm
- Filetree: fix search jump bug

## v0.16.2 - 2026-05-27

- Commit: fix diff scrolling — `j`/`k` now scroll through long hunks before jumping to the next/previous one
- Commit + Status: scroll one extra line past the hunk boundary before jumping, so the end of a hunk is visible before moving on
- Commit: show scroll percentage in the diff panel title
- Commit + Status: search counter (`⌕ N/M`) now appears in the commit diff title when searching

## v0.16.1 - 2026-05-26

- All views: add `g p` binding to open the GitHub PR for the current context in the browser
- Commit + Status: add `e`-prefix chords for editor split variants (`ee`, `es`, `ev`, `et`)
- Commit: add `[`/`]` bindings to decrease/increase diff context lines
- Commit: show diff context count on the frame instead of as a notification
- Worktrees: fix delete progress mode — wait for exit before quitting
- Tabs: remove extra padding

## v0.16.0 - 2026-05-25

- Log + Commit: add `yh`, `ys`, `ym` to yank commit hash, subject, and message
- Log: show worktree root in panel title
- Log: `q`/`esc` from a worktree's log view returns to the worktrees list
- Log: update ref badge colors
- Massive navigation refactor

## v0.15.7 - 2026-05-19

- Worktrees: add bulk-delete with multi-select and progress modal
- Push: fix PR URL detection
- Push: clean state on startup

## v0.15.6 - 2026-05-19

- Log: add `important-refs` config to highlight specific refs with custom colors and control their sort order
- Log: add `hide-refs` config to hide specific refs by regex pattern

## v0.15.5 - 2026-05-19

- Log: move the status icon next to the commit subject

## v0.15.4 - 2026-05-19

- Log: color commits based on their status
- Log: add filtering by file path and line range
- Log: interactive rebase window no longer closes automatically after rebase completes
- Pull: confirm before stashing when there are uncommitted changes
- Worktrees: use the shared pull UI for the pull command
- Status: change fullscreen key mapping back to `f`
- Change reword key mapping from `cr` to `rw`
- Consolidate keybinding and help management across all UI modules
- Fix chord hints display issue

## v0.15.3 - 2026-05-17

- Log: add `ri` keybinding to launch `git rebase -i` from the selected commit; if there are unstaged changes, a modal confirms stashing first; after the rebase completes, a second modal confirms popping the stash
- Fix log view not reloading on focus (missing `ReportFocus = true`); as a side-effect, `FocusMsg` now works in log, enabling the stash-pop prompt after an interactive rebase in a terminal split
- Extract `ui.NewMainView` shared helper that sets `AltScreen`, `MouseMode`, and `ReportFocus` — used by all top-level page views (log, status, commit, worktrees) to prevent flags from being missed
- Consolidate all view-specific Settings structs into a single `ui.Settings` struct
- Fix double force-push confirmation in `gx push`
- `make install` now embeds the version string in the binary via `-ldflags`
- Parallelize git and UI tests with `t.Parallel()`

- Fix commit view header scrolling broken when expanded body fills the viewport

## v0.15.2 - 2026-05-16

- Commit view: top panel now sizes to fit its content, capped at 50% of screen height; `(b to expand)` hint only appears when a commit body actually exists
- Bump flow: remove the redundant "Push to origin?" confirmation inside the bump modal — the push modal's own confirmation is used instead; push confirm prompt now highlights the branch name in orange and remote in teal, and shows a different message when a tag will also be pushed
- Status view: remove stage/unstage notifications

## v0.15.1 - 2026-05-16

- Fix bump push flow not pushing the tag — the push modal now runs `git push <remote> <tag>` as an additional step after a successful branch push

## v0.15.0 - 2026-05-16

- Add `B` keybinding in `gx status` and `gx log` to bump the version — shows a picker (patch/minor/major), creates an annotated tag, then optionally triggers the push flow
- Add `R` keybinding in `gx log` for refresh (previously only available via `m r` chord); remove the `m r` alias
- Add refresh notifications: synchronous views (status, commit) show "refreshed"; background views (worktrees, log) show a spinner while loading then "refreshed" on completion
- Change `ya` keybinding title and output format to "yank for AI agent" — wraps the diff in a `\`\`\`diff`code block matching the`cm` comment format
- Notification system: migrate all views to a shared `ui/notify` overlay model with Info/Success/Warning/Error/Progress kinds

## v0.14.6

- Add `p`/`P` keybindings in `gx log` to pull and push the current branch (same flow as `gx status`, including credential prompting, stash/pop for dirty worktrees, and divergence handling)
- Add `ctrl+d`/`ctrl+u` vim-style co-scroll in `gx commit`, `gx status`, and `gx log`
- Wire mouse scroll in `gx commit`, `gx status`, and `gx log`
- Add `cr` chord in reword view to open `$EDITOR` for editing the commit message
- Flash and re-focus the log entry after returning from amend or reword
- Fix wide-character rendering causing a CPU spike or skipped characters (bubbletea v2.0.6 upstream fix)

## v0.14.5

- Add `cm` keybinding in `gx status` and `gx commit` to open `$EDITOR` and write a comment on the currently selected diff hunk
- Fix help modal scroll — scrolling past the last visible line no longer gets stuck
- Add `A` keybinding in log and commit views to amend a specific commit with currently staged changes

## v0.14.4

- Make the panel frame color darker

## v0.14.3

- Introduce a `keybindings.Manager` to centralize key binding definitions, chord dispatch, and help-text generation across `gx status`, filetree, and diff area — replacing scattered per-component key structs and manual chord tracking
- Extract keybinding definitions for filetree and diffarea into dedicated `model_keys.go` files; generate help content from binding metadata instead of hand-maintained lists
- Fix `q` in `gx status` not quitting — `bindingQuit` was registered in the keybindings manager but had no dispatch case

## v0.14.2

- Align filetree left/right behavior across `gx status` and `gx commit`: `h/left` now collapses expanded folders first (then moves to parent), `h/left` on files moves to parent folder, and `l/right` on expanded folders moves to first child
- Remove status-specific filetree key interception so status delegates folder/file navigation to `ui/filetree` consistently
- Refresh `gx log` when the log page is activated via app navigation and when terminal focus returns, so new on-disk commits appear without manual reload

## v0.14.1

- Refactor diff interaction ownership so `gx status` and `gx commit` route navigation/search/yank/viewport behavior through `ui/diffview.Model` methods instead of package-level helper wiring
- Extract a dedicated `ui/status/diffarea` model to clarify staged/unstaged orchestration boundaries and reduce status-page coupling
- Rename diff state internals from buffer-oriented naming to `DiffData` and prune legacy status alias/host glue files
- Reduce `ui/diffview` exported helper surface by making internal navigation/search/runtime helpers private once call sites were migrated

## v0.14.0

- Redesign `gx status` with a focused-section layout: one expanded diff section plus always-visible collapsed strips for `Unstaged` and `Staged`
- Simplify status navigation and focus behavior: `h/l` now moves between filetree and diff, `Tab` switches diff sections, and section choice stays stable across reloads and file changes
- Improve status visual identity and focus clarity: filetree uses blue focus styling, diff sections keep stable unstaged/staged colors, active pane titles are bold, and filetree row focus is more explicit
- Keep stage/unstage context in place by removing auto-jumps when a section becomes empty, while keeping destination flash feedback
- Show selected file paths directly in the expanded diff section title (`Unstaged: …` / `Staged: …`), including renamed-file format (`old -> new`)
- Fix footer composition with app tabs so right-side hints remain readable: preserve the hint tail (context/mode/help), handle padded footer lines correctly, and use Unicode ellipsis (`…`) for truncation
- Update `gx log` commit highlighting so diverged local-only commits are styled distinctly instead of sharing normal local-only green

## v0.13.6

- Complete the status nested-model cleanup: focused-child key routing, per-pane search ownership, and removal of legacy status search-scope adapters
- Replace filetree sync bridge code with explicit status/filetree reconciliation helpers and cleaner state ownership
- Simplify status/filetree integration by caching filetree rows alongside status entries and removing the status-to-filetree conversion helper
- Prune dead status/diffview/filetree API surface used only during migration

## v0.13.5

- Fix status diff `Tab` cycle order to `sidebar -> unstaged -> staged -> sidebar`
- Preserve status unified diff colorization state while navigating hunks, preventing temporary raw gutter padding from reappearing

## v0.13.4

- Log view now reloads commits on open and supports `R` to reload manually
- Fix commit sidebar min width (25 instead of 45) so short file lists don't waste space

## v0.13.3

- Add left padding to unified diff while async delta colorization is pending, reducing flicker when switching files
- Fix syntax highlighting disappearing after staging a hunk
- Fix unified diff not reflowing to the new width when toggling fullscreen in the status view

## v0.13.2

- Normalize keymappings (partially, there's still work to do)
- Show chort keys overlay (like neovim's whichkey)
- Code cleanup

## v0.13.1

- Normalize commit message newlines

## v0.13.0

- Changed `gx` startup to open the status view by default (`gx worktrees` / `gx wt` still opens the worktree UI)
- Added a focused commit-view header with scrollable metadata, commit-body yanking, and cleaner subject/author rendering
- Added log/commit navigation refinements, including tag-jump chords in `gx log`, commit selection restoration when backing out of commit view, and tab-aware log history
- Kept the shared diff-explorer extraction moving forward so commit and status views continue to reuse the same rendering/navigation core

## v0.12.11

- Added a real `gx show [hash-or-ref]` entrypoint that opens a new commit view with commit metadata, ref badges, and body collapse/expand
- Added commit-ref plumbing in the app shell so commit routes resolve directly instead of using a placeholder page
- Continued extracting the shared diff-explorer core out of `gx status`, with host-style helpers and selection adapters to prepare for the future commit diff explorer

## v0.12.10

- Added app-level tab navigation across `gx wt`, `gx log`, and `gx status`, including persistent tab state, `gw` / `gl` / `gs` routing, and proper push/replace back-stack behavior for drill-down screens
- Added a new `gx log` view with commit history, ref badges, pseudo-row navigation into working-tree status, inline search with persistent highlights plus `n` / `N` result jumping, and support for `gx log <ref>` rooted at an explicit commitish
- Unified selected-row highlighting across log and worktrees with a neutral surface background that preserves existing foreground colors and search/status styling

## v0.12.9

- Refined `gx status` unified diff rendering so changed rows no longer show literal `+` / `-` markers, making the interactive unified view read closer to the side-by-side mode while keeping hunk/line staging behavior intact
- Extended added and removed row backgrounds to the full diff pane width for a cleaner, easier-to-scan unified diff

## v0.12.8

- `gx wt` text-input overlays now accept pasted text again for flows like new worktree, rename, clone, search, and credential prompts
- Added `name-aliases` config for kitty session naming so exact repo and worktree names can be replaced before the usual dash-segment compression runs

## v0.12.7

- Fixed release and local linker version stamping after the Go module path change so `gx version` now reports the injected tag/build version instead of falling back to Go's VCS build info and showing unexpected `+dirty` suffixes

## v0.12.6

- `gx status` now sizes the `Commits` pane from its rendered content instead of using a fixed split, while keeping both the status list and branch-commit history visible within min/max height bounds
- Fixed kitty session naming for `.bare`-style repos so new sessions use the outer repo directory name instead of generating `bre-...` prefixes from `.bare`

## v0.12.5

- `gx wt` now has a unified "open in terminal" menu: `o` opens the selected worktree and `N` creates a new worktree and opens it immediately; the menu offers session, hsplit, vsplit, and tab actions for tmux and kitty remote control, and shows a clear message when kitty remote control is unavailable
- `gx wt` and `gx status` output/lazygit chords now use a `g` prefix: `gg` jumps to top, `go` shows command output, and `gl` opens the lazygit log; this frees `o` for the new worktree open flow
- Kitty session names now use a vowel-stripped `repo-worktree` form for brevity, and session files are auto-created as `~/.local/share/kitty/sessions/<name>.kitty-session`
- `gx status` now shows a read-only `Commits` frame under the status tree with the branch history since the remote mainline (`origin/<default>`, `origin/main`, or `origin/master`); commits show wrapped subjects plus relative time and short hash, and are color-coded for shared, local-only, and remote-only/diverged commits

## v0.12.4

- `gx wt` and `gx status` search now appears as a framed bottom-center overlay instead of replacing the footer line; match count (`2/5`) and `no matches` appear on the right side of the modal border
- `gx wt` text-input overlays (rename, clone, new, search) are now wider (50 columns, capped at 80% of window width)
- Added `input-modal-bottom` config option to control the vertical position of text-input overlays: accepts an integer (fixed lines from bottom), a percentage string like `"20%"`, or `"center"`; `gx init` now writes `$schema` pointing to the published JSON schema
- Added `docs/config-schema.json` — reference it with `"$schema": "https://raw.githubusercontent.com/elentok/gx/main/docs/config-schema.json"` for editor autocompletion

## v0.12.3

- `gx wt` now opens keyboard help in a centered overlay like `gx status`, and both `gx wt` and `gx status` footers now show a compact `? help` prompt instead of inline keymaps
- Restyled `gx wt` help to match the brighter `gx status` help colors

## v0.12.2

- `gx status` now compresses single-child directory chains in the sidebar, so paths like `keyboards/iris/keymaps/` render as a single directory row instead of three nested rows

## v0.12.1

- Added an optional path argument to `gx status` / `gx s`; relative and absolute file paths now preselect the matching file in the status sidebar without jumping into diff focus
- Changed the Go module path to `github.com/elentok/gx` so `go install github.com/elentok/gx@latest` works correctly

## v0.12.0

- Added a shared UI design-system foundation across `gx status`, `gx wt`, and CLI flows: common theme colors, semantic icons, shared frame/overlay primitives, and reusable feedback/key-hint helpers
- Standardized menus, confirms, keybinding hints, and status messaging so CLI and TUI interactions now feel much more consistent across screens
- Migrated `gx status` onto structured `bubbles/help` and `key.Binding` driven help rendering
- Tightened final polish across remaining surfaces, including shared modal hint language, cleaner search/footer hints, and a more consistent interactive `gx bump` picker
- Added design-system research/spec docs under `docs/design-system/` and an AI-facing usage guide in `.ai/design-system.md` to keep future UI work aligned with the shared system

## v0.11.7

- `gx status` diff view now identifies symlinks: shows the target in a summary line (`symlink -> target`, `symlink: old -> new`, `symlink (target) -> regular file`, etc.) and labels the section header with `[symlink]`, `[regular -> symlink]`, or `[symlink -> regular]`; sidebar uses a dedicated symlink icon
- `gx stashify` now prints styled blue badge labels with nerd font icons before each step; icons are gated on the `use-nerdfont-icons` config option

Debugging related:

- `gx status` now shows the detected terminal (tmux/kitty) in the status bar
  (for debugging).
- `gx doctor` now shows the values of terminal-detection related env variables
  (`TMUX`, `KITTY_WINDOW_ID`, and `KITTY_LISTEN_ON`).
- `gx doctor` supports the `--pause` flag to wait for Enter before exiting

## v0.11.6

- `gx status` now shows branch sync relative to the current branch's upstream ref instead of always comparing against the repo's default main remote branch
- Restored `g` as jump-to-top in `gx status` and `gx wt`, and moved output/log actions to chords: `oo` for command output and `ol` for lazygit log
- Added `ot` in `gx wt` to open a tmux session in the selected worktree directory

## v0.11.5

- Fixed Ubuntu CI failures in the PTY-backed Git runner by treating Linux `/dev/ptmx` `EIO` shutdown reads as benign EOF-style conditions

## v0.11.4

- `gx status` and `gx wt` now keep SSH/HTTPS credential prompts inside the TUI by detecting Git/SSH input requests and opening an in-app input modal instead of suspending to the terminal
- User-initiated TUI network actions now run through a PTY-backed Git runner so passphrases, usernames, and passwords can be submitted directly from the app
- Fixed the PTY credential flow so handled passphrase prompts are not rediscovered and resubmitted incorrectly, which could cause repeated SSH key prompts and failed authentication
- Fixed GitHub PR URL detection after PTY-backed pushes by stripping terminal escape sequences before scanning push output

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
