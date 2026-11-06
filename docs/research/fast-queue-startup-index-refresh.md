# Fast queue startup: the cost of `Index.Refresh`

Ticket: `gx:fast-queue-startup/16`. Measured 2026-11-06 on the dev laptop (12 cores, APFS, load average about 12 during the runs, so the numbers lean pessimistic) against the live store `~/.local/share/gx/tickets`: 5 projects, 2,921 non-archived tickets (6,148 `.md` files with the archives), 9.4 MB of ticket bytes. Nothing in the store or the code was changed.

## Answer in short

1. There is no metric or log of `Refresh` time today. `server-log.jsonl` has no rescan line (1,003 lines, all session events), and `Rescan`/`Refresh` log only on error.
2. A warm `Index.Refresh` (the server's steady state) takes **p50 51ms, p90 63ms, worst 73ms** over 100 runs, 2,921 tickets. A cold first scan in a fresh process takes 95ms. It grows linearly, about 17µs per ticket (10k tickets would be about 170ms).
3. **Three quarters of it is a second, redundant read of every ticket file.** `scanStore` calls `tickets.LoadResolved`, which already skips the read and parse through an mtime+size cache (`tickets/loader.go:96-146`), and then `ticketInfo` calls `FileSum` (`server/index/index.go:238, 242-249`), which re-reads and SHA-256s every ticket file serially, cache or not. That is 38ms of the 51ms. The parse cache is defeated at the one place it matters.
4. Notifying stream subscribers is free: 1,000 `Publish` calls to 5 live subscribers take 0.5ms, and `Publish` never blocks. An unchanged store publishes nothing.
5. Overlapping runs are normal and each is a full scan: nothing coalesces them. Four at once take 92ms wall, eight take 192ms. Worse, a code-reading finding: `Refresh` scans outside the lock and installs whatever it read last, so an older scan that finishes late can overwrite a newer one (see "Overlap and a stale install").
6. A launch needs a refresh of the claimed ticket's epic, not of the whole store, but the correct first fix is to make the full `Refresh` cheap (about 15ms), and a launch can keep calling it.

## Commands and raw results

The harnesses are throwaway Go programs placed in the repo tree for one build (they import `server/index`), run under `scripts/proc-cap`, then deleted; the sources are in the appendix. They only read the store: `Index.New`, `Index.Refresh`, `tickets.ProjectDirs`, `tickets.LoadResolved`, `index.FileSum`, `os.ReadFile`, `os.Stat`. (`find` for files newer than the run start showed only `gx/one-offs`, `gx/one-offs-archived` and `.git`, which the live server writes on its own; the harness has no write call.)

```
go build -o $SCRATCH/measure ./zz_measure && scripts/proc-cap $SCRATCH/measure
go build -o $SCRATCH/measure2 ./zz_measure2 && scripts/proc-cap $SCRATCH/measure2
```

Run 1 (100 warm iterations after one cold scan):

```
cold New (first scan):            94.7ms
projects=5 tickets=2921
Refresh (total, warm cache)       n=100 min=44.7ms p50=50.6ms p90=62.7ms max=73.2ms
  ProjectDirs                     p50=0.1ms
  LoadResolved all projects       p50=8.7ms  p90=10.9ms  max=15.6ms
  FileSum all tickets (read+sha)  p50=37.9ms p90=47.4ms  max=62.5ms
1000 Publish w/ 5 subs:           0.5ms
ReadFile only, all tickets:       33.5ms (9,395,376 bytes)
Stat only, all tickets:           3.2ms
```

Run 2 (scaling and overlap):

```
FileSum n=250  best=2.5ms  (10.2us/ticket)
FileSum n=500  best=5.1ms  (10.1us/ticket)
FileSum n=1000 best=11.4ms (11.4us/ticket)
FileSum n=2000 best=21.6ms (10.8us/ticket)
FileSum n=2921 best=33.6ms (11.5us/ticket)
concurrent Refresh x1: wall=56.7ms
concurrent Refresh x2: wall=76.8ms
concurrent Refresh x4: wall=91.7ms
concurrent Refresh x8: wall=191.6ms
```

Counts of how often tickets change, for the frequency estimate: `find ~/.local/share/gx/tickets/gx -name '*.md' -mmin -60` gave 25 files, `-mmin -1440` gave 397; `git -C ~/.local/share/gx/tickets log --since='24 hours ago' --oneline | wc -l` gave 603 commits.

## Where the 51ms goes

| part | cost (p50) | share | how measured |
|---|---|---|---|
| `FileSum`: read + SHA-256 of every ticket, serial | 38ms | 74% | timed alone; `ReadFile` alone is 33.5ms, so the hash adds about 4ms |
| `LoadResolved`: stat every file, cache hit, `cloneTicket`, per-epic checks, cross-epic resolve | 8.7ms | 17% | timed alone; `Stat` of all files is 3.2ms of it |
| index build and diff: `ticketInfo` (minus the sum), sort, `reflect.DeepEqual` per ticket, `changedEpicMeta` | about 4ms | 8% | remainder: 50.6 - 8.7 - 37.9 - 0.1 |
| `ProjectDirs` | 0.1ms | 0% | timed alone |
| publish to subscribers | under 0.001ms per event | 0% | 1,000 events in 0.5ms |

Read, parse, build and notify as the ticket asked: the file read is the biggest part, but the part that is waste is the second one. A cold scan (empty parse cache, as at server start) is 95ms: about 38ms of that is `FileSum`, about 20ms is real parse, the rest read and build. In the server the cache is warm after the first scan, so steady state is the warm number. The parse cache does not trust a file whose mtime is within 2s of the time it was cached (`ticketCacheRacy`, `tickets/loader.go:104-107`), so a ticket written moments ago is re-read once; that is the right behaviour.

Growth: `FileSum` is flat at 10 to 11.5µs per ticket from 250 to 2,921 tickets, so the whole `Refresh` is linear. Only the `gx` project is big (2,920 tickets, `LoadResolved` 8.5ms); the other four projects hold 1 ticket or none. Archived tickets (about 3,060 files) are not loaded.

## How often it runs, and overlap

No log exists, so this is from the code.

Callers of a full rescan (`Index.Rescan`, which runs `Refresh` and then the four `OnRescan` steps):
- the 15s backstop poll while the watch is armed (`server/index/watch.go:17, 64-66, 87`), 5,760 a day, 2s when there is no watch;
- every file-watch event on a `.md` name, debounced 50ms (`watch.go:19, 93-104`);
- every `gx tickets` write's `POST /v1/tickets/changed` ping (`watch.go:124-145`; callers `apiclient/tickets.go:123`, `cmd/serverapi/server_reads.go:253`);
- server-side calls: the launch claim (`server/sched/run.go:55`), a one-off's end (`:234`), a finish (`:281`), `ticketwrite/blocked_pause.go:58`, `ticketops/{cancel,recommend,unmute,answer,ask}.go`, `merge/stashmerge.go:125`.

Callers of bare `Refresh`: `queue/oneoff.go:252,267,389`, `queue/oneoffsweep.go:54`, `ticketwrite/pause_ticket.go:30`, `recovery/investigate.go:78`.

Every write by `gx tickets` therefore runs at least two rescans: the ping's, at once, and the watcher's, 50ms later. A write takes at least 50ms to scan, so the two overlap on every write that goes through the server. With about 400 modified files a day, plus launches, that is roughly 800 to 1,500 write-driven scans on top of the 5,760 poll scans a day. At 51ms each this is about 0.3 to 0.4 CPU-seconds per minute, cheap in CPU. The problem is latency on the path that waits (the launch), not the load.

There is no mutex, queue or single-flight: `Refresh` runs the scan outside any lock, so concurrent calls all scan in parallel (x2 = 77ms wall, x8 = 192ms) and a launch that arrives while a poll scan is running pays for its own full scan plus the contention. The `OnRescan` steps add much more than `Refresh` itself on this store (ticket 15 covers that), and the launch decision in ticket 15 is to call only `Refresh`.

### Overlap and a stale install

`Refresh` (`server/index/index.go:112-125`) reads the store, then takes `i.mu` and installs the result with `i.list, i.loaded = list, loaded`. Two scans A (starts first) and B (starts after a write) can finish in the order B, A. B installs the new state, then A installs the older state it read before the write, and publishes `ticket-changed` for the difference. The index then shows the pre-write file until the next rescan (the poll, at most 15s; a launch's claim is the case that matters, since the launch's own `Refresh` could return before a stale one lands). I did not reproduce the race (it needs a write landing inside a scan window); it follows from the code and the fact that nothing orders scans. The scan being parallel across epics (8 workers) widens the window.

## What would make it cheaper

Each option, what it saves, what it risks, with the file kept as the truth. In order of recommendation.

1. **Compute the file sum in the loader, from the bytes it already read, and store it in the parse cache.** `loadTicketFile` reads the file to parse it; hash those bytes there and keep the sum on the `Ticket` (or in the cache entry) and let `ticketInfo` read it from the ticket. A cache hit (same mtime and size, outside the racy window) then returns the stored sum with no second read. Saves the whole 38ms: warm `Refresh` falls from about 51ms to about 13 to 15ms (the stat, clone and build that remain), and a cold scan falls by about 38ms. Risk: the sum must be of the bytes the parse saw, which is the more correct meaning (today `FileSum` is a second read that can see a newer file than the parse did, so the sum and the parsed ticket can disagree). The cache already refuses entries that are too fresh, so a same-size edit inside the mtime granularity is re-read, which keeps the claim check (`server/sched/claim.go:225`, "the file moved on since") sound. It does change `tickets.Ticket` or the cache entry shape; that is a small, contained change. This is the right first fix: no new invalidation logic, no state to go stale.
2. **Make overlapping scans safe and shared: one scan at a time with a trailing re-run (single-flight).** A caller that needs to see its own write asks for a scan that starts after the call; calls that arrive during a scan share the next one instead of each starting their own. Saves: the x2/x4/x8 cost (a write's ping and watcher scan collapse to one or two). Fixes the stale install, because only one scan installs at a time and scans install in start order (a monotonic scan number checked under `i.mu` is the minimum if single-flight is not wanted). Risk: a caller must not be satisfied by a scan that started before its write; the trailing run is what guarantees that. This is correctness work and should land with option 1, not after it.
3. **Reload only the epics whose files changed.** `Load` already works per epic (`LoadEpic`, 8 workers); a refresh that is told the changed paths (the ping and the claim know the ticket; the watcher event carries the path) reloads those epics, reuses the other epics' `Epic` values from the previous `i.loaded`, re-runs `resolveCrossEpic` for the project, and rebuilds only those epics' rows in `i.list`. Saves the remaining stat/clone/build for the untouched 166 epics. After option 1 that is a few ms of 13 to 15ms, so the gain is small and the risk is real: a missed path leaves a stale epic in memory (the poll would repair it in 15s), cross-epic blockers and `RenderedStatus` depend on other epics, and `changedEpicMeta` compares whole-store maps. Not worth doing until the warm full refresh is still shown to be too slow after options 1 and 2.
4. **Caching parsed tickets by mtime** already exists (`tickets/loader.go:96-146`), and it works: the `LoadResolved` part is 8.7ms for 2,921 tickets. The gap is that `FileSum` bypasses it (option 1). Nothing more is needed here.
5. **An index update for one ticket** (a `RefreshTicket(path)` patching one row) is the cheapest on paper but not safe as a patch: a claim changes the ticket's status, and the epic's `RenderedStatus`, `EpicStatus` and the blocked/ready state of its dependents and of other epics (`resolveCrossEpic`) are derived from the whole epic and project. A patch has to re-derive all of that, which is option 3 with extra risk. Rejected.

Do not use `mtime` alone as the claim's "moved on" check in place of the sum: the claim check exists for edits that keep size and mtime close; the sum from the parsed bytes (option 1) keeps its strength.

## Does a launch need the full `Refresh`?

The launch (`server/sched/run.go:55`, then ticket 15's decision to call only `Index.Refresh`) needs the index to show the claimed ticket as in progress before the next scheduling pass and for the stream. Consumers of the index after a claim are the scheduler's slot counts (`sched/iterations.go:33`, `Snapshot`), `EpicsOf` readers (`sched/finishedclaims.go:37`, `projects/projects.go:118`) and the stream's `ticket-changed` event. They read derived fields (rendered status, epic status), so a one-row patch is not enough (option 5).

Recommendation: keep the launch calling the full `Refresh`, after making it cheap (option 1) and ordered (option 2). Then the wait is about 15ms, not 51ms, and it cannot be overwritten by a late stale scan. A claimed-ticket-only update (option 3 narrowed to the claimed ticket's epic) is the follow-up if the number must go lower, and it should be measured after options 1 and 2 first.

A smaller point for the launch path: `Refresh` is gated by nothing but runs `ProjectDirs` and every project, including the four near-empty ones; that costs 0.3ms and is not worth special-casing.

## Not measured

- The `OnRescan` steps (`ScanParentDefects`, `ScanLateFinishes`, the one-offs `Sweep`, `ScanCleanups`): ticket 15 covers them. A launch skips them; the poll and the watcher run them every time.
- A live count of `Refresh` calls per minute: there is no counter. Adding a timing log line and a call counter in `Refresh` (duration, tickets read from cache, who called) would give the real rate and overlap; recommended with option 2 so the numbers here can be checked on the running server.
- The stale-install race was not reproduced, only derived from the code.

## Appendix: harness sources

The two programs are in the scratchpad of the session that ran them and are reproduced in short here.

Run 1 (`zz_measure/main.go`): `broker.New(0)`, `index.New(store, b, nil)` timed as the cold scan; list every ticket path from `tickets.LoadResolved` per `tickets.ProjectDirs`; 100 iterations each timing `idx.Refresh()`, `tickets.ProjectDirs`, `tickets.LoadResolved` for every project, and `index.FileSum` for every path; 20 iterations of `LoadResolved` per project; 1,000 `b.Publish` with 5 draining subscribers; one `os.ReadFile` and one `os.Stat` pass over all paths.

Run 2 (`zz_measure2/main.go`): the same index; `index.FileSum` over the first n paths (n = 250, 500, 1000, 2000, all), best of 20; then c = 1, 2, 4, 8 goroutines each calling `idx.Refresh()` at once, reporting wall time and the slowest call.
