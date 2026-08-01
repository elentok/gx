package ralphloop

import (
	"fmt"
	"io"
	"sync"

	"github.com/elentok/gx/tickets"
)

// PauseKind distinguishes why an iteration paused, since rendering (the
// headless text sink today, a future TUI tomorrow) says something different
// for each: a rate-limit pause clears itself on a timer, while a smart-zone
// pause needs an operator to run `gx ralph-loop resume`.
type PauseKind string

const (
	PauseRateLimit PauseKind = "rate-limit"
	PauseSmartZone PauseKind = "smart-zone"
	// PauseNeedsAttention marks the operator-intervention pause
	// waitForAttentionRecovery drives (Codex blocked on a permission/
	// intervention prompt): mechanically it's paused through the same
	// pauseGate as the other two kinds, but a renderer (see ticket 04a) treats
	// it as its own "needs attention" state rather than a generic pause,
	// since it needs a human at the agent's pane rather than clearing itself
	// or via `gx ralph-loop resume`.
	PauseNeedsAttention PauseKind = "needs-attention"
)

// EventSink receives every orchestrator-lifecycle event a `gx ralph-loop`
// invocation produces, so the headless CLI and the TUI (tickets 03-06) can
// each render the same underlying event stream their own way instead of the
// engine committing to one output format itself. Every method must be safe
// to call concurrently: each running iteration reports through the same
// sink from its own goroutine.
type EventSink interface {
	// NoTicketsFound reports that epicName has no tickets at all; Run exits
	// immediately without doing anything else.
	NoTicketsFound(epicName string)
	// AlreadyComplete reports that every one of epicName's tickets was
	// already done/needs-info before Run did anything.
	AlreadyComplete(epicName string, done, total int)

	// TicketReverted reports that a ticket left `Status: claimed` by a prior
	// crashed/killed invocation had no live iteration to reattach to, and was
	// reverted to open for normal scheduling. identifier is the ticket's
	// Identifier (not Number), so lettered split siblings sharing a Number
	// (see UnresolvedBlockers) render distinctly.
	TicketReverted(identifier string)
	// TicketReattached reports that a ticket left `Status: claimed` or
	// `Status: needs-attention` by a prior invocation still has a live
	// iteration, being resumed under label.
	TicketReattached(identifier string, label string)
	// TicketStillNeedsAttention reports that a needs-attention ticket has no
	// live iteration to reattach to — it stays needs-attention for a human to
	// inspect, unlike a claimed ticket in the same spot (see TicketReverted).
	TicketStillNeedsAttention(identifier string)

	// TicketClaimed reports that ticket was claimed off the frontier for a
	// fresh iteration.
	TicketClaimed(ticket tickets.Ticket)
	// IterationStarted reports that label's agent has launched and been sent
	// its initial prompt. identifier is the ticket's Identifier (see
	// TicketReverted).
	IterationStarted(identifier string, label string)
	// IterationPaused reports that label paused for reason, of the given
	// kind.
	IterationPaused(label string, kind PauseKind, reason string)
	// IterationResumed reports that label resumed from a kind pause.
	IterationResumed(label string, kind PauseKind)
	// IterationFinished reports that ticket's iteration landed its commits on
	// epicName.
	IterationFinished(ticket tickets.Ticket, epicName string)
	// TranscriptLine reports one line of label's live agent transcript.
	TranscriptLine(label, line string)

	// TicketCleanupFinished reports that a done ticket's commits had already
	// landed, but a crash left its worktree/tab/branch behind between marking
	// it done and cleaning up right after — startup reconciliation just
	// finished that interrupted cleanup.
	TicketCleanupFinished(identifier string)
	// TicketRecovered reports that a done ticket's commits were missing from
	// epicName (a crash landed between cherry-pick and marking done) but its
	// iteration branch still held them, so startup reconciliation
	// re-cherry-picked and restored them as landedSHA.
	TicketRecovered(identifier, epicName, branch, landedSHA string)
	// TicketUnrecoverable reports that a done ticket's commits are missing
	// from epicName and no iteration branch survived to recover them from —
	// flagged needs-attention for a human to inspect.
	TicketUnrecoverable(identifier, epicName string)

	// EpicComplete reports that every ticket in epicName reached a terminal
	// state, with completed tickets landed by this Run call.
	EpicComplete(epicName string, completed int)
}

// noopEventSink implements EventSink with every method a no-op, used
// wherever a launchAndPromptParams is built directly in isolation (mostly
// tests exercising pause/resume plumbing) without wiring up a real sink.
type noopEventSink struct{}

func (noopEventSink) NoTicketsFound(epicName string)                                 {}
func (noopEventSink) AlreadyComplete(epicName string, done, total int)               {}
func (noopEventSink) TicketReverted(identifier string)                               {}
func (noopEventSink) TicketReattached(identifier string, label string)               {}
func (noopEventSink) TicketStillNeedsAttention(identifier string)                    {}
func (noopEventSink) TicketClaimed(ticket tickets.Ticket)                            {}
func (noopEventSink) IterationStarted(identifier string, label string)               {}
func (noopEventSink) IterationPaused(label string, kind PauseKind, reason string)    {}
func (noopEventSink) IterationResumed(label string, kind PauseKind)                  {}
func (noopEventSink) IterationFinished(ticket tickets.Ticket, epicName string)       {}
func (noopEventSink) TranscriptLine(label, line string)                              {}
func (noopEventSink) TicketCleanupFinished(identifier string)                        {}
func (noopEventSink) TicketRecovered(identifier, epicName, branch, landedSHA string) {}
func (noopEventSink) TicketUnrecoverable(identifier, epicName string)                {}
func (noopEventSink) EpicComplete(epicName string, completed int)                    {}

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

func (s *textEventSink) NoTicketsFound(epicName string) {
	s.printf("no tickets found for epic %q; nothing to do\n", epicName)
}

func (s *textEventSink) AlreadyComplete(epicName string, done, total int) {
	s.printf("epic %q is already complete (%d/%d done)\n", epicName, done, total)
}

func (s *textEventSink) TicketReverted(identifier string) {
	s.printf("ticket %s: no live iteration found on restart; reverted to open\n", identifier)
}

func (s *textEventSink) TicketReattached(identifier string, label string) {
	s.printf("ticket %s: reattaching to live iteration %s\n", identifier, label)
}

func (s *textEventSink) TicketStillNeedsAttention(identifier string) {
	s.printf("ticket %s still needs attention; no live iteration found\n", identifier)
}

func (s *textEventSink) TicketClaimed(ticket tickets.Ticket) {}

func (s *textEventSink) IterationStarted(identifier string, label string) {}

func (s *textEventSink) IterationPaused(label string, kind PauseKind, reason string) {
	switch kind {
	case PauseRateLimit:
		s.printf("paused %s: %s; waiting for automatic reset\n", label, reason)
	case PauseNeedsAttention:
		s.printf("paused %s: %s\n", label, reason)
	default:
		s.printf("paused %s: %s; run `gx ralph-loop resume` to continue\n", label, reason)
	}
}

func (s *textEventSink) IterationResumed(label string, kind PauseKind) {
	switch kind {
	case PauseRateLimit:
		s.printf("resumed %s after rate-limit reset\n", label)
	case PauseNeedsAttention:
		s.printf("resumed %s after operator intervention\n", label)
	default:
		s.printf("resumed %s\n", label)
	}
}

func (s *textEventSink) IterationFinished(ticket tickets.Ticket, epicName string) {
	s.printf("ticket %s %q landed on %s\n", ticket.Identifier, ticket.Title, epicName)
}

func (s *textEventSink) TranscriptLine(label, line string) {}

func (s *textEventSink) TicketCleanupFinished(identifier string) {
	s.printf("ticket %s: done and commits landed, but leftover iteration state was never cleaned up; finished the interrupted cleanup\n", identifier)
}

func (s *textEventSink) TicketRecovered(identifier, epicName, branch, landedSHA string) {
	s.printf("ticket %s: done but commits were missing from %s; auto re-cherry-picked from iteration branch %s and restored (%s)\n", identifier, epicName, branch, landedSHA)
}

func (s *textEventSink) TicketUnrecoverable(identifier, epicName string) {
	s.printf("ticket %s: done but commits missing from %s and no iteration branch left to recover them; marked needs-attention\n", identifier, epicName)
}

func (s *textEventSink) EpicComplete(epicName string, completed int) {
	s.printf("ralph-loop %q complete: %d ticket(s) landed on %s\n", epicName, completed, epicName)
}
