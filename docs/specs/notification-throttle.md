# Notification throttling

Handoff spec from the Notification throttling wayfinder map (`.scratch/notification-throttle-map/map.md`
in the local ticket tracker — decisions `01-mute-record-schema.md` and
`02-global-breaker-attribution.md` in that epic's `issues/` directory).

## Problem Statement

gx sends Telegram/Slack notifications from three places — ralph-loop's own lifecycle events
(epic started, iteration finished, ticket needs a human, epic failed), the `gx notify` CLI, and
epic-failure reporting — with no rate limiting, batching, or dedup anywhere in the path. A stuck
iteration can loop claim → false-finish → needs-answer → reopen indefinitely, and every pass through
that loop fires a fresh chat message: one incident sent a message roughly every 3 seconds for 12
minutes straight before anyone noticed and intervened. The operator's phone becomes unusable, and
because nothing bounds the behavior, any future bug with a similar shape reproduces the same storm.

Separately, `gx-investigate` (the skill that diagnoses ralph-loop misbehavior) files its diagnosis
tickets into whatever epic is under investigation, mixing research/diagnosis tickets in with that
epic's implementation tickets — confusing when reading the epic's ticket list later.

## Solution

A shared budget-and-mute layer sits in front of every notification send, independent of what causes
a future storm: a fixed per-minute budget per transport (Telegram and Slack tracked independently),
a per-source mute that trips when one (event type, ticket) pair floods repeatedly, and a global
circuit breaker that stops all sends on a transport once total volume crosses budget regardless of
source. Internal ralph-loop events additionally batch on a fixed periodic tick rather than sending
one message per event. `gx-investigate` is changed to always file its diagnosis tickets into the
existing `follow-ups` backlog epic instead of the epic under investigation.

## User Stories

1. As an operator running ralph-loop unattended, I want notification volume capped per transport, so
   that a stuck loop can't turn my phone into a pager storm no matter what causes it.
2. As an operator, I want internal lifecycle events (iteration started/finished, epic complete, etc.)
   batched into one message every few seconds rather than one message per event, so that a burst of
   fast, legitimate events (several tickets landing back to back) doesn't itself read as spam.
3. As an operator, I want identical repeated messages within one batch window collapsed to a single
   line with a `×N` count, so that "iteration finished" repeated three times in one flush doesn't
   look like three separate incidents.
4. As an operator, I want `gx notify` (my own deliberate, one-off CLI messages) to send immediately
   without waiting for a batch flush, so that a manual heads-up isn't delayed by the batching meant
   for high-frequency internal events.
5. As an operator, I want `gx notify` to still be subject to the same budget/mute checks as internal
   events, so that a scripted loop calling `gx notify` repeatedly can't bypass the protection by
   using the "immediate" path.
6. As an operator, I want a ticket that's individually flooding notifications (same event type,
   same ticket, repeatedly within a minute) to get muted on its own — not the whole transport — so
   that one runaway ticket doesn't silence notifications for every other ticket in the epic.
7. As an operator, I want exactly one "muting this" notification when a per-source mute trips, not a
   notification per suppressed message, so that the mute itself can't become a second source of spam.
8. As an operator, I want a per-source-muted ticket parked `needs-repair`, so that it stops
   iterating and shows up in the Tickets tab as something that needs my attention.
9. As an operator, I want the Suggested Actions menu (`m`) to offer "Unmute & Reopen" on a ticket
   that's `needs-repair` specifically because of a storm mute — distinguishable from a ticket parked
   `needs-repair` for a real conflict or design question — so that clearing a false-alarm mute is a
   single keypress instead of hand-editing the ticket file.
10. As an operator, I want the per-source mute to be manual-clear only (no auto-expiry), so that a
    ticket that was genuinely misbehaving doesn't silently start iterating again on its own.
11. As an operator, I want a global circuit breaker per transport that trips when total send volume
    (across all sources) crosses the per-minute budget, so that a storm made of many individually
    reasonable sources still gets stopped even though none of them alone crossed the per-source mute
    threshold.
12. As an operator, I want exactly one "notifications globally muted, re-enable with `gx notify
    --enable`" message when the global breaker trips, so that I still get told it happened even
    though every subsequent send on that transport is suppressed.
13. As an operator, I want the global mute state to persist across restarts, so that a crash-looping
    process doesn't self-heal into repeating the storm the moment it restarts.
14. As an operator, I want `gx notify --enable`/`--disable`/`--status` to inspect and control the
    global mute directly, so that I can manually silence notifications for a planned quiet period, or
    confirm current mute state, without editing a state file by hand.
15. As an operator, when a global trip is caused by one or two sources clearly dominating the
    trailing window's volume, I want those sources muted individually too (via the same per-source
    mute), so that re-enabling the transport doesn't immediately hand the same runaway ticket a
    second chance to re-trip it.
16. As an operator, when a global trip is genuinely diffuse (no single source responsible for a
    large share of the window), I want no individual ticket muted, so that tickets that were each
    behaving reasonably aren't punished for a trip that wasn't really their fault.
17. As an operator investigating a trip after the fact, I want the full per-source breakdown of
    every trip logged — not just whichever sources crossed the mute threshold — so that I can see the
    complete picture of what was sending during a storm, including sources that stayed under the
    per-source mute threshold.
18. As an operator debugging a symptom that turns out to be a notification mute rather than a live
    bug, I want `gx-investigate` to check the notification mute state as part of its diagnosis
    inventory, so that "ticket looks stuck" and "ticket is muted and waiting for me to unmute it"
    aren't confused with each other.
19. As a developer reading an epic's ticket list, I want `gx-investigate`'s diagnosis tickets filed
    into the `follow-ups` backlog epic instead of the epic under investigation, so that an epic's
    ticket list stays implementation tickets only, not mixed with diagnosis/research tickets.
20. As a developer, I want the mute-trip state on a ticket to be machine-readable (typed frontmatter,
    not prose in a body section), so that the Suggested Actions menu can reliably gate "Unmute &
    Reopen" without parsing free text.

## Implementation Decisions

- **Shared choke point** — two new, narrow seams, not duplicated per call site:
  - **Budget/mute gate**: a pure decision function every send path calls through first — the async
    chat-event path (currently `chatEventSink.send` → `sendNotification`, `eventlog.go:244`),
    `EpicFailureReporter.EpicFailed` (`epic_failure_reporter.go:61`, which also reuses
    `sendNotification`), and `gx notify`'s sync path (`ralphloop.SendMessage`/`sendMessage`,
    `notify.go:15`, which bypasses `sendNotification` entirely today). The gate takes
    `(transport, eventType, source string, now time.Time)` plus an optional injected
    `parkTicket(source, reason string) error` callback, applies the budget/per-source-mute/
    global-mute rules below, and returns a decision (allow / per-source-muted / globally-muted) —
    plus, as a side effect of a trip, persists the resulting mute state (ticket frontmatter and/or
    the global state file) and returns whether this call is the edge-triggered one that should also
    send the "muting this" / "globally muted" notification. The loop registry supplies `parkTicket`
    (it owns the ticket path and repair state); a bare `gx notify` process has none, so a trip there
    still writes the ticket's `mutes` field but skips the `needs-repair` park (see "Clearing a mute"
    below for how an unparked mute stays reachable).
  - **Batch queue**: wraps only the async chat-event path (`chatEventSink`) — the internal
    lifecycle events enumerated in `chat_eventsink.go`'s doc comment (`EpicStarted`,
    `IterationStarted`, `IterationPaused`/`Resumed` for non-park kinds, `IterationFinished`,
    `TicketNeedsHuman`, `EpicParked`, `EpicComplete`, `DrainComplete`). `gx notify` and `EpicFailureReporter` do not
    go through the batch queue — both already send exactly one message per call, and per the map,
    `gx notify` is a deliberate one-off action that shouldn't wait on a flush tick. The queue flushes
    synchronously on sink close / TUI shutdown (bounded by the transport timeout), not just on the
    periodic tick, so a run ending mid-window doesn't drop its last batch. If the transport is
    already globally muted when a close-time flush would fire, the flush is suppressed rather than
    sent — that suppression appends a `notification-suppressed` line to `run-log.jsonl` naming the
    queued event kinds, so the epic's outcome is still recoverable from the log even though chat
    never got it.
  - Call order for the batched path: event occurs → gate check, counted into the **event series**
    (may trip a per-source mute and short-circuit here) → enqueue into batch → periodic or
    close-time flush renders and sends, counted into the **send series** for the global breaker (see
    Budget below). `gx notify` and `EpicFailureReporter.EpicFailed` skip the enqueue step, sending
    immediately after the gate — their send is also counted directly into the send series (no
    separate event-series entry, since nothing batches them).
- **Budget accounting — two series, not one**: a single post-batch send counter can't feed the
  per-source mute (it's already aggregated away by the batch) or the global breaker's attribution
  (a flush is one send covering many sources — no single source to blame). The gate's persisted
  state therefore tracks two trailing-window series per transport:
  - **Event series** (pre-batch): every individual `(eventType, source, time)` the gate is asked
    about, whether or not it ends up batched. Feeds the per-source mute threshold and, on a global
    trip, the ≥25% attribution share (see Global-breaker attribution below) — both computed against
    total window **events**, not sends.
  - **Send series** (post-batch): one entry per actual wire send — each flush (periodic or
    close-time) counts as one, plus every immediate `gx notify`/`EpicFailed` send. Feeds the global
    breaker's budget check.
  - **Budget**: ~20 messages/minute on the send series, tracked per transport independently
    (Telegram and Slack each get their own budget; one transport's volume never affects the
    other's). Raised from the originally-floated 10/min specifically so it can't sit exactly at the
    ~10 sends/minute a steady 6s flush tick already implies — the breaker must trip on genuinely
    excessive volume, not on the batching mechanism's own baseline cadence.
- **Batching**: internal events accumulate in the queue and flush on a fixed ~6s periodic tick — not
  silence-debounced (deliberately not modeled on the `waitForFinish` idle-debounce pattern that
  caused the original incident). A flush renders all queued messages as one send, joined with
  separator lines between originally-distinct messages. Identical messages within one flush window
  collapse to a single line with a `×N` suffix.
- **Per-source mute** (terminology: *mute*, never "ban"): keyed by (event type, ticket). Trips when
  the event series records **5 identical (event_type, ticket) events within a trailing 60-second
  window**. On trip: send exactly one edge-triggered "muting this" notification (fires once on the
  allowed→muted state transition, never once per suppressed message), write the mute onto the
  ticket itself via the new `mutes` frontmatter field (see Schema below), and — when the gate has a
  `parkTicket` callback available — park the ticket via the existing
  `ralphloop.MarkNeedsRepairWithReason` path (`claim.go:168`) with a reason identifying it as a
  storm mute. Manual-clear only — no auto-expiry. `gx notify`/`EpicFailureReporter` sends use a
  synthetic ticket-less `source` (`"cli"` for `gx notify`, `"epic:<name>"` for `EpicFailureReporter`)
  that is counted in the event series like any other source but can never itself be per-source-muted
  (there is no ticket to write `mutes` onto or park).
- **Global mute** (coarser circuit breaker, per transport): trips when the send series' total volume
  — every source combined — crosses its ~20/min budget. On trip: send exactly one final
  "notifications globally muted on `<transport>` — re-enable with `gx notify --enable`" message,
  then suppress every further send attempt on that transport (both the gate's decision and,
  functionally, the batch queue's flush — see the suppressed-flush handling above) until re-enabled.
  Global mute state persists to `~/.config/gx/notifications-state.json` (matching the existing
  `queue-state.json` convention under `config.UserConfigDir`, not a new state-directory
  convention), one entry per transport: `{muted: bool, tripped_at: timestamp, reason: "auto-trip" |
  "manual-disable"}` plus the trailing event/send series and trip history described below —
  deliberately survives process restart, so a crash-loop can't self-heal into repeating the storm.
  Re-enable only via `gx notify --enable`. Writes are atomic (temp file + rename, matching
  `tickets/schema/write.go`'s existing pattern) under a short-lived file lock; lock acquisition times
  out after ~1s and **fails open** — the send still goes out, only the accounting write is skipped —
  so a contended lock (several epics closing concurrently) can't hang TUI shutdown or drop the
  message actually being sent.
- **Global-breaker attribution** — on a global trip, the gate inspects the trailing **event series'**
  per-source breakdown (not the send series — see Budget accounting above):
  - Any source responsible for ≥25% of the window's **events** gets an individual per-source mute
    too (same mechanism/schema as the ordinary per-source mute above — `mutes` frontmatter +
    `needs-repair` park when a callback is available), so re-enabling the transport doesn't
    immediately hand a dominant source a second chance to re-trip it.
  - A genuinely diffuse trip (no source at or above 25%) mutes no individual ticket — only the
    transport-level shutoff applies, and `gx notify --enable` requiring a human to look is treated as
    sufficient gate before resuming.
  - Regardless of the 25% cutoff, the *complete* per-source breakdown of every trip (every sender
    active in the event-series window, muted or not) is appended to a small trip history in the same
    global state file: `trips: [{tripped_at, reason, sources: [{source, count}, ...]}]`, capped at
    the 20 most recent trips per transport so this hot-path file can't grow unbounded. This is for
    post-hoc investigation, not gating — trimming behavior beyond the fixed 20-entry cap is left to
    this handoff epic's own ticket breakdown.
- **`gx notify` CLI surface** — new flags on the existing `notify` command
    (`cmd/commands.go:418`/`cmd/operations.go:376`): `gx notify --enable` and `gx notify --disable`
    (manually clear/trip the global mute per transport — the same manual path a planned quiet period
    would use), `gx notify --status` (report global mute state and any active per-source mutes, read
    from the state file and a ticket scan respectively). The command's `Args` relaxes from
    `cobra.ExactArgs(1)` to `cobra.MaximumNArgs(1)`, with explicit validation that a positional
    message and `--enable`/`--disable`/`--status` are mutually exclusive. Plain `gx notify <message>`
    keeps its current behavior (send immediately) plus now passes through the shared gate first.
    `gx config test-notifications` (`cmd/operations.go:333`) is a separate, fourth send path and
    stays **exempt** from the gate — it's the deliberate way to confirm a transport works right after
    `gx notify --enable`, so it must not itself be blockable by an active mute.
- **Ticket mute schema** — new typed frontmatter field on `tickets/schema.Ticket`
  (`tickets/schema/ticket.go`): `Mutes []MuteRecord`, `MuteRecord{EventType string, TrippedAt
  time.Time}` (a list, since a ticket can be storm-muted on more than one event type
  independently) — same treatment as the existing `Commitless`/`IterationStatus` fields: struct
  field, YAML (de)serialization in `tickets/schema/frontmatter.go`/`parse.go`/`write.go`, and a
  manual addition to `gx tickets schema`'s hand-written reference text
  (`cmd/tickets_set.go:21`, `ticketsSchemaText`). Like `IterationStatus`, enum/shape enforcement for
  `EventType` is write-conditional (belongs to the write path that sets it), not added to
  `schema.Validate` — consistent with how `IterationStatus` already documents that split. This is a
  machine-written-only field (no `gx tickets set --mutes` flag), the same posture as `SessionIDs`.
  Because the UI's Tickets-tab `tickets.Ticket` (`tickets/ticket.go`) hand-mirrors a subset of
  `schema.Ticket` rather than embedding it (`Commitless` is copied in `tickets/loader.go:97`, and
  `IterationStatus` isn't mirrored at all today), `Mutes` needs the same mirroring step added to
  `tickets/ticket.go` and `tickets/loader.go` for the Suggested Actions gate below to see it.
- **Clearing a mute** ("Unmute & Reopen", extends `ui/tickets/suggested_actions.go`): a new
  `actionUnmuteReopen` case added to `applySuggestedAction`'s switch, gated in
  `suggestedActionItems`/`ticketHasSuggestedActions` by **non-empty `Mutes` alone, independent of
  the ticket's current status** — not `needs-repair` + `Mutes` as an initial read might suggest,
  because a mute tripped with no `parkTicket` callback available (a bare `gx notify` process, or a
  global trip's attribution firing during a close-time flush after the run's own loop has already
  torn down) writes `Mutes` without ever parking the ticket `needs-repair`; gating on status too
  would make that mute invisible and unclearable from the Tickets tab. `suggestedActionItems`/
  `ticketHasSuggestedActions` change signature to take the whole ticket rather than just
  `tickets.RenderedStatus`, updating their four call sites (`actions_menu.go:104`,
  `queue_actions_menu.go:21`, `view.go:217`, `queue_view.go:234`). Clearing: empties the ticket's
  `Mutes` field, demotes any `## Needs Repair` section present, and appends a one-line `## Comments`
  note recording who/when unmuted (parity with how `## Needs Repair`/`## Needs Answer` are already
  retired into `## Comments` on reopen) — via a new `ralphloop.UnmuteTicket(path, now)` sibling to
  `UnparkTicket` (`ralphloop/unpark.go:79`), sharing its underlying body-update helpers rather than
  reusing `UnparkTicket` itself (which only knows how to demote `## Needs Answer` and doesn't touch
  `Mutes`). If the ticket was parked, this also reopens it (`status: open`); if it wasn't (the
  no-callback case above), clearing `Mutes` is the whole action — there's no park to undo. The menu
  widget itself (`ui/tickets/actions_menu.go`) needs no changes — it's already generic over a
  `[]components.MenuItem` list.
- **`gx-investigate` diagnosis destination**: change the skill's ticket-filing logic (currently
  "file into the active epic in scope" — `skills/gx-investigate/SKILL.md`) to always target the
  `follow-ups` epic instead, following the established backlog-ticket shape already used there
  (`status: draft`, `type: research` — matching gx-investigate's current type choice, which is
  commitless-by-type and therefore correct for a diagnosis-only ticket, unlike `type: task` — plus a
  `## Context` section naming the originating epic/investigation). If `follow-ups` doesn't exist yet
  under the current tracker root, gx-investigate creates the epic directory (no `epic.yaml` needed
  until a loop actually runs against it). Scoped specifically to gx-investigate — other "file into
  whatever epic is active" call sites (code-review fix tickets, spec-review follow-ups) are
  untouched. The skill doc also gains a step in its diagnosis inventory to read
  `~/.config/gx/notifications-state.json` and scan for ticket-level `Mutes`, so an investigation can
  tell a notification mute apart from a live bug before concluding one.

## Testing Decisions

- Tests should exercise external behavior only: given a sequence of send attempts (transport, event
  type, source, timestamps), assert the gate's allow/mute/global-mute decisions and the resulting
  persisted state (ticket frontmatter, state file contents) — not internal counters or timer
  implementation.
- **Gate**: table-driven tests over sequences of `(transport, eventType, source, time)` inputs,
  asserting the returned decision and any side effects (mute written, edge-triggered notification
  flagged, correct series — event vs. send — incremented). Prior art: `ralphloop/notify_test.go`'s
  table-driven style (`TestSendMessage_BothConfigured_SendsToBothAndReportsBoth` etc.) and its
  `fakeTelegramServer`/`fakeSlackServer` helpers for the two send paths the gate now fronts. Cases to
  cover: under budget (allowed); one source crossing the per-source threshold (5 identical events in
  60s → per-source mute + exactly one "muting this" send); total send-series volume crossing budget
  with one dominant source in the event series (global mute + that source also per-source-muted, via
  the event series' ≥25% share, not the send series); total volume crossing budget diffusely in the
  event series (global mute, no per-source mute); a second attempt against an already-muted source
  (suppressed, no second "muting this" send); a transport already globally muted (every attempt
  suppressed until `--enable`); a ticket-less source (`"cli"`/`"epic:<name>"`) counted in the event
  series but never individually muted even past the 25% share; and a trip with no `parkTicket`
  callback (mute written, park skipped). Also: lock-contention behavior — a send proceeds and the
  accounting write is skipped when the ~1s lock timeout elapses.
- **Batch queue**: prior art is `ralphloop/chat_eventsink_test.go`'s `fakeChatTransport` (records
  every sent text in memory) and its `waitForSentCount` helper for the async, goroutine-driven send
  path. New tests assert: multiple events enqueued within one flush window arrive as a single sent
  message with separator lines; identical messages within one window collapse to one line with
  `×N`; a flush tick with an empty queue sends nothing; `gx notify` and `EpicFailureReporter` sends
  do not appear batched (each still produces its own immediate send, verified via the existing
  `fakeChatTransport`/`fakeTelegramServer` patterns from `notify_test.go` and
  `epic_failure_reporter_test.go`); closing the sink with a non-empty queue sends exactly one
  message (flush-on-close); and closing the sink with a non-empty queue while the transport is
  already globally muted sends nothing but appends one `notification-suppressed` line to
  `run-log.jsonl` naming the queued event kinds.
- **Ticket schema**: extends `tickets/schema/ticket_test.go`/`parse_test.go`/`write_test.go`'s
  existing round-trip pattern (marshal → unmarshal → equal) to cover the new `Mutes` field,
  mirroring how `Commitless`/`IterationStatus` are already covered there. Also extends
  `tickets/ticket.go`/`loader.go`'s own tests to cover mirroring `Mutes` from `schema.Ticket`, the
  same way `Commitless` is already mirrored.
- **Suggested Actions**: `ui/tickets/suggested_actions.go` has no existing test file today, so this
  is new coverage, not an extension — asserting `ticketHasSuggestedActions` returns true for any
  ticket with non-empty `Mutes` regardless of status (not just `needs-repair`), and that the new
  action clears `Mutes`, demotes `## Needs Repair` if present, writes the `## Comments` note, and
  reopens the ticket only when it had actually been parked.
- **`gx notify` CLI flags**: `--enable`/`--disable`/`--status` tested at the `cmd` package level,
  following whatever existing pattern covers `runNotify`/`runConfigTestNotifications` today (direct
  function calls against a temp config/state dir, not a subprocess exec) — including a case
  asserting a positional message combined with `--status` is rejected as mutually exclusive, and
  that `gx config test-notifications` sends succeed even while a mute is active (exemption from the
  gate).
- **`gx-investigate` destination change**: this is a skill-doc change, not code — no automated test;
  verified by reading the updated `SKILL.md` against the `follow-ups` epic's established ticket
  shape.

## Out of Scope

- The root cause of the original incident (`test-suite-perf/04b2`'s `waitForFinish` idle-debounce
  heuristic) — tracked and fixed in its own epic, independent of this defense-in-depth layer.
- Extending "always file to `follow-ups`" beyond `gx-investigate` to other ticket-filing call sites
  (code-review fix tickets, spec-review follow-ups) — a bigger norm change, left for its own
  follow-up if wanted later.
- Implementing the Suggested Actions menu framework itself — it already exists
  (`ui/tickets/suggested_actions.go`, `ui/tickets/actions_menu.go`); this spec only adds one new
  action to it.
- Auto-expiring per-source or global mutes on any timer — both are manual-clear only, by decision.
- Trimming policy for the trip history beyond the fixed 20-entry cap (e.g. time-based expiry) — left
  to this epic's own ticket breakdown.
- Any change to the actual per-minute budget numbers (~20/min send-series budget, 5-events/60s
  per-source threshold) or the batching tick (~6s) becoming user-configurable — all are fixed
  constants for this spec; making them configurable is a possible future follow-up, not decided
  here.

## Further Notes

- Origin: `test-suite-perf/issues/04b2` — an infinite claim → false-finish → needs-answer → reopen
  loop fired a Telegram message roughly every 3 seconds for about 12 minutes before anyone noticed.
- Full decision trail, including the two grilling sessions this spec was synthesized from, lives in
  the local ticket tracker's `notification-throttle-map` epic (`map.md` and its two closed tickets,
  `01-mute-record-schema.md` and `02-global-breaker-attribution.md`).
- Realistic platform ceilings (for context, not the basis of the budget number): Telegram allows
  roughly 1 msg/sec to a single chat, ~20/min to a group, 30/sec global; Slack allows roughly 1
  msg/sec per channel. The chosen ~20/min budget sits well under both — this is a personal-attention
  budget, not an API-rate-limit-avoidance one.
- This spec was reviewed in a `/consult` second-opinion pass (Opus) after first draft, which verified
  every code claim against the actual files and surfaced 14 findings — all resolved and folded into
  the sections above (accounting split between event/send series, gate state persistence, the two
  missing constants, park-callback plumbing, batch-queue flush-on-close and suppressed-flush
  logging, the `UnmuteTicket` sibling function, Suggested Actions' signature change and UI-ticket
  mirroring, the state-file location/atomicity/lock-timeout, `test-notifications`' gate exemption,
  and gx-investigate's ticket type). Nothing remained unresolved.
- Next step: run `gx-to-tickets` against this spec to break it into tracer-bullet tickets with
  their own blocking edges and approved test seams.
