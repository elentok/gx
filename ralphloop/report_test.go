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

func writeFakeCodexSession(t *testing.T, cwd, sessionID string, start time.Time) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	path := filepath.Join(home, ".codex", "sessions", "2026", "01", "01", "rollout-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	contents := `{"timestamp":"` + start.UTC().Format(time.RFC3339Nano) + `","type":"session_meta","payload":{"id":"` + sessionID + `","cwd":"` + cwd + `"}}
{"timestamp":"` + start.Add(time.Second).UTC().Format(time.RFC3339Nano) + `","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":6000,"cached_input_tokens":1000,"output_tokens":400,"reasoning_output_tokens":100,"total_tokens":6400},"last_token_usage":{"input_tokens":3000}}}}
{"timestamp":"` + start.Add(3*time.Second).UTC().Format(time.RFC3339Nano) + `","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10000,"cached_input_tokens":2500,"output_tokens":500,"reasoning_output_tokens":150,"total_tokens":10500},"last_token_usage":{"input_tokens":4500}}}}
{"timestamp":"` + start.Add(4*time.Second).UTC().Format(time.RFC3339Nano) + `","type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":50,"resets_at":1786170140}}}}
`
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
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

	if err := logEvent(scratchDir, "epic", Event{Time: start, Type: eventIterationStarted, Ticket: "01", AgentSession: "sess-1", Cwd: cwd}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(scratchDir, "epic", Event{Time: start.Add(2 * time.Second), Type: eventIterationFinished, Ticket: "01", AgentSession: "sess-1", Cwd: cwd}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(scratchDir, "epic", Event{Time: start.Add(2 * time.Second), Type: eventCherryPicked, Ticket: "01"}); err != nil {
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

func TestReport_DepsInstalledEvent_PrintsCommand(t *testing.T) {
	scratchDir := t.TempDir()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := logEvent(scratchDir, "epic", Event{Time: start, Type: eventDepsInstalled, Ticket: "01", Cwd: "/fake/iter-01", Reason: "npm ci"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(scratchDir, "epic", Event{Time: start.Add(time.Second), Type: eventCherryPicked, Ticket: "01"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}

	var out bytes.Buffer
	if err := Report(ReportOptions{EpicName: "epic", ScratchDir: scratchDir}, &out); err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	text := out.String()

	if !strings.Contains(text, "deps: npm ci") {
		t.Errorf("output = %q, want a deps: npm ci line for ticket 1", text)
	}
}

func TestReport_NoDepsInstalledEvent_OmitsDepsLine(t *testing.T) {
	scratchDir := t.TempDir()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := logEvent(scratchDir, "epic", Event{Time: start, Type: eventCherryPicked, Ticket: "01"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}

	var out bytes.Buffer
	if err := Report(ReportOptions{EpicName: "epic", ScratchDir: scratchDir}, &out); err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	if strings.Contains(out.String(), "deps:") {
		t.Errorf("output = %q, want no deps: line when no deps-installed event was logged", out.String())
	}
}

func TestReport_CodexSessionPrintsDurationPeakContextTokensAndNoCost(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	scratchDir := t.TempDir()
	cwd := "/fake/iter-01"
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeFakeCodexSession(t, cwd, "codex-1", start)

	if err := logEvent(scratchDir, "epic", Event{Time: start, Type: eventIterationStarted, Ticket: "01", Agent: AgentCodex, AgentSession: "codex-1", Cwd: cwd}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(scratchDir, "epic", Event{Time: start.Add(4 * time.Second), Type: eventIterationFinished, Ticket: "01", Agent: AgentCodex, AgentSession: "codex-1", Cwd: cwd}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}

	var out bytes.Buffer
	if err := Report(ReportOptions{EpicName: "epic", ScratchDir: scratchDir}, &out); err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	text := out.String()
	for _, want := range []string{"duration=4s", "peak-context=4500", "tokens=10500", "cost=n/a"} {
		if !strings.Contains(text, want) {
			t.Errorf("output = %q, want %q", text, want)
		}
	}
}

func TestReport_CodexSessionMissingStillPrintsUnknownMetricsAndNoCost(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	scratchDir := t.TempDir()
	if err := logEvent(scratchDir, "epic", Event{Type: eventIterationStarted, Ticket: "01", Agent: AgentCodex, AgentSession: "not-local", Cwd: "/fake/iter-01"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}

	var out bytes.Buffer
	if err := Report(ReportOptions{EpicName: "epic", ScratchDir: scratchDir}, &out); err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	text := out.String()
	for _, want := range []string{"duration=unknown", "peak-context=unknown", "tokens=unknown", "cost=n/a"} {
		if !strings.Contains(text, want) {
			t.Errorf("output = %q, want %q", text, want)
		}
	}
}

func TestReport_MixedAgentsPreservesOrderConcurrencyAndClaudeCost(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	scratchDir := t.TempDir()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeFakeCodexSession(t, "/fake/iter-02", "codex-2", start)
	writeFakeTranscript(t, "/fake/iter-01", "claude-1", start.Add(time.Second),
		[3]any{"claude-sonnet-5", 1000, 0},
		[3]any{"claude-sonnet-5", 2000, 5000},
	)

	events := []Event{
		// Deliberately append the later event first: concurrent writers may
		// acquire the run-log lock in a different order than their timestamps.
		{Time: start.Add(time.Second), Type: eventIterationStarted, Ticket: "01", Agent: AgentClaude, AgentSession: "claude-1", Cwd: "/fake/iter-01"},
		{Time: start, Type: eventIterationStarted, Ticket: "02", Agent: AgentCodex, AgentSession: "codex-2", Cwd: "/fake/iter-02"},
		{Time: start.Add(4 * time.Second), Type: eventIterationFinished, Ticket: "02", Agent: AgentCodex, AgentSession: "codex-2", Cwd: "/fake/iter-02"},
		{Time: start.Add(5 * time.Second), Type: eventIterationFinished, Ticket: "01", Agent: AgentClaude, AgentSession: "claude-1", Cwd: "/fake/iter-01"},
	}
	for _, event := range events {
		if err := logEvent(scratchDir, "epic", event); err != nil {
			t.Fatalf("logEvent: %v", err)
		}
	}

	var out bytes.Buffer
	if err := Report(ReportOptions{EpicName: "epic", ScratchDir: scratchDir}, &out); err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "Task order:\n  02\n  01") {
		t.Errorf("output = %q, want Codex ticket 02 before Claude ticket 01", text)
	}
	if !strings.Contains(text, "02 + 01") {
		t.Errorf("output = %q, want mixed-agent concurrency group", text)
	}
	var claudeRow, codexRow string
	for line := range strings.SplitSeq(text, "\n") {
		if strings.Contains(line, "duration=") && strings.HasPrefix(strings.TrimSpace(line), "01") {
			claudeRow = line
		}
		if strings.Contains(line, "duration=") && strings.HasPrefix(strings.TrimSpace(line), "02") {
			codexRow = line
		}
	}
	if !strings.Contains(claudeRow, "cost=$0.0108") {
		t.Errorf("Claude row = %q, want existing Claude cost", claudeRow)
	}
	if !strings.Contains(codexRow, "tokens=10500") || !strings.Contains(codexRow, "cost=n/a") {
		t.Errorf("Codex row = %q, want Codex tokens and n/a cost", codexRow)
	}
	if !strings.Contains(text, "Total: duration=5s peak-context=7000 tokens=10500 cost=$0.0108 + n/a") {
		t.Errorf("output = %q, want mixed epic totals", text)
	}
}

func TestReport_OverlappingTickets_GroupedAsConcurrent(t *testing.T) {
	scratchDir := t.TempDir()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Ticket 1: [0s, 10s]; Ticket 2: [5s, 15s] — overlaps ticket 1.
	if err := logEvent(scratchDir, "epic", Event{Time: base, Type: eventIterationStarted, Ticket: "01"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(scratchDir, "epic", Event{Time: base.Add(5 * time.Second), Type: eventIterationStarted, Ticket: "02"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(scratchDir, "epic", Event{Time: base.Add(10 * time.Second), Type: eventIterationFinished, Ticket: "01"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(scratchDir, "epic", Event{Time: base.Add(15 * time.Second), Type: eventIterationFinished, Ticket: "02"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	// Ticket 3 runs entirely after both, no overlap.
	if err := logEvent(scratchDir, "epic", Event{Time: base.Add(20 * time.Second), Type: eventIterationStarted, Ticket: "03"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(scratchDir, "epic", Event{Time: base.Add(25 * time.Second), Type: eventIterationFinished, Ticket: "03"}); err != nil {
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

	if err := logEvent(scratchDir, "epic", Event{Type: eventIterationStarted, Ticket: "01", AgentSession: "sess-gone", Cwd: "/fake/iter-01"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	if err := logEvent(scratchDir, "epic", Event{Type: eventIterationFinished, Ticket: "01", AgentSession: "sess-gone", Cwd: "/fake/iter-01"}); err != nil {
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
		"01-first-ticket.md": "---\nid: \"01\"\nstatus: done\ntype: task\n---\n# First ticket\n",
	})
	if err := logEvent(scratchDir, "epic", Event{Type: eventIterationStarted, Ticket: "01"}); err != nil {
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
