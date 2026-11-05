# Launch timeline: where a queue launch spends its time

Ticket: `gx:fast-queue-startup/01`. Question: how long does each step of a queue launch take today, which steps have no marker, and is 15s (claim to launch prompt delivered, deps unchanged) reachable.

## Short answer

- A launch today takes a median of 97s in `gx:split-server-pkg-impl` (315 single-launch tickets, claim to prompt delivered, p90 151s, max 601s). Two steps own it: the deps install (median 66s, bimodal) and the 30s ready wait (30.8s from deps done to prompt delivered, every time).
- When the install is fast (median 2.2s over 178 launches), a launch is about 33s and the ready wait is 91% of it. Removing the wait leaves about 3s. The 15s target looks reachable if the deps install is skipped or is as fast as in those 178 launches, and the ready check stops waiting for a transcript.
- Most of the ten steps in the ticket have no marker. Only `claimed_at`, `deps-installed` and `iteration-started` can be timed from stored data.

## Sources and method

- Run logs: `~/.local/share/gx/tickets/gx/*/run-log.jsonl` (writer: `ralphloop/runlog`). 38 epics. I used events from 2026-10-29 on.
- Ticket files: `claimed_at` in the front matter of `.../issues/<id>-*.md`. It is stamped by `ralphloop/ticketops/claim.go:38` (`ClaimWithBase`), the first write of `ClaimAndLaunch` (`server/sched/run.go:52`).
- Server log: `~/.local/state/gx/server.log`. It holds only 22:04 to 23:51 on 2026-11-05 (7 server starts), so it cannot give a week of data.
- Code: `server/sched/claim.go`, `server/sched/run.go`, `server/sched/launch.go`, `ralphloop/lifecycle.go`, `ralphloop/install.go`, `nativerunner/pty.go`, `nativerunner/pty_launch.go`.
- Pairing: for each `iteration-started` event I took the latest earlier `deps-installed` of the same ticket. Claim to deps uses `claimed_at`, and only for tickets with one `iteration-started` in the log and a gap under 600s, because `claimed_at` holds only the latest claim.
- Scripts are in the session scratchpad, not committed.

## What each marker means

- `deps-installed` is appended by `installDepsIn` (`ralphloop/install.go:23`) after the worktree exists and the install finished.
- `iteration-started` is appended by `Launcher.launch` (`server/sched/launch.go:190`) after `supervise.StartAndPrompt` returned. That call returns after the holder is up, claude is idle, the ready wait ended, the prompt was submitted, the delivery proof passed and a turn started (`nativerunner/pty_launch.go:64-96`). So `iteration-started` is "launch prompt delivered", exactly the end of the target.
- So the two durations the logs give are claim to `deps-installed` (steps 1 to 4) and `deps-installed` to `iteration-started` (steps 5 to 10).

## Step table

Times are from run logs unless marked live. "Split" is `gx:split-server-pkg-impl`, launches since the ready check reached the server (2026-11-02 08:00 on).

| # | Step | Marker | Time |
|---|------|--------|------|
| 1 | claim written | ticket `claimed_at` (`ticketops/claim.go:38`); wire event `ticket-claimed` is not stored | start of the clock |
| 2 | base resolved | none (`Host.ResolveBase` runs before the claim, `run.go:39`; `resolved_base` has no time) | not measurable |
| 3 | worktree created | none | `git worktree add` of gx, live: 0.27 to 0.31s (5 runs, load 10.2). Claim to deps-installed was 1.3 to 3.5s when the install was fast, so steps 1 to 3 plus a warm install fit in about 1.5s. |
| 4 | deps installed | run-log `deps-installed` | claim to this event: split median 66.5s, p90 120s, max 570s. Over all 470 launches since 11-02: median 40.8s. Bimodal: 178 launches median 2.2s (max 8.8s), 292 launches median 71.8s (min 25s). |
| 5 | herdr tab opened | not a step: the runner is `pty` (`server.log`: `agent-runner selected ... "runner":"pty"`) | n/a |
| 5' | holder spawned, claude started | none (`ptyLauncher.spawn`, `RecordLaunch` write `meta.json`, no log) | not separable |
| 6 | agent process idle (input box seen) | none on the server. `Pty.Start` returns when the holder says claude is ready (`pty_launch.go:236-275`) | live, idle `claude --session-id`: 0.18, 0.19, 0.21, 0.47s (load 6 to 10) |
| 7 | ready wait ended | warning `ready signal missing, sending the launch prompt anyway` with `waited=30000000000`. Nothing on success. | 30.0s every time (cap `DefaultReadyWait`, `pty_launch.go:101`) |
| 8 | launch prompt sent | none | not separable |
| 9 | delivery proven | none by itself; `iteration-started` follows it | not separable |
| 10 | launch finished | run-log `iteration-started` | deps-installed to this event: median 30.9s, p90 31.6s, min 30.5s, max 66.3s (470 launches) |

Steps 5' to 9 together: deps-installed to iteration-started minus the 30.0s wait leaves 0.5 to 1.6s (p10 to p90). That covers holder spawn, claude idle, submit and the proof poll (`DefaultLaunchProofCaps.Poll` is 500ms, `claudeui/launchproof.go:51-56`).

## Share of total launch time

Split, median per phase, since 2026-11-02 (n=315): claim to deps 66.5s, deps to prompt delivered 30.8s, total 97.3s.

| Phase | Median | Share |
|-------|--------|-------|
| claim, base, worktree, deps install | 66.5s | 68% |
| holder start, ready wait, prompt, proof | 30.8s | 32% |
| of which the ready wait | 30.0s | 31% |
| of which everything else | about 0.8s | 1% |

When the install is fast (178 launches, median 2.2s; the same shape as `gx:slow-tests-impl`, 123 launches, median 2.2s, and the first 33 tickets of split on 11-04):

| Phase | Median | Share |
|-------|--------|-------|
| claim, base, worktree, warm install | 2.2s | 7% |
| ready wait | 30.0s | 91% |
| holder start, prompt, proof | about 0.9s | 3% |
| total | about 33.1s | |

Last 14 launches in split (`iteration-started` 19:47 to 22:52 on 11-04 and 11-05): claim to deps 25 to 133s (two rows excluded because `claimed_at` was reset), deps to delivered 30.7 to 66.3s. Tickets 203, 204 and 205 launched in one burst at 22:16 to 22:19 and took 35.8s, 40.4s and 66.3s after the install; the log does not say why.

## Findings

### 1. The ready wait is a flat 30s, and it began on 2026-11-02

- Before commit `6b7dbd911` (2026-11-02 01:02, `agent-lifecycle-impl/22`) reached the running server, deps-installed to delivered was 4.5 to 7s per hour (median about 4.8s over 10-29 to 11-01). Hourly medians jump to 31.1 to 33.7s from 11-02 08:00 and stay at 30.7 to 31.0s through 11-05.
- 1,501 launches since 10-29: 952 took under 29s, all of them before the change.
- I could not tell which runner produced the 4.8s numbers (herdr or pty) or why 11-01 19:00 to 21:00 ran at 0.6 to 2.4s. The run log has no runner field.
- `awaitReady` needs `len(files) == 1 && InputBox(screen)` (`pty_launch.go:118`). The transcript file is the failing half: an idle claude 2.1.318 shows its input box at 0.2 to 0.5s and still has no `.jsonl` 21s later. I ran this 4 times (load 9.9, 8.0, 6.2, 6.3): input box at 0.21s, 0.18s, 0.47s, 0.19s, no transcript in 25s, 21s, 21s, 21s.
- The cost of the glob: `FindByID` is `filepath.Glob(~/.claude/projects/*/<id>.jsonl)` (`transcript/transcript.go:65`). With 3,415 project dirs a glob took 11ms (Python, live, load 5.7). `awaitReady` repeats it every poll until the deadline. That is small next to the 30s, but it runs for the whole 30s.

### 2. The warning is not a reliable marker

- `server.log` has 9 `ready signal missing` warnings. All are after 23:34 on 11-05 (server restarted at 23:29).
- The log window holds 18 launches (`iteration-started` since 22:04). The 9 launches between 22:11 and 22:52 also took 30.7 to 31.2s from deps to delivered but have no warning. The code only warns when `p.log != nil` (`pty_launch.go:122`), and I did not establish why the log was missing for them.
- On success `awaitReady` logs nothing, so a fast ready is invisible.

### 3. Launches are serial, and launch N+1 starts about 1s after launch N is delivered

- `ClaimNext` loops `claimRootAfterAdoption` and calls `ClaimAndLaunch` inline (`server/sched/claim.go:56-80`), inside the `Serve` loop, which also runs `Projects.Check`, `CheckTrunks`, `releaseWaitingSlots`, `PublishChanges` and `SyncBanner` (`claim.go:35-42`). One 30s launch holds all of those.
- Run-log evidence, split on 11-04: `iteration-started` 02 at 12:19:45, `deps-installed` 03 at 12:19:46; 03 started 12:20:17, 04 deps at 12:20:19; the same pattern for 05, 06, 07, 11, 12 and 17. Each launch is about 32s and the next begins within 1 to 3s.
- Only the git worktree add is under a process lock (`worktree.Lock`, `ralphloop/worktree/worktree.go:12`, taken in `addIterationWorktrees`, `lifecycle.go:146`). The install runs outside it (`ralphloop/install.go:12-14`), so the lock does not stop parallel installs.

### 4. The deps install changed from about 1.5s to 25 to 150s on 2026-11-04 evening

- In split, claim to deps-installed was 1.3 to 3.5s for the first 33 tickets (11-04 12:01 to 16:04) and 25 to 157s from 17:59 on. Every row has the same command, `webui: pnpm install --frozen-lockfile` (`deps-installed` `reason`).
- `ralphloop/deps/install.go` runs the install on every worktree that has a tracked lockfile; there is no skip rule (`installDependenciesWithin`, line 63).
- I did not measure why. That is `gx:fast-queue-startup/02`. What this ticket adds: the same command took 1.5s warm and 60 to 150s later in one epic, so the cause is the store or machine load, not the command.
- Other epics see 2.2s medians since 11-02 (slow-tests-impl, agent-lifecycle-impl at 3.1s), but `better-epic-header-impl` (4 launches) had 47s and `follow-ups-2026-11-04` (3) had 70s.

### 5. Steps with no marker

1. Time from ticket becoming ready to the claim write (queue poll wait, base resolution, `LaunchModel`, budget mark). Only `ClaimNext`'s tick drives it; no log.
2. Base resolved (step 2). No time.
3. Worktree created. No event; feature worktree add, `RevParse` and iteration worktree add all sit inside the claim to `deps-installed` span.
4. Holder spawned and claude started. No event.
5. Claude idle (input box seen). No server-side event. The holder knows it (`StreamStatus` `Ready`).
6. Ready wait ended on success. No log. Only the timeout logs.
7. Launch prompt submitted. No event.
8. Delivery proof passed and turn started. No event; `iteration-started` follows it.
9. The `ticket-claimed` wire event exists (`run.go:56`) but is not stored.

The three stored timestamps (`claimed_at`, `deps-installed`, `iteration-started`) are the only ones that survive a restart. They bound two spans, not ten steps.

## Is 15s reachable?

Yes, with two changes, and no without the first one.

| Component | Today, median | Needed |
|-----------|---------------|--------|
| claim, base, worktree | about 1.5s | about 1.5s |
| deps install | 66s (current), 2.2s (fast epics) | skipped when deps match: 0s |
| holder start to idle | about 0.5s | about 0.5s |
| ready wait | 30.0s | the input box alone: 0.2 to 0.5s |
| submit and proof | about 0.5 to 1s | the same |

- With the install skipped and the ready check fixed: about 1.5 + 0.5 + 0.5 + 1 = 3.5s, plus margin for load. That is well under 15s. The pre-change launches at 4.8s median show the same floor from real data.
- With the install as it stands (66s median) the target fails whatever the ready check does. With a fast install (2.2s) it holds.
- With only the install fixed, the 30s wait alone breaks 15s.
- Risks I did not measure: a claude cold start after a CLI update, the trust dialog (the launcher presses a key for it, `pty_launch.go:256-270`), and launch under heavy load. Load average during my measurements was 5.7 to 10.2 with another agent timing `pnpm install`; input box times were still under 0.5s.

## Recommendations (for the later grilling tickets)

1. Treat the ready check as a fix, not a tuning: drop the transcript half of `awaitReady` and use the input box plus the holder's ready status. The delivery proof and resend loop already catch a prompt that is lost (`claudeui.CheckLaunchDelivered`). Cost: one less guard against a claude that shows its box too early; the proof backstops it. Ticket 03 covers what Claude gives before the first prompt.
2. Record launch time from stored events, not from the log. Add per-step events (claim, worktree, deps, spawn, idle, ready, submitted, delivered) so the span can be broken down after a restart. The `ready signal missing` warning disappears with recommendation 1 and cannot be the marker.
3. Fix the deps cost at the source (skip rule, ticket 02/07). Do not reduce the install timeout or run it in the background without the skip rule: the 66s is paid inside the launch.
4. Launch in parallel (tickets 04/08). The serial 30s wait is what cut 4 ready tickets to 2 running agents; once the wait is gone the serial cost is about 3.5s per launch, so parallel launching matters most when installs are not skipped.

## Out of this ticket's scope but noticed

- The server log keeps about 2 hours. A weekly view of the warning needs a longer log or a stored event.
- The launch holds the `Serve` loop for its whole duration (finding 3). With the 30s wait gone this shrinks, but a slow install still blocks trunk checks and banner sync. Ticket 04 should list what else the loop does.
