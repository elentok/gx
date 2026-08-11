package ralphloop

import (
	"context"
	"time"

	"github.com/elentok/gx/tickets"
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
}

// newChatEventSink wires inner to transport via style. Every chat-member
// event's rendered text is sent through transport and logged (success or
// failure) to scratchDir/epicName's run-log.jsonl — see send.
func newChatEventSink(inner EventSink, style mrkdwnStyle, transport chatTransport, scratchDir, epicName string) *chatEventSink {
	return &chatEventSink{
		EventSink:  inner,
		style:      style,
		transport:  transport,
		scratchDir: scratchDir,
		epicName:   epicName,
	}
}

func (s *chatEventSink) EpicStarted(epicName string, done, total int) {
	s.EventSink.EpicStarted(epicName, done, total)
	s.send(s.style.epicStartedText(epicName, loadEpicCounts(s.scratchDir, epicName)), notifyKindEpicStarted)
}

func (s *chatEventSink) IterationStarted(ticket tickets.Ticket, label, cwd, sessionID string) {
	s.EventSink.IterationStarted(ticket, label, cwd, sessionID)
	s.send(s.style.iterationStartedText(ticket, s.epicName), notifyKindIterationStarted)
}

func (s *chatEventSink) IterationPaused(identifier, label string, kind PauseKind, reason string) {
	s.EventSink.IterationPaused(identifier, label, kind, reason)
	if kind == PauseNeedsRepair {
		// A needs-repair pause always accompanies a TicketNeedsHuman park —
		// that's the one chat-visible message for this outcome, so this
		// pause itself stays TUI-only (see "park cardinality").
		return
	}
	s.send(s.style.iterationPausedText(label, reason, s.epicName, identifier), notifyKindIterationPaused)
}

func (s *chatEventSink) IterationResumed(identifier, label string, kind PauseKind) {
	s.EventSink.IterationResumed(identifier, label, kind)
	if kind == PauseNeedsRepair {
		return
	}
	s.send(s.style.iterationResumedText(label, s.epicName, identifier), notifyKindIterationResumed)
}

func (s *chatEventSink) IterationFinished(ticket tickets.Ticket, epicName string, stats IterationStats) {
	s.EventSink.IterationFinished(ticket, epicName, stats)
	s.send(s.style.iterationFinishedText(ticket, epicName, stats), notifyKindIterationFinished)
}

func (s *chatEventSink) TicketNeedsHuman(identifier, epicName, status, reason string) {
	s.EventSink.TicketNeedsHuman(identifier, epicName, status, reason)
	counts := loadEpicCounts(s.scratchDir, epicName)
	s.send(s.style.ticketNeedsHumanText(identifier, epicName, status, reason, counts), notifyKindTicketNeedsHuman)
}

func (s *chatEventSink) EpicParked(epicName string, stalled []StalledTicket) {
	s.EventSink.EpicParked(epicName, stalled)
	identifiers := make([]string, len(stalled))
	for i, t := range stalled {
		identifiers[i] = t.Identifier
	}
	s.send(s.style.epicParkedText(epicName, identifiers), notifyKindEpicParked)
}

func (s *chatEventSink) EpicComplete(epicName string, completed int, elapsedSeconds int) {
	s.EventSink.EpicComplete(epicName, completed, elapsedSeconds)
	counts := loadEpicCounts(s.scratchDir, epicName)
	s.send(s.style.epicCompleteText(epicName, counts, completed, elapsedSeconds), notifyKindEpicComplete)
}

// send hands text off to sendNotification (see eventlog.go), which runs
// transport.sendSync in its own goroutine bounded by transport.timeout() so
// a slow or unreachable endpoint never blocks the caller, retrying once and
// logging the final outcome to run-log.jsonl tagged with notifyKind (the
// live event that triggered it).
func (s *chatEventSink) send(text, notifyKind string) {
	sendNotification(s.scratchDir, s.epicName, s.transport.name(), notifyKind, s.transport.timeout(), func(ctx context.Context) error {
		return s.transport.sendSync(ctx, text)
	})
}
