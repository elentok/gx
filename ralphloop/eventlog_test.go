package ralphloop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLogEvent_AppendsOneJSONLinePerCall(t *testing.T) {
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
	dir := t.TempDir()
	before := time.Now()
	if err := logEvent(dir, "epic", Event{Type: eventNeedsInfo, Ticket: "02"}); err != nil {
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
	if err := logEvent("", "epic", Event{Type: eventNeedsInfo}); err != nil {
		t.Errorf("logEvent(scratchDir=\"\") error = %v, want nil no-op", err)
	}
	if err := logEvent(t.TempDir(), "", Event{Type: eventNeedsInfo}); err != nil {
		t.Errorf("logEvent(epicName=\"\") error = %v, want nil no-op", err)
	}
}

func TestReadEvents_NoLogYet_OkFalse(t *testing.T) {
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
	events := []Event{
		{Type: eventIterationStarted, Ticket: "01", Agent: AgentClaude, AgentSession: "sess-1a", Cwd: "/cwd-1a"},
		{Type: eventIterationFinished, Ticket: "01"},
		{Type: eventNeedsInfo, Ticket: "01"},
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

func TestLogNotificationsConfigured_RecordsBooleansForBothChannels(t *testing.T) {
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
	dir := t.TempDir()
	logNotificationSent(dir, "epic", "telegram", notifyKindEpicComplete)
	logNotificationFailed(dir, "epic", "slack", notifyKindIterationPaused, "post failed: 500")

	events, ok, err := readEvents(dir, "epic")
	if err != nil || !ok || len(events) != 2 {
		t.Fatalf("readEvents: events=%#v ok=%v err=%v", events, ok, err)
	}
	sent, failed := events[0], events[1]
	if sent.Type != eventNotificationSent || sent.Channel != "telegram" || sent.NotifyKind != notifyKindEpicComplete {
		t.Errorf("sent event = %#v", sent)
	}
	if failed.Type != eventNotificationFailed || failed.Channel != "slack" || failed.NotifyKind != notifyKindIterationPaused || failed.Reason != "post failed: 500" {
		t.Errorf("failed event = %#v", failed)
	}
}
