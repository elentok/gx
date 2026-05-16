# Retro: Notification Overlay System Migration

**Date**: 2026-05-16
**Scope**: Migrate all per-view `statusMsg` fields to app-global `ui/notify` overlay system across `ui/commit`, `ui/status`, and `ui/worktrees`.

---

## What Went Well

**Subagent isolation for `ui/worktrees`** kept the main context window clean. The subagent completed the migration correctly with 58/58 tests passing. The integration pattern (verify in worktree → diff -rq → cp → verify in main) worked cleanly once applied.

**Heartbeat diagnosis was precise.** `TestStageE2E_SideBySideShowsActiveHunkGutterIndicator` failed because removing `statusTickCmd()` eliminated the periodic wake-up the `teatest` framework depends on. The fix — a dedicated `renderTickCmd()` that fires every second and reschedules itself — was a clean separation of concerns with no status message logic attached.

**Icon recovery via `git checkout + Python`.** When the Write tool silently dropped nerd font PUA characters (U+E000–U+F8FF) from `ui/icons.go`, immediately restoring with `git checkout HEAD -- ui/icons.go` and adding the new icon via a Python script was the right approach. No time wasted trying to fix a corrupted file in place.

**DoD check at the end.** `grep -rn "m\.statusMsg\s*=" ./ui/` confirmed a clean boundary: zero remaining assignments outside `ui/log`, which is gated behind separate Beads tickets.

---

## What Didn't Go Well

**Silent non-application of changes (biggest failure).** `model_update.go` and `model_state.go` were described as modified in a prior summarized session but were actually at HEAD. This caused build failures, a misguided binary-search debug attempt, and significant re-work to re-apply all intended changes from scratch.

**Binary search left the codebase in a broken state.** During debugging, reverting files to HEAD as a bisect step created an inconsistent half-migrated state requiring `/tmp` backup restoration. The bisect was premature — a simple `git diff --stat` would have shown the files were unchanged.

**Subagent worktree had zero new commits.** The subagent left all changes as uncommitted working-tree edits (not staged, not committed). Reading `git log` on the branch showed identical history to main, which was confusing. The right signal was `git --work-tree=... status --short`, not `git log`.

**E2E tests can't observe `notify.NotifyMsg`.** Tests that instantiate `status.New()` or `worktrees.New()` directly bypass `app.Model`, so `notify.NotifyMsg` commands are emitted but never rendered. This constraint wasn't obvious upfront and required retrofitting several E2E tests with indirect verification (git state checks, model stability checks).

---

## What Would I Have Done Differently

1. **Run `git diff --stat` before any debugging.** If a prior session described changes, verify they're actually in the files before spending time on failures. Context compaction can summarize intent without confirming application.

2. **Check `git --work-tree=... status --short` immediately after a subagent finishes.** Before reading `git log` or assuming commits exist, check whether the work is committed or only in the working tree.

3. **Plan E2E test strategy before removing `statusMsg`.** Decide upfront how tests will verify notifications given the `app.Model` boundary. Don't discover the constraint mid-migration.

---

## Lessons Applied to `.ai/` Docs

- `workflow.md`: Added subagent worktree integration steps (verify → diff -rq → cp → verify in main) and a pre-debug rule (run `git diff --stat` before assuming files are changed).
- `design-system.md`: Added notification system usage rules, icon editing constraint (Python only for PUA range), E2E test caveat, and renumbered rules.
- `index.md`: Added one-line notify system reminder.
