# 06 — Persistent cross-tab loop-status indicator

**What to build:** While a loop is active, show a persistent status indicator on every tab (not
just the tickets tab) — a spinner plus text like "Implementing {epic} (ticket X/Y)...", updating as
the loop progresses. Composite it the same way the app shell already composites its other
always-on-top elements (the notify-toast stack and chord-hint overlay in `ui/app/model.go`'s
`View()`) so it survives tab switches without each tab needing to know about it. Drive it from the
same live-event stream ticket 02 uses for in-tab highlighting (event kind, current ticket,
done/total counts) rather than a separate polling mechanism. No indicator when no loop is running.

**Blocked by:** 01

**Status:** done
**Context window:** 61905
**Session:** 8b774c15-46b8-4ec3-820d-354c5115d497

**Split:** 06b — this ticket landed the data side only:
`ui/tickets.LoopStatus()`/`ralphLoopRegistry` now track `done`/`total` progress, seeded from
`epic.DoneCount()`/`TotalCount()` at launch and advanced by a live-event drain goroutine on
`LiveEventIterationFinished`. The app-shell overlay itself (rendering, compositing into
`ui/app/model.go`'s `View()`, and the cross-tab test) is scoped to 06b.
Tokens used: ~115K (compacted once; see 06b for remaining acceptance criteria).

Code-review fixes: none (not run — scope reduced to the data-plumbing half; 06b will carry its own
code-review pass once implemented)

- [x] Loop progress (done/total ticket count) is tracked live via the same event stream ticket 02
      uses, not disk polling alone (seeded from disk at launch, advanced via
      `LiveEventIterationFinished`)
- [ ] *(moved to 06b)* A spinner + status line showing the running epic name and current ticket
      appears on every tab while a loop is active
- [ ] *(moved to 06b)* The status updates live as the loop progresses through tickets/phases
- [ ] *(moved to 06b)* The indicator disappears when the loop finishes or is not running
- [ ] *(moved to 06b)* Switching tabs does not hide or reset the indicator
- [ ] *(moved to 06b)* A test drives synthetic live events and asserts the overlay renders on a
      non-tickets tab
