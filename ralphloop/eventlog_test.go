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
)

func TestLogEvent_AppendsOneJSONLinePerCall(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := logEvent(dir, "epic", Event{Type: eventIterationStarted, Ticket: "01", Pane: "pane-1", Tab: "tab-1", AgentSession: "sess-1"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(dir, "epic", Event{Type: eventIterationFinished, Ticket: "01"}); err != nil {
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

func TestLogEvent_FillsInTimeWhenZero(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	before := time.Now()
	if err := logEvent(dir, "epic", Event{Type: eventNeedsAnswer, Ticket: "02"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	events, ok, err := readEvents(dir, "epic")
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
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
	if err := logEvent("", "epic", Event{Type: eventNeedsAnswer}); err != nil {
		t.Errorf("logEvent(scratchDir=\"\") error = %v, want nil no-op", err)
	}
	if err := logEvent(t.TempDir(), "", Event{Type: eventNeedsAnswer}); err != nil {
		t.Errorf("logEvent(epicName=\"\") error = %v, want nil no-op", err)
	}
}

func TestReadEvents_NoLogYet_OkFalse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	events, ok, err := readEvents(dir, "epic")
	if err != nil {
		t.Fatalf("readEvents: %v", err)
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

	events, ok, err := readEvents(dir, "epic")
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	if len(events) != 1 || events[0].Type != eventIterationStarted {
		t.Errorf("events = %+v, want only the well-formed first line", events)
	}
}

func TestLastIterationSession_ReturnsMostRecentMatchingTicket(t *testing.T) {
	t.Parallel()
	events := []Event{
		{Type: eventIterationStarted, Ticket: "01", Agent: AgentClaude, AgentSession: "sess-1a", Cwd: "/cwd-1a"},
		{Type: eventIterationFinished, Ticket: "01"},
		{Type: eventNeedsAnswer, Ticket: "01"},
		{Type: eventIterationStarted, Ticket: "01", Agent: AgentClaude, AgentSession: "sess-1b", Cwd: "/cwd-1b"},
		{Type: eventIterationStarted, Ticket: "02", Agent: AgentClaude, AgentSession: "sess-2", Cwd: "/cwd-2"},
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
		{Type: eventIterationStarted, Ticket: "01", AgentSession: "sess-1"},
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
		{Type: eventIterationStarted, Ticket: "02", AgentSession: "sess-2"},
		{Type: eventIterationStarted, Ticket: "01", AgentSession: ""},
	}
	_, _, _, ok := lastIterationSession(events, "01")
	if ok {
		t.Error("ok = true, want false when no matching event has a recorded session")
	}
}

func TestLogEvent_ConcurrentAppends_NeverInterleave(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = logEvent(dir, "epic", Event{Type: eventIterationStarted, Ticket: fmt.Sprintf("%02d", n)})
		}(i)
	}
	wg.Wait()

	events, ok, err := readEvents(dir, "epic")
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
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
	dir := t.TempDir()
	if err := LogNotificationsConfigured(dir, "epic", true, false); err != nil {
		t.Fatalf("LogNotificationsConfigured: %v", err)
	}

	events, ok, err := readEvents(dir, "epic")
	if err != nil || !ok || len(events) != 1 {
		t.Fatalf("readEvents: events=%#v ok=%v err=%v", events, ok, err)
	}
	ev := events[0]
	if ev.Type != eventNotificationsConfigured {
		t.Errorf("Type = %q, want %q", ev.Type, eventNotificationsConfigured)
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
	dir := t.TempDir()
	logNotificationSent(dir, "epic", "telegram", notifyKindEpicComplete, "epic complete!")
	logNotificationFailed(dir, "epic", "slack", notifyKindIterationPaused, "post failed: 500", "iteration paused")

	events, ok, err := readEvents(dir, "epic")
	if err != nil || !ok || len(events) != 2 {
		t.Fatalf("readEvents: events=%#v ok=%v err=%v", events, ok, err)
	}
	sent, failed := events[0], events[1]
	if sent.Type != eventNotificationSent || sent.Channel != "telegram" || sent.NotifyKind != notifyKindEpicComplete || sent.Body != "epic complete!" {
		t.Errorf("sent event = %#v", sent)
	}
	if failed.Type != eventNotificationFailed || failed.Channel != "slack" || failed.NotifyKind != notifyKindIterationPaused || failed.Reason != "post failed: 500" || failed.Body != "iteration paused" {
		t.Errorf("failed event = %#v", failed)
	}
}

func TestSendNotification_FailsOnceThenSucceeds_LogsOneSentAndNoFailed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
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
		events, _, _ = readEvents(dir, "epic")
		if len(events) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(events) != 1 || events[0].Type != eventNotificationSent || events[0].Body != "epic complete!" {
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
	dir := t.TempDir()
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
		events, _, _ = readEvents(dir, "epic")
		if len(events) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(events) != 1 || events[0].Type != eventNotificationFailed || events[0].Body != "epic complete!" {
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
