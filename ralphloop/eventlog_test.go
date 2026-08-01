package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLogEvent_AppendsOneJSONLinePerCall(t *testing.T) {
	dir := t.TempDir()

	if err := logEvent(dir, "epic", Event{Type: eventIterationStarted, Ticket: 1, Pane: "pane-1", Tab: "tab-1", AgentSession: "sess-1"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(dir, "epic", Event{Type: eventIterationFinished, Ticket: 1}); err != nil {
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
	if err := logEvent(dir, "epic", Event{Type: eventNeedsInfo, Ticket: 2}); err != nil {
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
	content := `{"type":"iteration-started","ticket":1}` + "\n" + `{"type":"iteration-fin` // torn last line
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

func TestLogEvent_ConcurrentAppends_NeverInterleave(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = logEvent(dir, "epic", Event{Type: eventIterationStarted, Ticket: n})
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
