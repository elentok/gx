package ralphloop

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	eventsc "github.com/elentok/gx/events"
)

func TestLogEvent_AppendsOneJSONLinePerCall(t *testing.T) {
	t.Parallel()
	dir := epicScratchDir(t, "epic")

	if err := logEvent(dir, "epic", Event{Type: string(eventsc.IterationStarted), Ticket: "01", Pane: "pane-1", AgentSession: "sess-1"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(dir, "epic", Event{Type: string(eventsc.IterationFinished), Ticket: "01"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}

	raw, err := os.ReadFile(runLogPath(dir, "epic"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2:\n%s", len(lines), raw)
	}
	if !strings.Contains(lines[0], `"iteration-started"`) || !strings.Contains(lines[0], `"sess-1"`) {
		t.Errorf("line 0 = %q, want it to record type+agent_session", lines[0])
	}
	if !strings.Contains(lines[1], `"iteration-finished"`) {
		t.Errorf("line 1 = %q, want it to record iteration-finished", lines[1])
	}
}

func TestAppendEvent_NewFieldsRoundTripAsOneLine(t *testing.T) {
	t.Parallel()
	dir := epicScratchDir(t, "epic")

	err := AppendEvent(dir, "epic", Event{
		Type: string(eventsc.ManualLand), Ticket: "04", Outcome: "landed",
		TrailerValue: "epic/04", AtticRef: "refs/attic/04", Reason: "by hand",
	})
	if err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	if err := AppendEvent(dir, "epic", Event{Type: string(eventsc.TicketReset), Ticket: "05"}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	raw, err := os.ReadFile(runLogPath(dir, "epic"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if n := strings.Count(string(raw), "\n"); n != 2 {
		t.Fatalf("got %d lines, want 2:\n%s", n, raw)
	}
	events, ok, err := ReadEvents(dir, "epic")
	if err != nil || !ok || len(events) != 2 {
		t.Fatalf("ReadEvents = %v, %v, %v; want 2 events", events, ok, err)
	}
	got := events[0]
	if got.Type != "manual-land" || got.Outcome != "landed" || got.TrailerValue != "epic/04" || got.AtticRef != "refs/attic/04" {
		t.Errorf("event 0 = %+v, want new fields round-tripped", got)
	}
	if events[1].Type != "ticket-reset" || strings.Contains(strings.Split(string(raw), "\n")[1], "outcome") {
		t.Errorf("event 1 should omit empty new fields: %q", raw)
	}
}

func TestAppendEvent_OversizedReasonIsTruncatedBelowCap(t *testing.T) {
	t.Parallel()
	dir := epicScratchDir(t, "epic")

	// Multi-byte and escaped characters exercise the encoded-size accounting.
	reason := strings.Repeat("é\"\n", 5000)
	if err := AppendEvent(dir, "epic", Event{Type: string(eventsc.ManualLand), Ticket: "04", Reason: reason}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	raw, err := os.ReadFile(runLogPath(dir, "epic"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(raw) > eventsc.MaxLineBytes {
		t.Errorf("line is %d bytes, want <= %d", len(raw), eventsc.MaxLineBytes)
	}
	events, _, _ := ReadEvents(dir, "epic")
	if len(events) != 1 || events[0].Reason == "" || !strings.HasPrefix(reason, events[0].Reason) {
		t.Errorf("want one event with a non-empty truncated prefix of Reason, got %+v", events)
	}
}

func TestLogEvent_FillsInTimeWhenZero(t *testing.T) {
	t.Parallel()
	dir := epicScratchDir(t, "epic")
	before := time.Now()
	if err := logEvent(dir, "epic", Event{Type: string(eventsc.NeedsAnswer), Ticket: "02"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	events, ok, err := ReadEvents(dir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents: ok=%v err=%v", ok, err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Time.Before(before.Add(-time.Second)) {
		t.Errorf("Time = %v, want it defaulted to roughly now (%v)", events[0].Time, before)
	}
}

func TestLogEvent_EmptyScratchDirOrEpicName_NoOp(t *testing.T) {
	t.Parallel()
	if err := logEvent("", "epic", Event{Type: string(eventsc.NeedsAnswer)}); err != nil {
		t.Errorf("logEvent(scratchDir=\"\") error = %v, want nil no-op", err)
	}
	if err := logEvent(t.TempDir(), "", Event{Type: string(eventsc.NeedsAnswer)}); err != nil {
		t.Errorf("logEvent(epicName=\"\") error = %v, want nil no-op", err)
	}
}

func TestReadEvents_NoLogYet_OkFalse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	events, ok, err := ReadEvents(dir, "epic")
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if ok {
		t.Error("ok = true, want false when run-log.jsonl doesn't exist yet")
	}
	if events != nil {
		t.Errorf("events = %v, want nil", events)
	}
}

func TestReadEvents_SkipsMalformedTrailingLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := runLogPath(dir, "epic")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := `{"type":"iteration-started","ticket":"01"}` + "\n" + `{"type":"iteration-fin` // torn last line
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	events, ok, err := ReadEvents(dir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents: ok=%v err=%v", ok, err)
	}
	if len(events) != 1 || events[0].Type != string(eventsc.IterationStarted) {
		t.Errorf("events = %+v, want only the well-formed first line", events)
	}
}

func TestLastIterationSession_ReturnsMostRecentMatchingTicket(t *testing.T) {
	t.Parallel()
	events := []Event{
		{Type: string(eventsc.IterationStarted), Ticket: "01", Agent: AgentClaude, AgentSession: "sess-1a", Cwd: "/cwd-1a"},
		{Type: string(eventsc.IterationFinished), Ticket: "01"},
		{Type: string(eventsc.NeedsAnswer), Ticket: "01"},
		{Type: string(eventsc.IterationStarted), Ticket: "01", Agent: AgentClaude, AgentSession: "sess-1b", Cwd: "/cwd-1b"},
		{Type: string(eventsc.IterationStarted), Ticket: "02", Agent: AgentClaude, AgentSession: "sess-2", Cwd: "/cwd-2"},
	}

	session, cwd, agent, ok := lastIterationSession(events, "01")
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if session != "sess-1b" || cwd != "/cwd-1b" || agent != AgentClaude {
		t.Errorf("got (session=%q, cwd=%q, agent=%q), want the most recent iteration-started for ticket 1 (sess-1b/cwd-1b)", session, cwd, agent)
	}
}

func TestLastIterationSession_DefaultsAgentForHistoricalLogs(t *testing.T) {
	t.Parallel()
	events := []Event{
		{Type: string(eventsc.IterationStarted), Ticket: "01", AgentSession: "sess-1"},
	}
	_, _, agent, ok := lastIterationSession(events, "01")
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if agent != AgentClaude {
		t.Errorf("agent = %q, want AgentClaude default for an event with no recorded Agent", agent)
	}
}

func TestLastIterationSession_NoMatch_OkFalse(t *testing.T) {
	t.Parallel()
	events := []Event{
		{Type: string(eventsc.IterationStarted), Ticket: "02", AgentSession: "sess-2"},
		{Type: string(eventsc.IterationStarted), Ticket: "01", AgentSession: ""},
	}
	_, _, _, ok := lastIterationSession(events, "01")
	if ok {
		t.Error("ok = true, want false when no matching event has a recorded session")
	}
}

func TestLogEvent_ConcurrentAppends_NeverInterleave(t *testing.T) {
	t.Parallel()
	dir := epicScratchDir(t, "epic")
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = logEvent(dir, "epic", Event{Type: string(eventsc.IterationStarted), Ticket: fmt.Sprintf("%02d", n)})
		}(i)
	}
	wg.Wait()

	events, ok, err := ReadEvents(dir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents: ok=%v err=%v", ok, err)
	}
	if len(events) != 20 {
		t.Errorf("got %d events, want 20 (no interleaved/corrupted lines)", len(events))
	}
}

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

func TestLogNotificationsConfigured_RecordsBooleansForBothChannels(t *testing.T) {
	t.Parallel()
	dir := epicScratchDir(t, "epic")
	if err := LogNotificationsConfigured(dir, "epic", true, false); err != nil {
		t.Fatalf("LogNotificationsConfigured: %v", err)
	}

	events, ok, err := ReadEvents(dir, "epic")
	if err != nil || !ok || len(events) != 1 {
		t.Fatalf("ReadEvents: events=%#v ok=%v err=%v", events, ok, err)
	}
	ev := events[0]
	if ev.Type != string(eventsc.NotificationsConfigured) {
		t.Errorf("Type = %q, want %q", ev.Type, string(eventsc.NotificationsConfigured))
	}
	if ev.Telegram == nil || *ev.Telegram != true {
		t.Errorf("Telegram = %v, want true", ev.Telegram)
	}
	if ev.Slack == nil || *ev.Slack != false {
		t.Errorf("Slack = %v, want false (recorded explicitly, not omitted)", ev.Slack)
	}
}

func TestLogNotificationSentAndFailed_RecordChannelAndTriggeringKind(t *testing.T) {
	t.Parallel()
	dir := epicScratchDir(t, "epic")
	logNotificationSent(dir, "epic", "telegram", notifyKindEpicComplete, "epic complete!")
	logNotificationFailed(dir, "epic", "slack", notifyKindIterationPaused, "post failed: 500", "iteration paused")

	events, ok, err := ReadEvents(dir, "epic")
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
	dir := epicScratchDir(t, "epic")
	var attempts atomic.Int32
	sendSync := func(ctx context.Context) (sendResult, error) {
		if attempts.Add(1) == 1 {
			return sendResult{}, errors.New("transient failure")
		}
		return sendResult{}, nil
	}

	var onFailedCalls atomic.Int32
	sendNotification(dir, "epic", "slack", notifyKindEpicComplete, "epic complete!", time.Second, sendSync, func(string) { onFailedCalls.Add(1) })

	var events []Event
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		events, _, _ = ReadEvents(dir, "epic")
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
	dir := epicScratchDir(t, "epic")
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

	var events []Event
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		events, _, _ = ReadEvents(dir, "epic")
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

func TestLogEvent_FailureEventWithoutKindIsRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	err := logEvent(dir, "epic", Event{Type: string(eventsc.LaunchFailed), Ticket: "01", Reason: "free text only"})
	if err == nil {
		t.Fatal("logEvent accepted a launch-failed event without a kind")
	}
	if _, statErr := os.Stat(runLogPath(dir, "epic")); !os.IsNotExist(statErr) {
		t.Errorf("rejected event still reached the log (stat err %v)", statErr)
	}
}

// canonicalAddr is the address logEvent stamps for ticket id in epic under
// scratchDir (no project.json in tests, so the project is the dir name).
func canonicalAddr(scratchDir, epic, id string) string {
	return filepath.Base(scratchDir) + ":" + epic + "/" + id
}

func TestLogEvent_EveryTicketEventCarriesCanonicalAddressInEpicLog(t *testing.T) {
	t.Parallel()
	dir := epicScratchDir(t, "epic")

	if err := logEvent(dir, "epic", Event{Type: string(eventsc.IterationStarted), Ticket: "05"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	evs, ok, err := ReadEvents(dir, "epic")
	if err != nil || !ok || len(evs) != 1 {
		t.Fatalf("ReadEvents = %v, %v, %v; want one event in the epic's store dir", evs, ok, err)
	}
	if want := canonicalAddr(dir, "epic", "05"); evs[0].Address != want {
		t.Errorf("address = %q, want %q", evs[0].Address, want)
	}
}

func TestLogEvent_FailureEventCarriesAddressAndTruncatedReason(t *testing.T) {
	t.Parallel()
	dir := epicScratchDir(t, "epic")

	err := logEvent(dir, "epic", Event{
		Type: string(eventsc.LaunchFailed), Kind: string(eventsc.AgentNameTaken), Ticket: "07",
		Attempt: 2, Reason: strings.Repeat("x", 10000),
	})
	if err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	raw, _ := os.ReadFile(runLogPath(dir, "epic"))
	if len(raw) > eventsc.MaxLineBytes {
		t.Errorf("line is %d bytes, want <= %d", len(raw), eventsc.MaxLineBytes)
	}
	line := string(raw)
	for _, want := range []string{`"kind":"agent_name_taken"`, `"address":"` + canonicalAddr(dir, "epic", "07") + `"`, `"attempt":2`} {
		if !strings.Contains(line, want) {
			t.Errorf("line missing %s: %.200s", want, line)
		}
	}
	if strings.Contains(line, `"iteration"`) {
		t.Errorf("unknown iteration must be omitted, not a placeholder: %.200s", line)
	}
}

// epicScratchDir returns a fresh scratch dir with epicName's directory
// already created, since logEvent drops events for a missing epic dir.
func TestAppendEvent_MissingEpicDirIsNotCreated(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()

	if err := AppendEvent(scratchDir, "no-such-epic", Event{Type: string(eventsc.IterationStarted), Ticket: "01"}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	if _, err := os.Stat(filepath.Join(scratchDir, "no-such-epic")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("epic dir stat err = %v, want not-exist", err)
	}
}

func epicScratchDir(t *testing.T, epicName string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, epicName), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	return dir
}
