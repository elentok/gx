package schedule

import (
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// TestAllDone_RepairIsNotComplete: a paused (repair) ticket used to count as
// terminal and end the run, and now must not — the run pauses on it instead.
func TestAllDone_RepairIsNotComplete(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "done"},
		{Number: 2, Status: "paused", PauseReason: schema.PauseIterationError},
	}}
	if AllDone(epic) {
		t.Errorf("AllDone() = true, want false while ticket 2 paused for repair")
	}
}

func TestAllDone_OpenTicketIsNotComplete(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "done"},
		{Number: 2, Status: "open"},
	}}
	if AllDone(epic) {
		t.Errorf("AllDone() = true, want false while ticket 2 is still open")
	}
}

func TestAllDone_EveryTicketDone(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "done"},
		{Number: 2, Status: "done"},
	}}
	if !AllDone(epic) {
		t.Errorf("AllDone() = false, want true when every ticket is done")
	}
}

// TestAllDone_WaitingForChildrenNotDone: a done ticket whose fork subtree
// (Parent) is still open renders as waiting-for-children, which must not count
// as terminal — otherwise an epic would be reported complete with unfinished
// forked work still inside it.
func TestAllDone_WaitingForChildrenNotDone(t *testing.T) {
	t.Parallel()
	parent := "1"
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 1, Identifier: "01a", Status: "open", Parent: &parent},
	}}
	if AllDone(epic) {
		t.Errorf("AllDone() = true, want false: ticket 01 is waiting-for-children")
	}
}
