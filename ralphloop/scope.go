package ralphloop

import (
	"fmt"
	"sync"

	"github.com/elentok/gx/tickets"
)

// RunScope keeps the caller's requested membership stable across epic reloads.
// An empty request deliberately remains dynamic so whole-epic runs keep seeing
// tickets added while the run is active.
//
// A frozen scope's ticket set lives behind data, a pointer shared by every
// copy of the RunScope value: RunScope is passed and returned by value
// throughout this package, so the mutex must live behind a pointer too, or
// each copy would guard its own map instead of the one everyone reads.
type RunScope struct {
	wholeEpic bool
	data      *scopeData
}

type scopeData struct {
	mu        sync.RWMutex
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
	return RunScope{data: &scopeData{ticketIDs: ticketIDs}}, nil
}

// Add widens a frozen scope to also include ticketIDs, making each one
// eligible for the next claim exactly as if it had been in the original
// TicketIDs list at launch. Safe to call concurrently with Contains/Frontier
// reads (and with other Add calls) on the same scope.
//
// A no-op on a dynamic (whole-epic) scope: that scope is already
// unrestricted, so there's nothing to widen.
func (s RunScope) Add(ticketIDs ...string) {
	if s.wholeEpic || s.data == nil {
		return
	}
	s.data.mu.Lock()
	defer s.data.mu.Unlock()
	for _, id := range ticketIDs {
		s.data.ticketIDs[id] = struct{}{}
	}
}

// containsID reports whether id is directly in the frozen scope's ticket set.
func (s RunScope) containsID(id string) bool {
	if s.data == nil {
		return false
	}
	s.data.mu.RLock()
	defer s.data.mu.RUnlock()
	_, ok := s.data.ticketIDs[id]
	return ok
}

// requestedCount is how many ticket IDs are directly in the frozen scope's set.
func (s RunScope) requestedCount() int {
	if s.data == nil {
		return 0
	}
	s.data.mu.RLock()
	defer s.data.mu.RUnlock()
	return len(s.data.ticketIDs)
}

// Contains reports whether ticket is in scope: either directly requested, or
// reached from a requested ticket by walking one or more Parent hops (a
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

	if s.containsID(id) {
		return true
	}
	if ticket.Parent == nil {
		return false
	}
	parent, ok := findTicketByID(epic, *ticket.Parent)
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

// AllDone reports whether every originally-requested ticket is done. It
// counts against foundRequested (only tickets from the original request),
// not the scope's full membership - dynamically discovered Parent
// descendants are also required to be done below, but their presence must
// not trip the requested-count sanity check.
func (s RunScope) AllDone(epic tickets.Epic) bool {
	if s.wholeEpic {
		return allDone(epic)
	}

	foundRequested := 0
	for _, ticket := range epic.Tickets {
		if !s.Contains(ticket, epic) {
			continue
		}
		if s.containsID(ticket.DisplayNumber()) {
			foundRequested++
		}
		if epic.RenderedStatus(ticket) != tickets.StatusDone {
			return false
		}
	}
	return foundRequested == s.requestedCount()
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
