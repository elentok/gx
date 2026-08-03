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

func (s RunScope) Contains(ticket tickets.Ticket) bool {
	if s.wholeEpic {
		return true
	}
	_, ok := s.ticketIDs[ticket.DisplayNumber()]
	return ok
}

func (s RunScope) AllSettled(epic tickets.Epic) bool {
	if s.wholeEpic {
		return allSettled(epic)
	}

	found := 0
	for _, ticket := range epic.Tickets {
		if !s.Contains(ticket) {
			continue
		}
		found++
		if !isSettledStatus(epic.RenderedStatus(ticket)) {
			return false
		}
	}
	return found == len(s.ticketIDs)
}

// TotalCount is how many of epic's tickets belong to the scope.
func (s RunScope) TotalCount(epic tickets.Epic) int {
	if s.wholeEpic {
		return epic.TotalCount()
	}
	total := 0
	for _, ticket := range epic.Tickets {
		if s.Contains(ticket) {
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
		if s.Contains(ticket) && ticket.IsDone() {
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
		if s.Contains(ticket) {
			filtered = append(filtered, ticket)
		}
	}
	return filtered
}
