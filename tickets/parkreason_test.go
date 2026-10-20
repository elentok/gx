package tickets

import (
	"os"
	"path/filepath"
	"testing"
)

func parkedTicket(t *testing.T, body string) (Epic, Ticket) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "01-a.md")
	if err := os.WriteFile(path, []byte("---\nid: \"01\"\nstatus: needs-answer\ntype: implement\n---\n"+body), 0o644); err != nil {
		t.Fatal(err)
	}
	tk := Ticket{Path: path, Status: "needs-answer", Body: body}
	return Epic{Tickets: []Ticket{tk}}, tk
}

// Seam 7: a needs-answer ticket's park reason is the open question's summary.
func TestParkReason_NeedsAnswerReadsLogSummary(t *testing.T) {
	t.Parallel()
	e, tk := parkedTicket(t, "# A\n")
	q := LogEvent{Type: EventParkQuestion, By: "gx", Summary: "Adopt it as **done**?", Text: "Adopt it as done?"}
	if _, err := AppendLog(tk.Path, q); err != nil {
		t.Fatal(err)
	}
	if got, want := ParkReason(e, tk), "Adopt it as done?"; got != want {
		t.Errorf("ParkReason = %q, want %q", got, want)
	}
}

// An old ticket that still carries the section, with no log, keeps displaying.
func TestParkReason_NeedsAnswerFallsBackToOldSection(t *testing.T) {
	t.Parallel()
	e, tk := parkedTicket(t, "# A\n\n## Needs Answer\n\nOld question?\n")
	if got, want := ParkReason(e, tk), "Old question?"; got != want {
		t.Errorf("ParkReason = %q, want %q", got, want)
	}
}
