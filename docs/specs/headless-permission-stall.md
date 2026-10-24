# Headless permission stall: never leave a blocked ticket `claimed`

Source: wayfinder map `headless-permission-stall` (tickets 01-03). Decision tags like `[02]` point to
the map ticket that settled the point. Builds on `agent-runner.md` (stories 62-67) and ADR 0032
(slot caps, waiting-run grace).

## Problem Statement

A person runs an epic on the headless runner (`claude -p`). The agent asks for permission to use a
tool (`can_use_tool`). Nothing parks the ticket. It stays `claimed`, the Queue tab shows a healthy
running agent, and no one is told. The request waits until the person happens to run
`gx server agents answer`. This was reported several times on another machine. [01]

The cause: the server waits for the iteration with a wait that ends only on `idle` or `done`. A
`blocked` agent never ends that wait, so the code that pauses a blocked agent is never reached. The
runner's state tracking is correct: it already reports `blocked` with the tool and its input as the
reason. [01]

## Solution

The server notices a blocked headless agent and pauses the ticket at once, with a reason that names
the tool and its input and tells the person how to answer. The person answers with
`gx server agents answer`. The ticket goes back to `claimed`, the same session continues, and the
server keeps waiting for the iteration to end. [02]

The same fix covers a bare `requires_action` (the agent waits on a prompt, with no permission
request), a second permission request after the first answer, and tickets that are already stuck
today: the first check of an upgraded server pauses each one, with no migration. [02, 03]

## User Stories

### Notice and pause

1. As a user, I want a headless ticket whose agent raises a permission request to become Paused
   right away, so that it never sits `claimed` while the agent waits. [02]
2. As a user, I want no dwell time before the pause, so that the first person to look sees the
   request at once. The 15-second dwell of the herdr pane path does not apply. [02]
3. As a user, I want the Pause reason text to hold the tool name and its input, so that I can
   decide without opening the log. [02]
4. As a user, I want the Pause reason text to end with the hint "answer with
   `gx server agents answer`", so that I know the next step. [02]
5. As a user, I want the existing answer-class pause notification for this pause, with no new
   notification type, so that it shows up where I already look. [02]
6. As a user, I want the pause to use the reason `blocked-pane` (class `answer`), so that existing
   Queue labels, counts and `--wait` exit code 3 apply. Recovery stays off for `blocked-pane`, as
   decided in [03] question 6. [03]
7. As a user, I want a bare `requires_action` with no permission request to pause too, with the
   reason "waiting on a prompt" and the hint "interrupt with `gx server agents interrupt`", so
   that this case is never left `claimed` either. [03]
8. As a user, I want every new permission request to pause again at once with its own reason and
   notification, with no limit, so that a long run is never stuck on the second or tenth request.
   [03]
9. As a user, I want concurrent requests to pause on the latest one, because the runner tracks
   one open request at a time, so that the reason always matches what `answer` resolves. [03]
10. As a user, I want only the headless runner to pause this way, so that herdr and pty runs keep
    their own behaviour. [03]

### Waiting-run lifecycle

11. As a user, I want a paused run to leave the wait and use the waiting-run machinery, so that it
    behaves like a ticket that asked a question: the agent stays alive, and the slot is released
    after the ADR 0032 grace. [03]
12. As a user, I want `gx server agents answer` on such a ticket to put it back to `claimed`, so
    that the Queue shows the agent working again. Today `answer` only reaches the runner and never
    changes the ticket status. The flip applies only to tickets paused `blocked-pane` on a
    headless run. For every other ticket `answer` stays a no-op on status. [03]
13. As a user, I want the same session to continue after the answer, with the wait going on until
    the agent is idle or done, so that no work is repeated. [02]
14. As a user, I want an answer to re-settle the run through the same path as an answer to a
    question, so that the slot grace, caps and scheduling stay consistent. If caps are full when the person answers, the answer is delivered
    at once and the ticket re-acquires a slot by the existing ADR 0032 rule. [03]
15. As a user, I want a plain `resume` on a paused ticket with a live agent ("live" is the runner
    check behind `ReasonAgentWaiting`) to be refused like the waiting-agent refusal, with a message pointing to `gx server agents answer`, so that I do not
    start a second session beside a live one. [03]
16. As a user, I want `resume` to work once the agent is gone, so that a dead agent can be
    retried. [03]
17. As a user, I want `agents interrupt` on a blocked agent, or a `control_cancel_request` from
    claude, to act like an answer, so that the ticket returns to `claimed` and the wait goes on
    instead of leaving a stale pause. For a bare `requires_action`, `answer` cannot work
    (`ErrNotReady`), so `interrupt` is the exit and the doctor must prove it clears the state
    (story 25). [03]
17a. As a user, I want any other exit from `blocked` (a `running`, `idle` or `result` event) to
    return the ticket to `claimed` too: whenever the tracker is no longer `blocked`, the pause
    clears, whatever the cause. [03]
18. As a user, I want a dead agent behind a paused ticket to go through the existing dead-run
    handling, so that no new failure path exists. [03]

### Restart and upgrade

19. As a user, I want the pause decision to read the tracker's state, not server memory, so that
    it survives a restart. [02]
20. As a user, I want reclaim after a restart to pause again when the permission request is still
    open, so that a restart does not hide it. [02]
21. As a user, I want reclaim to not pause a request that was already answered, so that the ticket
    does not flip back to Paused. [02]
22. As a user upgrading gx, I want tickets stuck `claimed` today to recover with no migration: the
    first wait-loop check of the upgraded server pauses each one whose agent is alive. The slot
    grace clock starts when the pause is raised, not when the block began. [02]
23. As a user, I want stuck tickets whose agent is dead to go through the existing dead-run
    handling, so that they end up `needs-repair`-class like any other dead run. [02]

### Doctor

The doctor stories (24-27) come only from decision 03 item 8. They can ship separately from the
server fix.

24. As a user on Bedrock, I want `gx claude doctor` to assert that the tracker reaches `blocked`
    on a real `can_use_tool` request, so that I find out about drift in claude's events before a
    run stalls. Bedrock is a required target. [03]
25. As a user on Bedrock, I want the same check to assert `blocked` on a bare `requires_action`,
    and that `interrupt` clears it, so that story 7 stays true on my host. The map's evidence
    shows `requires_action` always arrives with `can_use_tool`, so the bare case uses a synthetic
    fixture. "Bedrock required" proves the `can_use_tool` case only. If `interrupt` does not clear
    a bare `requires_action`, the implementer adds a fallback exit and the spec is amended. [03]
26. As a maintainer, I want these checks to run in CI against recorded fixtures like the other
    doctor checks, so that the doctor and the tests cannot drift apart. [03]
27. As a maintainer, I understand that verifying the checks on a real Bedrock host is not a gate
    for this spec. It is tracked in `gx:follow-ups/130`. [03]

## Implementation Decisions

**Behaviour**

- The server's wait for a headless iteration must also react to `blocked`. When the agent is
  blocked, the ticket is paused at once with reason `blocked-pane` and the run is handed to the
  waiting-run machinery. [01, 02, 03]
- The pause reason is the tracker's blocked reason (tool and input) plus the answer hint. A bare
  `requires_action` has its own reason and hint (story 7). Both read tracker state, never a copy
  held in server memory. [02, 03]
- This reverses the current design of the iteration finisher, where a blocked iteration stays
  running on purpose and only a permission card is shown. The card stays. The ticket is now also
  paused, so the card and the pause show the same request. [03]
- Waiting-run machinery means: the run is marked waiting, the slot is released after the ADR 0032
  grace (release cap included), reclaim skips a waiting run, and an answer re-settles it. No new
  slot rule is added. [03]
- A successful `answer` on a paused headless ticket moves it from Paused to `claimed` and the
  server resumes waiting for `idle` or `done`. The status flip is limited to tickets paused
  `blocked-pane` on a headless run. Any other exit from `blocked` (`agents interrupt`,
  `control_cancel_request`, a `running`, `idle` or `result` event) does the same: whenever the
  tracker is no longer `blocked`, the ticket goes back to `claimed`. `recordAnswered` writes
  `meta.json`, which is why an answer survives a restart. [03]
- A new `blocked` after that pauses again by the same rule. There is no counter and no limit. [03]
- Plain `resume` is refused while the agent is alive, with the same refusal as the waiting-agent
  case and a pointer to `answer`. When the agent is gone it is allowed. [03]
- Pause kind is the existing `blocked-pane`. No new reason and no new pause class. [03]
- Scope is the headless runner only. Herdr, Codex and pty paths are unchanged. [03]

**Reclaim and recovery**

- Reclaim reads the tracker (replayed from the agent's event log) and pauses when a permission
  request has no response; it does not when the request was answered. [02]
- Stuck tickets heal on the first wait-loop check after the upgrade. There is no migration step
  and no new stored field required by the spec. [02]
- Dead agents keep the existing dead-run handling, with and without a `result` event. [02, 03]

**Doctor**

- The headless checks of `gx claude doctor` gain two assertions: the tracker reaches `blocked` on a
  real `can_use_tool` request, and on a bare `requires_action`. They run on the same check table
  that CI runs on fixtures. New fixtures are copied in by hand from `--record`, as for other
  checks. The map calls this the `gx doctor` headless check; plain `gx doctor` stays repo-only
  (story 95 of the agent-runner spec), so the checks go in `gx claude doctor`. [03]

**Wait and watchers** (decided in design review)

- The wait learns of a block from an event from the tracker, not by polling. `watchBlocked` polls
  every 30s (`blockWatchPoll`), which cannot meet "pause at once". `watchBlocked` is folded into
  the new pause path, so one watcher publishes and nothing double-publishes.

**Left to the implementer** (the map does not decide these; the implementer picks and keeps it
behind the behaviour above)

- Where the pause is announced (which call raises the pause and the notification).
- Names of any new marker, field or helper, including how a run is flagged as paused-but-alive.
- How reclaim reads tracker state (replay on reattach, a status call, or both).

## Testing Decisions

- **What is a good test:** it checks external behaviour only: the ticket's status and pause reason,
  the notification, what `answer` and `resume` return, and the claim after an answer. It does not
  assert which internal function pauses.
- **One seam: the server.** A server-level test starts the server on the in-process test harness
  with a store, and drives the headless runner fake. It is a table of subtests on one harness:
  - a permission request pauses the ticket with `blocked-pane` and a reason holding the tool,
    input and hint;
  - the notification record is raised;
  - `--wait` exits with code 3;
  - `resume` is refused while the agent is live, and allowed when it is gone;
  - `answer` returns the ticket to `claimed`; on a ticket that is not paused `blocked-pane`, it
    leaves the status alone;
  - a second request pauses again;
  - `agents interrupt`, a cancel request, and a `running`, `idle` or `result` event return the
    ticket to `claimed` (the fake drives the event; the doctor `interrupt` fixture covers the
    real exit);
  - a bare `requires_action` pauses with its own reason;
  - a server restart re-pauses an open request, leaves an answered one alone, and an
    answer-then-restart row stays `claimed`;
  - a stuck `claimed` ticket pauses on the first check;
  - a dead agent goes through dead-run handling;
  - herdr and pty runs stay `claimed` (story 10);
  - a full-caps answer re-acquires a slot by the ADR 0032 rule.
- **Doctor:** the extended `gx claude doctor` rows run in CI on recorded fixtures through the
  existing check table, for both new assertions.
- **Prior art:** the server tests for the permission card, for asking while waiting, and for
  reclaiming a waiting run, plus the doctor table tests of the agent-runner spec.
- No new seam is added. Real-claude and Bedrock runs are manual and not a gate.

## Out of Scope

- The Codex runner and the herdr pane runner: they use other detection code. [map]
- The pty runner: it has the same gap and is tracked in `gx:follow-ups/129`. [02]
- Verifying the doctor checks on a real Bedrock host: not a gate, tracked in `gx:follow-ups/130`.
  [03]
- Auto-allow of permission requests. The spec keeps the rule that gx never auto-allows (story 64
  of the agent-runner spec).
- A dwell time, retry limit or rate limit on repeated permission pauses. [03]
- Free-text or menu-number answers. The verb stays `answer allow|deny [reason]`.
- Any migration of stored tickets. [02]

## Further Notes

- Vocabulary: the map says "park `needs-answer`". The glossary retired those names. In this spec
  the ticket is Paused with Pause reason `blocked-pane`, whose Pause class is `answer`.
- The stuck state in the field reports is assumed `claimed`, not confirmed. The fix covers it
  for a `claimed` ticket. Any other ticket state with an open request is left alone. [map]
- Known limit: the tracker holds one open request, so a second request overwrites the first
  (`events.go:196-200`). Story 9 accepts this.
- Accepted risk: with no limit on repeated pauses (story 8), a looping agent can send many
  notifications. Add a limit only if it shows up in practice.
- The tracker, event dedupe, replay and flag handling were checked in ticket 01 and ruled out as
  causes. Do not change them. [01]
- Behaviour change to remember when reviewing: before this spec, a blocked headless iteration
  stayed `claimed` on purpose, with only a permission card. After it, the ticket is Paused.
