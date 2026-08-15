# Idle cost: stop gx burning battery while doing nothing

## Problem Statement

gx sits open all day. It is a TUI you leave running — a Queue tab watching an overnight ralph-loop
run, a Tickets tab open in a split, a Status tab beside an editor. While it sits there showing a
picture that is not changing, it costs real battery.

This was not found by profiling gx. It was found from the outside: a power report flagged an idle gx
process — one that had been open about twelve hours with the Queue tab showing — as a top-ten daily
battery consumer on the machine, alongside WindowServer and Chrome. `top` on that process looked
innocent, which is exactly why it survived so long.

Two rounds of investigation found that an idle gx is never actually idle:

- It re-identifies the repository every two seconds by spawning five `git` subprocesses, on the UI
  goroutine, blocking the event loop for ~20 ms each time — roughly 2.5 process spawns per second,
  forever, with nothing on screen changing.
- It repaints on an unconditional 60 Hz timer regardless of whether anything changed. Each wakeup is
  cheap; sixty of them per second is what keeps the process out of deep idle.
- It reloads `.scratch` from disk every two seconds whether or not anything wrote to it.
- Some spinners keep animating at 10 Hz in dialogs that are purely waiting for a keypress. Leave a
  failed pull dialog open and gx renders ten times a second until you close it.

Measured idle, doing nothing: 0.47% CPU on the Status tab, 0.57% on Worktrees, 1.03% on Tickets.

Alongside the cost there is a correctness bug in the same machinery: the periodic reload dies the
first time you switch away from the Tickets or Queue tab and never restarts, so those tabs silently
stop refreshing for the rest of the session. The user is not told; the tab just quietly goes stale.

## Solution

Make idle actually idle, and make "notice that the ticket files changed" event-driven rather than a
blind timer.

From the user's perspective:

- gx open and untouched costs approximately nothing, and does not appear in a battery report.
- Ticket and Queue changes made outside this gx process — by another gx, a ralph-loop in a sibling
  worktree, or a hand edit — appear in well under a second instead of up to two.
- The Tickets and Queue tabs keep refreshing after you switch tabs, which today they do not.
- Dialogs waiting on a keypress stop animating.
- Nothing about the UI feels slower.

The mechanism has two halves with different jobs, and the distinction is the heart of this spec. The
**scratch watch** notices changes via filesystem events — fast, but best-effort: on macOS it is
kqueue-based, non-recursive, can miss events, and can fail to start at all. The **scratch poll** is a
slow periodic reload that runs anyway. The watch decides how fast a change is noticed; the poll
decides whether it is noticed at all. Both are now defined in `CONTEXT.md`, and the reasoning is
recorded in ADR 0025 — specifically so that nobody later deletes the "redundant" poll and converts a
guarantee into a staleness bug that only reproduces on somebody else's filesystem.

The repo-identity work has its own ADR (0024): `git.FindRepo` becomes a permanent per-process memo,
which means repo identity is treated as immutable for the life of the process.

## User Stories

1. As someone who leaves gx open all day on a laptop, I want an idle gx to cost approximately no
   battery, so that I do not have to close it to get through an afternoon unplugged.
2. As someone auditing what is draining my battery, I want gx to be absent from the top consumers
   list, so that I can trust it as background furniture.
3. As a user with the Queue tab open watching an overnight run, I want that tab to be as cheap when
   nothing is happening as any other tab, so that monitoring a run does not cost more than the run.
4. As a user, I want gx to spawn no subprocesses on a repeating schedule while idle, so that it does
   not show up in process monitors as constantly forking.
5. As a user, I want gx's event loop never blocked by synchronous git work, so that a keypress is
   never waiting behind a subprocess.
6. As a user, I want the UI to feel exactly as responsive as before, so that the energy fix is not
   paid for in latency I can perceive.
7. As a user running a ralph-loop in another worktree, I want ticket status changes to appear in my
   Queue tab almost immediately, so that I can watch progress without pressing reload.
8. As a user who edited a ticket file by hand in an editor, I want the Tickets tab to pick it up
   without my pressing reload, so that the tab is never lying to me about what is on disk.
9. As a user, I want that pickup to happen even if the filesystem watcher missed the event or never
   started, so that "eventually correct" is a property I can rely on rather than hope for.
10. As a user on an unusual filesystem or with a low file-descriptor limit, I want gx to work
    normally when the watcher cannot start, so that a best-effort optimization is never a hard
    requirement.
11. As a user, I do not want to be nagged about a watcher that failed to start, so that a degraded
    optimization does not become my problem to fix.
12. As a user during a busy ralph-loop run, when `.scratch` is being written constantly, I want gx to
    coalesce that into a bounded reload rate, so that event-driven refresh is not more expensive than
    the timer it replaced.
13. As a user, I want to switch from Tickets to another tab and back and still have the tab
    refreshing, so that the tab does not silently go stale for the rest of the session.
14. As a user, I want a tab that is not visible to do no periodic work at all, so that having many
    tabs open costs no more than having one.
15. As a user, I want repeated tab switching not to accumulate duplicate refresh loops, so that the
    refresh rate does not creep up the longer I use the app.
16. As a user with both the Tickets and Queue tabs alive, I want each to refresh at its own rate, so
    that one tab's timer cannot double the other's work.
17. As a user who left a failed pull dialog open, I want gx to stop re-rendering, so that an
    unattended error state costs nothing.
18. As a user sitting on a stash confirmation prompt, I want gx to be idle while it waits for my
    answer, so that thinking time is free.
19. As a user watching a pull or push actually run, I still want the spinner to animate, so that I
    can tell the difference between working and hung.
20. As a user of every gx TUI command — worktrees, status, log, show, stash, prs, tickets — I want
    the same reduced idle cost, so that the fix is not limited to the tab that happened to be
    profiled.
21. As a developer adding a new gx TUI entry point, I want the frame-rate cap applied automatically,
    so that I cannot reintroduce the 60 Hz default by forgetting an option.
22. As a developer, I want a test that fails if a subprocess is reintroduced into the periodic reload
    path, so that this specific regression cannot come back silently.
23. As a developer, I want repo identification to be effectively free after the first call, so that
    I do not have to think about where in the codebase it is safe to call.
24. As a developer, I want the fact that repo identity is cached forever written down where I will
    find it, so that a stale main-branch name at 2am is explicable rather than baffling.
25. As a developer, I want a way to reset that cache in tests, so that cached identity does not leak
    between test cases.
26. As a developer reading the code later, I want the slow backstop poll to be visibly deliberate, so
    that I do not delete it as dead code next to a working watcher.
27. As a maintainer, I want correctness tests to pass with the watcher disabled, so that the test
    suite proves the guarantee rather than the optimization.
28. As a maintainer, I want at most one test that depends on real filesystem event timing, so that
    the suite does not acquire a new class of flake.
29. As a maintainer, I want the watcher's goroutine to exit promptly when its tab is deactivated, so
    that opening and closing tabs does not leak.
30. As a user with a second gx attached to the queue, I want the "someone else holds the attach lock"
    check not to fork a process on every reload, so that the multi-process case is not the expensive
    one.
31. As a user, I want the Queue's rendered rows to be validated in constant time rather than by deep
    comparison of every ticket body, so that rendering cost does not grow with the size of my ticket
    backlog.
32. As a user with a large backlog, I want the Queue to stay smooth as epics accumulate, so that
    using the tool more does not make it worse.
33. As a maintainer, I want the "after" numbers recorded next to the "before" numbers, so that the
    claim that this worked is evidence rather than assertion.
34. As a maintainer, I want an explicit acceptance bar checked at the end, so that "we made it
    better" is not allowed to stand in for "we hit the target".
35. As a maintainer, I want any missed target written down honestly rather than the bar relaxed, so
    that the record stays trustworthy.
36. As a developer working on another TUI later, I want the invariants and the measurement method
    captured in a reusable skill, so that the next app does not have to be found by a battery report.

## Implementation Decisions

**Optimization target.** The primary metric is *periodic wakeups and process spawns while idle*, not
CPU percent. A cheap-but-frequent timer is what keeps a process out of deep idle, so CPU% understates
it and instantaneous `top` readings hide a 2-second periodic cost entirely. The acceptance bar:
zero subprocesses spawned on a repeating schedule while idle, steady-state timer rate at or below
20 Hz, idle CPU at or below 0.15% (from 0.47%–1.03%).

**Repo identity is cached per process (ADR 0024).** `git.FindRepo` memoizes per cleaned absolute
path for the life of the process — no TTL, no invalidation, safe for concurrent use. A TTL was
rejected outright: it reintroduces exactly the periodic subprocess spawning being removed. The
accepted consequence is that repo identity is immutable per process; the only field that can go
stale is the main-branch name, and only if `origin/HEAD` is repointed mid-session, whose recovery is
restarting gx. Worktrees created or removed at runtime are not a problem — a new worktree is a new
cache key, a removed one leaves a harmless unreferenced entry. `git.ResetRepoCache()` is exported for
tests only.

**`FindRepo` hands out copies.** A permanent memo would otherwise give every caller in the process
the same pointer, converting a harmless local mutation into process-wide corruption plus a data
race. No production code mutates a `Repo` today; returning a copy per call keeps that true by
construction.

**Repo identification is also made cheaper at the source, but not by one call.** `IdentifyDir`
batches `--git-dir`, `--is-inside-work-tree` and `--git-common-dir` into one `rev-parse`, plus one
more for `--show-toplevel` only when inside a work tree. All four cannot share an invocation:
`--show-toplevel` fails the whole command with exit 128 in a bare repo, and the command runner
discards stdout on a nonzero exit — and gx's own canonical root is a bare repo. This takes the
worktree case from four invocations to two and the bare case from two to one. It matters for callers
outside the UI: the ralph-loop worktree add/remove path pays the uncached cost per worktree.

**Consumers resolve their scratch directory once.** The Tickets and Queue models resolve it at
construction rather than at the start of every reload. The memo alone would have fixed the cost, but
the reload path should not be *asking* — the worktree root is fixed when the model is built, so the
answer cannot change. This also removes the synchronous git work from the update path, where it was
blocking the event loop rather than running inside the returned command.

**The renderer is capped at 15 fps.** Applied through a single shared program constructor covering
every TUI entry point, so a future entry point cannot omit it. 15 fps costs at most ~67 ms of paint
latency; input is not gated by the render ticker, and the fastest thing on screen is a 10 Hz spinner,
so 15 still oversamples it. Deliberately *not* user-configurable — a knob here is a support burden
with no real use case, and the reasoning belongs in a doc comment rather than a config file.

**Scratch watch and scratch poll are two named domain concepts, not one mechanism.** Both are defined
in `CONTEXT.md`. The watch is event-driven and best-effort; the poll is a slow periodic reload and is
the correctness guarantee. ADR 0025 records why the poll survives the arrival of a working watcher.
Both are scoped to their tab's activation: started when the tab is activated, allowed to stop when it
is deactivated.

**The watcher owns its own rate control.** Raw filesystem events are unusable directly, because a
live ralph-loop run writes to `.scratch` continuously — naive event-driven reloading would be *more*
expensive than the 2-second timer it replaces. The watcher applies a 300 ms trailing debounce and a
ceiling of one emitted signal per second, so a burst collapses to one reload and a sustained write
storm cannot exceed 1 Hz.

**The watch set is explicit and re-synced.** The macOS mechanism is not recursive, so the component
watches the scratch root plus each epic directory and re-syncs that set after each load — newly
created epic directories start being watched, deleted ones are dropped.

**Watcher startup failure is a supported state.** Failing to start is reported, not fatal: gx logs
once and runs on the poll alone. Logging once per activation rather than once per process would turn
a degraded optimization into a recurring annoyance.

**The backup poll interval moves from 2 s to 30 s, per tab, as each tab gains its watch.** It is a
correctness backstop, not a latency knob — tuning it up is safe, deleting it is not. The interval is
today a single shared constant; it becomes per-tab specifically so that no tab is ever slow-polling
without an event source during the window between the two wiring tickets.

**Watch and poll are stopped by a direct call, not by a message.** The page-deactivation hook is a
value receiver returning only a command, and the shell routes messages to the *active* page — so a
message emitted on deactivation is delivered to the incoming page, and the outgoing model can never
mutate itself in response. The watcher is therefore a handle the model holds, closed directly from
the deactivation hook, with `Close` required to be idempotent and nil-safe because the hook cannot
clear the field it just closed.

**Adaptive polling was considered and rejected.** An mtime pre-check before each load, and a slower
interval when no run is in flight, were both on the table as ways to cheapen the 2-second poll. With
a watcher plus a 30 s backstop they buy nothing and add a second staleness story, so they are not
being built.

**Refresh lifecycle is tied to activation, with per-tab messages and a generation token.** The
existing bug has two causes, both fixed: the app shell routes messages to the active page only, so a
pending tick is delivered to a page that ignores it and nothing rearms; and the "already started"
flag is a process-lifetime latch, so returning to the tab never restarts the loop. The two tabs also
shared one tick message type, letting a tick started by one rearm on the other and doubling the
effective rate — each tab gets its own.

Two things make the obvious fix wrong, and both are decided here. **Ticks are not cancellable**: one
armed before you switched away still arrives when you switch back, while activation arms a fresh
one, so a naive "restart on activation" accumulates a loop per switch cycle — the epic's own metric,
inverted. Each model therefore carries a generation counter, bumped on activation and carried in the
tick message; a stale tick is dropped. And **arming must happen in exactly one place** — activation —
which means the shell must also fire activation for the tab gx starts on. It does not today: the
initial page is constructed directly and marked initialised without going through the tab-switch
path. Without that shell change, moving arming onto activation would leave `gx tickets` and
`gx queue` with no poll at all until the user switched tabs, which is strictly worse than the bug
being fixed and invisible to any test phrased around switching.

**Spinner rearm is gated on work in flight.** `ui/pull` and `ui/push` rearm unconditionally today,
including in wait-for-user phases. They are brought in line with `ui/bump` and `ui/amend`, which
already return no command when not running.

**Foreign-attachment liveness is cached ~10 s.** Checking whether another gx holds the attach lock
forks `ps` to test the holder's liveness. A self-attached instance already short-circuits before
touching the lock file, so this only bites when another gx really is attached — but then it is one
fork per reload. Correctness is unaffected: a foreign attachment appearing or disappearing is already
only observed on a reload boundary.

**The Queue entry cache's backlog-sized comparison becomes O(1) — not the whole validation.** The
rendered-entry cache deep-compares three inputs on every render; only one of them, the epic list
including every parsed ticket body, grows with the backlog. That one is replaced by a generation
counter bumped wherever the epic list is assigned — a counter rather than a content hash,
specifically because a counter cannot get slower as the backlog grows. The other two, the
checked-ticket set and the collapsed-ID set, are left deep-compared: both are sized by user
selections rather than by the backlog, and the checked set is rebuilt fresh from the queue store on
every message so identity comparison would not work anyway. The claim is deliberately scoped to the
epic list rather than to "validation is O(1)". The cache must still survive a reload of unchanged
data — that is the reason it exists.

**Sequencing.** Published as the `idle-cost` epic. Plumbing is separated from consumers: the git-side
memo lands before the model that consumes it, and the watcher component lands before the UI wiring,
which is itself split Queue-first then Tickets so that one tab establishes the lifecycle shapes and
the other copies them. The frame-rate cap, the lifecycle fix, and the spinner gate are independent of
everything else. The measurement ticket is blocked by all implementation tickets, and the
skill-writing ticket by the measurement — the skill is written from numbers that actually happened
rather than from remembered advice. The skill ticket writes only into a separate repository and is
therefore marked commitless, and must report itself finished as commitless or it parks for a human.

## Testing Decisions

A good test here asserts externally observable behavior — what commands a model returns, how many
times a seam is consulted, whether a goroutine exits — and never the shape of the internals. Two
specific hazards shape the choices below: this epic's whole subject is *cost*, which is normally
invisible to tests, and its main new dependency is *filesystem event timing*, which is the classic
source of flakes.

**Cost is tested by counting, not by timing.** The invariant "the reload path does no process work"
is asserted by counting calls through a seam, never by measuring elapsed time. Concretely: the
scratch-dir lookup becomes an injectable package var in `ui/tickets` and a test drives N reload
cycles and asserts it resolved exactly once. This mirrors the existing `processStartTime` package
var, which already exists in this package precisely so tests can simulate live, dead, and reused pids
without spawning real processes — the same shape, reused rather than invented.

**Modules under test.** The `git` package (memo behavior, cache reset, and `IdentifyDir`'s results
for a regular repo, a linked worktree, and a bare repo — the case most at risk from collapsing four
queries into one). The `ui/tickets` Tickets and Queue models (resolve-once invariant, activation
lifecycle, per-tab tick isolation, watch-driven reload, poll-only reload, foreign-attach caching,
entry-cache generation). `ui/pull` and `ui/push` (rearm in running vs. wait-for-user phases). The new
watcher component (debounce, rate ceiling, watch-set re-sync, start failure, shutdown). The shared
program constructor in `cmd`.

**The watcher is tested against a fake event source.** All debounce, ceiling, and re-sync logic lives
on our side of a small interface, so those tests are deterministic and use no sleeps. Exactly one
integration test uses the real filesystem watcher — touch a file, assert a reload within a generous
bound — and it is skippable. One, deliberately: this repo already fights an intermittent ralph-loop
test flake, and the point of the interface is that a second timing-dependent test would be testing
the library rather than gx.

**Correctness tests must pass with the watcher disabled.** This is the testable form of ADR 0025. A
test that only passes when events fire is testing the optimization instead of the guarantee, and is
wrong even if it is green.

**Goroutine shutdown is tested under a bound.** Per the repo's Go conventions, any new long-lived
goroutine gets an explicit shutdown-under-timeout test — signal shutdown, assert exit within a short
bound via a channel close and a select, not a bare sleep. The watcher is the new long-lived goroutine
here.

**Generation-counter coverage follows the mutation paths.** A missed bump where the epic list is
assigned is the one way the O(1) cache validation goes silently wrong — showing stale rows rather
than crashing — so the tests enumerate the assignment paths rather than testing the counter in
isolation.

**Measurement is not automated.** Idle CPU cannot be measured from inside `go test` without measuring
the test harness. The final numbers are produced by re-running, by hand, the procedure recorded in
`docs/cpu-usage-findings.md`: the app under a pty, accumulated CPU time sampled with `ps -o time=`
over a fixed window (not instantaneous `top`, which is what hid this for months), stacks via
`sample`, child-process spawns detected by polling the process table and filtering on parent pid, and
an A/B against a variant binary. That ticket's deliverable is the updated document, verified by
reading it.

## Out of Scope

- Any further reduction of the reload's disk work. Once the subprocess spawning is gone, a reload is
  a couple of milliseconds of disk work at 0.033 Hz; an mtime pre-check would be optimizing noise.
- Making the frame rate, the poll interval, or the debounce window user-configurable.
- Changing what the Tickets or Queue tabs display, how rows are laid out, or any Queue behavior
  beyond refresh timing and cache validation.
- The repo-epoch / auto-reload system. `.scratch` changes are invisible to the epoch by nature — that
  is why the poll exists — and this epic does not attempt to bring them under it.
- Cross-process notification of scratch changes (a socket, a lock-file heartbeat, an explicit signal
  from ralph-loop to attached gx processes). Filesystem events plus a backstop poll are sufficient at
  the observed write rates.
- Windows and Linux specifics. The watcher library is cross-platform and nothing here is
  macOS-only by construction, but the measurements, the acceptance bar, and the non-recursive-watch
  reasoning are all from macOS.
- Reducing bubbletea's renderer cost below what the frame-rate option provides. A dirty-region or
  demand-driven renderer is upstream's business.
- The unrelated untracked files in this working tree, which predate this work.

## Further Notes

The 1.03% Tickets-tab number and the 0.57% baseline differ almost entirely by the git subprocess
spawning; the baseline itself is almost entirely the 60 Hz ticker, confirmed by sampling an idle
process and finding nothing but the renderer's own view-comparison and flush frames. A 10 fps variant
binary measured 0.57% → 0.12%, which is the evidence behind the frame-rate decision; 15 was chosen
over 10 for headroom above the 10 Hz spinners.

An earlier commit on this branch already removed two unconditional render loops — a 1 Hz full
re-render on the Status tab that existed only as a test heartbeat, and a full Queue tree rebuild on
every render. That work is the reason the Status tab now measures at baseline, and the reason the
remaining Queue cost is validation rather than rebuilding. This epic is the second pass, informed by
profiling the fixed binary rather than by code reading.

Several findings here are a single mistake wearing different clothes: work that repeats on a timer
regardless of whether there is anything to do. The reload, the renderer, the spinners, and the
liveness check are four instances. That is the observation the reusable skill in the last ticket
exists to carry forward, along with the measurement trap that hid all of it — instantaneous CPU
readouts make a 2-second periodic cost look like zero, and only the accumulated-time delta over a
fixed window shows it.
