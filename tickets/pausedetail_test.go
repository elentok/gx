package tickets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/tickets/schema"
)

func pausedTicket(t *testing.T, body string) (Epic, Ticket) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "01-a.md")
	if err := os.WriteFile(path, []byte("---\nid: \"01\"\nstatus: paused\npause_reason: self-reported\ntype: implement\n---\n"+body), 0o644); err != nil {
		t.Fatal(err)
	}
	tk := Ticket{Path: path, Status: "paused", PauseReason: schema.PauseSelfReported, Body: body}
	return Epic{Tickets: []Ticket{tk}}, tk
}

// Seam 7: a ticket paused for an answer shows the open question's summary.
func TestPauseDetail_AnswerClassReadsLogSummary(t *testing.T) {
	t.Parallel()
	e, tk := pausedTicket(t, "# A\n")
	q := LogEvent{Type: EventPauseQuestion, By: "gx", Summary: "Adopt it as **done**?", Text: "Adopt it as done?"}
	if _, err := AppendLog(tk.Path, q); err != nil {
		t.Fatal(err)
	}
	if got, want := PauseDetail(e, tk), "Adopt it as done?"; got != want {
		t.Errorf("PauseDetail = %q, want %q", got, want)
	}
}

// An old ticket that still carries the section, with no log, keeps displaying.
func TestPauseDetail_AnswerClassFallsBackToOldSection(t *testing.T) {
	t.Parallel()
	e, tk := pausedTicket(t, "# A\n\n"+LegacyAnswerHeading+"\n\nOld question?\n")
	if got, want := PauseDetail(e, tk), "Old question?"; got != want {
		t.Errorf("PauseDetail = %q, want %q", got, want)
	}
}
