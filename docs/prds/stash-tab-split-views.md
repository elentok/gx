# PRD: Stash Tab + Split Views

## Problem Statement

The gx TUI has several friction points when working across commits, stashes, and worktrees:

- There is no way to browse stash contents inside the app. Stashes can be created from the status
  tab, but inspecting them requires leaving gx entirely.
- The commit tab is a dead-end destination: it can only be reached from the log (Enter on a
  commit), and its direct keyboard shortcut (`g+c`, `4`) is misleading — it lands on the last
  visited commit with no way to choose a different one.
- Switching between the log and status tabs drops worktree context silently. If the log is showing
  worktree Y, pressing `3` for status shows worktree X (the default), with no indication.
- There is no passive signal in the log tab that uncommitted changes exist. Users must switch to
  the status tab to find out.

## Solution

Replace the commit tab with a split view pattern used by both the log and a new stash tab:

- The **log tab** gains a detail panel on the right/bottom. The list starts collapsed (list only);
  pressing Enter on a commit expands the detail panel and focuses it. Navigating the list in split
  mode auto-updates the detail panel.
- A new **stash tab** (`4`, `g+S`) lists all stashes and opens in split mode by default, with the
  first stash's detail pre-loaded.
- A **pseudo-log-line** at the top of the log list shows working tree status (staged/unstaged/
  untracked counts), loaded in the background. Pressing Enter on it switches to the status tab.
- **Tab switching** between log and status now carries the active worktree automatically.

## User Stories

1. As a developer, I want to inspect stash contents inside gx without leaving the app, so that I
   can decide whether to pop or drop a stash.
2. As a developer, I want to see a diff of a stash entry, so that I can review exactly what changes
   are stored.
3. As a developer, I want to navigate between stash entries with j/k and see the diff update
   automatically, so that I can browse stashes quickly.
4. As a developer, I want to open the stash tab with a single keypress (`4` or `g+S`), so that I
   can access it as efficiently as other tabs.
5. As a developer, I want the log tab to load quickly without loading diff data upfront, so that I
   can scan the commit history without waiting.
6. As a developer, I want to press Enter on a commit in the log to expand the detail panel and see
   the diff, so that I can inspect a commit without leaving the log context.
7. As a developer, I want to navigate the commit list in split mode and have the detail panel
   update automatically, so that I can browse commit diffs fluidly.
8. As a developer, I want to press Esc from the detail panel to return focus to the list, so that I
   can resume navigating commits without closing the detail view.
9. As a developer, I want a second Esc (from the list while split) to collapse the detail panel,
   so that I can get back to a clean list-only view.
10. As a developer, I want to press `f` to expand the focused panel to fullscreen, so that I can
    read a long diff or a long stash list without distraction.
11. As a developer, I want to press `f` again to exit fullscreen and return to the previous split
    state, so that I can go back to the dual-panel view.
12. As a developer, I want the split orientation to auto-detect based on terminal width (side-by-
    side for wide terminals, stacked for narrow), so that the layout is useful in any terminal size.
13. As a developer, I want to toggle split orientation with `to`, so that I can override the auto-
    detected layout.
14. As a developer, I want the log tab to always show a working tree status line at the top, so
    that I can see at a glance whether I have uncommitted changes without switching to status.
15. As a developer, I want the working tree status line to show staged, unstaged, and untracked
    counts separately, so that I have enough context to decide whether to act.
16. As a developer, I want the working tree status line to show a loading indicator while status is
    being fetched, so that the log doesn't appear to freeze on startup.
17. As a developer, I want to press Enter on the working tree status line to jump to the status
    tab, so that I can act on uncommitted changes in one keystroke.
18. As a developer, I want the status line to say "no local changes" when the working tree is
    clean, so that it is always present and doesn't cause list layout jumps on load.
19. As a developer, I want switching from the log tab to the status tab to keep the same worktree,
    so that I don't land on the wrong worktree silently.
20. As a developer, I want switching from the status tab to the log tab to keep the same worktree,
    so that context is consistent when I move between the two.
21. As a developer, I want the stash tab to open in split mode by default, so that I can
    immediately see the diff of the first stash without an extra keypress.
22. As a developer, I want stash-specific actions (pop, drop, apply) to be accessible from the
    stash list panel, so that I can manage stashes without leaving the tab.
23. As a developer, I want the commit tab shortcut (`g+c`, `4`) to be repurposed for stash, so
    that the stash tab is as accessible as the old commit tab was.

## Implementation Decisions

### Module breakdown

**`git` package (modify)**
- Add `StashList(dir string) ([]StashEntry, error)`. A `StashEntry` carries: index (int), ref
  string (`stash@{N}`), message string, and timestamp. Parsed from `git stash list
  --format=...`.
- Add `WorktreeStatusSummary(dir string) (staged, unstaged, untracked int, err error)` for the
  pseudo-log-line background loader. Reuses the existing `git.Status` infrastructure.

**`ui/nav` (modify)**
- Remove `TabCommit` constant.
- Add `TabStash TabID = "stash"`.

**`ui/navstate` (modify)**
- Fix shared worktree context: when switching tabs with no explicit `WorktreeRoot`, inject the
  current active tab's `WorktreeRoot` into the target tab's view state (rather than restoring
  from independent per-tab memory).
- Seed `TabStash` in `initMissingTabs` with the default worktree.
- Remove `TabCommit` handling from `TabViewStateForViewContext`, `initMissingTabs`,
  `ResolveTabID`.

**`ui/splitview` (new — deep module)**
- Shared container package used by both log and stash tabs.
- Owns the panel visibility state machine:
  - `Collapsed` — list panel only (log default)
  - `Split` — both panels visible
  - `Fullscreen` — focused panel fills screen, other hidden
- Owns orientation: `Vertical` (side-by-side) or `Horizontal` (stacked). Auto-selected based on
  terminal width (`width <= 100` → horizontal, same threshold as `status.useStackedLayout`).
  Overridden by `to` chord (`toggle-layout-orientation`).
- Owns focus: list-focused or detail-focused.
- Key handling (at container level):
  - `enter` (list focused, collapsed) → expand to Split, focus detail, load selected item
  - `enter` (list focused, split) → focus detail
  - `esc` (detail focused) → focus list
  - `esc` (list focused, split) → collapse to Collapsed
  - `f` → toggle fullscreen for focused panel
  - `to` → toggle orientation
- Auto-update: when list selection changes in Split state, fires a load command for the new
  selection into the detail panel. The detail panel accepts a `SetRef(ref string)` call to
  replace the displayed commit.
- Layout helpers: `ListSize() (w, h int)` and `DetailSize() (w, h int)` derived from current
  state, orientation, and terminal dimensions.
- The list panel and detail panel are injected as interfaces; `splitview` does not import
  `ui/log` or `ui/commit` directly (no circular deps).

**`ui/log` (modify)**
- Embeds the split view container.
- Default panel visibility state: `Collapsed`.
- Pseudo-log-line: always the first row in the list. States:
  - Loading: spinner + "loading worktree status…"
  - Clean: "no local changes"
  - Dirty: staged count · unstaged count · untracked count (zero counts omitted)
  - Background-loaded via `Init` / worktree-change command.
  - Enter on pseudo-log-line: `nav.Switch(ViewState{Tab: TabStatus, WorktreeRoot: worktree})`.
  - Never opens the detail panel (it is not a commit ref).
- On Enter on a real commit row in Collapsed state: expand to Split, focus detail, load commit.
- Removes all `nav.Open(ViewState{Tab: TabCommit})` calls.

**`ui/stashlist` (new — deep module)**
- New package (distinct from `ui/stash` modal which handles stash creation).
- Loads `git.StashList` on Init.
- Renders stash rows: index badge, message, relative timestamp.
- Basic navigation: j/k, enter, esc.
- On selection change: emits the selected `StashEntry.Ref` for the split container to forward
  to the detail panel.

**`ui/app` (modify)**
- Remove `TabCommit` from `tabsView`, `orderedTabs`, `ensureLivePages`, `newHistoryEntry`.
- Remove `g+c` and `4 → TabCommit` bindings.
- Add `TabStash` to all of the above.
- Add `g+S` and `4 → TabStash` bindings.
- Instantiate stash split view (stashlist + commit view) for `TabStash`.
- Default panel visibility for stash: `Split`.

### Panel visibility state machine

```
Collapsed ──[enter on commit]──► Split (detail focused)
Split (detail focused) ──[esc]──► Split (list focused)
Split (list focused) ──[esc]──► Collapsed
Any ──[f on focused panel]──► Fullscreen (focused panel)
Fullscreen ──[f]──► previous state (Collapsed or Split)
```

### Shared worktree context rule

When `navstate.Switch` is called with `WorktreeRoot == ""`, the navstate resolves the worktree
from the *current* active tab's view state rather than from the target tab's per-tab memory. This
means `g+2` (log) and `g+3` (status) always stay in sync with whatever worktree was active.
Explicitly navigating from the worktrees tab (Enter on a row) remains the mechanism to change
which worktree a tab points at.

## Testing Decisions

Tests should verify observable behavior, not implementation details. Do not test private fields or
intermediate states — test what a user would see or what a downstream consumer would receive.

**`git.StashList`** — unit tests with a real git repo fixture (see `git/stash_test.go` for the
existing pattern). Cover: empty stash, single entry, multiple entries, entries with and without
custom names.

**`git.WorktreeStatusSummary`** — unit tests with real git fixtures (see `git/status_test.go`).
Cover: clean repo, staged only, unstaged only, untracked only, mixed.

**`ui/navstate`** — unit tests (see existing `navstate` test file). Add cases for: switching from
log (worktree Y) to status carries Y; switching from status (worktree Y) to log carries Y;
`TabStash` is seeded correctly; `TabCommit` is gone from `ResolveTabID`.

**`ui/splitview`** — unit tests for the state machine and layout helpers. Cover all state
transitions (see state machine above), layout sizes in each state and each orientation, auto-
orientation threshold, fullscreen size calculation. Prior art: `ui/status/model_test.go` for
layout sizing tests.

**`ui/stashlist`** — unit tests for list rendering and navigation. Cover: empty list, single entry,
multiple entries, selection changes. Prior art: `ui/log/model_test.go`.

**`ui/log`** — update existing test suite. Add cases for: pseudo-log-line in loading/clean/dirty
states; Enter on pseudo-log-line emits status switch; Enter on commit in collapsed state expands
to split; Esc from detail returns list focus; Esc from list (split) collapses. Prior art:
`ui/log/model_test.go` and `ui/log/e2e_test.go`.

**`ui/app`** — update existing test suite. Cases for: stash tab reachable via `4` and `g+S`;
commit tab shortcut gone; worktree context carried on log↔status switch. Prior art:
`ui/app/model_test.go`.

## Out of Scope

- Stash actions (pop, drop, apply) from the stash tab — listed in user stories for completeness
  but not implemented in this PRD. The stash tab is read-only in this iteration.
- Multi-select in the stash list.
- Search within the stash list.
- The `g+c` binding being repurposed for anything other than removal.
- Any changes to the worktrees tab.
- Changes to the status tab layout or behavior beyond the worktree context fix.

## Further Notes

- The existing `ui/stash` package (stash creation modal) is unaffected. The new list package is
  named `ui/stashlist` to avoid any naming conflict.
- ADR-0006 documents the removal of `TabCommit`. ADR-0007 documents the shared worktree context
  decision. Both are in `docs/adr/`.
- The `ui/splitview` package must not import `ui/log`, `ui/commit`, or `ui/stashlist` to avoid
  circular dependencies. It operates on injected interfaces.
- The commit view model (`ui/commit`) is reused as the detail panel in both log and stash split
  views. Stash refs (`stash@{N}`) are valid commit refs for `git show` and the commit view
  handles them without modification.
