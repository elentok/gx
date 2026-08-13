package ralphloop

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/logger"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// batchFlushInterval is how often a production chatEventSink's queue flushes
// — a fixed periodic tick, deliberately not modeled on the waitForFinish
// idle-debounce pattern that caused the incident this throttling exists to
// prevent (a debounce never fires while events keep arriving; a storm is
// exactly when they do).
const batchFlushInterval = 6 * time.Second

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

	// mu guards queue against concurrent enqueue (multiple running iterations
	// report through the same sink) and against a flush racing an enqueue.
	mu    sync.Mutex
	queue []batchedMessage

	// flushTicker/flushDone back the periodic flush loop (see
	// startFlushLoop/stopFlushLoop); both are nil until startFlushLoop runs,
	// which production sinks do at construction (see newSlackEventSink/
	// newTelegramEventSink) and tests do explicitly, if at all, to exercise
	// the periodic-tick behavior — most tests drive flush()/Close() directly
	// instead.
	flushTicker *time.Ticker
	flushDone   chan struct{}
	closeOnce   sync.Once
}

// batchedMessage is one distinct rendered text waiting in the queue for the
// next flush, with count tracking how many times an identical text was
// enqueued within the current window (see chatEventSink.enqueue) — the
// dedup ×N collapse. kind is the notifyKind that produced it, kept so a
// suppressed close-time flush can name what it dropped (see closeFlush).
type batchedMessage struct {
	text  string
	kind  string
	count int
}

// newChatEventSink wires inner to transport via style. Every chat-member
// event's rendered text passes through the budget/mute gate (see send) and,
// if allowed, is queued rather than sent immediately — a periodic flush
// (see flush) renders every queued message as one send. newChatEventSink
// itself does not start the flush loop (see startFlushLoop); production
// callers (newSlackEventSink/newTelegramEventSink) do, so a bare
// newChatEventSink is inert until flush()/Close() is driven explicitly —
// the shape tests rely on.
func newChatEventSink(inner EventSink, style mrkdwnStyle, transport chatTransport, scratchDir, epicName string) *chatEventSink {
	return &chatEventSink{
		EventSink:  inner,
		style:      style,
		transport:  transport,
		scratchDir: scratchDir,
		epicName:   epicName,
	}
}

// startFlushLoop begins flushing the queue every interval until Close stops
// it. Called once by production sinks at construction; a test that wants to
// exercise the periodic tick itself (rather than driving flush()/Close()
// directly) calls it with a short interval.
func (s *chatEventSink) startFlushLoop(interval time.Duration) {
	s.flushDone = make(chan struct{})
	ticker := time.NewTicker(interval)
	s.flushTicker = ticker
	go func() {
		for {
			select {
			case <-ticker.C:
				s.flush()
			case <-s.flushDone:
				ticker.Stop()
				return
			}
		}
	}()
}

// stopFlushLoop stops the periodic flush loop, if one was started. Safe to
// call more than once (Close does) and safe to call when startFlushLoop was
// never called at all.
func (s *chatEventSink) stopFlushLoop() {
	s.closeOnce.Do(func() {
		if s.flushDone != nil {
			close(s.flushDone)
		}
	})
}

// Close stops the periodic flush loop (if running) and flushes any queued
// messages synchronously, bounded by the transport's timeout — so a run
// ending mid flush-window doesn't drop its last batch. If the transport is
// already globally muted, the flush is suppressed rather than sent (see
// closeFlush).
func (s *chatEventSink) Close() {
	s.stopFlushLoop()
	s.closeFlush()
}

// enqueue adds text to the queue, collapsing it into an existing entry (and
// bumping its count) if an identical text is already queued this window —
// the dedup ×N behavior. kind is recorded so a suppressed close-time flush
// can name what it dropped.
func (s *chatEventSink) enqueue(text, kind string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.queue {
		if s.queue[i].text == text {
			s.queue[i].count++
			return
		}
	}
	s.queue = append(s.queue, batchedMessage{text: text, kind: kind, count: 1})
}

// drainQueue empties the queue and returns what it held, so flush/closeFlush
// can render or suppress it outside the lock.
func (s *chatEventSink) drainQueue() []batchedMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.queue
	s.queue = nil
	return items
}

// flush renders every queued message as one send (see renderBatch) and
// hands it to sendRaw. A tick with an empty queue sends nothing.
func (s *chatEventSink) flush() {
	items := s.drainQueue()
	if len(items) == 0 {
		return
	}
	s.sendRaw(renderBatch(items), notifyKindBatch)
}

// closeFlush is flush's close-time variant: if the transport is already
// globally muted, the queued messages are dropped rather than sent, and one
// notification-suppressed run-log line names the event kinds that were
// dropped — so the outcome stays recoverable from the log even though chat
// never got it. Otherwise it sends synchronously (bounded by the
// transport's own timeout, no retry) rather than through sendRaw's
// fire-and-forget goroutine, since the process may exit right after Close
// returns.
func (s *chatEventSink) closeFlush() {
	items := s.drainQueue()
	if len(items) == 0 {
		return
	}
	if s.transportGloballyMuted() {
		logNotificationSuppressed(s.scratchDir, s.epicName, s.transport.name(), strings.Join(distinctKinds(items), ","))
		return
	}
	s.sendSync(renderBatch(items), notifyKindBatch)
}

// transportGloballyMuted reports whether the transport is already
// globally muted, read directly from NotificationState (or, under test, the
// injected gateStatePath) rather than through the gate — a close-time flush
// has no single (eventType, source) to gate on, and this check must not
// itself record an event/send series entry. A read error fails open (no
// suppression), matching the gate's own fail-open policy elsewhere in this
// file.
func (s *chatEventSink) transportGloballyMuted() bool {
	var state NotificationState
	var err error
	if s.gateStatePath != "" {
		state, err = loadNotificationStateAt(s.gateStatePath)
	} else {
		state, err = LoadNotificationState()
	}
	if err != nil {
		return false
	}
	return state.Transports[s.transport.name()].Muted
}

// renderBatch joins every queued message into one send, separator lines
// between originally-distinct messages, applying each message's ×N dedup
// suffix.
func renderBatch(items []batchedMessage) string {
	lines := make([]string, len(items))
	for i, it := range items {
		lines[i] = it.text
		if it.count > 1 {
			lines[i] = fmt.Sprintf("%s ×%d", it.text, it.count)
		}
	}
	return strings.Join(lines, "\n---\n")
}

// distinctKinds returns items' notifyKinds in first-seen order, deduped —
// what a suppressed close-time flush names in its run-log line.
func distinctKinds(items []batchedMessage) []string {
	seen := map[string]bool{}
	kinds := make([]string, 0, len(items))
	for _, it := range items {
		if seen[it.kind] {
			continue
		}
		seen[it.kind] = true
		kinds = append(kinds, it.kind)
	}
	return kinds
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
// queueing: allowed enqueues text as before (see flush); a trip suppresses
// text and, if this call is the edge-triggered one, enqueues the "muting
// this"/"globally muted" notice instead — through the same transport, so
// the operator is told the storm was throttled rather than just going
// silent. A gate error (e.g. source's ticket file not found/parseable)
// fails open: the gate exists to bound a runaway storm, not to gate
// delivery on its own bookkeeping succeeding, so text is still queued as if
// the gate weren't there.
func (s *chatEventSink) send(text, notifyKind, source, ticketIdentifier string) {
	result, err := s.gate(notifyKind, source)
	if err != nil {
		logger.Debug("%s: notification gate: %v\n", s.transport.name(), err)
		s.enqueue(text, notifyKind)
		return
	}

	switch result.Decision {
	case Allowed:
		s.enqueue(text, notifyKind)
	case PerSourceMuted:
		if result.EdgeTriggered {
			s.enqueue(s.style.mutedText(s.epicName, ticketIdentifier), notifyKindMuted)
		}
	case GloballyMuted:
		if result.EdgeTriggered {
			s.enqueue(s.style.globallyMutedText(s.transport.name()), notifyKindGloballyMuted)
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

// sendSync sends text through the transport in the caller's own goroutine,
// bounded by a single transport.timeout() attempt with no retry — closeFlush's
// send, since a fire-and-forget goroutine (sendRaw) could still be in
// flight, or never scheduled, by the time the process exits right after
// Close returns.
func (s *chatEventSink) sendSync(text, notifyKind string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.transport.timeout())
	defer cancel()
	if err := s.transport.sendSync(ctx, text); err != nil {
		err = sanitizeSendError(err)
		logger.Debug("%s: %v\n", s.transport.name(), err)
		logNotificationFailed(s.scratchDir, s.epicName, s.transport.name(), notifyKind, err.Error())
		return
	}
	logNotificationSent(s.scratchDir, s.epicName, s.transport.name(), notifyKind)
}
