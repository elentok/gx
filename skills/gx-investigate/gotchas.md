# gx-investigate gotchas

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
  left `claimed`/`needs-attention` with a live herdr tab) — an epic that never got claimed at all
  falls through both paths and sits forever, looking "queued" in the UI but never starting. Found
  live: `tickets-tree` epic, ticket `03b1` (`ready-for-agent`, unblocked) never claimed after
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

Running list of previously-diagnosed gx/ralph-loop bugs, newest first. Append one line + a pointer
to the fixing commit or ticket whenever a bug diagnosed via [gx-investigate](SKILL.md) gets fixed
— don't re-explain what the linked commit/ticket already documents.

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
  `parent: X`), `tickets.Epic.fullyDone`'s recursion into `children` requires every entry in the
  *root* ticket's `children:` list to be done too. If the root's `children:` was written to list
  **both** X and Y directly (instead of just X, with Y nested under X's own `children:`), then
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
  `children:`/`parent:` to match (each root lists only its direct fork; the intermediate ticket
  lists the grandchild) via `gx tickets set <path> --children <id>`; (2) source —
  `tickets/status.go`'s `isSelfOrForkSibling` (renamed `isSelfOrForkSiblingOrDescendant`) now
  also excludes any ticket reached by walking `Parent` hops upward from the candidate to the
  ticket being resolved, not just same-`Parent` siblings, so this shape can't deadlock even if a
  future fork's `children:`/`parent:` end up mismatched the same way again. Regression test:
  `TestEpic_UnresolvedBlockers_InheritedTokenNotBlockedByOwnDescendant` in
  `tickets/status_test.go`. Uncommitted as of this diagnosis — see `tickets/status.go` diff.
- **Code-review-spawned tickets show up in the queue tree but never start.** `gx-code-review` set
  `children` on the review ticket but never `parent` on the tickets it spawned; both
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
  launch as non-fatal: marks that ticket `needs-attention` and fires `IterationPaused`
  (`"{label} paused"` notification quoting the herdr error) but keeps scheduling. Harmless when
  the original iteration finishes normally afterward (its `done` write overwrites the spurious
  `needs-attention`), but the underlying race — two unsynchronized writers (ralph-loop's
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
  reloads fresh per claim. Diagnosed via `tickets-tree` epic, ticket `02c`'s `needs-info`; see
  `tickets-tree/issues/08-blocked-by-split-sibling-not-enforced-research.md` (not fixed yet).
