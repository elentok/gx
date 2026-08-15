# Handoff: gx idle-CPU investigation

## Context

Started from a `blf power report` investigation (in `~/dev/blf`, separate repo) that flagged an
idle `gx` process (pid 6636, Queue tab open, running ~12h) as a top-10 daily battery-energy
consumer alongside WindowServer/Chrome/coreaudiod. Traced the cause into this repo
(`~/dev/gx/main`) and fixed it.

## What was found and fixed

Two independent unconditional-forever render loops, both already fixed and committed:

1. **`ui/status` Status tab** — `renderTickCmd()` (`ui/status/model_update.go`) forced a full
   page re-render every 1s forever, with no dirty check. It existed only as a `teatest` heartbeat
   (see `docs/retro/0001-notifications.md`), not for any real UI need (no live clock/timer
   depends on it).
2. **`ui/tickets` Queue tab** — `cmdAutoRefresh()` (`ui/tickets/auto_refresh.go`) polls `.scratch/`
   every 2s and `View()` (`ui/tickets/queue_view.go`) rebuilt the *entire* tree from scratch on
   every render regardless of whether anything changed — this was the one actually burning CPU on
   the live instance we found (confirmed via `top -pid 6636` sampling: CPU ticked up every ~2s).

Fix details are in the commit, not repeated here — see:

```
git show 474ea471
```

Summary of the fix shape:
- Status: added `Settings.RenderHeartbeat` (default `false`); only `status.DefaultSettings()`
  (test helper) turns it on. Production settings (`cmd/ui.go` `settingsFromConfig`) leave it off.
- Queue: added `entriesCache` (`ui/tickets/queue_rows.go`, `queueEntriesCache` type +
  `buildQueueEntriesCached()`), memoizing `buildQueueEntries()` against its real inputs (epics,
  checked map, hideComplete, collapsed IDs). Verified safe mid-run because live per-row state
  (running-epic elapsed time, spinner frame, parked/stalled) is read fresh at draw time by
  `queueRenderOpts`'s `Label` callback (`ui/tickets/queue_view.go`), not baked into cached
  entries.

Full test suite + `go vet` passed before commit. Binary was reinstalled (`go install ./...`) and
the stale pid 6636 was killed during the session — a fresh `gx` invocation runs the fixed code.

**Status: committed, not pushed.** Local commit `474ea471` on `main`, one commit ahead of
`origin/main`. Push is a user decision, not yet made.

## What's NOT done / possible follow-ups

- No profiling (pprof) was done — the diagnosis was via `powermetrics`-derived samples (from
  `blf power report`) plus live `top -pid` sampling and code reading, not a CPU profile. If CPU
  usage is still a concern after this fix, a real profile would be the next step.
- Other tabs (Tickets, worktrees, etc.) were checked by a research subagent and found to already
  gate their polling loops correctly (only while relevant tab active, only while a run is in
  flight) — not touched, no known issue there.
- No fsnotify/kqueue watcher exists anywhere in this codebase (verified) — the two kqueue fds seen
  on the live process were Go runtime netpoller + bubbletea's terminal raw-mode reader, unrelated
  to app logic.
- The untracked files sitting in this repo (`TODO.md`, `.scratch-cleanup-report.md`, `bugs-07.md`,
  `docs/plans/suggested-actions-menu.md`, `docs/schedulers-research.md`) are **unrelated** to this
  work — left alone, not staged, pre-existing in the working tree before this session touched
  anything.
- Have not re-measured actual CPU/energy savings post-fix (e.g. via `blf power report` in
  `~/dev/blf` after gx has run for a while under the new binary) — worth doing if you want to
  confirm the fix's real-world impact.

## Suggested skills for the next session

- `writing-go` — if continuing to touch this codebase, follow its Go conventions.
- `verify` — run before any further commit here (staged/working-changes checks).
- `commit` — already used once this session for the fix commit; reuse if further changes are made.
- `gx-investigate` — if further gx/ralph-loop-loop-shaped bugs turn up (queue state, ticket
  scheduling), this skill has the background/inventory for diagnosing them.
