# gx CPU/energy investigation — findings (follow-up to `cpu-usage-handoff.md`)

Continuation of the idle-CPU work that produced commit `474ea471`. That commit removed two
unconditional render loops; this pass profiled the *current* binary and read every remaining
periodic wakeup source in the codebase.

## How this was measured

- Built `main` at `474ea471` and ran it under a pty (`script -q /dev/null gx …`), idle, untouched.
- CPU time sampled with `ps -o time=` over 30–60s windows; stacks with `sample <pid>`.
- Child-process spawns detected by polling `ps -ax -o pid=,ppid=` at ~6 Hz.
- Go microbenchmarks for `git.FindRepo`, `tickets.Load`, and the Queue/Tickets `View()`
  (benchmarks were temporary and are not committed; numbers below).

Measured idle CPU (Apple M4, nothing happening, no run in flight):

| Instance | idle CPU |
| --- | --- |
| `gx` (Worktrees tab) | **0.57%** |
| `gx status` (Status tab) | **0.47%** |
| `gx tickets` (Tickets tab) | **1.03%** |
| `gx` (Worktrees) with `tea.WithFPS(10)` | **0.12%** |
| `gx tickets` with `tea.WithFPS(10)` | **0.67%** |

Microbenchmarks (5 epics × 15 tickets):

| Operation | cost |
| --- | --- |
| `git.FindRepo(".")` | **20 ms** (spawns 5 `git` processes) |
| `tickets.Load(scratchDir)` | 1.6 ms |
| `QueueModel.View()` (cache warm) | 0.77 ms |
| `tickets.Model.View()` | 0.98 ms |

## Finding 1 — every 2s poll shells out to git 5 times, on the UI goroutine

**Highest-impact remaining issue.**

`ui/tickets/model_data.go:190` (`cmdLoad`) and `ui/tickets/queue.go:194` (`cmdLoadQueue`) both start
with `scratchDirFor(m.worktreeRoot)`, which calls `git.FindRepo` → `git.IdentifyDir`
(`git/repo.go:53`). That runs `git rev-parse --git-dir`, `--is-inside-work-tree`,
`--show-toplevel`, `--git-common-dir`, plus `detectMainBranch` → `git symbolic-ref … origin/HEAD` —
**five `git` subprocesses, ~20 ms wall each call**.

Because `cmdAutoRefresh` (2s, `ui/tickets/auto_refresh.go`) re-fires `cmdLoad`/`cmdLoadQueue`
forever, an open Tickets or Queue tab forks ~2.5 processes per second, indefinitely, while
completely idle. Directly observed: `git` children of the idle `gx tickets` process, and `fork`/
`__wait4_nocancel`/`os.StartProcess` frames in the sample. It accounts for the whole 1.03% vs 0.57%
gap between the Tickets tab and the baseline.

Two aggravating details:

- `scratchDirFor` is called **synchronously inside `Update`**, not inside the returned `tea.Cmd`
  closure — so it blocks the event loop for ~20 ms every 2 seconds. Only `tickets.Load` runs async.
- The result cannot change for the life of the model: `worktreeRoot` is fixed at construction.

**Fix:** resolve the scratch dir once (in `NewModel`/`NewQueueModel`, or memoize `git.FindRepo`
per directory) and reuse it. Nothing else needs to change to remove ~2.5 forks/sec.

## Finding 2 — bubbletea's renderer ticker runs at 60 Hz forever

`charm.land/bubbletea/v2@v2.0.6` `tea.go:1392` starts an unconditional `time.NewTicker(1/fps)`, and
the goroutine at `startRenderer` calls `flush()` + `renderer.flush(false)` on every tick for the
whole life of the program. `defaultFPS = 60` and gx never passes `tea.WithFPS`.

Each tick is cheap (the `cursedRenderer.flush` early-outs on `viewEquals`), but it means **60 timer
wakeups per second regardless of activity** — that's the entire 0.47–0.57% idle floor, and it is
what keeps the process out of deep idle on battery. The sample of an idle Worktrees/Status tab shows
nothing but `viewEquals` and `flush`.

Dropping to 10 fps measured **0.57% → 0.12%** (≈5×). Input handling is not gated by this ticker —
messages are processed immediately; only the paint is deferred — so 15–20 fps costs at most
50–65 ms of paint latency and would be imperceptible for this app (no animations beyond the ~10 Hz
spinners).

**Fix:** pass `tea.WithFPS(15)` (or 20) at the 7 `tea.NewProgram` sites in `cmd/ui.go` — or better,
one shared constructor so it can't be forgotten.

## Finding 3 — the 2s poll is unconditional and never backs off

`cmdAutoRefresh` reloads and re-renders every 2 seconds whether or not anything on disk changed,
whether or not a run is in flight, and whether or not the terminal is even focused. Post-`474ea471`
the *render* is memoized, but the load still happens, and each `autoRefreshMsg` still drives a full
`Update` + `View` of the app.

With Finding 1 fixed this drops to ~1.6 ms of disk work + ~1 ms render per tick (~0.13% CPU), which
is acceptable; if it needs to go lower, cheap options are a `stat`-based mtime pre-check before the
full `tickets.Load`, or a slower interval when no run is in flight.

## Finding 4 — the auto-refresh loop dies on tab switch and never restarts

Not a CPU cost, but it makes the CPU behaviour nondeterministic and is a real bug.

The app shell routes every non-nav message to the **active page only**
(`ui/app/model_update.go:123-125`), while live page models persist in `livePageByTab`. So:

- Switch from Tickets/Queue to any other tab: the pending `autoRefreshMsg` is delivered to the new
  active page, which ignores it, so nothing rearms — **the poll loop dies**.
- Switch back: `applySwitch` takes the cached-model path, and `autoRefreshStarted` is already `true`
  (`ui/tickets/model.go:255`, `ui/tickets/queue.go:253`), so the loop is **never restarted**. The
  tab silently stops auto-refreshing for the rest of the process's life.
- Tickets and Queue share the same `autoRefreshMsg` type, so a tick started by one tab can rearm on
  the other — while both are alive the effective poll rate is 1 Hz, not 0.5 Hz.

**Fix:** restart the loop from `OnPageActivated` (and stop treating `autoRefreshStarted` as a
process-lifetime latch), or make each tab's tick message distinct and re-armed on activation.

## Finding 5 — pull/push spinner spins at 10 Hz while waiting for the user

`ui/pull/pull.go:134` and `ui/push/push.go:186` handle `spinner.TickMsg` unconditionally and always
return the rearm command. The poll loop stops correctly (`handlePoll` returns nil once
`activeRunner` is nil), but the spinner does not: once a pull/push modal has ticked, it keeps
re-rendering at 10 Hz in phases that are purely waiting on a keypress — `phaseStashConfirm`,
`phasePopStashConfirm`, `phaseFailed`. Leave a failed pull dialog open and gx renders 10×/s forever.

`ui/bump/bump.go:92` and `ui/amend/amend.go:222` already do this correctly (`if !running → nil`),
as does `ui/tickets` (`len(m.runningEpics) == 0 → nil`) and `ui/worktrees`.

**Fix:** gate on the running phase, matching bump/amend.

## Finding 6 — Queue tab forks `ps` every 2s when another gx holds the attach lock

`cmdLoadQueue` also calls `ForeignAttachPID` (`ui/tickets/loop_registry.go:722`) → `attachLockIsStale`
→ `processStartTime` → `exec.Command("ps", "-o", "lstart=", "-p", …)` (`ui/tickets/attach_lock.go:34`).
It early-outs when no lock file exists, so it is free in the common case — but whenever another gx
process is attached, that is one extra fork per 2s poll on top of Finding 1.

**Fix:** cache the liveness answer for a few seconds, or fold it into the same throttle as the load.

## Finding 7 — `reflect.DeepEqual` over every ticket body, inside `View()`

`buildQueueEntriesCached` (`ui/tickets/queue_rows.go:88-93`) validates its cache with
`reflect.DeepEqual(c.epics, m.epics)` — a deep compare of the full epic slice including every
ticket's parsed body — on every render. It is still much cheaper than the rebuild it avoids (and it
is what makes the cache survive a reload of unchanged data, which is the point), but a content hash
or an mtime/size fingerprint of the scratch dir would be a large constant-factor win.

## Checked and found clean

- `ui/status` `renderTickCmd` — gated behind `Settings.RenderHeartbeat`, off in production
  (`474ea471`). Measured Status tab idle = baseline, no child spawns.
- `handleFlashTick` (`ui/status/model_update.go:298`) — bounded by a frame counter.
- Spinner ticks in `ui/tickets` (both tabs), `ui/worktrees`, `ui/bump`, `ui/amend` — all gated on an
  active job.
- `implementPollInterval` = 300 ms, only while a run is tracked; `ralphloop` smart-zone polling is
  30 s; `runLandQueue` blocks on a channel rather than spinning; `chat_eventsink`'s flush ticker is
  stopped on close.
- `ui/log`, `ui/prs`, `ui/history`, `ui/worktrees` — no unconditional periodic work; only debounces.
- No busy-wait loops anywhere outside `testutil/`.

## Suggested order of work

1. Cache the scratch dir / memoize `git.FindRepo` (Finding 1) — biggest win, smallest change.
2. `tea.WithFPS(15)` (Finding 2) — ~5× cut of the idle floor for *every* gx TUI.
3. Restart the auto-refresh loop on page activation (Finding 4) — correctness.
4. Gate the pull/push spinner (Finding 5).
5. Optional: mtime pre-check for the poll (Finding 3), `ps` throttle (Finding 6), cache
   fingerprint instead of `DeepEqual` (Finding 7).
