package ralphloop

import "github.com/elentok/gx/tickets"

// LiveEventKind identifies which EventSink method produced a LiveEvent, so a
// channel consumer can switch on it without depending on bubbletea (that
// dependency lives in the TUI's own tea.Msg wrapper instead).
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
	LiveEventEpicParked
	LiveEventEpicComplete
	LiveEventDrainComplete
	LiveEventCherryPickStarted
	LiveEventConflictResolutionStarted
	LiveEventSmartZoneCompactStarted
	LiveEventSmartZoneFinishingUp
	LiveEventSmartZoneRecovered
	LiveEventContextOccupancy
	LiveEventNotificationFailed
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
	Status     string
	PauseKind  PauseKind
	Line       string
	Ticket     tickets.Ticket
	Done       int
	Total      int
	Completed  int
	Branch     string
	LandedSHA  string
	// Cwd/SessionID (IterationStarted/TicketReattached only) let a consumer
	// resolve the session's transcript itself (transcript.Path) to compute
	// elapsed time from its first line's timestamp.
	Cwd       string
	SessionID string
	// Tokens (ContextOccupancy only) is the session's current context-window
	// token occupancy.
	Tokens int
	// Stats (IterationFinished only) carries the landed iteration's metrics
	// and the epic's live progress counts.
	Stats IterationStats
	// ElapsedSeconds (EpicComplete only) is the run's total wall-clock
	// duration.
	ElapsedSeconds int
	// Stalled (EpicParked only) names the human-clearable tickets the parked
	// run is waiting on.
	Stalled []StalledTicket
	// Channel (NotificationFailed only) is which chat transport the failed
	// send used ("telegram"/"slack"); Reason carries the sanitized error.
	Channel string
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

// Close marks producer completion so the owner of Events can finish draining.
func (s *ChannelEventSink) Close() {
	close(s.events)
}

func (s *ChannelEventSink) emit(ev LiveEvent) {
	s.events <- ev
}

func (s *ChannelEventSink) EpicStarted(epicName string, done, total int) {
	s.emit(LiveEvent{Kind: LiveEventEpicStarted, EpicName: epicName, Done: done, Total: total})
}

func (s *ChannelEventSink) TicketReverted(identifier string) {
	s.emit(LiveEvent{Kind: LiveEventTicketReverted, Identifier: identifier})
}

func (s *ChannelEventSink) TicketReattached(identifier, label, cwd, sessionID string) {
	s.emit(LiveEvent{Kind: LiveEventTicketReattached, Identifier: identifier, Label: label, Cwd: cwd, SessionID: sessionID})
}

func (s *ChannelEventSink) TicketNeedsHuman(identifier, epicName, status, reason string) {
	s.emit(LiveEvent{Kind: LiveEventTicketNeedsHuman, Identifier: identifier, EpicName: epicName, Status: status, Reason: reason})
}

func (s *ChannelEventSink) TicketClaimed(ticket tickets.Ticket) {
	s.emit(LiveEvent{Kind: LiveEventTicketClaimed, Identifier: ticket.Identifier, Ticket: ticket})
}

func (s *ChannelEventSink) IterationStarted(ticket tickets.Ticket, label, cwd, sessionID string) {
	s.emit(LiveEvent{Kind: LiveEventIterationStarted, Identifier: ticket.Identifier, Ticket: ticket, Label: label, Cwd: cwd, SessionID: sessionID})
}

func (s *ChannelEventSink) IterationPaused(identifier, label string, kind PauseKind, reason string) {
	s.emit(LiveEvent{Kind: LiveEventIterationPaused, Identifier: identifier, Label: label, PauseKind: kind, Reason: reason})
}

func (s *ChannelEventSink) IterationResumed(identifier, label string, kind PauseKind) {
	s.emit(LiveEvent{Kind: LiveEventIterationResumed, Identifier: identifier, Label: label, PauseKind: kind})
}

func (s *ChannelEventSink) IterationFinished(ticket tickets.Ticket, epicName string, stats IterationStats) {
	s.emit(LiveEvent{Kind: LiveEventIterationFinished, Identifier: ticket.Identifier, Ticket: ticket, EpicName: epicName, Stats: stats})
}

func (s *ChannelEventSink) TranscriptLine(label, line string) {
	s.emit(LiveEvent{Kind: LiveEventTranscriptLine, Label: label, Line: line})
}

func (s *ChannelEventSink) TicketCleanupFinished(identifier string) {
	s.emit(LiveEvent{Kind: LiveEventTicketCleanupFinished, Identifier: identifier})
}

func (s *ChannelEventSink) TicketRecovering(identifier string) {
	s.emit(LiveEvent{Kind: LiveEventTicketRecovering, Identifier: identifier})
}

func (s *ChannelEventSink) TicketRecovered(identifier, epicName, branch, landedSHA string) {
	s.emit(LiveEvent{Kind: LiveEventTicketRecovered, Identifier: identifier, EpicName: epicName, Branch: branch, LandedSHA: landedSHA})
}

func (s *ChannelEventSink) TicketUnrecoverable(identifier, epicName string) {
	s.emit(LiveEvent{Kind: LiveEventTicketUnrecoverable, Identifier: identifier, EpicName: epicName})
}

func (s *ChannelEventSink) EpicParked(epicName string, stalled []StalledTicket) {
	s.emit(LiveEvent{Kind: LiveEventEpicParked, EpicName: epicName, Stalled: stalled})
}

func (s *ChannelEventSink) EpicComplete(epicName string, completed int, elapsedSeconds int) {
	s.emit(LiveEvent{Kind: LiveEventEpicComplete, EpicName: epicName, Completed: completed, ElapsedSeconds: elapsedSeconds})
}

func (s *ChannelEventSink) DrainComplete(epicName string, completed int, elapsedSeconds int) {
	s.emit(LiveEvent{Kind: LiveEventDrainComplete, EpicName: epicName, Completed: completed, ElapsedSeconds: elapsedSeconds})
}

// EpicFailed is a no-op here: the registry records a run's failure after
// this sink has already been closed and drained (see loop_registry.go's
// finish), so there is no live channel left to emit onto by the time this
// would ever be called.
func (s *ChannelEventSink) EpicFailed(epicName string, err error) {}

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

func (s *ChannelEventSink) ContextOccupancy(identifier string, tokens int) {
	s.emit(LiveEvent{Kind: LiveEventContextOccupancy, Identifier: identifier, Tokens: tokens})
}

func (s *ChannelEventSink) NotificationFailed(channel, reason string) {
	s.emit(LiveEvent{Kind: LiveEventNotificationFailed, Channel: channel, Reason: reason})
}
