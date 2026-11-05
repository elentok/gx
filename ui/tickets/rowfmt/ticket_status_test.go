package rowfmt

import (
	"testing"

	gxtickets "github.com/elentok/gx/tickets"
)

// With no server the Tickets and Archived tabs show the file's own status: the
// blocked overlay is the server's to compute.
func TestTicketStatus_NoServerShowsRawStatus(t *testing.T) {
	blocker := gxtickets.Ticket{Number: 1, Identifier: "01", Path: "p:e/01", Status: "open"}
	blocked := gxtickets.Ticket{Number: 2, Identifier: "02", Path: "p:e/02", Status: "open", BlockedBy: []string{"01"}}
	epic := gxtickets.Epic{Name: "p:e", Tickets: []gxtickets.Ticket{blocker, blocked}}

	if epic.RenderedStatus(blocked) != gxtickets.StatusBlocked {
		t.Fatal("the test needs the computed status to differ from the raw one")
	}
	if got := TicketStatus(epic, blocked); got != gxtickets.StatusOpen {
		t.Errorf("TicketStatus = %v, want open", got)
	}
}
