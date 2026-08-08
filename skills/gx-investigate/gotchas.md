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
