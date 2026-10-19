# CPU watchdog: gx investigates its own CPU spikes

When `gx server` or a gx TUI burns CPU for a sustained time, it captures a profile of itself and
the server files a ticket for an agent to find the cause.

## Design

- **Measure** (`cpuwatch`): every 10s, read `getrusage` for self and children. CPU% = delta CPU
  time / delta wall time, in percent of one core. Children matter: the last spike was `herdr`
  subprocesses. `RUSAGE_CHILDREN` only counts children that exited, which fits short `herdr`/`git`
  calls.
- **Trigger:** self + children above `threshold-percent` (15) for `sustain` (1m). One long spike
  fires once; the watch re-arms only after use stays below the threshold for `sustain`, so
  a process hovering around the threshold is one spike.
- **Evidence:** `<state dir>/cpu-spikes/<time>-<source>-<pid>/`:
  - `cpu.pprof`: a 20s `runtime/pprof` CPU profile (keeps Go stacks, unlike macOS `sample`)
  - `goroutines.txt`: every goroutine's stack
  - `summary.md`: the samples, self vs children
- **Report:** the server handles its own spikes directly; a TUI posts to `POST /v1/cpu-spike`
  (CLI: `gx server cpu-spike`).
- **Ticket:** only in evolve maintainer mode with a registered `gx` project. A front-queued,
  commitless one-off in `gx` (`cpu-spike-<source>-<time>`), deduped per source, with chat notify.
  Otherwise the spike is only logged.
- **Limits:** `cooldown` (6h) between evidence captures in each process and between filed tickets,
  kept in memory (a restart resets it).
- **Config:** `cpu-watchdog.{enabled,threshold-percent,sustain,cooldown}`, reloadable for the
  server; a TUI reads it at start.

## Tasks

- [x] `cpuwatch` package: sampler, trigger, evidence capture, tests
- [x] Config section, defaults, parse test, schema and reload docs
- [x] Server: watcher goroutine, `cpuSpike` filing with cooldown, `/v1/cpu-spike`, tests
- [x] API client `CPUSpike` and `gx server cpu-spike` verb
- [x] TUI: every `app.New` entry point runs through `runApp`, which starts a watcher
- [x] Manual check: a real server spike filed a ticket with a usable profile
- [x] Fix: a server hovering around 15% captured evidence every ~2 min (64 dirs overnight);
      re-arm needs a sustained drop, and the cooldown also holds captures
