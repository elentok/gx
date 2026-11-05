package ralphloop

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/logger"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/runlog"
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
	notifyKindEpicPaused        = "epic-paused"
	notifyKindTicketNeedsHuman  = "ticket-needs-human"
	notifyKindEpicFailed        = "epic-failed"
	notifyKindConflictStarted   = "conflict-resolution-started"
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

// logNotificationSent/logNotificationFailed record one Telegram/Slack
// delivery attempt from inside chatEventSink's send, tagged with the
// channel and the live event that triggered it. Errors are
// swallowed like every other runlog.AppendEvent call site here — a failure to log
// shouldn't compound the failure it was trying to record.
func logNotificationSent(scratchDir, epicName, channel, notifyKind, body string) {
	_ = runlog.AppendEvent(scratchDir, epicName, runlog.Event{
		Type: string(events.NotificationSent), Channel: channel, NotifyKind: notifyKind, Body: body,
	})
}

func logNotificationFailed(scratchDir, epicName, channel, notifyKind, reason, body string) {
	_ = runlog.AppendEvent(scratchDir, epicName, runlog.Event{
		Type: string(events.NotificationFailed), Channel: channel, NotifyKind: notifyKind, Reason: reason, Body: body,
	})
}

// logNotificationDegraded records a send that only succeeded via
// telegramTransport.sendSync's plain-text fallback — see
// string(events.NotificationDegraded).
func logNotificationDegraded(scratchDir, epicName, channel, notifyKind, body string) {
	_ = runlog.AppendEvent(scratchDir, epicName, runlog.Event{
		Type: string(events.NotificationDegraded), Channel: channel, NotifyKind: notifyKind, Body: body,
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
	_ = runlog.AppendEvent(scratchDir, epicName, runlog.Event{
		Type: string(events.NotificationSuppressed), Channel: channel, NotifyKind: notifyKindBatch, Reason: reason,
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
// unreachable notification endpoint never blocks the caller. Only tests call
// it now; chatEventSink.sendRaw instead
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
// NotificationFailed on its embedded EventSink, for a TUI toast; a nil
// onFailed skips that. A send that succeeds only via
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
// Agent defaults to agent.AgentClaude for historical logs that omitted it (see
// runlog.Event.Agent).
func lastIterationSession(evs []runlog.Event, identifier string) (agentSession, cwd string, agentKind agent.AgentKind, ok bool) {
	all := runlog.IterationSessions(evs, identifier)
	if len(all) == 0 {
		return "", "", "", false
	}
	return all[0].Session, all[0].Cwd, all[0].Agent, true
}
