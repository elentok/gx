# gx-investigate gotchas

Running list of previously-diagnosed gx/ralph-loop bugs, newest first. Append one line + a pointer
to the fixing commit or ticket whenever a bug diagnosed via [gx-investigate](SKILL.md) gets fixed
— don't re-explain what the linked commit/ticket already documents.

- **A ticket blocked_by its own split-parent can deadlock forever, even though the parent is
  `done`.** When a ticket splits sequentially (X, then Y where Y is `blocked_by: [X]` and
  `parent: X`), `tickets.Epic.fullyDone`'s recursion into `children` requires every entry in the
  *root* ticket's `children:` list to be done too. If the root's `children:` was written to list
  **both** X and Y directly (instead of just X, with Y nested under X's own `children:`), then
  resolving "is root fully done" pulls in Y — and `isSelfOrSplitSibling`'s exclusion (keyed on
  `Parent` equality to the ticket being resolved) doesn't catch this, because Y's `parent` is X,
  not root, so Y doesn't read as X's sibling. Net effect: X can never be considered to unblock
  anything until Y (which can't start until X finishes) is also done — a self-deadlock hiding
  behind a `blocked_by` token that looks resolved (the named ticket really is `done`). Found live
  in `drain-queue`'s `01`→`01a`→`01b` chain and `tickets-tree`'s `03b`→`03b1`→`03b2`,
  `06b`→`06b1`→`06b2`, `06c`→`06c1`→`06c2` (four separate splits, same shape — this looks
  systemic to whatever produced these tickets, not a one-off typo). Inert copies of the same
  malformed shape (already-`done` chains, so harmless) also exist at `tickets-tree`'s
  `02d`→`02e` and `06`→`06c`. Two-part fix: (1) data — corrected the four live tickets'
  `children:`/`parent:` to match (each root lists only its direct split; the intermediate ticket
  lists the grandchild) via `gx tickets set <path> --children <id>`; (2) source —
  `tickets/status.go`'s `isSelfOrSplitSibling` (renamed `isSelfOrSplitSiblingOrDescendant`) now
  also excludes any ticket reached by walking `Parent` hops upward from the candidate to the
  ticket being resolved, not just same-`Parent` siblings, so this shape can't deadlock even if a
  future split's `children:`/`parent:` end up mismatched the same way again. Regression test:
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
