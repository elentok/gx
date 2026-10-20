package schema

import (
	"strings"
	"testing"
	"time"
)

func TestTicket_CompletedAtRoundTrips(t *testing.T) {
	want := time.Date(2026, 10, 20, 1, 2, 3, 0, time.UTC)
	out, err := MarshalTicket(Ticket{ID: "01", Status: StatusDone, Type: TypePrompt, CompletedAt: want}, "\n# t\n")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseTicketFromRaw(string(out), "01-t.md")
	if err != nil {
		t.Fatal(err)
	}
	if !got.CompletedAt.Equal(want) {
		t.Errorf("CompletedAt = %v, want %v", got.CompletedAt, want)
	}
	out, err = MarshalTicket(Ticket{ID: "01", Status: StatusCancelled, Type: TypePrompt}, "\n# t\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "completed_at") {
		t.Errorf("zero CompletedAt was written:\n%s", out)
	}
}
