package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
)

const logQuestionLine = `{"seq":1,"ts":"2026-01-01T00:00:00Z","type":"pause.question","by":"agent","handoff":"h","summary":"Which one?"}` + "\n"

func writeLogFile(t *testing.T, ticketPath, content string) {
	t.Helper()
	if err := os.WriteFile(tickets.LogPath(ticketPath), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExecute_TicketsValidate_LogTornLastLinePasses(t *testing.T) {
	t.Parallel()
	path := writeEpicTicket(t, t.TempDir(), "e", "01-a.md", ticketFrontmatter("01", ""))
	writeLogFile(t, path, logQuestionLine+`{"seq":2,"ts":`)

	if stderr, err := validateStderr(t, path); err != nil || stderr != "" {
		t.Errorf("validate = %v, stderr %q, want clean pass", err, stderr)
	}
}

func TestExecute_TicketsValidate_LogMalformedMiddleLineFails(t *testing.T) {
	t.Parallel()
	path := writeEpicTicket(t, t.TempDir(), "e", "01-a.md", ticketFrontmatter("01", ""))
	writeLogFile(t, path, logQuestionLine+"not json\n"+logQuestionLine)

	_, err := validateStderr(t, path)
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error = %v, want an error naming line 2", err)
	}
}

func TestExecute_TicketsValidate_AnswerPauseWithoutOpenQuestionFails(t *testing.T) {
	t.Parallel()
	path := writeEpicTicket(t, t.TempDir(), "e", "01-a.md", strings.Replace(ticketFrontmatter("01", ""), "status: open", "status: paused\npause_reason: self-reported", 1))

	_, err := validateStderr(t, path)
	if err == nil || !strings.Contains(err.Error(), "no open question") {
		t.Errorf("error = %v, want no-open-question error", err)
	}

	closed := logQuestionLine + `{"seq":2,"ts":"2026-01-01T00:00:01Z","type":"pause.answer","question":1}` + "\n"
	writeLogFile(t, path, closed)
	if _, err := validateStderr(t, path); err == nil {
		t.Error("answered question: validate passed, want error")
	}
}

func TestExecute_TicketsValidate_AnswerPauseWithOpenQuestionPasses(t *testing.T) {
	t.Parallel()
	path := writeEpicTicket(t, t.TempDir(), "e", "01-a.md", strings.Replace(ticketFrontmatter("01", ""), "status: open", "status: paused\npause_reason: self-reported", 1))
	writeLogFile(t, path, logQuestionLine)

	if stderr, err := validateStderr(t, path); err != nil || stderr != "" {
		t.Errorf("validate = %v, stderr %q, want clean pass", err, stderr)
	}
}

func TestExecute_TicketsValidate_LongQuestionSummaryWarns(t *testing.T) {
	t.Parallel()
	path := writeEpicTicket(t, t.TempDir(), "e", "01-a.md", ticketFrontmatter("01", ""))
	long := strings.Repeat("x", tickets.MaxLogSummaryChars+1)
	writeLogFile(t, path, `{"seq":1,"ts":"2026-01-01T00:00:00Z","type":"pause.question","by":"agent","handoff":"h","summary":"`+long+`"}`+"\n")

	stderr, err := validateStderr(t, path)
	if err != nil {
		t.Fatalf("validate = %v, want pass with warning", err)
	}
	if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, "121 characters") {
		t.Errorf("stderr = %q, want a summary-length warning", stderr)
	}
}
