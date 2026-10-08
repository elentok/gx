# Server land path: canonical epic dir and explicit outcomes

Diagnosis: ticket store `follow-ups/issues/26-server-land-path-wrong-epic-dir-and-lost-outcomes.md`.

## Tasks

- [x] 1. Server passes the project dir (not the store root) as the iteration's scratch dir, in
      `server/runner.go` and `server/runs.go` (and its parks); test: a reclaimed land logs under
      the project and writes nothing under `<store>/<epic>`
- [x] 2. `landBuilt` surfaces `parkedOnChild`: the parent parks needs-repair, kind `land-conflict`,
      with a reason naming the conflict child
- [x] 3. `FinishIteration` returns the parked reason; the notification uses it instead of the fixed
      "iteration ended without landing the ticket"; a ticket neither done nor parked fails the
      finish (parked iteration-error with a reason) instead of a blank-kind notification
- [x] 4. A land never aborts a cherry-pick of a commit no iteration branch of the epic holds; it
      fails with a reason instead
- [x] 5. `land --continue` refuses (`land_superseded`) when HEAD no longer descends from the
      pre-pick head or another ticket landed since the conflict; it never stamps their commit
- [ ] 6. `alreadyApplied` trailer rung — skipped: 5 removes the only known source of wrong
      trailers; a patch check would reject legitimately re-resolved conflicts
- [x] 7. Land-lock wait cap outlasts a conflict resolution (70 min, was 30)
- [x] 8. Stop mid-finish keeps the run in the registry so the next start reclaims it
- [x] 9. Build, vet, tests (`go test ./...`: 4209 passed)

## Later (not in this change)

- Recovery (epic 5, not on main): `finishRun`'s non-landed parks should also call recovery, like
  `parkTicket` does.
- Per-epic land queue (lowest ticket first) instead of lock contention.
- Land in a temporary worktree + compare-and-swap `update-ref`.
- Unblock dependents only once the blocker's commits are on the branch.
- Clean up the phantom `<store>/<epic>` dirs and `land-locks/tickets/`.
