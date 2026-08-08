package ralphloop

import (
	"sync"

	"github.com/elentok/gx/tickets"
)

// recordingEventSink implements EventSink by recording every call as a
// LiveEvent (see live_events.go) in call order, synchronously — unlike
// ChannelEventSink's channel, a test can call Events() right after Run()
// returns with no drain goroutine to coordinate. It exists so tests can
// assert on the same structured data a real consumer (the TUI, via
// ChannelEventSink) would see, instead of parsing rendered CLI text.
type recordingEventSink struct {
	mu     sync.Mutex
	events []LiveEvent
}

func newRecordingEventSink() *recordingEventSink {
	return &recordingEventSink{}
}

// hasEvent reports whether sink recorded an event of kind matching match.
func hasEvent(sink *recordingEventSink, kind LiveEventKind, match func(LiveEvent) bool) bool {
	for _, ev := range sink.Events() {
		if ev.Kind == kind && match(ev) {
			return true
		}
	}
	return false
}

// Events returns every event recorded so far, in call order.
func (s *recordingEventSink) Events() []LiveEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]LiveEvent(nil), s.events...)
}

func (s *recordingEventSink) record(ev LiveEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
}

func (s *recordingEventSink) EpicStarted(epicName string, done, total int) {
	s.record(LiveEvent{Kind: LiveEventEpicStarted, EpicName: epicName, Done: done, Total: total})
}

func (s *recordingEventSink) TicketReverted(identifier string) {
	s.record(LiveEvent{Kind: LiveEventTicketReverted, Identifier: identifier})
}

func (s *recordingEventSink) TicketReattached(identifier, label, cwd, sessionID string) {
	s.record(LiveEvent{Kind: LiveEventTicketReattached, Identifier: identifier, Label: label, Cwd: cwd, SessionID: sessionID})
}

func (s *recordingEventSink) TicketNeedsHuman(identifier, epicName, status, reason string) {
	s.record(LiveEvent{Kind: LiveEventTicketNeedsHuman, Identifier: identifier, EpicName: epicName, Status: status, Reason: reason})
}

func (s *recordingEventSink) TicketClaimed(ticket tickets.Ticket) {
	s.record(LiveEvent{Kind: LiveEventTicketClaimed, Identifier: ticket.Identifier, Ticket: ticket})
}

func (s *recordingEventSink) IterationStarted(ticket tickets.Ticket, label, cwd, sessionID string) {
	s.record(LiveEvent{Kind: LiveEventIterationStarted, Identifier: ticket.Identifier, Ticket: ticket, Label: label, Cwd: cwd, SessionID: sessionID})
}

func (s *recordingEventSink) IterationPaused(identifier, label string, kind PauseKind, reason string) {
	s.record(LiveEvent{Kind: LiveEventIterationPaused, Identifier: identifier, Label: label, PauseKind: kind, Reason: reason})
}

func (s *recordingEventSink) IterationResumed(identifier, label string, kind PauseKind) {
	s.record(LiveEvent{Kind: LiveEventIterationResumed, Identifier: identifier, Label: label, PauseKind: kind})
}

func (s *recordingEventSink) IterationFinished(ticket tickets.Ticket, epicName string, stats IterationStats) {
	s.record(LiveEvent{Kind: LiveEventIterationFinished, Identifier: ticket.Identifier, Ticket: ticket, EpicName: epicName, Stats: stats})
}

func (s *recordingEventSink) TranscriptLine(label, line string) {
	s.record(LiveEvent{Kind: LiveEventTranscriptLine, Label: label, Line: line})
}

func (s *recordingEventSink) ContextOccupancy(identifier string, tokens int) {
	s.record(LiveEvent{Kind: LiveEventContextOccupancy, Identifier: identifier, Tokens: tokens})
}

func (s *recordingEventSink) CherryPickStarted(identifier string) {
	s.record(LiveEvent{Kind: LiveEventCherryPickStarted, Identifier: identifier})
}

func (s *recordingEventSink) ConflictResolutionStarted(identifier string) {
	s.record(LiveEvent{Kind: LiveEventConflictResolutionStarted, Identifier: identifier})
}

func (s *recordingEventSink) SmartZoneCompactStarted(identifier string) {
	s.record(LiveEvent{Kind: LiveEventSmartZoneCompactStarted, Identifier: identifier})
}

func (s *recordingEventSink) SmartZoneFinishingUp(identifier string) {
	s.record(LiveEvent{Kind: LiveEventSmartZoneFinishingUp, Identifier: identifier})
}

func (s *recordingEventSink) SmartZoneRecovered(identifier string) {
	s.record(LiveEvent{Kind: LiveEventSmartZoneRecovered, Identifier: identifier})
}

func (s *recordingEventSink) TicketCleanupFinished(identifier string) {
	s.record(LiveEvent{Kind: LiveEventTicketCleanupFinished, Identifier: identifier})
}

func (s *recordingEventSink) TicketRecovering(identifier string) {
	s.record(LiveEvent{Kind: LiveEventTicketRecovering, Identifier: identifier})
}

func (s *recordingEventSink) TicketRecovered(identifier, epicName, branch, landedSHA string) {
	s.record(LiveEvent{Kind: LiveEventTicketRecovered, Identifier: identifier, EpicName: epicName, Branch: branch, LandedSHA: landedSHA})
}

func (s *recordingEventSink) TicketUnrecoverable(identifier, epicName string) {
	s.record(LiveEvent{Kind: LiveEventTicketUnrecoverable, Identifier: identifier, EpicName: epicName})
}

func (s *recordingEventSink) EpicParked(epicName string, stalled []StalledTicket) {
	s.record(LiveEvent{Kind: LiveEventEpicParked, EpicName: epicName, Stalled: stalled})
}

func (s *recordingEventSink) EpicComplete(epicName string, completed int, elapsedSeconds int) {
	s.record(LiveEvent{Kind: LiveEventEpicComplete, EpicName: epicName, Completed: completed, ElapsedSeconds: elapsedSeconds})
}

func (s *recordingEventSink) DrainComplete(epicName string, completed int, elapsedSeconds int) {
	s.record(LiveEvent{Kind: LiveEventDrainComplete, EpicName: epicName, Completed: completed, ElapsedSeconds: elapsedSeconds})
}

// EpicFailed is a no-op here, matching ChannelEventSink: by the time the
// registry calls it, this sink has already been closed and drained (see
// EventSink's doc comment on EpicFailed).
func (s *recordingEventSink) EpicFailed(epicName string, err error) {}

func (s *recordingEventSink) NotificationFailed(channel, reason string) {}
