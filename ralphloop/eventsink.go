package ralphloop

import (
	"github.com/elentok/gx/tickets"
)

// PauseKind distinguishes why an iteration paused, since rendering (the
// headless text sink today, a future TUI tomorrow) says something different
// for each: a rate-limit pause clears itself on a timer, while a
// needs-repair pause needs a human at the agent's pane. A smart-zone
// breach is deliberately not a PauseKind: recoverSmartZoneBreach never calls
// Gate.pause (the scheduler keeps running other tickets throughout), so it
// reports through SmartZoneCompactStarted/SmartZoneFinishingUp/
// SmartZoneRecovered below instead — phase changes on a still-running
// iteration, not a pause an operator could ever be asked to resume.
type PauseKind string

// IterationStats carries the data IterationFinished needs beyond the ticket
// and epic name: the landed iteration's own metrics plus the epic's live
// progress counts, grouped into one type rather than five loose ints.
// Completed and Total describe the epic as a whole — recomputed from the
// scan loop's already-loaded epic on every landed iteration, not counted up
// by this Run call — so a resumed or scoped run reports the same numbers a
// fresh run over the same epic would (see RunScope.DoneCount/TotalCount).
type IterationStats struct {
	ElapsedSeconds    int
	PeakContextTokens int
	Cost              float64
	InProgress        int
	Completed         int
	Total             int
}

const (
	PauseRateLimit PauseKind = "rate-limit"
	// PauseNeedsRepair marks the fault-side recovery an iteration goroutine's
	// own error drives (loop.go's r.err handling): the ticket is flagged
	// needs-repair and dropped out of the frontier for a human to clear, while
	// the scheduler itself keeps running every other ticket. A renderer (see
	// ticket 04a) treats it as its own "needs repair" state rather than a
	// generic pause. A pane blocked on an operator prompt is a different park
	// entirely (needs-answer, no pause — see parkOnBlockedPane).
	PauseNeedsRepair PauseKind = "needs-repair"
)

// StalledTicket names one human-clearable ticket a parked epic is waiting
// on. Reattachable reports whether its prior iteration still has a live,
// owned herdr tab/agent (see resumeReattachable) — the epic-parked half of
// distinguishing a park a human clears by editing status alone from one a
// live run can also self-heal from once cleared.
type StalledTicket struct {
	Identifier   string
	Reattachable bool
}

// EventSink receives every orchestrator-lifecycle event a `gx ralph-loop`
// invocation produces, so the headless CLI and the TUI (tickets 03-06) can
// each render the same underlying event stream their own way instead of the
// engine committing to one output format itself. Every method must be safe
// to call concurrently: each running iteration reports through the same
// sink from its own goroutine.
type EventSink interface {
	// EpicStarted reports that epicName has begun running: fired exactly
	// once per epic that leaves the queue, after the concurrency permit is
	// acquired (not at Run's entry, so a queued epic waiting for a slot
	// never announces a start it hasn't reached yet). This folds what used
	// to be two separate events — no tickets at all, and every ticket
	// already done — into the same single start message (done/total cover
	// both: total 0 for no tickets, done == total for already complete), so
	// every epic that leaves the queue emits exactly one start message and
	// a missing one is itself meaningful.
	EpicStarted(epicName string, done, total int)

	// TicketReverted reports that a ticket left `Status: claimed` by a prior
	// crashed/killed invocation had no live iteration to reattach to, and was
	// reverted to open for normal scheduling. identifier is the ticket's
	// Identifier (not Number), so lettered split siblings sharing a Number
	// (see UnresolvedBlockers) render distinctly.
	TicketReverted(identifier string)
	// TicketReattached reports that a ticket left `Status: claimed` or
	// `Status: needs-repair` by a prior invocation still has a live
	// iteration, being resumed under label. cwd/sessionID (best-effort,
	// recovered from the run log's last iteration-started event; empty if
	// none found) let a consumer resolve the session's transcript itself, so
	// elapsed time survives a reattach instead of resetting to zero.
	TicketReattached(identifier string, label string, cwd string, sessionID string)
	// TicketNeedsHuman reports that a machine parked identifier on epicName
	// for a person, fired at gx's parking write itself (the gate's write, the
	// adoption path's write, and each fault-side write) so coverage is total
	// rather than incidental. status is a plain string ("needs-answer" /
	// "needs-repair", matching schema.StatusNeedsAnswer/StatusNeedsRepair's
	// raw values) — ask-versus-fault is a rendering choice a consumer makes
	// from status, not a plumbing fork into separate events the way
	// TicketNeedsAnswer/TicketStillNeedsRepair/the park-flavored
	// IterationPaused used to be.
	TicketNeedsHuman(identifier, epicName, status, reason string)

	// TicketClaimed reports that ticket was claimed off the frontier for a
	// fresh iteration.
	TicketClaimed(ticket tickets.Ticket)
	// IterationStarted reports that label's agent has launched and been sent
	// its initial prompt, for ticket. cwd/sessionID let a consumer resolve
	// the session's transcript itself (transcript.Path) to compute elapsed
	// time from its first line's timestamp, rather than stamping "now"
	// client-side.
	IterationStarted(ticket tickets.Ticket, label string, cwd string, sessionID string)
	// IterationPaused reports that identifier's iteration label paused for
	// reason, of the given kind. identifier is the ticket's Identifier (see
	// TicketReverted) — label alone is barred from a message's identity line,
	// so without it this event can't be attributed to a ticket.
	IterationPaused(identifier string, label string, kind PauseKind, reason string)
	// IterationResumed reports that identifier's iteration label resumed from
	// a kind pause.
	IterationResumed(identifier string, label string, kind PauseKind)
	// IterationFinished reports that ticket's iteration landed its commits on
	// epicName, along with stats about the landed iteration and the epic's
	// live progress.
	IterationFinished(ticket tickets.Ticket, epicName string, stats IterationStats)
	// TranscriptLine reports one line of label's live agent transcript.
	TranscriptLine(label, line string)
	// ContextOccupancy reports identifier's current context-window token
	// occupancy, emitted from waitForFinish's existing 30s smart-zone poll
	// (no dedicated poll loop of its own) plus once immediately at
	// IterationStarted/TicketReattached time, so a consumer never shows a
	// misleading "0 tok" for up to 30s after starting/reattaching.
	ContextOccupancy(identifier string, tokens int)

	// CherryPickStarted reports that identifier's finished iteration is now
	// being cherry-picked onto the feature branch.
	CherryPickStarted(identifier string)
	// ConflictResolutionStarted reports that identifier's cherry-pick hit a
	// conflict and a resolution agent has launched in the feature worktree to
	// fix it.
	ConflictResolutionStarted(identifier string)

	// SmartZoneCompactStarted reports that identifier's running iteration hit
	// the --smart-zone ceiling and is being auto-compacted in place. This is a
	// phase change on a still-running iteration, not a pause: the scheduler
	// keeps claiming and running other tickets throughout.
	SmartZoneCompactStarted(identifier string)
	// SmartZoneFinishingUp reports that identifier's compaction finished and
	// it's now being re-prompted to wrap up quickly.
	SmartZoneFinishingUp(identifier string)
	// SmartZoneRecovered reports that identifier's smart-zone recovery
	// sequence is over (whether or not it actually completed cleanly — see
	// recoverSmartZoneBreach), so a renderer can drop the compacting/
	// finishing-up phase suffix and go back to its normal running rendering.
	SmartZoneRecovered(identifier string)

	// TicketCleanupFinished reports that a done ticket's commits had already
	// landed, but a crash left its worktree/tab/branch behind between marking
	// it done and cleaning up right after — startup reconciliation just
	// finished that interrupted cleanup.
	TicketCleanupFinished(identifier string)
	// TicketRecovering reports that startup reconciliation is about to
	// re-cherry-pick identifier's commits from a surviving iteration branch
	// (a doneRecoverable ticket, or an orphaned claim with unlanded commits)
	// — fired before the CherryPickStarted/ConflictResolutionStarted events
	// that follow, so a renderer has a live row to update: unlike a normal
	// iteration, there's no LiveEventIterationStarted/TicketReattached ahead
	// of this recovery to seed one.
	TicketRecovering(identifier string)
	// TicketRecovered reports that a done ticket's commits were missing from
	// epicName (a crash landed between cherry-pick and marking done) but its
	// iteration branch still held them, so startup reconciliation
	// re-cherry-picked and restored them as landedSHA.
	TicketRecovered(identifier, epicName, branch, landedSHA string)
	// TicketUnrecoverable reports that a done ticket's commits are missing
	// from epicName and no iteration branch survived to recover them from —
	// flagged needs-repair for a human to inspect.
	TicketUnrecoverable(identifier, epicName string)

	// EpicParked reports that epicName has no runnable work left and is
	// waiting, without a timeout, on a person to clear one of the stalled
	// tickets (identifiers, in file order). Unlike EpicComplete this is not
	// the end of the run: the run keeps polling and carries on by itself once
	// a status clears.
	EpicParked(epicName string, stalled []StalledTicket)
	// EpicComplete reports that every ticket in epicName is done, with
	// completed tickets landed by this Run call, after elapsedSeconds of
	// total wall-clock run time.
	//
	// Notification-only invariant: every current implementer (chatEventSink,
	// ChannelEventSink, noopEventSink) only notifies — sends a chat message or
	// forwards a LiveEvent — and never itself persists ticket/epic state to
	// disk or elsewhere. Run's drain-exit path (see the drained guard in
	// loop.go) relies on this: it skips this call wholesale for a drained run
	// rather than only suppressing its chat/toast half, on the assumption
	// there's no other, non-notification behavior to lose. See
	// TestEpicComplete_ChatSinkIsNotificationOnly in
	// eventsink_contract_test.go, which pins this for chatEventSink (the one
	// implementer with enough behavior to plausibly grow a state mutation). A
	// future implementer that persists state here would need Run's drain path
	// updated to call it separately from the chat/toast half.
	EpicComplete(epicName string, completed int, elapsedSeconds int)
	// DrainComplete reports that epicName's run ended specifically because it
	// was draining (see Gate.Drain) — whether that end was immediate
	// (nothing in flight when Drain was called) or came after the last
	// in-flight iteration finished. It fires once per drained run,
	// immediately before the EpicComplete call every run's end reaches
	// (drained or not), and never fires for an ordinary non-draining
	// completion.
	DrainComplete(epicName string, completed int, elapsedSeconds int)

	// EpicFailed reports that epicName's Run call returned err. Unlike every
	// other event here, this one is not fired from inside the run: by the
	// time the loop registry records a run's failure the run has already
	// returned and its sink has been closed and drained, so there is no
	// live run-scoped emitter left to call this through. The registry's
	// EpicFailureReporter (see epic_failure_reporter.go) is the one path
	// that calls it — a chat message emitted straight to the transport
	// after the drain, not through this interface — which is why
	// EpicFailed exists on EventSink at all: purely so the contract test in
	// eventsink_contract_test.go can record its chat membership, not
	// because any EventSink implementation dispatches it.
	EpicFailed(epicName string, err error)

	// NotificationFailed reports that a chat notification send to channel
	// ("telegram"/"slack") ultimately failed (after chatEventSink's own
	// retry) — reason is the sanitized error text also written to
	// run-log.jsonl's notification-failed entry. TUI-only, never itself
	// re-dispatched to chat (that would risk looping a broken send back
	// through the very transport that just failed): only chatEventSink
	// calls this, on its own embedded EventSink, when its own send fails —
	// never something ralphloop's scheduling logic reports.
	NotificationFailed(channel, reason string)
}

// noopEventSink implements EventSink with every method a no-op, used
// wherever a launchAndPromptParams is built directly in isolation (mostly
// tests exercising pause/resume plumbing) without wiring up a real sink.
type noopEventSink struct{}

func (noopEventSink) EpicStarted(epicName string, done, total int)                            {}
func (noopEventSink) TicketReverted(identifier string)                                        {}
func (noopEventSink) TicketReattached(identifier, label, cwd, sessionID string)               {}
func (noopEventSink) TicketNeedsHuman(identifier, epicName, status, reason string)            {}
func (noopEventSink) TicketClaimed(ticket tickets.Ticket)                                     {}
func (noopEventSink) IterationStarted(ticket tickets.Ticket, label, cwd, sessionID string)    {}
func (noopEventSink) IterationPaused(identifier, label string, kind PauseKind, reason string) {}
func (noopEventSink) IterationResumed(identifier, label string, kind PauseKind)               {}
func (noopEventSink) IterationFinished(ticket tickets.Ticket, epicName string, stats IterationStats) {
}
func (noopEventSink) TranscriptLine(label, line string)                                {}
func (noopEventSink) ContextOccupancy(identifier string, tokens int)                   {}
func (noopEventSink) CherryPickStarted(identifier string)                              {}
func (noopEventSink) ConflictResolutionStarted(identifier string)                      {}
func (noopEventSink) SmartZoneCompactStarted(identifier string)                        {}
func (noopEventSink) SmartZoneFinishingUp(identifier string)                           {}
func (noopEventSink) SmartZoneRecovered(identifier string)                             {}
func (noopEventSink) TicketCleanupFinished(identifier string)                          {}
func (noopEventSink) TicketRecovering(identifier string)                               {}
func (noopEventSink) TicketRecovered(identifier, epicName, branch, landedSHA string)   {}
func (noopEventSink) TicketUnrecoverable(identifier, epicName string)                  {}
func (noopEventSink) EpicParked(epicName string, stalled []StalledTicket)              {}
func (noopEventSink) EpicComplete(epicName string, completed int, elapsedSeconds int)  {}
func (noopEventSink) EpicFailed(epicName string, err error)                            {}
func (noopEventSink) NotificationFailed(channel, reason string)                        {}
func (noopEventSink) DrainComplete(epicName string, completed int, elapsedSeconds int) {}
