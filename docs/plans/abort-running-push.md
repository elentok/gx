# Abort a running push (interrupt)

Let the user interrupt an in-flight `git push` by pressing **Esc**, gated behind an "Abort push?"
confirm modal. Scoped to the push flow's **network phases only**; the local `rebase` phase stays
non-interruptible.

See the "Pull and Push Lifecycle" section of `CONTEXT.md` for the Decline vs Interrupt terminology
this plan establishes.

## Decisions (from grilling session)

- **Interrupt vs Decline** are distinct concepts. This plan implements *Interrupt* (kill a running
  command). *Decline* (say No at a prompt) already exists as `Result{Aborted: true}`.
- **Scope:** push flow only, and only the clean-kill phases: `phaseFetching`, `phasePushing`,
  `phaseTagPushing`, `phaseForcePushing`. **Not** `phaseRebasing` (killing mid-rebase leaves
  `.git/rebase-merge`). Pull is out of scope for this first cut.
- **Trigger:** Esc during a running network phase opens an "Abort push?" confirm modal.
- **Confirm default: No** — guards against an accidental Esc. (`y` / `←` then Enter to actually
  abort; Esc again dismisses.)
- **Race — completion wins:** the git command keeps running while the confirm modal is shown. If it
  finishes first, the normal result is honored and the abort becomes a no-op. `Cancel()` only fires
  if the user confirms *before* `runnerDoneMsg` arrives.
- **Notification:** confirmed interrupt emits `notify.Warning("push aborted")`, emitted **directly
  from `push.go`** (decision A) — no new `Result` field. Reuses `Result{Aborted: true}` so the
  success notification is suppressed; both the log and status parents propagate the cmd for free.
- **Reload:** mirror the decline path — reload the view, do **not** emit `RepoMutated` (no local
  mutation occurred).

## Tasks

- [x] Add `phaseAbortConfirm` to the `phase` const block in `ui/push/push.go`.
- [x] Add model fields: `abortConfirmYes bool` and `phaseBeforeAbort phase` (the running phase to
      return to on decline).
- [x] Import `github.com/elentok/gx/ui/notify` in `ui/push/push.go`.
- [x] In `handleKey`, add cases for the interruptible running phases (`phaseFetching`,
      `phasePushing`, `phaseTagPushing`, `phaseForcePushing`): on Esc, set
      `phaseBeforeAbort = m.phase`, `abortConfirmYes = false`, `phase = phaseAbortConfirm`. Leave
      `activeRunner` running and untouched. Explicitly do **not** add `phaseRebasing`.
- [x] In `handleKey`, add `case phaseAbortConfirm`: call `components.UpdateConfirm(msg,
      m.abortConfirmYes)`.
  - Not decided → update `m.abortConfirmYes`, stay.
  - Declined → `m.phase = m.phaseBeforeAbort` (resume showing the running modal); the command was
    never touched so it just continues.
  - Accepted → `m.activeRunner.Cancel()`, `m.IsOpen = false`, return
    `Result{Done: true, Aborted: true, Output: m.log.String()}` with cmd `notify.Warning("push
    aborted")`.
- [x] Confirm completion-wins works for free: `handleRunnerDone` branches on `msg.phase`, not
      `m.phase`, so a `runnerDoneMsg` arriving during `phaseAbortConfirm` already transitions the
      model normally and drops the pending confirm. Verify no code path reads `m.phase` in a way
      that breaks when it equals `phaseAbortConfirm`.
- [x] In `View`, add `case phaseAbortConfirm`: render the existing steps plus `"Abort push?"` and
      `components.RenderConfirmChoices(m.abortConfirmYes, false)`, with `components.ConfirmHint`
      (model after the `phaseForceConfirm` view block).

## Tests (`ui/push`)

- [x] Esc during `phasePushing` transitions to `phaseAbortConfirm` without cancelling the runner.
- [x] Confirming (`y`) calls `activeRunner.Cancel()` and returns
      `Result{Done: true, Aborted: true}`.
- [x] Declining (Esc / `n`) returns to the prior running phase and does not cancel.
- [x] Completion-wins: deliver `runnerDoneMsg` while in `phaseAbortConfirm` → model completes
      normally (e.g. PR prompt / done), abort is a no-op.
- [x] Esc during `phaseRebasing` does nothing (no transition to `phaseAbortConfirm`).

## Out of scope (possible follow-ups)

- Interrupting **pull** (and its auto-stash cleanup obligation).
- Interrupting **rebase** with `git rebase --abort` recovery.
- Smart per-phase recovery (option B from the grilling session).
