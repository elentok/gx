# Domain Glossary

## Panels and Viewports

**Panel** — a bordered rectangular region of the screen. A view is composed of one or more panels
(e.g. commit view = header panel + filetree panel + diff panel).

**List panel** — a panel that renders a navigable list of items (filetree, file list, commit list).
Items have a fixed height of one display row each.

**Diff panel** — a panel that renders a unified or side-by-side diff. Items are hunks or changed
lines depending on nav mode.

**Viewport** — the visible window into a panel's content. Defined by a scroll offset (first visible
row index) and a height (number of visible rows).

**Scroll offset** — the index of the first visible row in a panel's viewport. Independent of
selection.

## Selection and Active Item

**Selection** (list panels) — the index of the currently highlighted item in a list panel. Used for
navigation, opening, and future multi-select operations.

**Active item** (diff panel) — the currently focused hunk (NavModeHunk) or changed line
(NavModeLine). Governs keyboard navigation and yank/comment targets.

**Snap** — when a scroll operation moves the viewport such that the selection or active item is no
longer visible, snap clamps it to the nearest visible item at the new viewport edge.

## Commit States (Log View)

**In-master** — the commit is reachable from `main`/`master` (it has been merged). Rendered in the
default color with no icon.

**Pushed** — the commit exists on both the local branch and its remote tracking branch (synced, not
yet in main). Rendered green with `✔`.

**Unpushed** — the commit exists locally only, and the branch has not diverged from its remote.
Rendered orange with `󰜷`.

**Diverged** — the commit exists locally only, and the branch has diverged from its remote tracking
branch. Rendered red with `󰃻`.

**Remote-only** — the commit exists on the remote tracking branch but not on the local branch (fetch
without pull). Rendered purple with `󰜮`.

## Navigation Modes

**NavModeHunk** — diff navigation moves between hunks. Active item is a hunk index.

**NavModeLine** — diff navigation moves between individual changed lines. Active item is a changed-
line index.

## App Navigation

**TabID** — canonical identifier of a top-level app destination (`worktrees`, `log`, `status`,
`stash`). This is the only term used for tab identity. `commit` no longer exists as a standalone
tab — commit detail is rendered as the right/bottom panel of the log split view.

**ViewState** — the full navigation payload for a screen. Composed of a `ViewContext` and
`ViewOptions`. This is the canonical navigation term.

**ViewContext** — the durable subset of `ViewState` that determines tab page identity: `Tab`,
`WorktreeRoot`, `Ref`, `InitialPath`. Tab reuse/reset decisions are keyed to `ViewContext`
equality only.

**ViewOptions** — the transient subset of `ViewState` that tunes behavior inside an active view:
`FocusSubject`, `FilterPath`, `FilterStartLine`, `FilterEndLine`. Changes to `ViewOptions` do not
trigger page reconstruction.

**Tab memory** — the app-shell record of the most recent `ViewState` seen for each `TabID`. Used
when switching tabs so users return to their last context in that tab.

**Selected worktree** — the currently highlighted worktree row in the worktrees tab. This is a
focus identity and is distinct from `worktreeRoot` (repository/worktree context used by other
tabs).

**Split view** — the layout used by the log and stash tabs. A list panel (left or top) paired with
a commit detail panel (right or bottom). Orientation is auto-detected from terminal width (same
threshold as status `useStackedLayout`) and toggled manually via the `to` chord
(`toggle-layout-orientation`).

**Panel visibility state** — the three states a split view can be in:
- *Collapsed* — only the list panel is visible. Default for the log tab.
- *Split* — both panels are visible. Default for the stash tab. Detail auto-updates as list
  selection changes (j/k navigation).
- *Fullscreen* — one panel fills the entire screen, the other is hidden. Toggled with `f` on
  the currently focused panel.

Focus and collapse rules: Enter on a list item in collapsed state → expands to split, focuses
detail. Esc from detail → returns focus to list (stays in split). Esc from list while split →
collapses back to collapsed state.

**Pseudo-log-line** — a always-present synthetic row at the top of the log list representing the
working tree. Background-loaded; shows three states: loading, clean ("no local changes"), or dirty
(staged · unstaged · untracked counts). Pressing Enter on it switches to the status tab carrying
the current worktree context.

**Shared worktree context** — log and status tabs share the same `WorktreeRoot`. Switching between
them (via number keys or `g+l`/`g+s`) carries the active worktree to the target tab. The worktrees
tab remains the explicit way to change which worktree the other tabs point at.

**Navigation messages** — the four app-shell message types that child models emit to drive
navigation. All are defined in `ui/nav`:

- `Open(ViewState)` — deep navigation: pushes a new entry onto the global history stack. Reversible
  with `Back`. Used for drill-down flows (e.g., log → commit, status → filtered log).
- `Switch(ViewState)` — tab switching: changes the active tab without adding history depth. Restores
  tab memory for the target tab when no explicit context is supplied. Does not pollute `Back` depth.
- `Back()` — reverse deep navigation: pops the top of the global history stack. When the stack is
  empty (at root), `Back` quits the app.
- `ViewStateChanged(ViewState)` — live view state update: emitted when the active page's internal
  state changes (selection moves, filter changes, ref advances). Updates tab memory but does not
  alter the history stack or trigger page reconstruction.

The app-shell `Update` wrapper calls `AppendViewStateChanged` after every child `Update`, comparing
pre/post `ViewState` and emitting `ViewStateChanged` automatically when navigation is enabled.
Explicit `ViewStateChanged` emissions remain supported for specialized timing needs.
