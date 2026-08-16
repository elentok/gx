package ralphloop

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/chatmarkup"
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
	sendSync(ctx context.Context, text chatmarkup.Text) (sendResult, error)
}

// sendResult carries a chatTransport.sendSync response's status beyond a bare
// error — the HTTP status code, plus (Telegram only, see telegramTransport's
// sendSync) the response body's description field, which names why a 400
// happened (e.g. a MarkdownV2-parse rejection vs. an invalid chat_id).
// Slack's webhook response body doesn't carry a comparable field, so
// slackTransport.sendSync leaves Description empty. Degraded marks a send
// that succeeded only via telegramTransport.sendSync's plain-text fallback —
// always false for slackTransport, which has no such fallback. On a
// degraded success, Description carries the original MarkdownV2-rejection
// description (rather than being left empty, as it is on any other success)
// so callers can report why the downgrade happened. RetryAfter carries a 429
// response's retry_after (seconds), if Telegram's body included one — nil
// when absent or when the transport doesn't support it (slackTransport never
// sets it), which sendWithRetry treats as "not honorable" rather than "retry
// immediately" (see eventlog.go's retryDelay).
type sendResult struct {
	StatusCode  int
	Description string
	Degraded    bool
	RetryAfter  *int
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
// chat message), IterationFinished, TicketNeedsHuman, EpicParked,
// EpicComplete, and DrainComplete. Every other event is a pure pass-through
// to the embedded EventSink.
// ChatEventSink is the interface NewTelegramEventSink/NewSlackEventSink's
// return value actually satisfies (both are declared to return the plainer
// EventSink so callers not wired to chat can ignore it) — a named interface
// a caller like ui/tickets/implement.go can assert against to close the
// sink's flush loop on shutdown, instead of an anonymous method-set cast
// that would silently no-op if the concrete type ever stopped implementing
// it.
type ChatEventSink interface {
	EventSink
	// Close stops the periodic flush loop and flushes any queued messages
	// synchronously (bounded by the transport's timeout) before returning.
	Close()
}

var _ ChatEventSink = (*chatEventSink)(nil)

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

	// flushMidpointHook, if non-nil, runs synchronously inside flush() right
	// after drainQueue but before the gate/sendRaw call that hands the
	// drained items off — a test-only seam for deterministically landing
	// Close() inside the exact window stopFlushLoop's wait-for-actual-exit
	// exists to close: a flush that has already taken items off the queue
	// but not yet reached sendRaw, and so not yet incremented inFlight. Nil
	// in production and every test but the one exercising that race.
	flushMidpointHook func()

	// mu guards queue and closed against concurrent enqueue (multiple running
	// iterations report through the same sink) and against a flush or
	// requeue racing an enqueue.
	mu     sync.Mutex
	queue  []batchedMessage
	closed bool

	// flushTicker/flushDone/flushLoopExited back the periodic flush loop (see
	// startFlushLoop/stopFlushLoop); all three are nil until startFlushLoop
	// runs, which production sinks do at construction (see
	// newSlackEventSink/newTelegramEventSink) and tests do explicitly, if at
	// all, to exercise the periodic-tick behavior — most tests drive
	// flush()/Close() directly instead. flushLoopExited is closed by the loop
	// goroutine itself right before it returns, so stopFlushLoop can wait for
	// the goroutine to actually be gone rather than just signaling it to stop
	// — see stopFlushLoop's race note.
	flushTicker     *time.Ticker
	flushDone       chan struct{}
	flushLoopExited chan struct{}
	closeOnce       sync.Once

	// sendCtx/sendCancel bound every sendRaw-launched goroutine; Close cancels
	// sendCtx so an in-flight retry/backoff fails fast instead of being
	// waited out (see sendRaw, Close). inFlight tracks those goroutines —
	// incremented synchronously in sendRaw before the goroutine launches, so
	// Close can wait for them to actually finish (quickly, once cancelled)
	// rather than just for the cancellation signal to have been sent.
	sendCtx    context.Context
	sendCancel context.CancelFunc
	inFlight   sync.WaitGroup
}

// batchedMessage is one distinct rendered text waiting in the queue for the
// next flush, with count tracking how many times an identical text was
// enqueued within the current window (see chatEventSink.enqueue) — the
// dedup ×N collapse. kind is the notifyKind that produced it, kept so a
// suppressed close-time flush can name what it dropped (see closeFlush).
type batchedMessage struct {
	text  chatmarkup.Text
	kind  string
	count int
	// requeued marks an entry that came back from a failed flush send (see
	// chatEventSink.requeue) rather than a fresh enqueue — flush uses it to
	// decide whether a retry attempt needs a new gate recordSend charge (see
	// flush's recordSend comment). Merging a requeued entry with a freshly
	// enqueued one clears the flag (see requeue): the merged entry now
	// carries new, never-charged content, so it should be charged again.
	requeued bool
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
	sendCtx, sendCancel := context.WithCancel(context.Background())
	return &chatEventSink{
		EventSink:  inner,
		style:      style,
		transport:  transport,
		scratchDir: scratchDir,
		epicName:   epicName,
		sendCtx:    sendCtx,
		sendCancel: sendCancel,
	}
}

// startFlushLoop begins flushing the queue every interval until Close stops
// it. Called once by production sinks at construction; a test that wants to
// exercise the periodic tick itself (rather than driving flush()/Close()
// directly) calls it with a short interval.
func (s *chatEventSink) startFlushLoop(interval time.Duration) {
	s.flushDone = make(chan struct{})
	s.flushLoopExited = make(chan struct{})
	ticker := time.NewTicker(interval)
	s.flushTicker = ticker
	go func() {
		defer close(s.flushLoopExited)
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

// stopFlushLoop stops the periodic flush loop, if one was started, and waits
// for its goroutine to actually exit before returning — not just for the
// done channel to close. This matters because flush() (called synchronously
// from inside that goroutine on every tick) is what increments the in-flight
// send counter (via sendRaw); if stopFlushLoop returned as soon as the done
// signal was sent, a tick that fired concurrently with Close() could still
// be inside flush(), on its way to sendRaw, at the moment Close() samples the
// in-flight count — making that message invisible and lost regardless of the
// cancel-and-collect logic below. Waiting for flushLoopExited guarantees any
// such in-progress flush() has already returned (and so already incremented
// inFlight, if it had anything to send) before Close moves on. Safe to call
// more than once (Close does) and safe to call when startFlushLoop was never
// called at all.
func (s *chatEventSink) stopFlushLoop() {
	s.closeOnce.Do(func() {
		if s.flushDone != nil {
			close(s.flushDone)
			<-s.flushLoopExited
		}
	})
}

// Close stops the periodic flush loop (if running), cancels any in-flight
// sendRaw send rather than waiting for it to finish naturally, then flushes
// the queue — now including anything that in-flight cancellation surfaced —
// synchronously through the same retrying send logic as the normal async
// path. The whole sequence (cancel-and-collect plus the final send) is
// bounded by shutdownTotalBudget so Close never blocks indefinitely; a
// failure after that budget is accepted, bounded message loss (see
// shutdownTotalBudget's doc comment).
func (s *chatEventSink) Close() {
	deadline := time.Now().Add(shutdownTotalBudget)
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.stopFlushLoop()
	s.cancelInFlight(deadline)
	s.closeFlush(deadline)
}

// shutdownTotalBudget bounds Close's whole shutdown sequence — the
// cancel-and-collect phase (cancelling any in-flight sendRaw and waiting for
// it to actually stop) plus closeFlush's own consolidated send. In practice
// cancel-and-collect resolves in under ~1s (a cancelled sendWithRetry attempt
// fails fast); the final send inherits whatever budget that phase didn't use,
// up to its own worst case of ~11.5s (a 5s transport attempt, then
// notificationRetryBackoff, then a second 5s attempt) — the ceiling doesn't
// stack on top of the ~12-13s total. A shutdown-time failure after this
// budget is exhausted is accepted, bounded message loss: full cross-process
// durability (surviving the gx process itself exiting) is out of scope for
// this epic (see follow-ups/08). Declared as a var, not a const, so
// TestChatEventSink_Close_TotalShutdownDeadline_ReturnsWithinBound can
// temporarily shrink it rather than actually waiting out ~12.5s.
var shutdownTotalBudget = 12500 * time.Millisecond

// cancelInFlight cancels sendCtx (cutting short any sendRaw attempt currently
// sleeping in a retry backoff or blocked on the transport) and waits for
// every sendRaw goroutine tracked by inFlight to actually return, bounded by
// deadline so a sendSync implementation that ignores context cancellation
// can't block Close forever. A cancelled attempt's own failure path
// (sendNotificationSync's onFailed) still runs before the goroutine
// returns, so by the time this call returns, every attempt that was in
// flight when Close was called has already reported its outcome.
func (s *chatEventSink) cancelInFlight(deadline time.Time) {
	s.sendCancel()

	done := make(chan struct{})
	go func() {
		s.inFlight.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Until(deadline)):
	}
}

// enqueue adds text to the queue, collapsing it into an existing entry (and
// bumping its count) if an identical text is already queued this window —
// the dedup ×N behavior. kind is recorded so a suppressed close-time flush
// can name what it dropped.
func (s *chatEventSink) enqueue(text chatmarkup.Text, kind string) {
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
// hands it to sendRaw — the one gate call per flush that records this batch
// against the send series (see gate's recordSend), so the global budget
// tracks actual wire sends rather than the events that were coalesced into
// them. A gate error fails open (send proceeds), matching send's own
// fail-open policy. A tick with an empty queue sends nothing and never
// calls the gate.
//
// recordSend is false when every drained item is a requeue (see requeue) of
// a batch that already recorded a send on its first attempt — a retry cycle
// during a sustained outage must not keep charging the budget for the same
// logical batch, or it could trip the global-mute breaker purely from
// retrying (see the ticket this guards). Any item that isn't a requeue (a
// fresh arrival, or one merged with a requeue — see requeue) still charges
// normally.
func (s *chatEventSink) flush() {
	items := s.drainQueue()
	if len(items) == 0 {
		return
	}
	if s.flushMidpointHook != nil {
		s.flushMidpointHook()
	}
	result, err := s.gate(notifyKindBatch, epicSource(s.epicName), !allRequeued(items))
	if err != nil {
		logger.Debug("%s: notification gate: %v\n", s.transport.name(), err)
		s.sendRaw(items, renderBatch(s.style, items), notifyKindBatch)
		return
	}
	if result.Decision == GloballyMuted {
		return
	}
	s.sendRaw(items, renderBatch(s.style, items), notifyKindBatch)
}

// allRequeued reports whether every item in a drained batch came back from a
// failed prior flush (see requeue) rather than a fresh enqueue.
func allRequeued(items []batchedMessage) bool {
	for _, it := range items {
		if !it.requeued {
			return false
		}
	}
	return true
}

// closeFlush is flush's close-time variant: if the gate reports the
// transport is now globally muted, the queued messages are dropped rather
// than sent, and one notification-suppressed run-log line names the event
// kinds that were dropped — so the outcome stays recoverable from the log
// even though chat never got it. Otherwise it sends synchronously, through
// the same retrying sendNotificationSync logic as the normal async path
// (unlike the old single-attempt-only close-time send), bounded by deadline
// rather than sendRaw's fire-and-forget goroutine, since the process may
// exit right after Close returns.
func (s *chatEventSink) closeFlush(deadline time.Time) {
	items := s.drainQueue()
	if len(items) == 0 {
		return
	}
	result, err := s.gate(notifyKindBatch, epicSource(s.epicName), true)
	if err != nil {
		logger.Debug("%s: notification gate: %v\n", s.transport.name(), err)
		s.sendFinal(renderBatch(s.style, items), notifyKindBatch, deadline)
		return
	}
	if result.Decision == GloballyMuted {
		logNotificationSuppressed(s.scratchDir, s.epicName, s.transport.name(), strings.Join(distinctKinds(items), ","))
		return
	}
	s.sendFinal(renderBatch(s.style, items), notifyKindBatch, deadline)
}

// sendFinal runs sendNotificationSync synchronously, bounded by deadline —
// closeFlush's consolidated shutdown-time send.
func (s *chatEventSink) sendFinal(text chatmarkup.Text, notifyKind string, deadline time.Time) {
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	sendNotificationSync(ctx, s.scratchDir, s.epicName, s.transport.name(), notifyKind, text.String(), s.transport.timeout(), func(ctx context.Context) (sendResult, error) {
		return s.transport.sendSync(ctx, text)
	}, func(reason string) {
		s.EventSink.NotificationFailed(s.transport.name(), reason)
	})
}

// batchSeparatorRaw is the literal divider renderBatch places between
// originally-distinct messages, before style-specific escaping. Every
// individual message's text arrives already escaped (see enqueue) — this is
// the one piece of the joined output renderBatch itself introduces, so it
// must go through style.escape the same as any other literal, or a dialect
// that reserves one of its characters (Telegram's MarkdownV2 reserves "-")
// rejects the whole send.
const batchSeparatorRaw = "---"

// renderBatch joins every queued message into one send, separator lines
// between originally-distinct messages, applying each message's ×N dedup
// suffix through chatmarkup.Text.WithSuffix. style escapes the separator for
// the target dialect — item text is already safe by the time it's queued
// (see enqueue), but the separator is renderBatch's own literal, so it needs
// the same treatment; chatmarkup.Join only accepts already-safe Text values,
// so the separator physically cannot reach the wire unescaped.
func renderBatch(style mrkdwnStyle, items []batchedMessage) chatmarkup.Text {
	texts := make([]chatmarkup.Text, len(items))
	for i, it := range items {
		texts[i] = it.text
		if it.count > 1 {
			texts[i] = it.text.WithSuffix(fmt.Sprintf(" ×%d", it.count), style.chatStyle)
		}
	}
	separator := style.chatStyle.Escape("\n\n" + batchSeparatorRaw + "\n\n")
	return chatmarkup.Join(separator, texts)
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

// epicSourcePrefix is the ticket-less NotificationGate source sentinel for
// epic-level events (no single ticket to attribute or mute it to) — shared
// by epicSource, EpicFailureReporter, and isTicketlessSource so the format
// can't desync across call sites.
const epicSourcePrefix = "epic:"

// epicSource is the ticket-less NotificationGate source for an epic-level
// event (no single ticket to attribute or mute it to).
func epicSource(epicName string) string {
	return epicSourcePrefix + epicName
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
// persisted event series and applying the budget/per-source-mute/
// global-mute rules. recordSend additionally records a send-series entry —
// callers pass true only at the point a batch actually goes out on the wire
// (see flush/closeFlush), never at enqueue time (see send), so the global
// budget tracks actual sends rather than raw event volume.
func (s *chatEventSink) gate(eventType, source string, recordSend bool) (GateResult, error) {
	if s.gateStatePath != "" {
		return notificationGateAt(s.gateStatePath, s.transport.name(), eventType, source, time.Now(), recordSend, s.parkTicket)
	}
	return NotificationGate(s.transport.name(), eventType, source, time.Now(), recordSend, s.parkTicket)
}

func (s *chatEventSink) EpicStarted(epicName string, done, total int) {
	s.EventSink.EpicStarted(epicName, done, total)
	s.send(s.style.epicStartedText(epicName, loadEpicCounts(s.scratchDir, epicName)), notifyKindEpicStarted, epicSource(epicName), "")
}

func (s *chatEventSink) IterationStarted(ticket tickets.Ticket, label, cwd, sessionID string, agent AgentKind, paneID, tabID string) {
	s.EventSink.IterationStarted(ticket, label, cwd, sessionID, agent, paneID, tabID)
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
	totalCost := loadEpicTotalCost(s.scratchDir, epicName)
	s.send(s.style.epicCompleteText(epicName, counts, completed, elapsedSeconds, totalCost), notifyKindEpicComplete, epicSource(epicName), "")
}

func (s *chatEventSink) DrainComplete(epicName string, completed int, elapsedSeconds int) {
	s.EventSink.DrainComplete(epicName, completed, elapsedSeconds)
	counts := loadEpicCounts(s.scratchDir, epicName)
	totalCost := loadEpicTotalCost(s.scratchDir, epicName)
	s.send(s.style.drainCompleteText(epicName, counts, completed, elapsedSeconds, totalCost), notifyKindDrainComplete, epicSource(epicName), "")
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
func (s *chatEventSink) send(text chatmarkup.Text, notifyKind, source, ticketIdentifier string) {
	result, err := s.gate(notifyKind, source, false)
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

// sendRaw runs sendNotificationSync in its own goroutine, bounded by
// s.sendCtx so Close can cancel it rather than wait it out (see Close,
// cancelInFlight). inFlight is incremented synchronously here, before the
// goroutine launches — incrementing from inside the goroutine would race
// Close's inFlight.Wait if the goroutine hasn't been scheduled yet by the
// time Close samples the count. items is the batch text was rendered from
// (see flush) — on a genuine failure (err != nil; a Degraded-but-delivered
// result is not a failure) it's handed to requeue so the next flush gets
// another chance at it instead of losing it.
func (s *chatEventSink) sendRaw(items []batchedMessage, text chatmarkup.Text, notifyKind string) {
	s.inFlight.Go(func() {
		_, err := sendNotificationSync(s.sendCtx, s.scratchDir, s.epicName, s.transport.name(), notifyKind, text.String(), s.transport.timeout(), func(ctx context.Context) (sendResult, error) {
			return s.transport.sendSync(ctx, text)
		}, func(reason string) {
			s.EventSink.NotificationFailed(s.transport.name(), reason)
		})
		if err != nil {
			s.requeue(items)
		}
	})
}

// maxRequeueSize bounds the queue after a requeue (see requeue) so a
// sustained outage can't grow it unboundedly: past this many distinct
// entries, the oldest are evicted first (they sit at the front after
// requeue's prepend) to make room for what just failed.
const maxRequeueSize = 50

// requeue puts a failed flush's items back at the front of the queue —
// prepended, since they're older than anything queued since the batch was
// drained, and merged into any identical text already queued rather than
// appended as a duplicate (see enqueue's same dedup rule). Every requeued
// entry is marked requeued so flush knows not to re-charge the notification
// gate's send budget for a batch it already charged once (see flush); an
// entry that merges with a fresh (non-requeued) one loses that mark, since
// it now carries new content flush hasn't charged for yet.
//
// If s.closed (Close has started shutting the sink down), the requeue is
// skipped and logged via logNotificationSuppressed instead — nothing will
// ever flush this queue again, so silently requeuing here would just lose
// the batch invisibly. 03a's cancel-and-consolidate shutdown redesign makes
// this a rare path in practice, not the primary loss-prevention mechanism.
func (s *chatEventSink) requeue(items []batchedMessage) {
	if len(items) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		logNotificationSuppressed(s.scratchDir, s.epicName, s.transport.name(), strings.Join(distinctKinds(items), ","))
		return
	}
	var unmatched []batchedMessage
	for _, item := range items {
		item.requeued = true
		merged := false
		for i := range s.queue {
			if s.queue[i].text == item.text {
				s.queue[i].count += item.count
				merged = true
				break
			}
		}
		if !merged {
			unmatched = append(unmatched, item)
		}
	}
	s.queue = append(unmatched, s.queue...)
	if len(s.queue) > maxRequeueSize {
		s.queue = s.queue[len(s.queue)-maxRequeueSize:]
	}
}
