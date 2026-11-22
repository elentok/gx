package ticketsflow

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func writeRunLog(t *testing.T, epicDir string, lines []string) {
	t.Helper()
	testutil.Mkdir(t, epicDir)
	path := filepath.Join(epicDir, "run-log.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("writing run-log.jsonl: %v", err)
	}
}

// filterRunLog runs runTicketsFilterRunLog over a run log holding lines.
func filterRunLog(t *testing.T, lines []string, ticket string, events []string, scans bool) string {
	t.Helper()
	epicDir := filepath.Join(testutil.TempRepo(t), ".scratch", "widget-epic")
	writeRunLog(t, epicDir, lines)
	var buf bytes.Buffer
	if err := runTicketsFilterRunLog(epicDir, ticket, events, scans, &buf); err != nil {
		t.Fatalf("runTicketsFilterRunLog: %v", err)
	}
	return buf.String()
}

func TestIsSchedulerScan(t *testing.T) {
	t.Parallel()
	if !isSchedulerScan("scheduler-scan") {
		t.Error("scheduler-scan: want true, so paused and filter-run-log hide it")
	}
	if isSchedulerScan("iteration-started") {
		t.Error("iteration-started: want false, so it is kept")
	}
}

func TestRunTicketsFilterRunLog_FiltersByTicket(t *testing.T) {
	t.Parallel()
	out := filterRunLog(t, []string{
		`{"time":"2026-01-01T00:00:00Z","type":"iteration-started","ticket":"01"}`,
		`{"time":"2026-01-01T00:01:00Z","type":"iteration-finished","ticket":"02"}`,
		`{"time":"2026-01-01T00:02:00Z","type":"iteration-started","ticket":"01"}`,
	}, "01", nil, false)

	if strings.Count(out, `"ticket": "01"`) != 2 {
		t.Errorf("output = %s, want exactly 2 events for ticket 01", out)
	}
	if strings.Contains(out, `"ticket": "02"`) {
		t.Errorf("output = %s, should not contain ticket 02", out)
	}
	firstIdx := strings.Index(out, `"iteration-started"`)
	secondIdx := strings.Index(out, `"time": "2026-01-01T00:02:00Z"`)
	if firstIdx == -1 || secondIdx == -1 || firstIdx > secondIdx {
		t.Errorf("events not printed in file order: %s", out)
	}
}

func TestRunTicketsFilterRunLog_FiltersByEvent(t *testing.T) {
	t.Parallel()
	out := filterRunLog(t, []string{
		`{"time":"2026-01-01T00:00:00Z","type":"iteration-started","ticket":"01"}`,
		`{"time":"2026-01-01T00:01:00Z","type":"paused","ticket":"01"}`,
		`{"time":"2026-01-01T00:02:00Z","type":"scheduler-scan","ticket":""}`,
	}, "", []string{"paused", "scheduler-scan"}, false)

	if strings.Contains(out, "iteration-started") {
		t.Errorf("output = %s, should not contain iteration-started", out)
	}
	if !strings.Contains(out, "paused") || !strings.Contains(out, "scheduler-scan") {
		t.Errorf("output = %s, want both paused and scheduler-scan events", out)
	}
}

func TestRunTicketsFilterRunLog_HidesSchedulerScansUnlessAsked(t *testing.T) {
	t.Parallel()
	lines := []string{
		`{"time":"2026-01-01T00:01:00Z","type":"paused","ticket":"01"}`,
		`{"time":"2026-01-01T00:02:00Z","type":"scheduler-scan","ticket":""}`,
	}
	if hidden := filterRunLog(t, lines, "", nil, false); !strings.Contains(hidden, "paused") || strings.Contains(hidden, "scheduler-scan") {
		t.Errorf("default output = %s, want paused only", hidden)
	}
	if shown := filterRunLog(t, lines, "", nil, true); !strings.Contains(shown, "paused") || !strings.Contains(shown, "scheduler-scan") {
		t.Errorf("--scans output = %s, want both events", shown)
	}
}

func TestRunTicketsFilterRunLog_CombinesTicketAndEventFilters(t *testing.T) {
	t.Parallel()
	out := filterRunLog(t, []string{
		`{"time":"2026-01-01T00:00:00Z","type":"iteration-started","ticket":"01"}`,
		`{"time":"2026-01-01T00:01:00Z","type":"paused","ticket":"01"}`,
		`{"time":"2026-01-01T00:02:00Z","type":"paused","ticket":"02"}`,
	}, "01", []string{"paused"}, false)

	if strings.Count(out, `"type": "paused"`) != 1 || !strings.Contains(out, `"ticket": "01"`) {
		t.Errorf("output = %s, want exactly one paused event for ticket 01", out)
	}
}

func TestRunTicketsFilterRunLog_NoRunLogYet(t *testing.T) {
	t.Parallel()
	epicDir := filepath.Join(testutil.TempRepo(t), ".scratch", "widget-epic")
	testutil.Mkdir(t, epicDir)

	var buf bytes.Buffer
	if err := runTicketsFilterRunLog(epicDir, "", nil, false, &buf); err != nil {
		t.Fatalf("runTicketsFilterRunLog: %v", err)
	}

	if !strings.Contains(buf.String(), "no run-log.jsonl yet") {
		t.Errorf("output = %q, want a clear no-run-log message", buf.String())
	}
}

func TestRunTicketsFilterRunLog_MissingEpicErrors(t *testing.T) {
	t.Parallel()
	epicDir := filepath.Join(testutil.TempRepo(t), ".scratch", "no-such-epic")

	if err := runTicketsFilterRunLog(epicDir, "", nil, false, &bytes.Buffer{}); err == nil {
		t.Fatal("runTicketsFilterRunLog: want error for missing epic, got nil")
	}
}
