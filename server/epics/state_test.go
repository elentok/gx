package epics_test

import (
	"testing"

	"github.com/elentok/gx/server/epics"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func tk(id, status string) tickets.Ticket {
	return tickets.Ticket{Identifier: id, Status: status}
}

func paused(id string) tickets.Ticket {
	t := tk(id, string(schema.StatusPaused))
	t.PauseReason = schema.PauseDirtyTrunk
	return t
}

func blocked(id, on string) tickets.Ticket {
	t := tk(id, string(schema.StatusOpen))
	t.BlockedBy = []string{on}
	return t
}

func epicOf(ts ...tickets.Ticket) tickets.Epic {
	return tickets.Epic{Name: "e", Tickets: ts}
}

func TestStateOf(t *testing.T) {
	mapEpic := epicOf(tk("1", "open"))
	mapEpic.IsMap = true
	tests := []struct {
		name string
		e    tickets.Epic
		want epics.State
	}{
		{"all done", epicOf(tk("1", "done"), tk("2", "cancelled")), epics.StateDone},
		{"no tickets", epicOf(), epics.StateDone},
		{"map epic holds no work", mapEpic, epics.StateDone},
		{"open ticket is runnable", epicOf(tk("1", "done"), tk("2", "open")), epics.StateRunnable},
		{"claimed ticket is runnable", epicOf(tk("1", "claimed")), epics.StateRunnable},
		{"runnable beats paused", epicOf(paused("1"), tk("2", "open")), epics.StateRunnable},
		{"paused ticket", epicOf(tk("1", "done"), paused("2")), epics.StatePaused},
		{"paused next to blocked", epicOf(paused("1"), blocked("2", "9")), epics.StatePaused},
		{"only blocked is a cycle", epicOf(blocked("1", "2"), blocked("2", "1")), epics.StateCycle},
		{"unknown status counts as paused", epicOf(tk("1", "bogus")), epics.StatePaused},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := epics.StateOf(tt.e); got != tt.want {
				t.Errorf("StateOf = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPausedReasonNamesPausedTickets(t *testing.T) {
	e := epicOf(tk("1", "done"), paused("2"))
	root := queue.RootRef{Project: "p", Epic: "e"}
	want := "every remaining ticket is paused: " + root.String() + "/2"
	if got := epics.PausedReason(root, e); got != want {
		t.Errorf("PausedReason = %q, want %q", got, want)
	}
}
