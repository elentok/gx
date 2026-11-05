package ralphloop

import (
	"testing"
	"time"

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
	if allDone(epic) {
		t.Errorf("allDone() = true, want false while ticket 2 paused for repair")
	}
}

func TestAllDone_OpenTicketIsNotComplete(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "done"},
		{Number: 2, Status: "open"},
	}}
	if allDone(epic) {
		t.Errorf("allDone() = true, want false while ticket 2 is still open")
	}
}

func TestAllDone_EveryTicketDone(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "done"},
		{Number: 2, Status: "done"},
	}}
	if !allDone(epic) {
		t.Errorf("allDone() = false, want true when every ticket is done")
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
	if allDone(epic) {
		t.Errorf("allDone() = true, want false: ticket 01 is waiting-for-children")
	}
}

// TestGate_WakePaused_ShortensPauseWait: WakePaused cuts a pause wait short
// instead of it running out a long pause-poll-interval wait.
func TestGate_WakePaused_ShortensPauseWait(t *testing.T) {
	t.Parallel()
	gate := NewGate()
	waiting := make(chan struct{})
	woke := make(chan struct{})

	go func() {
		timer := time.After(time.Hour)
		pauseWake := gate.PauseWake()
		close(waiting)
		select {
		case <-timer:
		case <-pauseWake:
		}
		close(woke)
	}()

	<-waiting
	gate.WakePaused()

	select {
	case <-woke:
	case <-time.After(5 * time.Second):
		t.Fatal("WakePaused() did not interrupt an in-progress pause wait")
	}
}
