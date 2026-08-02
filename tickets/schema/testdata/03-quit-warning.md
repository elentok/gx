# 03 — Warn before quitting gx while a loop is running

**What to build:** The main app shell (`ui/app/model.go`, `ui/app/model_update.go`'s `handleBack`)
has no hook today for a page to block or warn on quit — add one (e.g. a `CanQuit()`-style
interceptor the active page/tab can implement). Wire the tickets tab's "loop active" state (from
ticket 01) into it: when the user tries to quit gx (`q`/`ctrl+c`/backing out past the root) while a
ralph-loop is running, show a confirm dialog ("a ralph loop is in progress — closing gx may leave
the worktree mid-operation, quit anyway?" or similar wording) rather than quitting immediately.
Confirming quits as normal; canceling returns to the app. This is a warn-then-allow, not a hard
block — there's no need to prevent quitting outright, since an interrupted loop is already recovered
by `ralphloop/reconcile.go` on the next run.

**Blocked by:** 01

**Status:** done
**Context window:** 135265
**Session:** c38284f0-0df5-4f94-a14e-1924f461a107

Code-review fixes: inline (ctrl+c bypassed the guard on worktrees/status/log/commit pages; added
`nav.ForceQuit()` to route it through the same app-shell guard as `handleBack`)

- [x] Attempting to quit gx while a loop is running shows a confirm dialog instead of quitting
      immediately
- [x] Confirming the dialog quits gx as normal
- [x] Canceling the dialog returns to the app with the loop still running, unaffected
- [x] Quitting while no loop is running is unchanged (no dialog)
- [x] A test covers both the warn-and-cancel and warn-and-confirm paths
