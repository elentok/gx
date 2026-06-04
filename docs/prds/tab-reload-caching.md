# PRD: Epoch-based tab reload caching

## Problem Statement

When I switch tabs — log → status → log — the log tab flickers because it re-fetches `git log`
every time I land on it. It feels slow and janky even though I changed nothing. I expect switching
back to a tab to be instant, showing exactly what I left, and to only reload when the repository has
actually changed (e.g. after I commit, push, pull, rebase, or stash).

## Solution

Tab models are already cached (the live page cache preserves selection, scroll, and split state
across switches), so the fix is to stop reloading on every activation. Introduce a **repo epoch** — a
counter the app shell bumps once per mutating git operation. Each cached tab remembers the epoch it
last loaded at; on a bare tab switch the shell triggers a gentle, state-preserving **auto-reload**
only when the tab is stale. Mutations announce themselves with a scope-free `RepoMutated` signal, so
a commit made in the status tab correctly invalidates the log tab. The louder manual `R` reload is
unchanged and remains the way to pick up changes made outside gx. The `tea.ClearScreen` blank on
every switch (a second, independent flicker source) is removed.

See ADR-0008 (`docs/adr/0008-epoch-based-tab-reload.md`) and the "Tab Caching and Reload" section of
CONTEXT.md for the canonical vocabulary.

## User Stories

1. As a gx user, I want switching log → status → log to show my previous log state instantly, so
   that navigating between tabs feels immediate instead of flickering.
2. As a gx user, I want my log selection, scroll offset, and split state preserved when I return to
   a tab, so that I don't lose my place.
3. As a gx user, I want my status filetree selection and diff scroll preserved when I return to the
   status tab, so that I don't have to re-find what I was looking at.
4. As a gx user, I want my selected stash entry and split state preserved when I return to the stash
   tab, so that switching away and back is lossless.
5. As a gx user, after I commit from the log tab, I want the log to reflect the new commit, so that
   the view is never stale after my own action.
6. As a gx user, after I commit from the status tab (`cc`), I want the log tab to show the new
   commit the next time I switch to it, so that a mutation in one tab doesn't leave another stale.
7. As a gx user, after I push/pull/rebase/amend/reword/bump in the log tab, I want those changes
   reflected, so that the log is always consistent with what I just did.
8. As a gx user, after I stage/unstage files or apply/pop/drop/create a stash, I want the affected
   tabs to refresh the next time I visit them, so that cross-tab state stays consistent.
9. As a gx user, I want a tab that was *not* affected by my action to NOT reload when I switch to it,
   so that I never see a needless flicker.
10. As a gx user, when I navigate to a specific commit (e.g. from status into a filtered log focused
    on a subject), I want the log to load and jump to that commit even if it wasn't stale, so that
    targeted navigation always works.
11. As a gx user, I want the `R` key (and status `m r`) to still force a full manual reload, so that
    I can pick up changes I made in an external terminal that gx couldn't know about.
12. As a gx user, I want manual reload to keep its current louder behavior (notification, scroll
    reset where it already does that), so that I can tell the difference between "I asked for a
    refresh" and a quiet auto-reload.
13. As a gx user, I want the terminal to not blank on every tab switch, so that switching is smooth.
14. As a gx user on multiple worktrees, I accept that an action in one worktree may cause a one-time
    reload of another worktree's tab on next switch, so that the feature stays simple (global epoch).
15. As a gx developer, I want the epoch/staleness logic in a pure, isolation-testable module, so
    that I can verify the invalidation rules without driving the whole TUI.
16. As a gx developer, I want mutation sites to declare only "the repo changed" (not which tabs to
    invalidate), so that adding a new tab later doesn't require revisiting every mutation site.
17. As a gx developer, I want pages to expose a single `AutoReload()` method and stay ignorant of
    epochs, so that the page contract stays minimal.

## Implementation Decisions

### Modules

- **`ReloadGate` (new, extracted deep module).** Pure epoch/staleness bookkeeping with no bubbletea
  dependency. The app shell owns one instance and delegates all reload decisions to it. Interface:
  - `Mutated()` — bump the global epoch (called when `RepoMutated` is intercepted).
  - `MarkLoaded(tab TabID)` — stamp a tab as fresh at the current epoch (called on construction/Init,
    on a triggered auto-reload, and for the active page when a mutation is intercepted).
  - `ShouldAutoReload(tab TabID) bool` — true when the tab's recorded epoch is behind the current
    epoch.
  - Internally: `epoch uint64` + `loadedByTab map[TabID]uint64`.
- **`nav.RepoMutated`** — fifth navigation message in `ui/nav`, mirroring the existing message
  pattern: `RepoMutated() tea.Cmd`, a `repoMutatedMsg` type, and an `IsRepoMutated(msg) bool`
  predicate. Scope-free — no worktree argument (global epoch decision).
- **App shell (`ui/app`)** — holds the `ReloadGate`; intercepts `repoMutatedMsg` in `Update`
  (gate.Mutated + MarkLoaded for the active tab); gates activation in `applySwitch`; removes
  `tea.ClearScreen` from the three switch return paths; removes `log.OnPageActivated`. Adds
  `pageAutoReloadable interface { AutoReload() tea.Cmd }` and an `autoReloadCmd` helper paralleling
  the existing `pageActivationAware`/`pageDeactivationAware` wiring. The per-page `loadedEpoch` lives
  in the `ReloadGate`, not on `livePage` or in the page model.
- **Per-tab `AutoReload()` implementations** — thin wrappers over existing reload commands:
  - log → `cmdReload()` (preserves selection index).
  - status → `refreshPreserveScroll()` (preserves filetree selection + both diff viewports).
  - stash → its list reload, preserving the selected entry + split state.
  - worktrees → none (stays manual-refresh only; not gated).
- **Emission wiring** — a `RepoMutated()` cmd batched alongside each mutating op's existing
  self-reload, at: log push/pull/commit/amend/reword/bump/rebase completions; status
  `commitFinished`, stage/unstage, push, pull, stash actions; stash apply/pop/drop/create
  completions. Worktree create/delete deliberately do **not** emit (they don't make the current
  worktree's log/status content stale).

### Behavior contract

- **Bare tab switch** (number keys, `g+l/s/w/S`, `g+,/.`): reuse the cached model; call `AutoReload`
  and `MarkLoaded` only when `ShouldAutoReload` is true. Otherwise do nothing — instant.
- **Switch carrying a `FocusSubject`/filter payload**: force the page's focus-reload regardless of
  epoch (targeted navigation always loads + jumps).
- **Reconstruct path** (`ViewContext` changed): the page's `Init()` loads; `MarkLoaded` stamps it
  fresh — no second reload.
- **`RepoMutated` intercepted**: `gate.Mutated()` bumps the epoch; the **active** page is stamped
  fresh via `MarkLoaded` (trust-the-self-reload invariant — the mutator self-reloads to show its
  result). All other cached tabs are now stale and will auto-reload on their next activation.

### Locked decisions (from grilling; see ADR-0008)

- Global epoch, not per-worktree. Accepts occasional cross-worktree over-invalidation; the
  per-worktree map is a future internal upgrade since pages already carry `worktreeRoot`.
- Trust-the-self-reload invariant: stamp the active page fresh without verifying a reload occurred.
  Documented at the stamp site; the defensive stamp-on-completion alternative is deferred.
- Auto-reload is gentler than manual `R` by design (preserves max view state; no notification).

## Testing Decisions

Good tests here assert **external behavior**, not internals: given a sequence of mutations and tab
switches, does the right tab reload (and the wrong one stay put)? Tests should not assert on private
counter values beyond what the public `ReloadGate` interface exposes.

- **`ReloadGate` (unit, no TUI).** Table-driven tests for: fresh gate reports no reload; `Mutated`
  then `ShouldAutoReload` reports stale for un-stamped tabs; `MarkLoaded` clears staleness for that
  tab only; the active-page-stamped-fresh-while-others-stale pattern. This is the primary safety net
  and the cheapest to write.
- **App shell activation (unit).** Drive the app `Update`/switch path: bare switch to a fresh tab
  issues no reload cmd; bare switch to a stale tab issues exactly one; switch carrying a
  `FocusSubject` forces a reload even when not stale; reconstruct path doesn't double-reload. Prior
  art: `ui/app/model_test.go` (existing app-shell update/switch tests).
- **Emission / regression (E2E).** log → status → log issues no git re-fetch when nothing changed;
  commit-in-status → switch-to-log reflects the new commit. Prior art: `ui/log/e2e_test.go` and the
  teatest-based flows already in the repo.

## Out of Scope

- Per-worktree epoch keying and the `RepoMutated` scope/repo-wide escape hatch (deferred; global
  epoch only).
- Auto-reloading on changes made outside gx (external terminal git). Manual `R` remains the escape
  hatch; we do not watch the filesystem.
- Epoch-gating the worktrees tab.
- Renaming existing per-tab `refresh()`/`reload()` internals beyond adding `AutoReload()` (avoid
  churn; full naming symmetry is a separate cleanup).
- Defensive stamp-on-reload-completion (deferred unless a future op violates the self-reload
  invariant).

## Further Notes

- The `tea.ClearScreen` removal is independent of the epoch work and can ship first as an isolated,
  instantly-verifiable win. Watch for leftover-row artifacts on the reconstruct path and any page
  that renders short content on its first frame (every frame is currently padded full-screen by
  `normalizeFrameContent`, so it should be clean).
- The only new public surface is one nav message (`RepoMutated`) and one interface method
  (`AutoReload`). Everything else is internal to the app shell.
