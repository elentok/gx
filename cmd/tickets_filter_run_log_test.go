package cmd

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

func TestRunTicketsFilterRunLog_FiltersByTicket(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	epicDir := filepath.Join(repoDir, ".scratch", "widget-epic")
	writeRunLog(t, epicDir, []string{
		`{"time":"2026-01-01T00:00:00Z","type":"iteration-started","ticket":"01"}`,
		`{"time":"2026-01-01T00:01:00Z","type":"iteration-finished","ticket":"02"}`,
		`{"time":"2026-01-01T00:02:00Z","type":"iteration-started","ticket":"01"}`,
	})

	var buf bytes.Buffer
	if err := runTicketsFilterRunLog(epicDir, "01", nil, &buf); err != nil {
		t.Fatalf("runTicketsFilterRunLog: %v", err)
	}

	out := buf.String()
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
	repoDir := testutil.TempRepo(t)
	epicDir := filepath.Join(repoDir, ".scratch", "widget-epic")
	writeRunLog(t, epicDir, []string{
		`{"time":"2026-01-01T00:00:00Z","type":"iteration-started","ticket":"01"}`,
		`{"time":"2026-01-01T00:01:00Z","type":"needs-answer","ticket":"01"}`,
		`{"time":"2026-01-01T00:02:00Z","type":"scheduler-scan","ticket":""}`,
	})

	var buf bytes.Buffer
	if err := runTicketsFilterRunLog(epicDir, "", []string{"needs-answer", "scheduler-scan"}, &buf); err != nil {
		t.Fatalf("runTicketsFilterRunLog: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "iteration-started") {
		t.Errorf("output = %s, should not contain iteration-started", out)
	}
	if !strings.Contains(out, "needs-answer") || !strings.Contains(out, "scheduler-scan") {
		t.Errorf("output = %s, want both needs-answer and scheduler-scan events", out)
	}
}

func TestRunTicketsFilterRunLog_CombinesTicketAndEventFilters(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	epicDir := filepath.Join(repoDir, ".scratch", "widget-epic")
	writeRunLog(t, epicDir, []string{
		`{"time":"2026-01-01T00:00:00Z","type":"iteration-started","ticket":"01"}`,
		`{"time":"2026-01-01T00:01:00Z","type":"needs-answer","ticket":"01"}`,
		`{"time":"2026-01-01T00:02:00Z","type":"needs-answer","ticket":"02"}`,
	})

	var buf bytes.Buffer
	if err := runTicketsFilterRunLog(epicDir, "01", []string{"needs-answer"}, &buf); err != nil {
		t.Fatalf("runTicketsFilterRunLog: %v", err)
	}

	out := buf.String()
	if strings.Count(out, `"type": "needs-answer"`) != 1 || !strings.Contains(out, `"ticket": "01"`) {
		t.Errorf("output = %s, want exactly one needs-answer event for ticket 01", out)
	}
}

func TestRunTicketsFilterRunLog_NoRunLogYet(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	epicDir := filepath.Join(repoDir, ".scratch", "widget-epic")
	testutil.Mkdir(t, epicDir)

	var buf bytes.Buffer
	if err := runTicketsFilterRunLog(epicDir, "", nil, &buf); err != nil {
		t.Fatalf("runTicketsFilterRunLog: %v", err)
	}

	if !strings.Contains(buf.String(), "no run-log.jsonl yet") {
		t.Errorf("output = %q, want a clear no-run-log message", buf.String())
	}
}

func TestRunTicketsFilterRunLog_MissingEpicErrors(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	epicDir := filepath.Join(repoDir, ".scratch", "no-such-epic")

	var buf bytes.Buffer
	err := runTicketsFilterRunLog(epicDir, "", nil, &buf)
	if err == nil {
		t.Fatal("runTicketsFilterRunLog: want error for missing epic, got nil")
	}
}
