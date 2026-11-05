# Parallel launches: what a launch shares and what N launches cost

Ticket: `gx:fast-queue-startup/04`. Question: what shared state does a launch touch, what breaks or slows down if `ClaimNext` runs `ClaimAndLaunch` for several tickets at once, what do 1, 2, 4 and 6 launches cost in CPU and memory, and do launches in flight need their own cap below the slot limit.

Measured 2026-11-05 to 2026-11-06 on the dev laptop (12 cores, 24 GB, APFS, claude 2.1.318, pnpm 12.16.1 pinned to 12.8.1 by `webui/package.json`). Other agents were running all the time, so the load average is recorded with each row.

## Short answer

1. Most shared state is safe to touch from several goroutines. The registry, the broker, the ticket claim write, the run log and the pty runner already take their own locks. Nothing needs a new lock for correctness except the feature worktree create, which `worktree.Lock` already covers.
2. Two things are unsafe or wrong as the code stands. First, the caps (`CountRoot`, `ProjectAtCap`, `running`) are only right if the run is in the registry before the next cap check, and today the registry entry is written late in `ClaimAndLaunch`. A goroutine split must reserve the slot in the part that stays in the claim loop. Second, `Guards.AdoptMu` is held for the whole launch today. It only needs to cover the claim write.
3. Git: unlocked parallel `git worktree add` failed once in about 120 adds (a half-written `worktrees/<id>/commondir` read by a sibling add). Two adds of the same new branch always fail (8 of 8 tries). Keep `worktree.Lock` around the adds. It costs about 0.3s per launch, so 6 launches queue up about 2s on the lock.
4. Cost of a wave with a warm pnpm store (load 9 to 24, mostly other agents): 1 launch 0.9s wall and 2.0 CPU-s, 2 launches 1.4s and 4.3 CPU-s, 4 launches 2.4s and 10.8 CPU-s, 6 launches 3.5s and 19 CPU-s, 10 launches 5.9 to 7.3s and 42 to 45 CPU-s. Peak RSS about 330 MB per launch (200 MB for 1, 4.0 GB for 10). Memory pressure stayed at 67 to 75% free.
5. The costs that break 15s are not CPU or memory. With a cold shared pnpm store, 4 and 6 concurrent installs take 19 to 23s. With a warm store but a cold policy cache, 6 concurrent installs take 12 to 25s. Parallel launches do not speed up a cold install. One install first, then the siblings warm, took 6.8s for 6.
6. A cap on launches in flight below the slot limit is not needed for CPU or memory on this machine. Today's ready wait is the exception: each waiting launch polls the transcript glob at 100ms, which costs about 0.3 of a core per launch (81 CPU-s over 30s for 6 launches). That goes away with the ready-check fix.

## Sources and method

- Code read at `origin/main` 0a3354cf4: `server/sched/claim.go`, `server/sched/run.go`, `server/sched/launch.go`, `server/runs/registry.go`, `server/runs/guards.go`, `ralphloop/lifecycle.go`, `ralphloop/feature.go`, `ralphloop/worktree/worktree.go`, `ralphloop/deps/deps.go`, `nativerunner/pty.go`, `nativerunner/pty_registry.go`, `nativerunner/pty_launch.go`, `server/index/index.go`, `server/budget/budget.go`, `agentrunner/herdrrunner/herdrrunner.go`, `herdr/workspace.go`.
- Benchmark harness (throwaway Go program in the scratchpad, not committed, deleted after use). Per simulated launch it runs the real steps in this order: `git worktree add -b bN wN main` in a temporary bare clone of gx (taken under a mutex that stands in for `worktree.Lock`, or not, per row), then `pnpm install --frozen-lockfile` in `webui/` with a temporary `--store-dir` and `XDG_CACHE_HOME` (warm unless the row says cold), then the real `nativerunner.Pty.Start` of an idle `claude --session-id` through a built `gx agent-holder`. It does not send the launch prompt, so no API turn runs. The claude cwd was the research worktree.
- CPU: `getrusage(RUSAGE_CHILDREN)` delta over the whole round, which counts every child that exited, including claude and the holder after the round stopped them. "Launch CPU" is the delta when the last launch returned, which counts only git and pnpm (claude was still running). "Total CPU" is after the stop. Peak RSS is the sum of `ps rss` over the bench's process tree, sampled every 200ms. It counts shared pages more than once, so it is an upper bound. `ps pcpu` is a decaying average on macOS, so I did not use it.
- Free memory is `memory_pressure -Q` before and after.
- Limits of the harness: the bare clone has 1 worktree, the real `.bare` has 62; the network and registry were fast; claude was idle and untrusted-dir dialog handling was not part of it; the 30s ready wait and the launch prompt proof were not run (their glob cost is measured separately below).

## Shared state of a launch

Order of one launch from the claim loop to the agent, with the code line. The first block (steps 1 to 17) is the claim-to-worktree span ticket 14 is investigating. I list the steps and leave the cause of its delay to that ticket.

### Claim-to-worktree steps

| # | Step | Code |
|---|------|------|
| 1 | Pass gates: migrating, queue blocked, budget soft limit | `server/sched/claim.go:57` |
| 2 | Concurrency limit and running count from the registry | `claim.go:60-61` |
| 3 | Take `AdoptMu` and hold it until the launch returns | `claim.go:123-125` |
| 4 | Root cap, project cap (reads `project.json` from disk), path/disk/trunk/migration checks, `Projects.Of`, `Projects.Epics` | `claim.go:177-199` |
| 5 | Deadlock check, next untracked frontier ticket, parent adoption check | `claim.go:206-216` |
| 6 | Index sum vs file sum check | `claim.go:222-229` |
| 7 | Harness resolve and claim hold (usage gate, rate limit, herdr down) | `claim.go:230-233`, `server/sched/claimhold.go:19` |
| 8 | `Projects.Dir` | `server/sched/run.go:35` |
| 9 | `ResolveBase`: derive leaf base, `EpicRefs`, 2 to 3 git calls (`rev-parse`, `rev-parse`, `merge-base`) | `run.go:39`, `server/sched/baseref.go:19-46` |
| 10 | `AnsweredSession` (parse ticket, read log) | `run.go:51` |
| 11 | Claim write: shared project lock, ticket lock, atomic write | `run.go:52`, `ralphloop/ticketops/claim.go:38`, `:209-220` |
| 12 | `Index.Rescan` (full store scan) and `ticket-claimed` event | `run.go:55-56`, `server/index/watch.go:149-169` |
| 13 | Clear long-ticket mark, `LaunchModel` | `run.go:58-61` |
| 14 | `DepsWith`, harness lookups, `IterationModeFor`, commitless ref or merge target tip (`MergeTarget` loads the project's epics) | `run.go:69-89`, `baseref.go:85-101` |
| 15 | `Registry.PutIfAbsent` with state `launching` (writes `runs.json`) | `run.go:94`, `server/runs/registry.go:477-486` |
| 16 | Take `worktree.Lock`; feature worktree add; rebase of the epic branch onto its base when behind; `rev-parse`; iteration worktree add; release | `ralphloop/lifecycle.go:134-168` (lock at 146-147), `ralphloop/feature.go:158-182` |
| 17 | `git ls-files` for lockfiles, then the install, then the `deps-installed` event | `ralphloop/deps/install.go:63-80`, `ralphloop/install.go:23` |

Then: `Registry.Put` with workspace and cwd (`run.go:99`), `Pty.Start` (holder spawn, claude idle), the ready wait and prompt (`launch.go:140-196`), `Registry.Put` of the finished run (`run.go:117`), and `GoSettle` for the finish (`run.go:121`).

Correction to the brief: `worktree.Lock` is held across the whole of `addIterationWorktrees`, not only `git worktree add`. That is the feature worktree add, the status and ancestor checks, a possible rebase of the epic branch onto its base, a `rev-parse` and the iteration add (`lifecycle.go:146-161`). Other holders: `finish.go:470`, `worktree_removal.go:70`, `RemoveIterationWorktree`, the commitless and feature prepare. Held time for the common case (feature worktree exists), timed on the temporary clone at load 4: `rev-parse` 16ms, `merge-base --is-ancestor` 16ms, `git status --porcelain` 24ms, `rev-parse` 15ms, `git worktree add` 260ms. So about 0.35s per launch, more when the epic branch is rebased.

### Verdicts

Verdict key: safe = can run from several goroutines as written; needs a change = correct only after the change named; unsafe = fails without a lock.

| Shared thing | Verdict | Why | Code |
|---|---|---|---|
| Run registry `PutIfAbsent`, `Put`, `Delete*` | safe | One mutex; `PutIfAbsent` refuses a second entry for a ticket. Each write rewrites the whole `runs.json` under that mutex, so writes are serial (3 per launch). Not measured; the file holds one entry per live run. | `registry.go:477-503`, `registry.go:149-166` |
| Slot counts (`Count`, `CountRoot`, `CountProject`) include `launching` runs | needs a change | They count every entry that holds a slot, launching ones too, so the count is right once the entry exists. Today the entry is written at `run.go:94`, after the claim write, the rescan, the model lookup and the base calls. In the sequential loop that is fine because the next cap check runs after the launch returns. With goroutines the next check would run before the entry exists and exceed the caps. Reserve the slot (registry entry) in the part that stays in the loop, before the goroutine starts. | `registry.go:189-210`, `claim.go:61,70-79,177-181` |
| `running` count in `ClaimNext` | needs a change | A local counter, plus `running++` when `ClaimAndLaunch` returns true. With a split, "launched" must mean "slot reserved", or the counter and the registry disagree. | `claim.go:60-79` |
| Per-root cap, project cap | needs a change | Check then act, both read the registry. Safe only with the reservation above. `ProjectAtCap` reads `project.json` on each call. | `claim.go:148-160,177-181` |
| `Guards.AdoptMu` | needs a change | Today it is held through the whole launch (`claimRootAfterAdoption` calls `ClaimAndLaunch` under it), so a late-finish adoption waits for a 30s launch (`latefinish.go:219`). It only has to cover the claim write: adoption cancels only investigations whose status is still `open` (`latefinish.go:283`), and the claim write makes the ticket `claimed`. Release it after step 11. | `claim.go:123-125`, `server/merge/latefinish.go:219,283` |
| Ticket claim write | safe | Per-ticket file lock plus a shared project lock (a migration takes it exclusive), atomic rename. A claim does not trigger a store commit (only done, cancelled, paused do). | `ticketops/claim.go:209-220`, `:269-277` |
| Ticket index `Rescan` | safe, wasteful | `scanStore` runs outside the index lock; the swap takes it. Concurrent scans do not race, but the last writer wins, so a slow older scan can overwrite a newer one and hide a claim until the next poll. Cost: 1 scan 0.04s wall and 0.09 CPU-s, 6 at once 0.11s wall and 0.76 CPU-s (6,101 ticket files, load 10 to 16). Coalesce the rescans (one in flight, one queued) or number the scans. | `server/index/index.go:123-138`, `watch.go:149-169` |
| Event broker `Publish` | safe | One mutex and a sequence number. | `server/broker/broker.go:51-66` |
| Run log append | safe | `O_APPEND` single write under the line cap. | `ralphloop/runlog/eventlog.go:199-218` |
| Budget soft limit | safe | Read once per pass, not per launch, so a serial pass does not re-check it either. | `claim.go:57`, `budget.go:140` |
| Budget hard-limit kill and `LaunchingRuns` | safe | The kill list skips `launching` runs (`LaunchingRuns` is a test seam). More runs in `launching` at once changes nothing. | `server/budget/budget.go:285`, `testseams.go:44-47` |
| Claim hold (harness missing, herdr down, rate limit, usage gate) | safe | Reads only. The usage gate cannot count launches in flight, so N tickets pass it together. In the sequential loop each claim re-read the gate, but after a delay of up to 30s, not at launch time either. | `claimhold.go:13-40` |
| Base derivation (`ResolveBase`, `RootRefs`, `MergeTargetTip`, `EpicRefs`) | safe | Pure plus read-only git; the two caches are mutex-guarded. Each call costs 2 to 3 git processes. | `baseref.go:19-46,97-101`, `server/merge/epicmerge.go:221-232` |
| `git worktree add` for different branches and paths, no lock | needs a change (keep the lock) | About 120 unlocked adds on the bare clone: 1 failed, `fatal: failed to read worktrees/w10/commondir: Result too large`, at 6 concurrent adds and load 33. Locked: about 100 adds, 0 failures. Unlocked 6 adds took 0.5s wall; locked they take about 2s. | `lifecycle.go:146`, `worktree/worktree.go:12` |
| Feature worktree create for one epic (two tickets of the same epic, first launch) | unsafe without the lock | 3 concurrent `git worktree add -b <epic>` on one path: 8 of 8 tries had one winner and two `fatal: cannot lock ref ... reference already exists`. The lock plus the `os.Stat` check in `addWorktree` turns the second into a no-op. | `ralphloop/deps/deps.go:575-589`, `lifecycle.go:146` |
| Epic rebase under the lock (commit `0a3354cf4`) | safe, slow when it runs | It holds the lock for the rebase; the other launches of every epic wait. Not measured. | `feature.go:158-182` |
| Herdr tab creation | needs a lock (herdr runner only) | `EnsureWorkspace` is find, then create, with no lock. Two same-epic launches can create two workspaces. Read from code, not run: the active runner is `pty` (launch timeline doc). `TabCreate` and `AgentStart` take no shared state beyond the herdr server. | `herdr/workspace.go:54-71`, `herdrrunner/herdrrunner.go:109-115` |
| Pty runner `Start` | safe | Per-label reservation (`reserve`/`release`), the mutex is not held during the launch, each label has its own dir. Same label twice returns `ErrLabelTaken`. Run 6 and 10 at once, 0 errors. | `nativerunner/pty.go:111-159`, `pty_registry.go:17-33` |
| Pty ready wait (`awaitReady`) | needs a change (the ready fix) | Not unsafe, but each waiting launch globs `~/.claude/projects/*` every 100ms for 30s. See the cost table. | `pty_launch.go:107-136` |
| pnpm install into a shared store | safe, but contended | pnpm locks its own store. Cold store or cold policy cache makes N concurrent installs no faster than serial. See the cost table. | measured below |
| Server stop | needs a change | `claimDone` waits only for `KeepClaiming`, which today returns after the launch finishes. Launch goroutines need their own wait group (the `GoSettle` pattern), and the stop path already cancels a launch prompt wait through `Watches.Context` (`launch.go:140`, `sessionwatch.go:52`). | `server/server.go:555`, `runs/guards.go` (`GoSettle`, `WaitSettled`) |
| `ClaimNext` as a test seam | needs a change | "Tests call it to force a pass" and expect the launch to be done on return. A split needs a helper that waits for the launches in flight. | `claim.go:51-55` |
| Other caller of `ClaimAndLaunch` | needs a decision | The ticketwrite verbs call it from a request path and wait for it (`ticketwrite/verbs.go:118`). Keep a synchronous form or return after the reservation. | `server/ticketwritewire.go:71` |
| A failed `PutIfAbsent` after the claim write | existing flaw | The ticket stays `claimed` with no run. Not caused by parallel launches, but a second claim of the same ticket in flight makes it likelier. | `run.go:94-96` |

## Measured cost of N launches

Warm pnpm store and policy cache, `worktree.Lock` on (mutex around the add). "Wall" is claim of the first to the last launch returning from `Pty.Start` (claude idle). Three rounds each for 1 to 6, two for 10. Load is the 1-minute `uptime` value at the start of the round; much of the rise is the benchmark itself.

| Launches | Wall | Launch CPU (git + pnpm) | Total CPU after stop | Avg cores | Peak RSS | Load at start | Free mem |
|---|---|---|---|---|---|---|---|
| 1 | 0.86, 0.88, 0.90s | 0.8 to 0.9 | 1.9 to 2.0 | 2.2 | 201 to 384 MB | 8.8 to 9.1 | 73 to 74% |
| 2 | 1.28, 1.36, 1.40s | 1.8 to 2.0 | 4.2 to 4.4 | 3.0 to 3.4 | 564 to 752 MB | 8.2 to 9.4 | 74% |
| 4 | 2.31, 2.35, 2.47s | 3.9 | 10.3 to 11.5 | 4.2 to 4.9 | 1.30 to 1.64 GB | 10.6 to 19.0 | 73% |
| 6 | 3.44, 3.48, 3.74s | 6.0 to 6.2 | 18.6 to 19.8 | 5.1 to 5.8 | 1.98 to 2.05 GB | 21.6 to 23.5 | 71 to 73% |
| 10 | 5.90, 7.29s | 10.8, 11.3 | 41.8, 44.9 | 6.2, 7.1 | 3.64, 4.01 GB | 13.4, 21.5 | 69 to 71% |

Same, lock off (git adds run in parallel; one add of the 118 failed, see above):

| Launches | Wall | Total CPU | Peak RSS | Load at start |
|---|---|---|---|---|
| 1 | 0.90, 0.91s | 2.1 | 380 MB | 35.2, 33.4 |
| 6 | 2.73, 3.09s | 16.0, 19.9 | 1.83, 2.13 GB | 33.4, 32.2 |
| 10 | 4.62, 5.24s | 40.6, 42.7 | 3.59, 3.60 GB | 24.4, 21.8 |

Read these with the load: the 1-launch rows with the lock off ran at load 35 and still took 0.9s. Wall time is mostly the lock queue (6 launches x about 0.4s) plus the load. Per launch: about 1 CPU-s for git and a warm install, about 2.2 CPU-s and 330 MB for starting claude to idle, so about 3.2 CPU-s. For comparison, 10 launches is about 42 CPU-s, 3.5 s of the whole 12-core machine.

Pieces measured alone (load 24 to 34 for the first two, 5 to 9 for the last):

| Piece | 1 | 2 | 4 | 6 | Notes |
|---|---|---|---|---|---|
| `Pty.Start` only (holder + claude to idle) | 0.32 to 0.43s, 1.1 to 1.3 CPU-s | 0.35 to 0.43s, 2.6 to 2.8 | 0.77s, 6.3 to 7.0 | 0.89 to 1.01s, 11.5 to 14.1 | peak RSS 238 MB, 578, 1.5 GB, 2.1 GB |
| `git add` + warm `pnpm install`, lock on | 0.6s, 0.9 CPU-s | 1.0 to 1.2s, 1.8 to 2.1 | 1.75 to 1.94s, 3.6 to 3.9 | 2.5 to 4.4s, 5.5 to 6.6 | RSS 83 to 145 MB |
| `git add` only, lock off | | | | 0.47 to 0.57s, 2.6 CPU-s | 12 adds: 1.1 to 1.8s, 7.7 to 8.0 CPU-s |

The 6719% children CPU on 2026-11-03 (`gx:follow-ups/215`) cannot be rechecked, the evidence dir `~/.local/state/gx/cpu-spikes/20261103-161125-server-93902` is gone. A first sample is a 1-second `ps`-style delta, and a launch wave can plausibly reach it: 6 launches burn 19 CPU-s in 3.5s, 12 to 14 CPU-s of it in the first second of the claude starts. I could not reproduce a figure near 67 cores; the highest sum of `ps pcpu` over the process tree was 689% for 10 launches with the lock off.

### Cold installs under concurrency

Fresh shared store or cache per round, one round each (6 launches twice), load 5 to 12.

| Case | 1 | 2 | 4 | 6 | CPU-s |
|---|---|---|---|---|---|
| Cold store and cold cache | 5.3s | 8.3s | 23.3s | 19.1s, 21.9s | 9.1, 12.7, 21.7, 28.2, 25.6 |
| Warm store, cold policy cache (`lockfile-verified.jsonl`) | 4.7s | | | 13.0s, 24.9s | 2.8, 13.0, 12.9 |
| Warm store and cache | 0.3s | | | 0.3 to 0.5s each | |
| One cold install first, then 5 warm ones together | 4.3s + 2.5s = 6.8s | | | | for 6 launches |

So N concurrent cold installs take about as long as N serial ones (4 x 5.3s = 21s; observed 23s). A cold policy cache makes a wave of 6 miss 15s even though the store is warm. This is the time the 15s target loses on a cold start, and a cap on launches in flight does not help it. A single-flight rule per lockfile hash (the first install runs, the siblings wait, then run warm) does: 6.8s against 19 to 25s.

### The ready wait polled by N launches

`awaitReady` calls `transcript.FindByID` (a `filepath.Glob` over `~/.claude/projects/*`, 3,418 dirs) every 100ms for up to 30s (`pty_launch.go:107-136`, `transcript/transcript.go:59-71`). I ran that miss loop with k goroutines for 5s each (load 10 to 16):

| Waiting launches | Mean call | CPU | Share of a core | Over 30s |
|---|---|---|---|---|
| 1 | 46 ms | 1.67 CPU-s | 32% | about 10 CPU-s |
| 2 | 68 ms | 4.10 | 81% | about 25 |
| 4 | 76 ms | 8.50 | 166% | about 51 |
| 6 | 93 ms | 13.54 | 269% | about 81 |

This is the largest CPU cost of a parallel wave today, and it grows faster than the launch count because each glob slows down under the others. It is the same cost `gx:follow-ups/215` describes. It disappears when the ready check stops waiting for a transcript. The input-box half of the check also reads the holder screen each poll; I did not measure that part.

## Does a launch in flight need its own cap?

No, not for CPU or memory on this machine, and the slot limit already counts launching runs.

- The slot limit counts `launching` runs, so launches in flight are already capped by `max-agents` (config here is 10) and by the per-root cap (5). The cap needs the reservation described above, nothing more.
- A wave of 10 warm launches costs about 42 CPU-s, 3.6 to 4.0 GB RSS and 6 to 7s wall with the lock on, at a load of 13 to 21. Free memory stayed at 69 to 71%. That is inside the 15s target.
- A separate cap would be worth adding only if installs are not made single-flight, because cold installs are slow whether 1 or 6 run at once, and a lower cap only serializes them (4 x 5s).
- Three costs scale with N and are outside any cap: the `worktree.Lock` queue (about 0.35s per launch), the repeated `Index.Rescan` (0.12 CPU-s each), and the ready wait polling (until the ready fix lands).
- Not measured: a wave while 10 agents already run (the steady-state cost of a live agent is the `ps` poll of follow-ups/215, about 11% of a core each), a cold claude after an update, the trust dialog, and the herdr runner.

## Options for the launch split (the person decides in a grilling ticket)

A. Reserve in the loop, launch in a goroutine. `ClaimAndLaunch` splits at the registry write: the loop does steps 4 to 15 (claim write, rescan, model, base, `PutIfAbsent`), releases `AdoptMu`, and starts a goroutine for step 16 onward, counted by a wait group so a stop waits for it. The loop returns in about 0.1s per ticket (git base calls plus the rescan) instead of 1 to 30s.
   Cost: moves the registry write earlier, which changes the failure rollback (a failed launch already does `DeleteStarted` and a pause, `run.go:101-116`), and every test that calls `ClaimNext` needs a wait helper.
B. One goroutine per ticket of a pass, wait for all at the end of `ClaimNext`. Smallest change, keeps the registry write where it is, but then the cap checks still need the reservation, and the loop (trunk checks, slot release, banner sync) still blocks for the slowest launch.
C. A launch worker pool sized by the slot limit. More code than A, no gain, because the slot limit already bounds launches.

Recommendation: A. It is the only one that frees the loop, and the reservation it needs is what the caps need anyway. Then, in the same change set or the next: shrink `worktree.Lock` to the git admin work (the add and the feature create), move the status and ancestor checks and the rebase out where they can run per epic, coalesce `Index.Rescan`, and make the deps install single-flight per lockfile hash (the first install runs, siblings wait and run warm). Keep `worktree.Lock` for the adds, since the unlocked run failed once.

## Open points for the map

- The leak in my own harness, for the record: during the first rounds some bench worktrees and branches (`b3`, `b10` to `b17`, worktrees `w1` to `w17` under the research worktree) were created in the real `.bare` repo instead of the temporary clone. I cleaned them up (tips equalled `origin/main`, nothing else touched) and could not reproduce it in later rounds or find the cause. If a reader of the git race finding above worries it was a real-repo effect, the one failing add (`w14`, error read of `worktrees/w10`) ran against the temporary clone.
- The 15s target and parallel launches: with the ready fix and a warm store, the wave costs 3.5s at 6 launches and 6 to 7s at 10, so the target holds. With a cold policy cache or store it does not, whatever the launch parallelism. That is a map-level fact for the deps rule ticket.
- `tickets.LoadResolved` inside `MergeTarget` (`baseref.go:85-95`) loads a project's epics on every feature-mode claim. It is a candidate for the claim-to-worktree delay ticket 14 is chasing; I did not time it.
