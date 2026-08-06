package ralphloop

import (
	"fmt"

	"github.com/elentok/gx/tickets"
)

// RunScope keeps the caller's requested membership stable across epic reloads.
// An empty request deliberately remains dynamic so whole-epic runs keep seeing
// tickets added while the run is active.
type RunScope struct {
	wholeEpic bool
	ticketIDs map[string]struct{}
}

func ResolveRunScope(epic tickets.Epic, requestedIDs []string) (RunScope, error) {
	if len(requestedIDs) == 0 {
		return RunScope{wholeEpic: true}, nil
	}

	available := make(map[string]struct{}, len(epic.Tickets))
	for _, ticket := range epic.Tickets {
		available[ticket.DisplayNumber()] = struct{}{}
	}

	ticketIDs := make(map[string]struct{}, len(requestedIDs))
	for _, id := range requestedIDs {
		if _, ok := ticketIDs[id]; ok {
			return RunScope{}, fmt.Errorf("resolving run scope for epic %q: ticket identifier %q requested more than once", epic.Name, id)
		}
		if _, ok := available[id]; !ok {
			return RunScope{}, fmt.Errorf("resolving run scope for epic %q: ticket identifier %q not found", epic.Name, id)
		}
		ticketIDs[id] = struct{}{}
	}
	return RunScope{ticketIDs: ticketIDs}, nil
}

// Contains reports whether ticket is in scope: either directly requested, or
// reached from a requested ticket by walking one or more SplitFrom hops (a
// mid-run split's descendant is in scope dynamically, without requiring the
// original request to have named it upfront).
func (s RunScope) Contains(ticket tickets.Ticket, epic tickets.Epic) bool {
	if s.wholeEpic {
		return true
	}
	return s.containsChain(ticket, epic, make(map[string]bool))
}

func (s RunScope) containsChain(ticket tickets.Ticket, epic tickets.Epic, visited map[string]bool) bool {
	id := ticket.DisplayNumber()
	if visited[id] {
		return false
	}
	visited[id] = true

	if _, ok := s.ticketIDs[id]; ok {
		return true
	}
	if ticket.SplitFrom == nil {
		return false
	}
	parent, ok := findTicketByID(epic, *ticket.SplitFrom)
	if !ok {
		return false
	}
	return s.containsChain(parent, epic, visited)
}

func findTicketByID(epic tickets.Epic, id string) (tickets.Ticket, bool) {
	for _, ticket := range epic.Tickets {
		if ticket.DisplayNumber() == id {
			return ticket, true
		}
	}
	return tickets.Ticket{}, false
}

// AllSettled reports whether every originally-requested ticket has reached a
// terminal status. It counts against foundRequested (only tickets from the
// original request), not the scope's full membership - dynamically
// discovered SplitFrom descendants are also required to be settled below,
// but their presence must not trip the requested-count sanity check.
func (s RunScope) AllSettled(epic tickets.Epic) bool {
	if s.wholeEpic {
		return allSettled(epic)
	}

	foundRequested := 0
	for _, ticket := range epic.Tickets {
		if !s.Contains(ticket, epic) {
			continue
		}
		if _, requested := s.ticketIDs[ticket.DisplayNumber()]; requested {
			foundRequested++
		}
		if !isSettledStatus(epic.RenderedStatus(ticket)) {
			return false
		}
	}
	return foundRequested == len(s.ticketIDs)
}

// TotalCount is how many of epic's tickets belong to the scope.
func (s RunScope) TotalCount(epic tickets.Epic) int {
	if s.wholeEpic {
		return epic.TotalCount()
	}
	total := 0
	for _, ticket := range epic.Tickets {
		if s.Contains(ticket, epic) {
			total++
		}
	}
	return total
}

// DoneCount is how many of the scope's tickets are done.
func (s RunScope) DoneCount(epic tickets.Epic) int {
	if s.wholeEpic {
		return epic.DoneCount()
	}
	done := 0
	for _, ticket := range epic.Tickets {
		if s.Contains(ticket, epic) && ticket.IsDone() {
			done++
		}
	}
	return done
}

// Frontier keeps dependency resolution epic-wide: selecting a dependent
// ticket does not silently waive an unselected blocker.
func (s RunScope) Frontier(epic tickets.Epic) []tickets.Ticket {
	frontier := Frontier(epic)
	if s.wholeEpic {
		return frontier
	}

	filtered := make([]tickets.Ticket, 0, len(frontier))
	for _, ticket := range frontier {
		if s.Contains(ticket, epic) {
			filtered = append(filtered, ticket)
		}
	}
	return filtered
}
