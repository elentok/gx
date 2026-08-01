package ralphloop

import "github.com/elentok/gx/tickets"

// LiveEventKind identifies which EventSink method produced a LiveEvent, so a
// channel consumer can switch on it without depending on bubbletea (that
// dependency lives in the TUI's own tea.Msg wrapper instead).
type LiveEventKind int

const (
	LiveEventNoTicketsFound LiveEventKind = iota
	LiveEventAlreadyComplete
	LiveEventTicketReverted
	LiveEventTicketReattached
	LiveEventTicketStillNeedsAttention
	LiveEventTicketClaimed
	LiveEventIterationStarted
	LiveEventIterationPaused
	LiveEventIterationResumed
	LiveEventIterationFinished
	LiveEventTranscriptLine
	LiveEventTicketCleanupFinished
	LiveEventTicketRecovered
	LiveEventTicketUnrecoverable
	LiveEventEpicComplete
	LiveEventCherryPickStarted
	LiveEventConflictResolutionStarted
	LiveEventSmartZoneCompactStarted
	LiveEventSmartZoneFinishingUp
	LiveEventSmartZoneRecovered
)

// LiveEvent captures one EventSink call for asynchronous delivery over a
// channel — a single struct with the union of every method's arguments,
// mirroring eventlog.go's Event, since most fields are only meaningful for a
// handful of Kinds. Identifier is the ticket's Identifier (not Number) for
// every kind that names a ticket, matching EventSink's own convention.
type LiveEvent struct {
	Kind LiveEventKind

	EpicName   string
	Identifier string
	Label      string
	Reason     string
	PauseKind  PauseKind
	Line       string
	Ticket     tickets.Ticket
	Done       int
	Total      int
	Completed  int
	Branch     string
	LandedSHA  string
}

// ChannelEventSink implements EventSink by forwarding every call as a
// LiveEvent onto a channel, so a TUI can turn the orchestrator's live event
// stream into tea.Msgs (see ticket 04a) without EventSink itself knowing
// about bubbletea. Every EventSink method must be safe to call concurrently
// (multiple running iterations report through the same sink), which a
// channel send already is; the buffer just needs to be generous enough that
// a burst of startup-reconciliation events doesn't block a launching
// iteration on a TUI that hasn't started draining yet.
type ChannelEventSink struct {
	events chan LiveEvent
}

// NewChannelEventSink returns a ChannelEventSink ready to use as an
// EventSink; drain it with Events().
func NewChannelEventSink() *ChannelEventSink {
	return &ChannelEventSink{events: make(chan LiveEvent, 256)}
}

// Events returns the channel every call to s is forwarded onto, in call
// order.
func (s *ChannelEventSink) Events() <-chan LiveEvent {
	return s.events
}

func (s *ChannelEventSink) emit(ev LiveEvent) {
	s.events <- ev
}

func (s *ChannelEventSink) NoTicketsFound(epicName string) {
	s.emit(LiveEvent{Kind: LiveEventNoTicketsFound, EpicName: epicName})
}

func (s *ChannelEventSink) AlreadyComplete(epicName string, done, total int) {
	s.emit(LiveEvent{Kind: LiveEventAlreadyComplete, EpicName: epicName, Done: done, Total: total})
}

func (s *ChannelEventSink) TicketReverted(identifier string) {
	s.emit(LiveEvent{Kind: LiveEventTicketReverted, Identifier: identifier})
}

func (s *ChannelEventSink) TicketReattached(identifier string, label string) {
	s.emit(LiveEvent{Kind: LiveEventTicketReattached, Identifier: identifier, Label: label})
}

func (s *ChannelEventSink) TicketStillNeedsAttention(identifier string) {
	s.emit(LiveEvent{Kind: LiveEventTicketStillNeedsAttention, Identifier: identifier})
}

func (s *ChannelEventSink) TicketClaimed(ticket tickets.Ticket) {
	s.emit(LiveEvent{Kind: LiveEventTicketClaimed, Identifier: ticket.Identifier, Ticket: ticket})
}

func (s *ChannelEventSink) IterationStarted(identifier string, label string) {
	s.emit(LiveEvent{Kind: LiveEventIterationStarted, Identifier: identifier, Label: label})
}

func (s *ChannelEventSink) IterationPaused(label string, kind PauseKind, reason string) {
	s.emit(LiveEvent{Kind: LiveEventIterationPaused, Label: label, PauseKind: kind, Reason: reason})
}

func (s *ChannelEventSink) IterationResumed(label string, kind PauseKind) {
	s.emit(LiveEvent{Kind: LiveEventIterationResumed, Label: label, PauseKind: kind})
}

func (s *ChannelEventSink) IterationFinished(ticket tickets.Ticket, epicName string) {
	s.emit(LiveEvent{Kind: LiveEventIterationFinished, Identifier: ticket.Identifier, Ticket: ticket, EpicName: epicName})
}

func (s *ChannelEventSink) TranscriptLine(label, line string) {
	s.emit(LiveEvent{Kind: LiveEventTranscriptLine, Label: label, Line: line})
}

func (s *ChannelEventSink) TicketCleanupFinished(identifier string) {
	s.emit(LiveEvent{Kind: LiveEventTicketCleanupFinished, Identifier: identifier})
}

func (s *ChannelEventSink) TicketRecovered(identifier, epicName, branch, landedSHA string) {
	s.emit(LiveEvent{Kind: LiveEventTicketRecovered, Identifier: identifier, EpicName: epicName, Branch: branch, LandedSHA: landedSHA})
}

func (s *ChannelEventSink) TicketUnrecoverable(identifier, epicName string) {
	s.emit(LiveEvent{Kind: LiveEventTicketUnrecoverable, Identifier: identifier, EpicName: epicName})
}

func (s *ChannelEventSink) EpicComplete(epicName string, completed int) {
	s.emit(LiveEvent{Kind: LiveEventEpicComplete, EpicName: epicName, Completed: completed})
}

func (s *ChannelEventSink) CherryPickStarted(identifier string) {
	s.emit(LiveEvent{Kind: LiveEventCherryPickStarted, Identifier: identifier})
}

func (s *ChannelEventSink) ConflictResolutionStarted(identifier string) {
	s.emit(LiveEvent{Kind: LiveEventConflictResolutionStarted, Identifier: identifier})
}

func (s *ChannelEventSink) SmartZoneCompactStarted(identifier string) {
	s.emit(LiveEvent{Kind: LiveEventSmartZoneCompactStarted, Identifier: identifier})
}

func (s *ChannelEventSink) SmartZoneFinishingUp(identifier string) {
	s.emit(LiveEvent{Kind: LiveEventSmartZoneFinishingUp, Identifier: identifier})
}

func (s *ChannelEventSink) SmartZoneRecovered(identifier string) {
	s.emit(LiveEvent{Kind: LiveEventSmartZoneRecovered, Identifier: identifier})
}
