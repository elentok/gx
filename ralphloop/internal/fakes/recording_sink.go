package fakes

import (
	"sync"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets"
)

// RecordingSink implements EventSink by appending each call's method name to
// a slice, for asserting the exact event sequence a Run call produces.
type RecordingSink struct {
	mu                     sync.Mutex
	calls                  []string
	LastIterationStats     runlog.IterationStats
	LastEpicElapsedSeconds int
	// TicketNeedsHumanCalls records every TicketNeedsHuman call's
	// (identifier, epicName, status, reason) tuple, for asserting both the
	// count and the identity/status a pausing-write transition reports.
	TicketNeedsHumanCalls [][4]string
}

func (s *RecordingSink) record(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, name)
}

// Snapshot returns the recorded method names in call order.
func (s *RecordingSink) Snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.calls))
	copy(out, s.calls)
	return out
}

func (s *RecordingSink) EpicStarted(epicName string, done, total int) { s.record("EpicStarted") }
func (s *RecordingSink) TicketReverted(identifier string)             { s.record("TicketReverted") }
func (s *RecordingSink) TicketReattached(identifier, label, cwd, sessionID string) {
	s.record("TicketReattached")
}
func (s *RecordingSink) TicketNeedsHuman(identifier, epicName, status string, _ events.Kind, reason string) {
	s.mu.Lock()
	s.TicketNeedsHumanCalls = append(s.TicketNeedsHumanCalls, [4]string{identifier, epicName, status, reason})
	s.mu.Unlock()
	s.record("TicketNeedsHuman")
}
func (s *RecordingSink) TicketClaimed(ticket tickets.Ticket) { s.record("TicketClaimed") }
func (s *RecordingSink) IterationStarted(ticket tickets.Ticket, label, cwd, sessionID string, agent agent.AgentKind, paneID string) {
	s.record("IterationStarted")
}
func (s *RecordingSink) IterationPaused(identifier, label string, kind runlog.PauseKind, reason string) {
	s.record("IterationPaused")
}
func (s *RecordingSink) IterationResumed(identifier, label string, kind runlog.PauseKind) {
	s.record("IterationResumed")
}
func (s *RecordingSink) IterationFinished(ticket tickets.Ticket, epicName string, stats runlog.IterationStats) {
	s.mu.Lock()
	s.LastIterationStats = stats
	s.mu.Unlock()
	s.record("IterationFinished")
}
func (s *RecordingSink) TranscriptLine(label, line string) { s.record("TranscriptLine") }
func (s *RecordingSink) ContextOccupancy(identifier string, tokens int) {
	s.record("ContextOccupancy")
}
func (s *RecordingSink) TicketCleanupFinished(identifier string) {
	s.record("TicketCleanupFinished")
}
func (s *RecordingSink) TicketRecovering(identifier string) {
	s.record("TicketRecovering")
}
func (s *RecordingSink) TicketRecovered(identifier, epicName, branch, mergedSHA string) {
	s.record("TicketRecovered")
}
func (s *RecordingSink) TicketUnrecoverable(identifier, epicName string) {
	s.record("TicketUnrecoverable")
}
func (s *RecordingSink) EpicPaused(epicName string, stalled []runlog.StalledTicket) {
	s.record("EpicPaused")
}
func (s *RecordingSink) EpicComplete(epicName string, completed int, elapsedSeconds int) {
	s.mu.Lock()
	s.LastEpicElapsedSeconds = elapsedSeconds
	s.mu.Unlock()
	s.record("EpicComplete")
}
func (s *RecordingSink) EpicFailed(epicName string, err error) {
	s.record("EpicFailed")
}
func (s *RecordingSink) NotificationFailed(channel, reason string) {
	s.record("NotificationFailed")
}
func (s *RecordingSink) CherryPickStarted(identifier string) { s.record("CherryPickStarted") }
func (s *RecordingSink) ConflictResolutionStarted(identifier string) {
	s.record("ConflictResolutionStarted")
}
func (s *RecordingSink) SmartZoneCompactStarted(identifier string) {
	s.record("SmartZoneCompactStarted")
}
func (s *RecordingSink) SmartZoneFinishingUp(identifier string) { s.record("SmartZoneFinishingUp") }
func (s *RecordingSink) SmartZoneRecovered(identifier string)   { s.record("SmartZoneRecovered") }
func (s *RecordingSink) VerifyCapStopped(identifier string)     { s.record("VerifyCapStopped") }
