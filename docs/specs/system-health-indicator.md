# System health indicator

Handoff spec from the gx:system-health-indicator map. The decisions behind it are the closed tickets
gx:system-health-indicator/01 to 07. The research is in `docs/research/system-health-cpu-memory.md`
(branch `research/system-health-cpu-memory`) and `docs/research/system-health-top-processes.md`
(branch `research/system-health-top-processes`). The approved mock is
`$(gx tickets root)/system-health-indicator/mocks/03-tray-mock-variations.html`.

This epic builds on the disk-space epic (`docs/specs/disk-space.md` on `disk-space-impl`). Its
implementation tickets that touch the disk side block on `gx:disk-space-impl/06` (disk watcher) and
`gx:disk-space-impl/07` (disk status route).

## Problem Statement

A queue run can slow the machine down or fill its memory, and the person only finds out when
tickets crawl or fail. The web UI shows nothing about the gx server's machine. The disk-space epic
added a banner, but only once a disk is low. To check CPU or memory, the person opens Activity
Monitor or `top` on the server's machine. From a phone, they can't check it at all. When the
machine is busy, nothing tells them which agent or ticket is the cause.

## Solution

The web top bar gets a **Tray** on its right side, next to the Paused chip and the menu. The Tray
is one small button about 26px wide. It holds three thin vertical meters for the gx server's
machine: CPU, memory and disk, left to right, with a tiny C, M or D under each. A meter fills with
its value and takes the color of its state: grey for normal, amber for high, red for critical. A
meter with no reading is a hollow outline, so a missing value never looks normal.

Clicking or tapping the Tray opens one popover. It names the server's host and shows:

- CPU percent busy, its one-minute average and the thresholds, then the top 3 processes or agents
  by CPU.
- Memory percent used, GB used of total, the macOS pressure level, the thresholds, then the top 3
  by memory.
- Every watched disk with its free space, a bar, its state and the projects on it. A held disk
  says dispatch is held and points to the banner's "Clean now".
- A footer that says when the values were read.

A gx agent shows as one row with its whole process tree added up. The row is the ticket address,
linked to its ticket page, with the busiest process in grey: "gx:foo/03 · go test".

The Tray only shows. It holds nothing and sends no notice. The disk banner and "Clean now" stay as
the disk-space epic built them. The same Tray sits in the same place at phone width.

## User Stories

**The Tray**

1. As a person, I want a Tray on the right side of the web top bar on every page, so that I can see
   how the gx server's machine is doing from wherever I am in the web UI.
2. As a person, I want the Tray to show three meters, CPU, memory and disk, left to right, so that
   I can read all three at a glance.
3. As a person, I want a tiny C, M or D under each meter, so that I can tell them apart without
   opening the popover.
4. As a person, I want each meter to fill with its value (CPU percent busy, memory percent used,
   disk percent used on the worst watched disk), so that a fuller bar means a busier machine and a
   full bar always means trouble.
5. As a person, I want each meter and its letter colored by state (grey normal, amber high, red
   critical), so that a problem stands out without reading numbers.
6. As a person, I want a meter with no reading drawn as a hollow outline, so that a missing value
   never reads as normal.
7. As a person, I want the Tray to sit next to the Paused chip and the Offline dot, before the
   menu, so that it fits the top bar's existing order.
8. As a person on a phone, I want the same 26px Tray at 390px width, so that I can check the
   machine from my phone with no layout change.
9. As a person using a screen reader, I want the Tray button's label to name each meter's state, so
   that I get the same information without the colors.

**States and thresholds**

10. As a person, I want CPU to read high at 70% busy and critical at 90%, so that the meter turns
    amber when the machine is working hard and red when it is near its limit.
11. As a person, I want memory to read high at 80% used and critical at 90%, so that I see memory
    pressure before the machine starts to swap hard.
12. As a person, I want the CPU and memory states to use the average of the last minute of
    readings, so that a short spike does not turn a meter red.
13. As a person, I want a state to clear only when the average drops 5 points below its threshold,
    so that a meter near a line does not flicker.
14. As a person, I want the CPU and memory thresholds fixed in gx, not in `config.json`, so that I
    have nothing to set up for a display-only feature.
15. As a person, I want the disk meter to read normal above the warn threshold, high under the warn
    threshold and critical under the clean threshold or while dispatch is held, so that the Tray
    and the disk banner always agree.
16. As a person, I want the disk meter to show the worst watched disk, so that one low disk is
    never hidden behind a healthy one.
17. As a person, I want CPU read as percent busy between two readings, not as load average, so that
    the number matches what `top` and Activity Monitor show and has a top of 100%.
18. As a person, I want memory read as 100 minus the kernel's available percent, so that file
    cache does not make the meter look full.

**How values reach the web**

19. As a person, I want the Tray to update on its own when a meter's level or state changes, so that
    I never reload to see a change.
20. As a person, I want the Tray to cost almost nothing while the machine is steady, so that an
    open tab does not wake the browser every few seconds.
21. As a person, I want the meters to repaint from the cached snapshot after a reload, so that the
    Tray shows at once on a page iOS Safari dropped.
22. As a person, I want the server to read CPU and memory every 5 seconds whether or not a tab is
    open, so that the one-minute average is ready when I open the web UI.

**The popover**

23. As a person, I want clicking or tapping the Tray to open one popover with all three rows, so
    that one gesture shows every value.
24. As a person, I want Esc or a click outside to close the popover, so that it gets out of my way
    the usual way.
25. As a person, I want the popover header to name the gx server's host, so that I know which
    machine the values are from.
26. As a person, I want the CPU row to show the latest percent busy, the one-minute average and the
    thresholds, so that I see both the moment and the trend.
27. As a person, I want the memory row to show the percent used, GB used of total and the
    thresholds, so that I see the amount as well as the share.
28. As a person on macOS, I want the memory row to show the kernel's pressure level (normal, warn,
    critical), so that I see the pressure the percent alone can miss.
29. As a person, I want the disk row to list every watched disk with its free space, a bar and its
    state, plus the disk thresholds, so that I see each disk, not only the worst one.
30. As a person, I want a held disk's row to say dispatch is held for its projects and to point to
    the banner's "Clean now", so that I know where the fix is without a second button.
31. As a person, I want the popover's values to refresh every 5 seconds while it is open and to
    stop when it closes, so that I watch live numbers without a cost when I am not looking.
32. As a person, I want the footer to say how long ago CPU and memory were read, counting each
    second, and each disk entry older than 10 seconds to show its own age, so that I know how fresh
    every value is.
33. As a person, I want the age worked out from the server's clock, so that a wrong browser clock
    does not give a wrong age.
34. As a person, I want a meter with no reading to have its popover row say why (waiting for a
    second reading, not available in this build, or a read error), so that I know whether to wait.

**Cause lists**

35. As a person, I want the top 3 by CPU under the CPU row and the top 3 by memory under the memory
    row, so that I see what is using the machine.
36. As a person, I want the lists shown every time the popover is open, whatever the meter state,
    so that I can check the cause before a meter turns red.
37. As a person, I want each gx agent as one row with the CPU and memory of its whole process tree
    added up, so that a ticket's builds and tests count against that ticket.
38. As a person, I want an agent row to show its ticket address, linked to the ticket page, with
    the busiest process in grey ("gx:foo/03 · go test"), so that I can go straight to the ticket.
39. As a person, I want an agent run with no ticket (a design run, a plain session) to show its
    session name, so that I can tell which session it is.
40. As a person, I want a claude process gx did not start to show as "claude (not gx)", so that I
    can tell my own claude sessions from gx's agents.
41. As a person, I want every other process as its own row under its name, so that a non-gx hog
    shows up too.
42. As a person, I want process and agent CPU on the same whole-machine scale as the CPU row, so
    that the rows add up to at most the busy percent.
43. As a person on macOS, I want the CPU I cannot read (other users' processes) shown as one "other
    users" row, the machine's busy percent minus what gx could read, listed only when it makes the
    top 3, so that a system hog like `mds_stores` is not hidden.
44. As a person on Linux with `hidepid`, I want other users' processes handled the same way, so
    that both systems read alike.
45. As a person, I want the memory list to show the top 3 processes gx can read, with no "other
    users" row, so that the list never shows a made-up number.
46. As a person on macOS, I want process memory to be the footprint (Activity Monitor's "Memory"
    column), and RSS on Linux, so that the numbers match the tools I know.
47. As a person, I want each disk row to list the projects on that disk, so that I know which
    projects a low disk affects.
48. As a person, I want the lists to say "processes unavailable" when gx cannot read processes
    (a build without cgo, or a failed read), with the meters still working, so that one failure
    does not hide the rest.
49. As a person, I want the CPU lists to show "reading…" for about a second on the first open after
    a quiet spell, so that I see gx is taking the second reading, not that nothing is busy.
50. As a person on a phone, I want the same rows, with long names cut short with "…" and the numbers
    right-aligned, so that the popover fits 390px.
51. As a person, I want the process read to run only while a popover polls, so that the Tray costs
    nothing extra when nobody is looking.

**Offline**

52. As a person, I want the Tray to fade and keep its last levels when the web loses the server, so
    that I still see the last known state and know it is stale.
53. As a person, I want the popover, opened while offline, to keep its last fetched values with
    their age ("Offline. Values from 3m ago."), so that I know how old they are.
54. As a person, I want the Offline strip and the Offline dot to stay as they are, so that the
    Tray adds no second offline signal.

**CLI**

55. As a person or an agent, I want `gx server health --json` to print the same values and cause
    lists as the popover, so that the server API and the CLI stay in step (ADR 0028) and I can
    check the machine from a terminal.

## Implementation Decisions

**Reading CPU and memory (server)**

- A new system-reading source on the server `Config`, like the disk watcher's free-space source.
  The default reads the real machine. Tests pass a fake that returns set values and can change them
  between readings. A `Config` field sets the read interval; zero means 5 seconds.
- CPU: each read returns the kernel's cumulative CPU ticks. The health watcher keeps the last ticks
  and computes busy as `1 - delta(idle) / delta(total)`. There is no sleep inside a read. The first
  interval after start has no CPU value.
  - macOS: `host_statistics(HOST_CPU_LOAD_INFO)` (Mach) in a `darwin && cgo` file. The counters
    are 32-bit, so take deltas modulo 2^32. A `darwin && !cgo` build reports CPU as unavailable.
  - Linux: the first `cpu` line of `/proc/stat`. Idle is idle + iowait. Total is user, nice,
    system, idle, iowait, irq, softirq and steal. guest and guest_nice are already inside user and
    nice.
- Memory: percent used is 100 minus the kernel's available percent.
  - macOS: `kern.memorystatus_level` (0 to 100), `hw.memsize` for total bytes, and
    `kern.memorystatus_vm_pressure_level` (1 normal, 2 warn, 4 critical) for the pressure.
  - Linux: `MemAvailable / MemTotal` from `/proc/meminfo`. Linux has no pressure value; the popover
    leaves it out.
- No gopsutil. Use `golang.org/x/sys/unix` (already in `go.mod`) and `/proc`.
- ADR 0035: build tags only where code cannot compile on another OS (the Mach and libproc cgo
  calls, and the `unix.Sysctl*` calls, which do not exist on Linux). The `/proc/stat`,
  `/proc/meminfo` and `/proc/<pid>/stat` parsers live in plain files that compile on every OS,
  take their input as bytes or an `fs.FS`, and are tested on every OS. One selection function
  picks the backend from an injected `goos`.
- This is a new watcher next to the CPU watchdog (`cpuwatch`). The watchdog measures gx's own
  process for spike evidence; the two share nothing.

**States and levels**

- Three states for every meter: normal, high, critical. A fourth, unknown, means no reading.
- CPU: high at 70%, critical at 90%. Memory (percent used): high at 80%, critical at 90%. These are
  constants in code, not `config.json` keys.
- The CPU and memory states use the average of the readings in the last minute (12 readings at the
  5-second interval). A state rises when the average reaches its threshold. It clears only when the
  average drops 5 points below that threshold.
- macOS pressure never sets the memory state. It shows in the popover only.
- The level is the meter's fill in 5 steps of 20 points (0 to 4). The CPU and memory levels come
  from the one-minute average. A level changes only when the value moves 5 points past a step edge.
  The disk level comes from percent used on the worst disk, with the same 5-point rule.
- Unknown: the first interval after start (CPU), an unavailable backend (a macOS build without
  cgo), a failed read, or no watched disk yet. The state carries a reason code (waiting, unavailable,
  error) so the popover can say why. The implementer picks the codes.
- The disk state comes from the disk watcher's flags, never from new rules: held, or a reading
  under the clean threshold, is critical; warned is high; anything else is normal. The worst disk is
  the one with the worst state; ties go to the highest percent used.
- Percent used on a disk needs its total size. The disk watcher's reading today holds only free
  bytes, so the disk reading gains total bytes from the same statfs call. `DiskStatus` gains them
  too, so the disk status route reports them.

**Snapshot field and change event**

- The snapshot gets a server-filled `health` field, like `budget` and the disk list. It holds, for
  CPU, memory and disk: the level, the state (with its unknown reason) and the reading time.
- The server publishes a change event only when a level or a state changes. The web refetches the
  snapshot on it, as for any event. A busy machine causes a few refetches per hour, not per
  minute. The implementer picks the event name.
- The disk part reads the disk watcher's in-memory state through the same code that builds the
  disk status route, so the Tray and the banner never disagree. A disk status change already
  publishes an event; the disk level and state recompute on it and on each disk reading.
- The wire types are literal-valued so tygo generates real types (`make web-types`, no `any`).

**`GET /v1/health` and `gx server health`**

- A read-only route, `GET /v1/health`, in the route table, in the web mux's read switch, and in the
  `TestWeb_EverythingElseIs404` allowlist.
- Its CLI verb is `gx server health`, with `--json`, a thin client of the route (ADR 0028). It goes
  in `routeVerbs`. Without `--json` it prints the same values and lists as plain text; the
  implementer picks the layout. The CLI always calls the route with `wait` (see "Reading
  processes"), so it prints the CPU list even after a quiet spell.
- The response holds:
  - the host name (`os.Hostname`) and the server's current time;
  - CPU: latest percent busy, one-minute average, state, level, thresholds, reading time;
  - memory: latest percent used, used and total bytes, one-minute average, state, level,
    thresholds, the macOS pressure level when there is one, reading time;
  - every watched disk (from the disk watcher, the same data as the disk status route): id, name,
    free and total bytes, state, held, projects, reading time, plus the warn and clean thresholds;
  - the cause lists: top 3 by CPU and top 3 by memory, each row with its kind (agent, session,
    process, other users), its label, the busiest process name for agent rows, the ticket address
    when there is one, the CPU share and memory bytes; or a marker that processes are unavailable,
    or that the CPU list is still reading.
- The process read runs only on this route. A call reuses a process read taken less than 1 second
  ago, measured from the read's own time, not from the last call. So two tabs that poll together
  cost one read, and calls spaced 0.9 seconds apart still cause a new read every second. A `wait`
  call never gets a cached answer that carries the reading marker. A poll every 5 seconds always
  gets a fresh read. Two tabs out of step cost two reads, about 0.2% of a core. The implementer picks the
  cache shape.

**Reading processes (server)**

- A new process-reading source on the server `Config`. The default reads the real process table.
  Tests pass a fake table that can change between readings.
- macOS, in the `darwin && cgo` file: `proc_listpids(PROC_ALL_PIDS)`, then per pid
  `PROC_PIDT_SHORTBSDINFO` (parent pid, name, uid), `PROC_PIDTASKINFO` (cumulative CPU time in Mach
  units, converted with `mach_timebase_info`) and `proc_pid_rusage` for `ri_phys_footprint`.
  Other users' processes return EPERM for the CPU and memory calls; they count as unreadable.
  A `darwin && !cgo` build reports processes unavailable.
- Linux: one `/proc/<pid>/stat` read per pid: parent pid (field 4), utime + stime (14, 15), start
  time (22) and rss in pages (24). Split `comm` at the last `)`. Ticks per second are 100 in a pure
  Go build. A pid that vanished or is hidden by `hidepid` counts as unreadable.
- Never run `ps`.
- CPU per process needs two readings. The server keeps the last CPU time per (pid, start time). A
  pid reused with a new start time starts fresh.
- First read after a quiet spell: when there is no previous reading, or it is older than the
  cutoff, the route answers at once with the memory list and system values and marks the CPU list
  as reading. The server takes the second reading about 1 second later.
- Three `Config` fields set the timing so tests run in milliseconds: the second-read delay, the
  share window and the cutoff. Zero means 1 second, 1 second and 15 seconds. The cutoff must be
  longer than the popover's 5-second poll, or every poll would show "reading…".
- The route takes a `wait` option. With `wait`, a call that would get the reading marker blocks
  until the second reading is done, then answers with the full CPU list. A call that does not need
  to wait answers at once. The web's first call on open has no `wait`, so values and "reading…"
  show at once; its next call uses `wait` and gets the list as soon as it exists. The CLI always
  uses `wait`. No caller guesses a timer, so no fetch can land before the second reading.
- A `wait` call returns as soon as the second reading ends, whether it worked or not. A failed
  read gives the "processes unavailable" marker. The call stops when the request context ends.
  The server caps the wait at 5 seconds, then answers with the reading marker.
- Process names: `comm` is cut at 16 bytes on macOS, and claude shows as its version. For the rows
  that make a list, the server reads the full argv (`KERN_PROCARGS2` on macOS, same user only;
  `/proc/<pid>/cmdline` on Linux) and builds a short display name, using the first rule that
  matches:
  - a process the claude rule (see "Owners") matches: "claude";
  - tools with subcommands (go, npm, pnpm, yarn, bun, git, cargo, make): the base name of argv[0]
    plus the first argument that is not a flag, such as "go test" or "npm run";
  - interpreters (node, sh, bash, zsh, and any base name starting with `python`, such as
    `python3.12`): the base name of the script argument. With `-c`, `-e` or no script argument,
    the interpreter's own name;
  - anything else: the base name of argv[0];
  - with no argv (another user's process, or a failed read): `comm`.
- Scale: process and agent CPU is a share of the whole machine, the same scale as the CPU row. A
  process's share is its CPU time delta divided by the machine's total CPU time in the same window
  (the tick total delta, turned into seconds with the tick rate). So the rows and the busy total
  use one denominator, with no wall-clock time. A process using one full core on a 10-core machine
  reads 10%. The rows add up to at most the busy
  percent.
- The process read takes its own CPU tick reading at the same moments as the process table. The
  CPU list's busy total and "other users" use that window, not the watcher's 5-second tick, so the
  rows and the busy total cover the same time.
- "Other users" (CPU list only): the busy percent of the process read's window minus the summed
  CPU of every process gx could read, floored at zero. It is listed only when it would make the top 3. On a machine
  where gx read every process, the row never shows.
- The memory list has no "other users" row. It ranks the processes gx could read.
- Memory per row: footprint on macOS, RSS on Linux. An agent row adds up its tree.

**Owners: tying processes to gx agents**

- An optional runner interface (for example `ProcessOwner`, the implementer picks the name) returns
  the pids, with start times, that gx knows for a session:
  - the pty and headless native runners: the claude pid and holder pid from the agent's
    `meta.json`;
  - the herdr runner: the pane's shell pid from `herdr pane process-info`, cached once per pane,
    since a pane's shell pid never changes. herdr gives no start time, so when the pane is first
    cached the runner reads the shell pid's start time from the process reader and stores it with
    the pid;
  - web sessions: the holder pid and the claude pid from the holder ping. Today the ping returns
    only the child pid (`ChildPID`), so the ping is extended to also return the holder pid and the
    start times (Unix seconds) of the holder and the claude child, read with the process reader. A
    holder too old to send them gives no owner for that session;
  - the gx server: its own pid.
- Every known pid carries a start time. There is no match by pid alone.
- Each process read builds one pid-to-owner map. For each process, walk up the parent pids from the
  same read (no extra calls) until a known pid. Check start times so a reused pid never matches.
- Start times use one type everywhere in the owner walk: Unix seconds.
  - The macOS reader uses `pbi_start_tvsec`.
  - The Linux reader adds field 22 of `/proc/<pid>/stat`, divided by the tick rate, to `btime`
    from `/proc/stat`.
  - `meta.json` stores `pid_start` and `holder_pid_start` as `ps -o lstart` text
    (`nativerunner/procs.go`). `ProcessOwner` parses that text in the C locale and local time.
  - The compare allows 1 second of rounding.
  A process reparented to pid 1 has lost its link and shows under its own name.
- An owner maps to its label through the server's runs (`runs.json`: ticket address to runner and
  session) and session metadata (`Ticket`, `Name`). Never parse the iteration label.
  - A run or session with a ticket: the ticket address, linked to `/t/<address>`, with the busiest
    process of the tree in grey.
  - A run or session with no ticket: its session name.
  - A claude process with no gx owner: "claude (not gx)". A process is claude when argv[0]'s
    base name is `claude`, or its executable is under a `claude/versions/` folder (the native
    installer, where `comm` shows the version), or it is a node process running
    `@anthropic-ai/claude-code/cli.js`.
  - The gx server's own process: its own row, labelled "gx server". Its non-agent children (git,
    cleanup commands) are other processes and get their own rows.
  - Anything else: its display name.
- Agent rows rank by their summed CPU or memory, against single processes, in one list.

**Web Tray (TopBar)**

- The Tray is a new component in `webui/src/ui/`, rendered by `TopBar` on its right side: Paused
  chip, Offline dot, Tray, menu. `ConnectedTopBar` and every page that renders `TopBar` pass the
  snapshot's `health` field.
- One button, about 26px wide, three vertical meters with C, M, D under each, in the order CPU,
  memory, disk. Colors: normal grey (the mock's faint fill), high `tone.waiting`, critical the
  paused or deadlocked red. The letter takes its meter's color. Unknown is a hollow outline with no
  fill. The look follows the stored mock.
- Fill height comes from the level. The top step fills the bar. The implementer picks the exact
  height per step, as long as level 0 still shows a visible fill and never looks like unknown.
- The button's `aria-label` names each meter and its state, for example "System health: CPU high;
  memory normal; disk normal". The snapshot has no exact percent, so the label does not give one.
  The implementer writes the exact copy.
- Offline (the connection is lost): the Tray keeps the last levels from the cached snapshot and
  fades. The Offline dot and strip do not change.
- The Tray shows on phone width with no special layout.

**Web popover**

- Opens on click or tap, never on hover. Esc or a click outside closes it. It uses a Chakra
  popover anchored bottom-end, like the menu, at most 320px wide, and fits 390px.
- On open it calls `GET /v1/health`, then every 5 seconds, and stops when it closes. A "reading"
  marker on the CPU list triggers one extra call at once with `wait`, which returns when the list
  is ready.
- Rows: the header ("gx server" and the host name), CPU, memory, disk, footer. Each row has an
  icon, a name, a state tag when not normal, the value, a bar and a sub line, as in the mock.
  - CPU: "81% busy", sub line "1-minute average 76% · high at 70%, critical at 90%". Then its
    cause list.
  - Memory: "71% used", sub line "22.7 of 32 GB · macOS pressure: normal · high at 80%, critical
    at 90%". The pressure part shows only when the server sends one. Then its cause list.
  - Disk: the worst disk's free space as the value, then one entry per disk with its name, free
    space, a bar of percent used and its state color, and the projects on it. A held disk adds
    "Dispatch held for gx, dotfiles. Clean now is in the banner." The sub line gives the
    thresholds ("warns under 20G, cleans under 10G"). No button.
  - An unknown row says why in its sub line. The implementer writes the copy.
- Cause rows: label on the left, cut with "…" when long; numbers right-aligned. An agent row's
  label is a link to its ticket page, then " · " and the busiest process in grey. CPU rows show a
  percent; memory rows show bytes. "processes unavailable" and "reading…" replace a list's rows.
- Footer: "Updated 2s ago", from the CPU and memory reading time and the server's current time in
  the response, counted up once a second in the browser.
- A disk entry shows its own age ("read 45s ago") when its reading is more than 10 seconds older
  than the server's current time, since the disk watcher reads less often than the CPU and memory
  watcher.
- Offline: the popover keeps its last fetched values and the footer reads "Offline. Values from 3m
  ago.", counting up. Polling stops while offline and starts again on reconnect.
- The last `/v1/health` response goes into the page cache (`pageCache.ts`), so a reload offline
  still has values to show. With no cached response, the popover shows each row's state from the
  snapshot and says no values were fetched. The implementer writes the copy.

**Not done on purpose**

- No hold, notice or other action on CPU or memory.
- No `config.json` keys for the CPU and memory thresholds.
- No per-disk or per-worktree sizes. `gx:follow-ups/41` plans them.
- No Linux PSI pressure.
- No new pause kind and no ADR 0042 row: nothing pauses.

## Testing Decisions

A good test checks what a person or a client sees: the snapshot's `health` field, the change
events, the `/v1/health` response, the CLI output, the rendered Tray and popover. It does not check
the watcher's internal fields.

Seams:

1. **Server, through servertest.** Two new sources on the server `Config`: the system-reading source
   (CPU ticks, memory values, pressure) and the process-reading source (a process table). Each has a
   fake that returns set values and can change them between readings. A `Config` field sets the
   read interval, and three more set the process read timing, so tests run in milliseconds. The
   fake system source also returns a core count and a tick rate. Disk tests reuse the free-space fake from
   disk-space, with total bytes added; the file-backed source takes an optional total (`8G/500G`)
   and uses a fixed default total of 500G without one. Tests use `servertest.StartWithStore`, a temp store and
   `testutil.TempRepo`. They read the snapshot, follow the event stream and call `/v1/health`.
   Prior art: the disk watcher tests on `disk-space-impl` (`server/diskwatch_test.go` and its
   neighbours) and the budget snapshot tests.
2. **Parsers.** Table tests for the `/proc/stat`, `/proc/meminfo` and `/proc/<pid>/stat` parsers
   over byte inputs, run on every OS (ADR 0035), including a `comm` with spaces and `)`. Table
   tests for the state and level rules (thresholds, one-minute average, 5-point hysteresis, step
   edges) and for the owner walk and ranking over a fake process table (agent sum, busiest
   process, pid reuse, reparented process, "other users", "claude (not gx)"). Table tests for the
   display-name rule (each subcommand tool, each interpreter, `python3.12`, `sh -c`, `node -e`, an
   interpreter with no script, a claude match named "claude", no argv) and the claude rule (each
   of the three matches, and a non-claude node process). A test that a `meta.json` `lstart` value
   parses to the reader's start time for the same process. A holder test that the ping's holder pid
   and both start times match the reader's values, and that an older holder's ping reports them as
   not known. A herdr runner test that the shell pid's start time is read once, when the pane is
   first cached.
3. **CLI.** `cmd` tests for `gx server health` against a fake server, plus
   `TestRouteTableHasCLIVerbs` and `TestWeb_EverythingElseIs404`.
4. **Web.** Vitest for the Tray and the popover: every state of each meter, unknown, offline fade,
   the aria label, the popover rows, cause lists, "reading…", "processes unavailable", the agent
   link, the age footer and the offline footer. A new gallery entry for the Tray in each mock
   state. Prior art: `webui/src/ui/top-bar/TopBar.test.tsx`, the `TopBar` gallery entry, and the
   disk banner tests on `disk-space-impl`.
5. **Fixture.** A new `gxfixture` scenario, `health`, whose server reads system values and a
   process table from control files the flow can change at runtime, the way `disk-low` uses
   `GX_DISK_FREE_FILE`. It runs on the `disk-low` free-space file for its disk, with the optional
   total (`8G/500G`); `disk-low` keeps working on the default total. The scenario starts one fake
   agent on a ticket. After it starts the agent, the fixture writes the agent's known pids and start
   times into the process table file, with child rows under them, so the owner walk names the
   agent. It writes start times in the readers' Unix-seconds format, not the `meta.json` text. The implementer picks the file format and env var names.
6. **Real readers.** One test per real backend that runs on its own OS (cgo on macOS, `/proc` on
   Linux) and checks that a read returns sane values (CPU ticks rise, 0 to 100 percent, more than
   one process, the test's own pid present). It also compares its own pid's memory with
   `proc_pid_rusage` footprint on macOS and with `VmRSS` from `/proc/self/status` on Linux, within a
   small margin, and checks that its own start time matches its `ps -o lstart` text parsed the
   `ProcessOwner` way.

## Verification

A verify ticket runs these against a temporary gx server from `gxfixture`. "Go test" names the seam
from Testing Decisions that covers the story; the verify part reruns it. New flows take the next
free numbers in `webui/e2e/CATALOG.md` after the disk-space flows (72 onward); the implementer
assigns them.

New scenario to write: `health` (Testing seam 5). New flows to write:

- **Flow A, "Tray states":** scenario `health`. Set values for calm, then busy, then critical, then
  CPU unavailable. For each, check the three meters' state classes and fills on the Epics page, an
  epic page and a ticket page, at 1100px and 390px, with no reload between changes.
- **Flow B, "Tray popover":** scenario `health` in busy. Click the Tray. Check the header host, the
  three rows, the thresholds, the cause lists with the agent row linked to its ticket page and
  "other users", the disk entries with projects, and the "Updated" footer counting up. Change the
  CPU value and check the popover shows it within 5 seconds. Press Esc and check it closes. Tap it
  open at 390px and check rows fit with names cut short.
- **Flow C, "Tray offline":** scenario `health` in busy. Open the popover once and close it. Stop
  the fixture server. Check the Tray fades and keeps its levels, and the popover shows "Offline.
  Values from …" with the last values.
- **Flow D, "Held disk in the popover":** scenario `health` with the disk under the clean
  threshold. Open the popover; check the disk entry says dispatch is held and points to "Clean
  now", and the D meter is critical.

Entries:

1. Flow A (Tray on all three pages).
2. Flow A (three meters in order). Vitest (seam 4).
3. Vitest (seam 4): C, M, D under the meters. Mock state calm.
4. Flow A (fills change with the values). Go test (seam 1): levels follow the fake values; the disk
   level is percent used on the worst disk.
5. Flow A (state colors). Vitest (seam 4): the letter takes its meter's color. Mock states busy and
   critical.
6. Flow A (CPU unavailable step). Vitest (seam 4): unknown draws an outline. Mock state unknown.
7. Vitest (seam 4): order Paused chip, Offline dot, Tray, menu. Mock states busy and offline.
8. Flow A at 390px. Mock states calm-phone, busy-phone, critical-phone, unknown-phone,
   offline-phone.
9. Vitest (seam 4): the aria label names each meter's state.
10. Go test (seam 2): CPU 69 normal, 70 high, 90 critical. Go test (seam 1): the snapshot state.
11. Go test (seam 2): memory 79 normal, 80 high, 90 critical.
12. Go test (seam 1): a single high reading among a minute of low ones keeps the state normal.
13. Go test (seam 2): an average at 66 keeps high (threshold 70); one at 64 clears it.
14. Shell check: `rg` finds no CPU or memory threshold key in `docs/config-schema.json`. Go test
    (seam 2): the constants.
15. Go test (seam 1): with the disk fake, above warn is normal, under warn is high, under clean or
    held is critical, and the snapshot state matches the disk status route's flags.
16. Go test (seam 1): two disks, one warned; the disk meter reads high. Flow D.
17. Go test (seam 2): tick deltas give busy percent; a 32-bit wrap still gives the right value.
18. Go test (seam 2): the meminfo parser gives 100 minus `MemAvailable / MemTotal`. Go test
    (seam 6): the macOS backend's percent is in 0 to 100.
19. Flow A (no reload). Go test (seam 1): a level change publishes one event.
20. Go test (seam 1): fake readings that move within one step publish no event over many reads.
21. Vitest (seam 4): after a reload the Tray paints from the cached snapshot.
22. Go test (seam 1): with no client connected, the watcher reads on its interval and the one-minute
    average is ready on the first snapshot.
23. Flow B (one popover with all rows). Mock states busy and critical.
24. Flow B (Esc closes it). Vitest (seam 4): a click outside closes it.
25. Flow B (header host). CLI check: `gx server health --json` on the fixture prints the host name.
26. Flow B (CPU row). Vitest (seam 4).
27. Flow B (memory row). Vitest (seam 4).
28. Vitest (seam 4): the pressure shows when the response has one and is absent when not. Go test
    (seam 1): the response has pressure from the fake on darwin values.
29. Flow B (disk entries). Go test (seam 1): `/v1/health` lists every disk the disk status route
    lists, with total bytes.
30. Flow D. Vitest (seam 4): a held disk entry shows the held note and no button.
31. Flow B (value changes within 5 seconds). Vitest (seam 4): polling stops on close.
32. Flow B (footer counts up). Vitest (seam 4): the footer uses the CPU and memory reading time; a
    disk entry 45 seconds old shows "read 45s ago", one 5 seconds old shows no age.
33. Vitest (seam 4): a browser clock 1 hour off still shows "Updated 2s ago" from the server times.
34. Go test (seam 1): unknown reasons for the first interval and an unavailable source. Vitest
    (seam 4): the row says why.
35. Flow B (cause lists). Go test (seam 1): top 3 by CPU and by memory from the fake table.
36. Go test (seam 1): lists come back with every meter normal.
37. Go test (seam 2): an agent's tree adds up to one row.
38. Flow B (agent row link). Vitest (seam 4): label "gx:foo/03 · go test" links to the ticket page.
39. Go test (seam 2): a session without a ticket gives its session name.
40. Go test (seam 2): a claude with no owner gives "claude (not gx)", for each of the three claude
    rules.
41. Go test (seam 2): an unowned process gives its own row under the display-name rule; the gx
    server gives "gx server".
42. Go test (seam 2): a process at one full core on a 10-core fake reads 10%; the rows add up to at
    most the busy percent of the process read's own window. The test uses fake ticks, a fake core
    count and a fake tick rate, with no wall-clock time.
43. Go test (seam 2): unreadable processes give "other users" as busy minus the sum, shown only when
    it makes the top 3. Flow B (other users row).
44. Go test (seam 2): a vanished or hidden pid counts as unreadable, like EPERM.
45. Go test (seam 2): the memory list never has an "other users" row.
46. Go test (seam 6): the test's own row matches `proc_pid_rusage` footprint on macOS and `VmRSS`
    on Linux, within a small margin.
47. Flow B (projects on each disk).
48. Go test (seam 1): a failing process source gives the unavailable marker and the meters still
    read. Vitest (seam 4): "processes unavailable".
49. Go test (seam 1): the first call marks CPU as reading; a `wait` call made at once blocks until
    the second reading and has the CPU list; when the fake source fails the second read, the
    `wait` call returns the unavailable marker; a second read that never ends returns the reading
    marker after the cap. Vitest (seam 4): "reading…" and the extra `wait` call.
50. Flow B at 390px.
51. Go test (seam 1): with no `/v1/health` call, the fake process source records no reads; two
    callers within 1 second cause one read; calls spaced 0.9 seconds apart cause a second read;
    a poll every 5 seconds gets a fresh read each time.
52. Flow C (Tray fades, keeps levels). Mock state offline.
53. Flow C (popover offline footer). Vitest (seam 4): offline after a reload uses the cached
    response.
54. Flow C (Offline dot and strip unchanged). Mock states offline and offline-phone.
55. CLI check: `gx server health --json` on the fixture, run after a quiet spell, prints CPU,
    memory, disks and both lists; without `--json` it prints them as text. Go test (seam 3): the
    CLI calls the route with `wait`. `TestRouteTableHasCLIVerbs` and
    `TestWeb_EverythingElseIs404` pass.

**Mock states** (fixture scenario `health` for every line; the scenario does not exist yet and is
listed above):

- `$(gx tickets root)/system-health-indicator/mocks/03-tray-mock-variations.html`, calm, 1100px,
  parts: top-bar, tray, menu.
- `$(gx tickets root)/system-health-indicator/mocks/03-tray-mock-variations.html`, calm-phone,
  390px, parts: top-bar, tray, menu.
- `$(gx tickets root)/system-health-indicator/mocks/03-tray-mock-variations.html`, busy, 1100px,
  parts: top-bar, paused-chip, tray, menu, popover, popover-cpu, popover-memory, popover-disk.
- `$(gx tickets root)/system-health-indicator/mocks/03-tray-mock-variations.html`, busy-phone,
  390px, parts: top-bar, paused-chip, tray, menu, popover, popover-cpu, popover-memory,
  popover-disk.
- `$(gx tickets root)/system-health-indicator/mocks/03-tray-mock-variations.html`, critical,
  1100px, parts: top-bar, tray, menu, popover, popover-cpu, popover-memory, popover-disk.
- `$(gx tickets root)/system-health-indicator/mocks/03-tray-mock-variations.html`, critical-phone,
  390px, parts: top-bar, tray, menu, popover, popover-cpu, popover-memory, popover-disk.
- `$(gx tickets root)/system-health-indicator/mocks/03-tray-mock-variations.html`, unknown, 1100px,
  parts: top-bar, tray, menu.
- `$(gx tickets root)/system-health-indicator/mocks/03-tray-mock-variations.html`, unknown-phone,
  390px, parts: top-bar, tray, menu.
- `$(gx tickets root)/system-health-indicator/mocks/03-tray-mock-variations.html`, offline, 1100px,
  parts: top-bar, offline-dot, tray, menu.
- `$(gx tickets root)/system-health-indicator/mocks/03-tray-mock-variations.html`, offline-phone,
  390px, parts: top-bar, offline-dot, tray, menu.

The mock predates ticket 06, so its popover has no cause lists. For the popover parts, the look
check covers the header, the three rows and the footer; the cause lists are checked against this
spec (stories 35 to 50), not the mock.

## Out of Scope

- Acting on CPU or memory load (holding dispatch, chat notices). Its own epic if wanted.
- A Tray in the TUI.
- History and charts in the popover. It shows current values only.
- Per-disk and per-worktree sizes, and naming the biggest disk users (`gx:follow-ups/41`).
- `config.json` thresholds for CPU and memory.
- Linux PSI pressure.
- Platforms other than macOS and Linux.

## Further Notes

- Measured on a 10-core Mac with about 850 processes: three sysctls cost 70µs; a full libproc read
  with rusage costs about 5.5 ms of CPU. At one read per 5 seconds, the process read is about 0.1%
  of a core, and it runs only while a popover is open.
- macOS cannot read other users' processes without root: 220 of 847 here, including `mds_stores`,
  then the top CPU user. `ps` and `top` see them only because they are setuid root. The "other
  users" CPU row is how the popover accounts for them.
- The darwin release already builds with cgo (for FSEvents), and linux builds with
  `CGO_ENABLED=0`, so the Linux readers must be pure Go.
- The CPU tick read and the libproc read can share one `darwin && cgo` file or sit in two; the
  implementer picks the layout.
