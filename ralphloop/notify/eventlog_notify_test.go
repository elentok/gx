package notify

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	eventsc "github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/ralphloop/runlog"
)

func TestSanitizeSendError_StripsURLFromURLError(t *testing.T) {
	t.Parallel()
	underlying := errors.New("connection refused")
	err := &url.Error{Op: "Post", URL: "https://api.telegram.org/botsecret-token-abc/sendMessage", Err: underlying}

	got := sanitizeSendError(err)

	if strings.Contains(got.Error(), "secret-token-abc") {
		t.Errorf("sanitizeSendError(%v) = %q, must not contain the URL/token", err, got.Error())
	}
	if !errors.Is(got, underlying) {
		t.Errorf("sanitizeSendError(%v) = %v, want it to still wrap the underlying cause %v", err, got, underlying)
	}
}

func TestSanitizeSendError_LeavesNonURLErrorsUnchanged(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("send failed with status %d", 500)

	if got := sanitizeSendError(err); got != err {
		t.Errorf("sanitizeSendError(%v) = %v, want unchanged", err, got)
	}
}

func TestLogNotificationSentAndFailed_RecordChannelAndTriggeringKind(t *testing.T) {
	t.Parallel()
	dir := fixture.EpicScratchDir(t, "epic")
	logNotificationSent(dir, "epic", "telegram", notifyKindEpicComplete, "epic complete!")
	logNotificationFailed(dir, "epic", "slack", notifyKindIterationPaused, "post failed: 500", "iteration paused")

	events, ok, err := runlog.ReadEvents(dir, "epic")
	if err != nil || !ok || len(events) != 2 {
		t.Fatalf("ReadEvents: events=%#v ok=%v err=%v", events, ok, err)
	}
	sent, failed := events[0], events[1]
	if sent.Type != string(eventsc.NotificationSent) || sent.Channel != "telegram" || sent.NotifyKind != notifyKindEpicComplete || sent.Body != "epic complete!" {
		t.Errorf("sent event = %#v", sent)
	}
	if failed.Type != string(eventsc.NotificationFailed) || failed.Channel != "slack" || failed.NotifyKind != notifyKindIterationPaused || failed.Reason != "post failed: 500" || failed.Body != "iteration paused" {
		t.Errorf("failed event = %#v", failed)
	}
}

func TestSendNotification_FailsOnceThenSucceeds_LogsOneSentAndNoFailed(t *testing.T) {
	t.Parallel()
	dir := fixture.EpicScratchDir(t, "epic")
	var attempts atomic.Int32
	sendSync := func(ctx context.Context) (sendResult, error) {
		if attempts.Add(1) == 1 {
			return sendResult{}, errors.New("transient failure")
		}
		return sendResult{}, nil
	}

	var onFailedCalls atomic.Int32
	sendNotification(dir, "epic", "slack", notifyKindEpicComplete, "epic complete!", time.Second, sendSync, func(string) { onFailedCalls.Add(1) })

	var events []runlog.Event
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		events, _, _ = runlog.ReadEvents(dir, "epic")
		if len(events) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(events) != 1 || events[0].Type != string(eventsc.NotificationSent) || events[0].Body != "epic complete!" {
		t.Fatalf("run-log events = %#v, want exactly one notification-sent with body", events)
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
	if got := onFailedCalls.Load(); got != 0 {
		t.Errorf("onFailed calls = %d, want 0 (send eventually succeeded)", got)
	}
}

func TestSendNotification_FailsEveryAttempt_LogsOneFailedAndCallsOnFailed(t *testing.T) {
	t.Parallel()
	dir := fixture.EpicScratchDir(t, "epic")
	var attempts atomic.Int32
	sendSync := func(ctx context.Context) (sendResult, error) {
		attempts.Add(1)
		return sendResult{}, errors.New("permanent failure")
	}

	var onFailedReason string
	var onFailedCalls atomic.Int32
	sendNotification(dir, "epic", "slack", notifyKindEpicComplete, "epic complete!", time.Second, sendSync, func(reason string) {
		onFailedReason = reason
		onFailedCalls.Add(1)
	})

	var events []runlog.Event
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		events, _, _ = runlog.ReadEvents(dir, "epic")
		// onFailed runs after the event is logged; wait for both.
		if len(events) > 0 && onFailedCalls.Load() > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(events) != 1 || events[0].Type != string(eventsc.NotificationFailed) || events[0].Body != "epic complete!" {
		t.Fatalf("run-log events = %#v, want exactly one notification-failed with body", events)
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
	if got := onFailedCalls.Load(); got != 1 {
		t.Errorf("onFailed calls = %d, want exactly 1", got)
	}
	if onFailedReason != "permanent failure" {
		t.Errorf("onFailed reason = %q, want %q", onFailedReason, "permanent failure")
	}
}

func TestSendWithRetry_FirstAttemptFailsSecondSucceedsDegraded_ReturnsDegradedResult(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	sendSync := func(ctx context.Context) (sendResult, error) {
		if attempts.Add(1) == 1 {
			return sendResult{Degraded: false}, errors.New("markdown rejected")
		}
		return sendResult{StatusCode: 200, Degraded: true}, nil
	}

	result, err := sendWithRetry(context.Background(), time.Second, sendSync)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !result.Degraded {
		t.Errorf("result.Degraded = false, want true (naive first-attempt-only implementations would report false here)")
	}
	if result.StatusCode != 200 {
		t.Errorf("result.StatusCode = %d, want 200", result.StatusCode)
	}
}

func TestSendWithRetry_NonRetryable4xx_MakesOnlyOneAttempt(t *testing.T) {
	t.Parallel()
	for _, code := range []int{400, 401, 403, 404} {
		t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
			t.Parallel()
			var attempts atomic.Int32
			sendSync := func(ctx context.Context) (sendResult, error) {
				attempts.Add(1)
				return sendResult{StatusCode: code}, fmt.Errorf("send failed with status %d", code)
			}

			start := time.Now()
			_, err := sendWithRetry(context.Background(), time.Second, sendSync)
			elapsed := time.Since(start)

			if err == nil {
				t.Fatalf("err = nil, want failure")
			}
			if got := attempts.Load(); got != 1 {
				t.Errorf("attempts = %d, want 1 (no retry for status %d)", got, code)
			}
			if elapsed >= notificationRetryBackoff {
				t.Errorf("elapsed = %v, want well under notificationRetryBackoff (no backoff sleep)", elapsed)
			}
		})
	}
}

func TestSendWithRetry_429WithRetryAfterUnderCap_HonorsRetryAfterAsDelay(t *testing.T) {
	t.Parallel()
	retryAfter := 1
	var attempts atomic.Int32
	var secondAttemptAt time.Time
	firstAttemptAt := time.Now()
	sendSync := func(ctx context.Context) (sendResult, error) {
		if attempts.Add(1) == 1 {
			return sendResult{StatusCode: 429, RetryAfter: &retryAfter}, errors.New("rate limited")
		}
		secondAttemptAt = time.Now()
		return sendResult{StatusCode: 200}, nil
	}

	result, err := sendWithRetry(context.Background(), time.Second, sendSync)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if result.StatusCode != 200 {
		t.Errorf("result.StatusCode = %d, want 200", result.StatusCode)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
	delay := secondAttemptAt.Sub(firstAttemptAt)
	if delay < time.Duration(retryAfter)*time.Second {
		t.Errorf("delay = %v, want at least retry_after (%ds)", delay, retryAfter)
	}
	if delay >= notificationRetryBackoff {
		t.Errorf("delay = %v, want under the fixed notificationRetryBackoff (%v) — retry_after should have been honored instead", delay, notificationRetryBackoff)
	}
}

func TestSendWithRetry_429AboveCap_SkipsRetry(t *testing.T) {
	t.Parallel()
	aboveCap := 31
	var attempts atomic.Int32
	sendSync := func(ctx context.Context) (sendResult, error) {
		attempts.Add(1)
		return sendResult{StatusCode: 429, RetryAfter: &aboveCap}, errors.New("rate limited")
	}

	_, err := sendWithRetry(context.Background(), time.Second, sendSync)
	if err == nil {
		t.Fatalf("err = nil, want failure")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (retry skipped)", got)
	}
}

func TestSendWithRetry_OtherRetryableFailures_KeepFixedBackoffRetryOnce(t *testing.T) {
	t.Parallel()
	statusCodes := []int{0, 408, 429, 500, 503}
	for _, code := range statusCodes {
		t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
			t.Parallel()
			var attempts atomic.Int32
			sendSync := func(ctx context.Context) (sendResult, error) {
				if attempts.Add(1) == 1 {
					return sendResult{StatusCode: code}, errors.New("transient failure")
				}
				return sendResult{StatusCode: 200}, nil
			}

			result, err := sendWithRetry(context.Background(), time.Second, sendSync)
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if result.StatusCode != 200 {
				t.Errorf("result.StatusCode = %d, want 200", result.StatusCode)
			}
			if got := attempts.Load(); got != 2 {
				t.Errorf("attempts = %d, want 2 (fixed-backoff retry-once unchanged)", got)
			}
		})
	}
}

func TestSendWithRetry_DeadlineShorterThanRetryAfter_SkipsRetry(t *testing.T) {
	t.Parallel()
	retryAfter := 5
	var attempts atomic.Int32
	sendSync := func(ctx context.Context) (sendResult, error) {
		attempts.Add(1)
		return sendResult{StatusCode: 429, RetryAfter: &retryAfter}, errors.New("rate limited")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := sendWithRetry(ctx, 50*time.Millisecond, sendSync)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("err = nil, want failure")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (retry skipped: retry_after exceeds remaining deadline)", got)
	}
	if elapsed >= time.Duration(retryAfter)*time.Second {
		t.Errorf("elapsed = %v, want well under retry_after (%ds) — should not have waited it out", elapsed, retryAfter)
	}
}
