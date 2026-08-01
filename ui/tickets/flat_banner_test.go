package tickets

import (
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
)

// TestEpicStatusText covers the persistent epic-completion indicator: hidden
// before the epic has loaded, hidden for a zero-ticket epic (still being
// scaffolded, not "all done" — see tickets.Epic.AllDone), "N/M done" while
// open tickets remain, and "Epic complete" once every ticket is done.
func TestEpicStatusText(t *testing.T) {
	epicInProgress := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 2, Identifier: "02", Status: "open"},
	}}
	epicDone := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 2, Identifier: "02", Status: "resolved"},
	}}

	cases := []struct {
		name   string
		model  FlatModel
		want   string // "" for empty, otherwise a substring the text must contain
		wantIt bool
	}{
		{"not loaded yet", FlatModel{loaded: false, found: true, epic: epicInProgress}, "", false},
		{"epic not found", FlatModel{loaded: true, found: false, epic: epicInProgress}, "", false},
		{"zero-ticket epic", FlatModel{loaded: true, found: true, epic: tickets.Epic{}}, "", false},
		{"in progress", FlatModel{loaded: true, found: true, epic: epicInProgress}, "1/2", true},
		{"all done", FlatModel{loaded: true, found: true, epic: epicDone}, "Epic complete", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.model.epicStatusText()
			if !c.wantIt {
				if got != "" {
					t.Fatalf("epicStatusText() = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, c.want) {
				t.Fatalf("epicStatusText() = %q, want substring %q", got, c.want)
			}
		})
	}
}

// TestFooterView_IncludesEpicStatus guards the ticket 04 requirement that the
// epic indicator stays visible regardless of scroll position: it belongs on
// the always-rendered footer line, not just an in-list epic row.
func TestFooterView_IncludesEpicStatus(t *testing.T) {
	m := FlatModel{
		loaded: true,
		found:  true,
		epic: tickets.Epic{Tickets: []tickets.Ticket{
			{Number: 1, Identifier: "01", Status: "open"},
		}},
	}

	got := m.footerView()
	if !strings.Contains(got, "0/1") {
		t.Fatalf("footerView() = %q, want it to contain the epic status", got)
	}
	if !strings.Contains(got, "help") {
		t.Fatalf("footerView() = %q, want it to still contain the help hint", got)
	}
}
