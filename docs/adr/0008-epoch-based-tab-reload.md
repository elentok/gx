# ADR-0008: Epoch-based tab reload invalidation

**Status**: Accepted

## Context

The app shell already caches one live `tea.Model` per tab (`livePageByTab`), so switching tabs
preserves in-tab view state and never reconstructs a page unless its `ViewContext` changes. Despite
this, the log tab flickered on every activation because `OnPageActivated` unconditionally re-shelled
`git log` + branch classification every time the tab was entered — reloading even when nothing had
changed.

We want to reload a cached tab only when the repository has actually changed since that tab last
loaded, while still catching cross-tab mutations (e.g. committing from the status tab makes the log
stale).

## Decision

Introduce a **repo epoch**: a single monotonic counter on the app shell, bumped once per completed
mutating git operation. Mutating operations emit a new scope-free navigation message,
`nav.RepoMutated`, which only declares "the repo changed" — it does not name affected tabs. The
shell intercepts it, bumps the epoch, and stamps the currently-active page as fresh at the new
epoch.

The shell records, per cached page, the epoch its data was last loaded at (`loadedEpoch`, stored
shell-side on `livePage`, not in the page model). On a **bare** tab switch it calls the page's
`AutoReload() tea.Cmd` only when `loadedEpoch < epoch` (auto-reload preserves maximum view state).
Switches carrying a `FocusSubject`/filter payload force a reload regardless of epoch. User-initiated
`R` ("manual reload") is unchanged and remains the escape hatch for changes made outside gx.

## Considered Options

- **Global epoch vs per-worktree epoch.** Chose global for simplicity, accepting that a mutation in
  one worktree marks other worktrees' tabs stale (a needless reload on next switch). gx is
  multi-worktree, so per-worktree keying is the "correct" model — but shared repo state (fetch/pull
  remote-tracking refs, the repo-wide stash stack) makes per-worktree keying leaky anyway, and you
  usually view one worktree at a time. Upgrading to a per-worktree map later is an internal change;
  pages already carry `worktreeRoot`.
- **Per-tab dirty flags vs epoch.** Rejected per-tab flags: every mutation site would have to know
  *which* tabs to dirty and would get it wrong as tabs are added. The epoch lets each page
  self-decide.

## Consequences

- **Trust-the-self-reload invariant.** On `RepoMutated` the shell stamps the active page fresh
  without verifying it reloaded, relying on the invariant "a page that mutates the repo self-reloads
  to show its result." If a future op bumps the epoch without self-reloading, that page would show
  stale data until manual `R`. This is documented at the stamp site; the defensive alternative
  (stamp only on reload completion) was deferred.
- Epoch bookkeeping lives entirely in the shell; pages stay ignorant of epochs and expose only
  `AutoReload()`. The new public surface is one nav message and one interface method.
- External changes (terminal git commands) do not bump the epoch and so won't auto-appear on tab
  switch — manual `R` covers them, as before.
- `tea.ClearScreen` was removed from the tab-switch path (a separate flicker source); every frame is
  already padded full-screen by `normalizeFrameContent`, so the renderer's diff handles the swap.
