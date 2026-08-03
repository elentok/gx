// Package ralphloop implements the ticket-scheduling core for `gx
// ralph-loop`: computing which tickets in a to-tickets epic are ready to
// hand to an agent, and claiming/completing them by rewriting their Status:
// line on disk.
package ralphloop

import (
	"sort"

	"github.com/elentok/gx/tickets"
)

// Frontier returns e's open, unblocked, unclaimed tickets, lowest ticket
// number first. It's a thin wrapper over Epic.RenderedStatus: a ticket only
// renders as StatusOpen once its own Status: is unclaimed/missing and every
// Blocked by: number is done, so filtering on that single state gives the
// frontier for free.
func Frontier(e tickets.Epic) []tickets.Ticket {
	var frontier []tickets.Ticket
	for _, t := range e.Tickets {
		if e.RenderedStatus(t) == tickets.StatusOpen {
			frontier = append(frontier, t)
		}
	}
	sort.Slice(frontier, func(i, j int) bool { return frontier[i].Number < frontier[j].Number })
	return frontier
}

// FilterIDs restricts frontier to just the tickets whose DisplayNumber
// appears in ids, preserving frontier's order. An empty ids leaves frontier
// unchanged, so RunOptions.TicketIDs can be threaded straight through without
// a branch at the call site.
func FilterIDs(frontier []tickets.Ticket, ids []string) []tickets.Ticket {
	if len(ids) == 0 {
		return frontier
	}
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	var filtered []tickets.Ticket
	for _, t := range frontier {
		if want[t.DisplayNumber()] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}
