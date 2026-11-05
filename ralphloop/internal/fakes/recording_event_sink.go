package fakes

import (
	"sync"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets"
)

// LiveEventKind identifies which EventSink method produced a LiveEvent.
type LiveEventKind int

const (
	LiveEventEpicStarted LiveEventKind = iota
	LiveEventTicketReverted
	LiveEventTicketReattached
	LiveEventTicketNeedsHuman
	LiveEventTicketClaimed
	LiveEventIterationStarted
	LiveEventIterationPaused
	LiveEventIterationResumed
	LiveEventIterationFinished
	LiveEventTranscriptLine
	LiveEventTicketCleanupFinished
	LiveEventTicketRecovering
	LiveEventTicketRecovered
	LiveEventTicketUnrecoverable
	LiveEventEpicPaused
	LiveEventEpicComplete
	LiveEventCherryPickStarted
	LiveEventConflictResolutionStarted
	LiveEventSmartZoneCompactStarted
	LiveEventSmartZoneFinishingUp
	LiveEventSmartZoneRecovered
	LiveEventVerifyCapStopped
	LiveEventContextOccupancy
	LiveEventNotificationFailed
)

// LiveEvent captures one EventSink call: a single struct with the union of
// every method's arguments, since most fields only matter for a few Kinds.
// Identifier is the ticket's Identifier (not Number) for every kind that
// names a ticket, matching EventSink's own convention.
type LiveEvent struct {
	Kind LiveEventKind

	EpicName   string
	Identifier string
	Label      string
	Reason     string
	Status     string
	PauseKind  runlog.PauseKind
	Line       string
	Ticket     tickets.Ticket
	Done       int
	Total      int
	Completed  int
	Branch     string
	MergedSHA  string
	Cwd        string
	SessionID  string
	AgentKind  agent.AgentKind
	PaneID     string
	// Tokens is ContextOccupancy's context-window token count.
	Tokens int
	// Stats is IterationFinished's metrics and epic progress counts.
	Stats runlog.IterationStats
	// ElapsedSeconds is EpicComplete's total wall-clock duration.
	ElapsedSeconds int
	// Stalled is EpicPaused's human-clearable tickets.
	Stalled []runlog.StalledTicket
	// Channel is NotificationFailed's chat transport ("telegram"/"slack").
	Channel string
}

// RecordingEventSink implements EventSink by recording every call as a
// LiveEvent in call order, synchronously, so a test can call Events() right
// after Run() returns with no goroutine to coordinate.
type RecordingEventSink struct {
	mu     sync.Mutex
	events []LiveEvent
}

func NewRecordingEventSink() *RecordingEventSink {
	return &RecordingEventSink{}
}

// HasEvent reports whether sink recorded an event of kind matching match.
func HasEvent(sink *RecordingEventSink, kind LiveEventKind, match func(LiveEvent) bool) bool {
	for _, ev := range sink.Events() {
		if ev.Kind == kind && match(ev) {
			return true
		}
	}
	return false
}

// Events returns every event recorded so far, in call order.
func (s *RecordingEventSink) Events() []LiveEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]LiveEvent(nil), s.events...)
}

func (s *RecordingEventSink) record(ev LiveEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
}

func (s *RecordingEventSink) EpicStarted(epicName string, done, total int) {
	s.record(LiveEvent{Kind: LiveEventEpicStarted, EpicName: epicName, Done: done, Total: total})
}

func (s *RecordingEventSink) TicketReverted(identifier string) {
	s.record(LiveEvent{Kind: LiveEventTicketReverted, Identifier: identifier})
}

func (s *RecordingEventSink) TicketReattached(identifier, label, cwd, sessionID string) {
	s.record(LiveEvent{Kind: LiveEventTicketReattached, Identifier: identifier, Label: label, Cwd: cwd, SessionID: sessionID})
}

func (s *RecordingEventSink) TicketNeedsHuman(identifier, epicName, status string, _ events.Kind, reason string) {
	s.record(LiveEvent{Kind: LiveEventTicketNeedsHuman, Identifier: identifier, EpicName: epicName, Status: status, Reason: reason})
}

func (s *RecordingEventSink) TicketClaimed(ticket tickets.Ticket) {
	s.record(LiveEvent{Kind: LiveEventTicketClaimed, Identifier: ticket.Identifier, Ticket: ticket})
}

func (s *RecordingEventSink) IterationStarted(ticket tickets.Ticket, label, cwd, sessionID string, agent agent.AgentKind, paneID string) {
	s.record(LiveEvent{Kind: LiveEventIterationStarted, Identifier: ticket.Identifier, Ticket: ticket, Label: label, Cwd: cwd, SessionID: sessionID, AgentKind: agent, PaneID: paneID})
}

func (s *RecordingEventSink) IterationPaused(identifier, label string, kind runlog.PauseKind, reason string) {
	s.record(LiveEvent{Kind: LiveEventIterationPaused, Identifier: identifier, Label: label, PauseKind: kind, Reason: reason})
}

func (s *RecordingEventSink) IterationResumed(identifier, label string, kind runlog.PauseKind) {
	s.record(LiveEvent{Kind: LiveEventIterationResumed, Identifier: identifier, Label: label, PauseKind: kind})
}

func (s *RecordingEventSink) IterationFinished(ticket tickets.Ticket, epicName string, stats runlog.IterationStats) {
	s.record(LiveEvent{Kind: LiveEventIterationFinished, Identifier: ticket.Identifier, Ticket: ticket, EpicName: epicName, Stats: stats})
}

func (s *RecordingEventSink) TranscriptLine(label, line string) {
	s.record(LiveEvent{Kind: LiveEventTranscriptLine, Label: label, Line: line})
}

func (s *RecordingEventSink) ContextOccupancy(identifier string, tokens int) {
	s.record(LiveEvent{Kind: LiveEventContextOccupancy, Identifier: identifier, Tokens: tokens})
}

func (s *RecordingEventSink) CherryPickStarted(identifier string) {
	s.record(LiveEvent{Kind: LiveEventCherryPickStarted, Identifier: identifier})
}

func (s *RecordingEventSink) ConflictResolutionStarted(identifier string) {
	s.record(LiveEvent{Kind: LiveEventConflictResolutionStarted, Identifier: identifier})
}

func (s *RecordingEventSink) SmartZoneCompactStarted(identifier string) {
	s.record(LiveEvent{Kind: LiveEventSmartZoneCompactStarted, Identifier: identifier})
}

func (s *RecordingEventSink) SmartZoneFinishingUp(identifier string) {
	s.record(LiveEvent{Kind: LiveEventSmartZoneFinishingUp, Identifier: identifier})
}

func (s *RecordingEventSink) SmartZoneRecovered(identifier string) {
	s.record(LiveEvent{Kind: LiveEventSmartZoneRecovered, Identifier: identifier})
}

func (s *RecordingEventSink) VerifyCapStopped(identifier string) {
	s.record(LiveEvent{Kind: LiveEventVerifyCapStopped, Identifier: identifier})
}

func (s *RecordingEventSink) TicketCleanupFinished(identifier string) {
	s.record(LiveEvent{Kind: LiveEventTicketCleanupFinished, Identifier: identifier})
}

func (s *RecordingEventSink) TicketRecovering(identifier string) {
	s.record(LiveEvent{Kind: LiveEventTicketRecovering, Identifier: identifier})
}

func (s *RecordingEventSink) TicketRecovered(identifier, epicName, branch, mergedSHA string) {
	s.record(LiveEvent{Kind: LiveEventTicketRecovered, Identifier: identifier, EpicName: epicName, Branch: branch, MergedSHA: mergedSHA})
}

func (s *RecordingEventSink) TicketUnrecoverable(identifier, epicName string) {
	s.record(LiveEvent{Kind: LiveEventTicketUnrecoverable, Identifier: identifier, EpicName: epicName})
}

func (s *RecordingEventSink) EpicPaused(epicName string, stalled []runlog.StalledTicket) {
	s.record(LiveEvent{Kind: LiveEventEpicPaused, EpicName: epicName, Stalled: stalled})
}

func (s *RecordingEventSink) EpicComplete(epicName string, completed int, elapsedSeconds int) {
	s.record(LiveEvent{Kind: LiveEventEpicComplete, EpicName: epicName, Completed: completed, ElapsedSeconds: elapsedSeconds})
}

// EpicFailed is a no-op here: by the time the
// registry calls it, this sink has already been closed and emptied (see
// EventSink's doc comment on EpicFailed).
func (s *RecordingEventSink) EpicFailed(epicName string, err error) {}

func (s *RecordingEventSink) NotificationFailed(channel, reason string) {}
