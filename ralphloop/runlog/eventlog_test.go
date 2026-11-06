package runlog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	eventsc "github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/internal/fixture"
)

func TestAppendEvent_AppendsOneJSONLinePerCall(t *testing.T) {
	t.Parallel()
	dir := fixture.EpicScratchDir(t, "epic")

	if err := AppendEvent(dir, "epic", Event{Type: string(eventsc.IterationStarted), Ticket: "01", Pane: "pane-1", AgentSession: "sess-1"}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	if err := AppendEvent(dir, "epic", Event{Type: string(eventsc.IterationFinished), Ticket: "01"}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	raw, err := os.ReadFile(RunLogPath(dir, "epic"))
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
	dir := fixture.EpicScratchDir(t, "epic")

	err := AppendEvent(dir, "epic", Event{
		Type: string(eventsc.ManualMerge), Ticket: "04", Outcome: "merged",
		TrailerValue: "epic/04", AtticRef: "refs/attic/04", Reason: "by hand",
	})
	if err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	if err := AppendEvent(dir, "epic", Event{Type: string(eventsc.TicketStartedOver), Ticket: "05"}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	raw, err := os.ReadFile(RunLogPath(dir, "epic"))
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
	if got.Type != "manual-merge" || got.Outcome != "merged" || got.TrailerValue != "epic/04" || got.AtticRef != "refs/attic/04" {
		t.Errorf("event 0 = %+v, want new fields round-tripped", got)
	}
	if events[1].Type != "ticket-started-over" || strings.Contains(strings.Split(string(raw), "\n")[1], "outcome") {
		t.Errorf("event 1 should omit empty new fields: %q", raw)
	}
}

func TestAppendEvent_LaunchTimedRoundTrips(t *testing.T) {
	t.Parallel()
	dir := fixture.EpicScratchDir(t, "epic")
	claimed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	want := Event{
		Type: string(eventsc.LaunchTimed), Ticket: "04", Path: "resumed", ClaimedAt: claimed, Total: 10 * time.Second,
		WorktreeStep: time.Second, DepsStep: 2 * time.Second, AgentStep: 3 * time.Second, PromptStep: 4 * time.Second,
	}
	if err := AppendEvent(dir, "epic", want); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	got, ok, err := ReadEvents(dir, "epic")
	if err != nil || !ok || len(got) != 1 {
		t.Fatalf("ReadEvents = %v, %v, %v; want 1 event", got, ok, err)
	}
	g := got[0]
	if !g.ClaimedAt.Equal(claimed) || g.Total != want.Total || g.WorktreeStep != time.Second || g.DepsStep != 2*time.Second ||
		g.AgentStep != 3*time.Second || g.PromptStep != 4*time.Second || g.Path != "resumed" {
		t.Errorf("event = %+v, want %+v", g, want)
	}
}

func TestAppendEvent_OversizedReasonIsTruncatedBelowCap(t *testing.T) {
	t.Parallel()
	dir := fixture.EpicScratchDir(t, "epic")

	// Multi-byte and escaped characters exercise the encoded-size accounting.
	reason := strings.Repeat("é\"\n", 5000)
	if err := AppendEvent(dir, "epic", Event{Type: string(eventsc.ManualMerge), Ticket: "04", Reason: reason}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	raw, err := os.ReadFile(RunLogPath(dir, "epic"))
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

func TestAppendEvent_FillsInTimeWhenZero(t *testing.T) {
	t.Parallel()
	dir := fixture.EpicScratchDir(t, "epic")
	before := time.Now()
	if err := AppendEvent(dir, "epic", Event{Type: string(eventsc.Paused), Ticket: "02"}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
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

func TestAppendEvent_EmptyScratchDirOrEpicName_NoOp(t *testing.T) {
	t.Parallel()
	if err := AppendEvent("", "epic", Event{Type: string(eventsc.Paused)}); err != nil {
		t.Errorf("AppendEvent(scratchDir=\"\") error = %v, want nil no-op", err)
	}
	if err := AppendEvent(t.TempDir(), "", Event{Type: string(eventsc.Paused)}); err != nil {
		t.Errorf("AppendEvent(epicName=\"\") error = %v, want nil no-op", err)
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
	path := RunLogPath(dir, "epic")
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

func TestAppendEvent_ConcurrentAppends_NeverInterleave(t *testing.T) {
	t.Parallel()
	dir := fixture.EpicScratchDir(t, "epic")
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = AppendEvent(dir, "epic", Event{Type: string(eventsc.IterationStarted), Ticket: fmt.Sprintf("%02d", n)})
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

func TestAppendEvent_FailureEventWithoutKindIsRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	err := AppendEvent(dir, "epic", Event{Type: string(eventsc.LaunchFailed), Ticket: "01", Reason: "free text only"})
	if err == nil {
		t.Fatal("AppendEvent accepted a launch-failed event without a kind")
	}
	if _, statErr := os.Stat(RunLogPath(dir, "epic")); !os.IsNotExist(statErr) {
		t.Errorf("rejected event still reached the log (stat err %v)", statErr)
	}
}

func TestAppendEvent_EveryTicketEventCarriesCanonicalAddressInEpicLog(t *testing.T) {
	t.Parallel()
	dir := fixture.EpicScratchDir(t, "epic")

	if err := AppendEvent(dir, "epic", Event{Type: string(eventsc.IterationStarted), Ticket: "05"}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	evs, ok, err := ReadEvents(dir, "epic")
	if err != nil || !ok || len(evs) != 1 {
		t.Fatalf("ReadEvents = %v, %v, %v; want one event in the epic's store dir", evs, ok, err)
	}
	if want := fixture.CanonicalAddr(dir, "epic", "05"); evs[0].Address != want {
		t.Errorf("address = %q, want %q", evs[0].Address, want)
	}
}

func TestAppendEvent_FailureEventCarriesAddressAndTruncatedReason(t *testing.T) {
	t.Parallel()
	dir := fixture.EpicScratchDir(t, "epic")

	err := AppendEvent(dir, "epic", Event{
		Type: string(eventsc.LaunchFailed), Kind: string(eventsc.AgentNameTaken), Ticket: "07",
		Attempt: 2, Reason: strings.Repeat("x", 10000),
	})
	if err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	raw, _ := os.ReadFile(RunLogPath(dir, "epic"))
	if len(raw) > eventsc.MaxLineBytes {
		t.Errorf("line is %d bytes, want <= %d", len(raw), eventsc.MaxLineBytes)
	}
	line := string(raw)
	for _, want := range []string{`"kind":"agent_name_taken"`, `"address":"` + fixture.CanonicalAddr(dir, "epic", "07") + `"`, `"attempt":2`} {
		if !strings.Contains(line, want) {
			t.Errorf("line missing %s: %.200s", want, line)
		}
	}
	if strings.Contains(line, `"iteration"`) {
		t.Errorf("unknown iteration must be omitted, not a placeholder: %.200s", line)
	}
}

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
