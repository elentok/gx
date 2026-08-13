package ralphloop

import (
	"context"
	"path/filepath"
	"time"

	"github.com/elentok/gx/logger"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// chatTransport abstracts the wire format and destination a chatEventSink
// posts its rendered text to — Slack's {"text": ...} webhook body, or
// Telegram's Bot API sendMessage call. name tags run-log
// notification-sent/notification-failed lines (see eventlog.go); timeout
// bounds a single send attempt.
type chatTransport interface {
	name() string
	timeout() time.Duration
	sendSync(ctx context.Context, text string) error
}

// chatEventSink decorates another EventSink with one chat notification per
// chat-member event, replacing the slackEventSink/telegramEventSink pair
// that used to implement this independently (their divergent method lists
// are why the iteration-started event silently never reached chat). style
// picks the markup dialect (slackStyle/telegramStyle); transport picks the
// destination — one implementation, two configurations.
//
// Chat membership (see eventSinkVerdicts in eventsink_contract_test.go for
// the full yes/no map over every EventSink method): EpicStarted,
// IterationStarted, IterationPaused/IterationResumed (only for a
// non-park pause kind — PauseNeedsRepair is the same outcome
// TicketNeedsHuman already reports, and a park must produce exactly one
// chat message), IterationFinished, TicketNeedsHuman, EpicParked, and
// EpicComplete. Every other event is a pure pass-through to the embedded
// EventSink.
type chatEventSink struct {
	EventSink
	style      mrkdwnStyle
	transport  chatTransport
	scratchDir string
	epicName   string
	// gateStatePath overrides NotificationGate's real per-user state-file
	// path (see notificationStateFilePath) with a test-only fixed path, so
	// tests never touch the real ~/.config/gx/notifications-state.json.
	// Empty (the production default from newChatEventSink) means "use the
	// real path".
	gateStatePath string
}

// newChatEventSink wires inner to transport via style. Every chat-member
// event's rendered text passes through the budget/mute gate (see send) and,
// if allowed, is sent through transport and logged (success or failure) to
// scratchDir/epicName's run-log.jsonl.
func newChatEventSink(inner EventSink, style mrkdwnStyle, transport chatTransport, scratchDir, epicName string) *chatEventSink {
	return &chatEventSink{
		EventSink:  inner,
		style:      style,
		transport:  transport,
		scratchDir: scratchDir,
		epicName:   epicName,
	}
}

// epicSource is the ticket-less NotificationGate source for an epic-level
// event (no single ticket to attribute or mute it to) — mirrors
// EpicFailureReporter's "epic:<name>" sentinel (see isTicketlessSource).
func epicSource(epicName string) string {
	return "epic:" + epicName
}

// resolveTicketPath finds identifier's ticket file under
// scratchDir/epicName/issues, matching the "<id>-<slug>.md" naming
// convention (tickets.Load uses the same pattern). Returns "" if no file
// exists yet — gate() then hits a resolvable-but-not-found error, and send
// fails open (see gate's doc comment) rather than block the notification on
// a filesystem lookup.
func (s *chatEventSink) resolveTicketPath(identifier string) string {
	matches, err := filepath.Glob(filepath.Join(s.scratchDir, s.epicName, "issues", identifier+"-*.md"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	return matches[0]
}

// parkTicket is the ParkFunc the gate calls when it trips a per-source mute
// on a ticket-backed source: source is already the ticket's file path (the
// gate never calls this for a ticket-less source), so parking it is just
// MarkNeedsRepairWithReason with a reason identifying the trip as a storm
// mute.
func (s *chatEventSink) parkTicket(source, reason string) error {
	return MarkNeedsRepairWithReason(source, reason, schema.NeedsRepairState{})
}

// gate runs source through NotificationGate (or, under test, an injected
// fixed state-file path — see gateStatePath), recording this call in the
// persisted event/send series and applying the budget/per-source-mute/
// global-mute rules.
func (s *chatEventSink) gate(eventType, source string) (GateResult, error) {
	if s.gateStatePath != "" {
		return notificationGateAt(s.gateStatePath, s.transport.name(), eventType, source, time.Now(), true, s.parkTicket)
	}
	return NotificationGate(s.transport.name(), eventType, source, time.Now(), true, s.parkTicket)
}

func (s *chatEventSink) EpicStarted(epicName string, done, total int) {
	s.EventSink.EpicStarted(epicName, done, total)
	s.send(s.style.epicStartedText(epicName, loadEpicCounts(s.scratchDir, epicName)), notifyKindEpicStarted, epicSource(epicName), "")
}

func (s *chatEventSink) IterationStarted(ticket tickets.Ticket, label, cwd, sessionID string) {
	s.EventSink.IterationStarted(ticket, label, cwd, sessionID)
	s.send(s.style.iterationStartedText(ticket, s.epicName), notifyKindIterationStarted, ticket.Path, ticket.Identifier)
}

func (s *chatEventSink) IterationPaused(identifier, label string, kind PauseKind, reason string) {
	s.EventSink.IterationPaused(identifier, label, kind, reason)
	if kind == PauseNeedsRepair {
		// A needs-repair pause always accompanies a TicketNeedsHuman park —
		// that's the one chat-visible message for this outcome, so this
		// pause itself stays TUI-only (see "park cardinality").
		return
	}
	s.send(s.style.iterationPausedText(label, reason, s.epicName, identifier), notifyKindIterationPaused, s.resolveTicketPath(identifier), identifier)
}

func (s *chatEventSink) IterationResumed(identifier, label string, kind PauseKind) {
	s.EventSink.IterationResumed(identifier, label, kind)
	if kind == PauseNeedsRepair {
		return
	}
	s.send(s.style.iterationResumedText(label, s.epicName, identifier), notifyKindIterationResumed, s.resolveTicketPath(identifier), identifier)
}

func (s *chatEventSink) IterationFinished(ticket tickets.Ticket, epicName string, stats IterationStats) {
	s.EventSink.IterationFinished(ticket, epicName, stats)
	s.send(s.style.iterationFinishedText(ticket, epicName, stats), notifyKindIterationFinished, ticket.Path, ticket.Identifier)
}

func (s *chatEventSink) TicketNeedsHuman(identifier, epicName, status, reason string) {
	s.EventSink.TicketNeedsHuman(identifier, epicName, status, reason)
	counts := loadEpicCounts(s.scratchDir, epicName)
	s.send(s.style.ticketNeedsHumanText(identifier, epicName, status, reason, counts), notifyKindTicketNeedsHuman, s.resolveTicketPath(identifier), identifier)
}

func (s *chatEventSink) EpicParked(epicName string, stalled []StalledTicket) {
	s.EventSink.EpicParked(epicName, stalled)
	identifiers := make([]string, len(stalled))
	for i, t := range stalled {
		identifiers[i] = t.Identifier
	}
	s.send(s.style.epicParkedText(epicName, identifiers), notifyKindEpicParked, epicSource(epicName), "")
}

func (s *chatEventSink) EpicComplete(epicName string, completed int, elapsedSeconds int) {
	s.EventSink.EpicComplete(epicName, completed, elapsedSeconds)
	counts := loadEpicCounts(s.scratchDir, epicName)
	s.send(s.style.epicCompleteText(epicName, counts, completed, elapsedSeconds), notifyKindEpicComplete, epicSource(epicName), "")
}

// send runs (eventType, source) through the budget/mute gate before
// sending: allowed sends text as before; a trip suppresses text and, if
// this call is the edge-triggered one, sends the "muting this"/"globally
// muted" notice instead — through the same transport, so the operator is
// told the storm was throttled rather than just going silent. A gate error
// (e.g. source's ticket file not found/parseable) fails open: the gate
// exists to bound a runaway storm, not to gate delivery on its own
// bookkeeping succeeding, so text still sends as if the gate weren't there.
func (s *chatEventSink) send(text, notifyKind, source, ticketIdentifier string) {
	result, err := s.gate(notifyKind, source)
	if err != nil {
		logger.Debug("%s: notification gate: %v\n", s.transport.name(), err)
		s.sendRaw(text, notifyKind)
		return
	}

	switch result.Decision {
	case Allowed:
		s.sendRaw(text, notifyKind)
	case PerSourceMuted:
		if result.EdgeTriggered {
			s.sendRaw(s.style.mutedText(s.epicName, ticketIdentifier), notifyKindMuted)
		}
	case GloballyMuted:
		if result.EdgeTriggered {
			s.sendRaw(s.style.globallyMutedText(s.transport.name()), notifyKindGloballyMuted)
		}
	}
}

// sendRaw hands text off to sendNotification (see eventlog.go), which runs
// transport.sendSync in its own goroutine bounded by transport.timeout() so
// a slow or unreachable endpoint never blocks the caller, retrying once and
// logging the final outcome to run-log.jsonl tagged with notifyKind (the
// live event that triggered it).
func (s *chatEventSink) sendRaw(text, notifyKind string) {
	sendNotification(s.scratchDir, s.epicName, s.transport.name(), notifyKind, s.transport.timeout(), func(ctx context.Context) error {
		return s.transport.sendSync(ctx, text)
	})
}
