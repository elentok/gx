package server

import (
	"testing"

	"github.com/elentok/gx/tickets"
)

func TestHasManualMergeTicket_OnlyAManualMergeTicketCounts(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		ticket tickets.Ticket
		want   bool
	}{
		"manual merge":     {tickets.Ticket{Type: "merge", Manual: true}, true},
		"retry merge":      {tickets.Ticket{Type: "merge"}, false},
		"manual non-merge": {tickets.Ticket{Type: "implement", Manual: true}, false},
	} {
		t.Run(name, func(t *testing.T) {
			e := tickets.Epic{Tickets: []tickets.Ticket{{Type: "implement"}, tc.ticket}}
			if got := hasManualMergeTicket(e); got != tc.want {
				t.Errorf("hasManualMergeTicket = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMergeTicketsThisRun_CountsFromTheLatestManualOneByID(t *testing.T) {
	t.Parallel()
	merge := func(n int, manual bool) tickets.Ticket {
		return tickets.Ticket{Number: n, Type: "merge", Manual: manual}
	}
	work := tickets.Ticket{Number: 1, Type: "implement"}
	for name, tc := range map[string]struct {
		tickets []tickets.Ticket
		want    int
	}{
		"no merge tickets":     {[]tickets.Ticket{work}, 0},
		"none manual":          {[]tickets.Ticket{work, merge(2, false), merge(3, false)}, 2},
		"manual last by ID":    {[]tickets.Ticket{merge(10, true), merge(3, false), merge(4, false)}, 1},
		"manual in the middle": {[]tickets.Ticket{merge(4, false), merge(3, true), merge(10, false), merge(2, false)}, 3},
	} {
		t.Run(name, func(t *testing.T) {
			if got := mergeTicketsThisRun(tickets.Epic{Tickets: tc.tickets}); got != tc.want {
				t.Errorf("mergeTicketsThisRun = %d, want %d", got, tc.want)
			}
		})
	}
}
