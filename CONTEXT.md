# Domain Glossary

## Panels and Viewports

**Panel** — a bordered rectangular region of the screen. A view is composed of one or more panels
(e.g. commit view = header panel + filetree panel + diff panel).

**List panel** — a panel that renders a navigable list of items (filetree, file list, commit list).
Items have a fixed height of one display row each.

**Diff panel** — a panel that renders a unified or side-by-side diff. Items are hunks or changed
lines depending on nav mode.

**Image diff** — the rendering of a changed image file as a side-by-side comparison of its old and
new versions, in place of the generic binary-file summary line. Available in any diff panel that
opts in: the status diff panel (working-tree vs index) and the commit detail panel used by the log
and stash tabs (`<ref>^` vs `<ref>`). Falls back to that summary line whenever the comparison can't
be shown faithfully (unsupported terminal, decode failure, oversized file, or user opt-out). See
ADR 0010.

**Detail panel** — an interactive, focusable panel that mirrors the currently selected list item and
supports its own keyboard navigation (e.g. the commit detail shown beside the log and stash lists).
The user can move focus into it and back out. Contrast with a sidebar.

**Screen origin** — the absolute (column, row) of a panel's top-left cell on the terminal grid. A
page that owns the whole screen has origin (0, 0); a detail panel composed into a split view does
not — it only learns its width/height, so its origin is injected by the container that knows the
layout (`splitview.DetailOrigin`). Required only by features that paint outside bubbletea's render
loop at absolute coordinates — currently the image-diff kitty overlay (ADR 0010).

**Sidebar** — a passive, non-focusable panel that renders a read-only summary of the current
selection. The user never moves focus into it; it only reflects the selected item (e.g. the
worktrees sidebar, the commit header). Contrast with a detail panel.

**Viewport** — the visible window into a panel's content. Defined by a scroll offset (first visible
row index) and a height (number of visible rows).

**Scroll offset** — the index of the first visible row in a panel's viewport. Independent of
selection.

## Find: Search and Filter

Two distinct ways to locate things in a view. They are different interaction concepts, owned by
different components, and must not be conflated.

**Search** — *highlight-and-jump* over a content stream that stays fully visible. The user types a
query; every match is highlighted in place and `n`/`N` walk the viewport from one match to the next.
Nothing is hidden. Suited to long single-column streams (diffs, file trees) where staying oriented
matters. Owned by `ui/search`, which carries match positions (`ViewportRow`/`DataIndex`) and a match
cursor; the host computes what counts as a match.

**Filter** — *narrow-the-list*. The user types a query and non-matching items disappear; only matches
remain and the layout re-flows around them. There is no match cursor and no jump — the result *is*
the narrowing. Suited to short reference lists (keybindings help, and later the file tree / log)
where the goal is "show me only X." Owned by `ui/filter`, which carries only the query, mode, and
input box and emits `FilterChangedMsg`; the host owns the matching predicate (e.g. help matches a
binding's key *and* title). Deliberately a separate component from **Search**, not an extension of
it.

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

## Decorations and Badges (Log View)

**Decoration** — a git ref (local branch, remote branch, tag, or `HEAD`) that points directly at a
commit. A commit may carry zero, one, or several. Decorations are shown wherever a commit is
presented in detail: each log row, and the commit detail header.

**Badge** — the rendering of a single decoration as its own colored pill.

**Badge group** — multiple decorations on one commit rendered as a single merged pill: one shared
background with each decoration's name keeping its own text color, instead of one pill per
decoration. Used only by condensed rows; normal-width rows render each decoration as its own
separate badge.

**Condensed row** — the narrow-width rendering of a log row: relative dates drop their "ago" suffix,
decorations render as a badge group instead of separate badges, and the gap between subject and
decorations narrows from two spaces to one. Triggered below the same width threshold used elsewhere
for narrow layouts (see Split view). Normal-width rows are unaffected by any of this.

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

## Pull and Push Lifecycle

A pull or push runs as a sequence of **phases**, some of which are interactive prompts and some of
which execute a git command (fetch, pull, push, rebase, stash). The user can leave the flow early in
two distinct ways, which must not be conflated:

**Decline** — the user says No / Esc at a *prompt* phase (confirm push, stash-before-pull, diverged
menu, force-push confirm). Nothing was executing, so the repository is untouched. Surfaced as
`Result{Aborted: true}`; suppresses the success notification.

**Interrupt** — the user kills a git command that is *mid-execution*. Implemented on top of
`CommandRunner.Cancel()`, which kills the running process. Reuses `Result{Aborted: true}` (so the
success notification is suppressed, same as a decline) but additionally emits a
`notify.Warning("push aborted")`, so the two cases are distinguished only by that warning, not by a
separate `Result` field. Unlike a decline, the working tree *can* be left in a partial state, so
interrupt is only offered on phases where killing is clean — currently the push flow's network
phases (fetch, push, force-push, tag-push). The local `rebase` phase is deliberately
non-interruptible because killing it mid-rebase leaves the repo in a `rebase-in-progress` state.
Triggered by Esc, gated behind an "Abort push?" confirm modal (default No) to guard against an
accidental keypress; if the command completes while that confirm is showing, completion wins and the
abort becomes a no-op.

## Launching External Programs

gx runs external programs (`$EDITOR`, the comment editor, `lazygit`, `git commit`) in one of two
modes, which have opposite feedback needs and must not be conflated:

**Takeover launch** — gx suspends and hands the *entire* terminal to the program; the TUI is not
visible while it runs and resumes when the program exits. Because the takeover itself is the
feedback, no "opening…"/"closed" toast is shown. On return the screen simply refreshes (the diff /
filetree updates under the user). The exception is a **mutation** run this way — `git commit` —
which reports its outcome like every other mutation: `notify.Success("committed")` plus the
repo-mutated signal. Errors always surface loudly.

**Split launch** — the program opens in a tmux/kitty split and the TUI keeps running beside it. Here
a toast (`"opened <app> split: …"`) *is* shown, because it is the only signal that the program
launched and where it went.

## Tab Caching and Reload

**Live page cache** — the app shell keeps one live `tea.Model` per `TabID` (`livePageByTab`).
Switching tabs reuses the same instance, so in-tab view state (selection, scroll offset, split
state, filetree expansion) is preserved across switches. A page is only reconstructed when its
`ViewContext` changes (different worktree/ref). Switching tabs does **not** reconstruct or, by
itself, reload a cached page.

**Repo epoch** — a single monotonic counter on the app shell, bumped once per completed mutating
git operation. It is the canonical "the repository changed" signal. Global (not keyed per worktree)
for now: a mutation in any worktree advances the one epoch. The shell records, per cached page, the
epoch the page's data was last loaded at (`loadedEpoch`, stored shell-side on `livePage`, not inside
the page model).

**`RepoMutated`** — a fifth navigation message (`ui/nav`) emitted as a `tea.Cmd` by any operation
that mutates the repository (commit, amend, reword, bump, rebase, push, pull, stage/unstage, stash
apply/pop/drop/create, worktree create/delete). The emitter only declares "the repo changed"; it
does not name which tabs are affected. The shell intercepts it, bumps the **repo epoch**, and stamps
the currently active page as fresh at the new epoch (the active page is the mutator and self-reloads
to show its own result).

**Auto-reload** — a system-initiated, state-preserving reload the shell triggers on tab activation
*only when the page is stale* (`loadedEpoch < repo epoch`). Exposed by each cacheable page as
`AutoReload() tea.Cmd` (satisfying the `pageAutoReloadable` interface). Because the user did not ask
for it, it preserves maximum view state (e.g. status uses `refreshPreserveScroll`). This replaces
the previous unconditional reload-on-every-activation.

**Manual reload** — a user-initiated reload via the `R` key (and status `m r`). Louder than
auto-reload: it may reset scroll and flashes a "refreshed" notification. It is also the escape hatch
for changes made *outside* gx (external terminal git commands), which do not bump the repo epoch.
