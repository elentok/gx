package ralphloop

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/transcript"
)

// writeFakeTranscript writes a session transcript under a fake
// ~/.claude/projects/<slug>/<sessionID>.jsonl (HOME must already be set via
// t.Setenv by the caller), with one assistant line per (model, inputTokens,
// cacheReadTokens) entry, one second apart starting at start.
func writeFakeTranscript(t *testing.T, cwd, sessionID string, start time.Time, turns ...[3]any) {
	t.Helper()
	path, err := transcript.Path(cwd, sessionID)
	if err != nil {
		t.Fatalf("transcript.Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	var lines []string
	for i, turn := range turns {
		ts := start.Add(time.Duration(i) * time.Second).UTC().Format(time.RFC3339Nano)
		model := turn[0].(string)
		input := turn[1].(int)
		cacheRead := turn[2].(int)
		lines = append(lines, `{"type":"assistant","timestamp":"`+ts+`","message":{"model":"`+model+`","usage":{"input_tokens":`+strconv.Itoa(input)+`,"cache_read_input_tokens":`+strconv.Itoa(cacheRead)+`,"output_tokens":10}}}`)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestReport_NoRunLog_PrintsNoOpMessage(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if err := Report(ReportOptions{EpicName: "epic", ScratchDir: dir}, &out); err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	if !strings.Contains(out.String(), "no run-log events") {
		t.Errorf("output = %q, want a no-run-log message", out.String())
	}
}

func TestReport_SingleTicket_PrintsOrderAndCost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	scratchDir := t.TempDir()

	cwd := "/fake/iter-01"
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeFakeTranscript(t, cwd, "sess-1", start,
		[3]any{"claude-sonnet-5", 1000, 0},
		[3]any{"claude-sonnet-5", 2000, 5000},
	)

	if err := logEvent(scratchDir, "epic", Event{Time: start, Type: eventIterationStarted, Ticket: 1, AgentSession: "sess-1", Cwd: cwd}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(scratchDir, "epic", Event{Time: start.Add(2 * time.Second), Type: eventIterationFinished, Ticket: 1, AgentSession: "sess-1", Cwd: cwd}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(scratchDir, "epic", Event{Time: start.Add(2 * time.Second), Type: eventCherryPicked, Ticket: 1}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}

	var out bytes.Buffer
	if err := Report(ReportOptions{EpicName: "epic", ScratchDir: scratchDir}, &out); err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	text := out.String()

	if !strings.Contains(text, "Task order:") || !strings.Contains(text, "01") {
		t.Errorf("output = %q, want a task-order section mentioning ticket 01", text)
	}
	if !strings.Contains(text, "peak-context=7000") {
		t.Errorf("output = %q, want peak-context=7000 (the second turn's input+cache-read=2000+5000, not summed across turns)", text)
	}
	// cost: turn1 = 1000/1e6*3 + 10/1e6*15 = 0.003+0.00015 = 0.00315
	//       turn2 = 2000/1e6*3 + 5000/1e6*0.3 + 10/1e6*15 = 0.006+0.0015+0.00015 = 0.00765
	//       total = 0.0108
	if !strings.Contains(text, "cost=$0.0108") {
		t.Errorf("output = %q, want the summed-across-turns Sonnet-tier cost", text)
	}
	if !strings.Contains(text, "duration=2s") {
		t.Errorf("output = %q, want duration=2s (first-to-last transcript timestamp)", text)
	}
	if !strings.Contains(text, "Total:") {
		t.Errorf("output = %q, want a Total: summary line", text)
	}
}

func TestReport_OverlappingTickets_GroupedAsConcurrent(t *testing.T) {
	scratchDir := t.TempDir()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Ticket 1: [0s, 10s]; Ticket 2: [5s, 15s] — overlaps ticket 1.
	if err := logEvent(scratchDir, "epic", Event{Time: base, Type: eventIterationStarted, Ticket: 1}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(scratchDir, "epic", Event{Time: base.Add(5 * time.Second), Type: eventIterationStarted, Ticket: 2}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(scratchDir, "epic", Event{Time: base.Add(10 * time.Second), Type: eventIterationFinished, Ticket: 1}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(scratchDir, "epic", Event{Time: base.Add(15 * time.Second), Type: eventIterationFinished, Ticket: 2}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	// Ticket 3 runs entirely after both, no overlap.
	if err := logEvent(scratchDir, "epic", Event{Time: base.Add(20 * time.Second), Type: eventIterationStarted, Ticket: 3}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(scratchDir, "epic", Event{Time: base.Add(25 * time.Second), Type: eventIterationFinished, Ticket: 3}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}

	var out bytes.Buffer
	if err := Report(ReportOptions{EpicName: "epic", ScratchDir: scratchDir}, &out); err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	text := out.String()

	if !strings.Contains(text, "01 + 02") {
		t.Errorf("output = %q, want tickets 01 and 02 grouped as concurrent", text)
	}
	if !strings.Contains(text, "03 (solo)") {
		t.Errorf("output = %q, want ticket 03 reported solo (no overlap)", text)
	}
}

func TestReport_SessionTranscriptMissing_ShowsUnknownDurationNoError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	scratchDir := t.TempDir()

	if err := logEvent(scratchDir, "epic", Event{Type: eventIterationStarted, Ticket: 1, AgentSession: "sess-gone", Cwd: "/fake/iter-01"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(scratchDir, "epic", Event{Type: eventIterationFinished, Ticket: 1, AgentSession: "sess-gone", Cwd: "/fake/iter-01"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}

	var out bytes.Buffer
	if err := Report(ReportOptions{EpicName: "epic", ScratchDir: scratchDir}, &out); err != nil {
		t.Fatalf("Report() error = %v, want nil even with an unreadable transcript", err)
	}
	if !strings.Contains(out.String(), "duration=unknown") {
		t.Errorf("output = %q, want duration=unknown when the transcript can't be found", out.String())
	}
}

func TestReport_UsesTicketTitlesFromEpicWhenAvailable(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-first-ticket.md": "# First ticket\n\n**Status:** done\n",
	})
	if err := logEvent(scratchDir, "epic", Event{Type: eventIterationStarted, Ticket: 1}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}

	var out bytes.Buffer
	if err := Report(ReportOptions{EpicName: "epic", ScratchDir: scratchDir}, &out); err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	if !strings.Contains(out.String(), "First ticket") {
		t.Errorf("output = %q, want the ticket's title (derived from its filename slug)", out.String())
	}
}
