# gx-investigate gotchas

Running list of previously-diagnosed gx/ralph-loop bugs, newest first. Append one line + a pointer
to the fixing commit or ticket whenever a bug diagnosed via [gx-investigate](SKILL.md) gets fixed
— don't re-explain what the linked commit/ticket already documents.

- **A blocking `TaskOutput`/`TaskStop` retrieval never resolves the background-task gate, stalling
  a ticket `claimed` ("implementing...") until the 2h aged-out cap.**
  `transcript.ReadBackgroundTasks` (`transcript/background_task.go`) only recognized resolution
  via a `task-notification`-origin transcript line carrying `<task-id>`. An agent that instead
  calls `TaskOutput` to block on the same task id gets its result inline as `toolUseResult:
  {retrieval_status, task: {task_id, ...}}` (nested); one that calls `TaskStop` to kill it outright
  gets `toolUseResult: {task_id, message, ...}` (flat) — two more shapes, neither producing a
  `task-notification` line, so the marker reads `outstanding-fresh` forever even though the agent
  plainly saw the result (or killed the task itself) and finished for real. Found live: `idle-
  cost/01` (`TaskOutput` shape — commit landed, worktree clean, pane idle, `run-log.jsonl` shows
  only `background-task-gate-held`, no release) and `idle-cost/09` (`TaskStop` shape, same
  symptom, found right after the `TaskOutput` fix landed — proof there wasn't just one gap).
  Distinct failure mode from the entry below (that one resolves too eagerly; this one never
  resolves at all) in the same new gate. Fixed same session, in two passes:
  `ReadBackgroundTasks` now matches all three shapes. See
  `follow-ups/issues/12-taskoutput-retrieval-never-resolves-background-task-gate.md`. If a fourth
  stuck ticket turns up, check for yet another `toolUseResult` shape before assuming this is fully
  closed — nothing so far has enumerated every way an agent can learn a background task ended.
- **`background-task-gate-released` is treated as full-turn completion, parking the ticket
  `needs-answer`/zero-commit even though the agent kept working afterward.**
  `waitForBackgroundTasks` (`ralphloop/waitforfinish.go:774-824`) only tracks whether one specific
  backgrounded shell command's own completion notification appeared in the transcript — not pane
  status. Its callers (`waitforfinish.go:143-166`, `iteration.go:240-243`) log the finish event and
  fall straight into `finishIteration`'s commit count with no re-check that the pane is still idle;
  if the agent resumes real work once it sees the background task's result (the normal case), the
  park fires anyway. Found live: `idle-cost` tickets `03` and `06`, both parked right after a gate
  release; `03` (manually resumed from the Queue menu) then produced a genuine commit 7 minutes
  later in the same session/pane. Recovery: resume from the Queue menu, don't clear to `open` — the
  session is intact, not stalled. `background_task.go`/the gate wiring landed 2026-08-13
  (`d583e908`, `05489e3e`). Fixed same session: `waitForBackgroundTasks` now re-runs
  `confirmFinished` once a gate it actually held releases, and both call sites loop again instead
  of finishing if that recheck finds the pane busy. See
  `follow-ups/issues/11-background-task-gate-release-skips-pane-recheck.md`.
- **A parked (needs-answer/needs-repair, no commits) outcome also fires a spurious "done"
  notification with `0s · 0 tok · $0.00`.** `ralphloop/loop.go`'s main scheduling loop (~line
  759-817) only early-exits on `r.built`/`r.parkedOnChild`; a plain park falls through to
  `completed++`/`sink.IterationFinished` anyway, alongside the correct `TicketNeedsHuman` message
  fired earlier in `iteration.go`/`waitforfinish.go`. Reads as "ticket X needed an answer, then
  completed anyway" in the same notification batch. Diagnosed via `blf`'s `power-improvements`
  ticket `05`, `run-log.jsonl` batch at `2026-08-14T22:41:45+03:00`; not yet fixed. See
  `follow-ups/issues/09-parked-outcome-still-fires-iteration-finished.md`.
- **Draining a paused epic run hangs silently — indistinguishable from a drain still waiting on a
  long in-flight ticket.** `Gate.Drain()` doesn't wake a run parked in `waitForResume`, so a drain
  requested against a paused epic never completes: the run still shows `running`, no
  `DrainComplete` notification ever fires, and drain-then-replace never launches — no error or log
  line marks the difference from an in-flight ticket the drain is legitimately still waiting on.
  Recovery: resume the epic and it exits immediately. Not fixed as of this entry — see ticket `01`
  of `drain-queue-fixes`; drop this entry once `01` lands. See
  `drain-queue-fixes/issues/01-*.md`.
- **Telegram ticket-completion notifications silently drop almost every time (unescaped `.` in
  cost).** `formatCost` (`ralphloop/notification_text.go`) renders `"$1.23"` and
  `iterationFinishedText` splices it unescaped into the counts line `message()` sends — `.` is a
  reserved MarkdownV2 char, so any non-integer cost 400s the whole send; since almost every real
  iteration has a fractional cost, `✅` ticket-done messages drop nearly always. Explains a report
  of "got epic-start and every iteration-started, never a single completion" (those carry no
  cost, so they go through fine). Fixed by escaping the assembled counts/detail strings in
  `iterationFinishedText`/`epicCompleteText`; regression covered by
  `ralphloop/full_epic_notifications_e2e_test.go`. See
  `follow-ups/issues/07-iteration-finished-cost-decimal-unescaped-markdownv2.md`.
- **Telegram batch notifications silently drop whenever ≥2 messages coalesce into one flush.**
  `renderBatch` (`ralphloop/chat_eventsink.go:244-253`) joins each already-escaped queued
  message with a raw `"\n---\n"` separator — `-` is a reserved MarkdownV2 char
  (`telegramMarkdownV2SpecialChars`), so any batch with 2+ distinct messages 400s on send;
  single-message batches (no separator) go through fine, so the drop is intermittent, not total.
  Same failure shape as the `epicCompleteText` `(s)` gotcha below, in the newer batch-join path.
  Found live: `tickets-tree`'s 2026-08-13 re-run, 19/26 telegram `notify_kind: batch` sends
  400'd; the 7 that succeeded matched exactly the 7 notifications the user reported receiving.
  Not fixed. See `follow-ups/issues/06-batch-notification-separator-unescaped-markdownv2.md`.
- **Nudge-retry exhaustion on the initial `herdr agent wait --until working` launch step leaves a
  ticket `needs-repair` with a live, never-prompted pane leaked.** `promptWithNudge`
  (`ralphloop/deps.go:483-568`) only resends bare Enter, never the actual prompt text, across its
  3 nudges (`promptMaxNudges`, `deps.go:454`, 45s each, `deps.go:453`); if the agent never
  reaches `working` before all 3 exhaust, no prompt content was ever delivered, and the created
  pane/tab is left alive with an empty prompt box — `loop.go`'s generic `r.err != nil` catch-all
  (`ralphloop/loop.go:732-752`) marks the ticket `needs-repair` with no `iteration-started`
  logged and no cleanup of the orphaned pane. Found live: `fix-spinner/04`, pane `w2A:p5`
  confirmed idle at Claude Code's fresh welcome screen ~2min after launch. Fixed:
  `promptWithNudge` now diffs a pane-text snapshot per attempt and retypes the full prompt (capped
  by `promptMaxRetypes`) instead of nudging when nothing changed, returning `errStuckSubmission`
  once retypes are exhausted; `runIteration` retries once against a fresh pane on that error and
  closes whichever pane it finally gives up on. See
  `follow-ups/issues/05-launch-nudge-exhaustion-leaks-live-pane-research.md`.
- **Reclaiming an already-live pane via `reattachIteration` logs no `iteration-started`, so the
  Queue spinner and the epic's "N parked" count both go stale.** `resumeReattachable`
  (`ralphloop/loop.go:409-411`) routes a reclaim of a ticket with a still-live tab/pane through
  `reattachIteration`, which deliberately sets `StartEvent=""` (`ralphloop/iteration.go:189-191`);
  that gates off both the run-log event and `sink().IterationStarted`
  (`launch.go:260-262,291-293`). The UI spinner (`ui/tickets/live_row.go`) and the epic-parked
  count (`ui/tickets/loop_registry.go:322-379,413-415`) both only update on
  `IterationStarted`/`TicketReattached`/`IterationPaused` events, never recomputing from ticket
  status directly — so a ticket can sit `claimed` with `gx` genuinely still polling it (confirmed
  live via the `herdr agent wait` process) while the UI shows no spinner and the epic title stays
  stuck at a stale parked count. Found live: `model-config/04a`, same stalled pane as the entry
  below, retried 3x. Not fixed. See
  `follow-ups/issues/04-reattach-reclaim-skips-sink-events.md`.
- **`attachToLiveAgent` treats a permanently-stalled idle pane as "already finished," parking a
  second ticket zero-commit with no real work attempted.** A ticket that already leaked a pane/tab
  from a failed launch (the `04a1` gotcha below) can, on reclaim, collide (`agent_name_taken`) with
  that same leaked agent name; `attachToLiveAgent` (`ralphloop/launch.go:283-301`) reattaches to it
  and, since its status is `idle`, logs `iteration-started`/`-finished` back-to-back with no
  `AgentPrompt`/`waitForFinish` ever run — `promptWithNudge`'s stall-retry never gets a chance.
  `finishIteration` then parks the ticket `needs-answer`/`park_kind: zero-commit` against the
  *new*, unused tab, while the actually-stalled agent sits untouched in the *old* leaked one — two
  live tabs end up under one label. Found live: `model-config/04a`, session id on the ticket
  matched the *original* `04a1` stall session (`state_change_seq` unchanged since). Not fixed. See
  `follow-ups/issues/03-attach-to-live-agent-treats-stalled-idle-as-finished.md`.
- **A `herdr agent prompt agent_prompt_stalled` error leaves a ticket `needs-repair` with no
  `iteration-started` ever logged, and the retry code that exists doesn't cover it.**
  `iteration-started` is logged only after `AgentPrompt` succeeds (`ralphloop/launch.go:242-263`);
  a failed prompt-send returns before that line ever runs. There's a nudge-retry
  (`promptWithNudge`, `ralphloop/deps.go:457-568`) but it only fires when the error text contains
  "timed out" (`isPollTimeout`, `ralphloop/waitforfinish.go:1127-1129`) — herdr's stalled-prompt
  message ("produced no observed state change within 5000 ms") doesn't match, so it's one-shot.
  Lands in the same generic `r.err != nil` catch-all as `agent_pane_busy` below
  (`ralphloop/loop.go:691-711`); clear the ticket back to `open` to reclaim, same as that entry —
  but reclaim leaks a second herdr tab (`TabCreate` has no collision detection) since the failed
  launch never cleaned up its own pane. Fixed by widening `isPollTimeout` to also match
  `agent_prompt_stalled`/"no observed state change" (`ralphloop/waitforfinish.go:1127-1136`); the
  pane/tab-leak-on-reclaim part is still open. See `model-config/issues/04a1-*.md`.
- **`type: code-review` tickets always stall on `needs-answer` (`park_kind: zero-commit`)
  under ralph-loop.** `gx-implement/SKILL.md` told the agent to "follow the gx-code-review
  skill instead," which the agent read as invoking the Skill tool — but `gx-code-review` sets
  `disable-model-invocation: true`, so the call is refused and the iteration ends with zero
  commits every time. Fixed at the scheduler, not in-session: `runIteration`
  (`ralphloop/iteration.go`) now launches a `type: code-review` frontier ticket with
  `/gx-code-review` directly (`codeReviewSkill` const, `ralphloop/loop.go`) instead of handing
  off from inside a `gx-implement` session, since the harness starting a session with the
  command already resolved isn't blocked by `disable-model-invocation` the way an in-session
  Skill-tool call is. `gx-implement/SKILL.md` keeps a short note for the manual-invocation edge
  case only. See `follow-ups/issues/02-code-review-skill-invocation-research.md`.
- **Every ticket in an epic lands in `needs-repair` on first claim, whole epic parks.**
  `ralphloop/labels.go`'s `iterLabel` (`epicName + "-iter-" + identifier`, no length cap) is used
  verbatim as the herdr agent name; epic slugs of 25+ chars push double-digit-ticket labels past
  herdr's 32-char `invalid_agent_name` limit, and herdr's error isn't special-cased the way
  `agent_name_taken` is, so it falls into the generic non-fatal `needs-repair` path and every
  ticket fails identically. Found live: `notification-throttle-impl` (27-char slug), tickets `01`,
  `02`, `09` all rejected on `notification-throttle-impl-iter-01`/`-02`/`-09` (34 chars). Not fixed.
  See `notification-throttle-impl/issues/11-agent-name-too-long-research.md`.
- **`conflict-lifecycle/02` forked 4 times (`02a`→`02a1`→`02a2`→`02a3`) despite `gx-to-tickets`
  already having the rules to catch it.** The ticket named 3 independent variants (SKILL.md's
  "variant fan-out" rule, N>couple should split) whose tests landed across 4 different files each
  needing its own precedent read (SKILL.md's "explore/implement split," currently gated on
  "touches test/e2e coverage" as a whole-ticket judgment rather than a file-count trigger) —
  neither rule fired. Not fixed; diagnosis + suggested SKILL.md trigger tightening in
  `conflict-lifecycle/issues/07-ticket-02-fanout-splits-research.md`.

- **A smart-zone `/compact` gets cancelled by gx's own finish-up prompt when herdr reports the pane
  idle/done during compaction.** `waitForCompactionSignal` (`ralphloop/waitforfinish.go`) returns
  success on *any* non-error `AgentWait`, consulting the transcript's compaction-boundary count only
  on a wait *timeout* past `smartZoneCompactTimeoutMs` (5 min) — so a prematurely-idle pane bypasses
  the very signal `compactSignalUnconfirmed` had just used to prove compaction hadn't finished, and
  the finish-up prompt lands as queued input that aborts the compaction. Found live:
  `lifecycle-refactor` iter-13c — whole compact+finish-up contract completing in 4.5s, second breach
  reporting identical occupancy (132603), exactly one `compact_boundary` in the session and it
  belongs to the *second* `/compact`. Not fixed. See
  `lifecycle-refactor/issues/15-smart-zone-compact-cancelled-by-finishup-prompt-research.md`.

- **`agent_pane_busy` on `herdr agent start` puts a ticket into `needs-repair` with no
  `iteration-started` logged at all.** `--pane` is herdr's own `RootPaneID` from `tab create`, not
  allocated/pooled by gx (`ralphloop/iteration.go:56-66` → `herdr/tab.go:30-52` →
  `launch.go:76,220-225`); two tickets launching concurrently under `MaxParallel` can race inside
  herdr's own pane allocator (external binary, no gx-side mutex around the herdr CLI calls). Not a
  gx-source bug; unstick by clearing the ticket back to `open` for the scheduler to reclaim. Found
  live: `lifecycle-refactor` ticket `05`. See the `lifecycle-refactor` epic's ticket-05
  herdr-pane-busy research ticket (filed under the status's retired name).

Older entries below describe a `children` frontmatter field that no longer exists: fork descendants
are now derived from `parent` alone (see [gx-local-tracker.md](../gx-local-tracker.md)). They are
kept as history of what the code did at the time, not as a description of today's schema.

- **The `lifecycle-refactor` + `lifecycle-contract` epics made five of the bugs below
  unrepresentable rather than fixed — read them as history, and don't port their defenses forward.**
  The old model recorded a fork twice (`children` on the original, `parent` on the descendant) and
  gave each fork child a `blocked_by` naming its own parent. Both writes are gone: `parent` on the
  descendant is now the only structural edge, a fork child carries no `blocked_by` at all, and
  blocking is one predicate — a ticket blocks while its own status isn't `done` or while any ticket
  in its `parent`-derived fork subtree blocks. A ticket's `blocked_by` naming its own ancestor was
  the only thing that made resolution self-referential, and every carve-out that existed to undo
  that self-reference (the self/descendant exclusion, the fork-sibling exclusion, the ancestor walk,
  the `exclude` hook, the `visiting` cycle guard, the `children`-based descendant walk) was deleted
  with it; the parent graph is validated acyclic at `Epic` construction, so the recursion needs no
  guard. Newest-first, the five that can no longer occur: (1) *a `blocked_by` on a specific fork
  sibling isn't enforced* and (2) *`fullyDone`'s children walk / fork-sibling exclusion / permanent
  `visiting` memo each start a chain early* — all three mechanisms deleted, and there is no stored
  child list left to go stale against `parent`. (3) *A commitless fork placeholder's own
  `blocked_by` was never enforced* and (4) *a ticket `blocked_by` its own fork-parent deadlocks
  forever* — both need an inherited parent-naming token, which the protocol no longer writes and
  `gx tickets migrate` strips. (5) *Code-review-spawned tickets appear in the queue tree but never
  start* — that was `children` written without `parent`; with `children` gone there is no second
  edge to write instead of the one the scheduler reads. Mis-parented data is now just a
  differently-shaped acyclic graph, not a case the resolver defends against, so a `parent` that
  points at the wrong ticket shows up as wrong nesting rather than as a silent deadlock. See
  `lifecycle-refactor/issues/03-*.md` for the deleted tests and the reason each went, and
  `lifecycle-contract/issues/01-*.md` for the schema contraction.
- **The mid-flight-fork placeholder fix above only closed one of three ways a `blocked_by` chain
  could still start early.** Follow-up consult (Opus, read-only) on the same `06b`/`06c` incident
  found: (1) `fullyDone`'s descendant walk trusted the then-existing children list, a field
  `gx tickets add --parent` never wrote to the parent — a missed or partial write of it silently
  dropped a real subtree from the check. (2) The fork-sibling exclusion (then `isSelfOrForkSiblingOrDescendant`,
  keyed on shared `Parent`) discounted a blocker's own open child as "family" whenever mis-parented
  data (a child's `Parent` written as the fork root instead of its direct parent — the exact shape
  the earlier `01a`/`01b` deadlock entry below documents) made it share the blocked ticket's
  `Parent`, even for a token naming a *direct*, non-inherited blocker. (3) The `visiting` cycle
  guard was a permanent memo, never unwound, so re-checking the same ticket a second time within
  one resolution (e.g. two `blocked_by` tokens sharing a descendant) could wrongly reuse a stale
  `true`. Fixed together (splitting them risked shipping (1)'s wider descendant walk while (2)'s
  exclusion still discarded exactly the descendants it exposed): `fullyDone` now derives
  descendants from a `parent`-pointer reverse index (`Epic.childrenIndex`) in addition to the
  children list; `gx tickets add --parent` backfilled the parent's children list as a best-effort,
  non-authoritative bonus write (the list itself has since been retired, leaving `parent` the only
  edge); the sibling exclusion (now `isForkSibling`) only fires for an
  *inherited* (ancestor) token, while an unconditional `isSelfOrDescendant` check (self and t's own
  descendants) still applies regardless — narrowing only the sibling half, never the
  self/descendant half, is what stops a ticket from being required to wait on its own not-yet-run
  follow-on work; `visiting` now unwinds via `defer delete` so it guards only the active recursion
  path. Also added: `gx tickets set --status done` now refuses a ticket with unresolved
  `blocked_by` unless `--force` is passed (with a stderr warning when forced) — closing the actual
  write-time gap, since claim-time enforcement in `ralphloop/loop.go`'s `claimNext` was confirmed
  already sound. Regression tests: `TestEpic_UnresolvedBlockers_MisparentedGrandchildStillEnforced`,
  `TestEpic_UnresolvedBlockers_MisparentedSelfDoesNotDeadlockOnOwnBlocker`,
  `TestEpic_UnresolvedBlockers_SharedOpenDescendantCheckedIndependently` (`tickets/status_test.go`);
  `TestRunTicketsAdd_BackfillsParentChildren` (`cmd/tickets_add_test.go`);
  `TestExecute_TicketsSet_StatusDoneRefusedWithUnresolvedBlocker`,
  `TestExecute_TicketsSet_StatusDoneForcedWithUnresolvedBlockerWarns` (`cmd/tickets_set_test.go`).
- **A commitless mid-flight-fork placeholder's own `Blocked by:` was never enforced, letting its
  whole fork chain start before the real blocker landed.** `tickets.Epic.fullyDone` (used by
  `UnresolvedBlockers` to resolve whether a named blocker is really done) only checked
  `t.IsDone()` + recursion into `t.Children` — it never checked whether `t` itself had unresolved
  `Blocked by:`. A ticket created already `status: done` (a commitless split/fork bookkeeping
  placeholder, e.g. `06c` forked off `06` with `Blocked by: [06b]`) never passes through
  `claimNext`'s claim-time blocker check, so nothing ever verified its own declared blocker.
  Anything blocked on that placeholder (e.g. `06c1`, `Blocked by: [06c]`) trusted `06c.status ==
  done` at face value and started immediately. Found live in `tickets-tree`: `06c1` started within
  200ms of `06b1` (`06b`'s own fork chain, which `06c` was supposed to wait on) — before `06b`'s
  chain had done any real work. Fixed: `fullyDone` now also calls into the same
  `unresolvedBlockers` walk for `t` itself (shared `visiting` cycle-guard across both), so a
  blocker is only "fully done" once its own `Blocked by:` chain is genuinely resolved too, not
  just its status field. Regression test:
  `TestEpic_UnresolvedBlockers_TransitiveThroughPrematurelyDoneForkPlaceholder` in
  `tickets/status_test.go`. See `tickets-tree/issues/13-epic-run-stalled-two-finished-iterations-undetected.md`.
- **A checked/queued epic silently drops out of cross-epic auto-promotion across a `gx` process
  restart.** `MaxConcurrentEpics` (default 2, `ui/settings.go:31-35`) is meant to auto-start the
  next queued epic the instant a running one finishes (`QueueModel.startAvailableEpics`,
  `queue.go:824-850`, re-invoked from `implementPollMsg` on completion, `queue.go:252`), but
  `m.pendingEpics` is process-local, in-memory-only state, populated exclusively by
  `startCheckedEpic` (Enter key) or `handleDetachedLiveConfirmed`. If the `gx` TUI process
  restarts (crash, reattach) after an epic was checked/queued but before its turn came up, the
  new process's `pendingEpics` starts empty — `queue-state.json`'s `items` still durably marks
  the epic's tickets `"pending"`, but nothing reconstructs `pendingEpics` from that on load.
  `cmdCheckDetachedLive` (`queue_reattach.go`) only covers the *other* stranded case (a ticket
  left `claimed`/`needs-repair` with a live herdr tab) — an epic that never got claimed at all
  falls through both paths and sits forever, looking "queued" in the UI but never starting. Found
  live: `tickets-tree` epic, ticket `03b1` (unclaimed and unblocked) never claimed after
  `fork-term` finished and freed a slot, because the attached `gx` process had restarted in
  between. Note: the durable `items` state can't distinguish "checked, Enter never pressed" from
  "checked, was queued, restarted before its turn" — both look identical on disk — so silently
  auto-requeuing on that signal alone is wrong (an earlier attempt at this fix did exactly that
  and broke `TestQueueModelSchedulesCheckedEpicsInCheckOrderAndBackfillsAtCap` by auto-starting
  epics the test hadn't pressed Enter for yet). Fixed instead by mirroring
  `cmdCheckDetachedLive`'s own pattern: a new `cmdCheckStrandedPending`
  (`ui/tickets/queue_reattach.go`), run once on the Queue tab's first load per process, surfaces
  checked-but-unclaimed-and-not-running epics via the same kind of confirm dialog ("Resume?")
  rather than resuming them silently; accepting re-derives the plan from the checked selection and
  appends it to `pendingEpics`. See `tickets-tree/issues/12-epic-runner-not-active-research.md`.

- **`gx tickets add --parent <id>` allocates the correct lettered ID but never writes `parent` into
  the new ticket's own frontmatter.** `cmd/tickets_add.go`'s `runTicketsAdd` only used `parent` to
  compute the new ID via `tickets.NextTicketID`; the stub `schema.Ticket{}` literal never set
  `stub.Parent`, despite `gx-local-tracker.md`'s "Mid-flight forking" section documenting this
  command as the one that sets `parent` "at creation." Every forked ticket created the documented
  way silently lost its `parent` link — the same shape the code-review-scoping gotcha below already
  flagged as scheduler/Queue-tab-invisible. Found live: `drain-queue`'s `02a1` (forked off `02a` for
  deferred test coverage) had no `parent` field despite the agent running `gx tickets add
  drain-queue --parent 02a --slug ...` exactly as documented. Fixed by setting `stub.Parent` when
  `parent != ""` before marshaling (`cmd/tickets_add.go`), regression test
  `TestRunTicketsAdd_WritesParentFrontmatter`; `02a1` backfilled via `gx tickets set --parent 02a`.
  See `drain-queue/issues/04-tickets-add-parent-not-written-research.md`.

- **Telegram epic-complete notifications always fail with HTTP 400; ticket-finished ones work
  fine.** `ralphloop/notification_text.go`'s `epicCompleteText` hardcodes a literal `(s)` (from "N
  ticket(s) landed in...") straight into the MarkdownV2 text without routing it through
  `s.escape`, unlike every other template in that file. `(`/`)` are reserved MarkdownV2 chars
  (`telegramMarkdownV2SpecialChars`), so Telegram's `sendMessage` rejects the whole message with
  400 every single time — this isn't the transient-network issue `notifications-fix/01`'s retry
  fix addressed; retries don't help a deterministically-malformed message. Confirmed via
  `.scratch/fork-term/run-log.jsonl`: the run's `iteration-finished` sends all logged
  `notification-sent`, the one `epic-complete` send logged `notification-failed reason: "send
  failed with status 400"`. Slack is unaffected (`escapeSlackMrkdwn` doesn't touch parens). Fixed
  by dropping the literal parenthetical ("N tickets landed in..."), same commit as this entry.
- **A ticket blocked_by its own fork-parent can deadlock forever, even though the parent is
  `done`.** When a ticket forks sequentially (X, then Y where Y is `blocked_by: [X]` and
  `parent: X`), `tickets.Epic.fullyDone`'s recursion into the children list requires every entry in
  the *root* ticket's children list to be done too. If the root's children list was written to list
  **both** X and Y directly (instead of just X, with Y nested under X's own children list), then
  resolving "is root fully done" pulls in Y — and `isSelfOrForkSibling`'s exclusion (keyed on
  `Parent` equality to the ticket being resolved) doesn't catch this, because Y's `parent` is X,
  not root, so Y doesn't read as X's sibling. Net effect: X can never be considered to unblock
  anything until Y (which can't start until X finishes) is also done — a self-deadlock hiding
  behind a `blocked_by` token that looks resolved (the named ticket really is `done`). Found live
  in `drain-queue`'s `01`→`01a`→`01b` chain and `tickets-tree`'s `03b`→`03b1`→`03b2`,
  `06b`→`06b1`→`06b2`, `06c`→`06c1`→`06c2` (four separate forks, same shape — this looks
  systemic to whatever produced these tickets, not a one-off typo). Inert copies of the same
  malformed shape (already-`done` chains, so harmless) also exist at `tickets-tree`'s
  `02d`→`02e` and `06`→`06c`. Two-part fix: (1) data — corrected the four live tickets'
  children/`parent:` links to match (each root listed only its direct fork; the intermediate ticket
  listed the grandchild); (2) source —
  `tickets/status.go`'s `isSelfOrForkSibling` (renamed `isSelfOrForkSiblingOrDescendant`) now
  also excludes any ticket reached by walking `Parent` hops upward from the candidate to the
  ticket being resolved, not just same-`Parent` siblings, so this shape can't deadlock even if a
  future fork's children/`parent:` links end up mismatched the same way again. Regression test:
  `TestEpic_UnresolvedBlockers_InheritedTokenNotBlockedByOwnDescendant` in
  `tickets/status_test.go`. Uncommitted as of this diagnosis — see `tickets/status.go` diff.
- **Code-review-spawned tickets show up in the queue tree but never start.** `gx-code-review` set
  the review ticket's children list but never `parent` on the tickets it spawned; both
  `RunScope.Contains`/`containsChain` (scheduler scope) and the Queue tab's tree nesting walk the
  *child's own* `parent` field, so the new tickets stayed silently out-of-scope until directly
  requested. Fixed in `35c0d2e` (`ralphloop: log scheduler scans; fix review-ticket scoping`),
  `skills/gx-code-review/SKILL.md` step 7.
- **Telegram notifications silently drop on any transient network blip, including epic-complete.**
  `ralphloop/telegram_eventsink.go`'s `send` is one-shot, 10s timeout, zero retries
  (`ralphloop/eventlog.go:216-227`) — a ~3min DNS/network stall reaching `api.telegram.org`
  dropped 4 consecutive notifications (3 `iteration-finished` + the final `epic-complete`) in one
  `gx-merge` run, confirmed via `notification-failed` "context deadline exceeded" entries in
  `run-log.jsonl` with the send goroutine running to completion (ruling out the process-exit race
  theory for that run). Also found: the failed-send `reason` string leaks the Telegram bot token
  into `run-log.jsonl` verbatim (Go's `*url.Error` includes the request URL). See
  `notifications-fix/issues/01-telegram-notification-failures-research.md` (retry fix) and
  `notifications-fix/issues/02-redact-bot-token-from-run-log.md` (token leak); neither fixed yet.
- **Spurious "iterN paused" / `agent_name_taken` notification for a ticket that then finishes
  fine.** A parent ticket doing a mid-flight fork (e.g. `06` → `06b`) can still be actively
  authoring the child ticket's file (full-content `Write`, not `gx tickets set`) *after* the
  scheduler has already claimed and launched an independent iteration for that child — the
  child's own agent was already running under a live herdr session. The parent's plain-file
  overwrite of `06b.md` clobbers the `status: claimed` the scheduler had just set, reverting it to
  `open`; the next `scheduler-scan` sees it unclaimed and re-launches a second iteration under the
  *same* deterministic herdr agent name, which collides with the still-alive original
  (`agent_name_taken`). `ralphloop/loop.go`'s `r.err != nil` path (~line 402) treats the failed
  launch as non-fatal: marks that ticket `needs-repair` and fires `IterationPaused`
  (`"{label} paused"` notification quoting the herdr error) but keeps scheduling. Harmless when
  the original iteration finishes normally afterward (its `done` write overwrites the spurious
  `needs-repair`), but the underlying race — two unsynchronized writers (ralph-loop's
  `Claim()`/`SetStatus()` vs. an agent's raw file `Write`) sharing one ticket file with no locking
  — is real and not yet fixed. Diagnosed via bugs-03 iter-06b/2026-08-08 run-log.jsonl (lines
  56–69); no fix ticket filed yet.
- **A ticket `blocked_by` on a specific mid-flight-fork sibling isn't actually enforced.**
  `tickets/status.go`'s `UnresolvedBlockers`/`isSelfOrForkSibling` excludes any candidate blocker
  sharing the checked ticket's `Parent` — meant to stop a ticket deadlocking on its own *inherited*
  parent-blocker token (e.g. `05b`/`05c` both carrying `Blocked by: 05`) — but the exclusion is
  keyed only on `Parent` equality, not on which token is being resolved, so it also swallows a
  direct sibling dependency (`02c` declaring `blocked_by: [02b]`): `02b`'s real status is never
  checked, `02c` reads as immediately unblocked. No scheduler-side race — `ralphloop.claimNext`
  reloads fresh per claim. Diagnosed via `tickets-tree` epic, ticket `02c`'s `needs-answer`; see
  `tickets-tree/issues/08-blocked-by-split-sibling-not-enforced-research.md` (not fixed yet).
