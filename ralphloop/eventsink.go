package ralphloop

import (
	"fmt"
	"io"
	"strings"
	"sync"

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
	EpicComplete(epicName string, completed int, elapsedSeconds int)
}

// noopEventSink implements EventSink with every method a no-op, used
// wherever a launchAndPromptParams is built directly in isolation (mostly
// tests exercising pause/resume plumbing) without wiring up a real sink.
type noopEventSink struct{}

func (noopEventSink) EpicStarted(epicName string, done, total int)                 {}
func (noopEventSink) TicketReverted(identifier string)                             {}
func (noopEventSink) TicketReattached(identifier, label, cwd, sessionID string)    {}
func (noopEventSink) TicketNeedsHuman(identifier, epicName, status, reason string) {}
func (noopEventSink) TicketClaimed(ticket tickets.Ticket)                          {}
func (noopEventSink) IterationStarted(ticket tickets.Ticket, label, cwd, sessionID string) {}
func (noopEventSink) IterationPaused(identifier, label string, kind PauseKind, reason string) {}
func (noopEventSink) IterationResumed(identifier, label string, kind PauseKind)    {}
func (noopEventSink) IterationFinished(ticket tickets.Ticket, epicName string, stats IterationStats) {
}
func (noopEventSink) TranscriptLine(label, line string)                               {}
func (noopEventSink) ContextOccupancy(identifier string, tokens int)                  {}
func (noopEventSink) CherryPickStarted(identifier string)                             {}
func (noopEventSink) ConflictResolutionStarted(identifier string)                     {}
func (noopEventSink) SmartZoneCompactStarted(identifier string)                       {}
func (noopEventSink) SmartZoneFinishingUp(identifier string)                          {}
func (noopEventSink) SmartZoneRecovered(identifier string)                            {}
func (noopEventSink) TicketCleanupFinished(identifier string)                         {}
func (noopEventSink) TicketRecovering(identifier string)                              {}
func (noopEventSink) TicketRecovered(identifier, epicName, branch, landedSHA string)  {}
func (noopEventSink) TicketUnrecoverable(identifier, epicName string)                 {}
func (noopEventSink) EpicParked(epicName string, stalled []StalledTicket)             {}
func (noopEventSink) EpicComplete(epicName string, completed int, elapsedSeconds int) {}

// textEventSink implements EventSink by rendering each event as the same
// text line(s) `gx ralph-loop` has always printed for it, for the headless
// CLI path. TicketClaimed, IterationStarted, and TranscriptLine are no-ops,
// since the CLI has never printed anything for them.
type textEventSink struct {
	mu  sync.Mutex
	out io.Writer
}

// NewTextEventSink returns an EventSink that renders events to out as plain
// text lines, matching gx ralph-loop's historical stdout output exactly.
func NewTextEventSink(out io.Writer) EventSink {
	return &textEventSink{out: out}
}

func (s *textEventSink) printf(format string, args ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(s.out, format, args...)
}

func (s *textEventSink) EpicStarted(epicName string, done, total int) {
	switch {
	case total == 0:
		s.printf("no tickets found for epic %q; nothing to do\n", epicName)
	case done == total:
		s.printf("epic %q is already complete (%d/%d done)\n", epicName, done, total)
	default:
		s.printf("epic %q started (%d/%d done)\n", epicName, done, total)
	}
}

func (s *textEventSink) TicketReverted(identifier string) {
	s.printf("ticket %s: no live iteration found on restart; reverted to open\n", identifier)
}

func (s *textEventSink) TicketReattached(identifier, label, cwd, sessionID string) {
	s.printf("ticket %s: reattaching to live iteration %s\n", identifier, label)
}

func (s *textEventSink) TicketNeedsHuman(identifier, epicName, status, reason string) {
	switch status {
	case "needs-answer":
		s.printf("ticket %s: no commits landed; marked needs-answer\n", identifier)
	default:
		s.printf("ticket %s still needs repair; %s\n", identifier, reason)
	}
}

func (s *textEventSink) TicketClaimed(ticket tickets.Ticket) {}

func (s *textEventSink) IterationStarted(ticket tickets.Ticket, label, cwd, sessionID string) {}

func (s *textEventSink) IterationPaused(identifier, label string, kind PauseKind, reason string) {
	switch kind {
	case PauseRateLimit:
		s.printf("paused %s: %s; waiting for automatic reset\n", label, reason)
	case PauseNeedsRepair:
		s.printf("paused %s: %s\n", label, reason)
	default:
		s.printf("paused %s: %s; run `gx ralph-loop resume` to continue\n", label, reason)
	}
}

func (s *textEventSink) IterationResumed(identifier string, label string, kind PauseKind) {
	switch kind {
	case PauseRateLimit:
		s.printf("resumed %s after rate-limit reset\n", label)
	case PauseNeedsRepair:
		s.printf("resumed %s after operator intervention\n", label)
	default:
		s.printf("resumed %s\n", label)
	}
}

func (s *textEventSink) IterationFinished(ticket tickets.Ticket, epicName string, stats IterationStats) {
	s.printf("ticket %s %q landed on %s\n", ticket.Identifier, ticket.Title, epicName)
}

func (s *textEventSink) TranscriptLine(label, line string) {}

func (s *textEventSink) ContextOccupancy(identifier string, tokens int) {}

func (s *textEventSink) CherryPickStarted(identifier string) {}

func (s *textEventSink) ConflictResolutionStarted(identifier string) {}

func (s *textEventSink) SmartZoneCompactStarted(identifier string) {
	s.printf("ticket %s: context budget exceeded; compacting...\n", identifier)
}

func (s *textEventSink) SmartZoneFinishingUp(identifier string) {
	s.printf("ticket %s: compacted; telling the agent to finish up...\n", identifier)
}

func (s *textEventSink) SmartZoneRecovered(identifier string) {}

func (s *textEventSink) TicketCleanupFinished(identifier string) {
	s.printf("ticket %s: done and commits landed, but leftover iteration state was never cleaned up; finished the interrupted cleanup\n", identifier)
}

func (s *textEventSink) TicketRecovering(identifier string) {}

func (s *textEventSink) TicketRecovered(identifier, epicName, branch, landedSHA string) {
	s.printf("ticket %s: done but commits were missing from %s; auto re-cherry-picked from iteration branch %s and restored (%s)\n", identifier, epicName, branch, landedSHA)
}

func (s *textEventSink) TicketUnrecoverable(identifier, epicName string) {
	s.printf("ticket %s: done but commits missing from %s and no iteration branch left to recover them; marked needs-repair\n", identifier, epicName)
}

func (s *textEventSink) EpicParked(epicName string, stalled []StalledTicket) {
	identifiers := make([]string, len(stalled))
	for i, t := range stalled {
		identifiers[i] = t.Identifier
	}
	s.printf("ralph-loop %q parked: nothing runnable left; waiting on %s\n", epicName, strings.Join(identifiers, ", "))
}

func (s *textEventSink) EpicComplete(epicName string, completed int, elapsedSeconds int) {
	s.printf("ralph-loop %q complete: %d ticket(s) landed on %s\n", epicName, completed, epicName)
}
