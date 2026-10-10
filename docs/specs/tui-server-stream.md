# TUI server stream: one app-level subscription

## Problem Statement

Since v0.31.0 the `gx server` daemon is the only scheduler, and the TUI shows its state on two
tabs. Both tabs go stale:

- The **Queue tab** stops updating by itself. Only `R` refreshes it.
- The **Tickets tab** stops getting live events once you switch away from it, and stays stale after
  you switch back, until something forces a re-snapshot.

Both have one cause. Each tab keeps a loop alive by sending a message to itself: the Queue tab's
2s poll tick, and the Tickets tab's "read the next event" chain. The app shell delivers messages
only to the active tab. When the loop's message arrives while another tab is open, it is dropped
and the loop dies for good. A temporary test confirmed the Tickets case: with the Tickets tab open,
every event is read; after switching to Queue, the pending read's event is handed to the Queue tab,
no further read is started, and switching back does not restart it.

The same shape exists in three more page-owned loops: the Queue tab's running spinner, the Queue
tab's own down-mode probe, and the Tickets tab's down-mode store fallback (watch plus slow poll).

The Queue tab also polls a full server snapshot plus the queue every 2s, even when nothing changes,
and the two tabs keep separate copies of server state that can disagree. The queue's mode (running,
paused, draining) is not in the snapshot at all: the Queue tab only knows it from its own key
results, so it shows the wrong pause/resume action after another client changes the mode.

## Solution

The app shell owns one server subscription for the whole TUI: it takes the snapshot, reads the
event stream, applies events to one shared server state, and handles gaps, re-snapshots and
reconnects. Because the app shell sees every message whichever tab is open, its loop cannot be
dropped by a tab switch. The app shell also owns every other loop that must keep running while a
tab is hidden: the server probe and the down-mode store watch.

The Tickets and Queue tabs stop talking to the event stream. The app shell gives the open tab the
shared state on every change, and gives a tab the current state when you switch to it. The Queue
tab's 2s poll is removed. The snapshot gains the queue mode, so both tabs show it correctly.

## User Stories

1. As a gx user, I want the Queue tab to update by itself while I watch it, so that I see tickets
   start, finish and park without pressing `R`.
2. As a gx user, I want the Queue tab to show current state when I switch to it from any other
   tab, so that I never act on a stale queue.
3. As a gx user, I want the Tickets tab to show current state when I come back to it after
   visiting another tab, so that statuses and live columns are right.
4. As a gx user, I want the Tickets and Queue tabs to agree on every ticket's status, so that I am
   not confused by two different answers.
5. As a gx user, I want queue changes (add, remove, move, replace) to show on both tabs, whichever
   tab or client made them, so that my edit is visible everywhere.
6. As a gx user, I want the queue's mode (running, paused, draining) to show correctly on the Queue
   tab even when another client changed it, so that the pause/resume key does what I expect.
7. As a gx user, I want running tickets' spinners and timers on the Queue tab to start and stop as
   the server claims and finishes them, including claims that happened while the tab was hidden,
   so that I can see what is running now.
8. As a gx user, I want the Queue header's spend and the root rows' cost to follow the server's
   budget as it changes, so that I can see spend without refreshing.
9. As a gx user, I want the "herdr is down" state to show and clear on both tabs as the server
   reports it, so that I know when agents cannot start.
10. As a gx user, I want the TUI to recover by itself after the server restarts or drops the
    connection, so that I don't have to restart gx.
11. As a gx user, I want the TUI to recover by itself after missing events (a sequence gap), so
    that the state I see is never silently wrong.
12. As a gx user, I want the server-down banner and down mode to keep working on both tabs, so that
    I still know when the server is unreachable and how to start it.
13. As a gx user, I want the Tickets tab's down-mode view to keep following the store after I visit
    another tab, so that down mode does not go stale either.
14. As a gx user, I want the Queue tab to show the down banner when gx has no server client at all,
    so that it behaves the same as "server down".
15. As a gx user, I want the read-only (version mismatch) state to keep working as it does today,
    so that I can't write through an incompatible server.
16. As a gx user, I want `R` to keep forcing a fresh load, so that I have a manual way out if
    anything looks wrong.
17. As a gx user, I want the TUI to make no snapshot or queue fetch while idle (only the 2s handshake
    probe runs), so that leaving it open does not drain my battery.
18. As a gx user, I want a tab I have never opened to show current state the first time I open it,
    so that lazy tab creation does not show old data.
19. As a gx user, I want my Tickets tab project scope (`tp`) to survive every update, so that the
    view does not jump back to another scope.
20. As a gx developer, I want one place that owns the snapshot, stream, gap and reconnect logic, so
    that I fix stream bugs once.
21. As a gx developer, I want a rule that loops which must run while a tab is hidden live in the app
    shell, so that this bug class cannot come back.
22. As a gx developer, I want app-shell tests that switch tabs while events arrive, so that a
    regression is caught by `make test`.

## Implementation Decisions

- **One owner.** The app shell owns the server subscription: snapshot, event read loop, the shared
  `viewmodel.State`, applying events with the viewmodel reducer, and acting on its effects. Only one
  event stream is open per TUI process. A newer subscription always cancels the older one, and
  messages from a cancelled stream are ignored (the rule `e7987621` added to the Tickets tab, moved
  to the owner).
- **Freshness rule.** Events carry only a type and an address, not claim times, budget or cost. So
  the reducer asks for a re-snapshot after every event except two kinds:
  - `queue-changed` re-snapshots (for the queue mode) and also refetches the queue order;
  - `herdr-unavailable` / `herdr-available` are applied in place.

  This changes the reducer's effects: events it used to apply in place (claimed, started, done,
  cancelled, parked) and events it ignored (explain verdict change, root completed, root parked,
  land rolled back, nudged) now ask for a re-snapshot. A burst of events may coalesce into one
  re-snapshot.
- **Queue mode in the snapshot.** The server adds the queue mode (running, paused, draining) to
  the snapshot, filled in by the handler like budget and herdr state. A mode change already
  publishes `queue-changed`. The shared state carries the mode, and the Queue tab reads it from
  there instead of from its own key results. This is an additive change to the snapshot.
- **Subscription lifecycle.** The app shell takes the first snapshot and subscribes on the first
  probe result that is not down (up or read-only); startup goes from "unknown" to up without a
  down-line crossing, so this must not wait for one. When the link goes down, the stream is
  cancelled. When it comes back, the shell re-snapshots and resubscribes.
  - **Read-only:** the shell keeps reading events, as the Tickets tab does today. Only writes are
    blocked.
  - **Failed snapshot:** one toast per failure streak, then a retry on the next probe tick while
    the link is still up. There is no separate retry timer.
  - **Herdr state:** tabs use the stream's herdr state. The tab-bar server indicator keeps the
    probe's value.
- **Toasts move to the app shell.** The "X parked" toast, the "server snapshot: …" error toast
  and the first-load "unregistered project" hint come from the shell, not the Tickets tab. Park
  toasts therefore show even if the Tickets tab was never opened. Only events applied from the
  stream make toasts. A snapshot never does, as today.
- **Writes.** Tabs no longer fetch the queue or a snapshot after a write (enqueue, replace, remove,
  status changes, actions). They rely on the events the write causes. Relaunch publishes "claimed".
  Approve edits the ticket file, and the server's store watch turns that into `ticket-changed`. A
  write found to publish no event is a server bug, fixed by adding the publish on the server.
  Write-result toasts stay on the tab.
- **Page construction.** The Tickets and Queue tabs are built lazily on first open, and rebuilt when
  the worktree context changes. Both paths get the current shared state at build time, and `Init`
  no longer fetches. Before the first snapshot arrives, a tab shows "loading…", not an empty
  queue. These tabs are never opened as history pages.
- **`R` (forced refresh).** With the link up, `R` on either tab asks the app shell for a fresh
  snapshot plus the queue. In down mode, the Tickets tab reloads from disk as today, and the Queue
  tab asks the shell to probe the server.
- **Delivery to tabs: active tab only.** After every change to the shared state, the app shell
  delivers it to the active page only, if that page uses server state (Tickets or Queue). On every
  tab switch, it delivers the current state to the newly active page. Hidden pages may be stale;
  nobody sees them. Every command a page returns therefore runs while that page is on screen.
- **Loop rule.** Any loop that must keep running while a tab is hidden belongs to the app shell:
  the event stream, the server probe and the down-mode store watch. A page may own a loop only if it
  matters just while the page is on screen (for example the Queue spinner), and it must restart that
  loop when the page is activated. The Queue tab's own down-mode probe and retry are removed in
  favour of the shell's link state.
- **Down-mode store watch.** While the server is down, the app shell runs the store watch and slow
  poll that the Tickets tab runs today, and tells the active page that the store changed. A page
  reloads from disk when told and when it is activated in down mode. The Queue tab keeps its down
  banner and empty rows in down mode.
- **Project scope.** `CwdProject` and `AllProjects` move out of the shared state into the Tickets
  tab. `ScopedTickets` takes the scope as an argument. The Queue tab keeps its own project filter.
- **Tickets tab.** Its own snapshot, subscribe, event and stream-ended handling is removed. It
  takes rows, iterations, pending verdicts, budget and herdr state from the delivered state. Server
  writes (enqueue, replace, status changes, actions) stay on the tab and go through the server API
  as today.
- **Queue tab.** Its 2s poll, its own snapshot load and its disk-read branch are removed. With no
  server client it shows the same down banner as "server down". It builds its rows, queue order,
  claim times, budget, herdr state and queue mode from the delivered state. Running state
  (spinners, timers, conflict-resolution phase) is derived from the delivered state as it is today
  from the poll.

## Testing Decisions

- A good test drives the app shell with messages and a fake server client, and checks what the user
  sees or what the server is asked for. It does not check which internal message carried the data.
- **Main seam: the app shell `Model` with a fake server client.** The fake returns a snapshot and an
  event channel the test controls. Cover at least:
  - events keep being read while the Queue tab, the Tickets tab, or an unrelated tab is open;
  - a tab opened after events arrived shows the new state, including a tab never opened before;
  - a claim that arrives while the Queue tab is hidden shows a running spinner and timer once the
    tab is opened;
  - a queue mode change from another client shows on the Queue tab;
  - a sequence gap and a closed stream each lead to one re-snapshot and a new subscription;
  - a link change to down cancels the stream, and a change to up re-snapshots and resubscribes;
  - in down mode, a store change reaches the Tickets tab after a tab switch;
  - only one event stream is open at a time;
  - startup (unknown → up) takes a snapshot and subscribes; read-only keeps the stream;
  - a failed snapshot toasts once per failure streak and retries on the next probe tick;
  - a park event toasts once even if the Tickets tab was never opened, and a snapshot never toasts;
  - an enqueue from a tab reaches both tabs through the stream, with no fetch from the tab;
  - a tab rebuilt after a worktree context change shows current state, and shows "loading…"
    before the first snapshot;
  - `R` with the link up asks for one snapshot plus the queue;
  - no snapshot is taken while idle (the 2s poll is gone).
- **Reducer seam: the viewmodel reducer** (`ApplySnapshot`, `Reduce`, effects). Its tests change
  with the freshness rule: one case per event type, checking the effect it returns.
- **Server seam:** the snapshot handler test checks the queue mode is filled in.
- The Tickets tab's stream tests move to the app shell seam. Tab tests that only check drawing feed
  the tab a state directly.
- Prior art: the app shell's server connection tests (fake client, `Update` driven by hand) and the
  Queue tab's app-level test. A temporary test during diagnosis used exactly this shape: a fake with
  a buffered event channel, tab switches through the shell, and a check that the channel drains.

## Out of Scope

- New event types, or adding data (claim times, budget) to events.
- Snapshot changes other than adding the queue mode.
- Showing the queue mode on the Tickets tab.
- Other tabs (Status, Log, Worktrees, …) and their own refresh behaviour.
- New UI: no new indicators, keys or layout changes.

## Further Notes

- Diagnosis: `follow-ups/issues/40-queue-tab-poll-loop-dies-on-tab-switch.md`. The Queue poll loop
  lost its accidental keeper in `dd90acc2` (08a), when the Tickets tab stopped re-arming the shared
  `autoRefreshMsg`.
- Ticket 40 and its gx-investigate gotcha line should point at this spec once the work lands.
