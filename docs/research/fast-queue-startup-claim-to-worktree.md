# What takes up to 150s between claim and worktree creation

Ticket: `gx:fast-queue-startup/14`. Date of measurements: 2026-11-05 to 2026-11-06, on the live store at `~/.local/share/gx/tickets/gx` and the server log at `~/.local/state/gx/server.log`.

## Answer

The delay is `l.Index.Rescan()` in `ClaimAndLaunch` (`server/sched/run.go:55`), right after the claim write. A rescan runs the `OnRescan` steps, and one of them, the one-offs archive sweep (`server/queue/oneoffsweep.go:20`), waits 10s for a lock that nothing will ever release. It does that twice per sweep (two stuck tickets), so every rescan costs 20s, and rescans queue behind each other on `sweepMu`.

The lock is `~/.local/share/gx/tickets/gx/one-offs-archived/.alloc.lock`. It is an empty file made on 2026-11-04 at 16:39 (`stat`: `Nov 4 16:39`) by a process that exited without removing it. The lock code has no way to tell a dead holder from a live one (`tickets/ticketmove/allocate.go:44-65`).

It does not hit "only some epics". It hits every launch since the lock went stale. The slow epics are the only ones that launched after 16:39 on 2026-11-04. The 23 fast epics all finished launching before that.

## Evidence

### 1. The server log shows a sweep running back to back, forever

`~/.local/state/gx/server.log` holds 190 `archive one-off` warnings. They alternate between `gx:one-offs/52` and `gx:one-offs/52a`, exactly 10s apart, with no gap, for the whole log window:

```
23:44:22.696 WARN archive one-off ticket=gx:one-offs/52  err="timed out waiting for allocation lock at .../one-offs-archived/.alloc.lock"
23:44:32.711 WARN archive one-off ticket=gx:one-offs/52a err=...
23:44:42.726 WARN archive one-off ticket=gx:one-offs/52  err=...
23:44:52.735 WARN archive one-off ticket=gx:one-offs/52a err=...
```

Coverage: the log starts 2026-11-05 22:04:54 and ends 23:57 (597 lines, 7 server starts). It does not cover the claim of 2026-11-04 16:39 or the first slow launch (17:59), so I cannot say what killed the lock holder. The run logs and the lock file's mtime cover that.

The ticket files `one-offs/issues/52-*.md` and `52a-investigate.md` are terminal and untracked, so every sweep tries to archive both, fails after 10s each, and tries again on the next rescan.

### 2. The code path

- `ClaimAndLaunch` writes the claim (`ticketops.ClaimWithBase`, `server/sched/run.go:54`), then `l.Index.Rescan()` (`:55`). Nothing else on the way to `mode.Prepare` (`:145`) is slow, see "What I ruled out".
- `Index.Rescan` calls `rescanTree`, which runs `Refresh` and then every `OnRescan` step (`server/index/watch.go:149-169`).
- The steps are registered at `server/server.go:431-434`: `ScanParentDefects`, `ScanLateFinishes`, `oneOffs.Sweep`, `designs.ScanCleanups`.
- `Sweep` takes `sweepMu` (`server/queue/oneoffsweep.go:22`), loads the one-offs, and calls `archiveOneOff` for each terminal untracked ticket (`:43-51`).
- `archiveOneOff` calls `ticketmove.LockEpics(one-offs, one-offs-archived)` (`:99`). `LockEpics` sorts the paths and locks each (`tickets/ticketmove/move.go:26-43`). `one-offs` locks at once; `one-offs-archived` has the stale file.
- `lockEpicFile` creates `.alloc.lock` with `O_EXCL` and polls every 10ms for 10s, then returns `timed out waiting for allocation lock` (`tickets/ticketmove/allocate.go:44-65`). The file holds no pid and nothing checks its age.
- So one `Sweep` takes 2 x 10s = 20s, and `Rescan` returns only after it.

The other callers of `Rescan` also run it inline: the watcher goroutine's backstop poll every 15s and its debounced fsnotify path (`server/index/watch.go:87,99`), the `/changed` ping handler that every `gx tickets` write sends (`:142`), and the settle code (`server/sched/run.go:234,281`). Each takes `sweepMu` (`sync.Mutex`, not FIFO), so with a 15s poll, a ping per ticket write and the claim's own file-watch event, a sweep is always in flight. The claim's `Rescan` waits for the sweep in flight, then for any that win the mutex first, then runs its own 20s sweep.

### 3. Reproduction

A throwaway program (removed afterwards) made a temp dir with `one-offs/` and `one-offs-archived/.alloc.lock` and called `ticketmove.LockEpics`, twice:

```
LockEpics with stale archive lock: 10.01s err=timed out waiting for allocation lock at .../one-offs-archived/.alloc.lock
LockEpics with stale archive lock: 10.01s err=timed out waiting for allocation lock at .../one-offs-archived/.alloc.lock
```

So a sweep over the two stuck tickets costs 20.0s, as the log shows. I did not touch the real store's lock or tickets.

### 4. The numbers fit

Claim to `deps-installed` for the 307 launches since 2026-11-04 17:00 (claim from the ticket's `claimed_at`, which keeps only the last claim, so I kept tickets with one `deps-installed` and one `iteration-started` event; script output in the scratchpad):

- 5s buckets with the most launches: 25-30s (45), 40-45s (51), 60-65s (42), 80-85s (40), 100-105s (29), 120-125s (12). Steps of 20s, as the sweep length predicts. The minimum is 25s, which is one 20s sweep plus the claim's other steps.
- The median is 65s for `split-server-pkg-impl` and the worst values are 150 to 570s (those may include a re-claim).
- `webui-paste-image-impl/01`: claimed `21:44:44.628Z` (23:44:44 local, `issues/01-*.md`), worktree dir 23:47:13, `deps-installed` 23:47:14.05 (run log). That is 148.8s, about 7 sweeps.

The switch is sharp. All 18 launches of `split-server-pkg-impl` between 12:00 and 16:04 on 2026-11-04 took 1.5 to 3.1s. The next launch, ticket `33a` at 17:59:59, took 39.3s. The lock file was made at 16:39 and no launch ran between. After 17:00 the hourly median stays between 40s and 110s.

### 5. Why "only some epics"

It is the time, not the epic. After 2026-11-04 16:39 the only epics that launched anything are `split-server-pkg-impl`, `better-epic-header-impl`, `follow-ups-2026-11-04` and `webui-paste-image-impl`. They share no setting (no common `base:`, no stacked branch, 21 to 367 tickets). Every other epic's launches are dated before the lock. Any epic launched now would be slow.

Epic size does not matter either: `tickets.Load` over the real store (2,876 tickets in 166 epics, the part of `Rescan` that reads files) takes 0.01 to 0.07s (3 runs, read-only).

### What I ruled out

- Ticket store read: `tickets.Load` 0.01 to 0.07s.
- `git worktree add` of gx: 0.3s (from the deps-install findings).
- `worktree.Lock` contention: its holders are `addIterationWorktrees`, `PrepareFeature`, `finishCleanup` and the merged-epic removal (`ralphloop/lifecycle.go:146`, `ralphloop/finish.go:470`, `server/merge/worktree_removal.go:70`). They do git work and none has a 20s period. The claim stalls before it reaches `Prepare`, and the next phase after the stall (worktree dir to `deps-installed`) is 0.6 to 2s.
- Feature-branch rebase: `addFeatureWorktree` (`ralphloop/feature.go:158`) only rebases when the epic branch lacks the base's commits. It cannot explain the 20s steps, and the slowdown began before commit `0a3354cf4` (2026-11-05 23:05) made the rebase run on every launch. It may add its own cost on a long-lived branch; it is not what is measured here.
- Base on another epic, many tickets: not shared by the slow epics.
- herdr and git timers: no 20s period found. The only 20s is two 10s lock timeouts.

## Side effects of the same stuck lock

- The Serve loop runs `ClaimNext` and also blocks on the launch, so the 20s to 150s also delays trunk checks and slot release (see the launch-timeline findings).
- Every rescan, including the watcher's own, is 20s long. So the web UI's ticket stream and every `gx tickets` write that pings `/changed` lag by the same amount. The ping handler waits for the rescan before it returns.
- 190 warnings in a 2h log, one every 10s.

## Fix options

1. **Remove the stale file** (`rm ~/.local/share/gx/tickets/gx/one-offs-archived/.alloc.lock`). The sweep then archives `52` and `52a` and stops retrying. Cost: none, but it cures this one case and the next crash brings it back. I did not do it: it is a change in the real store, for the person to approve.
2. **Make the allocation lock die with its holder.** Use `flock` (the kernel drops it when the process exits), the way `migratefail/migratefail.go:65-76` and `gocachegate` already do, or write the pid into the file and break the lock when the pid is gone. This is the root fix. Cost: small change in `tickets/ticketmove/allocate.go`, a lock-file format change for readers of the old `O_EXCL` files (a leftover old file must be treated as free under flock, which it is).
3. **Take `OnRescan` steps off the launch path.** `ClaimAndLaunch` needs only `Refresh` ("the claim must show now", `run.go:55`). Add `Index.Refresh`-only call there, or run the steps on a goroutine or a ticker. This makes a launch immune to any slow step, not only this lock. Cost: the sweep and cleanup steps must be safe to run from one place, which `sweepMu` already assumes.
4. **Do not retry a failing archive on every rescan.** Back off per ticket (or after the first lock timeout skip the rest of the sweep). Cost: a few lines in `server/queue/oneoffsweep.go`.
5. **Shorten the 10s lock timeout.** Treats the symptom (20s becomes 2s); do not do it alone.

## Recommendation

Do 2 and 3, and 4 as a small guard, as the clean fix. 2 stops a crash from leaving a lock that blocks the store; 3 stops any `OnRescan` step from ever slowing a launch, and the spec for this map wants a 15s launch that cannot depend on unrelated sweeps. Do 1 now so the numbers on this map and the later timing work are not measured against a 20s offset. After 1, the launch timeline's "claim to worktree" should be 1 to 3s (as the 23 earlier epics show).

## Consequences for the map

- The 15s target stays reachable. This delay is a fault, not a step of the design, and it is not in the 3.5s floor from the timeline findings.
- Earlier numbers in this map that include claim to worktree (the timeline median of 97s, the deps-install "29s to 141s") carry this stall. The timeline's "deps install 66s" is really this wait, not the install.
- The timeline should be re-measured after fix 1 or 2, to get a clean baseline.
- A per-step timing event at the claim, after the rescan and at the worktree, would have shown this at once. That supports the per-step events in the spec.
