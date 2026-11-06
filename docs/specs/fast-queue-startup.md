# Fast queue startup

## Problem Statement

A person queues an epic and expects its ready tickets to start working together. Today they do not. In `split-server-pkg-impl` the queue ran 2 agents while 4 tickets were ready. Launches run one after another, and a measured launch (claim to launch prompt delivered) took a median of 97s (p90 151s) over 315 launches. Three causes sit on the launch path:

- The ready check waits a full 30s on every Claude launch. It needs the session transcript on disk, but Claude 2.1.318 writes the transcript only after the first prompt, so the signal never arrives.
- After the claim write, the launch runs a full index rescan. A rescan runs the archive sweep, which waits 10s on a stale allocation lock for each of two stuck one-offs, so each launch carried a 20s multiple of stall. The lock file was a leftover from a dead process, and the lock code has no way to tell.
- Launches are serial, so every delay is paid once per ticket.

The person also cannot see any of this. A launching ticket looks like a claimed or running one, an epic held by its agent cap shows nothing on the Queue row, the epic page or the TUI, and nothing records or reports launch time, so a regression would go unnoticed.

## Solution

A launch takes under 15s from claim to launch prompt delivered, and N ready tickets launch together when N slots are free.

- The ready check stops waiting for a transcript. The launch prompt waits only for the agent's own ready signal, then the empty-input check, then delivery. The lost-prompt protection (delivery proof and resend) is unchanged.
- The launch no longer runs the rescan steps; it refreshes the index only. The allocation lock dies with its holder, and a failing archive backs off.
- The claim loop stays serial and short; each launch runs in its own tracked goroutine.
- Every launch runs its package manager's normal frozen install in its own new worktree, with one in-flight install per lockfile hash shared by concurrent launches.
- Every launch records its step timings. The Stats page and `gx server stats` show launch time, and `gx-evolve` flags launches over 15s.
- A launching ticket shows "Launching: <step> (n/4)" on the web Queue row, ticket page, epic page and the TUI Queue row. A ticket held by a cap shows "Waiting for a slot: ..." in the same places.

## User Stories

1. As a person who queued an epic, I want a new queue agent to have its launch prompt delivered within 15s of its claim, so that tickets start working while I am still looking at the queue.
2. As a person with N free agent slots and N ready tickets, I want all N to launch together, including N launches into one new herdr workspace (which is created once), so that the queue does not run fewer agents than it is allowed.
3. As a person, I want a launch not to wait 30s for a session transcript that Claude has not written yet, so that the common case is not slowed by a signal that never comes.
4. As a person, I want a launch whose agent never signals ready to fail and pause the ticket at once as `iteration-error`, ended by the pause card's existing Retry button, so that a broken start is visible and not papered over by sending the prompt anyway.
5. As a person, I want the lost-prompt protection to keep working (delivery proof and resend), so that a faster launch does not lose launch prompts.
6. As a person, I want the transcript lookup to be cheap (a direct path check, with the all-directories glob only as a once-a-second fallback), so that launching several agents does not burn CPU polling.
7. As a person using a custom `CLAUDE_CONFIG_DIR`, I want the transcript lookup to honor it, so that the delivery proof finds the transcript.
8. As a person, I want a claim not to wait on the archive sweep or other rescan steps, so that one stuck one-off cannot slow every launch.
9. As a person, I want a claimed ticket to appear in the index at once after its claim write, so that the queue views and caps are right while the launch runs.
10. As a person, I want the allocation lock to be released by the kernel when its holder dies, and a leftover old lock file to count as free, so that a crashed process cannot stall the archive sweep again.
11. As a person, I want the one-offs archive to back off per failing ticket (1 minute up to a 1 hour cap, no retry limit) and log only the first failure at warn level, so that a stuck ticket does not hammer the lock or flood the log, and is still retried later.
12. As a person, I want concurrent index scans to coalesce into one in-flight scan plus one trailing re-run, so that a late older scan can never overwrite a newer one.
13. As a person, I want the index refresh to hash the bytes it already parsed and to log its duration and caller, so that a refresh costs about 15ms and a slow one can be diagnosed on a running server.
14. As a person, I want each launch's slot reserved in the claim loop before the next cap check, so that the per-epic, project, one-off and global caps stay right while launches run in parallel.
15. As a person, I want a failed launch to be handled in its own goroutine (registry entry removed, ticket paused with the matching kind, `iteration-launch-failed` published) and the claim loop kicked, so that the freed slot is refilled at once.
16. As a person stopping the server, I want every launch in flight cancelled and rolled back (a started agent session stopped and its tab closed, then the worktree discarded, then the claim handed back to `open`), so that nothing is left half-made or leaked and a stop never waits on a 5 minute install.
17. As a person, I want every package manager gx supports (npm, pnpm, yarn, bun) to run its normal frozen install in each new worktree, so that one rule covers every project and no stale `node_modules` is ever reused.
18. As a person launching several tickets at once, I want launches with the same lockfile key to share one in-flight install (the first installs, the rest wait and then install on the warm store), so that a cold store is not hit by 6 installs at once.
19. As a person, I want launches with a different key not to wait on each other, and a failed first install to hand over to the next waiting launch, so that one project's install never blocks another or strands its waiters.
20. As a person, I want the install lock not to hold the worktree lock, so that other launches can still add worktrees while an install runs.
21. As a person, I want a failed install to fail the launch, discard the worktree and pause the ticket at once as `iteration-error` like any launch failure, with the existing 5 minute timeout, so that install failure behaves as it does today.
22. As a person with an epic queued ahead of another, I want the first queued epic to fill its slots before the next epic gets a turn, so that queue order stays my priority list.
23. As a person, I want an epic at its per-epic cap skipped so that the next epic gets the remaining slots, and I want a reorder never to take back a claimed ticket, so that a launch in flight keeps its slot.
24. As a person, I want a launching ticket to show "Launching: <step> (n/4)" on the web Queue row, the web ticket page, the web epic page and the TUI Queue row, so that I can tell a launching ticket from a running one.
25. As a person, I want the four labels to be "creating worktree (1/4)", "installing dependencies (2/4)", "starting agent (3/4)" and "sending prompt (4/4)", matching the four launch steps, so that the label tells me where the time goes.
26. As a person, I want the existing elapsed timer from `claimed_at` to show next to the label and the agent's own timer to take over once the agent starts, so that I see how long a launch has taken.
27. As a person, I want each step change to update the views without a reload, so that the label advances while I watch.
28. As a person, I want no new ticket status for launching, so that status filters, dependents and the schema are unchanged.
29. As a person whose launch fails, I want the ticket to pause as `iteration-error` with the existing pause reason showing the launch error and the Launching label to go away, so that a failure reads like every other pause.
30. As a person who restarts the server mid-launch, I want a reclaimed run with no agent to reopen the ticket and launch again as a new launch, and a live agent to continue as running, so that a restart leaves no stuck claim.
31. As a person, I want an epic held by its agent cap to show "Waiting for a slot: epic cap 3/2, global cap 4/4" (every full cap as count/limit) on the Queue row, epic page, ticket page and TUI, so that I know why nothing is starting.
32. As a person who lowered a cap below the live count, I want the line to add "(over cap, no new starts until one ends)", so that I know running agents are not being stopped but no new ones start.
33. As a person, I want other holds (disk low, harness missing, usage gate, Herdr down, waiting in queue) to keep their current verdicts, so that this change does not rewrite them.
34. As a person, I want the Launching line and the Waiting for a slot line to share one line slot on each view, so that a ticket shows one or the other.
35. As a person, I want design runs to stay outside every agent cap, the Queue views, slot counts and launch timings, so that opening a design by hand is never held and never skews launch time.
36. As a person, I want a finished design session to leave no live design agent (pane or process), so that a design agent that holds no slot is never leaked.
37. As a person, I want every launch that reaches prompt delivered to write one `launch-timed` run-log event with its claim time, total, four step durations and path (cold-start or resumed), so that launch time is recorded for every launch.
38. As a person, I want a launch that waits for a shared install to count the wait in its deps step, so that the step shows what the launch really waited for.
39. As a person, I want a failed launch to keep its `launch-failed` event and get no timing, and a launch cut off by a restart to get none either (its reclaim is a new launch with its own timing), so that timings describe only complete launches.
40. As a person on the Stats page, I want a "Launch" section with the range median and the share of launches under 15s as headlines, a per-day median chart and the median per step, counting launches of finished tickets only (`gx-evolve` reads the run log and flags slow current launches, those over 15s), so that I can see whether the queue stays fast.
41. As a person, I want each launch counted on its own day (a reclaim is a second launch), so that the chart matches when launches ran.
42. As a person running `gx server stats`, I want text mode to print the launch median and the share under 15s and `--json` to carry the full launch data, so that I can read launch time from the terminal.
43. As a person running `gx-evolve`, I want `gx evolve inputs` to print a "Launch rows" block beside the check rows, with one row per launch over 15s (ticket, total, slowest step) and one row when the epic's median launch is over 15s, so that slow launches are reviewed.
44. As a person running `gx-evolve`, I want launch rows to be findings with category `idea` and no deep read, and a `slow-launch` flag of severity `rest`, so that they spend none of the deep-read cap.
45. As a person, I want the existing `### Check rows`, `slower:` and `flake:` anchors kept, so that the skill's checks and the anchor test still pass.
46. As a person upgrading gx, I want a migration step to register the `launch` stamp field and backfill nothing, so that old tickets load and old launches (which lack step data and carry the old stall) are not mis-reported.
47. As an implementer, I want the test hooks for the old ready wait removed (`GX_PTY_READY_WAIT`, the fixture's ready delay and wait knobs, the test that waits out the old wait), so that no test depends on a behavior that no longer exists.
48. As a maintainer reading the glossary, I want the terms **Launch time** and **Launch step**, and a line that a Design session takes no slot, so that the vocabulary matches the code.

## Implementation Decisions

Vocabulary: **Launch** is the glossary term (claim the ticket, create the worktree, open the herdr tab, start the agent). The UI state is "Launching", not "Claiming". New glossary terms: **Launch time** (claim to launch prompt delivered) and **Launch step**. `GLOSSARY.md` also gets one line: a Design session takes no slot (doc change only).

**Target.** Under 15s from claim to launch prompt delivered, for every launch. It is a code constant, not a config key. Measured: a clean launch is about 3.5s once the transcript wait is gone and the rescan stall is fixed. The old 97s median included the 30s ready wait and the 20s-multiple rescan stall.

**Ready check (Claude only, as today).**
- Delete `awaitReady`, `DefaultReadyWait` and `Pty.ReadyWait`. The launch prompt waits for `Pty.Start`'s ready signal (the earliest of the pid file going idle, the OSC idle mark and the input box, all within 0.9s), then `awaitEmptyInput`, then `Submit`.
- A missing ready signal is not worked around. `Start` fails the launch when its start timeout passes, and the ticket pauses at once as `iteration-error` (as a `Start` failure does today; only an undelivered prompt gets a second session). The pause card's existing Retry button ends the pause. There is no send-anyway fallback.
- Transcript lookup: `os.Stat` of `<projects dir>/<slug(EvalSymlinks(cwd))>/<id>.jsonl`, polled at 100ms, with the all-directories glob (`transcript.FindByID`) as a fallback at most once a second. It honors `CLAUDE_CONFIG_DIR` and serves `FirstUserRecord`, the delivery proof's reader.
- Slug rule, copied from Claude: every non-alphanumeric char becomes `-`, applied to the symlink-resolved cwd; a path over 200 chars is cut to 200 plus `-` plus a base-36 string hash. A test checks the rule against the real project dirs on the machine; the glob fallback covers a mismatch.
- Codex and non-slash prompts keep the `PromptNudged` path.
- The lost-prompt protection from `gx:agent-lifecycle` (delivery proof and resend) is unchanged. `editorMode: normal` handles the stray `i`; the proof catches the unknown-skill race.
- Remove the `GX_PTY_READY_WAIT` env hook, the fixture's `readyDelay` and `readyWait` knobs, and the test that waits out the old wait. The test fake stops touching the transcript at ready.

**Parallel launches.**
- The claim loop stays serial. Per ticket: resolve the base and write the claim under `Guards.AdoptMu`, which covers only those two. Then, still in the serial loop and without `AdoptMu`, call `Index.Refresh`, publish `ticket-claimed`, and reserve the slot with `Registry.PutIfAbsent` (`launching`).
- Everything after the reservation (model choice, worktree, deps install, agent start, launch prompt) runs in one tracked goroutine per ticket.
- `Registry.Count()` counts `launching` runs, so the reservation keeps the per-root, project and global caps right. There is no separate cap on launches in flight (a warm wave of 6 takes about 3.5s wall and about 2 GB; cold installs are held by the single-flight install).
- A failed launch is handled in its goroutine as today (remove registry entry, pause with the matching kind such as `iteration-error` or `agent-name-taken`, publish `iteration-launch-failed`, log), then kicks the claim loop.
- Server stop cancels every launch in flight, waits for the goroutines to end and rolls back each one in this order: stop the agent session and close its tab if step 3 has begun, discard the worktree, hand the claim back to `open`.
- `worktree.Lock` stays around the worktree add and the work it covers (feature create, possible epic rebase, ancestor and status checks). Unlocked parallel adds fail.
- A launcher wait group lets a test wait for the launch goroutines, so `ClaimNext` stays usable in tests.
- Herdr workspace creation is find-then-create with no lock. The launch goroutines make it reachable in parallel on the herdr runner, so the implementer serializes it with a lock or a single-flight in the herdr runner. N parallel launches into one new workspace create it once.

**Launch path and the rescan.**
- After the claim write the launch calls only `Index.Refresh`. It never runs an `OnRescan` step (`ScanParentDefects`, `ScanLateFinishes`, the one-offs `Sweep`, `ScanCleanups`). The watcher, the `/changed` ping and the 15s poll keep running them, off the launch path.
- `Index.Refresh` is still a full refresh. Patching one ticket row is rejected, because a claim changes derived state (rendered status, epic status, dependents' blocked state).
- The loader computes the file sum from the bytes it already read and keeps it in the parse cache. This drops a warm refresh from about 51ms to 13 to 15ms and makes the sum describe the bytes that were parsed.
- Scans become single-flight with a trailing re-run, so concurrent scans coalesce and an older scan can no longer install over a newer one.
- `Refresh` logs its duration and caller (or counts them), so the numbers can be checked on a running server. The format is the implementer's choice.

**Allocation lock and archive back-off.**
- `.alloc.lock` in the ticket-move allocator becomes a `flock`, as `migratefail` and `gocachegate` do. The kernel drops it when the holder exits, and a leftover old `O_EXCL` file counts as free.
- The one-offs sweep backs off per ticket, from 1 minute up to a 1 hour cap, with no hard retry limit. It logs the first failure at warn level, then stays quiet until the next try.
- A visible warnings indicator in the web UI is out of scope (`gx:webui-warnings`).

**Deps.**
- One rule for every manager (npm, pnpm, yarn, bun): the normal frozen install in each new worktree, no skip, no reuse (no symlink, no clone of `node_modules`, no marker file). Commands stay: `pnpm install --frozen-lockfile`, `npm ci`, `yarn install --immutable` or `--frozen-lockfile`, `bun install --frozen-lockfile`. No `--offline` and no `--prefer-offline`.
- The pnpm store and cache stay where they are (same volume as the worktrees, so pnpm clones files).
- Single-flight install: the key is the directory plus a hash of its lockfile, `package.json`, workspace file and `.npmrc`, plus the manager and node versions. The first launch with a key installs; launches with the same key wait, then run their own install on the warm store; launches with a different key do not wait. If the first install fails, the next waiting launch takes over as the first. The lock lives inside the server process and does not hold `worktree.Lock`.
- A failed install fails the launch as today (discard the worktree, pause at once as `iteration-error`; the pause card's Retry button ends the pause). The 5 minute shared timeout stays.

**Fill order and caps.**
- Fill-first stays: the first queued epic fills its slots (up to `max-agents-per-epic`, the global `max-agents` and any project cap) before the next epic gets a turn. No new setting. The one-offs root keeps `max-one-off-agents` and takes its turn by queue position. An epic at its per-epic cap is skipped. A claimed ticket is never taken back by a reorder.
- Design runs stay outside every cap (no change to ADR 0032 "No bypass"). Queue views, slot counts and launch timings leave them out; the Agents tab and page keep listing them as Design agents. They get no Launching state and no launch timing. A finished design session must leave no live design agent (pane or process). This is a requirement, covered by a Go test (design session finish: pane closed, process gone), and a leak is always fixed inside this epic.

**Launch timing event.**
- One new run-log event, `launch-timed`, is written when the launch prompt is delivered. It carries the claim time, the total, the four step durations and `path` (cold-start or resumed). Exact field names are the implementer's choice.
- The four steps: (1) claim to worktree created, (2) deps (the install, or the wait for the shared in-flight install, wait included), (3) agent started (holder ready), (4) prompt delivered (send to delivery proof). There is no ready-wait step and no deps-skipped flag.
- Only a launch that reaches prompt delivered is timed. A failed launch keeps its `launch-failed` event; a launch cut off by a restart gets no timing and its reclaim is a new launch.

**Stats.**
- A `launch` stamp is written at finish beside friction, holding each launch's claim time, total and four step durations, taken from the in-memory run record kept from the launch (the Stats build reads stamps only, never run logs). Because it is written at finish, Stats counts launches of finished tickets only; `gx-evolve` reads the run log and is where current launches show. Each launch counts on its own day, bucketed by its own claim time, not by the ticket's finish day. The stamp is per ticket and per project like friction; `wire.Stats` carries it.
- The Stats page gets a "Launch" section: range median and share under 15s as headlines, a per-day median chart, and the median per step. `StatCard` has no target-line prop, so the share-under-15s headline stands in for a target line.
- `gx server stats` text mode adds the median and the share under 15s; `--json` carries the full launch data.
- A `gx tickets migrate` step registers the `launch` field and backfills nothing. Per `.ai/index.md`, add a named step in `migrations/steps.go` and update `migrations/testdata/schema.snapshot`.

**Evolve.**
- `gx evolve inputs` reads `launch-timed` from the epic's run log and prints a "Launch rows" block next to `### Check rows`, keeping the `### Check rows`, `slower:` and `flake:` anchors. One row per launch over 15s (ticket, total, slowest step) and one row when the epic's median launch is over 15s. Rows are findings with category `idea` and get no deep read.
- A `slow-launch` flag in the inputs table has severity `rest`.
- `skills/gx-evolve/SKILL.md` documents the block, and `docs/specs/gx-evolve.md` (the verb contract) is updated to match.

**Launching and Waiting-for-a-slot state.**
- No new ticket status. A new in-memory `launch_step` field on the run registry (never saved, like `RateLimited` and `Settling`) is sent as a field on the ticket info next to `claimed_at`, filled where the snapshot sets `claimed_at` (root routes, epic detail and summary). Each step change publishes an event so clients re-snapshot.
- Labels: "Launching: creating worktree (1/4)" (the claim write, the refresh and the worktree), "Launching: installing dependencies (2/4)", "Launching: starting agent (3/4)", "Launching: sending prompt (4/4)". The TUI uses the same text and a spinner.
- The existing timer from `claimed_at` shows beside the label; the agent's own timer takes over when the agent starts. The web verdict line keeps hiding the `claimed` verdict.
- The TUI gets a new "launching" row phase set in `SyncRunState`, replacing "(implementing...)" for a launching ticket.
- A failed launch has no new state; the label goes when the run leaves the registry. A restart mid-launch has no special text.
- Waiting for a slot: the scheduler's "concurrency cap" verdict is relabelled "Waiting for a slot" in display only: the wire verdict value `concurrency cap` stays, and the web and TUI map it to the label. Its reason keeps naming every full cap (epic, project, global) as count/limit. When a count is above its limit, the line adds "(over cap, no new starts until one ends)". It shows in the same four places as the Launching state, sharing its line slot: Launching while a launch runs, Waiting for a slot while a cap holds a ready ticket. For an epic, the line comes from the verdict of its next ready ticket. Other verdicts are unchanged. No new pause kind, no schema change.

## Testing Decisions

A good test checks external behavior: what a launch delivers, how long its recorded steps say it took, what the views show, what the stores hold. It does not assert on goroutine layout, lock internals or private helpers.

Seams, highest first:

1. **Scheduler seam: `ClaimNext` with the fake runner and a fake agent clock.** Covers parallel launch of N tickets, slot reservation against every cap, failed launch handling and kick, server stop rollback, the single-flight install (same key waits, different key does not, first failure hands over), the `launch-timed` event and the launch-step transitions. Uses the launcher wait group. Prior art: the existing `server/sched` claim and run tests.
2. **Pty runner seam: the pty launch tests with the fake Claude fixture.** Covers the ready check (no transcript wait; a missing ready signal fails the launch; delivery proof and resend still work), the transcript lookup (path check, glob fallback at most once a second, `CLAUDE_CONFIG_DIR`) and the slug rule against the real project dirs. Prior art: the existing `pty_launch_test.go` and transcript tests, minus the sleep and the test that waits out the old wait.
3. **Store seam: the ticket-move allocator and the index.** Covers the `flock` (a leftover old lock file counts as free; a dead holder releases), the archive back-off schedule, coalesced scans and the stale-install guard, and the file sum from parsed bytes. Prior art: the allocate and index tests; `migratefail` and `gocachegate` for the flock pattern.
4. **Stats and evolve seam: Go tests on the stats build and `gx evolve inputs`.** Covers the stamp, bucketing each launch on its own day, the median and the share under 15s, the text and `--json` output of `gx server stats`, the Launch rows block and the `slow-launch` flag. Prior art: the friction series in the stats build tests and the check-rows tests. The skill anchor test pins the existing anchors.
5. **Web seam: vitest for label and line rendering; one existing Playwright flow extended per page.** Render and component logic (labels, the shared line slot, the over-cap clause) is vitest, and the TUI row phase is a TUI unit test. Per `.ai/index.md` and `gx-playwright-e2es`, no new flow: extend an existing queue/epic flow on the scenario that holds a launching ticket, and the Stats flow for the Launch section. If a launching ticket needs a new fixture scenario, the implementer adds the scenario, not a flow, and keeps `CATALOG.md` and `maxFlows` in step.

6. **Herdr runner seam: the herdr runner tests.** Covers the workspace being created once (story 2).
7. **Design session finish seam: a Go test on design session finish.** Covers the pane closed and the process gone (story 36).

Cold-store timing and real-hardware numbers are not unit-tested; the verify ticket checks them against a real launch.

## Verification

All checks run against a temporary gx server after the epic is built. The verify ticket first records a clean baseline from a real launch on gx: it reads the recorded `launch-timed` events and confirms the stale allocation lock is gone.

Mock states: none (no stored mock).

New flows or scenarios to write (these become implementation tickets): a fixture scenario with a ticket held in `launching` at a chosen step (the implementer picks the name and how a test steps it), and a fixture scenario with an epic at its agent cap. Both extend existing flows; no new Playwright flow unless the guard allows it.

1. Story 1 (15s): launch one ready ticket on the temporary server, then `grep launch-timed` the epic run log. The event's total is under 15s, and `gx evolve inputs` reads the same launch from the run log. (`gx server stats --json` shows it only after the ticket finishes.) Stats page flow (`stats.e2e.ts`).
2. Story 2 (N together): queue an epic with 4 ready tickets and 4 free slots. Four `launch-timed` events have overlapping claim-to-delivery windows, and `ticket-claimed` events for all four precede the first `launch-timed`. Epic flow `6. Queue an epic, and outside changes show up`. A herdr-runner test launches N tickets in parallel into one new workspace and checks it is created once.
3. Story 3 (no transcript wait): the deps-done to prompt-delivered gap in the `launch-timed` steps is under 5s with no `ready signal missing` warning in the server log.
4. Story 4 (missing ready signal): Go test with the fake agent that never signals ready; the launch fails and the ticket pauses at once as `iteration-error`, and the pause card's Retry resumes it. Checked by the scheduler and pty tests. No catalog flow.
5. Story 5 (lost-prompt protection): the existing delivery proof and resend pty tests pass; the real launch's step 4 is the proof time. Run a launch with a prompt that gets rejected via the fake and check the resend.
6. Story 6 (cheap lookup): pty test; on the real launch, the server averages under 50% of one core during a 4-ticket wave (read with `ps`; the old figure was about 269%).
7. Story 7 (`CLAUDE_CONFIG_DIR`): pty test with a custom directory; delivery proof finds the transcript.
8. Story 8 (no rescan on launch path): the claim-to-worktree step in `launch-timed` is under 3s even with a stuck one-off in the store.
9. Story 9 (claimed ticket in index): after the claim write, `gx tickets show` returns `status: claimed` before the worktree exists. Scheduler test.
10. Story 10 (flock): allocator test with a leftover lock file and with a killed holder; both count as free.
11. Story 11 (back-off): sweep test with a stuck ticket; retries at 1 minute rising to the 1 hour cap, one warn log line per ticket until the next try.
12. Story 12 (coalesced scans): index test with 8 scans at once; at most one in flight plus one trailing re-run, and the final index matches the newest scan. A `gx tickets` write that sends the `/changed` ping and then a watcher scan 50ms later is not required to merge into one scan (a debounce is out of scope).
13. Story 13 (refresh cost): index benchmark on a store of about 3000 tickets shows a warm refresh near 15ms; the server log shows a `Refresh` line with duration and caller.
14. Story 14 (caps): scheduler test with the epic cap, project cap and global cap each binding while launches run; running plus launching never exceeds the limit.
15. Story 15 (failed launch): scheduler test; the registry entry is gone, the ticket is paused with the matching kind, `iteration-launch-failed` is in the run log, and the freed slot is refilled.
16. Story 16 (stop mid-launch): start a launch with a slow fake install, stop the server, check the worktree is gone and the ticket is `open`; the stop returns without waiting for the install. A second case stops the server during step 4 and checks that no agent process or pane remains.
17. Story 17 (frozen install per manager): scheduler test for each of npm, pnpm, yarn and bun fixtures; the right command runs in the new worktree.
18. Story 18 (shared install): launch 6 tickets of one epic on a cold temporary pnpm store (`--store-dir`); one install starts first, the rest wait, and the total wave finishes under 10s (6.8s measured; 6 concurrent cold installs took 19 to 25s).
19. Story 19 (different keys, hand-over): scheduler test; a different-key launch does not wait, and when the first install fails the next waiting launch installs.
20. Story 20 (worktree lock free during install): scheduler test; a worktree add completes while another launch's install is blocked.
21. Story 21 (failed install): scheduler test; the launch fails, the worktree is discarded and the ticket pauses at once as `iteration-error`.
22. Story 22 (fill-first): scheduler test with two queued epics; the first fills its slots before the second gets one.
23. Story 23 (cap skip, reorder): scheduler test; an epic at its cap is skipped; reordering the queue mid-launch leaves the claimed ticket in place.
24. Story 24 (Launching label in four views): fixture scenario with a launching ticket; web Queue row, ticket page and epic page show the label (queue and epic flow in `epics.e2e.ts`, extended); TUI Queue row via the TUI unit test and `e2e/server_tui_test.go`.
25. Story 25 (four labels): vitest and TUI test render each of the four labels with its position.
26. Story 26 (timers): vitest; the claimed-at timer shows beside the label and the agent's timer takes over once the agent starts.
27. Story 27 (live step change): fixture advances the step; the page updates with no reload (the suite fails on a reload).
28. Story 28 (no new status): `gx tickets validate` and the schema snapshot show no new status value; a launching ticket still reads `claimed`.
29. Story 29 (failed launch display): fixture with a failing launch; the ticket pauses as `iteration-error` with the launch error shown and the Launching label gone.
30. Story 30 (restart mid-launch): scheduler test; a reclaimed launching run with no agent reopens the ticket and launches again as a new launch; a live agent continues as running. These cases arise only after an unclean exit (crash or kill): a clean stop rolls every launch back (story 16). Restart flows in `restart-run.e2e.ts` still pass.
31. Story 31 (Waiting for a slot): fixture scenario with an epic at its cap; the web Queue row, epic page, ticket page and TUI show "Waiting for a slot: epic cap n/m" naming every full cap.
32. Story 32 (over cap): same scenario with the cap lowered below the live count shows the "(over cap, no new starts until one ends)" clause. vitest for the text.
33. Story 33 (other verdicts): existing disk, harness, usage-gate and Herdr-down flows (`disk.e2e.ts` and the pause/queue flows) still show their own verdicts.
34. Story 34 (shared line slot): vitest; a ticket never shows both lines.
35. Story 35 (design outside caps): scheduler test; a design run does not count toward any cap and does not appear in the Queue views or launch timings; the Agents tab still lists it (`design.e2e.ts`).
36. Story 36 (no leaked design agent): a Go test on design session finish (pane closed, process gone); on the temporary server, open and finish a design session and check no design pane or process remains (`ps`, herdr pane list). A leak is fixed in this epic.
37. Story 37 (`launch-timed`): after any real launch, the run log holds one `launch-timed` event with claim time, total, four step durations and `path`.
38. Story 38 (wait in deps step): scheduler test; a launch that waited for a shared install has its deps step at least as long as the wait.
39. Story 39 (failed and cut-off launches untimed): scheduler test; a failed launch has `launch-failed` and no `launch-timed`; a restart-cut launch has none and its reclaim has one.
40. Story 40 (Launch section): Stats flow (`stats.e2e.ts`, extended) shows the range median, the share under 15s, the per-day median chart and the median per step. `gx server stats --json` carries the same.
41. Story 41 (own-day bucketing): stats build test; a ticket with launches on two days counts one launch on each.
42. Story 42 (`gx server stats`): `gx server stats` text output includes the launch median and the share under 15s; exit code 0; `--json` includes the launch data.
43. Story 43 (Launch rows): `gx evolve inputs` on a run log with one launch over 15s and a slow median prints both rows under "Launch rows", next to `### Check rows`.
44. Story 44 (no deep read): the inputs table shows `slow-launch` with severity `rest`, and the deep-read cap count is unchanged by it.
45. Story 45 (anchors): `skills/check_rules_test.go` passes.
46. Story 46 (migration): `go test ./migrations -run TestSchemaSnapshot` passes; `gx tickets migrate` on a copy of the store registers the field and changes no old ticket.
47. Story 47 (old hooks gone): `rg` finds no `GX_PTY_READY_WAIT`, `readyDelay`, `readyWait`, `awaitReady` or `DefaultReadyWait` in the tree.
48. Story 48 (glossary): `GLOSSARY.md` has the Launch time and Launch step terms and the Design session line.

## Out of Scope

- A visible warnings indicator in the web UI (icon and popover next to the health indicator): its own design, `gx:webui-warnings`.
- Skipping or reusing dependencies (symlinks, clones, marker files, a skip rule), `npm ci` switches and a lockfile change.
- A cap on launches in flight, a setting for fill order, round-robin between queued epics.
- A new ticket status, a new pause kind, special text for a restart mid-launch.
- A live alert for a slow launch. Stats and `gx-evolve` only.
- Timing the rest of the iteration. Only claim to launch prompt delivered is timed.
- A ready-check change for Codex or for non-slash prompts.
- Backfilling launch data for old tickets.
- Counting design runs toward any cap.
- A debounce between a `gx tickets` write's `/changed` ping and the watcher scan.
- Reloading only changed epics in the index (deferred until a measurement shows a need).
- Follow-ups noted on the map: `Pty.Wait` polling cost (`gx:follow-ups/215`), server start resuming every saved session at once (`gx:follow-ups/296`), slow full test runs, and `gx tickets validate` rejecting a map epic with no children.

## Further Notes

- The stale `.alloc.lock` file was removed on 2026-11-06 with the person's approval, so later timings no longer carry the 20s sweep. Earlier numbers (97s median, "deps install 66s") include that stall and the 30s ready wait; do not use them as a baseline.
- Measured install cost: `pnpm install --frozen-lockfile` takes 0.3s warm and 3.5s cold; `npm ci` 1.4s warm. gx's own pnpm setup is the benchmark.
- Parallel cost with a warm store: 6 launches about 3.5s wall and about 19 CPU-s, about 330 MB RSS per launch.
- Research findings live in `docs/research/fast-queue-startup-*.md` on the branches `research/fast-queue-startup-*`.
- The map is `gx:fast-queue-startup`; the spec is handed to a `fast-queue-startup-impl` epic.
