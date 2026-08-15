package ralphloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/logger"
)

// Event type strings recorded in an epic's run-log.jsonl.
const (
	eventIterationStarted        = "iteration-started"
	eventIterationFinished       = "iteration-finished"
	eventCherryPicked            = "cherry-picked"
	eventConflictHit             = "conflict-hit"
	eventConflictResolved        = "conflict-resolved"
	eventPausedSmartZone         = "paused-smart-zone"
	eventSmartZoneRecoveryFailed = "smart-zone-recovery-failed"
	// eventSmartZoneWaitExpired marks a compact-recovery wait that expired
	// past smartZoneCompactTimeoutMs without herdr's pane-status wait ever
	// confirming completion, but where the transcript's compaction-boundary
	// signal showed compaction actually finished anyway — a slower-than-usual
	// compact, not a failure, and deliberately distinct from
	// eventSmartZoneRecoveryFailed so it isn't misread as one.
	eventSmartZoneWaitExpired = "smart-zone-wait-expired"
	// eventSmartZoneGateReleased marks the other route to a confirmed
	// compaction: the pane reported completion straight away, the
	// compaction-boundary gate refused to believe it, and the boundary landed
	// a few poll ticks later. Nothing expired there, so it deliberately does
	// not share eventSmartZoneWaitExpired's name — telling "Claude Code
	// reported idle mid-compaction" apart from "compaction genuinely took more
	// than five minutes" is exactly what run-log.jsonl is read for.
	eventSmartZoneGateReleased = "smart-zone-gate-released"
	// eventBackgroundTaskGateHeld/Released/Expired mark waitForBackgroundTasks
	// holding confirmFinished's conclusion open for one outstanding-fresh
	// backgrounded-shell-command marker: Held once when first observed
	// outstanding (never once per poll tick), Released once its
	// task-notification lands, Expired once it ages out past
	// backgroundTaskAgedOutCap and the gate stops holding on it instead.
	eventBackgroundTaskGateHeld     = "background-task-gate-held"
	eventBackgroundTaskGateReleased = "background-task-gate-released"
	eventBackgroundTaskGateExpired  = "background-task-gate-expired"
	eventPausedRateLimit            = "paused-rate-limit"
	eventResumed                    = "resumed"
	eventNeedsAnswer                = "needs-answer"
	eventCommitless                 = "commitless"
	eventNeedsRepair                = "needs-repair"
	eventDepsInstalled              = "deps-installed"
	// eventSchedulerScan marks one claimNext pass: every ticket the epic
	// currently has, and why the scheduler did or didn't claim it. Added to
	// debug tickets that appear queued (e.g. a code-review ticket's freshly
	// published children) but never get picked up — the usual cause is a
	// ticket falling outside the run's RunScope (see ScanDecision's
	// "out-of-scope"), which the ticket-level events above never surface
	// since they only ever fire for a ticket the scheduler already claimed.
	eventSchedulerScan = "scheduler-scan"

	eventNotificationsConfigured = "notifications-configured"
	eventNotificationSent        = "notification-sent"
	eventNotificationFailed      = "notification-failed"
	// eventNotificationDegraded marks a send that only succeeded via
	// telegramTransport.sendSync's plain-text fallback (a MarkdownV2 parse
	// rejection on the first attempt) — deliberately distinct from
	// eventNotificationSent so a formatting downgrade, which signals the
	// chatmarkup escaper has a hole worth investigating, doesn't blend into
	// the ordinary-send count.
	eventNotificationDegraded = "notification-degraded"
	// eventNotificationSuppressed marks a close-time batch flush (see
	// chatEventSink.closeFlush) that was dropped rather than sent because the
	// transport was already globally muted — the one case a queued batch
	// never reaches chat at all, so the run-log line is what keeps the
	// outcome recoverable.
	eventNotificationSuppressed = "notification-suppressed"
)

// notifyKind* tag which live event triggered a notification-sent/
// notification-failed line — distinct from the Type field (which is always
// "notification-sent"/"notification-failed" itself).
const (
	notifyKindEpicStarted       = "epic-started"
	notifyKindIterationStarted  = "iteration-started"
	notifyKindIterationFinished = "iteration-finished"
	notifyKindIterationPaused   = "iteration-paused"
	notifyKindIterationResumed  = "iteration-resumed"
	notifyKindEpicComplete      = "epic-complete"
	notifyKindDrainComplete     = "drain-complete"
	notifyKindEpicParked        = "epic-parked"
	notifyKindTicketNeedsHuman  = "ticket-needs-human"
	notifyKindEpicFailed        = "epic-failed"
	// notifyKindMuted/notifyKindGloballyMuted tag the gate's own
	// edge-triggered "muting this"/"globally muted" notice (see
	// chatEventSink.send) — distinct from the live event that tripped the
	// mute, which is itself suppressed rather than sent.
	notifyKindMuted         = "muted"
	notifyKindGloballyMuted = "globally-muted"
	// notifyKindBatch tags a chatEventSink batch-queue flush (see
	// chatEventSink.flush/closeFlush) — one send covering every message
	// queued since the last flush, as opposed to notifyKind* tagging a single
	// live event under the pre-batch immediate-send model.
	notifyKindBatch = "batch"
)

// ScanDecision records one ticket's scheduling disposition for a single
// scheduler-scan log line, in Epic.Tickets order — every ticket
// loadNamedEpic returned at the moment claimNext evaluated the frontier, not
// just the ones in scope or in the frontier, so a ticket that's silently
// out-of-scope (the most common cause of "queued but never starts") still
// shows up.
type ScanDecision struct {
	// Ticket is the ticket's Identifier (see Event.Ticket).
	Ticket string `json:"ticket"`
	// Status is the ticket's RenderedStatus (see tickets.RenderedStatus.Word).
	Status string `json:"status"`
	// Decision is one of: "claimed" (this pass claimed it), "frontier"
	// (open/unblocked/in-scope but a different ticket was claimed instead,
	// or maxParallel was already reached elsewhere in this Run call),
	// "out-of-scope" (RunScope.Contains is false — e.g. a code-review
	// ticket's child missing its own parent: back-edge), "blocked", or
	// "settled" (already done/needs-answer/needs-repair).
	Decision string `json:"decision"`
	// Reason elaborates Decision — e.g. UnresolvedBlockers's tokens for
	// "blocked". Empty when Decision is self-explanatory.
	Reason string `json:"reason,omitempty"`
}

// Event is one line of an epic's run-log.jsonl: a single lifecycle
// occurrence, timestamped and attributed to the ticket/pane/tab it happened
// on. AgentSession is empty for events not tied to a specific agent session
// (e.g. conflict-hit, which precedes the conflict-resolution agent's own
// launch). Agent is omitted by historical logs and defaults to Claude when
// reports read them. Reason carries the reason for pause and attention
// events, or the install command run (empty if none matched) for
// deps-installed events.
type Event struct {
	Time time.Time `json:"time"`
	Type string    `json:"type"`
	// Ticket is the ticket's Identifier (the filename's full "NN[letter]"
	// prefix), not its Number, so lettered split siblings sharing a Number
	// (e.g. "04a"/"04b") remain distinguishable in the run log.
	Ticket       string    `json:"ticket"`
	Agent        AgentKind `json:"agent,omitempty"`
	Pane         string    `json:"pane,omitempty"`
	Tab          string    `json:"tab,omitempty"`
	AgentSession string    `json:"agent_session,omitempty"`
	// Cwd is the directory the agent session was launched in and is the local
	// session-data lookup key alongside AgentSession.
	Cwd    string `json:"cwd,omitempty"`
	Reason string `json:"reason,omitempty"`
	// SHA is the feature branch's tip commit right after a cherry-picked
	// event landed a ticket's iteration. Recorded because CherryPickRange
	// creates fresh commits (different hashes than the iteration branch's
	// originals), so it's the only durable record startup reconciliation can
	// check for reachability from the feature branch's later tip — the
	// iteration branch itself isn't guaranteed to still exist by then.
	SHA string `json:"sha,omitempty"`
	// Scan (scheduler-scan events only) is every ticket's disposition for
	// this claimNext pass — see ScanDecision.
	Scan []ScanDecision `json:"scan,omitempty"`
	// Telegram/Slack (notifications-configured only) record whether each
	// channel is active for this run — never the token/webhook values
	// themselves. Pointers (rather than plain bool) so "false" still
	// serializes explicitly instead of vanishing under omitempty.
	Telegram *bool `json:"telegram,omitempty"`
	Slack    *bool `json:"slack,omitempty"`
	// Channel (notification-sent/notification-failed only) is which channel
	// the attempt used: "telegram" or "slack".
	Channel string `json:"channel,omitempty"`
	// NotifyKind (notification-sent/notification-failed only) is the live
	// event that triggered the send attempt — see notifyKind* consts.
	NotifyKind string `json:"notify_kind,omitempty"`
	// Body (notification-sent/notification-failed only) is the message text
	// that was sent (or attempted), so counter/content bugs are diagnosable
	// from run-log.jsonl alone without reproducing the send.
	Body string `json:"body,omitempty"`
	// StateChangeSeq (iteration-started only, and only when the agent was
	// actually launched here rather than attached to) is herdr's
	// state_change_seq at the moment AgentStart returned — the launch-time
	// baseline a later collided reattach (attachToLiveAgent) compares its own
	// live reading against to tell "idle because it genuinely finished a
	// turn" from "idle because it never left this launch state" (see
	// stalledSinceLaunch).
	StateChangeSeq int `json:"state_change_seq,omitempty"`
}

// eventLogMu serializes appends across every goroutine in the process (each
// running iteration logs concurrently), so two events never interleave
// their JSON onto the same line.
var eventLogMu sync.Mutex

// runLogPath returns the append-only event log path for epicName under
// scratchDir.
func runLogPath(scratchDir, epicName string) string {
	return filepath.Join(scratchDir, epicName, "run-log.jsonl")
}

// logEvent appends ev as one JSON line to epicName's run-log.jsonl under
// scratchDir, creating the epic directory if it doesn't exist yet, and
// filling Time in if it's zero. It's a no-op if scratchDir or epicName is
// empty, so call sites that don't have logging wired up (e.g. the
// conflict-resolution launch, which logs conflict-hit/resolved manually
// instead of via the generic start/finish path) don't need to special-case
// it.
func logEvent(scratchDir, epicName string, ev Event) error {
	if scratchDir == "" || epicName == "" {
		return nil
	}
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	path := runLogPath(scratchDir, epicName)

	eventLogMu.Lock()
	defer eventLogMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// LogNotificationsConfigured records, once per epic run, which notification
// channels are active — never the underlying bot token/webhook URL — so an
// operator reading run-log.jsonl can tell a channel that was simply never
// configured apart from one whose sends are silently failing (see
// logNotificationSent/logNotificationFailed).
func LogNotificationsConfigured(scratchDir, epicName string, telegram, slack bool) error {
	return logEvent(scratchDir, epicName, Event{
		Type:     eventNotificationsConfigured,
		Telegram: &telegram,
		Slack:    &slack,
	})
}

// logNotificationSent/logNotificationFailed record one Telegram/Slack
// delivery attempt from inside chatEventSink's send, tagged with the
// channel and the live event that triggered it. Errors are
// swallowed like every other logEvent call site here — a failure to log
// shouldn't compound the failure it was trying to record.
func logNotificationSent(scratchDir, epicName, channel, notifyKind, body string) {
	_ = logEvent(scratchDir, epicName, Event{
		Type: eventNotificationSent, Channel: channel, NotifyKind: notifyKind, Body: body,
	})
}

func logNotificationFailed(scratchDir, epicName, channel, notifyKind, reason, body string) {
	_ = logEvent(scratchDir, epicName, Event{
		Type: eventNotificationFailed, Channel: channel, NotifyKind: notifyKind, Reason: reason, Body: body,
	})
}

// logNotificationDegraded records a send that only succeeded via
// telegramTransport.sendSync's plain-text fallback — see
// eventNotificationDegraded.
func logNotificationDegraded(scratchDir, epicName, channel, notifyKind, body string) {
	_ = logEvent(scratchDir, epicName, Event{
		Type: eventNotificationDegraded, Channel: channel, NotifyKind: notifyKind, Body: body,
	})
}

// degradedReason formats a degraded sendResult's original-failure
// description as an EventSink.NotificationFailed reason, for the TUI toast
// pipeline — reused rather than adding a dedicated EventSink method just for
// this label (see chatEventSink.sendSync/sendRaw's call sites).
func degradedReason(description string) string {
	return fmt.Sprintf("degraded to plain text: %s", description)
}

// logNotificationSuppressed records a close-time batch flush (see
// chatEventSink.closeFlush) that was dropped rather than sent because the
// transport was already globally muted — reason names the queued event
// kinds so the outcome stays recoverable from the run log alone.
func logNotificationSuppressed(scratchDir, epicName, channel, reason string) {
	_ = logEvent(scratchDir, epicName, Event{
		Type: eventNotificationSuppressed, Channel: channel, NotifyKind: notifyKindBatch, Reason: reason,
	})
}

// notificationRetryBackoff is the fixed delay between the first and second
// send attempt — long enough to ride out a brief network blip (the observed
// failure was a ~3-minute stall, but transient blips are typically much
// shorter) without materially adding to worst-case caller-visible latency.
const notificationRetryBackoff = 1500 * time.Millisecond

// maxHonoredRetryAfter caps how long sendWithRetry will honor a Telegram
// 429's retry_after as the retry delay. A long sleep happens inside a
// fire-and-forget goroutine (chatEventSink.sendRaw), so a wait past this
// cap would deliver badly out-of-order relative to the 6s periodic flush
// cycle (batchFlushInterval) — better to drop the retry than let that
// happen, so above the cap sendWithRetry skips the retry entirely rather
// than clamping to the cap.
const maxHonoredRetryAfter = 30 * time.Second

// nonRetryableStatusCodes are 4xx failures a retry cannot fix — the request
// itself is wrong (bad token/chat_id/payload), so a second identical attempt
// would just waste notificationRetryBackoff on a guaranteed-identical
// failure. 429 (rate limit) and 408 (request timeout) are deliberately
// excluded: both describe a transient condition a retry can resolve, and
// treating them as non-retryable would turn a recoverable failure into a
// dropped notification.
var nonRetryableStatusCodes = map[int]bool{
	400: true,
	401: true,
	403: true,
	404: true,
}

// retryDelay decides whether a failed attempt should be retried and, if so,
// after how long. A 429 honors the response's retry_after (capped at
// maxHonoredRetryAfter); every other retryable failure (network error,
// timeout, 5xx, 408 — anything not in nonRetryableStatusCodes) keeps the
// existing fixed notificationRetryBackoff.
func retryDelay(result sendResult) (time.Duration, bool) {
	if nonRetryableStatusCodes[result.StatusCode] {
		return 0, false
	}
	if result.StatusCode != http.StatusTooManyRequests {
		return notificationRetryBackoff, true
	}
	if result.RetryAfter == nil {
		return notificationRetryBackoff, true
	}
	delay := time.Duration(*result.RetryAfter) * time.Second
	if delay > maxHonoredRetryAfter {
		return 0, false
	}
	return delay, true
}

// sendNotification runs sendNotificationSync in its own goroutine, unbounded
// by any caller-supplied context (context.Background()), so a slow or
// unreachable notification endpoint never blocks the caller. This is
// epicFailureReporter's path — by the time it sends, the run's own sink has
// already closed and drained (see EventSink.EpicFailed), so there is no
// shutdown-ordering concern to cancel against; chatEventSink.sendRaw instead
// calls sendNotificationSync directly with its own cancellable context, so
// Close() can cut an in-flight attempt short (see chat_eventsink.go).
func sendNotification(scratchDir, epicName, channel, notifyKind, body string, timeout time.Duration, sendSync func(ctx context.Context) (sendResult, error), onFailed func(reason string)) {
	go sendNotificationSync(context.Background(), scratchDir, epicName, channel, notifyKind, body, timeout, sendSync, onFailed)
}

// sendNotificationSync blocks until sendSync succeeds or both attempts
// (initial + one retry after notificationRetryBackoff) fail, then records the
// final outcome — the attempt/backoff/retry/log logic extracted out of
// sendNotification's goroutine so a caller that needs to block (closeFlush's
// consolidated shutdown-time send) or cancel (chatEventSink.Close cutting an
// in-flight sendRaw short) can drive it directly instead of going through a
// fire-and-forget goroutine. ctx bounds the whole call, including the backoff
// wait between attempts (see sendWithRetry) — a caller that cancels ctx gets
// a fast failure instead of waiting out the full attempt+backoff+retry
// budget. Only the final outcome is logged — intermediate attempt failures
// are not — so run-log.jsonl gets one line per notification, not one per
// attempt. Any failure is logged via gx's logger and otherwise swallowed, but
// the final outcome is also durably recorded to run-log.jsonl via
// logNotificationSent/logNotificationFailed, tagged with channel and
// notifyKind (the live event that triggered it). onFailed, if non-nil, is
// called with the sanitized error once both attempts fail (or the call is
// cancelled) — chatEventSink.sendRaw uses it to also surface the failure as a
// LiveEventNotificationFailed on its embedded EventSink, for a TUI toast;
// epicFailureReporter's own call passes nil. A send that succeeds only via
// telegramTransport.sendSync's plain-text fallback (result.Degraded) is
// logged via logNotificationDegraded instead of logNotificationSent, and
// also routed through onFailed (with degradedReason's wording) so the
// downgrade still reaches the TUI toast — the message was delivered, so this
// is a deliberate reuse of the "failed" reporting channel for a "succeeded,
// but degraded" outcome, not a bug.
func sendNotificationSync(ctx context.Context, scratchDir, epicName, channel, notifyKind, body string, timeout time.Duration, sendSync func(ctx context.Context) (sendResult, error), onFailed func(reason string)) (sendResult, error) {
	result, err := sendWithRetry(ctx, timeout, sendSync)

	if err != nil {
		err = sanitizeSendError(err)
		logger.Debug("%s: %v\n", channel, err)
		logNotificationFailed(scratchDir, epicName, channel, notifyKind, err.Error(), body)
		if onFailed != nil {
			onFailed(err.Error())
		}
		return result, err
	}
	if result.Degraded {
		logNotificationDegraded(scratchDir, epicName, channel, notifyKind, body)
		if onFailed != nil {
			onFailed(degradedReason(result.Description))
		}
		return result, nil
	}
	logNotificationSent(scratchDir, epicName, channel, notifyKind, body)
	return result, nil
}

// sendWithRetry runs sendSync once, retrying after a delay decided by
// retryDelay if the first attempt fails and is retryable, and returns
// whichever attempt's sendResult/error is last — the one that succeeded, or
// the last failure if none did. A non-retryable failure (retryDelay's second
// return is false — a deterministic 4xx, or a 429 whose retry_after is
// absent/unparseable/above maxHonoredRetryAfter) returns the first attempt's
// result immediately, with no second attempt and no wait. ctx bounds both
// the per-attempt timeout (each attempt gets a child context.WithTimeout
// derived from ctx, so a cancelled/expired ctx cuts an in-progress attempt
// short too) and the backoff wait itself, so a cancelled ctx fails fast
// rather than sleeping out the full delay first; if ctx carries a deadline
// (03a's shutdown budget) shorter than the decided delay, the retry is
// skipped the same way rather than waiting past that deadline. Extracted
// from sendNotificationSync so the "first attempt fails, second succeeds
// degraded" carry-through is directly testable without racing a background
// goroutine and polling run-log.jsonl.
func sendWithRetry(ctx context.Context, timeout time.Duration, sendSync func(ctx context.Context) (sendResult, error)) (sendResult, error) {
	attempt := func() (sendResult, error) {
		attemptCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return sendSync(attemptCtx)
	}

	result, err := attempt()
	if err == nil {
		return result, err
	}
	delay, retryable := retryDelay(result)
	if !retryable {
		return result, err
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < delay {
		return result, err
	}
	select {
	case <-ctx.Done():
		return result, ctx.Err()
	case <-time.After(delay):
	}
	return attempt()
}

// sanitizeSendError strips the request URL from a failed send's error before
// it reaches logger.Debug or run-log.jsonl. telegramTransport/slackTransport
// build that URL with the bot token/webhook secret embedded in the path, and
// (*url.Error).Error() — what http.Client.Do returns on failure — includes
// the full URL verbatim, so logging it as-is leaks the secret. The
// underlying cause (timeout, connection refused, DNS failure, etc.) is kept
// so the diagnostic value survives; only the URL is dropped.
func sanitizeSendError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

// lastIterationSession finds the most recent iteration-started event for
// identifier (a ticket's Identifier, not Number, so lettered split siblings
// aren't cross-attributed) with a recorded AgentSession — the session
// id/cwd/agent to backfill a reattached ticket close's metadata with (ticket
// 06a), since the reattaching run never captured a fresh session of its own.
// Agent defaults to AgentClaude for historical logs that omitted it (see
// Event.Agent).
func lastIterationSession(events []Event, identifier string) (agentSession, cwd string, agent AgentKind, ok bool) {
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Ticket != identifier || ev.Type != eventIterationStarted || ev.AgentSession == "" {
			continue
		}
		agent = ev.Agent
		if agent == "" {
			agent = AgentClaude
		}
		return ev.AgentSession, ev.Cwd, agent, true
	}
	return "", "", "", false
}

// readEvents reads and parses every line of epicName's run-log.jsonl under
// scratchDir, skipping malformed lines rather than failing the whole read
// (a run-log written by a process killed mid-write may have a torn final
// line). ok is false if the log doesn't exist yet.
// ReadEvents is readEvents exported for callers outside the ralphloop package
// (e.g. `gx tickets filter-run-log`), so a run-log.jsonl reader never
// hand-rolls a second decoding of the same file.
func ReadEvents(scratchDir, epicName string) (events []Event, ok bool, err error) {
	return readEvents(scratchDir, epicName)
}

func readEvents(scratchDir, epicName string) (events []Event, ok bool, err error) {
	raw, err := os.ReadFile(runLogPath(scratchDir, epicName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	return events, true, nil
}
