# gx-investigate gotchas

Running list of previously-diagnosed gx/ralph-loop bugs, newest first. Append one line + a pointer
to the fixing commit or ticket whenever a bug diagnosed via [gx-investigate](SKILL.md) gets fixed
— don't re-explain what the linked commit/ticket already documents.

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
  fine.** A parent ticket doing a mid-flight split (e.g. `06` → `06b`) can still be actively
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
- **A ticket `blocked_by` on a specific mid-flight-split sibling isn't actually enforced.**
  `tickets/status.go`'s `UnresolvedBlockers`/`isSelfOrSplitSibling` excludes any candidate blocker
  sharing the checked ticket's `Parent` — meant to stop a ticket deadlocking on its own *inherited*
  parent-blocker token (e.g. `05b`/`05c` both carrying `Blocked by: 05`) — but the exclusion is
  keyed only on `Parent` equality, not on which token is being resolved, so it also swallows a
  direct sibling dependency (`02c` declaring `blocked_by: [02b]`): `02b`'s real status is never
  checked, `02c` reads as immediately unblocked. No scheduler-side race — `ralphloop.claimNext`
  reloads fresh per claim. Diagnosed via `tickets-tree` epic, ticket `02c`'s `needs-info`; see
  `tickets-tree/issues/08-blocked-by-split-sibling-not-enforced-research.md` (not fixed yet).
